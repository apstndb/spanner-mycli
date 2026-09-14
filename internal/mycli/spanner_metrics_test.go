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
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const metricsTestSQL = "SELECT secret_col FROM t WHERE id = @p"

type fakeMetricsSpanner struct {
	sppb.UnimplementedSpannerServer
	queryStarted chan<- struct{}
}

func (s *fakeMetricsSpanner) CreateSession(_ context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Database + "/sessions/s", Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *fakeMetricsSpanner) BatchCreateSessions(_ context.Context, r *sppb.BatchCreateSessionsRequest) (*sppb.BatchCreateSessionsResponse, error) {
	n := int(r.SessionCount)
	if n <= 0 {
		n = 1
	}
	sessions := make([]*sppb.Session, n)
	for i := range n {
		sessions[i] = &sppb.Session{Name: r.Database + "/sessions/b", CreateTime: timestamppb.Now()}
	}
	return &sppb.BatchCreateSessionsResponse{Session: sessions}, nil
}

func (s *fakeMetricsSpanner) GetSession(_ context.Context, r *sppb.GetSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Name, Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *fakeMetricsSpanner) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *fakeMetricsSpanner) BeginTransaction(context.Context, *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	return &sppb.Transaction{Id: []byte("tx"), ReadTimestamp: timestamppb.Now()}, nil
}

func (s *fakeMetricsSpanner) ExecuteStreamingSql(_ *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	if s.queryStarted != nil {
		select {
		case s.queryStarted <- struct{}{}:
		default:
		}
		<-stream.Context().Done()
		return stream.Context().Err()
	}
	return stream.Send(&sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "secret_col", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			}},
			Transaction: &sppb.Transaction{Id: []byte("tx"), ReadTimestamp: timestamppb.Now()},
		},
		Values: []*structpb.Value{structpb.NewStringValue("99")},
	})
}

func startFakeMetricsSpanner(t *testing.T) (host string, port int, stop func()) {
	t.Helper()
	return startFakeMetricsSpannerServer(t, &fakeMetricsSpanner{})
}

func startFakeMetricsSpannerServer(t *testing.T, srv sppb.SpannerServer) (host string, port int, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, srv)
	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	host, portStr, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		grpcServer.Stop()
		t.Fatal(err)
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		grpcServer.Stop()
		t.Fatal(err)
	}
	return host, port, func() {
		grpcServer.Stop()
		_ = lis.Close()
	}
}

func newCallerMetricsProvider() (*sdkmetric.ManualReader, *sdkmetric.MeterProvider) {
	reader := sdkmetric.NewManualReader()
	opts := append(spanner.ClientMetricsMeterProviderOptions(), sdkmetric.WithReader(reader))
	return reader, sdkmetric.NewMeterProvider(opts...)
}

func collectCallerMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	return rm
}

func callerMetricNames(rm metricdata.ResourceMetrics) []string {
	var names []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
		}
	}
	return names
}

func callerMetricAttrKeysAndValues(rm metricdata.ResourceMetrics) (keys, values []string) {
	visit := func(set attribute.Set) {
		iter := set.Iter()
		for iter.Next() {
			kv := iter.Attribute()
			keys = append(keys, string(kv.Key))
			values = append(values, kv.Value.String())
		}
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, p := range data.DataPoints {
					visit(p.Attributes)
				}
			case metricdata.Histogram[float64]:
				for _, p := range data.DataPoints {
					visit(p.Attributes)
				}
			}
		}
	}
	return keys, values
}

