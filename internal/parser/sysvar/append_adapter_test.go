package sysvar_test

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/parser"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

func TestProtoDescriptorFileParser(t *testing.T) {
	var files []string
	var setterCalled bool
	var lastAppended string

	p := sysvar.NewProtoDescriptorFileParser(
		"TEST_PROTO_FILES",
		"Test proto files",
		func() []string { return files },
		func(f []string) error {
			files = f
			setterCalled = true
			return nil
		},
		func(f string) error {
			files = append(files, f)
			lastAppended = f
			return nil
		},
		nil, // No validation
	)

	t.Run("Set operation with GoogleSQL mode", func(t *testing.T) {
		setterCalled = false
		err := p.ParseAndSetWithMode(`"file1.proto,file2.proto"`, parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseAndSetWithMode failed: %v", err)
		}
		if !setterCalled {
			t.Error("Setter was not called")
		}
		if len(files) != 2 || files[0] != "file1.proto" || files[1] != "file2.proto" {
			t.Errorf("Files = %v, want [file1.proto file2.proto]", files)
		}

		// Test GetValue
		value, err := p.GetValue()
		if err != nil {
			t.Fatalf("GetValue failed: %v", err)
		}
		if value != "file1.proto,file2.proto" {
			t.Errorf("GetValue = %q, want %q", value, "file1.proto,file2.proto")
		}
	})

	t.Run("Set operation with Simple mode", func(t *testing.T) {
		setterCalled = false
		err := p.ParseAndSetWithMode("file3.proto,file4.proto", parser.ParseModeSimple)
		if err != nil {
			t.Fatalf("ParseAndSetWithMode failed: %v", err)
		}
		if !setterCalled {
			t.Error("Setter was not called")
		}
		if len(files) != 2 || files[0] != "file3.proto" || files[1] != "file4.proto" {
			t.Errorf("Files = %v, want [file3.proto file4.proto]", files)
		}
	})

	t.Run("Append operation with GoogleSQL mode", func(t *testing.T) {
		// Start with existing files
		files = []string{"existing.proto"}
		lastAppended = ""

		// ProtoDescriptorFileParser implements AppendableVariableParser, so we can call AppendWithMode directly

		err := p.AppendWithMode(`"new.proto"`, parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("AppendWithMode failed: %v", err)
		}
		if lastAppended != "new.proto" {
			t.Errorf("lastAppended = %q, want %q", lastAppended, "new.proto")
		}
		if len(files) != 2 || files[0] != "existing.proto" || files[1] != "new.proto" {
			t.Errorf("Files = %v, want [existing.proto new.proto]", files)
		}
	})

	t.Run("Append operation with Simple mode", func(t *testing.T) {
		// Continue from previous test
		lastAppended = ""

		// ProtoDescriptorFileParser implements AppendableVariableParser, so we can call AppendWithMode directly

		err := p.AppendWithMode("another.proto", parser.ParseModeSimple)
		if err != nil {
			t.Fatalf("AppendWithMode failed: %v", err)
		}
		if lastAppended != "another.proto" {
			t.Errorf("lastAppended = %q, want %q", lastAppended, "another.proto")
		}
		if len(files) != 3 {
			t.Errorf("Files = %v, want 3 files", files)
		}
	})

	t.Run("Empty set clears files", func(t *testing.T) {
		err := p.ParseAndSetWithMode(`""`, parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseAndSetWithMode failed: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("Files = %v, want empty", files)
		}
	})
}

func TestRegistryAppendSupport(t *testing.T) {
	registry := sysvar.NewRegistry()

	// Register a regular variable
	regularVar := sysvar.NewStringParser(
		"REGULAR_VAR",
		"Regular variable",
		func() string { return testString },
		func(v string) error {
			testString = v
			return nil
		},
	)
	if err := registry.Register(regularVar); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Register an appendable variable
	var files []string
	appendableVar := sysvar.NewProtoDescriptorFileParser(
		"APPENDABLE_VAR",
		"Appendable variable",
		func() []string { return files },
		func(f []string) error { files = f; return nil },
		func(f string) error { files = append(files, f); return nil },
		nil,
	)
	if err := registry.Register(appendableVar); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Test HasAppendSupport
	if registry.HasAppendSupport("REGULAR_VAR") {
		t.Error("REGULAR_VAR should not have append support")
	}
	if !registry.HasAppendSupport("APPENDABLE_VAR") {
		t.Error("APPENDABLE_VAR should have append support")
	}

	// Test AppendFromGoogleSQL on regular variable
	err := registry.AppendFromGoogleSQL("REGULAR_VAR", `"value"`)
	if err == nil {
		t.Error("Expected error when appending to regular variable")
	}

	// Test AppendFromGoogleSQL on appendable variable
	err = registry.AppendFromGoogleSQL("APPENDABLE_VAR", `"file1.proto"`)
	if err != nil {
		t.Fatalf("AppendFromGoogleSQL failed: %v", err)
	}
	if len(files) != 1 || files[0] != "file1.proto" {
		t.Errorf("Files = %v, want [file1.proto]", files)
	}
}

var testString string // Used in tests
