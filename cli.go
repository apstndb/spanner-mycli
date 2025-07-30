//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/hymkor/go-multiline-ny"
	"github.com/kballard/go-shellquote"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
)

const (
	exitCodeSuccess = 0
	exitCodeError   = 1
)

type Cli struct {
	SessionHandler  *SessionHandler
	Credential      []byte
	SystemVariables *systemVariables
	waitingStatus   string
}

func NewCli(ctx context.Context, credential []byte, sysVars *systemVariables) (*Cli, error) {
	session, err := createSession(ctx, credential, sysVars)
	if err != nil {
		return nil, err
	}

	sessionHandler := NewSessionHandler(session)

	// StreamManager now manages the TTY stream internally

	return &Cli{
		SessionHandler:  sessionHandler,
		Credential:      credential,
		SystemVariables: sysVars,
	}, nil
}

// GetWriter returns the current output writer (with or without tee)
func (c *Cli) GetWriter() io.Writer {
	return c.SystemVariables.StreamManager.GetWriter()
}

// GetTtyStream returns the TTY stream for terminal operations
func (c *Cli) GetTtyStream() *os.File {
	return c.SystemVariables.StreamManager.GetTtyStream()
}

// GetErrStream returns the error stream
func (c *Cli) GetErrStream() io.Writer {
	return c.SystemVariables.StreamManager.GetErrStream()
}

// GetInStream returns the input stream
func (c *Cli) GetInStream() io.ReadCloser {
	return c.SystemVariables.StreamManager.GetInStream()
}

func (c *Cli) RunInteractive(ctx context.Context) error {
	// Only check database existence if we're not in detached mode
	if !c.SessionHandler.IsDetached() {
		exists, err := c.SessionHandler.DatabaseExists()
		if err != nil {
			return NewExitCodeError(c.ExitOnError(err))
		}

		if exists {
			fmt.Fprintf(c.GetWriter(), "Connected.\n")
		} else {
			return NewExitCodeError(c.ExitOnError(fmt.Errorf("unknown database %q", c.SystemVariables.Database)))
		}
	} else {
		fmt.Fprintf(c.GetWriter(), "Connected in detached mode.\n")
	}

	ed, history, err := initializeMultilineEditor(c)
	if err != nil {
		return NewExitCodeError(c.ExitOnError(err))
	}

	// ensure reset
	c.waitingStatus = ""

	for {
		// Reset everytime to reflect system variable
		setLineEditor(ed, c.SystemVariables.EnableHighlight)

		input, err := c.readInputLine(ctx, ed)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				fmt.Fprintln(c.GetWriter(), "Bye")
				return NewExitCodeError(c.handleExit())
			case isInterrupted(err):
				// This section is currently redundant but keep as intended
				c.PrintInteractiveError(err)
				continue
			default:
				c.PrintInteractiveError(err)
				continue
			}
		}

		stmt, err := c.parseStatement(input)
		if err != nil {
			c.PrintInteractiveError(err)
			continue
		}

		// Add to history with appropriate terminator
		if IsMetaCommand(input.statement) {
			history.Add(input.statement)
		} else {
			history.Add(input.statement + ";")
		}

		if exitCode, processed := c.handleSpecialStatements(ctx, stmt); processed {
			if exitCode >= 0 {
				return NewExitCodeError(exitCode)
			}
			continue
		}

		preInput, err := c.executeStatementInteractive(ctx, stmt, input)
		if err != nil {
			c.PrintInteractiveError(err)
			continue
		}

		ed.SetDefault(strings.Split(preInput, "\n"))
	}
}

// readInputLine reads and processes an input line from the editor.
func (c *Cli) readInputLine(ctx context.Context, ed *multiline.Editor) (*inputStatement, error) {
	input, err := readInteractiveInput(ctx, ed)

	// reset for next input before continue
	ed.SetDefault(nil)

	if err != nil {
		return nil, err
	}

	return input, nil
}

// parseStatement parses the input statement.
func (c *Cli) parseStatement(input *inputStatement) (Statement, error) {
	// Check if this is a meta command
	if IsMetaCommand(input.statement) {
		return ParseMetaCommand(input.statement)
	}
	return BuildStatementWithCommentsWithMode(input.statementWithoutComments, input.statement, c.SystemVariables.BuildStatementMode)
}

