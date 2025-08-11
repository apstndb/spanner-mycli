package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanemuboost"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/creack/pty"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jessevdk/go-flags"
	"github.com/samber/lo"
)

// Helper to create stdin based on usePTY flag
type stdinProvider func() (io.Reader, func(), error)

func nonPTYStdin(content string) stdinProvider {
	return func() (io.Reader, func(), error) {
		return strings.NewReader(content), func() {}, nil
	}
}

func ptyStdin() stdinProvider {
	return func() (io.Reader, func(), error) {
		pty, tty, err := pty.Open()
		if err != nil {
			return nil, nil, err
		}
		cleanup := func() {
			pty.Close()
			tty.Close()
		}
		return tty, cleanup, nil
	}
}

// TestParseFlagsCombinations tests various flag combinations for conflicts and mutual exclusivity
func TestParseFlagsCombinations(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
	}{
		// Alias conflicts
		{
			name:    "execute and sql are aliases (both allowed)",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--sql", "SELECT 2"},
			wantErr: false,
		},
		{
			name: "role and database-role both set (aliases allowed)",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--role", "role1", "--database-role", "role2"},
			// Note: Both can be set during parsing, but initializeSystemVariables prefers --role
			wantErr: false,
		},
		{
			name:    "insecure and skip-tls-verify are aliases (both allowed)",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--insecure", "--skip-tls-verify"},
			wantErr: false,
		},
		{
			name: "endpoint and deployment-endpoint both set (aliases allowed)",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "endpoint1:443", "--deployment-endpoint", "endpoint2:443"},
			// Note: Both can be set during parsing, but initializeSystemVariables prefers --endpoint
			wantErr: false,
		},

		// Mutually exclusive operations
		{
			name:        "execute and file are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--file", "query.sql"},
			wantErr:     true,
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
		{
			name:        "strong and read-timestamp are mutually exclusive",
			args:        []string{"--strong", "--read-timestamp", "2023-01-01T00:00:00Z"},
			wantErr:     true,
			errContains: "--strong and --read-timestamp are mutually exclusive",
		},
		{
			name:        "all three input methods (execute, file, sql) are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--file", "query.sql", "--sql", "SELECT 2"},
			wantErr:     true,
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},

		// Invalid combinations
		{
			name:        "endpoint and host are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "spanner.googleapis.com:443", "--host", "example.com"},
			wantErr:     true,
			errContains: "--endpoint and (--host or --port) are mutually exclusive",
		},
		{
			name:        "endpoint and port are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "spanner.googleapis.com:443", "--port", "9010"},
			wantErr:     true,
			errContains: "--endpoint and (--host or --port) are mutually exclusive",
		},
		{
			name:        "endpoint and both host/port are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "spanner.googleapis.com:443", "--host", "example.com", "--port", "9010"},
			wantErr:     true,
			errContains: "--endpoint and (--host or --port) are mutually exclusive",
		},
		{
			name:        "try-partition-query requires SQL input",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query"},
			wantErr:     true,
			errContains: "--try-partition-query requires SQL input via --execute(-e), --file(-f), --source, or --sql",
		},
		{
			name: "table flag without SQL input in batch mode",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--table"},
			// Table flag is valid without SQL input - it affects output format
			wantErr: false,
		},
		{
			name:    "try-partition-query with execute is valid",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--execute", "SELECT 1"},
			wantErr: false,
		},
		{
			name:    "try-partition-query with sql is valid",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--sql", "SELECT 1"},
			wantErr: false,
		},
		{
			name:    "try-partition-query with file is valid",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--file", "query.sql"},
			wantErr: false,
		},
		{
			name:    "try-partition-query with source is valid",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--source", "query.sql"},
			wantErr: false,
		},
		{
			name:        "execute and source are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--source", "query.sql"},
			wantErr:     true,
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
		{
			name:        "all four input methods are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--file", "query.sql", "--sql", "SELECT 2", "--source", "query3.sql"},
			wantErr:     true,
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},

		// Embedded emulator tests
		{
			name:    "embedded-emulator without connection params",
			args:    []string{"--embedded-emulator"},
			wantErr: false,
		},
		{
			name:    "embedded-emulator with custom image",
			args:    []string{"--embedded-emulator", "--emulator-image", "gcr.io/spanner-emulator/emulator:latest"},
			wantErr: false,
		},
		{
			name:    "embedded-emulator ignores connection params",
			args:    []string{"--embedded-emulator", "--project", "ignored", "--instance", "ignored", "--database", "ignored"},
			wantErr: false,
		},

		// Missing required parameters
		{
			name:        "missing project without embedded emulator",
			args:        []string{"--instance", "i", "--database", "d"},
			wantErr:     true,
			errContains: "missing parameters: -p, -i are required",
		},
		{
			name:        "missing instance without embedded emulator",
			args:        []string{"--project", "p", "--database", "d"},
			wantErr:     true,
			errContains: "missing parameters: -p, -i are required",
		},
		{
			name:        "missing database without embedded emulator or detached",
			args:        []string{"--project", "p", "--instance", "i"},
			wantErr:     true,
			errContains: "missing parameter: -d is required",
		},
		{
			name:    "detached mode doesn't require database",
			args:    []string{"--project", "p", "--instance", "i", "--detached"},
			wantErr: false,
		},
		{
			name:    "embedded emulator doesn't require project/instance/database",
			args:    []string{"--embedded-emulator"},
			wantErr: false,
		},

		// Valid combinations
		{
			name:    "minimal valid flags",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d"},
			wantErr: false,
		},
		{
			name:    "only insecure flag",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--insecure"},
			wantErr: false,
		},
		{
			name:    "only skip-tls-verify flag",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--skip-tls-verify"},
			wantErr: false,
		},
		{
			name:    "only strong flag",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--strong"},
			wantErr: false,
		},
		{
			name:    "only read-timestamp flag",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--read-timestamp", "2023-01-01T00:00:00Z"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			// First check if parsing itself failed
			if err != nil && !tt.wantErr {
				t.Errorf("ParseArgs() unexpected error: %v", err)
				return
			}

			// If parsing succeeded, validate the options
			if err == nil {
				err = ValidateSpannerOptions(&gopts.Spanner)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("flag validation error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestParseFlagsValidation tests flag value validation
func TestParseFlagsValidation(t *testing.T) {
	// Use an existing test proto descriptor file
	validProtoFile := "testdata/protos/order_descriptors.pb"

	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
	}{
		// Enum validation
		{
			name:        "invalid priority value",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "INVALID"},
			wantErr:     true,
			errContains: "must be one of:",
		},
		{
			name:    "valid priority HIGH",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "HIGH"},
			wantErr: false,
		},
		{
			name:    "valid priority MEDIUM",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "MEDIUM"},
			wantErr: false,
		},
		{
			name:    "valid priority LOW",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "LOW"},
			wantErr: false,
		},
		{
			name:        "invalid query-mode value",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "INVALID"},
			wantErr:     true,
			errContains: "Invalid value `INVALID' for option `--query-mode'",
		},
		{
			name:    "valid query-mode NORMAL",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "NORMAL"},
			wantErr: false,
		},
		{
			name:    "valid query-mode PLAN",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "PLAN"},
			wantErr: false,
		},
		{
			name:    "valid query-mode PROFILE",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "PROFILE"},
			wantErr: false,
		},
		{
			name:        "invalid database-dialect value",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--database-dialect", "INVALID"},
			wantErr:     true,
			errContains: "Invalid value `INVALID' for option `--database-dialect'",
		},
		{
			name:    "valid database-dialect POSTGRESQL",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--database-dialect", "POSTGRESQL"},
			wantErr: false,
		},
		{
			name:    "valid database-dialect GOOGLE_STANDARD_SQL",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--database-dialect", "GOOGLE_STANDARD_SQL"},
			wantErr: false,
		},

		// Format validation
		{
			name:        "invalid directed-read format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "invalid:format:too:many"},
			wantErr:     true,
			errContains: "directed read option must be in the form of <replica_location>:<replica_type>",
		},
		{
			name:    "valid directed-read with location only",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1"},
			wantErr: false,
		},
		{
			name:    "valid directed-read with READ_ONLY",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1:READ_ONLY"},
			wantErr: false,
		},
		{
			name:    "valid directed-read with READ_WRITE",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1:READ_WRITE"},
			wantErr: false,
		},
		{
			name:        "invalid directed-read replica type",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1:INVALID"},
			wantErr:     true,
			errContains: "<replica_type> must be either READ_WRITE or READ_ONLY",
		},
		{
			name:        "invalid read-timestamp format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--read-timestamp", "invalid-timestamp"},
			wantErr:     true,
			errContains: "error on parsing --read-timestamp",
		},
		{
			name:    "valid read-timestamp",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--read-timestamp", "2023-01-01T00:00:00Z"},
			wantErr: false,
		},
		{
			name:        "invalid timeout format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "invalid"},
			wantErr:     true,
			errContains: "invalid value of --timeout",
		},
		{
			name:    "valid timeout",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "30s"},
			wantErr: false,
		},
		{
			name:        "invalid param format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--param", "invalid syntax"},
			wantErr:     true,
			errContains: "error on parsing --param",
		},
		{
			name:    "valid param with string value",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--param", "p1='hello'"},
			wantErr: false,
		},
		{
			name:    "valid param with type",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--param", "p1=STRING"},
			wantErr: false,
		},
		{
			name:        "invalid set value for boolean",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "READONLY=not-a-bool"},
			wantErr:     true,
			errContains: "failed to set system variable",
		},
		{
			name:    "valid set value",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "READONLY=true"},
			wantErr: false,
		},

		// File validation
		{
			name:        "non-existent proto descriptor file",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--proto-descriptor-file", "non-existent-file.pb"},
			wantErr:     true,
			errContains: "error on --proto-descriptor-file",
		},
		{
			name:    "valid proto descriptor file",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--proto-descriptor-file", validProtoFile},
			wantErr: false,
		},
		{
			name:        "invalid log level",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--log-level", "INVALID"},
			wantErr:     true,
			errContains: "error on parsing --log-level",
		},
		{
			name:    "valid log level",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--log-level", "INFO"},
			wantErr: false,
		},

		// Credential file tests
		{
			name: "valid credential file",
			// Use a placeholder that will be replaced in the test
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--credential", "__TEMP_CRED_FILE__"},
			wantErr: false,
		},
		{
			name: "non-existent credential file",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--credential", "/non/existent/cred.json"},
			// Note: This won't fail during flag parsing/validation, only during run()
			wantErr: false,
		},

		// MCP mode tests
		{
			name:    "mcp mode with all required params",
			args:    []string{"--project", "p", "--instance", "i", "--database", "d", "--mcp"},
			wantErr: false,
		},
		{
			name:    "mcp mode with embedded emulator",
			args:    []string{"--embedded-emulator", "--mcp"},
			wantErr: false,
		},

		// Complex flag combinations
		{
			name: "multiple set flags",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--set", "CLI_FORMAT=VERTICAL", "--set", "READONLY=true", "--set", "AUTOCOMMIT_DML_MODE=PARTITIONED_NON_ATOMIC",
			},
			wantErr: false,
		},
		{
			name: "multiple param flags",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--param", "p1='value1'", "--param", "p2=INT64", "--param", "p3=ARRAY<STRING>",
			},
			wantErr: false,
		},
		{
			name: "invalid param syntax",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--param", "p1=@{invalid}",
			},
			wantErr:     true,
			errContains: "error on parsing --param",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Handle special placeholder for credential file test
			args := tt.args
			if tt.name == "valid credential file" {
				tempDir := t.TempDir()
				credFile := filepath.Join(tempDir, "cred.json")
				if err := os.WriteFile(credFile, []byte{}, 0o644); err != nil {
					t.Fatal(err)
				}
				// Replace placeholder with actual temp file path
				for i, arg := range args {
					if arg == "__TEMP_CRED_FILE__" {
						args[i] = credFile
						break
					}
				}
			}

			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(args)
			// First check if parsing itself failed
			if err != nil {
				if !tt.wantErr {
					t.Errorf("ParseArgs() unexpected error: %v", err)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("ParseArgs() error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
				return
			}

			// If parsing succeeded, validate the options and initialize system variables
			if err == nil {
				err = ValidateSpannerOptions(&gopts.Spanner)
				if err == nil {
					_, err = initializeSystemVariables(&gopts.Spanner)
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("flag validation error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestFlagSystemVariablePrecedence tests the precedence of flags vs system variables
func TestFlagSystemVariablePrecedence(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		envVars       map[string]string
		configContent string
		wantProject   string
		wantInstance  string
		wantDatabase  string
		wantPriority  sppb.RequestOptions_Priority
		wantEndpoint  string
		wantRole      string
		wantLogGrpc   bool
		wantInsecure  bool
	}{
		{
			name: "CLI flags override environment variables",
			args: []string{"--project", "cli-project", "--instance", "cli-instance", "--database", "cli-database", "--priority", "HIGH"},
			envVars: map[string]string{
				"SPANNER_PROJECT_ID":  "env-project",
				"SPANNER_INSTANCE_ID": "env-instance",
				"SPANNER_DATABASE_ID": "env-database",
			},
			wantProject:  "cli-project",
			wantInstance: "cli-instance",
			wantDatabase: "cli-database",
			wantPriority: sppb.RequestOptions_PRIORITY_HIGH,
		},
		{
			name: "Environment variables used when no CLI flags",
			args: []string{},
			envVars: map[string]string{
				"SPANNER_PROJECT_ID":  "env-project",
				"SPANNER_INSTANCE_ID": "env-instance",
				"SPANNER_DATABASE_ID": "env-database",
			},
			wantProject:  "env-project",
			wantInstance: "env-instance",
			wantDatabase: "env-database",
			wantPriority: defaultPriority,
		},
		{
			name: "Config file values with CLI overrides",
			args: []string{"--project", "cli-project"},
			configContent: `[spanner]
project = config-project
instance = config-instance
database = config-database
endpoint = config-endpoint:443
role = config-role
log-grpc = true
insecure = true
`,
			wantProject:  "cli-project",
			wantInstance: "config-instance",
			wantDatabase: "config-database",
			wantEndpoint: "config-endpoint:443",
			wantRole:     "config-role",
			wantLogGrpc:  true,
			wantInsecure: true,
			wantPriority: defaultPriority,
		},
		{
			name: "--set flag overrides direct flag for system variables",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--priority", "LOW",
				"--set", "RPC_PRIORITY=HIGH",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
			wantPriority: sppb.RequestOptions_PRIORITY_HIGH, // --set overrides --priority
		},
		{
			name: "database-role alias preferred over role",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--role", "role1",
				"--database-role", "role2",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
			wantRole:     "role1", // --role is preferred when both are set
			wantPriority: defaultPriority,
		},
		{
			name: "skip-tls-verify sets insecure",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--skip-tls-verify",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
			wantInsecure: true,
			wantPriority: defaultPriority,
		},
		{
			name: "deployment-endpoint alias sets endpoint",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--deployment-endpoint", "custom-endpoint:443",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
			wantEndpoint: "custom-endpoint:443",
			wantPriority: defaultPriority,
		},
		{
			name: "endpoint preferred over deployment-endpoint",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--endpoint", "primary:443",
				"--deployment-endpoint", "secondary:443",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
			wantEndpoint: "primary:443", // --endpoint is preferred when both are set
			wantPriority: defaultPriority,
		},
		{
			name: "detached mode clears database",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--detached",
			},
			envVars: map[string]string{
				"SPANNER_DATABASE_ID": "env-database",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "", // detached overrides everything
			wantPriority: defaultPriority,
		},
		{
			name: "Precedence order: CLI > env > config > defaults",
			args: []string{"--priority", "LOW"},
			envVars: map[string]string{
				"SPANNER_PROJECT_ID":  "env-project",
				"SPANNER_INSTANCE_ID": "env-instance",
				"SPANNER_DATABASE_ID": "env-database",
			},
			configContent: `[spanner]
project = config-project
instance = config-instance
database = config-database
priority = HIGH
`,
			wantProject:  "env-project",                    // env overrides config
			wantInstance: "env-instance",                   // env overrides config
			wantDatabase: "env-database",                   // env overrides config
			wantPriority: sppb.RequestOptions_PRIORITY_LOW, // CLI overrides config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables. t.Setenv automatically handles cleanup.
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// For config file tests, we need to handle parsing differently
			var gopts globalOptions
			if tt.configContent != "" {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, cnfFileName)
				if err := os.WriteFile(configFile, []byte(tt.configContent), 0o644); err != nil {
					t.Fatalf("Failed to create config file: %v", err)
				}

				// Initialize with defaults like parseFlags does
				gopts.Spanner.EmulatorImage = spanemuboost.DefaultEmulatorImage

				// Process config file directly by calling the INI parser
				// to avoid changing the global current working directory (os.Chdir).
				configParser := flags.NewParser(&gopts, flags.Default)
				iniParser := flags.NewIniParser(configParser)
				if err := iniParser.ParseFile(configFile); err != nil {
					t.Fatalf("Failed to read config file: %v", err)
				}

				// Then parse command line args
				flagParser := flags.NewParser(&gopts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
				_, err := flagParser.ParseArgs(tt.args)
				if err != nil {
					t.Fatalf("Failed to parse flags: %v", err)
				}
			} else {
				// For non-config tests, just parse args
				parser := flags.NewParser(&gopts, flags.Default)
				_, err := parser.ParseArgs(tt.args)
				if err != nil {
					t.Fatalf("Failed to parse flags: %v", err)
				}
			}

			// Initialize system variables
			sysVars, err := initializeSystemVariables(&gopts.Spanner)
			if err != nil {
				t.Fatalf("Failed to initialize system variables: %v", err)
			}

			// Check results
			if sysVars.Project != tt.wantProject {
				t.Errorf("Project = %q, want %q", sysVars.Project, tt.wantProject)
			}
			if sysVars.Instance != tt.wantInstance {
				t.Errorf("Instance = %q, want %q", sysVars.Instance, tt.wantInstance)
			}
			if sysVars.Database != tt.wantDatabase {
				t.Errorf("Database = %q, want %q", sysVars.Database, tt.wantDatabase)
			}
			if sysVars.RPCPriority != tt.wantPriority {
				t.Errorf("RPCPriority = %v, want %v", sysVars.RPCPriority, tt.wantPriority)
			}
			// Check endpoint by constructing from host and port
			var gotEndpoint string
			if sysVars.Host != "" && sysVars.Port != 0 {
				gotEndpoint = fmt.Sprintf("%s:%d", sysVars.Host, sysVars.Port)
			}
			if gotEndpoint != tt.wantEndpoint {
				t.Errorf("Endpoint (constructed from Host:Port) = %q, want %q", gotEndpoint, tt.wantEndpoint)
			}
			if sysVars.Role != tt.wantRole {
				t.Errorf("Role = %q, want %q", sysVars.Role, tt.wantRole)
			}
			if sysVars.LogGrpc != tt.wantLogGrpc {
				t.Errorf("LogGrpc = %v, want %v", sysVars.LogGrpc, tt.wantLogGrpc)
			}
			if sysVars.Insecure != tt.wantInsecure {
				t.Errorf("Insecure = %v, want %v", sysVars.Insecure, tt.wantInsecure)
			}
		})
	}
}

// TestFlagSpecialModes tests special modes like embedded emulator and MCP
func TestFlagSpecialModes(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantProject   string
		wantInstance  string
		wantDatabase  string
		wantInsecure  bool
		wantVerbose   bool
		wantMCP       bool
		wantCLIFormat enums.DisplayMode
		checkAfterRun bool // Some values are set in run() function
	}{
		{
			name:          "embedded emulator with no explicit values uses defaults",
			args:          []string{"--embedded-emulator"},
			wantProject:   "emulator-project",  // Default set in initializeSystemVariables
			wantInstance:  "emulator-instance", // Default set in initializeSystemVariables
			wantDatabase:  "emulator-database", // Default set in initializeSystemVariables
			wantInsecure:  true,
			checkAfterRun: true, // Endpoint and WithoutAuthentication set in run()
		},
		{
			name: "embedded emulator with explicit values uses user values",
			args: []string{
				"--embedded-emulator",
				"--project", "my-project",
				"--instance", "my-instance",
				"--database", "my-database",
			},
			wantProject:  "my-project",
			wantInstance: "my-instance",
			wantDatabase: "my-database",
			wantInsecure: true, // --embedded-emulator always sets insecure=true
		},
		{
			name: "embedded emulator with detached mode",
			args: []string{
				"--embedded-emulator",
				"--detached",
			},
			wantProject:  "emulator-project",  // Default set in initializeSystemVariables
			wantInstance: "emulator-instance", // Default set in initializeSystemVariables
			wantDatabase: "",                  // Empty - respects detached mode
			wantInsecure: true,
		},
		{
			name: "MCP mode sets verbose",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--mcp",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
			wantVerbose:  true,
			wantMCP:      true,
		},
		{
			name: "MCP mode always sets verbose",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--mcp",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
			wantVerbose:  true, // MCP always sets verbose
			wantMCP:      true,
		},
		{
			name: "batch mode with --table flag",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--execute", "SELECT 1",
				"--table",
			},
			wantProject:   "p",
			wantInstance:  "i",
			wantDatabase:  "d",
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name: "batch mode without --table defaults to table format",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--execute", "SELECT 1",
			},
			wantProject:   "p",
			wantInstance:  "i",
			wantDatabase:  "d",
			wantCLIFormat: enums.DisplayModeTable,
		},
		// Note: Interactive mode test removed because it requires a real terminal
		// which is difficult to simulate in unit tests
		{
			name: "--set CLI_FORMAT overrides defaults",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--execute", "SELECT 1",
				"--set", "CLI_FORMAT=VERTICAL",
			},
			wantProject:   "p",
			wantInstance:  "i",
			wantDatabase:  "d",
			wantCLIFormat: enums.DisplayModeVertical,
		},
		{
			name: "enable-partitioned-dml sets AUTOCOMMIT_DML_MODE",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--enable-partitioned-dml",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			if err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Validate and initialize system variables
			if err := ValidateSpannerOptions(&gopts.Spanner); err != nil {
				t.Fatalf("Failed to validate options: %v", err)
			}

			sysVars, err := initializeSystemVariables(&gopts.Spanner)
			if err != nil {
				t.Fatalf("Failed to initialize system variables: %v", err)
			}

			// Check results
			if sysVars.Project != tt.wantProject {
				t.Errorf("Project = %q, want %q", sysVars.Project, tt.wantProject)
			}
			if sysVars.Instance != tt.wantInstance {
				t.Errorf("Instance = %q, want %q", sysVars.Instance, tt.wantInstance)
			}
			if sysVars.Database != tt.wantDatabase {
				t.Errorf("Database = %q, want %q", sysVars.Database, tt.wantDatabase)
			}
			if sysVars.Insecure != tt.wantInsecure {
				t.Errorf("Insecure = %v, want %v", sysVars.Insecure, tt.wantInsecure)
			}
			if sysVars.Verbose != tt.wantVerbose {
				t.Errorf("Verbose = %v, want %v", sysVars.Verbose, tt.wantVerbose)
			}
			if sysVars.MCP != tt.wantMCP {
				t.Errorf("MCP = %v, want %v", sysVars.MCP, tt.wantMCP)
			}

			// For CLI_FORMAT, we need to simulate what run() does
			// Check if this test case has specified a desired CLI_FORMAT value
			if tt.wantCLIFormat != 0 {
				// Determine if this would be interactive mode
				_, _, err := determineInputAndMode(&gopts.Spanner, bytes.NewReader(nil))
				if err != nil {
					t.Fatalf("Failed to determine input mode: %v", err)
				}

				// Apply the same logic as in run()
				if _, hasSet := gopts.Spanner.Set["CLI_FORMAT"]; !hasSet {
					// Use the shared function to get format from options
					formatValue := getFormatFromOptions(&gopts.Spanner)
					var expectedFormat enums.DisplayMode
					if formatValue != "" {
						// Parse the format string to enum
						parsed, err := enums.DisplayModeString(formatValue)
						if err != nil {
							t.Fatalf("Invalid format value: %v", formatValue)
						}
						expectedFormat = parsed
					} else {
						// No format flags provided, use the new default
						expectedFormat = enums.DisplayModeTable
					}
					if expectedFormat != tt.wantCLIFormat {
						t.Errorf("Expected CLI_FORMAT = %v, but want %v (args: %v)",
							expectedFormat, tt.wantCLIFormat, tt.args)
					}
				} else {
					// CLI_FORMAT was set via --set, check it was applied
					if sysVars.CLIFormat != tt.wantCLIFormat {
						t.Errorf("CLIFormat = %v, want %v", sysVars.CLIFormat, tt.wantCLIFormat)
					}
				}
			}

			// Special checks for modes that set values in run()
			if tt.name == "embedded emulator with no explicit values uses defaults" && tt.checkAfterRun {
				// These would be set in run() after emulator starts
				// Just verify the flags that trigger this behavior are correct
				if !gopts.Spanner.EmbeddedEmulator {
					t.Errorf("EmbeddedEmulator flag not set")
				}
			}

			if tt.name == "enable-partitioned-dml sets AUTOCOMMIT_DML_MODE" {
				if sysVars.AutocommitDMLMode != enums.AutocommitDMLModePartitionedNonAtomic {
					t.Errorf("AutocommitDMLMode = %v, want %v", sysVars.AutocommitDMLMode, enums.AutocommitDMLModePartitionedNonAtomic)
				}
			}
		})
	}
}

