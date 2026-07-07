//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package mycli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/apstndb/spanstats"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/olekukonko/tablewriter/tw"
)

type Statement interface {
	Execute(ctx context.Context, session *Session) (*Result, error)
}

// MutationStatement is a marker interface for mutation statements.
// Mutation statements are not permitted in a read-only transaction. It determines pending transactions.
type MutationStatement interface {
	isMutationStatement()
}

// ConditionallyMutatingStatement is a marker interface for statements whose
// mutation-ness cannot be decided statically from their Go type but only from
// their content (e.g. a CQL statement text). The READONLY guard in
// Session.ExecuteStatement consults isConditionallyMutating at the same single
// call site as the static MutationStatement marker.
//
// Unlike MutationStatement, implementing this interface does NOT determine a
// pending Spanner transaction: it only participates in the READONLY guard,
// because such statements (CQL, BigQuery) do not use the Spanner transaction
// machinery.
type ConditionallyMutatingStatement interface {
	isConditionallyMutating() bool
}

// Compile-time assertions for every MutationStatement implementation.
// The marker method is unexported, so a typo (e.g. an exported
// IsMutationStatement) silently drops the type out of the interface and
// bypasses the READONLY guard in Session.ExecuteStatement (issue #695).
// Keep this list in sync when adding a mutation statement.
var (
	_ MutationStatement = (*MutateStatement)(nil)
	_ MutationStatement = (*ExplainAnalyzeDmlStatement)(nil)
	_ MutationStatement = (*BeginRwStatement)(nil)
	_ MutationStatement = (*DmlStatement)(nil)
	_ MutationStatement = (*DdlStatement)(nil)
	_ MutationStatement = (*CreateDatabaseStatement)(nil)
	_ MutationStatement = (*DropDatabaseStatement)(nil)
	_ MutationStatement = (*TruncateTableStatement)(nil)
	_ MutationStatement = (*PartitionedDmlStatement)(nil)
	_ MutationStatement = (*BulkDdlStatement)(nil)
	_ MutationStatement = (*BatchDMLStatement)(nil)
	_ MutationStatement = (*SyncProtoStatement)(nil)
	_ MutationStatement = (*AddSplitPointsStatement)(nil)
)

// No core statement implements ConditionallyMutatingStatement any more: both
// implementations were extracted into feature packages, where each carries its
// own compile-time assertion. BigQueryStatement moved to
// internal/mycli/feature/bigquery and CQLStatement to internal/mycli/feature/cql
// (#778); both embed mycli.MutationClassifier there. The unexported marker
// method means a feature that drops the embed silently bypasses the READONLY
// guard, so the assertion travels with the type.

// DetachedCompatible is a marker interface for statements that can run in Detached session mode (admin operation only mode).
// Statements implementing this interface can execute when session.IsDetached() is true.
type DetachedCompatible interface {
	isDetachedCompatible()
}

// rowCountType is type of modified rows count by DML.
type rowCountType int

const (
	// rowCountTypeExact is exact count type for DML result.
	rowCountTypeExact rowCountType = iota
	// rowCountTypeLowerBound is lower bound type for Partitioned DML result.
	rowCountTypeLowerBound
	// rowCountTypeLowerBound is upper bound type for batch DML result.
	rowCountTypeUpperBound
)

type BatchInfo struct {
	Mode batchMode
	Size int
}

type TableHeader interface {
	// Render returns the header strings. When verbose is true, type information is included.
	// Use renderTableHeader() as a nil-safe wrapper.
	Render(verbose bool) []string
	// structFields returns the complete field information with types if available.
	// Returns (fields, true) if type information is available.
	// Returns (nil, false) if only column names are available (e.g., simpleTableHeader).
	structFields() ([]*sppb.StructType_Field, bool)
}

type simpleTableHeader []string

func (th simpleTableHeader) Render(verbose bool) []string {
	return th
}

func (th simpleTableHeader) structFields() ([]*sppb.StructType_Field, bool) {
	// simpleTableHeader only contains column names, no type information
	return nil, false
}

