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
// See the License for the terms governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// stuckExporterCase is one exporter configuration for the startup-failure and
// cancellation runners. Do not add t.Parallel: metricsRunTest hooks and the
// tracer provider are process-global.
type stuckExporterCase struct {
	name          string
	restoreTracer bool
	opts          func(host string, port int, collectorURL string) *spannerOptions
}

func stuckExporterCases() []stuckExporterCase {
	return []stuckExporterCase{
		{
			name: "metrics-only",
			opts: metricsLifecycleOpts,
		},
		{
			name:          "traces-only",
			restoreTracer: true,
			opts: func(host string, port int, collectorURL string) *spannerOptions {
				return tracesLifecycleOpts(host, port, "", collectorURL)
			},
		},
	}
}

func TestRunWithOutputPreservesStartupErrorAfterStuckExport(t *testing.T) {
	for _, tc := range stuckExporterCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.restoreTracer {
				restoreTestTracerProvider(t)
			}
			clearSpannerEmulatorHost(t)
			host, port, stop := startFakeMetricsSpanner(t)
			t.Cleanup(stop)

			collector, saw := newHangingOTLPServer()
			t.Cleanup(collector.Close)

			opts := tc.opts(host, port, collector.URL)
			opts.InitCommand = "SET CLI_VERSION = 'x'"

			var diag lockedBuffer
			installMetricsRunTestHooks(t, &diag)

			started := time.Now()
			err := runWithOutput(context.Background(), opts, io.Discard)
			elapsed := time.Since(started)
			stderr := diag.String()

			// executeStartupSQL prints the read-only failure, then returns the same
			// ExitCodeError batch path used for a failed --init-command.
			if !strings.Contains(stderr, "read-only") {
				t.Fatalf("startup diagnostics missing original read-only failure; stderr=%q", stderr)
			}
			requirePreservedCommandResult(t, err)
			requireOneBoundedShutdownDiagnostic(t, stderr, elapsed)
			waitFor(t, saw, tc.name+": stuck exporter never received a flush")
		})
	}
}

func TestRunWithOutputPreservesCancelAfterStuckExport(t *testing.T) {
	for _, tc := range stuckExporterCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.restoreTracer {
				restoreTestTracerProvider(t)
			}
			clearSpannerEmulatorHost(t)
			queryStarted := make(chan struct{}, 1)
			host, port, stop := startFakeMetricsSpannerServer(t, &fakeMetricsSpanner{queryStarted: queryStarted})
			t.Cleanup(stop)

			collector, saw := newHangingOTLPServer()
			t.Cleanup(collector.Close)

			opts := tc.opts(host, port, collector.URL)
			opts.Execute = "SELECT 1"

			var diag lockedBuffer
			installMetricsRunTestHooks(t, &diag)

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			type outcome struct {
				err     error
				elapsed time.Duration
			}
			done := make(chan outcome, 1)
			go func() {
				started := time.Now()
				err := runWithOutput(ctx, opts, io.Discard)
				done <- outcome{err: err, elapsed: time.Since(started)}
			}()

			waitFor(t, queryStarted, "command never reached the fake ExecuteSql RPC")
			cancel()

			var got outcome
			select {
			case got = <-done:
			case <-time.After(spannerTelemetryCleanupBound + 3*time.Second):
				t.Fatalf("%s: runWithOutput did not return after cancel and bounded cleanup", tc.name)
			}

			stderr := diag.String()
			if got.err == nil {
				t.Fatal("command error = nil, want the cancelled batch result")
			}
			requirePreservedCommandResult(t, got.err)
			requireOneBoundedShutdownDiagnostic(t, stderr, got.elapsed)
			waitFor(t, saw, tc.name+": stuck exporter never received a flush")
		})
	}
}

func installMetricsRunTestHooks(t *testing.T, errStream io.Writer) {
	t.Helper()
	metricsRunTest.Store(&metricsRunTestHooks{
		errStream: errStream,
		adjust: func(sv *systemVariables) {
			// Same assignment the embedded-emulator path makes after
			// ValidateSpannerOptions: --without-authentication cannot be
			// combined with --insecure at flag validation.
			sv.Config.WithoutAuthentication = true
		},
	})
	t.Cleanup(func() { metricsRunTest.Store(nil) })
}

func requirePreservedCommandResult(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want the original command result")
	}
	if strings.Contains(err.Error(), "Spanner telemetry shutdown") || strings.Contains(err.Error(), "Spanner metrics shutdown") {
		t.Fatalf("cleanup replaced the original result: %v", err)
	}
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want the batch/startup ExitCodeError", err)
	}
	if got := GetExitCode(err); got != exitCodeError {
		t.Fatalf("exit code = %d, want %d", got, exitCodeError)
	}
}

func metricsLifecycleOpts(host string, port int, collectorURL string) *spannerOptions {
	insecure := true
	canonical, err := parseSpannerMetricsEndpoint(collectorURL)
	if err != nil {
		panic(err)
	}
	return &spannerOptions{
		ProjectId:              "p",
		InstanceId:             "i",
		DatabaseId:             "d",
		Host:                   host,
		Port:                   port,
		Insecure:               &insecure,
		LogLevel:               "ERROR",
		Quiet:                  true,
		SpannerMetricsExporter: spannerMetricsExporterOTLP,
		SpannerMetricsEndpoint: canonical,
	}
}

func newHangingOTLPServer() (*httptest.Server, <-chan struct{}) {
	saw := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		once.Do(func() { close(saw) })
		<-r.Context().Done()
	}))
	return srv, saw
}

func requireOneBoundedShutdownDiagnostic(t *testing.T, stderr string, elapsed time.Duration) {
	t.Helper()
	const warning = "WARNING: Spanner telemetry shutdown failed"
	if n := strings.Count(stderr, warning); n != 1 {
		t.Fatalf("shutdown diagnostics = %d, want 1; stderr=%q", n, stderr)
	}
	if elapsed < 4*time.Second || elapsed > spannerMetricsCleanupBound+2*time.Second {
		t.Fatalf("cleanup elapsed %s, want one bound near %s", elapsed, spannerMetricsCleanupBound)
	}
}

func waitFor(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal(msg)
	}
}

// lockedBuffer is an io.Writer used to capture runWithOutput diagnostics
// without racing Process-wide os.Stderr or concurrent exporter logs.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}
