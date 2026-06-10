package mycli

import (
	"bytes"
	"testing"

	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

func TestBuildSelectQueryWithColumns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		columns []string
		table   string
		wantSQL string
	}{
		{
			name:    "simple identifiers stay quoted",
			columns: []string{"UserId", "FirstName"},
			table:   "Users",
			wantSQL: "SELECT `UserId`, `FirstName` FROM `Users`",
		},
		{
			name:    "reserved identifiers are quoted",
			columns: []string{"Order", "Value"},
			table:   "Order",
			wantSQL: "SELECT `Order`, `Value` FROM `Order`",
		},
		{
			name:    "schema qualified table is quoted segment by segment",
			columns: []string{"UserId", "From"},
			table:   "select.Order",
			wantSQL: "SELECT `UserId`, `From` FROM `select`.`Order`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildSelectQueryWithColumns(dbadminpb.DatabaseDialect_GOOGLE_STANDARD_SQL, tt.columns, tt.table); got != tt.wantSQL {
				t.Fatalf("buildSelectQueryWithColumns() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestWriteCapturedDumpOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "empty output stays empty",
			output: "",
			want:   "",
		},
		{
			name:   "missing trailing newline is added",
			output: "INSERT INTO Singers VALUES (1)",
			want:   "INSERT INTO Singers VALUES (1)\n",
		},
		{
			name:   "single trailing newline is preserved",
			output: "INSERT INTO Singers VALUES (1)\n",
			want:   "INSERT INTO Singers VALUES (1)\n",
		},
		{
			name:   "extra trailing newlines are collapsed",
			output: "INSERT INTO Singers VALUES (1)\n\n",
			want:   "INSERT INTO Singers VALUES (1)\n",
		},
		{
			name:   "internal blank lines are preserved",
			output: "INSERT INTO Singers VALUES (1)\n\nINSERT INTO Singers VALUES (2)\n",
			want:   "INSERT INTO Singers VALUES (1)\n\nINSERT INTO Singers VALUES (2)\n",
		},
		{
			name:   "only newlines collapse to one newline",
			output: "\n\n",
			want:   "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeCapturedDumpOutput(&buf, tt.output); err != nil {
				t.Fatalf("writeCapturedDumpOutput() error = %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Fatalf("writeCapturedDumpOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}
