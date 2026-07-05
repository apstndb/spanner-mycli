// This file contains the registry-based implementation of system variables
package mycli

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	loi "github.com/samber/lo/it"
)

// ensureRegistry initializes the registry if needed.
func (sv *systemVariables) ensureRegistry() {
	if sv.Registry == nil {
		slog.Debug("Creating new registry", "svPtr", fmt.Sprintf("%p", sv))
		sv.Registry = NewVarRegistry(sv)
	} else {
		slog.Debug("Registry already exists", "svPtr", fmt.Sprintf("%p", sv))
	}
}

// setFrom sets a variable through the registry, translating registry error
// types into the legacy messages callers and tests expect. isGoogleSQL selects
// GoogleSQL vs simple value parsing.
func (sv *systemVariables) setFrom(name string, value string, isGoogleSQL bool) error {
	upperName := strings.ToUpper(name)

	// CLI_DIRECT_READ lives outside the registry (complex proto type; see get);
	// its setter is not yet implemented. COMMIT_RESPONSE is now a read-only
	// registry def, so it no longer needs a special case here (Registry.Set
	// rejects it with errSetterReadOnly).
	if upperName == "CLI_DIRECT_READ" {
		return errSetterUnimplemented{name}
	}

	slog.Debug("setFrom calling Registry.Set", "upperName", upperName, "value", value, "isGoogleSQL", isGoogleSQL)
	err := sv.Registry.Set(upperName, value, isGoogleSQL)
	slog.Debug("setFrom after Registry.Set", "err", err)
	// ErrUnknownVariable carries a different message ("unknown variable: X");
	// translate it to the legacy "unknown variable name: X" form. Other registry
	// errors (read-only, unimplemented setter, parse errors) already carry the
	// message callers expect, so they pass through unchanged.
	var unknownErr *ErrUnknownVariable
	if errors.As(err, &unknownErr) {
		return fmt.Errorf("unknown variable name: %v", name)
	}

	return err
}

// addFrom performs ADD through the registry, translating registry error types
// into the legacy messages. isGoogleSQL selects GoogleSQL value parsing.
func (sv *systemVariables) addFrom(name string, value string, isGoogleSQL bool) error {
	if isGoogleSQL {
		value = parseGoogleSQLValue(value)
	}

	err := sv.Registry.Add(name, value)
	var addErr *ErrAddNotSupported
	if errors.As(err, &addErr) {
		return fmt.Errorf("%s does not support ADD operation", name)
	}
	var unknownErr *ErrUnknownVariable
	if errors.As(err, &unknownErr) {
		return fmt.Errorf("unknown variable name: %v", name)
	}

	return err
}

// get retrieves variable value
func (sv *systemVariables) get(name string) (map[string]string, error) {
	upperName := strings.ToUpper(name)

	// Multi-valued variables (COMMIT_RESPONSE) return several columns, which
	// don't fit the single-string Variable.Get; SHOW VARIABLE reads them via the
	// MultiValueVar capability. GetVariable resolves aliases and returns nil for
	// unknown names (the nil type assertion below simply falls through).
	if mv, ok := sv.Registry.GetVariable(upperName).(MultiValueVar); ok {
		return mv.GetMulti()
	}

	// Special case for CLI_DIRECT_READ (complex proto type not in registry)
	if upperName == "CLI_DIRECT_READ" {
		if sv.Query.DirectedRead == nil {
			return nil, errIgnored
		}
		// Format DirectedRead for display
		values := strings.Join(slices.Collect(loi.Map(
			slices.Values(sv.Query.DirectedRead.GetIncludeReplicas().GetReplicaSelections()),
			func(rs *sppb.DirectedReadOptions_ReplicaSelection) string {
				return fmt.Sprintf("%s:%s", rs.GetLocation(), rs.GetType())
			},
		)), ";")
		return singletonMap(name, values), nil
	}

	value, err := sv.Registry.Get(upperName)
	if err != nil {
		// Convert to legacy error types for compatibility
		if errors.Is(err, errGetterUnimplemented{}) {
			return nil, errGetterUnimplemented{name}
		}
		var unknownErr *ErrUnknownVariable
		if errors.As(err, &unknownErr) {
			return nil, fmt.Errorf("unknown variable name: %v", name)
		}
		return nil, err
	}

	return singletonMap(name, value), nil
}
