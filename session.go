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

package main

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
	if _, isPartitionedDML := stmt.(*PartitionedDmlStatement); isPartitionedDML && s.systemVariables.StatementTimeout == nil {
		return 24 * time.Hour // PDML default
	}
	
	// Use custom timeout if set, otherwise default
	if s.systemVariables.StatementTimeout != nil {
		return *s.systemVariables.StatementTimeout
	}
	
	return 10 * time.Minute // default timeout
}

type Session struct {
	mode            SessionMode
	client          *spanner.Client // can be nil in Detached mode
	adminClient     *adminapi.DatabaseAdminClient
	clientConfig    spanner.ClientConfig
	clientOpts      []option.ClientOption
	tc              *transactionContext
	tcMutex         sync.Mutex // Guard a critical section for transaction.
	systemVariables *systemVariables

	currentBatch Statement

	// experimental support of Cassandra interface
	cqlCluster *gocql.ClusterConfig
	cqlSession *gocql.Session

	// metrics collector for client-side metrics
	metricsCollector *MetricsCollector
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

	newSession, err := h.createSessionWithOpts(ctx, &newSystemVariables)
	if err != nil {
		return nil, err
	}

	// Check if the target database exists
	exists, err := newSession.DatabaseExists()
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

type transactionMode string

const (
	transactionModeUndetermined = ""
	transactionModePending      = "pending"
	transactionModeReadOnly     = "read-only"
	transactionModeReadWrite    = "read-write"
)

type transactionContext struct {
	mode          transactionMode
	tag           string
	priority      sppb.RequestOptions_Priority
	sendHeartbeat bool // Becomes true only after a user-driven query is executed on the transaction.

	// txn holds either a read-write or read-only transaction.
	// Design rationale: Using a single transaction interface field maintains mutual exclusivity
	// by preventing both RW and RO transactions from existing simultaneously.
	// The transaction interface provides type safety while still requiring type assertions
	// for specific transaction types.
	txn            transaction
	isolationLevel sppb.TransactionOptions_IsolationLevel
}

func (tc *transactionContext) RWTxn() *spanner.ReadWriteStmtBasedTransaction {
	if tc == nil || tc.txn == nil {
		panic("read-write transaction is not available")
	}
	if tc.mode != transactionModeReadWrite {
		panic(fmt.Sprintf("must be in read-write transaction, but: %v", tc.mode))
	}
	return tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
}

func (tc *transactionContext) ROTxn() *spanner.ReadOnlyTransaction {
	if tc == nil || tc.txn == nil {
		panic("read-only transaction is not available")
	}
	if tc.mode != transactionModeReadOnly {
		panic(fmt.Sprintf("must be in read-only transaction, but: %v", tc.mode))
	}
	return tc.txn.(*spanner.ReadOnlyTransaction)
}

var (
	_ transaction = (*spanner.ReadOnlyTransaction)(nil)
	_ transaction = (*spanner.ReadWriteTransaction)(nil)
)

func (tc *transactionContext) Txn() transaction {
	if tc == nil || tc.txn == nil {
		panic("transaction is not available")
	}
	if tc.mode != transactionModeReadOnly && tc.mode != transactionModeReadWrite {
		panic(fmt.Sprintf("must be in transaction, but: %v", tc.mode))
	}
	return tc.txn
}

type transaction interface {
	QueryWithOptions(ctx context.Context, statement spanner.Statement, opts spanner.QueryOptions) *spanner.RowIterator
	Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator
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
		))}
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

	// Initialize metrics collector if enabled
	var metricsCollector *MetricsCollector
	if sysVars.EnableClientMetrics {
		slog.Info("Initializing client metrics collector")
		collector, err := NewMetricsCollector()
		if err != nil {
			return nil, fmt.Errorf("failed to create metrics collector: %w", err)
		}
		metricsCollector = collector
		
		// Enable OpenTelemetry metrics in Spanner
		spanner.EnableOpenTelemetryMetrics()
		slog.Info("Enabled OpenTelemetry metrics in Spanner")
		
		// Configure client to use our meter provider
		clientConfig.OpenTelemetryMeterProvider = collector.MeterProvider()
		
		// Enable native metrics when client metrics are enabled
		clientConfig.DisableNativeMetrics = false
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
		mode:             DatabaseConnected,
		client:           client,
		clientConfig:     clientConfig,
		clientOpts:       opts,
		adminClient:      adminClient,
		systemVariables:  sysVars,
		metricsCollector: metricsCollector,
	}
	sysVars.CurrentSession = session
	go session.startHeartbeat()

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

	wasDetached := s.mode == Detached
	
	// Update state only after successful client creation
	s.systemVariables.Database = databaseId
	s.client = client
	s.mode = DatabaseConnected
	
	// Start heartbeat if transitioning from Detached mode
	if wasDetached {
		go s.startHeartbeat()
	}

	return nil
}

