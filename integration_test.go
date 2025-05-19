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
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanemuboost"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/samber/lo"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"google.golang.org/protobuf/testing/protocmp"

	"cloud.google.com/go/spanner"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/apstndb/spantype/typector"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
)

type testTableSchema struct {
	Id     int64 `spanner:"id"`
	Active bool  `spanner:"active"`
}

var testTableRowType = typector.MustNameCodeSlicesToStructTypeFields(
	sliceOf("id", "active"),
	sliceOf(sppb.TypeCode_INT64, sppb.TypeCode_BOOL),
)

const testTableDDL = `
CREATE TABLE tbl (
  id INT64 NOT NULL,
  active BOOL NOT NULL
) PRIMARY KEY (id)
`

var testTableDDLs = sliceOf(testTableDDL)

var emulator *gcloud.GCloudContainer

func TestMain(m *testing.M) {
	emu, teardown, err := spanemuboost.NewEmulator(context.Background(),
		spanemuboost.EnableInstanceAutoConfigOnly(),
	)
	if err != nil {
		// testing.M doesn't have output method
		slog.Error("failed to create emulator", "err", err)
		os.Exit(1)
	}

	defer teardown()

	emulator = emu

	m.Run()
}

func initializeSession(ctx context.Context, emulator *gcloud.GCloudContainer, clients *spanemuboost.Clients) (session *Session, err error) {
	options := defaultClientOptions(emulator)
	session, err = NewSession(ctx, &systemVariables{
		Project:     clients.ProjectID,
		Instance:    clients.InstanceID,
		Database:    clients.DatabaseID,
		Params:      make(map[string]ast.Node),
		RPCPriority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}, options...)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func initializeDedicatedInstance(t *testing.T, database string, ddls, dmls []string) (clients *spanemuboost.Clients, session *Session, teardown func()) {
	t.Helper()
	ctx := t.Context()

	emulator, clients, clientsTeardown, err := spanemuboost.NewEmulatorWithClients(ctx,
		spanemuboost.WithDatabaseID(database),
		spanemuboost.EnableAutoConfig(),
		spanemuboost.WithClientConfig(spanner.ClientConfig{SessionPoolConfig: spanner.SessionPoolConfig{MinOpened: 5}}),
		spanemuboost.WithSetupDDLs(ddls),
		spanemuboost.WithSetupRawDMLs(dmls),
	)
	if err != nil {
		t.Fatal(err)
	}

	session, err = initializeSession(ctx, emulator, clients)
	if err != nil {
		clientsTeardown()
		t.Fatalf("failed to create test session: err=%s", err)
	}

	return clients, session, func() {
		session.Close()
		clientsTeardown()
	}
}

func initialize(t *testing.T, ddls, dmls []string) (clients *spanemuboost.Clients, session *Session, teardown func()) {
	t.Helper()
	ctx := t.Context()

	clients, clientsTeardown, err := spanemuboost.NewClients(ctx, emulator,
		spanemuboost.WithRandomDatabaseID(),
		spanemuboost.EnableDatabaseAutoConfigOnly(),
		spanemuboost.WithClientConfig(spanner.ClientConfig{SessionPoolConfig: spanner.SessionPoolConfig{MinOpened: 5}}),
		spanemuboost.WithSetupDDLs(ddls),
		spanemuboost.WithSetupRawDMLs(dmls),
	)
	if err != nil {
		t.Fatal(err)
	}

	session, err = initializeSession(ctx, emulator, clients)
	if err != nil {
		clientsTeardown()
		t.Fatalf("failed to create test session: err=%s", err)
	}

	return clients, session, func() {
		session.Close()
		clientsTeardown()
	}
}

// spannerContainer is a global variable but it receives explicitly.
func defaultClientOptions(spannerContainer *gcloud.GCloudContainer) []option.ClientOption {
	return sliceOf(
		option.WithEndpoint(spannerContainer.URI),
		option.WithoutAuthentication(),
		internaloption.SkipDialSettingsValidation(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
}

func compareResult[T any](t *testing.T, got T, expected T, customCmpOptions ...cmp.Option) {
	t.Helper()
	opts := sliceOf[cmp.Option](
		cmpopts.IgnoreFields(Result{}, "Stats"),
		cmpopts.IgnoreFields(Result{}, "Timestamp"),
		// Commit Stats is only provided by real instances
		cmpopts.IgnoreFields(Result{}, "CommitStats"),
		cmpopts.EquateEmpty(),
		protocmp.Transform(),
	)
	opts = append(opts, customCmpOptions...)

	if !cmp.Equal(got, expected, opts...) {
		t.Errorf("diff(-got, +expected): %s", cmp.Diff(got, expected, opts...))
	}
}

func TestSelect(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
	defer teardown()

	stmt, err := BuildStatement("SELECT id, active FROM tbl ORDER BY id ASC")
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		Rows: sliceOf(
			toRow("1", "true"),
			toRow("2", "false"),
		),
		AffectedRows: 2,
		TableHeader:  toTableHeader(testTableRowType),
		IsMutation:   false,
	})
}

func TestDml(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initialize(t, testTableDDLs, nil)
	defer teardown()

	stmt, err := BuildStatement("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)")
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
	query := spanner.NewStatement("SELECT id, active FROM tbl ORDER BY id ASC")
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
	_, session, teardown := initialize(t, nil, nil)
	defer teardown()

	t.Run("set and show string system variables", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
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
				if diff := cmp.Diff(sliceOf(tt.varname), result.TableHeader.Render(false)); diff != "" {
					t.Errorf("SHOW column names differ: %v", diff)
				}
				if diff := cmp.Diff(sliceOf(toRow(tt.value)), result.Rows); diff != "" {
					t.Errorf("SHOW rows differ: %v", diff)
				}
			})
		}
	})
}

