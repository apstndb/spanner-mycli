package mycli

import "testing"

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
			if got := buildSelectQueryWithColumns(tt.columns, tt.table); got != tt.wantSQL {
				t.Fatalf("buildSelectQueryWithColumns() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}
