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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/olekukonko/tablewriter/tw"
)

// Partitioned DML tends to take long time to be finished.
// See: https://github.com/cloudspannerecosystem/spanner-cli/issues/102
const pdmlTimeout = time.Hour * 24

type Statement interface {
	Execute(ctx context.Context, session *Session) (*Result, error)
}

// MutationStatement is a marker interface for mutation statements.
// Mutation statements are not permitted in a read-only transaction. It determines pending transactions.
type MutationStatement interface {
	isMutationStatement()
}

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
	// internalRender shouldn't be called directly. Use renderTableHeader().
	internalRender(verbose bool) []string
}

type simpleTableHeader []string

func (th simpleTableHeader) internalRender(verbose bool) []string {
	return th
}

// toTableHeader convert slice or variable arguments to TableHeader.
// nil or empty slice will return untyped nil.
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

func (th typesTableHeader) internalRender(verbose bool) []string {
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

type Result struct {
	ColumnAlign      []tw.Align // optional
	Rows             []Row
	Predicates       []string
	AffectedRows     int
	AffectedRowsType rowCountType
	Stats            QueryStats

	// Used for switch output("rows in set" / "rows affected")
	IsMutation bool

	Timestamp     time.Time
	ForceVerbose  bool
	CommitStats   *sppb.CommitResponse_CommitStats
	KeepVariables bool

	TableHeader TableHeader

	ForceWrap   bool
	LintResults []string
	PreInput    string

	BatchInfo      *BatchInfo
	PartitionCount int
}

type Row []string

// QueryStats contains query statistics.
// Some fields may not have a valid value depending on the environment.
// For example, only ElapsedTime and RowsReturned has valid value for Cloud Spanner Emulator.
type QueryStats struct {
	ElapsedTime                string `json:"elapsed_time"`
	CPUTime                    string `json:"cpu_time"`
	RowsReturned               string `json:"rows_returned"`
	RowsScanned                string `json:"rows_scanned"`
	DeletedRowsScanned         string `json:"deleted_rows_scanned"`
	OptimizerVersion           string `json:"optimizer_version"`
	OptimizerStatisticsPackage string `json:"optimizer_statistics_package"`
	RemoteServerCalls          string `json:"remote_server_calls"`
	MemoryPeakUsageBytes       string `json:"memory_peak_usage_bytes"`
	TotalMemoryPeakUsageByte   string `json:"total_memory_peak_usage_byte"`
	QueryText                  string `json:"query_text"`
	BytesReturned              string `json:"bytes_returned"`
	RuntimeCreationTime        string `json:"runtime_creation_time"`
	StatisticsLoadTime         string `json:"statistics_load_time"`
	MemoryUsagePercentage      string `json:"memory_usage_percentage"`
	FilesystemDelaySeconds     string `json:"filesystem_delay_seconds"`
	LockingDelay               string `json:"locking_delay"`
	QueryPlanCreationTime      string `json:"query_plan_creation_time"`
	ServerQueueDelay           string `json:"server_queue_delay"`
	DataBytesRead              string `json:"data_bytes_read"`
	IsGraphQuery               string `json:"is_graph_query"`
	RuntimeCached              string `json:"runtime_cached"`
	QueryPlanCached            string `json:"query_plan_cached"`

	Unknown jsontext.Value `json:",unknown" pp:"-"`
}

type parseMode string

const (
	parseModeUnspecified parseMode = ""
	parseModeFallback    parseMode = "FALLBACK"
	parseModeNoMemefish  parseMode = "NO_MEMEFISH"
	parseMemefishOnly    parseMode = "MEMEFISH_ONLY"
)

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
	trimmed := strings.TrimSpace(stripped)
	if trimmed == "" {
		return nil, errors.New("empty statement")
	}

	for _, cs := range clientSideStatementDefs {
		if cs.Pattern.MatchString(trimmed) {
			matches := cs.Pattern.FindStringSubmatch(trimmed)
			stmt, err := cs.HandleSubmatch(matches)
			if err != nil {
				return nil, err
			}
			return stmt, nil
		}
	}

	return nil, errStatementNotMatched
}

func BuildStatementWithComments(stripped, raw string) (Statement, error) {
	return BuildStatementWithCommentsWithMode(stripped, raw, parseModeFallback)
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
func getParserForMode(mode parseMode) (statementParseFunc, error) {
	switch mode {
	case parseModeNoMemefish:
		return composeStatementParseFunc(
			BuildCLIStatement,
			BuildNativeStatementLexical,
		), nil
	case parseMemefishOnly:
		return composeStatementParseFunc(
			BuildCLIStatement,
			BuildNativeStatementMemefish,
		), nil
	case parseModeFallback, parseModeUnspecified:
		return composeStatementParseFunc(
			BuildCLIStatement,
			ignoreParseError(BuildNativeStatementMemefish),
			BuildNativeStatementLexical,
		), nil
	default:
		return nil, fmt.Errorf("invalid parseMode: %q", mode)
	}
}

func BuildStatementWithCommentsWithMode(stripped, raw string, mode parseMode) (Statement, error) {
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
		if instanceOf[*ast.CreateDatabase](stmt) {
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

func unquoteIdentifier(input string) string {
	return strings.Trim(strings.TrimSpace(input), "`")
}

// buildCommands parses the input and builds a list of commands for batch execution.
// It can compose BulkDdlStatement from consecutive DDL statements.
func buildCommands(input string, mode parseMode) ([]Statement, error) {
	var cmds []Statement
	var pendingDdls []string

	stmts, err := separateInput(input)
	if err != nil {
		return nil, err
	}
	for _, separated := range stmts {
		// Ignore the last empty statement
		if separated.delim == delimiterUndefined && separated.statementWithoutComments == "" {
			continue
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