func (s *Session) TransactionMode() transactionMode {
	if s.tc == nil {
		return transactionModeUndetermined
	}
	return s.tc.mode
}

// InReadWriteTransaction returns true if the session is running read-write transaction.
func (s *Session) InReadWriteTransaction() bool {
	return s.tc != nil && s.tc.mode == transactionModeReadWrite
}

// InReadOnlyTransaction returns true if the session is running read-only transaction.
func (s *Session) InReadOnlyTransaction() bool {
	return s.tc != nil && s.tc.mode == transactionModeReadOnly
}

// InPendingTransaction returns true if the session is running pending transaction.
func (s *Session) InPendingTransaction() bool {
	return s.tc != nil && s.tc.mode == transactionModePending
}

// InTransaction returns true if the session is running transaction.
func (s *Session) InTransaction() bool {
	return s.tc != nil
}

// validateNoActiveTransaction checks if there's no active transaction and returns an error if one exists.
func (s *Session) validateNoActiveTransaction() error {
	if s.InReadWriteTransaction() {
		return errors.New("read-write transaction is already running")
	}

	if s.InReadOnlyTransaction() {
		return errors.New("read-only transaction is already running")
	}

	return nil
}

// resolveTransactionPriority returns the effective priority for a transaction.
// If the provided priority is unspecified, it uses the session's default priority.
func (s *Session) resolveTransactionPriority(priority sppb.RequestOptions_Priority) sppb.RequestOptions_Priority {
	if priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		return s.systemVariables.RPCPriority
	}
	return priority
}

// resolveIsolationLevel returns the effective isolation level for a transaction.
// If the provided isolation level is unspecified, it uses the session's default isolation level.
func (s *Session) resolveIsolationLevel(isolationLevel sppb.TransactionOptions_IsolationLevel) sppb.TransactionOptions_IsolationLevel {
	if isolationLevel == sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED {
		return s.systemVariables.DefaultIsolationLevel
	}
	return isolationLevel
}

// BeginPendingTransaction starts pending transaction.
// The actual start of the transaction is delayed until the first operation in the transaction is executed.
func (s *Session) BeginPendingTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	if err := s.validateNoActiveTransaction(); err != nil {
		return err
	}

	resolvedIsolationLevel := s.resolveIsolationLevel(isolationLevel)
	resolvedPriority := s.resolveTransactionPriority(priority)

	s.tc = &transactionContext{
		mode:           transactionModePending,
		priority:       resolvedPriority,
		isolationLevel: resolvedIsolationLevel,
	}
	return nil
}

// DetermineTransaction determines the type of transaction to start based on the pending transaction
// and system variables. It returns the timestamp for read-only transactions or a zero time for read-write transactions.
func (s *Session) DetermineTransaction(ctx context.Context) (time.Time, error) {
	var zeroTime time.Time

	// If there's no pending transaction, no action is needed
	if s.tc == nil || s.tc.mode != transactionModePending {
		return zeroTime, nil
	}

	// Determine transaction type based on system variables
	if s.systemVariables.ReadOnly {
		// Start a read-only transaction with the pending transaction's priority
		return s.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, s.tc.priority)
	}

	// Start a read-write transaction with the pending transaction's isolation level and priority
	return zeroTime, s.BeginReadWriteTransaction(ctx, s.tc.isolationLevel, s.tc.priority)
}

// getTransactionTag returns the transaction tag from a pending transaction if it exists.
func (s *Session) getTransactionTag() string {
	if s.tc != nil && s.tc.mode == transactionModePending {
		return s.tc.tag
	}
	return ""
}

// BeginReadWriteTransaction starts read-write transaction.
func (s *Session) BeginReadWriteTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return err
	}
	
	if err := s.validateNoActiveTransaction(); err != nil {
		return err
	}

	tag := s.getTransactionTag()
	resolvedPriority := s.resolveTransactionPriority(priority)
	resolvedIsolationLevel := s.resolveIsolationLevel(isolationLevel)

	opts := spanner.TransactionOptions{
		CommitOptions:               spanner.CommitOptions{ReturnCommitStats: s.systemVariables.ReturnCommitStats, MaxCommitDelay: s.systemVariables.MaxCommitDelay},
		CommitPriority:              resolvedPriority,
		TransactionTag:              tag,
		ExcludeTxnFromChangeStreams: s.systemVariables.ExcludeTxnFromChangeStreams,
		IsolationLevel:              resolvedIsolationLevel,
	}

	txn, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, s.client, opts)
	if err != nil {
		return err
	}
	s.tc = &transactionContext{
		mode:           transactionModeReadWrite,
		tag:            tag,
		priority:       resolvedPriority,
		txn:            txn,
		isolationLevel: resolvedIsolationLevel,
	}
	return nil
}

