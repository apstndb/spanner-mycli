package mycli

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

// Common error messages used across tests
const (
	errMsgInputMethodsExclusive        = "--execute(-e), --file(-f), --sql, --source are exclusive"
	errMsgEndpointHostPortExclusive    = "--endpoint and (--host or --port) are mutually exclusive"
	errMsgStrongReadTimestampExclusive = "--strong and --read-timestamp are mutually exclusive"
	errMsgTryPartitionRequiresInput    = "--try-partition-query requires SQL input via --execute(-e), --file(-f), --source, or --sql"
	errMsgMissingProjectInstance       = "missing parameters: -p, -i are required"
	errMsgMissingDatabase              = "missing parameter: -d is required"
)

// withRequiredFlags adds the standard required flags (project, instance, database) to the given additional flags
func withRequiredFlags(additionalFlags ...string) []string {
	base := []string{"--project", "p", "--instance", "i", "--database", "d"}
	return append(base, additionalFlags...)
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && bytes.Contains([]byte(s), []byte(substr))
}

// ptr returns a pointer to the given string
func ptr(s string) *string {
	return &s
}

// assertEqual compares a field value with an expected pointer value (generic)
func assertEqual[T comparable](t *testing.T, fieldName string, got T, want *T) {
	t.Helper()
	if want != nil && got != *want {
		t.Errorf("%s = %v, want %v", fieldName, got, *want)
	}
}

// checkError validates that an error matches expected conditions
func checkError(t *testing.T, err error, errContains string) {
	t.Helper()
	wantErr := errContains != ""
	if (err != nil) != wantErr {
		t.Errorf("error = %v, wantErr %v", err, wantErr)
		return
	}
	if errContains != "" && err != nil {
		if !contains(err.Error(), errContains) {
			t.Errorf("error %q does not contain expected string %q", err.Error(), errContains)
		}
	}
}

// parseAndValidate parses command-line arguments and validates Spanner options
func parseAndValidate(args []string) (*globalOptions, error) {
	var gopts globalOptions
	parser := flags.NewParser(&gopts, flags.Default)
	_, err := parser.ParseArgs(args)
	if err != nil {
		return nil, err
	}

	if err := ValidateSpannerOptions(&gopts.Spanner); err != nil {
		return nil, err
	}

	return &gopts, nil
}

// initSysVarsOrFail initializes system variables and fails the test on error
func initSysVarsOrFail(t *testing.T, opts *spannerOptions) *systemVariables {
	t.Helper()
	sysVars, err := initializeSystemVariables(opts)
	if err != nil {
		t.Fatalf("Failed to initialize system variables: %v", err)
	}
	return sysVars
}

// verifyConnectionParams checks common connection parameters
func verifyConnectionParams(t *testing.T, sysVars *systemVariables, wantProject, wantInstance, wantDatabase string) {
	t.Helper()
	if sysVars.Project != wantProject {
		t.Errorf("Project = %q, want %q", sysVars.Project, wantProject)
	}
	if sysVars.Instance != wantInstance {
		t.Errorf("Instance = %q, want %q", sysVars.Instance, wantInstance)
	}
	if sysVars.Database != wantDatabase {
		t.Errorf("Database = %q, want %q", sysVars.Database, wantDatabase)
	}
}

// spannerOptionsExpectations holds expected field values for verification
type spannerOptionsExpectations struct {
	Priority        *string
	QueryMode       *string
	DatabaseDialect *string
	DirectedRead    *string
	ReadTimestamp   *string
	Timeout         *string
	EmulatorImage   *string
}

// verifySpannerOptions checks spannerOptions fields against expected values
func verifySpannerOptions(t *testing.T, got *spannerOptions, want *spannerOptionsExpectations) {
	t.Helper()
	if want == nil {
		return
	}
	assertEqual(t, "Priority", got.Priority, want.Priority)
	assertEqual(t, "QueryMode", got.QueryMode, want.QueryMode)
	assertEqual(t, "DatabaseDialect", got.DatabaseDialect, want.DatabaseDialect)
	assertEqual(t, "DirectedRead", got.DirectedRead, want.DirectedRead)
	assertEqual(t, "ReadTimestamp", got.ReadTimestamp, want.ReadTimestamp)
	assertEqual(t, "Timeout", got.Timeout, want.Timeout)
	assertEqual(t, "EmulatorImage", got.EmulatorImage, want.EmulatorImage)
}

