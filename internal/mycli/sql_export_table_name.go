// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"fmt"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// extractTableNameFromQuery attempts to extract a table name from a simple SELECT query
// for the SQL export formats (SQL_INSERT and variants).
// It supports simple SELECT patterns including:
//   - SELECT * FROM table_name
//   - SELECT columns FROM table_name
//   - SELECT * FROM table_name WHERE ...
//   - SELECT * FROM table_name ORDER BY ...
//   - SELECT * FROM table_name LIMIT ...
//   - SELECT DISTINCT columns FROM table_name (DISTINCT is allowed since Spanner tables always have PKs)
//   - Combinations of the above
//
// NOT supported:
//   - GROUP BY / HAVING (aggregations change the result set structure)
//   - JOINs (combine multiple tables)
//   - Subqueries, CTEs, UNIONs (complex structures)
//
// Note: DISTINCT is allowed because Spanner tables always have primary keys,
// making SELECT * results inherently unique. DISTINCT on unique results is a no-op.
//
// Returns:
//   - (tableName, nil) when extraction succeeds
//   - ("", error) when extraction fails with a reason
//
// The error messages are intended for debug logging to help understand why
// auto-detection failed, allowing users to adjust their queries or use explicit
// CLI_SQL_TABLE_NAME setting.
func extractTableNameFromQuery(sql string) (string, error) {
	// Validation flow:
	// 1. Parse SQL and verify it's a SELECT statement
	// 2. Navigate through AST structure (handling Query wrapper for ORDER BY/LIMIT)
	// 3. Check for unsupported features (GROUP BY, HAVING, CTEs)
	// 4. Verify it's SELECT * (not specific columns)
	// 5. Extract table name from FROM clause

	// Parse the SQL statement
	stmt, err := memefish.ParseStatement("", sql)
	if err != nil {
		// Parse error - can't auto-detect
		return "", fmt.Errorf("cannot parse SQL: %w", err)
	}

	// Check if it's a QueryStatement with a Select query
	queryStmt, ok := stmt.(*ast.QueryStatement)
	if !ok {
		return "", fmt.Errorf("not a SELECT statement")
	}

	// Handle different query structures
	// When ORDER BY or LIMIT is present at the top level, memefish wraps the Select in a Query
	var selectStmt *ast.Select

	switch q := queryStmt.Query.(type) {
	case *ast.Select:
		// Simple SELECT without top-level ORDER BY/LIMIT
		selectStmt = q
	case *ast.Query:
		// SELECT with ORDER BY/LIMIT at the top level
		// Check for CTE (WITH clause) - not supported
		if q.With != nil {
			return "", fmt.Errorf("CTE (WITH clause) not supported for auto-detection")
		}
		// Check if the inner query is a Select
		if innerSelect, ok := q.Query.(*ast.Select); ok {
			selectStmt = innerSelect
		} else {
			// Complex query structure - not supported
			return "", fmt.Errorf("complex query structure not supported (subqueries, UNION, etc.)")
		}
	default:
		// Could be a subquery, UNION, etc. - not supported
		return "", fmt.Errorf("query type not supported for auto-detection")
	}

	// Check for SELECT AS STRUCT - not supported for auto-detection
	if selectStmt.As != nil {
		return "", fmt.Errorf("SELECT AS STRUCT not supported for auto-detection")
	}

	// Check if there's a FROM clause first
	if selectStmt.From == nil {
		// No FROM clause - not a table query
		return "", fmt.Errorf("no FROM clause found")
	}

	// Check for GROUP BY - not supported because aggregation changes result structure
	if selectStmt.GroupBy != nil {
		return "", fmt.Errorf("GROUP BY not supported (aggregation changes result set structure)")
	}

	// Check for HAVING - not supported (only appears with GROUP BY)
	if selectStmt.Having != nil {
		return "", fmt.Errorf("HAVING not supported (aggregation changes result set structure)")
	}

	// Note: DISTINCT is allowed because Spanner tables always have primary keys,
	// so SELECT * results are inherently unique. DISTINCT doesn't change the result set.

	// Check for SELECT * mixed with other columns, which is not supported
	// because it can lead to duplicate column names in the generated INSERT statements.
	hasStar := false
	for _, r := range selectStmt.Results {
		if _, ok := r.(*ast.Star); ok {
			hasStar = true
			break
		}
	}
	if hasStar && len(selectStmt.Results) > 1 {
		return "", fmt.Errorf("SELECT * cannot be mixed with other columns or used multiple times for auto-detection")
	}

	// Check SELECT list - only allow * or simple column names (Ident)
	// Other patterns like table.*, expressions, functions are not auto-detected
	// Also check for duplicate column names to avoid invalid INSERT statements
	seenColumns := make(map[string]struct{})
	for _, result := range selectStmt.Results {
		switch r := result.(type) {
		case *ast.Star:
			// SELECT * is allowed
			continue
		case *ast.ExprSelectItem:
			// Check if the expression is a simple identifier
			// Reject any select items that are more complex than a simple identifier
			if ident, ok := r.Expr.(*ast.Ident); ok {
				// Check for duplicate column names (case-insensitive)
				// In Spanner, ALL identifiers (including column names) are case-insensitive,
				// even when quoted. For example, `A` and `a` refer to the same column.
				lowerName := strings.ToLower(ident.Name)
				if _, exists := seenColumns[lowerName]; exists {
					return "", fmt.Errorf("duplicate column name %q in SELECT list not supported for auto-detection", ident.Name)
				}
				seenColumns[lowerName] = struct{}{}
				// Simple column name is allowed
				continue
			}
			// Other expressions (functions, operators, path expressions) are not supported
			return "", fmt.Errorf("only * or simple column names supported for auto-detection (found expression: %T)", r.Expr)
		default:
			// Any other pattern is not supported for auto-detection
			// This includes ast.Alias (column aliases like "name AS full_name"),
			// ast.DotStar (table.* notation), and other complex SelectItem types
			return "", fmt.Errorf("only * or simple column names supported for auto-detection (found %T)", result)
		}
	}

	// Check if it's a simple table reference (no JOINs, subqueries, etc.)
	// memefish returns different types for simple vs qualified table names
	var tableName string

	switch source := selectStmt.From.Source.(type) {
	case *ast.TableName:
		// Simple table name (e.g., Users, `Order`)
		if source.Table == nil {
			// This shouldn't happen with valid memefish AST, but handle it gracefully
			return "", fmt.Errorf("unable to extract table name from query structure")
		}
		tableName = source.Table.SQL()

	case *ast.PathTableExpr:
		// Schema-qualified table name (e.g., myschema.Users)
		if source.Path == nil || len(source.Path.Idents) == 0 {
			// This shouldn't happen with valid memefish AST, but handle it gracefully
			return "", fmt.Errorf("unable to extract table path from query structure")
		}
		tableName = source.Path.SQL()

	default:
		// Could be JOIN, subquery, UNNEST, etc. - not supported
		return "", fmt.Errorf("only simple table references are supported (no JOINs, subqueries, UNNEST, etc.)")
	}

	return tableName, nil
}
