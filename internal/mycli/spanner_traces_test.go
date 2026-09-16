// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	tracesTestSQL     = "SELECT secret_col FROM t WHERE id = @p"
	sampledTraceIDHex = "0102030405060708090a0b0c0d0e0f10"
)

func TestNormalizeSpannerTracesOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc         string
		exporter     string
		endpoint     string
		ratio        float64
		wantExporter string
		wantEndpoint string
		wantRatio    float64
		errContains  string
	}{
		{desc: "default empty is off", exporter: "", endpoint: "", wantExporter: "off", wantRatio: 0.01},
		{desc: "off plus endpoint rejected", exporter: "off", endpoint: "http://127.0.0.1:4318/v1/traces", errContains: "invalid combination"},
		{desc: "otlp requires endpoint", exporter: "otlp", endpoint: "", errContains: "requires --spanner-traces-endpoint"},
		{desc: "otlp fills default path", exporter: "otlp", endpoint: "http://127.0.0.1:4318", ratio: 0.01, wantExporter: "otlp", wantEndpoint: "http://127.0.0.1:4318/v1/traces", wantRatio: 0.01},
		{desc: "otlp ratio 0 allowed", exporter: "otlp", endpoint: "http://127.0.0.1:4318", ratio: 0, wantExporter: "otlp", wantEndpoint: "http://127.0.0.1:4318/v1/traces"},
		{desc: "ratio above 1 rejected", exporter: "otlp", endpoint: "http://127.0.0.1:4318", ratio: 1.1, errContains: "[0,1]"},
		{desc: "userinfo rejected", exporter: "otlp", endpoint: "http://user:pass@127.0.0.1:4318/v1/traces", ratio: 0.01, errContains: "userinfo"},
		{desc: "unknown exporter rejected", exporter: "jaeger", errContains: "must be off or otlp"},
		{desc: "ratio below 0 rejected", exporter: "otlp", endpoint: "http://127.0.0.1:4318", ratio: -0.1, errContains: "[0,1]"},
		{desc: "NaN rejected", exporter: "otlp", endpoint: "http://127.0.0.1:4318", ratio: math.NaN(), errContains: "[0,1]"},
		{desc: "+Inf rejected", exporter: "otlp", endpoint: "http://127.0.0.1:4318", ratio: math.Inf(1), errContains: "[0,1]"},
		{desc: "-Inf rejected", exporter: "otlp", endpoint: "http://127.0.0.1:4318", ratio: math.Inf(-1), errContains: "[0,1]"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			gotExp, gotEnd, gotRatio, err := normalizeSpannerTracesOptions(tt.exporter, tt.endpoint, tt.ratio)
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %v, want containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if gotExp != tt.wantExporter || gotEnd != tt.wantEndpoint || gotRatio != tt.wantRatio {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", gotExp, gotEnd, gotRatio, tt.wantExporter, tt.wantEndpoint, tt.wantRatio)
			}
		})
	}
}

func TestParseOTLPEndpointParameterizesPath(t *testing.T) {
	t.Parallel()
	metrics, err := parseOTLPEndpoint("http://127.0.0.1:4318", "--spanner-metrics-endpoint", "/v1/metrics")
	if err != nil {
		t.Fatal(err)
	}
	if metrics != "http://127.0.0.1:4318/v1/metrics" {
		t.Fatalf("metrics path = %q", metrics)
	}
	traces, err := parseOTLPEndpoint("http://127.0.0.1:4318", "--spanner-traces-endpoint", "/v1/traces")
	if err != nil {
		t.Fatal(err)
	}
	if traces != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("traces path = %q", traces)
	}
}

func TestParseFlagsSpannerTraces(t *testing.T) {
	t.Parallel()
	gopts, err := parseTestFlags(withRequiredFlags(
		"--spanner-traces-exporter=otlp",
		"--spanner-traces-endpoint=http://127.0.0.1:4318",
		"--spanner-traces-sample-ratio=1",
	))
	if err != nil {
		t.Fatal(err)
	}
	if gopts.Spanner.SpannerTracesExporter != "otlp" {
		t.Fatalf("exporter = %q", gopts.Spanner.SpannerTracesExporter)
	}
	if gopts.Spanner.SpannerTracesEndpoint != "http://127.0.0.1:4318" {
		t.Fatalf("endpoint = %q", gopts.Spanner.SpannerTracesEndpoint)
	}
	if gopts.Spanner.SpannerTracesSampleRatio != 1 {
		t.Fatalf("ratio = %v", gopts.Spanner.SpannerTracesSampleRatio)
	}
}

