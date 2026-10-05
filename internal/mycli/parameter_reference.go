// Copyright 2026 apstndb
// SPDX-License-Identifier: Apache-2.0

package mycli

import (
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/memebridge"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// boundParameter carries an immutable wire value in the existing parameter
// store. It implements Node only as a storage/display contract, not Expr:
// bound snapshots must never pass through local SQL expression evaluation.
type boundParameter struct{ value spanner.GenericColumnValue }

func (*boundParameter) Pos() token.Pos { return token.InvalidPos }
func (*boundParameter) End() token.Pos { return token.InvalidPos }
func (p *boundParameter) SQL() string {
	return fmt.Sprintf("<bound %s: %s>", p.value.Type, protojson.Format(p.value.Value))
}

func cloneParameterValue(v spanner.GenericColumnValue) spanner.GenericColumnValue {
	return spanner.GenericColumnValue{Type: proto.Clone(v.Type).(*sppb.Type), Value: proto.Clone(v.Value).(*structpb.Value)}
}

// resolveParameterReference snapshots references at SET time. Arithmetic and
// function calls keep the existing literal evaluator's supported subset.
func resolveParameterReference(expr ast.Expr, params map[string]ast.Node) (ast.Node, error) {
	switch e := expr.(type) {
	case *ast.Param:
		node, ok, err := lookupParam(params, e.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("unknown parameter: %s", e.Name)
		}
		switch n := node.(type) {
		case *boundParameter:
			return &boundParameter{value: cloneParameterValue(n.value)}, nil
		case ast.Expr:
			v, err := memebridge.MemefishExprToGCV(n)
			if err != nil {
				return nil, err
			}
			return &boundParameter{value: cloneParameterValue(v)}, nil
		default:
			return nil, fmt.Errorf("parameter %s has a type but no value", e.Name)
		}
	case *ast.SelectorExpr:
		node, err := resolveParameterReference(e.Expr, params)
		if err != nil {
			return nil, err
		}
		bound, ok := node.(*boundParameter)
		if !ok {
			return nil, fmt.Errorf("field access requires a parameter reference")
		}
		v := bound.value
		if v.Type.GetCode() != sppb.TypeCode_STRUCT {
			return nil, fmt.Errorf("field access requires STRUCT")
		}
		index := -1
		for i, f := range v.Type.GetStructType().GetFields() {
			if strings.EqualFold(f.GetName(), e.Ident.Name) {
				if index >= 0 {
					return nil, fmt.Errorf("ambiguous STRUCT field: %s", e.Ident.Name)
				}
				index = i
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("unknown STRUCT field: %s", e.Ident.Name)
		}
		field := v.Type.GetStructType().GetFields()[index]
		value := structpb.NewNullValue()
		if _, isNull := v.Value.GetKind().(*structpb.Value_NullValue); !isNull {
			values := v.Value.GetListValue().GetValues()
			if index >= len(values) {
				return nil, fmt.Errorf("invalid STRUCT value")
			}
			value = values[index]
		}
		return &boundParameter{value: cloneParameterValue(spanner.GenericColumnValue{Type: field.Type, Value: value})}, nil
	default:
		hasReference := false
		ast.Inspect(expr, func(n ast.Node) bool {
			if _, ok := n.(*ast.Param); ok {
				hasReference = true
			}
			return !hasReference
		})
		if hasReference {
			return nil, fmt.Errorf("only direct parameter and STRUCT field references are supported")
		}
		return expr, nil
	}
}
