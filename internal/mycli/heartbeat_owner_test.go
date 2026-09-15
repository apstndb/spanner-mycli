// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/option"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const heartbeatSQL = "SELECT 1"

type heartbeatRecord struct {
	txnID    string
	sql      string
	reqTag   string
	priority sppb.RequestOptions_Priority
}

type sqlObservation struct {
	sql         string
	reqTag      string
	txnID       string
	readOnly    bool
	hadReadTs   bool
	queryMode   sppb.ExecuteSqlRequest_QueryMode
	params      map[string]string
	priority    sppb.RequestOptions_Priority
	optimizer   string
	rpc         string
	partitioned bool
	deadline    time.Time
	hasDeadline bool
}

type beginObservation struct {
	txnID  string
	txnTag string
}

type batchDMLObservation struct {
	txnID             string
	sqls              []string
	hasDeadline       bool
	deadlineRemaining time.Duration
}

type commitObservation struct {
	txnID    string
	priority sppb.RequestOptions_Priority
}

type heartbeatRPCServer struct {
	sppb.UnimplementedSpannerServer

	mu                        sync.Mutex
	next                      atomic.Uint64
	sqlTxn                    map[string]string
	roIDs                     map[string]struct{}
	rwIDs                     map[string]struct{}
	heartbeats                []heartbeatRecord
	sqlObs                    []sqlObservation
	batchObs                  []batchDMLObservation
	begins                    []beginObservation
	rollbacks                 []string
	commits                   []string
	commitObs                 []commitObservation
	failROQuery               error
	failSQL                   error
	failStreamingSQL          error
	failStreamingSQLLeft      int
	failStreamingSQLAfterRows int
	streamingResumeTokens     bool
	failUnarySQL              error
	failCommit                error
	failCommitLeft            int
	pdmlIDs                   map[string]struct{}
	failBatchDML              error
	failBatchDMLLeft          int
	failBegin                 error
	blockBegin                <-chan struct{}
	beginBlocked              func()
	partialBatchDMLCount      int64
	partialBatchDMLStatus     *statuspb.Status
	sqlRowCount               map[string]int64
	sqlValue                  map[string]string
	sqlRows                   map[string][]string
	sqlCallN                  map[string]int
	sqlRowCountSeq            map[string][]int64
	sqlRowsSeq                map[string][][]string
	sqlType                   map[string]sppb.TypeCode
	queryPlan                 *sppb.QueryPlan

	heartbeatStarted     chan struct{}
	heartbeatStartedOnce sync.Once
	blockHeartbeat       <-chan struct{}

	blockSQL   <-chan struct{}
	skipSQL    int
	sqlBlocked func()

	blockBatchDML <-chan struct{}
	batchBlocked  func()
}

func (s *heartbeatRPCServer) newTxnID() []byte {
	n := s.next.Add(1)
	return fmt.Appendf(nil, "hb-txn-%d", n)
}

func (s *heartbeatRPCServer) txnIDFor(r *sppb.ExecuteSqlRequest) []byte {
	sel := r.GetTransaction()
	if id := sel.GetId(); len(id) > 0 {
		return slices.Clone(id)
	}
	id := s.newTxnID()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noteSelectorLocked(id, sel)
	return id
}

func (s *heartbeatRPCServer) ensureTxnMapsLocked() {
	if s.roIDs == nil {
		s.roIDs = make(map[string]struct{})
	}
	if s.rwIDs == nil {
		s.rwIDs = make(map[string]struct{})
	}
}

func (s *heartbeatRPCServer) noteSelectorLocked(id []byte, sel *sppb.TransactionSelector) {
	if sel == nil {
		return
	}
	opts := sel.GetBegin()
	if opts == nil {
		opts = sel.GetSingleUse()
	}
	if opts == nil {
		return
	}
	s.ensureTxnMapsLocked()
	key := string(id)
	switch {
	case opts.GetReadOnly() != nil:
		s.roIDs[key] = struct{}{}
	case opts.GetReadWrite() != nil:
		s.rwIDs[key] = struct{}{}
	}
	if opts.GetPartitionedDml() != nil {
		if s.pdmlIDs == nil {
			s.pdmlIDs = make(map[string]struct{})
		}
		s.pdmlIDs[key] = struct{}{}
	}
}

