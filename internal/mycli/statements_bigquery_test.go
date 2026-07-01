package mycli

import (
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

func TestFormatBigQueryValue(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	for _, tt := range []struct {
		name string
		in   bigquery.Value
		want string
	}{
		{name: "nil", in: nil, want: "NULL"},
		{name: "string", in: "hello", want: "hello"},
		{name: "bool true", in: true, want: "true"},
		{name: "bool false", in: false, want: "false"},
		{name: "int64", in: int64(42), want: "42"},
		{name: "float64", in: float64(3.14), want: "3.14"},
		{name: "bytes", in: []byte{0xde, 0xad}, want: "3q0="},
		{name: "timestamp", in: ts, want: ts.Format(time.RFC3339Nano)},
		{name: "date", in: civil.Date{Year: 2024, Month: 3, Day: 15}, want: "2024-03-15"},
		{name: "rat", in: big.NewRat(1, 2), want: "0.500000000"},
		{name: "array", in: []bigquery.Value{"a", int64(1)}, want: `["a",1]`},
		{name: "record", in: map[string]bigquery.Value{"k": "v"}, want: `{"k":"v"}`},
		{name: "null string", in: bigquery.NullString{}, want: "NULL"},
		{name: "valid null string", in: bigquery.NullString{StringVal: "x", Valid: true}, want: "x"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatBigQueryValue(tt.in); got != tt.want {
				t.Fatalf("formatBigQueryValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateBigQueryClientOptionsSkipsEmulatorAuth(t *testing.T) {
	t.Parallel()

	sysVars := &systemVariables{
		Connection: ConnectionVars{
			Host:                  "localhost",
			Port:                  9010,
			WithoutAuthentication: true,
		},
	}

	opts, err := createBigQueryClientOptions(t.Context(), nil, sysVars)
	if err != nil {
		t.Fatalf("createBigQueryClientOptions() error = %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("createBigQueryClientOptions() len = %d, want 0 (ADC, not emulator auth)", len(opts))
	}
}

func TestBigQueryProject(t *testing.T) {
	t.Parallel()

	t.Run("explicit project", func(t *testing.T) {
		t.Parallel()
		sv := &systemVariables{
			Connection: ConnectionVars{Project: "spanner-project"},
			Feature:    FeatureVars{BigQueryProject: "bq-project"},
		}
		if got := bigQueryProject(sv); got != "bq-project" {
			t.Fatalf("bigQueryProject() = %q, want bq-project", got)
		}
	})

	t.Run("fallback to CLI_PROJECT", func(t *testing.T) {
		t.Parallel()
		sv := &systemVariables{
			Connection: ConnectionVars{Project: "spanner-project"},
		}
		if got := bigQueryProject(sv); got != "spanner-project" {
			t.Fatalf("bigQueryProject() = %q, want spanner-project", got)
		}
	})
}
