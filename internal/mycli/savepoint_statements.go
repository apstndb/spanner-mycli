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

package mycli

import (
	"context"
	"fmt"
	"strings"
)

func parseSavepointName(raw string) (string, error) {
	name, err := parseIdentifierArg(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid savepoint name: %w", err)
	}
	if err := validateSavepointName(name); err != nil {
		return "", err
	}
	return name, nil
}

func rejectSavepointCommand(session *Session) error {
	if session != nil && session.batch.IsActive() {
		return errSavepointInManualBatch
	}
	return nil
}

type SavepointStatement struct {
	Name string
}

func (s *SavepointStatement) Execute(ctx context.Context, session *Session, _ OperationOutput) (*Result, error) {
	if err := rejectSavepointCommand(session); err != nil {
		return nil, err
	}
	if err := session.txn.CreateSavepoint(ctx, s.Name); err != nil {
		return nil, err
	}
	return &Result{}, nil
}

type RollbackToSavepointStatement struct {
	Name string
}

func (s *RollbackToSavepointStatement) allowedDuringSavepointRecovery() {}

func (s *RollbackToSavepointStatement) Execute(ctx context.Context, session *Session, _ OperationOutput) (*Result, error) {
	if err := rejectSavepointCommand(session); err != nil {
		return nil, err
	}
	if err := session.txn.RollbackToSavepoint(ctx, s.Name); err != nil {
		return nil, err
	}
	return &Result{}, nil
}

type ReleaseSavepointStatement struct {
	Name string
}

func (s *ReleaseSavepointStatement) Execute(_ context.Context, session *Session, _ OperationOutput) (*Result, error) {
	if err := rejectSavepointCommand(session); err != nil {
		return nil, err
	}
	if err := session.txn.ReleaseSavepoint(s.Name); err != nil {
		return nil, err
	}
	return &Result{}, nil
}
