package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
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
	// Get all variables from the registry
	merged := session.systemVariables.ListVariables()

	// Special handling for COMMIT_RESPONSE
	if session.systemVariables.CommitResponse != nil {
		merged["COMMIT_TIMESTAMP"] = formatTimestamp(&session.systemVariables.CommitTimestamp)
		merged["MUTATION_COUNT"] = strconv.FormatInt(session.systemVariables.CommitResponse.GetCommitStats().GetMutationCount(), 10)
	}

	// Special handling for CLI_DIRECT_READ
	if session.systemVariables.DirectedRead != nil {
		values := scxiter.Join(scxiter.Map(
			slices.Values(session.systemVariables.DirectedRead.GetIncludeReplicas().GetReplicaSelections()),
			func(rs *sppb.DirectedReadOptions_ReplicaSelection) string {
				return fmt.Sprintf("%s:%s", rs.GetLocation(), rs.GetType())
			},
		), ";")
		merged["CLI_DIRECT_READ"] = values
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
	slog.Debug("SetStatement.Execute", "varName", s.VarName, "value", s.Value, 
		"sessionPtr", fmt.Sprintf("%p", session),
		"sysVarsPtr", fmt.Sprintf("%p", session.systemVariables),
		"streamingEnabledPtr", fmt.Sprintf("%p", &session.systemVariables.StreamingEnabled))
	if err := session.systemVariables.SetFromGoogleSQL(s.VarName, s.Value); err != nil {
		return nil, err
	}
	slog.Debug("After SET", "StreamingEnabled", session.systemVariables.StreamingEnabled,
		"streamingEnabledPtr", fmt.Sprintf("%p", &session.systemVariables.StreamingEnabled))
	return &Result{KeepVariables: true}, nil
}

type SetAddStatement struct {
	VarName string
	Value   string
}

func (s *SetAddStatement) isDetachedCompatible() {}

func (s *SetAddStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.systemVariables.AddFromGoogleSQL(s.VarName, s.Value); err != nil {
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

	// Get all variable info from the registry
	var varInfo map[string]struct {
		Description string
		ReadOnly    bool
		CanAdd      bool
	}

	if session != nil {
		varInfo = session.systemVariables.ListVariableInfo()
	} else {
		// If session is nil, create a temporary systemVariables to get the variable info
		tmpSV := newSystemVariablesWithDefaults()
		tmpSV.ensureRegistry()
		varInfo = tmpSV.ListVariableInfo()
	}

	var merged []variableDesc
	for name, info := range varInfo {
		var ops []string

		// All variables support read
		ops = append(ops, "read")

		// Check if variable supports write
		if !info.ReadOnly {
			ops = append(ops, "write")
		}

		// Check if variable supports ADD
		if info.CanAdd {
			ops = append(ops, "add")
		}

		merged = append(merged, variableDesc{Name: name, Operations: ops, Description: info.Description})
	}

	// Add special variables not in registry
	// COMMIT_RESPONSE - virtual variable
	merged = append(merged, variableDesc{
		Name:        "COMMIT_RESPONSE",
		Operations:  []string{"read"},
		Description: "The most recent response for a read-write transaction. This is a virtual variable: it can be used in SHOW COMMIT_RESPONSE and SHOW COMMIT_RESPONSE.COMMIT_TIMESTAMP and SHOW COMMIT_RESPONSE.MUTATION_COUNT, but attempting to read its value directly will give an error. Instead use the sub-fields COMMIT_TIMESTAMP and MUTATION_COUNT.",
	})

	// CLI_DIRECT_READ - complex proto type
	merged = append(merged, variableDesc{
		Name:        "CLI_DIRECT_READ",
		Operations:  []string{"read"},
		Description: "",
	})

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

// formatTimestamp formats a timestamp for display
func formatTimestamp(t *time.Time) string {
	if t.IsZero() {
		return "NULL"
	}
	return t.Format(time.RFC3339Nano)
}