func TestStatements(t *testing.T) {
	tests := []struct {
		desc        string
		ddls, dmls  []string // initialize statements
		stmt        []string
		wantResults []*Result
		cmpOpts     []cmp.Option
		database    string
	}{
		{
			desc: "query parameters",
			stmt: sliceOf(
				`SET PARAM b = true`,
				`SET PARAM bs = b"foo"`,
				`SET PARAM i64 = 1`,
				`SET PARAM f64 = 1.0`,
				`SET PARAM f32 = CAST(1.0 AS FLOAT32)`,
				`SET PARAM n = NUMERIC "1"`,
				`SET PARAM s = "foo"`,
				`SET PARAM js = JSON "{}"`,
				`SET PARAM ts = TIMESTAMP "2000-01-01T00:00:00Z"`,
				`SET PARAM ival_single = INTERVAL 3 DAY`,
				`SET PARAM ival_range = INTERVAL "3-4 5 6:7:8.999999999" YEAR TO SECOND`,
				`SET PARAM a_b = [true]`,
				`SET PARAM n_b = CAST(NULL AS BOOL)`,
				`SET PARAM n_ival = CAST(NULL AS INTERVAL)`,
				`SELECT @b AS b, @bs AS bs, @i64 AS i64, @f64 AS f64, @f32 AS f32, @n AS n, @s AS s, @js AS js, @ts AS ts,
				        @ival_single AS ival_single, @ival_range AS ival_range,
 				        @a_b AS a_b, @n_b AS n_b, @n_ival AS n_ival`,
			),
			wantResults: []*Result{
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{KeepVariables: true},
				{
					TableHeader: toTableHeader(sliceOf(
						typector.NameTypeToStructTypeField("b", typector.CodeToSimpleType(sppb.TypeCode_BOOL)),
						typector.NameTypeToStructTypeField("bs", typector.CodeToSimpleType(sppb.TypeCode_BYTES)),
						typector.NameTypeToStructTypeField("i64", typector.CodeToSimpleType(sppb.TypeCode_INT64)),
						typector.NameTypeToStructTypeField("f64", typector.CodeToSimpleType(sppb.TypeCode_FLOAT64)),
						typector.NameTypeToStructTypeField("f32", typector.CodeToSimpleType(sppb.TypeCode_FLOAT32)),
						typector.NameTypeToStructTypeField("n", typector.CodeToSimpleType(sppb.TypeCode_NUMERIC)),
						typector.NameTypeToStructTypeField("s", typector.CodeToSimpleType(sppb.TypeCode_STRING)),
						typector.NameTypeToStructTypeField("js", typector.CodeToSimpleType(sppb.TypeCode_JSON)),
						typector.NameTypeToStructTypeField("ts", typector.CodeToSimpleType(sppb.TypeCode_TIMESTAMP)),
						typector.NameTypeToStructTypeField("ival_single", typector.CodeToSimpleType(sppb.TypeCode_INTERVAL)),
						typector.NameTypeToStructTypeField("ival_range", typector.CodeToSimpleType(sppb.TypeCode_INTERVAL)),
						typector.NameTypeToStructTypeField("a_b", typector.ElemCodeToArrayType(sppb.TypeCode_BOOL)),
						typector.NameTypeToStructTypeField("n_b", typector.CodeToSimpleType(sppb.TypeCode_BOOL)),
						typector.NameTypeToStructTypeField("n_ival", typector.CodeToSimpleType(sppb.TypeCode_INTERVAL)),
					)),
					Rows: sliceOf(
						toRow("true", "Zm9v", "1", "1.000000", "1.000000", "1", "foo", "{}", "2000-01-01T00:00:00Z",
							"P3D", "P3Y4M5DT6H7M8.999999999S",
							"[true]", "NULL", "NULL"),
					),
					AffectedRows: 1,
				},
			},
		},
		{
			desc: "SHOW VARIABLE CLI_VERSION",
			stmt: sliceOf(
				`SHOW VARIABLE CLI_VERSION`,
			),
			wantResults: []*Result{
				{
					KeepVariables: true,
					ColumnNames:   sliceOf("CLI_VERSION"),
					Rows:          sliceOf(toRow(getVersion()))},
			},
		},
		{
			desc: "SHOW LOCAL PROTO with pb file",
			stmt: sliceOf(
				`SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/order_descriptors.pb"`,
				`SHOW LOCAL PROTO`,
			),
			wantResults: []*Result{
				{KeepVariables: true},
				{
					TableHeader: toTableHeader("full_name", "kind", "package", "file"),
					Rows: sliceOf(
						toRow("examples.shipping.Order", "PROTO", "examples.shipping", "order_protos.proto"),
						toRow("examples.shipping.Order.Address", "PROTO", "examples.shipping", "order_protos.proto"),
						toRow("examples.shipping.Order.Item", "PROTO", "examples.shipping", "order_protos.proto"),
						toRow("examples.shipping.OrderHistory", "PROTO", "examples.shipping", "order_protos.proto"),
					),
					AffectedRows:  4,
					KeepVariables: true,
				},
			},
		},
		{
			desc: "SHOW LOCAL PROTO with proto file",
			stmt: sliceOf(
				`SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/singer.proto"`,
				`SHOW LOCAL PROTO`,
			),
			wantResults: []*Result{
				{KeepVariables: true},
				{
					TableHeader: toTableHeader("full_name", "kind", "package", "file"),
					Rows: sliceOf(
						toRow("examples.spanner.music.SingerInfo", "PROTO", "examples.spanner.music", "testdata/protos/singer.proto"),
						toRow("examples.spanner.music.CustomSingerInfo", "PROTO", "examples.spanner.music", "testdata/protos/singer.proto"),
						toRow("examples.spanner.music.Genre", "ENUM", "examples.spanner.music", "testdata/protos/singer.proto"),
						toRow("examples.spanner.music.CustomGenre", "ENUM", "examples.spanner.music", "testdata/protos/singer.proto"),
					),
					AffectedRows:  4,
					KeepVariables: true,
				},
			},
		},
		{
			desc: "BATCH DML with parameters",
			stmt: sliceOf(
				"CREATE TABLE TestTable(id INT64, active BOOL) PRIMARY KEY(id)",
				"START BATCH DML",
				"SET PARAM n = 1",
				"SET PARAM b = true",
				"INSERT INTO TestTable (id, active) VALUES (@n, @b)",
				"SET PARAM n = 2",
				"SET PARAM b = false",
				"INSERT INTO TestTable (id, active) VALUES (@n, @b)",
				"RUN BATCH",
				"SELECT id, active FROM TestTable ORDER BY id ASC",
			),
			wantResults: []*Result{
				{IsMutation: true},
				{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDML}},
				{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDML}},
				{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDML}},
				{IsMutation: true, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: 1}},
				{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: 1}},
				{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: 1}},
				{IsMutation: true, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: 2}},
				{
					TableHeader: toTableHeader("DML", "Rows"),
					Rows: sliceOf(
						toRow("INSERT INTO TestTable (id, active) VALUES (@n, @b)", "1"),
						toRow("INSERT INTO TestTable (id, active) VALUES (@n, @b)", "1"),
					),
					AffectedRows:     2,
					AffectedRowsType: rowCountTypeUpperBound,
					IsMutation:       true,
					KeepVariables:    false,
				},
				{
					AffectedRows: 2,
					Rows: sliceOf(
						toRow("1", "true"),
						toRow("2", "false"),
					),
					TableHeader: toTableHeader(testTableRowType),
				},
			},
		},
		{
			desc: "begin, insert THEN RETURN, rollback, select",
			stmt: sliceOf(
				"CREATE TABLE TestTable1(id INT64, active BOOL) PRIMARY KEY(id)",
				"BEGIN",
				"INSERT INTO TestTable1 (id, active) VALUES (1, true), (2, false) THEN RETURN *",
				"ROLLBACK",
				"SELECT id, active FROM TestTable1 ORDER BY id ASC",
			),
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true},
				{
					IsMutation: true, AffectedRows: 2,
					Rows: sliceOf(
						toRow("1", "true"),
						toRow("2", "false"),
					),
					TableHeader: toTableHeader(testTableRowType),
				},
				{IsMutation: true},
				{
					Rows:        nil,
					TableHeader: toTableHeader(testTableRowType),
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
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 2},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					TableHeader:  toTableHeader(testTableRowType),
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
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 2},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					TableHeader:  toTableHeader(testTableRowType),
				},
				{IsMutation: true},
				{IsMutation: true},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					TableHeader:  toTableHeader(testTableRowType),
				},
				{IsMutation: true},
				{KeepVariables: true},
				{IsMutation: true},
				{
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					TableHeader:  toTableHeader(testTableRowType),
				},
				{IsMutation: true},
			},
		},
		{
			desc: "SET PARAM TYPE and SHOW PARAMS",
			stmt: sliceOf(
				"SET PARAM i INT64",
				"SHOW PARAMS",
				"DESCRIBE SELECT @i AS i",
			),
			wantResults: []*Result{
				{KeepVariables: true},
				{
					KeepVariables: true,
					ColumnNames:   sliceOf("Param_Name", "Param_Kind", "Param_Value"),
					Rows:          sliceOf(toRow("i", "TYPE", "INT64")),
				},
				{
					AffectedRows: 1,
					ColumnNames:  sliceOf("Column_Name", "Column_Type"),
					Rows:         sliceOf(toRow("i", "INT64")),
				},
			},
		},
		{
			desc: "HELP",
			stmt: sliceOf("HELP"),
			wantResults: []*Result{
				// It should be safe because HELP doesn't depend on ctx and session.
				lo.Must((&HelpStatement{}).Execute(context.TODO(), nil)),
			},
		},
		{
			desc: "SHOW DDLS",
			stmt: sliceOf("SHOW DDLS"),
			ddls: sliceOf("CREATE TABLE TestTable (id INT64, active BOOL) PRIMARY KEY (id)"),
			wantResults: []*Result{
				{
					ColumnNames:   sliceOf(""),
					KeepVariables: true,
					Rows: sliceOf(
						toRow(heredoc.Doc(`
						CREATE TABLE TestTable (
						  id INT64,
						  active BOOL,
						) PRIMARY KEY(id);
						`))),
				},
			},
		},
		{
			desc: "SPLIT POINTS statements",
			// TODO: Split points are not yet supported by cloud-spanner-emulator.
		},
		{
			desc: "EXPLAIN & EXPLAIN ANALYZE statements",
			// TODO: QueryMode PLAN(EXPLAIN) and PROFILE(EXPLAIN ANALYZE) are not yet supported by cloud-spanner-emulator.
		},
		{
			desc: "PROTO BUNDLE statements",
			// Note: Current cloud-spanner-emulator only accepts DDL, but it is nop.
			stmt: sliceOf(
				"SHOW REMOTE PROTO",
				`SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/order_descriptors.pb"`,
				"CREATE PROTO BUNDLE (`examples.shipping.Order`)",
				"ALTER PROTO BUNDLE DELETE (`examples.shipping.Order`)",
				"SYNC PROTO BUNDLE DELETE (`examples.shipping.Order`)",
			),
			wantResults: []*Result{
				{KeepVariables: true, ColumnNames: sliceOf("full_name", "kind", "package")},
				{KeepVariables: true},
				{IsMutation: true},
				{IsMutation: true},
				{IsMutation: true},
			},
		},
		{
			desc:     "DATABASE statements",
			database: "test-database",
			stmt: sliceOf("SHOW DATABASES",
				"CREATE DATABASE `new-database`",
				"SHOW DATABASES",

				// Note: The USE statement is not processed by Session, so we can't test the effect of them.
				"USE `new-database` ROLE spanner_info_reader", // nop
				"USE `test-database`",                         // nop

				"DROP DATABASE `new-database`",
				"SHOW DATABASES",
			),
			wantResults: []*Result{
				{ColumnNames: sliceOf("Database"), Rows: sliceOf(toRow("test-database")), AffectedRows: 1},
				{IsMutation: true},
				{ColumnNames: sliceOf("Database"), Rows: sliceOf(toRow("new-database"), toRow("test-database")), AffectedRows: 2},
				{},
				{},
				{IsMutation: true},
				{ColumnNames: sliceOf("Database"), Rows: sliceOf(toRow("test-database")), AffectedRows: 1},
			},
		},
		{
			desc: "SHOW TABLES",
			stmt: sliceOf("SHOW TABLES"),
			ddls: sliceOf("CREATE TABLE TestTable (id INT64, active BOOL) PRIMARY KEY (id)"),
			wantResults: []*Result{
				{ColumnNames: sliceOf(""), Rows: sliceOf(toRow("TestTable")), AffectedRows: 1},
			},
			cmpOpts: sliceOf(cmp.FilterPath(func(path cmp.Path) bool {
				return regexp.MustCompile(`\.ColumnNames`).MatchString(path.GoString())
			}, cmp.Ignore())),
		},
		{
			desc: "TRY PARTITIONED QUERY",
			stmt: sliceOf("TRY PARTITIONED QUERY SELECT 1"),
			wantResults: []*Result{
				{
					ForceWrap:    true,
					AffectedRows: 1,
					ColumnNames:  sliceOf("Root_Partitionable"),
					Rows:         sliceOf(toRow("TRUE")),
				},
			},
		},
		{
			desc: "mutation, pdml, partitioned query",
			ddls: sliceOf("CREATE TABLE TestTable(id INT64, active BOOL) PRIMARY KEY(id)"),
			stmt: sliceOf(
				"MUTATE TestTable INSERT STRUCT(1 AS id, TRUE AS active)",
				"PARTITIONED UPDATE TestTable SET active = FALSE WHERE id = 1",
				"RUN PARTITIONED QUERY SELECT id, active FROM TestTable",
			),
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 1, AffectedRowsType: rowCountTypeLowerBound},
				{
					AffectedRows:   1,
					PartitionCount: 2,
					ColumnNames:    sliceOf("id", "active"),
					ColumnTypes:    testTableRowType,
					Rows:           sliceOf(toRow("1", "false")),
				},
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
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 2},
				{IsMutation: true},
				{
					IsMutation:   true,
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					TableHeader:  toTableHeader(testTableRowType),
				},
				{IsMutation: true},
				{IsMutation: true},
				{IsMutation: true},
				{
					IsMutation:   true,
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					TableHeader:  toTableHeader(testTableRowType),
				},
				{IsMutation: true},
				{IsMutation: true},
				{
					IsMutation:   true,
					AffectedRows: 2,
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
					TableHeader:  toTableHeader(testTableRowType),
				},
				{IsMutation: true},
			},
		},
		{
			desc: "BATCH DDL",
			stmt: sliceOf(
				"SET CLI_ECHO_EXECUTED_DDL = TRUE",
				"START BATCH DDL",
				heredoc.Doc(`
								CREATE TABLE TestTable (
									id		INT64,
									active	BOOL,
								) PRIMARY KEY(id)`),
				`CREATE TABLE TestTable2 (id INT64, active BOOL) PRIMARY KEY(id)`,
				"RUN BATCH",
			),
			wantResults: []*Result{
				{KeepVariables: true},
				{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDDL}},
				{IsMutation: true, BatchInfo: &BatchInfo{Mode: batchModeDDL, Size: 1}},
				{IsMutation: true, BatchInfo: &BatchInfo{Mode: batchModeDDL, Size: 2}},
				// tab is pass-through as is in this layer
				{IsMutation: true, AffectedRows: 0, TableHeader: toTableHeader("Executed", "Commit Timestamp"), Rows: sliceOf(
					toRow(
						heredoc.Doc(`
										CREATE TABLE TestTable (
											id		INT64,
											active	BOOL,
										) PRIMARY KEY(id);`), "(ignored)"),
					toRow(`CREATE TABLE TestTable2 (id INT64, active BOOL) PRIMARY KEY(id);`, "(ignored)"),
				),
				},
			},
			cmpOpts: sliceOf(
				// Ignore Commit Timestamp column value
				cmp.FilterPath(func(path cmp.Path) bool {
					return regexp.MustCompile(`\.Rows\[\d*]\[1]`).MatchString(path.GoString())
				}, cmp.Ignore()),
			),
		},
		{
			desc: "AUTO_BATCH_DML",
			stmt: sliceOf(
				"CREATE TABLE TestTable6(id INT64, active BOOL) PRIMARY KEY(id)",
				"SET AUTO_BATCH_DML = TRUE",
				"INSERT INTO TestTable6 (id, active) VALUES (1,true)",
				"BEGIN",
				"INSERT INTO TestTable6 (id, active) VALUES (2,	false)", // includes tab character
				"COMMIT",
				"SELECT * FROM TestTable6 ORDER BY id",
			),
			wantResults: []*Result{
				{IsMutation: true},
				{IsMutation: false, KeepVariables: true},
				{IsMutation: true, AffectedRows: 1},
				{IsMutation: true},
				{IsMutation: true, AffectedRows: 0, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: 1}},
				// tab is pass-through as is in this layer
				{IsMutation: true, AffectedRows: 1, TableHeader: toTableHeader("DML", "Rows"), Rows: sliceOf(Row{"INSERT INTO TestTable6 (id, active) VALUES (2,	false)", "1"})},
				{
					AffectedRows: 2,
					TableHeader:  toTableHeader(testTableRowType),
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
				},
			},
		},

		// --- Added Test Cases ---
		{
			desc: "SHOW VARIABLES",
			stmt: sliceOf("SHOW VARIABLES"),
			wantResults: []*Result{
				{
					ColumnNames:   sliceOf("name", "value"),
					KeepVariables: true,
					// Rows and AffectedRows are dynamic, so we don't check them here.
				},
			},
			cmpOpts: sliceOf(
				cmp.FilterPath(func(path cmp.Path) bool {
					return regexp.MustCompile(`\.Rows`).MatchString(path.GoString()) ||
						regexp.MustCompile(`\.AffectedRows`).MatchString(path.GoString())
				}, cmp.Ignore()),
			),
		},
		{
			desc: "HELP VARIABLES",
			stmt: sliceOf("HELP VARIABLES"),
			wantResults: []*Result{
				lo.Must((&HelpVariablesStatement{}).Execute(context.TODO(), nil)),
			},
		},
		{
			desc: "ABORT BATCH DML",
			ddls: sliceOf("CREATE TABLE TestAbortBatchDML(id INT64 PRIMARY KEY)"),
			stmt: sliceOf(
				"START BATCH DML",
				"INSERT INTO TestAbortBatchDML (id) VALUES (1)",
				"ABORT BATCH",
				"SELECT COUNT(*) FROM TestAbortBatchDML",
			),
			wantResults: []*Result{
				{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDML}},       // START BATCH
				{IsMutation: true, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: 1}}, // INSERT (batched)
				{KeepVariables: true}, // ABORT BATCH
				{ // SELECT COUNT(*)
					ColumnNames:  sliceOf(""),
					ColumnTypes:  sliceOf(typector.NameTypeToStructTypeField("", typector.CodeToSimpleType(sppb.TypeCode_INT64))),
					Rows:         sliceOf(toRow("0")),
					AffectedRows: 1,
				},
			},
			cmpOpts: sliceOf(cmp.FilterPath(func(path cmp.Path) bool {
				return regexp.MustCompile(`\.ColumnNames`).MatchString(path.String()) &&
					!strings.Contains(path.String(), "wantResults[3]")
			}, cmp.Ignore())),
		},
		{
			desc: "CQL SELECT",
			// It can't be tested because cloud-spanner-emulator doesn't support Cassandra interface.
		},
		{
			desc: "SET ADD statement for CLI_PROTO_FILES",
			stmt: sliceOf(
				`SET CLI_PROTO_DESCRIPTOR_FILE += "testdata/protos/order_descriptors.pb"`,
				`SHOW VARIABLE CLI_PROTO_DESCRIPTOR_FILE`,
				`SET CLI_PROTO_DESCRIPTOR_FILE += "testdata/protos/singer.proto"`,
				`SHOW VARIABLE CLI_PROTO_DESCRIPTOR_FILE`,
			),
			wantResults: []*Result{
				{KeepVariables: true}, // SET +=
				{ // SHOW VARIABLE
					KeepVariables: true,
					ColumnNames:   sliceOf("CLI_PROTO_DESCRIPTOR_FILE"),
					Rows:          sliceOf(toRow(`testdata/protos/order_descriptors.pb`)),
				},
				{KeepVariables: true}, // SET +=
				{ // SHOW VARIABLE
					KeepVariables: true,
					ColumnNames:   sliceOf("CLI_PROTO_DESCRIPTOR_FILE"),
					Rows:          sliceOf(toRow(`testdata/protos/order_descriptors.pb,testdata/protos/singer.proto`)),
				},
			},
		},
		{
			desc: "SHOW CREATE INDEX",
			ddls: sliceOf(
				"CREATE TABLE TestShowCreateIndexTbl(id INT64, val INT64) PRIMARY KEY(id)",
				"CREATE INDEX TestShowCreateIndexIdx ON TestShowCreateIndexTbl(val)",
			),
			stmt: sliceOf("SHOW CREATE INDEX TestShowCreateIndexIdx"),
			wantResults: []*Result{
				{
					ColumnNames:  sliceOf("Name", "DDL"),
					Rows:         sliceOf(toRow("TestShowCreateIndexIdx", "CREATE INDEX TestShowCreateIndexIdx ON TestShowCreateIndexTbl(val)")),
					AffectedRows: 1,
				},
			},
		},
		{
			desc: "DESCRIBE DML (INSERT with literal)",
			ddls: sliceOf("CREATE TABLE TestDescribeDMLTbl(id INT64 PRIMARY KEY)"),
			stmt: sliceOf("DESCRIBE INSERT INTO TestDescribeDMLTbl (id) VALUES (1)"),
			wantResults: []*Result{
				{
					// For DML without THEN RETURN, result is empty.
					ColumnNames:  sliceOf("Column_Name", "Column_Type"),
					Rows:         nil, // No parameters in this DML
					AffectedRows: 0,   // 0 parameters
				},
			},
		},
		{
			desc: "PARTITION SELECT query",
			ddls: sliceOf("CREATE TABLE TestPartitionQueryTbl(id INT64 PRIMARY KEY)"),
			stmt: sliceOf("PARTITION SELECT id FROM TestPartitionQueryTbl"),
			wantResults: []*Result{
				{
					ColumnNames:  sliceOf("Partition_Token"),
					AffectedRows: 2, // Emulator usually creates a couple of partitions for simple queries
					ForceWrap:    true,
				},
			},
			cmpOpts: sliceOf(
				cmp.FilterPath(func(path cmp.Path) bool {
					return regexp.MustCompile(`\.Rows`).MatchString(path.GoString()) // Ignore actual token values
				}, cmp.Ignore()),
			),
		},
		{
			desc: "SHOW SCHEMA UPDATE OPERATIONS (empty result expected)",
			stmt: sliceOf("SHOW SCHEMA UPDATE OPERATIONS"),
			wantResults: []*Result{
				{
					ColumnNames:  sliceOf("OPERATION_ID", "STATEMENTS", "DONE", "PROGRESS", "COMMIT_TIMESTAMP", "ERROR"),
					Rows:         nil, // Expect no operations on a fresh emulator DB
					AffectedRows: 0,
				},
			},
		},
		{
			desc: "MUTATE DELETE",
			ddls: sliceOf("CREATE TABLE TestMutateDeleteTbl(id INT64 PRIMARY KEY)"),
			stmt: sliceOf(
				"INSERT INTO TestMutateDeleteTbl (id) VALUES (1)", // Standard DML to insert
				"MUTATE TestMutateDeleteTbl DELETE (1)",
				"SELECT COUNT(*) FROM TestMutateDeleteTbl",
			),
			wantResults: []*Result{
				{IsMutation: true, AffectedRows: 1}, // Result of INSERT
				{IsMutation: true},                  // Result of MUTATE (AffectedRows not set by MutateStatement)
				{ // SELECT COUNT(*)
					ColumnNames:  sliceOf(""),
					ColumnTypes:  sliceOf(typector.NameTypeToStructTypeField("", typector.CodeToSimpleType(sppb.TypeCode_INT64))),
					Rows:         sliceOf(toRow("0")),
					AffectedRows: 1,
				},
			},
			cmpOpts: sliceOf(cmp.FilterPath(func(path cmp.Path) bool {
				return regexp.MustCompile(`\.ColumnNames`).MatchString(path.String()) &&
					!strings.Contains(path.String(), "wantResults[2]") // Allow ColumnNames for SELECT
			}, cmp.Ignore())),
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()

			var session *Session
			var teardown func()
			if tt.database == "" {
				_, session, teardown = initialize(t, tt.ddls, tt.dmls)
			} else {
				_, session, teardown = initializeDedicatedInstance(t, tt.database, tt.ddls, tt.dmls)
			}
			defer teardown()

			var gots []*Result
			for i, s := range tt.stmt {
				// begin
				stmt, err := BuildStatementWithCommentsWithMode(strings.TrimSpace(lo.Must(gsqlutils.StripComments("", s))), s, parseModeNoMemefish)
				if err != nil {
					t.Fatalf("invalid statement[%d]: error=%s", i, err)
				}

				result, err := session.ExecuteStatement(ctx, stmt)
				if err != nil {
					t.Fatalf("unexpected error happened[%d]: %s", i, err)
				}
				gots = append(gots, result)
			}
			compareResult(t, gots, tt.wantResults, tt.cmpOpts...)
		})
	}

}

