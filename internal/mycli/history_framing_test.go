// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/nyaosorg/go-readline-ny/simplehistory"
	"github.com/spf13/afero"
)

func TestHistoryAppendFraming(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		initial []byte
		items   []string
	}{
		{name: "missing file"},
		{name: "empty file", initial: []byte{}},
		{name: "unterminated record", initial: []byte(`"SELECT 1;"`), items: []string{"SELECT 1;"}},
		{name: "terminated record", initial: []byte("\"SELECT 1;\"\n"), items: []string{"SELECT 1;"}},
		{name: "blank separator lines", initial: []byte("\n\"SELECT 1;\"\n\n"), items: []string{"SELECT 1;"}},
		{name: "quoted empty record", initial: []byte(`""`), items: []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := &historyAppendFS{Fs: afero.NewMemMapFs()}
			const path = "synthetic-history"
			if tc.initial != nil {
				if err := afero.WriteFile(fs, path, tc.initial, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			h := loadHistoryForFraming(t, fs, path)
			assertHistoryItems(t, h, tc.items)
			value := "SELECT 'quoted';\nSELECT \"two\";"
			h.Add(value)
			assertHistoryItems(t, loadHistoryForFraming(t, fs, path), append(slices.Clone(tc.items), value))
			payload := "\n" + strconv.Quote(value) + "\n"
			if !slices.Equal(fs.writes, []string{payload}) {
				t.Fatalf("append writes=%q, want one framed payload %q", fs.writes, payload)
			}
			data, err := afero.ReadFile(fs, path)
			if err != nil {
				t.Fatal(err)
			}
			if want := string(tc.initial) + payload; string(data) != want {
				t.Fatalf("file=%q, want existing bytes plus payload %q", data, want)
			}
		})
	}
}

func TestHistoryFramingTwoLoadedSessions(t *testing.T) {
	t.Parallel()
	fs := afero.NewMemMapFs()
	const path = "synthetic-history"
	const initial = `"SELECT 1;"`
	if err := afero.WriteFile(fs, path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	a, b := loadHistoryForFraming(t, fs, path), loadHistoryForFraming(t, fs, path)
	a.Add("SELECT 2;")
	b.Add("SELECT 3;")
	assertHistoryItems(t, a, []string{"SELECT 1;", "SELECT 2;"})
	assertHistoryItems(t, b, []string{"SELECT 1;", "SELECT 3;"})
	assertHistoryItems(t, loadHistoryForFraming(t, fs, path), []string{"SELECT 1;", "SELECT 2;", "SELECT 3;"})
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		t.Fatal(err)
	}
	if want := initial + "\n\"SELECT 2;\"\n\n\"SELECT 3;\"\n"; string(data) != want {
		t.Fatalf("file=%q, want %q", data, want)
	}
}

func TestHistoryFramingFailureAndRetry(t *testing.T) {
	t.Parallel()
	fs := &historyAppendFS{Fs: afero.NewMemMapFs()}
	const path = "synthetic-history"
	const initial = `"SELECT 1;"`
	if err := afero.WriteFile(fs, path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	h := loadHistoryForFraming(t, fs, path)
	for _, failure := range []string{"open", "write"} {
		if failure == "open" {
			fs.openErr = errors.New("injected history append open failure")
		} else {
			fs.openErr = nil
			fs.writeErr = io.ErrClosedPipe
		}
		h.Add(failure)
		data, err := afero.ReadFile(fs, path)
		if err != nil || string(data) != initial {
			t.Fatalf("%s failure changed file: %q, error=%v", failure, data, err)
		}
	}
	fs.writeErr = nil
	h.Add("SELECT 2;")
	assertHistoryItems(t, h, []string{"SELECT 1;", "open", "write", "SELECT 2;"})
	assertHistoryItems(t, loadHistoryForFraming(t, fs, path), []string{"SELECT 1;", "SELECT 2;"})
	if want := []string{"\n\"write\"\n", "\n\"SELECT 2;\"\n"}; !slices.Equal(fs.writes, want) {
		t.Fatalf("failed/successful write payloads=%q, want %q", fs.writes, want)
	}
}

func TestHistoryMalformedTailRemainsUntouched(t *testing.T) {
	t.Parallel()
	fs := afero.NewMemMapFs()
	const path = "synthetic-history"
	const initial = "\"SELECT 1;\"\n\"unfinished"
	if err := afero.WriteFile(fs, path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if h, err := newPersistentHistoryWithFS(path, simplehistory.New(), fs); err == nil || h != nil {
		t.Fatalf("malformed history accepted: history=%v, error=%v", h, err)
	}
	if data, err := afero.ReadFile(fs, path); err != nil || string(data) != initial {
		t.Fatalf("malformed file changed: %q, error=%v", data, err)
	}
}

func loadHistoryForFraming(t *testing.T, fs afero.Fs, path string) History {
	t.Helper()
	h, err := newPersistentHistoryWithFS(path, simplehistory.New(), fs)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func assertHistoryItems(t *testing.T, h History, want []string) {
	t.Helper()
	if h.Len() != len(want) {
		t.Fatalf("history length=%d, want %d", h.Len(), len(want))
	}
	for i, value := range want {
		if got := h.At(i); got != value {
			t.Fatalf("history[%d]=%q, want %q", i, got, value)
		}
	}
}

// Observe only append writes; fixture creation and loader reads use the
// embedded in-memory filesystem unchanged. No personal history is accessed.
type historyAppendFS struct {
	afero.Fs
	openErr, writeErr error
	writes            []string
}

func (fs *historyAppendFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag&os.O_APPEND == 0 {
		return fs.Fs.OpenFile(name, flag, perm)
	}
	if fs.openErr != nil {
		return nil, fs.openErr
	}
	file, err := fs.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &historyAppendFile{File: file, fs: fs}, nil
}

type historyAppendFile struct {
	afero.File
	fs *historyAppendFS
}

func (f *historyAppendFile) Write(p []byte) (int, error) {
	f.fs.writes = append(f.fs.writes, string(p))
	if f.fs.writeErr != nil {
		return 0, f.fs.writeErr
	}
	return f.File.Write(p)
}
