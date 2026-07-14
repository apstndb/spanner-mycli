// Copyright 2026 apstndb
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

package mycli

import (
	"fmt"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// recoverMemefishParserPanic converts upstream parser panics to returned errors.
// In memefish v0.7.0, and still in v0.8.0, unclosed string literals and
// backtick identifiers can reach the lexer's panicfAtPosition path through
// ParseExpr or ParseType instead of returning an error. Remove this wrapper
// once a released memefish version converts these cases to returned errors.
func recoverMemefishParserPanic[T any](parse func() (T, error)) (result T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var zero T
			result = zero
			if recoveredErr, ok := recovered.(error); ok {
				err = fmt.Errorf("memefish parser panic: %w", recoveredErr)
			} else {
				err = fmt.Errorf("memefish parser panic: %v", recovered)
			}
		}
	}()

	return parse()
}

func parseMemefishExpr(filepath, input string) (ast.Expr, error) {
	return recoverMemefishParserPanic(func() (ast.Expr, error) {
		return memefish.ParseExpr(filepath, input)
	})
}

func parseMemefishType(filepath, input string) (ast.Type, error) {
	return recoverMemefishParserPanic(func() (ast.Type, error) {
		return memefish.ParseType(filepath, input)
	})
}
