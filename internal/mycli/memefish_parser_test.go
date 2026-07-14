// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"runtime"
	"testing"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/stretchr/testify/require"
)

func TestRecoverMemefishParserPanic(t *testing.T) {
	t.Parallel()

	t.Run("known malformed input becomes an error", func(t *testing.T) {
		t.Parallel()

		const input = "'unclosed"
		rawPanic := capturePanicValue(func() {
			_, _ = memefish.ParseExpr("", input)
		})
		_, ok := rawPanic.(*memefish.Error)
		require.True(t, ok, "raw memefish.ParseExpr panic = %T, want *memefish.Error", rawPanic)

		_, err := parseMemefishExpr("", input)
		require.Error(t, err)
		var parseErr *memefish.Error
		require.ErrorAs(t, err, &parseErr)
	})

	t.Run("string panic escapes", func(t *testing.T) {
		t.Parallel()

		const want = "programmer bug"
		got := capturePanicValue(func() {
			_, _ = recoverMemefishParserPanic(func() (int, error) {
				panic(want)
			})
		})
		require.Equal(t, want, got)
	})

	t.Run("runtime panic escapes", func(t *testing.T) {
		t.Parallel()

		got := capturePanicValue(func() {
			_, _ = recoverMemefishParserPanic(func() (int, error) {
				var values []int
				return values[0], nil
			})
		})
		_, ok := got.(runtime.Error)
		require.True(t, ok, "panic = %T, want runtime.Error", got)
	})
}

func capturePanicValue(f func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	f()
	return nil
}
