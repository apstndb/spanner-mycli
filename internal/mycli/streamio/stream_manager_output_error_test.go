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

package streamio

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamManagerOutputWriteFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		silent bool
	}{
		{name: "sole file", silent: true},
		{name: "tee retains stdout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			sm := NewStreamManager(nil, &stdout, &stderr)
			t.Cleanup(sm.Close)
			path := filepath.Join(t.TempDir(), "output.txt")
			if err := sm.EnableTee(path, tc.silent); err != nil {
				t.Fatal(err)
			}
			writer := sm.GetWriter()
			if err := sm.teeFile.Close(); err != nil {
				t.Fatal(err)
			}
			const data = "result\n"
			for i := range 2 {
				if sm.GetWriter() != writer {
					t.Fatal("writer cache changed after destination failure")
				}
				n, err := sm.GetWriter().Write([]byte(data))
				if tc.silent {
					if n != 0 || !errors.Is(err, os.ErrClosed) {
						t.Errorf("write %d = (%d, %v), want (0, ErrClosed)", i, n, err)
					}
				} else if n != len(data) || err != nil {
					t.Errorf("tee write %d = (%d, %v), want (%d, nil)", i, n, err, len(data))
				}
			}
			if tc.silent {
				if stdout.Len() != 0 || stderr.Len() != 0 {
					t.Errorf("sole-file writer leaked data or swallowed errors into warnings: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			} else {
				if stdout.String() != strings.Repeat(data, 2) {
					t.Errorf("tee stdout = %q", stdout.String())
				}
				if strings.Count(stderr.String(), "Failed to write to output file") != 1 {
					t.Errorf("want one file failure warning, got %q", stderr.String())
				}
			}

			// Disabling output discards the failed cached writer. Re-enabling a
			// new destination must work instead of retaining its error state.
			sm.DisableTee()
			stdout.Reset()
			if _, err := sm.GetWriter().Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != data {
				t.Errorf("stdout after disable = %q", stdout.String())
			}
			newPath := filepath.Join(t.TempDir(), "new-output.txt")
			if err := sm.EnableTee(newPath, tc.silent); err != nil {
				t.Fatal(err)
			}
			if _, err := sm.GetWriter().Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(newPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != data {
				t.Errorf("new file = %q, want %q", got, data)
			}
		})
	}
}
