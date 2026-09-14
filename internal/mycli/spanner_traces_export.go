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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	spannerTracesScopeName    = "cloud.google.com/go/spanner"
	spannerTracesScopeVersion = "1.95.0" // pin: cloud.google.com/go/spanner in go.mod
	spannerTracesServiceName  = "spanner-mycli"
)

// Known startSpan names after prependPackageName in cloud.google.com/go/spanner
// v1.95.0 (client.go, sessionclient.go, transaction.go, read.go, pdml.go).
// Review this allowlist on every go-spanner upgrade. Unknown names are dropped.
var spannerTracesAllowedNames = map[string]string{
	"cloud.google.com/go/spanner.NewClient":                       "NewClient",
	"cloud.google.com/go/spanner.CreateSession":                   "CreateSession",
	"cloud.google.com/go/spanner.Read":                            "Read",
	"cloud.google.com/go/spanner.Query":                           "Query",
	"cloud.google.com/go/spanner.Update":                          "Update",
	"cloud.google.com/go/spanner.BatchUpdate":                     "BatchUpdate",
	"cloud.google.com/go/spanner.RowIterator":                     "RowIterator",
	"cloud.google.com/go/spanner.PartitionedUpdate":               "PartitionedUpdate",
	"cloud.google.com/go/spanner.ReadWriteTransaction":            "ReadWriteTransaction",
	"cloud.google.com/go/spanner.ReadWriteTransactionWithOptions": "ReadWriteTransactionWithOptions",
	"cloud.google.com/go/spanner.Apply":                           "Apply",
	"cloud.google.com/go/spanner.BatchWrite":                      "BatchWrite",
	"cloud.google.com/go/spanner.BatchWriteResponseIterator":      "BatchWriteResponseIterator",
}

var (
	spannerTracesFixedResource = sdkresource.NewSchemaless(attribute.String("service.name", spannerTracesServiceName))
	spannerTracesFixedScope    = instrumentation.Scope{Name: spannerTracesScopeName, Version: spannerTracesScopeVersion}
)

// sanitizedReadOnlySpan embeds the sealed SDK interface so private() is
// supplied by the embedded value. Every string/privacy carrier that
// otlptrace tracetransform reads must be overridden here.
type sanitizedReadOnlySpan struct {
	sdktrace.ReadOnlySpan
	name string
}

var _ sdktrace.ReadOnlySpan = sanitizedReadOnlySpan{}

func (s sanitizedReadOnlySpan) Name() string { return s.name }

func (s sanitizedReadOnlySpan) SpanContext() trace.SpanContext {
	return stripTraceState(s.ReadOnlySpan.SpanContext())
}

func (s sanitizedReadOnlySpan) Parent() trace.SpanContext {
	return stripTraceState(s.ReadOnlySpan.Parent())
}

func (s sanitizedReadOnlySpan) Attributes() []attribute.KeyValue { return nil }

func (s sanitizedReadOnlySpan) Links() []sdktrace.Link { return nil }

func (s sanitizedReadOnlySpan) Events() []sdktrace.Event { return nil }

func (s sanitizedReadOnlySpan) Status() sdktrace.Status {
	return sdktrace.Status{Code: s.ReadOnlySpan.Status().Code}
}

func (s sanitizedReadOnlySpan) InstrumentationScope() instrumentation.Scope {
	return spannerTracesFixedScope
}

// InstrumentationLibrary remains a privacy carrier in otlptrace v1.44.0.
//
//nolint:staticcheck // SA1019: override the deprecated getter the exporter still reads
func (s sanitizedReadOnlySpan) InstrumentationLibrary() instrumentation.Library {
	return spannerTracesFixedScope
}

func (s sanitizedReadOnlySpan) Resource() *sdkresource.Resource {
	return spannerTracesFixedResource
}

func (s sanitizedReadOnlySpan) DroppedAttributes() int { return 0 }

func (s sanitizedReadOnlySpan) DroppedLinks() int { return 0 }

func (s sanitizedReadOnlySpan) DroppedEvents() int { return 0 }

func stripTraceState(sc trace.SpanContext) trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    sc.TraceID(),
		SpanID:     sc.SpanID(),
		TraceFlags: sc.TraceFlags(),
		Remote:     sc.IsRemote(),
	})
}

// sanitizingSpanExporter is a small concrete wrapper around the official
// OTLP HTTP exporter. It does not implement HTTP, protobuf conversion, or retry.
type sanitizingSpanExporter struct {
	next sdktrace.SpanExporter
}

var _ sdktrace.SpanExporter = (*sanitizingSpanExporter)(nil)

func (e *sanitizingSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if e.next == nil {
		return nil
	}
	out := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, s := range spans {
		if s == nil || s.InstrumentationScope().Name != spannerTracesScopeName {
			continue
		}
		short, ok := spannerTracesAllowedNames[s.Name()]
		if !ok {
			continue
		}
		out = append(out, sanitizedReadOnlySpan{ReadOnlySpan: s, name: short})
	}
	if len(out) == 0 {
		return nil
	}
	return e.next.ExportSpans(ctx, out)
}

func (e *sanitizingSpanExporter) Shutdown(ctx context.Context) error {
	if e.next == nil {
		return nil
	}
	return e.next.Shutdown(ctx)
}