// toTableHeader convert slice or variable arguments to TableHeader.
// nil or empty slice will return untyped nil.
//
// Note: In practice, this should never receive empty input from Spanner query results
// because Spanner requires at least one column in SELECT queries. Empty input might
// occur only in client-side statements or error conditions.
func toTableHeader[T interface {
	string | []string | *sppb.StructType_Field | []*sppb.StructType_Field
}](ss ...T) TableHeader {
	if len(ss) == 0 {
		return nil
	}

	switch any(ss[0]).(type) {
	case *sppb.StructType_Field:
		var result typesTableHeader
		for _, s := range ss {
			result = append(result, any(s).(*sppb.StructType_Field))
		}

		return result
	case string:
		var result simpleTableHeader
		for _, s := range ss {
			result = append(result, any(s).(string))
		}

		return result
	case []*sppb.StructType_Field:
		var result typesTableHeader
		for _, s := range ss {
			result = append(result, any(s).([]*sppb.StructType_Field)...)
		}

		if len(result) == 0 {
			return nil
		}

		return result
	case []string:
		var result simpleTableHeader
		for _, s := range ss {
			result = append(result, any(s).([]string)...)
		}

		if len(result) == 0 {
			return nil
		}

		return result
	default:
		// This should be unreachable due to type constraints, but log instead of panic
		slog.Warn("toTableHeader received unexpected type", "type", fmt.Sprintf("%T", ss))
		return nil
	}
}

type typesTableHeader []*sppb.StructType_Field

func (th typesTableHeader) Render(verbose bool) []string {
	var result []string
	for _, f := range th {
		if verbose {
			result = append(result, formatTypedHeaderColumn(f))
		} else {
			result = append(result, f.Name)
		}
	}
	return result
}

func (th typesTableHeader) structFields() ([]*sppb.StructType_Field, bool) {
	// typesTableHeader contains complete field information with types
	return []*sppb.StructType_Field(th), true
}

// QueryIndexAdvice holds an index recommendation from the Spanner query advisor.
type QueryIndexAdvice struct {
	DDL               []string
	ImprovementFactor float64
}

// TypedRows preserves the raw result-set (metadata + typed rows) for a buffered
// Result so export formats re-render from values, not display text. It is the
// (c) "typed buffered" body payload described in issue #738: at most one of
// Result.Rows, Result.Typed, Result.RenderedOutput is set, and Result.Streamed
// means the body was already emitted during execution.
type TypedRows struct {
	// Metadata is the authoritative column names + types for the result set.
	Metadata *sppb.ResultSetMetadata
	// Rows are the raw decoded rows; they remain valid after RowIterator.Stop
	// because each row owns its decoded values.
	Rows []*spanner.Row
	// SQLExportAllowed preserves today's rule that only real query results are
	// exported as INSERTs; other typed producers fall back to TABLE under
	// SQL_INSERT* (successor of the buffered-query use of SQLExportAllowed).
	SQLExportAllowed bool
}

type Result struct {
	ColumnAlign []tw.Align // optional
	Rows        []Row

	// Typed holds the raw typed rows for a buffered query result; rendering
	// derives display cells or replays raw rows at display time (printTableData).
	// Mutually exclusive with Rows/RenderedOutput/Streamed (see TypedRows).
	Typed *TypedRows

	// RenderedOutput holds pre-rendered text (kind (d)) that should be written
	// before appendices and result lines. Used when output must stay atomic until
	// after execution side effects such as implicit DML commit have succeeded.
	// A non-nil RenderedOutput is the signal to write it instead of formatting a
	// body payload (printResult); the two producers (DUMP buffered fallback, DML
	// THEN RETURN) always set it together with their content.
	RenderedOutput   []byte
	Predicates       []string
	Appendices       []ResultAppendix
	AffectedRows     int
	AffectedRowsType rowCountType
	Stats            QueryStats

	// IsExecutedDML indicates this is an executed DML statement (INSERT/UPDATE/DELETE) that can report affected rows
	IsExecutedDML bool

	ReadTimestamp   time.Time // For SELECT/read-only transactions
	CommitTimestamp time.Time // For COMMIT/DML operations
	ForceVerbose    bool
	CommitStats     *sppb.CommitResponse_CommitStats
	KeepVariables   bool

	TableHeader TableHeader

	ForceWrap   bool
	LintResults []string
	IndexAdvice []QueryIndexAdvice // Index recommendations from query advisor
	PreInput    string

	// SQLExportAllowed indicates that the display-text row values in Rows have
	// been formatted as SQL literals using spanvalue.LiteralFormatConfig instead
	// of regular display formatting, so they may be replayed into INSERT
	// statements. This flag governs the display-text (Rows) replay path; the
	// typed (Typed) path carries its own TypedRows.SQLExportAllowed.
	// This flag is set to true only when:
	// - executeSQL is called with SQL export format (SQL_INSERT, SQL_INSERT_OR_IGNORE, SQL_INSERT_OR_UPDATE)
	// - The values in Rows are valid SQL literals that can be used in INSERT statements
	// When false, SQL export formats will fall back to table format to prevent invalid SQL generation.
	// Examples of statements that have this as false:
	// - SHOW CREATE TABLE, SHOW TABLES (metadata queries)
	// - EXPLAIN, EXPLAIN ANALYZE (query plan information)
	// - DML with THEN RETURN (uses regular formatting)
	SQLExportAllowed bool

	// SQLTableNameForExport stores the auto-detected or explicitly set table name for SQL export.
	// This field is populated during query execution when SQL export formats are used.
	// It ensures the table name is available during the formatting phase, even in buffered mode.
	SQLTableNameForExport string

	BatchInfo      *BatchInfo
	PartitionCount int
	Streamed       bool                      // Indicates rows were streamed and not buffered
	Metrics        *metrics.ExecutionMetrics // Performance metrics for query execution
}

