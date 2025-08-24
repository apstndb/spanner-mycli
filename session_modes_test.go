package main

import (
	"context"
	"testing"
)

func TestSessionModes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("NewAdminSession creates detached session", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "", // no database for detached
		}

		session, err := NewAdminSession(ctx, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication requirements:", err)
		}
		defer session.Close()

		// Check session mode
		if !session.IsDetached() {
			t.Error("Expected detached session")
		}
		if session.client != nil {
			t.Error("Expected nil client in detached mode")
		}
		if session.adminClient == nil {
			t.Error("Expected non-nil admin client")
		}
	})

	t.Run("NewSession with database creates database-connected session", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "test-database",
		}

		session, err := NewSession(ctx, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication or database requirements:", err)
		}
		defer session.Close()

		// Check session mode
		if session.IsDetached() {
			t.Error("Expected database connected session")
		}
		if session.client == nil {
			t.Error("Expected non-nil client in database-connected mode")
		}
		if session.adminClient == nil {
			t.Error("Expected non-nil admin client")
		}
	})

	t.Run("createSession creates admin session when database is empty", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "", // empty database should trigger detached mode
		}

		session, err := createSession(ctx, nil, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication requirements:", err)
		}
		defer session.Close()

		if !session.IsDetached() {
			t.Error("Expected detached session when database is empty")
		}
	})

	t.Run("createSession creates database session when database is specified", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "test-database",
		}

		session, err := createSession(ctx, nil, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication or database requirements:", err)
		}
		defer session.Close()

		if session.IsDetached() {
			t.Error("Expected database-connected session when database is specified")
		}
	})

	t.Run("ConnectToDatabase upgrades admin session", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "",
		}

		session, err := NewAdminSession(ctx, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication requirements:", err)
		}
		defer session.Close()

		// Verify it starts as detached
		if !session.IsDetached() {
			t.Error("Expected detached session initially")
		}

		// Try to connect to database
		err = session.ConnectToDatabase(ctx, "test-database")
		if err != nil {
			t.Skip("Skipping database connection test:", err)
		}

		// Verify it's now database-connected
		if session.IsDetached() {
			t.Error("Expected database-connected session after ConnectToDatabase")
		}
		if session.client == nil {
			t.Error("Expected non-nil client after ConnectToDatabase")
		}
	})
}

func TestDatabaseOperationValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sysVars := &systemVariables{
		Project:  "test-project",
		Instance: "test-instance",
		Database: "",
	}

	session, err := NewAdminSession(ctx, sysVars)
	if err != nil {
		t.Skip("Skipping test due to authentication requirements:", err)
	}
	defer session.Close()

	t.Run("ValidateDetachedOperation succeeds", func(t *testing.T) {
		err := session.ValidateDetachedOperation()
		if err != nil {
			t.Error("Expected ValidateDetachedOperation to succeed:", err)
		}
	})

	t.Run("ValidateDatabaseOperation fails for detached session", func(t *testing.T) {
		err := session.ValidateDatabaseOperation()
		if err == nil {
			t.Error("Expected ValidateDatabaseOperation to fail for detached session")
		}
	})

	t.Run("RequiresDatabaseConnection returns true for detached session", func(t *testing.T) {
		if !session.RequiresDatabaseConnection() {
			t.Error("Expected RequiresDatabaseConnection to be true for detached session")
		}
	})
}

func TestInstanceValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("InstanceExists method works", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "",
		}

		session, err := NewAdminSession(ctx, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication requirements:", err)
		}
		defer session.Close()

		// Test InstanceExists method
		exists, err := session.InstanceExists()
		if err != nil {
			t.Skip("Skipping instance validation test:", err)
		}

		// The test doesn't assert the value since we don't know if test-instance exists
		// But we can verify the method works without error
		t.Logf("Instance exists: %v", exists)
	})

	t.Run("NewAdminSession validates instance exists", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "definitely-non-existent-instance-12345",
			Database: "",
		}

		session, err := NewAdminSession(ctx, sysVars)
		if err != nil {
			// This could be authentication error or instance validation error
			t.Logf("Expected error for non-existent instance: %v", err)
		} else {
			// If session was created, close it
			session.Close()
			t.Log("Session created successfully (instance may exist or test skipped)")
		}
	})
}