func (s *heartbeatRPCServer) isReadOnlyLocked(r *sppb.ExecuteSqlRequest, txnID []byte) bool {
	sel := r.GetTransaction()
	if sel.GetBegin().GetReadOnly() != nil || sel.GetSingleUse().GetReadOnly() != nil {
		return true
	}
	if sel.GetBegin().GetReadWrite() != nil || sel.GetSingleUse().GetReadWrite() != nil {
		return false
	}
	_, ok := s.roIDs[string(txnID)]
	return ok
}

func (s *heartbeatRPCServer) setFailROQuery(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failROQuery = err
}

func (s *heartbeatRPCServer) setFailSQL(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failSQL = err
}

func (s *heartbeatRPCServer) setFailStreamingSQL(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failStreamingSQL = err
	s.failStreamingSQLLeft = 0
}

func (s *heartbeatRPCServer) setFailStreamingSQLTimes(n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failStreamingSQL = err
	s.failStreamingSQLLeft = n
	s.failStreamingSQLAfterRows = 0
}

func (s *heartbeatRPCServer) setFailStreamingSQLAfterRows(after int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failStreamingSQL = err
	s.failStreamingSQLLeft = 1
	s.failStreamingSQLAfterRows = after
}

func (s *heartbeatRPCServer) setStreamingResumeTokens(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamingResumeTokens = enabled
}

func (s *heartbeatRPCServer) setFailUnarySQL(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failUnarySQL = err
}

func (s *heartbeatRPCServer) setFailCommit(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCommit = err
	s.failCommitLeft = 0
}

func (s *heartbeatRPCServer) setFailCommitTimes(n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCommit = err
	s.failCommitLeft = n
}

func (s *heartbeatRPCServer) setSQLRowCount(sql string, n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlRowCount == nil {
		s.sqlRowCount = make(map[string]int64)
	}
	s.sqlRowCount[sql] = n
}

func (s *heartbeatRPCServer) setSQLValue(sql, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlValue == nil {
		s.sqlValue = make(map[string]string)
	}
	s.sqlValue[sql] = value
}

func (s *heartbeatRPCServer) setSQLRows(sql string, values []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlRows == nil {
		s.sqlRows = make(map[string][]string)
	}
	s.sqlRows[sql] = slices.Clone(values)
}

func (s *heartbeatRPCServer) setQueryPlan(plan *sppb.QueryPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queryPlan = plan
}

func (s *heartbeatRPCServer) setSQLRowCountSequence(sql string, counts ...int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlRowCountSeq == nil {
		s.sqlRowCountSeq = make(map[string][]int64)
	}
	s.sqlRowCountSeq[sql] = slices.Clone(counts)
}

func (s *heartbeatRPCServer) setSQLRowsSequence(sql string, rows ...[]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlRowsSeq == nil {
		s.sqlRowsSeq = make(map[string][][]string)
	}
	cloned := make([][]string, len(rows))
	for i, r := range rows {
		cloned[i] = slices.Clone(r)
	}
	s.sqlRowsSeq[sql] = cloned
}

func (s *heartbeatRPCServer) setSQLType(sql string, code sppb.TypeCode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlType == nil {
		s.sqlType = make(map[string]sppb.TypeCode)
	}
	s.sqlType[sql] = code
}

func (s *heartbeatRPCServer) rollbackIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.rollbacks)
}

func (s *heartbeatRPCServer) commitIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.commits)
}

func (s *heartbeatRPCServer) commitObservations() []commitObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.commitObs)
}

func (s *heartbeatRPCServer) setFailBatchDML(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failBatchDML = err
	s.failBatchDMLLeft = 0
}

func (s *heartbeatRPCServer) setFailBatchDMLTimes(n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failBatchDML = err
	s.failBatchDMLLeft = n
}

func consumeLimitedFailLocked(left *int, sticky *error) error {
	if sticky == nil || *sticky == nil {
		return nil
	}
	if *left > 0 {
		*left--
		err := *sticky
		if *left == 0 {
			*sticky = nil
		}
		return err
	}
	return *sticky
}

// setPartialBatchDML fakes a successful prefix ResultSet plus a non-OK
// embedded ExecuteBatchDmlResponse.Status. The Go client returns the prefix
// counts with a statement error; this is not a transport-level RPC failure.
func (s *heartbeatRPCServer) setPartialBatchDML(prefixRowCount int64, st *statuspb.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.partialBatchDMLCount = prefixRowCount
	s.partialBatchDMLStatus = st
	s.failBatchDML = nil
}

func (s *heartbeatRPCServer) batchObservations() []batchDMLObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]batchDMLObservation, len(s.batchObs))
	for i, obs := range s.batchObs {
		out[i] = obs
		out[i].sqls = slices.Clone(obs.sqls)
	}
	return out
}