// TestFlagErrorMessages tests that error messages are user-friendly and actionable
func TestFlagErrorMessages(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantErrKeyword string // Key part of the error message to check
	}{
		{
			name:           "missing required parameters shows what's missing",
			args:           []string{"--instance", "i"},
			wantErrKeyword: "-p, -i are required",
		},
		{
			name:           "missing database shows helpful message",
			args:           []string{"--project", "p", "--instance", "i"},
			wantErrKeyword: "-d is required (or use --detached",
		},
		{
			name:           "conflicting input flags shows which flags conflict",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--file", "query.sql"},
			wantErrKeyword: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
		{
			name:           "invalid enum shows valid options",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "INVALID"},
			wantErrKeyword: "must be one of:",
		},
		{
			name:           "invalid directed read shows format",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "a:b:c:d"},
			wantErrKeyword: "directed read option must be in the form of <replica_location>:<replica_type>",
		},
		{
			name:           "invalid replica type shows valid options",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1:INVALID"},
			wantErrKeyword: "<replica_type> must be either READ_WRITE or READ_ONLY",
		},
		{
			name:           "try-partition-query without input shows requirement",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query"},
			wantErrKeyword: "--try-partition-query requires SQL input via --execute(-e), --file(-f), --source, or --sql",
		},
		{
			name:           "invalid timeout shows it's a timeout error",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "invalid"},
			wantErrKeyword: "invalid value of --timeout",
		},
		{
			name:           "invalid param shows which param failed",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--param", "p1=invalid syntax"},
			wantErrKeyword: "error on parsing --param=p1=invalid syntax",
		},
		{
			name:           "non-existent proto file shows file error",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--proto-descriptor-file", "missing.pb"},
			wantErrKeyword: "error on --proto-descriptor-file, file: missing.pb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, parseErr := parser.ParseArgs(tt.args)

			var err error
			if parseErr == nil {
				// If parsing succeeded, try validation and initialization
				err = ValidateSpannerOptions(&gopts.Spanner)
				if err == nil {
					_, err = initializeSystemVariables(&gopts.Spanner)
				}
			} else {
				err = parseErr
			}

			if err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !contains(err.Error(), tt.wantErrKeyword) {
				t.Errorf("Error message %q does not contain expected keyword %q", err.Error(), tt.wantErrKeyword)
			}
		})
	}
}

