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
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"log"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/ngicks/go-iterator-helper/hiter"

	"github.com/k0kubun/pp/v3"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/mattn/go-runewidth"
	"github.com/olekukonko/tablewriter"
	"google.golang.org/protobuf/encoding/protojson"

	"cloud.google.com/go/spanner"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/apstndb/lox"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	scxiter "spheric.cloud/xiter"
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

	BatchInfo *BatchInfo
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

var (
	// DDL needing special treatment
	createDatabaseRe = regexp.MustCompile(`(?is)^CREATE\s+DATABASE\s.+$`)
	dropDatabaseRe   = regexp.MustCompile(`(?is)^DROP\s+DATABASE\s+(.+)$`)

	// Start spanner-mycli statements

	// Partitioned DML
	// In fact, INSERT is not supported in a Partitioned DML, but accept it for showing better error message.
	// https://cloud.google.com/spanner/docs/dml-partitioned#features_that_arent_supported
	pdmlRe = regexp.MustCompile(`(?is)^PARTITIONED\s+(.*)$`)

	// Truncate is a syntax sugar of PDML.
	truncateTableRe = regexp.MustCompile(`(?is)^TRUNCATE\s+TABLE\s+(.+)$`)

	// Transaction
	beginRe    = regexp.MustCompile(`(?is)^BEGIN(?:\s+TRANSACTION)?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`)
	beginRwRe  = regexp.MustCompile(`(?is)^BEGIN\s+RW(?:\s+TRANSACTION)?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`)
	beginRoRe  = regexp.MustCompile(`(?is)^BEGIN\s+RO(?:\s+TRANSACTION)?(?:\s+([^\s]+))?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`)
	commitRe   = regexp.MustCompile(`(?is)^COMMIT(?:\s+TRANSACTION)?$`)
	rollbackRe = regexp.MustCompile(`(?is)^(?:ROLLBACK|CLOSE)(?:\s+TRANSACTION)?$`)

	// Other
	syncProtoBundleRe     = regexp.MustCompile(`(?is)^SYNC\s+PROTO\s+BUNDLE(?:\s+(.*))?$`)
	exitRe                = regexp.MustCompile(`(?is)^EXIT$`)
	useRe                 = regexp.MustCompile(`(?is)^USE\s+([^\s]+)(?:\s+ROLE\s+(.+))?$`)
	showLocalProtoRe      = regexp.MustCompile(`(?is)^SHOW\s+LOCAL\s+PROTO$`)
	showRemoteProtoRe     = regexp.MustCompile(`(?is)^SHOW\s+REMOTE\s+PROTO$`)
	showDatabasesRe       = regexp.MustCompile(`(?is)^SHOW\s+DATABASES$`)
	showCreateTableRe     = regexp.MustCompile(`(?is)^SHOW\s+CREATE\s+TABLE\s+(.+)$`)
	showTablesRe          = regexp.MustCompile(`(?is)^SHOW\s+TABLES(?:\s+(.+))?$`)
	showColumnsRe         = regexp.MustCompile(`(?is)^(?:SHOW\s+COLUMNS\s+FROM)\s+(.+)$`)
	showIndexRe           = regexp.MustCompile(`(?is)^SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(.+)$`)
	explainRe             = regexp.MustCompile(`(?is)^EXPLAIN\s+(ANALYZE\s+)?(.+)$`)
	describeRe            = regexp.MustCompile(`(?is)^DESCRIBE\s+(.+)$`)
	showVariableRe        = regexp.MustCompile(`(?is)^SHOW\s+VARIABLE\s+(.+)$`)
	setTransactionRe      = regexp.MustCompile(`(?is)^SET\s+TRANSACTION\s+(.*)$`)
	setParamTypeRe        = regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*([^=]*)$`)
	setParamRe            = regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*=\s*(.*)$`)
	setRe                 = regexp.MustCompile(`(?is)^SET\s+([^\s=]+)\s*=\s*(\S.*)$`)
	setAddRe              = regexp.MustCompile(`(?is)^SET\s+([^\s+=]+)\s*\+=\s*(\S.*)$`)
	showParamsRe          = regexp.MustCompile(`(?is)^SHOW\s+PARAMS$`)
	showVariablesRe       = regexp.MustCompile(`(?is)^SHOW\s+VARIABLES$`)
	partitionRe           = regexp.MustCompile(`(?is)^PARTITION\s(\S.*)$`)
	runPartitionedQueryRe = regexp.MustCompile(`(?is)^RUN\s+PARTITIONED\s+QUERY\s(\S.*)$`)
	runPartitionRe        = regexp.MustCompile(`(?is)^RUN\s+PARTITION\s+('[^']*'|"[^"]*")$`)
	tryPartitionedQueryRe = regexp.MustCompile(`(?is)^TRY\s+PARTITIONED\s+QUERY\s(\S.*)$`)
	mutateRe              = regexp.MustCompile(`(?is)^MUTATE\s+(\S+)\s+(INSERT|UPDATE|INSERT_OR_UPDATE|REPLACE|DELETE)\s+(.+)$`)
	showQueryProfilesRe   = regexp.MustCompile(`(?is)^SHOW\s+QUERY\s+PROFILES$`)
	showQueryProfileRe    = regexp.MustCompile(`(?is)^SHOW\s+QUERY\s+PROFILE\s+(.*)$`)
	showDdlsRe            = regexp.MustCompile(`(?is)^SHOW\s+DDLS$`)
	geminiRe              = regexp.MustCompile(`(?is)^GEMINI\s+(.*)$`)

	startBatchRe = regexp.MustCompile(`(?is)^START\s+BATCH\s+(DDL|DML)$`)
	runBatchRe   = regexp.MustCompile(`(?is)^RUN\s+BATCH$`)
	abortBatchRe = regexp.MustCompile(`(?is)^ABORT\s+BATCH(?:\s+TRANSACTION)?$`)
)

var (
	explainColumnNames = []string{"ID", "Query_Execution_Plan"}
	explainColumnAlign = []int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT}

	explainAnalyzeColumnNames = []string{"ID", "Query_Execution_Plan", "Rows_Returned", "Executions", "Total_Latency"}
	explainAnalyzeColumnAlign = []int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT}

	describeColumnNames = []string{"Column_Name", "Column_Type"}
)

func BuildStatement(input string) (Statement, error) {
	return BuildStatementWithComments(input, input)
}

var errStatementNotMatched = errors.New("statement not matched")