func TestReadWriteTransaction(t *testing.T) {
	t.Run("begin, insert, and commit", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initialize(t, testTableDDLs, nil)
		defer teardown()

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
		stmt, err = BuildStatement("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)")
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
		query := spanner.NewStatement("SELECT id, active FROM tbl ORDER BY id ASC")
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
		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initialize(t, testTableDDLs, nil)
		defer teardown()

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
		stmt, err = BuildStatement("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)")
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
		query := spanner.NewStatement("SELECT id, active FROM tbl ORDER BY id ASC")
		iter := session.client.Single().Query(ctx, query)
		defer iter.Stop()
		_ = iter.Do(func(row *spanner.Row) error {
			t.Errorf("rollbacked, but written row found: %#v", row)
			return nil
		})
	})

	t.Run("heartbeat: transaction is not aborted even if the transaction is idle", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
		defer teardown()

		// begin
		stmt, err := BuildStatement("BEGIN")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// first query
		query := spanner.NewStatement("SELECT id, active FROM tbl")
		iter := session.client.Single().Query(ctx, query)
		defer iter.Stop()
		if _, err := iter.Next(); err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// default transaction idle time is 10 secs
		time.Sleep(10 * time.Second)

		// second query
		query = spanner.NewStatement("SELECT id, active FROM tbl")
		iter = session.client.Single().Query(ctx, query)
		defer iter.Stop()
		if _, err := iter.Next(); err != nil {
			t.Fatalf("error should not happen: %s", err)
		}
	})
}

