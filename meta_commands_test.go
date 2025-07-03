package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestIsMetaCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "shell command",
			input: "\\! ls",
			want:  true,
		},
		{
			name:  "shell command with spaces",
			input: "  \\! echo hello  ",
			want:  true,
		},
		{
			name:  "regular SQL",
			input: "SELECT 1",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "just backslash",
			input: "\\",
			want:  true,
		},
		{
			name:  "other meta command",
			input: "\\d ;",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMetaCommand(tt.input); got != tt.want {
				t.Errorf("IsMetaCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMetaCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Statement
		wantErr bool
	}{
		{
			name:  "shell command simple",
			input: "\\! ls",
			want:  &ShellMetaCommand{Command: "ls"},
		},
		{
			name:  "shell command with arguments",
			input: "\\! ls -la /tmp",
			want:  &ShellMetaCommand{Command: "ls -la /tmp"},
		},
		{
			name:  "shell command with extra spaces",
			input: "  \\!   echo   hello world  ",
			want:  &ShellMetaCommand{Command: "echo   hello world"},
		},
		{
			name:    "shell command without arguments",
			input:   "\\!",
			wantErr: true,
		},
		{
			name:  "source command simple",
			input: "\\. test.sql",
			want:  &SourceMetaCommand{FilePath: "test.sql"},
		},
		{
			name:  "source command with path",
			input: "\\. /path/to/script.sql",
			want:  &SourceMetaCommand{FilePath: "/path/to/script.sql"},
		},
		{
			name:  "source command with quotes",
			input: `\. "file with spaces.sql"`,
			want:  &SourceMetaCommand{FilePath: "file with spaces.sql"},
		},
		{
			name:  "source command with single quotes",
			input: `\. 'another file.sql'`,
			want:  &SourceMetaCommand{FilePath: "another file.sql"},
		},
		{
			name:    "source command without filename",
			input:   "\\.",
			wantErr: true,
		},
		{
			name:  "source command with multiple files",
			input: `\. file1.sql file2.sql`,
			wantErr: true,
		},
		{
			name:  "use database simple",
			input: "\\u mydb",
			want:  &UseDatabaseMetaCommand{Database: "mydb"},
		},
		{
			name:  "use database with hyphens",
			input: "\\u my-database",
			want:  &UseDatabaseMetaCommand{Database: "my-database"},
		},
		{
			name:  "use database with underscores",
			input: "\\u my_database",
			want:  &UseDatabaseMetaCommand{Database: "my_database"},
		},
		{
			name:  "use database with backticks",
			input: "\\u `my-database`",
			want:  &UseDatabaseMetaCommand{Database: "my-database"},
		},
		{
			name:  "use database with extra spaces",
			input: "  \\u   test_db  ",
			want:  &UseDatabaseMetaCommand{Database: "test_db"},
		},
		{
			name:    "use database without name",
			input:   "\\u",
			wantErr: true,
		},
		{
			name:    "use database with empty name",
			input:   "\\u ``",
			wantErr: true,
		},
		{
			name:    "unsupported meta command",
			input:   "\\d table_name",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "not a meta command",
			wantErr: true,
		},
		{
			name:  "prompt command simple",
			input: "\\R spanner> ",
			want:  &PromptMetaCommand{PromptString: "spanner>"},
		},
		{
			name:  "prompt command with percent expansion",
			input: "\\R %n@%p> ",
			want:  &PromptMetaCommand{PromptString: "%n@%p>"},
		},
		{
			name:  "prompt command with extra spaces",
			input: "  \\R   my custom prompt>  ",
			want:  &PromptMetaCommand{PromptString: "my custom prompt>"},
		},
		{
			name:  "prompt command with only spaces",
			input: "\\R    ",
			wantErr: true,
		},
		{
			name:  "prompt command with only trailing space",
			input: "\\R prompt ",
			want:  &PromptMetaCommand{PromptString: "prompt"},
		},
		{
			name:    "prompt command without string",
			input:   "\\R",
			wantErr: true,
		},
		{
			name:  "tee output simple",
			input: "\\T output.log",
			want:  &TeeOutputMetaCommand{FilePath: "output.log"},
		},
		{
			name:  "tee output with path",
			input: "\\T /path/to/output.log",
			want:  &TeeOutputMetaCommand{FilePath: "/path/to/output.log"},
		},
		{
			name:  "tee output with quotes",
			input: `\T "file with spaces.log"`,
			want:  &TeeOutputMetaCommand{FilePath: "file with spaces.log"},
		},
		{
			name:  "tee output with single quotes",
			input: `\T 'another file.log'`,
			want:  &TeeOutputMetaCommand{FilePath: "another file.log"},
		},
		{
			name:    "tee output without filename",
			input:   "\\T",
			wantErr: true,
		},
		{
			name:  "tee output with multiple files",
			input: `\T file1.log file2.log`,
			wantErr: true,
		},
		{
			name:  "disable tee",
			input: "\\t",
			want:  &DisableTeeMetaCommand{},
		},
		{
			name:  "disable tee with extra spaces",
			input: "  \\t  ",
			want:  &DisableTeeMetaCommand{},
		},
		{
			name:    "disable tee with arguments",
			input:   "\\t something",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMetaCommand(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMetaCommand(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				switch want := tt.want.(type) {
				case *ShellMetaCommand:
					if shell, ok := got.(*ShellMetaCommand); ok {
						if shell.Command != want.Command {
							t.Errorf("ParseMetaCommand(%q) = %q, want %q", tt.input, shell.Command, want.Command)
						}
					} else {
						t.Errorf("ParseMetaCommand(%q) returned %T, want *ShellMetaCommand", tt.input, got)
					}
				case *SourceMetaCommand:
					if source, ok := got.(*SourceMetaCommand); ok {
						if source.FilePath != want.FilePath {
							t.Errorf("ParseMetaCommand(%q) = %q, want %q", tt.input, source.FilePath, want.FilePath)
						}
					} else {
						t.Errorf("ParseMetaCommand(%q) returned %T, want *SourceMetaCommand", tt.input, got)
					}
				case *PromptMetaCommand:
					if prompt, ok := got.(*PromptMetaCommand); ok {
						if prompt.PromptString != want.PromptString {
							t.Errorf("ParseMetaCommand(%q) = %q, want %q", tt.input, prompt.PromptString, want.PromptString)
						}
					} else {
						t.Errorf("ParseMetaCommand(%q) returned %T, want *PromptMetaCommand", tt.input, got)
					}
				case *UseDatabaseMetaCommand:
					if use, ok := got.(*UseDatabaseMetaCommand); ok {
						if use.Database != want.Database {
							t.Errorf("ParseMetaCommand(%q) = %q, want %q", tt.input, use.Database, want.Database)
						}
					} else {
						t.Errorf("ParseMetaCommand(%q) returned %T, want *UseDatabaseMetaCommand", tt.input, got)
					}
				case *TeeOutputMetaCommand:
					if tee, ok := got.(*TeeOutputMetaCommand); ok {
						if tee.FilePath != want.FilePath {
							t.Errorf("ParseMetaCommand(%q) = %q, want %q", tt.input, tee.FilePath, want.FilePath)
						}
					} else {
						t.Errorf("ParseMetaCommand(%q) returned %T, want *TeeOutputMetaCommand", tt.input, got)
					}
				case *DisableTeeMetaCommand:
					if _, ok := got.(*DisableTeeMetaCommand); !ok {
						t.Errorf("ParseMetaCommand(%q) returned %T, want *DisableTeeMetaCommand", tt.input, got)
					}
				}
			}
		})
	}
}

func TestShellMetaCommand_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("system commands disabled", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = true
		sysVars.StreamManager = NewTeeManager(io.Discard, io.Discard)
		session := &Session{
			systemVariables: &sysVars,
		}

		cmd := &ShellMetaCommand{Command: "echo hello"}
		_, err := cmd.Execute(ctx, session)
		if err == nil {
			t.Error("Execute() should fail when system commands are disabled")
		}
		if err.Error() != "system commands are disabled" {
			t.Errorf("Execute() error = %v, want 'system commands are disabled'", err)
		}
	})

	t.Run("system commands enabled", func(t *testing.T) {
		var output bytes.Buffer
		var errOutput bytes.Buffer
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = false
		sysVars.StreamManager = NewTeeManager(&output, &errOutput)
		session := &Session{
			systemVariables: &sysVars,
		}

		cmd := &ShellMetaCommand{Command: "echo hello"}
		result, err := cmd.Execute(ctx, session)
		if err != nil {
			t.Errorf("Execute() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("Execute() returned nil result")
		}
		// Check that output was written
		if !strings.Contains(output.String(), "hello") {
			t.Errorf("Expected output to contain 'hello', got: %s", output.String())
		}
	})

	t.Run("exit status vs command not found", func(t *testing.T) {
		var output bytes.Buffer
		var errOutput bytes.Buffer
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = false
		sysVars.StreamManager = NewTeeManager(&output, &errOutput)
		session := &Session{
			systemVariables: &sysVars,
		}

		// Test case: Command that exits with non-zero status (should not return error)
		cmd := &ShellMetaCommand{Command: "exit 1"}
		_, err := cmd.Execute(ctx, session)
		if err != nil {
			t.Errorf("Execute() should not return error for exit status: %v", err)
		}
		
		// Test case: Command that fails (should also not return error since it's ExitError)
		cmd2 := &ShellMetaCommand{Command: "ls /nonexistent/directory"}
		_, err2 := cmd2.Execute(ctx, session)
		if err2 != nil {
			t.Errorf("Execute() should not return error for command that exits with error status: %v", err2)
		}
	})
}