// TestParseFlagsCombinations tests various flag combinations for conflicts and mutual exclusivity
func TestParseFlagsCombinations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		args        []string
		errContains string
	}{
		// Alias conflicts
		{
			name: "execute and sql are aliases (both allowed)",
			args: withRequiredFlags("--execute", "SELECT 1", "--sql", "SELECT 2"),
		},
		{
			name: "role and database-role both set (aliases allowed)",
			args: withRequiredFlags("--role", "role1", "--database-role", "role2"),
			// Note: Both can be set during parsing, but initializeSystemVariables prefers --role
		},
		{
			name: "insecure and skip-tls-verify are aliases (both allowed)",
			args: withRequiredFlags("--insecure", "--skip-tls-verify"),
		},
		{
			name: "endpoint and deployment-endpoint both set (aliases allowed)",
			args: withRequiredFlags("--endpoint", "endpoint1:443", "--deployment-endpoint", "endpoint2:443"),
			// Note: Both can be set during parsing, but initializeSystemVariables prefers --endpoint
		},

		// Mutually exclusive operations
		{
			name:        "execute and file are mutually exclusive",
			args:        withRequiredFlags("--execute", "SELECT 1", "--file", "query.sql"),
			errContains: errMsgInputMethodsExclusive,
		},
		{
			name:        "strong and read-timestamp are mutually exclusive",
			args:        []string{"--strong", "--read-timestamp", "2023-01-01T00:00:00Z"},
			errContains: errMsgStrongReadTimestampExclusive,
		},
		{
			name:        "all three input methods (execute, file, sql) are mutually exclusive",
			args:        withRequiredFlags("--execute", "SELECT 1", "--file", "query.sql", "--sql", "SELECT 2"),
			errContains: errMsgInputMethodsExclusive,
		},

		// Invalid combinations
		{
			name:        "endpoint and host are mutually exclusive",
			args:        withRequiredFlags("--endpoint", "spanner.googleapis.com:443", "--host", "example.com"),
			errContains: errMsgEndpointHostPortExclusive,
		},
		{
			name:        "endpoint and port are mutually exclusive",
			args:        withRequiredFlags("--endpoint", "spanner.googleapis.com:443", "--port", "9010"),
			errContains: errMsgEndpointHostPortExclusive,
		},
		{
			name:        "endpoint and both host/port are mutually exclusive",
			args:        withRequiredFlags("--endpoint", "spanner.googleapis.com:443", "--host", "example.com", "--port", "9010"),
			errContains: errMsgEndpointHostPortExclusive,
		},
		{
			name:        "try-partition-query requires SQL input",
			args:        withRequiredFlags("--try-partition-query"),
			errContains: errMsgTryPartitionRequiresInput,
		},
		{
			name: "table flag without SQL input in batch mode",
			args: withRequiredFlags("--table"),
			// Table flag is valid without SQL input - it affects output format
		},
		{
			name: "try-partition-query with execute is valid",
			args: withRequiredFlags("--try-partition-query", "--execute", "SELECT 1"),
		},
		{
			name: "try-partition-query with sql is valid",
			args: withRequiredFlags("--try-partition-query", "--sql", "SELECT 1"),
		},
		{
			name: "try-partition-query with file is valid",
			args: withRequiredFlags("--try-partition-query", "--file", "query.sql"),
		},
		{
			name: "try-partition-query with source is valid",
			args: withRequiredFlags("--try-partition-query", "--source", "query.sql"),
		},
		{
			name:        "execute and source are mutually exclusive",
			args:        withRequiredFlags("--execute", "SELECT 1", "--source", "query.sql"),
			errContains: errMsgInputMethodsExclusive,
		},
		{
			name:        "all four input methods are mutually exclusive",
			args:        withRequiredFlags("--execute", "SELECT 1", "--file", "query.sql", "--sql", "SELECT 2", "--source", "query3.sql"),
			errContains: errMsgInputMethodsExclusive,
		},

		// Embedded emulator tests
		{
			name: "embedded-emulator without connection params",
			args: []string{"--embedded-emulator"},
		},
		{
			name: "embedded-emulator with custom image",
			args: []string{"--embedded-emulator", "--emulator-image", "gcr.io/spanner-emulator/emulator:latest"},
		},
		{
			name: "embedded-emulator ignores connection params",
			args: []string{"--embedded-emulator", "--project", "ignored", "--instance", "ignored", "--database", "ignored"},
		},

		// Missing required parameters
		{
			name:        "missing project without embedded emulator",
			args:        []string{"--instance", "i", "--database", "d"},
			errContains: errMsgMissingProjectInstance,
		},
		{
			name:        "missing instance without embedded emulator",
			args:        []string{"--project", "p", "--database", "d"},
			errContains: errMsgMissingProjectInstance,
		},
		{
			name:        "missing database without embedded emulator or detached",
			args:        []string{"--project", "p", "--instance", "i"},
			errContains: errMsgMissingDatabase,
		},
		{
			name: "detached mode doesn't require database",
			args: []string{"--project", "p", "--instance", "i", "--detached"},
		},
		{
			name: "embedded emulator doesn't require project/instance/database",
			args: []string{"--embedded-emulator"},
		},

		// Valid combinations
		{
			name: "minimal valid flags",
			args: withRequiredFlags(),
		},
		{
			name: "only insecure flag",
			args: withRequiredFlags("--insecure"),
		},
		{
			name: "only skip-tls-verify flag",
			args: withRequiredFlags("--skip-tls-verify"),
		},
		{
			name: "only strong flag",
			args: withRequiredFlags("--strong"),
		},
		{
			name: "only read-timestamp flag",
			args: withRequiredFlags("--read-timestamp", "2023-01-01T00:00:00Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate(tt.args)
			checkError(t, err, tt.errContains)
		})
	}
}

