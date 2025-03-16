// Copyright 2020 Google LLC
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
package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/lox"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/k0kubun/pp/v3"
	"github.com/mattn/go-runewidth"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"
	scxiter "spheric.cloud/xiter"
)

var transactionRe = regexp.MustCompile(`(?is)^(?:(READ\s+ONLY)|(READ\s+WRITE))$$`)

// Order and sections should be matched in client_side_statement_def.go

// Native

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

type DdlStatement struct {
	Ddl string
}

func (DdlStatement) isMutationStatement() {}

func (s *DdlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return bufferOrExecuteDdlStatements(ctx, session, []string{s.Ddl})
}

type CreateDatabaseStatement struct {
	CreateStatement string
}

func (CreateDatabaseStatement) IsMutationStatement() {}

func (s *CreateDatabaseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	op, err := session.adminClient.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
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

// Database

// Implementation of UseStatement is in cli.go because it needs to replace Session pointer in Cli.
type UseStatement struct {
	Database string
	Role     string
	NopStatement
}

type DropDatabaseStatement struct {
	DatabaseId string
}

func (DropDatabaseStatement) isMutationStatement() {}

func (s *DropDatabaseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if err := session.adminClient.DropDatabase(ctx, &databasepb.DropDatabaseRequest{
		Database: databasePath(session.systemVariables.Project, session.systemVariables.Instance, session.systemVariables.Database),
	}); err != nil {
		return nil, err
	}

	return &Result{
		IsMutation: true,
	}, nil
}

type ShowDatabasesStatement struct {
}

var extractDatabaseRe = regexp.MustCompile(`projects/[^/]+/instances/[^/]+/databases/(.+)`)

func (s *ShowDatabasesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	dbIter := session.adminClient.ListDatabases(ctx, &databasepb.ListDatabasesRequest{
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

// Schema

type ShowCreateStatement struct {
	ObjectType string
	Schema     string
	Name       string
}

func (s *ShowCreateStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	ddlResponse, err := session.adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	var rows []Row
	for _, stmt := range ddlResponse.Statements {
		if isCreateDDL(stmt, s.ObjectType, s.Schema, s.Name) {
			fqn := lox.IfOrEmpty(s.Schema != "", s.Schema+".") + s.Name
			rows = append(rows, toRow(fqn, stmt))
			break
		}
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s %q doesn't exist in schema %q", s.ObjectType, s.Name, s.Schema)
	}

	result := &Result{
		ColumnNames:  []string{"Name", "DDL"},
		Rows:         rows,
		AffectedRows: len(rows),
	}

	return result, nil
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

type ShowDdlsStatement struct{}

func (s *ShowDdlsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	resp, err := session.adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		KeepVariables: true,
		// intentionally empty column name to make TAB format valid DDL
		ColumnNames: sliceOf(""),
		Rows: sliceOf(toRow(stringsiter.Collect(xiter.Map(
			func(s string) string { return s + ";\n" },
			slices.Values(resp.GetStatements()))))),
	}, nil
}

// Protocol Buffers related statements are defined in statements_proto.go

// TRUNCATE TABLE

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

// EXPLAIN & EXPLAIN ANALYZE related statements are defined in statements_explain.go

// DESCRIBE

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

// Partitioned DML

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

// Partitioned Query related statements are defined in statements_partitioned_query.go

// Transaction related statements are defined in statements_transaction.go

// Batching

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

// System Variable

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

// Query Parameter

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

// Mutation related statements are defined in statements_mutations.go

// Query Profiles

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

// LLM related statements are defined in statements_llm.go

// CLI control

type HelpStatement struct{}

func (s *HelpStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	var rows []Row
	for _, stmt := range clientSideStatementDefs {
		for _, desc := range stmt.Descriptions {
			rows = append(rows, toRow(desc.Usage, desc.Syntax+";"))
		}
	}
	return &Result{
		ColumnNames:   sliceOf("Usage", "Syntax"),
		Rows:          rows,
		AffectedRows:  len(rows),
		KeepVariables: true,
	}, nil
}

type ExitStatement struct {
	NopStatement
}

type NopStatement struct{}

func (s *NopStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	// do nothing
	return &Result{}, nil
}

// end of statements

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

// Helper function for statements.go.

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

func isCreateDDL(ddl string, objectType string, schema string, table string) bool {
	objectType = strings.ReplaceAll(objectType, " ", `\s+`)
	table = regexp.QuoteMeta(table)

	re := fmt.Sprintf("(?i)^CREATE (?:(?:NULL_FILTERED|UNIQUE) )?(?:OR REPLACE )?%s ", objectType)

	if schema != "" {
		re += fmt.Sprintf("(%[1]s|`%[1]s`)", schema)
		re += `\.`
	}

	re += fmt.Sprintf("(%[1]s|`%[1]s`)", table)
	re += `(?:\s+[^.]|$)`

	return regexp.MustCompile(re).MatchString(ddl)
}

func extractSchemaAndName(s string) (string, string) {
	schema, name, found := strings.Cut(s, ".")
	if !found {
		return "", unquoteIdentifier(s)
	}
	return unquoteIdentifier(schema), unquoteIdentifier(name)
}
