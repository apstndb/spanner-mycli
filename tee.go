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
		// Return success to io.MultiWriter to avoid interrupting stdout
		return len(p), nil
	}
	
	return n, nil
}

// openTeeFile validates and opens a file for tee output
func openTeeFile(filePath string) (*os.File, error) {
	// Open tee file in append mode (creates if doesn't exist)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	
	// Validate after opening to avoid TOCTOU race
	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat tee file %q: %w", filePath, err)
	}
	
	// Ensure it's a regular file (not a device, FIFO, etc.)
	if !fi.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("tee output to a non-regular file is not supported: %s", filePath)
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

// TeeManager manages tee output functionality for both --tee option and \T/\t meta-commands
type TeeManager struct {
	mu           sync.Mutex
	teeFile      *os.File
	originalOut  io.Writer
	errStream    io.Writer
	cachedWriter io.Writer // Cache the writer to ensure consistent behavior
	ttyStream    *os.File  // Original TTY stream for terminal operations
}

// NewTeeManager creates a new TeeManager instance
func NewTeeManager(originalOut, errStream io.Writer) *TeeManager {
	tm := &TeeManager{
		originalOut: originalOut,
		errStream:   errStream,
	}
	
	// If originalOut is a terminal, keep a reference for terminal operations
	if f, ok := originalOut.(*os.File); ok {
		tm.ttyStream = f
	}
	
	return tm
}

// SetTtyStream sets the TTY stream for terminal operations
// This is useful when the original output is not a TTY (e.g., when piped)
func (tm *TeeManager) SetTtyStream(tty *os.File) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.ttyStream = tty
}

// EnableTee starts tee output to the specified file
func (tm *TeeManager) EnableTee(filePath string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Open the new tee file first
	teeFile, err := openTeeFile(filePath)
	if err != nil {
		return err
	}
	
	// Close any existing tee file after successful open
	if tm.teeFile != nil {
		if err := tm.teeFile.Close(); err != nil {
			// Log the error, but continue. The main operation (enabling the new tee) has succeeded.
			slog.Warn("failed to close previous tee file", "path", tm.teeFile.Name(), "error", err)
		}
	}
	
	tm.teeFile = teeFile
	tm.cachedWriter = nil // Invalidate cached writer
	return nil
}

// DisableTee stops tee output and closes the tee file
func (tm *TeeManager) DisableTee() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if tm.teeFile != nil {
		if err := tm.teeFile.Close(); err != nil {
			// Log the error, but continue. We're disabling tee, so we don't want to fail.
			slog.Warn("failed to close tee file", "path", tm.teeFile.Name(), "error", err)
		}
		tm.teeFile = nil
		tm.cachedWriter = nil // Invalidate cached writer
	}
}

// GetWriter returns the current output writer (with or without tee)
func (tm *TeeManager) GetWriter() io.Writer {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if tm.teeFile == nil {
		return tm.originalOut
	}
	
	// Return cached writer if available
	if tm.cachedWriter != nil {
		return tm.cachedWriter
	}
	
	// Create and cache new writer
	tm.cachedWriter = createTeeWriter(tm.originalOut, tm.teeFile, tm.errStream)
	return tm.cachedWriter
}

// IsEnabled returns whether tee output is currently active
func (tm *TeeManager) IsEnabled() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.teeFile != nil
}

// Close closes any open tee file and cleans up resources
func (tm *TeeManager) Close() {
	tm.DisableTee()
}

// GetTerminalWidth returns the current terminal width
// Returns 0 and an error if the output is not a terminal
func (tm *TeeManager) GetTerminalWidth() (int, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Use ttyStream if available (preferred)
	if tm.ttyStream != nil {
		width, _, err := term.GetSize(int(tm.ttyStream.Fd()))
		if err != nil {
			return 0, fmt.Errorf("failed to get terminal size: %w", err)
		}
		return width, nil
	}
	
	// Fall back to originalOut if it's a file
	if f, ok := tm.originalOut.(*os.File); ok {
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
func (tm *TeeManager) GetTerminalWidthString() string {
	width, err := tm.GetTerminalWidth()
	if err != nil {
		slog.Warn("failed to get terminal size", "error", err)
		return "NULL"
	}
	return strconv.Itoa(width)
}

// IsTerminal returns whether the output is a terminal
func (tm *TeeManager) IsTerminal() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Check ttyStream first
	if tm.ttyStream != nil {
		return term.IsTerminal(int(tm.ttyStream.Fd()))
	}
	
	// Check originalOut
	if f, ok := tm.originalOut.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	
	return false
}