// TestFileFlagBehavior tests file-related flag behavior
func TestFileFlagBehavior(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.sql")
	if err := os.WriteFile(testFile, []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		args        []string
		stdin       string
		wantErr     bool
		errContains string
		checkInput  bool
		wantInput   string
	}{
		{
			name:       "valid file flag",
			args:       []string{"--project", "p", "--instance", "i", "--database", "d", "--file", testFile},
			wantErr:    false,
			checkInput: true,
			wantInput:  "SELECT 1;",
		},
		{
			name:        "non-existent file",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--file", "/non/existent/file.sql"},
			wantErr:     true,
			errContains: "read from file /non/existent/file.sql failed",
			checkInput:  true, // Need to call determineInputAndMode to get the error
		},
		{
			name:       "file flag with dash reads from stdin",
			args:       []string{"--project", "p", "--instance", "i", "--database", "d", "--file", "-"},
			stdin:      "SELECT 2;",
			wantErr:    false,
			checkInput: true,
			wantInput:  "SELECT 2;",
		},
		{
			name:        "file flag with execute is mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--file", testFile, "--execute", "SELECT 1"},
			wantErr:     true,
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
		{
			name:        "file flag with sql is mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--file", testFile, "--sql", "SELECT 1"},
			wantErr:     true,
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			if err == nil {
				err = ValidateSpannerOptions(&gopts.Spanner)
			}

			// If no validation error, test determineInputAndMode
			if err == nil && tt.checkInput {
				input, _, determineErr := determineInputAndMode(&gopts.Spanner, bytes.NewReader([]byte(tt.stdin)))
				if determineErr != nil {
					err = determineErr
				} else if input != tt.wantInput {
					t.Errorf("determineInputAndMode() input = %q, want %q", input, tt.wantInput)
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestSpecialFlags tests special flags like --async, --statement-help, etc.
func TestSpecialFlags(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		wantAsync         bool
		wantStatementHelp bool
		wantTryPartition  bool
		wantEmulatorImage string
		wantErr           bool
		errContains       string
	}{
		{
			name:      "async flag",
			args:      []string{"--project", "p", "--instance", "i", "--database", "d", "--async"},
			wantAsync: true,
		},
		{
			name:              "statement-help flag",
			args:              []string{"--statement-help"},
			wantStatementHelp: true,
			// Note: --statement-help doesn't require project/instance/database
			// but our test calls ValidateSpannerOptions which would fail.
			// In real usage, run() checks StatementHelp before validation.
		},
		{
			name:             "try-partition-query with valid input",
			args:             []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--execute", "SELECT 1"},
			wantTryPartition: true,
		},
		{
			name:        "try-partition-query without input",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query"},
			wantErr:     true,
			errContains: "--try-partition-query requires SQL input",
		},
		{
			name:              "emulator-image flag",
			args:              []string{"--embedded-emulator", "--emulator-image", "gcr.io/my-project/my-emulator:latest"},
			wantEmulatorImage: "gcr.io/my-project/my-emulator:latest",
		},
		{
			name:              "emulator-image without embedded-emulator is ignored",
			args:              []string{"--project", "p", "--instance", "i", "--database", "d", "--emulator-image", "gcr.io/my-project/my-emulator:latest"},
			wantEmulatorImage: "gcr.io/my-project/my-emulator:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			if err == nil && !gopts.Spanner.StatementHelp {
				// Skip validation for --statement-help as it's handled before validation in run()
				err = ValidateSpannerOptions(&gopts.Spanner)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
				return
			}

			// Check flag values
			if gopts.Spanner.Async != tt.wantAsync {
				t.Errorf("Async = %v, want %v", gopts.Spanner.Async, tt.wantAsync)
			}
			if gopts.Spanner.StatementHelp != tt.wantStatementHelp {
				t.Errorf("StatementHelp = %v, want %v", gopts.Spanner.StatementHelp, tt.wantStatementHelp)
			}
			if gopts.Spanner.TryPartitionQuery != tt.wantTryPartition {
				t.Errorf("TryPartitionQuery = %v, want %v", gopts.Spanner.TryPartitionQuery, tt.wantTryPartition)
			}
			if tt.wantEmulatorImage != "" && gopts.Spanner.EmulatorImage != tt.wantEmulatorImage {
				t.Errorf("EmulatorImage = %q, want %q", gopts.Spanner.EmulatorImage, tt.wantEmulatorImage)
			}

			// If async flag is set, check it's propagated to system variables
			if tt.wantAsync && err == nil {
				sysVars, err := initializeSystemVariables(&gopts.Spanner)
				if err != nil {
					t.Fatalf("Failed to initialize system variables: %v", err)
				}
				if !sysVars.AsyncDDL {
					t.Errorf("AsyncDDL not set in system variables when --async flag is used")
				}
			}
		})
	}
}

// TestTimeoutAsyncInteraction tests the interaction between --timeout and --async flags
func TestTimeoutAsyncInteraction(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantTimeout *time.Duration
		wantAsync   bool
		wantErr     bool
		errContains string
	}{
		{
			name:        "timeout without async",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "30s"},
			wantTimeout: lo.ToPtr(30 * time.Second),
			wantAsync:   false,
		},
		{
			name:        "async without timeout uses default",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--async"},
			wantTimeout: lo.ToPtr(10 * time.Minute), // default
			wantAsync:   true,
		},
		{
			name:        "both timeout and async",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "5m", "--async"},
			wantTimeout: lo.ToPtr(5 * time.Minute),
			wantAsync:   true,
		},
		{
			name:        "invalid timeout format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "invalid"},
			wantErr:     true,
			errContains: "invalid value of --timeout",
		},
		{
			name:        "zero timeout",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "0s"},
			wantTimeout: lo.ToPtr(0 * time.Second),
			wantAsync:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			if err == nil {
				err = ValidateSpannerOptions(&gopts.Spanner)
			}

			if err == nil {
				sysVars, initErr := initializeSystemVariables(&gopts.Spanner)
				if initErr != nil {
					err = initErr
				}
				if err == nil {
					// Check timeout
					if tt.wantTimeout != nil {
						if sysVars.StatementTimeout == nil {
							t.Errorf("StatementTimeout = nil, want %v", *tt.wantTimeout)
						} else if *sysVars.StatementTimeout != *tt.wantTimeout {
							t.Errorf("StatementTimeout = %v, want %v", *sysVars.StatementTimeout, *tt.wantTimeout)
						}
					}

					// Check async
					if sysVars.AsyncDDL != tt.wantAsync {
						t.Errorf("AsyncDDL = %v, want %v", sysVars.AsyncDDL, tt.wantAsync)
					}
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestOutputTemplateValidation tests output template flag validation
func TestOutputTemplateValidation(t *testing.T) {
	// Create test template files
	tmpDir := t.TempDir()
	validTemplate := filepath.Join(tmpDir, "valid.tmpl")
	if err := os.WriteFile(validTemplate, []byte("{{.Result}}"), 0o644); err != nil {
		t.Fatalf("Failed to create valid template file: %v", err)
	}

	invalidTemplate := filepath.Join(tmpDir, "invalid.tmpl")
	if err := os.WriteFile(invalidTemplate, []byte("{{.Result"), 0o644); err != nil { // Missing closing }}
		t.Fatalf("Failed to create invalid template file: %v", err)
	}

	tests := []struct {
		name        string
		args        []string
		setVars     map[string]string
		wantErr     bool
		errContains string
		checkFile   bool
		wantFile    string
	}{
		{
			name:      "valid output template file",
			args:      []string{"--project", "p", "--instance", "i", "--database", "d", "--output-template", validTemplate},
			wantErr:   false,
			checkFile: true,
			wantFile:  validTemplate,
		},
		{
			name:        "non-existent output template file",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--output-template", "/non/existent/template.tmpl"},
			wantErr:     true,
			errContains: "parse error of output template",
		},
		{
			name:        "invalid template syntax",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--output-template", invalidTemplate},
			wantErr:     true,
			errContains: "parse error of output template",
		},
		{
			name:      "output template via --set",
			args:      []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "CLI_OUTPUT_TEMPLATE_FILE=" + validTemplate},
			wantErr:   false,
			checkFile: true,
			wantFile:  validTemplate,
		},
		{
			name:      "clear output template with NULL",
			args:      []string{"--project", "p", "--instance", "i", "--database", "d", "--output-template", validTemplate, "--set", "CLI_OUTPUT_TEMPLATE_FILE=NULL"},
			wantErr:   false,
			checkFile: true,
			wantFile:  "", // Should be cleared
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			if err == nil {
				err = ValidateSpannerOptions(&gopts.Spanner)
			}

			if err == nil {
				sysVars, initErr := initializeSystemVariables(&gopts.Spanner)
				if initErr != nil {
					err = initErr
				} else if tt.checkFile {
					if sysVars.OutputTemplateFile != tt.wantFile {
						t.Errorf("OutputTemplateFile = %q, want %q", sysVars.OutputTemplateFile, tt.wantFile)
					}
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestDetermineInitialDatabase tests the determineInitialDatabase function
func TestDetermineInitialDatabase(t *testing.T) {
	tests := []struct {
		name         string
		opts         *spannerOptions
		envDatabase  string
		wantDatabase string
	}{
		{
			name:         "detached flag overrides everything",
			opts:         &spannerOptions{Detached: true, DatabaseId: "db-from-flag"},
			envDatabase:  "db-from-env",
			wantDatabase: "",
		},
		{
			name:         "explicit database flag takes precedence",
			opts:         &spannerOptions{DatabaseId: "db-from-flag"},
			envDatabase:  "db-from-env",
			wantDatabase: "db-from-flag",
		},
		{
			name:         "environment variable used when no flag",
			opts:         &spannerOptions{},
			envDatabase:  "db-from-env",
			wantDatabase: "db-from-env",
		},
		{
			name:         "default to empty (detached) when nothing specified",
			opts:         &spannerOptions{},
			envDatabase:  "",
			wantDatabase: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variable. t.Setenv automatically handles cleanup.
			if tt.envDatabase != "" {
				t.Setenv("SPANNER_DATABASE_ID", tt.envDatabase)
			} else {
				t.Setenv("SPANNER_DATABASE_ID", "")
			}

			got := determineInitialDatabase(tt.opts)
			if got != tt.wantDatabase {
				t.Errorf("determineInitialDatabase() = %q, want %q", got, tt.wantDatabase)
			}
		})
	}
}

// TestParsePriority tests the parsePriority function
func TestParsePriority(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		want     sppb.RequestOptions_Priority
		wantErr  bool
	}{
		{
			name:     "empty string returns unspecified",
			priority: "",
			want:     sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			wantErr:  false,
		},
		{
			name:     "HIGH",
			priority: "HIGH",
			want:     sppb.RequestOptions_PRIORITY_HIGH,
			wantErr:  false,
		},
		{
			name:     "MEDIUM",
			priority: "MEDIUM",
			want:     sppb.RequestOptions_PRIORITY_MEDIUM,
			wantErr:  false,
		},
		{
			name:     "LOW",
			priority: "LOW",
			want:     sppb.RequestOptions_PRIORITY_LOW,
			wantErr:  false,
		},
		{
			name:     "lowercase high",
			priority: "high",
			want:     sppb.RequestOptions_PRIORITY_HIGH,
			wantErr:  false,
		},
		{
			name:     "with PRIORITY_ prefix",
			priority: "PRIORITY_HIGH",
			want:     sppb.RequestOptions_PRIORITY_HIGH,
			wantErr:  false,
		},
		{
			name:     "invalid priority",
			priority: "INVALID",
			want:     sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePriority(tt.priority)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePriority() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parsePriority() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseDirectedReadOptionMain tests the parseDirectedReadOption function
func TestParseDirectedReadOptionMain(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *sppb.DirectedReadOptions
		wantErr bool
	}{
		{
			name:  "location only",
			input: "us-east1",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-east1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_TYPE_UNSPECIFIED,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name:  "location with READ_ONLY",
			input: "us-east1:READ_ONLY",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-east1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name:  "location with READ_WRITE",
			input: "us-east1:READ_WRITE",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-east1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name:  "lowercase replica type",
			input: "us-east1:read_only",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-east1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "too many colons",
			input:   "us-east1:READ_ONLY:extra",
			wantErr: true,
		},
		{
			name:    "invalid replica type",
			input:   "us-east1:INVALID",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDirectedReadOption(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDirectedReadOption() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(sppb.DirectedReadOptions{},
					sppb.DirectedReadOptions_IncludeReplicas_{},
					sppb.DirectedReadOptions_IncludeReplicas{},
					sppb.DirectedReadOptions_ReplicaSelection{})); diff != "" {
					t.Errorf("parseDirectedReadOption() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

// TestBatchModeTableFormatLogic tests the complex logic for determining CLI_FORMAT
// TestReadCredentialFile tests the readCredentialFile function
func TestReadCredentialFile(t *testing.T) {
	tests := []struct {
		name        string
		setupFile   func(t *testing.T) string
		wantContent string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid credential file",
			setupFile: func(t *testing.T) string {
				tempDir := t.TempDir()
				credFile := filepath.Join(tempDir, "cred.json")
				content := `{"type": "service_account", "project_id": "test"}`
				if err := os.WriteFile(credFile, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				return credFile
			},
			wantContent: `{"type": "service_account", "project_id": "test"}`,
			wantErr:     false,
		},
		{
			name: "non-existent file",
			setupFile: func(t *testing.T) string {
				return "/non/existent/file.json"
			},
			wantErr:     true,
			errContains: "no such file",
		},
		{
			name: "empty file",
			setupFile: func(t *testing.T) string {
				tempDir := t.TempDir()
				credFile := filepath.Join(tempDir, "cred.json")
				if err := os.WriteFile(credFile, []byte{}, 0o644); err != nil {
					t.Fatal(err)
				}
				return credFile
			},
			wantContent: "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filepath := tt.setupFile(t)

			got, err := readCredentialFile(filepath)
			if (err != nil) != tt.wantErr {
				t.Errorf("readCredentialFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("readCredentialFile() error = %v, does not contain %v", err, tt.errContains)
			}
			if !tt.wantErr && string(got) != tt.wantContent {
				t.Errorf("readCredentialFile() = %v, want %v", string(got), tt.wantContent)
			}
		})
	}
}

func TestBatchModeTableFormatLogic(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		stdinProvider stdinProvider
		wantCLIFormat enums.DisplayMode
	}{
		{
			name:          "interactive mode (terminal) defaults to table",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d"},
			stdinProvider: ptyStdin(),
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name:          "batch mode with --table flag uses table format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--table"},
			stdinProvider: nonPTYStdin(""),
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name:          "batch mode without --table uses table format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1"},
			stdinProvider: nonPTYStdin(""),
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name:          "piped input defaults to table format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d"},
			stdinProvider: nonPTYStdin("SELECT 1;"),
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name:          "piped input with --table uses table format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--table"},
			stdinProvider: nonPTYStdin("SELECT 1;"),
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name:          "--set CLI_FORMAT overrides all defaults",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "CLI_FORMAT=VERTICAL"},
			stdinProvider: ptyStdin(), // Can be any, as --set overrides
			wantCLIFormat: enums.DisplayModeVertical,
		},
		{
			name:          "--set CLI_FORMAT overrides --table flag",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--table", "--set", "CLI_FORMAT=VERTICAL"},
			stdinProvider: nonPTYStdin(""),
			wantCLIFormat: enums.DisplayModeVertical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			if err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Initialize system variables
			sysVars, err := initializeSystemVariables(&gopts.Spanner)
			if err != nil {
				t.Fatalf("Failed to initialize system variables: %v", err)
			}

			// Simulate the logic in run() that determines CLI_FORMAT
			stdin, cleanup, err := tt.stdinProvider()
			if err != nil {
				t.Skipf("Failed to create stdin: %v", err)
			}
			defer cleanup()

			input, _, err := determineInputAndMode(&gopts.Spanner, stdin)
			if err != nil {
				t.Fatalf("Failed to determine input mode: %v", err)
			}

			// Apply the same logic as in run() - check case-insensitively
			hasSet := false
			for k := range gopts.Spanner.Set {
				if strings.EqualFold(k, "CLI_FORMAT") {
					hasSet = true
					break
				}
			}
			if !hasSet {
				// Use the shared function to get format from options
				formatValue := getFormatFromOptions(&gopts.Spanner)
				var expectedFormat enums.DisplayMode
				if formatValue != "" {
					// Parse the format string to enum
					parsed, err := enums.DisplayModeString(formatValue)
					if err != nil {
						t.Fatalf("Invalid format value: %v", formatValue)
					}
					expectedFormat = parsed
				} else {
					// No format flags provided, use the new default
					expectedFormat = enums.DisplayModeTable
				}
				if expectedFormat != tt.wantCLIFormat {
					t.Errorf("Expected CLI_FORMAT = %v, but want %v (args: %v, input: %q)",
						expectedFormat, tt.wantCLIFormat, tt.args, input)
				}
			} else {
				// If CLI_FORMAT was explicitly set, verify it
				if sysVars.CLIFormat != tt.wantCLIFormat {
					t.Errorf("CLIFormat = %v, want %v (explicitly set)", sysVars.CLIFormat, tt.wantCLIFormat)
				}
			}
		})
	}
}

// TestHelpAndVersionFlags tests help and version flag behavior
func TestHelpAndVersionFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantHelp    bool
		wantVersion bool
	}{
		{
			name:     "help flag",
			args:     []string{"--help"},
			wantHelp: true,
		},
		{
			name:     "short help flag",
			args:     []string{"-h"},
			wantHelp: true,
		},
		{
			name:        "version flag",
			args:        []string{"--version"},
			wantVersion: true,
		},
		{
			name:     "help with other flags still shows help",
			args:     []string{"--help", "--project", "p"},
			wantHelp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			if tt.wantHelp {
				if err == nil || !flags.WroteHelp(err) {
					t.Errorf("Expected help to be written, but got err = %v", err)
				}
			} else if tt.wantVersion {
				if err != nil {
					t.Errorf("Unexpected error parsing version flag: %v", err)
				}
				if !gopts.Spanner.Version {
					t.Errorf("Version flag not set")
				}
			}
		})
	}
}

// TestComplexFlagInteractions tests complex interactions between multiple flags
func TestComplexFlagInteractions(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		envVars       map[string]string
		wantProject   string
		wantInstance  string
		wantDatabase  string
		wantRole      string
		wantInsecure  bool
		wantVerbose   bool
		wantReadOnly  bool
		wantPriority  sppb.RequestOptions_Priority
		wantStaleness bool // true if ReadOnlyStaleness is set
		wantErr       bool
		errContains   string
	}{
		{
			name: "embedded emulator with --set overrides",
			args: []string{
				"--embedded-emulator",
				"--set", "READONLY=true",
				"--set", "RPC_PRIORITY=LOW",
			},
			wantProject:  "emulator-project",  // Default set in initializeSystemVariables
			wantInstance: "emulator-instance", // Default set in initializeSystemVariables
			wantDatabase: "emulator-database", // Default set in initializeSystemVariables
			wantInsecure: true,
			wantReadOnly: true,
			wantPriority: sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name: "detached mode with role and directed read",
			args: []string{
				"--project", "p", "--instance", "i",
				"--detached",
				"--role", "my-role",
				"--directed-read", "us-east1:READ_ONLY",
			},
			wantProject:  "p",
			wantInstance: "i",
			wantDatabase: "", // detached
			wantRole:     "my-role",
			wantPriority: defaultPriority, // Should use default priority
		},
		{
			name: "strong read with priority and role",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--strong",
				"--priority", "HIGH",
				"--database-role", "analyst",
			},
			wantProject:   "p",
			wantInstance:  "i",
			wantDatabase:  "d",
			wantRole:      "analyst",
			wantPriority:  sppb.RequestOptions_PRIORITY_HIGH,
			wantStaleness: true,
		},
		{
			name: "environment variables with partial CLI overrides",
			args: []string{
				"--instance", "cli-instance",
				"--priority", "MEDIUM",
			},
			envVars: map[string]string{
				"SPANNER_PROJECT_ID":  "env-project",
				"SPANNER_INSTANCE_ID": "env-instance",
				"SPANNER_DATABASE_ID": "env-database",
			},
			wantProject:  "env-project",
			wantInstance: "cli-instance", // CLI overrides env
			wantDatabase: "env-database",
			wantPriority: sppb.RequestOptions_PRIORITY_MEDIUM,
		},
		{
			name: "conflicting staleness settings",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--strong",
				"--read-timestamp", "2023-01-01T00:00:00Z",
			},
			wantErr:     true,
			errContains: "--strong and --read-timestamp are mutually exclusive",
		},
		{
			name: "MCP mode with embedded emulator",
			args: []string{
				"--embedded-emulator",
				"--mcp",
			},
			wantProject:  "emulator-project",  // Default set in initializeSystemVariables
			wantInstance: "emulator-instance", // Default set in initializeSystemVariables
			wantDatabase: "emulator-database", // Default set in initializeSystemVariables
			wantInsecure: true,
			wantVerbose:  true,            // MCP sets verbose
			wantPriority: defaultPriority, // Should use default priority
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables. t.Setenv automatically handles cleanup.
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Validate and initialize
			if err == nil {
				err = ValidateSpannerOptions(&gopts.Spanner)
				if err == nil {
					sysVars, err := initializeSystemVariables(&gopts.Spanner)
					if err == nil {
						// Check results
						if sysVars.Project != tt.wantProject {
							t.Errorf("Project = %q, want %q", sysVars.Project, tt.wantProject)
						}
						if sysVars.Instance != tt.wantInstance {
							t.Errorf("Instance = %q, want %q", sysVars.Instance, tt.wantInstance)
						}
						if sysVars.Database != tt.wantDatabase {
							t.Errorf("Database = %q, want %q", sysVars.Database, tt.wantDatabase)
						}
						if sysVars.Role != tt.wantRole {
							t.Errorf("Role = %q, want %q", sysVars.Role, tt.wantRole)
						}
						if sysVars.Insecure != tt.wantInsecure {
							t.Errorf("Insecure = %v, want %v", sysVars.Insecure, tt.wantInsecure)
						}
						if sysVars.Verbose != tt.wantVerbose {
							t.Errorf("Verbose = %v, want %v", sysVars.Verbose, tt.wantVerbose)
						}
						if sysVars.ReadOnly != tt.wantReadOnly {
							t.Errorf("ReadOnly = %v, want %v", sysVars.ReadOnly, tt.wantReadOnly)
						}
						if sysVars.RPCPriority != tt.wantPriority {
							t.Errorf("RPCPriority = %v, want %v", sysVars.RPCPriority, tt.wantPriority)
						}
						if tt.wantStaleness && sysVars.ReadOnlyStaleness == nil {
							t.Errorf("ReadOnlyStaleness = nil, want non-nil")
						}
					}
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && bytes.Contains([]byte(s), []byte(substr))
}

// TestAliasFlagPrecedence tests that non-hidden flags take precedence over hidden aliases
func TestAliasFlagPrecedence(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantRole     string
		wantInsecure bool
		wantEndpoint string
	}{
		{
			name:     "role takes precedence over database-role",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--role", "primary-role", "--database-role", "secondary-role"},
			wantRole: "primary-role",
		},
		{
			name:     "database-role used when role not specified",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--database-role", "db-role"},
			wantRole: "db-role",
		},
		{
			name:         "insecure and skip-tls-verify both set",
			args:         []string{"--project", "p", "--instance", "i", "--database", "d", "--insecure", "--skip-tls-verify"},
			wantInsecure: true,
		},
		{
			name:         "only skip-tls-verify set",
			args:         []string{"--project", "p", "--instance", "i", "--database", "d", "--skip-tls-verify"},
			wantInsecure: true,
		},
		{
			name:         "endpoint takes precedence over deployment-endpoint",
			args:         []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "primary-endpoint:443", "--deployment-endpoint", "secondary-endpoint:443"},
			wantEndpoint: "primary-endpoint:443",
		},
		{
			name:         "deployment-endpoint used when endpoint not specified",
			args:         []string{"--project", "p", "--instance", "i", "--database", "d", "--deployment-endpoint", "deployment-endpoint:443"},
			wantEndpoint: "deployment-endpoint:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			if err != nil {
				t.Fatalf("ParseArgs() error: %v", err)
			}

			sysVars, err := initializeSystemVariables(&gopts.Spanner)
			if err != nil {
				t.Fatalf("initializeSystemVariables() error: %v", err)
			}

			if sysVars.Role != tt.wantRole {
				t.Errorf("Role = %q, want %q", sysVars.Role, tt.wantRole)
			}
			if sysVars.Insecure != tt.wantInsecure {
				t.Errorf("Insecure = %v, want %v", sysVars.Insecure, tt.wantInsecure)
			}
			// Check endpoint by constructing from host and port
			var gotEndpoint string
			if sysVars.Host != "" && sysVars.Port != 0 {
				gotEndpoint = fmt.Sprintf("%s:%d", sysVars.Host, sysVars.Port)
			}
			if gotEndpoint != tt.wantEndpoint {
				t.Errorf("Endpoint (constructed from Host:Port) = %q, want %q", gotEndpoint, tt.wantEndpoint)
			}
		})
	}
}

