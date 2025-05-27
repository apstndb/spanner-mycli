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
					Statement:                `SELECT "123"`,
					StatementWithoutComments: `SELECT "123"`,
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "double queries",
			input: `SELECT "123"; SELECT "456";`,
			want: []inputStatement{
				{
					Statement:                `SELECT "123"`,
					StatementWithoutComments: `SELECT "123"`,
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                `SELECT "456"`,
					StatementWithoutComments: `SELECT "456"`,
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "quoted identifier",
			input: "SELECT `1`, `2`; SELECT `3`, `4`;",
			want: []inputStatement{
				{
					Statement:                "SELECT `1`, `2`",
					StatementWithoutComments: "SELECT `1`, `2`",
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                "SELECT `3`, `4`",
					StatementWithoutComments: "SELECT `3`, `4`",
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "sql query",
			input: `SELECT * FROM t1 WHERE id = "123" AND "456"; DELETE FROM t2 WHERE true;`,
			want: []inputStatement{
				{
					Statement:                `SELECT * FROM t1 WHERE id = "123" AND "456"`,
					StatementWithoutComments: `SELECT * FROM t1 WHERE id = "123" AND "456"`,
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                `DELETE FROM t2 WHERE true`,
					StatementWithoutComments: `DELETE FROM t2 WHERE true`,
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "second query is empty",
			input: `SELECT 1; ;`,
			want: []inputStatement{
				{
					Statement:                `SELECT 1`,
					StatementWithoutComments: `SELECT 1`,
					Delim:                    delimiterHorizontal,
				},
				{
					Statement: ``,
					Delim:     delimiterHorizontal,
				},
			},
		},
		{
			desc:  "new line just after delim",
			input: "SELECT 1;\n SELECT 2;\n",
			want: []inputStatement{
				{
					Statement:                `SELECT 1`,
					StatementWithoutComments: `SELECT 1`,
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                `SELECT 2`,
					StatementWithoutComments: `SELECT 2`,
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "horizontal delimiter in string",
			input: `SELECT "1;2;3"; SELECT 'TL;DR';`,
			want: []inputStatement{
				{
					Statement:                `SELECT "1;2;3"`,
					StatementWithoutComments: `SELECT "1;2;3"`,
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                `SELECT 'TL;DR'`,
					StatementWithoutComments: `SELECT 'TL;DR'`,
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  "delimiter in quoted identifier",
			input: "SELECT `1;2`; SELECT `3;4`;",
			want: []inputStatement{
				{
					Statement:                "SELECT `1;2`",
					StatementWithoutComments: "SELECT `1;2`",
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                "SELECT `3;4`",
					StatementWithoutComments: "SELECT `3;4`",
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  `query has new line just before delimiter`,
			input: "SELECT '123'\n; SELECT '456'\n;",
			want: []inputStatement{
				{
					Statement:                "SELECT '123'\n",
					StatementWithoutComments: "SELECT '123'\n",
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                "SELECT '456'\n",
					StatementWithoutComments: "SELECT '456'\n",
					Delim:                    delimiterHorizontal,
				},
			},
		},
		{
			desc:  `DDL`,
			input: "CREATE t1 (\nId INT64 NOT NULL\n) PRIMARY KEY (Id);",
			want: []inputStatement{
				{
					Statement:                "CREATE t1 (\nId INT64 NOT NULL\n) PRIMARY KEY (Id)",
					StatementWithoutComments: "CREATE t1 (\nId INT64 NOT NULL\n) PRIMARY KEY (Id)",
					Delim:                    delimiterHorizontal,
				},
			},
		},

		{
			desc:  `statement with multiple comments`,
			input: "# comment;\nSELECT /* comment */ 1; --comment\nSELECT 2;/* comment */",
			want: []inputStatement{
				{
					Statement:                "# comment;\nSELECT /* comment */ 1",
					StatementWithoutComments: "SELECT  1",
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                "--comment\nSELECT 2",
					StatementWithoutComments: "SELECT 2",
					Delim:                    delimiterHorizontal,
				},
				{
					Statement: "/* comment */",
				},
			},
		},
		{
			desc:  `only comments`,
			input: "# comment;\n/* comment */--comment\n/* comment */",
			want: []inputStatement{
				{
					Statement: "# comment;\n/* comment */--comment\n/* comment */",
				},
			},
		},
		{
			desc:  `second query ends in the middle of string`,
			input: `SELECT "123"; SELECT "45`,
			want: []inputStatement{
				{
					Statement:                `SELECT "123"`,
					StatementWithoutComments: `SELECT "123"`,
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                `SELECT "45`,
					StatementWithoutComments: `SELECT "45`,
					Delim:                    delimiterUndefined,
				},
			},
			wantAnyErr: true,
		},
		{
			desc:  `totally incorrect query`,
			input: `a"""""""""'''''''''b`,
			want: []inputStatement{
				{
					Statement:                `a"""""""""'''''''''b`,
					StatementWithoutComments: `a"""""""""'''''''''b`,
					Delim:                    delimiterUndefined,
				},
			},
			wantAnyErr: true,
		},
		{
			desc:  `statement with multiple comments`,
			input: "SELECT 0x1/* comment */A; SELECT 0x2--\nB; SELECT 0x3#\nC",
			want: []inputStatement{
				{
					Statement:                "SELECT 0x1/* comment */A",
					StatementWithoutComments: "SELECT 0x1 A",
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                "SELECT 0x2--\nB",
					StatementWithoutComments: "SELECT 0x2\nB",
					Delim:                    delimiterHorizontal,
				},
				{
					Statement:                "SELECT 0x3#\nC",
					StatementWithoutComments: "SELECT 0x3\nC",
					Delim:                    delimiterUndefined,
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
