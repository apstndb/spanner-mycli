// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package format

import (
	"math"
	"strings"
	"testing"

	"github.com/apstndb/go-tabwrap"
)

func TestFitCellTruncateLines(t *testing.T) {
	t.Parallel()
	cond := &tabwrap.Condition{TabWidth: 4, ControlSequences: true}

	if got := fitCell(cond, "abcdefghij", 8, false); !strings.Contains(got, "\n") {
		t.Fatalf("wrap path should keep wrapping, got %q", got)
	}

	got := truncateLines(cond, "abcdefghij", 8)
	if got != "abcde..." {
		t.Fatalf("truncate ASCII = %q, want abcde...", got)
	}
	if w := cond.StringWidth(got); w > 8 {
		t.Fatalf("truncated width %d > 8", w)
	}

	multiline := truncateLines(cond, "Name\nSTRING_TOO_LONG", 6)
	if !strings.Contains(multiline, "\n") {
		t.Fatalf("multiline newline lost: %q", multiline)
	}
	lines := strings.Split(multiline, "\n")
	if len(lines) != 2 || lines[0] != "Name" {
		t.Fatalf("verbose-style header lines = %q", multiline)
	}
	if !strings.HasSuffix(lines[1], "...") || cond.StringWidth(lines[1]) > 6 {
		t.Fatalf("second line not per-line truncated: %q", lines[1])
	}

	for _, width := range []int{1, 2} {
		got := truncateLines(cond, "abcdef", width)
		if w := cond.StringWidth(got); w > width {
			t.Fatalf("width %d result %q has display width %d", width, got, w)
		}
		if strings.Contains(got, "abcdef") {
			t.Fatalf("width %d should truncate, got %q", width, got)
		}
	}
	if got := truncateLines(cond, "abcdef", 1); got != "." {
		t.Fatalf("width 1 = %q, want .", got)
	}
	if got := truncateLines(cond, "abcdef", 2); got != ".." {
		t.Fatalf("width 2 = %q, want ..", got)
	}

	cjk := truncateLines(cond, "日本語テスト", 5)
	if w := cond.StringWidth(cjk); w > 5 {
		t.Fatalf("CJK width %d > 5 for %q", w, cjk)
	}
	combining := truncateLines(cond, "e\u0301 extra text", 4)
	if w := cond.StringWidth(combining); w > 4 {
		t.Fatalf("combining width %d > 4 for %q", w, combining)
	}
	if !strings.Contains(combining, "e\u0301") && !strings.HasPrefix(combining, "...") {
		t.Fatalf("combining cluster dropped: %q", combining)
	}
}

func TestFitRowPreservesNoWrapMetadata(t *testing.T) {
	t.Parallel()
	cond := &tabwrap.Condition{TabWidth: 4, ControlSequences: true}
	row := Row{NoWrapCell{Cell: PlainCell{Text: "abcdefghij"}}}
	got := fitRowPreserving(row, []int{6}, cond, true)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if _, ok := got[0].(NoWrapCell); !ok {
		t.Fatalf("WithText lost NoWrapCell: %T", got[0])
	}
	if text := got[0].RawText(); !strings.HasSuffix(text, "...") {
		t.Fatalf("truncated text = %q", text)
	}
}

func TestEllipsisConstrainedTableAgreesBufferedStreaming(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	rows := []Row{StringsToRow("abcdefghijklmnop")}
	config := FormatConfig{Ellipsis: true}
	const screen = 12

	for _, mode := range []Mode{ModeTable, ModeTableComment, ModeTableDetailComment} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			got, widths := renderStreamingTable(t, mode, config, screen, 1, headers, rows, nil)
			if !strings.Contains(got, "...") {
				t.Fatalf("constrained ellipsis missing marker: %q widths=%v", got, widths)
			}
			if strings.Contains(got, "abcdefghijklmnop") {
				t.Fatalf("constrained ellipsis still wrapped full value: %q", got)
			}
			want := renderBufferedTable(t, mode, config, screen, headers, rows)
			if got != want {
				t.Fatalf("equivalent buffered/streaming mismatch\nstreamed:\n%s\nbuffered:\n%s", got, want)
			}
		})
	}
}

func TestEllipsisDefaultAndUnconstrainedKeepWrap(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	long := "abcdefghijklmnop"
	rows := []Row{StringsToRow(long)}

	wrapped, _ := renderStreamingTable(t, ModeTable, FormatConfig{}, 12, 1, headers, rows, nil)
	if !strings.Contains(wrapped, "abcdefgh") || !strings.Contains(wrapped, "ijklmnop") {
		t.Fatalf("default wrap lost value: %q", wrapped)
	}
	if strings.Contains(wrapped, "...") {
		t.Fatalf("default output grew an ellipsis: %q", wrapped)
	}

	preview := []Row{StringsToRow("ab")}
	later := []Row{StringsToRow(streamWidthFullValue)}
	on, onWidths := renderStreamingTable(t, ModeTable, FormatConfig{Ellipsis: true}, math.MaxInt, 1, headers, preview, later)
	off, _ := renderStreamingTable(t, ModeTable, FormatConfig{}, math.MaxInt, 1, headers, preview, later)
	if on != off {
		t.Fatalf("unconstrained ellipsis changed wrap\non:\n%s\noff:\n%s", on, off)
	}
	if joined := joinedStreamingCellText(on); !strings.Contains(joined, streamWidthFullValue) {
		t.Fatalf("unconstrained later row truncated: joined=%q widths=%v", joined, onWidths)
	}
	if strings.Contains(on, "ab...") {
		t.Fatalf("unconstrained later row used preview-width ellipsis: %q", on)
	}
}

