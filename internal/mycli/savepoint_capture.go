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
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/proto"
)

// captureToken identifies one admitted owner operation. Completion must
// present this exact token: a rejected or uncaptured path cannot finish
// another operation, owner, or physical attempt.
type captureToken struct {
	owner    *transactionContext
	attempt  uint64
	frozen   frozenStatement
	rec      *operationReceipt
	reserved int64
	dml      bool
}

type savepointAdmissionError struct {
	err error
}

func (e *savepointAdmissionError) Error() string {
	if e == nil || e.err == nil {
		return "savepoint admission rejected"
	}
	return e.err.Error()
}

func (e *savepointAdmissionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func admitError(err error) error {
	if err == nil {
		return nil
	}
	var ae *savepointAdmissionError
	if errors.As(err, &ae) {
		return err
	}
	return &savepointAdmissionError{err: err}
}

func isAdmissionError(err error) bool {
	var ae *savepointAdmissionError
	return errors.As(err, &ae)
}

func freezeTxnCtor(opts spanner.TransactionOptions) spanner.TransactionOptions {
	out := opts
	if opts.CommitOptions.MaxCommitDelay != nil {
		d := *opts.CommitOptions.MaxCommitDelay
		out.CommitOptions.MaxCommitDelay = &d
	}
	if opts.ClientContext != nil {
		out.ClientContext = proto.Clone(opts.ClientContext).(*sppb.RequestOptions_ClientContext)
	}
	return out
}

func queryModeIsPlan(opts spanner.QueryOptions) bool {
	return opts.Mode != nil && *opts.Mode == sppb.ExecuteSqlRequest_PLAN
}

func (tm *TransactionManager) enableSavepointCaptureForTest() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.savepointEnabled = true
}

func (tm *TransactionManager) ensureReplayLocked() {
	if tm == nil || !tm.savepointEnabled || tm.tc == nil {
		return
	}
	if tm.tc.replay == nil {
		tm.tc.replay = &replayState{}
	}
}

func (tm *TransactionManager) capturingLocked() bool {
	return tm != nil && tm.tc != nil && tm.tc.replay != nil
}

func (tok *captureToken) receipt() *operationReceipt {
	if tok == nil {
		return nil
	}
	return tok.rec
}

func (tok *captureToken) matchesLocked(tm *TransactionManager) bool {
	return tok != nil && tm != nil && tm.tc != nil && tok.owner == tm.tc && tok.attempt == tm.tc.attempt && tm.tc.pending == tok
}

func (tm *TransactionManager) startOwnerSQLCaptureLocked(stmt spanner.Statement, opts spanner.QueryOptions, dml bool) (*captureToken, error) {
	if err := tm.rejectIfRecoveringLocked(); err != nil {
		return nil, err
	}
	if !tm.capturingLocked() || queryModeIsPlan(opts) {
		return nil, nil
	}
	if tm.tc.attrs.mode != transactionModeReadWrite {
		return nil, nil
	}
	if tm.tc.replacing {
		return nil, fmt.Errorf("savepoint journal: physical replacement is in progress")
	}
	if tm.tc.pending != nil || tm.tc.inFlight > 0 {
		return nil, fmt.Errorf("savepoint journal: a query is already in flight")
	}
	frozen, err := freezeStatement(stmt.SQL, stmt.Params, opts)
	if err != nil {
		return nil, err
	}
	e := replayEntry{kind: replayKindSQL, stmt: frozen, fingerprint: make([]byte, 32)}
	n := e.accountedBytes()
	if err := tm.tc.replay.reserve(n); err != nil {
		return nil, err
	}
	tok := &captureToken{
		owner:    tm.tc,
		attempt:  tm.tc.attempt,
		frozen:   frozen,
		rec:      &operationReceipt{},
		reserved: n,
		dml:      dml,
	}
	tm.tc.pending = tok
	tm.tc.inFlight++
	return tok, nil
}

