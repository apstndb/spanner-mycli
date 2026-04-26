package mycli

import (
	"strings"
	"testing"

	"github.com/apstndb/spanemuboost"
)

func TestRuntimePlatform(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}

	platform, err := spanemuboost.RuntimePlatform(t.Context(), lazyRuntime)
	if err != nil {
		t.Fatalf("RuntimePlatform() error = %v", err)
	}

	if platform == "" || platform == "unknown" {
		t.Fatalf("RuntimePlatform() = %q, want a resolved platform", platform)
	}
	if platform != "linux" && !strings.HasPrefix(platform, "linux/") {
		t.Fatalf("RuntimePlatform() = %q, want linux or linux/*", platform)
	}
}
