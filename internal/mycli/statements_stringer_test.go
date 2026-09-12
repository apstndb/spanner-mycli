package mycli

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
