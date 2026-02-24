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
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/hymkor/go-multiline-ny"
	fzf "github.com/junegunn/fzf/src"
	"github.com/nyaosorg/go-readline-ny"
	"google.golang.org/api/iterator"
)

const fuzzyFetchTimeout = 10 * time.Second

// fuzzyFinderCommand implements readline.Command for the fuzzy finder feature.
// It detects the current input context and launches a fuzzy finder with
// appropriate candidates. The selected value replaces the current argument
// (completion-style behavior).
type fuzzyFinderCommand struct {
	editor *multiline.Editor
	cli    *Cli
}

func (f *fuzzyFinderCommand) String() string {
	return "FUZZY_FINDER"
}

// SetEditor is called by go-multiline-ny's BindKey to inject the editor reference.
func (f *fuzzyFinderCommand) SetEditor(e *multiline.Editor) {
	f.editor = e
}

func (f *fuzzyFinderCommand) Call(ctx context.Context, B *readline.Buffer) readline.Result {
	// Use text up to cursor position, not the full line buffer.
	// This ensures completion context depends on where the cursor is,
	// not what follows it (e.g., cursor in middle of "USE db ROLE admin").
	input := B.SubString(0, B.Cursor)
	result := detectFuzzyContext(input)

	// Resolve candidates (may show loading indicator for network fetches).
	candidates, err := f.resolveCandidates(ctx, result.completionType, result.context)
	if err != nil {
		slog.Debug("fuzzy finder: failed to fetch candidates", "completionType", result.completionType, "err", err)
		return readline.CONTINUE
	}
	if len(candidates) == 0 {
		return readline.CONTINUE
	}

	// Terminal handoff: move cursor below editor, run fzf, then restore
	rewind := f.editor.GotoEndLine()

	chosen, ok := runFzf(candidates, result.argPrefix, completionHeader(result.completionType))

	rewind()
	B.RepaintLastLine()

	if !ok {
		// User cancelled (Escape/Ctrl+C) or no match
		return readline.CONTINUE
	}

	var selected string
	if result.completionType != 0 {
		// Argument completion: insert the candidate with optional suffix
		selected = chosen + result.suffix
	} else {
		// Statement name completion: insert the fixed prefix text.
		// No-arg statements (no trailing space) get a semicolon for immediate submit.
		insertText := statementNameInsertText(chosen)
		if strings.HasSuffix(insertText, " ") {
			selected = insertText
		} else {
			selected = insertText + ";"
		}
	}

	// Replace the argument portion: delete from argStartPos to end of buffer,
	// then insert the selected value.
	bufLen := len(B.Buffer)
	if result.argStartPos < bufLen {
		B.Delete(result.argStartPos, bufLen-result.argStartPos)
	}
	B.Cursor = result.argStartPos
	B.InsertAndRepaint(selected)

	return readline.CONTINUE
}

// fuzzyContextResult holds the detected context, the argument prefix typed so far,
// and the buffer position where the argument starts.
type fuzzyContextResult struct {
	completionType fuzzyCompletionType // 0 means statement name completion (fallback)
	argPrefix      string              // partial argument already typed (used as initial fzf query)
	argStartPos    int                 // position in the current line buffer where the argument starts (in runes)
	context        string              // additional context from earlier capture groups (e.g., variable name for value completion)
	suffix         string              // appended after selected candidate (e.g., " = " for SET variable name)
}

// detectFuzzyContext analyzes the current editor buffer to determine
// what kind of fuzzy completion is appropriate.
// Priority: argument completion (if input matches a completable statement) > statement name completion.
func detectFuzzyContext(input string) fuzzyContextResult {
	// Try argument completion first: iterate all defs with Completion entries.
	for _, def := range clientSideStatementDefs {
		for _, comp := range def.Completion {
			loc := comp.PrefixPattern.FindStringSubmatchIndex(input)
			if loc == nil {
				continue
			}
			// Use the last capture group as argPrefix, earlier groups as context.
			numGroups := len(loc)/2 - 1
			lastIdx := numGroups * 2
			argPrefix := input[loc[lastIdx]:loc[lastIdx+1]]
			argStart := utf8.RuneCountInString(input[:loc[lastIdx]])

			var context string
			if numGroups > 1 {
				context = input[loc[2]:loc[3]]
			}

			return fuzzyContextResult{
				completionType: comp.CompletionType,
				argPrefix:      argPrefix,
				argStartPos:    argStart,
				context:        context,
				suffix:         comp.Suffix,
			}
		}
	}

	// Fallback: statement name completion.
	// argPrefix is the trimmed input; argStartPos is after leading spaces.
	trimmed := strings.TrimLeftFunc(input, unicode.IsSpace)
	leadingSpaces := len([]rune(input)) - len([]rune(trimmed))
	return fuzzyContextResult{
		completionType: 0, // statement name completion
		argPrefix:      trimmed,
		argStartPos:    leadingSpaces,
	}
}

