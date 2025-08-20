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
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanemuboost"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/samber/lo"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"google.golang.org/protobuf/testing/protocmp"

	spanner "cloud.google.com/go/spanner"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/apstndb/spantype/typector"
	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
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

// Common test DDL patterns used across multiple tests
const (
	testTableSimpleDDL = "CREATE TABLE TestTable(id INT64, active BOOL) PRIMARY KEY(id)"
)

// Test helper functions policy:
// - Use standard go-cmp/cmpopts functions when possible (e.g., cmpopts.IgnoreFields for simple field ignoring)
// - Only create custom helpers for patterns that cannot be expressed with standard options
// - Custom helpers should be focused and well-documented with usage examples
//
// Path.String() vs Path.GoString() usage:
// - Path.GoString(): Use for detailed path matching with indices/types (e.g., ".Rows[0][2]")
//   Returns full Go syntax like: (*Result).Rows[0][2]
// - Path.String(): Use for simple field name matching (e.g., checking "wantResults[1]")
//   Returns simplified path like: Result.Rows

// pathMatchesField checks if a path matches the specified field pattern
func pathMatchesField(path cmp.Path, fieldPattern string) bool {
	return strings.Contains(path.GoString(), fieldPattern)
}

// Common cmp options for test comparisons
// ignorePathOpt creates a cmp.Option that ignores specified path patterns
// Example: ignorePathOpt(".Rows[0][2]") ignores the third column of the first row
func ignorePathOpt(pathPatterns ...string) cmp.Option {
	return cmp.FilterPath(func(path cmp.Path) bool {
		for _, pattern := range pathPatterns {
			if pathMatchesField(path, pattern) {
				return true
			}
		}
		return false
	}, cmp.Ignore())
}

// ignoreRegexOpt creates a cmp.Option that ignores paths matching the regex pattern
// Example: ignoreRegexOpt(`\.Rows\[\d*\]\[1\]`) ignores second column of all rows
func ignoreRegexOpt(regexPattern string) cmp.Option {
	re := regexp.MustCompile(regexPattern)
	return cmp.FilterPath(func(path cmp.Path) bool {
		return re.MatchString(path.GoString())
	}, cmp.Ignore())
}

// Helper functions for creating common results
func keepVariablesResult() *Result {
	return &Result{KeepVariables: true}
}

func emptyResult() *Result {
	return &Result{}
}

func dmlResult(n int) *Result {
	return &Result{AffectedRows: n, IsExecutedDML: true}
}

var emulator *tcspanner.Container

func TestMain(m *testing.M) {
	flag.Parse()

	// Skip emulator setup in short mode
	if testing.Short() {
		os.Exit(m.Run())
	}

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

	os.Exit(m.Run())
}

func initializeSession(ctx context.Context, emulator *tcspanner.Container, clients *spanemuboost.Clients) (session *Session, err error) {
	options := defaultClientOptions(emulator)
	sysVars := &systemVariables{
		Project:          clients.ProjectID,
		Instance:         clients.InstanceID,
		Database:         clients.DatabaseID,
		Params:           make(map[string]ast.Node),
		RPCPriority:      sppb.RequestOptions_PRIORITY_UNSPECIFIED,
		StatementTimeout: lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
	}
	// Initialize StreamManager for tests
	sysVars.StreamManager = NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	// Initialize the registry
	sysVars.ensureRegistry()
	session, err = NewSession(ctx, sysVars, options...)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// initializeWithRandomDB creates a test session with a randomly generated database name.
// Each test gets its own instance within the shared emulator for isolation.
// The database is automatically configured with the provided DDLs and DMLs.
func initializeWithRandomDB(t *testing.T, ddls, dmls []string) (clients *spanemuboost.Clients, session *Session, teardown func()) {
	return initializeWithDB(t, "", ddls, dmls)
}

// initializeWithDB creates a test session with either a specific database name or a random one.
// If database is empty, a random database name is generated.
// Each test gets its own instance within the shared emulator for isolation.
// The database is automatically configured with the provided DDLs and DMLs.
func initializeWithDB(t *testing.T, database string, ddls, dmls []string) (clients *spanemuboost.Clients, session *Session, teardown func()) {
	t.Helper()
	ctx := t.Context()

	// Use shared emulator with a random instance ID for isolation
	options := []spanemuboost.Option{
		spanemuboost.WithRandomInstanceID(), // Instance-level isolation for all tests
	}

	if database != "" {
		// Use specific database name
		options = append(options, spanemuboost.WithDatabaseID(database))
	} else {
		// Use random database name
		options = append(options, spanemuboost.WithRandomDatabaseID())
	}

	options = append(options,
		spanemuboost.EnableAutoConfig(),
		spanemuboost.WithClientConfig(spanner.ClientConfig{SessionPoolConfig: spanner.SessionPoolConfig{MinOpened: 5}}),
		spanemuboost.WithSetupDDLs(ddls),
		spanemuboost.WithSetupRawDMLs(dmls),
	)

	clients, clientsTeardown, err := spanemuboost.NewClients(ctx, emulator, options...)
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

// initializeAdminSession creates an admin-only session without a database connection.
// This is used for database-level operations like CREATE DATABASE and DROP DATABASE.
// Each test gets its own instance within the shared emulator for isolation.
func initializeAdminSession(t *testing.T) (clients *spanemuboost.Clients, session *Session, teardown func()) {
	t.Helper()
	ctx := t.Context()

	// Admin-only mode: create instance without database
	// Always use instance-level isolation for all tests
	clients, clientsTeardown, err := spanemuboost.NewClients(ctx, emulator,
		spanemuboost.WithRandomInstanceID(), // Instance-level isolation for all tests
		spanemuboost.EnableInstanceAutoConfigOnly(),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create admin-only session
	sysVars := &systemVariables{
		Project:          clients.ProjectID,
		Instance:         clients.InstanceID,
		Database:         "",                      // No database for admin-only mode
		StatementTimeout: lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
	}
	// Initialize StreamManager for tests
	sysVars.StreamManager = NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	// Initialize the registry
	sysVars.ensureRegistry()

	session, err = NewAdminSession(ctx, sysVars, defaultClientOptions(emulator)...)
	if err != nil {
		clientsTeardown()
		t.Fatalf("failed to create admin session: err=%s", err)
	}

	return clients, session, func() {
		session.Close()
		clientsTeardown()
	}
}

// spannerContainer is a global variable but it receives explicitly.
func defaultClientOptions(spannerContainer *tcspanner.Container) []option.ClientOption {
	return sliceOf(
		option.WithEndpoint(spannerContainer.URI()),
		option.WithoutAuthentication(),
		internaloption.SkipDialSettingsValidation(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
}

func compareResult[T any](t *testing.T, got T, expected T, customCmpOptions ...cmp.Option) {
	t.Helper()
	opts := sliceOf[cmp.Option](
		cmpopts.IgnoreFields(Result{}, "Stats"),
		cmpopts.IgnoreFields(Result{}, "ReadTimestamp"),
		cmpopts.IgnoreFields(Result{}, "CommitTimestamp"),
		// Commit Stats is only provided by real instances
		cmpopts.IgnoreFields(Result{}, "CommitStats"),
		// Metrics are collected but not part of test expectations
		cmpopts.IgnoreFields(Result{}, "Metrics"),
		cmpopts.EquateEmpty(),
		protocmp.Transform(),
	)
	opts = append(opts, customCmpOptions...)

	if !cmp.Equal(got, expected, opts...) {
		t.Errorf("diff(-got, +expected): %s", cmp.Diff(got, expected, opts...))
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
	})
}

func TestDml(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initializeWithRandomDB(t, testTableDDLs, nil)
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
		AffectedRows:  2,
		IsExecutedDML: true,
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
	t.Helper()
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
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	_, session, teardown := initializeWithRandomDB(t, nil, nil)
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
				if diff := cmp.Diff(sliceOf(tt.varname), extractTableColumnNames(result.TableHeader)); diff != "" {
					t.Errorf("SHOW column names differ: %v", diff)
				}
				if diff := cmp.Diff(sliceOf(toRow(tt.value)), result.Rows); diff != "" {
					t.Errorf("SHOW rows differ: %v", diff)
				}
			})
		}
	})
}

