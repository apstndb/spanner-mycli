// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

func newConnectedTestCli(t *testing.T, out io.Writer) *Cli {
	t.Helper()
	if out == nil {
		out = io.Discard
	}
	session := newDetachedTestSession(out)
	t.Cleanup(session.Close)
	session.mode = DatabaseConnected
	session.systemVariables.Query.BuildStatementMode = enums.ParseModeFallback
	session.systemVariables.Display.CLIFormat = enums.DisplayModeTab
	return &Cli{
		SessionHandler:  NewSessionHandler(session),
		SystemVariables: session.systemVariables,
	}
}

func TestCli_streamAccessors(t *testing.T) {
	t.Parallel()
	in := io.NopCloser(strings.NewReader("in"))
	var out, errOut bytes.Buffer
	sysVars := &systemVariables{
		StreamManager: streamio.NewStreamManager(in, &out, &errOut),
	}
	cli := &Cli{SystemVariables: sysVars}

	if cli.GetWriter() != sysVars.StreamManager.GetWriter() {
		t.Fatal("GetWriter should return StreamManager writer")
	}
	if cli.GetErrStream() != sysVars.StreamManager.GetErrStream() {
		t.Fatal("GetErrStream should return StreamManager error stream")
	}
	if cli.GetInStream() != sysVars.StreamManager.GetInStream() {
		t.Fatal("GetInStream should return StreamManager input")
	}
	if cli.GetTtyStream() != nil {
		t.Fatal("GetTtyStream should be nil until a TTY is attached")
	}
	tty := attachTTYPipe(t, cli)
	if cli.GetTtyStream() == nil {
		t.Fatal("GetTtyStream should return the attached TTY")
	}
	_ = tty
}

func TestCli_parseStatement_metaCommands(t *testing.T) {
	t.Parallel()
	cli := &Cli{SystemVariables: &systemVariables{Query: QueryVars{BuildStatementMode: enums.ParseModeFallback}}}

	got, err := cli.parseStatement(&inputStatement{statement: `\! echo hello`})
	if err != nil {
		t.Fatalf("parseStatement: %v", err)
	}
	shell, ok := got.(*ShellMetaCommand)
	if !ok || shell.Command != "echo hello" {
		t.Fatalf("got %#v, want ShellMetaCommand echo hello", got)
	}

	got, err = cli.parseStatement(&inputStatement{statement: `\. setup.sql`})
	if err != nil {
		t.Fatalf("parseStatement: %v", err)
	}
	src, ok := got.(*SourceMetaCommand)
	if !ok || src.FilePath != "setup.sql" {
		t.Fatalf("got %#v, want SourceMetaCommand setup.sql", got)
	}

	_, err = cli.parseStatement(&inputStatement{statement: `\d unknown`})
	if err == nil || !strings.Contains(err.Error(), "unsupported meta command") {
		t.Fatalf("error = %v, want unsupported meta command", err)
	}
}

func TestCli_handleSpecialStatements_sourceFile(t *testing.T) {
	t.Parallel()

	t.Run("missing file prints interactive error and continues", func(t *testing.T) {
		var out, errOut bytes.Buffer
		session := newDetachedTestSession(&out)
		t.Cleanup(session.Close)
		session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), &out, &errOut)
		cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

		exitCode, processed := cli.handleSpecialStatements(context.Background(), &SourceMetaCommand{FilePath: filepath.Join(t.TempDir(), "missing.sql")})
		if exitCode != -1 || !processed {
			t.Fatalf("exitCode=%d processed=%v, want -1, true", exitCode, processed)
		}
		if !strings.Contains(errOut.String(), "ERROR:") || !strings.Contains(errOut.String(), "no such file") {
			t.Fatalf("stderr = %q, want sourced-file error", errOut.String())
		}
	})

	t.Run("valid file executes and continues", func(t *testing.T) {
		var out bytes.Buffer
		cli := newDetachedEchoCli(t, &out)
		path := writeTempSQL(t, "SET CLI_PROMPT = 'from-source';")
		exitCode, processed := cli.handleSpecialStatements(context.Background(), &SourceMetaCommand{FilePath: path})
		if exitCode != -1 || !processed {
			t.Fatalf("exitCode=%d processed=%v, want -1, true", exitCode, processed)
		}
		if cli.SystemVariables.Display.Prompt != "from-source" {
			t.Fatalf("CLI_PROMPT = %q, want from-source", cli.SystemVariables.Display.Prompt)
		}
	})
}

