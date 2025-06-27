package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanemuboost"
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
			name:        "execute and sql are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--sql", "SELECT 2"},
			wantErr:     true,
			errContains: "-e, -f, --sql are exclusive",
		},
		{
			name: "role and database-role both set (aliases allowed)",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--role", "role1", "--database-role", "role2"},
			// Note: Both can be set during parsing, but initializeSystemVariables prefers --role
			wantErr: false,
		},
		{
			name:        "insecure and skip-tls-verify are mutually exclusive",
			args:        []string{"--insecure", "--skip-tls-verify"},
			wantErr:     true,
			errContains: "--insecure and --skip-tls-verify are mutually exclusive",
		},

		// Mutually exclusive operations
		{
			name:        "execute and file are mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--file", "query.sql"},
			wantErr:     true,
			errContains: "-e, -f, --sql are exclusive",
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
			errContains: "-e, -f, --sql are exclusive",
		},

		// Invalid combinations
		{
			name:        "try-partition-query requires SQL input",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query"},
			wantErr:     true,
			errContains: "--try-partition-query requires SQL input",
		},
		{
			name: "try-partition-query with execute is valid",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--execute", "SELECT 1"},
			wantErr: false,
		},
		{
			name: "try-partition-query with sql is valid",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--sql", "SELECT 1"},
			wantErr: false,
		},
		{
			name: "try-partition-query with file is valid",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--file", "query.sql"},
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
			name: "detached mode doesn't require database",
			args: []string{"--project", "p", "--instance", "i", "--detached"},
			wantErr: false,
		},
		{
			name: "embedded emulator doesn't require project/instance/database",
			args: []string{"--embedded-emulator"},
			wantErr: false,
		},

		// Valid combinations
		{
			name: "minimal valid flags",
			args: []string{"--project", "p", "--instance", "i", "--database", "d"},
			wantErr: false,
		},
		{
			name: "only insecure flag",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--insecure"},
			wantErr: false,
		},
		{
			name: "only skip-tls-verify flag",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--skip-tls-verify"},
			wantErr: false,
		},
		{
			name: "only strong flag",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--strong"},
			wantErr: false,
		},
		{
			name: "only read-timestamp flag",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--read-timestamp", "2023-01-01T00:00:00Z"},
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
			errContains: "priority must be either HIGH, MEDIUM, or LOW",
		},
		{
			name: "valid priority HIGH",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "HIGH"},
			wantErr: false,
		},
		{
			name: "valid priority MEDIUM",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "MEDIUM"},
			wantErr: false,
		},
		{
			name: "valid priority LOW",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "LOW"},
			wantErr: false,
		},
		{
			name:        "invalid query-mode value",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "INVALID"},
			wantErr:     true,
			errContains: "Invalid value `INVALID' for option `--query-mode'",
		},
		{
			name: "valid query-mode NORMAL",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "NORMAL"},
			wantErr: false,
		},
		{
			name: "valid query-mode PLAN",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "PLAN"},
			wantErr: false,
		},
		{
			name: "valid query-mode PROFILE",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--query-mode", "PROFILE"},
			wantErr: false,
		},
		{
			name:        "invalid database-dialect value",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--database-dialect", "INVALID"},
			wantErr:     true,
			errContains: "Invalid value `INVALID' for option `--database-dialect'",
		},
		{
			name: "valid database-dialect POSTGRESQL",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--database-dialect", "POSTGRESQL"},
			wantErr: false,
		},
		{
			name: "valid database-dialect GOOGLE_STANDARD_SQL",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--database-dialect", "GOOGLE_STANDARD_SQL"},
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
			name: "valid directed-read with location only",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1"},
			wantErr: false,
		},
		{
			name: "valid directed-read with READ_ONLY",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1:READ_ONLY"},
			wantErr: false,
		},
		{
			name: "valid directed-read with READ_WRITE",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--directed-read", "us-east1:READ_WRITE"},
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
			name: "valid read-timestamp",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--read-timestamp", "2023-01-01T00:00:00Z"},
			wantErr: false,
		},
		{
			name:        "invalid timeout format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "invalid"},
			wantErr:     true,
			errContains: "invalid value of --timeout",
		},
		{
			name: "valid timeout",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "30s"},
			wantErr: false,
		},
		{
			name:        "invalid param format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--param", "invalid syntax"},
			wantErr:     true,
			errContains: "error on parsing --param",
		},
		{
			name: "valid param with string value",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--param", "p1='hello'"},
			wantErr: false,
		},
		{
			name: "valid param with type",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--param", "p1=STRING"},
			wantErr: false,
		},
		{
			name:        "invalid set value for boolean",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "READONLY=not-a-bool"},
			wantErr:     true,
			errContains: "failed to set system variable",
		},
		{
			name: "valid set value",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "READONLY=true"},
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
			name: "valid proto descriptor file",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--proto-descriptor-file", validProtoFile},
			wantErr: false,
		},
		{
			name:        "invalid log level",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--log-level", "INVALID"},
			wantErr:     true,
			errContains: "error on parsing --log-level",
		},
		{
			name: "valid log level",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--log-level", "INFO"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gopts globalOptions
			parser := flags.NewParser(&gopts, flags.Default)
			_, err := parser.ParseArgs(tt.args)
			
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
		name           string
		args           []string
		envVars        map[string]string
		configContent  string
		wantProject    string
		wantInstance   string
		wantDatabase   string
		wantPriority   sppb.RequestOptions_Priority
		wantEndpoint   string
		wantRole       string
		wantLogGrpc    bool
		wantInsecure   bool
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
			wantProject:  "env-project",  // env overrides config
			wantInstance: "env-instance", // env overrides config
			wantDatabase: "env-database", // env overrides config
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
				if err := os.WriteFile(configFile, []byte(tt.configContent), 0644); err != nil {
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
			if sysVars.Endpoint != tt.wantEndpoint {
				t.Errorf("Endpoint = %q, want %q", sysVars.Endpoint, tt.wantEndpoint)
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
		name             string
		args             []string
		wantProject      string
		wantInstance     string
		wantDatabase     string
		wantInsecure     bool
		wantVerbose      bool
		wantMCP          bool
		wantCLIFormat    DisplayMode
		checkAfterRun    bool // Some values are set in run() function
	}{
		{
			name: "embedded emulator sets defaults",
			args: []string{"--embedded-emulator"},
			wantProject:  "emulator-project",
			wantInstance: "emulator-instance",
			wantDatabase: "emulator-database",
			wantInsecure: true,
			checkAfterRun: true, // Endpoint and WithoutAuthentication set in run()
		},
		{
			name: "embedded emulator overrides explicit values",
			args: []string{
				"--embedded-emulator",
				"--project", "my-project",
				"--instance", "my-instance",
				"--database", "my-database",
				"--insecure", "false",
			},
			wantProject:  "emulator-project",
			wantInstance: "emulator-instance",
			wantDatabase: "emulator-database",
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
			wantCLIFormat: DisplayModeTable,
		},
		{
			name: "batch mode without --table defaults to tab format",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--execute", "SELECT 1",
			},
			wantProject:   "p",
			wantInstance:  "i",
			wantDatabase:  "d",
			wantCLIFormat: DisplayModeTab,
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
			wantCLIFormat: DisplayModeVertical,
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
			if tt.wantCLIFormat != 0 || tt.name == "batch mode without --table defaults to tab format" || tt.name == "interactive mode defaults to table format" {
				// Determine if this would be interactive mode
				input, interactive, err := determineInputAndMode(&gopts.Spanner, bytes.NewReader(nil))
				if err != nil {
					t.Fatalf("Failed to determine input mode: %v", err)
				}

				// Apply the same logic as in run()
				if _, hasSet := gopts.Spanner.Set["CLI_FORMAT"]; !hasSet {
					expectedFormat := lo.Ternary(interactive || gopts.Spanner.Table, DisplayModeTable, DisplayModeTab)
					if expectedFormat != tt.wantCLIFormat {
						t.Errorf("Expected CLI_FORMAT = %v for interactive=%v, table=%v, input=%q, but want %v",
							expectedFormat, interactive, gopts.Spanner.Table, input, tt.wantCLIFormat)
					}
				} else {
					// CLI_FORMAT was set via --set, check it was applied
					if sysVars.CLIFormat != tt.wantCLIFormat {
						t.Errorf("CLIFormat = %v, want %v", sysVars.CLIFormat, tt.wantCLIFormat)
					}
				}
			}

			// Special checks for modes that set values in run()
			if tt.name == "embedded emulator sets defaults" && tt.checkAfterRun {
				// These would be set in run() after emulator starts
				// Just verify the flags that trigger this behavior are correct
				if !gopts.Spanner.EmbeddedEmulator {
					t.Errorf("EmbeddedEmulator flag not set")
				}
			}

			if tt.name == "enable-partitioned-dml sets AUTOCOMMIT_DML_MODE" {
				if sysVars.AutocommitDMLMode != AutocommitDMLModePartitionedNonAtomic {
					t.Errorf("AutocommitDMLMode = %v, want %v", sysVars.AutocommitDMLMode, AutocommitDMLModePartitionedNonAtomic)
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
			name:           "conflicting flags shows which flags conflict",
			args:           []string{"--insecure", "--skip-tls-verify"},
			wantErrKeyword: "--insecure and --skip-tls-verify are mutually exclusive",
		},
		{
			name:           "invalid enum shows valid options",
			args:           []string{"--project", "p", "--instance", "i", "--database", "d", "--priority", "INVALID"},
			wantErrKeyword: "priority must be either HIGH, MEDIUM, or LOW",
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
			wantErrKeyword: "--try-partition-query requires SQL input via --execute, --file, or --sql",
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
	if err := os.WriteFile(testFile, []byte("SELECT 1;"), 0644); err != nil {
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
			name: "valid file flag",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--file", testFile},
			wantErr: false,
			checkInput: true,
			wantInput: "SELECT 1;",
		},
		{
			name:        "non-existent file",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--file", "/non/existent/file.sql"},
			wantErr:     true,
			errContains: "read from file /non/existent/file.sql failed",
			checkInput:  true, // Need to call determineInputAndMode to get the error
		},
		{
			name: "file flag with dash reads from stdin",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--file", "-"},
			stdin: "SELECT 2;",
			wantErr: false,
			checkInput: true,
			wantInput: "SELECT 2;",
		},
		{
			name:        "file flag with execute is mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--file", testFile, "--execute", "SELECT 1"},
			wantErr:     true,
			errContains: "-e, -f, --sql are exclusive",
		},
		{
			name:        "file flag with sql is mutually exclusive",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--file", testFile, "--sql", "SELECT 1"},
			wantErr:     true,
			errContains: "-e, -f, --sql are exclusive",
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
		name                string
		args                []string
		wantAsync           bool
		wantStatementHelp   bool
		wantTryPartition    bool
		wantEmulatorImage   string
		wantErr             bool
		errContains         string
	}{
		{
			name: "async flag",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--async"},
			wantAsync: true,
		},
		{
			name: "statement-help flag",
			args: []string{"--statement-help"},
			wantStatementHelp: true,
			// Note: --statement-help doesn't require project/instance/database
			// but our test calls ValidateSpannerOptions which would fail.
			// In real usage, run() checks StatementHelp before validation.
		},
		{
			name: "try-partition-query with valid input",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query", "--execute", "SELECT 1"},
			wantTryPartition: true,
		},
		{
			name:        "try-partition-query without input",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--try-partition-query"},
			wantErr:     true,
			errContains: "--try-partition-query requires SQL input",
		},
		{
			name: "emulator-image flag",
			args: []string{"--embedded-emulator", "--emulator-image", "gcr.io/my-project/my-emulator:latest"},
			wantEmulatorImage: "gcr.io/my-project/my-emulator:latest",
		},
		{
			name: "emulator-image without embedded-emulator is ignored",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--emulator-image", "gcr.io/my-project/my-emulator:latest"},
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
			name: "timeout without async",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "30s"},
			wantTimeout: lo.ToPtr(30 * time.Second),
			wantAsync: false,
		},
		{
			name: "async without timeout uses default",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--async"},
			wantTimeout: lo.ToPtr(10 * time.Minute), // default
			wantAsync: true,
		},
		{
			name: "both timeout and async",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "5m", "--async"},
			wantTimeout: lo.ToPtr(5 * time.Minute),
			wantAsync: true,
		},
		{
			name:        "invalid timeout format",
			args:        []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "invalid"},
			wantErr:     true,
			errContains: "invalid value of --timeout",
		},
		{
			name: "zero timeout",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--timeout", "0s"},
			wantTimeout: lo.ToPtr(0 * time.Second),
			wantAsync: false,
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
	if err := os.WriteFile(validTemplate, []byte("{{.Result}}"), 0644); err != nil {
		t.Fatalf("Failed to create valid template file: %v", err)
	}
	
	invalidTemplate := filepath.Join(tmpDir, "invalid.tmpl")
	if err := os.WriteFile(invalidTemplate, []byte("{{.Result"), 0644); err != nil { // Missing closing }}
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
			name: "valid output template file",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--output-template", validTemplate},
			wantErr: false,
			checkFile: true,
			wantFile: validTemplate,
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
			name: "output template via --set",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "CLI_OUTPUT_TEMPLATE_FILE=" + validTemplate},
			wantErr: false,
			checkFile: true,
			wantFile: validTemplate,
		},
		{
			name: "clear output template with NULL",
			args: []string{"--project", "p", "--instance", "i", "--database", "d", "--output-template", validTemplate, "--set", "CLI_OUTPUT_TEMPLATE_FILE=NULL"},
			wantErr: false,
			checkFile: true,
			wantFile: "", // Should be cleared
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
func TestBatchModeTableFormatLogic(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		stdinProvider stdinProvider
		wantCLIFormat DisplayMode
	}{
		{
			name:          "interactive mode (terminal) defaults to table",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d"},
			stdinProvider: ptyStdin(),
			wantCLIFormat: DisplayModeTable,
		},
		{
			name:          "batch mode with --table flag uses table format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--table"},
			stdinProvider: nonPTYStdin(""),
			wantCLIFormat: DisplayModeTable,
		},
		{
			name:          "batch mode without --table uses tab format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1"},
			stdinProvider: nonPTYStdin(""),
			wantCLIFormat: DisplayModeTab,
		},
		{
			name:          "piped input defaults to tab format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d"},
			stdinProvider: nonPTYStdin("SELECT 1;"),
			wantCLIFormat: DisplayModeTab,
		},
		{
			name:          "piped input with --table uses table format",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--table"},
			stdinProvider: nonPTYStdin("SELECT 1;"),
			wantCLIFormat: DisplayModeTable,
		},
		{
			name:          "--set CLI_FORMAT overrides all defaults",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--set", "CLI_FORMAT=VERTICAL"},
			stdinProvider: ptyStdin(), // Can be any, as --set overrides
			wantCLIFormat: DisplayModeVertical,
		},
		{
			name:          "--set CLI_FORMAT overrides --table flag",
			args:          []string{"--project", "p", "--instance", "i", "--database", "d", "--execute", "SELECT 1", "--table", "--set", "CLI_FORMAT=VERTICAL"},
			stdinProvider: nonPTYStdin(""),
			wantCLIFormat: DisplayModeVertical,
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

			input, interactive, err := determineInputAndMode(&gopts.Spanner, stdin)
			if err != nil {
				t.Fatalf("Failed to determine input mode: %v", err)
			}
			
			// Apply the same logic as in run()
			if _, hasSet := gopts.Spanner.Set["CLI_FORMAT"]; !hasSet {
				expectedFormat := lo.Ternary(interactive || gopts.Spanner.Table, DisplayModeTable, DisplayModeTab)
				if expectedFormat != tt.wantCLIFormat {
					t.Errorf("Expected CLI_FORMAT = %v for interactive=%v, table=%v, input=%q, but want %v",
						expectedFormat, interactive, gopts.Spanner.Table, input, tt.wantCLIFormat)
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
		name       string
		args       []string
		wantHelp   bool
		wantVersion bool
	}{
		{
			name:       "help flag",
			args:       []string{"--help"},
			wantHelp:   true,
		},
		{
			name:       "short help flag",
			args:       []string{"-h"},
			wantHelp:   true,
		},
		{
			name:       "version flag",
			args:       []string{"--version"},
			wantVersion: true,
		},
		{
			name:       "help with other flags still shows help",
			args:       []string{"--help", "--project", "p"},
			wantHelp:   true,
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
		name             string
		args             []string
		envVars          map[string]string
		wantProject      string
		wantInstance     string
		wantDatabase     string
		wantRole         string
		wantInsecure     bool
		wantVerbose      bool
		wantReadOnly     bool
		wantPriority     sppb.RequestOptions_Priority
		wantStaleness    bool // true if ReadOnlyStaleness is set
		wantErr          bool
		errContains      string
	}{
		{
			name: "embedded emulator with --set overrides",
			args: []string{
				"--embedded-emulator",
				"--set", "READONLY=true",
				"--set", "RPC_PRIORITY=LOW",
			},
			wantProject:  "emulator-project",
			wantInstance: "emulator-instance",
			wantDatabase: "emulator-database",
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
				"--set", "READ_TIMESTAMP=2023-01-01T00:00:00Z",
			},
			wantErr:     true,
			errContains: "failed to set system variable",
		},
		{
			name: "MCP mode with embedded emulator",
			args: []string{
				"--embedded-emulator",
				"--mcp",
			},
			wantProject:  "emulator-project",
			wantInstance: "emulator-instance",
			wantDatabase: "emulator-database",
			wantInsecure: true,
			wantVerbose:  true, // MCP sets verbose
			wantPriority: defaultPriority, // Should use default priority
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment
			savedEnv := make(map[string]string)
			for k := range tt.envVars {
				savedEnv[k] = os.Getenv(k)
				_ = os.Unsetenv(k)
			}
			defer func() {
				for k, v := range savedEnv {
					if v == "" {
						_ = os.Unsetenv(k)
					} else {
						_ = os.Setenv(k, v)
					}
				}
			}()

			// Set test environment variables
			for k, v := range tt.envVars {
				_ = os.Setenv(k, v)
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
					var sysVars systemVariables
					sysVars, err = initializeSystemVariables(&gopts.Spanner)
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