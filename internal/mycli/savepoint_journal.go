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
	"slices"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// savepointJournalLimit is the initial retained payload budget for one
// transaction journal. It counts frozen SQL/parameter/option/mutation bytes,
// marker names, and the constant-sized fingerprint. Observed result rows are
// hashed, not retained. The limit is a starting policy, not total process RSS.
const savepointJournalLimit int64 = 16 << 20

var errSavepointJournalFull = errors.New("savepoint journal exceeded 16 MiB")

type replayKind uint8

const (
	replayKindUnspecified replayKind = iota
	replayKindSQL
	replayKindBatchDML
	replayKindMutate
)

type savepoint struct {
	name     string
	position int
	bytes    int64
}

type frozenQueryOptions struct {
	Mode                        *sppb.ExecuteSqlRequest_QueryMode
	Options                     *sppb.ExecuteSqlRequest_QueryOptions
	Priority                    sppb.RequestOptions_Priority
	RequestTag                  string
	DirectedReadOptions         *sppb.DirectedReadOptions
	DataBoostEnabled            bool
	ExcludeTxnFromChangeStreams bool
	LastStatement               bool
	ClientContext               *sppb.RequestOptions_ClientContext
}

type frozenStatement struct {
	SQL    string
	Params map[string]spanner.GenericColumnValue
	Opts   frozenQueryOptions
}

type frozenKeyRange struct {
	Start []spanner.GenericColumnValue
	End   []spanner.GenericColumnValue
	Kind  spanner.KeyRangeKind
}

type frozenMutation struct {
	Table     string
	Op        string
	Columns   []string
	Values    []spanner.GenericColumnValue
	DeleteAll bool
	Keys      [][]spanner.GenericColumnValue
	KeyRange  *frozenKeyRange
}

type replayEntry struct {
	kind         replayKind
	stmt         frozenStatement
	batch        []frozenStatement
	mutations    []frozenMutation
	fingerprint  []byte
	affected     int64
	counts       []int64
	payloadBytes int64
	dml          bool
}

type replayState struct {
	entries          []replayEntry
	savepoints       []savepoint
	retainedBytes    int64
	recoveryRequired error
	// queued holds frozen automatic DML reserved at enqueue. It is not a
	// replay entry until BatchUpdate succeeds.
	queued []frozenStatement
	// admittedBatch/admittedMut hold payload reserved before BatchUpdate or
	// BufferWrite. Local admission failure must not send those RPCs.
	admittedBatch []frozenStatement
	admittedMut   []frozenMutation
	admittedBytes int64
}

func (rs *replayState) reserve(n int64) error {
	if n < 0 {
		return fmt.Errorf("savepoint journal reserve is negative")
	}
	if rs == nil {
		return fmt.Errorf("savepoint journal is nil")
	}
	if rs.retainedBytes+n > savepointJournalLimit {
		return errSavepointJournalFull
	}
	rs.retainedBytes += n
	return nil
}

func (rs *replayState) release(n int64) {
	if rs == nil || n <= 0 {
		return
	}
	rs.retainedBytes -= n
	if rs.retainedBytes < 0 {
		rs.retainedBytes = 0
	}
}

func (rs *replayState) commitPrepared(e replayEntry) {
	if e.payloadBytes == 0 {
		e.payloadBytes = e.accountedBytes()
	}
	rs.entries = append(rs.entries, e)
}

func (rs *replayState) dropQueued() {
	if rs == nil {
		return
	}
	rs.release(queuedAccounted(rs.queued))
	rs.queued = nil
}

func (rs *replayState) needsRecovery() bool {
	return rs != nil && rs.recoveryRequired != nil
}

func (rs *replayState) appendEntry(e replayEntry) error {
	if e.payloadBytes == 0 {
		e.payloadBytes = e.accountedBytes()
	}
	if err := rs.reserve(e.payloadBytes); err != nil {
		return err
	}
	rs.entries = append(rs.entries, e)
	return nil
}

func (rs *replayState) lookup(name string) (int, savepoint, bool) {
	if rs == nil {
		return -1, savepoint{}, false
	}
	for i, sp := range rs.savepoints {
		if sp.name == name {
			return i, sp, true
		}
	}
	return -1, savepoint{}, false
}

func (rs *replayState) hasMarkers() bool {
	return rs != nil && len(rs.savepoints) > 0
}

func (rs *replayState) releaseNamed(name string) error {
	idx, _, ok := rs.lookup(name)
	if !ok {
		return errSavepointUnknown
	}
	for _, sp := range rs.savepoints[idx:] {
		rs.retainedBytes -= sp.bytes
	}
	clear(rs.savepoints[idx:])
	rs.savepoints = rs.savepoints[:idx]
	if rs.retainedBytes < 0 {
		rs.retainedBytes = 0
	}
	return nil
}