// handleSpecialStatements handles special client-side statements.
// Returns (exitCode, processed) where:
// - exitCode: if >= 0, the function should return this exit code
// - processed: if true, the main loop should continue (either returning exitCode or skipping to next iteration)
func (c *Cli) handleSpecialStatements(ctx context.Context, stmt Statement) (exitCode int, processed bool) {
	// Handle ExitStatement
	if _, ok := stmt.(*ExitStatement); ok {
		fmt.Fprintln(c.GetWriter(), "Bye")
		return c.handleExit(), true
	}

	// Handle SourceMetaCommand
	if s, ok := stmt.(*SourceMetaCommand); ok {
		err := c.executeSourceFile(ctx, s.FilePath)
		if err != nil {
			c.PrintInteractiveError(err)
		}
		return -1, true
	}

	// Handle DropDatabaseStatement
	if s, ok := stmt.(*DropDatabaseStatement); ok {
		if c.SystemVariables.Database == s.DatabaseId {
			c.PrintInteractiveError(
				fmt.Errorf("database %q is currently used, it can not be dropped", s.DatabaseId),
			)
			return -1, true
		}

		// Interactive confirmations require a TTY
		ttyStream := c.GetTtyStream()
		if ttyStream == nil {
			c.PrintInteractiveError(fmt.Errorf("cannot confirm DROP DATABASE without a TTY for output; stdout is not a terminal"))
			return -1, true
		}
		if !confirm(c.GetInStream(), ttyStream, fmt.Sprintf("Database %q will be dropped.\nDo you want to continue?", s.DatabaseId)) {
			return -1, true
		}
	}

	return -1, false
}

// executeStatementInteractive executes the statement and displays the result.
func (c *Cli) executeStatementInteractive(ctx context.Context, stmt Statement, input *inputStatement) (string, error) {
	preInput, err := c.executeStatement(ctx, stmt, true, input.statement, c.GetWriter())
	if err != nil {
		return "", err
	}

	return preInput, nil
}

func (c *Cli) updateSystemVariables(result *Result) {
	if result.IsMutation {
		c.SystemVariables.ReadTimestamp = time.Time{}
		c.SystemVariables.CommitTimestamp = result.Timestamp
	} else {
		c.SystemVariables.ReadTimestamp = result.Timestamp
		c.SystemVariables.CommitTimestamp = time.Time{}
	}

	if result.CommitStats != nil {
		c.SystemVariables.CommitResponse = &sppb.CommitResponse{CommitStats: result.CommitStats, CommitTimestamp: timestamppb.New(result.Timestamp)}
	} else {
		c.SystemVariables.CommitResponse = nil
	}
}

// executeSourceFile executes SQL statements from a file
func (c *Cli) executeSourceFile(ctx context.Context, filePath string) error {
	// Open the file to get a stable file handle
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer f.Close()

	// Check if the file is a regular file to prevent DoS from special files
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("sourcing from a non-regular file is not supported: %s", filePath)
	}

	// Add a check to prevent reading excessively large files.
	const maxFileSize = 100 * 1024 * 1024 // 100MB
	if fi.Size() > maxFileSize {
		return fmt.Errorf("file %s is too large to be sourced (size: %d bytes, max: %d bytes)", filePath, fi.Size(), maxFileSize)
	}

	// Read the file contents from the opened handle
	contents, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Parse the contents using buildCommands (same as batch mode)
	// IMPORTANT: buildCommands explicitly rejects meta-commands (including \.) with the error
	// "meta commands are not supported in batch mode". This means nested sourcing
	// (i.e., having \. commands within a sourced file) is not possible by design.
	// Meta-commands are only processed in the interactive mode's main loop.
	stmts, err := buildCommands(string(contents), c.SystemVariables.BuildStatementMode)
	if err != nil {
		return fmt.Errorf("failed to parse SQL from file %s: %w", filePath, err)
	}

	// Execute each statement sequentially
	for i, fileStmt := range stmts {
		// Skip ExitStatement in source files (exit should only end the file, not the session)
		if _, ok := fileStmt.(*ExitStatement); ok {
			continue
		}

		// Extract SQL text for ECHO support
		// TODO: Currently we reconstruct the SQL text using Statement.String() method.
		// Ideally, buildCommands() should return the original text from the file
		// to echo exactly what was written in the source file.
		// See: https://github.com/apstndb/spanner-mycli/issues/380
		sqlText := ""
		if stringer, ok := fileStmt.(fmt.Stringer); ok {
			sqlText = stringer.String()
		}

		// Execute the statement in interactive mode to get proper output formatting
		_, err = c.executeStatement(ctx, fileStmt, true, sqlText, c.GetWriter())
		if err != nil {
			return fmt.Errorf("error executing statement %d from file %s: %w", i+1, filePath, err)
		}
	}

	return nil
}

