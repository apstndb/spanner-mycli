//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package mycli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instancepb "cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"github.com/apstndb/adcplus"
	"github.com/apstndb/adcplus/tokensource"
	"github.com/apstndb/go-grpcinterceptors/selectlogging"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/gocql/gocql"
	"github.com/samber/lo"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"google.golang.org/grpc"

	"cloud.google.com/go/spanner"
	instanceapi "cloud.google.com/go/spanner/admin/instance/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	selector "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"go.uber.org/zap"

	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// newStoppedIterator creates a stopped RowIterator.
// When a stopped iterator is used (via Next() or Do()), it immediately returns iterator.Done.
// This is the idiomatic way in the Google Cloud Spanner library to handle cases where
// we need to return a non-nil iterator that indicates no results.
//
// Note: We cannot return custom errors through a stopped iterator. The caller will need
// to check for other indicators (like nil transaction) to understand why the query failed.
func newStoppedIterator() *spanner.RowIterator {
	iter := &spanner.RowIterator{}
	iter.Stop()
	return iter
}

var defaultClientConfig = spanner.ClientConfig{
	DisableNativeMetrics: true,
	SessionPoolConfig: spanner.SessionPoolConfig{
		MinOpened: 1,
		MaxOpened: 10, // FIXME: integration_test requires more than a single session
	},
}

var defaultClientOpts = []option.ClientOption{
	option.WithGRPCConnectionPool(1),
}

// Use MEDIUM priority not to disturb regular workloads on the database.
const defaultPriority = sppb.RequestOptions_PRIORITY_MEDIUM

// Transaction state errors
var (
	ErrNoTransaction             = errors.New("no active transaction")
	ErrNotInReadWriteTransaction = errors.New("not in read-write transaction")
	ErrNotInReadOnlyTransaction  = errors.New("not in read-only transaction")
	ErrNotInPendingTransaction   = errors.New("not in pending transaction")
)

// getTimeoutForStatement returns the appropriate timeout for the given statement type
func (s *Session) getTimeoutForStatement(stmt Statement) time.Duration {
	// For partitioned DML, use longer default if no custom timeout is set
	if _, isPartitionedDML := stmt.(*PartitionedDmlStatement); isPartitionedDML && s.systemVariables.StatementTimeout == nil {
		return 24 * time.Hour // PDML default
	}

	// Use custom timeout if set, otherwise default
	if s.systemVariables.StatementTimeout != nil {
		return *s.systemVariables.StatementTimeout
	}

	return 10 * time.Minute // default timeout
}

// Session represents a database session with transaction management.
//
// Mutex Protection Rationale:
//
// While spanner-mycli is primarily a CLI tool with sequential user interactions,
// mutex protection is essential for several reasons:
//
// 1. **Goroutine safety**: Even in a CLI, operations may spawn goroutines:
//   - Background heartbeat for long-running transactions
//   - Progress bars and monitoring features
//   - Future parallel query execution features
//   - Signal handlers (Ctrl+C) that may access transaction state
//
// 2. **Correctness over performance**: The mutex ensures:
//   - Atomic check-and-act operations (e.g., checking transaction state before modifying)
//   - Prevention of use-after-free bugs if transaction is closed concurrently
//   - Consistent transaction state across all operations
//
// 3. **Future extensibility**: The mutex-based design allows for:
//   - Safe addition of concurrent features without major refactoring
//   - Integration with tools that may access session state concurrently
//   - Support for multiplexed connections or parallel operations
//
// 4. **Best practices**: Following Go's concurrency principles:
//   - "Don't communicate by sharing memory; share memory by communicating"
//   - When sharing is necessary (as with session state), protect it properly
//   - Make the zero value useful - mutex provides safe default behavior
//
// The performance overhead of mutex operations is negligible for CLI usage patterns,
// while the safety guarantees prevent subtle bugs that could corrupt user data.
type Session struct {
	mode         SessionMode
	client       *spanner.Client // can be nil in Detached mode
	adminClient  *adminapi.DatabaseAdminClient
	clientConfig spanner.ClientConfig
	clientOpts   []option.ClientOption
	tc           *transactionContext
	// tcMutex protects the transaction context (tc) from concurrent access.
	// Using RWMutex allows multiple concurrent reads while maintaining exclusive writes.
	// Direct access to tc is managed through withTransactionContextWithLock base method.
	// Helper functions that work with transaction context:
	// - withTransactionContextWithLock: Base method for all tc manipulation (acquires write lock)
	// - TransitTransaction: Atomic state transitions with cleanup
	// - withReadWriteTransaction, withReadWriteTransactionContext, withReadOnlyTransaction: Type-safe transaction access
	// - clearTransactionContext, TransactionAttrsWithLock: Safe context management
	// All transaction context access MUST go through these helpers.
	tcMutex         sync.RWMutex
	systemVariables *systemVariables

	currentBatch Statement

	// experimental support of Cassandra interface
	cqlCluster *gocql.ClusterConfig
	cqlSession *gocql.Session
}

// SessionHandler manages a session pointer and can handle session-changing statements
type SessionHandler struct {
	*Session
}

func NewSessionHandler(session *Session) *SessionHandler {
	return &SessionHandler{
		Session: session,
	}
}

func (h *SessionHandler) GetSession() *Session {
	return h.Session
}

func (h *SessionHandler) Close() {
	if h.Session != nil {
		h.Session.Close()
	}
}

// ExecuteStatement executes a statement, handling session-changing statements appropriately
func (h *SessionHandler) ExecuteStatement(ctx context.Context, stmt Statement) (*Result, error) {
	// Handle session-changing statements
	switch s := stmt.(type) {
	case *UseStatement:
		return h.handleUse(ctx, s)
	case *UseDatabaseMetaCommand:
		// Convert UseDatabaseMetaCommand to UseStatement and handle it
		useStmt := &UseStatement{
			Database: s.Database,
			Role:     "", // \u command doesn't support ROLE parameter
		}
		return h.handleUse(ctx, useStmt)
	case *DetachStatement:
		return h.handleDetach(ctx, s)
	default:
		// For regular statements, delegate to the embedded session
		return h.Session.ExecuteStatement(ctx, stmt)
	}
}

// createSessionWithOpts creates a new session using current session's client options
func (h *SessionHandler) createSessionWithOpts(ctx context.Context, sysVars *systemVariables) (*Session, error) {
	// Create admin-only session if no database is specified
	if sysVars.Database == "" {
		return NewAdminSession(ctx, sysVars, h.clientOpts...)
	}

	return NewSession(ctx, sysVars, h.clientOpts...)
}

func (h *SessionHandler) handleUse(ctx context.Context, s *UseStatement) (*Result, error) {
	newSystemVariables := *h.systemVariables
	newSystemVariables.Database = s.Database
	newSystemVariables.Role = s.Role
	// Clear the registry so it gets recreated with the new systemVariables instance
	newSystemVariables.Registry = nil

	newSession, err := h.createSessionWithOpts(ctx, &newSystemVariables)
	if err != nil {
		return nil, err
	}

	// Check if the target database exists
	exists, err := newSession.DatabaseExists(ctx)
	if err != nil {
		newSession.Close()
		return nil, err
	}

	if !exists {
		newSession.Close()
		return nil, fmt.Errorf("unknown database %q", s.Database)
	}

	// Replace the old session with the new one
	h.Session.Close()
	h.Session = newSession

	return &Result{}, nil
}

func (h *SessionHandler) handleDetach(ctx context.Context, s *DetachStatement) (*Result, error) {
	newSystemVariables := *h.systemVariables

	// Clear database and role to switch to detached mode
	newSystemVariables.Database = ""
	newSystemVariables.Role = ""
	// Clear the registry so it gets recreated with the new systemVariables instance
	newSystemVariables.Registry = nil

	newSession, err := h.createSessionWithOpts(ctx, &newSystemVariables)
	if err != nil {
		return nil, err
	}

	// Replace the old session with the new one
	h.Session.Close()
	h.Session = newSession

	return &Result{}, nil
}

type SessionMode int

const (
	Detached SessionMode = iota
	DatabaseConnected
)

