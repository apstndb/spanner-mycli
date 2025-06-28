package main

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

// TestInspectImagePlatform tests the inspectImagePlatform function
func TestInspectImagePlatform(t *testing.T) {
	// Skip this test in short mode as it requires Docker access
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Enable debug logging for this test
	// Configure slog to output to test logger at debug level
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(handler))

	// Log test environment
	t.Logf("Running TestInspectImagePlatform in environment: CI=%s, DOCKER_HOST=%s", 
		os.Getenv("CI"), os.Getenv("DOCKER_HOST"))

	// In CI, we need to use an image that's already available from testcontainers
	// The emulator image should be available as it's used in other tests
	tests := []struct {
		name      string
		imageName string
		wantOS    string
		wantEmpty bool // true if we expect empty string (error case)
	}{
		{
			name:      "testcontainers ryuk image (always available)",
			imageName: "testcontainers/ryuk:0.11.0",
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
			platform := inspectImagePlatform(context.Background(), tt.imageName)
			t.Logf("Result: platform=%q", platform)
			
			if tt.wantEmpty && platform != "" {
				t.Errorf("inspectImagePlatform(%q) = %q, want empty string", tt.imageName, platform)
			}
			
			if !tt.wantEmpty {
				if platform == "" {
					t.Errorf("inspectImagePlatform(%q) = empty, want non-empty platform", tt.imageName)
				} else if !contains(platform, tt.wantOS) {
					t.Errorf("inspectImagePlatform(%q) = %q, want to contain %q", tt.imageName, platform, tt.wantOS)
				}
			}
		})
	}
}