// CommitReadWriteTransaction commits read-write transaction and returns commit timestamp if successful.
func (s *Session) CommitReadWriteTransaction(ctx context.Context) (spanner.CommitResponse, error) {
	_, err := s.DetermineTransaction(ctx)
	if err != nil {
		return spanner.CommitResponse{}, err
	}

	if !s.InReadWriteTransaction() {
		return spanner.CommitResponse{}, errors.New("read-write transaction is not running")
	}

	s.tcMutex.Lock()
	defer s.tcMutex.Unlock()

	resp, err := s.tc.RWTxn().CommitWithReturnResp(ctx)
	s.tc = nil
	return resp, err
}

// RollbackReadWriteTransaction rollbacks read-write transaction.
func (s *Session) RollbackReadWriteTransaction(ctx context.Context) error {
	_, err := s.DetermineTransaction(ctx)
	if err != nil {
		return err
	}

	if !s.InReadWriteTransaction() {
		return errors.New("read-write transaction is not running")
	}

	s.tcMutex.Lock()
	defer s.tcMutex.Unlock()

	s.tc.RWTxn().Rollback(ctx)
	s.tc = nil
	return nil
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

// BeginReadOnlyTransaction starts read-only transaction and returns the snapshot timestamp for the transaction if successful.
func (s *Session) BeginReadOnlyTransaction(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return time.Time{}, err
	}
	
	if err := s.validateNoActiveTransaction(); err != nil {
		return time.Time{}, err
	}

	tb := s.resolveTimestampBound(typ, staleness, timestamp)
	resolvedPriority := s.resolveTransactionPriority(priority)

	txn := s.client.ReadOnlyTransaction().WithTimestampBound(tb)

	// Because google-cloud-go/spanner defers calling BeginTransaction RPC until an actual query is run,
	// we explicitly run a "SELECT 1" query so that we can determine the timestamp of read-only transaction.
	opts := spanner.QueryOptions{Priority: resolvedPriority}
	if _, _, _, _, err := consumeRowIterDiscard(txn.QueryWithOptions(ctx, spanner.NewStatement("SELECT 1"), opts)); err != nil {
		return time.Time{}, err
	}

	s.tc = &transactionContext{
		mode:     transactionModeReadOnly,
		priority: resolvedPriority,
		txn:      txn,
	}

	return txn.Timestamp()
}

// CloseReadOnlyTransaction closes a running read-only transaction.
func (s *Session) CloseReadOnlyTransaction() error {
	if !s.InReadOnlyTransaction() {
		return errors.New("read-only transaction is not running")
	}

	s.tc.ROTxn().Close()
	s.tc = nil
	return nil
}

func (s *Session) ClosePendingTransaction() error {
	if !s.InPendingTransaction() {
		return errors.New("pending transaction is not running")
	}

	s.tc = nil
	return nil
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
		Priority: s.currentPriority(),
	}
	iter, _ := s.runQueryWithOptions(ctx, stmt, opts)

	_, _, metadata, plan, err := consumeRowIterDiscard(iter)
	return plan, metadata, err
}

