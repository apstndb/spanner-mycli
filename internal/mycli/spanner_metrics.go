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
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	spannerMetricsExporterOff    = "off"
	spannerMetricsExporterOTLP   = "otlp"
	spannerMetricsDefaultPath    = "/v1/metrics"
	spannerTelemetryCleanupBound = 5 * time.Second
	spannerMetricsCleanupBound   = spannerTelemetryCleanupBound
)

// metricsOwner is the process-level caller-owned Spanner client-metrics
// pipeline. It is created once in runWithOutput and must not be shut down by
// Session.Close, USE/DETACH, or RecreateClient.
type metricsOwner struct {
	provider *sdkmetric.MeterProvider
}

func normalizeSpannerMetricsOptions(exporter, endpoint string) (string, string, error) {
	exp := strings.ToLower(strings.TrimSpace(exporter))
	if exp == "" {
		exp = spannerMetricsExporterOff
	}
	end := strings.TrimSpace(endpoint)
	switch exp {
	case spannerMetricsExporterOff:
		if end != "" {
			return "", "", fmt.Errorf("invalid combination: --spanner-metrics-exporter=off cannot be used with --spanner-metrics-endpoint")
		}
		return spannerMetricsExporterOff, "", nil
	case spannerMetricsExporterOTLP:
		if end == "" {
			return "", "", fmt.Errorf("--spanner-metrics-exporter=otlp requires --spanner-metrics-endpoint")
		}
		canonical, err := parseSpannerMetricsEndpoint(end)
		if err != nil {
			return "", "", err
		}
		return spannerMetricsExporterOTLP, canonical, nil
	default:
		return "", "", fmt.Errorf("invalid --spanner-metrics-exporter %q: must be off or otlp", exporter)
	}
}

func parseOTLPEndpoint(raw, flagName, defaultPath string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid %s %q: %w", flagName, raw, err)
	}
	if !u.IsAbs() || u.Opaque != "" || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid %s %q: must be an absolute http or https URL with a host", flagName, raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid %s %q: userinfo, query, and fragment are not allowed", flagName, raw)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = defaultPath
	}
	return u.String(), nil
}

func parseSpannerMetricsEndpoint(raw string) (string, error) {
	return parseOTLPEndpoint(raw, "--spanner-metrics-endpoint", spannerMetricsDefaultPath)
}

func applySpannerMetricsOptions(sysVars *systemVariables, opts *spannerOptions) error {
	exporter, endpoint, err := normalizeSpannerMetricsOptions(opts.SpannerMetricsExporter, opts.SpannerMetricsEndpoint)
	if err != nil {
		return err
	}
	sysVars.Config.SpannerMetricsExporter = exporter
	sysVars.Config.SpannerMetricsEndpoint = endpoint
	return nil
}

// startSpannerMetrics builds the dedicated MeterProvider when opted in.
// Off mode constructs nothing and leaves any preexisting injected provider
// (tests / embed) untouched. It never installs a global MeterProvider.
func startSpannerMetrics(sysVars *systemVariables) (*metricsOwner, error) {
	if sysVars == nil || sysVars.Config.SpannerMetricsExporter != spannerMetricsExporterOTLP {
		return &metricsOwner{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), spannerTelemetryCleanupBound)
	defer cancel()
	exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(sysVars.Config.SpannerMetricsEndpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to create Spanner metrics exporter: %w", err)
	}
	reader := sdkmetric.NewPeriodicReader(exporter)
	opts := append(spanner.ClientMetricsMeterProviderOptions(), sdkmetric.WithReader(reader))
	provider := sdkmetric.NewMeterProvider(opts...)
	sysVars.Config.ClientMetricsProvider = provider
	return &metricsOwner{provider: provider}, nil
}

// Shutdown flushes and stops the owned reader/provider. The caller supplies
// the shared cleanup context and writes any combined diagnostic.
func (o *metricsOwner) Shutdown(ctx context.Context) error {
	if o == nil || o.provider == nil {
		return nil
	}
	err := o.provider.Shutdown(ctx)
	o.provider = nil
	return err
}

func closeCliClients(cli *Cli) {
	if cli != nil && cli.SessionHandler != nil {
		cli.SessionHandler.Close()
	}
}

// metricsRunTestHooks is a process-local test seam for runWithOutput.
// Production leaves the pointer nil. Tests store and clear it with
// atomic.Pointer so parallel package tests do not race on the hook.
type metricsRunTestHooks struct {
	errStream io.Writer
	adjust    func(*systemVariables)
}

var metricsRunTest atomic.Pointer[metricsRunTestHooks]

func applyMetricsRunTestSysVars(sysVars *systemVariables) {
	if hooks := metricsRunTest.Load(); hooks != nil && hooks.adjust != nil {
		hooks.adjust(sysVars)
	}
}

func runWithOutputErrStream(fallback io.Writer) io.Writer {
	if hooks := metricsRunTest.Load(); hooks != nil && hooks.errStream != nil {
		return hooks.errStream
	}
	return fallback
}

// releaseOwnedClientsAndMetrics keeps the metrics-only cleanup entry point.
func releaseOwnedClientsAndMetrics(cli *Cli, metrics *metricsOwner, errw io.Writer) {
	releaseOwnedClientsAndTelemetry(cli, metrics, nil, errw)
}

// releaseOwnedClientsAndTelemetry closes owned CLI clients first, then
// flushes/stops enabled metrics and traces against one fresh five-second
// budget. Both Shutdown calls run even if the first exhausts the context.
// Diagnostics go to errw and must not replace the caller's original result.
func releaseOwnedClientsAndTelemetry(cli *Cli, metrics *metricsOwner, traces *tracesOwner, errw io.Writer) {
	closeCliClients(cli)
	ctx, cancel := context.WithTimeout(context.Background(), spannerTelemetryCleanupBound)
	defer cancel()
	var errs []error
	if err := metrics.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := traces.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 && errw != nil {
		fmt.Fprintf(errw, "WARNING: Spanner telemetry shutdown failed: %v\n", errors.Join(errs...))
	}
}

func overlayClientMetricsProvider(cfg *spanner.ClientConfig, sysVars *systemVariables) {
	if sysVars == nil || sysVars.Config.ClientMetricsProvider == nil {
		return
	}
	cfg.ClientMetricsProvider = sysVars.Config.ClientMetricsProvider
	cfg.DisableNativeMetrics = true
}
