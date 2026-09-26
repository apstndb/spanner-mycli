// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
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
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/memebridge"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// Parse only existing function-call/literal syntax. This does not require
// memefish to recognize queue DDL or acknowledgement DELETE extensions.
func freezeQueueMutate(queue, op, body string) ([]frozenMutation, []*spanner.Mutation, error) {
	expr, err := parseMemefishExpr("", "QUEUE_MUTATION"+strings.TrimSpace(body))
	if err != nil {
		return nil, nil, fmt.Errorf("MUTATE %s expects named arguments in parentheses: %w", op, err)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Func.Idents) != 1 || call.Func.Idents[0].Name != "QUEUE_MUTATION" || len(call.Args) != 0 || call.Distinct || call.NullHandling != nil || call.Having != nil || call.OrderBy != nil || call.Limit != nil || call.Hint != nil {
		return nil, nil, fmt.Errorf("MUTATE %s expects only named arguments in parentheses", op)
	}
	columns := make([]string, 0, len(call.NamedArgs))
	values := make([]spanner.GenericColumnValue, 0, len(call.NamedArgs))
	for _, arg := range call.NamedArgs {
		value, err := memebridge.MemefishExprToGCV(arg.Value)
		if err != nil {
			return nil, nil, fmt.Errorf("MUTATE %s argument %s must be a supported literal: %w", op, arg.Name.Name, err)
		}
		columns = append(columns, strings.ToLower(arg.Name.Name))
		values = append(values, value)
	}
	// Reuse the journal's deep cloning and payload accounting for named values,
	// including nested keys and payloads. Replay rebuilds the native mutation.
	frozen := freezeMutationWrite(queue, op, columns, values)
	mutation, err := frozen.Mutation()
	if err != nil {
		return nil, nil, err
	}
	return []frozenMutation{frozen}, []*spanner.Mutation{mutation}, nil
}

func (m frozenMutation) queueMutation() (*spanner.Mutation, error) {
	args := make(map[string]spanner.GenericColumnValue, len(m.Columns))
	for i, name := range m.Columns {
		allowed := name == "key" || (m.Op == "SEND" && (name == "payload" || name == "deliver_time")) || (m.Op == "ACK" && name == "ignore_not_found")
		if !allowed {
			return nil, fmt.Errorf("unknown MUTATE %s argument %q", m.Op, name)
		}
		if _, exists := args[name]; exists {
			return nil, fmt.Errorf("duplicate MUTATE %s argument %q", m.Op, name)
		}
		args[name] = m.Values[i]
	}
	keyValue, ok := args["key"]
	if !ok {
		return nil, fmt.Errorf("MUTATE %s requires key", m.Op)
	}
	if keyValue.Type.GetCode() == sppb.TypeCode_ARRAY {
		return nil, fmt.Errorf("MUTATE %s key must be one scalar or tuple, not an array or key set", m.Op)
	}
	_, rows, err := convertToColumnsValues(keyValue)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 || len(rows[0]) == 0 {
		return nil, fmt.Errorf("MUTATE %s requires a nonempty key", m.Op)
	}
	key, err := toKeys(rows[0])
	if err != nil {
		return nil, fmt.Errorf("MUTATE %s key: %w", m.Op, err)
	}
	for _, v := range key {
		if v == nil {
			return nil, fmt.Errorf("MUTATE %s key components must not be NULL", m.Op)
		}
	}
	if m.Op == "ACK" {
		ignore := false
		if value, ok := args["ignore_not_found"]; ok {
			ignore, err = decode[bool](value)
			if err != nil {
				return nil, fmt.Errorf("MUTATE ACK ignore_not_found must be a non-NULL BOOL: %w", err)
			}
		}
		return spanner.Ack(m.Table, key, spanner.WithIgnoreNotFound(ignore)), nil
	}
	payload, ok := args["payload"]
	if !ok {
		return nil, fmt.Errorf("MUTATE SEND requires payload")
	}
	var opts []spanner.SendOption
	if value, ok := args["deliver_time"]; ok {
		delivery, err := decode[time.Time](value)
		if err != nil {
			return nil, fmt.Errorf("MUTATE SEND deliver_time must be a non-NULL TIMESTAMP: %w", err)
		}
		// The SDK uses time.Time{} to mean absent; do not silently change an
		// explicitly specified zero timestamp into immediate delivery.
		if delivery.IsZero() {
			return nil, fmt.Errorf("MUTATE SEND deliver_time cannot be the zero timestamp; omit it for immediate delivery")
		}
		opts = append(opts, spanner.WithDeliveryTime(delivery))
	}
	return spanner.Send(m.Table, key, payload, opts...), nil
}
