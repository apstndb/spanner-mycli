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
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/apstndb/spancodec"
)

// HelpTopicStatement gives focused help without maintaining another variable catalog.
type HelpTopicStatement struct {
	Topic string
}

func (s *HelpTopicStatement) isDetachedCompatible()           {}
func (s *HelpTopicStatement) allowedDuringSavepointRecovery() {}

type helpDetailRow struct {
	Item   string `spanner:"Item"`
	Detail string `spanner:"Detail"`
}

var helpDetailRowEncoder = spancodec.MustNewRowEncoder[helpDetailRow]()

func (s *HelpTopicStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	topic := strings.ToUpper(s.Topic)
	var rows []helpDetailRow
	switch topic {
	case "OUTPUT":
		rows = []helpDetailRow{
			{"Result format", "HELP CLI_FORMAT; shows the current format and allowed values."},
			{"Change format", "SET CLI_FORMAT = 'VERTICAL'; (or 'TABLE', 'CSV', 'JSONL', 'TSV', etc.)"},
			{"Inspect format", "SHOW VARIABLE CLI_FORMAT;"},
			{"Restore startup format", "RESET CLI_FORMAT; restores the value captured after flags and --set."},
			{"Save query output", `\o results.txt redirects output to a file; \O restores screen output.`},
			{"Copy query output", `\T results.txt tees output to screen and file; \t disables tee.`},
			{"Run a script", `\. file.sql in the interactive CLI; --file file.sql at startup.`},
		}
	case "QUEUES":
		rows = []helpDetailRow{
			{"Server support", "Requires a queue-capable Spanner/Omni server; availability depends on the server build."},
			{"Receive promptly", "SET CLI_FORMAT = 'JSONL'; (or 'CSV', 'TAB', 'VERTICAL') avoids table width buffering."},
			{"Streaming table", "SET CLI_TABLE_STREAMING = 'TRUE'; SET CLI_TABLE_PREVIEW_ROWS = 1; previews one row; 0 uses header widths only."},
			{"Default table", "TABLE buffers results by default. Even with streaming enabled, the default width preview waits for 50 rows or query completion."},
			{"Strong read", "Outside an active transaction: SET READ_ONLY_STALENESS = 'STRONG'; RECEIVE rejects stale reads."},
			{"Bounded receive", "SET STATEMENT_TIMEOUT = '1m'; SELECT * FROM RECEIVE_Tasks(max_duration => '30s'); replace Tasks with your queue name."},
			{"Timeout", "max_duration is the server receive window; STATEMENT_TIMEOUT independently bounds the client statement."},
			{"Delivery semantics", "RECEIVE leases messages; it is not a passive peek. Delivery is at least once. Displaying a row does not acknowledge it."},
			{"Lease management", "The CLI does not automatically renew or acknowledge messages. Use RENEWLEASE_<queue> explicitly and acknowledge only after processing."},
			{"Parser compatibility", "If MEMEFISH_ONLY rejects queue DDL or ASSERT_ROWS_MODIFIED, SET CLI_PARSE_MODE = 'NO_MEMEFISH'; passes native SQL using lexical classification."},
			{"Restore settings", "RESET CLI_FORMAT; RESET CLI_TABLE_STREAMING; RESET CLI_TABLE_PREVIEW_ROWS; RESET READ_ONLY_STALENESS; RESET STATEMENT_TIMEOUT; RESET CLI_PARSE_MODE; restore startup values."},
		}
	case "KEYS":
		rows = []helpDetailRow{
			{"Complete a statement or argument", "Ctrl+T by default; configured once when the interactive editor starts."},
			{"Choose a different key", "--set CLI_FUZZY_FINDER_KEY=M_F at startup (or use startup config)."},
			{"Change active binding", "In-session SET/RESET does not rebind; configure at startup and relaunch."},
			{"Cancel editing", "Ctrl+C interrupts the current input and returns to the prompt."},
			{"Insert a line", "Ctrl+J inserts a newline without submitting the statement."},
			{"Tab", "Inserts indentation; it does not invoke completion."},
			{"Help", `\? displays this guide without a semicolon; HELP OUTPUT; covers formats and files.`},
			{"Run a script", `\. file.sql in the interactive CLI; --file file.sql at startup.`},
			{"Shell command", `\! pwd runs a noninteractive shell command.`},
			{"Switch database", `\u mydb switches database; USE mydb; is the SQL form.`},
			{"Change prompt", `\R mycli> changes the prompt.`},
			{"Output to file", `\o results.txt redirects; \O restores screen output.`},
			{"Copy output", `\T session.log tees to file; \t stops tee.`},
		}
		if session != nil {
			configured := session.systemVariables.Feature.FuzzyFinderKey
			if configured == "" {
				configured = "(empty; disables completion when set before editor startup)"
			}
			rows = append(rows, helpDetailRow{"Configured key now", configured + " (stored value; active binding may differ)"})
		}
	default:
		var sysVars *systemVariables
		if session == nil {
			defaults := newSystemVariablesWithDefaults()
			defaults.ensureRegistry()
			sysVars = &defaults
		} else {
			sysVars = session.systemVariables
			sysVars.ensureRegistry()
		}
		def := sysVars.Registry.lookupDef(topic)
		if def == nil {
			return nil, fmt.Errorf("unknown help topic %q; try HELP OUTPUT;, HELP KEYS;, HELP QUEUES;, or HELP VARIABLES;", s.Topic)
		}
		name := def.name
		info := sysVars.ListVariableInfo()[name]
		access := info.operations()
		if def.initOnly && !info.ReadOnly && !info.Unimplemented {
			access += " (write before session creation only)"
		}
		rows = []helpDetailRow{
			{"Variable", name},
			{"Description", info.Description},
			{"Access", access},
		}
		if session != nil {
			if value, err := currentHelpValue(sysVars.Registry, name); err == nil {
				rows = append(rows, helpDetailRow{"Current value", value})
			} else if errors.Is(err, errIgnored) {
				rows = append(rows, helpDetailRow{"Current value", "Unavailable (no value yet)"})
			} else {
				rows = append(rows, helpDetailRow{"Current value", "Unavailable: " + err.Error()})
			}
		}
		// Built-in defaults come from the core constructor; startup baselines
		// below reflect actual flags and --set values and may differ.
		defaults := newSystemVariablesWithDefaults()
		defaults.ensureRegistry()
		if defaults.Registry.lookupDef(name) != nil {
			if value, err := currentHelpValue(defaults.Registry, name); err == nil {
				rows = append(rows, helpDetailRow{"Built-in default", value})
			}
		}
		if baseline, ok := sysVars.startupSnapshots[name]; ok {
			rows = append(rows, helpDetailRow{"Startup baseline", baseline + " (captured after flags and --set; RESET restores this value)"})
		} else if info.Resettable && session != nil {
			rows = append(rows, helpDetailRow{"Startup baseline", "Unavailable in this session"})
		}
		if values := validValuesForHelp(sysVars.Registry.GetVariable(name)); len(values) > 0 {
			rows = append(rows, helpDetailRow{"Allowed values", strings.Join(values, ", ")})
		}
		rows = append(rows, helpDetailRow{"Inspect", "SHOW VARIABLE " + name + ";"})
		if def.initOnly && !info.ReadOnly && !info.Unimplemented {
			example := "--set " + name + "=<value>"
			if name == "CLI_ENABLE_ADC_PLUS" {
				example = "--set CLI_ENABLE_ADC_PLUS=FALSE"
			}
			rows = append(rows, helpDetailRow{"Change before session creation", example + " (at startup; interactive SET is not supported)"})
		} else if !info.ReadOnly && !info.Unimplemented {
			example := "SET " + name + " = <value>;"
			if name == "CLI_FORMAT" {
				example = "SET CLI_FORMAT = 'VERTICAL';"
			}
			rows = append(rows, helpDetailRow{"Change", example})
		}
		if info.LocalAllowed {
			rows = append(rows, helpDetailRow{"Transaction scope", "BEGIN; SET LOCAL " + name + " = <value>; COMMIT;"})
		}
		if info.Resettable {
			rows = append(rows, helpDetailRow{"Restore startup baseline", "RESET " + name + ";"})
		}
	}
	result, err := executeStructRows(helpDetailRowEncoder, rows, session, out)
	if err != nil {
		return nil, err
	}
	result.KeepVariables = true
	return result, nil
}

func validValuesForHelp(v Variable) []string {
	if enumerator, ok := unwrapCustomVar(v).(ValidValuesEnumerator); ok {
		return enumerator.ValidValues()
	}
	return nil
}

func currentHelpValue(registry *VarRegistry, name string) (string, error) {
	if multi, ok := registry.GetVariable(name).(MultiValueVar); ok {
		values, err := multi.GetMulti()
		if err != nil {
			return "", err
		}
		parts := make([]string, 0, len(values))
		for key, value := range values {
			parts = append(parts, key+"="+value)
		}
		slices.Sort(parts)
		return strings.Join(parts, ", "), nil
	}
	return registry.Get(name)
}
