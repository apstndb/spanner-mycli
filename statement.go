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
	"github.com/cloudspannerecosystem/memefish"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
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
	ElapsedTime                string
	CPUTime                    string
	RowsReturned               string
	RowsScanned                string
	DeletedRowsScanned         string
	OptimizerVersion           string
	OptimizerStatisticsPackage string
}

var (
	// SQL
	selectRe = regexp.MustCompile(`(?is)^(?:WITH|CALL|@{.+|SELECT|GRAPH)\s.+$`)

	// DDL
	createDatabaseRe = regexp.MustCompile(`(?is)^CREATE\s+DATABASE\s.+$`)
	dropDatabaseRe   = regexp.MustCompile(`(?is)^DROP\s+DATABASE\s+(.+)$`)
	createRe         = regexp.MustCompile(`(?is)^CREATE\s.+$`)
	dropRe           = regexp.MustCompile(`(?is)^DROP\s.+$`)
	grantRe          = regexp.MustCompile(`(?is)^GRANT\s.+$`)
	revokeRe         = regexp.MustCompile(`(?is)^REVOKE\s.+$`)
	alterRe          = regexp.MustCompile(`(?is)^ALTER\s.+$`)
	renameRe         = regexp.MustCompile(`(?is)^RENAME\s.+$`)
	truncateTableRe  = regexp.MustCompile(`(?is)^TRUNCATE\s+TABLE\s+(.+)$`)
	analyzeRe        = regexp.MustCompile(`(?is)^ANALYZE$`)

	// DML
	dmlRe = regexp.MustCompile(`(?is)^(INSERT|UPDATE|DELETE)\s+.+$`)

	// Partitioned DML
	// In fact, INSERT is not supported in a Partitioned DML, but accept it for showing better error message.
	// https://cloud.google.com/spanner/docs/dml-partitioned#features_that_arent_supported
	pdmlRe = regexp.MustCompile(`(?is)^PARTITIONED\s+((?:INSERT|UPDATE|DELETE)\s+.+$)`)

	// Transaction
	beginRwRe  = regexp.MustCompile(`(?is)^BEGIN(?:\s+RW)?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?(?:\s+TAG\s+(.+))?$`)
	beginRoRe  = regexp.MustCompile(`(?is)^BEGIN\s+RO(?:\s+([^\s]+))?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?(?:\s+TAG\s+(.+))?$`)
	commitRe   = regexp.MustCompile(`(?is)^COMMIT$`)
	rollbackRe = regexp.MustCompile(`(?is)^ROLLBACK$`)
	closeRe    = regexp.MustCompile(`(?is)^CLOSE$`)

	// Other
	exitRe            = regexp.MustCompile(`(?is)^EXIT$`)
	useRe             = regexp.MustCompile(`(?is)^USE\s+([^\s]+)(?:\s+ROLE\s+(.+))?$`)
	showDatabasesRe   = regexp.MustCompile(`(?is)^SHOW\s+DATABASES$`)
	showCreateTableRe = regexp.MustCompile(`(?is)^SHOW\s+CREATE\s+TABLE\s+(.+)$`)
	showTablesRe      = regexp.MustCompile(`(?is)^SHOW\s+TABLES(?:\s+(.+))?$`)
	showColumnsRe     = regexp.MustCompile(`(?is)^(?:SHOW\s+COLUMNS\s+FROM)\s+(.+)$`)
	showIndexRe       = regexp.MustCompile(`(?is)^SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(.+)$`)
	explainRe         = regexp.MustCompile(`(?is)^EXPLAIN\s+(ANALYZE\s+)?(.+)$`)
	describeRe        = regexp.MustCompile(`(?is)^DESCRIBE\s+(.+)$`)
	showVariableRe    = regexp.MustCompile(`(?is)^SHOW\s+VARIABLE\s+(.+)$`)
	setRe             = regexp.MustCompile(`(?is)^SET\s+([^\s=]+)\s*=\s*(\S.*)$`)
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

func BuildStatementWithComments(stripped, raw string) (Statement, error) {
	trimmed := strings.TrimSpace(stripped)
	switch {
	case exitRe.MatchString(trimmed):
		return &ExitStatement{}, nil
	case useRe.MatchString(trimmed):
		matched := useRe.FindStringSubmatch(trimmed)
		return &UseStatement{Database: unquoteIdentifier(matched[1]), Role: unquoteIdentifier(matched[2])}, nil
	case selectRe.MatchString(trimmed):
		return &SelectStatement{Query: raw}, nil
	case createDatabaseRe.MatchString(trimmed):
		return &CreateDatabaseStatement{CreateStatement: trimmed}, nil
	case createRe.MatchString(trimmed):
		return &DdlStatement{Ddl: trimmed}, nil
	case dropDatabaseRe.MatchString(trimmed):
		matched := dropDatabaseRe.FindStringSubmatch(trimmed)
		return &DropDatabaseStatement{DatabaseId: unquoteIdentifier(matched[1])}, nil
	case dropRe.MatchString(trimmed):
		return &DdlStatement{Ddl: trimmed}, nil
	case alterRe.MatchString(trimmed):
		return &DdlStatement{Ddl: trimmed}, nil
	case renameRe.MatchString(trimmed):
		return &DdlStatement{Ddl: trimmed}, nil
	case grantRe.MatchString(trimmed):
		return &DdlStatement{Ddl: trimmed}, nil
	case revokeRe.MatchString(trimmed):
		return &DdlStatement{Ddl: trimmed}, nil
	case truncateTableRe.MatchString(trimmed):
		matched := truncateTableRe.FindStringSubmatch(trimmed)
		return &TruncateTableStatement{Table: unquoteIdentifier(matched[1])}, nil
	case analyzeRe.MatchString(trimmed):
		return &DdlStatement{Ddl: trimmed}, nil
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
		isDML := dmlRe.MatchString(matched[1])
		switch {
		case isDML:
			return &DescribeStatement{Statement: matched[1], IsDML: true}, nil
		default:
			return &DescribeStatement{Statement: matched[1]}, nil
		}
	case explainRe.MatchString(trimmed):
		matched := explainRe.FindStringSubmatch(trimmed)
		isAnalyze := matched[1] != ""
		isDML := dmlRe.MatchString(matched[2])
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
	case dmlRe.MatchString(trimmed):
		return &DmlStatement{Dml: raw}, nil
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
	case setRe.MatchString(trimmed):
		matched := setRe.FindStringSubmatch(trimmed)
		return &SetStatement{VarName: matched[1], Value: matched[2]}, nil
	case showVariablesRe.MatchString(trimmed):
		return &ShowVariablesStatement{}, nil
	}

	return nil, errors.New("invalid statement")
}

func unquoteIdentifier(input string) string {
	return strings.Trim(strings.TrimSpace(input), "`")
}

type SelectStatement struct {
	Query string
}

func (s *SelectStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt := spanner.NewStatement(s.Query)

	iter, roTxn := session.RunQueryWithStats(ctx, stmt)
	defer iter.Stop()

	rows, columnNames, err := parseQueryResult(iter)
	if err != nil {
		if session.InReadWriteTransaction() && spanner.ErrCode(err) == codes.Aborted {
			// Need to call rollback to free the acquired session in underlying google-cloud-go/spanner.
			rollback := &RollbackStatement{}
			rollback.Execute(ctx, session)
		}
		return nil, err
	}
	result := &Result{
		ColumnNames: columnNames,
		Rows:        rows,
	}
	result.ColumnTypes = iter.Metadata.GetRowType().GetFields()

	queryStats := parseQueryStats(iter.QueryStats)
	rowsReturned, err := strconv.Atoi(queryStats.RowsReturned)
	if err != nil {
		return nil, fmt.Errorf("rowsReturned is invalid: %v", err)
	}

	result.AffectedRows = rowsReturned
	result.Stats = queryStats

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	if roTxn != nil {
		result.Timestamp, _ = roTxn.Timestamp()
	}

	return result, nil
}

// extractColumnNames extract column names from ResultSetMetadata.RowType.Fields.
func extractColumnNames(fields []*sppb.StructType_Field) []string {
	var names []string
	for _, field := range fields {
		names = append(names, field.GetName())
	}
	return names
}

// parseQueryResult parses rows and columnNames from spanner.RowIterator.
// A caller is responsible for calling iterator.Stop().
func parseQueryResult(iter *spanner.RowIterator) ([]Row, []string, error) {
	var rows []Row
	for {
		row, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		columns, err := DecodeRow(row)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, Row{
			Columns: columns,
		})
	}
	return rows, extractColumnNames(iter.Metadata.GetRowType().GetFields()), nil
}