func (s *heartbeatRPCServer) setBlockBatchDML(ch <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockBatchDML = ch
}

func (s *heartbeatRPCServer) setBatchBlocked(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchBlocked = fn
}

func (s *heartbeatRPCServer) sqlObservations() []sqlObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sqlObservation, len(s.sqlObs))
	for i, obs := range s.sqlObs {
		out[i] = obs
		if obs.params != nil {
			out[i].params = maps.Clone(obs.params)
		}
	}
	return out
}

func (s *heartbeatRPCServer) beginObservations() []beginObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.begins)
}

func lastSQLObservation(obs []sqlObservation, sql string) (sqlObservation, bool) {
	for i := len(obs) - 1; i >= 0; i-- {
		if obs[i].sql == sql {
			return obs[i], true
		}
	}
	return sqlObservation{}, false
}

func (s *heartbeatRPCServer) noteSQL(r *sppb.ExecuteSqlRequest, txnID []byte) {
	rec := heartbeatRecord{
		txnID:    string(txnID),
		sql:      r.GetSql(),
		reqTag:   r.GetRequestOptions().GetRequestTag(),
		priority: r.GetRequestOptions().GetPriority(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlTxn == nil {
		s.sqlTxn = make(map[string]string)
	}
	s.sqlTxn[r.GetSql()] = rec.txnID
	if rec.reqTag == "spanner_mycli_heartbeat" {
		s.heartbeats = append(s.heartbeats, rec)
		s.heartbeatStartedOnce.Do(func() {
			if s.heartbeatStarted != nil {
				close(s.heartbeatStarted)
			}
		})
	}
}

func (s *heartbeatRPCServer) txnIDForSQL(sql string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sqlTxn[sql]
}

func (s *heartbeatRPCServer) heartbeatIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, len(s.heartbeats))
	for i, rec := range s.heartbeats {
		ids[i] = rec.txnID
	}
	return ids
}

func (s *heartbeatRPCServer) heartbeatRecords() []heartbeatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.heartbeats)
}

func (s *heartbeatRPCServer) CreateSession(_ context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Database + "/sessions/heartbeat", Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *heartbeatRPCServer) BatchCreateSessions(_ context.Context, r *sppb.BatchCreateSessionsRequest) (*sppb.BatchCreateSessionsResponse, error) {
	n := int(r.SessionCount)
	if n <= 0 {
		n = 1
	}
	sessions := make([]*sppb.Session, n)
	for i := range n {
		sessions[i] = &sppb.Session{
			Name:       fmt.Sprintf("%s/sessions/heartbeat-%d", r.Database, i),
			CreateTime: timestamppb.Now(),
		}
	}
	return &sppb.BatchCreateSessionsResponse{Session: sessions}, nil
}

func (s *heartbeatRPCServer) GetSession(_ context.Context, r *sppb.GetSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Name, Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *heartbeatRPCServer) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *heartbeatRPCServer) setBlockSQL(ch <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockSQL = ch
}

func (s *heartbeatRPCServer) setSkipSQL(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipSQL = n
}

func (s *heartbeatRPCServer) setSQLBlocked(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sqlBlocked = fn
}

