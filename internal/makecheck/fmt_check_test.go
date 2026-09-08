// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package makecheck

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	successMsg = "Code formatting is correct"
	issuesMsg  = "Code formatting issues found. Run 'make fmt' to fix."
	failedMsg  = "Code formatting check failed"
)

func TestFmtCheck(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("make fmt-check is a POSIX recipe")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available")
	}

	tests := []struct {
		name            string
		mode            string
		wantOK          bool
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:            "clean",
			mode:            "clean",
			wantOK:          true,
			wantContains:    []string{successMsg},
			wantNotContains: []string{issuesMsg, failedMsg},
		},
		{
			name:            "diff",
			mode:            "diff",
			wantContains:    []string{issuesMsg, "diff --git a/example.go b/example.go"},
			wantNotContains: []string{successMsg},
		},
		{
			name:            "stderr-only exit 23",
			mode:            "error",
			wantContains:    []string{failedMsg + " (exit 23).", "injected formatter failure", "Error 23"},
			wantNotContains: []string{successMsg},
		},
		{
			name:            "failed formatter with diff-looking stdout",
			mode:            "error-diff",
			wantContains:    []string{failedMsg + " (exit 23).", "diff --git a/example.go b/example.go", "injected formatter failure", "Error 23"},
			wantNotContains: []string{successMsg},
		},
		{
			name:            "large output",
			mode:            "large",
			wantContains:    []string{issuesMsg, "diff --git a/f0.go b/f0.go", "diff --git a/f199.go b/f199.go"},
			wantNotContains: []string{successMsg},
		},
	}

	root := repoRoot(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, status, calls := runFmtCheck(t, root, tt.mode)
			if tt.wantOK {
				if status != 0 {
					t.Fatalf("make fmt-check exit = %d, want 0\n%s", status, out)
				}
			} else if status == 0 {
				t.Fatalf("make fmt-check succeeded, want failure\n%s", out)
			}
			if calls != 1 {
				t.Fatalf("golangci-lint invocations = %d, want 1\n%s", calls, out)
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(out, s) {
					t.Fatalf("output missing %q:\n%s", s, out)
				}
			}
			for _, s := range tt.wantNotContains {
				if strings.Contains(out, s) {
					t.Fatalf("output unexpectedly contains %q:\n%s", s, out)
				}
			}
		})
	}
}

func TestLegacyFmtCheckRecipeMasksStderrOnlyFailure(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("make fmt-check is a POSIX recipe")
	}
	root := repoRoot(t)
	stubDir, countFile, tmpDir := installFormatterStub(t, "error")
	// The pre-fix recipe observed grep, not the formatter. Keep it here as a
	// control: the same stderr-only exit 23 stub must not succeed through
	// the current target (TestFmtCheck/stderr-only_exit_23).
	script := `echo "Checking code formatting..."
if golangci-lint fmt --diff . | grep -q "^diff"; then
	echo "Code formatting issues found. Run 'make fmt' to fix."
	golangci-lint fmt --diff . | head -100
	exit 1
else
	echo "Code formatting is correct"
fi`
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = root
	cmd.Env = isolatedEnv(stubDir, tmpDir, "error")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("legacy recipe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), successMsg) {
		t.Fatalf("legacy recipe did not print success:\n%s", out)
	}
	calls := readCount(t, countFile)
	if calls != 1 {
		// grep -q can close the pipe early, but this stub exits before writing
		// a diff so grep reads EOF once. A second invocation would be the
		// then-branch reprint; that must not happen on this control.
		t.Fatalf("legacy recipe invocations = %d, want 1\n%s", calls, out)
	}
}

func runFmtCheck(t *testing.T, root, mode string) (output string, exitCode, calls int) {
	t.Helper()
	stubDir, countFile, tmpDir := installFormatterStub(t, mode)
	cmd := exec.Command("make", "fmt-check")
	cmd.Dir = root
	cmd.Env = isolatedEnv(stubDir, tmpDir, mode)
	out, err := cmd.CombinedOutput()
	exitCode = 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("make fmt-check: %v\n%s", err, out)
		}
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "spanner-mycli-fmt-check.") {
			t.Fatalf("temporary formatter output was not cleaned up: %s", e.Name())
		}
	}
	return string(out), exitCode, readCount(t, countFile)
}

func installFormatterStub(t *testing.T, mode string) (stubDir, countFile, tmpDir string) {
	t.Helper()
	stubDir = t.TempDir()
	tmpDir = t.TempDir()
	countFile = filepath.Join(stubDir, "count")
	if err := os.WriteFile(countFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/sh
count_file=%q
echo x >> "$count_file"
case %q in
  clean) exit 0 ;;
  diff) printf '%%s\n' 'diff --git a/example.go b/example.go'; exit 0 ;;
  error) printf '%%s\n' 'injected formatter failure' >&2; exit 23 ;;
  error-diff)
    printf '%%s\n' 'diff --git a/example.go b/example.go'
    printf '%%s\n' 'injected formatter failure' >&2
    exit 23
    ;;
  large)
    i=0
    while [ "$i" -lt 200 ]; do
      printf '%%s\n' "diff --git a/f$i.go b/f$i.go"
      i=$((i + 1))
    done
    exit 0
    ;;
  *) echo "unknown FMT_CHECK_MODE" >&2; exit 99 ;;
esac
`, countFile, mode)
	path := filepath.Join(stubDir, "golangci-lint")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return stubDir, countFile, tmpDir
}

func isolatedEnv(stubDir, tmpDir, mode string) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+3)
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "TMPDIR=") || strings.HasPrefix(kv, "FMT_CHECK_MODE=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	filtered = append(filtered,
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+tmpDir,
		"FMT_CHECK_MODE="+mode,
	)
	return filtered
}

func readCount(t *testing.T, countFile string) int {
	t.Helper()
	b, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Makefile not found")
		}
		dir = parent
	}
}
