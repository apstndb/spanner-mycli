package mycli

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
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
	if session.systemVariables.Transaction.CommitResponse != nil {
		merged["COMMIT_TIMESTAMP"] = formatTimestamp(session.systemVariables.Transaction.CommitTimestamp, "NULL")
		merged["MUTATION_COUNT"] = strconv.FormatInt(session.systemVariables.Transaction.CommitResponse.GetCommitStats().GetMutationCount(), 10)
	}

	// Special handling for CLI_DIRECT_READ
	if session.systemVariables.Query.DirectedRead != nil {
		values := strings.Join(slices.Collect(loi.Map(
			slices.Values(session.systemVariables.Query.DirectedRead.GetIncludeReplicas().GetReplicaSelections()),
			func(rs *sppb.DirectedReadOptions_ReplicaSelection) string {
				return fmt.Sprintf("%s:%s", rs.GetLocation(), rs.GetType())
			},
		)), ";")
		merged["CLI_DIRECT_READ"] = values
	}

	items := lo.MapToSlice(merged, func(k, v string) nameValueRow {
		return nameValueRow{Name: k, Value: v}
	})
	slices.SortFunc(items, func(lhs, rhs nameValueRow) int {
		return cmp.Compare(lhs.Name, rhs.Name)
	})

	fc, err := clientSideFormatConfig(session)
	if err != nil {
		return nil, err
	}
	result, err := resultFromRowEncoder(
		nameValueRowEncoder,
		items,
		fc,
		session.systemVariables.typeStyles,
		session.systemVariables.nullStyle,
	)
	if err != nil {
		return nil, err
	}
	// Preserve pre-bridge behavior: SHOW VARIABLES did not set AffectedRows, so no
	// "N rows in set" footer is printed for this statement.
	result.AffectedRows = 0
	result.KeepVariables = true
	return result, nil
}

type SetStatement struct {
	VarName string
	Value   string
}

func (s *SetStatement) isDetachedCompatible() {}

func (s *SetStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.systemVariables.SetFromGoogleSQL(s.VarName, s.Value); err != nil {
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
	if err := session.systemVariables.AddFromGoogleSQL(s.VarName, s.Value); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type HelpVariablesStatement struct{}

func (s *HelpVariablesStatement) isDetachedCompatible() {}

func (s *HelpVariablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
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

	var merged []helpVariableRow
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

		merged = append(merged, helpVariableRow{
			Name:        name,
			Operations:  strings.Join(ops, ","),
			Description: info.Description,
		})
	}

	// Add special variables not in registry
	// COMMIT_RESPONSE - virtual variable
	merged = append(merged, helpVariableRow{
		Name:        "COMMIT_RESPONSE",
		Operations:  "read",
		Description: "The most recent response for a read-write transaction. This is a virtual variable: it can be used in SHOW COMMIT_RESPONSE and SHOW COMMIT_RESPONSE.COMMIT_TIMESTAMP and SHOW COMMIT_RESPONSE.MUTATION_COUNT, but attempting to read its value directly will give an error. Instead use the sub-fields COMMIT_TIMESTAMP and MUTATION_COUNT.",
	})

	// CLI_DIRECT_READ - complex proto type
	merged = append(merged, helpVariableRow{
		Name:        "CLI_DIRECT_READ",
		Operations:  "read",
		Description: "",
	})

	slices.SortFunc(merged, func(lhs, rhs helpVariableRow) int {
		return cmp.Compare(lhs.Name, rhs.Name)
	})

	fc, err := clientSideFormatConfig(session)
	if err != nil {
		return nil, err
	}
	var typeStyles map[sppb.TypeCode]string
	var nullStyle string
	if session != nil {
		typeStyles = session.systemVariables.typeStyles
		nullStyle = session.systemVariables.nullStyle
	}
	result, err := resultFromRowEncoder(helpVariablesRowEncoder, merged, fc, typeStyles, nullStyle)
	if err != nil {
		return nil, err
	}
	result.KeepVariables = true
	return result, nil
}

// formatTimestamp formats a timestamp for display.
// Returns defaultValue for zero time, RFC3339Nano format otherwise.
func formatTimestamp(t time.Time, defaultValue string) string {
	if t.IsZero() {
		return defaultValue
	}
	return t.Format(time.RFC3339Nano)
}