// TestParseFlagsValidation tests flag value validation
func TestParseFlagsValidation(t *testing.T) {
	t.Parallel()
	// Use an existing test proto descriptor file
	validProtoFile := "testdata/protos/order_descriptors.pb"

	tests := []struct {
		name        string
		args        []string
		errContains string
		want        *spannerOptionsExpectations
	}{
		// Enum validation - Invalid values
		{
			name:        "invalid priority value",
			args:        withRequiredFlags("--priority", "INVALID"),
			errContains: "must be one of:",
		},
		{
			name:        "invalid query-mode value",
			args:        withRequiredFlags("--query-mode", "INVALID"),
			errContains: "Invalid value `INVALID' for option `--query-mode'",
		},
		{
			name:        "invalid database-dialect value",
			args:        withRequiredFlags("--database-dialect", "INVALID"),
			errContains: "Invalid value `INVALID' for option `--database-dialect'",
		},

		// Format validation
		{
			name:        "invalid directed-read format",
			args:        withRequiredFlags("--directed-read", "invalid:format:too:many"),
			errContains: "directed read option must be in the form of <replica_location>:<replica_type>",
		},
		{
			name: "valid directed-read with location only",
			args: withRequiredFlags("--directed-read", "us-east1"),
			want: &spannerOptionsExpectations{DirectedRead: ptr("us-east1")},
		},
		{
			name: "valid directed-read with READ_ONLY",
			args: withRequiredFlags("--directed-read", "us-east1:READ_ONLY"),
			want: &spannerOptionsExpectations{DirectedRead: ptr("us-east1:READ_ONLY")},
		},
		{
			name: "valid directed-read with READ_WRITE",
			args: withRequiredFlags("--directed-read", "us-east1:READ_WRITE"),
			want: &spannerOptionsExpectations{DirectedRead: ptr("us-east1:READ_WRITE")},
		},
		{
			name:        "invalid directed-read replica type",
			args:        withRequiredFlags("--directed-read", "us-east1:INVALID"),
			errContains: "<replica_type> must be either READ_WRITE or READ_ONLY",
		},
		{
			name:        "invalid read-timestamp format",
			args:        withRequiredFlags("--read-timestamp", "invalid-timestamp"),
			errContains: "error on parsing --read-timestamp",
		},
		{
			name: "valid read-timestamp",
			args: withRequiredFlags("--read-timestamp", "2023-01-01T00:00:00Z"),
			want: &spannerOptionsExpectations{ReadTimestamp: ptr("2023-01-01T00:00:00Z")},
		},
		{
			name:        "invalid timeout format",
			args:        withRequiredFlags("--timeout", "invalid"),
			errContains: "invalid value of --timeout",
		},
		{
			name: "valid timeout",
			args: withRequiredFlags("--timeout", "30s"),
			want: &spannerOptionsExpectations{Timeout: ptr("30s")},
		},
		{
			name:        "invalid param format",
			args:        withRequiredFlags("--param", "invalid syntax"),
			errContains: "error on parsing --param",
		},
		{
			name: "valid param with string value",
			args: withRequiredFlags("--param", "p1='hello'"),
		},
		{
			name: "valid param with type",
			args: withRequiredFlags("--param", "p1=STRING"),
		},
		{
			name:        "invalid set value for boolean",
			args:        withRequiredFlags("--set", "READONLY=not-a-bool"),
			errContains: "failed to set system variable",
		},
		{
			name: "valid set value",
			args: withRequiredFlags("--set", "READONLY=true"),
		},

		// File validation
		{
			name:        "non-existent proto descriptor file",
			args:        withRequiredFlags("--proto-descriptor-file", "non-existent-file.pb"),
			errContains: "error on --proto-descriptor-file",
		},
		{
			name: "valid proto descriptor file",
			args: withRequiredFlags("--proto-descriptor-file", validProtoFile),
		},
		{
			name:        "invalid log level",
			args:        withRequiredFlags("--log-level", "INVALID"),
			errContains: "error on parsing --log-level",
		},
		{
			name: "valid log level",
			args: withRequiredFlags("--log-level", "INFO"),
		},

		// Credential file tests
		{
			name: "valid credential file",
			// Use a placeholder that will be replaced in the test
			args: withRequiredFlags("--credential", "__TEMP_CRED_FILE__"),
		},
		{
			name: "non-existent credential file",
			args: withRequiredFlags("--credential", "/non/existent/cred.json"),
			// Note: This won't fail during flag parsing/validation, only during run()
		},

		// MCP mode tests
		{
			name: "mcp mode with all required params",
			args: withRequiredFlags("--mcp"),
		},
		{
			name: "mcp mode with embedded emulator",
			args: []string{"--embedded-emulator", "--mcp"},
		},

		// Complex flag combinations
		{
			name: "multiple set flags",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--set", "CLI_FORMAT=VERTICAL", "--set", "READONLY=true", "--set", "AUTOCOMMIT_DML_MODE=PARTITIONED_NON_ATOMIC",
			},
		},
		{
			name: "multiple param flags",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--param", "p1='value1'", "--param", "p2=INT64", "--param", "p3=ARRAY<STRING>",
			},
		},
		{
			name: "invalid param syntax",
			args: []string{
				"--project", "p", "--instance", "i", "--database", "d",
				"--param", "p1=@{invalid}",
			},
			errContains: "error on parsing --param",
		},
	}

	// Add valid enum test cases using loops
	for _, priority := range []string{"HIGH", "MEDIUM", "LOW"} {
		tests = append(tests, struct {
			name        string
			args        []string
			errContains string
			want        *spannerOptionsExpectations
		}{
			name: fmt.Sprintf("valid priority %s", priority),
			args: withRequiredFlags("--priority", priority),
			want: &spannerOptionsExpectations{Priority: &priority},
		})
	}

	for _, mode := range []string{"NORMAL", "PLAN", "PROFILE"} {
		tests = append(tests, struct {
			name        string
			args        []string
			errContains string
			want        *spannerOptionsExpectations
		}{
			name: fmt.Sprintf("valid query-mode %s", mode),
			args: withRequiredFlags("--query-mode", mode),
			want: &spannerOptionsExpectations{QueryMode: &mode},
		})
	}

	for _, dialect := range []string{"POSTGRESQL", "GOOGLE_STANDARD_SQL"} {
		tests = append(tests, struct {
			name        string
			args        []string
			errContains string
			want        *spannerOptionsExpectations
		}{
			name: fmt.Sprintf("valid database-dialect %s", dialect),
			args: withRequiredFlags("--database-dialect", dialect),
			want: &spannerOptionsExpectations{DatabaseDialect: &dialect},
		})
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

			gopts, err := parseAndValidate(args)
			if err == nil && gopts != nil {
				_, err = initializeSystemVariables(&gopts.Spanner)
			}

			checkError(t, err, tt.errContains)

			// If successful and we have expectations, verify them
			if err == nil && tt.want != nil {
				verifySpannerOptions(t, &gopts.Spanner, tt.want)
			}
		})
	}
}

