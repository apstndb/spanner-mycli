package main

import (
	"testing"
)

func TestStatementStringer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		stmt     Statement
		expected string
	}{
		// Core SQL statements
		{
			name:     "SelectStatement",
			stmt:     &SelectStatement{Query: "SELECT * FROM users WHERE id = 1"},
			expected: "SELECT * FROM users WHERE id = 1",
		},
		{
			name:     "DmlStatement",
			stmt:     &DmlStatement{Dml: "UPDATE users SET name = 'Alice' WHERE id = 1"},
			expected: "UPDATE users SET name = 'Alice' WHERE id = 1",
		},
		{
			name:     "DdlStatement",
			stmt:     &DdlStatement{Ddl: "CREATE TABLE users (id INT64) PRIMARY KEY (id)"},
			expected: "CREATE TABLE users (id INT64) PRIMARY KEY (id)",
		},
		{
			name:     "BulkDdlStatement single",
			stmt:     &BulkDdlStatement{Ddls: []string{"CREATE TABLE t1 (id INT64) PRIMARY KEY (id)"}},
			expected: "CREATE TABLE t1 (id INT64) PRIMARY KEY (id)",
		},
		{
			name: "BulkDdlStatement multiple",
			stmt: &BulkDdlStatement{Ddls: []string{
				"CREATE TABLE t1 (id INT64) PRIMARY KEY (id)",
				"CREATE TABLE t2 (id INT64) PRIMARY KEY (id)",
			}},
			expected: "CREATE TABLE t1 (id INT64) PRIMARY KEY (id);\nCREATE TABLE t2 (id INT64) PRIMARY KEY (id)",
		},
		{
			name:     "ExplainStatement",
			stmt:     &ExplainStatement{Explain: "SELECT * FROM users"},
			expected: "EXPLAIN SELECT * FROM users",
		},
		{
			name:     "DescribeStatement",
			stmt:     &DescribeStatement{Statement: "SELECT * FROM users"},
			expected: "DESCRIBE SELECT * FROM users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if the statement implements fmt.Stringer
			stringer, ok := tt.stmt.(interface{ String() string })
			if !ok {
				t.Fatalf("%T does not implement String() method", tt.stmt)
			}

			// Check the String() output
			got := stringer.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestSourceFileEchoIntegration tests that ECHO works correctly with sourced files
func TestSourceFileEchoIntegration(t *testing.T) {
	t.Parallel()
	// This is a conceptual test showing what we want to verify
	// In a real integration test, this would:
	// 1. Create a SQL file with various statement types
	// 2. Execute it with CLI_ECHO_INPUT = TRUE
	// 3. Verify that all statements are echoed correctly

	stmts := []Statement{
		&SelectStatement{Query: "SELECT 1"},
		&DmlStatement{Dml: "UPDATE t SET x = 1"},
		&DdlStatement{Ddl: "CREATE TABLE t (id INT64) PRIMARY KEY (id)"},
		&ExplainStatement{Explain: "SELECT * FROM t"},
		&DescribeStatement{Statement: "SELECT * FROM t"},
	}

	// Verify all can produce String() output
	for _, stmt := range stmts {
		stringer, ok := stmt.(interface{ String() string })
		if !ok {
			t.Errorf("%T should implement String() for ECHO support in source files", stmt)
			continue
		}

		// Verify String() returns non-empty result
		str := stringer.String()
		if str == "" {
			t.Errorf("%T.String() returned empty string", stmt)
		}
	}
}

// TestStatementStringerConsistency verifies that String() output can be parsed back
func TestStatementStringerConsistency(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		stmt Statement
	}{
		{
			name: "SelectStatement",
			stmt: &SelectStatement{Query: "SELECT * FROM users"},
		},
		{
			name: "DmlStatement",
			stmt: &DmlStatement{Dml: "INSERT INTO users (id, name) VALUES (1, 'Alice')"},
		},
		{
			name: "DdlStatement",
			stmt: &DdlStatement{Ddl: "ALTER TABLE users ADD COLUMN age INT64"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stringer, ok := tt.stmt.(interface{ String() string })
			if !ok {
				t.Skip("Statement does not implement String()")
			}

			// Get the String() output
			str := stringer.String()

			// Try to parse it back (this verifies the output is valid SQL)
			_, err := BuildStatement(str)
			if err != nil {
				t.Errorf("String() output could not be parsed: %v", err)
			}
		})
	}
}
