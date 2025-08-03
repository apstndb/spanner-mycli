package main

import (
	"context"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// TestDetachedSessionSystemVariables tests system variable operations in Detached session mode (admin operation only mode)
// without requiring actual Google Cloud authentication.
// Note: This is a basic test to ensure system variables work in Detached mode.
// We don't need exhaustive examples here - just verify the mechanism works.
func TestDetachedSessionSystemVariables(t *testing.T) {
	ctx := context.Background()

	// Create a mock admin session without actual authentication
	sysVars := &systemVariables{
		Project:   "test-project",
		Instance:  "test-instance",
		Database:  "",
		CLIFormat: enums.DisplayModeTable,
		Verbose:   false,
		ReadOnly:  false,
		Prompt:    "spanner> ",
		Params:    make(map[string]ast.Node),
	}

	// Create a minimal admin session for testing
	session := &Session{
		mode:            Detached,
		client:          nil, // no database client in detached mode
		adminClient:     nil, // we won't actually use the admin client in these tests
		systemVariables: sysVars,
	}
	// Important: Set CurrentSession reference
	sysVars.CurrentSession = session

	// Just test that SHOW VARIABLES works without error
	t.Run("SHOW VARIABLES works in AdminOnly session", func(t *testing.T) {
		stmt := &ShowVariablesStatement{}
		result, err := stmt.Execute(ctx, session)

		if err != nil {
			t.Errorf("SHOW VARIABLES failed: %v", err)
		} else if result == nil {
			t.Error("SHOW VARIABLES returned nil result")
		} else if len(result.Rows) == 0 {
			t.Error("SHOW VARIABLES returned no rows")
		} else {
			t.Logf("SHOW VARIABLES returned %d rows successfully", len(result.Rows))
		}
	})

	// Test that SET and SHOW PARAMS work without error
	t.Run("SET/SHOW PARAMS work in AdminOnly session", func(t *testing.T) {
		// Set param type
		typeStmt := &SetParamTypeStatement{Name: "p1", Type: "STRING"}
		_, err := typeStmt.Execute(ctx, session)
		if err != nil {
			t.Errorf("SET PARAM TYPE failed: %v", err)
		}

		// Verify with SHOW PARAMS
		showStmt := &ShowParamsStatement{}
		result, err := showStmt.Execute(ctx, session)
		if err != nil {
			t.Errorf("SHOW PARAMS failed: %v", err)
		} else if result == nil {
			t.Error("SHOW PARAMS returned nil result")
		} else {
			t.Logf("SHOW PARAMS returned %d rows successfully", len(result.Rows))
		}
	})
}
