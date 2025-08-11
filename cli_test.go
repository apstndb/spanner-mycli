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
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spantype/typector"
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/apstndb/spanner-mycli/internal/protostruct"
)

func TestBuildCommands(t *testing.T) {
	tests := []struct {
		Desc        string
		Input       string
		Expected    []Statement
		ExpectError bool
	}{
		{Desc: "SELECT", Input: `SELECT * FROM t1;`, Expected: []Statement{&SelectStatement{"SELECT * FROM t1"}}},
		{Desc: "EXIT", Input: `EXIT;`, Expected: []Statement{&ExitStatement{}}},
		{Desc: "CREATE TABLE(Invalid)", Input: `CREATE TABLE t1;`, Expected: []Statement{&BulkDdlStatement{[]string{"CREATE TABLE t1"}}}},
		{
			Desc:  "DDLs",
			Input: `CREATE TABLE t1(pk INT64) PRIMARY KEY(pk); ALTER TABLE t1 ADD COLUMN col INT64; CREATE INDEX i1 ON t1(col); DROP INDEX i1; DROP TABLE t1;`,
			Expected: []Statement{&BulkDdlStatement{[]string{
				"CREATE TABLE t1(pk INT64) PRIMARY KEY(pk)",
				"ALTER TABLE t1 ADD COLUMN col INT64",
				"CREATE INDEX i1 ON t1(col)",
				"DROP INDEX i1",
				"DROP TABLE t1",
			}}},
		},
		{
			Desc: "mixed statements",
			Input: `CREATE TABLE t1 (pk INT64) PRIMARY KEY(pk);
                CREATE TABLE t2 (pk INT64) PRIMARY KEY(pk);
                SELECT * FROM t1;
                DROP TABLE t1;
                DROP TABLE t2;
                SELECT 1;`,
			Expected: []Statement{
				&BulkDdlStatement{
					[]string{
						"CREATE TABLE t1 (pk INT64) PRIMARY KEY(pk)",
						"CREATE TABLE t2 (pk INT64) PRIMARY KEY(pk)",
					},
				},
				&SelectStatement{"SELECT * FROM t1"},
				&BulkDdlStatement{[]string{"DROP TABLE t1", "DROP TABLE t2"}},
				&SelectStatement{"SELECT 1"},
			},
		},
		{
			Desc: "mixed statements with comments",
			Input: `
			CREATE TABLE t1(pk INT64 /* NOT NULL*/, col INT64) PRIMARY KEY(pk);
			INSERT t1(pk/*, col*/) VALUES(1/*, 2*/);
			UPDATE t1 SET col = /* pk + */ col + 1 WHERE TRUE;
			DELETE t1 WHERE TRUE /* AND pk = 1 */;
			SELECT 0x1/**/A`,
			Expected: []Statement{
				&BulkDdlStatement{
					[]string{`
			CREATE TABLE t1(pk INT64 /* NOT NULL*/, col INT64) PRIMARY KEY(pk)`},
				},
				&DmlStatement{"INSERT t1(pk/*, col*/) VALUES(1/*, 2*/)"},
				&DmlStatement{"UPDATE t1 SET col = /* pk + */ col + 1 WHERE TRUE"},
				&DmlStatement{"DELETE t1 WHERE TRUE /* AND pk = 1 */"},
				&SelectStatement{"SELECT 0x1/**/A"},
			},
		},
		{
			Desc: "empty statement",
			// spanner-cli don't permit empty statements.
			Input:       `SELECT 1; /* comment */; SELECT 2`,
			ExpectError: true,
		},
		{
			Desc:        "empty statements",
			Input:       `SELECT 1; /* comment 1 */; /* comment 2 */`,
			ExpectError: true,
		},
		{
			Desc: "comment after semicolon",
			// A comment after the last semicolon is permitted.
			Input: `SELECT 1; /* comment */`,
			Expected: []Statement{
				&SelectStatement{"SELECT 1"},
			},
		},
		{
			Desc: "multi-line string with meta-command-like content",
			Input: `SELECT r"""
\! echo "hoge"
""";`,
			Expected: []Statement{
				&SelectStatement{`SELECT r"""
\! echo "hoge"
"""`},
			},
			ExpectError: false, // Should not error - it's just a string literal
		},
		{
			Desc:        "meta command at start of input",
			Input:       `\! echo test`,
			ExpectError: true, // Meta commands not supported in batch mode
		},
		{
			Desc:        "meta command after SQL statement",
			Input:       `SELECT 1; \! echo test`,
			ExpectError: true, // Meta commands not supported in batch mode
		},
	}

	for _, test := range tests {
		t.Run(test.Desc, func(t *testing.T) {
			got, err := buildCommands(test.Input, enums.ParseModeFallback)
			if test.ExpectError && err == nil {
				t.Errorf("expect error but not error, input: %v", test.Input)
			}
			if !test.ExpectError && err != nil {
				t.Errorf("err: %v, input: %v", err, test.Input)
			}

			if !cmp.Equal(got, test.Expected) {
				t.Errorf("invalid result: %v", cmp.Diff(test.Expected, got))
			}
		})
	}
}

// TODO: Consider test of readline

