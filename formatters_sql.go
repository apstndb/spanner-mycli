// formatters_sql.go implements SQL export formatting for query results.
// It generates INSERT, INSERT OR IGNORE, and INSERT OR UPDATE statements
// that can be used for database migration, backup/restore, and test data generation.
//
// Current Design Constraints:
// - Values are expected to be pre-formatted as SQL literals using spanvalue.LiteralFormatConfig
// - The formatter receives []string (Row) rather than raw *spanner.Row data
// - Format decision is made early in execute_sql.go, not at formatting time
//
// Future Improvements:
// - Consider passing raw *spanner.Row to formatters for late-binding format decisions
// - This would allow formatters to choose appropriate FormatConfig based on their needs
// - Would enable format-specific optimizations and better separation of concerns
//
// The implementation uses memefish's ast.Path for correct identifier handling.
package main

import (
	"fmt"
	"io"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// SQLFormatter handles SQL export formatting for different INSERT variants.
// It supports both single-row and multi-row INSERT statements based on batchSize.
// The formatter buffers rows when batchSize > 1 to generate multi-row INSERTs.
type SQLFormatter struct {
	out         io.Writer
	mode        enums.DisplayMode
	tablePath   *ast.Path // Parsed table name (may include schema)
	columnNames []string
	batchSize   int        // 0 or 1: single-row INSERTs, 2+: multi-row INSERTs
	rowBuffer   [][]string // Buffer for batching rows
}

// NewSQLFormatter creates a new SQL formatter for streaming output.
func NewSQLFormatter(out io.Writer, mode enums.DisplayMode, tableName string, batchSize int64) (*SQLFormatter, error) {
	if batchSize < 0 {
		return nil, fmt.Errorf("CLI_SQL_BATCH_SIZE cannot be negative: %d", batchSize)
	}

	// Spanner limit: 80,000 mutations per commit
	// Since each row is at least one mutation, limit batch size to be safe
	// Using 10,000 as a reasonable upper limit that's well below Spanner's limits
	// and prevents excessive memory usage
	const maxBatchSize = 10000
	if batchSize > maxBatchSize {
		return nil, fmt.Errorf("CLI_SQL_BATCH_SIZE %d exceeds maximum supported value of %d (limited for Spanner mutation constraints)", batchSize, maxBatchSize)
	}

	// Check if batchSize fits in an int on this platform (should always pass given maxBatchSize)
	const maxInt = int(^uint(0) >> 1)
	if batchSize > int64(maxInt) {
		return nil, fmt.Errorf("CLI_SQL_BATCH_SIZE %d exceeds maximum supported value on this platform", batchSize)
	}

	tablePath, err := parseSimpleTablePath(tableName)
	if err != nil {
		return nil, err
	}

	batchSizeInt := int(batchSize)
	return &SQLFormatter{
		out:       out,
		mode:      mode,
		tablePath: tablePath,
		batchSize: batchSizeInt,
		rowBuffer: make([][]string, 0, max(batchSizeInt, 1)),
	}, nil
}

// parseSimpleTablePath converts a simple table path string from CLI input to an ast.Path.
// This function handles user-friendly input where reserved words don't need quoting.
// Examples: "Users", "Order" (reserved word OK), "myschema.Users"
// The function does NOT parse SQL expressions - it simply splits on dots.
// Quoting for reserved words is handled automatically by ast.Ident.SQL() during output.
func parseSimpleTablePath(input string) (*ast.Path, error) {
	// Trim spaces and check for empty input
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("CLI_SQL_TABLE_NAME must be set for SQL export formats")
	}

	// For CLI_SQL_TABLE_NAME, we want simple dot-separated parsing
	// Users shouldn't need to worry about reserved words or quoting
	parts := strings.Split(input, ".")
	idents := make([]*ast.Ident, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("empty identifier in table path: %q", input)
		}
		// ast.Ident.SQL() will handle quoting if needed (e.g., for reserved words)
		idents = append(idents, &ast.Ident{Name: part})
	}

	return &ast.Path{Idents: idents}, nil
}

// WriteHeader sets up column names for the formatter.
func (f *SQLFormatter) WriteHeader(columnNames []string) error {
	// Validate that all columns have names (SQL export requires column names)
	for i, name := range columnNames {
		if name == "" {
			return fmt.Errorf("column %d has no name; SQL export requires all columns to have names (consider using aliases in your query)", i)
		}
	}
	f.columnNames = columnNames
	return nil
}

// WriteRow processes a single row and outputs SQL when batch is full.
func (f *SQLFormatter) WriteRow(values []string) error {
	f.rowBuffer = append(f.rowBuffer, values)

	// Check if we should flush the batch
	shouldFlush := false
	if f.batchSize <= 1 {
		// No batching: flush immediately
		shouldFlush = true
	} else if len(f.rowBuffer) >= f.batchSize {
		// Batch is full
		shouldFlush = true
	}

	if shouldFlush {
		return f.flushBatch()
	}

	return nil
}

