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
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// shutdownErrExporter is a SpanExporter whose Shutdown returns a fixed error.
// ExportSpans is a no-op so this path does not depend on batch-processor
// export-timeout versus cleanup-budget scheduling, and does not leave export
// work running after the test returns.
type shutdownErrExporter struct {
	err            error
	shutdownCalled atomic.Bool
}

var _ sdktrace.SpanExporter = (*shutdownErrExporter)(nil)

func (e *shutdownErrExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *shutdownErrExporter) Shutdown(context.Context) error {
	e.shutdownCalled.Store(true)
	return e.err
}

func TestReleaseOwnedClientsAndTelemetryShutdownDiagnostic(t *testing.T) {
	sentinel := errors.New("issue-1036 exporter stop failed")
	for _, tc := range []struct {
		name        string
		shutdownErr error
		wantWarning bool
	}{
		{name: "sentinel", shutdownErr: sentinel, wantWarning: true},
		{name: "nil", shutdownErr: nil, wantWarning: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoreTestTracerProvider(t)
			if installedTraces.Load() != nil {
				t.Fatal("installedTraces already set")
			}

			exp := &shutdownErrExporter{err: tc.shutdownErr}
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
			owner, err := installTracesOwner(provider)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if o := installedTraces.Load(); o != nil {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					_ = o.Shutdown(ctx)
				}
			})

			_, span := provider.Tracer("telemetry-shutdown-diagnostic").Start(context.Background(), "probe")
			span.End()

			var buf bytes.Buffer
			releaseOwnedClientsAndTelemetry(nil, &metricsOwner{}, owner, &buf)
			if !exp.shutdownCalled.Load() {
				t.Fatal("exporter.Shutdown was not called")
			}

			stderr := buf.String()
			n := strings.Count(stderr, telemetryShutdownWarning)
			if tc.wantWarning {
				if n != 1 {
					t.Fatalf("shutdown diagnostics = %d, want 1; stderr=%q", n, stderr)
				}
				if !strings.Contains(stderr, tc.shutdownErr.Error()) {
					t.Fatalf("diagnostic missing shutdown cause %q; stderr=%q", tc.shutdownErr.Error(), stderr)
				}
				return
			}
			if n != 0 {
				t.Fatalf("shutdown diagnostics = %d, want 0; stderr=%q", n, stderr)
			}
		})
	}
}
