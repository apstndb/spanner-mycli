// This file contains the registry-based implementation of system variables
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"spheric.cloud/xiter"
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

// setFromGoogleSQL implements setting variables from GoogleSQL mode
func (sv *systemVariables) setFromGoogleSQL(name string, value string) error {
	upperName := strings.ToUpper(name)

	// Special case for COMMIT_RESPONSE (unimplemented setter)
	if upperName == "COMMIT_RESPONSE" {
		return errSetterUnimplemented{name}
	}

	// Special case for CLI_DIRECT_READ (unimplemented setter)
	if upperName == "CLI_DIRECT_READ" {
		return errSetterUnimplemented{name}
	}

	slog.Debug("setFromGoogleSQL calling Registry.Set", "upperName", upperName, "value", value)
	err := sv.Registry.Set(upperName, value, true)
	slog.Debug("setFromGoogleSQL after Registry.Set", "err", err)
	// Convert to legacy error types for compatibility
	if err != nil {
		if errors.Is(err, errSetterReadOnly) {
			return errSetterReadOnly
		}
		var unknownErr *ErrUnknownVariable
		if errors.As(err, &unknownErr) {
			return fmt.Errorf("unknown variable name: %v", name)
		}
		var unimplementedErr *errSetterUnimplemented
		if errors.As(err, &unimplementedErr) {
			return errSetterUnimplemented{name}
		}
	}

	return err
}

// setFromSimple implements setting variables from simple mode
func (sv *systemVariables) setFromSimple(name string, value string) error {
	upperName := strings.ToUpper(name)

	// Special case for COMMIT_RESPONSE (unimplemented setter)
	if upperName == "COMMIT_RESPONSE" {
		return errSetterUnimplemented{name}
	}

	// Special case for CLI_DIRECT_READ (unimplemented setter)
	if upperName == "CLI_DIRECT_READ" {
		return errSetterUnimplemented{name}
	}

	err := sv.Registry.Set(upperName, value, false)
	// Convert to legacy error types for compatibility
	if err != nil {
		if errors.Is(err, errSetterReadOnly) {
			return errSetterReadOnly
		}
		var unknownErr *ErrUnknownVariable
		if errors.As(err, &unknownErr) {
			return fmt.Errorf("unknown variable name: %v", name)
		}
		var unimplementedErr *errSetterUnimplemented
		if errors.As(err, &unimplementedErr) {
			return errSetterUnimplemented{name}
		}
	}

	return err
}

// addFromGoogleSQL implements ADD from GoogleSQL mode
func (sv *systemVariables) addFromGoogleSQL(name string, value string) error {
	// Parse GoogleSQL value
	value = parseGoogleSQLValue(value)

	err := sv.Registry.Add(name, value)
	// Convert to legacy error types for compatibility
	if err != nil {
		var addErr *ErrAddNotSupported
		if errors.As(err, &addErr) {
			return fmt.Errorf("%s does not support ADD operation", name)
		}
		var unknownErr *ErrUnknownVariable
		if errors.As(err, &unknownErr) {
			return fmt.Errorf("unknown variable name: %v", name)
		}
	}

	return err
}

// addFromSimple implements ADD from simple mode
func (sv *systemVariables) addFromSimple(name string, value string) error {
	err := sv.Registry.Add(name, value)
	// Convert to legacy error types for compatibility
	if err != nil {
		var addErr *ErrAddNotSupported
		if errors.As(err, &addErr) {
			return fmt.Errorf("%s does not support ADD operation", name)
		}
		var unknownErr *ErrUnknownVariable
		if errors.As(err, &unknownErr) {
			return fmt.Errorf("unknown variable name: %v", name)
		}
	}

	return err
}

// get retrieves variable value
func (sv *systemVariables) get(name string) (map[string]string, error) {
	upperName := strings.ToUpper(name)

	// Special case for COMMIT_RESPONSE
	// This variable returns multiple key-value pairs (COMMIT_TIMESTAMP and MUTATION_COUNT)
	// which doesn't fit the single-string return type of the Variable interface.
	// This behavior is maintained for java-spanner compatibility where SHOW VARIABLE
	// COMMIT_RESPONSE returns both values as a result set.
	if upperName == "COMMIT_RESPONSE" {
		if sv.CommitResponse == nil {
			return nil, errIgnored
		}
		return map[string]string{
			"COMMIT_TIMESTAMP": formatTimestamp(sv.CommitTimestamp, "NULL"),
			"MUTATION_COUNT":   strconv.FormatInt(sv.CommitResponse.GetCommitStats().GetMutationCount(), 10),
		}, nil
	}

	// Special case for CLI_DIRECT_READ (complex proto type not in registry)
	if upperName == "CLI_DIRECT_READ" {
		if sv.DirectedRead == nil {
			return nil, errIgnored
		}
		// Format DirectedRead for display
		values := xiter.Join(xiter.Map(
			slices.Values(sv.DirectedRead.GetIncludeReplicas().GetReplicaSelections()),
			func(rs *sppb.DirectedReadOptions_ReplicaSelection) string {
				return fmt.Sprintf("%s:%s", rs.GetLocation(), rs.GetType())
			},
		), ";")
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