// statementNameCandidate holds the display and insert text for statement name completion.
type statementNameCandidate struct {
	DisplayText string // shown in fzf (e.g., "SHOW COLUMNS FROM <table_fqn>")
	InsertText  string // inserted into buffer (e.g., "SHOW COLUMNS FROM ")
}

// statementNameCandidates is built at init time from clientSideStatementDefs.
var statementNameCandidates []statementNameCandidate

// statementNameInsertMap maps display text to insert text for statement name completion.
// fzf returns the selected string (not an index), so we need this lookup.
var statementNameInsertMap map[string]string

func init() {
	statementNameCandidates = buildStatementNameCandidates()
	statementNameInsertMap = make(map[string]string, len(statementNameCandidates))
	for _, c := range statementNameCandidates {
		statementNameInsertMap[c.DisplayText] = c.InsertText
	}
}

// statementNameInsertText returns the insert text for a given display text.
// Falls back to the display text itself if not found in the map.
func statementNameInsertText(displayText string) string {
	if t, ok := statementNameInsertMap[displayText]; ok {
		return t
	}
	return displayText
}

// completionHeader returns a header string for the fzf UI based on completion type.
func completionHeader(ct fuzzyCompletionType) string {
	switch ct {
	case fuzzyCompleteDatabase:
		return "Databases"
	case fuzzyCompleteVariable:
		return "System Variables"
	case fuzzyCompleteTable:
		return "Tables"
	case fuzzyCompleteVariableValue:
		return "Variable Values"
	default:
		return "Statements"
	}
}

// runFzf runs the fzf fuzzy finder with the given candidates and optional initial query.
// Returns the selected string and true, or ("", false) if cancelled or no match.
func runFzf(candidates []string, query string, header string) (string, bool) {
	opts, err := fzf.ParseOptions(false, []string{
		"--reverse", "--no-sort",
		"--height=~20",
		"--border=rounded",
		"--info=inline-right",
	})
	if err != nil {
		slog.Debug("fuzzy finder: parse fzf options", "err", err)
		return "", false
	}
	if query != "" {
		opts.Query = query
	}
	if header != "" {
		opts.Header = []string{header}
	}
	opts.ForceTtyIn = true

	inputChan := make(chan string, len(candidates))
	for _, c := range candidates {
		inputChan <- c
	}
	close(inputChan)
	opts.Input = inputChan

	var result string
	var selected bool
	opts.Printer = func(s string) {
		result = s
		selected = true
	}

	code, err := fzf.Run(opts)
	if err != nil && code != fzf.ExitInterrupt && code != fzf.ExitNoMatch {
		slog.Debug("fuzzy finder: fzf run error", "code", code, "err", err)
	}
	return result, selected
}

// statementNameDisplayTexts returns the display texts for fzf.
func statementNameDisplayTexts() []string {
	texts := make([]string, len(statementNameCandidates))
	for i, c := range statementNameCandidates {
		texts[i] = c.DisplayText
	}
	return texts
}

// buildStatementNameCandidates builds the candidate list from all client-side statement defs.
func buildStatementNameCandidates() []statementNameCandidate {
	var candidates []statementNameCandidate
	for _, def := range clientSideStatementDefs {
		for _, desc := range def.Descriptions {
			if desc.Syntax == "" {
				continue
			}
			insertText := extractFixedPrefix(desc.Syntax)
			candidates = append(candidates, statementNameCandidate{
				DisplayText: desc.Syntax,
				InsertText:  insertText,
			})
		}
	}
	return candidates
}

// extractFixedPrefix walks words in a syntax string until it hits a placeholder
// indicator (<, [, {, or ...), returning the keyword prefix.
// For no-arg statements, returns the full syntax (no trailing space).
// For statements with args, returns the keyword prefix with a trailing space.
func extractFixedPrefix(syntax string) string {
	words := strings.Fields(syntax)
	var fixed []string
	for _, w := range words {
		if len(w) > 0 && (w[0] == '<' || w[0] == '[' || w[0] == '{' || strings.HasPrefix(w, "...")) {
			break
		}
		fixed = append(fixed, w)
	}
	if len(fixed) == len(words) {
		// No-arg statement: return full text without trailing space.
		return strings.Join(fixed, " ")
	}
	// Has args: return prefix with trailing space.
	return strings.Join(fixed, " ") + " "
}

// requiresNetwork reports whether the completion type requires a network call.
func requiresNetwork(ct fuzzyCompletionType) bool {
	switch ct {
	case fuzzyCompleteDatabase, fuzzyCompleteTable:
		return true
	default:
		return false
	}
}

// unwrapCustomVar extracts the base Variable from a CustomVar, if applicable.
// This is needed because variables wrapped in CustomVar (e.g., for custom setters)
// still have their original type implementing ValidValuesEnumerator on the base.
func unwrapCustomVar(v Variable) Variable {
	if cv, ok := v.(*CustomVar); ok {
		return cv.base
	}
	return v
}

