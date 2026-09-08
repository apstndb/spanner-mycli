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
	"slices"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
)

// prepareDumpDDLForReplay only changes inline forward foreign keys. The admin
// API can fold ALTER ADD FOREIGN KEY into CREATE TABLE even for a cycle, but
// Spanner requires the referenced table to exist when the FK is created:
// https://cloud.google.com/spanner/docs/foreign-keys/overview
// Install moved constraints after all CREATEs and BEFORE data, not after data
// loading. TABLES dumps do not use this DDL transformation.
func prepareDumpDDLForReplay(statements []string) ([]string, error) {
	return recoverMemefishParserPanic(func() ([]string, error) {
		created := make(map[tableID]bool)
		result := make([]string, 0, len(statements))
		type deferredFK struct {
			target tableID
			sql    string
		}
		var deferred []deferredFK
		for _, statement := range statements {
			lexer := &memefish.Lexer{File: &token.File{Buffer: statement}}
			if err := lexer.NextToken(); err != nil {
				return nil, err
			}
			isCreate := strings.EqualFold(lexer.Token.Raw, "CREATE")
			if isCreate {
				if err := lexer.NextToken(); err != nil {
					return nil, err
				}
			}
			if !isCreate || !strings.EqualFold(lexer.Token.Raw, "TABLE") {
				result = append(result, statement)
				continue
			}
			ddl, err := parseMemefishDDL("dump-replay", statement)
			if err != nil {
				// A future non-FK column/option syntax must not make a dump
				// depend on full parser support. A lexical prefix and absence
				// of FOREIGN KEY are sufficient to leave this statement raw.
				id, hasFK, inspectErr := inspectDumpCreateTable(statement)
				if inspectErr == nil && !hasFK {
					key := dumpDDLTableKey(id)
					if created[key] {
						return nil, fmt.Errorf("duplicate CREATE TABLE for %s in dump DDL", id.FQN())
					}
					created[key] = true
					result = append(result, statement)
					continue
				}
				return nil, fmt.Errorf("prepare DUMP table DDL for replay: %w", err)
			}
			ct, ok := ddl.(*ast.CreateTable)
			if !ok {
				return nil, fmt.Errorf("expected CREATE TABLE in dump DDL")
			}
			id, err := tableIDFromPath(ct.Name)
			if err != nil {
				return nil, err
			}
			key := dumpDDLTableKey(id)
			if created[key] {
				return nil, fmt.Errorf("duplicate CREATE TABLE for %s in dump DDL", id.FQN())
			}
			// A self-reference is valid in CREATE TABLE. Synonyms name the
			// same already-created table, including inside named schemas.
			created[key] = true
			for _, synonym := range ct.Synonyms {
				created[dumpDDLTableKey(tableID{Schema: id.Schema, Name: synonym.Name.Name})] = true
			}
			var moved []*ast.TableConstraint
			for _, constraint := range ct.TableConstraints {
				fk, ok := constraint.Constraint.(*ast.ForeignKey)
				if !ok {
					continue
				}
				target, err := tableIDFromPath(fk.ReferenceTable)
				if err != nil {
					return nil, err
				}
				if created[dumpDDLTableKey(target)] {
					continue
				}
				start, end := int(constraint.Pos()), int(constraint.End())
				if start < 0 || end <= start || end > len(statement) {
					return nil, fmt.Errorf("invalid FK source range for %s", id.FQN())
				}
				moved = append(moved, constraint)
				deferred = append(deferred, deferredFK{
					target: target,
					sql:    "ALTER TABLE " + statement[ct.Name.Pos():ct.Name.End()] + " ADD " + statement[start:end],
				})
			}
			if len(moved) > 0 {
				statement, err = removeDumpInlineFKs(statement, ct, moved)
				if err != nil {
					return nil, fmt.Errorf("prepare DUMP table %s: %w", id.FQN(), err)
				}
			}
			result = append(result, statement)
		}
		for _, fk := range deferred {
			if !created[dumpDDLTableKey(fk.target)] {
				return nil, fmt.Errorf("dump DDL foreign key references missing CREATE TABLE %s", fk.target.FQN())
			}
			result = append(result, fk.sql)
		}
		return result, nil
	})
}

func dumpDDLTableKey(id tableID) tableID {
	return tableID{Schema: strings.ToLower(id.Schema), Name: strings.ToLower(id.Name)}
}

