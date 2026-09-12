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
	"fmt"
	"io"
	"slices"
	"strings"

	"cloud.google.com/go/spanner"
	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spantype"
	"github.com/apstndb/spanvalue"
)

const dumpCyclicWarning = "-- Cyclic MUTATE restore: one transaction per group; never automatically split.\n" +
	"-- No prediction of the 80,000-mutation or 100 MiB service commit limits.\n" +
	"-- A group may fail atomically at COMMIT; earlier groups, DDL and ordinary INSERTs may remain committed.\n"

const (
	dumpCyclicBegin = dumpCyclicWarning + "BEGIN RW;\n"
	dumpCyclicEnd   = "COMMIT;\n\n"
)

// dumpCyclicData retains only generated text, not decoded rows or a second
// source query. Row statements are sorted within each table for deterministic
// output. Their order is not the referential-integrity mechanism: COMMIT is.
type dumpCyclicData struct {
	Statements []string
}

func (data *dumpCyclicData) writeTo(out io.Writer) error {
	if _, err := io.WriteString(out, dumpCyclicBegin); err != nil {
		return err
	}
	for _, statement := range data.Statements {
		if _, err := io.WriteString(out, statement); err != nil {
			return err
		}
	}
	_, err := io.WriteString(out, dumpCyclicEnd)
	return err
}

// dumpCyclicBudget is shared across every component of one dump. It counts
// retained generated text including transaction framing, not heap overhead,
// transient formatter allocations or Spanner's server-side commit size.
type dumpCyclicBudget struct {
	limit int64
	used  int64
}

func (budget *dumpCyclicBudget) retain(size int) error {
	if budget.limit <= 0 || int64(size) > budget.limit-budget.used {
		return fmt.Errorf("cyclic DUMP exceeds CLI_DUMP_CYCLIC_MAX_BYTES (%d); this is a local encoded-text cap, not a Spanner commit-size estimate", budget.limit)
	}
	budget.used += int64(size)
	return nil
}

func dumpMutationFormatConfig() *spanvalue.FormatConfig {
	// The literal preset intentionally formats some wire strings without
	// decoding them. Validate scalars (also inside arrays) before falling
	// through to the shared literal policy, so malformed values fail before
	// any dump output. Do not format the decoded wrapper: retain wire fidelity.
	return sqlLiteralFormatConfig().WithComplexPlugin(spanvalue.PluginFromNullable(
		func(spanvalue.NullableValue) (string, error) { return "", spanvalue.ErrFallthrough },
	)).WithComplexPlugin(func(_ spanvalue.Formatter, value spanner.GenericColumnValue, _ bool) (string, error) {
		if value.Value == nil || value.Value.Kind == nil {
			return "", fmt.Errorf("cyclic DUMP value has no wire payload")
		}
		return "", spanvalue.ErrFallthrough
	})
}

// dumpMutationType derives an independent wire-surrogate type. Target PROTO
// descriptors must already match: transporting descriptors is not this mode's
// contract. Reject unsupported types even for NULL/empty values.
func dumpMutationType(typ *sppb.Type) (*sppb.Type, error) {
	if typ == nil || typ.TypeAnnotation != sppb.TypeAnnotationCode_TYPE_ANNOTATION_CODE_UNSPECIFIED {
		return nil, fmt.Errorf("unsupported cyclic DUMP column type: %v", typ)
	}
	code := typ.Code
	switch code {
	case sppb.TypeCode_ARRAY:
		if typ.ArrayElementType.GetCode() == sppb.TypeCode_ARRAY {
			return nil, fmt.Errorf("nested ARRAY is not a supported cyclic DUMP column type")
		}
		element, err := dumpMutationType(typ.ArrayElementType)
		if err != nil {
			return nil, err
		}
		return &sppb.Type{Code: code, ArrayElementType: element}, nil
	case sppb.TypeCode_PROTO, sppb.TypeCode_ENUM:
		if typ.ProtoTypeFqn == "" {
			return nil, fmt.Errorf("cyclic DUMP %s column has no descriptor name", code)
		}
		if code == sppb.TypeCode_PROTO {
			code = sppb.TypeCode_BYTES
		} else {
			code = sppb.TypeCode_INT64
		}
	case sppb.TypeCode_BOOL, sppb.TypeCode_INT64, sppb.TypeCode_FLOAT32, sppb.TypeCode_FLOAT64,
		sppb.TypeCode_STRING, sppb.TypeCode_BYTES, sppb.TypeCode_DATE, sppb.TypeCode_TIMESTAMP,
		sppb.TypeCode_NUMERIC, sppb.TypeCode_JSON, sppb.TypeCode_UUID:
	default:
		return nil, fmt.Errorf("unsupported cyclic DUMP column type: %v", typ)
	}
	return &sppb.Type{Code: code}, nil
}

