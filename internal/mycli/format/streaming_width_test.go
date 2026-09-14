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
	"bytes"
	"math"
	"strings"
	"testing"
)

const streamWidthFullValue = "abcdefghij"

func renderStreamingTable(t *testing.T, mode Mode, config FormatConfig, screenWidth, previewSize int, headers []string, preview, later []Row) (string, []int) {
	t.Helper()
	var out bytes.Buffer
	f := NewTableStreamingFormatter(&out, config, screenWidth, previewSize, mode)
	if err := f.InitFormat(headers, config, preview); err != nil {
		t.Fatal(err)
	}
	for _, row := range preview {
		if err := f.WriteRow(row); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range later {
		if err := f.WriteRow(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.FinishFormat(); err != nil {
		t.Fatal(err)
	}
	return out.String(), f.widths
}

func renderBufferedTable(t *testing.T, mode Mode, config FormatConfig, screenWidth int, headers []string, rows []Row) string {
	t.Helper()
	var out bytes.Buffer
	f := NewTableFormatterForBuffered(&out, config, screenWidth, mode, TableParams{})
	if err := ExecuteWithFormatter(f, rows, headers, config); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestStreamingTablePreservesPreviewedValue(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	preview := []Row{StringsToRow(streamWidthFullValue)}
	config := FormatConfig{}

	for _, mode := range []Mode{ModeTable, ModeTableComment, ModeTableDetailComment} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			got, widths := renderStreamingTable(t, mode, config, 80, 50, headers, preview, nil)
			if len(widths) != 1 || widths[0] < len(streamWidthFullValue) {
				t.Fatalf("calculated widths = %v, want content width >= %d", widths, len(streamWidthFullValue))
			}
			if !strings.Contains(got, streamWidthFullValue) {
				t.Fatalf("streaming truncated previewed value: %q", got)
			}
			want := renderBufferedTable(t, mode, config, 80, headers, preview)
			if !strings.Contains(want, streamWidthFullValue) {
				t.Fatalf("buffered lost previewed value: %q", want)
			}
			if mode == ModeTableComment {
				if !strings.Contains(got, "/*") || !strings.Contains(got, "*/") {
					t.Fatalf("TABLE_COMMENT lost framing: %q", got)
				}
			}
			if mode == ModeTableDetailComment && !strings.Contains(got, "/*") {
				t.Fatalf("TABLE_DETAIL_COMMENT lost opening: %q", got)
			}
		})
	}
}

func TestStreamingTableHeadersOnlyAndSkipNames(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	later := []Row{StringsToRow(streamWidthFullValue)}

	t.Run("headers-only preview", func(t *testing.T) {
		t.Parallel()
		got, _ := renderStreamingTable(t, ModeTable, FormatConfig{}, 80, 0, headers, nil, later)
		if !strings.Contains(got, streamWidthFullValue) {
			t.Fatalf("headers-only preview truncated later row: %q", got)
		}
	})

	t.Run("skip column names", func(t *testing.T) {
		t.Parallel()
		config := FormatConfig{SkipColumnNames: true}
		preview := []Row{StringsToRow(streamWidthFullValue)}
		got, _ := renderStreamingTable(t, ModeTable, config, 80, 50, headers, preview, nil)
		if !strings.Contains(got, streamWidthFullValue) {
			t.Fatalf("skip-headers truncated value: %q", got)
		}
		if strings.Contains(got, "| id") {
			t.Fatalf("skip-headers still printed header: %q", got)
		}
		want := renderBufferedTable(t, ModeTable, config, 80, headers, preview)
		if !strings.Contains(want, streamWidthFullValue) {
			t.Fatalf("skip-headers buffered lost value: %q", want)
		}
	})
}

func TestStreamingTableLaterRowUsesExistingWrap(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	preview := []Row{StringsToRow("ab")}
	later := []Row{StringsToRow(streamWidthFullValue)}
	all := append(append([]Row{}, preview...), later...)

	t.Run("screen fits later row", func(t *testing.T) {
		t.Parallel()
		got, _ := renderStreamingTable(t, ModeTable, FormatConfig{}, 80, 1, headers, preview, later)
		if !strings.Contains(got, streamWidthFullValue) {
			t.Fatalf("later row truncated: %q", got)
		}
		want := renderBufferedTable(t, ModeTable, FormatConfig{}, 80, headers, all)
		if !strings.Contains(want, streamWidthFullValue) {
			t.Fatalf("later-row buffered lost value: %q", want)
		}
	})

	t.Run("narrow screen wraps instead of cutting", func(t *testing.T) {
		t.Parallel()
		long := "abcdefghijklmnop"
		rows := []Row{StringsToRow(long)}
		got, widths := renderStreamingTable(t, ModeTable, FormatConfig{}, 12, 1, headers, rows, nil)
		if !strings.Contains(got, "abcdefgh") || !strings.Contains(got, "ijklmnop") {
			t.Fatalf("narrow stream lost wrapped value: %q widths=%v", got, widths)
		}
		if strings.Contains(got, long) {
			t.Fatalf("narrow stream should wrap, got single-line %q", got)
		}
		want := renderBufferedTable(t, ModeTable, FormatConfig{}, 12, headers, rows)
		if got != want {
			t.Fatalf("narrow wrap streaming/buffered mismatch\nstreamed:\n%s\nbuffered:\n%s", got, want)
		}
	})
}

func TestStreamingTableUnconstrainedScreenUsesNaturalWidths(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	preview := []Row{StringsToRow(streamWidthFullValue)}
	got, _ := renderStreamingTable(t, ModeTable, FormatConfig{}, math.MaxInt, 50, headers, preview, nil)
	if !strings.Contains(got, streamWidthFullValue) {
		t.Fatalf("unconstrained stream truncated: %q", got)
	}
	for line := range strings.SplitSeq(got, "\n") {
		if len(line) > 80 {
			t.Fatalf("unconstrained stream locked MaxInt width: %q", line)
		}
	}
}

func TestStreamingTableANSIAndNoWrapFollowExistingHelpers(t *testing.T) {
	t.Parallel()
	headers := []string{"id"}
	colored := "\x1b[31m" + streamWidthFullValue + "\x1b[0m"

	t.Run("ANSI visible width", func(t *testing.T) {
		t.Parallel()
		rows := []Row{StringsToRow(colored)}
		got, _ := renderStreamingTable(t, ModeTable, FormatConfig{}, 80, 1, headers, rows, nil)
		if !strings.Contains(got, streamWidthFullValue) {
			t.Fatalf("ANSI stream truncated visible value: %q", got)
		}
		if !strings.Contains(got, "\x1b[31m") || !strings.Contains(got, "\x1b[0m") {
			t.Fatalf("ANSI codes lost: %q", got)
		}
		want := renderBufferedTable(t, ModeTable, FormatConfig{}, 80, headers, rows)
		if !strings.Contains(ansiWidthFixtureStrip.Replace(want), streamWidthFullValue) {
			t.Fatalf("ANSI buffered lost value: %q", want)
		}
	})

	t.Run("NoWrap preferred min", func(t *testing.T) {
		t.Parallel()
		rows := []Row{{NoWrapCell{Cell: PlainCell{Text: streamWidthFullValue}}}}
		got, widths := renderStreamingTable(t, ModeTable, FormatConfig{}, 80, 1, headers, rows, nil)
		if !strings.Contains(got, streamWidthFullValue) {
			t.Fatalf("NoWrap stream truncated: %q widths=%v", got, widths)
		}
		want := renderBufferedTable(t, ModeTable, FormatConfig{}, 80, headers, rows)
		if !strings.Contains(want, streamWidthFullValue) {
			t.Fatalf("NoWrap buffered lost value: %q", want)
		}
	})
}