// Finish flushes any remaining rows.
func (f *SQLFormatter) Finish() error {
	if len(f.rowBuffer) > 0 {
		return f.flushBatch()
	}
	return nil
}

// flushBatch writes the buffered rows as SQL statements.
func (f *SQLFormatter) flushBatch() error {
	if len(f.rowBuffer) == 0 {
		return nil
	}

	// Determine the INSERT clause based on mode
	var insertClause string
	switch f.mode {
	case enums.DisplayModeSQLInsert:
		insertClause = "INSERT"
	case enums.DisplayModeSQLInsertOrIgnore:
		insertClause = "INSERT OR IGNORE"
	case enums.DisplayModeSQLInsertOrUpdate:
		insertClause = "INSERT OR UPDATE"
	default:
		return fmt.Errorf("unsupported SQL mode: %v", f.mode)
	}

	// Build column list
	columnList := make([]string, len(f.columnNames))
	for i, col := range f.columnNames {
		columnList[i] = (&ast.Ident{Name: col}).SQL()
	}

	// Generate SQL statement(s)
	if f.batchSize <= 1 || len(f.rowBuffer) == 1 {
		// Single-row INSERT statements
		for _, row := range f.rowBuffer {
			// Values are already formatted as SQL literals
			_, err := fmt.Fprintf(f.out, "%s INTO %s (%s) VALUES (%s);\n",
				insertClause,
				f.tablePath.SQL(),
				strings.Join(columnList, ", "),
				strings.Join(row, ", "))
			if err != nil {
				return err
			}
		}
	} else {
		// Multi-row INSERT statement
		_, err := fmt.Fprintf(f.out, "%s INTO %s (%s) VALUES",
			insertClause,
			f.tablePath.SQL(),
			strings.Join(columnList, ", "))
		if err != nil {
			return err
		}

		for i, row := range f.rowBuffer {
			// Values are already formatted as SQL literals
			if i == 0 {
				_, err = fmt.Fprintf(f.out, "\n  (%s)", strings.Join(row, ", "))
			} else {
				_, err = fmt.Fprintf(f.out, ",\n  (%s)", strings.Join(row, ", "))
			}
			if err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(f.out, ";")
		if err != nil {
			return err
		}
	}

	// Clear the buffer
	f.rowBuffer = f.rowBuffer[:0]
	return nil
}

// SQLStreamingFormatter implements StreamingFormatter for SQL export.
// Note: While this supports streaming, partitioned queries currently buffer all results
// before formatting, so streaming benefits are not realized for partitioned queries.
type SQLStreamingFormatter struct {
	formatter   *SQLFormatter
	initialized bool
}

// NewSQLStreamingFormatter creates a new streaming SQL formatter.
func NewSQLStreamingFormatter(out io.Writer, sysVars *systemVariables, mode enums.DisplayMode) (*SQLStreamingFormatter, error) {
	if sysVars.SQLTableName == "" {
		return nil, fmt.Errorf("CLI_SQL_TABLE_NAME must be set for SQL export formats")
	}

	formatter, err := NewSQLFormatter(out, mode, sysVars.SQLTableName, sysVars.SQLBatchSize)
	if err != nil {
		return nil, err
	}

	return &SQLStreamingFormatter{
		formatter:   formatter,
		initialized: false,
	}, nil
}

// InitFormat initializes the formatter with column information.
func (s *SQLStreamingFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	s.initialized = true
	// WriteHeader will validate column names
	return s.formatter.WriteHeader(columns)
}

// WriteRow outputs a single row.
func (s *SQLStreamingFormatter) WriteRow(row Row) error {
	if !s.initialized {
		return fmt.Errorf("header not processed before row")
	}
	return s.formatter.WriteRow(row)
}

// FinishFormat completes the SQL export.
func (s *SQLStreamingFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	return s.formatter.Finish()
}

// formatSQL is the non-streaming formatter for SQL export.
// PRECONDITION: result.TableHeader must contain valid column information.
// The TableHeader is essential for generating the column names in INSERT statements
// (e.g., INSERT INTO table(col1, col2, ...) VALUES ...).
// Without valid column headers, SQL export cannot generate syntactically correct INSERT statements.
func formatSQL(mode enums.DisplayMode) FormatFunc {
	return func(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error {
		if sysVars.SQLTableName == "" {
			return fmt.Errorf("CLI_SQL_TABLE_NAME must be set for SQL export formats")
		}

		formatter, err := NewSQLFormatter(out, mode, sysVars.SQLTableName, sysVars.SQLBatchSize)
		if err != nil {
			return err
		}

		// Write header (will validate column names)
		if err := formatter.WriteHeader(columnNames); err != nil {
			return err
		}

		// Write all rows
		for _, row := range result.Rows {
			if err := formatter.WriteRow(row); err != nil {
				return err
			}
		}

		// Finish and flush
		return formatter.Finish()
	}
}