func TestMetaCommandStatement_Interface(t *testing.T) {
	// Verify that ShellMetaCommand implements both interfaces
	var _ Statement = (*ShellMetaCommand)(nil)
	var _ MetaCommandStatement = (*ShellMetaCommand)(nil)

	// Verify that SourceMetaCommand implements both interfaces
	var _ Statement = (*SourceMetaCommand)(nil)
	var _ MetaCommandStatement = (*SourceMetaCommand)(nil)

	// Verify that PromptMetaCommand implements both interfaces
	var _ Statement = (*PromptMetaCommand)(nil)
	var _ MetaCommandStatement = (*PromptMetaCommand)(nil)

	// Verify that TeeOutputMetaCommand implements both interfaces
	var _ Statement = (*TeeOutputMetaCommand)(nil)
	var _ MetaCommandStatement = (*TeeOutputMetaCommand)(nil)

	// Verify that DisableTeeMetaCommand implements both interfaces
	var _ Statement = (*DisableTeeMetaCommand)(nil)
	var _ MetaCommandStatement = (*DisableTeeMetaCommand)(nil)

	// Test the marker method
	cmd := &ShellMetaCommand{Command: "test"}
	cmd.isMetaCommand() // This should compile

	srcCmd := &SourceMetaCommand{FilePath: "test.sql"}
	srcCmd.isMetaCommand() // This should compile

	promptCmd := &PromptMetaCommand{PromptString: "test> "}
	promptCmd.isMetaCommand() // This should compile

	teeCmd := &TeeOutputMetaCommand{FilePath: "test.log"}
	teeCmd.isMetaCommand() // This should compile

	disableTeeCmd := &DisableTeeMetaCommand{}
	disableTeeCmd.isMetaCommand() // This should compile
}

