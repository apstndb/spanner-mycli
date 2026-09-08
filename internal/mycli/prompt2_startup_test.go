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

package mycli

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
)

func TestPrompt2StartupValidation(t *testing.T) {
	oldLogger, oldLevel := slog.Default(), cliLogLevel.Level()
	t.Cleanup(func() { slog.SetDefault(oldLogger); cliLogLevel.Set(oldLevel) })
	for _, tt := range []struct {
		name, config, want string
		args               []string
		wantErr            bool
	}{
		{name: "unset", want: defaultPrompt2},
		{name: "empty flag", args: []string{"--prompt2="}, wantErr: true},
		{name: "empty config", config: "prompt2 = ''\n", wantErr: true},
		{name: "empty set", args: []string{"--set=CLI_PROMPT2="}, wantErr: true},
		{name: "nonempty flag", args: []string{"--prompt2=next> "}, want: "next> "},
		{name: "nonempty config", config: "prompt2 = 'config> '\n", want: "config> "},
		{name: "flag overrides config", config: "prompt2 = ''\n", args: []string{"--prompt2=flag> "}, want: "flag> "},
		{name: "set overrides flag", args: []string{"--prompt2=flag> ", "--set=CLI_PROMPT2=set> "}, want: "set> "},
		{name: "invalid flag precedes set", args: []string{"--prompt2=", "--set=CLI_PROMPT2=set> "}, wantErr: true},
		{name: "indent only", args: []string{"--prompt2=%P"}, want: "%P"},
		{name: "whitespace", args: []string{"--prompt2= "}, want: " "},
		{name: "empty primary prompt unaffected", args: []string{"--prompt="}, want: defaultPrompt2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var configFiles []string
			if tt.config != "" {
				path := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(path, []byte(tt.config), 0o600); err != nil {
					t.Fatal(err)
				}
				configFiles = []string{path}
			}
			opts, _, err := parseFlagsArgs(tt.args, "test", configFiles, io.Discard, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			sv, err := initializeSystemVariables(&opts.Spanner)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "CLI_PROMPT2 cannot be empty") || sv != nil {
					t.Fatalf("initialization: state nil=%v, error=%v; want nil state and empty-prompt error", sv == nil, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := sv.Display.Prompt2; got != tt.want {
				t.Fatalf("Prompt2 = %q, want %q", got, tt.want)
			}
			// Startup values must be restorable by the same registry used for SET LOCAL.
			if err := sv.SetFromSimple("CLI_PROMPT2", sv.Display.Prompt2); err != nil {
				t.Fatalf("startup value cannot round-trip: %v", err)
			}
			for _, end := range []string{"COMMIT", "ROLLBACK"} {
				session := &Session{
					mode: DatabaseConnected, systemVariables: sv,
					txn: NewTransactionManager(nil, sv, spanner.ClientConfig{}),
				}
				sv.inTransaction = session.txn.InTransaction
				for _, sql := range []string{"BEGIN", "SET LOCAL CLI_PROMPT2 = 'temporary> '", end} {
					stmt, err := BuildStatement(sql)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := session.ExecuteStatement(t.Context(), stmt); err != nil {
						t.Fatalf("%s: %v", sql, err)
					}
				}
				if got := sv.Display.Prompt2; got != tt.want || session.txn.InTransaction() {
					t.Fatalf("after %s: Prompt2=%q, want %q; active=%v", end, got, tt.want, session.txn.InTransaction())
				}
			}
		})
	}
}
