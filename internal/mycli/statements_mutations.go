package mycli

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/memebridge"
	"github.com/apstndb/spanner-mycli/internal/mycli/iterutil"
	"github.com/apstndb/spanvalue"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/char"
	"github.com/google/uuid"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
	"google.golang.org/protobuf/types/known/structpb"
)

type MutateStatement struct {
	Table     string
	Operation string
	Body      string
}

func (MutateStatement) isMutationStatement() {}

func (s *MutateStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	frozen, mutations, err := freezeMutate(s.Table, s.Operation, s.Body)
	if err != nil {
		return nil, err
	}
	result, err := session.txn.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		if admitErr := session.txn.admitMutationsLocked(frozen); admitErr != nil {
			return 0, nil, nil, admitError(admitErr)
		}
		err = tx.BufferWrite(mutations)
		if recErr := session.txn.completeMutationsLocked(err); err == nil {
			err = recErr
		}
		return 0, nil, nil, err
	})
	if err != nil {
		return nil, err
	}
	return &Result{
		CommitStats:     result.CommitResponse.CommitStats,
		CommitTimestamp: result.CommitResponse.CommitTs,
	}, nil
}

// Helper functions for this file

func decode[T any](gcv spanner.GenericColumnValue) (T, error) {
	var v T
	err := gcv.Decode(&v)
	return v, err
}

func gcvToKeyable(gcv spanner.GenericColumnValue) (any, error) {
	// See spanner.Key.
	if _, ok := gcv.Value.GetKind().(*structpb.Value_NullValue); ok {
		return nil, nil
	}

	switch gcv.Type.GetCode() {
	case sppb.TypeCode_INT64, sppb.TypeCode_ENUM:
		return decode[int64](gcv)
	case sppb.TypeCode_FLOAT64:
		return decode[float64](gcv)
	case sppb.TypeCode_FLOAT32:
		return decode[float32](gcv)
	case sppb.TypeCode_BOOL:
		return decode[bool](gcv)
	case sppb.TypeCode_BYTES:
		return decode[[]byte](gcv)
	case sppb.TypeCode_STRING:
		return decode[string](gcv)
	case sppb.TypeCode_TIMESTAMP:
		return decode[time.Time](gcv)
	case sppb.TypeCode_DATE:
		return decode[civil.Date](gcv)
	case sppb.TypeCode_NUMERIC:
		return decode[big.Rat](gcv)
	case sppb.TypeCode_UUID:
		return decode[uuid.UUID](gcv)
	default:
		s, err := spanvalue.FormatColumnLiteral(gcv)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unsupported value for key: %v", s)
	}
}

func freezeKeyRangeCall(e *ast.CallExpr) (*frozenKeyRange, error) {
	if len(e.Func.Idents) != 1 || !char.EqualFold(e.Func.Idents[0].Name, "KEY_RANGE") {
		return nil, fmt.Errorf("func name is not KEY_RANGE: %v", e.SQL())
	}
	if len(e.Args) > 0 {
		return nil, fmt.Errorf("unknown args: %v", e.SQL())
	}
	namedArgMap := lo.Associate(e.NamedArgs, func(u *ast.NamedArg) (string, ast.Expr) {
		return strings.ToLower(u.Name.Name), u.Value
	})
	startClosed, hasStartClosed := namedArgMap["start_closed"]
	startOpen, hasStartOpen := namedArgMap["start_open"]
	endClosed, hasEndClosed := namedArgMap["end_closed"]
	endOpen, hasEndOpen := namedArgMap["end_open"]
	var kind spanner.KeyRangeKind
	var start, end ast.Expr
	switch {
	case hasStartOpen && hasStartClosed:
		return nil, fmt.Errorf("start_open and start_closed are mutually exclusive")
	case hasEndOpen && hasEndClosed:
		return nil, fmt.Errorf("end_open and end_closed are mutually exclusive")
	case hasStartClosed && hasEndOpen:
		kind = spanner.ClosedOpen
		start, end = startClosed, endOpen
	case hasStartClosed && hasEndClosed:
		kind = spanner.ClosedClosed
		start, end = startClosed, endClosed
	case hasStartOpen && hasEndClosed:
		kind = spanner.OpenClosed
		start, end = startOpen, endClosed
	case hasStartOpen && hasEndOpen:
		kind = spanner.OpenOpen
		start, end = startOpen, endOpen
	default:
		return nil, fmt.Errorf("unknown status: %v", e.SQL())
	}

	startRow, err := keyRangeBound(start)
	if err != nil {
		return nil, err
	}
	endRow, err := keyRangeBound(end)
	if err != nil {
		return nil, err
	}
	return &frozenKeyRange{
		Start: cloneGCVRow(startRow),
		End:   cloneGCVRow(endRow),
		Kind:  kind,
	}, nil
}

