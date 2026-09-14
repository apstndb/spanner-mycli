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
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	spannerMetricsExporterOff  = "off"
	spannerMetricsExporterOTLP = "otlp"
	spannerMetricsDefaultPath  = "/v1/metrics"
	spannerMetricsCleanupBound = 5 * time.Second
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

func parseSpannerMetricsEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid --spanner-metrics-endpoint %q: %w", raw, err)
	}
	if !u.IsAbs() || u.Opaque != "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid --spanner-metrics-endpoint %q: must be an absolute http or https URL with a host", raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid --spanner-metrics-endpoint %q: userinfo, query, and fragment are not allowed", raw)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = spannerMetricsDefaultPath
	}
	return u.String(), nil
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

	ctx, cancel := context.WithTimeout(context.Background(), spannerMetricsCleanupBound)
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

// Shutdown flushes and stops the owned reader/provider using one fresh
// five-second budget. It is independent of the cancelled command context.
// Diagnostics go to errw; the caller keeps the original exit result.
func (o *metricsOwner) Shutdown(errw io.Writer) {
	if o == nil || o.provider == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), spannerMetricsCleanupBound)
	defer cancel()
	if err := o.provider.Shutdown(ctx); err != nil && errw != nil {
		fmt.Fprintf(errw, "WARNING: Spanner metrics shutdown failed: %v\n", err)
	}
	o.provider = nil
}

func closeCliClients(cli *Cli) {
	if cli != nil && cli.SessionHandler != nil {
		cli.SessionHandler.Close()
	}
}

func overlayClientMetricsProvider(cfg *spanner.ClientConfig, sysVars *systemVariables) {
	if sysVars == nil || sysVars.Config.ClientMetricsProvider == nil {
		return
	}
	cfg.ClientMetricsProvider = sysVars.Config.ClientMetricsProvider
	cfg.DisableNativeMetrics = true
}