func TestPromptMetaCommand_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		promptString   string
		initialPrompt  string
		expectedPrompt string
		wantErr        bool
	}{
		{
			name:           "set simple prompt",
			promptString:   "my-prompt>",
			initialPrompt:  "",
			expectedPrompt: "my-prompt> ",  // Space added automatically
		},
		{
			name:           "set prompt with percent expansion",
			promptString:   "%n@%p>",
			initialPrompt:  "",
			expectedPrompt: "%n@%p> ",  // Space added automatically
		},
		{
			name:           "overwrite existing prompt",
			promptString:   "new-prompt>",
			initialPrompt:  "old-prompt> ",
			expectedPrompt: "new-prompt> ",  // Space added automatically
		},
		{
			name:           "empty prompt string",
			promptString:   "",
			initialPrompt:  "test> ",
			expectedPrompt: " ",  // Just a space
		},
		{
			name:           "complex prompt with multiple expansions",
			promptString:   "[%n/%d@%i:%p] %R%R>",
			initialPrompt:  "",
			expectedPrompt: "[%n/%d@%i:%p] %R%R> ",  // Space added automatically
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysVars := newSystemVariablesWithDefaults()
			if tt.initialPrompt != "" {
				sysVars.Prompt = tt.initialPrompt
			}
			session := &Session{
				systemVariables: &sysVars,
			}

			cmd := &PromptMetaCommand{PromptString: tt.promptString}
			result, err := cmd.Execute(ctx, session)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if result == nil {
					t.Error("Execute() returned nil result")
				}
				if sysVars.Prompt != tt.expectedPrompt {
					t.Errorf("Execute() prompt = %q, want %q", sysVars.Prompt, tt.expectedPrompt)
				}
			}
		})
	}
}