func (s *heartbeatRPCServer) BeginTransaction(ctx context.Context, r *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	s.mu.Lock()
	block := s.blockBegin
	blocked := s.beginBlocked
	fail := s.failBegin
	s.mu.Unlock()
	if blocked != nil {
		blocked()
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if fail != nil {
		return nil, fail
	}
	id := s.newTxnID()
	txn := &sppb.Transaction{Id: id}
	s.mu.Lock()
	s.ensureTxnMapsLocked()
	s.begins = append(s.begins, beginObservation{
		txnID:  string(id),
		txnTag: r.GetRequestOptions().GetTransactionTag(),
	})
	if r.GetOptions().GetReadOnly() != nil {
		s.roIDs[string(id)] = struct{}{}
		txn.ReadTimestamp = timestamppb.Now()
	} else {
		s.rwIDs[string(id)] = struct{}{}
	}
	if r.GetOptions().GetPartitionedDml() != nil {
		if s.pdmlIDs == nil {
			s.pdmlIDs = make(map[string]struct{})
		}
		s.pdmlIDs[string(id)] = struct{}{}
	}
	s.mu.Unlock()
	return txn, nil
}

func (s *heartbeatRPCServer) Rollback(_ context.Context, r *sppb.RollbackRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	s.rollbacks = append(s.rollbacks, string(r.GetTransactionId()))
	s.mu.Unlock()
	return &emptypb.Empty{}, nil
}

func (s *heartbeatRPCServer) Commit(_ context.Context, r *sppb.CommitRequest) (*sppb.CommitResponse, error) {
	s.mu.Lock()
	s.commits = append(s.commits, string(r.GetTransactionId()))
	s.commitObs = append(s.commitObs, commitObservation{
		txnID:    string(r.GetTransactionId()),
		priority: r.GetRequestOptions().GetPriority(),
	})
	fail := consumeLimitedFailLocked(&s.failCommitLeft, &s.failCommit)
	s.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	return &sppb.CommitResponse{CommitTimestamp: timestamppb.Now()}, nil
}

func (s *heartbeatRPCServer) ExecuteBatchDml(ctx context.Context, r *sppb.ExecuteBatchDmlRequest) (*sppb.ExecuteBatchDmlResponse, error) {
	s.mu.Lock()
	block := s.blockBatchDML
	blocked := s.batchBlocked
	s.mu.Unlock()
	if blocked != nil {
		blocked()
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	var id []byte
	if existing := r.GetTransaction().GetId(); len(existing) > 0 {
		id = slices.Clone(existing)
	} else {
		id = s.newTxnID()
		s.mu.Lock()
		s.noteSelectorLocked(id, r.GetTransaction())
		s.mu.Unlock()
	}
	sqls := make([]string, 0, len(r.GetStatements()))
	for _, st := range r.GetStatements() {
		sqls = append(sqls, st.GetSql())
	}

	obs := batchDMLObservation{txnID: string(id), sqls: sqls}
	if dl, ok := ctx.Deadline(); ok {
		obs.hasDeadline = true
		obs.deadlineRemaining = time.Until(dl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchObs = append(s.batchObs, obs)
	if fail := consumeLimitedFailLocked(&s.failBatchDMLLeft, &s.failBatchDML); fail != nil {
		return nil, fail
	}
	if s.partialBatchDMLStatus != nil {
		return &sppb.ExecuteBatchDmlResponse{
			ResultSets: []*sppb.ResultSet{{
				Stats: &sppb.ResultSetStats{
					RowCount: &sppb.ResultSetStats_RowCountExact{RowCountExact: s.partialBatchDMLCount},
				},
			}},
			Status: s.partialBatchDMLStatus,
		}, nil
	}
	results := make([]*sppb.ResultSet, len(r.GetStatements()))
	for i := range results {
		sql := ""
		if i < len(sqls) {
			sql = sqls[i]
		}
		results[i] = &sppb.ResultSet{
			Stats: &sppb.ResultSetStats{
				RowCount: &sppb.ResultSetStats_RowCountExact{RowCountExact: s.rowCountLocked(sql)},
			},
		}
	}
	return &sppb.ExecuteBatchDmlResponse{ResultSets: results}, nil
}

func (s *heartbeatRPCServer) ExecuteSql(ctx context.Context, r *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	txnID, readTs, err := s.prepareSQL(ctx, r, "ExecuteSql")
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	fail := s.failUnarySQL
	s.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	return s.resultSet(txnID, readTs, r.GetSql()), nil
}

func (s *heartbeatRPCServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	txnID, readTs, err := s.prepareSQL(stream.Context(), r, "ExecuteStreamingSql")
	if err != nil {
		return err
	}
	isHeartbeat := r.GetRequestOptions().GetRequestTag() == "spanner_mycli_heartbeat"
	s.mu.Lock()
	afterRows := s.failStreamingSQLAfterRows
	var fail error
	if !isHeartbeat {
		fail = consumeLimitedFailLocked(&s.failStreamingSQLLeft, &s.failStreamingSQL)
		if fail != nil {
			s.failStreamingSQLAfterRows = 0
		}
	}
	useResume := s.streamingResumeTokens
	s.mu.Unlock()
	if fail != nil && afterRows <= 0 {
		return fail
	}
	rs := s.resultSet(txnID, readTs, r.GetSql())
	if len(rs.Rows) == 0 {
		if err := stream.Send(&sppb.PartialResultSet{Metadata: rs.Metadata, Stats: rs.Stats}); err != nil {
			return err
		}
		if fail != nil && r.GetRequestOptions().GetRequestTag() != "spanner_mycli_heartbeat" {
			return fail
		}
		return nil
	}
	for i, row := range rs.Rows {
		prs := &sppb.PartialResultSet{Values: row.GetValues()}
		if i == 0 {
			prs.Metadata = rs.Metadata
		}
		if i == len(rs.Rows)-1 {
			prs.Stats = rs.Stats
		}
		if useResume {
			prs.ResumeToken = []byte(fmt.Sprintf("resume-%d", i+1))
		}
		if err := stream.Send(prs); err != nil {
			return err
		}
		if fail != nil && afterRows > 0 && i+1 >= afterRows && !isHeartbeat {
			return fail
		}
	}
	if fail != nil && afterRows > len(rs.Rows) && r.GetRequestOptions().GetRequestTag() != "spanner_mycli_heartbeat" {
		return fail
	}
	return nil
}

func (s *heartbeatRPCServer) prepareSQL(ctx context.Context, r *sppb.ExecuteSqlRequest, rpc string) ([]byte, *timestamppb.Timestamp, error) {
	txnID := s.txnIDFor(r)
	s.mu.Lock()
	block := s.blockSQL
	if s.skipSQL > 0 && block != nil && r.GetRequestOptions().GetRequestTag() != "spanner_mycli_heartbeat" {
		s.skipSQL--
		block = nil
	}
	s.mu.Unlock()
	if block != nil && r.GetRequestOptions().GetRequestTag() != "spanner_mycli_heartbeat" {
		s.mu.Lock()
		blocked := s.sqlBlocked
		s.mu.Unlock()
		if blocked != nil {
			blocked()
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-block:
		}
	}
	s.noteSQL(r, txnID)
	if err := s.waitHeartbeatIfNeeded(ctx, r); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failSQL != nil {
		return nil, nil, s.failSQL
	}
	ro := s.isReadOnlyLocked(r, txnID)
	if ro && s.failROQuery != nil {
		return nil, nil, s.failROQuery
	}
	var readTs *timestamppb.Timestamp
	if ro {
		readTs = timestamppb.Now()
	}
	obs := sqlObservation{
		sql:         r.GetSql(),
		reqTag:      r.GetRequestOptions().GetRequestTag(),
		txnID:       string(txnID),
		readOnly:    ro,
		hadReadTs:   readTs != nil,
		queryMode:   r.GetQueryMode(),
		params:      paramStringValues(r.GetParams()),
		priority:    r.GetRequestOptions().GetPriority(),
		optimizer:   r.GetQueryOptions().GetOptimizerVersion(),
		rpc:         rpc,
		partitioned: s.isPartitionedLocked(r, txnID),
	}
	if dl, ok := ctx.Deadline(); ok {
		obs.deadline = dl
		obs.hasDeadline = true
	}
	s.sqlObs = append(s.sqlObs, obs)
	return txnID, readTs, nil
}

func (s *heartbeatRPCServer) isPartitionedLocked(r *sppb.ExecuteSqlRequest, txnID []byte) bool {
	sel := r.GetTransaction()
	if sel.GetBegin().GetPartitionedDml() != nil || sel.GetSingleUse().GetPartitionedDml() != nil {
		return true
	}
	_, ok := s.pdmlIDs[string(txnID)]
	return ok
}

func (s *heartbeatRPCServer) rowCountLocked(sql string) int64 {
	if s.sqlRowCount != nil {
		if n, ok := s.sqlRowCount[sql]; ok {
			return n
		}
	}
	return 1
}

func (s *heartbeatRPCServer) valueLocked(sql string) string {
	if s.sqlValue != nil {
		if v, ok := s.sqlValue[sql]; ok {
			return v
		}
	}
	return "1"
}

func (s *heartbeatRPCServer) rowValuesLocked(sql string) []string {
	if s.sqlRows != nil {
		if values, ok := s.sqlRows[sql]; ok {
			return values
		}
	}
	return []string{s.valueLocked(sql)}
}

func (s *heartbeatRPCServer) waitHeartbeatIfNeeded(ctx context.Context, r *sppb.ExecuteSqlRequest) error {
	if r.GetRequestOptions().GetRequestTag() != "spanner_mycli_heartbeat" || s.blockHeartbeat == nil {
		return nil
	}
	select {
	case <-s.blockHeartbeat:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *heartbeatRPCServer) resultSet(txnID []byte, readTs *timestamppb.Timestamp, sql string) *sppb.ResultSet {
	s.mu.Lock()
	if s.sqlCallN == nil {
		s.sqlCallN = make(map[string]int)
	}
	call := s.sqlCallN[sql]
	s.sqlCallN[sql] = call + 1
	count := s.rowCountLocked(sql)
	if seq := s.sqlRowCountSeq[sql]; len(seq) > 0 {
		i := min(call, len(seq)-1)
		count = seq[i]
	}
	values := s.rowValuesLocked(sql)
	if seq := s.sqlRowsSeq[sql]; len(seq) > 0 {
		i := min(call, len(seq)-1)
		values = slices.Clone(seq[i])
	}
	plan := s.queryPlan
	typ := sppb.TypeCode_INT64
	if code, ok := s.sqlType[sql]; ok {
		typ = code
	}
	s.mu.Unlock()
	rows := make([]*structpb.ListValue, len(values))
	for i, value := range values {
		rows[i] = &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue(value)}}
	}
	stats := &sppb.ResultSetStats{
		RowCount: &sppb.ResultSetStats_RowCountExact{RowCountExact: count},
	}
	if plan != nil {
		stats.QueryPlan = plan
	}
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "", Type: &sppb.Type{Code: typ}},
			}},
			Transaction: &sppb.Transaction{Id: txnID, ReadTimestamp: readTs},
		},
		Rows:  rows,
		Stats: stats,
	}
}

