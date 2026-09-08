// Copyright 2026 apstndb
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

package mycli

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"google.golang.org/api/option"
)

func captureLogger(lv slog.Leveler) (*bytes.Buffer, *slog.Logger) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})).With("component", "cli")
	return &buf, logger
}

func restoreProcessLogger(t *testing.T) {
	t.Helper()
	prevLogger := slog.Default()
	prevLevel := cliLogLevel.Level()
	t.Cleanup(func() {
		slog.SetDefault(prevLogger)
		cliLogLevel.Set(prevLevel)
	})
}

func boundLogLevelVars(t *testing.T, initial slog.Level) (*systemVariables, *slog.LevelVar) {
	t.Helper()
	var lv slog.LevelVar
	lv.Set(initial)
	sv := newSystemVariablesWithDefaultsForTest()
	sv.runtimeLogLevel = &lv
	sv.Feature.LogLevel = initial
	sv.ensureRegistry()
	return sv, &lv
}

func mustLogLevel(t *testing.T, sv *systemVariables) string {
	t.Helper()
	got, err := sv.Registry.Get("CLI_LOG_LEVEL")
	if err != nil {
		t.Fatalf("GET CLI_LOG_LEVEL: %v", err)
	}
	return got
}

func debugEnabled(logger *slog.Logger) bool {
	return logger.Enabled(context.Background(), slog.LevelDebug)
}

func emitDebug(logger *slog.Logger, buf *bytes.Buffer, msg string) string {
	buf.Reset()
	logger.Debug(msg)
	return buf.String()
}

// originalFieldOnlyLogLevelSet is the pre-A16 LogLevelVar.Set: it writes the
// session field and never a runtime LevelVar.
func originalFieldOnlyLogLevelSet(ptr *slog.Level, value string) error {
	if strings.EqualFold(value, "WARNING") {
		*ptr = slog.LevelWarn
		return nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return err
	}
	*ptr = level
	return nil
}

func TestLogLevelP1BoundSetterUpdatesEnabledAndRecords(t *testing.T) {
	t.Parallel()
	sv, lv := boundLogLevelVars(t, slog.LevelWarn)
	buf, logger := captureLogger(lv)

	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG"); err != nil {
		t.Fatalf("SET DEBUG: %v", err)
	}
	if got := mustLogLevel(t, sv); got != "DEBUG" {
		t.Fatalf("SHOW after SET DEBUG = %q", got)
	}
	if !debugEnabled(logger) {
		t.Fatal("Enabled(Debug) false after SET DEBUG")
	}
	out := emitDebug(logger, buf, "probe-debug")
	if !strings.Contains(out, "msg=probe-debug") || !strings.Contains(out, "component=cli") {
		t.Fatalf("DEBUG record missing destination/attributes: %q", out)
	}

	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "WARN"); err != nil {
		t.Fatalf("SET WARN: %v", err)
	}
	if got := mustLogLevel(t, sv); got != "WARN" {
		t.Fatalf("SHOW after SET WARN = %q", got)
	}
	if debugEnabled(logger) {
		t.Fatal("Enabled(Debug) true after SET WARN")
	}
	out = emitDebug(logger, buf, "probe-quiet")
	if out != "" {
		t.Fatalf("DEBUG record still emitted after SET WARN: %q", out)
	}
}

func TestLogLevelNilFixtureSetterDoesNotTouchRuntime(t *testing.T) {
	t.Parallel()
	var lv slog.LevelVar
	lv.Set(slog.LevelWarn)
	buf, logger := captureLogger(&lv)

	sv := newSystemVariablesWithDefaultsForTest()
	if sv.runtimeLogLevel != nil {
		t.Fatal("newSystemVariablesWithDefaults bound a runtime LevelVar")
	}
	sv.ensureRegistry()
	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG"); err != nil {
		t.Fatalf("SET DEBUG: %v", err)
	}
	if got := mustLogLevel(t, sv); got != "DEBUG" {
		t.Fatalf("SHOW = %q, want DEBUG", got)
	}
	if debugEnabled(logger) || lv.Level() != slog.LevelWarn {
		t.Fatal("nil-bound fixture mutated a LevelVar")
	}
	if out := emitDebug(logger, buf, "probe-debug"); out != "" {
		t.Fatalf("nil-bound fixture emitted DEBUG: %q", out)
	}
}

