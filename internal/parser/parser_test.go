package parser_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

func TestBoolParser(t *testing.T) {
	p := parser.NewBoolParser()

	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{"true lowercase", "true", true, false},
		{"TRUE uppercase", "TRUE", true, false},
		{"false", "false", false, false},
		{"FALSE uppercase", "FALSE", false, false},
		{"with spaces", "  true  ", true, false},
		{"1", "1", true, false},
		{"0", "0", false, false},
		{"t", "t", true, false},
		{"f", "f", false, false},
		{"T", "T", true, false},
		{"F", "F", false, false},
		{"invalid", "invalid", false, true},
		{"empty", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseAndValidate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseAndValidate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIntParser(t *testing.T) {
	t.Run("basic parsing", func(t *testing.T) {
		p := parser.NewIntParser()

		tests := []struct {
			name    string
			input   string
			want    int64
			wantErr bool
		}{
			{"positive", "42", 42, false},
			{"negative", "-42", -42, false},
			{"zero", "0", 0, false},
			{"with spaces", "  123  ", 123, false},
			{"invalid", "abc", 0, true},
			{"empty", "", 0, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := p.ParseAndValidate(tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("ParseAndValidate() = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("with range validation", func(t *testing.T) {
		p := parser.NewIntParser().WithRange(1, 100)

		tests := []struct {
			name    string
			input   string
			want    int64
			wantErr bool
		}{
			{"in range", "50", 50, false},
			{"min value", "1", 1, false},
			{"max value", "100", 100, false},
			{"below min", "0", 0, true},
			{"above max", "101", 0, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := p.ParseAndValidate(tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !tt.wantErr && got != tt.want {
					t.Errorf("ParseAndValidate() = %v, want %v", got, tt.want)
				}
			})
		}
	})
}

func TestDurationParser(t *testing.T) {
	p := parser.NewDurationParser()

	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"seconds", "10s", 10 * time.Second, false},
		{"minutes", "5m", 5 * time.Minute, false},
		{"hours", "2h", 2 * time.Hour, false},
		{"complex", "1h30m45s", time.Hour + 30*time.Minute + 45*time.Second, false},
		{"with spaces", "  100ms  ", 100 * time.Millisecond, false},
		{"invalid", "abc", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseAndValidate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseAndValidate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnumParser(t *testing.T) {
	type Color int
	const (
		Red Color = iota
		Green
		Blue
	)

	p := parser.NewEnumParser(map[string]Color{
		"RED":   Red,
		"GREEN": Green,
		"BLUE":  Blue,
	})

	tests := []struct {
		name    string
		input   string
		want    Color
		wantErr bool
	}{
		{"exact match", "RED", Red, false},
		{"lowercase", "red", Red, false},
		{"mixed case", "GrEeN", Green, false},
		{"with quotes", "'BLUE'", 0, true}, // Quotes are not removed
		{"invalid", "YELLOW", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseAndValidate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseAndValidate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringParser(t *testing.T) {
	t.Run("simple parser", func(t *testing.T) {
		p := parser.NewStringParser()

		tests := []struct {
			name  string
			input string
			want  string
		}{
			{"simple string", "hello", "hello"},
			{"with spaces", "  hello world  ", "  hello world  "},
			{"keeps quotes", "'quoted'", "'quoted'"},
			{"keeps double quotes", `"quoted"`, `"quoted"`},
			{"empty", "", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := p.ParseAndValidate(tt.input)
				if err != nil {
					t.Errorf("ParseAndValidate() unexpected error = %v", err)
					return
				}
				if got != tt.want {
					t.Errorf("ParseAndValidate() = %q, want %q", got, tt.want)
				}
			})
		}
	})
}

func TestChainValidators(t *testing.T) {
	// Test chaining multiple validators
	isPositive := func(v int64) error {
		if v <= 0 {
			return fmt.Errorf("value must be positive")
		}
		return nil
	}

	isEven := func(v int64) error {
		if v%2 != 0 {
			return fmt.Errorf("value must be even")
		}
		return nil
	}

	p := parser.WithValidation(
		parser.NewIntParser(),
		isPositive,
		isEven,
	)

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
		errMsg  string
	}{
		{"valid even positive", "42", 42, false, ""},
		{"zero", "0", 0, true, "positive"},
		{"negative", "-2", 0, true, "positive"},
		{"odd positive", "3", 0, true, "even"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseAndValidate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %v, should contain %v", err, tt.errMsg)
				}
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseAndValidate() = %v, want %v", got, tt.want)
			}
		})
	}
}
