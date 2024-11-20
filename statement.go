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
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/apstndb/lox"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
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

// rowCountType is type of modified rows count by DML.
type rowCountType int

const (
	// rowCountTypeExact is exact count type for DML result.
	rowCountTypeExact rowCountType = iota
	// rowCountTypeLowerBound is lower bound type for Partitioned DML result.
	rowCountTypeLowerBound
)

type Result struct {
	ColumnNames      []string
	Rows             []Row
	Predicates       []string
	AffectedRows     int
	AffectedRowsType rowCountType
	Stats            QueryStats
	IsMutation       bool
	Timestamp        time.Time
	ForceVerbose     bool
	CommitStats      *sppb.CommitResponse_CommitStats
	KeepVariables    bool

	// ColumnTypes will be printed in `--verbose` mode if it is not empty
	ColumnTypes []*sppb.StructType_Field
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
	beginRwRe  = regexp.MustCompile(`(?is)^BEGIN(?:\s+RW)?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?(?:\s+TAG\s+(.+))?$`)
	beginRoRe  = regexp.MustCompile(`(?is)^BEGIN\s+RO(?:\s+([^\s]+))?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?(?:\s+TAG\s+(.+))?$`)
	commitRe   = regexp.MustCompile(`(?is)^COMMIT$`)
	rollbackRe = regexp.MustCompile(`(?is)^ROLLBACK$`)
	closeRe    = regexp.MustCompile(`(?is)^CLOSE$`)

	// Other
	exitRe            = regexp.MustCompile(`(?is)^EXIT$`)
	useRe             = regexp.MustCompile(`(?is)^USE\s+([^\s]+)(?:\s+ROLE\s+(.+))?$`)
	showLocalProtoRe  = regexp.MustCompile(`(?is)^SHOW\s+LOCAL\s+PROTO$`)
	showRemoteProtoRe = regexp.MustCompile(`(?is)^SHOW\s+REMOTE\s+PROTO$`)
	showDatabasesRe   = regexp.MustCompile(`(?is)^SHOW\s+DATABASES$`)
	showCreateTableRe = regexp.MustCompile(`(?is)^SHOW\s+CREATE\s+TABLE\s+(.+)$`)
	showTablesRe      = regexp.MustCompile(`(?is)^SHOW\s+TABLES(?:\s+(.+))?$`)
	showColumnsRe     = regexp.MustCompile(`(?is)^(?:SHOW\s+COLUMNS\s+FROM)\s+(.+)$`)
	showIndexRe       = regexp.MustCompile(`(?is)^SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(.+)$`)
	explainRe         = regexp.MustCompile(`(?is)^EXPLAIN\s+(ANALYZE\s+)?(.+)$`)
	describeRe        = regexp.MustCompile(`(?is)^DESCRIBE\s+(.+)$`)
	showVariableRe    = regexp.MustCompile(`(?is)^SHOW\s+VARIABLE\s+(.+)$`)
	setParamTypeRe    = regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*([^=]*)$`)
	setParamRe        = regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*=\s*(.*)$`)
	setRe             = regexp.MustCompile(`(?is)^SET\s+([^\s=]+)\s*=\s*(\S.*)$`)
	setAddRe          = regexp.MustCompile(`(?is)^SET\s+([^\s+=]+)\s*\+=\s*(\S.*)$`)
	showParamsRe      = regexp.MustCompile(`(?is)^SHOW\s+PARAMS$`)
	showVariablesRe   = regexp.MustCompile(`(?is)^SHOW\s+VARIABLES$`)
)

var (
	explainColumnNames        = []string{"ID", "Query_Execution_Plan"}
	explainAnalyzeColumnNames = []string{"ID", "Query_Execution_Plan", "Rows_Returned", "Executions", "Total_Latency"}
	describeColumnNames       = []string{"Column_Name", "Column_Type"}
)

func BuildStatement(input string) (Statement, error) {
	return BuildStatementWithComments(input, input)
}

var errStatementNotMatched = errors.New("statement not matched")