func TestCli_handleSpecialStatements_dropDatabaseConfirm(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantProcessed bool
	}{
		{name: "user declines", input: "no\n", wantProcessed: true},
		{name: "user confirms", input: "yes\n", wantProcessed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			sysVars := &systemVariables{Connection: ConnectionVars{Database: "current-db"}}
			sysVars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader(tt.input)), &out, &errOut)
			cli := &Cli{
				SessionHandler:  NewSessionHandler(&Session{systemVariables: sysVars}),
				SystemVariables: sysVars,
			}
			attachTTYPipe(t, cli)

			exitCode, processed := cli.handleSpecialStatements(context.Background(), &DropDatabaseStatement{DatabaseId: "other-db"})
			if exitCode != -1 {
				t.Fatalf("exitCode = %d, want -1", exitCode)
			}
			if processed != tt.wantProcessed {
				t.Fatalf("processed = %v, want %v", processed, tt.wantProcessed)
			}
			if errOut.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestCli_PrintInteractiveError(t *testing.T) {
	t.Parallel()
	var errOut bytes.Buffer
	cli := &Cli{
		SystemVariables: &systemVariables{
			StreamManager: streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), io.Discard, &errOut),
		},
	}
	cli.PrintInteractiveError(errors.New("interactive boom"))
	if errOut.String() != "ERROR: interactive boom\n" {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCli_PrintResult_nilWriterUsesStream(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cli := &Cli{
		SystemVariables: &systemVariables{
			Display:       DisplayVars{CLIFormat: enums.DisplayModeTab},
			StreamManager: streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), &out, io.Discard),
		},
	}
	err := cli.PrintResult(80, &Result{TableHeader: toTableHeader("col"), Body: PresentationBody([]Row{toRow("v")})}, false, "", nil)
	if err != nil {
		t.Fatalf("PrintResult: %v", err)
	}
	if !strings.Contains(out.String(), "col") || !strings.Contains(out.String(), "v") {
		t.Fatalf("stream output = %q", out.String())
	}
}

