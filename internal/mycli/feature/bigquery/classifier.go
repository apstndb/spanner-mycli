// Copyright 2026 apstndb
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

package bigquery

import (
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
)

// bigQueryClassificationCopy returns a classification-only view of sql.
//
// memefish v0.8.1 ends `--`/`#` comments at LF only and does not treat ASCII
// backspace as whitespace. GoogleSQL line comments end at CR or LF, and
// backspace is whitespace. Mapping CR to LF and backspace to space makes the
// token walk conservative for the READONLY guard (a comment cannot hide a later
// DELETE across a raw CR). The substitutions do not add or remove quote, slash,
// or semicolon bytes. The original SQL is not modified and remains the payload
// passed to BigQuery.
func bigQueryClassificationCopy(sql string) string {
	return strings.NewReplacer("\r", "\n", "\b", " ").Replace(sql)
}

// bigQueryStatementMutates reports whether a BIGQUERY payload should be treated
// as mutating for the READONLY guard.
//
// The classifier is fail-closed and local: it inspects the complete payload
// before client, auth, or job construction. Only scripts whose every nonempty
// semicolon region begins with an unquoted query root (SELECT, WITH, or FROM,
// allowing leading parentheses) are read-only. Empty or comment-only payloads,
// unknown/DML/DDL/procedural roots, lexer errors, and unexpected panics are
// mutating. A positive verdict requires a complete walk to EOF. Pipe CALL/SET/
// DROP inside a query are not statement-root operations and are not blacklisted.
//
// Truncated octal/hex/unicode escapes in memefish v0.8.1 can panic with
// runtime.Error while building a diagnostic whose end exceeds Buffer.
// Lexer.NextToken recovers only *memefish.Error and re-panics the rest. The
// recover here is A15-local: any panic becomes unknown/mutating. Do not reuse
// recoverMemefishParserPanic, which deliberately re-panics runtime.Error.
func bigQueryStatementMutates(sql string) (mutating bool) {
	mutating = true
	defer func() {
		if recover() != nil {
			mutating = true
		}
	}()
	return !bigQueryPayloadIsReadOnly(bigQueryClassificationCopy(sql))
}

func bigQueryPayloadIsReadOnly(sql string) bool {
	lex := &memefish.Lexer{File: &token.File{FilePath: "bigquery-readonly", Buffer: sql}}
	sawQuery := false
	atStart := true
	for n := 0; n <= len(sql)+1; n++ {
		if err := lex.NextToken(); err != nil {
			return false
		}
		switch lex.Token.Kind {
		case token.TokenEOF:
			return sawQuery
		case ";":
			atStart = true
		case token.TokenBad:
			return false
		case "(":
			if atStart {
				continue
			}
		default:
			if atStart {
				if !isBigQueryQueryRoot(lex.Token) {
					return false
				}
				sawQuery = true
				atStart = false
			}
		}
	}
	return false
}

func isBigQueryQueryRoot(tok token.Token) bool {
	switch tok.Kind {
	case "SELECT", "WITH", "FROM":
		return true
	default:
		return false
	}
}
