// StreamManager manages all I/O streams for the CLI.
// All methods are safe for concurrent use.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	
	"golang.org/x/term"
)

// safeTeeWriter wraps a file writer to handle write errors gracefully.
// On write error, it prints a warning once and then discards subsequent writes
// to prevent interrupting the main output stream.
type safeTeeWriter struct {
	file      *os.File
	errStream io.Writer
	hasWarned bool
	mu        sync.Mutex
}

// Write implements io.Writer, handling errors gracefully
func (s *safeTeeWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// If we've already warned about an error, just discard the write
	if s.hasWarned {
		return len(p), nil
	}
	
	n, err = s.file.Write(p)
	if err != nil {
		// Print warning only once to avoid spamming
		s.hasWarned = true
		fmt.Fprintf(s.errStream, "WARNING: Failed to write to tee file: %v\n", err)
		fmt.Fprintf(s.errStream, "WARNING: Tee logging disabled for remainder of session\n")
		// CRITICAL: We must return success here even though the write failed.
		// io.MultiWriter will stop writing to ALL writers (including stdout!)
		// if ANY writer returns an error. By returning success, we ensure
		// that tee failures never interrupt the main CLI output.
		return len(p), nil
	}
	
	return n, nil
}

// openTeeFile validates and opens a file for tee output
func openTeeFile(filePath string) (*os.File, error) {
	// Check if the file exists and is a regular file before opening.
	// This prevents blocking on special files like FIFOs.
	fi, err := os.Stat(filePath)
	
	// Handle three cases:
	// 1. File exists (err == nil) - check if it's a regular file
	// 2. File doesn't exist (os.IsNotExist(err)) - will create it
	// 3. Other stat error - return the error
	switch {
	case err == nil:
		// File exists - ensure it's a regular file
		if !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("tee output to a non-regular file is not supported: %q", filePath)
		}
	case os.IsNotExist(err):
		// File doesn't exist - OpenFile will create it
		// Continue to OpenFile
	default:
		// Unexpected stat error (e.g., permission denied)
		return nil, fmt.Errorf("failed to stat tee file %q: %w", filePath, err)
	}
	
	// Open tee file in append mode (creates if doesn't exist)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	
	// Double-check the file after opening to handle TOCTOU race conditions.
	// Without this check, a regular file could be replaced with a FIFO
	// between our initial stat and open calls, causing the CLI to block.
	fi, err = file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat opened file %q: %w", filePath, err)
	}
	
	if !fi.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("tee output to a non-regular file is not supported: %q", filePath)
	}
	
	return file, nil
}

// createTeeWriter creates a MultiWriter that writes to both the original stream and a tee file
func createTeeWriter(originalStream io.Writer, teeFile *os.File, errStream io.Writer) io.Writer {
	// Wrap the tee file with error handling to prevent write failures from interrupting stdout
	safeWriter := &safeTeeWriter{
		file:      teeFile,
		errStream: errStream,
		hasWarned: false,
	}
	
	// Create a MultiWriter that writes to both original stream and the safe tee writer
	return io.MultiWriter(originalStream, safeWriter)
}

// StreamManager manages all input/output streams for the CLI.
// It centralizes handling of stdin, stdout, stderr, TTY operations, and tee functionality.
// This component ensures proper separation between data output (which may be tee'd)
// and terminal control operations (which should not be tee'd).
// It consolidates stream handling, tee functionality, and terminal operations
type StreamManager struct {
	mu           sync.Mutex
	inStream     io.ReadCloser // Input stream (stdin or custom)
	outStream    io.Writer     // Main output stream (stdout or custom)
	errStream    io.Writer     // Error output stream (stderr or custom)
	ttyStream    *os.File      // Original TTY stream for terminal operations
	teeFile      *os.File      // Tee file when enabled
	cachedWriter io.Writer     // Cache the writer to ensure consistent behavior
}

// NewStreamManager creates a new StreamManager instance
func NewStreamManager(inStream io.ReadCloser, outStream, errStream io.Writer) *StreamManager {
	sm := &StreamManager{
		inStream:  inStream,
		outStream: outStream,
		errStream: errStream,
	}
	
	// If outStream is a terminal, keep a reference for terminal operations
	if f, ok := outStream.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		sm.ttyStream = f
	}
	
	return sm
}


// SetTtyStream sets the TTY stream for terminal operations
// This is useful when the original output is not a TTY (e.g., when piped)
func (sm *StreamManager) SetTtyStream(tty *os.File) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ttyStream = tty
}