var (
	_ transaction = (*spanner.ReadOnlyTransaction)(nil)
	_ transaction = (*spanner.ReadWriteTransaction)(nil)
)

// withTransactionLocked is a generic helper that executes fn while holding the transaction mutex.
// It validates the transaction mode and returns an appropriate error if the mode doesn't match.
// NOTE: The mutex must NOT be held by the caller - this function acquires it.
func (s *Session) withTransactionLocked(mode transactionMode, fn func() error) error {
	return s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil || (*tcPtr).txn == nil || (*tcPtr).attrs.mode != mode {
			switch mode {
			case transactionModeReadWrite:
				return ErrNotInReadWriteTransaction
			case transactionModeReadOnly:
				return ErrNotInReadOnlyTransaction
			default:
				return fmt.Errorf("not in %s transaction", mode)
			}
		}
		return fn()
	})
}

func (s *Session) withReadWriteTransaction(fn func(*spanner.ReadWriteStmtBasedTransaction) error) error {
	// Reuse withReadWriteTransactionContext, ignoring the context parameter
	return s.withReadWriteTransactionContext(func(txn *spanner.ReadWriteStmtBasedTransaction, _ *transactionContext) error {
		return fn(txn)
	})
}

// withReadWriteTransactionContext executes fn with both the transaction and context under mutex protection.
// This allows safe access to both the transaction and its context fields.
func (s *Session) withReadWriteTransactionContext(fn func(*spanner.ReadWriteStmtBasedTransaction, *transactionContext) error) error {
	return s.withTransactionLocked(transactionModeReadWrite, func() error {
		// At this point, we know tc is not nil and mode is correct (validated by withTransactionLocked)
		// Type assert the transaction safely
		txn, ok := s.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
		if !ok {
			// This should never happen if the mode check is correct, but handle it defensively
			return fmt.Errorf("internal error: transaction has incorrect type for read-write mode (got %T)", s.tc.txn)
		}
		return fn(txn, s.tc)
	})
}

// withReadOnlyTransaction executes fn with the current read-only transaction under mutex protection.
// Returns ErrNotInReadOnlyTransaction if not in a read-only transaction.
func (s *Session) withReadOnlyTransaction(fn func(*spanner.ReadOnlyTransaction) error) error {
	return s.withTransactionLocked(transactionModeReadOnly, func() error {
		// At this point, we know tc is not nil and mode is correct (validated by withTransactionLocked)
		// Type assert the transaction safely
		txn, ok := s.tc.txn.(*spanner.ReadOnlyTransaction)
		if !ok {
			// This should never happen if the mode check is correct, but handle it defensively
			return fmt.Errorf("internal error: transaction has incorrect type for read-only mode (got %T)", s.tc.txn)
		}
		return fn(txn)
	})
}

// withReadOnlyTransactionOrStart executes fn with a read-only transaction.
// If already in a read-only transaction, uses it.
// If not in any transaction, starts a new read-only transaction and closes it after fn completes.
// If in a read-write transaction, returns an error as INFORMATION_SCHEMA queries cannot be used there.
//
// IMPORTANT: The callback fn must NOT call methods that try to acquire the transaction mutex
// (e.g., RunQueryWithStats). Only use methods designed for transaction contexts
// or direct transaction operations (like txn.Query).
func (s *Session) withReadOnlyTransactionOrStart(ctx context.Context, fn func(*spanner.ReadOnlyTransaction) error) error {
	// First try to use existing RO transaction
	err := s.withReadOnlyTransaction(fn)
	if err != ErrNotInReadOnlyTransaction {
		// Either succeeded with existing transaction or different error
		return err
	}

	// Not in RO transaction, start one
	_, err = s.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err != nil {
		return fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	defer func() {
		_ = s.CloseReadOnlyTransaction()
	}()

	// Now execute within the transaction we created
	return s.withReadOnlyTransaction(fn)
}

// clearTransactionContext atomically clears the transaction context.
// This is equivalent to setTransactionContext(nil) but more expressive.
func (s *Session) clearTransactionContext() {
	_ = s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		// Close the transaction context to stop any running heartbeat
		if *tcPtr != nil {
			(*tcPtr).Close()
		}
		*tcPtr = nil
		return nil
	})
}

// withTransactionContextWithLock is the most generic base method for transaction context manipulation.
// It acquires a write lock and provides direct access to the transaction context pointer,
// allowing atomic read-modify-write operations.
// The function fn receives a pointer to the transaction context pointer, enabling it to replace the entire context.
// NOTE: This is the foundation for all transaction state management operations that modify state.
func (s *Session) withTransactionContextWithLock(fn func(tc **transactionContext) error) error {
	s.tcMutex.Lock()
	defer s.tcMutex.Unlock()
	return fn(&s.tc)
}

// TransitTransaction implements a functional state transition pattern for transaction management.
// It atomically transitions from one transaction state to another, handling cleanup of the old state.
// The transition function receives the current context and returns the new context.
// If an error occurs during transition, the original state is preserved.
func (s *Session) TransitTransaction(ctx context.Context, fn func(tc *transactionContext) (*transactionContext, error)) error {
	return s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		oldTc := *tcPtr
		newTc, err := fn(oldTc)
		if err != nil {
			return err
		}

		// Cleanup old transaction context if it's being replaced
		if oldTc != nil && oldTc != newTc {
			oldTc.Close()
		}

		*tcPtr = newTc
		return nil
	})
}

// QueryResult holds the result of a query operation with optional transaction.
type QueryResult struct {
	Iterator    *spanner.RowIterator
	Transaction *spanner.ReadOnlyTransaction
}

// UpdateResult holds the complete result of an update operation.
type UpdateResult struct {
	Rows     []Row                   // Rows returned by the update operation (e.g., from RETURNING clause)
	Stats    map[string]any          // Query statistics from the operation
	Count    int64                   // Number of rows affected by the update
	Metadata *sppb.ResultSetMetadata // Metadata about the result set
	Plan     *sppb.QueryPlan         // Query execution plan (when requested)
}

// DMLResult holds the results of a DML operation execution including commit information.
type DMLResult struct {
	Affected       int64                   // Total number of rows affected by the DML operation
	CommitResponse spanner.CommitResponse  // Commit response including timestamp and stats
	Plan           *sppb.QueryPlan         // Query execution plan (when requested)
	Metadata       *sppb.ResultSetMetadata // Metadata about the result set
}

func logGrpcClientOptions() []option.ClientOption {
	zapDevelopmentConfig := zap.NewDevelopmentConfig()
	zapDevelopmentConfig.DisableCaller = true
	zapLogger, _ := zapDevelopmentConfig.Build(zap.Fields())

	return []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(
			selector.UnaryClientInterceptor(
				logging.UnaryClientInterceptor(InterceptorLogger(zapLogger),
					logging.WithLogOnEvents(logging.FinishCall, logging.PayloadSent, logging.PayloadReceived)),
				selector.MatchFunc(func(ctx context.Context, callMeta interceptors.CallMeta) bool {
					return true
				})))),
		option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(
			selectlogging.StreamClientInterceptor(InterceptorLogger(zapLogger), selector.MatchFunc(func(ctx context.Context, callMeta interceptors.CallMeta) bool {
				req, ok := callMeta.ReqOrNil.(*sppb.ExecuteSqlRequest)
				return !ok || req.GetRequestOptions().GetRequestTag() != "spanner_mycli_heartbeat"
			}), selectlogging.WithLogOnEvents(selectlogging.FinishCall, selectlogging.PayloadSent, selectlogging.PayloadReceived)),
		)),
	}
}

func NewSession(ctx context.Context, sysVars *systemVariables, opts ...option.ClientOption) (*Session, error) {
	dbPath := sysVars.DatabasePath()
	clientConfig := defaultClientConfig
	clientConfig.DatabaseRole = sysVars.Role
	clientConfig.DirectedReadOptions = sysVars.DirectedRead

	if sysVars.Insecure {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	if sysVars.LogGrpc {
		opts = append(opts, logGrpcClientOptions()...)
	}

	opts = append(opts, defaultClientOpts...)
	client, err := spanner.NewClientWithConfig(ctx, dbPath, clientConfig, opts...)
	if err != nil {
		return nil, err
	}

	adminClient, err := adminapi.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return nil, err
	}

	session := &Session{
		mode:            DatabaseConnected,
		client:          client,
		clientConfig:    clientConfig,
		clientOpts:      opts,
		adminClient:     adminClient,
		systemVariables: sysVars,
	}
	sysVars.CurrentSession = session

	return session, nil
}

