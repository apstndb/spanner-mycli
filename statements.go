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
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/lox"
	"github.com/apstndb/memebridge"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/gocql/gocql"
	spancql "github.com/googleapis/go-spanner-cassandra/cassandra/gocql"
	"github.com/samber/lo"
	"go.uber.org/zap/zapcore"
)

var transactionRe = regexp.MustCompile(`(?is)^(?:(READ\s+ONLY)|(READ\s+WRITE))$`)

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
	switch {
	case qm != nil && *qm == sppb.ExecuteSqlRequest_PLAN:
		return executeExplain(ctx, session, s.Query, false)
	case qm != nil && *qm == sppb.ExecuteSqlRequest_PROFILE:
		return executeExplainAnalyze(ctx, session, s.Query)
	default:
		if !session.InTransaction() && session.systemVariables.AutoPartitionMode {
			return runPartitionedQuery(ctx, session, s.Query)
		}
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

// UseStatement is actually implemented in cli.go because it needs to replace Session pointer in Cli.
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

type ShowDatabasesStatement struct{}

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

// Split Points

// Schema related statements are defined in statements_schema.go

// Operations

type ShowSchemaUpdateOperations struct{}

func (s *ShowSchemaUpdateOperations) Execute(ctx context.Context, session *Session) (*Result, error) {
	num := 0

	var rows []Row
	for op, err := range session.adminClient.ListOperations(ctx, &longrunningpb.ListOperationsRequest{
		Name: session.DatabasePath() + "/operations",
	}).All() {
		if err != nil {
			return nil, err
		}

		switch op.GetMetadata().GetTypeUrl() {
		case "type.googleapis.com/google.spanner.admin.database.v1.UpdateDatabaseDdlMetadata":
			// Only GetOperation contains progresses.
			if !op.GetDone() {
				op, err = session.adminClient.GetOperation(ctx, &longrunningpb.GetOperationRequest{
					Name: op.GetName(),
				})
				if err != nil {
					return nil, err
				}
			}

			var md databasepb.UpdateDatabaseDdlMetadata
			if err := op.GetMetadata().UnmarshalTo(&md); err != nil {
				return nil, err
			}

			for i := range md.GetStatements() {
				rows = append(rows, toRow(lo.Ternary(i == 0, lo.LastOrEmpty(strings.Split(op.GetName(), "/")), ""),
					md.GetStatements()[i]+";",
					lox.IfOrEmpty(i == 0, strconv.FormatBool(op.GetDone())),
					lox.IfOrEmptyF(len(md.GetProgress()) > i, func() string {
						return fmt.Sprint(md.GetProgress()[i].GetProgressPercent())
					}),
					lox.IfOrEmptyF(len(md.GetCommitTimestamps()) > i, func() string {
						return md.GetCommitTimestamps()[i].AsTime().Format(time.RFC3339Nano)
					}),
					op.GetError().GetMessage()))
			}
			num++
		}
	}
	return &Result{
		ColumnNames:  []string{"OPERATION_ID", "STATEMENTS", "DONE", "PROGRESS", "COMMIT_TIMESTAMP", "ERROR"},
		Rows:         rows,
		AffectedRows: num,
	}, nil
}

// Protocol Buffers related statements are defined in statements_proto.go

// TRUNCATE TABLE

type TruncateTableStatement struct {
	Schema string
	Table  string
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

	var schemaPart string
	if s.Schema != "" {
		schemaPart = fmt.Sprintf("`%s`.", s.Schema)
	}

	return executePDML(ctx, session, fmt.Sprintf("DELETE FROM %s`%s` WHERE true", schemaPart, s.Table))
}

// EXPLAIN, EXPLAIN ANALYZE and DESCRIBE related statements are defined in statements_explain.go

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

	return executePDML(ctx, session, s.Dml)
}

// Partitioned Query related statements are defined in statements_partitioned_query.go

// Transaction related statements are defined in statements_transaction.go

// Batching

type batchMode int

const (
	batchModeDDL batchMode = iota + 1
	batchModeDML
)

type BulkDdlStatement struct {
	Ddls []string
}

func (BulkDdlStatement) IsMutationStatement() {}

func (s *BulkDdlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDdlStatements(ctx, session, s.Ddls)
}

type BatchDMLStatement struct {
	DMLs []spanner.Statement
}

func (BatchDMLStatement) IsMutationStatement() {}

