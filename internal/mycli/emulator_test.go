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
	"log/slog"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
)

func TestTestcontainersSlogLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		level      slog.Level
		wantOutput bool
	}{
		{name: "warn is quiet", level: slog.LevelWarn},
		{name: "info is visible", level: slog.LevelInfo, wantOutput: true},
		{name: "debug is visible", level: slog.LevelDebug, wantOutput: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: test.level}))
			testcontainersSlogLogger{logger: logger}.Printf("container %s started\n", "example")

			got := output.String()
			if !test.wantOutput {
				if got != "" {
					t.Fatalf("Printf() output = %q, want no output", got)
				}
				return
			}
			if !strings.Contains(got, "level=INFO") ||
				!strings.Contains(got, "msg=testcontainers") ||
				!strings.Contains(got, `message="container example started"`) {
				t.Fatalf("Printf() output = %q, want structured informational log", got)
			}
		})
	}
}

func TestConfigureTestcontainersLogger(t *testing.T) {
	previousLogger := tclog.Default()
	t.Cleanup(func() {
		tclog.SetDefault(previousLogger)
	})

	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo}))
	customizer := configureTestcontainersLogger(logger)

	tclog.Printf("global %s", "message")
	req := testcontainers.GenericContainerRequest{}
	if err := customizer.Customize(&req); err != nil {
		t.Fatalf("Customize() error = %v", err)
	}
	if req.Logger == nil {
		t.Fatal("Customize() did not configure the per-container logger")
	}
	req.Logger.Printf("container %s", "message")

	got := output.String()
	for _, want := range []string{"global message", "container message"} {
		if !strings.Contains(got, want) {
			t.Errorf("logger output = %q, want %q", got, want)
		}
	}
}