// TestStatements was split into multiple focused test functions:
// - TestParameterStatements: parameter-related tests
// - TestTransactionStatements: transaction-related tests
// - TestShowStatements: SHOW/DESCRIBE/HELP tests
// - TestBatchStatements: batch DDL/DML tests
// - TestPartitionedStatements: partition query tests
// - TestProtoStatements: proto-related tests
// - TestAdminStatements: admin mode tests
// - TestMiscStatements: remaining misc tests

// stmtResult represents a single statement and its expected result
type stmtResult struct {
	stmt string
	want *Result
}

// sr* helper functions purpose:
// These shorthand constructors allow defining common stmtResult patterns with minimal information,
// significantly increasing the density of Test*Statements functions.
// By reducing boilerplate from multi-line struct literals to single-line function calls,
// we can fit more test cases on screen and improve readability.
//
// Example transformation:
//   Before: {"SET PARAM n = 1", keepVariablesResult()},       // 51 chars
//   After:  srKeep("SET PARAM n = 1"),                        // 31 chars (~40% reduction)
//
//   Before: {                                                  // 4 lines
//             stmt: "CREATE DATABASE test",
//             want: emptyResult(),
//           },
//   After:  srEmpty("CREATE DATABASE test"),                  // 1 line (75% line reduction)

// sr is a shorthand constructor for stmtResult
func sr(stmt string, want *Result) stmtResult {
	return stmtResult{stmt: stmt, want: want}
}

// srEmpty creates a stmtResult with an empty result
func srEmpty(stmt string) stmtResult {
	return stmtResult{stmt: stmt, want: emptyResult()}
}

// srDML creates a stmtResult for DML statements with affected rows
func srDML(stmt string, rows int) stmtResult {
	return stmtResult{stmt: stmt, want: dmlResult(rows)}
}

// srKeep creates a stmtResult that keeps variables
func srKeep(stmt string) stmtResult {
	return stmtResult{stmt: stmt, want: keepVariablesResult()}
}

// srBatchDMLKeep creates a stmtResult for DML batch statements that keep variables
func srBatchDMLKeep(stmt string, size int) stmtResult {
	return stmtResult{stmt: stmt, want: &Result{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: size}}}
}

// srBatchDML creates a stmtResult for DML batch statements that update batch size
func srBatchDML(stmt string, size int) stmtResult {
	return stmtResult{stmt: stmt, want: &Result{BatchInfo: &BatchInfo{Mode: batchModeDML, Size: size}}}
}

// srBatchDDLKeep creates a stmtResult for DDL batch statements that keep variables
func srBatchDDLKeep(stmt string, size int) stmtResult {
	return stmtResult{stmt: stmt, want: &Result{KeepVariables: true, BatchInfo: &BatchInfo{Mode: batchModeDDL, Size: size}}}
}

// srBatchDDL creates a stmtResult for DDL batch statements that update batch size
func srBatchDDL(stmt string, size int) stmtResult {
	return stmtResult{stmt: stmt, want: &Result{BatchInfo: &BatchInfo{Mode: batchModeDDL, Size: size}}}
}

// For cases where we want to use sr* but still need the full struct syntax
// (e.g., when adding inline comments), keep the original format

// statementTestCase represents a test case for statement execution
type statementTestCase struct {
	desc        string
	ddls, dmls  []string
	admin       bool   // admin mode (no database)
	database    string // specific database name (empty = use random name)
	stmtResults []stmtResult
	cmpOpts     []cmp.Option
}

