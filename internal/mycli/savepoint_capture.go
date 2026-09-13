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
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/proto"
)

type sqlCapture struct {
	frozen   frozenStatement
	rec      *operationReceipt
	reserved int64
	dml      bool
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

func (tm *TransactionManager) startOwnerSQLCaptureLocked(stmt spanner.Statement, opts spanner.QueryOptions, dml bool) error {
	if !tm.capturingLocked() || queryModeIsPlan(opts) {
		return nil
	}
	if tm.tc.attrs.mode != transactionModeReadWrite {
		return nil
	}
	if tm.tc.pending != nil || tm.tc.inFlight > 0 {
		return fmt.Errorf("savepoint journal: a query is already in flight")
	}
	frozen, err := freezeStatement(stmt.SQL, stmt.Params, opts)
	if err != nil {
		return err
	}
	e := replayEntry{kind: replayKindSQL, stmt: frozen, fingerprint: make([]byte, 32)}
	n := e.accountedBytes()
	if err := tm.tc.replay.reserve(n); err != nil {
		return err
	}
	tm.tc.pending = &sqlCapture{frozen: frozen, rec: &operationReceipt{}, reserved: n, dml: dml}
	tm.tc.inFlight++
	return nil
}

func (tm *TransactionManager) queryReceipt() *operationReceipt {
	if tm == nil {
		return nil
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.queryReceiptLocked()
}

func (tm *TransactionManager) queryReceiptLocked() *operationReceipt {
	if tm.tc == nil || tm.tc.pending == nil {
		return nil
	}
	return tm.tc.pending.rec
}

func (tm *TransactionManager) finishQueryCapture(consumeErr error) error {
	if tm == nil {
		return consumeErr
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.finishQueryCaptureLocked(consumeErr)
}

func (tm *TransactionManager) finishQueryCaptureLocked(consumeErr error) error {
	if tm.tc == nil || tm.tc.pending == nil {
		return consumeErr
	}
	pending := tm.tc.pending
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

func (tm *TransactionManager) finishDMLCaptureLocked(count int64, consumeErr error) error {
	if tm.tc == nil || tm.tc.pending == nil {
		return consumeErr
	}
	pending := tm.tc.pending
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
	}
	tm.tc.replay.commitPrepared(e)
	return nil
}

func (tm *TransactionManager) recordBatchDMLLocked(dmls []spanner.Statement, opts spanner.QueryOptions, counts []int64, rpcErr error) error {
	if !tm.capturingLocked() {
		return rpcErr
	}
	if rpcErr != nil {
		tm.tc.replay.dropQueued()
		return rpcErr
	}
	batch := tm.tc.replay.queued
	reservedQueued := int64(0)
	if len(batch) == 0 {
		var err error
		batch, err = freezeStatements(dmls, opts)
		if err != nil {
			return err
		}
	} else {
		for _, stmt := range batch {
			reservedQueued += stmt.payloadBytes()
		}
		tm.tc.replay.queued = nil
	}
	rec := &operationReceipt{}
	fp, err := rec.FinishBatch(counts, nil)
	if err != nil {
		if reservedQueued > 0 {
			tm.tc.replay.release(reservedQueued)
		}
		return err
	}
	e := replayEntry{
		kind:        replayKindBatchDML,
		batch:       batch,
		fingerprint: fp,
		counts:      append([]int64(nil), counts...),
	}
	n := e.accountedBytes()
	if reservedQueued > 0 {
		if n > reservedQueued {
			if err := tm.tc.replay.reserve(n - reservedQueued); err != nil {
				tm.tc.replay.release(reservedQueued)
				return err
			}
		}
		e.payloadBytes = n
		tm.tc.replay.commitPrepared(e)
		return nil
	}
	return tm.tc.replay.appendEntry(e)
}

func (tm *TransactionManager) recordMutationsLocked(frozen []frozenMutation, rpcErr error) error {
	if !tm.capturingLocked() {
		return rpcErr
	}
	if rpcErr != nil {
		return rpcErr
	}
	rec := &operationReceipt{}
	fp, err := rec.Finish(nil)
	if err != nil {
		return err
	}
	e := replayEntry{
		kind:        replayKindMutate,
		mutations:   frozen,
		fingerprint: fp,
	}
	return tm.tc.replay.appendEntry(e)
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
	if err := tm.tc.replay.reserve(frozen.payloadBytes()); err != nil {
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
	if strings.ToUpper(strings.TrimSpace(body)) == "ALL" {
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
	if _, ok := expr.(*ast.CallExpr); ok {
		return nil, nil, fmt.Errorf("savepoint freeze: MUTATE DELETE key range is not journaled")
	}
	_, valuesList, err := parseLiteralExpr(expr)
	if err != nil {
		return nil, nil, err
	}
	keys := make([][]spanner.GenericColumnValue, len(valuesList))
	for i, row := range valuesList {
		keys[i] = make([]spanner.GenericColumnValue, len(row))
		for j, v := range row {
			keys[i][j] = cloneGenericColumnValue(v)
		}
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