// TestHostPortFlags tests the --host and --port flag behaviors
func TestHostPortFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "only port specified uses localhost",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--port", "9010"},
			wantHost: "localhost",
			wantPort: 9010,
		},
		{
			name:     "only host specified uses port 443",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--host", "example.com"},
			wantHost: "example.com",
			wantPort: 443,
		},
		{
			name:     "both host and port specified",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--host", "example.com", "--port", "9010"},
			wantHost: "example.com",
			wantPort: 9010,
		},
		{
			name:     "endpoint flag parses into host and port",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "spanner.googleapis.com:443"},
			wantHost: "spanner.googleapis.com",
			wantPort: 443,
		},
		{
			name:     "endpoint with IPv6 address",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "[2001:db8::1]:443"},
			wantHost: "2001:db8::1",
			wantPort: 443,
		},
		{
			name:     "endpoint with bare IPv6 address should fail",
			args:     []string{"--project", "p", "--instance", "i", "--database", "d", "--endpoint", "2001:db8::1"},
			wantHost: "",
			wantPort: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			if err != nil {
				t.Fatalf("ParseArgs() error: %v", err)
			}

			sysVars, err := initializeSystemVariables(&gopts.Spanner)
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("initializeSystemVariables() error: %v", err)
				}
				return
			}

			if sysVars.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", sysVars.Host, tt.wantHost)
			}
			if sysVars.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", sysVars.Port, tt.wantPort)
			}
		})
	}
}

