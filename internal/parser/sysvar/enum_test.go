package sysvar

import (
	"testing"
)

func TestEnumParser(t *testing.T) {
	t.Run("NewEnumParser", func(t *testing.T) {
		values := map[string]string{
			"DEBUG": "debug",
			"INFO":  "info",
			"WARN":  "warn",
			"ERROR": "error",
		}
		p := newEnumParser(values)

		tests := []struct {
			name    string
			input   string
			want    string
			wantErr bool
		}{
			{"valid DEBUG", "DEBUG", "debug", false},
			{"valid INFO", "INFO", "info", false},
			{"valid WARN", "WARN", "warn", false},
			{"valid ERROR", "ERROR", "error", false},
			{"invalid value", "INVALID", "", true},
			{"empty string", "", "", true},
			{"lowercase accepted", "debug", "debug", false},
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

	t.Run("NewEnumParser", func(t *testing.T) {
		values := map[string]int{
			"LOW":    1,
			"MEDIUM": 5,
			"HIGH":   10,
		}
		p := newEnumParser(values)

		tests := []struct {
			name    string
			input   string
			want    int
			wantErr bool
		}{
			{"valid LOW", "LOW", 1, false},
			{"valid MEDIUM", "MEDIUM", 5, false},
			{"valid HIGH", "HIGH", 10, false},
			{"invalid value", "INVALID", 0, true},
			{"numeric not accepted", "5", 0, true},
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

	t.Run("CaseSensitive option", func(t *testing.T) {
		values := map[string]string{
			"Debug": "debug",
			"Info":  "info",
		}

		t.Run("case insensitive (default)", func(t *testing.T) {
			p := newEnumParser(values)

			// Exact case should work
			got, err := p.ParseAndValidate("Debug")
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			if got != "debug" {
				t.Errorf("Expected 'debug', got %v", got)
			}

			// Different case should also work (case-insensitive)
			got2, err := p.ParseAndValidate("DEBUG")
			if err != nil {
				t.Errorf("Expected no error for case-insensitive match, got %v", err)
			}
			if got2 != "debug" {
				t.Errorf("Expected 'debug', got %v", got2)
			}
		})

		t.Run("case sensitive (when enabled)", func(t *testing.T) {
			// Test case-sensitive mode
			p := newEnumParser(values).CaseSensitive()

			// Only exact case should work
			got, err := p.ParseAndValidate("Debug")
			if err != nil {
				t.Errorf("ParseAndValidate(Debug) error = %v", err)
			}
			if got != "debug" {
				t.Errorf("ParseAndValidate(Debug) = %v, want 'debug'", got)
			}

			// Different cases should fail
			testCases := []string{"DEBUG", "debug", "DeBuG"}
			for _, tc := range testCases {
				_, err := p.ParseAndValidate(tc)
				if err == nil {
					t.Errorf("Expected error for case mismatch with %s", tc)
				}
			}
		})
	})
}

func TestCreateDualModeEnumParser(t *testing.T) {
	type Status int
	const (
		StatusPending Status = iota
		StatusActive
		StatusInactive
	)

	enumValues := map[string]Status{
		"PENDING":  StatusPending,
		"ACTIVE":   StatusActive,
		"INACTIVE": StatusInactive,
	}

	p := createDualModeEnumParser(enumValues)

	t.Run("Simple mode", func(t *testing.T) {
		got, err := p.ParseAndValidateWithMode("ACTIVE", ParseModeSimple)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if got != StatusActive {
			t.Errorf("Expected StatusActive, got %v", got)
		}
	})

	t.Run("GoogleSQL mode", func(t *testing.T) {
		got, err := p.ParseAndValidateWithMode(`"PENDING"`, ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if got != StatusPending {
			t.Errorf("Expected StatusPending, got %v", got)
		}
	})

	t.Run("Invalid value", func(t *testing.T) {
		_, err := p.ParseAndValidateWithMode("INVALID", ParseModeSimple)
		if err == nil {
			t.Error("Expected error for invalid enum value")
		}
	})
}
