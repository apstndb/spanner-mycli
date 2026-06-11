package mycli

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/apstndb/lox"
	"github.com/apstndb/spancodec"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/samber/lo"
)

type ShowParamsStatement struct{}

func (s *ShowParamsStatement) isDetachedCompatible() {}

// paramRow is the row shape for SHOW PARAMS. Param_Value is the memefish
// SQL rendering of the parameter, so it stays a string by design.
type paramRow struct {
	Name  string `spanner:"Param_Name"`
	Kind  string `spanner:"Param_Kind"`
	Value string `spanner:"Param_Value"`
}

var showParamsRowEncoder = spancodec.MustNewRowEncoder[paramRow]()

func (s *ShowParamsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	items := lo.MapToSlice(session.systemVariables.Params, func(k string, v ast.Node) paramRow {
		return paramRow{
			Name:  k,
			Kind:  lo.Ternary(lox.InstanceOf[ast.Type](v), "TYPE", "VALUE"),
			Value: v.SQL(),
		}
	})
	slices.SortFunc(items, func(lhs, rhs paramRow) int { return cmp.Compare(lhs.Name, rhs.Name) })

	result, err := executeStructRows(showParamsRowEncoder, items, session.systemVariables)
	if err != nil {
		return nil, err
	}
	result.KeepVariables = true
	return result, nil
}

type UnsetParamStatement struct {
	Name string
}

func (s *UnsetParamStatement) isDetachedCompatible() {}

func (s *UnsetParamStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if _, ok := session.systemVariables.Params[s.Name]; !ok {
		return nil, fmt.Errorf("unknown parameter: %s", s.Name)
	}
	delete(session.systemVariables.Params, s.Name)
	return &Result{KeepVariables: true}, nil
}

type SetParamTypeStatement struct {
	Name string
	Type string
}

func (s *SetParamTypeStatement) isDetachedCompatible() {}

func (s *SetParamTypeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if expr, err := memefish.ParseType("", s.Type); err != nil {
		return nil, err
	} else {
		session.systemVariables.Params[s.Name] = expr
		return &Result{KeepVariables: true}, nil
	}
}

type SetParamValueStatement struct {
	Name  string
	Value string
}

func (s *SetParamValueStatement) isDetachedCompatible() {}

func (s *SetParamValueStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if expr, err := memefish.ParseExpr("", s.Value); err != nil {
		return nil, err
	} else {
		session.systemVariables.Params[s.Name] = expr
		return &Result{KeepVariables: true}, nil
	}
}
