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
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/hymkor/go-multiline-ny"
	fzf "github.com/junegunn/fzf/src"
	"github.com/nyaosorg/go-readline-ny"
	"google.golang.org/api/iterator"
)

const (
	fuzzyFetchTimeout = 10 * time.Second
	fuzzyCacheTTL     = 30 * time.Second
)

// fuzzyCacheEntry holds cached completion candidates with expiry metadata.
type fuzzyCacheEntry struct {
	candidates       []fzfItem
	expiresAt        time.Time
	session          *Session // session when cache was populated
	schemaGeneration uint64   // schema generation when cache was populated
}

// valid reports whether the cache entry is still usable for the given session.
// If checkSchema is true, the schema generation is also compared.
func (e *fuzzyCacheEntry) valid(session *Session, checkSchema bool) bool {
	if e == nil || e.session != session {
		return false
	}
	if time.Now().After(e.expiresAt) {
		return false
	}
	if checkSchema && e.schemaGeneration != session.SchemaGeneration() {
		return false
	}
	return true
}

// fzfItem represents a fuzzy finder candidate with separate display and insert texts.
// When Label is empty, Value is used for both display and insertion.
type fzfItem struct {
	Value string // inserted into buffer
	Label string // displayed/searched in fzf; empty means use Value
}

// toFzfItems converts a plain string slice to fzfItems where Value == Label.
func toFzfItems(ss []string) []fzfItem {
	items := make([]fzfItem, len(ss))
	for i, s := range ss {
		items[i] = fzfItem{Value: s}
	}
	return items
}

// fuzzyFinderCommand implements readline.Command for the fuzzy finder feature.
// It detects the current input context and launches a fuzzy finder with
// appropriate candidates. The selected value replaces the current argument
// (completion-style behavior).
type fuzzyFinderCommand struct {
	editor *multiline.Editor
	cli    *Cli

	// Caches for network-dependent candidates.
	databaseCache     *fuzzyCacheEntry
	tableCache        *fuzzyCacheEntry
	viewCache         *fuzzyCacheEntry
	indexCache        *fuzzyCacheEntry
	changeStreamCache *fuzzyCacheEntry
	sequenceCache     *fuzzyCacheEntry
	modelCache        *fuzzyCacheEntry
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
		if strings.HasSuffix(chosen, " ") {
			selected = chosen
		} else {
			selected = chosen + ";"
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
	case fuzzyCompleteRole:
		return "Database Roles"
	case fuzzyCompleteOperation:
		return "Operations"
	case fuzzyCompleteView:
		return "Views"
	case fuzzyCompleteIndex:
		return "Indexes"
	case fuzzyCompleteChangeStream:
		return "Change Streams"
	case fuzzyCompleteSequence:
		return "Sequences"
	case fuzzyCompleteModel:
		return "Models"
	default:
		return "Statements"
	}
}

// fzfDelimiter is the separator between Value and Label in fzf input lines.
// Uses a non-printable control character (SOH) to avoid collision with actual data.
const fzfDelimiter = "\x01"

// fzfPrepared holds the output of prepareFzfOptions: the fzf arguments,
// the formatted input lines, and whether Value/Label separation is active.
type fzfPrepared struct {
	args           []string
	formattedLines []string
	hasLabels      bool
}