// TestExecuteSQLAliasPrecedence tests execute/sql alias precedence
func TestExecuteSQLAliasPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInput string
	}{
		{
			name:      "execute takes precedence over sql",
			args:      []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--sql", "SELECT 2"},
			wantInput: "SELECT 1",
		},
		{
			name:      "sql used when execute not specified",
			args:      []string{"--project", "p", "--instance", "i", "--database", "d", "--sql", "SELECT 2"},
			wantInput: "SELECT 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			if err != nil {
				t.Fatalf("ParseArgs() error: %v", err)
			}

			stdin := strings.NewReader("")
			input, _, err := determineInputAndMode(&gopts.Spanner, stdin)
			if err != nil {
				t.Fatalf("determineInputAndMode() error: %v", err)
			}

			if input != tt.wantInput {
				t.Errorf("input = %q, want %q", input, tt.wantInput)
			}
		})
	}
}

// TestEmulatorPlatformFlag tests the --emulator-platform flag parsing
func TestEmulatorPlatformFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantPlatform string
		wantEmbedded bool
	}{
		{
			name:         "emulator-platform with linux/amd64",
			args:         []string{"--embedded-emulator", "--emulator-platform", "linux/amd64"},
			wantPlatform: "linux/amd64",
			wantEmbedded: true,
		},
		{
			name:         "emulator-platform with linux/arm64",
			args:         []string{"--embedded-emulator", "--emulator-platform", "linux/arm64"},
			wantPlatform: "linux/arm64",
			wantEmbedded: true,
		},
		{
			name:         "emulator-platform with variant",
			args:         []string{"--embedded-emulator", "--emulator-platform", "linux/arm/v7"},
			wantPlatform: "linux/arm/v7",
			wantEmbedded: true,
		},
		{
			name:         "embedded-emulator without platform",
			args:         []string{"--embedded-emulator"},
			wantPlatform: "",
			wantEmbedded: true,
		},
		{
			name:         "emulator-platform without embedded-emulator",
			args:         []string{"--emulator-platform", "linux/amd64"},
			wantPlatform: "linux/amd64",
			wantEmbedded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			if err != nil {
				t.Fatalf("ParseArgs() error: %v", err)
			}

			if gopts.Spanner.EmulatorPlatform != tt.wantPlatform {
				t.Errorf("EmulatorPlatform = %q, want %q", gopts.Spanner.EmulatorPlatform, tt.wantPlatform)
			}
			if gopts.Spanner.EmbeddedEmulator != tt.wantEmbedded {
				t.Errorf("EmbeddedEmulator = %v, want %v", gopts.Spanner.EmbeddedEmulator, tt.wantEmbedded)
			}
		})
	}
}
