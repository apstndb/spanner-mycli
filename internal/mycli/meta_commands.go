package mycli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/kballard/go-shellquote"
)

// MetaCommandStatement is a marker interface for meta commands (commands starting with \).
// Meta commands are not SQL statements and have special handling in the CLI.
type MetaCommandStatement interface {
	Statement
	isMetaCommand()
}

// ShellMetaCommand executes system shell commands using \! syntax
type ShellMetaCommand struct {
	Command string
}

// Ensure ShellMetaCommand implements MetaCommandStatement
var _ MetaCommandStatement = (*ShellMetaCommand)(nil)

// isMetaCommand marks this as a meta command
func (s *ShellMetaCommand) isMetaCommand() {}

// Execute runs the shell command
func (s *ShellMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	// Check if system commands are disabled
	if session.systemVariables.SkipSystemCommand {
		return nil, errors.New("system commands are disabled")
	}

	// Choose shell based on platform
	var shellCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shellCmd = exec.CommandContext(ctx, "cmd", "/c", s.Command)
	} else {
		shellCmd = exec.CommandContext(ctx, "sh", "-c", s.Command)
	}

	// Check if StreamManager is configured
	if session.systemVariables.StreamManager == nil {
		slog.Error("StreamManager is nil, cannot execute shell command", "command", s.Command)
		return nil, errors.New("internal error: StreamManager not configured")
	}

	// Stream stdout and stderr directly to avoid buffering large amounts of data in memory
	shellCmd.Stdout = session.systemVariables.StreamManager.GetWriter()
	shellCmd.Stderr = session.systemVariables.StreamManager.GetErrStream()

	// Execute the command
	if err := shellCmd.Run(); err != nil {
		// If it's an ExitError, the command ran but returned a non-zero status.
		// The command's own stderr has already been printed. We can consider this
		// a "successful" execution from the CLI's perspective and not print a
		// redundant error message.
		if _, ok := err.(*exec.ExitError); ok {
			return &Result{}, nil
		}
		// For other errors (e.g., command not found), it's a genuine execution error.
		return nil, fmt.Errorf("command failed: %w", err)
	}

	// Return empty result
	return &Result{}, nil
}

// metaCommandPattern matches meta commands starting with \ followed by a single character
var metaCommandPattern = regexp.MustCompile(`^\\(.)(?:\s+(.*))?$`)

// ParseMetaCommand parses a meta command string into a Statement
func ParseMetaCommand(input string) (Statement, error) {
	trimmed := strings.TrimSpace(input)
	matches := metaCommandPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return nil, errors.New("invalid meta command format")
	}

	command := matches[1]
	args := ""
	if len(matches) > 2 {
		args = matches[2]
	}

	switch command {
	case "!":
		if args == "" {
			return nil, errors.New("\\! requires a shell command")
		}
		return &ShellMetaCommand{Command: args}, nil
	case ".":
		if args == "" {
			return nil, errors.New("\\. requires a filename")
		}
		// Use shellquote for proper parsing of quoted filenames
		words, err := shellquote.Split(args)
		if err != nil {
			return nil, fmt.Errorf("invalid filename quoting: %w", err)
		}
		if len(words) != 1 {
			return nil, errors.New("\\. requires exactly one filename")
		}
		return &SourceMetaCommand{FilePath: words[0]}, nil
	case "R":
		trimmedArgs := strings.TrimSpace(args)
		if trimmedArgs == "" {
			return nil, errors.New("\\R requires a prompt string")
		}
		return &PromptMetaCommand{PromptString: trimmedArgs}, nil
	case "u":
		if args == "" {
			return nil, errors.New("\\u requires a database name")
		}
		// Trim spaces and remove backticks if present (SQL-style quoting)
		database := strings.Trim(strings.TrimSpace(args), "`")
		if database == "" {
			return nil, errors.New("\\u requires a database name")
		}
		return &UseDatabaseMetaCommand{Database: database}, nil
	case "T":
		if args == "" {
			return nil, errors.New("\\T requires a filename")
		}
		// Use shellquote for proper parsing of quoted filenames
		words, err := shellquote.Split(args)
		if err != nil {
			return nil, fmt.Errorf("invalid filename quoting: %w", err)
		}
		if len(words) != 1 {
			return nil, errors.New("\\T requires exactly one filename")
		}
		return &TeeOutputMetaCommand{FilePath: words[0]}, nil
	case "o":
		// \o with no args disables output redirect (returns to stdout)
		if args == "" {
			return &DisableOutputRedirectMetaCommand{}, nil
		}
		// Use shellquote for proper parsing of quoted filenames
		words, err := shellquote.Split(args)
		if err != nil {
			return nil, fmt.Errorf("invalid filename quoting: %w", err)
		}
		if len(words) != 1 {
			return nil, errors.New("\\o requires exactly one filename")
		}
		return &OutputRedirectMetaCommand{FilePath: words[0]}, nil
	case "O":
		// \O command disables output redirect (symmetric with \t for tee)
		if args != "" {
			return nil, errors.New("\\O takes no arguments")
		}
		return &DisableOutputRedirectMetaCommand{}, nil
	case "t":
		// \t command takes no arguments
		if args != "" {
			return nil, errors.New("\\t does not accept arguments")
		}
		return &DisableTeeMetaCommand{}, nil
	default:
		return nil, fmt.Errorf("unsupported meta command: \\%s", command)
	}
}

