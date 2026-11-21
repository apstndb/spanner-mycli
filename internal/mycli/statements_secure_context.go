// Copyright 2026 apstndb
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

package mycli

import (
	"context"
	"errors"
	"maps"
	"slices"
	"unicode/utf8"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type SetSecureContextStatement struct {
	Key   string
	Value string
}

func parseSetSecureContext(key, value string) (Statement, error) {
	expr, err := parseMemefishExpr("", value)
	literal, ok := expr.(*ast.StringLiteral)
	if err != nil || !ok || !utf8.ValidString(literal.Value) {
		return nil, errors.New("SET SECURE CONTEXT requires a GoogleSQL string literal with valid UTF-8")
	}
	return &SetSecureContextStatement{Key: key, Value: literal.Value}, nil
}

func (s *SetSecureContextStatement) Execute(ctx context.Context, session *Session, _ OperationOutput) (*Result, error) {
	if err := session.checkSecureContextChange(); err != nil {
		return nil, err
	}
	if old := session.secureContext[s.Key]; old != nil && old.GetStringValue() == s.Value {
		return &Result{KeepVariables: true}, nil
	}
	values := make(map[string]*structpb.Value, len(session.secureContext)+1)
	maps.Copy(values, session.secureContext)
	values[s.Key] = structpb.NewStringValue(s.Value)
	if err := session.replaceDataClient(ctx, values); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

type ShowSecureContextStatement struct{}

func (*ShowSecureContextStatement) isDetachedCompatible()           {}
func (*ShowSecureContextStatement) allowedDuringSavepointRecovery() {}

func (*ShowSecureContextStatement) Execute(_ context.Context, session *Session, out OperationOutput) (*Result, error) {
	rows := make([]nameValueRow, 0, len(session.secureContext))
	for _, key := range slices.Sorted(maps.Keys(session.secureContext)) {
		rows = append(rows, nameValueRow{Name: key, Value: session.secureContext[key].GetStringValue()})
	}
	result, err := executeStructRows(nameValueRowEncoder, rows, session, out)
	if err != nil {
		return nil, err
	}
	result.KeepVariables = true
	return result, nil
}

type UnsetSecureContextStatement struct{ Key string }

func (s *UnsetSecureContextStatement) Execute(ctx context.Context, session *Session, _ OperationOutput) (*Result, error) {
	if err := session.checkSecureContextChange(); err != nil {
		return nil, err
	}
	if len(session.secureContext) == 0 || (s.Key != "" && session.secureContext[s.Key] == nil) {
		return &Result{KeepVariables: true}, nil
	}
	var values map[string]*structpb.Value
	if s.Key != "" {
		values = maps.Clone(session.secureContext)
		delete(values, s.Key)
	}
	if err := session.replaceDataClient(ctx, values); err != nil {
		return nil, err
	}
	return &Result{KeepVariables: true}, nil
}

func (s *Session) checkSecureContextChange() error {
	if err := s.ValidateDatabaseOperation(); err != nil {
		return err
	}
	if s.txn.InTransaction() || s.batch.IsActive() {
		return errors.New("changing Secure Context requires an idle connection without an active transaction or batch")
	}
	return nil
}

func (s *Session) checkPartitionSecureContext() error {
	if s == nil {
		return nil
	}
	if len(s.clientConfig.ClientContext.GetSecureContext()) != 0 {
		return errors.New("partitioned queries are unavailable with an embedded Secure Context")
	}
	if len(s.secureContext) != 0 {
		return errors.New("partitioned queries require UNSET SECURE CONTEXT")
	}
	return nil
}

// The SDK supplies ClientConfig.ClientContext as a default for request
// options, including DML, plan inspection, and transaction RPCs. Keep the
// baseline intact for embedded callers and build a fresh context on update.
func withSecureContext(base *sppb.RequestOptions_ClientContext, values map[string]*structpb.Value) *sppb.RequestOptions_ClientContext {
	if len(values) == 0 {
		return base
	}
	ctx := &sppb.RequestOptions_ClientContext{}
	if base != nil {
		ctx = proto.CloneOf(base)
	}
	if ctx.SecureContext == nil {
		ctx.SecureContext = make(map[string]*structpb.Value, len(values))
	}
	maps.Copy(ctx.SecureContext, values)
	return ctx
}