func TestReadOnlyTransaction(t *testing.T) {
	t.Run("begin ro, query, and close", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
		defer teardown()

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
		stmt, err = BuildStatement("SELECT id, active FROM tbl ORDER BY id ASC")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err = stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		compareResult(t, result, &Result{
			Rows: sliceOf(
				toRow("1", "true"),
				toRow("2", "false"),
			),

			TableHeader:  toTableHeader(testTableRowType),
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
		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
		defer teardown()

		// stale read also can't recognize the recent created table itself,
		// so sleep for a while
		time.Sleep(10 * time.Second)

		// insert more fixture
		stmt, err := BuildStatement("INSERT INTO tbl (id, active) VALUES (3, true), (4, false)")
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
		stmt, err = BuildStatement("SELECT id, active FROM tbl ORDER BY id ASC")
		if err != nil {
			t.Fatalf("invalid statement: error=%s", err)
		}

		result, err := stmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("unexpected error happened: %s", err)
		}

		// should not include id=3 and id=4
		compareResult(t, result, &Result{
			Rows: sliceOf(
				toRow("1", "true"),
				toRow("2", "false"),
			),
			TableHeader:  toTableHeader(testTableRowType),
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
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
	defer teardown()

	stmt, err := BuildStatement("SHOW CREATE TABLE tbl")
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		TableHeader: toTableHeader("Name", "DDL"),
		Rows: sliceOf(
			toRow("tbl", "CREATE TABLE tbl (\n  id INT64 NOT NULL,\n  active BOOL NOT NULL,\n) PRIMARY KEY(id)"),
		),
		AffectedRows: 1,
		IsMutation:   false,
	})
}

func TestShowColumns(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
	defer teardown()

	stmt, err := BuildStatement("SHOW COLUMNS FROM tbl")
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		TableHeader: toTableHeader("Field", "Type", "NULL", "Key", "Key_Order", "Options"),
		Rows: sliceOf(
			toRow("id", "INT64", "NO", "PRIMARY_KEY", "ASC", "NULL"),
			toRow("active", "BOOL", "NO", "NULL", "NULL", "NULL"),
		),
		AffectedRows: 2,
		IsMutation:   false,
	})
}