func TestPrintResult(t *testing.T) {
	t.Run("DisplayModeTable", func(t *testing.T) {
		tests := []struct {
			sysVars     *systemVariables
			desc        string
			result      *Result
			screenWidth int
			input       string
			want        string
		}{
			{
				desc: "DisplayModeTable: simple table",
				sysVars: &systemVariables{
					CLIFormat: enums.DisplayModeTable,
				},
				result: &Result{
					TableHeader: toTableHeader("foo", "bar"),
					Rows: []Row{
						{"1", "2"},
						{"3", "4"},
					},
				},
				want: strings.TrimPrefix(`
+-----+-----+
| foo | bar |
+-----+-----+
| 1   | 2   |
| 3   | 4   |
+-----+-----+
`, "\n"),
			},
			{
				desc: "DisplayModeTableComment: simple table",
				sysVars: &systemVariables{
					CLIFormat: enums.DisplayModeTableComment,
				},
				result: &Result{
					TableHeader: toTableHeader("foo", "bar"),
					Rows: []Row{
						{"1", "2"},
						{"3", "4"},
					},
				},
				want: strings.TrimPrefix(`
/*-----+-----+
 | foo | bar |
 +-----+-----+
 | 1   | 2   |
 | 3   | 4   |
 +-----+-----*/
`, "\n"),
			},
			{
				desc: "DisplayModeTableCommentDetail, echo, verbose, markdown",
				sysVars: &systemVariables{
					CLIFormat:         enums.DisplayModeTableDetailComment,
					EchoInput:         true,
					Verbose:           true,
					MarkdownCodeblock: true,
				},
				input: "SELECT foo, bar\nFROM input",
				result: &Result{
					TableHeader: toTableHeader("foo", "bar"),
					Rows: []Row{
						{"1", "2"},
						{"3", "4"},
					},
				},
				want: "```sql" + `
SELECT foo, bar
FROM input;
/*-----+-----+
 | foo | bar |
 +-----+-----+
 | 1   | 2   |
 | 3   | 4   |
 +-----+-----+
Empty set
*/
` + "```\n",
			},
			{
				desc: "DisplayModeTable: most preceding column name",
				sysVars: &systemVariables{
					CLIFormat: enums.DisplayModeTable,
					Verbose:   true,
				},
				screenWidth: 20,
				result: &Result{
					TableHeader: toTableHeader(typector.MustNameCodeSlicesToStructTypeFields(
						sliceOf("NAME", "LONG_NAME"),
						sliceOf(sppb.TypeCode_STRING, sppb.TypeCode_STRING),
					)),
					Rows: sliceOf(
						toRow("1", "2"),
						toRow("3", "4"),
					),
				},
				want: strings.TrimPrefix(`
+------+-----------+
| NAME | LONG_NAME |
| STRI | STRING    |
| NG   |           |
+------+-----------+
| 1    | 2         |
| 3    | 4         |
+------+-----------+
Empty set
`, "\n"),
			},
			{
				desc: "DisplayModeTable: also respect column type",
				sysVars: &systemVariables{
					CLIFormat: enums.DisplayModeTable,
					Verbose:   true,
				},
				screenWidth: 19,
				result: &Result{
					TableHeader: toTableHeader(typector.MustNameCodeSlicesToStructTypeFields(
						sliceOf("NAME", "LONG_NAME"),
						sliceOf(sppb.TypeCode_STRING, sppb.TypeCode_STRING),
					)),
					Rows: sliceOf(
						toRow("1", "2"),
						toRow("3", "4"),
					),
				},
				want: strings.TrimPrefix(`
+--------+--------+
| NAME   | LONG_N |
| STRING | AME    |
|        | STRING |
+--------+--------+
| 1      | 2      |
| 3      | 4      |
+--------+--------+
Empty set
`, "\n"),
			},
			{
				desc: "DisplayModeTable: also respect column value",
				sysVars: &systemVariables{
					CLIFormat: enums.DisplayModeTable,
					Verbose:   true,
				},
				screenWidth: 25,
				result: &Result{
					TableHeader: toTableHeader(typector.MustNameCodeSlicesToStructTypeFields(
						sliceOf("English", "Japanese"),
						sliceOf(sppb.TypeCode_STRING, sppb.TypeCode_STRING),
					)),
					Rows: sliceOf(
						toRow("Hello World", "こんにちは"),
						toRow("Bye", "さようなら"),
					),
				},
				want: strings.TrimPrefix(`
+----------+------------+
| English  | Japanese   |
| STRING   | STRING     |
+----------+------------+
| Hello Wo | こんにちは |
| rld      |            |
| Bye      | さようなら |
+----------+------------+
Empty set
`, "\n"),
			},
		}
		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				out := &bytes.Buffer{}
				err := printResult(test.sysVars, test.screenWidth, out, test.result, false, test.input)
				if err != nil {
					t.Errorf("printResult() unexpected error: %v", err)
				}

				got := out.String()
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("result differ: %v", diff)
				}
			})
		}
	})

	t.Run("DisplayModeVertical", func(t *testing.T) {
		out := &bytes.Buffer{}
		result := &Result{
			TableHeader: toTableHeader("foo", "bar"),
			Rows: sliceOf(
				toRow("1", "2"),
				toRow("3", "4"),
			),
		}
		err := printResult(&systemVariables{CLIFormat: enums.DisplayModeVertical}, math.MaxInt, out, result, false, "")
		if err != nil {
			t.Errorf("printResult() unexpected error: %v", err)
		}

		expected := strings.TrimPrefix(`
*************************** 1. row ***************************
foo: 1
bar: 2
*************************** 2. row ***************************
foo: 3
bar: 4
`, "\n")

		got := out.String()
		if got != expected {
			t.Errorf("invalid print: expected = %s, but got = %s", expected, got)
		}
	})

	t.Run("DisplayModeTab", func(t *testing.T) {
		out := &bytes.Buffer{}
		result := &Result{
			TableHeader: toTableHeader("foo", "bar"),
			Rows: sliceOf(
				toRow("1", "2"),
				toRow("3", "4"),
			),
		}
		err := printResult(&systemVariables{CLIFormat: enums.DisplayModeTab}, math.MaxInt, out, result, false, "")
		if err != nil {
			t.Errorf("printResult() unexpected error: %v", err)
		}

		expected := "foo\tbar\n" +
			"1\t2\n" +
			"3\t4\n"

		got := out.String()
		if got != expected {
			t.Errorf("invalid print: expected = %s, but got = %s", expected, got)
		}
	})

	t.Run("SkipColumnNames with DisplayModeTable", func(t *testing.T) {
		out := &bytes.Buffer{}
		result := &Result{
			TableHeader: toTableHeader("foo", "bar"),
			Rows: sliceOf(
				toRow("1", "2"),
				toRow("3", "4"),
			),
		}
		err := printResult(&systemVariables{CLIFormat: enums.DisplayModeTable, SkipColumnNames: true}, math.MaxInt, out, result, false, "")
		if err != nil {
			t.Errorf("printResult() unexpected error: %v", err)
		}

		expected := strings.TrimPrefix(`
+---+---+
| 1 | 2 |
| 3 | 4 |
+---+---+
`, "\n")

		got := out.String()
		if got != expected {
			t.Errorf("invalid print: expected = %s, but got = %s", expected, got)
		}
	})

	t.Run("SkipColumnNames with DisplayModeTab", func(t *testing.T) {
		out := &bytes.Buffer{}
		result := &Result{
			TableHeader: toTableHeader("foo", "bar"),
			Rows: sliceOf(
				toRow("1", "2"),
				toRow("3", "4"),
			),
		}
		err := printResult(&systemVariables{CLIFormat: enums.DisplayModeTab, SkipColumnNames: true}, math.MaxInt, out, result, false, "")
		if err != nil {
			t.Errorf("printResult() unexpected error: %v", err)
		}

		expected := "1\t2\n" +
			"3\t4\n"

		got := out.String()
		if got != expected {
			t.Errorf("invalid print: expected = %s, but got = %s", expected, got)
		}
	})
}

