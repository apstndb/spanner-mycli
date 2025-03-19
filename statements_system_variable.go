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
		ColumnNames:   columnNames,
		Rows:          sliceOf(toRow(row...)),
		KeepVariables: true,
	}, nil
}

type ShowVariablesStatement struct{}

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
		ColumnNames:   []string{"name", "value"},
		Rows:          rows,
		KeepVariables: true,
	}, nil
}

type SetStatement struct {
	VarName string
	Value   string
}

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

func (s *SetAddStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.systemVariables.Add(s.VarName, s.Value); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type HelpVariablesStatement struct{}

func (s *HelpVariablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	type variableDesc struct {
		Name        string
		typ         []string
		Description string
	}

	var merged []variableDesc
	for k, v := range systemVariableDefMap {
		var typ []string
		if v.Accessor.Getter != nil {
			typ = append(typ, "read")
		}

		if v.Accessor.Setter != nil {
			typ = append(typ, "write")
		}

		if v.Accessor.Adder != nil {
			typ = append(typ, "add")
		}

		if len(typ) == 0 {
			continue
		}
		merged = append(merged, variableDesc{Name: k, typ: typ, Description: v.Description})
	}

	rows := slices.SortedFunc(xiter.Map(func(v variableDesc) Row { return toRow(v.Name, strings.Join(v.typ, ","), v.Description) }, slices.Values(merged)), func(lhs Row, rhs Row) int {
		return strings.Compare(lhs[0], rhs[0])
	})

	return &Result{
		ColumnNames:   []string{"name", "type", "desc"},
		Rows:          rows,
		KeepVariables: true,
	}, nil
}