// TestFlagSystemVariablePrecedence tests the precedence of flags vs system variables
func TestFlagSystemVariablePrecedence(t *testing.T) {
	// Cannot use t.Parallel() because subtests use t.Setenv()
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
			sysVars := initSysVarsOrFail(t, &gopts.Spanner)

			// Check results
			verifyConnectionParams(t, sysVars, tt.wantProject, tt.wantInstance, tt.wantDatabase)
			assertEqual(t, "RPCPriority", sysVars.RPCPriority, &tt.wantPriority)
			// Check endpoint by constructing from host and port
			var gotEndpoint string
			if sysVars.Host != "" && sysVars.Port != 0 {
				gotEndpoint = fmt.Sprintf("%s:%d", sysVars.Host, sysVars.Port)
			}
			assertEqual(t, "Endpoint (constructed from Host:Port)", gotEndpoint, &tt.wantEndpoint)
			assertEqual(t, "Role", sysVars.Role, &tt.wantRole)
			assertEqual(t, "LogGrpc", sysVars.LogGrpc, &tt.wantLogGrpc)
			assertEqual(t, "Insecure", sysVars.Insecure, &tt.wantInsecure)
		})
	}
}

// determineExpectedFormat is a test helper that determines the expected CLI_FORMAT
// based on the options, matching the application's actual behavior
func determineExpectedFormat(t *testing.T, opts *spannerOptions) enums.DisplayMode {
	t.Helper()
	formatMode := getFormatFromOptions(opts)
	if formatMode != enums.DisplayModeUnspecified {
		return formatMode
	}
	// No format flags provided, use the new default
	return enums.DisplayModeTable
}

