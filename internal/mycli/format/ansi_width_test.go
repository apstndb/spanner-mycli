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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
)

// Remove only complete sequences used by these fixtures and tab markers. A
// split escape must remain in the output and fail the exact layout comparison.
var ansiWidthFixtureStrip = strings.NewReplacer(
	"\x1b[31m", "", "\x1b[32m", "", "\x1b[0m", "",
	"\x1b[2m", "", "\x1b[22m", "",
)

func TestTableANSIWidthCondition(t *testing.T) {
	f := &TableStreamingFormatter{config: FormatConfig{TabWidth: 8}}
	cond := f.newCondition()
	if got := cond.StringWidth("\x1b[31mred\x1b[0m\tx"); got != 9 {
		t.Fatalf("visible width = %d, want 9", got)
	}
	wc := &widthCalculator{Condition: cond}
	plain := wc.adjustByHeader([]string{"first", "second"}, 15)
	colored := wc.adjustByHeader([]string{"first", "\x1b[31msecond\x1b[0m"}, 15)
	if !reflect.DeepEqual(plain, colored) {
		t.Fatalf("header allocation plain=%v colored=%v", plain, colored)
	}
	wrapped := cond.Wrap("\x1b[31mabcdef\x1b[0m", 3)
	if want := "\x1b[31mabc\x1b[0m\n\x1b[31mdef\x1b[0m"; wrapped != want {
		t.Fatalf("SGR carry-over = %q, want %q", wrapped, want)
	}
}

func TestTableANSILayout(t *testing.T) {
	for _, strategy := range enums.WidthStrategyValues() {
		for _, styled := range []bool{false, true} {
			for _, streaming := range []bool{false, true} {
				for _, visualize := range []bool{false, true} {
					for _, width := range []int{20, 40, 80} {
						t.Run(fmt.Sprintf("%s/styled%v/stream%v/tabs%v/width%d", strategy, styled, streaming, visualize, width), func(t *testing.T) {
							config := FormatConfig{WidthStrategy: strategy, Styled: styled, TabVisualize: visualize, TabWidth: 4}
							for _, coloredHeader := range []bool{false, true} {
								render := func(color bool) ([]int, string) {
									headers := []string{"first", "second"}
									values := []string{"abcdefghijklmnop", "界e\u0301\tx"}
									if color {
										if coloredHeader {
											headers[1] = "\x1b[31m" + headers[1] + "\x1b[0m"
										} else {
											for i := range values {
												values[i] = "\x1b[31m" + values[i] + "\x1b[0m"
											}
										}
									}
									rows := []Row{StringsToRow(values...)}
									// Exercise the same raw text with CLI-added styles independently.
									rows[0][0] = StyledCell{Text: values[0], Style: "\x1b[32m"}
									var out bytes.Buffer
									var f *TableStreamingFormatter
									if streaming {
										f = NewTableStreamingFormatter(&out, config, width, 1, ModeTable)
									} else {
										f = NewTableFormatterForBuffered(&out, config, width, ModeTable, TableParams{})
									}
									if err := ExecuteWithFormatter(f, rows, headers, config); err != nil {
										t.Fatal(err)
									}
									return f.widths, out.String()
								}
								plainWidths, plain := render(false)
								coloredWidths, colored := render(true)
								if !reflect.DeepEqual(plainWidths, coloredWidths) {
									t.Errorf("header%v widths plain=%v colored=%v", coloredHeader, plainWidths, coloredWidths)
								}
								if ansiWidthFixtureStrip.Replace(colored) != ansiWidthFixtureStrip.Replace(plain) {
									t.Errorf("header%v layout mismatch:\nplain=%q\ncolored=%q", coloredHeader, plain, colored)
								}
								if !strings.Contains(colored, "\x1b[31m") || !strings.Contains(colored, "\x1b[0m") {
									t.Errorf("input color lost: %q", colored)
								}
								if !styled && strings.Contains(colored, "\x1b[32m") {
									t.Errorf("disabled CLI style was emitted: %q", colored)
								}
							}
						})
					}
				}
			}
		}
	}
}
