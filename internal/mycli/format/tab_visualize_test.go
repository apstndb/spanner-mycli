package format

import (
	"bytes"
	"strings"
	"testing"

	"github.com/apstndb/go-tabwrap"
	"github.com/google/go-cmp/cmp"
)

func newTestCondition(tabWidth int) *tabwrap.Condition {
	return &tabwrap.Condition{TabWidth: tabWidth}
}

func TestVisualizeTab(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		tabWidth  int
		wantPlain string
	}{
		{
			name:      "no tabs",
			text:      "hello",
			tabWidth:  4,
			wantPlain: "hello",
		},
		{
			name:      "basic tab at col 3",
			text:      "abc\tdef",
			tabWidth:  4,
			wantPlain: "abc→def",
		},
		{
			name:      "tab at col 0",
			text:      "\tdef",
			tabWidth:  4,
			wantPlain: "→   def",
		},
		{
			name:      "tab at end",
			text:      "abc\t",
			tabWidth:  4,
			wantPlain: "abc→",
		},
		{
			name:      "multiple tabs",
			text:      "\t\t",
			tabWidth:  4,
			wantPlain: "→   →   ",
		},
		{
			name:      "CJK before tab",
			text:      "日\t",
			tabWidth:  4,
			wantPlain: "日→ ",
		},
		{
			name:      "newline resets column",
			text:      "ab\n\tdef",
			tabWidth:  4,
			wantPlain: "ab\n→   def",
		},
		{
			name:      "tab at col 4 (aligned)",
			text:      "abcd\tef",
			tabWidth:  4,
			wantPlain: "abcd→   ef",
		},
		{
			name:      "tab width 8",
			text:      "ab\tcd",
			tabWidth:  8,
			wantPlain: "ab→     cd",
		},
		{
			name:      "tab between CJK",
			text:      "日本\ttest",
			tabWidth:  4,
			wantPlain: "日本→   test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cond := newTestCondition(tt.tabWidth)
			gotPlain := visualizeTab(tt.text, cond, false)

			if diff := cmp.Diff(tt.wantPlain, gotPlain); diff != "" {
				t.Errorf("plain mismatch (-want +got):\n%s", diff)
			}

			// Styled version should have the same visible content when ANSI codes are stripped.
			gotStyled := visualizeTab(tt.text, cond, true)
			stripped := stripANSI(gotStyled)
			if diff := cmp.Diff(tt.wantPlain, stripped); diff != "" {
				t.Errorf("styled (stripped) mismatch (-want +got):\n%s", diff)
			}

			// When there are tabs, styled version should contain dim markers.
			if strings.Contains(tt.text, "\t") {
				if !strings.Contains(gotStyled, "\033[2m→\033[22m") {
					t.Errorf("styled should contain dim arrow marker, got: %q", gotStyled)
				}
			}
		})
	}
}

func TestVisualizeTab_WidthPreserved(t *testing.T) {
	t.Parallel()

	// The visualized text should have the same display width as tab-expanded text.
	tests := []struct {
		name     string
		text     string
		tabWidth int
	}{
		{"basic", "abc\tdef", 4},
		{"col 0", "\tdef", 4},
		{"multiple", "\t\t", 4},
		{"CJK", "日\t", 4},
		{"newline", "ab\n\tdef", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cond := newTestCondition(tt.tabWidth)
			plain := visualizeTab(tt.text, cond, false)

			origWidth := cond.StringWidth(tt.text)
			vizWidth := cond.StringWidth(plain)
			if origWidth != vizWidth {
				t.Errorf("width mismatch: original=%d, visualized=%d (plain=%q)", origWidth, vizWidth, plain)
			}
		})
	}
}

func TestVisualizeTabsInRow_NoTabs(t *testing.T) {
	t.Parallel()

	row := Row{PlainCell{Text: "hello"}, PlainCell{Text: "world"}}
	cond := newTestCondition(4)

	got := visualizeTabsInRow(row, cond, false)

	// Should return original row (same pointer) when no tabs.
	if &got[0] != &row[0] {
		t.Error("expected original row to be returned when no tabs present")
	}
}

func TestVisualizeTabsInRow_Unstyled(t *testing.T) {
	t.Parallel()

	row := Row{
		PlainCell{Text: "no tabs"},
		PlainCell{Text: "abc\tdef"},
	}
	cond := newTestCondition(4)

	got := visualizeTabsInRow(row, cond, false)

	// First cell unchanged.
	if got[0].RawText() != "no tabs" {
		t.Errorf("cell 0: got %q, want %q", got[0].RawText(), "no tabs")
	}

	// Second cell has tabs visualized via WithText (PlainCell preserved).
	if _, ok := got[1].(PlainCell); !ok {
		t.Errorf("cell 1: expected PlainCell, got %T", got[1])
	}
	want := "abc→def"
	if got[1].RawText() != want {
		t.Errorf("cell 1: got %q, want %q", got[1].RawText(), want)
	}
}

