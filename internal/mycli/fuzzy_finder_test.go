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

package mycli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectFuzzyContext(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantContextType fuzzyContextType
		wantArgPrefix   string
		wantArgStartPos int
	}{
		{
			name:            "USE with trailing space",
			input:           "USE ",
			wantContextType: fuzzyContextDatabase,
			wantArgPrefix:   "",
			wantArgStartPos: 4,
		},
		{
			name:            "USE without space",
			input:           "USE",
			wantContextType: fuzzyContextDatabase,
			wantArgPrefix:   "",
			wantArgStartPos: 3,
		},
		{
			name:            "USE with partial db name",
			input:           "USE apst",
			wantContextType: fuzzyContextDatabase,
			wantArgPrefix:   "apst",
			wantArgStartPos: 4,
		},
		{
			name:            "USE with full db name",
			input:           "USE my-database",
			wantContextType: fuzzyContextDatabase,
			wantArgPrefix:   "my-database",
			wantArgStartPos: 4,
		},
		{
			name:            "use lowercase",
			input:           "use db",
			wantContextType: fuzzyContextDatabase,
			wantArgPrefix:   "db",
			wantArgStartPos: 4,
		},
		{
			name:            "USE with leading space",
			input:           "  USE ",
			wantContextType: fuzzyContextDatabase,
			wantArgPrefix:   "",
			wantArgStartPos: 6,
		},
		{
			name:            "SELECT query",
			input:           "SELECT 1",
			wantContextType: "",
		},
		{
			name:            "empty input",
			input:           "",
			wantContextType: "",
		},
		{
			name:            "SHOW DATABASES",
			input:           "SHOW DATABASES",
			wantContextType: "",
		},
		// SET variable context
		{
			name:            "SET with partial name",
			input:           "SET CLI_",
			wantContextType: fuzzyContextVariable,
			wantArgPrefix:   "CLI_",
			wantArgStartPos: 4,
		},
		{
			name:            "SET with no name",
			input:           "SET ",
			wantContextType: fuzzyContextVariable,
			wantArgPrefix:   "",
			wantArgStartPos: 4,
		},
		{
			name:            "SET without space should not match",
			input:           "SET",
			wantContextType: "",
		},
		{
			name:            "set lowercase",
			input:           "set cli_f",
			wantContextType: fuzzyContextVariable,
			wantArgPrefix:   "cli_f",
			wantArgStartPos: 4,
		},
		{
			name:            "SET with = should not match",
			input:           "SET CLI_FORMAT = TABLE",
			wantContextType: "",
		},
		{
			name:            "SET with = no space should not match",
			input:           "SET CLI_FORMAT=TABLE",
			wantContextType: "",
		},
		// SHOW VARIABLE context
		{
			name:            "SHOW VARIABLE with partial name",
			input:           "SHOW VARIABLE CLI_",
			wantContextType: fuzzyContextVariable,
			wantArgPrefix:   "CLI_",
			wantArgStartPos: 14,
		},
		{
			name:            "SHOW VARIABLE with no name",
			input:           "SHOW VARIABLE ",
			wantContextType: fuzzyContextVariable,
			wantArgPrefix:   "",
			wantArgStartPos: 14,
		},
		{
			name:            "show variable lowercase",
			input:           "show variable read",
			wantContextType: fuzzyContextVariable,
			wantArgPrefix:   "read",
			wantArgStartPos: 14,
		},
		{
			name:            "SHOW VARIABLES should not match",
			input:           "SHOW VARIABLES",
			wantContextType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFuzzyContext(tt.input)
			assert.Equal(t, tt.wantContextType, got.contextType)
			if tt.wantContextType != "" {
				assert.Equal(t, tt.wantArgPrefix, got.argPrefix)
				assert.Equal(t, tt.wantArgStartPos, got.argStartPos)
			}
		})
	}
}