// parseQueryStats parses spanner.RowIterator.QueryStats.
func parseQueryStats(stats map[string]interface{}) QueryStats {
	var queryStats QueryStats

	if v, ok := stats["elapsed_time"]; ok {
		if elapsed, ok := v.(string); ok {
			queryStats.ElapsedTime = elapsed
		}
	}

	if v, ok := stats["rows_returned"]; ok {
		if returned, ok := v.(string); ok {
			queryStats.RowsReturned = returned
		}
	}

	if v, ok := stats["rows_scanned"]; ok {
		if scanned, ok := v.(string); ok {
			queryStats.RowsScanned = scanned
		}
	}

	if v, ok := stats["deleted_rows_scanned"]; ok {
		if deletedRowsScanned, ok := v.(string); ok {
			queryStats.DeletedRowsScanned = deletedRowsScanned
		}
	}

	if v, ok := stats["cpu_time"]; ok {
		if cpu, ok := v.(string); ok {
			queryStats.CPUTime = cpu
		}
	}

	if v, ok := stats["optimizer_version"]; ok {
		if version, ok := v.(string); ok {
			queryStats.OptimizerVersion = version
		}
	}

	if v, ok := stats["optimizer_statistics_package"]; ok {
		if pkg, ok := v.(string); ok {
			queryStats.OptimizerStatisticsPackage = pkg
		}
	}

	return queryStats
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
		Database: fmt.Sprintf("projects/%s/instances/%s/databases/%s", session.projectId, session.instanceId, s.DatabaseId),
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

func executeDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	logParseStatements(ddls)
	op, err := session.adminClient.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
		Database:   session.DatabasePath(),
		Statements: ddls,
	})
	if err != nil {
		return nil, err
	}
	if err := op.Wait(ctx); err != nil {
		return nil, err
	}

	return &Result{IsMutation: true}, nil
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
		Rows:          []Row{{Columns: row}},
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

	var rows []Row
	for k, v := range merged {
		rows = append(rows, Row{sliceOf(k, v)})
	}

	slices.SortFunc(rows, func(a, b Row) int {
		switch {
		case a.Columns[0] == b.Columns[0]:
			return 0
		case a.Columns[0] < b.Columns[0]:
			return -1
		default:
			return 1
		}
	})

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