func TestLogLevelOriginalFieldOnlySetterFailsP1(t *testing.T) {
	t.Parallel()
	var lv slog.LevelVar
	lv.Set(slog.LevelWarn)
	buf, logger := captureLogger(&lv)
	reported := slog.LevelWarn

	if err := originalFieldOnlyLogLevelSet(&reported, "DEBUG"); err != nil {
		t.Fatal(err)
	}
	if reported != slog.LevelDebug {
		t.Fatalf("reported = %v, want DEBUG", reported)
	}
	if debugEnabled(logger) || lv.Level() != slog.LevelWarn {
		t.Fatal("original field-only setter updated runtime level")
	}
	if out := emitDebug(logger, buf, "probe-debug"); out != "" {
		t.Fatalf("original setter emitted DEBUG: %q", out)
	}
}

func TestLogLevelProcessBindPreservesHandlerIdentity(t *testing.T) {
	restoreProcessLogger(t)
	buf, logger := captureLogger(&cliLogLevel)
	slog.SetDefault(logger)

	sv, err := createSystemVariablesFromOptions(&spannerOptions{LogLevel: "WARN"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.runtimeLogLevel != &cliLogLevel {
		t.Fatal("createSystemVariablesFromOptions did not bind process LevelVar")
	}
	sv.ensureRegistry()

	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG"); err != nil {
		t.Fatal(err)
	}
	if slog.Default() != logger {
		t.Fatal("SET replaced slog.Default")
	}
	if !debugEnabled(slog.Default()) {
		t.Fatal("process Enabled(Debug) false after SET DEBUG")
	}
	out := emitDebug(slog.Default(), buf, "probe-debug")
	if !strings.Contains(out, "msg=probe-debug") || !strings.Contains(out, "component=cli") {
		t.Fatalf("process DEBUG record missing attrs: %q", out)
	}

	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "WARN"); err != nil {
		t.Fatal(err)
	}
	if slog.Default() != logger {
		t.Fatal("SET WARN replaced slog.Default")
	}
	if debugEnabled(slog.Default()) {
		t.Fatal("process Enabled(Debug) true after SET WARN")
	}
}