func TestParseAndValidateRejectsNonFiniteSampleRatio(t *testing.T) {
	t.Parallel()
	for _, ratio := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf"} {
		t.Run(ratio, func(t *testing.T) {
			t.Parallel()
			_, err := parseAndValidate(withRequiredFlags(
				"--spanner-traces-exporter=otlp",
				"--spanner-traces-endpoint=http://127.0.0.1:4318",
				"--spanner-traces-sample-ratio="+ratio,
			))
			if err == nil || !strings.Contains(err.Error(), "[0,1]") {
				t.Fatalf("real CLI parser/validation error = %v, want [0,1]", err)
			}
		})
	}
}

func TestInitializeSystemVariablesAppliesTracesFlags(t *testing.T) {
	t.Parallel()
	sysVars, err := initializeSystemVariables(&spannerOptions{
		ProjectId:                "p",
		InstanceId:               "i",
		DatabaseId:               "d",
		SpannerTracesExporter:    "otlp",
		SpannerTracesEndpoint:    "http://127.0.0.1:4318",
		SpannerTracesSampleRatio: 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sysVars.Config.SpannerTracesExporter != "otlp" {
		t.Fatalf("exporter = %q", sysVars.Config.SpannerTracesExporter)
	}
	if sysVars.Config.SpannerTracesEndpoint != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("endpoint = %q", sysVars.Config.SpannerTracesEndpoint)
	}
	if sysVars.Config.SpannerTracesSampleRatio != 0.2 {
		t.Fatalf("ratio = %v", sysVars.Config.SpannerTracesSampleRatio)
	}

	offVars, err := initializeSystemVariables(&spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if offVars.Config.SpannerTracesExporter != "off" || offVars.Config.SpannerTracesEndpoint != "" || offVars.Config.SpannerTracesSampleRatio != 0.01 {
		t.Fatalf("defaults = %q / %q / %v", offVars.Config.SpannerTracesExporter, offVars.Config.SpannerTracesEndpoint, offVars.Config.SpannerTracesSampleRatio)
	}
}

func TestSpannerTracesResetRejected(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	if err := sysVars.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"CLI_SPANNER_TRACES_EXPORTER", "CLI_SPANNER_TRACES_ENDPOINT", "CLI_SPANNER_TRACES_SAMPLE_RATIO"} {
		if err := sysVars.Reset(name); err == nil || !strings.Contains(err.Error(), "does not support RESET") {
			t.Fatalf("RESET %s: %v", name, err)
		}
	}
}

func TestStartSpannerTracesOffLeavesGlobal(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:1/v1/traces")
	before := otel.GetTracerProvider()
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOff
	owner, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	if owner.provider != nil {
		t.Fatal("off mode constructed a TracerProvider")
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("off mode changed the global TracerProvider")
	}
}

func TestStartSpannerTracesRejectsSecondOwner(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	sink := newOTLPSink()
	collector := httptest.NewServer(sink)
	t.Cleanup(collector.Close)

	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOTLP
	sysVars.Config.SpannerTracesEndpoint = mustTracesEndpoint(t, collector.URL)
	sysVars.Config.SpannerTracesSampleRatio = 1

	first, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Shutdown(context.Background()) })

	second, err := startSpannerTraces(sysVars)
	if !errors.Is(err, errTracesOwnerExists) {
		t.Fatalf("second owner: %v", err)
	}
	if second != nil && second.provider != nil {
		t.Fatal("failed second install leaked a provider")
	}
	if otel.GetTracerProvider() != first.provider {
		t.Fatal("second install replaced the first provider")
	}
}