type ShowDatabasesStatement struct {
}

func (s *ShowDatabasesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{ColumnNames: []string{"Database"}}

	dbIter := session.adminClient.ListDatabases(ctx, &adminpb.ListDatabasesRequest{
		Parent: session.InstancePath(),
	})

	for {
		database, err := dbIter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}

		re := regexp.MustCompile(`projects/[^/]+/instances/[^/]+/databases/(.+)`)
		matched := re.FindStringSubmatch(database.GetName())
		dbname := matched[1]
		resultRow := Row{
			Columns: []string{dbname},
		}
		result.Rows = append(result.Rows, resultRow)
	}

	result.AffectedRows = len(result.Rows)

	return result, nil
}

type ShowCreateTableStatement struct {
	Schema string
	Table  string
}

func (s *ShowCreateTableStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{ColumnNames: []string{"Table", "Create Table"}}

	ddlResponse, err := session.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}
	for _, stmt := range ddlResponse.Statements {
		if isCreateTableDDL(stmt, s.Schema, s.Table) {
			var fqn string
			if s.Schema == "" {
				fqn = s.Table
			} else {
				fqn = fmt.Sprintf("%s.%s", s.Schema, s.Table)
			}

			resultRow := Row{
				Columns: []string{fqn, stmt},
			}
			result.Rows = append(result.Rows, resultRow)
			break
		}
	}
	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	}

	result.AffectedRows = len(result.Rows)

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
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can not be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, errors.New(`"SHOW TABLES" can not be used in a read-write transaction`)
	}

	alias := fmt.Sprintf("Tables_in_%s", session.databaseId)
	stmt := spanner.NewStatement(fmt.Sprintf("SELECT t.TABLE_NAME AS `%s` FROM INFORMATION_SCHEMA.TABLES AS t WHERE t.TABLE_CATALOG = '' and t.TABLE_SCHEMA = @schema", alias))
	stmt.Params["schema"] = s.Schema

	iter, _ := session.RunQuery(ctx, stmt)
	defer iter.Stop()

	rows, columnNames, err := parseQueryResult(iter)
	if err != nil {
		return nil, err
	}

	return &Result{
		ColumnNames:  columnNames,
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}