func TestResultLine(t *testing.T) {
	timestamp := "2020-04-01T15:00:00.999999999+09:00"
	ts, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		t.Fatalf("unexpected time.Parse error: %v", err)
	}

	for _, tt := range []struct {
		desc    string
		result  *Result
		verbose bool
		want    string
	}{
		{
			desc: "mutation in normal mode",
			result: &Result{
				AffectedRows:  3,
				IsExecutedDML: true,
				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
			},
			verbose: false,
			want:    "Query OK, 3 rows affected (10 msec)\n",
		},
		{
			desc: "mutation in verbose mode (timestamp exist)",
			result: &Result{
				AffectedRows:  3,
				IsExecutedDML: true,
				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
				CommitTimestamp: ts,
			},
			verbose: true,
			want:    fmt.Sprintf("Query OK, 3 rows affected (10 msec)\ncommit_timestamp:     %s\n", timestamp),
		},
		{
			desc: "mutation in verbose mode (both of timestamp and mutation count exist)",
			result: &Result{
				AffectedRows:  3,
				IsExecutedDML: true,
				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
				CommitStats:     &sppb.CommitResponse_CommitStats{MutationCount: 6},
				CommitTimestamp: ts,
			},
			verbose: true,
			want:    fmt.Sprintf("Query OK, 3 rows affected (10 msec)\ncommit_timestamp:     %s\nmutation_count:       6\n", timestamp),
		},
		{
			desc: "mutation in verbose mode (timestamp not exist)",
			result: &Result{
				AffectedRows:  0,
				IsExecutedDML: true,
				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
				CommitStats: &sppb.CommitResponse_CommitStats{}, // Add CommitStats to show "0 rows affected"
			},
			verbose: true,
			want:    "Query OK, 0 rows affected (10 msec)\n",
		},
		{
			desc: "query in normal mode (rows exist)",
			result: &Result{
				TableHeader:  toTableHeader("col1"), // Add TableHeader for query results
				AffectedRows: 3,

				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
			},
			verbose: false,
			want:    "3 rows in set (10 msec)\n",
		},
		{
			desc: "query in normal mode (no rows exist)",
			result: &Result{
				TableHeader:  toTableHeader("col1"), // Add TableHeader for query results
				AffectedRows: 0,

				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
			},
			verbose: false,
			want:    "Empty set (10 msec)\n",
		},
		{
			desc: "query in verbose mode (all stats fields exist)",
			result: &Result{
				TableHeader:  toTableHeader("col1"), // Add TableHeader for query results
				AffectedRows: 3,

				Stats: QueryStats{
					ElapsedTime:                "10 msec",
					CPUTime:                    "5 msec",
					RowsScanned:                "10",
					RowsReturned:               "3",
					DeletedRowsScanned:         "1",
					OptimizerVersion:           "2",
					OptimizerStatisticsPackage: "auto_20210829_05_22_28UTC",
				},
				ReadTimestamp: ts,
			},
			verbose: true,
			want: fmt.Sprintf(`3 rows in set (10 msec)
read_timestamp:       %s
cpu time:             5 msec
rows scanned:         10 rows
deleted rows scanned: 1 rows
optimizer version:    2
optimizer statistics: auto_20210829_05_22_28UTC
`, timestamp),
		},
		{
			desc: "query in verbose mode (only stats fields supported by Cloud Spanner Emulator)",
			result: &Result{
				TableHeader:  toTableHeader("col1"), // Add TableHeader for query results
				AffectedRows: 3,

				Stats: QueryStats{
					ElapsedTime:  "10 msec",
					RowsReturned: "3",
				},
				ReadTimestamp: ts,
			},
			verbose: true,
			want:    fmt.Sprintf("3 rows in set (10 msec)\nread_timestamp:       %s\n", timestamp),
		},
		// New test cases for issue #414
		{
			desc: "SET statement (no result set, not mutation)",
			result: &Result{
				TableHeader: nil, // No result set

				KeepVariables: true,
				Stats: QueryStats{
					ElapsedTime: "1 msec",
				},
			},
			verbose: false,
			want:    "Query OK (1 msec)\n",
		},
		{
			desc: "DDL statement (no result set, not DML)",
			result: &Result{
				TableHeader: nil, // No result set
				Stats: QueryStats{
					ElapsedTime: "100 msec",
				},
			},
			verbose: false,
			want:    "Query OK (100 msec)\n",
		},
		{
			desc: "DML without THEN RETURN (no result set, is mutation, has affected rows)",
			result: &Result{
				TableHeader:   nil, // No result set
				IsExecutedDML: true,
				AffectedRows:  5,
				CommitStats:   &sppb.CommitResponse_CommitStats{MutationCount: 5},
				Stats: QueryStats{
					ElapsedTime: "20 msec",
				},
			},
			verbose: false,
			want:    "Query OK, 5 rows affected (20 msec)\n",
		},
		{
			desc: "DML without THEN RETURN (no result set, is mutation, 0 affected rows)",
			result: &Result{
				TableHeader:   nil, // No result set
				IsExecutedDML: true,
				AffectedRows:  0,
				CommitStats:   &sppb.CommitResponse_CommitStats{MutationCount: 0},
				Stats: QueryStats{
					ElapsedTime: "15 msec",
				},
			},
			verbose: false,
			want:    "Query OK, 0 rows affected (15 msec)\n",
		},
		{
			desc: "MUTATE statement (no result set, not DML, has CommitStats)",
			result: &Result{
				TableHeader:  nil,                                                // No result set
				AffectedRows: 0,                                                  // MUTATE doesn't provide affected rows
				CommitStats:  &sppb.CommitResponse_CommitStats{MutationCount: 3}, // MUTATE has CommitStats
				Stats: QueryStats{
					ElapsedTime: "5 msec",
				},
			},
			verbose: false,
			want:    "Query OK (5 msec)\n",
		},
		{
			desc: "SELECT with results (has result set)",
			result: &Result{
				TableHeader:  toTableHeader("id", "name"), // Has result set
				AffectedRows: 10,

				Stats: QueryStats{
					ElapsedTime: "25 msec",
				},
			},
			verbose: false,
			want:    "10 rows in set (25 msec)\n",
		},
		{
			desc: "SELECT with no results (has result set, empty)",
			result: &Result{
				TableHeader:  toTableHeader("id", "name"), // Has result set
				AffectedRows: 0,

				Stats: QueryStats{
					ElapsedTime: "8 msec",
				},
			},
			verbose: false,
			want:    "Empty set (8 msec)\n",
		},
		{
			desc: "DML with THEN RETURN (has result set, is mutation)",
			result: &Result{
				TableHeader:   toTableHeader("id", "name"), // Has result set from THEN RETURN
				AffectedRows:  3,
				IsExecutedDML: true,
				CommitStats:   &sppb.CommitResponse_CommitStats{MutationCount: 3},
				Stats: QueryStats{
					ElapsedTime: "30 msec",
				},
			},
			verbose: false,
			want:    "3 rows in set (30 msec)\n",
		},
		{
			desc: "DML with THEN RETURN no rows (has result set, is mutation, empty)",
			result: &Result{
				TableHeader:   toTableHeader("id", "name"), // Has result set from THEN RETURN
				AffectedRows:  0,
				IsExecutedDML: true,
				CommitStats:   &sppb.CommitResponse_CommitStats{MutationCount: 0},
				Stats: QueryStats{
					ElapsedTime: "12 msec",
				},
			},
			verbose: false,
			want:    "Empty set (12 msec)\n",
		},
		{
			desc: "SHOW VARIABLES (has result set)",
			result: &Result{
				TableHeader:  toTableHeader("name", "value"), // Has result set
				AffectedRows: 15,

				KeepVariables: true,
				Stats: QueryStats{
					ElapsedTime: "2 msec",
				},
			},
			verbose: false,
			want:    "15 rows in set (2 msec)\n",
		},
		{
			desc: "Partitioned DML (no result set, lower bound affected rows)",
			result: &Result{
				TableHeader:      nil,
				IsExecutedDML:    true,
				AffectedRows:     1000,
				AffectedRowsType: rowCountTypeLowerBound,
				CommitStats:      &sppb.CommitResponse_CommitStats{MutationCount: 1000},
				Stats: QueryStats{
					ElapsedTime: "200 msec",
				},
			},
			verbose: false,
			want:    "Query OK, at least 1000 rows affected (200 msec)\n",
		},
		{
			desc: "Batch DML (no result set, upper bound affected rows)",
			result: &Result{
				TableHeader:      nil,
				IsExecutedDML:    true,
				AffectedRows:     50,
				AffectedRowsType: rowCountTypeUpperBound,
				CommitStats:      &sppb.CommitResponse_CommitStats{MutationCount: 50},
				BatchInfo:        &BatchInfo{Mode: batchModeDML, Size: 3},
				Stats: QueryStats{
					ElapsedTime: "40 msec",
				},
			},
			verbose: false,
			want:    "Query OK, at most 50 rows affected (40 msec) (3 DMLs in batch)\n",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			if got := resultLine(defaultOutputFormat, tt.result, tt.verbose); tt.want != got {
				t.Errorf("resultLine(%v, %v) = %q, but want = %q", tt.result, tt.verbose, got, tt.want)
			}
		})
	}
}