func TestRestoreGlobalDoesNotOverwriteLaterProvider(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	sink := newOTLPSink()
	collector := httptest.NewServer(sink)
	t.Cleanup(collector.Close)

	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOTLP
	sysVars.Config.SpannerTracesEndpoint = mustTracesEndpoint(t, collector.URL)
	sysVars.Config.SpannerTracesSampleRatio = 1
	owner, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	other := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = other.Shutdown(context.Background()) })
	otel.SetTracerProvider(other)
	owner.restoreGlobal()
	if otel.GetTracerProvider() != other {
		t.Fatal("restore overwrote a later provider")
	}
	_ = owner.Shutdown(context.Background())
	if otel.GetTracerProvider() != other {
		t.Fatal("shutdown overwrote a later provider")
	}
}

func TestOverlayEndToEndTracingCopiesEmbed(t *testing.T) {
	t.Parallel()
	embed := &spanner.ClientConfig{
		DisableNativeMetrics:  true,
		Type:                  spanner.OMNI,
		EnableEndToEndTracing: false,
		UserAgent:             "embedded-traces",
	}
	sysVars := &systemVariables{
		Connection: ConnectionVars{Project: "p", Instance: "i", Database: "d"},
		Config: StartupConfig{
			EmbeddedClientConfig:  embed,
			SpannerTracesExporter: spannerTracesExporterOTLP,
		},
	}
	var gotConfig spanner.ClientConfig
	session, err := newSessionWithFactories(
		context.Background(),
		sysVars,
		sysVars.Connection,
		func(_ context.Context, _ string, cfg spanner.ClientConfig, _ ...option.ClientOption) (*spanner.Client, error) {
			gotConfig = cfg
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
	if !gotConfig.EnableEndToEndTracing {
		t.Fatal("otlp traces did not set EnableEndToEndTracing on the copy")
	}
	if embed.EnableEndToEndTracing {
		t.Fatal("source EmbeddedClientConfig mutated")
	}
	if session.clientConfig.EnableEndToEndTracing != gotConfig.EnableEndToEndTracing {
		t.Fatal("session clientConfig lost e2e overlay")
	}
}

func TestFakeRPCOTLPTracesPrivacyAndReuse(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	unsetEnvForTest(t,
		"SPANNER_ENABLE_END_TO_END_TRACING",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_COMPRESSION",
		"OTEL_EXPORTER_OTLP_TRACES_COMPRESSION",
	)

	fake := &fakeTracesSpanner{}
	host, port, stop := startFakeMetricsSpannerServer(t, fake)
	t.Cleanup(stop)

	sink := newOTLPSink()
	collector := httptest.NewServer(sink)
	t.Cleanup(collector.Close)

	sysVars := newMetricsTestSysVars(host, port)
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOTLP
	sysVars.Config.SpannerTracesEndpoint = mustTracesEndpoint(t, collector.URL)
	sysVars.Config.SpannerTracesSampleRatio = 1

	owner, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown(context.Background()) })

	session, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatal(err)
	}
	if !session.clientConfig.EnableEndToEndTracing {
		t.Fatal("session missing EnableEndToEndTracing")
	}
	runFakeTracesSelect(t, session.client, sampledTraceContext())

	oldClient := session.client
	if err := session.RecreateClient(context.Background()); err != nil {
		t.Fatal(err)
	}
	if session.client == oldClient {
		t.Fatal("RecreateClient did not replace the data client")
	}
	runFakeTracesSelect(t, session.client, sampledTraceContext())
	session.Close()
	if otel.GetTracerProvider() != owner.provider {
		t.Fatal("Session.Close replaced the CLI TracerProvider")
	}

	replacement, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatal(err)
	}
	runFakeTracesSelect(t, replacement.client, sampledTraceContext())
	replacement.Close()

	injectAdversarialAllowedSpan(t, sampledTraceContextWithState(t))
	injectRejectedUnknownSpans(t, sampledTraceContextWithState(t))
	if err := owner.provider.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !e2eHeaderPresent(fake.md) {
		t.Fatal("CLI traces opt-in did not send x-goog-spanner-end-to-end-tracing")
	}
	names := requireSanitizedTraceExport(t, sink, true)
	if !containsAllStrings(names, []string{"Query", "RowIterator", "NewClient"}) {
		t.Fatalf("useful spans missing: %v", names)
	}
	if containsAnyString(names, []string{"ExecuteStreamingSql", "UnknownOp", "ADV_NAME_SELECT_secret_col"}) {
		t.Fatalf("out-of-scope names leaked: %v", names)
	}
}