func (rs *replayState) addSavepoint(name string) error {
	if _, _, ok := rs.lookup(name); ok {
		return errSavepointDuplicate
	}
	n := savepointMarkerBytes(name)
	if err := rs.reserve(n); err != nil {
		return err
	}
	rs.savepoints = append(rs.savepoints, savepoint{
		name:     name,
		position: len(rs.entries),
		bytes:    n,
	})
	return nil
}

func (rs *replayState) commitSavepoint(name string, n int64) error {
	if _, _, ok := rs.lookup(name); ok {
		rs.release(n)
		return errSavepointDuplicate
	}
	rs.savepoints = append(rs.savepoints, savepoint{
		name:     name,
		position: len(rs.entries),
		bytes:    n,
	})
	return nil
}

func (rs *replayState) rollbackToMarker(idx int) {
	if rs == nil || idx < 0 || idx >= len(rs.savepoints) {
		return
	}
	position := rs.savepoints[idx].position
	if position > len(rs.entries) {
		position = len(rs.entries)
	}
	for _, e := range rs.entries[position:] {
		rs.retainedBytes -= e.payloadBytes
	}
	clear(rs.entries[position:])
	rs.entries = rs.entries[:position]
	for _, sp := range rs.savepoints[idx+1:] {
		rs.retainedBytes -= sp.bytes
	}
	clear(rs.savepoints[idx+1:])
	rs.savepoints = rs.savepoints[:idx+1]
	if rs.retainedBytes < 0 {
		rs.retainedBytes = 0
	}
}

func (rs *replayState) truncateAfter(position int) {
	if rs == nil || position < 0 {
		return
	}
	if position > len(rs.entries) {
		position = len(rs.entries)
	}
	for _, e := range rs.entries[position:] {
		rs.retainedBytes -= e.payloadBytes
	}
	clear(rs.entries[position:])
	rs.entries = rs.entries[:position]
	kept := 0
	for _, sp := range rs.savepoints {
		if sp.position > position {
			rs.retainedBytes -= sp.bytes
			continue
		}
		rs.savepoints[kept] = sp
		kept++
	}
	clear(rs.savepoints[kept:])
	rs.savepoints = rs.savepoints[:kept]
	if rs.retainedBytes < 0 {
		rs.retainedBytes = 0
	}
}

func (e replayEntry) accountedBytes() int64 {
	n := int64(len(e.fingerprint))
	n += e.stmt.payloadBytes()
	for _, stmt := range e.batch {
		n += stmt.payloadBytes()
	}
	for _, m := range e.mutations {
		n += m.payloadBytes()
	}
	n += int64(8 * (1 + len(e.counts)))
	return n
}

func (e replayEntry) dmlObservation() (int64, []int64) {
	return e.affected, slices.Clone(e.counts)
}

func savepointMarkerBytes(name string) int64 {
	return int64(len(name)) + 8
}

func freezeStatement(sql string, params map[string]any, opts spanner.QueryOptions) (frozenStatement, error) {
	frozenParams, err := freezeParams(params)
	if err != nil {
		return frozenStatement{}, err
	}
	return frozenStatement{
		SQL:    sql,
		Params: frozenParams,
		Opts:   freezeQueryOptions(opts),
	}, nil
}

func freezeParams(params map[string]any) (map[string]spanner.GenericColumnValue, error) {
	if params == nil {
		return nil, nil
	}
	out := make(map[string]spanner.GenericColumnValue, len(params))
	for name, v := range params {
		gcv, ok := v.(spanner.GenericColumnValue)
		if !ok {
			return nil, fmt.Errorf("savepoint freeze: parameter %q is %T, want GenericColumnValue", name, v)
		}
		out[name] = cloneGenericColumnValue(gcv)
	}
	return out, nil
}

func freezeQueryOptions(opts spanner.QueryOptions) frozenQueryOptions {
	out := frozenQueryOptions{
		Priority:                    opts.Priority,
		RequestTag:                  opts.RequestTag,
		DataBoostEnabled:            opts.DataBoostEnabled,
		ExcludeTxnFromChangeStreams: opts.ExcludeTxnFromChangeStreams,
		LastStatement:               opts.LastStatement,
	}
	if opts.Mode != nil {
		mode := *opts.Mode
		out.Mode = &mode
	}
	if opts.Options != nil {
		out.Options = proto.Clone(opts.Options).(*sppb.ExecuteSqlRequest_QueryOptions)
	}
	if opts.DirectedReadOptions != nil {
		out.DirectedReadOptions = proto.Clone(opts.DirectedReadOptions).(*sppb.DirectedReadOptions)
	}
	if opts.ClientContext != nil {
		out.ClientContext = proto.Clone(opts.ClientContext).(*sppb.RequestOptions_ClientContext)
	}
	return out
}