// EnableTee starts tee output to the specified file
func (sm *StreamManager) EnableTee(filePath string) error {
	// DESIGN NOTE: File opening is intentionally done OUTSIDE the mutex lock.
	// This prevents blocking other StreamManager operations (GetWriter, etc.)
	// during potentially slow file I/O (e.g., network filesystems).
	//
	// This creates a harmless race condition where concurrent EnableTee calls
	// may result in "last writer wins" behavior. This is acceptable because:
	// 1. Tee configuration is typically done single-threaded (CLI startup or REPL)
	// 2. The semantics are "replace current tee file", not "add if not exists"
	// 3. Proper resource cleanup is maintained regardless of ordering
	teeFile, err := openTeeFile(filePath)
	if err != nil {
		return err
	}
	
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Close any existing tee file after successful open
	if sm.teeFile != nil {
		if err := sm.teeFile.Close(); err != nil {
			// If we can't close the old file, we're in a risky state.
			// To be safe, close the new file and abort the operation to prevent a leak.
			_ = teeFile.Close()
			return fmt.Errorf("failed to switch tee file: could not close previous file %q: %w", sm.teeFile.Name(), err)
		}
	}
	
	sm.teeFile = teeFile
	// IMPORTANT: Invalidate cached writer to ensure the new tee file is included
	// in the io.MultiWriter. This is critical for maintaining correct output behavior.
	sm.cachedWriter = nil
	return nil
}

// DisableTee stops tee output and closes the tee file
func (sm *StreamManager) DisableTee() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if sm.teeFile != nil {
		if err := sm.teeFile.Close(); err != nil {
			// Log the error, but continue. We're disabling tee, so we don't want to fail.
			slog.Warn("failed to close tee file", "path", sm.teeFile.Name(), "error", err)
		}
		sm.teeFile = nil
		sm.cachedWriter = nil // Invalidate cached writer
	}
}

// GetWriter returns the current output writer (with or without tee)
func (sm *StreamManager) GetWriter() io.Writer {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if sm.teeFile == nil {
		return sm.outStream
	}
	
	// Return cached writer if available.
	// Caching is critical for two reasons:
	// 1. It ensures all code paths use the same writer instance, preventing
	//    multiple safeTeeWriter instances that would each show their own warning
	// 2. It improves performance by avoiding repeated MultiWriter creation
	if sm.cachedWriter != nil {
		return sm.cachedWriter
	}
	
	// Create and cache new writer
	sm.cachedWriter = createTeeWriter(sm.outStream, sm.teeFile, sm.errStream)
	return sm.cachedWriter
}

// IsEnabled returns whether tee output is currently active
func (sm *StreamManager) IsEnabled() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.teeFile != nil
}

// Close closes any open tee file and cleans up resources
func (sm *StreamManager) Close() {
	sm.DisableTee()
}

// GetTerminalWidth returns the current terminal width
// Returns 0 and an error if the output is not a terminal
func (sm *StreamManager) GetTerminalWidth() (int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Use ttyStream if available (preferred)
	if sm.ttyStream != nil {
		width, _, err := term.GetSize(int(sm.ttyStream.Fd()))
		if err != nil {
			return 0, fmt.Errorf("failed to get terminal size: %w", err)
		}
		return width, nil
	}
	
	// Fall back to outStream if it's a file
	if f, ok := sm.outStream.(*os.File); ok {
		width, _, err := term.GetSize(int(f.Fd()))
		if err != nil {
			return 0, fmt.Errorf("failed to get terminal size: %w", err)
		}
		return width, nil
	}
	
	return 0, fmt.Errorf("output is not a terminal")
}

// GetTerminalWidthString returns the terminal width as a string
// Returns "NULL" if the terminal width cannot be determined
func (sm *StreamManager) GetTerminalWidthString() string {
	width, err := sm.GetTerminalWidth()
	if err != nil {
		slog.Warn("failed to get terminal size", "error", err)
		return "NULL"
	}
	return strconv.Itoa(width)
}

// IsTerminal returns whether the output is a terminal
func (sm *StreamManager) IsTerminal() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Check ttyStream first
	if sm.ttyStream != nil {
		return term.IsTerminal(int(sm.ttyStream.Fd()))
	}
	
	// Check outStream
	if f, ok := sm.outStream.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	
	return false
}

// GetErrStream returns the error stream
func (sm *StreamManager) GetErrStream() io.Writer {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	return sm.errStream
}

// GetInStream returns the input stream
func (sm *StreamManager) GetInStream() io.ReadCloser {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	return sm.inStream
}

// GetOutStream returns the output stream (without tee)
// For tee-enabled output, use GetWriter() instead
func (sm *StreamManager) GetOutStream() io.Writer {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	return sm.outStream
}

// GetTtyStream returns the TTY stream for terminal operations
func (sm *StreamManager) GetTtyStream() *os.File {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	return sm.ttyStream
}