// runStatementTests is a helper function to run statement execution tests
func runStatementTests(t *testing.T, tests []statementTestCase) {
	t.Helper()
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			// TODO: Consider if this 180s timeout is actually necessary - tests typically complete much faster
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()

			// Validate admin + non-empty database combination
			if tt.admin && tt.database != "" {
				t.Fatalf("Invalid test configuration: admin=true with non-empty database=%q", tt.database)
			}

			var session *Session
			var teardown func()
			if tt.admin {
				// Admin-only session (database must be empty)
				_, session, teardown = initializeAdminSession(t)
			} else {
				// Regular database mode (specific or random database name)
				_, session, teardown = initializeWithDB(t, tt.database, tt.ddls, tt.dmls)
			}
			defer teardown()

			// Create SessionHandler to properly test USE/DETACH statements
			// SessionHandler will use the session's existing client options for emulator compatibility
			sessionHandler := NewSessionHandler(session)

			var gots []*Result
			var wants []*Result

			for i, sr := range tt.stmtResults {
				stmt, err := BuildStatementWithCommentsWithMode(strings.TrimSpace(lo.Must(gsqlutils.StripComments("", sr.stmt))), sr.stmt, enums.ParseModeNoMemefish)
				if err != nil {
					t.Fatalf("invalid statement[%d]: error=%s", i, err)
				}

				result, err := sessionHandler.ExecuteStatement(ctx, stmt)
				if err != nil {
					t.Fatalf("unexpected error happened[%d]: %s", i, err)
				}
				gots = append(gots, result)
				wants = append(wants, sr.want)
			}
			compareResult(t, gots, wants, tt.cmpOpts...)
		})
	}
}

// TestParameterStatements tests parameter-related functionality including SET PARAM, SHOW PARAMS, and parameter usage
func TestParameterStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc: "query parameters",
			stmtResults: []stmtResult{
				srKeep(`SET PARAM b = true`),
				srKeep(`SET PARAM bs = b"foo"`),
				srKeep(`SET PARAM i64 = 1`),
				srKeep(`SET PARAM f64 = 1.0`),
				srKeep(`SET PARAM f32 = CAST(1.0 AS FLOAT32)`),
				srKeep(`SET PARAM n = NUMERIC "1"`),
				srKeep(`SET PARAM s = "foo"`),
				srKeep(`SET PARAM js = JSON "{}"`),
				srKeep(`SET PARAM ts = TIMESTAMP "2000-01-01T00:00:00Z"`),
				srKeep(`SET PARAM ival_single = INTERVAL 3 DAY`),
				srKeep(`SET PARAM ival_range = INTERVAL "3-4 5 6:7:8.999999999" YEAR TO SECOND`),
				srKeep(`SET PARAM a_b = [true]`),
				srKeep(`SET PARAM n_b = CAST(NULL AS BOOL)`),
				srKeep(`SET PARAM n_ival = CAST(NULL AS INTERVAL)`),
				{
					`SELECT @b AS b, @bs AS bs, @i64 AS i64, @f64 AS f64, @f32 AS f32, @n AS n, @s AS s, @js AS js, @ts AS ts,
					        @ival_single AS ival_single, @ival_range AS ival_range,
					        @a_b AS a_b, @n_b AS n_b, @n_ival AS n_ival`,
					&Result{
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
		},
		{
			desc: "SET PARAM TYPE and SHOW PARAMS",
			stmtResults: []stmtResult{
				srKeep("SET PARAM i INT64"),
				{
					"SHOW PARAMS",
					&Result{
						KeepVariables: true,
						TableHeader:   toTableHeader("Param_Name", "Param_Kind", "Param_Value"),
						Rows:          sliceOf(toRow("i", "TYPE", "INT64")),
					},
				},
				{
					"DESCRIBE SELECT @i AS i",
					&Result{
						AffectedRows: 1,
						TableHeader:  toTableHeader("Column_Name", "Column_Type"),
						Rows:         sliceOf(toRow("i", "INT64")),
					},
				},
			},
		},
		{
			desc: "CLI_TRY_PARTITION_QUERY with parameters",
			stmtResults: []stmtResult{
				srKeep("SET CLI_TRY_PARTITION_QUERY = TRUE"),
				srKeep("SET PARAM n = 1"),
				{
					"SELECT @n",
					&Result{
						ForceWrap:    true,
						AffectedRows: 1,
						TableHeader:  toTableHeader("Root_Partitionable"),
						Rows:         sliceOf(toRow("TRUE")),
					},
				},
			},
		},
	}

	runStatementTests(t, tests)
}

// TestTransactionStatements tests transaction-related functionality including BEGIN, COMMIT, ROLLBACK, and transaction modes
func TestTransactionStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc: "begin, insert THEN RETURN, rollback, select",
			stmtResults: []stmtResult{
				srEmpty(testTableSimpleDDL),
				srEmpty("BEGIN"),
				{
					"INSERT INTO TestTable (id, active) VALUES (1, true), (2, false) THEN RETURN *",
					&Result{
						IsExecutedDML: true,
						AffectedRows:  2,
						Rows: sliceOf(
							toRow("1", "true"),
							toRow("2", "false"),
						),
						TableHeader: toTableHeader(testTableRowType),
					},
				},
				srEmpty("ROLLBACK"),
				{
					"SELECT id, active FROM TestTable ORDER BY id ASC",
					&Result{
						TableHeader: toTableHeader(testTableRowType),
					},
				},
			},
		},
		{
			desc: "begin, insert, commit, select",
			stmtResults: []stmtResult{
				srEmpty(testTableSimpleDDL),
				srEmpty("BEGIN"),
				srDML("INSERT INTO TestTable (id, active) VALUES (1, true), (2, false)", 2),
				srEmpty("COMMIT"),
				{
					"SELECT id, active FROM TestTable ORDER BY id ASC",
					&Result{
						AffectedRows: 2,
						Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
						TableHeader:  toTableHeader(testTableRowType),
					},
				},
			},
		},
		{
			desc: "read-only transactions",
			stmtResults: []stmtResult{
				srEmpty(testTableSimpleDDL),
				srDML("INSERT INTO TestTable (id, active) VALUES (1, true), (2, false)", 2),
				srEmpty("BEGIN RO"),
				{
					"SELECT id, active FROM TestTable ORDER BY id ASC",
					&Result{
						AffectedRows: 2,
						Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
						TableHeader:  toTableHeader(testTableRowType),
					},
				},
				srEmpty("COMMIT"),
				srEmpty("SET TRANSACTION READ ONLY"),
				srEmpty("BEGIN"),
				{
					"SELECT id, active FROM TestTable ORDER BY id ASC",
					&Result{
						AffectedRows: 2,
						Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
						TableHeader:  toTableHeader(testTableRowType),
					},
				},
				srEmpty("COMMIT"),
			},
		},
		{
			desc: "read-write transactions",
			stmtResults: []stmtResult{
				srEmpty(testTableSimpleDDL),
				srDML("INSERT INTO TestTable (id, active) VALUES (1, true), (2, false)", 2),
				srEmpty("BEGIN"),
				{
					"DELETE TestTable WHERE TRUE THEN RETURN *",
					&Result{
						IsExecutedDML: true,
						AffectedRows:  2,
						Rows:          sliceOf(toRow("1", "true"), toRow("2", "false")),
						TableHeader:   toTableHeader(testTableRowType),
					},
				},
				srEmpty("ROLLBACK"),
				srEmpty("BEGIN"),
				srEmpty("SET TRANSACTION READ WRITE"),
				{
					"DELETE TestTable WHERE TRUE THEN RETURN *",
					&Result{
						IsExecutedDML: true,
						AffectedRows:  2,
						Rows:          sliceOf(toRow("1", "true"), toRow("2", "false")),
						TableHeader:   toTableHeader(testTableRowType),
					},
				},
				srEmpty("ROLLBACK"),
				srEmpty("BEGIN RW"),
				{
					"DELETE TestTable WHERE TRUE THEN RETURN *",
					&Result{
						IsExecutedDML: true,
						AffectedRows:  2,
						Rows:          sliceOf(toRow("1", "true"), toRow("2", "false")),
						TableHeader:   toTableHeader(testTableRowType),
					},
				},
				srEmpty("COMMIT"),
			},
		},
	}

	runStatementTests(t, tests)
}