func (s *BatchDMLStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeBatchDML(ctx, session, s.DMLs)
}

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
	return runBatch(ctx, session)
}

func runBatch(ctx context.Context, session *Session) (*Result, error) {
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

// System Variable related statements are defined in statements_system_variable.go

// Query Parameter related statements are defined in statements_params.go

// Mutation related statements are defined in statements_mutations.go

// Query Profiles related statements are defined in statements_query_profile

// LLM related statements are defined in statements_llm.go

// Cassandra interface
type CQLStatement struct {
	CQL string
}

func (cs *CQLStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	// lazy initialize gocql.ClusterConfig
	if session.cqlCluster == nil {
		cluster := spancql.NewCluster(&spancql.Options{
			LogLevel:    zapcore.WarnLevel.String(),
			DatabaseUri: session.DatabasePath(),
		})
		if cluster == nil {
			return nil, fmt.Errorf("failed to create cluster")
		}

		session.cqlCluster = cluster

		// You can still configure your cluster as usual after connecting to your
		// spanner database
		cluster.Timeout = 5 * time.Second
	}
	// lazy initialize gocql.Session
	if session.cqlSession == nil {
		s, err := session.cqlCluster.CreateSession()
		if err != nil {
			return nil, err
		}
		session.cqlSession = s
	}

	s := session.cqlSession

	q := s.Query(cs.CQL)
	if err := q.Exec(); err != nil {
		return nil, err
	}

	it := s.Query(cs.CQL).WithContext(ctx).Iter()
	defer it.Close()

	var headers []string
	for _, col := range it.Columns() {
		headers = append(headers, col.Name+"\n"+formatCassandraTypeName(col.TypeInfo))
	}

	var rows []Row
	for {
		rd, err := it.RowData()
		if err != nil {
			return nil, err
		}

		if !it.Scan(rd.Values...) {
			break
		}

		var row Row
		for _, value := range rd.Values {
			row = append(row, fmt.Sprint(reflect.Indirect(reflect.ValueOf(value)).Interface()))
		}
		rows = append(rows, row)
	}

	return &Result{ColumnNames: headers, Rows: rows, AffectedRows: len(rows)}, nil
}

func formatCassandraTypeName(typeInfo gocql.TypeInfo) string {
	if ct, ok := typeInfo.(gocql.CollectionType); ok {
		return fmt.Sprintf("%v<%v%v>",
			ct.Type(),
			lo.Ternary(ct.Key != nil, fmt.Sprint(ct.Key)+", ", ""),
			ct.Elem)
	} else {
		return fmt.Sprint(typeInfo)
	}
}

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

// Helper function for statements.go.

func newStatement(sql string, params map[string]ast.Node, includeType bool) (spanner.Statement, error) {
	usedParamNames, err := usedQueryParameterNames(sql)
	if err != nil {
		return spanner.Statement{}, err
	}

	filteredParams := make(map[string]ast.Node)
	for _, name := range usedParamNames {
		if _, ok := params[name]; ok {
			filteredParams[name] = params[name]
		}
	}

	genParams, err := generateParams(filteredParams, includeType)
	if err != nil {
		return spanner.Statement{}, err
	}
	return spanner.Statement{
		SQL:    sql,
		Params: genParams,
	}, nil
}

func usedQueryParameterNames(s string) ([]string, error) {
	set := make(map[string]struct{})
	for tok, err := range gsqlutils.NewLexerSeq("", s) {
		if err != nil {
			return nil, err
		}

		if tok.Kind == token.TokenParam {
			set[tok.AsString] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(set)), nil
}

func extractSchemaAndName(s string) (string, string) {
	schema, name, found := strings.Cut(s, ".")
	if !found {
		return "", unquoteIdentifier(s)
	}
	return unquoteIdentifier(schema), unquoteIdentifier(name)
}

func generateParams(paramsNodeMap map[string]ast.Node, includeType bool) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range paramsNodeMap {
		switch v := v.(type) {
		case ast.Type:
			if !includeType {
				continue
			}

			typ, err := memebridge.MemefishTypeToSpannerpbType(v)
			if err != nil {
				return nil, err
			}
			result[k] = gcvctor.TypedNull(typ)
		case ast.Expr:
			expr, err := memebridge.MemefishExprToGCV(v)
			if err != nil {
				return nil, err
			}
			result[k] = expr
		}
	}
	return result, nil
}