func (c *Cli) RunBatch(ctx context.Context, input string) error {
	stmts, err := buildCommands(input, c.SystemVariables.BuildStatementMode)
	if err != nil {
		c.PrintBatchError(err)
		return NewExitCodeError(exitCodeError)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go handleInterrupt(ctx, cancel)

	for _, stmt := range stmts {
		if _, ok := stmt.(*ExitStatement); ok {
			return NewExitCodeError(c.handleExit())
		}

		_, err = c.executeStatement(ctx, stmt, false, input, c.GetWriter())
		if err != nil {
			c.PrintBatchError(err)
			return NewExitCodeError(exitCodeError)
		}
	}

	return nil
}

// handleExit processes EXIT statement.
func (c *Cli) handleExit() int {
	c.SessionHandler.Close()
	return exitCodeSuccess
}

func (c *Cli) ExitOnError(err error) int {
	c.SessionHandler.Close()
	printError(c.GetErrStream(), err)
	return exitCodeError
}

func (c *Cli) PrintInteractiveError(err error) {
	printError(c.GetWriter(), err)
}

func printError(w io.Writer, err error) {
	code := spanner.ErrCode(err)
	before, _, found := strings.Cut(err.Error(), "spanner:")
	if code == codes.Unknown || !found {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}

	desc := spanner.ErrDesc(err)

	unescaped := strings.NewReplacer(`\"`, `"`,
		`\'`, `'`,
		`\\`, `\`,
		`\n`, "\n").Replace(desc)

	fmt.Fprintf(w, "ERROR: %vspanner: code=%q, desc: %v\n", before, code, unescaped)
}

func (c *Cli) PrintBatchError(err error) {
	printError(c.GetErrStream(), err)
}

func (c *Cli) PrintResult(screenWidth int, result *Result, interactive bool, input string, w io.Writer) error {
	// If no writer is provided, use the CLI's OutStream
	if w == nil {
		w = c.GetWriter()
	}

	ostream := w
	var cmd *exec.Cmd
	if c.SystemVariables.UsePager {
		pagerpath := cmp.Or(os.Getenv("PAGER"), "less")

		split, err := shellquote.Split(pagerpath)
		if err != nil {
			return fmt.Errorf("failed to parse pager command: %w", err)
		}
		cmd = exec.CommandContext(context.Background(), split[0], split[1:]...)

		pr, pw := io.Pipe()
		ostream = pw
		cmd.Stdin = pr
		cmd.Stdout = w

		err = cmd.Start()
		if err != nil {
			slog.Error("failed to start pager command", "err", err)
			return fmt.Errorf("failed to start pager: %w", err)
		}
		defer func() {
			err := pw.Close()
			if err != nil {
				slog.Error("failed to close pipe", "err", err)
			}
			err = cmd.Wait()
			if err != nil {
				slog.Error("failed to wait for pager command", "err", err)
			}
		}()
	}
	return printResult(c.SystemVariables, screenWidth, ostream, result, interactive, input)
}

func (c *Cli) PrintProgressingMark(w io.Writer) func() {
	// Progress marks use terminal control characters, so they should always
	// go to TtyOutStream to avoid polluting tee output. If no TTY is available,
	// disable progress marks.
	ttyStream := c.GetTtyStream()
	if ttyStream == nil {
		return func() {}
	}
	ttyWriter := ttyStream

	progressMarks := []string{`-`, `\`, `|`, `/`}
	ticker := time.NewTicker(time.Millisecond * 100)
	done := make(chan struct{})

	go func() {
		// wait to avoid corruption with first output of command
		select {
		case <-ticker.C:
		case <-done:
			return
		}

		i := 0
		for {
			select {
			case <-ticker.C:
				mark := progressMarks[i%len(progressMarks)]
				fmt.Fprintf(ttyWriter, "\r%s", mark)
				i++
			case <-done:
				return
			}
		}
	}()

	stop := func() {
		close(done)
		ticker.Stop()
		fmt.Fprintf(ttyWriter, "\r \r") // ensure to clear progressing mark
	}
	return stop
}

var (
	promptRe               = regexp.MustCompile(`(%[^{])|%\{[^}]+}`)
	promptSystemVariableRe = regexp.MustCompile(`%\{([^}]+)}`)
)

// getInterpolatedPrompt returns the prompt string with the values of system variables interpolated.
func (c *Cli) getInterpolatedPrompt(prompt string) string {
	sysVars := c.SessionHandler.GetSession().systemVariables
	return promptRe.ReplaceAllStringFunc(prompt, func(s string) string {
		switch s {
		case "%%":
			return "%"
		case "%n":
			return "\n"
		case "%p":
			return sysVars.Project
		case "%i":
			return sysVars.Instance
		case "%d":
			if c.SessionHandler.IsDetached() {
				return "*detached*"
			}
			return sysVars.Database
		case "%t":
			switch {
			case c.SessionHandler.InReadWriteTransaction():
				return "(rw txn)"
			case c.SessionHandler.InReadOnlyTransaction():
				return "(ro txn)"
			case c.SessionHandler.InPendingTransaction():
				return "(txn)"
			default:
				return ""
			}
		case "%R":
			return runewidth.FillLeft(
				lo.CoalesceOrEmpty(strings.ReplaceAll(c.waitingStatus, "*/", "/*"), "-"), 3)
		default:
			// Check if it's a system variable pattern %{...}
			matches := promptSystemVariableRe.FindStringSubmatch(s)
			if len(matches) > 1 {
				varName := matches[1]
				value, err := sysVars.Get(varName)
				if err != nil {
					// Return error pattern to be interpolated.
					return fmt.Sprintf("INVALID_VAR{%v}", varName)
				}
				return value[varName]
			}
			// For unrecognized percent sequences, return them unchanged
			return s
		}
	})
}

func confirm(in io.Reader, out io.Writer, msg string) bool {
	fmt.Fprintf(out, "%s [yes/no] ", msg)

	s := bufio.NewScanner(in)
	for {
		s.Scan()
		switch strings.ToLower(s.Text()) {
		case "yes":
			return true
		case "no":
			return false
		default:
			fmt.Fprint(out, "Please answer yes or no: ")
		}
	}
}

func (c *Cli) executeStatement(ctx context.Context, stmt Statement, interactive bool, input string, w io.Writer) (string, error) {
	// If no writer is provided, use the CLI's OutStream
	if w == nil {
		w = c.GetWriter()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // Ensure context is cancelled when function returns

	// Start interrupt handler in interactive mode only
	if interactive {
		go handleInterrupt(ctx, cancel)
	}

	// Setup progress mark and timing
	t0 := time.Now()
	// Only call setupProgressMark in interactive mode
	var stop func()
	if interactive {
		stop = c.setupProgressMark(stmt, w)
	} else {
		stop = func() {}
	}

	// Execute the statement
	result, err := c.SessionHandler.ExecuteStatement(ctx, stmt)

	// Stop progress mark
	stop()
	elapsed := time.Since(t0).Seconds()

	if err != nil {
		if spanner.ErrCode(err) == codes.Aborted {
			// Once the transaction is aborted, the underlying session gains higher lock priority for the next transaction.
			// This makes the result of subsequent transaction in spanner-cli inconsistent, so we recreate the client to replace
			// the Cloud Spanner's session with new one to revert the lock priority of the session.
			innerErr := c.SessionHandler.RecreateClient()
			if innerErr != nil {
				err = errors.Join(err, innerErr)
			}
		}
		return "", err
	} else {
		// Handle special output messages for session-changing statements
		switch stmt.(type) {
		case *UseStatement, *UseDatabaseMetaCommand:
			fmt.Fprintf(w, "Database changed")
		case *DetachStatement:
			fmt.Fprintf(w, "Detached from database")
		}
	}

	// Update result stats and system variables
	c.updateResultStats(result, elapsed)

	// Display the result (skip for meta commands)
	if _, isMetaCommand := stmt.(MetaCommandStatement); !isMetaCommand {
		if err := c.displayResult(result, interactive, input, w); err != nil {
			return "", fmt.Errorf("failed to display result: %w", err)
		}
	}

	return result.PreInput, nil
}

// setupProgressMark sets up the progress mark display for the statement execution.
// Returns a function to stop the progress mark.
// Statements that have their own progress displays (like DDL operations or SHOW OPERATION SYNC)
// are excluded to avoid conflicting progress indicators.
func (c *Cli) setupProgressMark(stmt Statement, w io.Writer) func() {
	switch stmt.(type) {
	case *DdlStatement, *SyncProtoStatement, *BulkDdlStatement, *RunBatchStatement, *ExitStatement, *ShowOperationStatement:
		return func() {}
	default:
		return c.PrintProgressingMark(w)
	}
}

// updateResultStats updates the result stats and system variables.
func (c *Cli) updateResultStats(result *Result, elapsed float64) {
	// only SELECT statement has the elapsed time measured by the server
	if result.Stats.ElapsedTime == "" {
		result.Stats.ElapsedTime = fmt.Sprintf("%0.2f sec", elapsed)
	}

	if !result.KeepVariables {
		c.updateSystemVariables(result)
	}
}

// GetTerminalSize returns the width of the terminal for the given io.Writer.
// It attempts to type assert the writer to *os.File to get the file descriptor.
// Returns an error if the terminal size cannot be determined.
func GetTerminalSize(w io.Writer) (int, error) {
	// Try to type assert to *os.File
	f, ok := w.(*os.File)
	if !ok {
		return 0, fmt.Errorf("writer is not a file")
	}

	// Get terminal size using the file descriptor
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, err
	}

	return width, nil
}

// GetTerminalSizeWithTty returns the width of the terminal.
// It uses the TtyOutStream from StreamManager if available, otherwise falls back to
// attempting to type assert the writer to *os.File.
// Returns an error if the terminal size cannot be determined.
func (c *Cli) GetTerminalSizeWithTty(w io.Writer) (int, error) {
	var f *os.File

	// Prefer TtyOutStream if available
	ttyStream := c.GetTtyStream()
	if ttyStream != nil {
		f = ttyStream
	} else {
		// Fallback to type assertion
		var ok bool
		f, ok = w.(*os.File)
		if !ok {
			return 0, fmt.Errorf("writer is not a file and TtyOutStream is not set")
		}
	}

	// Get terminal size using the file descriptor
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, err
	}

	return width, nil
}

// displayResult displays the result of the statement execution.
// It returns an error if the output operation fails.
func (c *Cli) displayResult(result *Result, interactive bool, input string, w io.Writer) error {
	// If no writer is provided, use the CLI's OutStream
	if w == nil {
		w = c.GetWriter()
	}

	size := math.MaxInt
	if c.SystemVariables.AutoWrap {
		if c.SystemVariables.FixedWidth != nil {
			// Use fixed width if set
			size = int(*c.SystemVariables.FixedWidth)
		} else {
			// Otherwise get terminal width
			width, err := c.GetTerminalSizeWithTty(w)
			if err != nil {
				// Add warning log when terminal size cannot be obtained
				// and CLI_AUTOWRAP = TRUE
				slog.Warn("failed to get terminal size", "err", err)
				size = math.MaxInt
			} else {
				size = width
			}
		}
	}

	if err := c.PrintResult(size, result, interactive, input, w); err != nil {
		return err
	}

	if interactive {
		if _, err := fmt.Fprintf(w, "\n"); err != nil {
			return err
		}
	}

	return nil
}

func handleInterrupt(ctx context.Context, cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer signal.Stop(c)

	select {
	case <-c:
		cancel()
	case <-ctx.Done():
		// Context cancelled, exit gracefully
		return
	}
}
