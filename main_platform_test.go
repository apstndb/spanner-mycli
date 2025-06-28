package main

import (
	"context"
	"testing"
)

// TestInspectImagePlatform tests the inspectImagePlatform function
func TestInspectImagePlatform(t *testing.T) {
	// Skip this test in short mode as it requires Docker
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	tests := []struct {
		name      string
		imageName string
		wantOS    string
		wantEmpty bool // true if we expect empty string (error case)
	}{
		{
			name:      "valid public image",
			imageName: "hello-world:latest",
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
			platform := inspectImagePlatform(context.Background(), tt.imageName)
			
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