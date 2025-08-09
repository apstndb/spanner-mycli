package sysvar

import (
	"errors"
	"testing"
)

func TestGetValue(t *testing.T) {
	t.Run("returns closure over value", func(t *testing.T) {
		value := "test"
		getter := GetValue(&value)

		if got := getter(); got != "test" {
			t.Errorf("getter() = %q, want %q", got, "test")
		}

		// Change the value
		value = "changed"
		if got := getter(); got != "changed" {
			t.Errorf("getter() after change = %q, want %q", got, "changed")
		}
	})

	t.Run("works with different types", func(t *testing.T) {
		intValue := 42
		intGetter := GetValue(&intValue)
		if got := intGetter(); got != 42 {
			t.Errorf("intGetter() = %d, want %d", got, 42)
		}

		boolValue := true
		boolGetter := GetValue(&boolValue)
		if got := boolGetter(); got != true {
			t.Errorf("boolGetter() = %v, want %v", got, true)
		}
	})
}

func TestSetValue(t *testing.T) {
	t.Run("sets value through pointer", func(t *testing.T) {
		var value string
		setter := SetValue(&value)

		if err := setter("test"); err != nil {
			t.Fatalf("setter failed: %v", err)
		}

		if value != "test" {
			t.Errorf("value = %q, want %q", value, "test")
		}
	})

	t.Run("works with different types", func(t *testing.T) {
		var intValue int
		intSetter := SetValue(&intValue)
		if err := intSetter(42); err != nil {
			t.Fatalf("intSetter failed: %v", err)
		}
		if intValue != 42 {
			t.Errorf("intValue = %d, want %d", intValue, 42)
		}

		var boolValue bool
		boolSetter := SetValue(&boolValue)
		if err := boolSetter(true); err != nil {
			t.Fatalf("boolSetter failed: %v", err)
		}
		if boolValue != true {
			t.Errorf("boolValue = %v, want %v", boolValue, true)
		}
	})
}

type mockSession struct {
	active bool
}

func TestSetSessionInitOnly(t *testing.T) {
	t.Run("allows setting when session is nil", func(t *testing.T) {
		var value string
		var session *mockSession
		setter := SetSessionInitOnly(&value, "TEST_VAR", &session)

		if err := setter("test"); err != nil {
			t.Fatalf("setter failed: %v", err)
		}

		if value != "test" {
			t.Errorf("value = %q, want %q", value, "test")
		}
	})

	t.Run("allows setting when session pointer points to nil", func(t *testing.T) {
		var value string
		var session *mockSession = nil
		setter := SetSessionInitOnly(&value, "TEST_VAR", &session)

		if err := setter("test"); err != nil {
			t.Fatalf("setter failed: %v", err)
		}

		if value != "test" {
			t.Errorf("value = %q, want %q", value, "test")
		}
	})

	t.Run("prevents setting when session exists", func(t *testing.T) {
		var value string
		session := &mockSession{active: true}
		setter := SetSessionInitOnly(&value, "TEST_VAR", &session)

		err := setter("test")
		if err == nil {
			t.Fatal("expected error when session exists")
		}

		expectedError := "TEST_VAR cannot be changed after session creation"
		if err.Error() != expectedError {
			t.Errorf("error = %q, want %q", err.Error(), expectedError)
		}

		// Value should not have changed
		if value != "" {
			t.Errorf("value = %q, want empty string", value)
		}
	})

	t.Run("works with different types", func(t *testing.T) {
		var intValue int
		var session *mockSession
		intSetter := SetSessionInitOnly(&intValue, "TEST_INT", &session)

		if err := intSetter(42); err != nil {
			t.Fatalf("intSetter failed: %v", err)
		}
		if intValue != 42 {
			t.Errorf("intValue = %d, want %d", intValue, 42)
		}
	})
}

// Test error propagation
func TestSetterWithError(t *testing.T) {
	// This tests that custom setters can return errors
	var value string
	customSetter := func(v string) error {
		if v == "invalid" {
			return errors.New("invalid value")
		}
		value = v
		return nil
	}

	// Test valid value
	if err := customSetter("valid"); err != nil {
		t.Fatalf("customSetter with valid value failed: %v", err)
	}
	if value != "valid" {
		t.Errorf("value = %q, want %q", value, "valid")
	}

	// Test invalid value
	err := customSetter("invalid")
	if err == nil {
		t.Fatal("expected error for invalid value")
	}
	if err.Error() != "invalid value" {
		t.Errorf("error = %q, want %q", err.Error(), "invalid value")
	}
}