func (s *Session) runQueryWithOptions(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {

	if opts.Options == nil {
		opts.Options = &sppb.ExecuteSqlRequest_QueryOptions{}
	}

	opts.Options.OptimizerVersion = s.systemVariables.OptimizerVersion
	opts.Options.OptimizerStatisticsPackage = s.systemVariables.OptimizerStatisticsPackage
	opts.RequestTag = s.systemVariables.RequestTag

	// Reset STATEMENT_TAG
	s.systemVariables.RequestTag = ""

	switch {
	case s.InReadWriteTransaction():
		// The current Go Spanner client library does not apply client-level directed read options to read-write transactions.
		// Therefore, we explicitly set query-level options here to fail the query during a read-write transaction.
		opts.DirectedReadOptions = s.clientConfig.DirectedReadOptions
		iter := s.tc.RWTxn().QueryWithOptions(ctx, stmt, opts)
		s.tc.sendHeartbeat = true
		return iter, nil
	case s.InReadOnlyTransaction():
		return s.tc.ROTxn().QueryWithOptions(ctx, stmt, opts), s.tc.ROTxn()
	default:
		// s.client should never be nil here due to validation in RunQuery/RunQueryWithStats
		// and DetachedCompatible interface checks in ExecuteStatement
		if s.client == nil {
			// This is a programming error - log it and return a failing iterator
			slog.Error("INTERNAL ERROR: runQueryWithOptions called with nil client despite validations", 
				"sessionMode", s.mode,
				"statement", stmt.SQL)
			// Create a failing iterator that will return an error when used
			iter := &spanner.RowIterator{}
			iter.Stop()
			return iter, nil
		}
		txn := s.client.Single()
		if s.systemVariables.ReadOnlyStaleness != nil {
			txn = txn.WithTimestampBound(*s.systemVariables.ReadOnlyStaleness)
		}
		return txn.QueryWithOptions(ctx, stmt, opts), txn
	}
}

// RunUpdate executes a DML statement on the running read-write transaction.
// It returns error if there is no running read-write transaction.
func (s *Session) RunUpdate(ctx context.Context, stmt spanner.Statement, implicit bool) ([]Row, map[string]any, int64,
	*sppb.ResultSetMetadata, *sppb.QueryPlan, error) {
	fc, err := formatConfigWithProto(s.systemVariables.ProtoDescriptor, s.systemVariables.MultilineProtoText)
	if err != nil {
		return nil, nil, 0, nil, nil, err
	}

	if !s.InReadWriteTransaction() {
		return nil, nil, 0, nil, nil, errors.New("read-write transaction is not running")
	}

	opts := s.queryOptions(sppb.ExecuteSqlRequest_PROFILE.Enum())
	opts.LastStatement = implicit

	// Reset STATEMENT_TAG
	s.systemVariables.RequestTag = ""

	rows, stats, count, metadata, plan, err := consumeRowIterCollect(s.tc.RWTxn().QueryWithOptions(ctx, stmt, opts), spannerRowToRow(fc))
	s.tc.sendHeartbeat = true
	return rows, stats, count, metadata, plan, err
}

func (s *Session) queryOptions(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	return spanner.QueryOptions{
		Mode:       mode,
		Priority:   s.currentPriority(),
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

func (s *Session) DatabaseExists() (bool, error) {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return false, err
	}
	
	// For users who don't have `spanner.databases.get` IAM permission,
	// check database existence by running an actual query.
	// cf. https://github.com/cloudspannerecosystem/spanner-cli/issues/10
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	stmt := spanner.NewStatement("SELECT 1")
	iter := s.client.Single().
		QueryWithOptions(ctx, stmt, spanner.QueryOptions{Priority: s.currentPriority()})
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

func (s *Session) currentPriority() sppb.RequestOptions_Priority {
	if s.tc != nil {
		return s.tc.priority
	}
	return s.systemVariables.RPCPriority
}

func (s *Session) buildQueryOptions(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	opts := spanner.QueryOptions{
		Mode:       mode,
		Priority:   s.currentPriority(),
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
func (s *Session) startHeartbeat() {
	interval := time.NewTicker(5 * time.Second)
	defer interval.Stop()

	for range interval.C {
		func() {
			s.tcMutex.Lock()
			defer s.tcMutex.Unlock()
			if s.tc != nil && s.tc.mode == transactionModeReadWrite && s.tc.sendHeartbeat {
				err := heartbeat(s.tc.RWTxn(), s.currentPriority())
				if err != nil {
					slog.Error("heartbeat error", "err", err)
				}
			}
		}()
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

// RunInNewOrExistRwTx is a helper function for DML execution.
// It executes a function in the current RW transaction or an implicit RW transaction.
// If there is an error, the transaction will be rolled back.
func (s *Session) RunInNewOrExistRwTx(ctx context.Context,
	f func(implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (affected int64, commitResponse spanner.CommitResponse, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
	_, err = s.DetermineTransaction(ctx)
	if err != nil {
		return 0, spanner.CommitResponse{}, nil, nil, err
	}

	var implicitRWTx bool
	if !s.InReadWriteTransaction() {
		// Start implicit transaction.
		// Note: isolation level is not session level property so it is left as unspecified.
		if err := s.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, s.currentPriority()); err != nil {
			return 0, spanner.CommitResponse{}, nil, nil, err
		}
		implicitRWTx = true
	}

	affected, plan, metadata, err = f(implicitRWTx)
	if err != nil {
		// once error has happened, escape from the current transaction
		if rollbackErr := s.RollbackReadWriteTransaction(ctx); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
		}
		return 0, spanner.CommitResponse{}, nil, nil, fmt.Errorf("transaction was aborted: %w", err)
	}

	if !implicitRWTx {
		return affected, spanner.CommitResponse{}, plan, metadata, nil
	}

	// query mode PLAN doesn't have any side effects, but use commit to get commit timestamp.
	resp, err := s.CommitReadWriteTransaction(ctx)
	if err != nil {
		return 0, spanner.CommitResponse{}, nil, nil, err
	}
	return affected, resp, plan, metadata, nil
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
		result := &Result{IsMutation: true}
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
	// When EnableADCPlus is false and no credential is provided,
	// the client library will use Application Default Credentials (ADC) automatically

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
