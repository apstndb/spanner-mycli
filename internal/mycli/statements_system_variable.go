package mycli

import (
	"cmp"
	"context"
	"errors"
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
	if session.systemVariables.LastResult.CommitResponse != nil {
		merged["COMMIT_TIMESTAMP"] = formatTimestamp(session.systemVariables.LastResult.CommitTimestamp, "NULL")
		merged["MUTATION_COUNT"] = strconv.FormatInt(session.systemVariables.LastResult.CommitResponse.GetCommitStats().GetMutationCount(), 10)
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

	result, err := executeStructRows(nameValueRowEncoder, items, session)
	if err != nil {
		return nil, err
	}
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

// SetLocalStatement implements `SET LOCAL <name> = <value>`: the change is
// scoped to the current transaction. The previous value is recorded in the
// transaction's undo log (TransactionManager.localVarUndo) and restored when
// the transaction ends, whether by COMMIT, ROLLBACK, or CLOSE.
// Following java-spanner, SET LOCAL outside a transaction is an error.
type SetLocalStatement struct {
	VarName string
	Value   string
}

func (s *SetLocalStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.txn == nil || !session.txn.InTransaction() {
		return nil, errors.New("SET LOCAL requires an active transaction; start one with BEGIN")
	}

	sysVars := session.systemVariables
	upperName := strings.ToUpper(s.VarName)

	// Mirror the SET special cases for variables living outside the registry.
	if upperName == "COMMIT_RESPONSE" || upperName == "CLI_DIRECT_READ" {
		return nil, errSetterUnimplemented{s.VarName}
	}

	oldValue, err := sysVars.Registry.Get(upperName)
	if err != nil {
		var unknownErr *ErrUnknownVariable
		if errors.As(err, &unknownErr) {
			return nil, fmt.Errorf("unknown variable name: %v", s.VarName)
		}
		return nil, err
	}

	// Pre-flight: verify the saved value can be set back, so the restore at
	// transaction end cannot fail. This also rejects read-only variables and
	// variables whose setter refuses writes mid-transaction, before any state
	// changes. Setting the current value again is semantically a no-op.
	if err := sysVars.Registry.Set(upperName, oldValue, false); err != nil {
		return nil, fmt.Errorf("%s does not support SET LOCAL: %w", upperName, err)
	}

	if err := sysVars.SetFromGoogleSQL(s.VarName, s.Value); err != nil {
		return nil, err
	}

	if err := session.txn.pushLocalVarUndo(upperName, oldValue); err != nil {
		// The transaction ended between the check above and the push;
		// undo the set so the value does not silently outlive the transaction.
		if restoreErr := sysVars.Registry.Set(upperName, oldValue, false); restoreErr != nil {
			err = errors.Join(err, restoreErr)
		}
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

// helpVariableRows returns sorted rows describing every system variable known
// to the registry, plus the special variables handled outside the registry
// (COMMIT_RESPONSE, CLI_DIRECT_READ). It is shared by HELP VARIABLES and the
// documentation generator behind the hidden --sysvars-help flag.
func helpVariableRows(sysVars *systemVariables) []helpVariableRow {
	varInfo := sysVars.ListVariableInfo()

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
		Description: "Directed read options for read-only operations, in replica_location:replica_type format. Set by the --directed-read flag.",
	})

	slices.SortFunc(merged, func(lhs, rhs helpVariableRow) int {
		return cmp.Compare(lhs.Name, rhs.Name)
	})

	return merged
}

func (s *HelpVariablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	var sysVars *systemVariables
	if session != nil {
		sysVars = session.systemVariables
	} else {
		// If session is nil, create a temporary systemVariables to get the variable info
		tmpSV := newSystemVariablesWithDefaults()
		tmpSV.ensureRegistry()
		sysVars = &tmpSV
	}

	merged := helpVariableRows(sysVars)

	// executeStructRows handles a nil session by rendering a buffered result
	// with default formatting, preserving the pre-existing detached behavior.
	result, err := executeStructRows(helpVariablesRowEncoder, merged, session)
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
