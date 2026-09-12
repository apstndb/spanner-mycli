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
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func requireContains(t *testing.T, got, want, label string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("%s: got %q, want substring %q", label, got, want)
	}
}

func requireErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
}

func connectionOpts(modify func(*spannerOptions)) *spannerOptions {
	opts := &spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
	}
	if modify != nil {
		modify(opts)
	}
	return opts
}

func runOffline(t *testing.T, opts *spannerOptions, features ...Feature) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := runWithOutput(t.Context(), opts, &stdout, features...)
	return stdout.String(), err
}

func TestRunOfflineHelp(t *testing.T) {
	t.Parallel()

	t.Run("statement-help prints client statement table and skips validation", func(t *testing.T) {
		t.Parallel()

		got, err := runOffline(t, &spannerOptions{StatementHelp: true})
		if err != nil {
			t.Fatalf("runWithOutput(--statement-help) error = %v", err)
		}
		for _, want := range []string{
			"| Usage",
			"| Syntax",
			"| Note",
			"Show help",
			"`HELP;`",
			"Exit CLI",
			"`EXIT;`",
			// Pipe characters in syntax must be escaped so markdown cells stay intact.
			"{READ ONLY\\|READ WRITE}",
		} {
			requireContains(t, got, want, "statement-help")
		}
	})

	t.Run("sysvars-help prints registry table and feature variables", func(t *testing.T) {
		t.Parallel()

		feat := Feature{
			Name: "OFFLINE",
			Vars: []FeatureVar{{
				Name: "CLI_OFFLINE_COVERAGE_VAR",
				Desc: "offline coverage lane probe variable",
				Var:  &stubVar{val: "probe"},
			}},
		}

		withoutFeature, err := runOffline(t, &spannerOptions{SysVarsHelp: true})
		if err != nil {
			t.Fatalf("runWithOutput(--sysvars-help) error = %v", err)
		}
		for _, want := range []string{
			"| Name",
			"| Operations",
			"| Description",
			"`CLI_FORMAT`",
			"`COMMIT_RESPONSE`",
			"read,write",
			`\<name\>:\<template\>`,
		} {
			requireContains(t, withoutFeature, want, "sysvars-help")
		}
		if strings.Contains(withoutFeature, "CLI_OFFLINE_COVERAGE_VAR") {
			t.Error("sysvars-help without features included CLI_OFFLINE_COVERAGE_VAR")
		}

		withFeature, err := runOffline(t, &spannerOptions{SysVarsHelp: true}, feat)
		if err != nil {
			t.Fatalf("runWithOutput(--sysvars-help with feature) error = %v", err)
		}
		requireContains(t, withFeature, "`CLI_OFFLINE_COVERAGE_VAR`", "sysvars-help with feature")
		requireContains(t, withFeature, "offline coverage lane probe variable", "sysvars-help with feature")
	})

	t.Run("list-samples prints built-in samples and skips validation", func(t *testing.T) {
		t.Parallel()

		got, err := runOffline(t, &spannerOptions{ListSamples: true})
		if err != nil {
			t.Fatalf("runWithOutput(--list-samples) error = %v", err)
		}
		for _, want := range []string{
			"Available sample databases:",
			"fingraph",
			"singers",
			"--sample-database=",
			".json, .yaml, or .yml",
		} {
			requireContains(t, got, want, "list-samples")
		}
	})
}

func TestRunReturnsValidationErrorWithoutOutputSeam(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), &spannerOptions{})
	requireErrorContains(t, err, errMsgMissingProjectInstance)
}

