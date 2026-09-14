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
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync/atomic"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	spannerTracesExporterOff        = "off"
	spannerTracesExporterOTLP       = "otlp"
	spannerTracesDefaultPath        = "/v1/traces"
	spannerTracesDefaultSampleRatio = 0.01
)

var errTracesOwnerExists = errors.New("spanner traces owner already installed")

// tracesOwner is the process-level CLI-owned TracerProvider. It is created
// once in runWithOutput and must not be shut down by Session.Close, USE/DETACH,
// or RecreateClient. This is not a private caller-owned tracing API: the
// pinned Go SDK uses the process-global provider.
type tracesOwner struct {
	provider *sdktrace.TracerProvider
	prev     trace.TracerProvider
}

var installedTraces atomic.Pointer[tracesOwner]

func normalizeSpannerTracesOptions(exporter, endpoint string, ratio float64) (string, string, float64, error) {
	exp := strings.ToLower(strings.TrimSpace(exporter))
	if exp == "" {
		exp = spannerTracesExporterOff
	}
	end := strings.TrimSpace(endpoint)
	switch exp {
	case spannerTracesExporterOff:
		if end != "" {
			return "", "", 0, fmt.Errorf("invalid combination: --spanner-traces-exporter=off cannot be used with --spanner-traces-endpoint")
		}
		return spannerTracesExporterOff, "", spannerTracesDefaultSampleRatio, nil
	case spannerTracesExporterOTLP:
		if end == "" {
			return "", "", 0, fmt.Errorf("--spanner-traces-exporter=otlp requires --spanner-traces-endpoint")
		}
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
			return "", "", 0, fmt.Errorf("invalid --spanner-traces-sample-ratio %v: must be in [0,1]", ratio)
		}
		canonical, err := parseSpannerTracesEndpoint(end)
		if err != nil {
			return "", "", 0, err
		}
		return spannerTracesExporterOTLP, canonical, ratio, nil
	default:
		return "", "", 0, fmt.Errorf("invalid --spanner-traces-exporter %q: must be off or otlp", exporter)
	}
}

func parseSpannerTracesEndpoint(raw string) (string, error) {
	return parseOTLPEndpoint(raw, "--spanner-traces-endpoint", spannerTracesDefaultPath)
}

func applySpannerTracesOptions(sysVars *systemVariables, opts *spannerOptions) error {
	ratio := opts.SpannerTracesSampleRatio
	if ratio == 0 && opts.SpannerTracesExporter != spannerTracesExporterOTLP {
		ratio = spannerTracesDefaultSampleRatio
	}
	exporter, endpoint, ratio, err := normalizeSpannerTracesOptions(opts.SpannerTracesExporter, opts.SpannerTracesEndpoint, ratio)
	if err != nil {
		return err
	}
	sysVars.Config.SpannerTracesExporter = exporter
	sysVars.Config.SpannerTracesEndpoint = endpoint
	sysVars.Config.SpannerTracesSampleRatio = ratio
	return nil
}

// startSpannerTraces installs the process-owned TracerProvider when opted in.
// Off mode constructs nothing and does not call SetTracerProvider or mutate
// environment variables. The SDK may still honor SPANNER_ENABLE_END_TO_END_TRACING
// independently. It never installs a global MeterProvider.
func startSpannerTraces(sysVars *systemVariables) (*tracesOwner, error) {
	if sysVars == nil || sysVars.Config.SpannerTracesExporter != spannerTracesExporterOTLP {
		return &tracesOwner{}, nil
	}
	if installedTraces.Load() != nil {
		return nil, errTracesOwnerExists
	}

	ctx, cancel := context.WithTimeout(context.Background(), spannerTelemetryCleanupBound)
	defer cancel()
	next, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(sysVars.Config.SpannerTracesEndpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to create Spanner traces exporter: %w", err)
	}
	wrap := &sanitizingSpanExporter{next: next}
	ratio := sysVars.Config.SpannerTracesSampleRatio
	// Batcher (not Syncer): End() must not block on a stalled collector.
	// Shutdown/ForceFlush is the bounded export path, matching metrics.
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(wrap, sdktrace.WithExportTimeout(spannerTelemetryCleanupBound)),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	owner, err := installTracesOwner(provider)
	if err != nil {
		_ = provider.Shutdown(ctx)
		return nil, err
	}
	return owner, nil
}

func installTracesOwner(provider *sdktrace.TracerProvider) (*tracesOwner, error) {
	if installedTraces.Load() != nil {
		return nil, errTracesOwnerExists
	}
	prev := otel.GetTracerProvider()
	o := &tracesOwner{provider: provider, prev: prev}
	if !installedTraces.CompareAndSwap(nil, o) {
		return nil, errTracesOwnerExists
	}
	otel.SetTracerProvider(provider)
	return o, nil
}

func (o *tracesOwner) restoreGlobal() {
	if o == nil || o.provider == nil {
		return
	}
	if otel.GetTracerProvider() != o.provider {
		return
	}
	// The pinned OTel v1.44.0 initial proxy delegates irreversibly on the
	// first SetTracerProvider. Reinstalling that proxy is not exact
	// restoration. A narrow no-op substitution stops new CLI recording after
	// shutdown. Tracers obtained from the proxy before our Set keep
	// delegating; that composition is unsupported.
	if isPinnedInitialProxy(o.prev) {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return
	}
	otel.SetTracerProvider(o.prev)
}

func (o *tracesOwner) Shutdown(ctx context.Context) error {
	if o == nil || o.provider == nil {
		return nil
	}
	err := o.provider.Shutdown(ctx)
	o.restoreGlobal()
	installedTraces.CompareAndSwap(o, nil)
	o.provider = nil
	return err
}

func overlayEndToEndTracing(cfg *spanner.ClientConfig, sysVars *systemVariables) {
	if sysVars == nil || sysVars.Config.SpannerTracesExporter != spannerTracesExporterOTLP {
		return
	}
	cfg.EnableEndToEndTracing = true
}

func isPinnedInitialProxy(tp trace.TracerProvider) bool {
	if tp == nil {
		return false
	}
	rt := reflect.TypeOf(tp)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	return rt.PkgPath() == "go.opentelemetry.io/otel/internal/global" && rt.Name() == "tracerProvider"
}
