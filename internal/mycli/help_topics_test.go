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
	"context"
	"strings"
	"testing"
)

func TestHelpTopicsParse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input string
		topic string
	}{
		{"HELP CLI_FORMAT", "CLI_FORMAT"},
		{"help output", "OUTPUT"},
		{"HELP KEYS", "KEYS"},
		{"HELP UNKNOWN_VARIABLE", "UNKNOWN_VARIABLE"},
	} {
		stmt, err := BuildStatement(tc.input)
		if err != nil {
			t.Fatalf("%q: %v", tc.input, err)
		}
		help, ok := stmt.(*HelpTopicStatement)
		if !ok || help.Topic != tc.topic {
			t.Errorf("%q: got %#v, want topic %q", tc.input, stmt, tc.topic)
		}
	}
	if stmt, err := BuildStatement("HELP VARIABLES"); err != nil {
		t.Fatal(err)
	} else if _, ok := stmt.(*HelpVariablesStatement); !ok {
		t.Fatalf("HELP VARIABLES dispatched to %T", stmt)
	}
	if stmt, err := BuildStatement("HELP"); err != nil {
		t.Fatal(err)
	} else if _, ok := stmt.(*HelpStatement); !ok {
		t.Fatalf("HELP dispatched to %T", stmt)
	}
}

func TestHelpTopicVariableUsesRegistryAndStartupBaseline(t *testing.T) {
	t.Parallel()
	session := newDetachedTestSession(&bytes.Buffer{})
	defer session.Close()
	sv := session.systemVariables
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("CLI_FORMAT", "JSONL"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := (&HelpTopicStatement{Topic: "CLI_FORMAT"}).Execute(t.Context(), session, OperationOutput{w: &out}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Controls output format", "Current value", "JSONL", "Built-in default", "TABLE", "Startup baseline", "CSV",
		"Allowed values", "'VERTICAL'", "read,write,local,reset", "SET CLI_FORMAT = 'VERTICAL';",
		"SET LOCAL CLI_FORMAT", "RESET CLI_FORMAT;",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help missing %q in %s", want, out.String())
		}
	}
}

func TestHelpTopicsInteractiveExamplesAndMetaEntry(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cli := newConnectedTestCli(t, &out)
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"HELP OUTPUT", []string{"SET CLI_FORMAT = 'VERTICAL'", `\. file.sql`, `\o results.txt`}},
		{"HELP KEYS", []string{"Ctrl+T", "Ctrl+C", "Ctrl+J", "Tab", `\?`}},
		{`\?`, []string{"Ctrl+T", `\?`}},
	} {
		out.Reset()
		var stmt Statement
		var err error
		if IsMetaCommand(tc.input) {
			stmt, err = ParseMetaCommand(tc.input)
		} else {
			stmt, err = BuildStatement(tc.input)
		}
		if err != nil {
			t.Fatalf("%q: %v", tc.input, err)
		}
		if _, err := cli.executeStatement(context.Background(), stmt, false, tc.input, &out); err != nil {
			t.Fatalf("%q: %v", tc.input, err)
		}
		for _, want := range tc.want {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%q output missing %q in %s", tc.input, want, out.String())
			}
		}
	}
	if _, err := ParseMetaCommand(`\? extra`); err == nil {
		t.Fatal("\\? accepted arguments")
	}
	if _, err := (&HelpTopicStatement{Topic: "MISSING"}).Execute(t.Context(), cli.SessionHandler.GetSession(), OperationOutput{}); err == nil || !strings.Contains(err.Error(), "HELP VARIABLES") {
		t.Fatalf("unknown topic error = %v", err)
	}
}
