//
// Copyright 2025 apstndb
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

package filesafety

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestValidateFileSafety(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a regular file
	regularFile := filepath.Join(tmpDir, "regular.txt")
	content := []byte("test content")
	if err := os.WriteFile(regularFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	// Create a named pipe (FIFO) - only on Unix-like systems
	var fifoFile string
	if runtime.GOOS != "windows" {
		fifoFile = filepath.Join(tmpDir, "fifo")
		if err := syscall.Mkfifo(fifoFile, 0o644); err != nil {
			t.Skipf("Skipping FIFO test: %v", err)
		}
	}

	tests := []struct {
		name    string
		path    string
		opts    *FileSafetyOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "regular file within size limit",
			path:    regularFile,
			opts:    nil,
			wantErr: false,
		},
		{
			name:    "regular file with custom size limit",
			path:    regularFile,
			opts:    &FileSafetyOptions{MaxSize: 100},
			wantErr: false,
		},
		{
			name:    "regular file exceeds size limit",
			path:    regularFile,
			opts:    &FileSafetyOptions{MaxSize: 5},
			wantErr: true,
			errMsg:  "too large",
		},
		{
			name:    "directory rejected",
			path:    tmpDir,
			opts:    nil,
			wantErr: true,
			errMsg:  "cannot read special file",
		},
	}

	// Add FIFO tests only on Unix-like systems
	if runtime.GOOS != "windows" && fifoFile != "" {
		tests = append(tests, []struct {
			name    string
			path    string
			opts    *FileSafetyOptions
			wantErr bool
			errMsg  string
		}{
			{
				name:    "named pipe rejected by default",
				path:    fifoFile,
				opts:    nil,
				wantErr: true,
				errMsg:  "cannot read named pipe",
			},
			{
				name:    "named pipe allowed with option",
				path:    fifoFile,
				opts:    &FileSafetyOptions{AllowNonRegular: true},
				wantErr: false,
			},
		}...)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi, err := os.Stat(tt.path)
			if err != nil {
				t.Fatalf("Failed to stat file: %v", err)
			}

			err = ValidateFileSafety(fi, tt.path, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileSafety() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateFileSafety() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestSafeReadFile(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a regular file
	regularFile := filepath.Join(tmpDir, "regular.txt")
	expectedContent := "test content for safe read"
	if err := os.WriteFile(regularFile, []byte(expectedContent), 0o644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	// Create a large file
	largeFile := filepath.Join(tmpDir, "large.txt")
	largeContent := strings.Repeat("x", 1024*1024) // 1MB
	if err := os.WriteFile(largeFile, []byte(largeContent), 0o644); err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		opts        *FileSafetyOptions
		wantContent string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "read regular file",
			path:        regularFile,
			opts:        nil,
			wantContent: expectedContent,
			wantErr:     false,
		},
		{
			name:        "read large file within default limit",
			path:        largeFile,
			opts:        nil,
			wantContent: largeContent,
			wantErr:     false,
		},
		{
			name:    "reject large file with custom limit",
			path:    largeFile,
			opts:    &FileSafetyOptions{MaxSize: 1024}, // 1KB limit
			wantErr: true,
			errMsg:  "too large",
		},
		{
			name:    "non-existent file",
			path:    filepath.Join(tmpDir, "nonexistent.txt"),
			opts:    nil,
			wantErr: true,
			errMsg:  "no such file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := SafeReadFile(tt.path, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeReadFile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("SafeReadFile() error = %v, want error containing %q", err, tt.errMsg)
			}
			if !tt.wantErr && string(data) != tt.wantContent {
				t.Errorf("SafeReadFile() got %d bytes, want %d bytes", len(data), len(tt.wantContent))
			}
		})
	}
}

func TestFileSafetyConstants(t *testing.T) {
	// Verify constants have expected values
	if DefaultMaxFileSize != 100*1024*1024 {
		t.Errorf("DefaultMaxFileSize = %d, want %d", DefaultMaxFileSize, 100*1024*1024)
	}
	if SampleDatabaseMaxFileSize != 10*1024*1024 {
		t.Errorf("SampleDatabaseMaxFileSize = %d, want %d", SampleDatabaseMaxFileSize, 10*1024*1024)
	}
}