func TestVisualizeTabsInRow_Styled_PlainCell(t *testing.T) {
	t.Parallel()

	row := Row{PlainCell{Text: "abc\tdef"}}
	cond := newTestCondition(4)

	got := visualizeTabsInRow(row, cond, true)

	vc, ok := got[0].(*tabVisualizedCell)
	if !ok {
		t.Fatalf("expected *tabVisualizedCell, got %T", got[0])
	}
	if vc.cellStyle != "" {
		t.Errorf("expected empty cellStyle for PlainCell, got %q", vc.cellStyle)
	}
	if vc.RawText() != "abc→def" {
		t.Errorf("RawText() = %q, want %q", vc.RawText(), "abc→def")
	}
}

func TestVisualizeTabsInRow_Styled_StyledCell(t *testing.T) {
	t.Parallel()

	row := Row{StyledCell{Text: "abc\tdef", Style: "\033[32m"}}
	cond := newTestCondition(4)

	got := visualizeTabsInRow(row, cond, true)

	vc, ok := got[0].(*tabVisualizedCell)
	if !ok {
		t.Fatalf("expected *tabVisualizedCell, got %T", got[0])
	}
	if vc.cellStyle != "\033[32m" {
		t.Errorf("cellStyle = %q, want %q", vc.cellStyle, "\033[32m")
	}

	// Format() should wrap with cell style.
	formatted := vc.Format()
	if !strings.HasPrefix(formatted, "\033[32m") {
		t.Errorf("Format() should start with cell style, got %q", formatted)
	}
	if !strings.HasSuffix(formatted, ansiReset) {
		t.Errorf("Format() should end with ansiReset, got %q", formatted)
	}
	// Should contain dim arrow.
	if !strings.Contains(formatted, "\033[2m→\033[22m") {
		t.Errorf("Format() should contain dim arrow, got %q", formatted)
	}
}

func TestTabVisualizedCell_WithText(t *testing.T) {
	t.Parallel()

	vc := &tabVisualizedCell{styled: "abc\033[2m→\033[22mdef", cellStyle: "\033[32m"}
	got := vc.WithText("abc→def")

	tc, ok := got.(*tabVisualizedCell)
	if !ok {
		t.Fatalf("WithText should return *tabVisualizedCell, got %T", got)
	}
	if tc.cellStyle != "\033[32m" {
		t.Errorf("cellStyle = %q, want %q", tc.cellStyle, "\033[32m")
	}
	// Styled should have dim arrows re-applied.
	if !strings.Contains(tc.styled, "\033[2m→\033[22m") {
		t.Errorf("styled should contain dim arrow, got %q", tc.styled)
	}
	// RawText (plain) should be lazily derived without ANSI codes.
	if tc.RawText() != "abc→def" {
		t.Errorf("RawText() = %q, want %q", tc.RawText(), "abc→def")
	}
}

func TestTabVisualizedCell_RawText_Cached(t *testing.T) {
	t.Parallel()

	vc := &tabVisualizedCell{styled: "abc\033[2m→\033[22mdef"}

	// First call should derive and cache plain.
	got1 := vc.RawText()
	if got1 != "abc→def" {
		t.Errorf("RawText() = %q, want %q", got1, "abc→def")
	}

	// Second call should return the same cached value.
	got2 := vc.RawText()
	if got1 != got2 {
		t.Errorf("RawText() not stable: %q vs %q", got1, got2)
	}
}

func TestWriteTable_WithTabs(t *testing.T) {
	t.Parallel()

	t.Run("disabled_by_default", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		rows := []Row{StringsToRow("abc\tdef", "no tabs")}
		columns := []string{"data", "other"}

		err := WriteTable(&buf, rows, columns, FormatConfig{}, 80, ModeTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if strings.Contains(output, "→") {
			t.Error("output should not contain arrow when TabVisualize is false")
		}
	})

	t.Run("unstyled", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		rows := []Row{StringsToRow("abc\tdef", "no tabs")}
		columns := []string{"data", "other"}

		err := WriteTable(&buf, rows, columns, FormatConfig{TabVisualize: true}, 80, ModeTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		// Should contain the arrow visualization, not a raw tab.
		if strings.Contains(output, "\t") {
			t.Error("output should not contain raw tab characters")
		}
		if !strings.Contains(output, "→") {
			t.Error("output should contain arrow visualization")
		}
		if !strings.Contains(output, "no tabs") {
			t.Error("non-tab cell should be unchanged")
		}
		verifyTableAlignment(t, output)
	})

	t.Run("styled", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		rows := []Row{{StyledCell{Text: "abc\tdef", Style: "\033[32m"}, PlainCell{Text: "ok"}}}
		columns := []string{"data", "other"}

		err := WriteTable(&buf, rows, columns, FormatConfig{Styled: true, TabVisualize: true}, 80, ModeTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if strings.Contains(output, "\t") {
			t.Error("output should not contain raw tab characters")
		}
		// Should contain dim arrow marker.
		if !strings.Contains(output, "\033[2m→\033[22m") {
			t.Error("styled output should contain dim arrow marker")
		}
		verifyTableAlignment(t, output)
	})
}