type ExplainStatement struct {
	Explain string
	IsDML   bool
}

// Execute processes `EXPLAIN` statement for queries and DMLs.
func (s *ExplainStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	queryPlan, timestamp, _, err := runAnalyzeQuery(ctx, session, spanner.NewStatement(s.Explain), s.IsDML)
	if err != nil {
		return nil, err
	}

	if queryPlan == nil {
		return nil, errors.New("EXPLAIN statement is not supported for Cloud Spanner Emulator.")
	}

	rows, predicates, err := processPlanWithoutStats(queryPlan)
	if err != nil {
		return nil, err
	}

	result := &Result{
		ColumnNames:  explainColumnNames,
		AffectedRows: len(rows),
		Rows:         rows,
		Timestamp:    timestamp,
		Predicates:   predicates,
	}

	return result, nil
}

type DescribeStatement struct {
	Statement string
	IsDML     bool
}

// Execute processes `DESCRIBE` statement for queries and DMLs.
func (s *DescribeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	_, timestamp, metadata, err := runAnalyzeQuery(ctx, session, spanner.NewStatement(s.Statement), s.IsDML)
	if err != nil {
		return nil, err
	}

	var rows []Row
	for _, field := range metadata.GetRowType().GetFields() {
		rows = append(rows, Row{Columns: []string{field.GetName(), formatTypeVerbose(field.GetType())}})
	}

	result := &Result{
		AffectedRows: len(rows),
		ColumnNames:  describeColumnNames,
		Timestamp:    timestamp,
		Rows:         rows,
	}

	return result, nil
}

func runAnalyzeQuery(ctx context.Context, session *Session, stmt spanner.Statement, isDML bool) (queryPlan *sppb.QueryPlan, commitTimestamp time.Time, metadata *sppb.ResultSetMetadata, err error) {
	if !isDML {
		queryPlan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return queryPlan, time.Time{}, metadata, err
	}

	_, timestamp, queryPlan, metadata, err := runInNewOrExistRwTxForExplain(ctx, session, func() (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		plan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return 0, plan, metadata, err
	})
	return queryPlan, timestamp, metadata, err
}

type ExplainAnalyzeStatement struct {
	Query string
}