func (opts frozenQueryOptions) toQueryOptions() spanner.QueryOptions {
	out := spanner.QueryOptions{
		Priority:                    opts.Priority,
		RequestTag:                  opts.RequestTag,
		DataBoostEnabled:            opts.DataBoostEnabled,
		ExcludeTxnFromChangeStreams: opts.ExcludeTxnFromChangeStreams,
		// Replay never uses implicit-transaction LastStatement=true.
		LastStatement: false,
	}
	if opts.Mode != nil {
		mode := *opts.Mode
		out.Mode = &mode
	}
	if opts.Options != nil {
		out.Options = proto.Clone(opts.Options).(*sppb.ExecuteSqlRequest_QueryOptions)
	}
	if opts.DirectedReadOptions != nil {
		out.DirectedReadOptions = proto.Clone(opts.DirectedReadOptions).(*sppb.DirectedReadOptions)
	}
	if opts.ClientContext != nil {
		out.ClientContext = proto.Clone(opts.ClientContext).(*sppb.RequestOptions_ClientContext)
	}
	return out
}

func freezeMutationWrite(table, op string, columns []string, values []spanner.GenericColumnValue) frozenMutation {
	cols := append([]string(nil), columns...)
	vals := make([]spanner.GenericColumnValue, len(values))
	for i, v := range values {
		vals[i] = cloneGenericColumnValue(v)
	}
	return frozenMutation{Table: table, Op: op, Columns: cols, Values: vals}
}

func (m frozenMutation) Mutation() (*spanner.Mutation, error) {
	vals := make([]any, len(m.Values))
	for i, v := range m.Values {
		vals[i] = v
	}
	switch m.Op {
	case "INSERT":
		return spanner.Insert(m.Table, m.Columns, vals), nil
	case "UPDATE":
		return spanner.Update(m.Table, m.Columns, vals), nil
	case "INSERT_OR_UPDATE":
		return spanner.InsertOrUpdate(m.Table, m.Columns, vals), nil
	case "REPLACE":
		return spanner.Replace(m.Table, m.Columns, vals), nil
	case "DELETE":
		if m.DeleteAll {
			return spanner.Delete(m.Table, spanner.AllKeys()), nil
		}
		if m.KeyRange != nil {
			kr, err := m.KeyRange.toKeyRange()
			if err != nil {
				return nil, err
			}
			return spanner.Delete(m.Table, kr), nil
		}
		keys := make([]spanner.Key, 0, len(m.Keys))
		for _, row := range m.Keys {
			key, err := toKeys(row)
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}
		if len(keys) == 1 {
			return spanner.Delete(m.Table, keys[0]), nil
		}
		return spanner.Delete(m.Table, spanner.KeySetFromKeys(keys...)), nil
	default:
		return nil, fmt.Errorf("savepoint freeze: unsupported mutation op %q", m.Op)
	}
}

func cloneGenericColumnValue(v spanner.GenericColumnValue) spanner.GenericColumnValue {
	out := spanner.GenericColumnValue{}
	if v.Type != nil {
		out.Type = proto.Clone(v.Type).(*sppb.Type)
	}
	if v.Value != nil {
		out.Value = proto.Clone(v.Value).(*structpb.Value)
	}
	return out
}

func (s frozenStatement) payloadBytes() int64 {
	n := int64(len(s.SQL)) + int64(len(s.Opts.RequestTag))
	for name, v := range s.Params {
		n += int64(len(name)) + gcvPayloadBytes(v)
	}
	if s.Opts.Options != nil {
		n += int64(proto.Size(s.Opts.Options))
	}
	if s.Opts.DirectedReadOptions != nil {
		n += int64(proto.Size(s.Opts.DirectedReadOptions))
	}
	if s.Opts.ClientContext != nil {
		n += int64(proto.Size(s.Opts.ClientContext))
	}
	return n
}

func (m frozenMutation) payloadBytes() int64 {
	n := int64(len(m.Table)) + int64(len(m.Op))
	for _, col := range m.Columns {
		n += int64(len(col))
	}
	for _, v := range m.Values {
		n += gcvPayloadBytes(v)
	}
	for _, key := range m.Keys {
		for _, v := range key {
			n += gcvPayloadBytes(v)
		}
	}
	if m.KeyRange != nil {
		for _, v := range m.KeyRange.Start {
			n += gcvPayloadBytes(v)
		}
		for _, v := range m.KeyRange.End {
			n += gcvPayloadBytes(v)
		}
	}
	return n
}

func cloneGCVRow(row []spanner.GenericColumnValue) []spanner.GenericColumnValue {
	out := make([]spanner.GenericColumnValue, len(row))
	for i, v := range row {
		out[i] = cloneGenericColumnValue(v)
	}
	return out
}

func (kr frozenKeyRange) toKeyRange() (spanner.KeyRange, error) {
	start, err := toKeys(kr.Start)
	if err != nil {
		return spanner.KeyRange{}, err
	}
	end, err := toKeys(kr.End)
	if err != nil {
		return spanner.KeyRange{}, err
	}
	return spanner.KeyRange{Start: start, End: end, Kind: kr.Kind}, nil
}

func gcvPayloadBytes(v spanner.GenericColumnValue) int64 {
	var n int64
	if v.Type != nil {
		n += int64(proto.Size(v.Type))
	}
	if v.Value != nil {
		n += int64(proto.Size(v.Value))
	}
	return n
}