func TestParentBasedSampling(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	sink := newOTLPSink()
	collector := httptest.NewServer(sink)
	t.Cleanup(collector.Close)
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOTLP
	sysVars.Config.SpannerTracesEndpoint = mustTracesEndpoint(t, collector.URL)
	sysVars.Config.SpannerTracesSampleRatio = 0.01
	owner, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown(context.Background()) })

	tr := otel.Tracer(spannerTracesScopeName)
	_, dropped := tr.Start(unsampledTraceContext(), "cloud.google.com/go/spanner.Query")
	if dropped.IsRecording() {
		t.Fatal("unsampled parent recorded")
	}
	dropped.End()
	_, kept := tr.Start(sampledTraceContext(), "cloud.google.com/go/spanner.Query")
	if !kept.IsRecording() {
		t.Fatal("sampled parent did not record")
	}
	kept.End()

	zero := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0))))
	t.Cleanup(func() { _ = zero.Shutdown(context.Background()) })
	_, root0 := zero.Tracer(spannerTracesScopeName).Start(context.Background(), "cloud.google.com/go/spanner.Query")
	if root0.IsRecording() {
		t.Fatal("root ratio 0 recorded")
	}
	root0.End()
	one := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1))))
	t.Cleanup(func() { _ = one.Shutdown(context.Background()) })
	_, root1 := one.Tracer(spannerTracesScopeName).Start(context.Background(), "cloud.google.com/go/spanner.Query")
	if !root1.IsRecording() {
		t.Fatal("root ratio 1 did not record")
	}
	root1.End()
}

func TestSDKEnvE2EHeaderWithoutCLIPipeline(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	t.Setenv("SPANNER_ENABLE_END_TO_END_TRACING", "true")
	fake := &fakeTracesSpanner{}
	host, port, stop := startFakeMetricsSpannerServer(t, fake)
	t.Cleanup(stop)
	before := otel.GetTracerProvider()
	sysVars := newMetricsTestSysVars(host, port)
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOff
	owner, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	if owner.provider != nil || otel.GetTracerProvider() != before {
		t.Fatal("env override started a CLI traces pipeline")
	}
	session, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	runFakeTracesSelect(t, session.client, context.Background())
	if !e2eHeaderPresent(fake.md) {
		t.Fatal("SDK env override did not send the e2e header")
	}
}

func TestRepeatTracesOwnerAfterShutdown(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	sink := newOTLPSink()
	collector := httptest.NewServer(sink)
	t.Cleanup(collector.Close)
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOTLP
	sysVars.Config.SpannerTracesEndpoint = mustTracesEndpoint(t, collector.URL)
	sysVars.Config.SpannerTracesSampleRatio = 1
	first, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Shutdown(context.Background()) })
	if second.provider == nil || otel.GetTracerProvider() != second.provider {
		t.Fatal("repeat install did not own an actual provider")
	}
}

func TestStartSpannerTracesLeavesMetricsAndEnv(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:1/v1/traces")
	beforeMeter := otel.GetMeterProvider()
	injected := sdkmetric.NewMeterProvider()
	t.Cleanup(func() { _ = injected.Shutdown(context.Background()) })

	sink := newOTLPSink()
	collector := httptest.NewServer(sink)
	t.Cleanup(collector.Close)
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Config.ClientMetricsProvider = injected
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOTLP
	sysVars.Config.SpannerTracesEndpoint = mustTracesEndpoint(t, collector.URL)
	sysVars.Config.SpannerTracesSampleRatio = 1
	owner, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown(context.Background()) })
	if sysVars.Config.ClientMetricsProvider != injected {
		t.Fatal("traces setup replaced an injected ClientMetricsProvider")
	}
	if otel.GetMeterProvider() != beforeMeter {
		t.Fatal("traces setup installed a global MeterProvider")
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "http://127.0.0.1:1/v1/traces" {
		t.Fatal("traces setup mutated OTEL env")
	}
}