func TestCli_displayResult_nilWriterAndInteractiveNewline(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cli := &Cli{
		SystemVariables: &systemVariables{
			Display:       DisplayVars{CLIFormat: enums.DisplayModeTab},
			StreamManager: streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), &out, io.Discard),
		},
	}
	result := &Result{TableHeader: toTableHeader("col"), Body: PresentationBody([]Row{toRow("v")})}
	sink := cli.newResultSink(context.Background(), &out, "")
	if err := cli.displayResult(sink, result, true, nil); err != nil {
		t.Fatalf("displayResult: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "col") || !strings.Contains(got, "v") {
		t.Fatalf("output missing table content: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("interactive display should end with a newline, got %q", got)
	}
}

func TestCli_readInputLine_readError(t *testing.T) {
	cli := newReadlineTestCli(t)
	ed := newIsolatedReadlineEditor(t, nil, nil)

	stmt, err := cli.readInputLine(context.Background(), ed)
	if stmt != nil {
		t.Fatalf("statement = %+v, want nil", stmt)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("error = %v, want failed to read input wrapping io.EOF", err)
	}
	if !strings.Contains(err.Error(), "failed to read input") {
		t.Fatalf("error = %v, want failed to read input wrapping io.EOF", err)
	}
}

func TestCli_executeStatementInteractive_error(t *testing.T) {
	t.Parallel()
	cli := newDetachedEchoCli(t, io.Discard)
	_, err := cli.executeStatementInteractive(context.Background(), &SelectStatement{Query: "SELECT 1"}, &inputStatement{statement: "SELECT 1"})
	if err == nil || !strings.Contains(err.Error(), "not compatible with detached session mode") {
		t.Fatalf("error = %v, want detached-mode rejection", err)
	}
}

func TestCli_updateSystemVariables(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	cli := &Cli{SystemVariables: &systemVariables{}}

	cli.updateSystemVariables(&Result{
		ReadTimestamp:   ts,
		CommitTimestamp: ts,
		CommitStats:     &sppb.CommitResponse_CommitStats{MutationCount: 4},
	})
	if !cli.SystemVariables.LastResult.ReadTimestamp.Equal(ts) || !cli.SystemVariables.LastResult.CommitTimestamp.Equal(ts) {
		t.Fatalf("timestamps not stored: %+v", cli.SystemVariables.LastResult)
	}
	if cli.SystemVariables.LastResult.CommitResponse == nil || cli.SystemVariables.LastResult.CommitResponse.CommitStats.MutationCount != 4 {
		t.Fatalf("commit response = %#v", cli.SystemVariables.LastResult.CommitResponse)
	}
	if !cli.SystemVariables.LastResult.CommitResponse.CommitTimestamp.AsTime().Equal(ts) {
		t.Fatalf("commit timestamp proto = %v", cli.SystemVariables.LastResult.CommitResponse.CommitTimestamp)
	}

	cli.updateSystemVariables(&Result{ReadTimestamp: ts.Add(time.Second)})
	if cli.SystemVariables.LastResult.CommitResponse != nil {
		t.Fatal("CommitResponse should be cleared when CommitStats is nil")
	}

	cli.updateSystemVariables(&Result{CommitStats: &sppb.CommitResponse_CommitStats{MutationCount: 1}})
	if cli.SystemVariables.LastResult.CommitResponse == nil {
		t.Fatal("CommitResponse should be set when CommitStats is present")
	}
	if cli.SystemVariables.LastResult.CommitResponse.CommitTimestamp != nil {
		t.Fatal("zero commit timestamp should not be copied into the proto")
	}
}

func TestGetTerminalSize_nonFile(t *testing.T) {
	t.Parallel()
	_, err := GetTerminalSize(&bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "writer is not a file") {
		t.Fatalf("error = %v, want writer is not a file", err)
	}
}

func TestCli_GetTerminalSizeWithTty_andResolveScreenWidth(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cli := &Cli{
		SystemVariables: &systemVariables{
			Display:       DisplayVars{AutoWrap: true},
			StreamManager: streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), &out, io.Discard),
		},
	}

	_, err := cli.GetTerminalSizeWithTty(&out)
	if err == nil || !strings.Contains(err.Error(), "writer is not a file and TtyOutStream is not set") {
		t.Fatalf("error = %v, want missing TTY and non-file writer", err)
	}

	if width := cli.resolveScreenWidth(&out); width != math.MaxInt {
		t.Fatalf("AutoWrap with unknown size = %d, want MaxInt", width)
	}

	cli.SystemVariables.Display.AutoWrap = false
	if width := cli.resolveScreenWidth(&out); width != math.MaxInt {
		t.Fatalf("AutoWrap=false width = %d, want MaxInt", width)
	}

	fixed := int64(72)
	cli.SystemVariables.Display.AutoWrap = true
	cli.SystemVariables.Display.FixedWidth = &fixed
	if width := cli.resolveScreenWidth(&out); width != 72 {
		t.Fatalf("fixed width = %d, want 72", width)
	}

	f, err := os.CreateTemp(t.TempDir(), "notty")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	cli.SystemVariables.Display.FixedWidth = nil
	if width := cli.resolveScreenWidth(f); width != math.MaxInt {
		t.Fatalf("non-terminal file width = %d, want MaxInt", width)
	}
}

type abortStatement struct{}

func (abortStatement) isDetachedCompatible() {}

func (abortStatement) Execute(context.Context, *Session, OperationOutput) (*Result, error) {
	return nil, spanner.ToSpannerError(status.Error(codes.Aborted, "txn aborted"))
}

func TestCli_executeStatement_abortedJoinsRecreateError(t *testing.T) {
	t.Parallel()
	cli := newConnectedTestCli(t, io.Discard)
	_, err := cli.executeStatement(context.Background(), abortStatement{}, false, "", io.Discard)
	if err == nil {
		t.Fatal("expected aborted execution error")
	}
	if spanner.ErrCode(err) != codes.Aborted {
		t.Fatalf("ErrCode = %v, want Aborted", spanner.ErrCode(err))
	}
	if !strings.Contains(err.Error(), "database operation requires a database connection") {
		t.Fatalf("error = %v, want joined RecreateClient failure", err)
	}
}

