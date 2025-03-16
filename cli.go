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
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"time"

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
	Session         *Session
	Credential      []byte
	InStream        io.ReadCloser
	OutStream       io.Writer
	ErrStream       io.Writer
	SystemVariables *systemVariables
	waitingStatus   string
}

func NewCli(ctx context.Context, credential []byte, inStream io.ReadCloser, outStream io.Writer, errStream io.Writer, sysVars *systemVariables) (*Cli, error) {
	session, err := createSession(ctx, credential, sysVars)
	if err != nil {
		return nil, err
	}

	return &Cli{
		Session:         session,
		Credential:      credential,
		InStream:        inStream,
		OutStream:       outStream,
		ErrStream:       errStream,
		SystemVariables: sysVars,
	}, nil
}

func (c *Cli) RunInteractive(ctx context.Context) int {
	exists, err := c.Session.DatabaseExists()
	if err != nil {
		return c.ExitOnError(err)
	}

	if exists {
		fmt.Fprintf(c.OutStream, "Connected.\n")
	} else {
		return c.ExitOnError(fmt.Errorf("unknown database %q", c.SystemVariables.Database))
	}

	ed, history, err := initializeMultilineEditor(c)
	if err != nil {
		return c.ExitOnError(err)
	}

	// ensure reset
	c.waitingStatus = ""

	for {
		// Reset everytime to reflect system variable
		setLineEditor(ed, c.SystemVariables.EnableHighlight)

		input, err := readInteractiveInput(ctx, ed)

		// reset for next input before continue
		ed.SetDefault(nil)

		switch {
		case errors.Is(err, io.EOF):
			fmt.Fprintln(c.OutStream, "Bye")
			return c.handleExit()
		case isInterrupted(err), err != nil:
			c.PrintInteractiveError(err)
			continue
		}

		stmt, err := BuildStatementWithCommentsWithMode(input.statementWithoutComments, input.statement, c.SystemVariables.BuildStatementMode)
		if err != nil {
			c.PrintInteractiveError(err)
			continue
		}

		history.Add(input.statement + ";")

		// Some statements are needed to be handled.

		if _, ok := stmt.(*ExitStatement); ok {
			fmt.Fprintln(c.OutStream, "Bye")
			return c.handleExit()
		}

		// DropDatabaseStatement requires confirmation in interactive mode.
		if s, ok := stmt.(*DropDatabaseStatement); ok {
			if c.SystemVariables.Database == s.DatabaseId {
				c.PrintInteractiveError(
					fmt.Errorf("database %q is currently used, it can not be dropped", s.DatabaseId),
				)
				continue
			}

			if !confirm(c.OutStream, fmt.Sprintf("Database %q will be dropped.\nDo you want to continue?", s.DatabaseId)) {
				continue
			}
		}

		preInput, err := c.executeStatement(ctx, stmt, true, input.statement)
		if err != nil {
			c.PrintInteractiveError(err)
			continue
		}

		ed.SetDefault(strings.Split(preInput, "\n"))
	}
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

func (c *Cli) RunBatch(ctx context.Context, input string) int {
	stmts, err := buildCommands(input, c.SystemVariables.BuildStatementMode)
	if err != nil {
		c.PrintBatchError(err)
		return exitCodeError
	}

	ctx, cancel := context.WithCancel(ctx)
	go handleInterrupt(cancel)

	for _, stmt := range stmts {
		if _, ok := stmt.(*ExitStatement); ok {
			return c.handleExit()
		}

		_, err = c.executeStatement(ctx, stmt, false, input)
		if err != nil {
			c.PrintBatchError(err)
			return exitCodeError
		}
	}

	return exitCodeSuccess
}

// handleExit processes EXIT statement.
func (c *Cli) handleExit() int {
	c.Session.Close()
	return exitCodeSuccess
}

func (c *Cli) ExitOnError(err error) int {
	c.Session.Close()
	printError(c.ErrStream, err)
	return exitCodeError
}

func (c *Cli) PrintInteractiveError(err error) {
	printError(c.OutStream, err)
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
	printError(c.ErrStream, err)
}

func (c *Cli) PrintResult(screenWidth int, result *Result, interactive bool, input string) {
	ostream := c.OutStream
	var cmd *exec.Cmd
	if c.SystemVariables.UsePager {
		pagerpath := cmp.Or(os.Getenv("PAGER"), "less")

		split, err := shellquote.Split(pagerpath)
		if err != nil {
			return
		}
		cmd = exec.CommandContext(context.Background(), split[0], split[1:]...)

		pr, pw := io.Pipe()
		ostream = pw
		cmd.Stdin = pr
		cmd.Stdout = c.OutStream

		err = cmd.Start()
		if err != nil {
			log.Println(err)
			return
		}
		defer func() {
			err := pw.Close()
			if err != nil {
				log.Println(err)
			}
			err = cmd.Wait()
			if err != nil {
				log.Println(err)
			}
		}()
	}
	printResult(c.SystemVariables, screenWidth, ostream, result, interactive, input)
}

func (c *Cli) PrintProgressingMark() func() {
	progressMarks := []string{`-`, `\`, `|`, `/`}
	ticker := time.NewTicker(time.Millisecond * 100)
	go func() {
		// wait to avoid corruption with first output of command
		<-ticker.C

		i := 0
		for {
			<-ticker.C
			mark := progressMarks[i%len(progressMarks)]
			fmt.Fprintf(c.OutStream, "\r%s", mark)
			i++
		}
	}()

	stop := func() {
		ticker.Stop()
		fmt.Fprintf(c.OutStream, "\r") // clear progressing mark
	}
	return stop
}

var promptRe = regexp.MustCompile(`(%[^{])|%\{[^}]+}`)
var promptSystemVariableRe = regexp.MustCompile(`%\{([^}]+)}`)

// getInterpolatedPrompt returns the prompt string with the values of system variables interpolated.
func (c *Cli) getInterpolatedPrompt(prompt string) string {
	sysVars := c.Session.systemVariables
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
			return sysVars.Database
		case "%t":
			switch {
			case c.Session.InReadWriteTransaction():
				return "(rw txn)"
			case c.Session.InReadOnlyTransaction():
				return "(ro txn)"
			case c.Session.InPendingTransaction():
				return "(txn)"
			default:
				return ""
			}
		case "%R":
			return runewidth.FillLeft(
				lo.CoalesceOrEmpty(strings.ReplaceAll(c.waitingStatus, "*/", "/*"), "-"), 3)
		default:
			varName := promptSystemVariableRe.FindStringSubmatch(s)[1]
			value, err := sysVars.Get(varName)
			if err != nil {
				// Return error pattern to be interpolated.
				return fmt.Sprintf("INVALID_VAR{%v}", varName)
			}
			return value[varName]
		}
	})
}

func confirm(out io.Writer, msg string) bool {
	fmt.Fprintf(out, "%s [yes/no] ", msg)

	s := bufio.NewScanner(os.Stdin)
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

func (c *Cli) executeStatement(ctx context.Context, stmt Statement, interactive bool, input string) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	go handleInterrupt(cancel)

	if s, ok := stmt.(*UseStatement); ok {
		err := c.handleUse(ctx, s, interactive)
		if err != nil {
			return "", err
		}

		fmt.Fprintf(c.OutStream, "Database changed")
	}

	t0 := time.Now()
	stop := func() {}
	switch stmt.(type) {
	case *DdlStatement, *SyncProtoStatement, *BulkDdlStatement, *RunBatchStatement, *ExitStatement:
		break
	default:
		stop = c.PrintProgressingMark()
	}

	result, err := c.Session.ExecuteStatement(ctx, stmt)

	stop()
	elapsed := time.Since(t0).Seconds()

	if err != nil {
		if spanner.ErrCode(err) == codes.Aborted {
			// Once the transaction is aborted, the underlying session gains higher lock priority for the next transaction.
			// This makes the result of subsequent transaction in spanner-cli inconsistent, so we recreate the client to replace
			// the Cloud Spanner's session with new one to revert the lock priority of the session.
			innerErr := c.Session.RecreateClient()
			if innerErr != nil {
				err = errors.Join(err, innerErr)
			}
		}
		return "", err
	}

	// only SELECT statement has the elapsed time measured by the server
	if result.Stats.ElapsedTime == "" {
		result.Stats.ElapsedTime = fmt.Sprintf("%0.2f sec", elapsed)
	}

	if !result.KeepVariables {
		c.updateSystemVariables(result)
	}

	size := math.MaxInt
	if c.SystemVariables.AutoWrap {
		sz, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			size = math.MaxInt
		} else {
			size = sz
		}
	}

	c.PrintResult(size, result, interactive, input)

	if interactive {
		fmt.Fprintf(c.OutStream, "\n")
	}

	return result.PreInput, nil
}

func (c *Cli) handleUse(ctx context.Context, s *UseStatement, interactive bool) error {
	newSystemVariables := *c.SystemVariables

	newSystemVariables.Database = s.Database
	newSystemVariables.Role = s.Role

	newSession, err := createSession(ctx, c.Credential, &newSystemVariables)
	if err != nil {
		return err
	}

	exists, err := newSession.DatabaseExists()
	if err != nil {
		newSession.Close()
		return err
	}

	if !exists {
		newSession.Close()
		return fmt.Errorf("unknown database %q", s.Database)
	}

	c.Session.Close()
	c.Session = newSession

	c.SystemVariables = &newSystemVariables

	return nil
}

func handleInterrupt(cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	cancel()
}
