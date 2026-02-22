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