type SyncProtoStatement struct {
	UpsertPaths []string
	DeletePaths []string
}

func fdsToInfoSeq(fds *descriptorpb.FileDescriptorSet) iter.Seq[*descriptorInfo] {
	return scxiter.Flatmap(slices.Values(fds.GetFile()), fdpToInfo)
}

func (s *SyncProtoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	_, fds, err := session.GetDatabaseSchema(ctx)
	if err != nil {
		return nil, err

	}

	return bufferOrExecuteDdlStatements(ctx, session, composeProtoBundleDDLs(fds, s.UpsertPaths, s.DeletePaths))
}

func composeProtoBundleDDLs(fds *descriptorpb.FileDescriptorSet, upsertPaths, deletePaths []string) []string {
	fullNameSetFds := maps.Collect(
		scxiter.MapLift(fdsToInfoSeq(fds), func(info *descriptorInfo) (string, struct{}) {
			return info.FullName, struct{}{}
		}),
	)

	upsertExists, upsertNotExists := splitExistence(fullNameSetFds, upsertPaths)
	deleteExists, _ := splitExistence(fullNameSetFds, deletePaths)

	ddl := lo.Ternary(len(fds.GetFile()) == 0,
		lox.IfOrEmpty[ast.DDL](len(upsertNotExists) > 0,
			&ast.CreateProtoBundle{
				Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertNotExists)},
			}),
		lo.If[ast.DDL](len(upsertNotExists) == 0 && len(upsertExists) == 0 && len(deleteExists) == len(fullNameSetFds),
			&ast.DropProtoBundle{}).
			ElseIf(len(upsertNotExists) > 0 || len(upsertExists) > 0 || len(deleteExists) > 0,
				&ast.AlterProtoBundle{
					Insert: lox.IfOrEmpty(len(upsertNotExists) > 0,
						&ast.AlterProtoBundleInsert{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertNotExists)}}),
					Update: lox.IfOrEmpty(len(upsertExists) > 0,
						&ast.AlterProtoBundleUpdate{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertExists)}}),
					Delete: lox.IfOrEmpty(len(deleteExists) > 0,
						&ast.AlterProtoBundleDelete{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(deleteExists)}}),
				}).
			Else(nil),
	)

	if ddl == nil {
		return nil
	}

	return sliceOf(ddl.SQL())
}

func hasKey[K comparable, V any, M map[K]V](m M) func(key K) bool {
	return func(key K) bool {
		_, ok := m[key]
		return ok
	}
}
func splitExistence(fullNameSet map[string]struct{}, paths []string) ([]string, []string) {
	grouped := lo.GroupBy(paths, hasKey(fullNameSet))
	return grouped[true], grouped[false]
}

func parsePaths(p *memefish.Parser) ([]string, error) {
	expr, err := p.ParseExpr()
	if err != nil {
		return nil, err
	}

	switch e := expr.(type) {
	case *ast.ParenExpr:
		name, err := exprToFullName(e.Expr)
		if err != nil {
			return nil, err
		}
		return sliceOf(name), nil
	case *ast.TupleStructLiteral:
		names, err := scxiter.TryCollect(scxiter.MapErr(
			slices.Values(e.Values),
			exprToFullName))
		if err != nil {
			return nil, err
		}

		return names, err
	default:
		return nil, fmt.Errorf("must be paren expr or tuple of path, but: %T", expr)
	}
}

func parseSyncProtoBundle(s string) (Statement, error) {
	p := &memefish.Parser{Lexer: &memefish.Lexer{
		File: &token.File{
			Buffer: s,
		},
	}}
	err := p.NextToken()
	if err != nil {
		return nil, err
	}

	var upsertPaths, deletePaths []string
loop:
	for {
		switch {
		case p.Token.Kind == token.TokenEOF:
			break loop
		case p.Token.IsKeywordLike("UPSERT"):
			paths, err := parsePaths(p)
			if err != nil {
				return nil, fmt.Errorf("failed to parsePaths: %w", err)
			}
			upsertPaths = append(upsertPaths, paths...)
		case p.Token.IsKeywordLike("DELETE"):
			paths, err := parsePaths(p)
			if err != nil {
				return nil, err
			}
			deletePaths = append(deletePaths, paths...)
		default:
			return nil, fmt.Errorf("expected UPSERT or DELETE, but: %q", p.Token.AsString)
		}
	}
	return &SyncProtoStatement{UpsertPaths: upsertPaths, DeletePaths: deletePaths}, nil
}