// Row is a type alias for format.Row (= []Cell).
type Row = format.Row

func toRow(vs ...string) Row {
	return format.StringsToRow(vs...)
}

func sliceOf[V any](vs ...V) []V {
	return vs
}

// QueryStats contains query statistics.
// Some fields may not have a valid value depending on the environment.
// For example, only ElapsedTime and RowsReturned has valid value for Cloud Spanner Emulator.
type QueryStats = spanstats.QueryStats

var (
	operatorColumnName       = "Operator <execution_method> (metadata, ...)"
	operatorColumnNameLength = int64(len(operatorColumnName))

	// default EXPLAIN columns
	explainColumnNames = []string{"ID", operatorColumnName}

	// EXPLAIN columns for limited width
	explainColumnNamesShort = []string{"ID", "Operator"}

	explainColumnAlign = []tw.Align{tw.AlignRight, tw.AlignLeft}

	describeColumnNames = []string{"Column_Name", "Column_Type"}

	// DDL needing special treatment
	createDatabaseRe = regexp.MustCompile(`(?is)^CREATE\s+DATABASE\s.+$`)
)

var errStatementNotMatched = errors.New("statement not matched")

type statementParseFunc func(stripped, raw string) (Statement, error)

func BuildStatement(input string) (Statement, error) {
	return BuildStatementWithComments(input, input)
}

func BuildCLIStatement(stripped, raw string) (Statement, error) {
	// activeStatementDefs is the merged (core + feature) table set by Main;
	// it defaults to the core table so pre-Main paths and tests still dispatch.
	return BuildStatementWithDefs(activeStatementDefs, stripped)
}

func BuildStatementWithComments(stripped, raw string) (Statement, error) {
	return BuildStatementWithCommentsWithMode(stripped, raw, enums.ParseModeFallback)
}

func composeStatementParseFunc(funcs ...statementParseFunc) statementParseFunc {
	return func(stripped, raw string) (Statement, error) {
		for _, f := range funcs {
			stmt, err := f(stripped, raw)
			switch {
			case errors.Is(err, errStatementNotMatched):
				slog.Debug("fallback to next parser", "err", err)
				continue
			case err != nil:
				return nil, err
			default:
				return stmt, nil
			}
		}
		return nil, errStatementNotMatched
	}
}

func ignoreParseError(f statementParseFunc) statementParseFunc {
	return func(stripped, raw string) (Statement, error) {
		s, err := f(stripped, raw)
		switch {
		case errors.Is(err, errStatementNotMatched):
			return nil, err
		case err != nil:
			slog.Warn("error ignored", "err", err)
			return nil, fmt.Errorf("error ignored: %w", errors.Join(err, errStatementNotMatched))
		default:
			return s, nil
		}
	}
}