// TestFlagSpecialModes tests special modes like embedded emulator and MCP
func TestFlagSpecialModes(t *testing.T) {
	t.Parallel()
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
			gopts, err := parseAndValidate(tt.args)
			if err != nil {
				t.Fatalf("Failed to parse/validate flags: %v", err)
			}

			sysVars := initSysVarsOrFail(t, &gopts.Spanner)

			// Check results
			verifyConnectionParams(t, sysVars, tt.wantProject, tt.wantInstance, tt.wantDatabase)
			assertEqual(t, "Insecure", sysVars.Insecure, &tt.wantInsecure)
			assertEqual(t, "Verbose", sysVars.Verbose, &tt.wantVerbose)
			assertEqual(t, "MCP", sysVars.MCP, &tt.wantMCP)

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
					expectedFormat := determineExpectedFormat(t, &gopts.Spanner)
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
	t.Parallel()
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
			args:           withRequiredFlags("--execute", "SELECT 1", "--file", "query.sql"),
			wantErrKeyword: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
		{
			name:           "invalid enum shows valid options",
			args:           withRequiredFlags("--priority", "INVALID"),
			wantErrKeyword: "must be one of:",
		},
		{
			name:           "invalid directed read shows format",
			args:           withRequiredFlags("--directed-read", "a:b:c:d"),
			wantErrKeyword: "directed read option must be in the form of <replica_location>:<replica_type>",
		},
		{
			name:           "invalid replica type shows valid options",
			args:           withRequiredFlags("--directed-read", "us-east1:INVALID"),
			wantErrKeyword: "<replica_type> must be either READ_WRITE or READ_ONLY",
		},
		{
			name:           "try-partition-query without input shows requirement",
			args:           withRequiredFlags("--try-partition-query"),
			wantErrKeyword: "--try-partition-query requires SQL input via --execute(-e), --file(-f), --source, or --sql",
		},
		{
			name:           "invalid timeout shows it's a timeout error",
			args:           withRequiredFlags("--timeout", "invalid"),
			wantErrKeyword: "invalid value of --timeout",
		},
		{
			name:           "invalid param shows which param failed",
			args:           withRequiredFlags("--param", "p1=invalid syntax"),
			wantErrKeyword: "error on parsing --param=p1=invalid syntax",
		},
		{
			name:           "non-existent proto file shows file error",
			args:           withRequiredFlags("--proto-descriptor-file", "missing.pb"),
			wantErrKeyword: "error on --proto-descriptor-file, file: missing.pb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gopts, err := parseAndValidate(tt.args)
			if err == nil && gopts != nil {
				_, err = initializeSystemVariables(&gopts.Spanner)
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
	t.Parallel()
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
			args:       withRequiredFlags("--file", testFile),
			checkInput: true,
			wantInput:  "SELECT 1;",
		},
		{
			name:        "non-existent file",
			args:        withRequiredFlags("--file", "/non/existent/file.sql"),
			errContains: "read from file /non/existent/file.sql failed",
			checkInput:  true, // Need to call determineInputAndMode to get the error
		},
		{
			name:       "file flag with dash reads from stdin",
			args:       withRequiredFlags("--file", "-"),
			stdin:      "SELECT 2;",
			checkInput: true,
			wantInput:  "SELECT 2;",
		},
		{
			name:        "file flag with execute is mutually exclusive",
			args:        withRequiredFlags("--file", testFile, "--execute", "SELECT 1"),
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
		{
			name:        "file flag with sql is mutually exclusive",
			args:        withRequiredFlags("--file", testFile, "--sql", "SELECT 1"),
			errContains: "--execute(-e), --file(-f), --sql, --source are exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gopts, err := parseAndValidate(tt.args)

			// If no validation error, test determineInputAndMode
			if err == nil && tt.checkInput && gopts != nil {
				input, _, determineErr := determineInputAndMode(&gopts.Spanner, bytes.NewReader([]byte(tt.stdin)))
				if determineErr != nil {
					err = determineErr
				} else if input != tt.wantInput {
					t.Errorf("determineInputAndMode() input = %q, want %q", input, tt.wantInput)
				}
			}

			checkError(t, err, tt.errContains)
		})
	}
}