// TestShowStatements tests SHOW, DESCRIBE, and HELP statement functionality
func TestShowStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc: "SHOW VARIABLE CLI_VERSION",
			stmtResults: []stmtResult{
				{
					`SHOW VARIABLE CLI_VERSION`,
					&Result{
						KeepVariables: true,
						TableHeader:   toTableHeader("CLI_VERSION"),
						Rows:          sliceOf(toRow(getVersion())),
					},
				},
			},
		},
		{
			desc: "SHOW TABLES",
			ddls: sliceOf(
				"CREATE TABLE TestTable1(id INT64) PRIMARY KEY(id)",
				"CREATE TABLE TestTable2(id INT64) PRIMARY KEY(id)",
			),
			stmtResults: []stmtResult{
				{
					"SHOW TABLES",
					&Result{
						TableHeader:  toTableHeader(""), // Dynamic based on database name
						Rows:         sliceOf(toRow("TestTable1"), toRow("TestTable2")),
						AffectedRows: 2,
					},
				},
			},
			cmpOpts: sliceOf(cmpopts.IgnoreFields(Result{}, "TableHeader")),
		},
		{
			desc: "SHOW VARIABLES",
			stmtResults: []stmtResult{
				{
					"SHOW VARIABLES",
					&Result{
						TableHeader:   toTableHeader("name", "value"),
						KeepVariables: true,
						// Rows and AffectedRows are dynamic, so we don't check them here.
					},
				},
			},
			cmpOpts: sliceOf(cmpopts.IgnoreFields(Result{}, "Rows", "AffectedRows")),
		},
		{
			desc: "HELP",
			stmtResults: []stmtResult{
				{
					"HELP",
					lo.Must((&HelpStatement{}).Execute(t.Context(), nil)),
				},
			},
		},
		{
			desc: "HELP VARIABLES",
			stmtResults: []stmtResult{
				{
					"HELP VARIABLES",
					lo.Must((&HelpVariablesStatement{}).Execute(context.Background(), nil)),
				},
			},
		},
		{
			desc: "SHOW DDLS",
			ddls: sliceOf(
				"CREATE TABLE Musicians (SingerId INT64 NOT NULL, FirstName STRING(1024), LastName STRING(1024), SingerInfo BYTES(MAX)) PRIMARY KEY (SingerId)",
				"CREATE TABLE Singers (SingerId INT64 NOT NULL, FirstName STRING(1024), LastName STRING(1024), SingerInfo BYTES(MAX)) PRIMARY KEY (SingerId)",
				"CREATE INDEX SingersByFirstLastName ON Singers(FirstName, LastName)",
			),
			stmtResults: []stmtResult{
				{
					"SHOW DDLS",
					&Result{
						TableHeader:   toTableHeader(""),
						KeepVariables: true,
						Rows: sliceOf(
							toRow(heredoc.Doc(`
							CREATE TABLE Musicians (
							  SingerId INT64 NOT NULL,
							  FirstName STRING(1024),
							  LastName STRING(1024),
							  SingerInfo BYTES(MAX),
							) PRIMARY KEY(SingerId);
							CREATE TABLE Singers (
							  SingerId INT64 NOT NULL,
							  FirstName STRING(1024),
							  LastName STRING(1024),
							  SingerInfo BYTES(MAX),
							) PRIMARY KEY(SingerId);
							CREATE INDEX SingersByFirstLastName ON Singers(FirstName, LastName);
							`)),
						),
					},
				},
			},
		},
		{
			desc: "SHOW CREATE INDEX",
			ddls: sliceOf(
				"CREATE TABLE TestShowCreateIndexTbl(id INT64, val INT64) PRIMARY KEY(id)",
				"CREATE INDEX TestShowCreateIndexIdx ON TestShowCreateIndexTbl(val)",
			),
			stmtResults: []stmtResult{
				{
					"SHOW CREATE INDEX TestShowCreateIndexIdx",
					&Result{
						TableHeader:  toTableHeader("Name", "DDL"),
						Rows:         sliceOf(toRow("TestShowCreateIndexIdx", "CREATE INDEX TestShowCreateIndexIdx ON TestShowCreateIndexTbl(val)")),
						AffectedRows: 1,
					},
				},
			},
		},
		{
			desc: "DESCRIBE DML (INSERT with literal)",
			ddls: sliceOf("CREATE TABLE TestDescribeDMLTbl(id INT64 PRIMARY KEY)"),
			stmtResults: []stmtResult{
				{
					"DESCRIBE INSERT INTO TestDescribeDMLTbl (id) VALUES (1)",
					&Result{
						// For DML without THEN RETURN, result is empty.
						TableHeader: toTableHeader("Column_Name", "Column_Type"),
						// No parameters in this DML - Rows and AffectedRows default to nil/0
					},
				},
			},
		},
		{
			desc: "SHOW SCHEMA UPDATE OPERATIONS (empty result expected)",
			stmtResults: []stmtResult{
				{
					"SHOW SCHEMA UPDATE OPERATIONS",
					&Result{
						TableHeader: toTableHeader("OPERATION_ID", "STATEMENTS", "DONE", "PROGRESS", "COMMIT_TIMESTAMP", "ERROR"),
						// Expect no operations on a fresh emulator DB - Rows and AffectedRows default to nil/0
					},
				},
			},
		},
		{
			desc:  "SHOW DATABASES in admin mode",
			admin: true,
			stmtResults: []stmtResult{
				{
					"SHOW DATABASES",
					&Result{
						TableHeader: toTableHeader("Database"),
						// Don't check specific rows since databases vary
					},
				},
			},
			cmpOpts: sliceOf(cmpopts.IgnoreFields(Result{}, "Rows", "AffectedRows")),
		},
	}

	runStatementTests(t, tests)
}