func NewAdminSession(ctx context.Context, sysVars *systemVariables, opts ...option.ClientOption) (*Session, error) {
	clientConfig := defaultClientConfig
	clientConfig.DatabaseRole = sysVars.Role
	clientConfig.DirectedReadOptions = sysVars.DirectedRead

	if sysVars.Insecure {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	if sysVars.LogGrpc {
		opts = append(opts, logGrpcClientOptions()...)
	}

	opts = append(opts, defaultClientOpts...)

	adminClient, err := adminapi.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return nil, err
	}

	session := &Session{
		mode:            Detached,
		client:          nil, // no database client in detached mode
		clientConfig:    clientConfig,
		clientOpts:      opts,
		adminClient:     adminClient,
		systemVariables: sysVars,
	}
	sysVars.CurrentSession = session

	// Validate instance exists
	exists, err := session.InstanceExists()
	if err != nil {
		session.Close()
		return nil, err
	}
	if !exists {
		session.Close()
		return nil, fmt.Errorf("unknown instance %q", sysVars.Instance)
	}

	return session, nil
}

func (s *Session) Mode() SessionMode {
	return s.mode
}

func (s *Session) IsDetached() bool {
	return s.mode == Detached
}

func (s *Session) RequiresDatabaseConnection() bool {
	return s.client == nil
}

func (s *Session) ValidateDetachedOperation() error {
	// Detached operations only require adminClient, which is always present
	return nil
}

func (s *Session) ValidateDatabaseOperation() error {
	if s.client == nil {
		return errors.New("database operation requires a database connection")
	}
	return nil
}

func (s *Session) ValidateStatementExecution(stmt Statement) error {
	if s.IsDetached() {
		// In Detached mode, only DetachedCompatible statements can be executed
		if _, ok := stmt.(DetachedCompatible); !ok {
			return fmt.Errorf("statement %T is not compatible with detached session mode", stmt)
		}
	}
	// In DatabaseConnected mode, all statements can be executed
	return nil
}

func (s *Session) ConnectToDatabase(ctx context.Context, databaseId string) error {
	if s.mode == DatabaseConnected && s.client != nil {
		return errors.New("session is already connected to a database")
	}

	// Construct database path directly to avoid modifying state before success
	dbPath := databasePath(s.systemVariables.Project, s.systemVariables.Instance, databaseId)
	clientConfig := s.clientConfig

	client, err := spanner.NewClientWithConfig(ctx, dbPath, clientConfig, s.clientOpts...)
	if err != nil {
		return err
	}

	// Close existing client if any
	if s.client != nil {
		s.client.Close()
	}

	// Update state only after successful client creation
	s.systemVariables.Database = databaseId
	s.client = client
	s.mode = DatabaseConnected

	// Heartbeat is now managed per-transaction, not per-session
	// so we don't need to start it here anymore

	return nil
}

// TransactionState returns the current transaction mode and whether a transaction is active.
// This consolidates multiple transaction state checks into a single method.
func (s *Session) TransactionState() (mode transactionMode, isActive bool) {
	attrs := s.TransactionAttrsWithLock()
	isActive = attrs.mode != transactionModeUndetermined && attrs.mode != ""
	return attrs.mode, isActive
}

// TransactionAttrs returns a copy of all transaction attributes.
// This allows safe inspection of transaction state without holding the mutex.
// If no transaction is active, returns a zero-value struct with mode=transactionModeUndetermined.
//
// Design decision: This method uses RLock for read-only access, allowing
// concurrent reads of transaction state.
//
// Naming convention:
// - TransactionAttrsWithLock() - Acquires read lock internally (this method)
// - transactionAttrsLocked() - Assumes caller holds lock (read or write)
func (s *Session) TransactionAttrsWithLock() transactionAttributes {
	s.tcMutex.RLock()
	defer s.tcMutex.RUnlock()

	return s.transactionAttrsLocked()
}

// transactionAttrsLocked returns transaction attributes without locking.
// Caller must hold s.tcMutex.
//
// IMPORTANT: This method exists to prevent deadlocks in DML execution paths.
// The withTransactionLocked method holds tcMutex and calls callbacks that may
// need to access transaction attributes. Using TransactionAttrs() in those
// callbacks would attempt to acquire tcMutex again, causing a deadlock since
// Go mutexes are not reentrant.
//
// Example deadlock scenario:
// 1. withTransactionLocked acquires tcMutex
// 2. Calls callback (e.g., runUpdateOnTransaction)
// 3. Callback calls TransactionAttrs() which tries to acquire tcMutex
// 4. Deadlock occurs
//
// Always use this locked version when already holding tcMutex.
func (s *Session) transactionAttrsLocked() transactionAttributes {
	if s.tc == nil {
		// Return zero-value struct (mode will be transactionModeUndetermined = "")
		return transactionAttributes{}
	}

	// Return a copy of the attributes
	return s.tc.attrs
}

// TransactionMode returns the current transaction mode.
// Deprecated: Use TransactionState() for new code.
func (s *Session) TransactionMode() transactionMode {
	mode, _ := s.TransactionState()
	return mode
}

// InReadWriteTransaction returns true if the session is running read-write transaction.
func (s *Session) InReadWriteTransaction() bool {
	mode, _ := s.TransactionState()
	return mode == transactionModeReadWrite
}

// InReadOnlyTransaction returns true if the session is running read-only transaction.
func (s *Session) InReadOnlyTransaction() bool {
	mode, _ := s.TransactionState()
	return mode == transactionModeReadOnly
}

// InPendingTransaction returns true if the session is running pending transaction.
func (s *Session) InPendingTransaction() bool {
	mode, _ := s.TransactionState()
	return mode == transactionModePending
}

// InTransaction returns true if the session is running transaction.
func (s *Session) InTransaction() bool {
	_, isActive := s.TransactionState()
	return isActive
}

// GetTransactionFlagsWithLock returns multiple transaction state flags in a single lock acquisition.
// This is useful for avoiding multiple lock acquisitions when checking different transaction states.
// Acquires RLock for concurrent read access.
func (s *Session) GetTransactionFlagsWithLock() (inTransaction bool, inReadWriteTransaction bool) {
	s.tcMutex.RLock()
	defer s.tcMutex.RUnlock()

	if s.tc == nil {
		return false, false
	}

	inTransaction = true
	inReadWriteTransaction = s.tc.attrs.mode == transactionModeReadWrite
	return
}

// TransactionOptionsBuilder helps build transaction options with proper defaults.
type TransactionOptionsBuilder struct {
	session        *Session
	priority       sppb.RequestOptions_Priority
	isolationLevel sppb.TransactionOptions_IsolationLevel
	tag            string
}

// NewTransactionOptionsBuilder creates a new builder with session defaults.
func (s *Session) NewTransactionOptionsBuilder() *TransactionOptionsBuilder {
	return &TransactionOptionsBuilder{
		session:        s,
		priority:       sppb.RequestOptions_PRIORITY_UNSPECIFIED,
		isolationLevel: sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
	}
}

// WithPriority sets the transaction priority.
func (b *TransactionOptionsBuilder) WithPriority(priority sppb.RequestOptions_Priority) *TransactionOptionsBuilder {
	b.priority = priority
	return b
}

// WithIsolationLevel sets the transaction isolation level.
func (b *TransactionOptionsBuilder) WithIsolationLevel(level sppb.TransactionOptions_IsolationLevel) *TransactionOptionsBuilder {
	b.isolationLevel = level
	return b
}

// WithTag sets the transaction tag.
func (b *TransactionOptionsBuilder) WithTag(tag string) *TransactionOptionsBuilder {
	b.tag = tag
	return b
}