func BuildCLIStatement(trimmed string) (Statement, error) {
	switch {
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
	case commitRe.MatchString(trimmed):
		return &CommitStatement{}, nil
	case rollbackRe.MatchString(trimmed):
		return &RollbackStatement{}, nil
	case closeRe.MatchString(trimmed):
		return &CloseStatement{}, nil
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
	default:
		return nil, errStatementNotMatched
	}
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

func (s *DdlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDdlStatements(ctx, session, []string{s.Ddl})
}

type BulkDdlStatement struct {
	Ddls []string
}

func (s *BulkDdlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDdlStatements(ctx, session, s.Ddls)
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

	rows := slices.Collect(scxiter.Flatmap(slices.Values(fds.GetFile()), fdpToRows))

	return &Result{
		ColumnNames:   []string{"full_name", "package", "file"},
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

	rows := slices.Collect(scxiter.Map(scxiter.Flatmap(slices.Values(fds.GetFile()), fdpToRows), func(in Row) Row {
		return toRow(in.Columns[:2]...)
	}))

	return &Result{
		ColumnNames:   []string{"full_name", "package"},
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

func toRow(vs ...string) Row {
	return Row{Columns: vs}
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

func (s *DmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	qm := session.systemVariables.QueryMode
	if qm == nil {
		return executeDML(ctx, session, s.Dml)
	}
	switch *qm {
	case sppb.ExecuteSqlRequest_NORMAL:
		return executeDML(ctx, session, s.Dml)
	case sppb.ExecuteSqlRequest_PLAN:
		return executeExplain(ctx, session, s.Dml, true)
	case sppb.ExecuteSqlRequest_PROFILE:
		return executeExplainAnalyzeDML(ctx, session, s.Dml)
	default:
		return executeDML(ctx, session, s.Dml)
	}
}

type PartitionedDmlStatement struct {
	Dml string
}

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

func (s *ExplainAnalyzeDmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplainAnalyzeDML(ctx, session, s.Dml)
}

type BeginRwStatement struct {
	Priority sppb.RequestOptions_Priority
	Tag      string
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

	if matched[2] != "" {
		stmt.Tag = matched[2]
	}

	return stmt, nil
}

func (s *BeginRwStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		return nil, errors.New("you're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	if session.InReadOnlyTransaction() {
		return nil, errors.New("you're in read-only transaction. Please finish the transaction by 'CLOSE;'")
	}

	if err := session.BeginReadWriteTransaction(ctx, s.Priority, s.Tag); err != nil {
		return nil, err
	}

	return &Result{IsMutation: true}, nil
}

type CommitStatement struct{}

func (s *CommitStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadOnlyTransaction() {
		return nil, errors.New("you're in read-only transaction. Please finish the transaction by 'CLOSE;'")
	}

	result := &Result{IsMutation: true}

	if !session.InReadWriteTransaction() {
		return result, nil
	}

	resp, err := session.CommitReadWriteTransaction(ctx)
	if err != nil {
		return nil, err
	}

	result.Timestamp = resp.CommitTs
	result.CommitStats = resp.CommitStats
	return result, nil
}

type RollbackStatement struct{}

func (s *RollbackStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadOnlyTransaction() {
		return nil, errors.New("you're in read-only transaction. Please finish the transaction by 'CLOSE;'")
	}

	result := &Result{IsMutation: true}
	if !session.InReadWriteTransaction() {
		return result, nil
	}

	if err := session.RollbackReadWriteTransaction(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

type timestampBoundType int

const (
	strong timestampBoundType = iota
	exactStaleness
	readTimestamp
)

type BeginRoStatement struct {
	TimestampBoundType timestampBoundType
	Staleness          time.Duration
	Timestamp          time.Time
	Priority           sppb.RequestOptions_Priority
	Tag                string
}

func newBeginRoStatement(input string) (*BeginRoStatement, error) {
	stmt := &BeginRoStatement{
		TimestampBoundType: strong,
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

	if matched[3] != "" {
		stmt.Tag = matched[3]
	}

	return stmt, nil
}

func (s *BeginRoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		return nil, errors.New("invalid state: You're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}
	if session.InReadOnlyTransaction() {
		// close current transaction implicitly
		if _, err := (&CloseStatement{}).Execute(ctx, session); err != nil {
			return nil, fmt.Errorf("error on close current transaction: %w", err)
		}
	}

	ts, err := session.BeginReadOnlyTransaction(ctx, s.TimestampBoundType, s.Staleness, s.Timestamp, s.Priority, s.Tag)
	if err != nil {
		return nil, err
	}

	return &Result{
		IsMutation: true,
		Timestamp:  ts,
	}, nil
}

type CloseStatement struct{}

func (s *CloseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		return nil, errors.New("You're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	result := &Result{IsMutation: true}

	if !session.InReadOnlyTransaction() {
		return result, nil
	}

	if err := session.CloseReadOnlyTransaction(); err != nil {
		return nil, err
	}

	return result, nil
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