func TestRunOfflineStartupErrors(t *testing.T) {
	// Embedded-runtime setup changes the process-wide testcontainers logger.
	// Keep startup-error cases sequential, including their subtests.

	t.Run("validation errors", func(t *testing.T) {

		tests := []struct {
			name string
			opts *spannerOptions
			want string
		}{
			{
				name: "missing project and instance",
				opts: &spannerOptions{},
				want: errMsgMissingProjectInstance,
			},
			{
				name: "missing database",
				opts: &spannerOptions{ProjectId: "p", InstanceId: "i"},
				want: errMsgMissingDatabase,
			},
			{
				name: "strong and read-timestamp exclusive",
				opts: connectionOpts(func(opts *spannerOptions) {
					opts.Strong = true
					opts.ReadTimestamp = "2024-01-01T00:00:00Z"
				}),
				want: errMsgStrongReadTimestampExclusive,
			},
			{
				name: "sample database requires embedded runtime",
				opts: connectionOpts(func(opts *spannerOptions) {
					opts.SampleDatabase = "fingraph"
				}),
				want: "--sample-database requires --embedded-emulator or --embedded-omni",
			},
			{
				name: "sample database cannot combine with detached",
				opts: &spannerOptions{EmbeddedEmulator: true, Detached: true, SampleDatabase: "fingraph"},
				want: "--sample-database cannot be used with --detached",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := runOffline(t, tt.opts)
				requireErrorContains(t, err, tt.want)
				if got != "" {
					t.Errorf("stdout = %q, want empty on validation error", got)
				}
			})
		}
	})

	t.Run("system variable initialization errors", func(t *testing.T) {

		tests := []struct {
			name string
			opts *spannerOptions
			want string
		}{
			{
				name: "invalid log level",
				opts: connectionOpts(func(opts *spannerOptions) {
					opts.LogLevel = "INVALID"
				}),
				want: "error on parsing --log-level=INVALID",
			},
			{
				name: "invalid timeout",
				opts: connectionOpts(func(opts *spannerOptions) {
					opts.Timeout = "not-a-duration"
				}),
				want: "invalid value of --timeout",
			},
			{
				name: "invalid --set value",
				opts: connectionOpts(func(opts *spannerOptions) {
					opts.Set = map[string]string{"CLI_FORMAT": "NOT_A_FORMAT"}
				}),
				want: "failed to set system variable. name: CLI_FORMAT, value: NOT_A_FORMAT",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := runOffline(t, tt.opts)
				requireErrorContains(t, err, tt.want)
			})
		}
	})

	t.Run("credential file errors", func(t *testing.T) {

		t.Run("missing file", func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "missing-cred.json")
			_, err := runOffline(t, connectionOpts(func(opts *spannerOptions) {
				opts.Credential = missing
			}))
			requireErrorContains(t, err, "failed to read the credential file")
			requireErrorContains(t, err, "failed to stat file")
		})

		t.Run("directory rejected", func(t *testing.T) {
			dir := t.TempDir()
			_, err := runOffline(t, connectionOpts(func(opts *spannerOptions) {
				opts.Credential = dir
			}))
			requireErrorContains(t, err, "failed to read the credential file")
			requireErrorContains(t, err, "cannot read directory")
		})
	})

	t.Run("sample metadata errors before runtime start", func(t *testing.T) {

		writeMeta := func(t *testing.T, name, contents string) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}

		tests := []struct {
			name string
			opts *spannerOptions
			want string
		}{
			{
				name: "unknown built-in sample name",
				opts: &spannerOptions{EmbeddedEmulator: true, SampleDatabase: "no-such-sample-936"},
				want: "sample database not found: no-such-sample-936",
			},
			{
				name: "missing metadata file",
				opts: &spannerOptions{
					EmbeddedEmulator: true,
					SampleDatabase:   filepath.Join(t.TempDir(), "missing.yaml"),
				},
				want: "failed to load sample metadata",
			},
			{
				name: "invalid metadata yaml",
				opts: &spannerOptions{
					EmbeddedEmulator: true,
					SampleDatabase:   writeMeta(t, "broken.yaml", ": this is not: valid: yaml: ["),
				},
				want: "failed to parse metadata",
			},
			{
				name: "metadata missing name",
				opts: &spannerOptions{
					EmbeddedEmulator: true,
					SampleDatabase: writeMeta(t, "noname.yaml", `
dialect: GOOGLE_STANDARD_SQL
schemaURI: schema.sql
`),
				},
				want: "sample name is required",
			},
			{
				name: "metadata missing schemaURI",
				opts: &spannerOptions{
					EmbeddedEmulator: true,
					SampleDatabase: writeMeta(t, "noschema.json", `{
  "name": "probe",
  "dialect": "GOOGLE_STANDARD_SQL"
}`),
				},
				want: "schemaURI is required",
			},
			{
				name: "unknown dialect",
				opts: &spannerOptions{
					EmbeddedEmulator: true,
					SampleDatabase: writeMeta(t, "baddialect.yaml", `
name: probe
dialect: NOT_A_DIALECT
schemaURI: schema.sql
`),
				},
				want: "unknown dialect: NOT_A_DIALECT",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := runOffline(t, tt.opts)
				requireErrorContains(t, err, tt.want)
			})
		}
	})
}

func TestWriteUsageTo(t *testing.T) {
	t.Parallel()

	t.Run("nil context is a no-op", func(t *testing.T) {
		t.Parallel()
		var dest bytes.Buffer
		writeUsageTo(nil, nil, &dest)
		if dest.Len() != 0 {
			t.Errorf("writeUsageTo(nil) wrote %q", dest.String())
		}
	})

	t.Run("writes usage to destination and restores parser stdout", func(t *testing.T) {
		t.Parallel()

		var parserStdout, parserStderr, dest bytes.Buffer
		gopts := newGlobalOptions()
		parser, err := newFlagParser(&gopts, "built from source", nil, &parserStdout, &parserStderr)
		if err != nil {
			t.Fatalf("newFlagParser() error = %v", err)
		}
		kctx, err := parser.Parse([]string{"--project", "p", "--instance", "i", "--database", "d"})
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		originalStdout := kctx.Stdout
		before := parserStdout.String()
		writeUsageTo(kctx, parser, &dest)

		if kctx.Stdout != originalStdout {
			t.Error("kong context stdout was not restored")
		}
		if parserStdout.String() != before {
			t.Errorf("parser stdout changed: before %q after %q", before, parserStdout.String())
		}

		usage := dest.String()
		if usage == "" {
			t.Fatal("writeUsageTo() wrote no usage text")
		}
		for _, want := range []string{
			"Usage:",
			"spanner-mycli",
			"--project",
			"--instance",
			"--database",
		} {
			requireContains(t, usage, want, "usage destination")
		}
	})

	t.Run("parse error context usage goes to destination not original stdout", func(t *testing.T) {
		t.Parallel()

		var parserStdout, dest bytes.Buffer
		_, parser, err := parseFlagsArgs([]string{"--not-a-real-flag"}, "built from source", nil, &parserStdout, io.Discard)
		var parseErr *kong.ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("Parse() error = %v (%T), want *kong.ParseError", err, err)
		}
		if parseErr.Context == nil {
			t.Fatal("ParseError.Context is nil")
		}

		originalStdout := parseErr.Context.Stdout
		before := parserStdout.String()
		writeUsageTo(parseErr.Context, parser, &dest)

		if parseErr.Context.Stdout != originalStdout {
			t.Error("ParseError context stdout was not restored")
		}
		if parserStdout.String() != before {
			t.Errorf("original stdout gained %q", parserStdout.String()[len(before):])
		}
		requireContains(t, dest.String(), "Usage:", "parse-error usage")
		requireContains(t, dest.String(), "--project", "parse-error usage")
	})
}