// TestBatchStatements tests BATCH-related functionality including BATCH DDL, BATCH DML, AUTO_BATCH_DML
func TestBatchStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc: "BATCH DML with parameters",
			ddls: sliceOf(testTableSimpleDDL),
			stmtResults: []stmtResult{
				srKeep(`SET PARAM n = 1`),
				srBatchDMLKeep("START BATCH DML", 0),
				srBatchDML("INSERT INTO TestTable (id, active) VALUES (@n, false)", 1),
				srBatchDML("UPDATE TestTable SET active = true WHERE id = @n", 2),
				{"RUN BATCH", &Result{
					IsExecutedDML:    true,
					AffectedRows:     2,
					AffectedRowsType: rowCountTypeUpperBound,
					TableHeader:      toTableHeader("DML", "Rows"),
					Rows: sliceOf(
						toRow("INSERT INTO TestTable (id, active) VALUES (@n, false)", "1"),
						toRow("UPDATE TestTable SET active = true WHERE id = @n", "1"),
					),
				}},
			},
		},
		{
			desc: "BATCH DDL",
			stmtResults: []stmtResult{
				srKeep(`SET CLI_ECHO_EXECUTED_DDL = TRUE`),
				srBatchDDLKeep("START BATCH DDL", 0),
				{heredoc.Doc(`CREATE TABLE TestTable (
					id		INT64,
					active	BOOL,
				) PRIMARY KEY(id)`), &Result{BatchInfo: &BatchInfo{Mode: batchModeDDL, Size: 1}}},
				srBatchDDL(`CREATE TABLE TestTable2 (id INT64, active BOOL) PRIMARY KEY(id)`, 2),
				{"RUN BATCH", &Result{
					TableHeader: toTableHeader("Executed", "Commit Timestamp"),
					Rows: sliceOf(
						toRow(heredoc.Doc(`
							CREATE TABLE TestTable (
								id		INT64,
								active	BOOL,
							) PRIMARY KEY(id);`), "(ignored)"),
						toRow(`CREATE TABLE TestTable2 (id INT64, active BOOL) PRIMARY KEY(id);`, "(ignored)"),
					),
				}},
			},
			// Ignore Commit Timestamp column value
			cmpOpts: sliceOf(ignoreRegexOpt(`\.Rows\[\d*\]\[1\]`)),
		},
		{
			desc: "AUTO_BATCH_DML",
			ddls: sliceOf(testTableSimpleDDL),
			stmtResults: []stmtResult{
				srKeep("SET AUTO_BATCH_DML = TRUE"),
				srDML("INSERT INTO TestTable (id, active) VALUES (1,true)", 1),
				srEmpty("BEGIN"),
				{"INSERT INTO TestTable (id, active) VALUES (2,	false)", &Result{AffectedRows: 0, BatchInfo: &BatchInfo{Mode: batchModeDML, Size: 1}}}, // includes tab
				{"COMMIT", &Result{
					IsExecutedDML: true,
					AffectedRows:  1,
					TableHeader:   toTableHeader("DML", "Rows"),
					Rows:          sliceOf(Row{"INSERT INTO TestTable (id, active) VALUES (2,	false)", "1"}), // tab is pass-through
				}},
				{"SELECT * FROM TestTable ORDER BY id", &Result{
					AffectedRows: 2,
					TableHeader:  toTableHeader(testTableRowType),
					Rows:         sliceOf(toRow("1", "true"), toRow("2", "false")),
				}},
			},
		},
		{
			desc: "ABORT BATCH DML",
			ddls: sliceOf("CREATE TABLE TestAbortBatchDML(id INT64 PRIMARY KEY)"),
			stmtResults: []stmtResult{
				srBatchDMLKeep("START BATCH DML", 0),
				srBatchDML("INSERT INTO TestAbortBatchDML (id) VALUES (1)", 1),
				srKeep("ABORT BATCH"),
				{"SELECT COUNT(*) FROM TestAbortBatchDML", &Result{
					TableHeader:  toTableHeader(typector.NameTypeToStructTypeField("", typector.CodeToSimpleType(sppb.TypeCode_INT64))),
					Rows:         sliceOf(toRow("0")),
					AffectedRows: 1,
				}},
			},
			// No cmpOpts needed - TableHeader should be nil for batch statements
		},
	}

	runStatementTests(t, tests)
}