func TestCli_executeStatement_metaCommandSkipsDecoratedDisplay(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cli := newConnectedTestCli(t, &out)
	cli.SystemVariables.Display.MarkdownCodeblock = true
	cli.SystemVariables.Feature.EchoInput = true

	pre, err := cli.executeStatement(context.Background(), &PromptMetaCommand{PromptString: "meta>"}, true, `\R meta>`, &out)
	if err != nil {
		t.Fatalf("executeStatement: %v", err)
	}
	if pre != "" {
		t.Fatalf("PreInput = %q, want empty", pre)
	}
	if out.Len() != 0 {
		t.Fatalf("meta command wrote decorated output %q", out.String())
	}
	if cli.SystemVariables.Display.Prompt != "meta> " {
		t.Fatalf("prompt = %q, want %q", cli.SystemVariables.Display.Prompt, "meta> ")
	}
}

func TestCli_executeStatementInteractive_andNilWriter(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cli := newConnectedTestCli(t, &out)
	pre, err := cli.executeStatementInteractive(context.Background(), &SetStatement{VarName: "CLI_PROMPT", Value: "'interactive'"}, &inputStatement{statement: "SET CLI_PROMPT = 'interactive'"})
	if err != nil {
		t.Fatalf("executeStatementInteractive: %v", err)
	}
	if pre != "" {
		t.Fatalf("PreInput = %q", pre)
	}
	if cli.SystemVariables.Display.Prompt != "interactive" {
		t.Fatalf("prompt = %q, want interactive", cli.SystemVariables.Display.Prompt)
	}
	if out.Len() == 0 {
		t.Fatal("interactive execution produced no output")
	}

	_, err = cli.executeStatement(context.Background(), &SetStatement{VarName: "CLI_PROMPT2", Value: "'from-nil'"}, false, "", nil)
	if err != nil {
		t.Fatalf("executeStatement nil writer: %v", err)
	}
	if cli.SystemVariables.Display.Prompt2 != "from-nil" {
		t.Fatalf("prompt2 = %q, want from-nil", cli.SystemVariables.Display.Prompt2)
	}
}

func TestCli_executeStatement_updatesResultTimestamps(t *testing.T) {
	t.Parallel()
	cli := newConnectedTestCli(t, io.Discard)
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err := cli.executeStatement(context.Background(), timestampResultStatement{ts: ts}, false, "", io.Discard)
	if err != nil {
		t.Fatalf("executeStatement: %v", err)
	}
	if !cli.SystemVariables.LastResult.ReadTimestamp.Equal(ts) {
		t.Fatalf("ReadTimestamp = %v, want %v", cli.SystemVariables.LastResult.ReadTimestamp, ts)
	}
	if cli.SystemVariables.LastResult.CommitResponse == nil || cli.SystemVariables.LastResult.CommitResponse.CommitStats.MutationCount != 2 {
		t.Fatalf("CommitResponse = %#v", cli.SystemVariables.LastResult.CommitResponse)
	}
}

type timestampResultStatement struct{ ts time.Time }

func (timestampResultStatement) isDetachedCompatible() {}

func (s timestampResultStatement) Execute(context.Context, *Session, OperationOutput) (*Result, error) {
	return &Result{
		ReadTimestamp:   s.ts,
		CommitTimestamp: s.ts,
		CommitStats:     &sppb.CommitResponse_CommitStats{MutationCount: 2},
	}, nil
}

func TestCli_executeSourceFile_executionError(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cli := newDetachedEchoCli(t, &out)
	err := cli.executeSourceFile(context.Background(), writeTempSQL(t, "SELECT 1;"))
	if err == nil || !strings.Contains(err.Error(), "error executing statement 1") {
		t.Fatalf("error = %v, want statement execution failure", err)
	}
}

