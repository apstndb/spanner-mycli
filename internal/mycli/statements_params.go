package mycli

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/apstndb/lox"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/samber/lo"
	scxiter "spheric.cloud/xiter"
)

type ShowParamsStatement struct{}

func (s *ShowParamsStatement) isDetachedCompatible() {}

func (s *ShowParamsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	strMap := make(map[string]string)
	for k, v := range session.systemVariables.Params {
		strMap[k] = v.SQL()
	}

	rows := slices.SortedFunc(
		scxiter.MapLower(maps.All(session.systemVariables.Params), func(k string, v ast.Node) Row {
			return toRow(k, lo.Ternary(lox.InstanceOf[ast.Type](v), "TYPE", "VALUE"), v.SQL())
		}),
		func(lhs, rhs Row) int { return cmp.Compare(lhs[0], rhs[0]) /* parameter name */ })

	return &Result{
		TableHeader:   toTableHeader("Param_Name", "Param_Kind", "Param_Value"),
		Rows:          rows,
		KeepVariables: true,
	}, nil
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