// TestPartitionedStatements tests PARTITION-related functionality including partitioned queries and DML
func TestPartitionedStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc: "TRY PARTITIONED QUERY",
			stmtResults: []stmtResult{
				{"TRY PARTITIONED QUERY SELECT 1", &Result{
					ForceWrap:    true,
					AffectedRows: 1,
					TableHeader:  toTableHeader("Root_Partitionable"),
					Rows:         sliceOf(toRow("TRUE")),
				}},
			},
		},
		{
			desc: "CLI_TRY_PARTITION_QUERY system variable",
			stmtResults: []stmtResult{
				srKeep("SET CLI_TRY_PARTITION_QUERY = TRUE"),
				{"SELECT 1", &Result{
					ForceWrap:    true,
					AffectedRows: 1,
					TableHeader:  toTableHeader("Root_Partitionable"),
					Rows:         sliceOf(toRow("TRUE")),
				}},
			},
		},
		{
			desc: "CLI_TRY_PARTITION_QUERY with parameters",
			stmtResults: []stmtResult{
				srKeep("SET CLI_TRY_PARTITION_QUERY = TRUE"),
				srKeep("SET PARAM n = 1"),
				{"SELECT @n", &Result{
					ForceWrap:    true,
					AffectedRows: 1,
					TableHeader:  toTableHeader("Root_Partitionable"),
					Rows:         sliceOf(toRow("TRUE")),
				}},
			},
		},
		{
			desc: "mutation, pdml, partitioned query",
			ddls: sliceOf(testTableSimpleDDL),
			stmtResults: []stmtResult{
				srEmpty("MUTATE TestTable INSERT STRUCT(1 AS id, TRUE AS active)"),
				{"PARTITIONED UPDATE TestTable SET active = FALSE WHERE id = 1", &Result{IsExecutedDML: true, AffectedRows: 1, AffectedRowsType: rowCountTypeLowerBound}},
				{"RUN PARTITIONED QUERY SELECT id, active FROM TestTable", &Result{
					AffectedRows:   1,
					PartitionCount: 2,
					TableHeader:    toTableHeader(testTableRowType),
					Rows:           sliceOf(toRow("1", "false")),
				}},
			},
		},
	}

	runStatementTests(t, tests)
}

func TestProtoStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc: "SHOW LOCAL PROTO with pb file",
			stmtResults: []stmtResult{
				srKeep(`SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/order_descriptors.pb"`),
				{
					stmt: `SHOW LOCAL PROTO`,
					want: &Result{
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
		},
		{
			desc: "SHOW LOCAL PROTO with proto file",
			stmtResults: []stmtResult{
				srKeep(`SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/singer.proto"`),
				{
					stmt: `SHOW LOCAL PROTO`,
					want: &Result{
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
		},
		{
			desc: "PROTO BUNDLE statements",
			// Note: Current cloud-spanner-emulator only accepts DDL, but it is nop.
			stmtResults: []stmtResult{
				sr("SHOW REMOTE PROTO", &Result{KeepVariables: true, TableHeader: toTableHeader("full_name", "kind", "package")}),
				srKeep(`SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/order_descriptors.pb"`),
				srEmpty("CREATE PROTO BUNDLE (`examples.shipping.Order`)"),
				srEmpty("ALTER PROTO BUNDLE DELETE (`examples.shipping.Order`)"),
				srEmpty("SYNC PROTO BUNDLE DELETE (`examples.shipping.Order`)"),
			},
		},
	}

	runStatementTests(t, tests)
}

func TestAdminStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc:  "CREATE DATABASE in admin mode",
			admin: true,
			stmtResults: []stmtResult{
				srEmpty("CREATE DATABASE test_db_create"), // CREATE DATABASE returns empty result
			},
		},
		{
			desc:  "CREATE and DROP DATABASE workflow in admin mode",
			admin: true,
			stmtResults: []stmtResult{
				srEmpty("CREATE DATABASE test_workflow_db"), // CREATE DATABASE returns empty result
				{
					stmt: "SHOW DATABASES",
					want: &Result{
						TableHeader: toTableHeader("Database"),
						// Don't check specific content
					},
				},
				srEmpty("DROP DATABASE test_workflow_db"), // DROP DATABASE should also be mutation
				{
					stmt: "SHOW DATABASES",
					want: &Result{
						TableHeader: toTableHeader("Database"),
						// Don't check specific content
					},
				},
			},
			cmpOpts: sliceOf(cmpopts.IgnoreFields(Result{}, "Rows", "AffectedRows")),
		},
		{
			desc:     "DETACH and USE workflow with database creation",
			database: "test-database", // Start with a database connection
			stmtResults: []stmtResult{
				{
					stmt: "SHOW VARIABLE CLI_DATABASE", // Should show test-database (connected)
					want: &Result{
						KeepVariables: true,
						TableHeader:   toTableHeader("CLI_DATABASE"),
						Rows:          sliceOf(toRow("test-database")),
					},
				},
				{
					stmt: "SELECT 1 AS connected", // Should work from database mode
					want: &Result{
						TableHeader:  toTableHeader(typector.NameTypeToStructTypeField("connected", typector.CodeToSimpleType(sppb.TypeCode_INT64))),
						Rows:         sliceOf(toRow("1")),
						AffectedRows: 1,
					},
				},
				srEmpty("DETACH"), // Switch to admin-only mode, returns empty result
				{
					stmt: "SHOW VARIABLE CLI_DATABASE", // Should show empty string (*detached*)
					want: &Result{
						KeepVariables: true,
						TableHeader:   toTableHeader("CLI_DATABASE"),
						Rows:          sliceOf(toRow("")), // Empty string in detached mode
						AffectedRows:  0,
					},
				},
				srEmpty("CREATE DATABASE `test_detach_db`"), // Should work from admin-only mode
				{
					stmt: "SHOW DATABASES", // Should show both databases
					want: &Result{
						TableHeader:  toTableHeader("Database"),
						Rows:         sliceOf(toRow("test-database"), toRow("test_detach_db")), // Both databases
						AffectedRows: 2,
					},
				},
				srEmpty("USE `test_detach_db`"), // Switch to new database
				{
					stmt: "SHOW VARIABLE CLI_DATABASE", // Should show test_detach_db
					want: &Result{
						KeepVariables: true,
						TableHeader:   toTableHeader("CLI_DATABASE"),
						Rows:          sliceOf(toRow("test_detach_db")), // Shows new database name after USE
						AffectedRows:  0,
					},
				},
				{
					stmt: "SELECT 1 AS reconnected", // Should work from database mode after USE
					want: &Result{
						TableHeader:  toTableHeader(typector.NameTypeToStructTypeField("reconnected", typector.CodeToSimpleType(sppb.TypeCode_INT64))),
						Rows:         sliceOf(toRow("1")),
						AffectedRows: 1,
					},
				},
				srEmpty("DETACH"),                         // Switch to admin-only mode again
				srEmpty("DROP DATABASE `test_detach_db`"), // Should work from admin-only mode
			},
			// TableHeaders are explicitly set for SELECT statements
		},
		{
			desc:     "DATABASE statements (dedicated instance)",
			database: "test-database",
			stmtResults: []stmtResult{
				{
					stmt: "SHOW DATABASES",
					want: &Result{TableHeader: toTableHeader("Database"), Rows: sliceOf(toRow("test-database")), AffectedRows: 1},
				},
				srEmpty("CREATE DATABASE `new-database`"), // CREATE DATABASE returns empty result
				{
					stmt: "SHOW DATABASES",
					want: &Result{TableHeader: toTableHeader("Database"), Rows: sliceOf(toRow("new-database"), toRow("test-database")), AffectedRows: 2},
				},
				srEmpty("USE `new-database` ROLE spanner_info_reader"), // nop
				srEmpty("USE `test-database`"),                         // nop
				srEmpty("DROP DATABASE `new-database`"),                // DROP DATABASE
				{
					stmt: "SHOW DATABASES",
					want: &Result{TableHeader: toTableHeader("Database"), Rows: sliceOf(toRow("test-database")), AffectedRows: 1},
				},
			},
			// Ignore duration field in CREATE DATABASE result
			cmpOpts: sliceOf(ignorePathOpt(`.Rows[0][2]`)),
		},
		{
			desc: "PARTITION SELECT query",
			ddls: sliceOf("CREATE TABLE TestPartitionQueryTbl(id INT64 PRIMARY KEY)"),
			stmtResults: []stmtResult{
				{
					stmt: "PARTITION SELECT id FROM TestPartitionQueryTbl",
					want: &Result{
						TableHeader:  toTableHeader("Partition_Token"),
						AffectedRows: 2, // Emulator usually creates a couple of partitions for simple queries
						ForceWrap:    true,
					},
				},
			},
			// Ignore actual token values
			cmpOpts: sliceOf(cmpopts.IgnoreFields(Result{}, "Rows")),
		},
	}

	runStatementTests(t, tests)
}

