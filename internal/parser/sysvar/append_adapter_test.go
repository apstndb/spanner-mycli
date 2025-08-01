package sysvar_test

import (
	"fmt"
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
		
		// Test empty string in Simple mode
		files = []string{"test.proto"}
		err = p.ParseAndSetWithMode("", parser.ParseModeSimple)
		if err != nil {
			t.Fatalf("ParseAndSetWithMode (simple) failed: %v", err)
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

func TestProtoDescriptorFileParserEdgeCases(t *testing.T) {
	var files []string
	p := sysvar.NewProtoDescriptorFileParser(
		"PROTO_FILES",
		"Proto descriptor files",
		func() []string { return files },
		func(f []string) error { files = f; return nil },
		func(f string) error {
			files = append(files, f)
			return nil
		},
		func(f string) error {
			if f == "invalid.pb" {
				return fmt.Errorf("invalid file")
			}
			return nil
		},
	)
	
	// Test Description
	if p.Description() != "Proto descriptor files" {
		t.Errorf("Description() = %q, want %q", p.Description(), "Proto descriptor files")
	}
	
	// Test IsReadOnly (should be false since we have setter and appender)
	if p.IsReadOnly() {
		t.Error("Expected IsReadOnly to return false")
	}
	
	// Test validation error in set
	if err := p.ParseAndSetWithMode("invalid.pb", parser.ParseModeSimple); err == nil {
		t.Error("Expected validation error for invalid.pb")
	}
	
	// Test validation error in append
	if err := p.AppendWithMode("invalid.pb", parser.ParseModeSimple); err == nil {
		t.Error("Expected validation error for invalid.pb in append")
	}
	
	// Test append with existing file (should call appender anyway)
	files = []string{"existing.pb"}
	if err := p.AppendWithMode("existing.pb", parser.ParseModeSimple); err != nil {
		t.Fatalf("AppendWithMode(existing.pb) failed: %v", err)
	}
	// Files should still have appended the duplicate
	if len(files) != 2 {
		t.Errorf("Expected 2 files after appending duplicate, got %d", len(files))
	}
	
	// Test unsupported parse mode
	if err := p.ParseAndSetWithMode("test", parser.ParseMode(999)); err == nil {
		t.Error("Expected error for unsupported parse mode")
	}
	if err := p.AppendWithMode("test", parser.ParseMode(999)); err == nil {
		t.Error("Expected error for unsupported parse mode in append")
	}
	
	// Test invalid GoogleSQL string literal in ParseAndSetWithMode
	if err := p.ParseAndSetWithMode("unclosed'", parser.ParseModeGoogleSQL); err == nil {
		t.Error("Expected error for invalid GoogleSQL string")
	}
	
	// Test invalid GoogleSQL string literal in AppendWithMode
	if err := p.AppendWithMode("unclosed'", parser.ParseModeGoogleSQL); err == nil {
		t.Error("Expected error for invalid GoogleSQL string in append")
	}
}

func TestProtoDescriptorFileParserReadOnly(t *testing.T) {
	// Create a read-only parser (no setter and no appender)
	parser := sysvar.NewProtoDescriptorFileParser(
		"RO_PROTO",
		"Read-only proto files",
		func() []string { return []string{"file1.pb", "file2.pb"} },
		nil, // No setter
		nil, // No appender
		nil,
	)
	
	// Test IsReadOnly
	if !parser.IsReadOnly() {
		t.Error("Expected IsReadOnly to return true when setter and appender are nil")
	}
}

func TestAppendFromSimple(t *testing.T) {
	registry := sysvar.NewRegistry()
	
	var files []string
	parser := sysvar.NewProtoDescriptorFileParser(
		"TEST_FILES",
		"Test files",
		func() []string { return files },
		func(f []string) error { files = f; return nil },
		func(f string) error {
			files = append(files, f)
			return nil
		},
		nil,
	)
	
	if err := registry.Register(parser); err != nil {
		t.Fatalf("Failed to register parser: %v", err)
	}
	
	// Test AppendFromSimple
	if err := registry.AppendFromSimple("TEST_FILES", "test.txt"); err != nil {
		t.Fatalf("AppendFromSimple failed: %v", err)
	}
	if len(files) != 1 || files[0] != "test.txt" {
		t.Errorf("files = %v, want [test.txt]", files)
	}
	
	// Test append to non-existent variable
	if err := registry.AppendFromSimple("NON_EXISTENT", "value"); err == nil {
		t.Error("Expected error when appending to non-existent variable")
	}
}

func TestAppendableTypedVariableParserAppendWithMode(t *testing.T) {
	// AppendableTypedVariableParser is an internal implementation detail
	// It's tested indirectly through ProtoDescriptorFileParser which uses it internally
	// The AppendWithMode method is exercised through the tests above
	t.Skip("AppendableTypedVariableParser.AppendWithMode is tested through ProtoDescriptorFileParser")
}
