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
	"log"
	"strings"
	"sync"
	"time"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/go-grpcinterceptors/selectlogging"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"google.golang.org/grpc/credentials/insecure"

	"google.golang.org/grpc"

	"cloud.google.com/go/spanner"
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

type Session struct {
	client          *spanner.Client
	adminClient     *adminapi.DatabaseAdminClient
	clientConfig    spanner.ClientConfig
	clientOpts      []option.ClientOption
	tc              *transactionContext
	tcMutex         sync.Mutex // Guard a critical section for transaction.
	systemVariables *systemVariables

	currentBatch Statement
}

type transactionMode string

const (
	transactionModePending   = "undetermined"
	transactionModeReadOnly  = "read-only"
	transactionModeReadWrite = "read-write"
)

type transactionContext struct {
	mode          transactionMode
	tag           string
	priority      sppb.RequestOptions_Priority
	sendHeartbeat bool // Becomes true only after a user-driven query is executed on the transaction.

	txn any
	// rwTxn         *spanner.ReadWriteStmtBasedTransaction
	// roTxn         *spanner.ReadOnlyTransaction
}

func (tc *transactionContext) RWTxn() *spanner.ReadWriteStmtBasedTransaction {
	if tc.mode != transactionModeReadWrite {
		panic(fmt.Sprintf("must be in read-write transaction, but: %v", tc.mode))
	}
	return tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
}

func (tc *transactionContext) ROTxn() *spanner.ReadOnlyTransaction {
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
	if tc.mode != transactionModeReadOnly && tc.mode != transactionModeReadWrite {
		panic(fmt.Sprintf("must be in transaction, but: %v", tc.mode))
	}
	return tc.txn.(transaction)
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
		client:          client,
		clientConfig:    clientConfig,
		clientOpts:      opts,
		adminClient:     adminClient,
		systemVariables: sysVars,
	}
	sysVars.CurrentSession = session
	go session.startHeartbeat()

	return session, nil
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

// BeginPendingTransaction starts pending transaction.
// The actual start of the transaction is delayed until the first operation in the transaction is executed.
func (s *Session) BeginPendingTransaction(ctx context.Context, priority sppb.RequestOptions_Priority) error {
	if s.InReadWriteTransaction() {
		return errors.New("read-write transaction is already running")
	}

	if s.InReadOnlyTransaction() {
		return errors.New("read-only transaction is already running")
	}

	// Use session's priority if transaction priority is not set.
	if priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		priority = s.systemVariables.RPCPriority
	}

	s.tc = &transactionContext{
		mode:     transactionModePending,
		priority: priority,
	}
	return nil
}

func (s *Session) DetermineTransaction(ctx context.Context) (time.Time, error) {
	var zeroTime time.Time
	if s.tc == nil || s.tc.mode != transactionModePending {
		return zeroTime, nil
	}

	if s.systemVariables.ReadOnly {
		return s.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, s.tc.priority)
	}

	return zeroTime, s.BeginReadWriteTransaction(ctx, s.tc.priority)
}