func BuildCLIStatement(trimmed string) (Statement, error) {
	switch {
	case syncProtoBundleRe.MatchString(trimmed):
		return parseSyncProtoBundle(syncProtoBundleRe.FindStringSubmatch(trimmed)[1])
	case exitRe.MatchString(trimmed):
		return &ExitStatement{}, nil
	case useRe.MatchString(trimmed):
		matched := useRe.FindStringSubmatch(trimmed)
		return &UseStatement{Database: unquoteIdentifier(matched[1]), Role: unquoteIdentifier(matched[2])}, nil
	// DROP DATABASE is not native Cloud Spanner statement
	case dropDatabaseRe.MatchString(trimmed):
		matched := dropDatabaseRe.FindStringSubmatch(trimmed)
		return &DropDatabaseStatement{DatabaseId: unquoteIdentifier(matched[1])}, nil
	case truncateTableRe.MatchString(trimmed):
		matched := truncateTableRe.FindStringSubmatch(trimmed)
		return &TruncateTableStatement{Table: unquoteIdentifier(matched[1])}, nil
	case showLocalProtoRe.MatchString(trimmed):
		return &ShowLocalProtoStatement{}, nil
	case showRemoteProtoRe.MatchString(trimmed):
		return &ShowRemoteProtoStatement{}, nil
	case showDatabasesRe.MatchString(trimmed):
		return &ShowDatabasesStatement{}, nil
	case showCreateTableRe.MatchString(trimmed):
		matched := showCreateTableRe.FindStringSubmatch(trimmed)
		schema, table := extractSchemaAndTable(unquoteIdentifier(matched[1]))
		return &ShowCreateTableStatement{Schema: schema, Table: table}, nil
	case showTablesRe.MatchString(trimmed):
		matched := showTablesRe.FindStringSubmatch(trimmed)
		return &ShowTablesStatement{Schema: unquoteIdentifier(matched[1])}, nil
	case describeRe.MatchString(trimmed):
		matched := describeRe.FindStringSubmatch(trimmed)
		isDML := stmtkind.IsDMLLexical(matched[1])
		switch {
		case isDML:
			return &DescribeStatement{Statement: matched[1], IsDML: true}, nil
		default:
			return &DescribeStatement{Statement: matched[1]}, nil
		}
	case explainRe.MatchString(trimmed):
		matched := explainRe.FindStringSubmatch(trimmed)
		isAnalyze := matched[1] != ""
		isDML := stmtkind.IsDMLLexical(matched[2])
		switch {
		case isAnalyze && isDML:
			return &ExplainAnalyzeDmlStatement{Dml: matched[2]}, nil
		case isAnalyze:
			return &ExplainAnalyzeStatement{Query: matched[2]}, nil
		default:
			return &ExplainStatement{Explain: matched[2], IsDML: isDML}, nil
		}
	case showColumnsRe.MatchString(trimmed):
		matched := showColumnsRe.FindStringSubmatch(trimmed)
		schema, table := extractSchemaAndTable(unquoteIdentifier(matched[1]))
		return &ShowColumnsStatement{Schema: schema, Table: table}, nil
	case showIndexRe.MatchString(trimmed):
		matched := showIndexRe.FindStringSubmatch(trimmed)
		schema, table := extractSchemaAndTable(unquoteIdentifier(matched[1]))
		return &ShowIndexStatement{Schema: schema, Table: table}, nil
	case pdmlRe.MatchString(trimmed):
		matched := pdmlRe.FindStringSubmatch(trimmed)
		return &PartitionedDmlStatement{Dml: matched[1]}, nil
	case beginRwRe.MatchString(trimmed):
		return newBeginRwStatement(trimmed)
	case beginRoRe.MatchString(trimmed):
		return newBeginRoStatement(trimmed)
	case beginRe.MatchString(trimmed):
		return newBeginStatement(trimmed)
	case commitRe.MatchString(trimmed):
		return &CommitStatement{}, nil
	case rollbackRe.MatchString(trimmed):
		return &RollbackStatement{}, nil
	case setTransactionRe.MatchString(trimmed):
		matched := setTransactionRe.FindStringSubmatch(trimmed)
		isReadOnly, err := parseTransaction(matched[1])
		if err != nil {
			return nil, err
		}
		return &SetTransactionStatement{IsReadOnly: isReadOnly}, nil
	case showVariableRe.MatchString(trimmed):
		matched := showVariableRe.FindStringSubmatch(trimmed)
		return &ShowVariableStatement{VarName: matched[1]}, nil
	case setParamTypeRe.MatchString(trimmed):
		matched := setParamTypeRe.FindStringSubmatch(trimmed)
		return &SetParamTypeStatement{Name: matched[1], Type: matched[2]}, nil
	case setParamRe.MatchString(trimmed):
		matched := setParamRe.FindStringSubmatch(trimmed)
		return &SetParamValueStatement{Name: matched[1], Value: matched[2]}, nil
	case setRe.MatchString(trimmed):
		matched := setRe.FindStringSubmatch(trimmed)
		return &SetStatement{VarName: matched[1], Value: matched[2]}, nil
	case setAddRe.MatchString(trimmed):
		matched := setAddRe.FindStringSubmatch(trimmed)
		return &SetAddStatement{VarName: matched[1], Value: matched[2]}, nil
	case showParamsRe.MatchString(trimmed):
		return &ShowParamsStatement{}, nil
	case showVariablesRe.MatchString(trimmed):
		return &ShowVariablesStatement{}, nil
	case partitionRe.MatchString(trimmed):
		matched := partitionRe.FindStringSubmatch(trimmed)
		return &PartitionStatement{SQL: matched[1]}, nil
	case runPartitionedQueryRe.MatchString(trimmed):
		matched := runPartitionedQueryRe.FindStringSubmatch(trimmed)
		return &RunPartitionedQueryStatement{SQL: matched[1]}, nil
	case runPartitionRe.MatchString(trimmed):
		matched := runPartitionRe.FindStringSubmatch(trimmed)
		return &RunPartitionStatement{Token: unquoteString(matched[1])}, nil
	case tryPartitionedQueryRe.MatchString(trimmed):
		matched := tryPartitionedQueryRe.FindStringSubmatch(trimmed)
		return &TryPartitionedQueryStatement{SQL: matched[1]}, nil
	case mutateRe.MatchString(trimmed):
		matched := mutateRe.FindStringSubmatch(trimmed)
		return &MutateStatement{Table: unquoteIdentifier(matched[1]), Operation: matched[2], Body: matched[3]}, nil
	case showQueryProfilesRe.MatchString(trimmed):
		return &ShowQueryProfilesStatement{}, nil
	case showQueryProfileRe.MatchString(trimmed):
		matched := showQueryProfileRe.FindStringSubmatch(trimmed)
		fprint, err := strconv.ParseInt(strings.TrimSpace(matched[1]), 10, 64)
		if err != nil {
			return nil, err
		}
		return &ShowQueryProfileStatement{Fprint: fprint}, nil
	case showDdlsRe.MatchString(trimmed):
		return &ShowDdlsStatement{}, nil
	case geminiRe.MatchString(trimmed):
		matched := geminiRe.FindStringSubmatch(trimmed)
		return &GeminiStatement{Text: unquoteString(matched[1])}, nil
	case startBatchRe.MatchString(trimmed):
		matched := startBatchRe.FindStringSubmatch(trimmed)
		return &StartBatchStatement{Mode: lo.Ternary(strings.ToUpper(matched[1]) == "DDL", batchModeDDL, batchModeDML)}, nil
	case runBatchRe.MatchString(trimmed):
		return &RunBatchStatement{}, nil
	case abortBatchRe.MatchString(trimmed):
		return &AbortBatchStatement{}, nil
	default:
		return nil, errStatementNotMatched
	}
}

var transactionRe = regexp.MustCompile(`(?is)^(?:(READ\s+ONLY)|(READ\s+WRITE))$$`)

func parseTransaction(s string) (isReadOnly bool, err error) {
	if !transactionRe.MatchString(s) {
		return false, fmt.Errorf(`must be "READ ONLY" or "READ WRITE", but: %q`, s)
	}

	submatch := transactionRe.FindStringSubmatch(s)
	return submatch[1] != "", nil
}

func BuildStatementWithComments(stripped, raw string) (Statement, error) {
	return BuildStatementWithCommentsWithMode(stripped, raw, parseModeFallback)
}

type parseMode string