func TestCli_getInterpolatedPrompt(t *testing.T) {
	tests := []struct {
		desc   string
		prompt string

		// sysVars are used to referenced from Session and Cli.
		sysVars *systemVariables

		// session.systemVariables are not needed to be populated because it is populated by sysVars.
		session *Session

		waitingStatus string
		want          string
	}{
		{
			desc:   "basic variable substitution",
			prompt: "Project: %p, Instance: %i, Database: %d",
			sysVars: &systemVariables{
				Project:  "test-project",
				Instance: "test-instance",
				Database: "test-database",
			},
			session: &Session{
				mode: DatabaseConnected,
			},
			want: "Project: test-project, Instance: test-instance, Database: test-database",
		},
		{
			desc:    "transaction status - read-write",
			prompt:  "%t> ",
			sysVars: &systemVariables{},
			session: &Session{
				tc: &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}},
			},
			want: "(rw txn)> ",
		},
		{
			desc:   "transaction status - read-only",
			prompt: "%t> ",
			session: &Session{
				tc: &transactionContext{attrs: transactionAttributes{mode: transactionModeReadOnly}},
			},
			want: "(ro txn)> ",
		},
		{
			desc:   "transaction status - none",
			prompt: "%t> ",
			want:   "> ",
		},
		{
			desc:   "waiting status",
			prompt: "%R> ",
			want:   "  -> ",
		},
		{
			desc:          "waiting status - with multiline comment",
			prompt:        "%R> ",
			waitingStatus: "*/",
			want:          " /*> ",
		},
		{
			desc:   "custom system variable",
			prompt: "Format: %{CLI_FORMAT}",
			sysVars: &systemVariables{
				CLIFormat: enums.DisplayModeTable,
			},
			want: "Format: TABLE",
		},
		{
			desc:   "invalid variable",
			prompt: "Invalid: %{INVALID_VAR}",
			want:   "Invalid: INVALID_VAR{INVALID_VAR}",
		},
		{
			desc:   "escaped percent sign",
			prompt: "Percent: %%",
			want:   "Percent: %",
		},
		{
			desc:   "newline",
			prompt: "Newline: %n",
			want:   "Newline: \n",
		},
		{
			desc:   "database name - when in admin-only mode shows *detached*",
			prompt: "spanner:%d%t> ",
			sysVars: &systemVariables{
				Database: "",
			},
			session: &Session{
				mode: Detached,
			},
			want: "spanner:*detached*> ",
		},
		{
			desc:   "database name - when connected to database shows database name",
			prompt: "spanner:%d%t> ",
			sysVars: &systemVariables{
				Database: "test-database",
			},
			session: &Session{
				mode: DatabaseConnected,
			},
			want: "spanner:test-database> ",
		},
		{
			desc:   "invalid percent sequence - single character",
			prompt: "Invalid: %z",
			want:   "Invalid: %z",
		},
		{
			desc:   "invalid percent sequence - multiple invalid sequences",
			prompt: "Test %x %y %z end",
			want:   "Test %x %y %z end",
		},
		{
			desc:   "mixed valid and invalid percent sequences",
			prompt: "%p %z %i %q %d",
			sysVars: &systemVariables{
				Project:  "proj",
				Instance: "inst",
				Database: "db",
			},
			session: &Session{
				mode: DatabaseConnected,
			},
			want: "proj %z inst %q db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// populate empty value for session and sysVars if not set to avoid nil pointer dereference
			if tt.session == nil {
				tt.session = &Session{}
			}

			if tt.sysVars == nil {
				tt.sysVars = &systemVariables{}
			}

			tt.session.systemVariables = tt.sysVars
			cli := &Cli{
				SessionHandler:  NewSessionHandler(tt.session),
				SystemVariables: tt.sysVars,
				waitingStatus:   tt.waitingStatus,
			}

			got := cli.getInterpolatedPrompt(tt.prompt)
			if got != tt.want {
				t.Errorf("getInterpolatedPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

//go:embed testdata/stats/distributed_cross_apply_stats.json
var dcaStatsJSON []byte

func protojsonUnmarshal[M any, MP interface {
	*M
	proto.Message
}](b []byte) (MP, error) {
	var m M
	var mp MP = &m

	if err := protojson.Unmarshal(b, mp); err != nil {
		return nil, err
	} else {
		return &m, nil
	}
}

func TestRenderPlanTree(t *testing.T) {
	tcases := []struct {
		desc           string
		sysVars        *systemVariables
		resultSetStats *sppb.ResultSetStats
		want           string
	}{
		{
			desc: "PROFILE with ParsedAnalyzeColumns",
			sysVars: &systemVariables{
				ParsedAnalyzeColumns: lo.Must(customListToTableRenderDefs("Rows:{{.Rows.Total}},Scanned:{{.ScannedRows.Total}},Filtered:{{.FilteredRows.Total}}")),
			},
			resultSetStats: lo.Must(protojsonUnmarshal[sppb.ResultSetStats, *sppb.ResultSetStats](dcaStatsJSON)),
			want: `+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
| ID  | Operator <execution_method> (metadata, ...)                                               | Rows | Scanned | Filtered |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row>                                             |   33 |         |          |
|  *1 | +- Distributed Cross Apply <Row>                                                          |   33 |         |          |
|   2 |    +- [Input] Create Batch <Row>                                                          |      |         |          |
|   3 |    |  +- Local Distributed Union <Row>                                                    |    7 |         |          |
|   4 |    |     +- Compute Struct <Row>                                                          |    7 |         |          |
|   5 |    |        +- Index Scan on AlbumsByAlbumTitle <Row> (Full scan, scan_method: Automatic) |    7 |       7 |        0 |
|  11 |    +- [Map] Serialize Result <Row>                                                        |   33 |         |          |
|  12 |       +- Cross Apply <Row>                                                                |   33 |         |          |
|  13 |          +- [Input] Batch Scan on $v2 <Row> (scan_method: Row)                            |    7 |         |          |
|  16 |          +- [Map] Local Distributed Union <Row>                                           |   33 |         |          |
| *17 |             +- Filter Scan <Row> (seekable_key_size: 0)                                   |      |         |          |
|  18 |                +- Index Scan on SongsBySongGenre <Row> (Full scan, scan_method: Row)      |   33 |      63 |       30 |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)

12 rows in set (28.99 msecs)
cpu time:             28.52 msecs
rows scanned:         70 rows
deleted rows scanned: 0 rows
optimizer version:    7
optimizer statistics: auto_20250421_21_29_41UTC
`,
		},
	}
	for _, tcase := range tcases {
		t.Run(tcase.desc, func(t *testing.T) {
			stats := protostruct.DecodeToMap(tcase.resultSetStats.QueryStats)
			result, err := generateExplainAnalyzeResult(tcase.sysVars, tcase.resultSetStats.QueryPlan, stats, enums.ExplainFormatUnspecified, 0)
			if err != nil {
				t.Errorf("shouldn't fail, but: %v", err)
			}

			var sb strings.Builder
			err = printResult(tcase.sysVars, 0, &sb, result, false, "")
			if err != nil {
				t.Errorf("printResult() unexpected error: %v", err)
			}

			if diff := cmp.Diff(tcase.want, sb.String()); diff != "" {
				t.Errorf("result differ: %v", diff)
			}
		})
	}
}

func Test_printError(t *testing.T) {
	tests := []struct {
		desc string
		err  error
		want string
	}{
		{
			desc: "normal error",
			err:  errors.New("some error"),
			want: "ERROR: some error\n",
		},
		{
			desc: "Spanner error with unknown code",
			err:  spanner.ToSpannerError(status.New(codes.Unknown, "some spanner error").Err()),
			want: `ERROR: spanner: code = "Unknown", desc = "rpc error: code = Unknown desc = some spanner error"
`,
		},
		{
			desc: "Spanner error with specific code and unescaped characters",
			err:  spanner.ToSpannerError(status.New(codes.InvalidArgument, `invalid argument: \"foo\" \'bar\' \\baz\\ \nnewline`).Err()),
			want: `ERROR: spanner: code="InvalidArgument", desc: invalid argument: "foo" 'bar' \baz\ 
newline
`,
		},
		{
			desc: "Spanner error with specific code and no unescaped characters",
			err:  spanner.ToSpannerError(status.New(codes.NotFound, `database not found`).Err()),
			want: `ERROR: spanner: code="NotFound", desc: database not found
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			outBuf := &bytes.Buffer{}
			printError(outBuf, tt.err)
			if outBuf.String() != tt.want {
				t.Errorf("printError() got = %q, want %q", outBuf.String(), tt.want)
			}
		})
	}
}

func Test_confirm(t *testing.T) {
	tests := []struct {
		desc     string
		input    string
		expected bool
	}{
		{
			desc:     "user enters yes",
			input:    "yes\n",
			expected: true,
		},
		{
			desc:     "user enters YES",
			input:    "YES\n",
			expected: true,
		},
		{
			desc:     "user enters no",
			input:    "no\n",
			expected: false,
		},
		{
			desc:     "user enters NO",
			input:    "NO\n",
			expected: false,
		},
		{
			desc:     "user enters invalid then yes",
			input:    "maybe\nyes\n",
			expected: true,
		},
		{
			desc:     "user enters invalid then no",
			input:    "invalid\nno\n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			inBuf := strings.NewReader(tt.input)
			outBuf := &bytes.Buffer{}
			got := confirm(inBuf, outBuf, "Do you confirm?")

			if got != tt.expected {
				t.Errorf("confirm() got = %v, want %v", got, tt.expected)
			}
			// Check prompt messages
			expectedPrompt := "Do you confirm? [yes/no] "
			if strings.Contains(tt.input, "invalid") || strings.Contains(tt.input, "maybe") {
				expectedPrompt += "Please answer yes or no: "
			}
			if !strings.HasPrefix(outBuf.String(), expectedPrompt) {
				t.Errorf("Prompt message mismatch: got %q, want prefix %q", outBuf.String(), expectedPrompt)
			}
		})
	}
}

func TestCli_handleExit(t *testing.T) {
	outBuf := &bytes.Buffer{}
	sysVars := &systemVariables{
		StreamManager: NewStreamManager(io.NopCloser(bytes.NewReader(nil)), outBuf, outBuf),
	}
	cli := &Cli{
		SessionHandler:  NewSessionHandler(&Session{}), // Dummy session, Close() is now safe with nil client
		SystemVariables: sysVars,
	}

	exitCode := cli.handleExit()

	if exitCode != exitCodeSuccess {
		t.Errorf("handleExit() exitCode = %d, want %d", exitCode, exitCodeSuccess)
	}
	if outBuf.String() != "" { // Corrected: handleExit itself does not print "Bye\n"
		t.Errorf("OutStream got = %q, want %q", outBuf.String(), "")
	}
}

func TestCli_ExitOnError(t *testing.T) {
	tests := []struct {
		desc         string
		err          error
		wantErrorOut string
	}{
		{
			desc:         "normal error",
			err:          errors.New("some error"),
			wantErrorOut: "ERROR: some error\n",
		},
		{
			desc: "Spanner error with unknown code",
			err:  spanner.ToSpannerError(status.New(codes.Unknown, "some spanner error").Err()),
			wantErrorOut: `ERROR: spanner: code = "Unknown", desc = "rpc error: code = Unknown desc = some spanner error"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			errBuf := &bytes.Buffer{}
			sysVars := &systemVariables{
				StreamManager: NewStreamManager(io.NopCloser(bytes.NewReader(nil)), errBuf, errBuf),
			}
			cli := &Cli{
				SessionHandler:  NewSessionHandler(&Session{}), // Dummy session, Close() is now safe with nil client
				SystemVariables: sysVars,
			}

			exitCode := cli.ExitOnError(tt.err)

			if exitCode != exitCodeError {
				t.Errorf("ExitOnError() exitCode = %d, want %d", exitCode, exitCodeError)
			}
			if errBuf.String() != tt.wantErrorOut {
				t.Errorf("ErrStream got = %q, want %q", errBuf.String(), tt.wantErrorOut)
			}
		})
	}
}

func TestCli_handleSpecialStatements(t *testing.T) {
	tests := []struct {
		desc          string
		stmt          Statement
		currentDB     string
		confirmInput  string // "yes", "no", or "invalid\nyes" etc. for confirm mock
		wantExitCode  int
		wantProcessed bool
		wantOut       string
		wantErrorOut  string
	}{
		{
			desc:          "EXIT statement",
			stmt:          &ExitStatement{},
			wantExitCode:  exitCodeSuccess,
			wantProcessed: true,
			wantOut:       "Bye\n",
		},
		{
			desc:          "DROP DATABASE on current database, user says no",
			stmt:          &DropDatabaseStatement{DatabaseId: "my-db"},
			currentDB:     "my-db",
			confirmInput:  "no\n",
			wantExitCode:  -1,
			wantProcessed: true,
			wantOut:       "ERROR: database \"my-db\" is currently used, it can not be dropped\n",
			wantErrorOut:  "",
		},
		{
			desc:          "DROP DATABASE on current database, user says yes (should still error)",
			stmt:          &DropDatabaseStatement{DatabaseId: "my-db"},
			currentDB:     "my-db",
			confirmInput:  "yes\n", // Even if user says yes, it should still error due to current DB check
			wantExitCode:  -1,
			wantProcessed: true,
			wantOut:       "ERROR: database \"my-db\" is currently used, it can not be dropped\n",
			wantErrorOut:  "",
		},
		{
			desc:          "DROP DATABASE on different database, user says no",
			stmt:          &DropDatabaseStatement{DatabaseId: "other-db"},
			currentDB:     "my-db",
			confirmInput:  "no\n",
			wantExitCode:  -1,
			wantProcessed: true,
			wantOut:       "Database \"other-db\" will be dropped.\nDo you want to continue? [yes/no] ",
		},
		{
			desc:          "DROP DATABASE on different database, user says yes (not processed by this func)",
			stmt:          &DropDatabaseStatement{DatabaseId: "other-db"},
			currentDB:     "my-db",
			confirmInput:  "yes\n",
			wantExitCode:  -1,
			wantProcessed: false, // This statement is not fully processed here, it proceeds to executeStatement
			wantOut:       "Database \"other-db\" will be dropped.\nDo you want to continue? [yes/no] ",
		},
		{
			desc:          "Non-special statement",
			stmt:          &SelectStatement{Query: "SELECT 1"}, // Corrected: use Query field
			wantExitCode:  -1,
			wantProcessed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			sysVars := &systemVariables{Database: tt.currentDB}
			outBuf := &bytes.Buffer{}
			errBuf := &bytes.Buffer{}

			// For DROP DATABASE tests that need confirmation, check if we're testing the confirmation case
			if dropStmt, ok := tt.stmt.(*DropDatabaseStatement); ok && tt.currentDB != dropStmt.DatabaseId {
				// These test cases now expect the error about no TTY
				// since TtyOutStream is nil by default in tests
				tt.wantOut = "ERROR: cannot confirm DROP DATABASE without a TTY for output; stdout is not a terminal\n"
				tt.wantProcessed = true
			}

			// Create StreamManager with the test streams
			sysVars.StreamManager = NewStreamManager(
				io.NopCloser(strings.NewReader(tt.confirmInput)), // InStream for confirm
				outBuf,
				errBuf,
			)
			cli := &Cli{
				SessionHandler:  NewSessionHandler(&Session{systemVariables: sysVars}), // Dummy Session
				SystemVariables: sysVars,
			}

			exitCode, processed := cli.handleSpecialStatements(context.Background(), tt.stmt)

			if exitCode != tt.wantExitCode {
				t.Errorf("handleSpecialStatements() exitCode = %d, want %d", exitCode, tt.wantExitCode)
			}
			if processed != tt.wantProcessed {
				t.Errorf("handleSpecialStatements() processed = %t, want %t", processed, tt.wantProcessed)
			}
			if outBuf.String() != tt.wantOut {
				t.Errorf("OutStream got = %q, want %q", outBuf.String(), tt.wantOut)
			}
			if errBuf.String() != tt.wantErrorOut {
				t.Errorf("ErrStream got = %q, want %q", errBuf.String(), tt.wantErrorOut)
			}
		})
	}
}

func TestCli_PrintResult(t *testing.T) {
	tests := []struct {
		desc        string
		usePager    bool
		result      *Result
		interactive bool
		input       string
		wantOut     string
	}{
		{
			desc:     "UsePager is false, simple result",
			usePager: false,
			result: &Result{
				TableHeader: toTableHeader("col1"),
				Rows:        []Row{{"foo"}},
			},
			interactive: false,
			input:       "SELECT 'foo'",
			wantOut:     "col1\nfoo\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			outBuf := &bytes.Buffer{}
			sysVars := &systemVariables{
				UsePager:      tt.usePager,
				CLIFormat:     enums.DisplayModeTab, // Use TAB format for predictable output
				StreamManager: NewStreamManager(io.NopCloser(bytes.NewReader(nil)), outBuf, outBuf),
			}
			cli := &Cli{
				SystemVariables: sysVars,
			}

			err := cli.PrintResult(80, tt.result, tt.interactive, tt.input, outBuf)
			if err != nil {
				t.Errorf("PrintResult() unexpected error: %v", err)
			}

			got := outBuf.String()
			t.Logf("PrintResult() got = %q, want %q", got, tt.wantOut)
			if got != tt.wantOut {
				t.Errorf("PrintResult() got = %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestCli_PrintBatchError(t *testing.T) {
	tests := []struct {
		desc         string
		err          error
		wantErrorOut string
	}{
		{
			desc:         "normal error",
			err:          errors.New("batch error"),
			wantErrorOut: "ERROR: batch error\n",
		},
		{
			desc: "Spanner error in batch",
			err:  spanner.ToSpannerError(status.New(codes.Internal, "internal batch error").Err()),
			wantErrorOut: `ERROR: spanner: code="Internal", desc: internal batch error
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			errBuf := &bytes.Buffer{}
			sysVars := &systemVariables{
				StreamManager: NewStreamManager(io.NopCloser(bytes.NewReader(nil)), errBuf, errBuf),
			}
			cli := &Cli{
				SystemVariables: sysVars,
			}

			cli.PrintBatchError(tt.err)

			if errBuf.String() != tt.wantErrorOut {
				t.Errorf("PrintBatchError() got = %q, want %q", errBuf.String(), tt.wantErrorOut)
			}
		})
	}
}

func TestCli_parseStatement(t *testing.T) {
	tests := []struct {
		desc          string
		input         *inputStatement
		wantStatement Statement
		wantErr       bool
	}{
		{
			desc: "valid select statement",
			input: &inputStatement{
				statementWithoutComments: "SELECT 1",
				statement:                "SELECT 1;",
			},
			wantStatement: &SelectStatement{Query: "SELECT 1;"},
			wantErr:       false,
		},
		{
			desc: "invalid statement",
			input: &inputStatement{
				statementWithoutComments: "INVALID SYNTAX",
				statement:                "INVALID SYNTAX;",
			},
			wantStatement: nil,
			wantErr:       true,
		},
		{
			desc: "empty statement",
			input: &inputStatement{
				statementWithoutComments: "",
				statement:                "",
			},
			wantStatement: nil,
			wantErr:       true,
		},
		{
			desc: "statement with comments",
			input: &inputStatement{
				statementWithoutComments: "SELECT 1",
				statement:                "SELECT 1; -- comment",
			},
			wantStatement: &SelectStatement{Query: "SELECT 1; -- comment"},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cli := &Cli{
				SystemVariables: &systemVariables{BuildStatementMode: enums.ParseModeFallback},
			}
			got, err := cli.parseStatement(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseStatement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.wantStatement) {
				t.Errorf("parseStatement() got = %v, want %v", got, tt.wantStatement)
			}
		})
	}
}

// TestUpdateResultStatsElapsedTime tests that updateResultStats correctly populates ElapsedTime
func TestUpdateResultStatsElapsedTime(t *testing.T) {
	tests := []struct {
		name            string
		existingElapsed string
		measuredElapsed float64
		expectedElapsed string
	}{
		{
			name:            "Server elapsed time already set (SELECT query)",
			existingElapsed: "5.23 msecs",
			measuredElapsed: 0.1,
			expectedElapsed: "5.23 msecs", // Should keep server-measured time
		},
		{
			name:            "No server elapsed time (non-SELECT or batch)",
			existingElapsed: "",
			measuredElapsed: 0.15,
			expectedElapsed: "0.15 sec", // Should use client-measured time
		},
		{
			name:            "Batch mode timing",
			existingElapsed: "",
			measuredElapsed: 2.5,
			expectedElapsed: "2.50 sec", // Should format with 2 decimal places
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &Cli{
				SystemVariables: &systemVariables{},
			}

			result := &Result{
				Stats: QueryStats{
					ElapsedTime: tt.existingElapsed,
				},
			}

			cli.updateResultStats(result, tt.measuredElapsed)

			if result.Stats.ElapsedTime != tt.expectedElapsed {
				t.Errorf("Expected ElapsedTime to be %q, got %q", tt.expectedElapsed, result.Stats.ElapsedTime)
			}
		})
	}
}

// TestCli_executeSourceFile tests the executeSourceFile method
func TestCli_executeSourceFile(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		expectError   bool
		errorContains string
	}{
		{
			name:          "File with syntax error",
			fileContent:   "INVALID SYNTAX;",
			expectError:   true,
			errorContains: "failed to parse SQL from file",
		},
		{
			name:          "Meta command in file (should error)",
			fileContent:   "SELECT 1;\n\\! echo test;",
			expectError:   true,
			errorContains: "meta commands are not supported in batch mode",
		},
		{
			name:        "Empty file",
			fileContent: "",
			expectError: false, // Empty file should not error
		},
		{
			name:        "File with only comments",
			fileContent: "-- This is a comment\n/* Another comment */",
			expectError: false, // Comments only should not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file
			tmpfile, err := os.CreateTemp("", "test_source_*.sql")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = os.Remove(tmpfile.Name())
			}()

			// Write test content
			if _, err := tmpfile.Write([]byte(tt.fileContent)); err != nil {
				t.Fatal(err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatal(err)
			}

			// Setup session and CLI
			outBuf := &bytes.Buffer{}
			sysVars := &systemVariables{
				BuildStatementMode: enums.ParseModeFallback,
				CLIFormat:          enums.DisplayModeTab,
				StreamManager:      NewStreamManager(io.NopCloser(bytes.NewReader(nil)), outBuf, outBuf),
			}
			session := &Session{systemVariables: sysVars}

			cli := &Cli{
				SessionHandler:  NewSessionHandler(session),
				SystemVariables: sysVars,
			}

			// Execute the source file
			err = cli.executeSourceFile(context.Background(), tmpfile.Name())

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestCli_executeSourceFile_NonExistentFile tests executeSourceFile with a non-existent file
func TestCli_executeSourceFile_NonExistentFile(t *testing.T) {
	cli := &Cli{
		SessionHandler:  NewSessionHandler(&Session{}),
		SystemVariables: &systemVariables{},
	}

	err := cli.executeSourceFile(context.Background(), "/non/existent/file.sql")
	if err == nil {
		t.Error("Expected error for non-existent file")
	} else if !strings.Contains(err.Error(), "failed to open file") {
		t.Errorf("Expected error to contain 'failed to open file', got: %v", err)
	}
}

// TestCli_executeSourceFile_NonRegularFile tests executeSourceFile with a non-regular file
func TestCli_executeSourceFile_NonRegularFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires /dev/null")
	}

	cli := &Cli{
		SessionHandler:  NewSessionHandler(&Session{}),
		SystemVariables: &systemVariables{},
	}

	// Try to source from /dev/null (a special file)
	err := cli.executeSourceFile(context.Background(), "/dev/null")
	if err == nil {
		t.Error("Expected error for non-regular file")
	} else if !strings.Contains(err.Error(), "sourcing from a non-regular file is not supported") {
		t.Errorf("Expected error to contain 'sourcing from a non-regular file is not supported', got: %v", err)
	}
}

// TestCli_executeSourceFile_FileTooLarge tests executeSourceFile with a file that exceeds the size limit
func TestCli_executeSourceFile_FileTooLarge(t *testing.T) {
	// Create a temporary file that simulates a large file
	tmpFile, err := os.CreateTemp(t.TempDir(), "large_file_*.sql")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	// Write a small amount of data but use Truncate to set the file size
	// This avoids actually writing 100MB+ of data
	const largeSize = 101 * 1024 * 1024 // 101MB, just over the limit
	if err := tmpFile.Truncate(largeSize); err != nil {
		t.Fatalf("Failed to truncate file: %v", err)
	}
	tmpFile.Close()

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	sysVars := &systemVariables{
		StreamManager: NewStreamManager(io.NopCloser(bytes.NewReader(nil)), outBuf, errBuf),
	}
	cli := &Cli{
		SessionHandler:  NewSessionHandler(&Session{}),
		SystemVariables: sysVars,
	}

	// Try to source the large file
	err = cli.executeSourceFile(context.Background(), tmpFile.Name())
	if err == nil {
		t.Error("Expected error for file too large")
	} else if !strings.Contains(err.Error(), "is too large to be sourced") {
		t.Errorf("Expected error to contain 'is too large to be sourced', got: %v", err)
	}
}
