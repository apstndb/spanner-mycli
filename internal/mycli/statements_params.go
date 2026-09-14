package mycli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/apstndb/spancodec"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/samber/lo"
)

type ShowParamsStatement struct{}

func (s *ShowParamsStatement) isDetachedCompatible() {}

func (s *ShowParamsStatement) allowedDuringSavepointRecovery() {}

// paramRow is the row shape for SHOW PARAMS. Param_Value is the memefish
// SQL rendering of the parameter, so it stays a string by design.
type paramRow struct {
	Name  string `spanner:"Param_Name"`
	Kind  string `spanner:"Param_Kind"`
	Value string `spanner:"Param_Value"`
}

var showParamsRowEncoder = spancodec.MustNewRowEncoder[paramRow]()

func paramKind(v ast.Node) string {
	switch v.(type) {
	case ast.Type:
		return "TYPE"
	default:
		return "VALUE"
	}
}

func (s *ShowParamsStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	items := lo.MapToSlice(session.systemVariables.Params, func(k string, v ast.Node) paramRow {
		return paramRow{
			Name:  k,
			Kind:  paramKind(v),
			Value: v.SQL(),
		}
	})
	slices.SortFunc(items, func(lhs, rhs paramRow) int { return cmp.Compare(lhs.Name, rhs.Name) })

	result, err := executeStructRows(showParamsRowEncoder, items, session, out)
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

func (s *UnsetParamStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	if err := unsetParam(session.systemVariables.Params, s.Name); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type SetParamTypeStatement struct {
	Name string
	Type string
}

func (s *SetParamTypeStatement) isDetachedCompatible() {}

func (s *SetParamTypeStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	expr, err := parseMemefishType("", s.Type)
	if err != nil {
		return nil, err
	}
	if err := setParam(session.systemVariables.Params, s.Name, expr); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type SetParamValueStatement struct {
	Name  string
	Value string
}

func (s *SetParamValueStatement) isDetachedCompatible() {}

func (s *SetParamValueStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	expr, err := parseMemefishExpr("", s.Value)
	if err != nil {
		return nil, err
	}
	if err := setParam(session.systemVariables.Params, s.Name, expr); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

// errAmbiguousQueryParameter is the sentinel for a preexisting parameter map
// that contains case aliases with different stored identities.
var errAmbiguousQueryParameter = errors.New("ambiguous query parameter")

// ambiguousQueryParameterError lists the stored spellings of a logical
// parameter that cannot be used as one binding. Aliases is sorted.
type ambiguousQueryParameterError struct {
	Name    string
	Aliases []string
}

func (e *ambiguousQueryParameterError) Error() string {
	return fmt.Sprintf("%s %q: conflicting bindings %s", errAmbiguousQueryParameter.Error(), e.Name, strings.Join(e.Aliases, ", "))
}

func (e *ambiguousQueryParameterError) Unwrap() error {
	return errAmbiguousQueryParameter
}

// paramNodeIdentity is the stored parameter identity. It is the param kind
// plus the memefish SQL() rendering, not a semantic comparison of AST values.
func paramNodeIdentity(n ast.Node) string {
	return paramKind(n) + "\x00" + n.SQL()
}

// paramAliases returns the stored keys that EqualFold name, sorted so callers
// never observe Go map iteration order.
func paramAliases(params map[string]ast.Node, name string) []string {
	var aliases []string
	for k := range params {
		if strings.EqualFold(k, name) {
			aliases = append(aliases, k)
		}
	}
	slices.Sort(aliases)
	return aliases
}

// conflictingParamAliases reports an error when aliases of one logical name
// have different stored identities. Identical aliases (same kind and SQL()
// rendering) are accepted as one logical parameter.
func conflictingParamAliases(name string, aliases []string, params map[string]ast.Node) error {
	if len(aliases) < 2 {
		return nil
	}
	ident := paramNodeIdentity(params[aliases[0]])
	for _, alias := range aliases[1:] {
		if paramNodeIdentity(params[alias]) != ident {
			return &ambiguousQueryParameterError{Name: name, Aliases: slices.Clone(aliases)}
		}
	}
	return nil
}

// checkParamMapAmbiguity walks a preexisting parameter map in sorted key order
// and rejects groups of case aliases whose stored identities differ.
func checkParamMapAmbiguity(params map[string]ast.Node) error {
	seen := make(map[string]struct{}, len(params))
	for _, name := range slices.Sorted(maps.Keys(params)) {
		if _, ok := seen[name]; ok {
			continue
		}
		aliases := paramAliases(params, name)
		for _, alias := range aliases {
			seen[alias] = struct{}{}
		}
		if err := conflictingParamAliases(name, aliases, params); err != nil {
			return err
		}
	}
	return nil
}

// lookupParam resolves one logical parameter for name. Identical aliases
// share a value; conflicting aliases are an error. ok is false when name is
// unknown.
func lookupParam(params map[string]ast.Node, name string) (ast.Node, bool, error) {
	aliases := paramAliases(params, name)
	if len(aliases) == 0 {
		return nil, false, nil
	}
	if err := conflictingParamAliases(name, aliases, params); err != nil {
		return nil, false, err
	}
	return params[aliases[0]], true, nil
}

// setParam stores value under one logical name. A sequential SET PARAM of a
// name that already exists updates that entry and keeps the first stored
// spelling. If a preexisting map already contains multiple identical aliases,
// the lexicographically first stored spelling is kept because Go maps have no
// insertion order. Conflicting aliases are rejected without mutating params.
func setParam(params map[string]ast.Node, name string, value ast.Node) error {
	aliases := paramAliases(params, name)
	if len(aliases) == 0 {
		params[name] = value
		return nil
	}
	if err := conflictingParamAliases(name, aliases, params); err != nil {
		return err
	}
	keep := aliases[0]
	params[keep] = value
	for _, alias := range aliases[1:] {
		delete(params, alias)
	}
	return nil
}

func unsetParam(params map[string]ast.Node, name string) error {
	aliases := paramAliases(params, name)
	if len(aliases) == 0 {
		return fmt.Errorf("unknown parameter: %s", name)
	}
	if err := conflictingParamAliases(name, aliases, params); err != nil {
		return err
	}
	for _, alias := range aliases {
		delete(params, alias)
	}
	return nil
}