const (
	parseModeUnspecified parseMode = ""
	parseModeFallback    parseMode = "FALLBACK"
	parseModeNoMemefish  parseMode = "NO_MEMEFISH"
	parseMemefishOnly    parseMode = "MEMEFISH_ONLY"
)

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

func newStatement(sql string, params map[string]ast.Node, includeType bool) (spanner.Statement, error) {
	genParams, err := generateParams(params, includeType)
	if err != nil {
		return spanner.Statement{}, err
	}
	return spanner.Statement{
		SQL:    sql,
		Params: genParams,
	}, nil
}

type SelectStatement struct {
	Query string
}

func (s *SelectStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	_, err := session.DetermineTransaction(ctx)
	if err != nil {
		return nil, err
	}

	qm := session.systemVariables.QueryMode
	if qm == nil {
		return executeSQL(ctx, session, s.Query)
	}
	switch *qm {
	case sppb.ExecuteSqlRequest_NORMAL:
		return executeSQL(ctx, session, s.Query)
	case sppb.ExecuteSqlRequest_PLAN:
		return executeExplain(ctx, session, s.Query, false)
	case sppb.ExecuteSqlRequest_PROFILE:
		return executeExplainAnalyze(ctx, session, s.Query)
	default:
		return executeSQL(ctx, session, s.Query)
	}
}

type CreateDatabaseStatement struct {
	CreateStatement string
}

func (CreateDatabaseStatement) IsMutationStatement() {}

func (s *CreateDatabaseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	op, err := session.adminClient.CreateDatabase(ctx, &adminpb.CreateDatabaseRequest{
		Parent:          session.InstancePath(),
		CreateStatement: s.CreateStatement,
	})
	if err != nil {
		return nil, err
	}
	if _, err := op.Wait(ctx); err != nil {
		return nil, err
	}

	return &Result{IsMutation: true}, nil
}

type DropDatabaseStatement struct {
	DatabaseId string
}

func (DropDatabaseStatement) isMutationStatement() {}

func (s *DropDatabaseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.adminClient.DropDatabase(ctx, &adminpb.DropDatabaseRequest{
		Database: databasePath(session.systemVariables.Project, session.systemVariables.Instance, session.systemVariables.Database),
	}); err != nil {
		return nil, err
	}

	return &Result{
		IsMutation: true,
	}, nil
}

type DdlStatement struct {
	Ddl string
}

func (DdlStatement) isMutationStatement() {}

func (s *DdlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return bufferOrExecuteDdlStatements(ctx, session, []string{s.Ddl})
}

type BulkDdlStatement struct {
	Ddls []string
}

func (BulkDdlStatement) IsMutationStatement() {}

func (s *BulkDdlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDdlStatements(ctx, session, s.Ddls)
}

type BatchDMLStatement struct {
	DMLs []string
}

func (BatchDMLStatement) IsMutationStatement() {}

func (s *BatchDMLStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeBatchDML(ctx, session, s.DMLs)
}

type batchMode int

const (
	batchModeDDL batchMode = iota + 1
	batchModeDML
)

type StartBatchStatement struct {
	Mode batchMode
}

func (s *StartBatchStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.currentBatch != nil {
		return nil, fmt.Errorf("already in batch, you should execute ABORT BATCH")
	}

	switch s.Mode {
	case batchModeDDL:
		session.currentBatch = &BulkDdlStatement{}
	case batchModeDML:
		session.currentBatch = &BatchDMLStatement{}
	default:
		return nil, fmt.Errorf("unknown batchMode: %v", s.Mode)
	}

	return &Result{KeepVariables: true}, nil
}

type AbortBatchStatement struct{}

func (s *AbortBatchStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	session.currentBatch = nil
	return &Result{KeepVariables: true}, nil
}

type RunBatchStatement struct{}

