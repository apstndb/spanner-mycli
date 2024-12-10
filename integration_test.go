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

//go:build !skip_slow_test

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"

	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"google.golang.org/protobuf/testing/protocmp"

	"cloud.google.com/go/spanner"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"

	// Use dot import to simplify tests
	. "github.com/apstndb/spantype/testutil"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
	"spheric.cloud/xiter"
)

const (
	testInstanceId = "fake-instance"
	testDatabaseId = "fake-database"
)

var (
	tableIdCounter uint32
)

type testTableSchema struct {
	Id     int64 `spanner:"id"`
	Active bool  `spanner:"active"`
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func initialize(tb testing.TB) (container *gcloud.GCloudContainer, teardown func()) {
	tb.Helper()
	ctx := context.Background()
	spannerContainer, err := gcloud.RunSpanner(ctx, defaultEmulatorImage,
		testcontainers.WithLogger(testcontainers.TestLogger(tb)))
	if err != nil {
		tb.Fatal(err)
	}

	err = setupInstance(ctx, spannerContainer, testInstanceId)
	if err != nil {
		testcontainers.CleanupContainer(tb, spannerContainer)
		tb.Fatal(err)
	}

	err = setupDatabase(ctx, spannerContainer, testInstanceId, testDatabaseId, nil, nil)
	if err != nil {
		testcontainers.CleanupContainer(tb, spannerContainer)
		tb.Fatal(err)
	}

	return spannerContainer, func() {
		if err := spannerContainer.Terminate(ctx); err != nil {
			tb.Log(err)
		}
	}
}

func defaultClientOptions(spannerContainer *gcloud.GCloudContainer) []option.ClientOption {
	opts := []option.ClientOption{
		option.WithEndpoint(spannerContainer.URI),
		option.WithoutAuthentication(),
		internaloption.SkipDialSettingsValidation(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials()))}
	return opts
}