func (s *ExplainAnalyzeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt := spanner.NewStatement(s.Query)

	iter, roTxn := session.RunQueryWithStats(ctx, stmt)

	// consume iter
	err := iter.Do(func(*spanner.Row) error {
		return nil
	})
	if err != nil {
		return nil, err
	}

	queryStats := parseQueryStats(iter.QueryStats)
	rowsReturned, err := strconv.Atoi(queryStats.RowsReturned)
	if err != nil {
		return nil, fmt.Errorf("rowsReturned is invalid: %v", err)
	}

	// Cloud Spanner Emulator doesn't set query plan nodes to the result.
	// See: https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/77188b228e7757cd56ecffb5bc3ee85dce5d6ae1/frontend/handlers/queries.cc#L224-L230
	if iter.QueryPlan == nil {
		return nil, errors.New("EXPLAIN ANALYZE statement is not supported for Cloud Spanner Emulator.")
	}

	rows, predicates, err := processPlanWithStats(iter.QueryPlan)
	if err != nil {
		return nil, err
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	var timestamp time.Time
	if roTxn != nil {
		timestamp, _ = roTxn.Timestamp()
	}

	result := &Result{
		ColumnNames:  explainAnalyzeColumnNames,
		ForceVerbose: true,
		AffectedRows: rowsReturned,
		Stats:        queryStats,
		Timestamp:    timestamp,
		Rows:         rows,
		Predicates:   predicates,
	}
	return result, nil
}

func processPlanWithStats(plan *sppb.QueryPlan) (rows []Row, predicates []string, err error) {
	return processPlanImpl(plan, true)
}

func processPlanWithoutStats(plan *sppb.QueryPlan) (rows []Row, predicates []string, err error) {
	return processPlanImpl(plan, false)
}

func processPlanImpl(plan *sppb.QueryPlan, withStats bool) (rows []Row, predicates []string, err error) {
	planNodes := plan.GetPlanNodes()
	maxWidthOfNodeID := len(fmt.Sprint(getMaxRelationalNodeID(plan)))
	widthOfNodeIDWithIndicator := maxWidthOfNodeID + 1

	tree := BuildQueryPlanTree(plan, 0)

	treeRows, err := tree.RenderTreeWithStats(planNodes)
	if err != nil {
		return nil, nil, err
	}
	for _, row := range treeRows {
		var formattedID string
		if len(row.Predicates) > 0 {
			formattedID = fmt.Sprintf("%*s", widthOfNodeIDWithIndicator, "*"+fmt.Sprint(row.ID))
		} else {
			formattedID = fmt.Sprintf("%*d", widthOfNodeIDWithIndicator, row.ID)
		}
		if withStats {
			rows = append(rows, Row{[]string{formattedID, row.Text, row.RowsTotal, row.Execution, row.LatencyTotal}})
		} else {
			rows = append(rows, Row{[]string{formattedID, row.Text}})
		}
		for i, predicate := range row.Predicates {
			var prefix string
			if i == 0 {
				prefix = fmt.Sprintf("%*d:", maxWidthOfNodeID, row.ID)
			} else {
				prefix = strings.Repeat(" ", maxWidthOfNodeID+1)
			}
			predicates = append(predicates, fmt.Sprintf("%s %s", prefix, predicate))
		}
	}
	return rows, predicates, nil
}

type ShowColumnsStatement struct {
	Schema string
	Table  string
}

func (s *ShowColumnsStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can not be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, errors.New(`"SHOW COLUMNS" can not be used in a read-write transaction`)
	}

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
		Params: map[string]interface{}{"table_name": s.Table, "table_schema": s.Schema}}

	iter, _ := session.RunQuery(ctx, stmt)
	defer iter.Stop()

	rows, columnNames, err := parseQueryResult(iter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	}

	return &Result{
		ColumnNames:  columnNames,
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
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
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can not be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, errors.New(`"SHOW INDEX" can not be used in a read-write transaction`)
	}

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
		Params: map[string]interface{}{"table_name": s.Table, "table_schema": s.Schema}}

	iter, _ := session.RunQuery(ctx, stmt)
	defer iter.Stop()

	rows, columnNames, err := parseQueryResult(iter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("table %q doesn't exist in schema %q", s.Table, s.Schema)
	}

	return &Result{
		ColumnNames:  columnNames,
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
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
	stmt := spanner.NewStatement(s.Dml)

	result := &Result{IsMutation: true}

	var rows []Row
	var columnNames []string
	var numRows int64
	var metadata *sppb.ResultSetMetadata
	var err error
	if session.InReadWriteTransaction() {
		rows, columnNames, numRows, metadata, err = session.RunUpdate(ctx, stmt, false)
		if err != nil {
			// Need to call rollback to free the acquired session in underlying google-cloud-go/spanner.
			rollback := &RollbackStatement{}
			rollback.Execute(ctx, session)
			return nil, fmt.Errorf("transaction was aborted: %v", err)
		}
	} else {
		// Start implicit transaction.
		begin := BeginRwStatement{}
		if _, err = begin.Execute(ctx, session); err != nil {
			return nil, err
		}

		rows, columnNames, numRows, metadata, err = session.RunUpdate(ctx, stmt, false)
		if err != nil {
			// once error has happened, escape from implicit transaction
			rollback := &RollbackStatement{}
			rollback.Execute(ctx, session)
			return nil, err
		}

		commit := CommitStatement{}
		txnResult, err := commit.Execute(ctx, session)
		if err != nil {
			return nil, err
		}
		result.Timestamp = txnResult.Timestamp
		result.CommitStats = txnResult.CommitStats
	}

	result.ColumnTypes = metadata.GetRowType().GetFields()
	result.Rows = rows
	result.ColumnNames = columnNames
	result.AffectedRows = int(numRows)

	return result, nil
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

	stmt := spanner.NewStatement(s.Dml)
	ctx, cancel := context.WithTimeout(ctx, pdmlTimeout)
	defer cancel()

	count, err := session.client.PartitionedUpdate(ctx, stmt)
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
	stmt := spanner.NewStatement(s.Dml)

	affectedRows, timestamp, queryPlan, _, err := runInNewOrExistRwTxForExplain(ctx, session, func() (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		iter, _ := session.RunQueryWithStats(ctx, stmt)
		defer iter.Stop()
		err := iter.Do(func(r *spanner.Row) error { return nil })
		if err != nil {
			return 0, nil, nil, err
		}
		return iter.RowCount, iter.QueryPlan, iter.Metadata, nil
	})
	if err != nil {
		return nil, err
	}

	rows, predicates, err := processPlanWithStats(queryPlan)
	if err != nil {
		return nil, err
	}
	result := &Result{
		IsMutation:   true,
		ColumnNames:  explainAnalyzeColumnNames,
		ForceVerbose: true,
		AffectedRows: int(affectedRows),
		Rows:         rows,
		Predicates:   predicates,
		Timestamp:    timestamp,
	}

	return result, nil
}

