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

package main

import (
	"fmt"
	"os"
)

const (
	// DefaultMaxFileSize is the default maximum file size for safety checks (100MB)
	DefaultMaxFileSize = 100 * 1024 * 1024 // 100MB

	// SampleDatabaseMaxFileSize is the maximum file size for sample database files (10MB)
	// Sample databases should be smaller since they're downloaded from remote sources
	SampleDatabaseMaxFileSize = 10 * 1024 * 1024 // 10MB
)

// FileSafetyOptions configures file safety checks
type FileSafetyOptions struct {
	// MaxSize is the maximum allowed file size (0 means use DefaultMaxFileSize)
	MaxSize int64
	// AllowNonRegular allows reading from non-regular files (not recommended)
	AllowNonRegular bool
}

// ValidateFileSafety checks if a file is safe to read based on the given options
func ValidateFileSafety(fi os.FileInfo, path string, opts *FileSafetyOptions) error {
	if opts == nil {
		opts = &FileSafetyOptions{}
	}

	maxSize := opts.MaxSize
	if maxSize == 0 {
		maxSize = DefaultMaxFileSize
	}

	// Check for special files (devices, named pipes, sockets, etc.)
	if !opts.AllowNonRegular {
		if !fi.Mode().IsRegular() {
			// Check specific modes for better error messages
			mode := fi.Mode()
			switch {
			case mode&os.ModeDevice != 0:
				return fmt.Errorf("cannot read device file %s", path)
			case mode&os.ModeNamedPipe != 0:
				return fmt.Errorf("cannot read named pipe %s", path)
			case mode&os.ModeSocket != 0:
				return fmt.Errorf("cannot read socket file %s", path)
			case mode&os.ModeCharDevice != 0:
				return fmt.Errorf("cannot read character device %s", path)
			default:
				return fmt.Errorf("cannot read special file %s (mode: %v)", path, mode)
			}
		}
	}

	// Check file size
	if fi.Size() > maxSize {
		return fmt.Errorf("file %s too large: %d bytes (max %d)", path, fi.Size(), maxSize)
	}

	return nil
}

// SafeReadFile reads a file after performing safety checks
func SafeReadFile(path string, opts *FileSafetyOptions) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", path, err)
	}

	if err := ValidateFileSafety(fi, path, opts); err != nil {
		return nil, err
	}

	return os.ReadFile(path)
}