func (tm *TransactionManager) finishQueryCapture(tok *captureToken, consumeErr error) error {
	if tm == nil {
		return consumeErr
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.finishQueryCaptureLocked(tok, consumeErr)
}

func (tm *TransactionManager) finishQueryCaptureLocked(tok *captureToken, consumeErr error) error {
	if !tok.matchesLocked(tm) {
		return consumeErr
	}
	pending := tok
	tm.tc.pending = nil
	if tm.tc.inFlight > 0 {
		tm.tc.inFlight--
	}
	if tm.tc.replay == nil {
		return consumeErr
	}
	if consumeErr != nil {
		_, _ = pending.rec.Finish(consumeErr)
		tm.tc.replay.release(pending.reserved)
		return consumeErr
	}
	if !pending.rec.Succeeded() {
		tm.tc.replay.release(pending.reserved)
		return fmt.Errorf("savepoint journal: owner query was not observed")
	}
	e := replayEntry{
		kind:         replayKindSQL,
		stmt:         pending.frozen,
		fingerprint:  pending.rec.fingerprint,
		payloadBytes: pending.reserved,
	}
	tm.tc.replay.commitPrepared(e)
	return nil
}

func (tm *TransactionManager) finishDMLCaptureLocked(tok *captureToken, count int64, consumeErr error) error {
	if !tok.matchesLocked(tm) {
		return consumeErr
	}
	pending := tok
	tm.tc.pending = nil
	if tm.tc.inFlight > 0 {
		tm.tc.inFlight--
	}
	if tm.tc.replay == nil {
		return consumeErr
	}
	if consumeErr != nil {
		_, _ = pending.rec.FinishDML(count, consumeErr)
		tm.tc.replay.release(pending.reserved)
		return consumeErr
	}
	fp, err := pending.rec.FinishDML(count, nil)
	if err != nil {
		tm.tc.replay.release(pending.reserved)
		return err
	}
	if !pending.rec.Succeeded() {
		tm.tc.replay.release(pending.reserved)
		return fmt.Errorf("savepoint journal: DML was not observed")
	}
	e := replayEntry{
		kind:         replayKindSQL,
		stmt:         pending.frozen,
		fingerprint:  fp,
		affected:     count,
		payloadBytes: pending.reserved,
		dml:          true,
	}
	tm.tc.replay.commitPrepared(e)
	return nil
}

func batchReplayAccounted(batch []frozenStatement) int64 {
	return replayEntry{
		kind:        replayKindBatchDML,
		batch:       batch,
		fingerprint: make([]byte, 32),
		counts:      make([]int64, len(batch)),
	}.accountedBytes()
}

func mutateReplayAccounted(mutations []frozenMutation) int64 {
	return replayEntry{
		kind:        replayKindMutate,
		mutations:   mutations,
		fingerprint: make([]byte, 32),
	}.accountedBytes()
}

func (tm *TransactionManager) admitBatchDMLLocked(dmls []spanner.Statement, opts spanner.QueryOptions) error {
	if !tm.capturingLocked() {
		return nil
	}
	if len(tm.tc.replay.queued) > 0 {
		return nil
	}
	frozen, err := freezeStatements(dmls, opts)
	if err != nil {
		return err
	}
	n := batchReplayAccounted(frozen)
	if err := tm.tc.replay.reserve(n); err != nil {
		return err
	}
	tm.tc.replay.admittedBatch = frozen
	tm.tc.replay.admittedBytes = n
	return nil
}

func (tm *TransactionManager) completeBatchDMLLocked(counts []int64, rpcErr error) error {
	if !tm.capturingLocked() {
		return rpcErr
	}
	rs := tm.tc.replay
	batch := rs.queued
	reserved := int64(0)
	if len(batch) > 0 {
		reserved = batchReplayAccounted(batch)
		rs.queued = nil
	} else {
		batch = rs.admittedBatch
		reserved = rs.admittedBytes
		rs.admittedBatch = nil
		rs.admittedBytes = 0
	}
	if rpcErr != nil {
		rs.release(reserved)
		return rpcErr
	}
	rec := &operationReceipt{}
	fp, err := rec.FinishBatch(counts, nil)
	if err != nil {
		rs.release(reserved)
		return err
	}
	e := replayEntry{
		kind:         replayKindBatchDML,
		batch:        batch,
		fingerprint:  fp,
		counts:       append([]int64(nil), counts...),
		payloadBytes: reserved,
	}
	rs.commitPrepared(e)
	return nil
}

func (tm *TransactionManager) admitMutationsLocked(frozen []frozenMutation) error {
	if !tm.capturingLocked() {
		return nil
	}
	n := mutateReplayAccounted(frozen)
	if err := tm.tc.replay.reserve(n); err != nil {
		return err
	}
	tm.tc.replay.admittedMut = frozen
	tm.tc.replay.admittedBytes = n
	return nil
}

func (tm *TransactionManager) completeMutationsLocked(rpcErr error) error {
	if !tm.capturingLocked() {
		return rpcErr
	}
	rs := tm.tc.replay
	frozen := rs.admittedMut
	reserved := rs.admittedBytes
	rs.admittedMut = nil
	rs.admittedBytes = 0
	if rpcErr != nil {
		rs.release(reserved)
		return rpcErr
	}
	rec := &operationReceipt{}
	fp, err := rec.Finish(nil)
	if err != nil {
		rs.release(reserved)
		return err
	}
	e := replayEntry{
		kind:         replayKindMutate,
		mutations:    frozen,
		fingerprint:  fp,
		payloadBytes: reserved,
	}
	rs.commitPrepared(e)
	return nil
}

func freezeStatements(dmls []spanner.Statement, opts spanner.QueryOptions) ([]frozenStatement, error) {
	out := make([]frozenStatement, 0, len(dmls))
	for _, stmt := range dmls {
		frozen, err := freezeStatement(stmt.SQL, stmt.Params, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, frozen)
	}
	return out, nil
}

func (tm *TransactionManager) enqueueFrozenAutomaticDMLLocked(stmt spanner.Statement) error {
	if !tm.capturingLocked() {
		return nil
	}
	frozen, err := freezeStatement(stmt.SQL, stmt.Params, spanner.QueryOptions{LastStatement: false})
	if err != nil {
		return err
	}
	old := batchReplayAccounted(tm.tc.replay.queued)
	next := append(append([]frozenStatement(nil), tm.tc.replay.queued...), frozen)
	delta := batchReplayAccounted(next) - old
	if err := tm.tc.replay.reserve(delta); err != nil {
		return err
	}
	tm.tc.replay.queued = append(tm.tc.replay.queued, frozen)
	return nil
}

func freezeMutate(table, op, body string) ([]frozenMutation, []*spanner.Mutation, error) {
	op = canonicalMutateOperation(op)
	if op == "DELETE" {
		return freezeDeleteMutate(table, body)
	}
	columns, values, err := parseLiteralString(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid write mutations: %w", err)
	}
	if len(columns) == 0 {
		return nil, nil, fmt.Errorf("column names can't be inferenced")
	}
	frozen := make([]frozenMutation, 0, len(values))
	mutations := make([]*spanner.Mutation, 0, len(values))
	for _, v := range values {
		fm := freezeMutationWrite(table, op, columns, v)
		m, err := fm.Mutation()
		if err != nil {
			return nil, nil, err
		}
		frozen = append(frozen, fm)
		mutations = append(mutations, m)
	}
	return frozen, mutations, nil
}

func freezeDeleteMutate(table, body string) ([]frozenMutation, []*spanner.Mutation, error) {
	if strings.EqualFold(strings.TrimSpace(body), "ALL") {
		fm := frozenMutation{Table: table, Op: "DELETE", DeleteAll: true}
		m, err := fm.Mutation()
		if err != nil {
			return nil, nil, err
		}
		return []frozenMutation{fm}, []*spanner.Mutation{m}, nil
	}
	expr, err := parseMemefishExpr("", body)
	if err != nil {
		return nil, nil, err
	}
	if call, ok := expr.(*ast.CallExpr); ok {
		kr, err := freezeKeyRangeCall(call)
		if err != nil {
			return nil, nil, err
		}
		fm := frozenMutation{Table: table, Op: "DELETE", KeyRange: kr}
		m, err := fm.Mutation()
		if err != nil {
			return nil, nil, err
		}
		return []frozenMutation{fm}, []*spanner.Mutation{m}, nil
	}
	columns, valuesList, err := parseLiteralExpr(expr)
	if err != nil {
		return nil, nil, err
	}
	if len(columns) > 0 {
		slog.Warn("delete mutation ignores column names", "columns", columns)
	}
	keys := make([][]spanner.GenericColumnValue, len(valuesList))
	for i, row := range valuesList {
		keys[i] = cloneGCVRow(row)
	}
	fm := frozenMutation{Table: table, Op: "DELETE", Keys: keys}
	m, err := fm.Mutation()
	if err != nil {
		return nil, nil, err
	}
	return []frozenMutation{fm}, []*spanner.Mutation{m}, nil
}

func replayJournal(tm *TransactionManager) []replayEntry {
	if tm == nil {
		return nil
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil || tm.tc.replay == nil {
		return nil
	}
	out := make([]replayEntry, len(tm.tc.replay.entries))
	copy(out, tm.tc.replay.entries)
	return out
}

func replayCtor(tm *TransactionManager) spanner.TransactionOptions {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil {
		return spanner.TransactionOptions{}
	}
	return tm.tc.ctorOpts
}

func replayQueued(tm *TransactionManager) []frozenStatement {
	if tm == nil {
		return nil
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil || tm.tc.replay == nil {
		return nil
	}
	return append([]frozenStatement(nil), tm.tc.replay.queued...)
}