// Build creates the final TransactionOptions with resolved defaults.
func (b *TransactionOptionsBuilder) Build() spanner.TransactionOptions {
	// Resolve priority
	priority := b.priority
	if priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		priority = b.session.systemVariables.RPCPriority
	}

	// Resolve isolation level
	isolationLevel := b.isolationLevel
	if isolationLevel == sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED {
		isolationLevel = b.session.systemVariables.DefaultIsolationLevel
	}

	return spanner.TransactionOptions{
		CommitOptions:               spanner.CommitOptions{ReturnCommitStats: b.session.systemVariables.ReturnCommitStats, MaxCommitDelay: b.session.systemVariables.MaxCommitDelay},
		CommitPriority:              priority,
		TransactionTag:              b.tag,
		ExcludeTxnFromChangeStreams: b.session.systemVariables.ExcludeTxnFromChangeStreams,
		IsolationLevel:              isolationLevel,
		ReadLockMode:                b.session.systemVariables.ReadLockMode,
	}
}

// BuildPriority returns just the resolved priority.
func (b *TransactionOptionsBuilder) BuildPriority() sppb.RequestOptions_Priority {
	if b.priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		return b.session.systemVariables.RPCPriority
	}
	return b.priority
}

// BuildIsolationLevel returns just the resolved isolation level.
func (b *TransactionOptionsBuilder) BuildIsolationLevel() sppb.TransactionOptions_IsolationLevel {
	if b.isolationLevel == sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED {
		return b.session.systemVariables.DefaultIsolationLevel
	}
	return b.isolationLevel
}

// BeginPendingTransaction starts pending transaction.
// The actual start of the transaction is delayed until the first operation in the transaction is executed.
func (s *Session) BeginPendingTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	opts := s.NewTransactionOptionsBuilder().
		WithIsolationLevel(isolationLevel).
		WithPriority(priority)
	resolvedIsolationLevel := opts.BuildIsolationLevel()
	resolvedPriority := opts.BuildPriority()

	return s.TransitTransaction(ctx, func(tc *transactionContext) (*transactionContext, error) {
		// Check for any type of existing transaction (including pending)
		if tc != nil {
			return nil, fmt.Errorf("%s transaction is already running", tc.attrs.mode)
		}

		// Return new pending transaction context
		return &transactionContext{
			attrs: transactionAttributes{
				mode:           transactionModePending,
				priority:       resolvedPriority,
				isolationLevel: resolvedIsolationLevel,
			},
		}, nil
	})
}

// DetermineTransactionLocked determines the type of transaction to start based on the pending transaction
// and system variables. It returns the timestamp for read-only transactions or a zero time for read-write transactions.
// Caller must hold s.tcMutex.
func (s *Session) DetermineTransactionLocked(ctx context.Context) (time.Time, error) {
	var zeroTime time.Time

	// Check if there's a pending transaction
	if s.tc == nil || s.tc.attrs.mode != transactionModePending {
		return zeroTime, nil
	}

	priority := s.tc.attrs.priority
	isolationLevel := s.tc.attrs.isolationLevel

	// Determine transaction type based on system variables
	if s.systemVariables.ReadOnly {
		// Start a read-only transaction with the pending transaction's priority
		return s.BeginReadOnlyTransactionLocked(ctx, timestampBoundUnspecified, 0, time.Time{}, priority)
	}

	// Start a read-write transaction with the pending transaction's isolation level and priority
	return zeroTime, s.BeginReadWriteTransactionLocked(ctx, isolationLevel, priority)
}

// DetermineTransaction determines the type of transaction to start based on the pending transaction
// and system variables. It returns the timestamp for read-only transactions or a zero time for read-write transactions.
func (s *Session) DetermineTransaction(ctx context.Context) (time.Time, error) {
	var result time.Time
	err := s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		ts, err := s.DetermineTransactionLocked(ctx)
		result = ts
		return err
	})
	return result, err
}

// DetermineTransactionAndState determines transaction and returns both the timestamp and current transaction state.
// This combines DetermineTransaction and InTransaction checks in a single lock acquisition.
func (s *Session) DetermineTransactionAndState(ctx context.Context) (time.Time, bool, error) {
	var result time.Time
	var inTransaction bool
	err := s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		ts, err := s.DetermineTransactionLocked(ctx)
		result = ts
		if err != nil {
			return err
		}
		// Check if we're in a transaction after determining
		inTransaction = *tcPtr != nil
		return nil
	})
	return result, inTransaction, err
}

// getTransactionTagLocked returns the transaction tag from a pending transaction if it exists.
// Caller must hold s.tcMutex.
func (s *Session) getTransactionTagLocked() string {
	if s.tc != nil && s.tc.attrs.mode == transactionModePending {
		return s.tc.attrs.tag
	}
	return ""
}

// BeginReadWriteTransactionLocked starts read-write transaction.
// Caller must hold s.tcMutex.
func (s *Session) BeginReadWriteTransactionLocked(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return err
	}

	// Get transaction tag if there's a pending transaction
	tag := s.getTransactionTagLocked()

	// Build transaction options using the builder
	builder := s.NewTransactionOptionsBuilder().
		WithPriority(priority).
		WithIsolationLevel(isolationLevel).
		WithTag(tag)

	opts := builder.Build()
	resolvedPriority := builder.BuildPriority()
	resolvedIsolationLevel := builder.BuildIsolationLevel()

	// Check for existing transaction
	if s.tc != nil && s.tc.attrs.mode != transactionModePending {
		return fmt.Errorf("%s transaction is already running", s.tc.attrs.mode)
	}

	oldTc := s.tc

	// Create the new transaction
	txn, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, s.client, opts)
	if err != nil {
		return err
	}

	// Cleanup old transaction context if it's being replaced
	if oldTc != nil {
		oldTc.Close()
	}

	// Set new transaction context
	s.tc = &transactionContext{
		attrs: transactionAttributes{
			mode:           transactionModeReadWrite,
			tag:            tag,
			priority:       resolvedPriority,
			isolationLevel: resolvedIsolationLevel,
		},
		txn:           txn,
		heartbeatFunc: s.startHeartbeat,
	}

	return nil
	// Heartbeat will be started by EnableHeartbeat() after the first operation
	// For implicit transactions, they commit immediately so heartbeat isn't needed
}

// BeginReadWriteTransaction starts read-write transaction.
func (s *Session) BeginReadWriteTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	return s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		return s.BeginReadWriteTransactionLocked(ctx, isolationLevel, priority)
	})
}

// CommitReadWriteTransactionLocked commits read-write transaction and returns commit timestamp if successful.
// Caller must hold s.tcMutex.
func (s *Session) CommitReadWriteTransactionLocked(ctx context.Context) (spanner.CommitResponse, error) {
	if s.tc == nil || s.tc.txn == nil {
		return spanner.CommitResponse{}, ErrNotInReadWriteTransaction
	}

	rwTxn, ok := s.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return spanner.CommitResponse{}, ErrNotInReadWriteTransaction
	}

	resp, err := rwTxn.CommitWithReturnResp(ctx)

	// Always clear transaction context after commit attempt.
	// A failed commit invalidates the transaction on the server,
	// so we must clear the context regardless of the outcome.
	s.tc.Close()
	s.tc = nil

	// Return the response and error as-is, preserving any partial commit info
	return resp, err
}

// CommitReadWriteTransaction commits read-write transaction and returns commit timestamp if successful.
func (s *Session) CommitReadWriteTransaction(ctx context.Context) (spanner.CommitResponse, error) {
	var resp spanner.CommitResponse
	err := s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		var commitErr error
		resp, commitErr = s.CommitReadWriteTransactionLocked(ctx)
		return commitErr
	})
	return resp, err
}

// RollbackReadWriteTransactionLocked rollbacks read-write transaction.
// Caller must hold s.tcMutex.
func (s *Session) RollbackReadWriteTransactionLocked(ctx context.Context) error {
	if s.tc == nil || s.tc.txn == nil {
		return ErrNotInReadWriteTransaction
	}

	rwTxn, ok := s.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return ErrNotInReadWriteTransaction
	}

	rwTxn.Rollback(ctx)

	// Clear transaction context after rollback
	s.tc.Close()
	s.tc = nil

	return nil
}