func TestStartSpannerTracesCLIURLWinsOverEnv(t *testing.T) {
	restoreTestTracerProvider(t)
	clearSpannerEmulatorHost(t)
	unsetEnvForTest(t, "OTEL_EXPORTER_OTLP_COMPRESSION", "OTEL_EXPORTER_OTLP_TRACES_COMPRESSION")

	fake := &fakeTracesSpanner{}
	host, port, stop := startFakeMetricsSpannerServer(t, fake)
	t.Cleanup(stop)

	wantPath := "/custom/otlp/traces"
	sink := newOTLPSink()
	mux := http.NewServeMux()
	mux.Handle(wantPath, sink)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("CLI collector received unexpected path %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	collector := httptest.NewServer(mux)
	t.Cleanup(collector.Close)

	decoy := newOTLPSink()
	decoySrv := httptest.NewServer(decoy)
	t.Cleanup(decoySrv.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", decoySrv.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", decoySrv.URL+"/v1/traces")

	sysVars := newMetricsTestSysVars(host, port)
	sysVars.Config.SpannerTracesExporter = spannerTracesExporterOTLP
	canonical, err := parseSpannerTracesEndpoint(collector.URL + wantPath)
	if err != nil {
		t.Fatal(err)
	}
	sysVars.Config.SpannerTracesEndpoint = canonical
	sysVars.Config.SpannerTracesSampleRatio = 1
	owner, err := startSpannerTraces(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown(context.Background()) })

	session, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatal(err)
	}
	runFakeTracesSelect(t, session.client, sampledTraceContext())
	session.Close()
	if err := owner.provider.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if decoy.count() != 0 {
		t.Fatalf("env decoy received %d posts", decoy.count())
	}
	for _, p := range sink.snapshot() {
		if p.path != wantPath {
			t.Fatalf("path = %q, want %q", p.path, wantPath)
		}
	}
	names := requireSanitizedTraceExport(t, sink, false)
	if !containsAllStrings(names, []string{"Query", "RowIterator"}) {
		t.Fatalf("useful spans missing on CLI path: %v", names)
	}
}

func TestOverlayEndToEndTracingOffLeavesEmbed(t *testing.T) {
	t.Parallel()
	embed := &spanner.ClientConfig{EnableEndToEndTracing: false, UserAgent: "embedded-off"}
	sysVars := &systemVariables{
		Connection: ConnectionVars{Project: "p", Instance: "i", Database: "d"},
		Config: StartupConfig{
			EmbeddedClientConfig:  embed,
			SpannerTracesExporter: spannerTracesExporterOff,
		},
	}
	var gotConfig spanner.ClientConfig
	_, err := newSessionWithFactories(
		context.Background(),
		sysVars,
		sysVars.Connection,
		func(_ context.Context, _ string, cfg spanner.ClientConfig, _ ...option.ClientOption) (*spanner.Client, error) {
			gotConfig = cfg
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
	if gotConfig.EnableEndToEndTracing {
		t.Fatal("off mode set EnableEndToEndTracing")
	}
	if embed.EnableEndToEndTracing {
		t.Fatal("source EmbeddedClientConfig mutated")
	}
	if gotConfig.UserAgent != "embedded-off" {
		t.Fatalf("embed fields not preserved: %+v", gotConfig)
	}
}

type fakeTracesSpanner struct {
	fakeMetricsSpanner
	md metadata.MD
}

func (s *fakeTracesSpanner) CreateSession(ctx context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok && s.md == nil {
		s.md = md.Copy()
	}
	return s.fakeMetricsSpanner.CreateSession(ctx, r)
}

func mustTracesEndpoint(t *testing.T, raw string) string {
	t.Helper()
	canonical, err := parseSpannerTracesEndpoint(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func sampledTraceContext() context.Context {
	return sampledRemoteContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
}

func sampledTraceContextWithState(t *testing.T) context.Context {
	t.Helper()
	ts, err := trace.ParseTraceState("adv=ADV_TS_SELECT_secret_col")
	if err != nil {
		t.Fatal(err)
	}
	return sampledRemoteContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		TraceState: ts,
		Remote:     true,
	})
}

func sampledRemoteContext(cfg trace.SpanContextConfig) context.Context {
	return trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(cfg))
}

func unsampledTraceContext() context.Context {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{9},
		SpanID:  trace.SpanID{9},
		Remote:  true,
	})
	return trace.ContextWithRemoteSpanContext(context.Background(), sc)
}