// BeginReadWriteTransaction starts read-write transaction.
func (s *Session) BeginReadWriteTransaction(ctx context.Context, priority sppb.RequestOptions_Priority) error {
	if s.InReadWriteTransaction() {
		return errors.New("read-write transaction is already running")
	}

	if s.InReadOnlyTransaction() {
		return errors.New("read-only transaction is already running")
	}

	var tag string
	if s.tc != nil && s.tc.mode == transactionModePending {
		tag = s.tc.tag
	}

	// Use session's priority if transaction priority is not set.
	if priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		priority = s.systemVariables.RPCPriority
	}

	opts := spanner.TransactionOptions{
		CommitOptions:  spanner.CommitOptions{ReturnCommitStats: true},
		CommitPriority: priority,
		TransactionTag: tag,
	}

	txn, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, s.client, opts)
	if err != nil {
		return err
	}
	s.tc = &transactionContext{
		mode:     transactionModeReadWrite,
		tag:      tag,
		priority: priority,
		txn:      txn,
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

// BeginReadOnlyTransaction starts read-only transaction and returns the snapshot timestamp for the transaction if successful.
func (s *Session) BeginReadOnlyTransaction(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	if s.InReadOnlyTransaction() {
		return time.Time{}, errors.New("read-only transaction is already running")
	}

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

	txn := s.client.ReadOnlyTransaction().WithTimestampBound(tb)

	// Use session's priority if transaction priority is not set.
	if priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		priority = s.systemVariables.RPCPriority
	}

	// Because google-cloud-go/spanner defers calling BeginTransaction RPC until an actual query is run,
	// we explicitly run a "SELECT 1" query so that we can determine the timestamp of read-only transaction.
	opts := spanner.QueryOptions{Priority: priority}
	if _, _, _, _, err := consumeRowIterDiscard(txn.QueryWithOptions(ctx, spanner.NewStatement("SELECT 1"), opts)); err != nil {
		return time.Time{}, err
	}

	s.tc = &transactionContext{
		mode:     transactionModeReadOnly,
		priority: priority,
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
func (s *Session) RunQueryWithStats(ctx context.Context, stmt spanner.Statement) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	mode := sppb.ExecuteSqlRequest_PROFILE
	opts := spanner.QueryOptions{
		Mode:     &mode,
		Priority: s.currentPriority(),
	}
	return s.runQueryWithOptions(ctx, stmt, opts)
}

// RunQuery executes a statement either on the running transaction or on the temporal read-only transaction.
// It returns row iterator and read-only transaction if the statement was executed on the read-only transaction.
func (s *Session) RunQuery(ctx context.Context, stmt spanner.Statement) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	opts := spanner.QueryOptions{
		Priority: s.currentPriority(),
	}
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
	logParseStatement(stmt.SQL)

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
		txn := s.client.Single()
		if s.systemVariables.ReadOnlyStaleness != nil {
			txn = txn.WithTimestampBound(*s.systemVariables.ReadOnlyStaleness)
		}
		return txn.QueryWithOptions(ctx, stmt, opts), txn
	}
}

// RunUpdate executes a DML statement on the running read-write transaction.
// It returns error if there is no running read-write transaction.
func (s *Session) RunUpdate(ctx context.Context, stmt spanner.Statement) ([]Row, []string, int64, *sppb.ResultSetMetadata, error) {
	logParseStatement(stmt.SQL)

	if !s.InReadWriteTransaction() {
		return nil, nil, 0, nil, errors.New("read-write transaction is not running")
	}

	opts := s.queryOptions(nil)

	// Reset STATEMENT_TAG
	s.systemVariables.RequestTag = ""

	fc, err := formatConfigWithProto(s.systemVariables.ProtoDescriptor)
	if err != nil {
		return nil, nil, 0, nil, err
	}

	rows, _, count, metadata, _, err := consumeRowIterCollect(s.tc.RWTxn().QueryWithOptions(ctx, stmt, opts), spannerRowToRow(fc))
	s.tc.sendHeartbeat = true
	return rows, extractColumnNames(metadata.GetRowType().GetFields()), count, metadata, err
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
	s.client.Close()
	err := s.adminClient.Close()
	if err != nil {
		log.Printf("error on adminClient.Close(): %v", err)
	}
}

func (s *Session) DatabasePath() string {
	return s.systemVariables.DatabasePath()
}

func (s *Session) InstancePath() string {
	return s.systemVariables.InstancePath()
}

func (s *Session) DatabaseExists() (bool, error) {
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
					log.Printf("heartbeat error: %v", err)
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
	f func() (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (affected int64, commitResponse spanner.CommitResponse, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
	_, err = s.DetermineTransaction(ctx)
	if err != nil {
		return 0, spanner.CommitResponse{}, nil, nil, err
	}

	var implicitRWTx bool
	if !s.InReadWriteTransaction() {
		// Start implicit transaction.
		if err := s.BeginReadWriteTransaction(ctx, s.currentPriority()); err != nil {
			return 0, spanner.CommitResponse{}, nil, nil, err
		}
		implicitRWTx = true
	}

	affected, plan, metadata, err = f()
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

func (s *Session) failStatementIfReadOnly() error {
	if s.systemVariables.ReadOnly {
		return errors.New("can't execute this statement in READONLY mode")
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
