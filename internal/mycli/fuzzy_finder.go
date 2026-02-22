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
	"log/slog"
	"regexp"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/hymkor/go-multiline-ny"
	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/nyaosorg/go-readline-ny"
)

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
	// Use current line buffer, not the full multiline text.
	// B is the buffer for the current line only, so argStartPos must be relative to it.
	input := B.String()
	result := detectFuzzyContext(input)
	if result.contextType == "" {
		return readline.CONTINUE
	}

	candidates, err := f.fetchCandidates(ctx, result.contextType)
	if err != nil {
		slog.Debug("fuzzy finder: failed to fetch candidates", "context", result.contextType, "err", err)
		return readline.CONTINUE
	}
	if len(candidates) == 0 {
		return readline.CONTINUE
	}

	// Terminal handoff: move cursor below editor, run fzf, then restore
	rewind := f.editor.GotoEndLine()

	opts := []fuzzyfinder.Option{}
	if result.argPrefix != "" {
		opts = append(opts, fuzzyfinder.WithQuery(result.argPrefix))
	}

	idx, err := fuzzyfinder.Find(candidates, func(i int) string {
		return candidates[i]
	}, opts...)

	rewind()
	B.RepaintLastLine()

	if err != nil {
		// User cancelled (Escape/Ctrl+C) or other error
		return readline.CONTINUE
	}

	selected := candidates[idx]

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

// fuzzyContextType represents what kind of candidates to provide.
type fuzzyContextType = string

const (
	fuzzyContextDatabase fuzzyContextType = "database"
)

// fuzzyContextResult holds the detected context, the argument prefix typed so far,
// and the buffer position where the argument starts.
type fuzzyContextResult struct {
	contextType fuzzyContextType
	argPrefix   string // partial argument already typed (used as initial fzf query)
	argStartPos int    // position in the current line buffer where the argument starts (in runes)
}

// useContextRe matches "USE" followed by optional whitespace and captures any partial argument.
var useContextRe = regexp.MustCompile(`(?i)^\s*USE(\s+(\S*))?$`)

// detectFuzzyContext analyzes the current editor buffer to determine
// what kind of fuzzy completion is appropriate.
func detectFuzzyContext(input string) fuzzyContextResult {
	if m := useContextRe.FindStringSubmatch(input); m != nil {
		argPrefix := m[2] // may be empty
		// argStartPos: position after "USE " in runes.
		// Find where the argument starts by locating USE + whitespace.
		argStart := len([]rune(input)) - len([]rune(argPrefix))
		return fuzzyContextResult{
			contextType: fuzzyContextDatabase,
			argPrefix:   argPrefix,
			argStartPos: argStart,
		}
	}
	return fuzzyContextResult{}
}

// fetchCandidates returns completion candidates for the given context type.
func (f *fuzzyFinderCommand) fetchCandidates(ctx context.Context, ctxType fuzzyContextType) ([]string, error) {
	switch ctxType {
	case fuzzyContextDatabase:
		return f.fetchDatabaseCandidates(ctx)
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