// runInNewOrExistRwTxForExplain is a helper function for runAnalyzeQuery and ExplainAnalyzeDmlStatement.
// It execute a function in the current RW transaction or an implicit RW transaction.
func runInNewOrExistRwTxForExplain(ctx context.Context, session *Session, f func() (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error)) (affected int64, ts time.Time, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
	if session.InReadWriteTransaction() {
		affected, plan, metadata, err := f()
		if err != nil {
			// Need to call rollback to free the acquired session in underlying google-cloud-go/spanner.
			rollback := &RollbackStatement{}
			rollback.Execute(ctx, session)
			return 0, time.Time{}, nil, nil, fmt.Errorf("transaction was aborted: %v", err)
		}
		return affected, time.Time{}, plan, metadata, nil
	} else {
		// Start implicit transaction.
		begin := BeginRwStatement{}
		if _, err := begin.Execute(ctx, session); err != nil {
			return 0, time.Time{}, nil, nil, err
		}

		affected, plan, metadata, err := f()
		if err != nil {
			// once error has happened, escape from implicit transaction
			rollback := &RollbackStatement{}
			rollback.Execute(ctx, session)
			return 0, time.Time{}, nil, nil, err
		}

		commit := CommitStatement{}
		txnResult, err := commit.Execute(ctx, session)
		if err != nil {
			return 0, time.Time{}, nil, nil, err
		}
		return affected, txnResult.Timestamp, plan, metadata, nil
	}
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
	result := &Result{IsMutation: true}
	if session.InReadOnlyTransaction() {
		return nil, errors.New("you're in read-only transaction. Please finish the transaction by 'CLOSE;'")
	}
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
	result := &Result{IsMutation: true}
	if session.InReadOnlyTransaction() {
		return nil, errors.New("you're in read-only transaction. Please finish the transaction by 'CLOSE;'")
	}
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
		return nil, errors.New("You're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}
	if session.InReadOnlyTransaction() {
		// close current transaction implicitly
		(&CloseStatement{}).Execute(ctx, session)
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