func keyRangeBound(expr ast.Expr) ([]spanner.GenericColumnValue, error) {
	_, values, err := parseLiteralExpr(expr)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("unknown start: %v", expr.SQL())
	}
	return values[0], nil
}

func toKeys(values []spanner.GenericColumnValue) (spanner.Key, error) {
	key, err := lo.MapErr(values, func(value spanner.GenericColumnValue, _ int) (any, error) {
		return gcvToKeyable(value)
	})
	if err != nil {
		return nil, err
	}
	return spanner.Key(key), nil
}

func typeValueToGCV(k *sppb.StructType_Field, v *structpb.Value) spanner.GenericColumnValue {
	return spanner.GenericColumnValue{
		Type:  k.GetType(),
		Value: v,
	}
}

func extractStructValuesUsingType(fields []*sppb.StructType_Field) func(v *structpb.Value) []spanner.GenericColumnValue {
	return func(v *structpb.Value) []spanner.GenericColumnValue {
		return extractStructValues(fields, v.GetListValue().GetValues())
	}
}

func convertToColumnsValues(gcv spanner.GenericColumnValue) ([]string, [][]spanner.GenericColumnValue, error) {
	switch gcv.Type.GetCode() {
	case sppb.TypeCode_STRUCT:
		structTypefields := gcv.Type.GetStructType().GetFields()

		// [values]
		return extractColumnNames(structTypefields),
			sliceOf(extractStructValues(structTypefields, gcv.Value.GetListValue().GetValues())),
			nil
	case sppb.TypeCode_ARRAY:
		if gcv.Type.GetArrayElementType().GetCode() != sppb.TypeCode_STRUCT {
			return nil, slices.Collect(loi.Map(slices.Values(gcv.Value.GetListValue().GetValues()), func(v *structpb.Value) []spanner.GenericColumnValue {
				return sliceOf(spanner.GenericColumnValue{
					Type:  gcv.Type.GetArrayElementType(),
					Value: v,
				})
			})), nil
		}
		structTypeFields := gcv.Type.GetArrayElementType().GetStructType().GetFields()
		return extractColumnNames(structTypeFields),
			slices.Collect(loi.Map(slices.Values(gcv.Value.GetListValue().GetValues()), extractStructValuesUsingType(structTypeFields))),
			nil
	default:
		// [[value]]
		return nil, sliceOf(sliceOf(gcv)), nil
	}
}

func parseLiteralExpr(expr ast.Expr) ([]string, [][]spanner.GenericColumnValue, error) {
	gcv, err := memebridge.MemefishExprToGCV(expr)
	if err != nil {
		return nil, nil, fmt.Errorf("expression is not a supported literal, expr: %v, err: %w", expr.SQL(), err)
	}
	return convertToColumnsValues(gcv)
}

func parseLiteralString(s string) ([]string, [][]spanner.GenericColumnValue, error) {
	expr, err := parseMemefishExpr("", s)
	if err != nil {
		return nil, nil, err
	}
	return parseLiteralExpr(expr)
}

func extractStructValues(structTypefields []*sppb.StructType_Field, structValues []*structpb.Value) []spanner.GenericColumnValue {
	return slices.Collect(iterutil.ZipShortestBy(slices.Values(structTypefields), slices.Values(structValues), func(field *sppb.StructType_Field, value *structpb.Value) spanner.GenericColumnValue {
		return typeValueToGCV(field, value)
	}))
}

func parseMutation(table, op, s string) ([]*spanner.Mutation, error) {
	_, mutations, err := freezeMutate(table, op, s)
	return mutations, err
}