func (s *RunBatchStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.currentBatch == nil {
		return nil, errors.New("no active batch")
	}

	batch := session.currentBatch
	session.currentBatch = nil

	result, err := session.ExecuteStatement(ctx, batch)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type ShowVariableStatement struct {
	VarName string
}

func (s *ShowVariableStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	value, err := session.systemVariables.Get(s.VarName)
	if err != nil {
		return nil, err
	}

	columnNames := slices.Sorted(maps.Keys(value))
	var row []string
	for n := range slices.Values(columnNames) {
		row = append(row, value[n])
	}
	return &Result{
		ColumnNames:   columnNames,
		Rows:          sliceOf(toRow(row...)),
		KeepVariables: true,
	}, nil
}

type ShowParamsStatement struct{}

func (s *ShowParamsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	strMap := make(map[string]string)
	for k, v := range session.systemVariables.Params {
		strMap[k] = v.SQL()
	}

	rows := slices.SortedFunc(
		scxiter.MapLower(maps.All(session.systemVariables.Params), func(k string, v ast.Node) Row {
			return toRow(k, lo.Ternary(lox.InstanceOf[ast.Type](v), "TYPE", "VALUE"), v.SQL())
		}),
		ToSortFunc(func(r Row) string { return r.Columns[0] }))

	return &Result{
		ColumnNames:   []string{"Param_Name", "Param_Kind", "Param_Value"},
		Rows:          rows,
		KeepVariables: true,
	}, nil
}

type ShowVariablesStatement struct{}

func (s *ShowVariablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	merged := make(map[string]string)
	for k, v := range accessorMap {
		if v.Getter == nil {
			continue
		}

		value, err := v.Getter(session.systemVariables, k)
		if errors.Is(err, errIgnored) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for k, v := range value {
			merged[k] = v
		}
	}

	rows := slices.SortedFunc(
		scxiter.MapLower(maps.All(merged), func(k, v string) Row { return toRow(k, v) }),
		ToSortFunc(func(r Row) string { return r.Columns[0] }))

	return &Result{
		ColumnNames:   []string{"name", "value"},
		Rows:          rows,
		KeepVariables: true,
	}, nil
}

type SetParamTypeStatement struct {
	Name string
	Type string
}

func (s *SetParamTypeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if expr, err := memefish.ParseType("", s.Type); err != nil {
		return nil, err
	} else {
		session.systemVariables.Params[s.Name] = expr
		return &Result{KeepVariables: true}, nil
	}
}

type SetParamValueStatement struct {
	Name  string
	Value string
}

func (s *SetParamValueStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if expr, err := memefish.ParseExpr("", s.Value); err != nil {
		return nil, err
	} else {
		session.systemVariables.Params[s.Name] = expr
		return &Result{KeepVariables: true}, nil
	}
}

type SetStatement struct {
	VarName string
	Value   string
}

func (s *SetStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.systemVariables.Set(s.VarName, s.Value); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type SetAddStatement struct {
	VarName string
	Value   string
}

func (s *SetAddStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.systemVariables.Add(s.VarName, s.Value); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type ShowLocalProtoStatement struct{}

func (s *ShowLocalProtoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	fds := session.systemVariables.ProtoDescriptor

	rows := slices.Collect(
		scxiter.Map(
			scxiter.Flatmap(slices.Values(fds.GetFile()), fdpToInfo),
			func(info *descriptorInfo) Row {
				return toRow(info.FullName, info.Kind, info.Package, info.FileName)
			},
		),
	)

	return &Result{
		ColumnNames:   []string{"full_name", "kind", "package", "file"},
		Rows:          rows,
		AffectedRows:  len(rows),
		KeepVariables: true,
	}, nil
}

type ShowRemoteProtoStatement struct{}

func (s *ShowRemoteProtoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	resp, err := session.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(resp.GetProtoDescriptors(), &fds); err != nil {
		return nil, err
	}

	rows := slices.Collect(
		scxiter.Map(
			scxiter.Flatmap(slices.Values(fds.GetFile()), fdpToInfo),
			func(info *descriptorInfo) Row {
				return toRow(info.FullName, info.Kind, info.Package)
			},
		),
	)

	return &Result{
		ColumnNames:   []string{"full_name", "kind", "package"},
		Rows:          rows,
		AffectedRows:  len(rows),
		KeepVariables: true,
	}, nil
}

type ShowDatabasesStatement struct {
}

var extractDatabaseRe = regexp.MustCompile(`projects/[^/]+/instances/[^/]+/databases/(.+)`)

func (s *ShowDatabasesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	dbIter := session.adminClient.ListDatabases(ctx, &adminpb.ListDatabasesRequest{
		Parent: session.InstancePath(),
	})

	var rows []Row
	for database, err := range dbIter.All() {
		if err != nil {
			return nil, err
		}

		matched := extractDatabaseRe.FindStringSubmatch(database.GetName())
		rows = append(rows, toRow(matched[1]))
	}

	return &Result{ColumnNames: []string{"Database"},
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}

type ShowCreateTableStatement struct {
	Schema string
	Table  string
}

func (s *ShowCreateTableStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	ddlResponse, err := session.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	var rows []Row
	for _, stmt := range ddlResponse.Statements {
		if isCreateTableDDL(stmt, s.Schema, s.Table) {
			fqn := lox.IfOrEmpty(s.Schema != "", s.Schema+".") + s.Table
			rows = append(rows, toRow(fqn, stmt))
			break
		}
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	}

	result := &Result{
		ColumnNames:  []string{"Table", "Create Table"},
		Rows:         rows,
		AffectedRows: len(rows),
	}

	return result, nil
}

func isCreateTableDDL(ddl string, schema string, table string) bool {
	table = regexp.QuoteMeta(table)
	var re string
	if schema == "" {
		re = fmt.Sprintf("(?i)^CREATE TABLE (%s|`%s`)\\s*\\(", table, table)
	} else {
		re = fmt.Sprintf("(?i)^CREATE TABLE (%s|`%s`)\\.(%s|`%s`)\\s*\\(", schema, schema, table, table)
	}
	return regexp.MustCompile(re).MatchString(ddl)
}

type ShowTablesStatement struct {
	Schema string
}

func (s *ShowTablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	alias := fmt.Sprintf("Tables_in_%s", session.systemVariables.Database)
	stmt := spanner.Statement{
		SQL:    fmt.Sprintf("SELECT t.TABLE_NAME AS `%s` FROM INFORMATION_SCHEMA.TABLES AS t WHERE t.TABLE_CATALOG = '' and t.TABLE_SCHEMA = @schema", alias),
		Params: map[string]any{"schema": s.Schema},
	}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW TABLES", stmt, nil)
}

type ExplainStatement struct {
	Explain string
	IsDML   bool
}

// Execute processes `EXPLAIN` statement for queries and DMLs.
func (s *ExplainStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplain(ctx, session, s.Explain, s.IsDML)
}

type DescribeStatement struct {
	Statement string
	IsDML     bool
}

// Execute processes `DESCRIBE` statement for queries and DMLs.
func (s *DescribeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt, err := newStatement(s.Statement, session.systemVariables.Params, true)
	if err != nil {
		return nil, err
	}

	_, timestamp, metadata, err := runAnalyzeQuery(ctx, session, stmt, s.IsDML)
	if err != nil {
		return nil, err
	}

	var rows []Row
	for _, field := range metadata.GetRowType().GetFields() {
		rows = append(rows, toRow(field.GetName(), formatTypeVerbose(field.GetType())))
	}

	result := &Result{
		AffectedRows: len(rows),
		ColumnNames:  describeColumnNames,
		Timestamp:    timestamp,
		Rows:         rows,
	}

	return result, nil
}

type ExplainAnalyzeStatement struct {
	Query string
}

func (s *ExplainAnalyzeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	sql := s.Query

	return executeExplainAnalyze(ctx, session, sql)
}

type ShowColumnsStatement struct {
	Schema string
	Table  string
}

func (s *ShowColumnsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt := spanner.Statement{SQL: `SELECT
  C.COLUMN_NAME as Field,
  C.SPANNER_TYPE as Type,
  C.IS_NULLABLE as ` + "`NULL`" + `,
  I.INDEX_TYPE as Key,
  IC.COLUMN_ORDERING as Key_Order,
  CONCAT(CO.OPTION_NAME, "=", CO.OPTION_VALUE) as Options
FROM
  INFORMATION_SCHEMA.COLUMNS C
LEFT JOIN
  INFORMATION_SCHEMA.INDEX_COLUMNS IC USING(TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME)
LEFT JOIN
  INFORMATION_SCHEMA.INDEXES I USING(TABLE_SCHEMA, TABLE_NAME, INDEX_NAME)
LEFT JOIN
  INFORMATION_SCHEMA.COLUMN_OPTIONS CO USING(TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME)
WHERE
  LOWER(C.TABLE_SCHEMA) = LOWER(@table_schema) AND LOWER(C.TABLE_NAME) = LOWER(@table_name)
ORDER BY
  C.ORDINAL_POSITION ASC`,
		Params: map[string]any{"table_name": s.Table, "table_schema": s.Schema}}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW COLUMNS", stmt, func() error {
		return fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	})
}

func extractSchemaAndTable(s string) (string, string) {
	schema, table, found := strings.Cut(s, ".")
	if !found {
		return "", unquoteIdentifier(s)
	}
	return unquoteIdentifier(schema), unquoteIdentifier(table)
}

type ShowIndexStatement struct {
	Schema string
	Table  string
}

func (s *ShowIndexStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt := spanner.Statement{
		SQL: `SELECT
  TABLE_NAME as Table,
  PARENT_TABLE_NAME as Parent_table,
  INDEX_NAME as Index_name,
  INDEX_TYPE as Index_type,
  IS_UNIQUE as Is_unique,
  IS_NULL_FILTERED as Is_null_filtered,
  INDEX_STATE as Index_state
FROM
  INFORMATION_SCHEMA.INDEXES I
WHERE
  LOWER(I.TABLE_SCHEMA) = @table_schema AND LOWER(TABLE_NAME) = LOWER(@table_name)`,
		Params: map[string]any{"table_name": s.Table, "table_schema": s.Schema}}

	return executeInformationSchemaBasedStatement(ctx, session, "SHOW INDEX", stmt, func() error {
		return fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	})
}

type TruncateTableStatement struct {
	Table string
}

func (TruncateTableStatement) isMutationStatement() {}

func (s *TruncateTableStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		// PartitionedUpdate creates a new transaction and it could cause dead lock with the current running transaction.
		return nil, errors.New(`"TRUNCATE TABLE" can not be used in a read-write transaction`)
	}
	if session.InReadOnlyTransaction() {
		// Just for user-friendly.
		return nil, errors.New(`"TRUNCATE TABLE" can not be used in a read-only transaction`)
	}

	stmt := spanner.NewStatement(fmt.Sprintf("DELETE FROM `%s` WHERE true", s.Table))
	ctx, cancel := context.WithTimeout(ctx, pdmlTimeout)
	defer cancel()

	count, err := session.client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		return nil, err
	}
	return &Result{
		IsMutation:   true,
		AffectedRows: int(count),
	}, nil
}

