package main

import (
	"context"
	"errors"
	"fmt"
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

	// Execute the command and capture output
	output, err := shellCmd.CombinedOutput()
	if err != nil {
		// Include command output in error message if available
		if len(output) > 0 {
			return nil, fmt.Errorf("command failed: %w\n%s", err, string(output))
		}
		return nil, fmt.Errorf("command failed: %w", err)
	}

	// Print output directly (matching official spannercli behavior)
	if len(output) > 0 {
		fmt.Print(string(output))
	}

	// Return empty result
	return &Result{}, nil
}

// metaCommandPattern matches meta commands starting with \
var metaCommandPattern = regexp.MustCompile(`^\\(\S+)(?:\s+(.*))?$`)

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