func runFakeTracesSelect(t *testing.T, client *spanner.Client, ctx context.Context) {
	t.Helper()
	if client == nil {
		t.Fatal("client is nil")
	}
	iter := client.Single().Query(ctx, spanner.Statement{
		SQL:    tracesTestSQL,
		Params: map[string]any{"p": "bindval_sentinel_7"},
	})
	defer iter.Stop()
	for {
		if _, err := iter.Next(); err != nil {
			if errors.Is(err, iterator.Done) {
				return
			}
			t.Fatalf("Query: %v", err)
		}
	}
}

func injectAdversarialAllowedSpan(t *testing.T, ctx context.Context) {
	t.Helper()
	ts, err := trace.ParseTraceState("adv=ADV_TS_SELECT_secret_col")
	if err != nil {
		t.Fatal(err)
	}
	linkSC := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{2},
		SpanID:     trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
		TraceState: ts,
		Remote:     true,
	})
	tr := otel.Tracer(spannerTracesScopeName, trace.WithInstrumentationVersion("ADV_SCOPE_secret_col"))
	_, span := tr.Start(ctx, "cloud.google.com/go/spanner.Query",
		trace.WithAttributes(attribute.String("db.statement", tracesTestSQL)),
		trace.WithLinks(trace.Link{SpanContext: linkSC, Attributes: []attribute.KeyValue{attribute.String("link", "ADV_LINK_secret_col")}}),
	)
	span.SetStatus(codes.Error, "Syntax error near secret_col")
	span.RecordError(errors.New(tracesTestSQL))
	span.AddEvent("ADV_ERR_Syntax_error_near_secret_col")
	span.End()
	_, row := tr.Start(ctx, "cloud.google.com/go/spanner.RowIterator")
	row.SetStatus(codes.Error, tracesTestSQL)
	row.End()
}

func injectRejectedUnknownSpans(t *testing.T, ctx context.Context) {
	t.Helper()
	tr := otel.Tracer(spannerTracesScopeName, trace.WithInstrumentationVersion("ADV_SCOPE_secret_col"))
	_, renamed := tr.Start(ctx, "cloud.google.com/go/spanner.Query")
	renamed.SetName("ADV_NAME_SELECT_secret_col")
	renamed.End()
	_, ev := otel.Tracer("google.golang.org/grpc").Start(ctx, "google.spanner.v1.Spanner/ExecuteStreamingSql")
	ev.End()
	_, unk := tr.Start(ctx, "cloud.google.com/go/spanner.UnknownOp")
	unk.End()
}

func unsetEnvForTest(t *testing.T, keys ...string) {
	t.Helper()
	type snap struct {
		key     string
		val     string
		present bool
	}
	saved := make([]snap, 0, len(keys))
	for _, key := range keys {
		val, present := os.LookupEnv(key)
		saved = append(saved, snap{key: key, val: val, present: present})
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, s := range saved {
			if s.present {
				if err := os.Setenv(s.key, s.val); err != nil {
					t.Errorf("restore %s: %v", s.key, err)
				}
				continue
			}
			if err := os.Unsetenv(s.key); err != nil {
				t.Errorf("restore unset %s: %v", s.key, err)
			}
		}
	})
}

func restoreTestTracerProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetTracerProvider()
	t.Cleanup(func() {
		if isPinnedInitialProxy(prev) {
			otel.SetTracerProvider(noop.NewTracerProvider())
			return
		}
		otel.SetTracerProvider(prev)
	})
}

func e2eHeaderPresent(md metadata.MD) bool {
	if md == nil {
		return false
	}
	return slices.Contains(md.Get("x-goog-spanner-end-to-end-tracing"), "true")
}

func decodeTraceBody(body []byte, encoding string) ([]byte, error) {
	if encoding == "gzip" {
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gr.Close() }()
		return io.ReadAll(gr)
	}
	return body, nil
}