func runFakeMetricsSelect(t *testing.T, client *spanner.Client) {
	t.Helper()
	if client == nil {
		t.Fatal("client is nil")
	}
	iter := client.Single().Query(context.Background(), spanner.Statement{
		SQL:    metricsTestSQL,
		Params: map[string]any{"p": 1},
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

func newMetricsTestSysVars(host string, port int) *systemVariables {
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Connection = ConnectionVars{
		Project:  "p",
		Instance: "i",
		Database: "d",
	}
	sysVars.Config.Host = host
	sysVars.Config.Port = port
	sysVars.Config.Insecure = true
	sysVars.Config.WithoutAuthentication = true
	return sysVars
}

func clearSpannerEmulatorHost(t *testing.T) {
	t.Helper()
	if os.Getenv("SPANNER_EMULATOR_HOST") != "" {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
	}
}

func TestNormalizeSpannerMetricsOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc         string
		exporter     string
		endpoint     string
		wantExporter string
		wantEndpoint string
		errContains  string
	}{
		{desc: "default empty is off", exporter: "", endpoint: "", wantExporter: "off"},
		{desc: "off trims", exporter: " OFF ", endpoint: "", wantExporter: "off"},
		{desc: "off plus endpoint rejected", exporter: "off", endpoint: "http://127.0.0.1:4318/v1/metrics", errContains: "invalid combination"},
		{desc: "otlp requires endpoint", exporter: "otlp", endpoint: "", errContains: "requires --spanner-metrics-endpoint"},
		{desc: "otlp fills default path", exporter: "otlp", endpoint: "http://127.0.0.1:4318", wantExporter: "otlp", wantEndpoint: "http://127.0.0.1:4318/v1/metrics"},
		{desc: "otlp root path becomes default", exporter: "otlp", endpoint: "https://collector.example/", wantExporter: "otlp", wantEndpoint: "https://collector.example/v1/metrics"},
		{desc: "otlp preserves explicit path", exporter: "otlp", endpoint: "http://127.0.0.1:4318/otlp/v1/metrics", wantExporter: "otlp", wantEndpoint: "http://127.0.0.1:4318/otlp/v1/metrics"},
		{desc: "otlp accepts IPv6 host", exporter: "otlp", endpoint: "http://[::1]:4318", wantExporter: "otlp", wantEndpoint: "http://[::1]:4318/v1/metrics"},
		{desc: "port-only authority rejected", exporter: "otlp", endpoint: "http://:4318/v1/metrics", errContains: "absolute http or https"},
		{desc: "relative url rejected", exporter: "otlp", endpoint: "/v1/metrics", errContains: "absolute http or https"},
		{desc: "missing scheme rejected", exporter: "otlp", endpoint: "127.0.0.1:4318/v1/metrics", errContains: "invalid --spanner-metrics-endpoint"},
		{desc: "userinfo rejected", exporter: "otlp", endpoint: "http://user:pass@127.0.0.1:4318/v1/metrics", errContains: "userinfo"},
		{desc: "query rejected", exporter: "otlp", endpoint: "http://127.0.0.1:4318/v1/metrics?foo=1", errContains: "query"},
		{desc: "fragment rejected", exporter: "otlp", endpoint: "http://127.0.0.1:4318/v1/metrics#frag", errContains: "fragment"},
		{desc: "unknown exporter rejected", exporter: "prometheus", endpoint: "", errContains: "must be off or otlp"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			gotExp, gotEnd, err := normalizeSpannerMetricsOptions(tt.exporter, tt.endpoint)
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %v, want containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotExp != tt.wantExporter || gotEnd != tt.wantEndpoint {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotExp, gotEnd, tt.wantExporter, tt.wantEndpoint)
			}
		})
	}
}