func paramStringValues(params *structpb.Struct) map[string]string {
	fields := params.GetFields()
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		out[k] = v.GetStringValue()
	}
	return out
}

type heartbeatHarness struct {
	tm         *TransactionManager
	server     *heartbeatRPCServer
	clientOpts []option.ClientOption
	ticks      chan time.Time

	arrived chan struct{}
	release chan struct{}
	attempt chan struct{}
	unblock chan struct{}
}

func newHeartbeatHarness(t *testing.T) *heartbeatHarness {
	t.Helper()
	server := &heartbeatRPCServer{
		heartbeatStarted: make(chan struct{}),
	}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, server)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///heartbeat",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	clientOpts := []option.ClientOption{option.WithGRPCConn(conn)}
	client, err := spanner.NewClientWithConfig(t.Context(), "projects/test/instances/test/databases/test",
		spanner.ClientConfig{DisableNativeMetrics: true}, clientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	sysVars := newSystemVariablesWithDefaultsForTest()
	tm := NewTransactionManager(client, sysVars, spanner.ClientConfig{DisableNativeMetrics: true})
	ticks := make(chan time.Time)
	h := &heartbeatHarness{
		tm:         tm,
		server:     server,
		clientOpts: clientOpts,
		ticks:      ticks,
		arrived:    make(chan struct{}),
		release:    make(chan struct{}),
		attempt:    make(chan struct{}, 8),
		unblock:    make(chan struct{}),
	}
	tm.heartbeatTicks = ticks
	t.Cleanup(func() {
		h.releaseBarriers()
		_ = tm.RollbackReadWriteTransaction(context.Background())
		tm.clearTransactionContext()
	})
	return h
}

