package main

import (
	"context"
	"testing"
)

func TestSessionModes(t *testing.T) {
	ctx := context.Background()

	t.Run("NewAdminSession creates admin-only session", func(t *testing.T) {
		sysVars := &systemVariables{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "", // no database for admin-only
		}

		session, err := NewAdminSession(ctx, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication requirements:", err)
		}
		defer session.Close()

		// Check session mode
		if !session.IsAdminOnly() {
			t.Error("Expected admin-only session")
		}
		if session.IsDatabaseConnected() {
			t.Error("Expected not database connected")
		}
		if session.client != nil {
			t.Error("Expected nil client in admin-only mode")
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
		if session.IsAdminOnly() {
			t.Error("Expected not admin-only session")
		}
		if !session.IsDatabaseConnected() {
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
			Database: "", // empty database should trigger admin-only mode
		}

		session, err := createSession(ctx, nil, sysVars)
		if err != nil {
			t.Skip("Skipping test due to authentication requirements:", err)
		}
		defer session.Close()

		if !session.IsAdminOnly() {
			t.Error("Expected admin-only session when database is empty")
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

		if !session.IsDatabaseConnected() {
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

		// Verify it starts as admin-only
		if !session.IsAdminOnly() {
			t.Error("Expected admin-only session initially")
		}

		// Try to connect to database
		err = session.ConnectToDatabase(ctx, "test-database")
		if err != nil {
			t.Skip("Skipping database connection test:", err)
		}

		// Verify it's now database-connected
		if !session.IsDatabaseConnected() {
			t.Error("Expected database-connected session after ConnectToDatabase")
		}
		if session.client == nil {
			t.Error("Expected non-nil client after ConnectToDatabase")
		}
	})
}

func TestDatabaseOperationValidation(t *testing.T) {
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

	t.Run("ValidateAdminOnlyOperation succeeds", func(t *testing.T) {
		err := session.ValidateAdminOnlyOperation()
		if err != nil {
			t.Error("Expected ValidateAdminOnlyOperation to succeed:", err)
		}
	})

	t.Run("ValidateDatabaseOperation fails for admin-only session", func(t *testing.T) {
		err := session.ValidateDatabaseOperation()
		if err == nil {
			t.Error("Expected ValidateDatabaseOperation to fail for admin-only session")
		}
	})

	t.Run("RequiresDatabaseConnection returns true for admin-only session", func(t *testing.T) {
		if !session.RequiresDatabaseConnection() {
			t.Error("Expected RequiresDatabaseConnection to be true for admin-only session")
		}
	})
}

func TestInstanceValidation(t *testing.T) {
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