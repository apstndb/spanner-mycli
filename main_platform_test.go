package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

// TestInspectImagePlatform tests the inspectImagePlatform function
func TestInspectImagePlatform(t *testing.T) {
	t.Parallel()
	// Skip this test in short mode as it requires Docker access
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Enable debug logging for this test to help diagnose CI failures.
	// IMPORTANT: We modify the global logger here, which could affect other tests
	// if they run in parallel. To prevent test interference:
	// 1. Save the original logger before modification
	// 2. Restore it in a defer to ensure it always gets restored
	// This pattern ensures test isolation without requiring dependency injection.
	oldLogger := slog.Default()
	defer slog.SetDefault(oldLogger)

	// Configure slog to output to stderr at debug level for detailed diagnostics
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(handler))

	// Log test environment
	t.Logf("Running TestInspectImagePlatform in environment: CI=%s, DOCKER_HOST=%s",
		os.Getenv("CI"), os.Getenv("DOCKER_HOST"))

	// Get provider once for all tests
	provider, err := testcontainers.ProviderDefault.GetProvider()
	if err != nil {
		t.Fatalf("Failed to get testcontainers provider: %v", err)
	}
	defer func() {
		if err := provider.Close(); err != nil {
			t.Logf("Failed to close provider: %v", err)
		}
	}()

	// Pull the test image explicitly to ensure it's available in CI environments
	// where images may not be cached
	testImage := "testcontainers/ryuk:0.12.0"
	t.Logf("Pulling test image: %s", testImage)
	ctx := context.Background()
	err = provider.PullImage(ctx, testImage)
	if err != nil {
		t.Fatalf("Failed to pull test image %s: %v", testImage, err)
	}
	t.Logf("Successfully pulled test image: %s", testImage)

	// Test cases for inspectImagePlatform function.
	// Note: We explicitly pull the testcontainers/ryuk image above to ensure
	// it's available in CI environments where images may not be cached.
	tests := []struct {
		name      string
		imageName string
		wantOS    string
		wantEmpty bool // true if we expect empty string (error case)
	}{
		{
			name:      "testcontainers ryuk image (explicitly pulled)",
			imageName: testImage, // Use the same image we pulled above
			wantOS:    "linux",
			wantEmpty: false,
		},
		{
			name:      "non-existent image",
			imageName: "non-existent-image:tag",
			wantOS:    "",
			wantEmpty: true,
		},
		{
			name:      "empty image name",
			imageName: "",
			wantOS:    "",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing inspectImagePlatform with image: %q", tt.imageName)
			platform := inspectImagePlatform(context.Background(), tt.imageName, provider)
			t.Logf("Result: platform=%q", platform)

			if tt.wantEmpty && platform != "" {
				t.Errorf("inspectImagePlatform(%q) = %q, want empty string", tt.imageName, platform)
			}

			if !tt.wantEmpty {
				if platform == "" {
					t.Errorf("inspectImagePlatform(%q) = empty, want non-empty platform", tt.imageName)
				} else if !strings.Contains(platform, tt.wantOS) {
					t.Errorf("inspectImagePlatform(%q) = %q, want to contain %q", tt.imageName, platform, tt.wantOS)
				}
			}
		})
	}
}
