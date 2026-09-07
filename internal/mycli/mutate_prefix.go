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
	"fmt"
	"strings"
	"unicode"

	"github.com/cloudspannerecosystem/memefish/token"
)

var mutateOperations = []string{"INSERT", "UPDATE", "INSERT_OR_UPDATE", "REPLACE", "DELETE"}

func canonicalMutateOperation(op string) string {
	return strings.ToUpper(op)
}

func parseMutateArgs(rest string) (*MutateStatement, error) {
	if strings.TrimSpace(rest) == "" {
		return nil, fmt.Errorf("MUTATE requires <table_fqn> and an operation")
	}
	p := newParser("", rest)
	if err := p.NextToken(); err != nil {
		return nil, fmt.Errorf("invalid MUTATE table: %w", err)
	}
	idents, err := parseFQNParts(p)
	if err != nil {
		return nil, fmt.Errorf("invalid MUTATE table: %w", err)
	}
	id, err := tableIDFromIdents(idents)
	if err != nil {
		return nil, fmt.Errorf("invalid MUTATE table: %w", err)
	}
	if p.Token.Kind == token.TokenEOF {
		return nil, fmt.Errorf("MUTATE requires an operation after table %s", id.FQN())
	}
	if p.Token.Space == "" {
		return nil, fmt.Errorf("MUTATE requires whitespace before operation, got %q", p.Token.Raw)
	}
	op, err := mutateOperationFromToken(&p.Token)
	if err != nil {
		return nil, err
	}
	suffix := rest[int(p.Token.End):]
	if suffix == "" {
		return nil, fmt.Errorf("MUTATE %s %s requires a body", id.FQN(), op)
	}
	body := strings.TrimLeftFunc(suffix, unicode.IsSpace)
	if len(body) == len(suffix) {
		return nil, fmt.Errorf("MUTATE requires whitespace after operation, got %q", suffix)
	}
	if body == "" {
		return nil, fmt.Errorf("MUTATE %s %s requires a body", id.FQN(), op)
	}
	return &MutateStatement{Table: id.FQN(), Operation: op, Body: body}, nil
}

func tableIDFromIdents(idents []string) (tableID, error) {
	switch len(idents) {
	case 1:
		if idents[0] == "" {
			return tableID{}, fmt.Errorf("empty table name")
		}
		return tableID{Name: idents[0]}, nil
	case 2:
		if idents[0] == "" || idents[1] == "" {
			return tableID{}, fmt.Errorf("empty identifier in table path")
		}
		return tableID{Schema: idents[0], Name: idents[1]}, nil
	default:
		return tableID{}, fmt.Errorf("expected [<schema>.]<table>, but %q has %d components", strings.Join(idents, "."), len(idents))
	}
}

func mutateOperationFromToken(tok *token.Token) (string, error) {
	for _, op := range mutateOperations {
		if tok.IsKeywordLike(op) {
			return op, nil
		}
	}
	if tok.Kind == token.TokenIdent && strings.HasPrefix(tok.Raw, "`") {
		return "", fmt.Errorf("MUTATE operation must be an unquoted keyword, got %s", tok.Raw)
	}
	return "", fmt.Errorf("invalid MUTATE operation %q", tok.Raw)
}
