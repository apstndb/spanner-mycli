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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSeparateInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		desc       string
		input      string
		want       []inputStatement
		wantAnyErr bool
	}{
		{
			desc:  "single query",
			input: `SELECT "123";`,
			want: []inputStatement{
				{
					statement:                `SELECT "123"`,
					statementWithoutComments: `SELECT "123"`,
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "double queries",
			input: `SELECT "123"; SELECT "456";`,
			want: []inputStatement{
				{
					statement:                `SELECT "123"`,
					statementWithoutComments: `SELECT "123"`,
					delim:                    delimiterHorizontal,
				},
				{
					statement:                `SELECT "456"`,
					statementWithoutComments: `SELECT "456"`,
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "quoted identifier",
			input: "SELECT `1`, `2`; SELECT `3`, `4`;",
			want: []inputStatement{
				{
					statement:                "SELECT `1`, `2`",
					statementWithoutComments: "SELECT `1`, `2`",
					delim:                    delimiterHorizontal,
				},
				{
					statement:                "SELECT `3`, `4`",
					statementWithoutComments: "SELECT `3`, `4`",
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "sql query",
			input: `SELECT * FROM t1 WHERE id = "123" AND "456"; DELETE FROM t2 WHERE true;`,
			want: []inputStatement{
				{
					statement:                `SELECT * FROM t1 WHERE id = "123" AND "456"`,
					statementWithoutComments: `SELECT * FROM t1 WHERE id = "123" AND "456"`,
					delim:                    delimiterHorizontal,
				},
				{
					statement:                `DELETE FROM t2 WHERE true`,
					statementWithoutComments: `DELETE FROM t2 WHERE true`,
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "second query is empty",
			input: `SELECT 1; ;`,
			want: []inputStatement{
				{
					statement:                `SELECT 1`,
					statementWithoutComments: `SELECT 1`,
					delim:                    delimiterHorizontal,
				},
				{
					statement: ``,
					delim:     delimiterHorizontal,
				},
			},
		},
		{
			desc:  "new line just after delim",
			input: "SELECT 1;\n SELECT 2;\n",
			want: []inputStatement{
				{
					statement:                `SELECT 1`,
					statementWithoutComments: `SELECT 1`,
					delim:                    delimiterHorizontal,
				},
				{
					statement:                `SELECT 2`,
					statementWithoutComments: `SELECT 2`,
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "horizontal delimiter in string",
			input: `SELECT "1;2;3"; SELECT 'TL;DR';`,
			want: []inputStatement{
				{
					statement:                `SELECT "1;2;3"`,
					statementWithoutComments: `SELECT "1;2;3"`,
					delim:                    delimiterHorizontal,
				},
				{
					statement:                `SELECT 'TL;DR'`,
					statementWithoutComments: `SELECT 'TL;DR'`,
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "delimiter in quoted identifier",
			input: "SELECT `1;2`; SELECT `3;4`;",
			want: []inputStatement{
				{
					statement:                "SELECT `1;2`",
					statementWithoutComments: "SELECT `1;2`",
					delim:                    delimiterHorizontal,
				},
				{
					statement:                "SELECT `3;4`",
					statementWithoutComments: "SELECT `3;4`",
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  `query has new line just before delimiter`,
			input: "SELECT '123'\n; SELECT '456'\n;",
			want: []inputStatement{
				{
					statement:                "SELECT '123'\n",
					statementWithoutComments: "SELECT '123'\n",
					delim:                    delimiterHorizontal,
				},
				{
					statement:                "SELECT '456'\n",
					statementWithoutComments: "SELECT '456'\n",
					delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  `DDL`,
			input: "CREATE t1 (\nId INT64 NOT NULL\n) PRIMARY KEY (Id);",
			want: []inputStatement{
				{
					statement:                "CREATE t1 (\nId INT64 NOT NULL\n) PRIMARY KEY (Id)",
					statementWithoutComments: "CREATE t1 (\nId INT64 NOT NULL\n) PRIMARY KEY (Id)",
					delim:                    delimiterHorizontal,
				},
			},
		},

		{
			desc:  `statement with multiple comments`,
			input: "# comment;\nSELECT /* comment */ 1; --comment\nSELECT 2;/* comment */",
			want: []inputStatement{
				{
					statement:                "# comment;\nSELECT /* comment */ 1",
					statementWithoutComments: "SELECT  1",
					delim:                    delimiterHorizontal,
				},
				{
					statement:                "--comment\nSELECT 2",
					statementWithoutComments: "SELECT 2",
					delim:                    delimiterHorizontal,
				},
				{
					statement: "/* comment */",
				},
			},
		},
		{
			desc:  `only comments`,
			input: "# comment;\n/* comment */--comment\n/* comment */",
			want: []inputStatement{
				{
					statement: "# comment;\n/* comment */--comment\n/* comment */",
				},
			},
		},
		{
			desc:  `second query ends in the middle of string`,
			input: `SELECT "123"; SELECT "45`,
			want: []inputStatement{
				{
					statement:                `SELECT "123"`,
					statementWithoutComments: `SELECT "123"`,
					delim:                    delimiterHorizontal,
				},
				{
					statement:                `SELECT "45`,
					statementWithoutComments: `SELECT "45`,
					delim:                    delimiterUndefined,
				},
			},
			wantAnyErr: true,
		},
		{
			desc:  `totally incorrect query`,
			input: `a"""""""""'''''''''b`,
			want: []inputStatement{
				{
					statement:                `a"""""""""'''''''''b`,
					statementWithoutComments: `a"""""""""'''''''''b`,
					delim:                    delimiterUndefined,
				},
			},
			wantAnyErr: true,
		},
		{
			desc:  `statement with multiple comments`,
			input: "SELECT 0x1/* comment */A; SELECT 0x2--\nB; SELECT 0x3#\nC",
			want: []inputStatement{
				{
					statement:                "SELECT 0x1/* comment */A",
					statementWithoutComments: "SELECT 0x1 A",
					delim:                    delimiterHorizontal,
				},
				{
					statement:                "SELECT 0x2--\nB",
					statementWithoutComments: "SELECT 0x2\nB",
					delim:                    delimiterHorizontal,
				},
				{
					statement:                "SELECT 0x3#\nC",
					statementWithoutComments: "SELECT 0x3\nC",
					delim:                    delimiterUndefined,
				},
			},
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			got, err := separateInput(tt.input)
			if !tt.wantAnyErr && err != nil {
				t.Errorf("should not fail, but: %v", err)
			}
			if tt.wantAnyErr && err == nil {
				t.Errorf("should fail, but success")
			}
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(inputStatement{})); diff != "" {
				t.Errorf("difference in statements: (-want +got):\n%s", diff)
			}
		})
	}
}