// SourceMetaCommand executes SQL statements from a file using \. syntax
type SourceMetaCommand struct {
	FilePath string
}

// Ensure SourceMetaCommand implements MetaCommandStatement
var _ MetaCommandStatement = (*SourceMetaCommand)(nil)

// isMetaCommand marks this as a meta command
func (s *SourceMetaCommand) isMetaCommand() {}

// Execute is not used for SourceMetaCommand as it's handled specially in CLI
func (s *SourceMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	// This should not be called as SourceMetaCommand is handled in handleSpecialStatements.
	// While panic might be more appropriate for this logic error, we follow the
	// codebase convention of avoiding panics and return an error instead.
	return nil, errors.New("SourceMetaCommand.Execute should not be called; it must be handled by the CLI")
}

// PromptMetaCommand changes the prompt string using \R syntax
type PromptMetaCommand struct {
	PromptString string
}

// Ensure PromptMetaCommand implements MetaCommandStatement
var _ MetaCommandStatement = (*PromptMetaCommand)(nil)

// isMetaCommand marks this as a meta command
func (p *PromptMetaCommand) isMetaCommand() {}

// Execute updates the CLI_PROMPT system variable
func (p *PromptMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	// Add a trailing space to the prompt for better UX (separation between prompt and input)
	// This ensures compatibility with Google Cloud Spanner CLI behavior
	promptWithSpace := p.PromptString + " "
	// Use SetFromSimple since meta commands provide raw strings, not GoogleSQL expressions
	if err := session.systemVariables.SetFromSimple("CLI_PROMPT", promptWithSpace); err != nil {
		return nil, fmt.Errorf("failed to set prompt: %w", err)
	}
	return &Result{}, nil
}

// UseDatabaseMetaCommand switches database using \u syntax
type UseDatabaseMetaCommand struct {
	Database string
}

// Ensure UseDatabaseMetaCommand implements both Statement and MetaCommandStatement
var (
	_ Statement            = (*UseDatabaseMetaCommand)(nil)
	_ MetaCommandStatement = (*UseDatabaseMetaCommand)(nil)
)

// isMetaCommand marks this as a meta command
func (s *UseDatabaseMetaCommand) isMetaCommand() {}

// isDetachedCompatible allows this command to run in detached mode
func (s *UseDatabaseMetaCommand) isDetachedCompatible() {}

