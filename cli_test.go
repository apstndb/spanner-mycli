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
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spantype/typector"
	"github.com/google/go-cmp/cmp"
)

func TestBuildCommands(t *testing.T) {
	tests := []struct {
		Desc        string
		Input       string
		Expected    []*command
		ExpectError bool
	}{
		{Desc: "SELECT", Input: `SELECT * FROM t1;`, Expected: []*command{{&SelectStatement{"SELECT * FROM t1"}}}},
		{Desc: "CREATE TABLE(Invalid)", Input: `CREATE TABLE t1;`, Expected: []*command{{&BulkDdlStatement{[]string{"CREATE TABLE t1"}}}}},
		{Desc: "DDLs",
			Input: `CREATE TABLE t1(pk INT64) PRIMARY KEY(pk); ALTER TABLE t1 ADD COLUMN col INT64; CREATE INDEX i1 ON t1(col); DROP INDEX i1; DROP TABLE t1;`,
			Expected: []*command{{&BulkDdlStatement{[]string{
				"CREATE TABLE t1 (\n  pk INT64\n) PRIMARY KEY (pk)",
				"ALTER TABLE t1 ADD COLUMN col INT64",
				"CREATE INDEX i1 ON t1(col)",
				"DROP INDEX i1",
				"DROP TABLE t1",
			}}}},
		},
		{Desc: "mixed statements",
			Input: `CREATE TABLE t1 (pk INT64) PRIMARY KEY(pk);
                CREATE TABLE t2 (pk INT64) PRIMARY KEY(pk);
                SELECT * FROM t1;
                DROP TABLE t1;
                DROP TABLE t2;
                SELECT 1;`,
			Expected: []*command{
				{
					&BulkDdlStatement{
						[]string{
							"CREATE TABLE t1 (\n  pk INT64\n) PRIMARY KEY (pk)",
							"CREATE TABLE t2 (\n  pk INT64\n) PRIMARY KEY (pk)",
						},
					},
				},
				{&SelectStatement{"SELECT * FROM t1"}},
				{&BulkDdlStatement{[]string{"DROP TABLE t1", "DROP TABLE t2"}}},
				{&SelectStatement{"SELECT 1"}},
			}},
		{Desc: "mixed statements with comments",
			Input: `
			CREATE TABLE t1(pk INT64 /* NOT NULL*/, col INT64) PRIMARY KEY(pk);
			INSERT t1(pk/*, col*/) VALUES(1/*, 2*/);
			UPDATE t1 SET col = /* pk + */ col + 1 WHERE TRUE;
			DELETE t1 WHERE TRUE /* AND pk = 1 */;
			SELECT 0x1/**/A`,
			Expected: []*command{
				{
					&BulkDdlStatement{
						[]string{"CREATE TABLE t1 (\n  pk INT64,\n  col INT64\n) PRIMARY KEY (pk)"},
					},
				},
				{&DmlStatement{"INSERT t1(pk/*, col*/) VALUES(1/*, 2*/)"}},
				{&DmlStatement{"UPDATE t1 SET col = /* pk + */ col + 1 WHERE TRUE"}},
				{&DmlStatement{"DELETE t1 WHERE TRUE /* AND pk = 1 */"}},
				{&SelectStatement{"SELECT 0x1/**/A"}},
			}},
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
			Expected: []*command{
				{&SelectStatement{"SELECT 1"}},
			}},
	}

	for _, test := range tests {
		t.Run(test.Desc, func(t *testing.T) {
			got, err := buildCommands(test.Input, parseModeFallback)
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
					CLIFormat: DisplayModeTable,
				},
				result: &Result{
					ColumnNames: []string{"foo", "bar"},
					Rows: []Row{
						{[]string{"1", "2"}},
						{[]string{"3", "4"}},
					},
					IsMutation: false,
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
					CLIFormat: DisplayModeTableComment,
				},
				result: &Result{
					ColumnNames: []string{"foo", "bar"},
					Rows: []Row{
						{[]string{"1", "2"}},
						{[]string{"3", "4"}},
					},
					IsMutation: false,
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
					CLIFormat:         DisplayModeTableDetailComment,
					EchoInput:         true,
					Verbose:           true,
					MarkdownCodeblock: true,
				},
				input: "SELECT foo, bar\nFROM input",
				result: &Result{
					ColumnNames: []string{"foo", "bar"},
					Rows: []Row{
						{[]string{"1", "2"}},
						{[]string{"3", "4"}},
					},
					IsMutation: false,
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
					CLIFormat: DisplayModeTable,
					Verbose:   true,
				},
				screenWidth: 20,
				result: &Result{
					ColumnTypes: typector.MustNameCodeSlicesToStructTypeFields(
						sliceOf("NAME", "LONG_NAME"),
						sliceOf(sppb.TypeCode_STRING, sppb.TypeCode_STRING),
					),
					Rows: sliceOf(
						toRow("1", "2"),
						toRow("3", "4"),
					),
					IsMutation: false,
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
					CLIFormat: DisplayModeTable,
					Verbose:   true,
				},
				screenWidth: 19,
				result: &Result{
					ColumnTypes: typector.MustNameCodeSlicesToStructTypeFields(
						sliceOf("NAME", "LONG_NAME"),
						sliceOf(sppb.TypeCode_STRING, sppb.TypeCode_STRING),
					),
					Rows: sliceOf(
						toRow("1", "2"),
						toRow("3", "4"),
					),
					IsMutation: false,
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
					CLIFormat: DisplayModeTable,
					Verbose:   true,
				},
				screenWidth: 25,
				result: &Result{
					ColumnTypes: typector.MustNameCodeSlicesToStructTypeFields(
						sliceOf("English", "Japanese"),
						sliceOf(sppb.TypeCode_STRING, sppb.TypeCode_STRING),
					),
					Rows: sliceOf(
						toRow("Hello World", "こんにちは"),
						toRow("Bye", "さようなら"),
					),
					IsMutation: false,
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
				printResult(test.sysVars, test.screenWidth, out, test.result, false, test.input)

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
			ColumnNames: sliceOf("foo", "bar"),
			Rows: sliceOf(
				toRow("1", "2"),
				toRow("3", "4"),
			),
			IsMutation: false,
		}
		printResult(&systemVariables{CLIFormat: DisplayModeVertical}, math.MaxInt, out, result, false, "")

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
			ColumnNames: sliceOf("foo", "bar"),
			Rows: sliceOf(
				toRow("1", "2"),
				toRow("3", "4"),
			),
			IsMutation: false,
		}
		printResult(&systemVariables{CLIFormat: DisplayModeTab}, math.MaxInt, out, result, false, "")

		expected := "foo\tbar\n" +
			"1\t2\n" +
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
				AffectedRows: 3,
				IsMutation:   true,
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
				AffectedRows: 3,
				IsMutation:   true,
				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
				Timestamp: ts,
			},
			verbose: true,
			want:    fmt.Sprintf("Query OK, 3 rows affected (10 msec)\ntimestamp:      %s\n", timestamp),
		},
		{
			desc: "mutation in verbose mode (both of timestamp and mutation count exist)",
			result: &Result{
				AffectedRows: 3,
				IsMutation:   true,
				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
				CommitStats: &sppb.CommitResponse_CommitStats{MutationCount: 6},
				Timestamp:   ts,
			},
			verbose: true,
			want:    fmt.Sprintf("Query OK, 3 rows affected (10 msec)\ntimestamp:      %s\nmutation_count: 6\n", timestamp),
		},
		{
			desc: "mutation in verbose mode (timestamp not exist)",
			result: &Result{
				AffectedRows: 0,
				IsMutation:   true,
				Stats: QueryStats{
					ElapsedTime: "10 msec",
				},
			},
			verbose: true,
			want:    "Query OK, 0 rows affected (10 msec)\n",
		},
		{
			desc: "query in normal mode (rows exist)",
			result: &Result{
				AffectedRows: 3,
				IsMutation:   false,
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
				AffectedRows: 0,
				IsMutation:   false,
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
				AffectedRows: 3,
				IsMutation:   false,
				Stats: QueryStats{
					ElapsedTime:                "10 msec",
					CPUTime:                    "5 msec",
					RowsScanned:                "10",
					RowsReturned:               "3",
					DeletedRowsScanned:         "1",
					OptimizerVersion:           "2",
					OptimizerStatisticsPackage: "auto_20210829_05_22_28UTC",
				},
				Timestamp: ts,
			},
			verbose: true,
			want: fmt.Sprintf(`3 rows in set (10 msec)
timestamp:            %s
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
				AffectedRows: 3,
				IsMutation:   false,
				Stats: QueryStats{
					ElapsedTime:  "10 msec",
					RowsReturned: "3",
				},
				Timestamp: ts,
			},
			verbose: true,
			want:    fmt.Sprintf("3 rows in set (10 msec)\ntimestamp:            %s\n", timestamp),
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			if got := resultLine(tt.result, tt.verbose); tt.want != got {
				t.Errorf("resultLine(%v, %v) = %q, but want = %q", tt.result, tt.verbose, got, tt.want)
			}
		})
	}
}