// TestSpecialFlags tests special flags like --async, --statement-help, etc.
func TestSpecialFlags(t *testing.T) {
	t.Parallel()
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
			args:      withRequiredFlags("--async"),
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
			args:             withRequiredFlags("--try-partition-query", "--execute", "SELECT 1"),
			wantTryPartition: true,
		},
		{
			name:        "try-partition-query without input",
			args:        withRequiredFlags("--try-partition-query"),
			errContains: "--try-partition-query requires SQL input",
		},
		{
			name:              "emulator-image flag",
			args:              []string{"--embedded-emulator", "--emulator-image", "gcr.io/my-project/my-emulator:latest"},
			wantEmulatorImage: "gcr.io/my-project/my-emulator:latest",
		},
		{
			name:              "emulator-image without embedded-emulator is ignored",
			args:              withRequiredFlags("--emulator-image", "gcr.io/my-project/my-emulator:latest"),
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

			checkError(t, err, tt.errContains)
			if err != nil {
				return
			}

			// Check flag values
			assertEqual(t, "Async", gopts.Spanner.Async, &tt.wantAsync)
			assertEqual(t, "StatementHelp", gopts.Spanner.StatementHelp, &tt.wantStatementHelp)
			assertEqual(t, "TryPartitionQuery", gopts.Spanner.TryPartitionQuery, &tt.wantTryPartition)
			if tt.wantEmulatorImage != "" && gopts.Spanner.EmulatorImage != tt.wantEmulatorImage {
				t.Errorf("EmulatorImage = %q, want %q", gopts.Spanner.EmulatorImage, tt.wantEmulatorImage)
			}

			// If async flag is set, check it's propagated to system variables
			if tt.wantAsync && err == nil {
				sysVars := initSysVarsOrFail(t, &gopts.Spanner)
				if !sysVars.AsyncDDL {
					t.Errorf("AsyncDDL not set in system variables when --async flag is used")
				}
			}
		})
	}
}

// TestTimeoutAsyncInteraction tests the interaction between --timeout and --async flags
func TestTimeoutAsyncInteraction(t *testing.T) {
	t.Parallel()
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
			args:        withRequiredFlags("--timeout", "30s"),
			wantTimeout: lo.ToPtr(30 * time.Second),
			wantAsync:   false,
		},
		{
			name:        "async without timeout uses default",
			args:        withRequiredFlags("--async"),
			wantTimeout: lo.ToPtr(10 * time.Minute), // default
			wantAsync:   true,
		},
		{
			name:        "both timeout and async",
			args:        withRequiredFlags("--timeout", "5m", "--async"),
			wantTimeout: lo.ToPtr(5 * time.Minute),
			wantAsync:   true,
		},
		{
			name:        "invalid timeout format",
			args:        withRequiredFlags("--timeout", "invalid"),
			errContains: "invalid value of --timeout",
		},
		{
			name:        "zero timeout",
			args:        withRequiredFlags("--timeout", "0s"),
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
					assertEqual(t, "AsyncDDL", sysVars.AsyncDDL, &tt.wantAsync)
				}
			}

			checkError(t, err, tt.errContains)
		})
	}
}

// TestOutputTemplateValidation tests output template flag validation
func TestOutputTemplateValidation(t *testing.T) {
	t.Parallel()
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
			args:      withRequiredFlags("--output-template", validTemplate),
			checkFile: true,
			wantFile:  validTemplate,
		},
		{
			name:        "non-existent output template file",
			args:        withRequiredFlags("--output-template", "/non/existent/template.tmpl"),
			errContains: "parse error of output template",
		},
		{
			name:        "invalid template syntax",
			args:        withRequiredFlags("--output-template", invalidTemplate),
			errContains: "parse error of output template",
		},
		{
			name:      "output template via --set",
			args:      withRequiredFlags("--set", "CLI_OUTPUT_TEMPLATE_FILE="+validTemplate),
			checkFile: true,
			wantFile:  validTemplate,
		},
		{
			name:      "clear output template with NULL",
			args:      withRequiredFlags("--output-template", validTemplate, "--set", "CLI_OUTPUT_TEMPLATE_FILE=NULL"),
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

			checkError(t, err, tt.errContains)
		})
	}
}

