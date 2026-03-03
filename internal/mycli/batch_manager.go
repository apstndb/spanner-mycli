// Copyright 2025 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"errors"
	"fmt"
)

// BatchManager encapsulates batch statement accumulation state.
// It owns the lifecycle of a batch (start → buffer → run/abort) and provides
// a single source of truth for "am I in a batch?" checks.
type BatchManager struct {
	current Statement
}

// IsActive reports whether a batch is currently in progress.
func (b *BatchManager) IsActive() bool {
	return b.current != nil
}

// Start begins a new batch of the given mode.
// Returns an error if a batch is already active.
func (b *BatchManager) Start(mode batchMode) error {
	if b.current != nil {
		return fmt.Errorf("already in batch, you should execute ABORT BATCH")
	}

	switch mode {
	case batchModeDDL:
		b.current = &BulkDdlStatement{}
	case batchModeDML:
		b.current = &BatchDMLStatement{}
	default:
		return fmt.Errorf("unknown batchMode: %v", mode)
	}
	return nil
}

// Abort discards the current batch without executing it.
func (b *BatchManager) Abort() {
	b.current = nil
}

// TakeForExecution removes and returns the current batch for execution.
// Returns an error if no batch is active.
func (b *BatchManager) TakeForExecution() (Statement, error) {
	if b.current == nil {
		return nil, errors.New("no active batch")
	}
	batch := b.current
	b.current = nil
	return batch, nil
}

// Info returns metadata about the current batch, or nil if no batch is active.
func (b *BatchManager) Info() *BatchInfo {
	switch s := b.current.(type) {
	case *BulkDdlStatement:
		return &BatchInfo{Mode: batchModeDDL, Size: len(s.Ddls)}
	case *BatchDMLStatement:
		return &BatchInfo{Mode: batchModeDML, Size: len(s.DMLs)}
	default:
		return nil
	}
}

// Current returns the current batch statement for type-switching.
// Returns nil if no batch is active.
func (b *BatchManager) Current() Statement {
	return b.current
}

// SetCurrent directly sets the current batch statement.
// Used by auto-batch-DML to create a batch with an initial statement.
func (b *BatchManager) SetCurrent(stmt Statement) {
	b.current = stmt
}
