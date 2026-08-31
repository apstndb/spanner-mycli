//
// Copyright 2020 Google LLC
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
//

package mycli

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
)

// testcontainersSlogLogger routes Testcontainers lifecycle diagnostics through
// the CLI logger. Testcontainers does not attach levels to these messages, so
// treat them as informational: the default WARN level stays quiet, while
// --log-level=INFO or DEBUG makes them visible.
type testcontainersSlogLogger struct {
	logger *slog.Logger
}

func (l testcontainersSlogLogger) Printf(format string, args ...any) {
	if l.logger == nil || !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}

	message := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	l.logger.Info("testcontainers", "message", message)
}

func configureTestcontainersLogger(logger *slog.Logger) testcontainers.ContainerCustomizer {
	adapter := testcontainersSlogLogger{logger: logger}
	// Docker discovery and some wait strategies log through the package-global
	// logger instead of the per-container logger, so both must be configured.
	tclog.SetDefault(adapter)
	return testcontainers.WithLogger(adapter)
}

// withPlatform creates a ContainerCustomizer that sets the container platform
func withPlatform(platform string) testcontainers.ContainerCustomizer {
	return testcontainers.CustomizeRequest(
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				ImagePlatform: platform,
			},
		},
	)
}
