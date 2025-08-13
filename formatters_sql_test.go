package main

import (
	"testing"

	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSimpleTablePath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantPath  *ast.Path
		wantError string
	}{
		{
			name:  "simple table name",
			input: "Users",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "Users"}},
			},
		},
		{
			name:  "schema qualified name",
			input: "myschema.Users",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "myschema"}, {Name: "Users"}},
			},
		},
		{
			name:  "reserved word as table name",
			input: "Order",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "Order"}},
			},
		},
		{
			name:  "three-part name",
			input: "catalog.schema.table",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "catalog"}, {Name: "schema"}, {Name: "table"}},
			},
		},
		{
			name:  "table name with spaces around",
			input: "  Users  ",
			wantPath: &ast.Path{
				Idents: []*ast.Ident{{Name: "Users"}},
			},
		},
		{
			name:      "empty string",
			input:     "",
			wantError: "CLI_SQL_TABLE_NAME must be set for SQL export formats",
		},
		{
			name:      "only spaces",
			input:     "   ",
			wantError: "CLI_SQL_TABLE_NAME must be set for SQL export formats",
		},
		{
			name:      "empty part in path",
			input:     "schema..table",
			wantError: `empty identifier in table path: "schema..table"`,
		},
		{
			name:      "trailing dot",
			input:     "schema.table.",
			wantError: `empty identifier in table path: "schema.table."`,
		},
		{
			name:      "leading dot",
			input:     ".table",
			wantError: `empty identifier in table path: ".table"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSimpleTablePath(tt.input)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantError, err.Error())
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, len(tt.wantPath.Idents), len(got.Idents))
				for i, ident := range tt.wantPath.Idents {
					assert.Equal(t, ident.Name, got.Idents[i].Name)
				}
				// Verify SQL output works (important for reserved words)
				_ = got.SQL()
			}
		})
	}
}

func TestParseSimpleTablePathSQL(t *testing.T) {
	// Test that reserved words are properly quoted in SQL output
	tests := []struct {
		name     string
		input    string
		wantSQL  string
	}{
		{
			name:    "simple table",
			input:   "Users",
			wantSQL: "Users",
		},
		{
			name:    "reserved word ORDER", 
			input:   "Order",
			wantSQL: "`Order`", // Reserved words are auto-quoted by memefish
		},
		{
			name:    "schema qualified",
			input:   "myschema.Users",
			wantSQL: "myschema.Users",
		},
		{
			name:    "reserved words in path",
			input:   "select.from.where",
			wantSQL: "`select`.`from`.`where`", // All reserved words are quoted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := parseSimpleTablePath(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSQL, path.SQL())
		})
	}
}