func (h *heartbeatHarness) attachSessionClient(session *Session) {
	session.client = h.tm.client
	session.clientConfig = h.tm.clientConfig
	session.clientOpts = h.clientOpts
	session.connection = ConnectionVars{Project: "test", Instance: "test", Database: "test"}
}

func (h *heartbeatHarness) releaseBarriers() {
	select {
	case <-h.release:
	default:
		close(h.release)
	}
	select {
	case <-h.unblock:
	default:
		close(h.unblock)
	}
}

func (h *heartbeatHarness) installBeforeAcquireBarrier() {
	var once sync.Once
	h.tm.heartbeatBeforeAcquire = func() {
		once.Do(func() { close(h.arrived) })
		<-h.release
	}
	h.tm.heartbeatAfterAttempt = func() {
		select {
		case h.attempt <- struct{}{}:
		default:
		}
	}
}

func waitChan(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("timeout waiting for %s", what)
	case <-t.Context().Done():
		t.Fatalf("test cancelled waiting for %s: %v", what, t.Context().Err())
	}
}

func sendTick(t *testing.T, ticks chan<- time.Time) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case ticks <- time.Time{}:
	case <-timer.C:
		t.Fatal("timeout sending heartbeat tick")
	case <-t.Context().Done():
		t.Fatalf("test cancelled sending heartbeat tick: %v", t.Context().Err())
	}
}

