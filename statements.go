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
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/memebridge"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/cloudspannerecosystem/memefish/ast"
	spancql "github.com/googleapis/go-spanner-cassandra/cassandra/gocql"
	"github.com/samber/lo"
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

// Schema related statements are defined in statements_schema.go

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

	return executePDML(ctx, session, fmt.Sprintf("DELETE FROM `%s` WHERE true", s.Table))
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
	DMLs []string
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

func (stmt *CQLStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.cqlCluster == nil {
		cluster := spancql.NewCluster(&spancql.Options{
			DatabaseUri: session.DatabasePath(),
		})
		if cluster == nil {
			return nil, fmt.Errorf("failed to create cluster")
		}

		session.cqlCluster = cluster

		// You can still configure your cluster as usual after connecting to your
		// spanner database
		cluster.Timeout = 5 * time.Second
		// cluster.Keyspace = "your_db_name"
	}

	if session.cqlSession == nil {
		s, err := session.cqlCluster.CreateSession()
		if err != nil {
			return nil, err
		}
		session.cqlSession = s
	}

	s := session.cqlSession

	q := s.Query(stmt.CQL)
	if err := q.Exec(); err != nil {
		return nil, err
	}

	it := q.Iter()
	defer it.Close()

	columns := it.Columns()
	var columnNames []string
	for _, column := range columns {
		columnNames = append(columnNames, column.Name)
	}
	var rows []Row
	for {
		m := make(map[string]interface{})
		if !it.MapScan(m) {
			break
		}
		var rowStrs []string
		for _, name := range columnNames {
			rowStrs = append(rowStrs, fmt.Sprint(m[name]))
		}
		rows = append(rows, rowStrs)
	}

	return &Result{ColumnNames: columnNames, Rows: rows}, nil
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
	genParams, err := generateParams(params, includeType)
	if err != nil {
		return spanner.Statement{}, err
	}
	return spanner.Statement{
		SQL:    sql,
		Params: genParams,
	}, nil
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
