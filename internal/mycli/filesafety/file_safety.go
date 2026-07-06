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
	"fmt"
	"io"
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

func maxFileSize(opts *FileSafetyOptions) int64 {
	if opts == nil || opts.MaxSize == 0 {
		return DefaultMaxFileSize
	}
	return opts.MaxSize
}

// ValidateFileSafety checks if a file is safe to read based on the given options
func ValidateFileSafety(fi os.FileInfo, path string, opts *FileSafetyOptions) error {
	if opts == nil {
		opts = &FileSafetyOptions{}
	}

	maxSize := maxFileSize(opts)

	// Directories are never readable as files, regardless of AllowNonRegular;
	// reject them here for a clear error instead of a system-dependent
	// os.ReadFile failure (EISDIR etc.).
	if fi.IsDir() {
		return fmt.Errorf("cannot read directory %s", path)
	}

	// Check for special files (devices, named pipes, sockets, etc.)
	if !opts.AllowNonRegular {
		if !fi.Mode().IsRegular() {
			// Check specific modes for better error messages
			mode := fi.Mode()
			switch {
			case mode&os.ModeCharDevice != 0:
				return fmt.Errorf("cannot read character device %s", path)
			case mode&os.ModeDevice != 0:
				return fmt.Errorf("cannot read device file %s", path)
			case mode&os.ModeNamedPipe != 0:
				return fmt.Errorf("cannot read named pipe %s", path)
			case mode&os.ModeSocket != 0:
				return fmt.Errorf("cannot read socket file %s", path)
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

// SafeReadFile reads a file after performing safety checks, validating the same
// file descriptor it reads from to avoid a stat-then-reopen TOCTOU.
//
// The flow mirrors streamio.openOutputFile's open-then-fstat pattern:
//  1. A pre-open Stat rejects unsafe targets (e.g. non-regular files when
//     AllowNonRegular is unset, or directories) before we open them. This
//     matters because opening a FIFO O_RDONLY blocks until a writer appears, so
//     we must not open one we are only going to reject.
//  2. After os.Open, f.Stat() re-validates against the descriptor actually
//     opened. If the path was swapped between the two stats (e.g. a regular
//     file replaced by a device or FIFO), this catches it and we never read
//     from an unexpected target.
//
// The read itself is always capped via io.LimitReader for BOTH regular and
// allowed non-regular inputs. Stat sizes are meaningless for pipes and can be
// stale for regular files, so the limit is enforced at read time rather than
// only from the FileInfo size.
func SafeReadFile(path string, opts *FileSafetyOptions) ([]byte, error) {
	maxSize := maxFileSize(opts)

	// Pre-open validation: reject unsafe targets before opening so we never
	// block opening (e.g. a FIFO) a file we would only reject anyway.
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", path, err)
	}
	if err := ValidateFileSafety(fi, path, opts); err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer f.Close()

	// Re-validate against the opened descriptor to close the stat-then-reopen
	// TOCTOU: the file we read from is exactly the one we validated here.
	fi, err = f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat opened file %s: %w", path, err)
	}
	if err := ValidateFileSafety(fi, path, opts); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("file %s too large: exceeded %d bytes", path, maxSize)
	}
	return data, nil
}
