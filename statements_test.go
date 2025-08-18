package main

import (
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestNewStatement(t *testing.T) {
	t.Parallel()
	type args struct {
		sql         string
		params      map[string]ast.Node
		includeType bool
	}
	tests := []struct {
		name    string
		args    args
		want    spanner.Statement
		wantErr bool
	}{
		{
			name: "Statement without params",
			args: args{
				sql: "SELECT 1",
			},
			want:    spanner.Statement{SQL: "SELECT 1"},
			wantErr: false,
		},
		{
			name: "Statement with unused params",
			args: args{
				sql: "SELECT 1",
				params: map[string]ast.Node{
					"unused": &ast.BoolLiteral{Value: true},
				},
			},
			want:    spanner.Statement{SQL: "SELECT 1"},
			wantErr: false,
		},
		{
			name: "Statement with used params",
			args: args{
				sql: "SELECT @n",
				params: map[string]ast.Node{
					"n": &ast.IntLiteral{Base: 10, Value: "1"},
				},
			},
			want: spanner.Statement{
				SQL: "SELECT @n",
				Params: map[string]interface{}{
					"n": gcvctor.Int64Value(1),
				},
			},
			wantErr: false,
		},
		{
			name: "Statement with used and unused params",
			args: args{
				sql: "SELECT @n",
				params: map[string]ast.Node{
					"n":      &ast.IntLiteral{Base: 10, Value: "1"},
					"unused": &ast.BoolLiteral{Value: true},
				},
			},
			want: spanner.Statement{
				SQL: "SELECT @n",
				Params: map[string]interface{}{
					"n": gcvctor.Int64Value(1),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newStatement(tt.args.sql, tt.args.params, tt.args.includeType)
			if (err != nil) != tt.wantErr {
				t.Errorf("newStatement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.EquateEmpty(), protocmp.Transform()); diff != "" {
				t.Errorf("newStatement() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