func TestParseMetaCommand_SingleCharacterOnly(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid shell command with space",
			input:       `\! echo test`,
			shouldError: false,
		},
		{
			name:        "shell command without arguments",
			input:       `\!`,
			shouldError: true,
			errorMsg:    "\\! requires a shell command",
		},
		{
			name:        "multi-char meta command",
			input:       `\foo`,
			shouldError: true,
			errorMsg:    "invalid meta command format",
		},
		{
			name:        "numeric meta command",
			input:       `\123`,
			shouldError: true,
			errorMsg:    "invalid meta command format",
		},
		{
			name:        "command-like string",
			input:       `\test command`,
			shouldError: true,
			errorMsg:    "invalid meta command format",
		},
		{
			name:        "no space after \\! (like \\!echo)",
			input:       `\!echo test`,
			shouldError: true,
			errorMsg:    "invalid meta command format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMetaCommand(tt.input)
			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for input %q, but got none", tt.input)
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input %q: %v", tt.input, err)
				}
			}
		})
	}
}

// Helper functions for tee meta command tests
func createTestSession(t *testing.T) (*Session, *systemVariables) {
	sysVars := newSystemVariablesWithDefaults()
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	sysVars.StreamManager = NewTeeManager(outBuf, errBuf)
	session := &Session{
		systemVariables: &sysVars,
	}
	return session, &sysVars
}

func TestTeeOutputMetaCommand_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		filePath   string
		setupFunc  func() (string, func()) // returns path and cleanup func
		wantErr    bool
		errContains string
	}{
		{
			name:     "enable tee to new file",
			filePath: "test_tee_output.log",
			setupFunc: func() (string, func()) {
				tmpDir := t.TempDir()
				path := tmpDir + "/test_tee_output.log"
				return path, func() {}
			},
		},
		{
			name:     "enable tee to existing file",
			filePath: "existing_test.log",
			setupFunc: func() (string, func()) {
				tmpDir := t.TempDir()
				path := tmpDir + "/existing_test.log"
				// Create file with some content
				if err := os.WriteFile(path, []byte("existing content\n"), 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {}
			},
		},
		{
			name:     "enable tee to directory",
			filePath: "test_dir",
			setupFunc: func() (string, func()) {
				tmpDir := t.TempDir()
				path := tmpDir + "/test_dir"
				// Create a directory
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
				return path, func() {}
			},
			wantErr: true,
			errContains: "is a directory", // OpenFile returns this error for directories
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup := tt.setupFunc()
			defer cleanup()

			session, sysVars := createTestSession(t)
			// Store the original writer before enabling tee
			originalWriter := sysVars.StreamManager.GetWriter()
			
			cmd := &TeeOutputMetaCommand{FilePath: path}
			result, err := cmd.Execute(ctx, session)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Execute() error = %v, want error containing %q", err, tt.errContains)
			}
			if !tt.wantErr {
				if result == nil {
					t.Error("Execute() returned nil result")
				}
				// Verify that writer has been updated after enabling tee
				newWriter := sysVars.StreamManager.GetWriter()
				if newWriter == originalWriter {
					t.Error("StreamManager writer was not updated after enabling tee")
				}
			}
		})
	}
}

func TestDisableTeeMetaCommand_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		setupTee      bool
		expectChanged bool
	}{
		{
			name:          "disable tee when enabled",
			setupTee:      true,
			expectChanged: true,
		},
		{
			name:          "disable tee when not enabled",
			setupTee:      false,
			expectChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, sysVars := createTestSession(t)
			// Store the original writer before enabling tee
			originalWriter := sysVars.StreamManager.GetWriter()
			
			if tt.setupTee {
				// Enable tee first
				tmpDir := t.TempDir()
				teeFile := tmpDir + "/test.log"
				if err := sysVars.StreamManager.EnableTee(teeFile); err != nil {
					t.Fatal(err)
				}
				// Verify tee is enabled (writer should be different from original)
				newWriter := sysVars.StreamManager.GetWriter()
				if newWriter == originalWriter {
					t.Error("Tee was not properly enabled")
				}
			}

			cmd := &DisableTeeMetaCommand{}
			result, err := cmd.Execute(ctx, session)
			if err != nil {
				t.Errorf("Execute() error = %v, want nil", err)
			}
			if result == nil {
				t.Error("Execute() returned nil result")
			}
			
			// Verify that writer reverts to the original after disable
			finalWriter := sysVars.StreamManager.GetWriter()
			if finalWriter != originalWriter {
				t.Error("StreamManager writer should revert to original after disable")
			}
		})
	}
}