// RollbackReadWriteTransaction rollbacks read-write transaction.
func (s *Session) RollbackReadWriteTransaction(ctx context.Context) error {
	return s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		return s.RollbackReadWriteTransactionLocked(ctx)
	})
}

// resolveTimestampBound returns the effective timestamp bound for a read-only transaction.
func (s *Session) resolveTimestampBound(typ timestampBoundType, staleness time.Duration, timestamp time.Time) spanner.TimestampBound {
	tb := spanner.StrongRead()
	switch typ {
	case strong:
		tb = spanner.StrongRead()
	case exactStaleness:
		tb = spanner.ExactStaleness(staleness)
	case readTimestamp:
		tb = spanner.ReadTimestamp(timestamp)
	default:
		if s.systemVariables.ReadOnlyStaleness != nil {
			tb = *s.systemVariables.ReadOnlyStaleness
		}
	}
	return tb
}

// BeginReadOnlyTransactionLocked starts read-only transaction and returns the snapshot timestamp for the transaction if successful.
// Caller must hold s.tcMutex.
func (s *Session) BeginReadOnlyTransactionLocked(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return time.Time{}, err
	}

	tb := s.resolveTimestampBound(typ, staleness, timestamp)
	resolvedPriority := s.NewTransactionOptionsBuilder().WithPriority(priority).BuildPriority()

	// Check for existing transaction
	if s.tc != nil && s.tc.attrs.mode != transactionModePending {
		return time.Time{}, fmt.Errorf("%s transaction is already running", s.tc.attrs.mode)
	}

	oldTc := s.tc
	txn := s.client.ReadOnlyTransaction().WithTimestampBound(tb)

	// Because google-cloud-go/spanner defers calling BeginTransaction RPC until an actual query is run,
	// we explicitly run a "SELECT 1" query so that we can determine the timestamp of read-only transaction.
	opts := spanner.QueryOptions{Priority: resolvedPriority}
	if _, _, _, _, err := consumeRowIterDiscard(txn.QueryWithOptions(ctx, spanner.NewStatement("SELECT 1"), opts)); err != nil {
		txn.Close()
		return time.Time{}, err
	}

	// Get timestamp first to ensure the transaction is valid
	resultTimestamp, err := txn.Timestamp()
	if err != nil {
		txn.Close()
		return time.Time{}, err
	}

	// Cleanup old transaction context if it's being replaced
	if oldTc != nil {
		oldTc.Close()
	}

	// Set new transaction context
	s.tc = &transactionContext{
		attrs: transactionAttributes{
			mode:     transactionModeReadOnly,
			priority: resolvedPriority,
		},
		txn: txn,
	}

	return resultTimestamp, nil
}

// BeginReadOnlyTransaction starts read-only transaction and returns the snapshot timestamp for the transaction if successful.
func (s *Session) BeginReadOnlyTransaction(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	var resultTimestamp time.Time
	err := s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		ts, err := s.BeginReadOnlyTransactionLocked(ctx, typ, staleness, timestamp, priority)
		resultTimestamp = ts
		return err
	})
	return resultTimestamp, err
}

// closeTransactionWithMode closes a transaction of the specified mode.
// It validates the transaction mode and performs appropriate cleanup.
// The closeFn receives the transaction object to avoid direct tc access.
// closeFn can be nil for transaction types that don't require specific cleanup (e.g., pending transactions).
func (s *Session) closeTransactionWithMode(mode transactionMode, closeFn func(txn transaction) error) error {
	return s.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil || (*tcPtr).attrs.mode != mode {
			switch mode {
			case transactionModeReadWrite:
				return ErrNotInReadWriteTransaction
			case transactionModeReadOnly:
				return ErrNotInReadOnlyTransaction
			case transactionModePending:
				return ErrNotInPendingTransaction
			default:
				return fmt.Errorf("not in %s transaction", mode)
			}
		}

		// Execute the close function if provided
		if closeFn != nil && (*tcPtr).txn != nil {
			if err := closeFn((*tcPtr).txn); err != nil {
				return err
			}
		}

		// Close and clear the transaction context
		(*tcPtr).Close()
		*tcPtr = nil
		return nil
	})
}

// CloseReadOnlyTransaction closes a running read-only transaction.
func (s *Session) CloseReadOnlyTransaction() error {
	return s.closeTransactionWithMode(transactionModeReadOnly, func(txn transaction) error {
		// The transaction is passed as a parameter, maintaining encapsulation
		roTxn, ok := txn.(*spanner.ReadOnlyTransaction)
		if !ok {
			// This should never happen given the mode check in closeTransactionWithMode,
			// but we handle it explicitly to prevent resource leaks
			return fmt.Errorf("internal error: transaction object has incorrect type for read-only mode (got %T)", txn)
		}
		roTxn.Close()
		return nil
	})
}

// ClosePendingTransaction closes a pending transaction.
func (s *Session) ClosePendingTransaction() error {
	// Pending transactions don't have an underlying transaction to close
	return s.closeTransactionWithMode(transactionModePending, nil)
}

// runQueryWithStatsOnTransaction executes a query on the given transaction with statistics
// This is a helper function to be used within transaction closures to avoid direct tc access
// NOTE: This method is called from within transaction callbacks where tcMutex is already held.
func (s *Session) runQueryWithStatsOnTransaction(ctx context.Context, tx transaction, stmt spanner.Statement, implicit bool) *spanner.RowIterator {
	opts := s.queryOptionsLocked(sppb.ExecuteSqlRequest_PROFILE.Enum())
	opts.LastStatement = implicit
	return tx.QueryWithOptions(ctx, stmt, opts)
}

// runAnalyzeQueryOnTransaction executes an analyze query on the given transaction
// This is a helper function to be used within transaction closures to avoid direct tc access
// NOTE: This method is called from within transaction callbacks where tcMutex is already held.
func (s *Session) runAnalyzeQueryOnTransaction(ctx context.Context, tx transaction, stmt spanner.Statement) (*sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
	mode := sppb.ExecuteSqlRequest_PLAN
	opts := spanner.QueryOptions{
		Mode:     &mode,
		Priority: s.currentPriorityLocked(),
	}
	if opts.Options == nil {
		opts.Options = &sppb.ExecuteSqlRequest_QueryOptions{}
	}
	iter := tx.QueryWithOptions(ctx, stmt, opts)
	_, _, metadata, plan, err := consumeRowIterDiscard(iter)
	return plan, metadata, err
}

// runUpdateOnTransaction executes an update statement on a transaction.
// NOTE: This method is always called from within withReadWriteTransactionContext,
// so the tcMutex is already held. We must use the locked versions of methods
// to avoid deadlock.
//
// CRITICAL: This method is part of a callback chain where tcMutex is already held:
//
//	RunInNewOrExistRwTx -> withReadWriteTransactionContext -> withTransactionLocked -> callback -> runUpdateOnTransaction
//
// Using non-locked versions of methods like TransactionAttrs(), CurrentPriority(),
// or QueryOptions() here will cause a deadlock. Always use the *Locked variants.
func (s *Session) runUpdateOnTransaction(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, stmt spanner.Statement, implicit bool) (*UpdateResult, error) {
	fc, err := decoder.FormatConfigWithProto(s.systemVariables.ProtoDescriptor, s.systemVariables.MultilineProtoText)
	if err != nil {
		return nil, err
	}

	opts := s.queryOptionsLocked(sppb.ExecuteSqlRequest_PROFILE.Enum())
	opts.LastStatement = implicit

	// Reset STATEMENT_TAG
	s.systemVariables.RequestTag = ""

	rows, stats, count, metadata, plan, err := consumeRowIterCollect(
		tx.QueryWithOptions(ctx, stmt, opts),
		spannerRowToRow(fc),
	)
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Rows:     rows,
		Stats:    stats,
		Count:    count,
		Metadata: metadata,
		Plan:     plan,
	}, nil
}

