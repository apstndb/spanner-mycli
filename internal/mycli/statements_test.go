package mycli

import (
	"errors"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spantype/typector"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"
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
				Params: map[string]any{
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
				Params: map[string]any{
					"n": gcvctor.Int64Value(1),
				},
			},
			wantErr: false,
		},
		{
			name: "binds SQL spelling for a case-insensitive match",
			args: args{
				sql: "SELECT @mixedcase",
				params: map[string]ast.Node{
					"MixedCase": &ast.IntLiteral{Base: 10, Value: "42"},
				},
			},
			want: spanner.Statement{
				SQL: "SELECT @mixedcase",
				Params: map[string]any{
					"mixedcase": gcvctor.Int64Value(42),
				},
			},
		},
		{
			name: "binds the first SQL spelling when one statement uses multiple casings",
			args: args{
				sql: "SELECT @MixedCase AS exact_match, @mixedcase AS folded_match",
				params: map[string]ast.Node{
					"MixedCase": &ast.IntLiteral{Base: 10, Value: "42"},
				},
			},
			want: spanner.Statement{
				SQL: "SELECT @MixedCase AS exact_match, @mixedcase AS folded_match",
				Params: map[string]any{
					"MixedCase": gcvctor.Int64Value(42),
				},
			},
		},
		{
			name: "leaves SQL bytes unchanged around comments and string literals",
			args: args{
				sql: "SELECT @MixedCase /* @ignored */, '@also'",
				params: map[string]ast.Node{
					"mixedcase": &ast.IntLiteral{Base: 10, Value: "1"},
				},
			},
			want: spanner.Statement{
				SQL: "SELECT @MixedCase /* @ignored */, '@also'",
				Params: map[string]any{
					"MixedCase": gcvctor.Int64Value(1),
				},
			},
		},
		{
			name: "DML binds the SQL spelling",
			args: args{
				sql: "INSERT INTO T (id) VALUES (@mixedcase)",
				params: map[string]ast.Node{
					"MixedCase": &ast.IntLiteral{Base: 10, Value: "7"},
				},
			},
			want: spanner.Statement{
				SQL: "INSERT INTO T (id) VALUES (@mixedcase)",
				Params: map[string]any{
					"mixedcase": gcvctor.Int64Value(7),
				},
			},
		},
		{
			name: "type-only parameter uses the SQL spelling when types are requested",
			args: args{
				sql:         "SELECT @mixedtype",
				includeType: true,
				params: map[string]ast.Node{
					"MixedType": lo.Must(parseMemefishType("", "INT64")),
				},
			},
			want: spanner.Statement{
				SQL: "SELECT @mixedtype",
				Params: map[string]any{
					"mixedtype": gcvctor.NullOf(typector.CodeToSimpleType(sppb.TypeCode_INT64)),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newStatement(tt.args.sql, tt.args.params, tt.args.includeType)
			if (err != nil) != tt.wantErr {
				t.Errorf("newStatement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.EquateEmpty(), protocmp.Transform()); diff != "" {
				t.Errorf("newStatement() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewStatementAmbiguousParamAliases(t *testing.T) {
	t.Parallel()
	_, err := newStatement("SELECT @mixedcase", map[string]ast.Node{
		"MixedCase": &ast.IntLiteral{Base: 10, Value: "1"},
		"mixedcase": &ast.IntLiteral{Base: 10, Value: "2"},
	}, false)
	if !errors.Is(err, errAmbiguousQueryParameter) {
		t.Fatalf("error = %v, want errAmbiguousQueryParameter", err)
	}
}

func TestUsedQueryParameterNamesIgnoresCommentsAndLiterals(t *testing.T) {
	t.Parallel()
	got, err := usedQueryParameterNames("SELECT @MixedCase /* @ignored */, '@also'")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"MixedCase"}, got); diff != "" {
		t.Fatalf("usedQueryParameterNames() mismatch (-want +got):\n%s", diff)
	}
}

func TestUsedQueryParameterNamesFirstOccurrenceForCaseAliases(t *testing.T) {
	t.Parallel()
	got, err := usedQueryParameterNames("SELECT @mixedcase, @MixedCase, @n")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"mixedcase", "n"}, got); diff != "" {
		t.Fatalf("usedQueryParameterNames() mismatch (-want +got):\n%s", diff)
	}
}