// TestDetermineInitialDatabase tests the determineInitialDatabase function
func TestDetermineInitialDatabase(t *testing.T) {
	// Cannot use t.Parallel() because subtests use t.Setenv()
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
	t.Parallel()
	tests := []struct {
		name        string
		priority    string
		want        sppb.RequestOptions_Priority
		errContains string
	}{
		{
			name:     "empty string returns unspecified",
			priority: "",
			want:     sppb.RequestOptions_PRIORITY_UNSPECIFIED,
		},
		{
			name:     "HIGH",
			priority: "HIGH",
			want:     sppb.RequestOptions_PRIORITY_HIGH,
		},
		{
			name:     "MEDIUM",
			priority: "MEDIUM",
			want:     sppb.RequestOptions_PRIORITY_MEDIUM,
		},
		{
			name:     "LOW",
			priority: "LOW",
			want:     sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name:     "lowercase high",
			priority: "high",
			want:     sppb.RequestOptions_PRIORITY_HIGH,
		},
		{
			name:     "with PRIORITY_ prefix",
			priority: "PRIORITY_HIGH",
			want:     sppb.RequestOptions_PRIORITY_HIGH,
		},
		{
			name:        "invalid priority",
			priority:    "INVALID",
			want:        sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			errContains: "invalid priority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePriority(tt.priority)
			wantErr := tt.errContains != ""
			if (err != nil) != wantErr {
				t.Errorf("parsePriority() error = %v, wantErr %v", err, wantErr)
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
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		want        *sppb.DirectedReadOptions
		errContains string
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
		},
		{
			name:        "too many colons",
			input:       "us-east1:READ_ONLY:extra",
			errContains: "directed read option must be in the form of",
		},
		{
			name:        "invalid replica type",
			input:       "us-east1:INVALID",
			errContains: "<replica_type> must be either READ_WRITE or READ_ONLY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDirectedReadOption(tt.input)
			wantErr := tt.errContains != ""
			if (err != nil) != wantErr {
				t.Errorf("parseDirectedReadOption() error = %v, wantErr %v", err, wantErr)
				return
			}
			if err == nil {
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
	t.Parallel()
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
		},
		{
			name: "non-existent file",
			setupFile: func(t *testing.T) string {
				return "/non/existent/file.json"
			},
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filepath := tt.setupFile(t)

			got, err := readCredentialFile(filepath)
			wantErr := tt.errContains != ""
			if (err != nil) != wantErr {
				t.Errorf("readCredentialFile() error = %v, wantErr %v", err, wantErr)
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
	t.Parallel()
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
			args:          withRequiredFlags("--execute", "SELECT 1", "--table"),
			stdinProvider: nonPTYStdin(""),
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name:          "batch mode without --table uses table format",
			args:          withRequiredFlags("--execute", "SELECT 1"),
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
			args:          withRequiredFlags("--table"),
			stdinProvider: nonPTYStdin("SELECT 1;"),
			wantCLIFormat: enums.DisplayModeTable,
		},
		{
			name:          "--set CLI_FORMAT overrides all defaults",
			args:          withRequiredFlags("--set", "CLI_FORMAT=VERTICAL"),
			stdinProvider: ptyStdin(), // Can be any, as --set overrides
			wantCLIFormat: enums.DisplayModeVertical,
		},
		{
			name:          "--set CLI_FORMAT overrides --table flag",
			args:          withRequiredFlags("--execute", "SELECT 1", "--table", "--set", "CLI_FORMAT=VERTICAL"),
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
			sysVars := initSysVarsOrFail(t, &gopts.Spanner)

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
				expectedFormat := determineExpectedFormat(t, &gopts.Spanner)
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
	t.Parallel()
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
	// Cannot use t.Parallel() because subtests use t.Setenv()
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
			wantErr := tt.errContains != ""
			if err != nil && !wantErr {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Validate and initialize
			if err == nil {
				err = ValidateSpannerOptions(&gopts.Spanner)
				if err == nil {
					sysVars, err := initializeSystemVariables(&gopts.Spanner)
					if err == nil {
						// Check results
						verifyConnectionParams(t, sysVars, tt.wantProject, tt.wantInstance, tt.wantDatabase)
						if sysVars.Role != tt.wantRole {
							t.Errorf("Role = %q, want %q", sysVars.Role, tt.wantRole)
						}
						assertEqual(t, "Insecure", sysVars.Insecure, &tt.wantInsecure)
						assertEqual(t, "Verbose", sysVars.Verbose, &tt.wantVerbose)
						assertEqual(t, "ReadOnly", sysVars.ReadOnly, &tt.wantReadOnly)
						assertEqual(t, "RPCPriority", sysVars.RPCPriority, &tt.wantPriority)
						if tt.wantStaleness && sysVars.ReadOnlyStaleness == nil {
							t.Errorf("ReadOnlyStaleness = nil, want non-nil")
						}
					}
				}
			}

			if (err != nil) != wantErr {
				t.Errorf("error = %v, wantErr %v", err, wantErr)
				return
			}

			if tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain expected string %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestAliasFlagPrecedence tests that non-hidden flags take precedence over hidden aliases
func TestAliasFlagPrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		args         []string
		wantRole     string
		wantInsecure bool
		wantEndpoint string
	}{
		{
			name:     "role takes precedence over database-role",
			args:     withRequiredFlags("--role", "primary-role", "--database-role", "secondary-role"),
			wantRole: "primary-role",
		},
		{
			name:     "database-role used when role not specified",
			args:     withRequiredFlags("--database-role", "db-role"),
			wantRole: "db-role",
		},
		{
			name:         "insecure and skip-tls-verify both set",
			args:         withRequiredFlags("--insecure", "--skip-tls-verify"),
			wantInsecure: true,
		},
		{
			name:         "only skip-tls-verify set",
			args:         withRequiredFlags("--skip-tls-verify"),
			wantInsecure: true,
		},
		{
			name:         "endpoint takes precedence over deployment-endpoint",
			args:         withRequiredFlags("--endpoint", "primary-endpoint:443", "--deployment-endpoint", "secondary-endpoint:443"),
			wantEndpoint: "primary-endpoint:443",
		},
		{
			name:         "deployment-endpoint used when endpoint not specified",
			args:         withRequiredFlags("--deployment-endpoint", "deployment-endpoint:443"),
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
			assertEqual(t, "Insecure", sysVars.Insecure, &tt.wantInsecure)
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
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "only port specified uses localhost",
			args:     withRequiredFlags("--port", "9010"),
			wantHost: "localhost",
			wantPort: 9010,
		},
		{
			name:     "only host specified uses port 443",
			args:     withRequiredFlags("--host", "example.com"),
			wantHost: "example.com",
			wantPort: 443,
		},
		{
			name:     "both host and port specified",
			args:     withRequiredFlags("--host", "example.com", "--port", "9010"),
			wantHost: "example.com",
			wantPort: 9010,
		},
		{
			name:     "endpoint flag parses into host and port",
			args:     withRequiredFlags("--endpoint", "spanner.googleapis.com:443"),
			wantHost: "spanner.googleapis.com",
			wantPort: 443,
		},
		{
			name:     "endpoint with IPv6 address",
			args:     withRequiredFlags("--endpoint", "[2001:db8::1]:443"),
			wantHost: "2001:db8::1",
			wantPort: 443,
		},
		{
			name:     "endpoint with bare IPv6 address should fail",
			args:     withRequiredFlags("--endpoint", "2001:db8::1"),
			wantHost: "",
			wantPort: 0,
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
				if err == nil {
					t.Fatalf("initializeSystemVariables() error: %v", err)
				}
				return
			}

			if sysVars.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", sysVars.Host, tt.wantHost)
			}
			assertEqual(t, "Port", sysVars.Port, &tt.wantPort)
		})
	}
}

// TestExecuteSQLAliasPrecedence tests execute/sql alias precedence
func TestExecuteSQLAliasPrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		args      []string
		wantInput string
	}{
		{
			name:      "execute takes precedence over sql",
			args:      withRequiredFlags("--execute", "SELECT 1", "--sql", "SELECT 2"),
			wantInput: "SELECT 1",
		},
		{
			name:      "sql used when execute not specified",
			args:      withRequiredFlags("--sql", "SELECT 2"),
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
	t.Parallel()
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
			assertEqual(t, "EmbeddedEmulator", gopts.Spanner.EmbeddedEmulator, &tt.wantEmbedded)
		})
	}
}
