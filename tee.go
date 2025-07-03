package main

import (
	"fmt"
	"io"
	"os"
	"sync"
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
			return nil, fmt.Errorf("tee output to a non-regular file is not supported: %s", filePath)
		}
	case os.IsNotExist(err):
		// File doesn't exist - OpenFile will create it
		// Continue to OpenFile
	default:
		// Unexpected stat error (e.g., permission denied)
		return nil, fmt.Errorf("failed to stat tee file %q: %w", filePath, err)
	}
	
	// Open tee file in append mode (creates if doesn't exist)
	return os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
	mu          sync.Mutex
	teeFile     *os.File
	originalOut io.Writer
	errStream   io.Writer
}

// NewTeeManager creates a new TeeManager instance
func NewTeeManager(originalOut, errStream io.Writer) *TeeManager {
	return &TeeManager{
		originalOut: originalOut,
		errStream:   errStream,
	}
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
		_ = tm.teeFile.Close()
	}
	
	tm.teeFile = teeFile
	return nil
}

// DisableTee stops tee output and closes the tee file
func (tm *TeeManager) DisableTee() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if tm.teeFile != nil {
		_ = tm.teeFile.Close()
		tm.teeFile = nil
	}
}

// GetWriter returns the current output writer (with or without tee)
func (tm *TeeManager) GetWriter() io.Writer {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if tm.teeFile == nil {
		return tm.originalOut
	}
	
	return createTeeWriter(tm.originalOut, tm.teeFile, tm.errStream)
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