type DmlStatement struct {
	Dml string
}

func (DmlStatement) isMutationStatement() {}

func (s *DmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	switch lo.FromPtr(session.systemVariables.QueryMode) {
	case sppb.ExecuteSqlRequest_PLAN:
		return executeExplain(ctx, session, s.Dml, true)
	case sppb.ExecuteSqlRequest_PROFILE:
		return executeExplainAnalyzeDML(ctx, session, s.Dml)
	default:
		return bufferOrExecuteDML(ctx, session, s.Dml)
	}
}

type PartitionedDmlStatement struct {
	Dml string
}

func (PartitionedDmlStatement) isMutationStatement() {}

func (s *PartitionedDmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		// PartitionedUpdate creates a new transaction and it could cause dead lock with the current running transaction.
		return nil, errors.New(`Partitioned DML statement can not be run in a read-write transaction`)
	}
	if session.InReadOnlyTransaction() {
		// Just for user-friendly.
		return nil, errors.New(`Partitioned DML statement can not be run in a read-only transaction`)
	}

	ctx, cancel := context.WithTimeout(ctx, pdmlTimeout)
	defer cancel()

	count, err := session.client.PartitionedUpdate(ctx, spanner.NewStatement(s.Dml))
	if err != nil {
		return nil, err
	}

	return &Result{
		IsMutation:       true,
		AffectedRows:     int(count),
		AffectedRowsType: rowCountTypeLowerBound,
	}, nil
}

type ExplainAnalyzeDmlStatement struct {
	Dml string
}

func (ExplainAnalyzeDmlStatement) isMutationStatement() {}

func (s *ExplainAnalyzeDmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplainAnalyzeDML(ctx, session, s.Dml)
}

type BeginRwStatement struct {
	Priority sppb.RequestOptions_Priority
}

func newBeginRwStatement(input string) (*BeginRwStatement, error) {
	matched := beginRwRe.FindStringSubmatch(input)
	stmt := &BeginRwStatement{}

	if matched[1] != "" {
		priority, err := parsePriority(matched[1])
		if err != nil {
			return nil, err
		}
		stmt.Priority = priority
	}

	return stmt, nil
}

func (BeginRwStatement) isMutationStatement() {}

func (s *BeginRwStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		return nil, errors.New("you're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	if session.InReadOnlyTransaction() {
		return nil, errors.New("you're in read-only transaction. Please finish the transaction by 'CLOSE;'")
	}

	if err := session.BeginReadWriteTransaction(ctx, s.Priority); err != nil {
		return nil, err
	}

	return &Result{IsMutation: true}, nil
}

type BeginStatement struct {
	Priority sppb.RequestOptions_Priority
}

func newBeginStatement(input string) (*BeginStatement, error) {
	matched := beginRe.FindStringSubmatch(input)
	stmt := &BeginStatement{}

	if matched[1] != "" {
		priority, err := parsePriority(matched[1])
		if err != nil {
			return nil, err
		}
		stmt.Priority = priority
	}

	return stmt, nil
}

func (s *BeginStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InTransaction() {
		return nil, errors.New("you're in transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	if session.systemVariables.ReadOnly {
		ts, err := session.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, s.Priority)
		if err != nil {
			return nil, err
		}

		return &Result{
			IsMutation: true,
			Timestamp:  ts,
		}, nil
	}

	err := session.BeginPendingTransaction(ctx, s.Priority)
	if err != nil {
		return nil, err
	}

	return &Result{IsMutation: true}, nil
}

type SetTransactionStatement struct {
	IsReadOnly bool
}

