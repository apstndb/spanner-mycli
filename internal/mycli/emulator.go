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
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
)

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

// detectContainerPlatform detects the platform of a running container
// It tries multiple methods in order of preference:
// 1. ImageManifestDescriptor.Platform (most accurate but not always available)
// 2. Docker API image inspect (reliable when Docker socket is accessible)
// 3. Basic Platform field from container (usually just OS, e.g., "linux")
// Returns "unknown" if platform detection fails - this is intentional as the function
// must return a string value for the system variable, and "unknown" accurately
// represents that the platform could not be determined.
func detectContainerPlatform(ctx context.Context, container *tcspanner.Container) string {
	inspectResult, err := container.Inspect(ctx)
	if err != nil {
		slog.Warn("Failed to inspect container", "error", err)
		// Return "unknown" rather than empty string to indicate detection attempted but failed
		return "unknown"
	}

	// Debug log the inspect result
	slog.Debug("Container inspect result",
		"Platform", inspectResult.Platform,
		"ImageManifestDescriptor", inspectResult.ImageManifestDescriptor != nil,
		"Image", inspectResult.Image,
		"ConfigNotNil", inspectResult.Config != nil,
	)

	// Log config details if available
	if inspectResult.Config != nil {
		slog.Debug("Container config details",
			"Image", inspectResult.Config.Image,
			"Labels", inspectResult.Config.Labels,
		)
	}

	// Method 1: Try ImageManifestDescriptor (OCI standard)
	if inspectResult.ImageManifestDescriptor != nil && inspectResult.ImageManifestDescriptor.Platform != nil {
		p := inspectResult.ImageManifestDescriptor.Platform
		slog.Debug("ImageManifestDescriptor.Platform details",
			"OS", p.OS,
			"Architecture", p.Architecture,
			"Variant", p.Variant,
			"OSVersion", p.OSVersion,
			"OSFeatures", p.OSFeatures,
		)
		platform := p.OS + "/" + p.Architecture
		if p.Variant != "" {
			platform += "/" + p.Variant
		}
		return platform
	}

	// Method 2: Try Docker API image inspect
	slog.Debug("ImageManifestDescriptor not available, trying image inspect")
	if inspectResult.Config != nil && inspectResult.Config.Image != "" {
		// Get provider for image inspection.
		// IMPORTANT: GetProvider() creates a NEW provider instance each time, not a singleton.
		// We must close it to avoid resource leaks.
		// See: https://github.com/testcontainers/testcontainers-go/blob/main/provider.go
		genericProvider, err := testcontainers.ProviderDefault.GetProvider()
		if err != nil {
			slog.Debug("Failed to get testcontainers provider", "error", err)
		} else {
			defer func() {
				if err := genericProvider.Close(); err != nil {
					slog.Debug("Failed to close testcontainers provider", "error", err)
				}
			}()

			platform := inspectImagePlatform(ctx, inspectResult.Config.Image, genericProvider)
			if platform != "" {
				return platform
			}
		}
	}

	// Method 3: Fallback to basic platform field
	if inspectResult.Platform != "" {
		slog.Debug("Using basic Platform field", "Platform", inspectResult.Platform)
		return inspectResult.Platform
	}

	// All detection methods failed - return "unknown" to indicate this state
	return "unknown"
}

// inspectImagePlatform uses container runtime API (Docker/Podman) to get platform information from an image.
// The provider parameter should be obtained from testcontainers.ProviderDefault.GetProvider().
// The caller is responsible for managing the provider lifecycle.
func inspectImagePlatform(ctx context.Context, imageName string, provider testcontainers.GenericProvider) string {
	slog.Debug("inspectImagePlatform called", "imageName", imageName)

	// Both Docker and Podman providers return *DockerProvider type, even for Podman.
	// This is because Podman provides Docker-compatible API.
	dockerProvider, ok := provider.(*testcontainers.DockerProvider)
	if !ok {
		slog.Debug("Provider is not a DockerProvider", "type", fmt.Sprintf("%T", provider))
		return ""
	}

	// The provider embeds *client.Client directly (not as a field), so we can access it
	// using provider.Client() which returns the embedded Docker/Podman client.
	dockerClient := dockerProvider.Client()
	slog.Debug("Using container runtime client from testcontainers provider")

	// Use a context with a timeout for Docker API calls to prevent hangs
	// if the container runtime is unresponsive
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Log Docker client info
	info, err := dockerClient.Info(inspectCtx)
	if err != nil {
		slog.Debug("Failed to get Docker info", "error", err)
	} else {
		slog.Debug("Docker client info",
			"ServerVersion", info.ServerVersion,
			"OSType", info.OSType,
			"Architecture", info.Architecture)
	}

	imageInspect, err := dockerClient.ImageInspect(inspectCtx, imageName)
	if err != nil {
		slog.Debug("Image inspect failed",
			"error", err,
			"imageName", imageName)
		return ""
	}

	slog.Debug("Image inspect successful",
		"imageName", imageName,
		"Architecture", imageInspect.Architecture,
		"Os", imageInspect.Os,
		"Variant", imageInspect.Variant,
		"ID", imageInspect.ID,
	)

	// Validate that we have the required fields
	if imageInspect.Os == "" || imageInspect.Architecture == "" {
		slog.Debug("Image inspect result missing Os or Architecture",
			"imageName", imageName,
			"Os", imageInspect.Os,
			"Architecture", imageInspect.Architecture)
		return ""
	}

	platform := imageInspect.Os + "/" + imageInspect.Architecture
	if imageInspect.Variant != "" {
		platform += "/" + imageInspect.Variant
	}
	slog.Debug("Returning platform", "platform", platform)
	return platform
}
