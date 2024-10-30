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
	"slices"

	"github.com/samber/lo"
	"spheric.cloud/xiter"
)

const (
	delimiterUndefined  = ""
	delimiterHorizontal = ";"
	delimiterVertical   = `\G`
)

type inputStatement struct {
	statement                string
	statementWithoutComments string
	delim                    string
}

func separateInput(input string) ([]inputStatement, error) {
	stmts, err := SeparateInputPreserveCommentsWithStatus("", input)
	return slices.Collect(xiter.Map(slices.Values(stmts), convertStatement)), err
}

func convertStatement(stmt RawStatement) inputStatement {
	stripped, err := stmt.StripComments()
	strippedStmt := lo.Ternary(err != nil, stmt, stripped).Statement
	return inputStatement{statement: stmt.Statement, statementWithoutComments: strippedStmt, delim: stmt.Terminator}
}