func beginRWAndProbe(t *testing.T, ctx context.Context, h *heartbeatHarness, sql string) string {
	t.Helper()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("BeginReadWriteTransaction: %v", err)
	}
	iter, _, err := h.tm.RunQuery(ctx, spanner.NewStatement(sql))
	if err != nil {
		t.Fatalf("RunQuery(%s): %v", sql, err)
	}
	if _, _, _, _, err := consumeRowIterDiscard(iter); err != nil {
		t.Fatalf("drain %s: %v", sql, err)
	}
	if !h.tm.heartbeatEnabled() {
		t.Fatal("first user operation did not enable heartbeat")
	}
	id := h.server.txnIDForSQL(sql)
	if id == "" {
		t.Fatalf("no transaction id captured for %q; records=%v", sql, h.server.heartbeatRecords())
	}
	return id
}

func assertHeartbeatMeta(t *testing.T, recs []heartbeatRecord) {
	t.Helper()
	for _, rec := range recs {
		if rec.sql != heartbeatSQL {
			t.Errorf("heartbeat SQL = %q, want %q", rec.sql, heartbeatSQL)
		}
		if rec.reqTag != "spanner_mycli_heartbeat" {
			t.Errorf("heartbeat request tag = %q, want spanner_mycli_heartbeat", rec.reqTag)
		}
		if rec.priority != sppb.RequestOptions_PRIORITY_LOW {
			t.Errorf("heartbeat priority = %v, want PRIORITY_LOW", rec.priority)
		}
	}
}

func TestHeartbeatDelayedTickDoesNotBorrowReplacementOwner(t *testing.T) {
	t.Parallel()
	// #922: a tick that snapshots owner A, then A is rolled back and B is
	// begun, must not issue SELECT 1 on B. This schedule reproduced that
	// borrow before the owner-pointer check (red: heartbeat IDs included B).
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.installBeforeAcquireBarrier()

	idA := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_a")
	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "owner A eligibility snapshot")

	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback A: %v", err)
	}
	idB := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_b")
	if idA == idB {
		t.Fatal("replacement owner reused transaction id A")
	}

	close(h.release)
	waitChan(t, h.attempt, "owner A delayed acquire attempt")

	if got := h.server.heartbeatIDs(); slices.Contains(got, idB) {
		t.Fatalf("delayed A tick issued SELECT 1 on replacement owner B (%s); heartbeats=%v A=%s B=%s", got, got, idA, idB)
	}
	if got := h.server.heartbeatIDs(); slices.Contains(got, idA) {
		t.Fatalf("delayed A tick issued SELECT 1 after A ended; heartbeats=%v A=%s", got, idA)
	}

	sendTick(t, h.ticks)
	waitChan(t, h.attempt, "owner B heartbeat attempt")
	got := h.server.heartbeatIDs()
	if !slices.Contains(got, idB) {
		t.Fatalf("B heartbeat missing on B; heartbeats=%v B=%s", got, idB)
	}
	if slices.Contains(got, idA) {
		t.Fatalf("B heartbeat also hit A; heartbeats=%v A=%s B=%s", got, idA, idB)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestHeartbeatCancelledTickWithNoReplacementIssuesNoRequest(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.installBeforeAcquireBarrier()

	idA := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_a")
	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "owner A eligibility snapshot")
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback A: %v", err)
	}
	close(h.release)
	waitChan(t, h.attempt, "cancelled acquire attempt")
	if got := h.server.heartbeatIDs(); len(got) != 0 {
		t.Fatalf("cancelled A tick issued heartbeat %v on owner %s", got, idA)
	}
}

