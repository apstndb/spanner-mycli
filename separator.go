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

import "log"

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

	var result []inputStatement
	for _, stmt := range stmts {
		stripped, err := stmt.StripComments()
		var stmtWithoutComments string
		if err != nil {
			log.Printf("separateInput error ignored: %v", err)
			stmtWithoutComments = stmt.Statement
		} else {
			stmtWithoutComments = stripped.Statement
		}
		result = append(result, inputStatement{
			statement:                stmt.Statement,
			statementWithoutComments: stmtWithoutComments,
			delim:                    stmt.Terminator,
		})
	}
	return result, err
}