func TestCli_executeStartupSQL_executionError(t *testing.T) {
	t.Parallel()
	var errOut bytes.Buffer
	cli := newDetachedEchoCli(t, io.Discard)
	cli.SystemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), io.Discard, &errOut)
	err := cli.executeStartupSQL(context.Background(), []string{"SET CLI_FORMAT = 'NOT_A_FORMAT'"})
	if GetExitCode(err) != exitCodeError {
		t.Fatalf("error = %v, want execution exit", err)
	}
	if !strings.Contains(errOut.String(), "ERROR:") {
		t.Fatalf("stderr = %q, want printed batch error", errOut.String())
	}
}

func TestCli_RunBatch_parseAndExecutionErrors(t *testing.T) {
	t.Parallel()

	t.Run("parse error", func(t *testing.T) {
		var errOut bytes.Buffer
		cli := newDetachedEchoCli(t, io.Discard)
		cli.SystemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), io.Discard, &errOut)
		err := cli.RunBatch(context.Background(), "INVALID SYNTAX;")
		if GetExitCode(err) != exitCodeError {
			t.Fatalf("error = %v, want parse exit", err)
		}
		if !strings.Contains(errOut.String(), "ERROR:") {
			t.Fatalf("stderr = %q", errOut.String())
		}
	})

	t.Run("execution error", func(t *testing.T) {
		var errOut bytes.Buffer
		cli := newDetachedEchoCli(t, io.Discard)
		cli.SystemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), io.Discard, &errOut)
		err := cli.RunBatch(context.Background(), "SELECT 1;")
		if GetExitCode(err) != exitCodeError {
			t.Fatalf("error = %v, want execution exit", err)
		}
		if !strings.Contains(errOut.String(), "ERROR:") {
			t.Fatalf("stderr = %q", errOut.String())
		}
	})
}

func TestHandleInterrupt_contextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		handleInterrupt(ctx, func() {})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleInterrupt did not return after context cancellation")
	}
}

func TestSetupProgressMark_excludedStatements(t *testing.T) {
	cli := newDecorationTestCli()
	tty := attachTTYPipe(t, cli)
	for _, stmt := range []Statement{
		&DdlStatement{Ddl: "CREATE TABLE t (id INT64) PRIMARY KEY (id)"},
		&ExitStatement{},
		&DumpSchemaStatement{},
		&RunBatchStatement{},
	} {
		stop := cli.setupProgressMark(stmt, io.Discard)
		stop()
	}
	if got := readPipeDeadline(t, tty, 50*time.Millisecond); got != "" {
		t.Fatalf("excluded statements should not start progress, TTY got %q", got)
	}
}

func TestCli_executeStatement_keepVariablesSkipsTimestampUpdate(t *testing.T) {
	t.Parallel()
	cli := newConnectedTestCli(t, io.Discard)
	prior := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	cli.SystemVariables.LastResult.ReadTimestamp = prior
	_, err := cli.executeStatement(context.Background(), keepVariablesStatement{}, false, "", io.Discard)
	if err != nil {
		t.Fatalf("executeStatement: %v", err)
	}
	if !cli.SystemVariables.LastResult.ReadTimestamp.Equal(prior) {
		t.Fatalf("KeepVariables should not update timestamps, got %v", cli.SystemVariables.LastResult.ReadTimestamp)
	}
}

type keepVariablesStatement struct{}

func (keepVariablesStatement) isDetachedCompatible() {}

func (keepVariablesStatement) Execute(context.Context, *Session, OperationOutput) (*Result, error) {
	return &Result{
		KeepVariables: true,
		ReadTimestamp: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
	}, nil
}

func TestCli_displayResult_outputError(t *testing.T) {
	t.Parallel()
	cli := &Cli{
		SystemVariables: &systemVariables{
			Display:       DisplayVars{CLIFormat: enums.DisplayModeTab},
			StreamManager: streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), io.Discard, io.Discard),
		},
	}
	cause := errors.New("display dest failed")
	w := &resultFailureWriter{err: cause}
	err := cli.displayResult(cli.newResultSink(context.Background(), w, ""), &Result{TableHeader: toTableHeader("c"), Body: PresentationBody([]Row{toRow("1")})}, false, w)
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want %v", err, cause)
	}
}
