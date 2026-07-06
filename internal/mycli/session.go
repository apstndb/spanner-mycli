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
	"encoding/json"
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

	"cloud.google.com/go/bigquery"
)

var defaultClientConfig = spanner.ClientConfig{
	DisableNativeMetrics: true,
}

var defaultClientOpts = []option.ClientOption{
	option.WithGRPCConnectionPool(1),
}

func clientConfigForSystemVariables(sysVars *systemVariables) spanner.ClientConfig {
	if override := sysVars.Config.EmbeddedClientConfig; override != nil {
		return *override
	}
	return defaultClientConfig
}

// Use MEDIUM priority not to disturb regular workloads on the database.
const defaultPriority = sppb.RequestOptions_PRIORITY_MEDIUM

// getTimeoutForStatement returns the timeout applied at the statement execution
// boundary: ExecuteStatement wraps the context with this value exactly once, so
// individual operations must not layer their own timeouts on top. A user-set
// STATEMENT_TIMEOUT (nil until set) overrides the defaults: 10 minutes for
// ordinary statements, and 24 hours for partitioned DML to accommodate its
// long-running semantics.
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

	// BigQuery client cached across BIGQUERY statements. The client and its
	// auth options are built lazily on the first BIGQUERY statement (never in
	// createSession) so ordinary Spanner/emulator sessions never touch BigQuery
	// credentials. bqCredential holds the raw credential for that lazy build,
	// and bqClientKey records the (project, location) the cached client was
	// built with so it can be rebuilt when CLI_BIGQUERY_PROJECT/LOCATION change.
	bqCredential []byte
	bqClient     *bigquery.Client
	bqClientKey  bigQueryClientKey

	// output is the per-statement output destination for streamed results,
	// set for the duration of one statement execution via withOutput /
	// ExecuteStatementWithOutput. See outputContext in output_context.go.
	// Zero value means "fall back to the StreamManager writer".
	output outputContext
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

// validateSessionSwitch rejects USE/DETACH while a transaction or batch is
// active. Switching sessions would silently abandon the transaction (its locks
// would persist until client teardown) and drop any buffered batch statements.
func (h *SessionHandler) validateSessionSwitch() error {
	if h.Session == nil {
		return nil
	}
	if h.txn.InTransaction() {
		return errors.New("cannot switch session while a transaction is active; COMMIT, ROLLBACK, or CLOSE it first")
	}
	if h.batch.IsActive() {
		return errors.New("cannot switch session while a batch is active; RUN BATCH or ABORT BATCH first")
	}
	return nil
}

// switchSession mutates the single live systemVariables instance in place and
// builds a replacement session around it. The registry holds pointers into this
// struct and other components (Cli, StreamManager consumers) hold the same
// pointer, so copying the struct would fork the state they read (split-brain).
// On any failure the connection fields and the inTransaction hook (which
// newSessionWithFactories points at the new session) are restored.
func (h *SessionHandler) switchSession(ctx context.Context, database, role string, validate func(*Session) error) (*Result, error) {
	if err := h.validateSessionSwitch(); err != nil {
		return nil, err
	}

	sysVars := h.systemVariables
	oldDatabase, oldRole := sysVars.Connection.Database, sysVars.Connection.Role
	oldInTransaction := sysVars.inTransaction
	sysVars.Connection.Database = database
	sysVars.Connection.Role = role

	restore := func() {
		sysVars.Connection.Database = oldDatabase
		sysVars.Connection.Role = oldRole
		sysVars.inTransaction = oldInTransaction
	}

	newSession, err := h.createSessionWithOpts(ctx, sysVars)
	if err != nil {
		restore()
		return nil, err
	}

	if validate != nil {
		if err := validate(newSession); err != nil {
			newSession.Close()
			restore()
			return nil, err
		}
	}

	// Replace the old session with the new one
	h.Session.Close()
	h.Session = newSession

	return &Result{}, nil
}

