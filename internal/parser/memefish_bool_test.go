package parser_test

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

func TestGoogleSQLBoolParser(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		// Valid boolean literals
		{
			name:  "TRUE uppercase",
			input: "TRUE",
			want:  true,
		},
		{
			name:  "FALSE uppercase",
			input: "FALSE",
			want:  false,
		},
		{
			name:  "true lowercase",
			input: "true",
			want:  true,
		},
		{
			name:  "false lowercase",
			input: "false",
			want:  false,
		},

		// Invalid inputs
		{
			name:    "identifier",
			input:   "identifier",
			wantErr: true,
		},
		{
			name:    "string literal",
			input:   "'true'",
			wantErr: true,
		},
		{
			name:    "number",
			input:   "1",
			wantErr: true,
		},
		{
			name:    "NULL",
			input:   "NULL",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GoogleSQLBoolParser.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("GoogleSQLBoolParser.Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("GoogleSQLBoolParser.Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}
