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

	"github.com/apstndb/go-grpcinterceptors/selectlogging"

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
}

type transactionContext struct {
	tag           string
	priority      sppb.RequestOptions_Priority
	sendHeartbeat bool // Becomes true only after a user-driven query is executed on the transaction.
	rwTxn         *spanner.ReadWriteStmtBasedTransaction
	roTxn         *spanner.ReadOnlyTransaction
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
	go session.startHeartbeat()

	return session, nil
}

// InReadWriteTransaction returns true if the session is running read-write transaction.
func (s *Session) InReadWriteTransaction() bool {
	return s.tc != nil && s.tc.rwTxn != nil
}

// InReadOnlyTransaction returns true if the session is running read-only transaction.
func (s *Session) InReadOnlyTransaction() bool {
	return s.tc != nil && s.tc.roTxn != nil
}

// BeginReadWriteTransaction starts read-write transaction.
func (s *Session) BeginReadWriteTransaction(ctx context.Context, priority sppb.RequestOptions_Priority, tag string) error {
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
		tag:      tag,
		priority: priority,
		rwTxn:    txn,
	}
	return nil
}

// CommitReadWriteTransaction commits read-write transaction and returns commit timestamp if successful.
func (s *Session) CommitReadWriteTransaction(ctx context.Context) (spanner.CommitResponse, error) {
	if !s.InReadWriteTransaction() {
		return spanner.CommitResponse{}, errors.New("read-write transaction is not running")
	}

	s.tcMutex.Lock()
	defer s.tcMutex.Unlock()

	resp, err := s.tc.rwTxn.CommitWithReturnResp(ctx)
	s.tc = nil
	return resp, err
}

// RollbackReadWriteTransaction rollbacks read-write transaction.
func (s *Session) RollbackReadWriteTransaction(ctx context.Context) error {
	if !s.InReadWriteTransaction() {
		return errors.New("read-write transaction is not running")
	}

	s.tcMutex.Lock()
	defer s.tcMutex.Unlock()

	s.tc.rwTxn.Rollback(ctx)
	s.tc = nil
	return nil
}

// BeginReadOnlyTransaction starts read-only transaction and returns the snapshot timestamp for the transaction if successful.
func (s *Session) BeginReadOnlyTransaction(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority, tag string) (time.Time, error) {
	if s.InReadOnlyTransaction() {
		return time.Time{}, errors.New("read-only transaction is already running")
	}

	txn := s.client.ReadOnlyTransaction()
	switch typ {
	case strong:
		txn = txn.WithTimestampBound(spanner.StrongRead())
	case exactStaleness:
		txn = txn.WithTimestampBound(spanner.ExactStaleness(staleness))
	case readTimestamp:
		txn = txn.WithTimestampBound(spanner.ReadTimestamp(timestamp))
	}

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
		tag:      tag,
		priority: priority,
		roTxn:    txn,
	}

	return txn.Timestamp()
}

// CloseReadOnlyTransaction closes a running read-only transaction.
func (s *Session) CloseReadOnlyTransaction() error {
	if !s.InReadOnlyTransaction() {
		return errors.New("read-only transaction is not running")
	}

	s.tc.roTxn.Close()
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

	switch {
	case s.InReadWriteTransaction():
		// The current Go Spanner client library does not apply client-level directed read options to read-write transactions.
		// Therefore, we explicitly set query-level options here to fail the query during a read-write transaction.
		opts.DirectedReadOptions = s.clientConfig.DirectedReadOptions
		opts.RequestTag = s.tc.tag
		iter := s.tc.rwTxn.QueryWithOptions(ctx, stmt, opts)
		s.tc.sendHeartbeat = true
		return iter, nil
	case s.InReadOnlyTransaction():
		opts.RequestTag = s.tc.tag
		return s.tc.roTxn.QueryWithOptions(ctx, stmt, opts), s.tc.roTxn
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
// useUpdate flag enforce to use Update function internally and disable `THEN RETURN` result printing.
func (s *Session) RunUpdate(ctx context.Context, stmt spanner.Statement, useUpdate bool) ([]Row, []string, int64, *sppb.ResultSetMetadata, error) {
	logParseStatement(stmt.SQL)

	if !s.InReadWriteTransaction() {
		return nil, nil, 0, nil, errors.New("read-write transaction is not running")
	}

	opts := spanner.QueryOptions{
		Priority:   s.currentPriority(),
		RequestTag: s.tc.tag,
	}

	// Workaround: Usually, we can execute DMLs using Query(ExecuteStreamingSql RPC),
	// but spannertest doesn't support DMLs execution using ExecuteStreamingSql RPC.
	// It enforces to use ExecuteSql RPC.
	// TODO: Remove useUpdate flag
	if useUpdate {
		rowCount, err := s.tc.rwTxn.UpdateWithOptions(ctx, stmt, opts)
		s.tc.sendHeartbeat = true
		return nil, nil, rowCount, nil, err
	}

	rows, _, count, metadata, _, err := consumeRowIterCollect(s.tc.rwTxn.QueryWithOptions(ctx, stmt, opts), spannerRowToRow)
	s.tc.sendHeartbeat = true
	return rows, extractColumnNames(metadata.GetRowType().GetFields()), count, metadata, err
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
			if s.tc != nil && s.tc.rwTxn != nil && s.tc.sendHeartbeat {
				err := heartbeat(s.tc.rwTxn, s.currentPriority())
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
	var implicitRWTx bool
	if !s.InReadWriteTransaction() {
		// Start implicit transaction.
		if err := s.BeginReadWriteTransaction(ctx, s.currentPriority(), ""); err != nil {
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