// RunQueryWithStats executes a statement with stats either on the running transaction or on the temporal read-only transaction.
// It returns row iterator and read-only transaction if the statement was executed on the read-only transaction.
func (s *Session) RunQueryWithStats(ctx context.Context, stmt spanner.Statement, implicit bool) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Validate that we have a database client for query operations
	if err := s.ValidateDatabaseOperation(); err != nil {
		// This should not happen if DetachedCompatible interface validation is working correctly
		// Log the error for debugging since we can't return it directly
		slog.Error("RunQueryWithStats called without database connection", "error", err, "statement", stmt.SQL)
		// Return nil to indicate error - caller should check for nil
		return nil, nil
	}

	mode := sppb.ExecuteSqlRequest_PROFILE
	opts := s.buildQueryOptions(&mode)
	opts.LastStatement = implicit
	return s.runQueryWithOptions(ctx, stmt, opts)
}

// RunQuery executes a statement either on the running transaction or on the temporal read-only transaction.
// It returns row iterator and read-only transaction if the statement was executed on the read-only transaction.
func (s *Session) RunQuery(ctx context.Context, stmt spanner.Statement) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Validate that we have a database client for query operations
	if err := s.ValidateDatabaseOperation(); err != nil {
		// This should not happen if DetachedCompatible interface validation is working correctly
		// Log the error for debugging since we can't return it directly
		slog.Error("RunQuery called without database connection", "error", err, "statement", stmt.SQL)
		// Return nil to indicate error - caller should check for nil
		return nil, nil
	}

	opts := s.buildQueryOptions(nil)
	return s.runQueryWithOptions(ctx, stmt, opts)
}

// RunAnalyzeQuery analyzes a statement either on the running transaction or on the temporal read-only transaction.
func (s *Session) RunAnalyzeQuery(ctx context.Context, stmt spanner.Statement) (*sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
	_, err := s.DetermineTransaction(ctx)
	if err != nil {
		return nil, nil, err
	}

	mode := sppb.ExecuteSqlRequest_PLAN
	opts := spanner.QueryOptions{
		Mode:     &mode,
		Priority: s.currentPriorityWithLock(),
	}
	iter, _ := s.runQueryWithOptions(ctx, stmt, opts)

	_, _, metadata, plan, err := consumeRowIterDiscard(iter)
	return plan, metadata, err
}

func (s *Session) runQueryWithOptions(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Prepare query options
	s.prepareQueryOptions(&opts)

	// Try to execute in existing transaction first
	if iter, txn := s.tryQueryInTransaction(ctx, stmt, opts); iter != nil {
		return iter, txn
	}

	// Fall back to single-use transaction
	return s.runSingleUseQuery(ctx, stmt, opts)
}

// prepareQueryOptions sets up the query options with system variables
func (s *Session) prepareQueryOptions(opts *spanner.QueryOptions) {
	if opts.Options == nil {
		opts.Options = &sppb.ExecuteSqlRequest_QueryOptions{}
	}

	opts.Options.OptimizerVersion = s.systemVariables.OptimizerVersion
	opts.Options.OptimizerStatisticsPackage = s.systemVariables.OptimizerStatisticsPackage
	opts.RequestTag = s.systemVariables.RequestTag

	// Reset STATEMENT_TAG
	s.systemVariables.RequestTag = ""
}

// tryQueryInTransaction attempts to execute a query within an existing transaction
// Returns (nil, nil) if no active transaction
func (s *Session) tryQueryInTransaction(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Hold the lock for the entire operation to ensure atomicity.
	// Performance impact: The single-lock approach adds minimal overhead for a CLI tool
	// where queries are user-initiated and not high-frequency. The correctness guarantee
	// (preventing transaction context changes mid-operation) outweighs the negligible
	// performance cost.
	s.tcMutex.Lock()
	defer s.tcMutex.Unlock()

	// Check if we're in an active transaction
	if s.tc == nil || (s.tc.attrs.mode != transactionModeReadWrite && s.tc.attrs.mode != transactionModeReadOnly) {
		return nil, nil
	}

	// Validate transaction state
	if s.tc.txn == nil {
		// This shouldn't happen - log the error and return a stopped iterator
		mode := s.tc.attrs.mode
		err := fmt.Errorf("internal error: %s transaction context exists but txn is nil", mode)
		slog.Error("INTERNAL ERROR", "error", err)
		return newStoppedIterator(), nil
	}

	// Apply read-write specific settings
	if s.tc.attrs.mode == transactionModeReadWrite {
		// The current Go Spanner client library does not apply client-level directed read options to read-write transactions.
		// Therefore, we explicitly set query-level options here to fail the query during a read-write transaction.
		opts.DirectedReadOptions = s.clientConfig.DirectedReadOptions
		s.tc.EnableHeartbeat()
	}

	// Execute query on the transaction
	iter := s.tc.txn.QueryWithOptions(ctx, stmt, opts)

	// For read-only transactions, return the transaction for timestamp access
	if s.tc.attrs.mode == transactionModeReadOnly {
		roTxn, ok := s.tc.txn.(*spanner.ReadOnlyTransaction)
		if !ok {
			// This shouldn't happen - log the error and return a stopped iterator
			err := fmt.Errorf("internal error: transaction is not a ReadOnlyTransaction (got %T)", s.tc.txn)
			slog.Error("INTERNAL ERROR", "error", err)
			return newStoppedIterator(), nil
		}
		return iter, roTxn
	}

	// For read-write transactions, return nil as the second value
	return iter, nil
}

// runSingleUseQuery executes a query in a single-use read-only transaction
func (s *Session) runSingleUseQuery(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	if s.client == nil {
		// This is a programming error - log the error and return a stopped iterator
		err := fmt.Errorf("internal error: runQueryWithOptions called with nil client despite validations (sessionMode: %v, statement: %v)",
			s.mode, stmt.SQL)
		slog.Error("INTERNAL ERROR", "error", err)
		return newStoppedIterator(), nil
	}

	txn := s.client.Single()
	if s.systemVariables.ReadOnlyStaleness != nil {
		txn = txn.WithTimestampBound(*s.systemVariables.ReadOnlyStaleness)
	}
	return txn.QueryWithOptions(ctx, stmt, opts), txn
}

// RunUpdate executes a DML statement on the running read-write transaction.
// It returns error if there is no running read-write transaction.
func (s *Session) RunUpdate(ctx context.Context, stmt spanner.Statement, implicit bool) ([]Row, map[string]any, int64,
	*sppb.ResultSetMetadata, *sppb.QueryPlan, error,
) {
	fc, err := decoder.FormatConfigWithProto(s.systemVariables.ProtoDescriptor, s.systemVariables.MultilineProtoText)
	if err != nil {
		return nil, nil, 0, nil, nil, err
	}

	opts := s.queryOptionsWithLock(sppb.ExecuteSqlRequest_PROFILE.Enum())
	opts.LastStatement = implicit

	// Reset STATEMENT_TAG
	s.systemVariables.RequestTag = ""

	var rows []Row
	var stats map[string]any
	var count int64
	var metadata *sppb.ResultSetMetadata
	var plan *sppb.QueryPlan

	err = s.withReadWriteTransactionContext(func(txn *spanner.ReadWriteStmtBasedTransaction, tc *transactionContext) error {
		var err error
		rows, stats, count, metadata, plan, err = consumeRowIterCollect(
			txn.QueryWithOptions(ctx, stmt, opts),
			spannerRowToRow(fc),
		)
		// Enable heartbeat after any operation (success or failure)
		// Even failed operations start the abort countdown
		tc.EnableHeartbeat()
		return err
	})

	if err == ErrNotInReadWriteTransaction {
		return nil, nil, 0, nil, nil, ErrNotInReadWriteTransaction
	}
	if err != nil {
		return nil, nil, 0, nil, nil, err
	}

	return rows, stats, count, metadata, plan, nil
}

func (s *Session) queryOptionsWithLock(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	return spanner.QueryOptions{
		Mode:       mode,
		Priority:   s.currentPriorityWithLock(),
		RequestTag: s.systemVariables.RequestTag,
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion:           s.systemVariables.OptimizerVersion,
			OptimizerStatisticsPackage: s.systemVariables.OptimizerStatisticsPackage,
		},
	}
}

