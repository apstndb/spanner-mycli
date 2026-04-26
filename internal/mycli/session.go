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
	"sync/atomic"
	"time"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instancepb "cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"github.com/apstndb/adcplus"
	"github.com/apstndb/adcplus/tokensource"
	"github.com/apstndb/go-grpcinterceptors/selectlogging"
	"github.com/gocql/gocql"
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

// getTimeoutForStatement returns the appropriate timeout for the given statement type
func (s *Session) getTimeoutForStatement(stmt Statement) time.Duration {
	// For partitioned DML, use longer default if no custom timeout is set
	if _, isPartitionedDML := stmt.(*PartitionedDmlStatement); isPartitionedDML && s.systemVariables.Query.StatementTimeout == nil {
		return 24 * time.Hour // PDML default
	}

	// Use custom timeout if set, otherwise default
	if s.systemVariables.Query.StatementTimeout != nil {
		return *s.systemVariables.Query.StatementTimeout
	}

	return 10 * time.Minute // default timeout
}

// Session represents a database session with transaction management.
type Session struct {
	mode            SessionMode
	client          *spanner.Client // can be nil in Detached mode
	adminClient     *adminapi.DatabaseAdminClient
	clientConfig    spanner.ClientConfig
	clientOpts      []option.ClientOption
	txn             *TransactionManager // transaction lifecycle + query execution
	systemVariables *systemVariables

	batch BatchManager

	// schemaGeneration is incremented after DDL execution to signal
	// that schema-dependent caches (e.g., fuzzy completion table list) are stale.
	schemaGeneration uint64

	// ddlCache caches the GetDatabaseDdl response to avoid redundant RPCs.
	// Invalidated on schemaGeneration change (DDL execution) and TTL expiry.
	ddlCache ddlCacheEntry

	// docCache caches Spanner reference documents across GEMINI invocations.
	// Lazily initialized on first GEMINI call; closed when session closes.
	// Protected by docCacheMu per .gemini/styleguide.md §Concurrency.
	docCacheMu sync.Mutex
	docCache   *docCache

	// experimental support of Cassandra interface
	cqlCluster *gocql.ClusterConfig
	cqlSession *gocql.Session
}

// SchemaGeneration returns the current schema generation counter.
func (s *Session) SchemaGeneration() uint64 {
	return atomic.LoadUint64(&s.schemaGeneration)
}

// IncrementSchemaGeneration bumps the schema generation counter,
// signaling that schema-dependent caches should be invalidated.
func (s *Session) IncrementSchemaGeneration() {
	atomic.AddUint64(&s.schemaGeneration, 1)
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
	if sysVars.Connection.Database == "" {
		return NewAdminSession(ctx, sysVars, h.clientOpts...)
	}

	return NewSession(ctx, sysVars, h.clientOpts...)
}

func (h *SessionHandler) handleUse(ctx context.Context, s *UseStatement) (*Result, error) {
	newSystemVariables := *h.systemVariables
	newSystemVariables.Connection.Database = s.Database
	newSystemVariables.Connection.Role = s.Role
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
	newSystemVariables.Connection.Database = ""
	newSystemVariables.Connection.Role = ""
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
	return newSessionWithFactories(
		ctx,
		sysVars,
		spanner.NewClientWithConfig,
		adminapi.NewDatabaseAdminClient,
		func(client *spanner.Client) { client.Close() },
		opts...,
	)
}