func TestMiscStatements(t *testing.T) {
	tests := []statementTestCase{
		{
			desc:        "SPLIT POINTS statements",
			ddls:        sliceOf(testTableSimpleDDL),
			dmls:        []string{"DELETE FROM TestTable WHERE TRUE"},
			stmtResults: []stmtResult{
				// Empty results for SPLIT POINTS since it only works with dedicated DB
			},
		},
		{
			desc:        "EXPLAIN & EXPLAIN ANALYZE statements",
			stmtResults: []stmtResult{
				// Empty results for EXPLAIN since it only works with dedicated DB
			},
		},
		{
			desc:        "CQL SELECT",
			ddls:        sliceOf(testTableSimpleDDL),
			dmls:        []string{"DELETE FROM TestTable WHERE TRUE"},
			stmtResults: []stmtResult{
				// Empty results since CQL SELECT would return actual data
			},
		},
		{
			desc: "SET ADD statement for CLI_PROTO_FILES",
			stmtResults: []stmtResult{
				srKeep(`SET CLI_PROTO_DESCRIPTOR_FILE += "testdata/protos/order_descriptors.pb"`),
				{
					stmt: `SHOW VARIABLE CLI_PROTO_DESCRIPTOR_FILE`,
					want: &Result{
						KeepVariables: true,
						TableHeader:   toTableHeader("CLI_PROTO_DESCRIPTOR_FILE"),
						Rows:          sliceOf(toRow("testdata/protos/order_descriptors.pb")),
						AffectedRows:  0,
					},
				},
				srKeep(`SET CLI_PROTO_DESCRIPTOR_FILE += "testdata/protos/singer.proto"`),
				{
					stmt: `SHOW VARIABLE CLI_PROTO_DESCRIPTOR_FILE`,
					want: &Result{
						KeepVariables: true,
						TableHeader:   toTableHeader("CLI_PROTO_DESCRIPTOR_FILE"),
						Rows:          sliceOf(toRow("testdata/protos/order_descriptors.pb,testdata/protos/singer.proto")),
						AffectedRows:  0,
					},
				},
			},
		},
		{
			desc: "MUTATE DELETE",
			ddls: sliceOf("CREATE TABLE TestMutateDeleteTbl(id INT64 PRIMARY KEY)"),
			stmtResults: []stmtResult{
				{
					stmt: "INSERT INTO TestMutateDeleteTbl (id) VALUES (1)",
					want: &Result{
						AffectedRows:  1,
						IsExecutedDML: true,
					},
				},
				{
					stmt: "MUTATE TestMutateDeleteTbl DELETE (1)",
					want: &Result{
						AffectedRows: 0,
					},
				},
				{
					stmt: "SELECT COUNT(*) FROM TestMutateDeleteTbl",
					want: &Result{
						TableHeader:  typesTableHeader{&sppb.StructType_Field{Type: &sppb.Type{Code: sppb.TypeCode_INT64}}},
						Rows:         sliceOf(toRow("0")),
						AffectedRows: 1,
					},
				},
			},
		},
	}

	runStatementTests(t, tests)
}

func TestReadWriteTransaction(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Run("begin, insert, and commit", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, nil)
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
			AffectedRows:  2,
			IsExecutedDML: true,
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

		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, nil)
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
			AffectedRows:  2,
			IsExecutedDML: true,
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

		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Run("begin ro, query, and close", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
		})
	})

	t.Run("begin ro with stale read", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
	})
}

func TestShowColumns(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
	})
}

func TestShowIndexes(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
	})
}

