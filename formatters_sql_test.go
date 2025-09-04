package main

import (
	"testing"

	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSimpleTablePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantPath  *ast.Path
		wantError string
	}{
		{
			name:  "simple table name",
			input: "Users",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "Users"}},
			},
		},
		{
			name:  "schema qualified name",
			input: "myschema.Users",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "myschema"}, {Name: "Users"}},
			},
		},
		{
			name:  "reserved word as table name",
			input: "Order",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "Order"}},
			},
		},
		{
			name:  "three-part name",
			input: "catalog.schema.table",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "catalog"}, {Name: "schema"}, {Name: "table"}},
			},
		},
		{
			name:  "table name with spaces around",
			input: "  Users  ",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "Users"}},
			},
		},
		{
			name:      "empty string",
			input:     "",
			wantError: "CLI_SQL_TABLE_NAME must be set for SQL export formats",
		},
		{
			name:      "only spaces",
			input:     "   ",
			wantError: "CLI_SQL_TABLE_NAME must be set for SQL export formats",
		},
		{
			name:      "empty part in path",
			input:     "schema..table",
			wantError: `empty identifier in table path: "schema..table"`,
		},
		{
			name:      "trailing dot",
			input:     "schema.table.",
			wantError: `empty identifier in table path: "schema.table."`,
		},
		{
			name:      "leading dot",
			input:     ".table",
			wantError: `empty identifier in table path: ".table"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSimpleTablePath(tt.input)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantError, err.Error())
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, len(tt.wantPath.Idents), len(got.Idents))
				for i, ident := range tt.wantPath.Idents {
					assert.Equal(t, ident.Name, got.Idents[i].Name)
				}
				// Verify SQL output works (important for reserved words)
				_ = got.SQL()
			}
		})
	}
}

func TestExtractTableNameFromQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		query         string
		wantTableName string
		wantError     string // Expected error message (substring match)
		description   string
	}{
		// Supported patterns
		{
			name:          "simple SELECT *",
			query:         "SELECT * FROM Users",
			wantTableName: "Users",
			wantError:     "",
			description:   "basic SELECT * FROM table",
		},
		{
			name:          "SELECT * with WHERE",
			query:         "SELECT * FROM Users WHERE age > 18",
			wantTableName: "Users",
			wantError:     "",
			description:   "SELECT * with WHERE clause",
		},
		{
			name:          "SELECT * with ORDER BY",
			query:         "SELECT * FROM Users ORDER BY created_at DESC",
			wantTableName: "Users",
			wantError:     "",
			description:   "SELECT * with ORDER BY clause",
		},
		{
			name:          "SELECT * with LIMIT",
			query:         "SELECT * FROM Users LIMIT 100",
			wantTableName: "Users",
			wantError:     "",
			description:   "SELECT * with LIMIT clause",
		},
		{
			name:          "SELECT * with WHERE and ORDER BY",
			query:         "SELECT * FROM Users WHERE status = 'ACTIVE' ORDER BY name",
			wantTableName: "Users",
			wantError:     "",
			description:   "SELECT * with multiple clauses",
		},
		{
			name:          "SELECT * with WHERE, ORDER BY, and LIMIT",
			query:         "SELECT * FROM Products WHERE price > 100 ORDER BY price DESC LIMIT 10",
			wantTableName: "Products",
			wantError:     "",
			description:   "SELECT * with all supported clauses",
		},
		{
			name:          "schema qualified table",
			query:         "SELECT * FROM myschema.Users",
			wantTableName: "myschema.Users",
			wantError:     "",
			description:   "schema-qualified table name",
		},
		{
			name:          "reserved word as table name",
			query:         "SELECT * FROM `Order`",
			wantTableName: "`Order`",
			wantError:     "",
			description:   "reserved word as table name (quoted)",
		},
		{
			name:          "table with alias",
			query:         "SELECT * FROM Users AS u",
			wantTableName: "Users",
			wantError:     "",
			description:   "table with alias should still extract actual table name",
		},
		{
			name:          "table with alias no AS",
			query:         "SELECT * FROM Users u",
			wantTableName: "Users",
			wantError:     "",
			description:   "table with alias without AS keyword",
		},
		{
			name:          "SELECT * with hint",
			query:         "@{FORCE_INDEX=UsersByAge} SELECT * FROM Users",
			wantTableName: "Users",
			wantError:     "",
			description:   "SELECT * with query hint",
		},
		{
			name:          "SELECT * with hint and table alias",
			query:         "@{FORCE_INDEX=UsersByAge} SELECT * FROM Users u",
			wantTableName: "Users",
			wantError:     "",
			description:   "Query hint with table alias should extract actual table name",
		},

		// Unsupported patterns (should return empty string with error)
		{
			name:          "SELECT with column list",
			query:         "SELECT id, name FROM Users",
			wantTableName: "Users",
			wantError:     "",
			description:   "column list is now supported",
		},
		{
			name:          "SELECT with single column",
			query:         "SELECT name FROM Users",
			wantTableName: "Users",
			wantError:     "",
			description:   "single column is now supported",
		},
		{
			name:          "SELECT columns with WHERE",
			query:         "SELECT id, name FROM Users WHERE status = 'ACTIVE'",
			wantTableName: "Users",
			wantError:     "",
			description:   "column selection with WHERE clause",
		},
		{
			name:          "SELECT columns with ORDER BY",
			query:         "SELECT id, name FROM Users ORDER BY created_at DESC",
			wantTableName: "Users",
			wantError:     "",
			description:   "column selection with ORDER BY",
		},
		{
			name:          "SELECT columns with LIMIT",
			query:         "SELECT id, name FROM Users LIMIT 100",
			wantTableName: "Users",
			wantError:     "",
			description:   "column selection with LIMIT",
		},
		{
			name:          "SELECT DISTINCT columns",
			query:         "SELECT DISTINCT status, region FROM Users",
			wantTableName: "Users",
			wantError:     "",
			description:   "DISTINCT with specific columns",
		},
		{
			name:          "SELECT * mixed with columns",
			query:         "SELECT *, name FROM Users",
			wantTableName: "",
			wantError:     "SELECT * cannot be mixed",
			description:   "* mixed with columns not supported (causes duplicate columns)",
		},
		{
			name:          "SELECT columns mixed with *",
			query:         "SELECT name, * FROM Users",
			wantTableName: "",
			wantError:     "SELECT * cannot be mixed",
			description:   "columns mixed with * not supported (causes duplicate columns)",
		},
		{
			name:          "SELECT with JOIN",
			query:         "SELECT * FROM Users u JOIN Orders o ON u.id = o.user_id",
			wantTableName: "",
			wantError:     "only simple table references are supported",
			description:   "JOIN not supported",
		},
		{
			name:          "SELECT with INNER JOIN",
			query:         "SELECT * FROM Users INNER JOIN Orders ON Users.id = Orders.user_id",
			wantTableName: "",
			wantError:     "only simple table references are supported",
			description:   "INNER JOIN not supported",
		},
		{
			name:          "SELECT with LEFT JOIN",
			query:         "SELECT * FROM Users LEFT JOIN Orders ON Users.id = Orders.user_id",
			wantTableName: "",
			wantError:     "only simple table references are supported",
			description:   "LEFT JOIN not supported",
		},
		{
			name:          "SELECT with subquery in FROM",
			query:         "SELECT * FROM (SELECT * FROM Users)",
			wantTableName: "",
			wantError:     "only simple table references are supported",
			description:   "subquery in FROM not supported",
		},
		{
			name:          "SELECT with UNION",
			query:         "SELECT * FROM Users UNION ALL SELECT * FROM Admins",
			wantTableName: "",
			wantError:     "query type not supported",
			description:   "UNION not supported",
		},
		{
			name:          "SELECT with CTE",
			query:         "WITH active_users AS (SELECT * FROM Users WHERE status = 'ACTIVE') SELECT * FROM active_users",
			wantTableName: "",
			wantError:     "CTE (WITH clause) not supported",
			description:   "CTE not supported",
		},
		{
			name:          "INSERT statement",
			query:         "INSERT INTO Users (name, email) VALUES ('John', 'john@example.com')",
			wantTableName: "",
			wantError:     "not a SELECT statement",
			description:   "non-SELECT statement",
		},
		{
			name:          "UPDATE statement",
			query:         "UPDATE Users SET name = 'Jane' WHERE id = 1",
			wantTableName: "",
			wantError:     "not a SELECT statement",
			description:   "non-SELECT statement",
		},
		{
			name:          "DELETE statement",
			query:         "DELETE FROM Users WHERE id = 1",
			wantTableName: "",
			wantError:     "not a SELECT statement",
			description:   "non-SELECT statement",
		},
		{
			name:          "invalid SQL",
			query:         "NOT VALID SQL",
			wantTableName: "",
			wantError:     "cannot parse SQL",
			description:   "invalid SQL should return empty",
		},
		{
			name:          "SELECT without FROM",
			query:         "SELECT 1",
			wantTableName: "",
			wantError:     "no FROM clause found",
			description:   "SELECT without FROM clause",
		},
		{
			name:          "SELECT * with GROUP BY",
			query:         "SELECT * FROM Users GROUP BY status",
			wantTableName: "",
			wantError:     "GROUP BY not supported",
			description:   "GROUP BY changes result set structure through aggregation",
		},
		{
			name:          "SELECT * with HAVING",
			query:         "SELECT * FROM Users GROUP BY status HAVING COUNT(*) > 5",
			wantTableName: "",
			wantError:     "GROUP BY not supported", // GROUP BY is checked before HAVING
			description:   "HAVING requires GROUP BY which changes result set structure",
		},
		{
			name:          "SELECT DISTINCT *",
			query:         "SELECT DISTINCT * FROM Users",
			wantTableName: "Users",
			wantError:     "",
			description:   "DISTINCT is allowed (no-op on unique Spanner results)",
		},
		{
			name:          "SELECT DISTINCT * with WHERE",
			query:         "SELECT DISTINCT * FROM Users WHERE status = 'ACTIVE'",
			wantTableName: "Users",
			wantError:     "",
			description:   "DISTINCT with WHERE is allowed",
		},
		{
			name:          "SELECT DISTINCT * with ORDER BY and LIMIT",
			query:         "SELECT DISTINCT * FROM Users ORDER BY name LIMIT 10",
			wantTableName: "Users",
			wantError:     "",
			description:   "DISTINCT with ORDER BY and LIMIT is allowed",
		},
		{
			name:          "SELECT with table.* notation",
			query:         "SELECT Users.* FROM Users",
			wantTableName: "",
			wantError:     "only * or simple column names supported",
			description:   "table.* notation is not supported for auto-detection",
		},
		{
			name:          "SELECT with multiple stars",
			query:         "SELECT *, * FROM Users",
			wantTableName: "",
			wantError:     "SELECT * cannot be mixed",
			description:   "multiple * not supported (leads to duplicate columns)",
		},
		{
			name:          "SELECT with expression",
			query:         "SELECT id, age * 2 FROM Users",
			wantTableName: "",
			wantError:     "only * or simple column names supported",
			description:   "expressions not supported for auto-detection",
		},
		{
			name:          "SELECT with function call",
			query:         "SELECT id, UPPER(name) FROM Users",
			wantTableName: "",
			wantError:     "only * or simple column names supported",
			description:   "function calls not supported for auto-detection",
		},
		{
			name:          "SELECT with aliased column",
			query:         "SELECT id, name AS full_name FROM Users",
			wantTableName: "",
			wantError:     "only * or simple column names supported",
			description:   "column aliases not supported for auto-detection",
		},
		{
			name:          "SELECT with qualified column",
			query:         "SELECT Users.id, Users.name FROM Users",
			wantTableName: "",
			wantError:     "only * or simple column names supported",
			description:   "qualified column names not supported for auto-detection",
		},
		{
			name:          "SELECT AS STRUCT",
			query:         "SELECT AS STRUCT id, name FROM Users",
			wantTableName: "",
			wantError:     "SELECT AS STRUCT not supported",
			description:   "SELECT AS STRUCT is not supported for auto-detection",
		},
		{
			name:          "SELECT * with UNNEST",
			query:         "SELECT * FROM UNNEST([1, 2, 3])",
			wantTableName: "",
			wantError:     "only simple table references are supported",
			description:   "UNNEST not supported",
		},
		{
			name:          "SELECT * with TABLESAMPLE",
			query:         "SELECT * FROM Users TABLESAMPLE BERNOULLI (10 PERCENT)",
			wantTableName: "Users",
			wantError:     "",
			description:   "TABLESAMPLE should still work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractTableNameFromQuery(tt.query)

			if tt.wantError != "" {
				// Expecting an error
				require.Error(t, err, "expected error for: %s", tt.description)
				assert.Contains(t, err.Error(), tt.wantError, "error message should contain expected substring")
				assert.Empty(t, got, "table name should be empty when error is returned")
			} else if tt.wantTableName != "" {
				// Expecting successful extraction
				require.NoError(t, err, "unexpected error for: %s", tt.description)
				assert.Equal(t, tt.wantTableName, got, tt.description)
			} else {
				// Expecting empty result with error (unsupported pattern)
				assert.Empty(t, got, "table name should be empty for unsupported pattern")
				// Error is expected for unsupported patterns
				require.Error(t, err, "should return error for unsupported pattern: %s", tt.description)
			}
		})
	}
}

func TestParseSimpleTablePathSQL(t *testing.T) {
	t.Parallel()
	// Test that reserved words are properly quoted in SQL output
	tests := []struct {
		name    string
		input   string
		wantSQL string
	}{
		{
			name:    "simple table",
			input:   "Users",
			wantSQL: "Users",
		},
		{
			name:    "reserved word ORDER",
			input:   "Order",
			wantSQL: "`Order`", // Reserved words are auto-quoted by memefish
		},
		{
			name:    "schema qualified",
			input:   "myschema.Users",
			wantSQL: "myschema.Users",
		},
		{
			name:    "reserved words in path",
			input:   "select.from.where",
			wantSQL: "`select`.`from`.`where`", // All reserved words are quoted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := parseSimpleTablePath(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSQL, path.SQL())
		})
	}
}
