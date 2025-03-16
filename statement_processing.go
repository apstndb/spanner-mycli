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
	"log"
	"regexp"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/olekukonko/tablewriter"
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

type Result struct {
	ColumnNames      []string
	ColumnAlign      []int // optional
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

	// ColumnTypes will be printed in `--verbose` mode if it is not empty
	ColumnTypes []*sppb.StructType_Field
	ForceWrap   bool
	LintResults []string
	PreInput    string

	BatchInfo      *BatchInfo
	PartitionCount int
}

type Row struct {
	Columns []string
}

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
	explainColumnNames = []string{"ID", "Query_Execution_Plan"}
	explainColumnAlign = []int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT}

	explainAnalyzeColumnNames = []string{"ID", "Query_Execution_Plan", "Rows_Returned", "Executions", "Total_Latency"}
	explainAnalyzeColumnAlign = []int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT}

	describeColumnNames = []string{"Column_Name", "Column_Type"}

	// DDL needing special treatment
	createDatabaseRe = regexp.MustCompile(`(?is)^CREATE\s+DATABASE\s.+$`)
)

var errStatementNotMatched = errors.New("statement not matched")

func BuildStatement(input string) (Statement, error) {
	return BuildStatementWithComments(input, input)
}

func BuildCLIStatement(trimmed string) (Statement, error) {
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

func BuildStatementWithCommentsWithMode(stripped, raw string, mode parseMode) (Statement, error) {
	trimmed := strings.TrimSpace(stripped)
	if trimmed == "" {
		return nil, errors.New("empty statement")
	}

	switch stmt, err := BuildCLIStatement(trimmed); {
	case err != nil && !errors.Is(err, errStatementNotMatched):
		return nil, err
	case stmt != nil:
		return stmt, nil
	default:
		// no action
	}

	if mode != parseModeNoMemefish && mode != parseModeUnspecified {
		switch stmt, err := BuildNativeStatementMemefish(raw); {
		case mode == parseMemefishOnly && err != nil:
			return nil, fmt.Errorf("invalid statement: %w", err)
		case errors.Is(err, errStatementNotMatched):
			log.Println(fmt.Errorf("ignore unknown statement, err: %w", err))
		case err != nil:
			log.Println(fmt.Errorf("ignore memefish parse error, err: %w", err))
		default:
			return stmt, nil
		}
	}

	return BuildNativeStatementFallback(trimmed, raw)
}

func BuildNativeStatementMemefish(raw string) (Statement, error) {
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
		// Currently, UpdateDdl doesn't permit comments, so we need to unparse DDLs.

		// Only CREATE DATABASE needs special treatment in DDL.
		if instanceOf[*ast.CreateDatabase](stmt) {
			return &CreateDatabaseStatement{CreateStatement: stmt.SQL()}, nil
		}
		return &DdlStatement{Ddl: stmt.SQL()}, nil
	default:
		return nil, fmt.Errorf("unknown memefish statement, stmt %T, err: %w", stmt, errStatementNotMatched)
	}
}

func BuildNativeStatementFallback(trimmed string, raw string) (Statement, error) {
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
		// Currently, UpdateDdl doesn't permit comments, so we need to use trimmed SQL tex.

		// Only CREATE DATABASE needs special treatment in DDL.
		if createDatabaseRe.MatchString(trimmed) {
			return &CreateDatabaseStatement{CreateStatement: trimmed}, nil
		}

		return &DdlStatement{Ddl: trimmed}, nil
	default:
		return nil, errors.New("invalid statement")
	}
}

func unquoteIdentifier(input string) string {
	return strings.Trim(strings.TrimSpace(input), "`")
}

func logParseStatement(stmt string) {
	if !logMemefish {
		return
	}
	n, err := memefish.ParseStatement("", stmt)
	if err != nil {
		log.Printf("SQL can't parsed as a statement, err: %v", err)
	} else {
		log.Printf("parsed: %v", n.SQL())
	}
}

func logParseStatements(stmts []string) {
	for _, stmt := range stmts {
		logParseStatement(stmt)
	}
}