func TestHeartbeatInFlightAcquireStaysOnOriginalOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.server.blockHeartbeat = h.unblock
	h.tm.heartbeatAfterAttempt = func() {
		select {
		case h.attempt <- struct{}{}:
		default:
		}
	}

	idA := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_a")
	sendTick(t, h.ticks)
	waitChan(t, h.server.heartbeatStarted, "in-flight heartbeat RPC")

	// The server barrier proves the RPC is in flight. Probe its lock directly;
	// an immediate receive from a rollback goroutine only checks scheduling.
	if h.tm.mu.TryLock() {
		h.tm.mu.Unlock()
		t.Fatal("heartbeat released tm.mu while its RPC was in flight")
	}

	rolled := make(chan error, 1)
	go func() {
		rolled <- h.tm.RollbackReadWriteTransaction(ctx)
	}()

	close(h.unblock)
	waitChan(t, h.attempt, "in-flight heartbeat completion")
	select {
	case err := <-rolled:
		if err != nil {
			t.Fatalf("rollback A: %v", err)
		}
	case <-t.Context().Done():
		t.Fatal("timeout waiting for rollback after in-flight heartbeat")
	}

	got := h.server.heartbeatIDs()
	if !slices.Equal(got, []string{idA}) {
		t.Fatalf("in-flight heartbeat IDs = %v, want only A=%s", got, idA)
	}

	idB := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_b")
	sendTick(t, h.ticks)
	waitChan(t, h.attempt, "owner B heartbeat attempt")
	got = h.server.heartbeatIDs()
	if !slices.Equal(got, []string{idA, idB}) {
		t.Fatalf("heartbeats=%v, want A then B (%s, %s)", got, idA, idB)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestHeartbeatPendingActivationKeepsOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.installBeforeAcquireBarrier()

	if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("BeginPendingTransaction: %v", err)
	}
	pending := txnContext(h.tm)
	if _, err := h.tm.DetermineTransaction(ctx); err != nil {
		t.Fatalf("pending activation: %v", err)
	}
	if txnContext(h.tm) != pending {
		t.Fatal("pending activation replaced heartbeat owner identity")
	}

	iter, _, err := h.tm.RunQuery(ctx, spanner.NewStatement("SELECT 1 AS owner_a"))
	if err != nil {
		t.Fatalf("RunQuery: %v", err)
	}
	if _, _, _, _, err := consumeRowIterDiscard(iter); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if !h.tm.heartbeatEnabled() {
		t.Fatal("first user operation did not enable heartbeat")
	}
	idA := h.server.txnIDForSQL("SELECT 1 AS owner_a")
	if idA == "" {
		t.Fatalf("no transaction id captured; records=%v", h.server.heartbeatRecords())
	}

	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "owner A eligibility snapshot")

	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback A: %v", err)
	}
	idB := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_b")
	if idA == idB {
		t.Fatal("replacement owner reused transaction id A")
	}
	if txnContext(h.tm) == pending {
		t.Fatal("replacement owner reused pending identity A")
	}

	close(h.release)
	waitChan(t, h.attempt, "owner A delayed acquire attempt")

	if got := h.server.heartbeatIDs(); slices.Contains(got, idB) {
		t.Fatalf("delayed A tick issued SELECT 1 on replacement owner B (%s); heartbeats=%v A=%s B=%s", got, got, idA, idB)
	}
	if got := h.server.heartbeatIDs(); slices.Contains(got, idA) {
		t.Fatalf("delayed A tick issued SELECT 1 after A ended; heartbeats=%v A=%s", got, idA)
	}

	sendTick(t, h.ticks)
	waitChan(t, h.attempt, "owner B heartbeat attempt")
	got := h.server.heartbeatIDs()
	if !slices.Contains(got, idB) {
		t.Fatalf("B heartbeat missing on B; heartbeats=%v B=%s", got, idB)
	}
	if slices.Contains(got, idA) {
		t.Fatalf("B heartbeat also hit A; heartbeats=%v A=%s B=%s", got, idA, idB)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestHeartbeatReadTimestampFollowsTransactionMode(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()

	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("BeginReadWriteTransaction: %v", err)
	}
	iter, _, err := h.tm.RunQuery(ctx, spanner.NewStatement("SELECT 1"))
	if err != nil {
		t.Fatalf("RW SELECT 1: %v", err)
	}
	if _, _, _, _, err := consumeRowIterDiscard(iter); err != nil {
		t.Fatalf("drain RW SELECT 1: %v", err)
	}
	var sawRW bool
	for _, rec := range h.server.sqlObservations() {
		if rec.sql != "SELECT 1" || rec.reqTag == "spanner_mycli_heartbeat" {
			continue
		}
		if rec.readOnly {
			t.Fatal("ordinary RW SELECT 1 classified as read-only")
		}
		if rec.hadReadTs {
			t.Fatal("ordinary RW SELECT 1 received a read timestamp")
		}
		sawRW = true
	}
	if !sawRW {
		t.Fatal("ordinary RW SELECT 1 was not observed")
	}
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback RW: %v", err)
	}

	if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("pending RO activation: %v", err)
	}
	var sawRO bool
	for _, rec := range h.server.sqlObservations() {
		if !rec.readOnly {
			continue
		}
		if !rec.hadReadTs {
			t.Fatalf("RO query %q missing read timestamp", rec.sql)
		}
		sawRO = true
	}
	if !sawRO {
		t.Fatal("RO activation query was not observed")
	}
}