// The caller has already established a CREATE TABLE prefix. This fallback
// does not validate the table definition; Spanner supplied the DDL. It only
// identifies the created table and determines whether an FK rewrite might be
// needed. Strings, quoted identifiers and comments are not keyword tokens.
func inspectDumpCreateTable(sql string) (tableID, bool, error) {
	p := newParser("dump-replay-prefix", sql)
	for range 3 {
		if err := p.NextToken(); err != nil {
			return tableID{}, false, err
		}
	}
	if strings.EqualFold(p.Token.Raw, "IF") {
		for _, word := range []string{"IF", "NOT", "EXISTS"} {
			if !strings.EqualFold(p.Token.Raw, word) {
				return tableID{}, false, fmt.Errorf("invalid CREATE TABLE prefix")
			}
			if err := p.NextToken(); err != nil {
				return tableID{}, false, err
			}
		}
	}
	parts, err := parseFQNParts(p)
	if err != nil {
		return tableID{}, false, err
	}
	id, err := tableIDFromIdents(parts)
	if err != nil {
		return tableID{}, false, err
	}
	foreign := false
	for p.Token.Kind != token.TokenEOF {
		if foreign && strings.EqualFold(p.Token.Raw, "KEY") {
			return id, true, nil
		}
		foreign = strings.EqualFold(p.Token.Raw, "FOREIGN")
		if err := p.NextToken(); err != nil {
			return tableID{}, false, err
		}
	}
	return id, false, nil
}

// Remove only each FK's source bytes and one adjacent comma, preserving all
// other declarations, options, interleave clauses and surrounding comments.
// AST SQL() would reserialize unrelated features. Token ranges avoid guessing
// comma positions inside expressions, quoted identifiers or comments.
func removeDumpInlineFKs(sql string, ct *ast.CreateTable, moved []*ast.TableConstraint) (string, error) {
	var declarations []ast.Node
	for _, col := range ct.Columns {
		declarations = append(declarations, col)
	}
	for _, constraint := range ct.TableConstraints {
		declarations = append(declarations, constraint)
	}
	for _, synonym := range ct.Synonyms {
		declarations = append(declarations, synonym)
	}
	slices.SortFunc(declarations, func(a, b ast.Node) int { return int(a.Pos() - b.Pos()) })
	type sourceRange struct{ start, end int }
	var cuts []sourceRange
	for _, fk := range moved {
		i := slices.IndexFunc(declarations, func(node ast.Node) bool { return node == fk })
		if i < 0 || len(declarations) < 2 {
			return "", fmt.Errorf("FK has no adjacent table declaration")
		}
		start, end := int(fk.Pos()), int(fk.End())
		cuts = append(cuts, sourceRange{start, end})
		left, right := end, 0
		if i+1 < len(declarations) {
			right = int(declarations[i+1].Pos())
		} else {
			// Prefer a trailing comma when present. Adjacent removed FKs
			// can otherwise select the same preceding comma twice, leaving
			// two commas after the last surviving column.
			trailing := &memefish.Lexer{File: &token.File{Buffer: sql[end:ct.Rparen]}}
			if err := trailing.NextToken(); err != nil {
				return "", err
			}
			if trailing.Token.Kind == "," {
				right = int(ct.Rparen)
			} else {
				left, right = int(declarations[i-1].End()), start
			}
		}
		if left < 0 || right < left || right > len(sql) {
			return "", fmt.Errorf("invalid FK delimiter range")
		}
		lexer := &memefish.Lexer{File: &token.File{Buffer: sql[left:right]}}
		if err := lexer.NextToken(); err != nil {
			return "", err
		}
		if lexer.Token.Kind != "," {
			return "", fmt.Errorf("missing comma adjacent to FK")
		}
		comma := left + int(lexer.Token.Pos)
		cuts = append(cuts, sourceRange{comma, comma + 1})
	}
	slices.SortFunc(cuts, func(a, b sourceRange) int { return a.start - b.start })
	var out strings.Builder
	previous := 0
	for _, cut := range cuts {
		if cut.start > previous {
			out.WriteString(sql[previous:cut.start])
		}
		previous = max(previous, cut.end)
	}
	out.WriteString(sql[previous:])
	result := out.String()
	if _, err := parseMemefishDDL("dump-replay-rewritten", result); err != nil {
		return "", fmt.Errorf("rewritten CREATE TABLE is invalid: %w", err)
	}
	return result, nil
}