// prepareFzfOptions is a pure function that deterministically computes fzf arguments
// and formatted input lines from candidates. This enables unit testing without TTY.
// The header parameter affects height calculation (adds 1 line if non-empty).
func prepareFzfOptions(candidates []fzfItem, header string) fzfPrepared {
	// Determine if any candidate needs display/insert separation.
	hasLabels := false
	for _, c := range candidates {
		if c.Label != "" && c.Label != c.Value {
			hasLabels = true
			break
		}
	}

	// Check if any candidate label contains newlines for multiline display.
	// Also count total display lines for dynamic height calculation.
	hasMultiline := false
	totalDisplayLines := 0
	for _, c := range candidates {
		display := c.Label
		if display == "" {
			display = c.Value
		}
		lines := strings.Count(display, "\n") + 1
		if lines > 1 {
			hasMultiline = true
		}
		totalDisplayLines += lines
	}

	// For multiline items, use fixed height (shrink-to-fit ~N breaks multiline rendering).
	// For single-line items, use shrink-to-fit ~N for compact display.
	height := "~20"
	if hasMultiline {
		extra := 4 // border(2) + prompt(1) + separator(1)
		if header != "" {
			extra++
		}
		gaps := len(candidates) - 1 // --gap adds 1 line between items
		h := min(totalDisplayLines+gaps+extra, 20)
		height = fmt.Sprintf("%d", h)
	}
	fzfArgs := []string{
		"--reverse", "--no-sort",
		"--height=" + height,
		"--border=rounded",
		"--info=inline-right",
	}
	if hasLabels {
		fzfArgs = append(fzfArgs, "--delimiter="+fzfDelimiter, "--with-nth=2..")
	}
	if hasMultiline {
		fzfArgs = append(fzfArgs, "--read0", "--multi-line", "--gap")
	}

	// Build the formatted lines.
	formattedLines := make([]string, len(candidates))
	for i, c := range candidates {
		if hasLabels {
			label := c.Label
			if label == "" {
				label = c.Value
			}
			formattedLines[i] = c.Value + fzfDelimiter + label
		} else {
			formattedLines[i] = c.Value
		}
	}

	return fzfPrepared{
		args:           fzfArgs,
		formattedLines: formattedLines,
		hasLabels:      hasLabels,
	}
}

// extractValue extracts the Value portion from a fzf output line.
// When hasLabels is true, the line format is "Value\tLabel" and we extract the Value.
// When hasLabels is false, the entire line is the Value.
func extractValue(line string, hasLabels bool) string {
	if hasLabels {
		if v, _, ok := strings.Cut(line, fzfDelimiter); ok {
			return v
		}
	}
	return line
}

