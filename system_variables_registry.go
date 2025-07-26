package main

import (
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

// createSystemVariableRegistry creates and configures the parser registry for system variables.
// This function sets up all the system variable parsers with their getters and setters.
func createSystemVariableRegistry(sv *systemVariables) *sysvar.Registry {
	// For now, return an empty registry to allow the code to compile
	// The full migration will be implemented in a subsequent PR
	return sysvar.NewRegistry()
}