func newSessionWithFactories(
	ctx context.Context,
	sysVars *systemVariables,
	clientFactory func(context.Context, string, spanner.ClientConfig, ...option.ClientOption) (*spanner.Client, error),
	adminClientFactory func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error),
	closeClient func(*spanner.Client),
	opts ...option.ClientOption,
) (*Session, error) {
	dbPath := sysVars.DatabasePath()
	clientConfig := defaultClientConfig
	clientConfig.DatabaseRole = sysVars.Connection.Role
	clientConfig.DirectedReadOptions = sysVars.Query.DirectedRead

	if sysVars.Connection.Insecure {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	if sysVars.Feature.LogGrpc {
		opts = append(opts, logGrpcClientOptions()...)
	}

	opts = append(opts, defaultClientOpts...)
	client, err := clientFactory(ctx, dbPath, clientConfig, opts...)
	if err != nil {
		return nil, err
	}

	adminClient, err := adminClientFactory(ctx, opts...)
	if err != nil {
		closeClient(client)
		return nil, err
	}

	session := &Session{
		mode:            DatabaseConnected,
		client:          client,
		clientConfig:    clientConfig,
		clientOpts:      opts,
		adminClient:     adminClient,
		txn:             NewTransactionManager(client, sysVars, clientConfig),
		systemVariables: sysVars,
	}
	sysVars.inTransaction = session.InTransaction

	return session, nil
}

func NewAdminSession(ctx context.Context, sysVars *systemVariables, opts ...option.ClientOption) (*Session, error) {
	clientConfig := defaultClientConfig
	clientConfig.DatabaseRole = sysVars.Connection.Role
	clientConfig.DirectedReadOptions = sysVars.Query.DirectedRead

	if sysVars.Connection.Insecure {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	if sysVars.Feature.LogGrpc {
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
		txn:             NewTransactionManager(nil, sysVars, clientConfig),
		systemVariables: sysVars,
	}
	sysVars.inTransaction = session.InTransaction

	// Validate instance exists
	exists, err := session.InstanceExists()
	if err != nil {
		session.Close()
		return nil, err
	}
	if !exists {
		session.Close()
		return nil, fmt.Errorf("unknown instance %q", sysVars.Connection.Instance)
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
	dbPath := databasePath(s.systemVariables.Connection.Project, s.systemVariables.Connection.Instance, databaseId)

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
	s.systemVariables.Connection.Database = databaseId
	s.client = client
	s.txn.client = client
	s.mode = DatabaseConnected

	// Heartbeat is now managed per-transaction, not per-session
	// so we don't need to start it here anymore

	return nil
}

// --- Delegation methods to TransactionManager ---
// These one-liner methods preserve the existing Session API so callers
// (statement files, fuzzy finder, etc.) don't need to change.

func (s *Session) TransactionState() (mode transactionMode, isActive bool) {
	if s.txn == nil {
		return transactionModeUndetermined, false
	}
	return s.txn.TransactionState()
}

func (s *Session) TransactionAttrsWithLock() transactionAttributes {
	if s.txn == nil {
		return transactionAttributes{}
	}
	return s.txn.TransactionAttrsWithLock()
}

func (s *Session) TransactionMode() transactionMode {
	if s.txn == nil {
		return transactionModeUndetermined
	}
	return s.txn.TransactionMode()
}

func (s *Session) InReadWriteTransaction() bool {
	if s.txn == nil {
		return false
	}
	return s.txn.InReadWriteTransaction()
}

func (s *Session) InReadOnlyTransaction() bool {
	if s.txn == nil {
		return false
	}
	return s.txn.InReadOnlyTransaction()
}

func (s *Session) InPendingTransaction() bool {
	if s.txn == nil {
		return false
	}
	return s.txn.InPendingTransaction()
}

func (s *Session) InTransaction() bool {
	if s.txn == nil {
		return false
	}
	return s.txn.InTransaction()
}

func (s *Session) GetTransactionFlagsWithLock() (inTransaction bool, inReadWriteTransaction bool) {
	if s.txn == nil {
		return false, false
	}
	return s.txn.GetTransactionFlagsWithLock()
}

func (s *Session) NewTransactionOptionsBuilder() *TransactionOptionsBuilder {
	return s.txn.NewTransactionOptionsBuilder()
}

func (s *Session) TransitTransaction(ctx context.Context, fn func(tc *transactionContext) (*transactionContext, error)) error {
	return s.txn.TransitTransaction(ctx, fn)
}

func (s *Session) BeginPendingTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	return s.txn.BeginPendingTransaction(ctx, isolationLevel, priority)
}

func (s *Session) DetermineTransactionLocked(ctx context.Context) (time.Time, error) {
	return s.txn.DetermineTransactionLocked(ctx)
}

func (s *Session) DetermineTransaction(ctx context.Context) (time.Time, error) {
	return s.txn.DetermineTransaction(ctx)
}

func (s *Session) DetermineTransactionAndState(ctx context.Context) (time.Time, bool, error) {
	return s.txn.DetermineTransactionAndState(ctx)
}

func (s *Session) BeginReadWriteTransactionLocked(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	return s.txn.BeginReadWriteTransactionLocked(ctx, isolationLevel, priority)
}

func (s *Session) BeginReadWriteTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	return s.txn.BeginReadWriteTransaction(ctx, isolationLevel, priority)
}

func (s *Session) CommitReadWriteTransactionLocked(ctx context.Context) (spanner.CommitResponse, error) {
	return s.txn.CommitReadWriteTransactionLocked(ctx)
}

func (s *Session) CommitReadWriteTransaction(ctx context.Context) (spanner.CommitResponse, error) {
	return s.txn.CommitReadWriteTransaction(ctx)
}

func (s *Session) RollbackReadWriteTransactionLocked(ctx context.Context) error {
	return s.txn.RollbackReadWriteTransactionLocked(ctx)
}

func (s *Session) RollbackReadWriteTransaction(ctx context.Context) error {
	return s.txn.RollbackReadWriteTransaction(ctx)
}

func (s *Session) BeginReadOnlyTransactionLocked(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	return s.txn.BeginReadOnlyTransactionLocked(ctx, typ, staleness, timestamp, priority)
}

func (s *Session) BeginReadOnlyTransaction(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	return s.txn.BeginReadOnlyTransaction(ctx, typ, staleness, timestamp, priority)
}

func (s *Session) CloseReadOnlyTransaction() error {
	return s.txn.CloseReadOnlyTransaction()
}

func (s *Session) ClosePendingTransaction() error {
	return s.txn.ClosePendingTransaction()
}

func (s *Session) RunQueryWithStats(ctx context.Context, stmt spanner.Statement, implicit bool) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	return s.txn.RunQueryWithStats(ctx, stmt, implicit)
}

