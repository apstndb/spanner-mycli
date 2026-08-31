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
	"context"
	"testing"

	"github.com/cloudspannerecosystem/memefish/ast"
)

func TestSetParamStatementMalformedInput(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		stmt Statement
	}{
		{name: "type", stmt: &SetParamTypeStatement{Name: "p", Type: "'"}},
		{name: "value", stmt: &SetParamValueStatement{Name: "p", Value: "'"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := &Session{systemVariables: &systemVariables{Params: make(map[string]ast.Node)}}
			if _, err := tt.stmt.Execute(context.Background(), session); err == nil {
				t.Fatal("Execute() error = nil, want malformed-input error")
			}
		})
	}
}