// runFzf runs the fzf fuzzy finder with the given candidates and optional initial query.
// When candidates have separate Label and Value, fzf displays/searches the Label
// but the returned string is the Value (for insertion into the buffer).
// Returns the selected Value and true, or ("", false) if cancelled or no match.
func runFzf(candidates []fzfItem, query string, header string) (string, bool) {
	prepared := prepareFzfOptions(candidates, header)

	opts, err := fzf.ParseOptions(false, prepared.args)
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

	inputChan := make(chan string, len(prepared.formattedLines))
	for _, line := range prepared.formattedLines {
		inputChan <- line
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

	if selected {
		result = extractValue(result, prepared.hasLabels)
	}

	return result, selected
}

// runFzfFilter runs fzf in non-interactive --filter mode for testing.
// It returns all matching Values (after Value/Label extraction).
// This tests the full pipeline: formatting → fzf matching → result extraction.
func runFzfFilter(candidates []fzfItem, filter string, header string) []string {
	prepared := prepareFzfOptions(candidates, header)

	// For filter mode, strip visual-only args and add --filter.
	filterArgs := make([]string, 0, len(prepared.args)+1)
	for _, arg := range prepared.args {
		// Skip args that don't apply in non-interactive filter mode.
		if strings.HasPrefix(arg, "--height") ||
			strings.HasPrefix(arg, "--border") ||
			strings.HasPrefix(arg, "--reverse") ||
			strings.HasPrefix(arg, "--info") ||
			strings.HasPrefix(arg, "--gap") {
			continue
		}
		filterArgs = append(filterArgs, arg)
	}
	filterArgs = append(filterArgs, "--filter="+filter)

	opts, err := fzf.ParseOptions(false, filterArgs)
	if err != nil {
		return nil
	}
	if header != "" {
		opts.Header = []string{header}
	}

	inputChan := make(chan string, len(prepared.formattedLines))
	for _, line := range prepared.formattedLines {
		inputChan <- line
	}
	close(inputChan)
	opts.Input = inputChan

	var results []string
	opts.Printer = func(s string) {
		results = append(results, extractValue(s, prepared.hasLabels))
	}

	_, _ = fzf.Run(opts)
	return results
}

// buildStatementNameItems builds fzfItem candidates from all client-side statement defs.
func buildStatementNameItems() []fzfItem {
	var items []fzfItem
	for _, def := range clientSideStatementDefs {
		for _, desc := range def.Descriptions {
			if desc.Syntax == "" {
				continue
			}
			insertText := extractFixedPrefix(desc.Syntax)
			items = append(items, fzfItem{
				Value: insertText,
				Label: desc.Syntax,
			})
		}
	}
	return items
}

// statementNameItems is built at init time from clientSideStatementDefs.
var statementNameItems []fzfItem

func init() {
	statementNameItems = buildStatementNameItems()
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
	case fuzzyCompleteDatabase, fuzzyCompleteTable, fuzzyCompleteRole, fuzzyCompleteOperation,
		fuzzyCompleteView, fuzzyCompleteIndex, fuzzyCompleteChangeStream, fuzzyCompleteSequence, fuzzyCompleteModel:
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

// getCachedCandidates returns cached candidates if the cache is valid, or nil.
func (f *fuzzyFinderCommand) getCachedCandidates(ct fuzzyCompletionType) []fzfItem {
	session := f.cli.SessionHandler.GetSession()
	switch ct {
	case fuzzyCompleteDatabase:
		if f.databaseCache.valid(session, false) {
			return f.databaseCache.candidates
		}
	case fuzzyCompleteTable:
		if f.tableCache.valid(session, true) {
			return f.tableCache.candidates
		}
	case fuzzyCompleteView:
		if f.viewCache.valid(session, true) {
			return f.viewCache.candidates
		}
	case fuzzyCompleteIndex:
		if f.indexCache.valid(session, true) {
			return f.indexCache.candidates
		}
	case fuzzyCompleteChangeStream:
		if f.changeStreamCache.valid(session, true) {
			return f.changeStreamCache.candidates
		}
	case fuzzyCompleteSequence:
		if f.sequenceCache.valid(session, true) {
			return f.sequenceCache.candidates
		}
	case fuzzyCompleteModel:
		if f.modelCache.valid(session, true) {
			return f.modelCache.candidates
		}
	}
	return nil
}

// setCachedCandidates stores candidates in the appropriate cache.
func (f *fuzzyFinderCommand) setCachedCandidates(ct fuzzyCompletionType, candidates []fzfItem) {
	session := f.cli.SessionHandler.GetSession()
	if session == nil {
		return
	}
	entry := &fuzzyCacheEntry{
		candidates:       candidates,
		expiresAt:        time.Now().Add(fuzzyCacheTTL),
		session:          session,
		schemaGeneration: session.SchemaGeneration(),
	}
	switch ct {
	case fuzzyCompleteDatabase:
		f.databaseCache = entry
	case fuzzyCompleteTable:
		f.tableCache = entry
	case fuzzyCompleteView:
		f.viewCache = entry
	case fuzzyCompleteIndex:
		f.indexCache = entry
	case fuzzyCompleteChangeStream:
		f.changeStreamCache = entry
	case fuzzyCompleteSequence:
		f.sequenceCache = entry
	case fuzzyCompleteModel:
		f.modelCache = entry
	}
}

// resolveCandidates returns candidates for the given completion type.
// For statement name completion (ct == 0), returns pre-built items.
// For network-dependent types, checks cache first, then shows a loading indicator and fetches.
func (f *fuzzyFinderCommand) resolveCandidates(ctx context.Context, ct fuzzyCompletionType, completionContext string) ([]fzfItem, error) {
	if ct == 0 {
		return statementNameItems, nil
	}
	if !requiresNetwork(ct) {
		return f.fetchCandidates(ctx, ct, completionContext)
	}

	// Check cache first.
	if candidates := f.getCachedCandidates(ct); candidates != nil {
		return candidates, nil
	}

	// Cache miss: show loading indicator below the editor.
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

	if err != nil {
		return nil, err
	}

	// Cache the results on success.
	f.setCachedCandidates(ct, candidates)

	return candidates, nil
}

// fetchCandidates returns completion candidates for the given completion type.
func (f *fuzzyFinderCommand) fetchCandidates(ctx context.Context, ct fuzzyCompletionType, completionContext string) ([]fzfItem, error) {
	switch ct {
	case fuzzyCompleteDatabase:
		return f.fetchDatabaseCandidates(ctx)
	case fuzzyCompleteVariable:
		return toFzfItems(f.fetchVariableCandidates()), nil
	case fuzzyCompleteTable:
		return f.fetchTableCandidates(ctx)
	case fuzzyCompleteVariableValue:
		return toFzfItems(f.fetchVariableValueCandidates(completionContext)), nil
	case fuzzyCompleteRole:
		return f.fetchRoleCandidates(ctx, completionContext)
	case fuzzyCompleteOperation:
		return f.fetchOperationCandidates(ctx)
	case fuzzyCompleteView:
		return f.fetchViewCandidates(ctx)
	case fuzzyCompleteIndex:
		return f.fetchIndexCandidates(ctx)
	case fuzzyCompleteChangeStream:
		return f.fetchChangeStreamCandidates(ctx)
	case fuzzyCompleteSequence:
		return f.fetchSequenceCandidates(ctx)
	case fuzzyCompleteModel:
		return f.fetchModelCandidates(ctx)
	default:
		return nil, nil
	}
}

// fetchDatabaseCandidates lists databases from the current instance.
func (f *fuzzyFinderCommand) fetchDatabaseCandidates(ctx context.Context) ([]fzfItem, error) {
	session := f.cli.SessionHandler.GetSession()
	if session == nil || session.adminClient == nil {
		return nil, nil
	}

	dbIter := session.adminClient.ListDatabases(ctx, &databasepb.ListDatabasesRequest{
		Parent: session.InstancePath(),
	})

	var items []fzfItem
	for db, err := range dbIter.All() {
		if err != nil {
			return nil, err
		}
		matched := extractDatabaseRe.FindStringSubmatch(db.GetName())
		if len(matched) > 1 {
			items = append(items, fzfItem{Value: matched[1]})
		}
	}
	return items, nil
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

// extractRoleRe extracts the role name from a full database role resource name.
var extractRoleRe = regexp.MustCompile(`databaseRoles/(.+)`)

// fetchRoleCandidates lists database roles for the given database.
func (f *fuzzyFinderCommand) fetchRoleCandidates(ctx context.Context, database string) ([]fzfItem, error) {
	session := f.cli.SessionHandler.GetSession()
	if session == nil || session.adminClient == nil {
		return nil, nil
	}

	dbPath := session.InstancePath() + "/databases/" + database
	roleIter := session.adminClient.ListDatabaseRoles(ctx, &databasepb.ListDatabaseRolesRequest{
		Parent: dbPath,
	})

	var items []fzfItem
	for role, err := range roleIter.All() {
		if err != nil {
			return nil, err
		}
		matched := extractRoleRe.FindStringSubmatch(role.GetName())
		if len(matched) > 1 {
			items = append(items, fzfItem{Value: matched[1]})
		}
	}
	slices.SortFunc(items, func(a, b fzfItem) int {
		return strings.Compare(a.Value, b.Value)
	})
	return items, nil
}

// fetchSchemaObjectCandidates queries INFORMATION_SCHEMA with 2 columns (schema, name)
// and returns schema-qualified fzfItems. Used by fetchTableCandidates and similar functions.
func (f *fuzzyFinderCommand) fetchSchemaObjectCandidates(ctx context.Context, query, funcName string) ([]fzfItem, error) {
	session := f.cli.SessionHandler.GetSession()
	if session == nil || session.client == nil {
		return nil, nil
	}

	iter := session.client.Single().Query(ctx, spanner.Statement{SQL: query})
	defer iter.Stop()

	var items []fzfItem
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", funcName, err)
		}

		var schema, name string
		if err := row.Columns(&schema, &name); err != nil {
			return nil, fmt.Errorf("%s: %w", funcName, err)
		}

		if schema == "" {
			items = append(items, fzfItem{Value: name})
		} else {
			items = append(items, fzfItem{Value: schema + "." + name})
		}
	}
	return items, nil
}

// fetchTableCandidates lists table names from INFORMATION_SCHEMA.TABLES.
// Returns table names formatted as "schema.name" for non-default schemas, or just "name" for default schema.
func (f *fuzzyFinderCommand) fetchTableCandidates(ctx context.Context) ([]fzfItem, error) {
	return f.fetchSchemaObjectCandidates(ctx,
		`SELECT TABLE_SCHEMA, TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_CATALOG = '' ORDER BY TABLE_SCHEMA, TABLE_NAME`,
		"fetchTableCandidates")
}

// fetchViewCandidates lists views from INFORMATION_SCHEMA.VIEWS.
func (f *fuzzyFinderCommand) fetchViewCandidates(ctx context.Context) ([]fzfItem, error) {
	return f.fetchSchemaObjectCandidates(ctx,
		`SELECT TABLE_SCHEMA, TABLE_NAME FROM INFORMATION_SCHEMA.VIEWS WHERE TABLE_CATALOG = '' ORDER BY TABLE_SCHEMA, TABLE_NAME`,
		"fetchViewCandidates")
}

// fetchIndexCandidates lists indexes from INFORMATION_SCHEMA.INDEXES.
// Excludes primary keys and managed (FK backing) indexes.
func (f *fuzzyFinderCommand) fetchIndexCandidates(ctx context.Context) ([]fzfItem, error) {
	return f.fetchSchemaObjectCandidates(ctx,
		`SELECT TABLE_SCHEMA, INDEX_NAME FROM INFORMATION_SCHEMA.INDEXES WHERE TABLE_CATALOG = '' AND INDEX_TYPE != 'PRIMARY_KEY' AND SPANNER_IS_MANAGED = FALSE ORDER BY TABLE_SCHEMA, INDEX_NAME`,
		"fetchIndexCandidates")
}

// fetchChangeStreamCandidates lists change streams from INFORMATION_SCHEMA.CHANGE_STREAMS.
func (f *fuzzyFinderCommand) fetchChangeStreamCandidates(ctx context.Context) ([]fzfItem, error) {
	return f.fetchSchemaObjectCandidates(ctx,
		`SELECT CHANGE_STREAM_SCHEMA, CHANGE_STREAM_NAME FROM INFORMATION_SCHEMA.CHANGE_STREAMS WHERE CHANGE_STREAM_CATALOG = '' ORDER BY CHANGE_STREAM_SCHEMA, CHANGE_STREAM_NAME`,
		"fetchChangeStreamCandidates")
}

// fetchSequenceCandidates lists sequences from INFORMATION_SCHEMA.SEQUENCES.
func (f *fuzzyFinderCommand) fetchSequenceCandidates(ctx context.Context) ([]fzfItem, error) {
	return f.fetchSchemaObjectCandidates(ctx,
		`SELECT SCHEMA, NAME FROM INFORMATION_SCHEMA.SEQUENCES WHERE CATALOG = '' ORDER BY SCHEMA, NAME`,
		"fetchSequenceCandidates")
}

// fetchModelCandidates lists models from INFORMATION_SCHEMA.MODELS.
func (f *fuzzyFinderCommand) fetchModelCandidates(ctx context.Context) ([]fzfItem, error) {
	return f.fetchSchemaObjectCandidates(ctx,
		`SELECT MODEL_SCHEMA, MODEL_NAME FROM INFORMATION_SCHEMA.MODELS WHERE MODEL_CATALOG = '' ORDER BY MODEL_SCHEMA, MODEL_NAME`,
		"fetchModelCandidates")
}

// fetchOperationCandidates lists DDL operations from the current database.
// Each candidate's Value is the operation ID (short form) and Label shows the DDL statements and status.
func (f *fuzzyFinderCommand) fetchOperationCandidates(ctx context.Context) ([]fzfItem, error) {
	session := f.cli.SessionHandler.GetSession()
	if session == nil || session.adminClient == nil {
		return nil, nil
	}

	var items []fzfItem
	for op, err := range session.adminClient.ListOperations(ctx, &longrunningpb.ListOperationsRequest{
		Name: session.DatabasePath() + "/operations",
	}).All() {
		if err != nil {
			return nil, err
		}

		if op.GetMetadata().GetTypeUrl() != "type.googleapis.com/google.spanner.admin.database.v1.UpdateDatabaseDdlMetadata" {
			continue
		}

		var md databasepb.UpdateDatabaseDdlMetadata
		if err := op.GetMetadata().UnmarshalTo(&md); err != nil {
			continue
		}

		// Extract short operation ID from the full name.
		parts := strings.Split(op.GetName(), "/")
		opID := parts[len(parts)-1]

		// Build label from DDL statements with trailing semicolons.
		label := strings.Join(md.GetStatements(), ";\n") + ";"
		items = append(items, fzfItem{Value: opID, Label: label})
	}
	return items, nil
}