func setupDatabase(
	ctx context.Context,
	spannerContainer *gcloud.GCloudContainer,
	instanceID, databaseID string,
	ddls []string,
	dmls []string,
) error {
	projectID := spannerContainer.Settings.ProjectID
	opts := defaultClientOptions(spannerContainer)

	dbCli, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed on new database admin client: %w", err)
	}

	createDatabaseOp, err := dbCli.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          instancePath(projectID, instanceID),
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%v`", databaseID),
		ExtraStatements: ddls,
	})
	if err != nil {
		return fmt.Errorf("failed on create database: %w", err)
	}

	_, err = createDatabaseOp.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed on waiting create database: %w", err)
	}

	if len(dmls) == 0 {
		return nil
	}

	cli, err := spanner.NewClientWithConfig(ctx, databasePath(projectID, instanceID, databaseID), spanner.ClientConfig{DisableNativeMetrics: true}, opts...)
	if err != nil {
		return fmt.Errorf("failed on new client for DMLs: %w", err)
	}
	defer cli.Close()

	_, err = cli.ReadWriteTransaction(
		ctx,
		func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
			_, err := tx.BatchUpdate(
				ctx,
				slices.Collect(xiter.Map(slices.Values(dmls), spanner.NewStatement)),
			)
			return err
		},
	)
	if err != nil {
		return fmt.Errorf("failed on batch update: %w", err)
	}
	return nil
}

func setupInstance(
	ctx context.Context,
	spannerContainer *gcloud.GCloudContainer,
	instanceID string,
) error {
	instanceClient, err := instance.NewInstanceAdminClient(
		ctx,
		defaultClientOptions(spannerContainer)...)
	if err != nil {
		return fmt.Errorf("failed on new instance admin client: %w", err)
	}
	defer instanceClient.Close()

	createInstance, err := instanceClient.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     projectPath(spannerContainer.Settings.ProjectID),
		InstanceId: instanceID,
		Instance: &instancepb.Instance{
			Name:            instancePath(spannerContainer.Settings.ProjectID, instanceID),
			Config:          "regional-asia-northeast1",
			DisplayName:     "fake",
			ProcessingUnits: 100,
		},
	})
	if err != nil {
		return fmt.Errorf("failed on create instance: %w", err)
	}

	_, err = createInstance.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed on waiting create instance: %w", err)
	}
	return nil
}

func generateUniqueTableId() string {
	count := atomic.AddUint32(&tableIdCounter, 1)
	return fmt.Sprintf("spanner_cli_test_%d_%d", time.Now().UnixNano(), count)
}

func setupSession(t *testing.T, ctx context.Context, spannerContainer *gcloud.GCloudContainer, teardownDDLs []string) (*Session, func()) {
	options := defaultClientOptions(spannerContainer)
	session, err := NewSession(ctx, &systemVariables{
		Project:     spannerContainer.Settings.ProjectID,
		Instance:    testInstanceId,
		Database:    testDatabaseId,
		RPCPriority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}, options...)
	if err != nil {
		t.Fatalf("failed to create test session: err=%s", err)
	}

	dbPath := databasePath(spannerContainer.Settings.ProjectID, testInstanceId, testDatabaseId)

	tearDown := func() {
		op, err := session.adminClient.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
			Database:   dbPath,
			Statements: teardownDDLs,
		})
		if err != nil {
			t.Fatalf("failed to drop table: err=%s", err)
		}
		if err := op.Wait(ctx); err != nil {
			t.Fatalf("failed to drop table: err=%s", err)
		}
	}
	return session, tearDown
}

var testTableRowType = sliceOf(
	NameCodeToStructTypeField("id", sppb.TypeCode_INT64),
	NameCodeToStructTypeField("active", sppb.TypeCode_BOOL),
)

func setup(t *testing.T, ctx context.Context, spannerContainer *gcloud.GCloudContainer, dmls []string) (*Session, string, func()) {
	options := defaultClientOptions(spannerContainer)
	session, err := NewSession(ctx, &systemVariables{
		Project:     spannerContainer.Settings.ProjectID,
		Instance:    testInstanceId,
		Database:    testDatabaseId,
		RPCPriority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}, options...)
	if err != nil {
		t.Fatalf("failed to create test session: err=%s", err)
	}

	dbPath := databasePath(spannerContainer.Settings.ProjectID, testInstanceId, testDatabaseId)

	tableId := generateUniqueTableId()
	tableSchema := fmt.Sprintf(`
	CREATE TABLE %s (
	  id INT64 NOT NULL,
	  active BOOL NOT NULL
	) PRIMARY KEY (id)
	`, tableId)

	op, err := session.adminClient.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
		Database:   dbPath,
		Statements: []string{tableSchema},
	})
	if err != nil {
		t.Fatalf("failed to create table: err=%s", err)
	}
	if err := op.Wait(ctx); err != nil {
		t.Fatalf("failed to create table: err=%s", err)
	}

	for _, dml := range dmls {
		dml = strings.Replace(dml, "[[TABLE]]", tableId, -1)
		stmt := spanner.NewStatement(dml)
		_, err := session.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			_, err = txn.Update(ctx, stmt)
			if err != nil {
				t.Fatalf("failed to apply DML: dml=%s, err=%s", dml, err)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to apply DML: dml=%s, err=%s", dml, err)
		}
	}

	tearDown := func() {
		op, err = session.adminClient.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
			Database:   dbPath,
			Statements: []string{fmt.Sprintf("DROP TABLE %s", tableId)},
		})
		if err != nil {
			t.Fatalf("failed to drop table: err=%s", err)
		}
		if err := op.Wait(ctx); err != nil {
			t.Fatalf("failed to drop table: err=%s", err)
		}
	}
	return session, tableId, tearDown
}

func compareResult[T any](t *testing.T, got T, expected T) {
	t.Helper()
	opts := []cmp.Option{
		cmpopts.IgnoreFields(Result{}, "Stats"),
		cmpopts.IgnoreFields(Result{}, "Timestamp"),
		// Commit Stats is only provided by real instances
		cmpopts.IgnoreFields(Result{}, "CommitStats"),
		cmpopts.EquateEmpty(),
		protocmp.Transform(),
	}
	if !cmp.Equal(got, expected, opts...) {
		t.Errorf("diff(-got, +expected): %s", cmp.Diff(got, expected, opts...))
	}
}

func TestSelect(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{
		"INSERT INTO [[TABLE]] (id, active) VALUES (1, true), (2, false)",
	})
	defer tearDown()

	stmt, err := BuildStatement(fmt.Sprintf("SELECT id, active FROM %s ORDER BY id ASC", tableId))
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		ColumnNames: []string{"id", "active"},
		Rows: []Row{
			Row{[]string{"1", "true"}},
			Row{[]string{"2", "false"}},
		},
		AffectedRows: 2,
		ColumnTypes:  testTableRowType,
		IsMutation:   false,
	})
}

func TestDml(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{})
	defer tearDown()

	stmt, err := BuildStatement(fmt.Sprintf("INSERT INTO %s (id, active) VALUES (1, true), (2, false)", tableId))
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		AffectedRows: 2,
		IsMutation:   true,
	})

	// check by query
	query := spanner.NewStatement(fmt.Sprintf("SELECT id, active FROM %s ORDER BY id ASC", tableId))
	iter := session.client.Single().Query(ctx, query)
	defer iter.Stop()
	var gotStructs []testTableSchema
	for {
		row, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var got testTableSchema
		if err := row.ToStruct(&got); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		gotStructs = append(gotStructs, got)
	}
	expectedStructs := []testTableSchema{
		{1, true},
		{2, false},
	}
	if !cmp.Equal(gotStructs, expectedStructs) {
		t.Errorf("diff: %s", cmp.Diff(gotStructs, expectedStructs))
	}
}

func buildAndExecute(t *testing.T, ctx context.Context, session *Session, s string) *Result {
	stmt, err := BuildStatement(s)
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}
	return result
}

func TestSystemVariables(t *testing.T) {
	spannerContainer, teardownContainer := initialize(t)
	defer teardownContainer()

	session, _, tearDown := setup(t, context.Background(), spannerContainer, []string{})
	defer tearDown()

	t.Run("set and show string system variables", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		tcases := []struct {
			varname string
			value   string
		}{
			{"OPTIMIZER_VERSION", "4"},
			{"OPTIMIZER_STATISTICS_PACKAGE", "analyze_20241017_15_59_17UTC"},
			{"RPC_PRIORITY", "HIGH"},
			{"CLI_FORMAT", "TABLE"},
		}

		for _, tt := range tcases {
			t.Run(tt.varname, func(t *testing.T) {
				_ = buildAndExecute(t, ctx, session, fmt.Sprintf(`SET %v = "%v"`, tt.varname, tt.value))
				result := buildAndExecute(t, ctx, session, fmt.Sprintf(`SHOW VARIABLE %v`, tt.varname))
				if diff := cmp.Diff([]string{tt.varname}, result.ColumnNames); diff != "" {
					t.Errorf("SHOW column names differ: %v", diff)
				}
				if diff := cmp.Diff([]Row{{Columns: []string{tt.value}}}, result.Rows); diff != "" {
					t.Errorf("SHOW rows differ: %v", diff)
				}
			})
		}
	})
}

func TestStatements(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	tests := []struct {
		desc         string
		stmt         []string
		wantResults  []*Result
		teardownDDLs []string
	}{
		{
			desc: "begin, insert THEN RETURN, rollback, select",
			stmt: sliceOf(
				"CREATE TABLE TestTable1(id INT64, active BOOL) PRIMARY KEY(id)",
				"BEGIN",
				"INSERT INTO TestTable1 (id, active) VALUES (1, true), (2, false) THEN RETURN *",
				"ROLLBACK",
				"SELECT id, active FROM TestTable1 ORDER BY id ASC",
			),
			teardownDDLs: sliceOf("DROP TABLE TestTable1"),
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true},
				{
					IsMutation: true, AffectedRows: 2,
					Rows: sliceOf(
						toRow("1", "true"),
						toRow("2", "false"),
					),
					ColumnNames: sliceOf("id", "active"),
					ColumnTypes: testTableRowType,
				},
				{IsMutation: true},
				{
					Rows:        []Row{},
					ColumnNames: sliceOf("id", "active"),
					ColumnTypes: testTableRowType,
				},
			},
		},
		{
			desc: "begin, insert, commit, select",
			stmt: sliceOf(
				"CREATE TABLE TestTable2(id INT64, active BOOL) PRIMARY KEY(id)",
				"BEGIN",
				"INSERT INTO TestTable2 (id, active) VALUES (1, true), (2, false)",
				"COMMIT",
				"SELECT id, active FROM TestTable2 ORDER BY id ASC",
			),
			teardownDDLs: sliceOf("DROP TABLE TestTable2"),
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 2},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					ColumnNames:  sliceOf("id", "active"),
					ColumnTypes:  testTableRowType,
				},
			},
		},
		{
			desc: "read-only transactions",
			stmt: sliceOf(
				"CREATE TABLE TestTable3(id INT64, active BOOL) PRIMARY KEY(id)",
				"INSERT INTO TestTable3 (id, active) VALUES (1, true), (2, false)",
				"BEGIN RO",
				"SELECT id, active FROM TestTable3 ORDER BY id ASC",
				"ROLLBACK",
				"BEGIN",
				"SET TRANSACTION READ ONLY",
				"SELECT id, active FROM TestTable3 ORDER BY id ASC",
				"COMMIT",
				"SET READONLY = TRUE",
				"BEGIN",
				"SELECT id, active FROM TestTable3 ORDER BY id ASC",
				"COMMIT",
			),
			teardownDDLs: sliceOf("DROP TABLE TestTable3"),
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 2},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					ColumnNames:  sliceOf("id", "active"),
					ColumnTypes:  testTableRowType,
				},
				{IsMutation: true},
				{IsMutation: true},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					ColumnNames:  sliceOf("id", "active"),
					ColumnTypes:  testTableRowType,
				},
				{IsMutation: true},
				{KeepVariables: true},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					ColumnNames:  sliceOf("id", "active"),
					ColumnTypes:  testTableRowType,
				},
				{IsMutation: true},
			},
		},
		{
			desc: "read-write transactions",
			stmt: sliceOf(
				"CREATE TABLE TestTable4(id INT64, active BOOL) PRIMARY KEY(id)",
				"INSERT INTO TestTable4 (id, active) VALUES (1, true), (2, false)",
				"BEGIN",
				"DELETE TestTable4 WHERE TRUE THEN RETURN *",
				"ROLLBACK",
				"BEGIN",
				"SET TRANSACTION READ WRITE",
				"DELETE TestTable4 WHERE TRUE THEN RETURN *",
				"ROLLBACK",
				"BEGIN RW",
				"DELETE TestTable4 WHERE TRUE THEN RETURN *",
				"COMMIT",
			),
			teardownDDLs: sliceOf("DROP TABLE TestTable4"),
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 2},
				{IsMutation: true},
				{
					IsMutation:   true,
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					ColumnNames:  sliceOf("id", "active"),
					ColumnTypes:  testTableRowType,
				},
				{IsMutation: true},
				{IsMutation: true},
				{IsMutation: true},
				{
					IsMutation:   true,
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					ColumnNames:  sliceOf("id", "active"),
					ColumnTypes:  testTableRowType,
				},
				{IsMutation: true},
				{IsMutation: true},
				{
					IsMutation:   true,
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					ColumnNames:  sliceOf("id", "active"),
					ColumnTypes:  testTableRowType,
				},
				{IsMutation: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()

			session, tearDown := setupSession(t, ctx, spannerContainer, tt.teardownDDLs)
			defer tearDown()

			var gots []*Result
			for i, s := range tt.stmt {
				// begin
				stmt, err := BuildStatement(s)
				if err != nil {
					t.Fatalf("invalid statement[%d]: error=%s", i, err)
				}

				result, err := stmt.Execute(ctx, session)
				if err != nil {
					t.Fatalf("unexpected error happened[%d]: %s", i, err)
				}
				gots = append(gots, result)
			}
			compareResult(t, gots, tt.wantResults)
		})
	}
}

func TestReadWriteTransaction(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	t.Run("begin, insert, and commit", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{})
		defer tearDown()

		// begin
		stmt, err := BuildStatement("BEGIN")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err := stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 0,
			IsMutation:   true,
		})

		// insert
		stmt, err = BuildStatement(fmt.Sprintf("INSERT INTO %s (id, active) VALUES (1, true), (2, false)", tableId))
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 2,
			IsMutation:   true,
		})

		// commit
		stmt, err = BuildStatement("COMMIT")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 0,
			IsMutation:   true,
		})

		// check by query
		query := spanner.NewStatement(fmt.Sprintf("SELECT id, active FROM %s ORDER BY id ASC", tableId))
		iter := session.client.Single().Query(ctx, query)
		defer iter.Stop()
		var gotStructs []testTableSchema
		for {
			row, err := iter.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			var got testTableSchema
			if err := row.ToStruct(&got); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			gotStructs = append(gotStructs, got)
		}
		expectedStructs := []testTableSchema{
			{1, true},
			{2, false},
		}
		if !cmp.Equal(gotStructs, expectedStructs) {
			t.Errorf("diff: %s", cmp.Diff(gotStructs, expectedStructs))
		}
	})

	t.Run("begin, insert, and rollback", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{})
		defer tearDown()

		// begin
		stmt, err := BuildStatement("BEGIN")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err := stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 0,
			IsMutation:   true,
		})

		// insert
		stmt, err = BuildStatement(fmt.Sprintf("INSERT INTO %s (id, active) VALUES (1, true), (2, false)", tableId))
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 2,
			IsMutation:   true,
		})

		// rollback
		stmt, err = BuildStatement("ROLLBACK")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 0,
			IsMutation:   true,
		})

		// check by query
		query := spanner.NewStatement(fmt.Sprintf("SELECT id, active FROM %s ORDER BY id ASC", tableId))
		iter := session.client.Single().Query(ctx, query)
		defer iter.Stop()
		_ = iter.Do(func(row *spanner.Row) error {
			t.Errorf("rollbacked, but written row found: %#v", row)
			return nil
		})
	})

	t.Run("heartbeat: transaction is not aborted even if the transaction is idle", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{"INSERT INTO [[TABLE]] (id, active) VALUES (1, true), (2, false)"})
		defer tearDown()

		// begin
		stmt, err := BuildStatement("BEGIN")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// first query
		query := spanner.NewStatement(fmt.Sprintf("SELECT id, active FROM %s", tableId))
		iter := session.client.Single().Query(ctx, query)
		defer iter.Stop()
		if _, err := iter.Next(); err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// default transaction idle time is 10 secs
		time.Sleep(10 * time.Second)

		// second query
		query = spanner.NewStatement(fmt.Sprintf("SELECT id, active FROM %s", tableId))
		iter = session.client.Single().Query(ctx, query)
		defer iter.Stop()
		if _, err := iter.Next(); err != nil {
			t.Fatalf("error should not happen: %s", err)
		}
	})
}

func TestReadOnlyTransaction(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	t.Run("begin ro, query, and close", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{"INSERT INTO [[TABLE]] (id, active) VALUES (1, true), (2, false)"})
		defer tearDown()

		// begin
		stmt, err := BuildStatement("BEGIN RO")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err := stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 0,
			IsMutation:   true,
		})

		// query
		stmt, err = BuildStatement(fmt.Sprintf("SELECT id, active FROM %s ORDER BY id ASC", tableId))
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			ColumnNames: []string{"id", "active"},
			Rows: []Row{
				Row{[]string{"1", "true"}},
				Row{[]string{"2", "false"}},
			},

			ColumnTypes: []*sppb.StructType_Field{
				{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				{Name: "active", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
			},
			AffectedRows: 2,
			IsMutation:   false,
		})

		// close
		stmt, err = BuildStatement("CLOSE")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			AffectedRows: 0,
			IsMutation:   true,
		})
	})

	t.Run("begin ro with stale read", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{"INSERT INTO [[TABLE]] (id, active) VALUES (1, true), (2, false)"})
		defer tearDown()

		// stale read also can't recognize the recent created table itself,
		// so sleep for a while
		time.Sleep(10 * time.Second)

		// insert more fixture
		stmt, err := BuildStatement(fmt.Sprintf("INSERT INTO %s (id, active) VALUES (3, true), (4, false)", tableId))
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// begin with stale read
		stmt, err = BuildStatement("BEGIN RO 5")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// query
		stmt, err = BuildStatement(fmt.Sprintf("SELECT id, active FROM %s ORDER BY id ASC", tableId))
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err := stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// should not include id=3 and id=4
		compareResult(t, result, &Result{
			ColumnNames: []string{"id", "active"},
			Rows: []Row{
				Row{[]string{"1", "true"}},
				Row{[]string{"2", "false"}},
			},
			ColumnTypes:  testTableRowType,
			AffectedRows: 2,
			IsMutation:   false,
		})

		// close
		stmt, err = BuildStatement("CLOSE")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		_, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}
	})
}

func TestShowCreateTable(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{})
	defer tearDown()

	stmt, err := BuildStatement(fmt.Sprintf("SHOW CREATE TABLE %s", tableId))
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		ColumnNames: []string{"Table", "Create Table"},
		Rows: []Row{
			{[]string{tableId, fmt.Sprintf("CREATE TABLE %s (\n  id INT64 NOT NULL,\n  active BOOL NOT NULL,\n) PRIMARY KEY(id)", tableId)}},
		},
		AffectedRows: 1,
		IsMutation:   false,
	})
}

func TestShowColumns(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{})
	defer tearDown()

	stmt, err := BuildStatement(fmt.Sprintf("SHOW COLUMNS FROM %s", tableId))
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		ColumnNames: []string{"Field", "Type", "NULL", "Key", "Key_Order", "Options"},
		Rows: []Row{
			{[]string{"id", "INT64", "NO", "PRIMARY_KEY", "ASC", "NULL"}},
			{[]string{"active", "BOOL", "NO", "NULL", "NULL", "NULL"}},
		},
		AffectedRows: 2,
		IsMutation:   false,
	})
}

func TestShowIndexes(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{})
	defer tearDown()

	stmt, err := BuildStatement(fmt.Sprintf("SHOW INDEXES FROM %s", tableId))
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		ColumnNames: []string{"Table", "Parent_table", "Index_name", "Index_type", "Is_unique", "Is_null_filtered", "Index_state"},
		Rows: []Row{
			{[]string{tableId, "", "PRIMARY_KEY", "PRIMARY_KEY", "true", "false", "NULL"}},
		},
		AffectedRows: 1,
		IsMutation:   false,
	})
}

func TestTruncateTable(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{
		"INSERT INTO [[TABLE]] (id, active) VALUES (1, true), (2, false)",
	})
	defer tearDown()

	stmt, err := BuildStatement(fmt.Sprintf("TRUNCATE TABLE %s", tableId))
	if err != nil {
		t.Fatalf("invalid statement: %v", err)
	}

	if _, err := stmt.Execute(ctx, session); err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	// We don't use the TRUNCATE TABLE's result since PartitionedUpdate may return estimated affected row counts.
	// Instead, we check if rows are remained in the table.
	var count int64
	countStmt := spanner.NewStatement(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableId))
	if err := session.client.Single().Query(ctx, countStmt).Do(func(r *spanner.Row) error {
		return r.Column(0, &count)
	}); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 0 {
		t.Errorf("TRUNCATE TABLE executed, but %d rows are remained", count)
	}
}

func TestPartitionedDML(t *testing.T) {
	spannerContainer, teardown := initialize(t)
	defer teardown()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	session, tableId, tearDown := setup(t, ctx, spannerContainer, []string{
		"INSERT INTO [[TABLE]] (id, active) VALUES (1, false)",
	})
	defer tearDown()

	stmt, err := BuildStatement(fmt.Sprintf("PARTITIONED UPDATE %s SET active = true WHERE true", tableId))
	if err != nil {
		t.Fatalf("invalid statement: %v", err)
	}

	if _, err := stmt.Execute(ctx, session); err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	selectStmt := spanner.NewStatement(fmt.Sprintf("SELECT active FROM %s", tableId))
	var got bool
	if err := session.client.Single().Query(ctx, selectStmt).Do(func(r *spanner.Row) error {
		return r.Column(0, &got)
	}); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if want := true; want != got {
		t.Errorf("PARTITIONED UPDATE was executed, but rows were not updated")
	}
}