func encodeDumpMutationRow(id tableID, row *spanner.Row, fc *spanvalue.FormatConfig) (string, error) {
	if row == nil || row.Size() == 0 {
		return "", fmt.Errorf("cyclic DUMP row has no columns")
	}
	fields := make([]string, row.Size())
	values := make([]string, row.Size())
	for i, name := range row.ColumnNames() {
		typ, err := dumpMutationType(row.ColumnType(i))
		if err != nil {
			return "", fmt.Errorf("column %s: %w", name, err)
		}
		wire := spanner.GenericColumnValue{Type: typ, Value: row.ColumnValue(i)}
		values[i], err = fc.FormatToplevelColumn(wire)
		if err != nil {
			return "", fmt.Errorf("column %s: %w", name, err)
		}
		fields[i] = spanvalue.QuoteIdentifier(dbadminpb.DatabaseDialect_GOOGLE_STANDARD_SQL, name) + " " + spantype.FormatTypeNormal(typ)
	}
	return "MUTATE " + quoteTableID(dbadminpb.DatabaseDialect_GOOGLE_STANDARD_SQL, id) +
		" INSERT STRUCT<" + strings.Join(fields, ", ") + ">(" + strings.Join(values, ", ") + ");\n", nil
}

func prepareDumpCyclicData(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, tables []dumpTablePlan, budget *dumpCyclicBudget, dro *sppb.DirectedReadOptions) (*dumpCyclicData, error) {
	data := &dumpCyclicData{}
	fc := dumpMutationFormatConfig()
	for _, table := range tables {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if session.dumpReadTxnProbe != nil {
			session.dumpReadTxnProbe("preflight", txn)
		}
		if session.dumpCyclePreflightProbe != nil {
			if err := session.dumpCyclePreflightProbe(table.ID, txn); err != nil {
				return nil, err
			}
		}
		if len(table.Columns) == 0 {
			hasRows, err := tableHasRowsWithTxn(ctx, txn, session.systemVariables.Feature.DatabaseDialect, table.ID, dro)
			if err != nil {
				return nil, err
			}
			if hasRows {
				return nil, fmt.Errorf("populated cyclic DUMP table %s has no writable columns", table.ID.FQN())
			}
			continue
		}
		start := len(data.Statements)
		iter := queryWithDirectedRead(ctx, txn, spanner.Statement{SQL: buildSelectQueryWithColumns(session.systemVariables.Feature.DatabaseDialect, table.Columns, table.ID)}, dro)
		err := iter.Do(func(row *spanner.Row) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			statement, err := encodeDumpMutationRow(table.ID, row, fc)
			if err != nil {
				return err
			}
			if len(data.Statements) == 0 {
				if err := budget.retain(len(dumpCyclicBegin) + len(dumpCyclicEnd)); err != nil {
					return err
				}
			}
			if err := budget.retain(len(statement)); err != nil {
				return err
			}
			data.Statements = append(data.Statements, statement)
			return nil
		})
		// Do stops the iterator itself, including on a callback error.
		if err != nil {
			return nil, fmt.Errorf("encode cyclic DUMP table %s: %w", table.ID.FQN(), err)
		}
		slices.Sort(data.Statements[start:])
	}
	return data, ctx.Err()
}

func prepareDumpMutationUnits(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, resolver *DependencyResolver, selected []tableID, dro *sppb.DirectedReadOptions) ([]dumpDataPlan, error) {
	components, err := resolver.orderedSafetyComponents(selected)
	if err != nil {
		return nil, err
	}
	budget := &dumpCyclicBudget{limit: session.systemVariables.Display.DumpCyclicMaxBytes}
	var units []dumpDataPlan
	hasMutations := false
	for _, component := range components {
		var tables []dumpTablePlan
		for _, id := range component.Tables {
			columns, err := getWritableColumnsWithTxn(ctx, txn, id, dro)
			if err != nil {
				return nil, err
			}
			tables = append(tables, dumpTablePlan{ID: id, Columns: columns})
		}
		if component.Cyclic {
			data, err := prepareDumpCyclicData(ctx, session, txn, tables, budget, dro)
			if err != nil {
				return nil, err
			}
			if len(data.Statements) > 0 {
				hasMutations = true
				units = append(units, dumpDataPlan{Cyclic: data})
				continue
			}
		}
		for _, table := range tables {
			units = append(units, dumpDataPlan{Table: table, Empty: component.Cyclic})
		}
	}
	if !hasMutations {
		// Preserve the legacy table ordering and exact comments when no
		// populated cycle needs mutation grouping. Empty cyclic tables have
		// already been scanned and must not be rescanned during output.
		order, err := resolver.GetOrderForTables(selected)
		if err != nil {
			return nil, err
		}
		positions := make(map[tableID]int, len(order))
		for i, id := range order {
			positions[id] = i
		}
		slices.SortFunc(units, func(a, b dumpDataPlan) int { return positions[a.Table.ID] - positions[b.Table.ID] })
	}
	return units, ctx.Err()
}