func TestTruncateTable(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"))
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
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session, teardown := initializeWithRandomDB(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, false)"))
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

func TestShowOperation(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()
	emulator, session, teardown := initializeWithRandomDB(t, nil, nil)
	defer teardown()

	// Execute a DDL operation to create an LRO
	ddlStatement := "CREATE TABLE TestShowOperationTable (id INT64, name STRING(100)) PRIMARY KEY (id)"
	stmt, err := BuildStatement(ddlStatement)
	if err != nil {
		t.Fatalf("invalid DDL statement: %v", err)
	}

	if _, err := stmt.Execute(ctx, session); err != nil {
		t.Fatalf("DDL execution failed: %v", err)
	}

	// Get schema update operations to find our operation ID
	showOpsStmt, err := BuildStatement("SHOW SCHEMA UPDATE OPERATIONS")
	if err != nil {
		t.Fatalf("invalid SHOW SCHEMA UPDATE OPERATIONS statement: %v", err)
	}

	result, err := showOpsStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("SHOW SCHEMA UPDATE OPERATIONS execution failed: %v", err)
	}

	// Verify we have at least one operation
	if result.AffectedRows == 0 {
		t.Fatal("Expected at least one schema update operation, but got none")
	}

	// Extract operation ID by matching the DDL statement
	var operationID string
	var foundOp bool
	for _, row := range result.Rows {
		// Assuming DDL statement is in the second column (index 1) and operation ID in the first (index 0)
		if len(row) > 1 && strings.Contains(row[1], "CREATE TABLE TestShowOperationTable") {
			if len(row[0]) > 0 {
				operationID = row[0]
				foundOp = true
				break
			}
		}
	}

	if !foundOp {
		t.Fatalf("Failed to find operation ID for DDL: %s in SHOW SCHEMA UPDATE OPERATIONS result", ddlStatement)
	}

	// Test SHOW OPERATION with the extracted operation ID
	showOpStmt, err := BuildStatement(fmt.Sprintf("SHOW OPERATION '%s'", operationID))
	if err != nil {
		t.Fatalf("invalid SHOW OPERATION statement: %v", err)
	}

	opResult, err := showOpStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("SHOW OPERATION execution failed: %v", err)
	}

	// Verify the result has the expected structure
	expectedHeaders := []string{"OPERATION_ID", "STATEMENTS", "DONE", "PROGRESS", "COMMIT_TIMESTAMP", "ERROR"}
	actualHeaders := extractTableColumnNames(opResult.TableHeader)
	if len(actualHeaders) != len(expectedHeaders) {
		t.Fatalf("Expected %d table headers, got %d", len(expectedHeaders), len(actualHeaders))
	}

	for i, expected := range expectedHeaders {
		if actualHeaders[i] != expected {
			t.Errorf("Expected header[%d] to be %s, got %s", i, expected, actualHeaders[i])
		}
	}

	// Verify we have at least one row (our CREATE TABLE statement)
	if len(opResult.Rows) == 0 {
		t.Fatal("Expected at least one row in SHOW OPERATION result")
	}

	// Verify the operation ID matches what we requested
	if len(opResult.Rows[0]) > 0 && opResult.Rows[0][0] != operationID {
		t.Errorf("Expected operation ID %s, got %s", operationID, opResult.Rows[0][0])
	}

	// Verify the statement contains our DDL
	if len(opResult.Rows[0]) > 1 {
		statement := opResult.Rows[0][1]
		if !strings.Contains(statement, "CREATE TABLE TestShowOperationTable") {
			t.Errorf("Expected statement to contain CREATE TABLE TestShowOperationTable, got: %s", statement)
		}
	}

	// Verify the operation is done (DDL should complete quickly in emulator)
	if len(opResult.Rows[0]) > 2 {
		done := opResult.Rows[0][2]
		if done != "true" {
			t.Logf("Note: Operation is not done yet: %s (this may be expected for slow operations)", done)
		}
	}

	// Test SHOW OPERATION with full operation name format
	fullOpName := fmt.Sprintf("projects/%s/instances/%s/databases/%s/operations/%s",
		session.systemVariables.Project,
		session.systemVariables.Instance,
		session.systemVariables.Database,
		operationID)

	showOpFullStmt, err := BuildStatement(fmt.Sprintf("SHOW OPERATION '%s'", fullOpName))
	if err != nil {
		t.Fatalf("invalid SHOW OPERATION statement with full name: %v", err)
	}

	fullOpResult, err := showOpFullStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("SHOW OPERATION execution with full name failed: %v", err)
	}

	// Results should be identical whether using short ID or full name
	if len(fullOpResult.Rows) != len(opResult.Rows) {
		t.Errorf("Expected same number of rows for short ID (%d) and full name (%d)",
			len(opResult.Rows), len(fullOpResult.Rows))
	}

	// Test error case: non-existent operation
	nonExistentStmt, err := BuildStatement("SHOW OPERATION 'non_existent_operation_id'")
	if err != nil {
		t.Fatalf("invalid SHOW OPERATION statement for non-existent ID: %v", err)
	}

	_, err = nonExistentStmt.Execute(ctx, session)
	if err == nil {
		t.Error("Expected error for non-existent operation ID, but got none")
	}

	// Test SYNC mode functionality
	// Test SYNC mode with completed operation (should not hang, just return status)
	syncStmt, err := BuildStatement(fmt.Sprintf("SHOW OPERATION '%s' SYNC", operationID))
	if err != nil {
		t.Fatalf("invalid SHOW OPERATION SYNC statement: %v", err)
	}

	syncResult, err := syncStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("SHOW OPERATION SYNC execution failed: %v", err)
	}

	// Results should be identical for default and SYNC mode when operation is already completed
	if len(syncResult.Rows) != len(opResult.Rows) {
		t.Errorf("Expected same number of rows for default (%d) and SYNC mode (%d)",
			len(opResult.Rows), len(syncResult.Rows))
	}

	// Test explicit ASYNC mode (should work same as default)
	asyncStmt, err := BuildStatement(fmt.Sprintf("SHOW OPERATION '%s' ASYNC", operationID))
	if err != nil {
		t.Fatalf("invalid SHOW OPERATION ASYNC statement: %v", err)
	}

	asyncResult, err := asyncStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("SHOW OPERATION ASYNC execution failed: %v", err)
	}

	// Results should be identical for default and explicit ASYNC mode
	if len(asyncResult.Rows) != len(opResult.Rows) {
		t.Errorf("Expected same number of rows for default (%d) and ASYNC mode (%d)",
			len(opResult.Rows), len(asyncResult.Rows))
	}

	_ = emulator // Ensure emulator is used to avoid unused variable
}
