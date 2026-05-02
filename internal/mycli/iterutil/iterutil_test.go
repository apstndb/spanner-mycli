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

package iterutil

import (
	"slices"
	"strconv"
	"testing"
)

func TestZipShortestBy(t *testing.T) {
	t.Parallel()

	got := slices.Collect(ZipShortestBy(
		slices.Values([]string{"a", "b", "c"}),
		slices.Values([]int{1, 2}),
		func(s string, n int) string {
			return s + strconv.Itoa(n)
		},
	))

	want := []string{"a1", "b2"}
	if !slices.Equal(got, want) {
		t.Fatalf("ZipShortestBy() = %v, want %v", got, want)
	}
}

func TestZipShortestByStopsWhenFirstSequenceIsShorter(t *testing.T) {
	t.Parallel()

	got := slices.Collect(ZipShortestBy(
		slices.Values([]string{"a"}),
		slices.Values([]int{1, 2}),
		func(s string, n int) string {
			return s + strconv.Itoa(n)
		},
	))

	want := []string{"a1"}
	if !slices.Equal(got, want) {
		t.Fatalf("ZipShortestBy() = %v, want %v", got, want)
	}
}
