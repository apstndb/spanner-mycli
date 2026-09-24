package mycli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Common errors for variable operations
var (
	errSetterReadOnly            = errors.New("variable is read-only")
	errSetterInTransaction       = errors.New("can't change variable when there is an active transaction")
	errSetterInManualBatch       = errors.New("can't change variable while a manual batch is open")
	errTransactionTagInReadWrite = errors.New("TRANSACTION_TAG cannot be changed while a read-write transaction is active")
	errResetSnapshotsMissing     = errors.New("RESET ALL: startup snapshots were not captured")
	errResetUnsupported          = errors.New("variable does not support RESET")
)

// Error types for proper error handling with errors.Is/As
type (
	// ErrUnknownVariable is returned when a variable name is not recognized
	ErrUnknownVariable struct {
		Name        string
		Suggestions []string
	}

	// ErrAddNotSupported is returned when ADD operation is not supported for a variable
	ErrAddNotSupported struct {
		Name string
	}

	// errSetterInitOnly is returned when an init-only variable (initOnly) is set
	// after the session has been created. It replaces the ad-hoc error the
	// CLI_ENABLE_ADC_PLUS custom setter used to return.
	errSetterInitOnly struct {
		Name string
	}
)

func (e *ErrUnknownVariable) Error() string {
	return fmt.Sprintf("unknown variable: %s%s", e.Name, e.hint())
}

func (e *ErrUnknownVariable) hint() string {
	if len(e.Suggestions) == 0 {
		return ""
	}
	return "; did you mean " + strings.Join(e.Suggestions, " or ") + "?"
}

func (r *VarRegistry) unknownVariable(name string) *ErrUnknownVariable {
	err := &ErrUnknownVariable{Name: name}
	upper := strings.ToUpper(name)
	r.forEachDef(func(def *varDef) {
		if nearbyVariableName(upper, def.name) {
			err.Suggestions = append(err.Suggestions, def.name)
		}
	})
	slices.Sort(err.Suggestions)
	// Ambiguous suggestions are less useful than the original diagnostic.
	if len(err.Suggestions) > 3 {
		err.Suggestions = nil
	}
	return err
}

// nearbyVariableName accepts one insertion, deletion, substitution, or adjacent
// transposition. Variable names are ASCII; distant guesses are intentionally
// excluded, and suggestions never change a setting automatically.
func nearbyVariableName(a, b string) bool {
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		a, b = a[1:], b[1:]
	}
	switch len(a) - len(b) {
	case 0:
		return len(a) == 0 || a[1:] == b[1:] ||
			(len(a) >= 2 && a[0] == b[1] && a[1] == b[0] && a[2:] == b[2:])
	case 1:
		return a[1:] == b
	case -1:
		return a == b[1:]
	default:
		return false
	}
}

func (e *errSetterInitOnly) Error() string {
	return fmt.Sprintf("%s cannot be changed after session creation", e.Name)
}

func (e *ErrAddNotSupported) Error() string {
	return fmt.Sprintf("ADD not supported for %s", e.Name)
}
