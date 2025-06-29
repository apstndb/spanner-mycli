package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
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

// Ensure ShellMetaCommand implements both Statement and MetaCommandStatement
var _ Statement = (*ShellMetaCommand)(nil)
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

	// Check if output stream is configured
	if session.systemVariables.CurrentOutStream == nil {
		slog.Error("CurrentOutStream is nil, cannot write shell command output", "command", s.Command)
		return nil, errors.New("internal error: output stream not configured")
	}
	if session.systemVariables.CurrentErrStream == nil {
		slog.Error("CurrentErrStream is nil, cannot write shell command error output", "command", s.Command)
		return nil, errors.New("internal error: error stream not configured")
	}

	// Stream stdout and stderr directly to avoid buffering large amounts of data in memory
	shellCmd.Stdout = session.systemVariables.CurrentOutStream
	shellCmd.Stderr = session.systemVariables.CurrentErrStream

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
	default:
		return nil, fmt.Errorf("unsupported meta command: \\%s", command)
	}
}

// IsMetaCommand checks if a line starts with a backslash (meta command)
func IsMetaCommand(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "\\")
}