func TestShowIndexes(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
	defer teardown()

	stmt, err := BuildStatement("SHOW INDEXES FROM tbl")
	if err != nil {
		t.Fatalf("invalid statement: error=%s", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("unexpected error happened: %s", err)
	}

	compareResult(t, result, &Result{
		TableHeader: toTableHeader("Table", "Parent_table", "Index_name", "Index_type", "Is_unique", "Is_null_filtered", "Index_state"),
		Rows: sliceOf(
			toRow("tbl", "", "PRIMARY_KEY", "PRIMARY_KEY", "true", "false", "NULL"),
		),
		AffectedRows: 1,
		IsMutation:   false,
	})
}

func TestTruncateTable(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
	defer teardown()

	stmt, err := BuildStatement("TRUNCATE TABLE tbl")
	if err != nil {
		t.Fatalf("invalid statement: %v", err)
	}

	if _, err := stmt.Execute(ctx, session); err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	// We don't use the TRUNCATE TABLE's result since PartitionedUpdate may return estimated affected row counts.
	// Instead, we check if rows are remained in the table.
	var count int64
	countStmt := spanner.NewStatement("SELECT COUNT(*) FROM tbl")
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
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, false)"))
	defer teardown()

	stmt, err := BuildStatement("PARTITIONED UPDATE tbl SET active = true WHERE true")
	if err != nil {
		t.Fatalf("invalid statement: %v", err)
	}

	if _, err := stmt.Execute(ctx, session); err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	selectStmt := spanner.NewStatement("SELECT active FROM tbl")
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