func (s *Session) RunQuery(ctx context.Context, stmt spanner.Statement) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	return s.txn.RunQuery(ctx, stmt)
}

func (s *Session) RunAnalyzeQuery(ctx context.Context, stmt spanner.Statement) (*sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
	return s.txn.RunAnalyzeQuery(ctx, stmt)
}

func (s *Session) RunUpdate(ctx context.Context, stmt spanner.Statement, implicit bool) ([]Row, map[string]any, int64, *sppb.ResultSetMetadata, *sppb.QueryPlan, error) {
	return s.txn.RunUpdate(ctx, stmt, implicit)
}

func (s *Session) RunInNewOrExistRwTxLocked(ctx context.Context,
	f func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (*DMLResult, error) {
	return s.txn.RunInNewOrExistRwTxLocked(ctx, f)
}

func (s *Session) RunInNewOrExistRwTx(ctx context.Context,
	f func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (*DMLResult, error) {
	return s.txn.RunInNewOrExistRwTx(ctx, f)
}

func (s *Session) RunPartitionQuery(ctx context.Context, stmt spanner.Statement) ([]*spanner.Partition, *spanner.BatchReadOnlyTransaction, error) {
	return s.txn.RunPartitionQuery(ctx, stmt)
}

func (s *Session) withReadOnlyTransactionOrStart(ctx context.Context, fn func(*spanner.ReadOnlyTransaction) error) error {
	return s.txn.withReadOnlyTransactionOrStart(ctx, fn)
}

func (s *Session) withReadWriteTransaction(fn func(*spanner.ReadWriteStmtBasedTransaction) error) error {
	return s.txn.withReadWriteTransaction(fn)
}

func (s *Session) withReadWriteTransactionContext(fn func(*spanner.ReadWriteStmtBasedTransaction, *transactionContext) error) error {
	return s.txn.withReadWriteTransactionContext(fn)
}

func (s *Session) withReadOnlyTransaction(fn func(*spanner.ReadOnlyTransaction) error) error {
	return s.txn.withReadOnlyTransaction(fn)
}

func (s *Session) runQueryWithStatsOnTransaction(ctx context.Context, tx transaction, stmt spanner.Statement, implicit bool) *spanner.RowIterator {
	return s.txn.runQueryWithStatsOnTransaction(ctx, tx, stmt, implicit)
}

func (s *Session) runAnalyzeQueryOnTransaction(ctx context.Context, tx transaction, stmt spanner.Statement) (*sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
	return s.txn.runAnalyzeQueryOnTransaction(ctx, tx, stmt)
}

func (s *Session) runUpdateOnTransaction(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, stmt spanner.Statement, implicit bool) (*UpdateResult, error) {
	return s.txn.runUpdateOnTransaction(ctx, tx, stmt, implicit)
}

func (s *Session) currentPriorityWithLock() sppb.RequestOptions_Priority {
	return s.txn.currentPriorityWithLock()
}

// --- End of delegation methods ---

// ddlCacheEntry stores a cached GetDatabaseDdl response.
type ddlCacheEntry struct {
	mu               sync.Mutex
	response         *adminpb.GetDatabaseDdlResponse
	fetchedAt        time.Time
	schemaGeneration uint64
	nowFunc          func() time.Time // for testing; defaults to time.Now
}

// now returns the current time, using nowFunc if set or time.Now otherwise.
func (c *ddlCacheEntry) now() time.Time {
	if c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

const ddlCacheTTL = 30 * time.Second

// GetDatabaseDdlCached returns the cached DDL response, fetching from the API
// only when the cache is stale (TTL expired or schema generation changed).
func (s *Session) GetDatabaseDdlCached(ctx context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
	s.ddlCache.mu.Lock()
	defer s.ddlCache.mu.Unlock()

	gen := s.SchemaGeneration()

	now := s.ddlCache.now()
	if s.ddlCache.response != nil &&
		s.ddlCache.schemaGeneration == gen &&
		now.Sub(s.ddlCache.fetchedAt) < ddlCacheTTL {
		return s.ddlCache.response, nil
	}

	resp, err := s.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: s.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	s.ddlCache.response = resp
	s.ddlCache.fetchedAt = now
	s.ddlCache.schemaGeneration = gen
	return resp, nil
}

// getOrCreateDocCache returns the session-scoped docCache, creating it on
// first call. The cache persists across GEMINI invocations so that API-fetched
// documents are reused. Closed when the session closes.
// The check-then-act is protected by docCacheMu (see .gemini/styleguide.md §Concurrency).
func (s *Session) getOrCreateDocCache(apiKey string) (*docCache, error) {
	s.docCacheMu.Lock()
	defer s.docCacheMu.Unlock()

	if s.docCache != nil {
		return s.docCache, nil
	}

	var opts []docCacheOption
	if apiKey != "" {
		apiClient := newDevKnowledgeClient(apiKey)
		searcher := &devKnowledgeDocSearcher{client: apiClient}
		opts = append(opts,
			withDocFetcher(searcher.GetDocument),
			withDocBatchFetcher(searcher.BatchGetDocuments),
			withDocAPISearcher(searcher.Search),
		)
		slog.Debug("Developer Knowledge API enabled for documentation")
	} else {
		slog.Debug("No API key set, using embedded docs only")
	}

	cache, err := newDocCache(opts...)
	if err != nil {
		return nil, fmt.Errorf("create doc cache: %w", err)
	}

	if err := loadEmbeddedDocs(cache); err != nil {
		cache.Close()
		return nil, fmt.Errorf("load embedded docs: %w", err)
	}

	s.docCache = cache
	return cache, nil
}

func (s *Session) GetDatabaseSchema(ctx context.Context) ([]string, *descriptorpb.FileDescriptorSet, error) {
	resp, err := s.GetDatabaseDdlCached(ctx)
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
	if s.txn != nil {
		s.txn.clearTransactionContext()
	}

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

	s.docCacheMu.Lock()
	dc := s.docCache
	s.docCacheMu.Unlock()
	if dc != nil {
		dc.Close()
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
	s.txn.client = c
	return nil
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

var errReadOnly = errors.New("can't execute this statement in READONLY mode")

func (s *Session) failStatementIfReadOnly() error {
	if s.systemVariables.Transaction.ReadOnly {
		return errReadOnly
	}

	return nil
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
			result.BatchInfo = s.batch.Info()
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

// createClientOptions creates client options based on credential and system variables
func createClientOptions(ctx context.Context, credential []byte, sysVars *systemVariables) ([]option.ClientOption, error) {
	var opts []option.ClientOption
	if sysVars.Connection.Host != "" && sysVars.Connection.Port != 0 {
		// Reconstruct the endpoint, adding brackets back for IPv6 addresses
		endpoint := net.JoinHostPort(sysVars.Connection.Host, strconv.Itoa(sysVars.Connection.Port))
		opts = append(opts, option.WithEndpoint(endpoint))
	}

	switch {
	case sysVars.Connection.WithoutAuthentication:
		opts = append(opts, option.WithoutAuthentication())
	case sysVars.Connection.EnableADCPlus:
		source, err := tokensource.SmartAccessTokenSource(ctx, adcplus.WithCredentialsJSON(credential), adcplus.WithTargetPrincipal(sysVars.Connection.ImpersonateServiceAccount))
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
	if sysVars.Connection.Database == "" {
		return NewAdminSession(ctx, sysVars, opts...)
	}

	return NewSession(ctx, sysVars, opts...)
}