func TestEllipsisFixedWidthBudget(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	rows := []Row{StringsToRow("abcdefghijklmnop")}
	const budget = 18
	got, widths := renderStreamingTable(t, ModeTable, FormatConfig{Ellipsis: true}, budget, 1, headers, rows, nil)
	if !strings.Contains(got, "...") {
		t.Fatalf("fixed budget missing marker: %q widths=%v", got, widths)
	}
	for line := range strings.SplitSeq(got, "\n") {
		if line != "" && len(line) > budget {
			t.Fatalf("line exceeds fixed budget %d: %q", budget, line)
		}
	}
}

func TestEllipsisVerboseHeaderAndStyledMultiline(t *testing.T) {
	t.Parallel()
	config := FormatConfig{Verbose: true, Styled: true, Ellipsis: true}
	headers := []string{"col"}
	params := TableParams{VerboseHeaders: []string{"Name\nSTRING_TOO_LONG_TYPE"}}
	rows := []Row{{StyledCell{
		Text:  "hello world extra\nsecond line extra",
		Style: "\x1b[32m",
	}}}

	var outStreaming, outBuffered string
	{
		var b strings.Builder
		f := NewTableStreamingFormatter(&b, config, 22, 1, ModeTable)
		f.params = params
		if err := ExecuteWithFormatter(f, rows, headers, config); err != nil {
			t.Fatal(err)
		}
		outStreaming = b.String()
	}
	{
		var b strings.Builder
		f := NewTableFormatterForBuffered(&b, config, 22, ModeTable, params)
		if err := ExecuteWithFormatter(f, rows, headers, config); err != nil {
			t.Fatal(err)
		}
		outBuffered = b.String()
	}
	if outStreaming != outBuffered {
		t.Fatalf("styled multiline streaming/buffered mismatch\nstreamed:\n%s\nbuffered:\n%s", outStreaming, outBuffered)
	}
	got := outStreaming
	if !strings.Contains(got, "Name") || !strings.Contains(got, "STRING") {
		t.Fatalf("verbose header lines lost: %q", got)
	}
	if strings.Contains(got, "STRING_TOO_LONG_TYPE") {
		t.Fatalf("verbose type line was not truncated: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "second") {
		t.Fatalf("multiline cell lines lost: %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("styled multiline missing marker: %q", got)
	}
	if !strings.Contains(got, "\x1b[32m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("styled SGR lost: %q", got)
	}
	if strings.Contains(got, "\x1b[32m|") || strings.Contains(got, "|\x1b[32m") {
		t.Fatalf("SGR bled into borders: %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "|") || strings.HasPrefix(strings.TrimSpace(line), "+") {
			continue
		}
		if strings.Contains(line, "\x1b[32m") && !strings.Contains(line, "\x1b[0m") {
			t.Fatalf("SGR not reset before row end: %q", line)
		}
	}
}

func TestEllipsisTabVisualizeAndNoWrapBudgets(t *testing.T) {
	t.Parallel()
	headers := []string{"v"}

	t.Run("tabs after visualization", func(t *testing.T) {
		t.Parallel()
		rows := []Row{StringsToRow("a\tb\tcdefghijklmnop")}
		got, _ := renderStreamingTable(t, ModeTable, FormatConfig{Ellipsis: true, TabVisualize: true, TabWidth: 4}, 18, 1, headers, rows, nil)
		if strings.Contains(got, "\t") {
			t.Fatalf("raw tab survived: %q", got)
		}
		if !strings.Contains(got, "→") {
			t.Fatalf("tab marker missing: %q", got)
		}
		if !strings.Contains(got, "...") {
			t.Fatalf("visualized cell not truncated: %q", got)
		}
	})

	t.Run("NoWrap ample vs impossible", func(t *testing.T) {
		t.Parallel()
		rows := []Row{{NoWrapCell{Cell: PlainCell{Text: "NULL"}}}}
		ample, ampleWidths := renderStreamingTable(t, ModeTable, FormatConfig{Ellipsis: true}, 80, 1, headers, rows, nil)
		if !strings.Contains(ample, "NULL") {
			t.Fatalf("ample budget truncated NULL: %q widths=%v", ample, ampleWidths)
		}
		tight, tightWidths := renderStreamingTable(t, ModeTable, FormatConfig{Ellipsis: true}, 5, 1, headers, rows, nil)
		if strings.Contains(tight, "NULL") {
			t.Fatalf("impossible budget kept NULL: %q widths=%v", tight, tightWidths)
		}
		if !strings.Contains(tight, "| . |") {
			t.Fatalf("impossible budget should shrink to '.', got %q widths=%v", tight, tightWidths)
		}
		cond := &tabwrap.Condition{TabWidth: 4, ControlSequences: true}
		if len(tightWidths) != 1 || cond.StringWidth(".") > tightWidths[0] {
			t.Fatalf("impossible budget overflow: %q widths=%v", tight, tightWidths)
		}
	})
}