// queryOptionsLocked returns query options without locking.
// Caller must hold s.tcMutex.
//
// This method exists to prevent deadlocks when accessing query options within
// callbacks that are invoked while tcMutex is already held.
// See transactionAttrsLocked for detailed explanation of the deadlock scenario.
func (s *Session) queryOptionsLocked(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	return spanner.QueryOptions{
		Mode:       mode,
		Priority:   s.currentPriorityLocked(),
		RequestTag: s.systemVariables.RequestTag,
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion:           s.systemVariables.OptimizerVersion,
			OptimizerStatisticsPackage: s.systemVariables.OptimizerStatisticsPackage,
		},
	}
}

func (s *Session) GetDatabaseSchema(ctx context.Context) ([]string, *descriptorpb.FileDescriptorSet, error) {
	resp, err := s.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: s.DatabasePath(),
	})
	if err != nil {
		return nil, nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(resp.GetProtoDescriptors(), &fds)
	if err != nil {
		return nil, nil, err
	}

	return resp.GetStatements(), &fds, nil
}

func (s *Session) Close() {
	// Close any active transaction context (which stops heartbeat)
	s.clearTransactionContext()

	if s.client != nil {
		s.client.Close()
	}
	if s.adminClient != nil {
		err := s.adminClient.Close()
		if err != nil {
			slog.Error("error on adminClient.Close()", "err", err)
		}
	}

	if s.cqlSession != nil {
		s.cqlSession.Close()
	}

	// No need to close tee file here as it's managed by StreamManager
}

func (s *Session) DatabasePath() string {
	return s.systemVariables.DatabasePath()
}

func (s *Session) InstancePath() string {
	return s.systemVariables.InstancePath()
}

func (s *Session) InstanceExists() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	// Method 1: Try listing databases (databases.list) first
	// This works for users with database-level permissions and uses the already available adminClient
	dbIter := s.adminClient.ListDatabases(ctx, &adminpb.ListDatabasesRequest{
		Parent:   s.InstancePath(),
		PageSize: 1, // Only check if instance is accessible
	})

	// Try to get the first item from iterator
	_, err := dbIter.Next()

	if err == nil {
		// Successfully got at least one database, instance exists
		return true, nil
	}

	// Check if it's an iterator.Done error (no databases but instance exists)
	if err == iterator.Done {
		return true, nil
	}

	switch status.Code(err) {
	case codes.NotFound:
		return false, nil
	case codes.PermissionDenied:
		// Fall through to try instance admin API
	default:
		// For other errors, fall through to try instance admin API
	}

	// Method 2: Try using instance admin API (instances.get)
	// This works for users with spanner.instances.get permission (like Database Reader role)
	instanceAdminClient, err := instanceapi.NewInstanceAdminClient(ctx, s.clientOpts...)
	if err != nil {
		// If we can't create the instance admin client, return the original database list error
		return false, fmt.Errorf("failed to create instance admin client: %v; original database list error: tried both spanner.databases.list and spanner.instances.get", err)
	}
	defer func() {
		if closeErr := instanceAdminClient.Close(); closeErr != nil {
			slog.Error("error on instanceAdminClient.Close()", "err", closeErr)
		}
	}()

	_, err = instanceAdminClient.GetInstance(ctx, &instancepb.GetInstanceRequest{
		Name: s.InstancePath(),
	})

	if err == nil {
		return true, nil
	}

	switch status.Code(err) {
	case codes.NotFound:
		return false, nil
	case codes.PermissionDenied:
		// Both methods failed with permission denied
		// The instance likely exists but we don't have sufficient permissions
		// to verify its existence. Return an error to inform the user.
		return false, fmt.Errorf("insufficient permissions to verify instance existence: tried both spanner.databases.list and spanner.instances.get")
	default:
		return false, fmt.Errorf("checking instance existence failed: %v", err)
	}
}

func (s *Session) DatabaseExists(ctx context.Context) (bool, error) {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return false, err
	}

	// For users who don't have `spanner.databases.get` IAM permission,
	// check database existence by running an actual query.
	// cf. https://github.com/cloudspannerecosystem/spanner-cli/issues/10
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	stmt := spanner.NewStatement("SELECT 1")
	iter := s.client.Single().
		QueryWithOptions(ctx, stmt, spanner.QueryOptions{Priority: s.currentPriorityWithLock()})
	defer iter.Stop()

	_, err := iter.Next()
	if err == nil {
		return true, nil
	}
	switch spanner.ErrCode(err) {
	case codes.NotFound:
		return false, nil
	case codes.InvalidArgument:
		return false, nil
	default:
		return false, fmt.Errorf("checking database existence failed: %v", err)
	}
}

// RecreateClient closes the current client and creates a new client for the session.
func (s *Session) RecreateClient() error {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := spanner.NewClientWithConfig(ctx, s.DatabasePath(), s.clientConfig, s.clientOpts...)
	if err != nil {
		return err
	}
	s.client.Close()
	s.client = c
	return nil
}

func (s *Session) currentPriorityWithLock() sppb.RequestOptions_Priority {
	attrs := s.TransactionAttrsWithLock()
	if attrs.mode != transactionModeUndetermined && attrs.mode != "" {
		return attrs.priority
	}
	return s.systemVariables.RPCPriority
}

// currentPriorityLocked returns the current priority without locking.
// Caller must hold s.tcMutex.
//
// This method exists to prevent deadlocks when accessing priority within
// callbacks that are invoked while tcMutex is already held.
// See transactionAttrsLocked for detailed explanation of the deadlock scenario.
func (s *Session) currentPriorityLocked() sppb.RequestOptions_Priority {
	attrs := s.transactionAttrsLocked()
	if attrs.mode != transactionModeUndetermined && attrs.mode != "" {
		return attrs.priority
	}
	return s.systemVariables.RPCPriority
}

func (s *Session) buildQueryOptions(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	opts := spanner.QueryOptions{
		Mode:       mode,
		Priority:   s.currentPriorityWithLock(),
		RequestTag: s.systemVariables.RequestTag,
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion:           s.systemVariables.OptimizerVersion,
			OptimizerStatisticsPackage: s.systemVariables.OptimizerStatisticsPackage,
		},
	}
	return opts
}

// startHeartbeat starts heartbeat for read-write transaction.
//
// If no reads or DMLs happen within 10 seconds, the rw-transaction is considered idle at Cloud Spanner server.
// This "SELECT 1" query prevents the transaction from being considered idle.
// cf. https://godoc.org/cloud.google.com/go/spanner#hdr-Idle_transactions
//
// We send an actual heartbeat only if the read-write transaction is active and
// at least one user-initialized SQL query has been executed on the transaction.
// Background: https://github.com/cloudspannerecosystem/spanner-cli/issues/100
func (s *Session) startHeartbeat(ctx context.Context) {
	interval := time.NewTicker(5 * time.Second)
	defer interval.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-interval.C:
			// Get transaction attributes safely without holding mutex to avoid contention.
			// This is critical for performance: the heartbeat runs every 5 seconds, and
			// acquiring the mutex each time was causing test timeouts in parallel tests.
			attrs := s.TransactionAttrsWithLock()

			// Check for invalid states and exit if detected
			if attrs.mode == transactionModeUndetermined || attrs.mode == "" {
				slog.Debug("heartbeat: no active transaction, exiting goroutine")
				return
			}

			// Only send heartbeat if we have an active read-write transaction with heartbeat enabled
			if attrs.mode == transactionModeReadWrite && attrs.sendHeartbeat {
				// Use withReadWriteTransaction to safely access the transaction
				err := s.withReadWriteTransaction(func(txn *spanner.ReadWriteStmtBasedTransaction) error {
					// Always use LOW priority for heartbeat to avoid interfering with real work
					err := heartbeat(txn, sppb.RequestOptions_PRIORITY_LOW)
					if err != nil {
						slog.Error("heartbeat error", "err", err)
					}
					return nil
				})

				// If we couldn't access the transaction, it might have been cleared
				if err == ErrNotInReadWriteTransaction {
					slog.Debug("heartbeat: transaction no longer active, exiting goroutine")
					return
				}
			}
		}
	}
}