// getParserForMode returns the appropriate StatementParser for the given mode
func getParserForMode(mode enums.ParseMode) (statementParseFunc, error) {
	switch mode {
	case enums.ParseModeNoMemefish:
		return composeStatementParseFunc(
			BuildCLIStatement,
			BuildNativeStatementLexical,
		), nil
	case enums.ParseModeMemefishOnly:
		return composeStatementParseFunc(
			BuildCLIStatement,
			BuildNativeStatementMemefish,
		), nil
	case enums.ParseModeFallback, enums.ParseModeUnspecified:
		return composeStatementParseFunc(
			BuildCLIStatement,
			ignoreParseError(BuildNativeStatementMemefish),
			BuildNativeStatementLexical,
		), nil
	default:
		return nil, fmt.Errorf("invalid parseMode: %q", mode)
	}
}

func BuildStatementWithCommentsWithMode(stripped, raw string, mode enums.ParseMode) (Statement, error) {
	parser, err := getParserForMode(mode)
	if err != nil {
		return nil, err
	}
	return parser(stripped, raw)
}

func BuildNativeStatementMemefish(stripped, raw string) (Statement, error) {
	stmt, err := memefish.ParseStatement("", raw)
	if err != nil {
		return nil, err
	}

	kind := stmtkind.DetectSemantic(stmt)
	switch {
	// DML statements are compatible with ExecuteSQL, but they should be executed with DmlStatement, not SelectStatement.
	case kind.IsDML():
		return &DmlStatement{Dml: raw}, nil
	// All ExecuteSQL compatible statements can be executed with SelectStatement.
	case kind.IsExecuteSQLCompatible():
		return &SelectStatement{Query: raw}, nil
	case kind.IsDDL():
		// Only CREATE DATABASE needs special treatment in DDL.
		if _, ok := stmt.(*ast.CreateDatabase); ok {
			return &CreateDatabaseStatement{CreateStatement: raw}, nil
		}

		return &DdlStatement{Ddl: raw}, nil
	default:
		return nil, fmt.Errorf("unknown memefish statement, stmt %T, err: %w", stmt, errStatementNotMatched)
	}
}

func BuildNativeStatementLexical(stripped string, raw string) (Statement, error) {
	kind, err := stmtkind.DetectLexical(raw)
	if err != nil {
		return nil, err
	}

	switch {
	// DML statements are compatible with ExecuteSQL, but they should be executed with DmlStatement, not SelectStatement.
	case kind.IsDML():
		return &DmlStatement{Dml: raw}, nil
	// All ExecuteSQL compatible statements can be executed with SelectStatement.
	case kind.IsExecuteSQLCompatible():
		return &SelectStatement{Query: raw}, nil
	case kind.IsDDL():
		// Only CREATE DATABASE needs special treatment in DDL.
		if createDatabaseRe.MatchString(stripped) {
			return &CreateDatabaseStatement{CreateStatement: raw}, nil
		}

		return &DdlStatement{Ddl: raw}, nil
	default:
		return nil, errors.New("invalid statement")
	}
}

// unquoteIdentifier strips surrounding backquotes from an argument.
// It is intentionally NOT GoogleSQL identifier parsing: it is kept for
// arguments that are not GoogleSQL identifiers (database IDs may contain "-",
// role names, ...). Statement arguments that are real GoogleSQL
// identifiers/FQNs must use parseIdentifierArg / parseSchemaAndName /
// parseTableNameList instead, which validate the input with the memefish
// lexer.
func unquoteIdentifier(input string) string {
	return strings.Trim(strings.TrimSpace(input), "`")
}

// parseFQNParts parses a dot-separated path of (possibly backquoted)
// identifiers at the current parser position and returns its components.
// Unlike parseFQN, it consumes the trailing identifier token, leaving the
// parser on the token that follows the path.
// Note: following GoogleSQL quoted identifier semantics, a backquoted
// identifier containing dots (e.g. `a.b`) is a single component.
func parseFQNParts(p *memefish.Parser) ([]string, error) {
	var idents []string
	for {
		if p.Token.Kind == token.TokenEOF {
			return nil, errors.New("expected identifier, but got end of input")
		}
		s, dot, err := parseIdentLikeWithOptDot(p)
		if err != nil {
			return nil, err
		}
		idents = append(idents, s)
		if !dot {
			break
		}
	}

	// parseIdentLikeWithOptDot leaves the last identifier token unconsumed.
	if err := p.NextToken(); err != nil {
		return nil, err
	}
	return idents, nil
}