// Execute is required by Statement interface but the actual logic is handled in SessionHandler
func (s *UseDatabaseMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	// This should not be called as UseDatabaseMetaCommand is handled in SessionHandler.
	// While panic might be more appropriate for this logic error, we follow the
	// codebase convention of avoiding panics and return an error instead.
	return nil, errors.New("UseDatabaseMetaCommand.Execute should not be called; it must be handled by the SessionHandler")
}

// IsMetaCommand checks if a line starts with a backslash (meta command)
func IsMetaCommand(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "\\")
}

// TeeOutputMetaCommand enables output tee to a file using \T syntax (MySQL-style: both screen and file)
type TeeOutputMetaCommand struct {
	FilePath string
}

// OutputRedirectMetaCommand redirects output to a file using \o syntax (PostgreSQL-style: file only)
type OutputRedirectMetaCommand struct {
	FilePath string
}

// Ensure TeeOutputMetaCommand implements MetaCommandStatement
var _ MetaCommandStatement = (*TeeOutputMetaCommand)(nil)

// isMetaCommand marks this as a meta command
func (t *TeeOutputMetaCommand) isMetaCommand() {}

// Execute enables tee output to the specified file (both screen and file)
func (t *TeeOutputMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	// Validate that we have system variables and stream manager available
	if session.systemVariables == nil {
		return nil, errors.New("internal error: system variables not initialized")
	}
	if session.systemVariables.StreamManager == nil {
		return nil, errors.New("internal error: stream manager not initialized")
	}

	// Enable tee for the specified file (normal mode: both screen and file)
	if err := session.systemVariables.StreamManager.EnableTee(t.FilePath, false); err != nil {
		return nil, err
	}

	return &Result{}, nil
}

// Ensure OutputRedirectMetaCommand implements MetaCommandStatement
var _ MetaCommandStatement = (*OutputRedirectMetaCommand)(nil)

// isMetaCommand marks this as a meta command
func (o *OutputRedirectMetaCommand) isMetaCommand() {}

// Execute enables output redirect to the specified file (file only)
func (o *OutputRedirectMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	// Validate that we have system variables and stream manager available
	if session.systemVariables == nil {
		return nil, errors.New("internal error: system variables not initialized")
	}
	if session.systemVariables.StreamManager == nil {
		return nil, errors.New("internal error: stream manager not initialized")
	}

	// Enable output redirect (silent mode: file only)
	if err := session.systemVariables.StreamManager.EnableTee(o.FilePath, true); err != nil {
		return nil, err
	}

	return &Result{}, nil
}

// DisableTeeMetaCommand disables output tee using \t syntax
type DisableTeeMetaCommand struct{}

// DisableOutputRedirectMetaCommand disables output redirect using \o (with no args) syntax
type DisableOutputRedirectMetaCommand struct{}

// Ensure DisableTeeMetaCommand implements MetaCommandStatement
var _ MetaCommandStatement = (*DisableTeeMetaCommand)(nil)

// isMetaCommand marks this as a meta command
func (d *DisableTeeMetaCommand) isMetaCommand() {}

// Ensure DisableOutputRedirectMetaCommand implements MetaCommandStatement
var _ MetaCommandStatement = (*DisableOutputRedirectMetaCommand)(nil)

// isMetaCommand marks this as a meta command
func (d *DisableOutputRedirectMetaCommand) isMetaCommand() {}

// disableOutput is a helper function to disable output (shared by \t and \o commands)
func disableOutput(session *Session) (*Result, error) {
	// Validate that we have system variables and stream manager available
	if session.systemVariables == nil {
		return nil, errors.New("internal error: system variables not initialized")
	}
	if session.systemVariables.StreamManager == nil {
		return nil, errors.New("internal error: stream manager not initialized")
	}

	// Disable output
	session.systemVariables.StreamManager.DisableTee()

	return &Result{}, nil
}

// Execute disables tee output
func (d *DisableTeeMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	return disableOutput(session)
}

// Execute disables output redirect (returns to stdout)
func (d *DisableOutputRedirectMetaCommand) Execute(ctx context.Context, session *Session) (*Result, error) {
	return disableOutput(session)
}