func heartbeat(txn *spanner.ReadWriteStmtBasedTransaction, priority sppb.RequestOptions_Priority) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	iter := txn.QueryWithOptions(ctx, spanner.NewStatement("SELECT 1"), spanner.QueryOptions{
		Priority:   priority,
		RequestTag: "spanner_mycli_heartbeat",
	})
	defer iter.Stop()
	_, err := iter.Next()
	return err
}

func parseDirectedReadOption(directedReadOptionText string) (*sppb.DirectedReadOptions, error) {
	directedReadOption := strings.Split(directedReadOptionText, ":")
	if len(directedReadOption) > 2 {
		return nil, fmt.Errorf("directed read option must be in the form of <replica_location>:<replica_type>, but got %q", directedReadOptionText)
	}

	replicaSelection := sppb.DirectedReadOptions_ReplicaSelection{
		Location: directedReadOption[0],
	}

	if len(directedReadOption) == 2 {
		switch strings.ToUpper(directedReadOption[1]) {
		case "READ_ONLY":
			replicaSelection.Type = sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY
		case "READ_WRITE":
			replicaSelection.Type = sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE
		default:
			return nil, fmt.Errorf("<replica_type> must be either READ_WRITE or READ_ONLY, but got %q", directedReadOption[1])
		}
	}

	return &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
			IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
				ReplicaSelections:    []*sppb.DirectedReadOptions_ReplicaSelection{&replicaSelection},
				AutoFailoverDisabled: true,
			},
		},
	}, nil
}

// RunInNewOrExistRwTxLocked is a helper function for DML execution.
// It executes a function in the current RW transaction or an implicit RW transaction.
// If there is an error, the transaction will be rolled back.
// Caller must hold s.tcMutex.
func (s *Session) RunInNewOrExistRwTxLocked(ctx context.Context,
	f func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (*DMLResult, error) {
	// Determine transaction type (this may start a transaction)
	_, err := s.DetermineTransactionLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Check transaction state (no lock needed, we already hold it)
	attrs := s.transactionAttrsLocked()
	mode := attrs.mode
	isActive := mode != transactionModeUndetermined && mode != ""
	var implicitRWTx bool

	if !isActive || mode != transactionModeReadWrite {
		// Start implicit transaction
		// Note: isolation level is not session level property so it is left as unspecified
		priority := s.currentPriorityLocked()
		if err := s.BeginReadWriteTransactionLocked(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, priority); err != nil {
			return nil, err
		}
		implicitRWTx = true
	}

	// Now we have a read-write transaction
	if s.tc == nil || s.tc.txn == nil {
		return nil, ErrNotInReadWriteTransaction
	}

	txn, ok := s.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return nil, ErrNotInReadWriteTransaction
	}

	var affected int64
	var plan *sppb.QueryPlan
	var metadata *sppb.ResultSetMetadata

	// Execute the function
	affected, plan, metadata, err = f(txn, implicitRWTx)

	// Enable heartbeat after any operation (success or failure)
	// Even failed operations start the abort countdown
	// BUT: Don't enable heartbeat for implicit transactions since they commit immediately
	if !implicitRWTx && s.tc != nil {
		s.tc.EnableHeartbeat()
	}

	if err != nil {
		// Rollback the transaction while holding the lock
		if rollbackErr := s.RollbackReadWriteTransactionLocked(ctx); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
		}
		return nil, fmt.Errorf("transaction was aborted: %w", err)
	}

	result := &DMLResult{
		Affected: affected,
		Plan:     plan,
		Metadata: metadata,
	}

	if !implicitRWTx {
		return result, nil
	}

	// For implicit transactions, commit immediately while holding the lock
	resp, err := s.CommitReadWriteTransactionLocked(ctx)
	if err != nil {
		return nil, err
	}

	result.CommitResponse = resp
	return result, nil
}

// RunInNewOrExistRwTx is a helper function for DML execution.
// It executes a function in the current RW transaction or an implicit RW transaction.
// If there is an error, the transaction will be rolled back.
func (s *Session) RunInNewOrExistRwTx(ctx context.Context,
	f func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (*DMLResult, error) {
	// Use a single lock acquisition for the entire operation
	s.tcMutex.Lock()
	defer s.tcMutex.Unlock()

	return s.RunInNewOrExistRwTxLocked(ctx, f)
}

var errReadOnly = errors.New("can't execute this statement in READONLY mode")

func (s *Session) failStatementIfReadOnly() error {
	if s.systemVariables.ReadOnly {
		return errReadOnly
	}

	return nil
}

func extractBatchInfo(stmt Statement) *BatchInfo {
	switch s := stmt.(type) {
	case *BulkDdlStatement:
		return &BatchInfo{
			Mode: batchModeDDL,
			Size: len(s.Ddls),
		}
	case *BatchDMLStatement:
		return &BatchInfo{
			Mode: batchModeDML,
			Size: len(s.DMLs),
		}
	default:
		return nil
	}
}

// ExecuteStatement executes stmt.
// If stmt is a MutationStatement, pending transaction is determined and fails if there is an active read-only transaction.
func (s *Session) ExecuteStatement(ctx context.Context, stmt Statement) (result *Result, err error) {
	// Validate statement compatibility with current session mode
	if err := s.ValidateStatementExecution(stmt); err != nil {
		return nil, err
	}

	// Apply statement timeout based on statement type
	timeout := s.getTimeoutForStatement(stmt)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	defer func() {
		if result != nil {
			result.BatchInfo = extractBatchInfo(s.currentBatch)
		}
	}()
	if _, ok := stmt.(MutationStatement); ok {
		result := &Result{}
		_, err := s.DetermineTransaction(ctx)
		if err != nil {
			return result, err
		}

		err = s.failStatementIfReadOnly()
		if err != nil {
			return result, err
		}
		return stmt.Execute(ctx, s)
	}

	return stmt.Execute(ctx, s)
}

func (s *Session) RunPartitionQuery(ctx context.Context, stmt spanner.Statement) ([]*spanner.Partition, *spanner.BatchReadOnlyTransaction, error) {
	tb := lo.FromPtrOr(s.systemVariables.ReadOnlyStaleness, spanner.StrongRead())

	batchROTx, err := s.client.BatchReadOnlyTransaction(ctx, tb)
	if err != nil {
		return nil, nil, err
	}

	partitions, err := batchROTx.PartitionQueryWithOptions(ctx, stmt, spanner.PartitionOptions{}, spanner.QueryOptions{
		DataBoostEnabled: s.systemVariables.DataBoostEnabled,
		Priority:         s.systemVariables.RPCPriority,
	})
	if err != nil {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
		return nil, nil, fmt.Errorf("query can't be a partition query: %w", err)
	}
	return partitions, batchROTx, nil
}

// createClientOptions creates client options based on credential and system variables
func createClientOptions(ctx context.Context, credential []byte, sysVars *systemVariables) ([]option.ClientOption, error) {
	var opts []option.ClientOption
	if sysVars.Host != "" && sysVars.Port != 0 {
		// Reconstruct the endpoint, adding brackets back for IPv6 addresses
		endpoint := net.JoinHostPort(sysVars.Host, strconv.Itoa(sysVars.Port))
		opts = append(opts, option.WithEndpoint(endpoint))
	}

	switch {
	case sysVars.WithoutAuthentication:
		opts = append(opts, option.WithoutAuthentication())
	case sysVars.EnableADCPlus:
		source, err := tokensource.SmartAccessTokenSource(ctx, adcplus.WithCredentialsJSON(credential), adcplus.WithTargetPrincipal(sysVars.ImpersonateServiceAccount))
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithTokenSource(source))
	case len(credential) > 0:
		opts = append(opts, option.WithCredentialsJSON(credential))
	}

	return opts, nil
}

func createSession(ctx context.Context, credential []byte, sysVars *systemVariables) (*Session, error) {
	opts, err := createClientOptions(ctx, credential, sysVars)
	if err != nil {
		return nil, err
	}

	// Create admin-only session if no database is specified
	if sysVars.Database == "" {
		return NewAdminSession(ctx, sysVars, opts...)
	}

	return NewSession(ctx, sysVars, opts...)
}