func (s *SetTransactionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{IsMutation: true}
	if !session.InPendingTransaction() {
		// nop
		return result, nil
	}

	if s.IsReadOnly {
		ts, err := session.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, session.tc.priority)
		if err != nil {
			return nil, err
		}
		result.Timestamp = ts
		return result, nil
	} else {
		err := session.BeginReadWriteTransaction(ctx, session.tc.priority)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

type CommitStatement struct{}

func (s *CommitStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{IsMutation: true}
	switch {
	case session.InPendingTransaction():
		if err := session.ClosePendingTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case !session.InReadWriteTransaction() && !session.InReadOnlyTransaction():
		return result, nil
	case session.InReadOnlyTransaction():
		if err := session.CloseReadOnlyTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case session.InReadWriteTransaction():
		resp, err := session.CommitReadWriteTransaction(ctx)
		if err != nil {
			return nil, err
		}

		result.Timestamp = resp.CommitTs
		result.CommitStats = resp.CommitStats
		return result, nil
	default:
		return nil, errors.New("invalid state")
	}
}

type RollbackStatement struct{}

func (s *RollbackStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{IsMutation: true}
	switch {
	case session.InPendingTransaction():
		if err := session.ClosePendingTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case !session.InReadWriteTransaction() && !session.InReadOnlyTransaction():
		return result, nil
	case session.InReadOnlyTransaction():
		if err := session.CloseReadOnlyTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case session.InReadWriteTransaction():
		if err := session.RollbackReadWriteTransaction(ctx); err != nil {
			return nil, err
		}

		return result, nil
	default:
		return nil, errors.New("invalid state")
	}
}

type timestampBoundType int

const (
	timestampBoundUnspecified timestampBoundType = iota
	strong
	exactStaleness
	readTimestamp
)

type BeginRoStatement struct {
	TimestampBoundType timestampBoundType
	Staleness          time.Duration
	Timestamp          time.Time
	Priority           sppb.RequestOptions_Priority
}

func newBeginRoStatement(input string) (*BeginRoStatement, error) {
	stmt := &BeginRoStatement{
		TimestampBoundType: timestampBoundUnspecified,
	}

	matched := beginRoRe.FindStringSubmatch(input)
	if matched[1] != "" {
		if t, err := time.Parse(time.RFC3339Nano, matched[1]); err == nil {
			stmt = &BeginRoStatement{
				TimestampBoundType: readTimestamp,
				Timestamp:          t,
			}
		}
		if i, err := strconv.Atoi(matched[1]); err == nil {
			stmt = &BeginRoStatement{
				TimestampBoundType: exactStaleness,
				Staleness:          time.Duration(i) * time.Second,
			}
		}
	}

	if matched[2] != "" {
		priority, err := parsePriority(matched[2])
		if err != nil {
			return nil, err
		}
		stmt.Priority = priority
	}

	return stmt, nil
}

func (s *BeginRoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		return nil, errors.New("invalid state: You're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	if session.InReadOnlyTransaction() {
		// close current transaction implicitly
		if _, err := (&RollbackStatement{}).Execute(ctx, session); err != nil {
			return nil, fmt.Errorf("error on close current transaction: %w", err)
		}
	}

	ts, err := session.BeginReadOnlyTransaction(ctx, s.TimestampBoundType, s.Staleness, s.Timestamp, s.Priority)
	if err != nil {
		return nil, err
	}

	return &Result{
		IsMutation: true,
		Timestamp:  ts,
	}, nil
}

type PartitionStatement struct{ SQL string }

func (s *PartitionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt, err := newStatement(s.SQL, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	partitions, batchROTx, err := session.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}

	rows := slices.Collect(xiter.Map(
		func(partition *spanner.Partition) Row {
			return toRow(base64.StdEncoding.EncodeToString(partition.GetPartitionToken()))
		},
		slices.Values(partitions)))

	ts, err := batchROTx.Timestamp()
	if err != nil {
		return nil, err
	}

	return &Result{
		ColumnNames:  sliceOf("Partition_Token"),
		Rows:         rows,
		AffectedRows: len(rows),
		Timestamp:    ts,
		ForceWrap:    true,
	}, nil
}

type TryPartitionedQueryStatement struct{ SQL string }

func (s *Session) RunPartitionQuery(ctx context.Context, stmt spanner.Statement) ([]*spanner.Partition, *spanner.BatchReadOnlyTransaction, error) {
	tb := lo.FromPtrOr(s.systemVariables.ReadOnlyStaleness, spanner.StrongRead())

	batchROTx, err := s.client.BatchReadOnlyTransaction(ctx, tb)
	if err != nil {
		return nil, nil, err
	}

	partitions, err := batchROTx.PartitionQueryWithOptions(ctx, stmt, spanner.PartitionOptions{}, spanner.QueryOptions{})
	if err != nil {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
		return nil, nil, fmt.Errorf("query can't be a partition query: %w", err)
	}
	return partitions, batchROTx, nil
}

func (s *TryPartitionedQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt, err := newStatement(s.SQL, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	_, batchROTx, err := session.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}

	defer func() {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
	}()

	ts, err := batchROTx.Timestamp()
	if err != nil {
		return nil, err
	}

	return &Result{
		ColumnNames:  sliceOf("Root_Partitionable"),
		Rows:         sliceOf(toRow("TRUE")),
		AffectedRows: 1,
		Timestamp:    ts,
		ForceWrap:    true,
	}, nil
}

type RunPartitionedQueryStatement struct{ SQL string }

func (s *RunPartitionedQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return nil, errors.New("unsupported statement")
}

type RunPartitionStatement struct{ Token string }

func (s *RunPartitionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return nil, errors.New("unsupported statement")
}

type MutateStatement struct {
	Table     string
	Operation string
	Body      string
}

func (MutateStatement) isMutationStatement() {}

func (s *MutateStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	mutations, err := parseMutation(s.Table, s.Operation, s.Body)
	if err != nil {
		return nil, err
	}
	_, stats, _, _, err := session.RunInNewOrExistRwTx(ctx, func() (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		err = session.tc.RWTxn().BufferWrite(mutations)
		if err != nil {
			return 0, nil, nil, err
		}
		return 0, nil, nil, err
	})
	if err != nil {
		return nil, err
	}
	return &Result{
		IsMutation:  true,
		CommitStats: stats.CommitStats,
		Timestamp:   stats.CommitTs,
	}, nil
}

type ShowQueryProfilesStatement struct{}

type queryProfiles struct {
	RawQueryPlan jsontext.Value  `json:"queryPlan"`
	QueryPlan    *sppb.QueryPlan `json:"-"`
	QueryStats   QueryStats      `json:"queryStats"`
	Fprint       string          `json:"fprint"`
}

type queryProfilesRow struct {
	IntervalEnd     time.Time        `spanner:"INTERVAL_END"`
	TextFingerprint int64            `spanner:"TEXT_FINGERPRINT"`
	LatencySeconds  float64          `spanner:"LATENCY_SECONDS"`
	RawQueryProfile spanner.NullJSON `spanner:"QUERY_PROFILE"`
	QueryProfile    *queryProfiles   `spanner:"-"`
}

func toQpr(row *spanner.Row) (*queryProfilesRow, error) {
	var qpr queryProfilesRow
	if err := row.ToStruct(&qpr); err != nil {
		return nil, err
	}

	var profile queryProfiles
	err := json.Unmarshal([]byte(qpr.RawQueryProfile.String()), &profile)
	if err != nil {
		return nil, err
	}
	qpr.QueryProfile = &profile

	var queryPlan sppb.QueryPlan
	err = protojson.Unmarshal(profile.RawQueryPlan, &queryPlan)
	if err != nil {
		return nil, err
	}
	profile.QueryPlan = &queryPlan

	return &qpr, nil
}

func (s *ShowQueryProfilesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can't be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, fmt.Errorf(`%q can not be used in a read-write transaction`, `SPANNER_SYS.QUERY_PROFILES_TOP_HOUR`)
	}

	stmt := spanner.Statement{
		SQL: `SELECT INTERVAL_END, TEXT_FINGERPRINT, LATENCY_SECONDS, PARSE_JSON(QUERY_PROFILE) AS QUERY_PROFILE FROM SPANNER_SYS.QUERY_PROFILES_TOP_HOUR`,
	}

	iter, _ := session.RunQuery(ctx, stmt)

	rows, _, _, _, _, err := consumeRowIterCollect(iter, toQpr)
	if err != nil {
		return nil, err
	}

	var resultRows []Row
	for _, row := range rows {
		rows, predicates, err := processPlanWithStats(row.QueryProfile.QueryPlan)
		if err != nil {
			return nil, err
		}

		maxIDLength := max(hiter.Max(xiter.Map(func(row Row) int { return len(row.Columns[0]) }, slices.Values(rows))), 2)

		pprinter := pp.New()
		pprinter.SetColoringEnabled(false)

		tree := strings.Join(slices.Collect(xiter.Map(
			func(r Row) string {
				return runewidth.FillLeft(r.Columns[0], maxIDLength) + " | " + r.Columns[1]
			},
			slices.Values(rows))), "\n")

		resultRows = append(resultRows, toRow(row.QueryProfile.QueryStats.QueryText+"\n"+runewidth.FillRight("ID", maxIDLength)+" | Plan\n"+tree+
			lox.IfOrEmpty(len(predicates) > 0, "\nPredicates:\n"+strings.Join(predicates, "\n"))+"\n"+
			formatStats(row)))
	}

	return &Result{
		ColumnNames:  sliceOf("Plan"),
		Rows:         resultRows,
		AffectedRows: len(resultRows),
	}, nil
}

