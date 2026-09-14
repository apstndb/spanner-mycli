// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or governed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestRunWithOutputPreservesStartupErrorAfterStuckTracesExport(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	host, port, stop := startFakeMetricsSpanner(t)
	t.Cleanup(stop)

	collector, saw := newHangingOTLPServer()
	t.Cleanup(collector.Close)

	opts := tracesLifecycleOpts(host, port, "", collector.URL)
	opts.InitCommand = "SET CLI_VERSION = 'x'"

	var diag lockedBuffer
	installMetricsRunTestHooks(t, &diag)

	started := time.Now()
	err := runWithOutput(context.Background(), opts, io.Discard)
	elapsed := time.Since(started)
	stderr := diag.String()

	if !strings.Contains(stderr, "read-only") {
		t.Fatalf("startup diagnostics missing original read-only failure; stderr=%q", stderr)
	}
	requirePreservedCommandResult(t, err)
	requireOneBoundedShutdownDiagnostic(t, stderr, elapsed)
	waitFor(t, saw, "stuck traces exporter never received a flush")
}

func TestRunWithOutputPreservesCancelAfterStuckTracesExport(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	queryStarted := make(chan struct{}, 1)
	host, port, stop := startFakeMetricsSpannerServer(t, &fakeMetricsSpanner{queryStarted: queryStarted})
	t.Cleanup(stop)

	collector, saw := newHangingOTLPServer()
	t.Cleanup(collector.Close)

	opts := tracesLifecycleOpts(host, port, "", collector.URL)
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
		t.Fatal("runWithOutput did not return after cancel and bounded traces cleanup")
	}

	stderr := diag.String()
	if got.err == nil {
		t.Fatal("command error = nil, want the cancelled batch result")
	}
	requirePreservedCommandResult(t, got.err)
	requireOneBoundedShutdownDiagnostic(t, stderr, got.elapsed)
	waitFor(t, saw, "stuck traces exporter never received a flush")
}

func TestRunWithOutputSharedBudgetWhenMetricsAndTracesHang(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	host, port, stop := startFakeMetricsSpanner(t)
	t.Cleanup(stop)

	collector, saw := newHangingOTLPServer()
	t.Cleanup(collector.Close)

	opts := tracesLifecycleOpts(host, port, collector.URL, collector.URL)
	opts.InitCommand = "SET CLI_VERSION = 'x'"

	var diag lockedBuffer
	installMetricsRunTestHooks(t, &diag)

	started := time.Now()
	err := runWithOutput(context.Background(), opts, io.Discard)
	elapsed := time.Since(started)
	stderr := diag.String()

	if !strings.Contains(stderr, "read-only") {
		t.Fatalf("startup diagnostics missing original read-only failure; stderr=%q", stderr)
	}
	requirePreservedCommandResult(t, err)
	requireOneBoundedShutdownDiagnostic(t, stderr, elapsed)
	waitFor(t, saw, "stuck telemetry exporter never received a flush")
}

func TestRunWithOutputTracesInitFailureCleansMetrics(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	host, port, stop := startFakeMetricsSpanner(t)
	t.Cleanup(stop)

	sink := newOTLPSink()
	collector := httptest.NewServer(sink)
	t.Cleanup(collector.Close)

	pre, err := startSpannerTraces(&systemVariables{Config: StartupConfig{
		SpannerTracesExporter:    spannerTracesExporterOTLP,
		SpannerTracesEndpoint:    mustTracesEndpoint(t, collector.URL),
		SpannerTracesSampleRatio: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pre.Shutdown(context.Background()) })

	opts := metricsLifecycleOpts(host, port, collector.URL)
	opts.SpannerTracesExporter = spannerTracesExporterOTLP
	opts.SpannerTracesEndpoint = mustTracesEndpoint(t, collector.URL)
	opts.SpannerTracesSampleRatio = 1
	opts.Execute = "SELECT 1"

	var diag lockedBuffer
	installMetricsRunTestHooks(t, &diag)
	err = runWithOutput(context.Background(), opts, io.Discard)
	if err == nil || !strings.Contains(err.Error(), errTracesOwnerExists.Error()) {
		t.Fatalf("init error = %v, want second-owner failure", err)
	}
	if strings.Contains(err.Error(), "Spanner telemetry shutdown") {
		t.Fatalf("cleanup replaced the traces init error: %v", err)
	}
	if otel.GetTracerProvider() != pre.provider {
		t.Fatal("failed run replaced the existing TracerProvider")
	}
	if installedTraces.Load() != pre {
		t.Fatal("failed run replaced the existing traces owner")
	}
}

func tracesLifecycleOpts(host string, port int, metricsURL, tracesURL string) *spannerOptions {
	insecure := true
	opts := &spannerOptions{
		ProjectId:                "p",
		InstanceId:               "i",
		DatabaseId:               "d",
		Host:                     host,
		Port:                     port,
		Insecure:                 &insecure,
		LogLevel:                 "ERROR",
		Quiet:                    true,
		SpannerTracesExporter:    spannerTracesExporterOTLP,
		SpannerTracesSampleRatio: 1,
	}
	if metricsURL != "" {
		canonical, err := parseSpannerMetricsEndpoint(metricsURL)
		if err != nil {
			panic(err)
		}
		opts.SpannerMetricsExporter = spannerMetricsExporterOTLP
		opts.SpannerMetricsEndpoint = canonical
	}
	canonical, err := parseSpannerTracesEndpoint(tracesURL)
	if err != nil {
		panic(err)
	}
	opts.SpannerTracesEndpoint = canonical
	return opts
}
