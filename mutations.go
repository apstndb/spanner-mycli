package main

import (
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/memebridge"
	"github.com/apstndb/spanvalue"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/char"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/structpb"
)

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
	case spannerpb.TypeCode_INT64, spannerpb.TypeCode_ENUM:
		return decode[int64](gcv)
	case spannerpb.TypeCode_FLOAT64:
		return decode[float64](gcv)
	case spannerpb.TypeCode_FLOAT32:
		return decode[float32](gcv)
	case spannerpb.TypeCode_BOOL:
		return decode[bool](gcv)
	case spannerpb.TypeCode_BYTES:
		return decode[[]byte](gcv)
	case spannerpb.TypeCode_STRING:
		return decode[string](gcv)
	case spannerpb.TypeCode_TIMESTAMP:
		return decode[time.Time](gcv)
	case spannerpb.TypeCode_DATE:
		return decode[civil.Date](gcv)
	default:
		s, err := spanvalue.FormatColumnLiteral(gcv)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unsupported value for key: %v", s)
	}
}

func parseCallExpr(e *ast.CallExpr) (spanner.KeyRange, error) {
	if len(e.Func.Idents) != 1 || !char.EqualFold(e.Func.Idents[0].Name, "KEY_RANGE") {
		return spanner.KeyRange{}, fmt.Errorf("func name is not KEY_RANGE: %v", e.SQL())
	}
	if len(e.Args) > 0 {
		return spanner.KeyRange{}, fmt.Errorf("unknown args: %v", e.SQL())
	}
	namedArgMap := maps.Collect(hiter.Divide(
		func(u *ast.NamedArg) (string, ast.Expr) {
			return strings.ToLower(u.Name.Name), u.Value
		},
		slices.Values(e.NamedArgs)))
	startClosed, hasStartClosed := namedArgMap["start_closed"]
	startOpen, hasStartOpen := namedArgMap["start_open"]
	endClosed, hasEndClosed := namedArgMap["end_closed"]
	endOpen, hasEndOpen := namedArgMap["end_open"]
	var kind spanner.KeyRangeKind
	var start, end ast.Expr
	switch {
	case hasStartOpen && hasStartClosed:
		return spanner.KeyRange{}, fmt.Errorf("start_open and start_closed are mutually exclusive")
	case hasEndOpen && hasEndClosed:
		return spanner.KeyRange{}, fmt.Errorf("end_open and end_closed are mutually exclusive")
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
		return spanner.KeyRange{}, fmt.Errorf("unknown status: %v", e.SQL())
	}

	var startKey spanner.Key
	if _, values, err := parseLiteralExpr(start); err != nil {
		return spanner.KeyRange{}, err
	} else if len(values) != 1 {
		return spanner.KeyRange{}, fmt.Errorf("unknown start: %v", start.SQL())
	} else {
		startKey, err = toKeys(values[0])
		if err != nil {
			return spanner.KeyRange{}, err
		}
	}

	var endKey spanner.Key
	if _, values, err := parseLiteralExpr(end); err != nil {
		return spanner.KeyRange{}, err
	} else if len(values) != 1 {
		return spanner.KeyRange{}, fmt.Errorf("unknown end: %v", end.SQL())
	} else {
		endKey, err = toKeys(values[0])
		if err != nil {
			return spanner.KeyRange{}, err
		}
	}

	return spanner.KeyRange{
		Start: startKey,
		End:   endKey,
		Kind:  kind,
	}, nil
}

func parseDeleteMutation(table, s string) ([]*spanner.Mutation, error) {
	if strings.ToUpper(strings.TrimSpace(s)) == "ALL" {
		return sliceOf(spanner.Delete(table, spanner.AllKeys())), nil
	}
	expr, err := memefish.ParseExpr("", s)
	if err != nil {
		return nil, err
	}
	switch e := expr.(type) {
	case *ast.CallExpr:
		keyrange, err := parseCallExpr(e)
		if err != nil {
			return nil, err
		}
		return sliceOf(spanner.Delete(table, keyrange)), nil
	default:
		columns, valuesList, err := parseLiteralExpr(e)
		if err != nil {
			return nil, err
		}
		if len(columns) > 0 {
			log.Printf("delete mutation ignores column names: %v", columns)
		}

		var keys []spanner.Key
		for _, values := range valuesList {
			key, err := toKeys(values)
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}

		if len(keys) == 1 {
			return sliceOf(spanner.Delete(table, keys[0])), nil
		}
		return sliceOf(spanner.Delete(table, spanner.KeySetFromKeys(keys...))), nil
	}
}

