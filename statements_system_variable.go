package main

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	scxiter "spheric.cloud/xiter"
)

type ShowVariableStatement struct {
	VarName string
}

func (s *ShowVariableStatement) isDetachedCompatible() {}

func (s *ShowVariableStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	value, err := session.systemVariables.Get(s.VarName)
	if err != nil {
		return nil, err
	}

	columnNames := slices.Sorted(maps.Keys(value))
	var row []string
	for n := range slices.Values(columnNames) {
		row = append(row, value[n])
	}
	return &Result{
		TableHeader:   toTableHeader(columnNames),
		Rows:          sliceOf(toRow(row...)),
		KeepVariables: true,
	}, nil
}

type ShowVariablesStatement struct{}

func (s *ShowVariablesStatement) isDetachedCompatible() {}

func (s *ShowVariablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	merged := make(map[string]string)
	for k, v := range systemVariableDefMap {
		if v.Accessor.Getter == nil {
			continue
		}

		value, err := v.Accessor.Getter(session.systemVariables, k)
		if errors.Is(err, errIgnored) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for k, v := range value {
			merged[k] = v
		}
	}

	rows := slices.SortedFunc(
		scxiter.MapLower(maps.All(merged), func(k, v string) Row { return toRow(k, v) }),
		ToSortFunc(func(r Row) string { return r[0] /* name */ }))

	return &Result{
		TableHeader:   toTableHeader("name", "value"),
		Rows:          rows,
		KeepVariables: true,
	}, nil
}

type SetStatement struct {
	VarName string
	Value   string
}

func (s *SetStatement) isDetachedCompatible() {}

func (s *SetStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.systemVariables.Set(s.VarName, s.Value); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type SetAddStatement struct {
	VarName string
	Value   string
}

func (s *SetAddStatement) isDetachedCompatible() {}

func (s *SetAddStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.systemVariables.Add(s.VarName, s.Value); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type HelpVariablesStatement struct{}

func (s *HelpVariablesStatement) isDetachedCompatible() {}

func (s *HelpVariablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	type variableDesc struct {
		Name        string
		Operations  []string
		Description string
	}

	var merged []variableDesc
	for k, v := range systemVariableDefMap {
		var ops []string
		if v.Accessor.Getter != nil {
			ops = append(ops, "read")
		}

		if v.Accessor.Setter != nil {
			ops = append(ops, "write")
		}

		if v.Accessor.Adder != nil {
			ops = append(ops, "add")
		}

		if len(ops) == 0 {
			continue
		}
		merged = append(merged, variableDesc{Name: k, Operations: ops, Description: v.Description})
	}

	rows := slices.SortedFunc(xiter.Map(func(v variableDesc) Row { return toRow(v.Name, strings.Join(v.Operations, ","), v.Description) }, slices.Values(merged)), func(lhs Row, rhs Row) int {
		return strings.Compare(lhs[0], rhs[0])
	})

	return &Result{
		TableHeader:   toTableHeader("name", "operations", "desc"),
		Rows:          rows,
		AffectedRows:  len(rows),
		KeepVariables: true,
	}, nil
}
