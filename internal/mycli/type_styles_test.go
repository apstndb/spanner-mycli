// Copyright 2025 apstndb
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
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
)

func TestParseTypeStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantStyles map[sppb.TypeCode]string
		wantNull   string
		wantErr    bool
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "whitespace only",
			input: "   ",
		},
		{
			name:  "single named color",
			input: "STRING=green",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_STRING: "\033[32m",
			},
		},
		{
			name:  "single raw SGR number",
			input: "INT64=1",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_INT64: "\033[1m",
			},
		},
		{
			name:  "combined named styles",
			input: "STRING=bold;green",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_STRING: "\033[1;32m",
			},
		},
		{
			name:  "multiple types",
			input: "STRING=green:INT64=bold:BOOL=yellow",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_STRING: "\033[32m",
				sppb.TypeCode_INT64:  "\033[1m",
				sppb.TypeCode_BOOL:   "\033[33m",
			},
		},
		{
			name:  "256 color",
			input: "TIMESTAMP=38;5;214",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_TIMESTAMP: "\033[38;5;214m",
			},
		},
		{
			name:       "NULL override",
			input:      "NULL=red",
			wantStyles: map[sppb.TypeCode]string{},
			wantNull:   "\033[31m",
		},
		{
			name:  "NULL with type styles",
			input: "STRING=green:NULL=dim",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_STRING: "\033[32m",
			},
			wantNull: "\033[2m",
		},
		{
			name:  "case insensitive type names",
			input: "string=green:int64=bold",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_STRING: "\033[32m",
				sppb.TypeCode_INT64:  "\033[1m",
			},
		},
		{
			name:  "all Spanner types",
			input: "BOOL=1:INT64=2:FLOAT32=3:FLOAT64=4:NUMERIC=5:STRING=6:BYTES=7:JSON=31:DATE=32:TIMESTAMP=33:ARRAY=34:STRUCT=35:PROTO=36:ENUM=37:INTERVAL=1;31:UUID=1;32",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_BOOL:      "\033[1m",
				sppb.TypeCode_INT64:     "\033[2m",
				sppb.TypeCode_FLOAT32:   "\033[3m",
				sppb.TypeCode_FLOAT64:   "\033[4m",
				sppb.TypeCode_NUMERIC:   "\033[5m",
				sppb.TypeCode_STRING:    "\033[6m",
				sppb.TypeCode_BYTES:     "\033[7m",
				sppb.TypeCode_JSON:      "\033[31m",
				sppb.TypeCode_DATE:      "\033[32m",
				sppb.TypeCode_TIMESTAMP: "\033[33m",
				sppb.TypeCode_ARRAY:     "\033[34m",
				sppb.TypeCode_STRUCT:    "\033[35m",
				sppb.TypeCode_PROTO:     "\033[36m",
				sppb.TypeCode_ENUM:      "\033[37m",
				sppb.TypeCode_INTERVAL:  "\033[1;31m",
				sppb.TypeCode_UUID:      "\033[1;32m",
			},
		},
		{
			name:  "whitespace in pairs",
			input: " STRING = green : INT64 = bold ",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_STRING: "\033[32m",
				sppb.TypeCode_INT64:  "\033[1m",
			},
		},
		{
			name:  "trailing colon ignored",
			input: "STRING=green:",
			wantStyles: map[sppb.TypeCode]string{
				sppb.TypeCode_STRING: "\033[32m",
			},
		},
		// Error cases
		{
			name:    "unknown type name",
			input:   "UNKNOWN=green",
			wantErr: true,
		},
		{
			name:    "missing equals",
			input:   "STRING",
			wantErr: true,
		},
		{
			name:    "empty value",
			input:   "STRING=",
			wantErr: true,
		},
		{
			name:    "unknown style name",
			input:   "STRING=neonpink",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTypeStyles(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTypeStyles(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if diff := cmp.Diff(tt.wantStyles, got.typeStyles); diff != "" {
				t.Errorf("typeStyles mismatch (-want +got):\n%s", diff)
			}

			if got.nullStyle != tt.wantNull {
				t.Errorf("nullStyle = %q, want %q", got.nullStyle, tt.wantNull)
			}
		})
	}
}

func TestResolveStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "bold", input: "bold", want: "1"},
		{name: "dim", input: "dim", want: "2"},
		{name: "green", input: "green", want: "32"},
		{name: "bold green", input: "bold;green", want: "1;32"},
		{name: "raw number", input: "31", want: "31"},
		{name: "256 color", input: "38;5;214", want: "38;5;214"},
		{name: "truecolor", input: "38;2;255;128;0", want: "38;2;255;128;0"},
		{name: "mixed named and raw", input: "bold;31", want: "1;31"},
		{name: "case insensitive", input: "Bold;GREEN", want: "1;32"},
		{name: "unknown name", input: "neon", wantErr: true},
		{name: "unknown mixed", input: "bold;neon", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveStyle(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveStyle(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("resolveStyle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatTypeStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config typeStyleConfig
		want   string
	}{
		{
			name:   "empty config",
			config: typeStyleConfig{},
			want:   "",
		},
		{
			name: "single type",
			config: typeStyleConfig{
				typeStyles: map[sppb.TypeCode]string{
					sppb.TypeCode_STRING: "\033[32m",
				},
			},
			want: "STRING=32",
		},
		{
			name: "multiple types in stable order",
			config: typeStyleConfig{
				typeStyles: map[sppb.TypeCode]string{
					sppb.TypeCode_STRING: "\033[32m",
					sppb.TypeCode_INT64:  "\033[1m",
				},
			},
			want: "INT64=1:STRING=32",
		},
		{
			name: "NULL only",
			config: typeStyleConfig{
				nullStyle: "\033[2m",
			},
			want: "NULL=2",
		},
		{
			name: "types and NULL",
			config: typeStyleConfig{
				typeStyles: map[sppb.TypeCode]string{
					sppb.TypeCode_STRING: "\033[32m",
				},
				nullStyle: "\033[31m",
			},
			want: "STRING=32:NULL=31",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatTypeStyles(tt.config)
			if got != tt.want {
				t.Errorf("formatTypeStyles() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAndFormatRoundTrip(t *testing.T) {
	t.Parallel()

	// Parsing and formatting should produce a normalized form
	input := "STRING=green:INT64=bold:NULL=dim"
	config, err := parseTypeStyles(input)
	if err != nil {
		t.Fatalf("parseTypeStyles(%q) = %v", input, err)
	}
	output := formatTypeStyles(config)
	// Re-parse the output
	config2, err := parseTypeStyles(output)
	if err != nil {
		t.Fatalf("parseTypeStyles(%q) = %v", output, err)
	}
	output2 := formatTypeStyles(config2)
	if output != output2 {
		t.Errorf("round-trip mismatch: %q → %q → %q", input, output, output2)
	}
}