var (
	t    = template.New("temp")
	temp = lo.Must(t.Parse(
		`
interval_end:                 {{.IntervalEnd}}
text_fingerprint:             {{.TextFingerprint}}
{{with .QueryProfile.QueryStats -}}
elapsed_time:                 {{.ElapsedTime}}
cpu_time:                     {{.CPUTime}}
rows_returned:                {{.RowsReturned}}
deleted_rows_scanned:         {{.DeletedRowsScanned}}
optimizer_version:            {{.OptimizerVersion}}
optimizer_statistics_package: {{.OptimizerStatisticsPackage}}
{{end}}`))
)

func formatStats(stats *queryProfilesRow) string {
	var sb strings.Builder
	if stats == nil {
		return ""
	}

	err := temp.Execute(&sb, stats)
	if err != nil {
		log.Println(err)
		return ""
	}

	return sb.String()
}

type ShowQueryProfileStatement struct {
	Fprint int64
}

func (s *ShowQueryProfileStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can't be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, fmt.Errorf(`%q can not be used in a read-write transaction`, `SPANNER_SYS.QUERY_PROFILES_TOP_HOUR`)
	}

	stmt := spanner.Statement{
		SQL: `SELECT INTERVAL_END, TEXT_FINGERPRINT, LATENCY_SECONDS, PARSE_JSON(QUERY_PROFILE) AS QUERY_PROFILE
FROM SPANNER_SYS.QUERY_PROFILES_TOP_HOUR
WHERE TEXT_FINGERPRINT = @fprint
ORDER BY INTERVAL_END DESC`,
		Params: map[string]interface{}{"fprint": s.Fprint},
	}

	iter, _ := session.RunQuery(ctx, stmt)

	qprs, _, _, _, _, err := consumeRowIterCollect(iter, toQpr)
	if err != nil {
		return nil, err
	}

	qpr, ok := lo.First(qprs)
	if !ok {
		return nil, errors.New("empty result")
	}

	rows, predicates, err := processPlanWithStats(qpr.QueryProfile.QueryPlan)
	if err != nil {
		return nil, err
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	result := &Result{
		ColumnNames:  explainAnalyzeColumnNames,
		ColumnAlign:  explainAnalyzeColumnAlign,
		ForceVerbose: true,
		AffectedRows: len(rows),
		Stats:        qpr.QueryProfile.QueryStats,
		Rows:         rows,
		Predicates:   predicates,
		LintResults:  lox.IfOrEmptyF(session.systemVariables.LintPlan, func() []string { return lintPlan(qpr.QueryProfile.QueryPlan) }),
	}
	return result, nil
}

type ShowDdlsStatement struct{}

func (s *ShowDdlsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	resp, err := session.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		KeepVariables: true,
		// intentionally empty column name to make TAB format valid DDL
		ColumnNames: sliceOf(""),
		Rows: sliceOf(toRow(hiter.StringsCollect(0, xiter.Map(
			func(s string) string { return s + ";\n" },
			slices.Values(resp.GetStatements()))))),
	}, nil
}

type GeminiStatement struct {
	Text string
}

func (s *GeminiStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	resp, err := session.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}
	sql, err := geminiComposeQuery(ctx, resp, session.systemVariables.VertexAIProject, s.Text)
	if err != nil {
		return nil, err
	}
	return &Result{PreInput: sql, Rows: sliceOf(toRow(sql)), ColumnNames: sliceOf("Answer")}, nil
}

type NopStatement struct{}

func (s *NopStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	// do nothing
	return &Result{}, nil
}

type ExitStatement struct {
	NopStatement
}

type UseStatement struct {
	Database string
	Role     string
	NopStatement
}

func parsePriority(priority string) (sppb.RequestOptions_Priority, error) {
	upper := strings.ToUpper(priority)

	var value string
	if !strings.HasPrefix(upper, "PRIORITY_") {
		value = "PRIORITY_" + upper
	} else {
		value = upper
	}

	p, ok := sppb.RequestOptions_Priority_value[value]
	if !ok {
		return sppb.RequestOptions_PRIORITY_UNSPECIFIED, fmt.Errorf("invalid priority: %q", value)
	}
	return sppb.RequestOptions_Priority(p), nil
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