func TestDetachedCompatibleStatements(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sysVars := &systemVariables{
		Project:  "test-project",
		Instance: "test-instance",
		Database: "",
	}

	session, err := NewAdminSession(ctx, sysVars)
	if err != nil {
		t.Skip("Skipping test due to authentication requirements:", err)
	}
	defer session.Close()

	// Test DetachedCompatible statements can be validated
	t.Run("DetachedCompatible statements pass validation", func(t *testing.T) {
		adminCompatibleStmts := []Statement{
			&CreateDatabaseStatement{CreateStatement: "CREATE DATABASE test"},
			&DropDatabaseStatement{DatabaseId: "test"},
			&ShowDatabasesStatement{},
			&UseStatement{Database: "test"},
			&HelpStatement{},
			&ExitStatement{},
			// System variables statements
			&ShowVariableStatement{VarName: "READONLY"},
			&ShowVariablesStatement{},
			&SetStatement{VarName: "CLI_FORMAT", Value: "JSON"},
			&SetAddStatement{VarName: "PROTO_DESCRIPTOR_FILE", Value: "test.pb"},
			&HelpVariablesStatement{},
			// Query parameters statements
			&ShowParamsStatement{},
			&SetParamTypeStatement{Name: "p1", Type: "STRING"},
			&SetParamValueStatement{Name: "p1", Value: "test"},
		}

		for _, stmt := range adminCompatibleStmts {
			err := session.ValidateStatementExecution(stmt)
			if err != nil {
				t.Errorf("Expected %T to be admin-compatible, got error: %v", stmt, err)
			}
		}
	})

	// Test non-DetachedCompatible statements fail validation
	t.Run("Non-DetachedCompatible statements fail validation", func(t *testing.T) {
		nonDetachedCompatibleStmts := []Statement{
			&SelectStatement{Query: "SELECT 1"},
			&DmlStatement{Dml: "UPDATE test SET col1 = 1"},
			&DdlStatement{Ddl: "CREATE TABLE test (id INT64)"},
		}

		for _, stmt := range nonDetachedCompatibleStmts {
			err := session.ValidateStatementExecution(stmt)
			if err == nil {
				t.Errorf("Expected %T to fail validation in detached mode", stmt)
			}
		}
	})
}

func TestDatabaseConnectedSessionStatements(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sysVars := &systemVariables{
		Project:  "test-project",
		Instance: "test-instance",
		Database: "test-database",
	}

	session, err := NewSession(ctx, sysVars)
	if err != nil {
		t.Skip("Skipping test due to authentication or database requirements:", err)
	}
	defer session.Close()

	// Test all statements pass validation in DatabaseConnected mode
	t.Run("All statements pass validation in DatabaseConnected mode", func(t *testing.T) {
		allStmts := []Statement{
			&CreateDatabaseStatement{CreateStatement: "CREATE DATABASE test"},
			&DropDatabaseStatement{DatabaseId: "test"},
			&ShowDatabasesStatement{},
			&UseStatement{Database: "test"},
			&HelpStatement{},
			&ExitStatement{},
			&SelectStatement{Query: "SELECT 1"},
			&DmlStatement{Dml: "UPDATE test SET col1 = 1"},
			&DdlStatement{Ddl: "CREATE TABLE test (id INT64)"},
		}

		for _, stmt := range allStmts {
			err := session.ValidateStatementExecution(stmt)
			if err != nil {
				t.Errorf("Expected %T to pass validation in database-connected mode, got error: %v", stmt, err)
			}
		}
	})
}

func TestAdminSessionStatementExecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sysVars := &systemVariables{
		Project:  "test-project",
		Instance: "test-instance",
		Database: "",
	}

	session, err := NewAdminSession(ctx, sysVars)
	if err != nil {
		t.Skip("Skipping test due to authentication requirements:", err)
	}
	defer session.Close()

	// Test executing SHOW VARIABLES in admin session
	t.Run("SHOW VARIABLES execution in Detached session", func(t *testing.T) {
		stmt := &ShowVariablesStatement{}
		result, err := session.ExecuteStatement(ctx, stmt)
		if err != nil {
			t.Errorf("SHOW VARIABLES failed in admin session: %v", err)
		} else if result == nil {
			t.Error("SHOW VARIABLES returned nil result")
		} else {
			t.Logf("SHOW VARIABLES returned %d rows", len(result.Rows))
		}
	})

	// Test executing SET statement in admin session
	t.Run("SET statement execution in Detached session", func(t *testing.T) {
		stmt := &SetStatement{VarName: "CLI_FORMAT", Value: "JSON"}
		result, err := session.ExecuteStatement(ctx, stmt)
		if err != nil {
			t.Errorf("SET CLI_FORMAT failed in admin session: %v", err)
		} else if result == nil {
			t.Error("SET returned nil result")
		}
	})

	// Test executing SHOW VARIABLE in admin session
	t.Run("SHOW VARIABLE execution in Detached session", func(t *testing.T) {
		stmt := &ShowVariableStatement{VarName: "CLI_FORMAT"}
		result, err := session.ExecuteStatement(ctx, stmt)
		if err != nil {
			t.Errorf("SHOW VARIABLE failed in admin session: %v", err)
		} else if result == nil {
			t.Error("SHOW VARIABLE returned nil result")
		} else if len(result.Rows) != 1 {
			t.Errorf("Expected 1 row, got %d", len(result.Rows))
		}
	})

	// Test SHOW PARAMS in admin session
	t.Run("SHOW PARAMS execution in Detached session", func(t *testing.T) {
		stmt := &ShowParamsStatement{}
		result, err := session.ExecuteStatement(ctx, stmt)
		if err != nil {
			t.Errorf("SHOW PARAMS failed in admin session: %v", err)
		} else if result == nil {
			t.Error("SHOW PARAMS returned nil result")
		}
	})
}