// parseIdentifierPath parses the whole input as a dot-separated path of
// (possibly backquoted) identifiers and returns its components. It fails on
// empty input, non-identifier tokens, and trailing input after the path.
func parseIdentifierPath(input string) ([]string, error) {
	p := newParser("", input)
	if err := p.NextToken(); err != nil {
		return nil, err
	}

	idents, err := parseFQNParts(p)
	if err != nil {
		return nil, err
	}

	if p.Token.Kind != token.TokenEOF {
		return nil, fmt.Errorf("unexpected input %q after identifier", p.Token.Raw)
	}
	return idents, nil
}

// parseIdentifierArg parses the whole input as exactly one (possibly
// backquoted) identifier.
func parseIdentifierArg(input string) (string, error) {
	idents, err := parseIdentifierPath(input)
	if err != nil {
		return "", err
	}
	if len(idents) != 1 {
		return "", fmt.Errorf("expected a single identifier, but %q has %d components", input, len(idents))
	}
	return idents[0], nil
}

// parseSchemaAndName parses the whole input as [<schema>.]<name> and returns
// the schema (empty when unqualified) and the object name.
func parseSchemaAndName(input string) (schema, name string, err error) {
	idents, err := parseIdentifierPath(input)
	if err != nil {
		return "", "", err
	}
	switch len(idents) {
	case 1:
		return "", idents[0], nil
	case 2:
		return idents[0], idents[1], nil
	default:
		return "", "", fmt.Errorf("expected [<schema>.]<name>, but %q has %d components", input, len(idents))
	}
}

// parseTableNameList parses the whole input as a comma-separated list of
// possibly-qualified table names. Empty elements (leading, trailing, or
// doubled commas) are errors so that a typo can't silently change the
// statement's meaning (e.g. DUMP TABLES with an empty list would previously
// fall back to dumping the whole database).
func parseTableNameList(input string) ([]string, error) {
	p := newParser("", input)
	if err := p.NextToken(); err != nil {
		return nil, err
	}

	var tables []string
	for {
		idents, err := parseFQNParts(p)
		if err != nil {
			return nil, fmt.Errorf("expected table name: %w", err)
		}
		tables = append(tables, strings.Join(idents, "."))

		switch p.Token.Kind {
		case token.TokenEOF:
			return tables, nil
		case ",":
			if err := p.NextToken(); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("expected ',' or end of input after table name, but got %q", p.Token.Raw)
		}
	}
}

// buildCommands parses the input and builds a list of commands for batch execution.
// It can compose BulkDdlStatement from consecutive DDL statements.
func buildCommands(input string, mode enums.ParseMode) ([]Statement, error) {
	var cmds []Statement
	var pendingDdls []string

	// Check if input starts with a meta command.
	// This check must be performed before separateInput() because meta-commands
	// (starting with '\') are not valid GoogleSQL lexical structure and would
	// cause parsing errors.
	if IsMetaCommand(strings.TrimSpace(input)) {
		return nil, errors.New("meta commands are not supported in batch mode")
	}

	stmts, err := separateInput(input)
	if err != nil {
		return nil, err
	}
	for _, separated := range stmts {
		// Ignore the last empty statement
		if separated.delim == delimiterUndefined && separated.statementWithoutComments == "" {
			continue
		}

		// Check each statement after splitting as a safety net
		if IsMetaCommand(strings.TrimSpace(separated.statement)) {
			return nil, errors.New("meta commands are not supported in batch mode")
		}

		stmt, err := BuildStatementWithCommentsWithMode(strings.TrimSpace(separated.statementWithoutComments), separated.statement, mode)
		if err != nil {
			return nil, fmt.Errorf("failed with statement, error: %w, statement: %q, without comments: %q", err, separated.statement, separated.statementWithoutComments)
		}
		if ddl, ok := stmt.(*DdlStatement); ok {
			pendingDdls = append(pendingDdls, ddl.Ddl)
			continue
		}

		// Flush pending DDLs
		if len(pendingDdls) > 0 {
			cmds = append(cmds, &BulkDdlStatement{pendingDdls})
			pendingDdls = nil
		}

		cmds = append(cmds, stmt)
	}

	// Flush pending DDLs
	if len(pendingDdls) > 0 {
		cmds = append(cmds, &BulkDdlStatement{pendingDdls})
	}

	return cmds, nil
}