func toKeys(values []spanner.GenericColumnValue) (spanner.Key, error) {
	var key []any
	for _, value := range values {
		keyElem, err := gcvToKeyable(value)
		if err != nil {
			return nil, err
		}
		key = append(key, keyElem)
	}
	return key, nil
}

func typeValueToGCV(k *spannerpb.StructType_Field, v *structpb.Value) spanner.GenericColumnValue {
	return spanner.GenericColumnValue{
		Type:  k.GetType(),
		Value: v,
	}
}

func extractStructValuesUsingType(fields []*spannerpb.StructType_Field) func(v *structpb.Value) []spanner.GenericColumnValue {
	return func(v *structpb.Value) []spanner.GenericColumnValue {
		return extractStructValues(fields, v.GetListValue().GetValues())
	}
}

func convertToColumnsValues(gcv spanner.GenericColumnValue) ([]string, [][]spanner.GenericColumnValue, error) {
	switch gcv.Type.GetCode() {
	case spannerpb.TypeCode_STRUCT:
		structTypefields := gcv.Type.GetStructType().GetFields()

		// [values]
		return extractColumnNames(structTypefields),
			sliceOf(extractStructValues(structTypefields, gcv.Value.GetListValue().GetValues())),
			nil
	case spannerpb.TypeCode_ARRAY:
		if gcv.Type.GetArrayElementType().GetCode() != spannerpb.TypeCode_STRUCT {
			return nil, slices.Collect(xiter.Map(func(v *structpb.Value) []spanner.GenericColumnValue {
				return sliceOf(spanner.GenericColumnValue{
					Type:  gcv.Type.GetArrayElementType(),
					Value: v,
				})
			}, slices.Values(gcv.Value.GetListValue().GetValues()))), nil
		}
		structTypeFields := gcv.Type.GetArrayElementType().GetStructType().GetFields()
		return extractColumnNames(structTypeFields),
			slices.Collect(xiter.Map(extractStructValuesUsingType(structTypeFields), slices.Values(gcv.Value.GetListValue().GetValues()))),
			nil
	default:
		// [[value]]
		return nil, sliceOf(sliceOf(gcv)), nil
	}
}

func parseLiteralExpr(expr ast.Expr) ([]string, [][]spanner.GenericColumnValue, error) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		gcv, err := memebridge.MemefishExprToGCV(e)
		if err != nil {
			return nil, nil, fmt.Errorf("expression is not a supported literal, expr: %v, err: %w", e.SQL(), err)
		}
		return convertToColumnsValues(gcv)
	case *ast.TypedStructLiteral, *ast.TupleStructLiteral, *ast.TypelessStructLiteral, *ast.ArrayLiteral:
		gcv, err := memebridge.MemefishExprToGCV(e)
		if err != nil {
			return nil, nil, fmt.Errorf("expression is not a supported literal, expr: %v, err: %w", e.SQL(), err)
		}
		return convertToColumnsValues(gcv)
	default:
		return nil, nil, fmt.Errorf("unsupported expr as literals: %v", expr.SQL())
	}
}

func parseLiteralString(s string) ([]string, [][]spanner.GenericColumnValue, error) {
	expr, err := memefish.ParseExpr("", s)
	if err != nil {
		return nil, nil, err
	}
	return parseLiteralExpr(expr)
}

func extractStructValues(structTypefields []*spannerpb.StructType_Field, structValues []*structpb.Value) []spanner.GenericColumnValue {
	return slices.Collect(hiter.Unify(
		typeValueToGCV,
		hiter.Pairs(slices.Values(structTypefields), slices.Values(structValues))))
}

func parseMutation(table, op, s string) ([]*spanner.Mutation, error) {
	if op == "DELETE" {
		return parseDeleteMutation(table, s)
	}

	columns, values, err := parseLiteralString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid write mutations: %w", err)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("column names can't be inferenced")
	}

	// The common signature of Mutation functions
	type mutationFunc func(string, []string, []any) *spanner.Mutation

	var mutationF mutationFunc
	switch strings.ToUpper(op) {
	case "INSERT":
		mutationF = spanner.Insert
	case "UPDATE":
		mutationF = spanner.Update
	case "INSERT_OR_UPDATE":
		mutationF = spanner.InsertOrUpdate
	case "REPLACE":
		mutationF = spanner.Replace
	default:
		return nil, fmt.Errorf("unsupported operation: %q", op)
	}

	var mutations []*spanner.Mutation
	for _, v := range values {
		mutations = append(mutations, mutationF(table, columns, lo.ToAnySlice(v)))
	}
	return mutations, nil
}