func (h *SessionHandler) handleUse(ctx context.Context, s *UseStatement) (*Result, error) {
	return h.switchSession(ctx, s.Database, s.Role, func(newSession *Session) error {
		exists, err := newSession.DatabaseExists(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("unknown database %q", s.Database)
		}
		return nil
	})
}

func (h *SessionHandler) handleDetach(ctx context.Context, s *DetachStatement) (*Result, error) {
	// Clear database and role to switch to detached mode
	return h.switchSession(ctx, "", "", nil)
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
	clientConfig := clientConfigForSystemVariables(sysVars)
	clientConfig.DatabaseRole = sysVars.Connection.Role
	clientConfig.DirectedReadOptions = sysVars.Query.DirectedRead

	if sysVars.Config.Insecure && len(sysVars.Config.EmbeddedClientOptions) == 0 {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	if sysVars.Config.LogGrpc {
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
	sysVars.inTransaction = session.txn.InTransaction

	return session, nil
}

func NewAdminSession(ctx context.Context, sysVars *systemVariables, opts ...option.ClientOption) (*Session, error) {
	clientConfig := clientConfigForSystemVariables(sysVars)
	clientConfig.DatabaseRole = sysVars.Connection.Role
	clientConfig.DirectedReadOptions = sysVars.Query.DirectedRead

	if sysVars.Config.Insecure && len(sysVars.Config.EmbeddedClientOptions) == 0 {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	if sysVars.Config.LogGrpc {
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
	sysVars.inTransaction = session.txn.InTransaction

	// Validate instance exists
	exists, err := session.InstanceExists(ctx)
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

	if s.bqClient != nil {
		if err := s.bqClient.Close(); err != nil {
			slog.Error("error on bqClient.Close()", "err", err)
		}
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

// InstanceExists reports whether the configured Spanner instance is reachable.
// The caller's ctx is honored (including cancellation); a 30s timeout is layered
// on top as an upper bound so a hung RPC cannot block indefinitely.
func (s *Session) InstanceExists(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	// Method 1: Try listing databases (databases.list) first
	// This works for users with database-level permissions and uses the already available adminClient
	dbIter := s.adminClient.ListDatabases(ctx, &adminpb.ListDatabasesRequest{
		Parent:   s.InstancePath(),
		PageSize: 1, // Only check if instance is accessible
	})

	// Try to get the first item from iterator
	_, listErr := dbIter.Next()

	if listErr == nil {
		// Successfully got at least one database, instance exists
		return true, nil
	}

	// Check if it's an iterator.Done error (no databases but instance exists)
	if listErr == iterator.Done {
		return true, nil
	}

	// If the failure is caused by context cancellation or deadline (either the
	// caller's ctx or the layered 30s bound), short-circuit and return that
	// error directly. Method 2 would only issue another RPC on the same dead
	// ctx and mislabel a cancellation as "checking instance existence failed".
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if status.Code(listErr) == codes.Canceled || status.Code(listErr) == codes.DeadlineExceeded {
		return false, listErr
	}

	switch status.Code(listErr) {
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
		// instances.get was never attempted here because the admin client could
		// not be created. Surface both the original databases.list error (listErr)
		// and the client-creation error via errors.Join so the message reflects
		// reality (do not claim instances.get was tried).
		return false, fmt.Errorf("checking instance existence failed: spanner.databases.list failed and the instance admin client could not be created: %w", errors.Join(listErr, err))
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
		// Wrap with %w to preserve the gRPC status so downstream printError can
		// still extract the status code and details instead of a flattened string.
		return false, fmt.Errorf("checking instance existence failed: %w", err)
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
		QueryWithOptions(ctx, stmt, spanner.QueryOptions{Priority: s.txn.currentPriorityWithLock()})
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
		// Wrap with %w to preserve the gRPC status for downstream printError.
		return false, fmt.Errorf("checking database existence failed: %w", err)
	}
}

// RecreateClient closes the current client and creates a new client for the session.
// The caller's ctx governs client creation so cancellation is respected.
func (s *Session) RecreateClient(ctx context.Context) error {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return err
	}

	c, err := spanner.NewClientWithConfig(ctx, s.DatabasePath(), s.clientConfig, s.clientOpts...)
	if err != nil {
		return err
	}
	// Refuse the swap if a transaction context is still live: SetClient enforces
	// the lifecycle invariant that the old client must not be closed while a
	// transaction (and its heartbeat goroutine) could still use it. On refusal,
	// close the NEW client and surface the error (it joins the Aborted error at
	// the caller). Only on success close the OLD client and update Session.client.
	if err := s.txn.SetClient(c); err != nil {
		c.Close()
		return err
	}
	s.client.Close()
	s.client = c
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
	// SET LOCAL values revert when the transaction ends for any reason:
	// COMMIT/ROLLBACK/CLOSE statements, or automatic rollback on statement error.
	// All those paths funnel through here, so one post-statement check suffices.
	defer func() {
		if s.txn != nil {
			s.txn.restoreLocalVarsIfIdle()
		}
	}()
	if _, ok := stmt.(MutationStatement); ok {
		result := &Result{}
		if err := s.failStatementIfReadOnly(); err != nil {
			return result, err
		}
		if _, err := s.txn.DetermineTransaction(ctx); err != nil {
			return result, err
		}
		return stmt.Execute(ctx, s)
	}

	// Conditionally mutating statements (e.g. mutating CQL) participate in the
	// READONLY guard but do not determine a pending Spanner transaction, since
	// they do not use the Spanner transaction machinery.
	if cm, ok := stmt.(ConditionallyMutatingStatement); ok && cm.isConditionallyMutating() {
		if err := s.failStatementIfReadOnly(); err != nil {
			return &Result{}, err
		}
	}

	return stmt.Execute(ctx, s)
}

// createAuthClientOptions builds credential-related client options.
// When allowWithoutAuthentication is false, Spanner emulator auth
// (WithoutAuthentication) is skipped so non-Spanner clients (e.g. BigQuery) use
// real credentials instead.
func createAuthClientOptions(ctx context.Context, credential []byte, sysVars *systemVariables, allowWithoutAuthentication bool) ([]option.ClientOption, error) {
	switch {
	case allowWithoutAuthentication && sysVars.Config.WithoutAuthentication:
		return []option.ClientOption{option.WithoutAuthentication()}, nil
	case sysVars.Config.EnableADCPlus:
		source, err := tokensource.SmartAccessTokenSource(ctx, adcplus.WithCredentialsJSON(credential), adcplus.WithTargetPrincipal(sysVars.Config.ImpersonateServiceAccount))
		if err != nil {
			return nil, err
		}
		return []option.ClientOption{option.WithTokenSource(source)}, nil
	case len(credential) > 0:
		opt, err := credentialsJSONOption(credential)
		if err != nil {
			return nil, err
		}
		return []option.ClientOption{opt}, nil
	default:
		return nil, nil
	}
}

// createClientOptions creates client options based on credential and system variables
func createClientOptions(ctx context.Context, credential []byte, sysVars *systemVariables) ([]option.ClientOption, error) {
	if len(sysVars.Config.EmbeddedClientOptions) > 0 {
		return append([]option.ClientOption(nil), sysVars.Config.EmbeddedClientOptions...), nil
	}

	var opts []option.ClientOption
	if sysVars.Config.Host != "" && sysVars.Config.Port != 0 {
		// Reconstruct the endpoint, adding brackets back for IPv6 addresses
		endpoint := net.JoinHostPort(sysVars.Config.Host, strconv.Itoa(sysVars.Config.Port))
		opts = append(opts, option.WithEndpoint(endpoint))
	}

	authOpts, err := createAuthClientOptions(ctx, credential, sysVars, true)
	if err != nil {
		return nil, err
	}
	return append(opts, authOpts...), nil
}

// createBigQueryClientOptions creates auth options for BigQuery without Spanner
// emulator endpoint or WithoutAuthentication settings.
func createBigQueryClientOptions(ctx context.Context, credential []byte, sysVars *systemVariables) ([]option.ClientOption, error) {
	return createAuthClientOptions(ctx, credential, sysVars, false)
}

func bigQueryProject(sysVars *systemVariables) string {
	if p := sysVars.Feature.BigQueryProject; p != "" {
		return p
	}
	return sysVars.Connection.Project
}

// bigQueryClientKey identifies the effective BigQuery client configuration.
// A cached client is reused only while these values are unchanged; a SET of
// CLI_BIGQUERY_PROJECT/CLI_BIGQUERY_LOCATION produces a new key and forces a
// rebuild.
type bigQueryClientKey struct {
	project  string
	location string
}

// bigQueryClient returns a BigQuery client for the current
// CLI_BIGQUERY_PROJECT/CLI_BIGQUERY_LOCATION, building auth options lazily on
// first use. Deferring option construction until the first BIGQUERY statement
// keeps BigQuery credential resolution (ADC/ADCPlus/impersonation) out of
// ordinary Spanner/emulator session creation, which must succeed without any
// BigQuery credentials.
//
// The check-then-act on s.bqClient below is intentionally lock-free: BIGQUERY
// statements run on the single-threaded statement execution path (same
// rationale as cqlSession), so no other goroutine reads or mutates these
// fields concurrently.
func (s *Session) bigQueryClient(ctx context.Context) (*bigquery.Client, error) {
	project := bigQueryProject(s.systemVariables)
	if project == "" {
		return nil, fmt.Errorf("BigQuery project not configured: set CLI_BIGQUERY_PROJECT or CLI_PROJECT")
	}
	location := s.systemVariables.Feature.BigQueryLocation
	key := bigQueryClientKey{project: project, location: location}

	if s.bqClient != nil {
		if s.bqClientKey == key {
			return s.bqClient, nil
		}
		// CLI_BIGQUERY_PROJECT/LOCATION changed since the client was built;
		// drop the stale client and rebuild for the new configuration.
		if err := s.bqClient.Close(); err != nil {
			slog.Warn("error on bigquery.Client.Close() during reconfigure", "err", err)
		}
		s.bqClient = nil
		s.bqClientKey = bigQueryClientKey{}
	}

	opts, err := createBigQueryClientOptions(ctx, s.bqCredential, s.systemVariables)
	if err != nil {
		return nil, err
	}
	client, err := bigquery.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, err
	}
	if location != "" {
		client.Location = location
	}
	s.bqClient = client
	s.bqClientKey = key
	return client, nil
}

func credentialsJSONOption(credential []byte) (option.ClientOption, error) {
	var metadata struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(credential, &metadata); err != nil {
		return nil, fmt.Errorf("parse credential type: %w", err)
	}
	if metadata.Type == "" {
		return nil, errors.New("credential JSON missing type")
	}

	switch metadata.Type {
	case string(option.ServiceAccount):
		return option.WithAuthCredentialsJSON(option.ServiceAccount, credential), nil
	case string(option.AuthorizedUser):
		return option.WithAuthCredentialsJSON(option.AuthorizedUser, credential), nil
	case string(option.ImpersonatedServiceAccount):
		return option.WithAuthCredentialsJSON(option.ImpersonatedServiceAccount, credential), nil
	case string(option.ExternalAccount):
		return option.WithAuthCredentialsJSON(option.ExternalAccount, credential), nil
	default:
		return nil, fmt.Errorf("unsupported credential type %q", metadata.Type)
	}
}

func createSession(ctx context.Context, credential []byte, sysVars *systemVariables) (*Session, error) {
	opts, err := createClientOptions(ctx, credential, sysVars)
	if err != nil {
		return nil, err
	}

	var session *Session
	// Create admin-only session if no database is specified
	if sysVars.Connection.Database == "" {
		session, err = NewAdminSession(ctx, sysVars, opts...)
	} else {
		session, err = NewSession(ctx, sysVars, opts...)
	}
	if err != nil {
		return nil, err
	}
	// Retain the raw credential for lazy BigQuery client construction on the
	// first BIGQUERY statement; no BigQuery auth is resolved here.
	session.bqCredential = credential
	return session, nil
}