func TestLogLevelP2ParseAndAtomicInvalid(t *testing.T) {
	restoreProcessLogger(t)

	level, err := SetLogLevel("WARNING")
	if err != nil {
		t.Fatalf("--log-level=WARNING: %v", err)
	}
	if level != slog.LevelWarn || cliLogLevel.Level() != slog.LevelWarn {
		t.Fatalf("WARNING parsed as %v", level)
	}

	sv, err := createSystemVariablesFromOptions(&spannerOptions{LogLevel: "WARN"})
	if err != nil {
		t.Fatal(err)
	}
	sv.ensureRegistry()
	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG"); err != nil {
		t.Fatal(err)
	}

	if err := sv.SetFromGoogleSQL("CLI_LOG_LEVEL", "'WARNING'"); err != nil {
		t.Fatalf("SQL WARNING: %v", err)
	}
	if got := mustLogLevel(t, sv); got != "WARN" {
		t.Fatalf("SHOW after SQL WARNING = %q", got)
	}

	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG-4"); err != nil {
		t.Fatalf("numeric offset: %v", err)
	}
	if got := mustLogLevel(t, sv); got != "DEBUG-4" {
		t.Fatalf("SHOW after DEBUG-4 = %q", got)
	}

	beforeShow := mustLogLevel(t, sv)
	beforeLevel := cliLogLevel.Level()
	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "NOT_A_LEVEL"); err == nil {
		t.Fatal("invalid SET succeeded")
	}
	if mustLogLevel(t, sv) != beforeShow || cliLogLevel.Level() != beforeLevel {
		t.Fatal("invalid SET mutated reported or effective level")
	}

	if _, err := createSystemVariablesFromOptions(&spannerOptions{LogLevel: "INVALID"}); err == nil {
		t.Fatal("invalid --log-level succeeded")
	}
	if cliLogLevel.Level() != beforeLevel {
		t.Fatal("invalid --log-level mutated process level")
	}

	sv2, err := initializeSystemVariables(&spannerOptions{
		LogLevel: "WARN",
		Set:      map[string]string{"CLI_LOG_LEVEL": "DEBUG"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv2.Config.EmbeddedLogLevel != slog.LevelWarn {
		t.Fatalf("embedded snapshot after --set = %v, want WARN", sv2.Config.EmbeddedLogLevel)
	}
	if sv2.Feature.LogLevel != slog.LevelDebug {
		t.Fatalf("Feature.LogLevel after --set = %v, want DEBUG", sv2.Feature.LogLevel)
	}
	if cliLogLevel.Level() != slog.LevelDebug {
		t.Fatalf("process level after --set = %v, want DEBUG", cliLogLevel.Level())
	}
}

func TestLogLevelP3LocalRestoreAndOrdinarySet(t *testing.T) {
	t.Parallel()
	sv, lv := boundLogLevelVars(t, slog.LevelWarn)
	buf, logger := captureLogger(lv)
	session := &Session{
		mode:            DatabaseConnected,
		systemVariables: sv,
		txn:             NewTransactionManager(nil, sv, spanner.ClientConfig{}),
	}
	sv.inTransaction = session.txn.InTransaction
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_LOG_LEVEL", Value: "'DEBUG'"}); err != nil {
		t.Fatalf("SET LOCAL DEBUG: %v", err)
	}
	if mustLogLevel(t, sv) != "DEBUG" || !debugEnabled(logger) {
		t.Fatal("SET LOCAL DEBUG did not update SHOW/Enabled")
	}
	if out := emitDebug(logger, buf, "local-debug"); !strings.Contains(out, "msg=local-debug") {
		t.Fatalf("SET LOCAL DEBUG did not emit: %q", out)
	}

	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if mustLogLevel(t, sv) != "WARN" || debugEnabled(logger) {
		t.Fatal("ROLLBACK did not restore WARN")
	}

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_LOG_LEVEL", Value: "'DEBUG'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_LOG_LEVEL", Value: "'INFO'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if mustLogLevel(t, sv) != "INFO" {
		t.Fatalf("ordinary SET after LOCAL did not keep INFO: %s", mustLogLevel(t, sv))
	}

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_LOG_LEVEL", Value: "'DEBUG'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_LOG_LEVEL", Value: "'NOPE'"}); err == nil {
		t.Fatal("invalid SET LOCAL succeeded")
	}
	if mustLogLevel(t, sv) != "DEBUG" || lv.Level() != slog.LevelDebug {
		t.Fatal("failed SET LOCAL changed value or undo target")
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if mustLogLevel(t, sv) != "INFO" || lv.Level() != slog.LevelInfo {
		t.Fatal("failed SET LOCAL retired undo")
	}
}

func TestLogLevelRegistryRebuildKeepsBinding(t *testing.T) {
	t.Parallel()
	sv, lv := boundLogLevelVars(t, slog.LevelWarn)
	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG"); err != nil {
		t.Fatal(err)
	}
	sv.Registry = nil
	sv.ensureRegistry()
	if mustLogLevel(t, sv) != "DEBUG" || lv.Level() != slog.LevelDebug {
		t.Fatal("registry rebuild dropped reported or runtime DEBUG")
	}
	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "ERROR"); err != nil {
		t.Fatal(err)
	}
	if lv.Level() != slog.LevelError {
		t.Fatal("rebuilt registry did not update runtime LevelVar")
	}
}

func TestLogLevelSessionReplacementKeepsBinding(t *testing.T) {
	t.Parallel()
	sv, lv := boundLogLevelVars(t, slog.LevelWarn)
	sv.Connection = ConnectionVars{Project: "p", Instance: "i", Database: "d"}
	newFake := func() *Session {
		session, err := newSessionWithFactories(
			t.Context(),
			sv,
			func(context.Context, string, spanner.ClientConfig, ...option.ClientOption) (*spanner.Client, error) {
				return &spanner.Client{}, nil
			},
			func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
				return &adminapi.DatabaseAdminClient{}, nil
			},
			func(*spanner.Client) {},
		)
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	handler := NewSessionHandler(newFake())
	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG"); err != nil {
		t.Fatal(err)
	}
	handler.Session = newFake()
	if handler.systemVariables != sv {
		t.Fatal("replacement session forked systemVariables")
	}
	if err := sv.SetFromSimple("CLI_LOG_LEVEL", "ERROR"); err != nil {
		t.Fatal(err)
	}
	if lv.Level() != slog.LevelError || mustLogLevel(t, sv) != "ERROR" {
		t.Fatal("session replacement dropped runtime binding")
	}
}