// resolveCandidates returns candidates for the given completion type.
// For statement name completion (ct == 0), returns pre-built display texts.
// For network-dependent types, shows a loading indicator on the terminal and applies a timeout.
func (f *fuzzyFinderCommand) resolveCandidates(ctx context.Context, ct fuzzyCompletionType, completionContext string) ([]string, error) {
	if ct == 0 {
		return statementNameDisplayTexts(), nil
	}
	if !requiresNetwork(ct) {
		return f.fetchCandidates(ctx, ct, completionContext)
	}

	// Show loading indicator below the editor.
	rewind := f.editor.GotoEndLine()
	out := f.editor.Out()
	fmt.Fprint(out, "Loading...")
	if err := out.Flush(); err != nil {
		slog.Debug("fuzzy finder: flush loading indicator", "err", err)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, fuzzyFetchTimeout)
	defer cancel()

	candidates, err := f.fetchCandidates(fetchCtx, ct, completionContext)

	// Clear loading indicator and restore cursor.
	fmt.Fprint(out, "\r\033[2K")
	if err := out.Flush(); err != nil {
		slog.Debug("fuzzy finder: flush clear loading", "err", err)
	}
	rewind()

	return candidates, err
}

// fetchCandidates returns completion candidates for the given completion type.
func (f *fuzzyFinderCommand) fetchCandidates(ctx context.Context, ct fuzzyCompletionType, completionContext string) ([]string, error) {
	switch ct {
	case fuzzyCompleteDatabase:
		return f.fetchDatabaseCandidates(ctx)
	case fuzzyCompleteVariable:
		return f.fetchVariableCandidates(), nil
	case fuzzyCompleteTable:
		return f.fetchTableCandidates(ctx)
	case fuzzyCompleteVariableValue:
		return f.fetchVariableValueCandidates(completionContext), nil
	default:
		return nil, nil
	}
}

// fetchDatabaseCandidates lists databases from the current instance.
func (f *fuzzyFinderCommand) fetchDatabaseCandidates(ctx context.Context) ([]string, error) {
	session := f.cli.SessionHandler.GetSession()
	if session == nil || session.adminClient == nil {
		return nil, nil
	}

	dbIter := session.adminClient.ListDatabases(ctx, &databasepb.ListDatabasesRequest{
		Parent: session.InstancePath(),
	})

	var databases []string
	for db, err := range dbIter.All() {
		if err != nil {
			return nil, err
		}
		matched := extractDatabaseRe.FindStringSubmatch(db.GetName())
		if len(matched) > 1 {
			databases = append(databases, matched[1])
		}
	}
	return databases, nil
}

// fetchVariableCandidates returns sorted system variable names from the registry.
func (f *fuzzyFinderCommand) fetchVariableCandidates() []string {
	sv := f.cli.SystemVariables
	if sv == nil {
		return nil
	}
	names := slices.Sorted(maps.Keys(sv.ListVariables()))
	return names
}

// fetchVariableValueCandidates returns valid values for a system variable, if enumerable.
// Values are returned as GoogleSQL literals (e.g., 'TABLE' for strings, TRUE for booleans).
// Returns nil if the variable is unknown or doesn't have constrained values.
func (f *fuzzyFinderCommand) fetchVariableValueCandidates(varName string) []string {
	sv := f.cli.SystemVariables
	if sv == nil || sv.Registry == nil {
		return nil
	}

	v := sv.Registry.GetVariable(varName)
	if v == nil {
		return nil
	}

	// Unwrap CustomVar to check the base handler for ValidValuesEnumerator.
	v = unwrapCustomVar(v)

	if enumerator, ok := v.(ValidValuesEnumerator); ok {
		return enumerator.ValidValues()
	}
	return nil
}

// fetchTableCandidates lists table names from INFORMATION_SCHEMA.TABLES.
// Returns table names formatted as "schema.name" for non-default schemas, or just "name" for default schema.
func (f *fuzzyFinderCommand) fetchTableCandidates(ctx context.Context) ([]string, error) {
	session := f.cli.SessionHandler.GetSession()
	if session == nil || session.client == nil {
		return nil, nil
	}

	stmt := spanner.Statement{
		SQL: `SELECT TABLE_SCHEMA, TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_CATALOG = '' ORDER BY TABLE_SCHEMA, TABLE_NAME`,
	}

	iter := session.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var tables []string
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("fetchTableCandidates: %w", err)
		}

		var schema, name string
		if err := row.Columns(&schema, &name); err != nil {
			return nil, fmt.Errorf("fetchTableCandidates: %w", err)
		}

		if schema == "" {
			tables = append(tables, name)
		} else {
			tables = append(tables, schema+"."+name)
		}
	}
	return tables, nil
}