func requireSanitizedTraceExport(t *testing.T, sink *otlpSink, wantErrorCode bool) []string {
	t.Helper()
	posts := sink.snapshot()
	if len(posts) == 0 {
		t.Fatal("CLI collector received no OTLP HTTP posts")
	}
	var names []string
	sawSampledTrace := false
	sawErrorCode := false
	sawFixedResource := false
	needles := []string{
		tracesTestSQL, "bindval_sentinel_7", "rowval_sentinel_99",
		"Syntax error near secret_col", "secret_col", "ADV_SCOPE_secret_col",
		"ADV_TS_SELECT_secret_col", "ADV_LINK_secret_col", "ADV_NAME_SELECT_secret_col",
	}
	for _, p := range posts {
		if p.path == "" {
			t.Fatal("official export missing URL path")
		}
		if p.method != "" && p.method != http.MethodPost {
			t.Fatalf("method = %q", p.method)
		}
		if p.contentType != "" && p.contentType != "application/x-protobuf" {
			t.Fatalf("content-type = %q", p.contentType)
		}
		body, err := decodeTraceBody(p.body, p.encoding)
		if err != nil {
			t.Fatal(err)
		}
		var req coltracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal official export: %v", err)
		}
		var fields []string
		walkTraceStrings(req.ProtoReflect(), &fields)
		joined := string(body) + prototext.Format(&req) + strings.Join(fields, "\n")
		for _, n := range needles {
			if strings.Contains(joined, n) {
				t.Fatalf("sentinel %q escaped official OTLP body", n)
			}
		}
		for _, rs := range req.GetResourceSpans() {
			if res := rs.GetResource(); res != nil {
				attrs := res.GetAttributes()
				if len(attrs) != 1 || attrs[0].GetKey() != "service.name" || attrs[0].GetValue().GetStringValue() != spannerTracesServiceName {
					t.Fatalf("resource attributes = %v", attrs)
				}
				sawFixedResource = true
			}
			for _, ss := range rs.GetScopeSpans() {
				if sc := ss.GetScope(); sc != nil {
					if sc.GetName() != spannerTracesScopeName || sc.GetVersion() != spannerTracesScopeVersion {
						t.Fatalf("scope = %s %s", sc.GetName(), sc.GetVersion())
					}
					if len(sc.GetAttributes()) != 0 {
						t.Fatal("scope attributes leaked")
					}
				}
				for _, sp := range ss.GetSpans() {
					names = append(names, sp.GetName())
					if len(sp.GetAttributes()) != 0 || len(sp.GetEvents()) != 0 || len(sp.GetLinks()) != 0 || sp.GetTraceState() != "" || sp.GetStatus().GetMessage() != "" {
						t.Fatalf("privacy carrier leaked on %s", sp.GetName())
					}
					if len(sp.GetTraceId()) != 16 || len(sp.GetSpanId()) != 8 {
						t.Fatalf("span %s lost IDs", sp.GetName())
					}
					if sp.GetStartTimeUnixNano() == 0 || sp.GetEndTimeUnixNano() == 0 {
						t.Fatalf("span %s lost timing", sp.GetName())
					}
					if hex.EncodeToString(sp.GetTraceId()) == sampledTraceIDHex {
						sawSampledTrace = true
					}
					if sp.GetStatus().GetCode() == 2 {
						if sp.GetStatus().GetMessage() != "" {
							t.Fatalf("error status leaked a description on %s", sp.GetName())
						}
						sawErrorCode = true
					}
				}
			}
		}
	}
	if !sawSampledTrace {
		t.Fatal("posted body lost the sampled parent trace id")
	}
	if !sawFixedResource {
		t.Fatal("posted body missing fixed service.name resource")
	}
	if wantErrorCode && !sawErrorCode {
		t.Fatal("posted body lost retained error status code on an exported allowed span")
	}
	return names
}

func walkTraceStrings(m protoreflect.Message, out *[]string) {
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.Kind() == protoreflect.StringKind && !fd.IsList():
			*out = append(*out, v.String())
		case fd.Kind() == protoreflect.StringKind && fd.IsList():
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				*out = append(*out, l.Get(i).String())
			}
		case fd.Kind() == protoreflect.MessageKind && !fd.IsList():
			walkTraceStrings(v.Message(), out)
		case fd.IsList() && fd.Kind() == protoreflect.MessageKind:
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				walkTraceStrings(l.Get(i).Message(), out)
			}
		}
		return true
	})
}

func containsAllStrings(have, need []string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, n := range need {
		if !set[n] {
			return false
		}
	}
	return true
}

func containsAnyString(have, bad []string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, b := range bad {
		if set[b] {
			return true
		}
	}
	return false
}
