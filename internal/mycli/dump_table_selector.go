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
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
)

var (
	errDumpTablesMissingSelection = errors.New("DUMP TABLES requires a table list or LIKE/EXCEPT selector")
	errDumpTablesNoMatch          = errors.New("DUMP TABLES: no base tables matched LIKE/EXCEPT")
	errDumpTablesMixedSelector    = errors.New("DUMP TABLES: exact list and LIKE/EXCEPT cannot be combined")
)

// dumpTableSelector is a parsed LIKE/EXCEPT filter. Empty string elements are
// real patterns. Values are decoded GoogleSQL STRING literals.
type dumpTableSelector struct {
	Like   []string
	Except []string
}

const dumpTableDisplayIdentityExpr = "IF(TABLE_SCHEMA = '', TABLE_NAME, CONCAT(TABLE_SCHEMA, '.', TABLE_NAME))"

func dumpTablesUnquotedKeyword(tok token.Token, word string) bool {
	if tok.Raw != "" && strings.HasPrefix(tok.Raw, "`") {
		return false
	}
	return tokenKeywordLike(tok, word)
}

func parseDumpTablesTail(input string) (*DumpTablesStatement, error) {
	p := newParser("", input)
	if err := p.NextToken(); err != nil {
		return nil, err
	}
	if dumpTablesUnquotedKeyword(p.Token, "LIKE") || dumpTablesUnquotedKeyword(p.Token, "EXCEPT") {
		sel, err := parseDumpTableSelector(p)
		if err != nil {
			return nil, err
		}
		return &DumpTablesStatement{Selector: sel}, nil
	}
	tables, err := parseDumpTableIDList(input)
	if err != nil {
		return nil, err
	}
	return &DumpTablesStatement{Tables: tables}, nil
}

func parseDumpTableSelector(p *memefish.Parser) (*dumpTableSelector, error) {
	sel := &dumpTableSelector{}
	switch {
	case dumpTablesUnquotedKeyword(p.Token, "LIKE"):
		if err := p.NextToken(); err != nil {
			return nil, err
		}
		like, err := parseDumpPatternStringList(p)
		if err != nil {
			return nil, fmt.Errorf("LIKE: %w", err)
		}
		sel.Like = like
		if dumpTablesUnquotedKeyword(p.Token, "EXCEPT") {
			if err := p.NextToken(); err != nil {
				return nil, err
			}
			except, err := parseDumpPatternStringList(p)
			if err != nil {
				return nil, fmt.Errorf("EXCEPT: %w", err)
			}
			sel.Except = except
		}
	case dumpTablesUnquotedKeyword(p.Token, "EXCEPT"):
		if err := p.NextToken(); err != nil {
			return nil, err
		}
		except, err := parseDumpPatternStringList(p)
		if err != nil {
			return nil, fmt.Errorf("EXCEPT: %w", err)
		}
		sel.Except = except
	default:
		return nil, fmt.Errorf("expected LIKE or EXCEPT")
	}
	if p.Token.Kind != token.TokenEOF {
		return nil, fmt.Errorf("unexpected input %q after DUMP TABLES selector", p.Token.Raw)
	}
	return sel, nil
}

func parseDumpPatternStringList(p *memefish.Parser) ([]string, error) {
	var patterns []string
	for {
		s, err := parseDumpPatternString(p)
		if err != nil {
			if len(patterns) == 0 {
				return nil, err
			}
			return nil, fmt.Errorf("expected GoogleSQL string literal after comma: %w", err)
		}
		patterns = append(patterns, s)
		switch p.Token.Kind {
		case ",":
			if err := p.NextToken(); err != nil {
				return nil, err
			}
			if p.Token.Kind == token.TokenEOF {
				return nil, fmt.Errorf("trailing comma in pattern list")
			}
		default:
			return patterns, nil
		}
	}
}

func parseDumpPatternString(p *memefish.Parser) (string, error) {
	if p.Token.Kind != token.TokenString {
		if p.Token.Kind == token.TokenEOF {
			return "", fmt.Errorf("expected GoogleSQL string literal, but got end of input")
		}
		return "", fmt.Errorf("expected GoogleSQL string literal, but got %q", p.Token.Raw)
	}
	expr, err := parseMemefishExpr("", p.Token.Raw)
	if err != nil {
		return "", fmt.Errorf("invalid GoogleSQL string literal %s: %w", p.Token.Raw, err)
	}
	lit, ok := expr.(*ast.StringLiteral)
	if !ok {
		return "", fmt.Errorf("expected GoogleSQL string literal, but got %T", expr)
	}
	if err := p.NextToken(); err != nil {
		return "", err
	}
	return lit.Value, nil
}

func buildDumpTableSelectorQuery(sel *dumpTableSelector) (spanner.Statement, error) {
	if sel == nil || (len(sel.Like) == 0 && len(sel.Except) == 0) {
		return spanner.Statement{}, errDumpTablesMissingSelection
	}
	params := make(map[string]interface{}, len(sel.Like)+len(sel.Except))
	likePred := "TRUE"
	if len(sel.Like) > 0 {
		parts := make([]string, 0, len(sel.Like))
		for i, pat := range sel.Like {
			name := fmt.Sprintf("like%d", i)
			params[name] = pat
			parts = append(parts, dumpTableDisplayIdentityExpr+" LIKE @"+name)
		}
		likePred = "(" + strings.Join(parts, " OR ") + ")"
	}
	exceptPred := "FALSE"
	if len(sel.Except) > 0 {
		parts := make([]string, 0, len(sel.Except))
		for i, pat := range sel.Except {
			name := fmt.Sprintf("except%d", i)
			params[name] = pat
			parts = append(parts, dumpTableDisplayIdentityExpr+" LIKE @"+name)
		}
		exceptPred = "(" + strings.Join(parts, " OR ") + ")"
	}
	return spanner.Statement{
		SQL: fmt.Sprintf(`
		SELECT TABLE_SCHEMA, TABLE_NAME
		FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_TYPE = 'BASE TABLE'
		  AND TABLE_SCHEMA NOT IN ('INFORMATION_SCHEMA', 'information_schema', 'SPANNER_SYS')
		  AND %s
		  AND NOT (%s)
		ORDER BY TABLE_SCHEMA, TABLE_NAME`, likePred, exceptPred),
		Params: params,
	}, nil
}

func resolveDumpTableSelector(ctx context.Context, txn *spanner.ReadOnlyTransaction, sel *dumpTableSelector, dro *sppb.DirectedReadOptions) ([]tableID, error) {
	stmt, err := buildDumpTableSelectorQuery(sel)
	if err != nil {
		return nil, err
	}
	iter := queryWithDirectedRead(ctx, txn, stmt, dro)
	defer iter.Stop()
	var ids []tableID
	err = iter.Do(func(r *spanner.Row) error {
		var schema, name string
		if err := r.Columns(&schema, &name); err != nil {
			return err
		}
		if userSchemaExcluded(schema) {
			return nil
		}
		ids = append(ids, tableID{Schema: schema, Name: name})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("DUMP TABLES selector: %w", err)
	}
	if len(ids) == 0 {
		return nil, errDumpTablesNoMatch
	}
	return ids, nil
}
