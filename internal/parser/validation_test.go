package parser_test

import (
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

func TestCreateRangeValidator(t *testing.T) {
	t.Run("no constraints", func(t *testing.T) {
		validator := parser.CreateRangeValidator[int](nil, nil)
		
		// Should accept any value
		testCases := []int{-1000, 0, 1000, 999999}
		for _, v := range testCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%d) failed: %v", v, err)
			}
		}
	})
	
	t.Run("min constraint only", func(t *testing.T) {
		min := 10
		validator := parser.CreateRangeValidator(&min, nil)
		
		// Valid values
		validCases := []int{10, 11, 100, 1000}
		for _, v := range validCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%d) failed: %v", v, err)
			}
		}
		
		// Invalid values
		invalidCases := []int{9, 0, -10}
		for _, v := range invalidCases {
			err := validator(v)
			if err == nil {
				t.Errorf("expected error for value %d", v)
			}
			expectedMsg := "value 9 is less than minimum 10"
			if v == 9 && err.Error() != expectedMsg {
				t.Errorf("error = %q, want %q", err.Error(), expectedMsg)
			}
		}
	})
	
	t.Run("max constraint only", func(t *testing.T) {
		max := 100
		validator := parser.CreateRangeValidator(nil, &max)
		
		// Valid values
		validCases := []int{-100, 0, 50, 100}
		for _, v := range validCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%d) failed: %v", v, err)
			}
		}
		
		// Invalid values
		invalidCases := []int{101, 200, 1000}
		for _, v := range invalidCases {
			err := validator(v)
			if err == nil {
				t.Errorf("expected error for value %d", v)
			}
			expectedMsg := "value 101 is greater than maximum 100"
			if v == 101 && err.Error() != expectedMsg {
				t.Errorf("error = %q, want %q", err.Error(), expectedMsg)
			}
		}
	})
	
	t.Run("both constraints", func(t *testing.T) {
		min, max := 10, 100
		validator := parser.CreateRangeValidator(&min, &max)
		
		// Valid values
		validCases := []int{10, 50, 100}
		for _, v := range validCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%d) failed: %v", v, err)
			}
		}
		
		// Invalid values - too small
		if err := validator(9); err == nil {
			t.Error("expected error for value below minimum")
		}
		
		// Invalid values - too large
		if err := validator(101); err == nil {
			t.Error("expected error for value above maximum")
		}
	})
	
	t.Run("works with different numeric types", func(t *testing.T) {
		// Test with float64
		minFloat, maxFloat := 1.5, 10.5
		floatValidator := parser.CreateRangeValidator(&minFloat, &maxFloat)
		
		if err := floatValidator(5.5); err != nil {
			t.Errorf("floatValidator(5.5) failed: %v", err)
		}
		if err := floatValidator(1.0); err == nil {
			t.Error("expected error for float below minimum")
		}
		if err := floatValidator(11.0); err == nil {
			t.Error("expected error for float above maximum")
		}
		
		// Test with int64
		minInt64, maxInt64 := int64(0), int64(65535)
		int64Validator := parser.CreateRangeValidator(&minInt64, &maxInt64)
		
		if err := int64Validator(8080); err != nil {
			t.Errorf("int64Validator(8080) failed: %v", err)
		}
		if err := int64Validator(-1); err == nil {
			t.Error("expected error for int64 below minimum")
		}
		if err := int64Validator(70000); err == nil {
			t.Error("expected error for int64 above maximum")
		}
	})
}

func TestCreateDurationRangeValidator(t *testing.T) {
	t.Run("no constraints", func(t *testing.T) {
		validator := parser.CreateDurationRangeValidator(nil, nil)
		
		// Should accept any duration
		testCases := []time.Duration{
			-10 * time.Second,
			0,
			10 * time.Second,
			24 * time.Hour,
		}
		for _, v := range testCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%v) failed: %v", v, err)
			}
		}
	})
	
	t.Run("min constraint only", func(t *testing.T) {
		min := time.Duration(0)
		validator := parser.CreateDurationRangeValidator(&min, nil)
		
		// Valid values
		validCases := []time.Duration{0, 1 * time.Second, 1 * time.Hour}
		for _, v := range validCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%v) failed: %v", v, err)
			}
		}
		
		// Invalid values
		err := validator(-1 * time.Second)
		if err == nil {
			t.Error("expected error for negative duration")
		}
		expectedMsg := "duration -1s is less than minimum 0s"
		if err.Error() != expectedMsg {
			t.Errorf("error = %q, want %q", err.Error(), expectedMsg)
		}
	})
	
	t.Run("max constraint only", func(t *testing.T) {
		max := 500 * time.Millisecond
		validator := parser.CreateDurationRangeValidator(nil, &max)
		
		// Valid values
		validCases := []time.Duration{0, 100 * time.Millisecond, 500 * time.Millisecond}
		for _, v := range validCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%v) failed: %v", v, err)
			}
		}
		
		// Invalid values
		err := validator(1 * time.Second)
		if err == nil {
			t.Error("expected error for duration above maximum")
		}
		expectedMsg := "duration 1s is greater than maximum 500ms"
		if err.Error() != expectedMsg {
			t.Errorf("error = %q, want %q", err.Error(), expectedMsg)
		}
	})
	
	t.Run("both constraints", func(t *testing.T) {
		min := time.Duration(0)
		max := 500 * time.Millisecond
		validator := parser.CreateDurationRangeValidator(&min, &max)
		
		// Valid values
		validCases := []time.Duration{0, 250 * time.Millisecond, 500 * time.Millisecond}
		for _, v := range validCases {
			if err := validator(v); err != nil {
				t.Errorf("validator(%v) failed: %v", v, err)
			}
		}
		
		// Invalid values - negative
		if err := validator(-1 * time.Second); err == nil {
			t.Error("expected error for negative duration")
		}
		
		// Invalid values - too large
		if err := validator(600 * time.Millisecond); err == nil {
			t.Error("expected error for duration above maximum")
		}
	})
}