func TestLogLevelP4EmbeddedSnapshotIgnoresLaterSet(t *testing.T) {
	t.Parallel()

	t.Run("startup WARN stays quiet after SET DEBUG", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Config.EmbeddedLogLevel = slog.LevelWarn
		var lv slog.LevelVar
		lv.Set(slog.LevelWarn)
		sv.runtimeLogLevel = &lv
		sv.ensureRegistry()
		if err := sv.SetFromSimple("CLI_LOG_LEVEL", "DEBUG"); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		fixed := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: sv.Config.EmbeddedLogLevel}))
		testcontainersSlogLogger{logger: fixed}.Printf("container started")
		if buf.Len() != 0 {
			t.Fatalf("embedded WARN logger emitted after SET DEBUG: %q", buf.String())
		}
		if lv.Level() != slog.LevelDebug {
			t.Fatal("CLI runtime did not follow SET DEBUG")
		}
	})

	t.Run("startup DEBUG stays visible after SET WARN", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Config.EmbeddedLogLevel = slog.LevelDebug
		sv.Feature.LogLevel = slog.LevelDebug
		var lv slog.LevelVar
		lv.Set(slog.LevelDebug)
		sv.runtimeLogLevel = &lv
		sv.ensureRegistry()
		if err := sv.SetFromSimple("CLI_LOG_LEVEL", "WARN"); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		fixed := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: sv.Config.EmbeddedLogLevel}))
		testcontainersSlogLogger{logger: fixed}.Printf("container started")
		if !strings.Contains(buf.String(), "container started") {
			t.Fatalf("embedded DEBUG logger quiet after SET WARN: %q", buf.String())
		}
		if lv.Level() != slog.LevelWarn {
			t.Fatal("CLI runtime did not follow SET WARN")
		}
	})
}

func TestLogLevelP4ConfigureBothAdaptersWithFixedLogger(t *testing.T) {
	previousLogger := tclog.Default()
	t.Cleanup(func() {
		tclog.SetDefault(previousLogger)
	})

	var buf bytes.Buffer
	fixed := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	customizer := configureTestcontainersLogger(fixed)
	tclog.Printf("global quiet")
	req := testcontainers.GenericContainerRequest{}
	if err := customizer.Customize(&req); err != nil {
		t.Fatalf("Customize() error = %v", err)
	}
	if req.Logger == nil {
		t.Fatal("Customize() did not configure the per-container logger")
	}
	req.Logger.Printf("container quiet")
	if buf.Len() != 0 {
		t.Fatalf("WARN snapshot emitted after CLI DEBUG would have: %q", buf.String())
	}

	var debugBuf bytes.Buffer
	debugFixed := slog.New(slog.NewTextHandler(&debugBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	debugCustomizer := configureTestcontainersLogger(debugFixed)
	tclog.Printf("global visible")
	req2 := testcontainers.GenericContainerRequest{}
	if err := debugCustomizer.Customize(&req2); err != nil {
		t.Fatalf("Customize() error = %v", err)
	}
	req2.Logger.Printf("container visible")
	got := debugBuf.String()
	for _, want := range []string{"global visible", "container visible"} {
		if !strings.Contains(got, want) {
			t.Fatalf("DEBUG snapshot missing %q: %q", want, got)
		}
	}
}

func TestCachedEnabledDoesNotFreezeDebugToWarn(t *testing.T) {
	t.Parallel()
	var lv slog.LevelVar
	lv.Set(slog.LevelDebug)
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: &lv}))
	enabled := logger.Enabled(context.Background(), slog.LevelInfo)
	lv.Set(slog.LevelWarn)
	if !enabled {
		t.Fatal("startup DEBUG should report Enabled(Info)")
	}
	logger.Info("still-called")
	if buf.Len() != 0 {
		t.Fatalf("cached Enabled plus mutable logger still emitted after WARN: %q", buf.String())
	}
}

func TestParseLogLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{in: "DEBUG", want: slog.LevelDebug},
		{in: "INFO", want: slog.LevelInfo},
		{in: "WARN", want: slog.LevelWarn},
		{in: "WARNING", want: slog.LevelWarn},
		{in: "warning", want: slog.LevelWarn},
		{in: "ERROR", want: slog.LevelError},
		{in: "DEBUG-4", want: slog.LevelDebug - 4},
		{in: "NOPE", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := parseLogLevel(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