func TestValidateSpannerOptionsMetricsCombos(t *testing.T) {
	t.Parallel()
	err := ValidateSpannerOptions(&spannerOptions{
		ProjectId:              "p",
		InstanceId:             "i",
		DatabaseId:             "d",
		SpannerMetricsExporter: "otlp",
	})
	if err == nil || !strings.Contains(err.Error(), "requires --spanner-metrics-endpoint") {
		t.Fatalf("otlp without endpoint: %v", err)
	}
	err = ValidateSpannerOptions(&spannerOptions{
		ProjectId:              "p",
		InstanceId:             "i",
		DatabaseId:             "d",
		SpannerMetricsExporter: "off",
		SpannerMetricsEndpoint: "http://127.0.0.1:4318/v1/metrics",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid combination") {
		t.Fatalf("off plus endpoint: %v", err)
	}
	err = ValidateSpannerOptions(&spannerOptions{
		ProjectId:              "p",
		InstanceId:             "i",
		DatabaseId:             "d",
		SpannerMetricsExporter: "otlp",
		SpannerMetricsEndpoint: "http://:4318/v1/metrics",
	})
	if err == nil || !strings.Contains(err.Error(), "absolute http or https") {
		t.Fatalf("port-only host: %v", err)
	}
}

func TestParseFlagsSpannerMetrics(t *testing.T) {
	t.Parallel()
	gopts, err := parseTestFlags(withRequiredFlags(
		"--spanner-metrics-exporter=otlp",
		"--spanner-metrics-endpoint=http://127.0.0.1:4318",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gopts.Spanner.SpannerMetricsExporter != "otlp" {
		t.Fatalf("exporter = %q", gopts.Spanner.SpannerMetricsExporter)
	}
	if gopts.Spanner.SpannerMetricsEndpoint != "http://127.0.0.1:4318" {
		t.Fatalf("endpoint = %q", gopts.Spanner.SpannerMetricsEndpoint)
	}

	_, err = parseTestFlags(withRequiredFlags("--spanner-metrics-exporter=jaeger"))
	if err == nil {
		t.Fatal("expected Kong enum rejection for jaeger")
	}
}

func TestInitializeSystemVariablesAppliesMetricsFlags(t *testing.T) {
	t.Parallel()
	sysVars, err := initializeSystemVariables(&spannerOptions{
		ProjectId:              "p",
		InstanceId:             "i",
		DatabaseId:             "d",
		SpannerMetricsExporter: "otlp",
		SpannerMetricsEndpoint: "http://127.0.0.1:4318",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sysVars.Config.SpannerMetricsExporter != "otlp" {
		t.Fatalf("exporter = %q", sysVars.Config.SpannerMetricsExporter)
	}
	if sysVars.Config.SpannerMetricsEndpoint != "http://127.0.0.1:4318/v1/metrics" {
		t.Fatalf("endpoint = %q", sysVars.Config.SpannerMetricsEndpoint)
	}

	offVars, err := initializeSystemVariables(&spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if offVars.Config.SpannerMetricsExporter != "off" || offVars.Config.SpannerMetricsEndpoint != "" {
		t.Fatalf("default exporter/endpoint = %q / %q", offVars.Config.SpannerMetricsExporter, offVars.Config.SpannerMetricsEndpoint)
	}
}

func TestSpannerMetricsResetRejected(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	if err := sysVars.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"CLI_SPANNER_METRICS_EXPORTER", "CLI_SPANNER_METRICS_ENDPOINT"} {
		if err := sysVars.Reset(name); err == nil || !strings.Contains(err.Error(), "does not support RESET") {
			t.Fatalf("RESET %s: %v", name, err)
		}
	}
}

func TestStartSpannerMetricsOffLeavesInjectedProvider(t *testing.T) {
	sink := newOTLPSink()
	srv := httptest.NewServer(sink)
	t.Cleanup(srv.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", srv.URL+"/v1/metrics")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")

	_, injected := newCallerMetricsProvider()
	t.Cleanup(func() { _ = injected.Shutdown(context.Background()) })

	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Config.SpannerMetricsExporter = spannerMetricsExporterOff
	sysVars.Config.ClientMetricsProvider = injected
	beforeGlobal := otel.GetMeterProvider()

	owner, err := startSpannerMetrics(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	if owner.provider != nil {
		t.Fatal("off mode constructed a MeterProvider")
	}
	if sysVars.Config.ClientMetricsProvider != injected {
		t.Fatal("off mode erased injected ClientMetricsProvider")
	}
	if otel.GetMeterProvider() != beforeGlobal {
		t.Fatal("off mode changed the global MeterProvider")
	}
	if got := sink.count(); got != 0 {
		t.Fatalf("disabled mode exported %d HTTP posts", got)
	}
}

func TestOverlayClientMetricsProviderCopiesEmbed(t *testing.T) {
	t.Parallel()
	_, owned := newCallerMetricsProvider()
	t.Cleanup(func() { _ = owned.Shutdown(context.Background()) })
	_, preexisting := newCallerMetricsProvider()
	t.Cleanup(func() { _ = preexisting.Shutdown(context.Background()) })

	embed := &spanner.ClientConfig{
		DisableNativeMetrics:  true,
		Type:                  spanner.OMNI,
		DisableRouteToLeader:  true,
		UserAgent:             "embedded-omni-metrics",
		ClientMetricsProvider: preexisting,
	}
	sysVars := &systemVariables{
		Connection: ConnectionVars{Project: "p", Instance: "i", Database: "d", Role: "role"},
		Config: StartupConfig{
			EmbeddedClientConfig:  embed,
			ClientMetricsProvider: owned,
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
	if gotConfig.ClientMetricsProvider != owned {
		t.Fatal("copied config did not receive the process-owned provider")
	}
	if !gotConfig.DisableNativeMetrics {
		t.Fatal("overlay cleared DisableNativeMetrics")
	}
	if gotConfig.Type != spanner.OMNI || !gotConfig.DisableRouteToLeader || gotConfig.UserAgent != "embedded-omni-metrics" {
		t.Fatalf("embed fields not preserved: %+v", gotConfig)
	}
	if embed != sysVars.Config.EmbeddedClientConfig {
		t.Fatal("EmbeddedClientConfig pointer replaced")
	}
	if embed.ClientMetricsProvider != preexisting || embed.Type != spanner.OMNI || embed.UserAgent != "embedded-omni-metrics" || !embed.DisableRouteToLeader {
		t.Fatal("source EmbeddedClientConfig mutated")
	}
	if session.clientConfig.ClientMetricsProvider != owned {
		t.Fatal("session clientConfig lost owned provider")
	}
}

func TestFakeRPCCallerMetricsAndReplacement(t *testing.T) {
	clearSpannerEmulatorHost(t)
	host, port, stop := startFakeMetricsSpanner(t)
	t.Cleanup(stop)

	reader, provider := newCallerMetricsProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	sysVars := newMetricsTestSysVars(host, port)
	sysVars.Config.ClientMetricsProvider = provider

	session, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	if session.clientConfig.ClientMetricsProvider != provider {
		t.Fatal("first session did not overlay the owned provider")
	}
	runFakeMetricsSelect(t, session.client)

	oldClient := session.client
	if err := session.RecreateClient(context.Background()); err != nil {
		t.Fatalf("RecreateClient: %v", err)
	}
	if session.client == oldClient {
		t.Fatal("RecreateClient did not replace the data client")
	}
	if session.clientConfig.ClientMetricsProvider != provider {
		t.Fatal("RecreateClient changed provider identity")
	}
	runFakeMetricsSelect(t, session.client)

	session.Close()
	session.Close()
	closeCliClients(&Cli{SessionHandler: NewSessionHandler(session)})

	replacement, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatalf("replacement createSession: %v", err)
	}
	if replacement.clientConfig.ClientMetricsProvider != provider {
		t.Fatal("USE-style reconstruct changed provider identity")
	}
	runFakeMetricsSelect(t, replacement.client)
	replacement.Close()

	rm := collectCallerMetrics(t, reader)
	names := callerMetricNames(rm)
	foundOp := false
	for _, name := range names {
		if name == "spanner/client/operation_count" {
			foundOp = true
		}
		if strings.HasPrefix(name, "spanner.googleapis.com/internal/client/") {
			t.Fatalf("native Cloud Monitoring name %q leaked; names=%v", name, names)
		}
	}
	if !foundOp {
		t.Fatalf("missing spanner/client/operation_count; got %v", names)
	}
	keys, values := callerMetricAttrKeysAndValues(rm)
	for _, k := range keys {
		lower := strings.ToLower(k)
		if k == "db.statement" || k == "sql" || strings.Contains(lower, "param") {
			t.Fatalf("sensitive metric attribute key %q; keys=%v", k, keys)
		}
	}
	for _, v := range values {
		if strings.Contains(v, metricsTestSQL) || strings.Contains(v, "secret_col") || strings.Contains(v, "@p") {
			t.Fatalf("SQL/parameter text in metric values: %v", values)
		}
	}

	counter, err := provider.Meter("replacement-proof").Int64Counter("after_session_close")
	if err != nil {
		t.Fatalf("meter after Session.Close: %v", err)
	}
	counter.Add(context.Background(), 1)
	rm = collectCallerMetrics(t, reader)
	found := false
	for _, name := range callerMetricNames(rm) {
		if name == "after_session_close" {
			found = true
		}
	}
	if !found {
		t.Fatal("Session.Close shut down the caller-owned provider")
	}
}

func TestEmulatorHostSuppressesCallerMetrics(t *testing.T) {
	host, port, stop := startFakeMetricsSpanner(t)
	t.Cleanup(stop)
	t.Setenv("SPANNER_EMULATOR_HOST", net.JoinHostPort(host, strconv.Itoa(port)))

	reader, provider := newCallerMetricsProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	sysVars := newMetricsTestSysVars(host, port)
	sysVars.Config.ClientMetricsProvider = provider

	session, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	t.Cleanup(session.Close)
	runFakeMetricsSelect(t, session.client)
	rm := collectCallerMetrics(t, reader)
	if len(rm.ScopeMetrics) != 0 {
		t.Fatalf("SPANNER_EMULATOR_HOST still recorded metrics: %v", callerMetricNames(rm))
	}
}

func TestFakeHTTPOTLPExportAndCLIURLWins(t *testing.T) {
	clearSpannerEmulatorHost(t)
	host, port, stop := startFakeMetricsSpanner(t)
	t.Cleanup(stop)

	wantPath := "/custom/otlp/metrics"
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
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", decoySrv.URL+"/v1/metrics")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "50")

	sysVars := newMetricsTestSysVars(host, port)
	sysVars.Config.SpannerMetricsExporter = spannerMetricsExporterOTLP
	canonical, err := parseSpannerMetricsEndpoint(collector.URL + wantPath)
	if err != nil {
		t.Fatal(err)
	}
	sysVars.Config.SpannerMetricsEndpoint = canonical

	beforeGlobal := otel.GetMeterProvider()
	owner, err := startSpannerMetrics(sysVars)
	if err != nil {
		t.Fatal(err)
	}
	if owner.provider == nil {
		t.Fatal("otlp mode did not construct a provider")
	}
	if sysVars.Config.ClientMetricsProvider != owner.provider {
		t.Fatal("owned provider was not stored on StartupConfig")
	}
	if otel.GetMeterProvider() != beforeGlobal {
		t.Fatal("startSpannerMetrics installed a global MeterProvider")
	}

	session, err := createSession(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	runFakeMetricsSelect(t, session.client)

	cmdCtx, cancel := context.WithCancel(context.Background())
	cancel()
	closeCliClients(&Cli{SessionHandler: NewSessionHandler(session)})
	var stderr bytes.Buffer
	started := time.Now()
	if err := owner.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if elapsed := time.Since(started); elapsed > spannerMetricsCleanupBound {
		t.Fatalf("Shutdown used %s, bound is %s; cancelled ctx = %v", elapsed, spannerMetricsCleanupBound, cmdCtx.Err())
	}
	if stderr.Len() != 0 {
		t.Fatalf("shutdown diagnostics: %s", stderr.String())
	}

	posts := sink.snapshot()
	if len(posts) == 0 {
		t.Fatal("CLI collector received no OTLP HTTP posts")
	}
	for _, p := range posts {
		if p.path != wantPath {
			t.Fatalf("path = %q, want %q", p.path, wantPath)
		}
		if p.method != http.MethodPost {
			t.Fatalf("method = %q", p.method)
		}
		if !strings.Contains(p.contentType, "protobuf") {
			t.Fatalf("content-type = %q, want protobuf", p.contentType)
		}
		requireOTLPMetricsRequest(t, p.body)
	}
	if decoy.count() != 0 {
		t.Fatalf("conflicting OTEL endpoint received %d posts; CLI URL must win", decoy.count())
	}

	after := sink.count()
	time.Sleep(200 * time.Millisecond)
	if sink.count() != after {
		t.Fatal("Shutdown did not stop the PeriodicReader")
	}
	_ = owner.Shutdown(context.Background())
}

func TestCloseCliClientsNilSafe(t *testing.T) {
	t.Parallel()
	closeCliClients(nil)
	closeCliClients(&Cli{})
	releaseOwnedClientsAndMetrics(nil, nil, io.Discard)
	_ = (*metricsOwner)(nil).Shutdown(context.Background())
	_ = (&metricsOwner{}).Shutdown(context.Background())
}

func requireOTLPMetricsRequest(t *testing.T, body []byte) *colmetricpb.ExportMetricsServiceRequest {
	t.Helper()
	var req colmetricpb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal ExportMetricsServiceRequest: %v", err)
	}
	var names []string
	found := false
	for _, rm := range req.GetResourceMetrics() {
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				names = append(names, m.GetName())
				if strings.HasPrefix(m.GetName(), "spanner/client") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("ExportMetricsServiceRequest has no spanner/client metric; names=%v", names)
	}
	if bytes.Contains(body, []byte(metricsTestSQL)) || bytes.Contains(body, []byte("secret_col")) {
		t.Fatal("encoded export contained SQL text")
	}
	return &req
}

type otlpPost struct {
	method      string
	path        string
	contentType string
	encoding    string
	body        []byte
}

type otlpSink struct {
	mu    sync.Mutex
	posts []otlpPost
}

func newOTLPSink() *otlpSink { return &otlpSink{} }

func (s *otlpSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.posts = append(s.posts, otlpPost{
		method:      r.Method,
		path:        r.URL.Path,
		contentType: r.Header.Get("Content-Type"),
		encoding:    r.Header.Get("Content-Encoding"),
		body:        body,
	})
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

func (s *otlpSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts)
}

func (s *otlpSink) snapshot() []otlpPost {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]otlpPost, len(s.posts))
	copy(out, s.posts)
	return out
}
