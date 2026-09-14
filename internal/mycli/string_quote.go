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
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanvalue"
)

// autoNumberLikeRe is the AUTO decimal-number-like grammar from #762.
// Entire-string match only: 123, 0123, +1, 1e3, .5, 1. — not 0xFF or 1+1.
var autoNumberLikeRe = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func isStringQuoteDisplayMode(mode enums.DisplayMode) bool {
	switch mode {
	case enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment, enums.DisplayModeVertical:
		return true
	default:
		return false
	}
}

func quoteDisplayString(s string) string {
	return strconv.Quote(s)
}

func needsAutoStringQuote(s string) bool {
	if s == "" || s != strings.TrimSpace(s) {
		return true
	}
	switch s {
	case "NULL", "null", "true", "false", "NaN", "+Inf", "-Inf":
		return true
	}
	if autoNumberLikeRe.MatchString(s) {
		return true
	}
	for _, r := range s {
		switch r {
		case '"', '\\', ',', '[', ']', '{', '}':
			return true
		}
		if !strconv.IsPrint(r) {
			return true
		}
	}
	return false
}

func stringQuoteModeOf(sysVars *systemVariables) enums.StringQuoteMode {
	if sysVars == nil {
		return enums.StringQuoteModeNone
	}
	return sysVars.Display.StringQuoteMode
}

// applyStringQuoteDisplay returns a display FormatConfig for CLI_STRING_QUOTE_MODE.
// NONE and excluded formats return the input pointer. AUTO and ALWAYS prepend a
// narrow non-NULL STRING plugin that uses strconv.Quote. SQL NULL keeps the
// existing NullString spelling. Shared presets are not mutated.
func applyStringQuoteDisplay(fc *spanvalue.FormatConfig, quoteMode enums.StringQuoteMode, mode enums.DisplayMode) (*spanvalue.FormatConfig, error) {
	if fc == nil || quoteMode == enums.StringQuoteModeNone || !isStringQuoteDisplayMode(mode) {
		return fc, nil
	}

	updated := fc.WithComplexPlugin(spanvalue.PluginFromNullable(quoteStringForMode(quoteMode)))
	if err := updated.Validate(); err != nil {
		return nil, err
	}
	return updated, nil
}

func quoteStringForMode(quoteMode enums.StringQuoteMode) spanvalue.FormatNullableFunc {
	return func(v spanvalue.NullableValue) (string, error) {
		ns, ok := v.(spanner.NullString)
		if !ok || !ns.Valid {
			return "", spanvalue.ErrFallthrough
		}
		if quoteMode == enums.StringQuoteModeAuto && !needsAutoStringQuote(ns.StringVal) {
			return "", spanvalue.ErrFallthrough
		}
		return quoteDisplayString(ns.StringVal), nil
	}
}
