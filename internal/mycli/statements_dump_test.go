package mycli

import (
	"errors"
	"io"
	"testing"

	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

func TestBuildSelectQueryWithColumns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		columns []string
		table   tableID
		wantSQL string
	}{
		{
			name:    "simple identifiers stay quoted",
			columns: []string{"UserId", "FirstName"},
			table:   tableID{Name: "Users"},
			wantSQL: "SELECT `UserId`, `FirstName` FROM `Users`",
		},
		{
			name:    "reserved identifiers are quoted",
			columns: []string{"Order", "Value"},
			table:   tableID{Name: "Order"},
			wantSQL: "SELECT `Order`, `Value` FROM `Order`",
		},
		{
			name:    "schema qualified table is quoted segment by segment",
			columns: []string{"UserId", "From"},
			table:   tableID{Schema: "select", Name: "Order"},
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

func TestExecuteDumpStreamingWithTxnPropagatesDDLWriteError(t *testing.T) {
	t.Parallel()

	session := newDetachedTestSession(io.Discard)
	t.Cleanup(session.Close)
	want := errors.New("output failed")
	result, err := executeDumpStreamingWithTxn(
		t.Context(), session, dumpModeSchema,
		&dumpPlan{DDL: []byte("CREATE TABLE T (Id INT64) PRIMARY KEY(Id);\n")},
		dumpFailWriter{err: want}, nil,
	)
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}
