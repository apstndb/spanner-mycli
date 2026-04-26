//
// Copyright 2020 Google LLC
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
//

package mycli

import (
	"cmp"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/alecthomas/kong"
	"github.com/apstndb/spanemuboost"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/samber/lo"
	"golang.org/x/term"
	"spheric.cloud/xiter"

	"github.com/apstndb/spanner-mycli/enums"
)

type globalOptions struct {
	Spanner spannerOptions `embed:""`
}

// Alias flags handling:
// Some flags have hidden aliases for compatibility with other tools:
// - --sql is an alias of --execute (gcloud spanner databases execute-sql compatibility)
// - --database-role is an alias of --role (gcloud spanner databases execute-sql compatibility)
// - --skip-tls-verify is an alias of --insecure (original spanner-cli compatibility)
// - --deployment-endpoint is an alias of --endpoint (Google Cloud Spanner CLI compatibility)
//
// Precedence behavior:
// - For string flags (role/database-role, endpoint/deployment-endpoint): Non-hidden flag takes precedence when both are non-empty
// - For boolean flags (insecure/skip-tls-verify): Non-hidden flag takes precedence when both are set
// This precedence behavior may override normal flag/env/TOML precedence.
type helpRequestedError struct{}

func (helpRequestedError) Error() string { return "help requested" }

type versionRequestedError struct{}

func (versionRequestedError) Error() string { return "version requested" }

const (
	defaultEmbeddedOmniProjectID  = "default"
	defaultEmbeddedOmniInstanceID = "default"
)

type showHelpFlag bool

// BeforeReset runs for matched flags in Kong's traced path, which lets help
// and version exit cleanly before defaults and resolvers are applied.
func (h showHelpFlag) BeforeReset(ctx *kong.Context) error {
	// Kong's PrintUsage(false) renders the full help text; only PrintUsage(true)
	// emits the one-line summary.
	if err := ctx.PrintUsage(false); err != nil {
		return err
	}
	return helpRequestedError{}
}

type showVersionFlag bool

func (v showVersionFlag) BeforeReset(app *kong.Kong, vars kong.Vars) error {
	fmt.Fprintln(app.Stdout, vars["version"])
	fmt.Fprintln(app.Stdout, vars["installFrom"])
	return versionRequestedError{}
}

// caseInsensitiveEnumValue accepts case-insensitive CLI input and normalizes
// it to uppercase so Kong enum validation runs on the normalized value.
type caseInsensitiveEnumValue string

func (v *caseInsensitiveEnumValue) UnmarshalText(text []byte) error {
	*v = caseInsensitiveEnumValue(strings.ToUpper(string(text)))
	return nil
}

type spannerOptions struct {
	ProjectId           string            `name:"project" short:"p" help:"(required) GCP Project ID ($SPANNER_PROJECT_ID)."`
	InstanceId          string            `name:"instance" short:"i" help:"(required) Cloud Spanner Instance ID ($SPANNER_INSTANCE_ID)"`
	DatabaseId          string            `name:"database" short:"d" help:"Cloud Spanner Database ID. Optional when --detached is used ($SPANNER_DATABASE_ID)."`
	Detached            bool              `name:"detached" help:"Start in detached mode, ignoring database env var/flag"`
	Execute             string            `name:"execute" short:"e" help:"Execute SQL statement and quit. --sql is an alias."`
	File                string            `name:"file" short:"f" help:"Execute SQL statement from file and quit. --source is an alias."`
	Source              string            `name:"source" hidden:"" help:"Hidden alias of --file for Google Cloud Spanner CLI compatibility"`
	Table               bool              `name:"table" short:"t" help:"Display output in table format for batch mode."`
	HTML                bool              `name:"html" help:"Display output in HTML format."`
	XML                 bool              `name:"xml" help:"Display output in XML format."`
	CSV                 bool              `name:"csv" help:"Display output in CSV format."`
	Format              string            `name:"format" help:"Output format (table, tab, vertical, html, xml, csv, jsonl)"`
	Verbose             bool              `name:"verbose" short:"v" help:"Display verbose output."`
	Credential          string            `name:"credential" help:"Use the specific credential file"`
	Prompt              *string           `name:"prompt" help:"Set the prompt to the specified format (default: ${defaultPromptQuoted})"`
	Prompt2             *string           `name:"prompt2" help:"Set the prompt2 to the specified format (default: ${defaultPrompt2Quoted})"`
	HistoryFile         *string           `name:"history" help:"Set the history file to the specified path (default: ${defaultHistoryFile})"`
	Priority            string            `name:"priority" help:"Set default request priority (HIGH|MEDIUM|LOW)"`
	Role                string            `name:"role" help:"Use the specific database role. --database-role is an alias."`
	Endpoint            string            `name:"endpoint" help:"Set the Spanner API endpoint (host:port)"`
	Host                string            `name:"host" help:"Host on which Spanner server is located"`
	Port                int               `name:"port" help:"Port number for Spanner connection"`
	DirectedRead        string            `name:"directed-read" help:"Directed read option (replica_location:replica_type). The replica_type is optional and either READ_ONLY or READ_WRITE"`
	SQL                 string            `name:"sql" hidden:"" help:"Hidden alias of --execute for gcloud spanner databases execute-sql compatibility"`
	Set                 map[string]string `name:"set" mapsep:"none" help:"Set system variables e.g. --set=name1=value1 --set=name2=value2"`
	Param               map[string]string `name:"param" mapsep:"none" help:"Set query parameters, it can be literal or type(EXPLAIN/DESCRIBE only) e.g. --param=\"p1='string_value'\" --param=p2=FLOAT64"`
	ProtoDescriptorFile string            `name:"proto-descriptor-file" help:"Path of a file that contains a protobuf-serialized google.protobuf.FileDescriptorSet message."`
	Insecure            *bool             `name:"insecure" help:"Skip TLS verification and permit plaintext gRPC. --skip-tls-verify is an alias."`
	SkipTlsVerify       *bool             `name:"skip-tls-verify" hidden:"" help:"Hidden alias of --insecure from original spanner-cli"`
	EmbeddedEmulator    bool              `name:"embedded-emulator" help:"Use embedded Cloud Spanner Emulator. --project, --instance, --database, --endpoint, --insecure will be automatically configured."`
	EmbeddedOmni        bool              `name:"embedded-omni" help:"Use embedded experimental Spanner Omni. --project, --instance, --database, --endpoint, --insecure will be automatically configured."`
	EmulatorImage       string            `name:"emulator-image" help:"container image for embedded runtime (--embedded-emulator or --embedded-omni)"`
	EmulatorPlatform    string            `name:"emulator-platform" help:"Container platform (e.g. linux/amd64, linux/arm64) for embedded runtime"`
	SampleDatabase      string            `name:"sample-database" help:"Initialize emulator with built-in sample (e.g. fingraph, singers, banking) or path to metadata.json file. Requires --embedded-emulator."`
	ListSamples         bool              `name:"list-samples" help:"List available sample databases and exit"`
	OutputTemplate      string            `name:"output-template" help:"Filepath of output template. (EXPERIMENTAL)"`
	LogLevel            string            `name:"log-level"`
	LogGrpc             bool              `name:"log-grpc" help:"Show gRPC logs"`
	// Kong only accepts enum validation on optional flags when they are modeled as
	// pointers. Keeping these as *string preserves "unset" semantics while still
	// letting Kong validate and document the allowed values natively.
	QueryMode                 *caseInsensitiveEnumValue `name:"query-mode" help:"Mode in which the query must be processed. Allowed values: NORMAL, PLAN, PROFILE." enum:"NORMAL,PLAN,PROFILE"`
	Strong                    bool                      `name:"strong" help:"Perform a strong query."`
	ReadTimestamp             string                    `name:"read-timestamp" help:"Perform a query at the given timestamp."`
	VertexAIProject           string                    `name:"vertexai-project" help:"Vertex AI project"`
	VertexAIModel             *string                   `name:"vertexai-model" help:"Vertex AI model (default: ${defaultVertexAIModel})"`
	VertexAILocation          *string                   `name:"vertexai-location" help:"Vertex AI location (default: ${defaultVertexAILocation})"`
	DatabaseDialect           *caseInsensitiveEnumValue `name:"database-dialect" help:"The SQL dialect of the Cloud Spanner Database. Allowed values: POSTGRESQL, GOOGLE_STANDARD_SQL, DATABASE_DIALECT_UNSPECIFIED. Omit this flag to leave it unset." enum:"POSTGRESQL,GOOGLE_STANDARD_SQL,DATABASE_DIALECT_UNSPECIFIED"`
	ImpersonateServiceAccount string                    `name:"impersonate-service-account" help:"Impersonate service account email"`
	Help                      showHelpFlag              `name:"help" short:"h" help:"Show this help message and exit."`
	Version                   showVersionFlag           `name:"version" help:"Show version string."`
	StatementHelp             bool                      `name:"statement-help" hidden:"" help:"Show statement help."`
	DatabaseRole              string                    `name:"database-role" hidden:"" help:"Hidden alias of --role for gcloud spanner databases execute-sql compatibility"`
	DeploymentEndpoint        string                    `name:"deployment-endpoint" hidden:"" help:"Hidden alias of --endpoint for Google Cloud Spanner CLI compatibility"`
	EnablePartitionedDML      bool                      `name:"enable-partitioned-dml" help:"Partitioned DML as default (AUTOCOMMIT_DML_MODE=PARTITIONED_NON_ATOMIC)"`
	Timeout                   string                    `name:"timeout" help:"Statement timeout (e.g., '10s', '5m', '1h')" default:"10m"`
	Async                     bool                      `name:"async" help:"Return immediately, without waiting for the operation in progress to complete"`
	TryPartitionQuery         bool                      `name:"try-partition-query" help:"Test whether the query can be executed as partition query without execution"`
	MCP                       bool                      `name:"mcp" help:"Run as MCP server"`
	// SkipSystemCommand is kept for compatibility with official Spanner CLI.
	// The official implementation uses --skip-system-command to disable shell commands,
	// so we maintain the same flag name and behavior for consistency.
	SkipSystemCommand bool `name:"skip-system-command" help:"Do not allow system commands"`
	// SystemCommand provides an alternative way to control system command
	// execution. Kong's default keeps the documented ON value aligned with the
	// effective default, while --skip-system-command still takes precedence.
	SystemCommand   *string `name:"system-command" help:"Enable or disable system commands (ON/OFF). Default: ON." enum:"ON,OFF" default:"ON" placeholder:"ON|OFF"`
	Tee             string  `name:"tee" help:"Append a copy of output to the specified file (both screen and file)"`
	Output          string  `name:"output" short:"o" help:"Redirect query/data output to file (overwrites existing file)"`
	SkipColumnNames bool    `name:"skip-column-names" help:"Suppress column headers in output"`
	Streaming       string  `name:"streaming" help:"Streaming output mode: AUTO (format-dependent default), TRUE (always stream), FALSE (never stream)" enum:"AUTO,TRUE,FALSE" default:"AUTO"`
	Color           string  `name:"color" help:"ANSI styling in output: AUTO (styled if TTY), TRUE (always styled), FALSE (never styled)" enum:"AUTO,TRUE,FALSE" default:"AUTO"`
	Quiet           bool    `name:"quiet" short:"q" help:"Suppress result lines like 'rows in set' for clean output"`
}

// determineInitialDatabase determines the initial database based on CLI flags and environment
func determineInitialDatabase(opts *spannerOptions) string {
	// Explicit --detached flag overrides everything
	if opts.Detached {
		return "" // Start in detached mode
	}

	// Explicit --database flag takes precedence
	if opts.DatabaseId != "" {
		return opts.DatabaseId
	}

	// Fall back to environment variable
	if envDB := os.Getenv("SPANNER_DATABASE_ID"); envDB != "" {
		return envDB
	}

	// Default: detached mode (empty string)
	return ""
}

func (opts *spannerOptions) usesEmbeddedRuntime() bool {
	return opts.EmbeddedEmulator || opts.EmbeddedOmni
}

func (opts *spannerOptions) embeddedRuntimeBackend() spanemuboost.Backend {
	if opts.EmbeddedOmni {
		return spanemuboost.BackendOmni
	}
	return spanemuboost.BackendEmulator
}

// ValidateSpannerOptions validates the spannerOptions struct.
func ValidateSpannerOptions(opts *spannerOptions) error {
	if opts.Strong && opts.ReadTimestamp != "" {
		return fmt.Errorf("invalid parameters: --strong and --read-timestamp are mutually exclusive")
	}

	// Check for mutual exclusivity between --endpoint and (--host or --port)
	if opts.Endpoint != "" && (opts.Host != "" || opts.Port != 0) {
		return fmt.Errorf("invalid combination: --endpoint and (--host or --port) are mutually exclusive")
	}

	if opts.EmbeddedEmulator && opts.EmbeddedOmni {
		return fmt.Errorf("invalid combination: --embedded-emulator and --embedded-omni are mutually exclusive")
	}

	if !opts.usesEmbeddedRuntime() && (opts.ProjectId == "" || opts.InstanceId == "") {
		return fmt.Errorf("missing parameters: -p, -i are required")
	}

	if !opts.usesEmbeddedRuntime() && !opts.Detached && opts.DatabaseId == "" {
		return fmt.Errorf("missing parameter: -d is required (or use --detached for detached mode)")
	}

	// Check for mutually exclusive input methods
	// Note: --execute and --sql are aliases, --file and --source are aliases.
	inputMethodsCount := 0
	if opts.File != "" || opts.Source != "" {
		inputMethodsCount++
	}
	if opts.Execute != "" || opts.SQL != "" {
		inputMethodsCount++
	}

	if inputMethodsCount > 1 {
		return fmt.Errorf("invalid combination: --execute(-e), --file(-f), --sql, --source are exclusive")
	}
	// Validate sample database flags
	if opts.SampleDatabase != "" && !opts.EmbeddedEmulator {
		return fmt.Errorf("--sample-database requires --embedded-emulator")
	}
	if opts.ListSamples && opts.SampleDatabase != "" {
		return fmt.Errorf("--list-samples and --sample-database are mutually exclusive")
	}

	if opts.TryPartitionQuery {
		hasInput := opts.Execute != "" || opts.SQL != "" || opts.File != "" || opts.Source != ""
		if !hasInput {
			return fmt.Errorf("--try-partition-query requires SQL input via --execute(-e), --file(-f), --source, or --sql")
		}
	}

	// Check for mutually exclusive output format options
	formatCount := 0
	if opts.Table {
		formatCount++
	}
	if opts.HTML {
		formatCount++
	}
	if opts.XML {
		formatCount++
	}
	if opts.CSV {
		formatCount++
	}
	if opts.Format != "" {
		formatCount++
	}
	if formatCount > 1 {
		return fmt.Errorf("invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive")
	}

	// Validate format string if provided
	if opts.Format != "" {
		if _, err := enums.DisplayModeString(opts.Format); err != nil {
			return err
		}
	}

	return nil
}

// parseParams converts command-line parameters to AST nodes for queries.
func parseParams(paramMap map[string]string) (map[string]ast.Node, error) {
	params := make(map[string]ast.Node)
	for k, v := range paramMap {
		if typ, err := memefish.ParseType("", v); err == nil {
			params[k] = typ
			continue
		}

		// ignore ParseType error
		if expr, err := memefish.ParseExpr("", v); err != nil {
			return nil, fmt.Errorf("error on parsing --param=%v=%v: %w", k, v, err)
		} else {
			params[k] = expr
		}
	}
	return params, nil
}

// getFormatFromOptions extracts the output format from spannerOptions based on flag precedence.
// Returns DisplayModeUnspecified if no format flag is set.
func getFormatFromOptions(opts *spannerOptions) enums.DisplayMode {
	// Individual format flags take precedence over --format for backward compatibility
	switch {
	case opts.HTML:
		return enums.DisplayModeHTML
	case opts.XML:
		return enums.DisplayModeXML
	case opts.CSV:
		return enums.DisplayModeCSV
	case opts.Table:
		return enums.DisplayModeTable
	case opts.Format != "":
		// Parse the format string to enum
		parsed, err := enums.DisplayModeString(opts.Format)
		if err != nil {
			// Return unspecified on parse error - will be handled by caller
			return enums.DisplayModeUnspecified
		}
		return parsed
	default:
		return enums.DisplayModeUnspecified
	}
}

// createSystemVariablesFromOptions creates a systemVariables instance from spannerOptions.
func createSystemVariablesFromOptions(opts *spannerOptions) (*systemVariables, error) {
	params, err := parseParams(opts.Param)
	if err != nil {
		return nil, err
	}

	l, err := SetLogLevel(cmp.Or(opts.LogLevel, "WARN"))
	if err != nil {
		return nil, fmt.Errorf("error on parsing --log-level=%v", opts.LogLevel)
	}

	// Start with defaults and override with options
	sysVars := newSystemVariablesWithDefaults()
	// Don't initialize registry here - it needs to be done after the final
	// systemVariables is in its permanent location to avoid closure issues

	// Override with command-line options (only when explicitly provided)
	sysVars.Connection.Project = opts.ProjectId
	sysVars.Connection.Instance = opts.InstanceId
	sysVars.Connection.Database = determineInitialDatabase(opts)
	sysVars.Display.Verbose = opts.Verbose || opts.MCP // Set Verbose to true when MCP is true
	sysVars.Feature.MCP = opts.MCP                     // Set MCP field for CLI_MCP system variable

	// Override defaults only if explicitly provided
	if opts.Prompt != nil {
		sysVars.Display.Prompt = *opts.Prompt
	}
	if opts.Prompt2 != nil {
		sysVars.Display.Prompt2 = *opts.Prompt2
	}
	if opts.HistoryFile != nil {
		sysVars.Display.HistoryFile = *opts.HistoryFile
	}
	if opts.VertexAIModel != nil {
		sysVars.Feature.VertexAIModel = *opts.VertexAIModel
	}
	if opts.VertexAILocation != nil {
		sysVars.Feature.VertexAILocation = *opts.VertexAILocation
	}

	// Handle alias flags with precedence for non-hidden flags
	// Note: This precedence may override normal flag/env/ini precedence
	if opts.Role != "" && opts.DatabaseRole != "" {
		slog.Warn("Both --role and --database-role are specified. Using --role (alias has lower precedence)")
	}
	sysVars.Connection.Role = cmp.Or(opts.Role, opts.DatabaseRole)

	// Handle --insecure/--skip-tls-verify aliases with proper precedence
	if opts.Insecure != nil && opts.SkipTlsVerify != nil {
		slog.Warn("Both --insecure and --skip-tls-verify are specified. Using --insecure (alias has lower precedence)")
	}

	// Implement precedence: non-hidden flag (--insecure) takes precedence when both are set
	if opts.Insecure != nil {
		sysVars.Connection.Insecure = *opts.Insecure
	} else if opts.SkipTlsVerify != nil {
		sysVars.Connection.Insecure = *opts.SkipTlsVerify
	} else {
		sysVars.Connection.Insecure = false // default value
	}

	// Handle --endpoint/--deployment-endpoint aliases with proper precedence
	if opts.Endpoint != "" && opts.DeploymentEndpoint != "" {
		slog.Warn("Both --endpoint and --deployment-endpoint are specified. Using --endpoint (alias has lower precedence)")
	}

	// Handle endpoint, host, and port logic
	endpoint := cmp.Or(opts.Endpoint, opts.DeploymentEndpoint)
	if endpoint != "" {
		// Parse endpoint into host and port
		host, port, err := parseEndpoint(endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to parse endpoint %q: %w", endpoint, err)
		}
		sysVars.Connection.Host, sysVars.Connection.Port = host, port
	} else if opts.Host != "" || opts.Port != 0 {
		// Handle --host and --port flags
		if opts.Port != 0 && opts.Host == "" {
			// Only port specified: use localhost for emulator
			sysVars.Connection.Host = "localhost"
			sysVars.Connection.Port = opts.Port
		} else if opts.Host != "" && opts.Port == 0 {
			// Only host specified: use standard Spanner port
			sysVars.Connection.Host = opts.Host
			sysVars.Connection.Port = 443
		} else {
			// Both specified
			sysVars.Connection.Host = opts.Host
			sysVars.Connection.Port = opts.Port
		}
	}
	sysVars.Params = params
	sysVars.Feature.LogGrpc = opts.LogGrpc
	sysVars.Feature.LogLevel = l
	sysVars.Connection.ImpersonateServiceAccount = opts.ImpersonateServiceAccount
	sysVars.Feature.VertexAIProject = opts.VertexAIProject
	sysVars.Feature.AsyncDDL = opts.Async

	// Handle system command options
	// Priority: --skip-system-command takes precedence over --system-command
	if opts.SkipSystemCommand {
		sysVars.Feature.SkipSystemCommand = true
	} else if opts.SystemCommand != nil {
		// --system-command=OFF disables system commands
		sysVars.Feature.SkipSystemCommand = *opts.SystemCommand == "OFF"
	}
	// If neither flag is set, system commands are enabled by default (SkipSystemCommand = false)

	sysVars.Display.SkipColumnNames = opts.SkipColumnNames

	// Handle quiet flag for suppressing result lines
	sysVars.Display.SuppressResultLines = opts.Quiet

	return &sysVars, nil
}

// optionMapping maps a CLI option value to a system variable via SetFromSimple.
type optionMapping struct {
	varName  string
	value    string
	flagName string
}

// applyOptionMappings applies a slice of option-to-system-variable mappings.
// Empty values are skipped.
func applyOptionMappings(sysVars *systemVariables, mappings []optionMapping) error {
	for _, m := range mappings {
		if m.value == "" {
			continue
		}
		if err := sysVars.SetFromSimple(m.varName, m.value); err != nil {
			return fmt.Errorf("invalid value of %s: %v: %w", m.flagName, m.value, err)
		}
	}
	return nil
}

func applyOutputTemplate(sysVars *systemVariables, opts *spannerOptions) error {
	if opts.OutputTemplate == "" {
		setDefaultOutputTemplate(sysVars)
	} else {
		if err := setOutputTemplateFile(sysVars, opts.OutputTemplate); err != nil {
			return fmt.Errorf("parse error of output template: %w", err)
		}
	}
	return nil
}

func applyStalenessOptions(sysVars *systemVariables, opts *spannerOptions) error {
	if opts.Strong {
		sysVars.Query.ReadOnlyStaleness = lo.ToPtr(spanner.StrongRead())
	}
	if opts.ReadTimestamp != "" {
		ts, err := time.Parse(time.RFC3339Nano, opts.ReadTimestamp)
		if err != nil {
			return fmt.Errorf("error on parsing --read-timestamp=%v: %w", opts.ReadTimestamp, err)
		}
		sysVars.Query.ReadOnlyStaleness = lo.ToPtr(spanner.ReadTimestamp(ts))
	}
	return nil
}

func applyProtoDescriptors(sysVars *systemVariables, opts *spannerOptions) error {
	ss := lo.Ternary(opts.ProtoDescriptorFile != "", strings.Split(opts.ProtoDescriptorFile, ","), nil)
	for _, s := range ss {
		if err := sysVars.AddFromGoogleSQL("CLI_PROTO_DESCRIPTOR_FILE", strconv.Quote(s)); err != nil {
			return fmt.Errorf("error on --proto-descriptor-file, file: %v: %w", s, err)
		}
	}
	return nil
}

func applyDirectedRead(sysVars *systemVariables, opts *spannerOptions) error {
	if opts.DirectedRead != "" {
		directedRead, err := parseDirectedReadOption(opts.DirectedRead)
		if err != nil {
			return fmt.Errorf("invalid directed read option: %w", err)
		}
		sysVars.Query.DirectedRead = directedRead
	}
	return nil
}

func applyFormatAndSetOptions(sysVars *systemVariables, opts *spannerOptions) error {
	// Set CLI_FORMAT defaults based on flags before processing --set
	// This allows --set CLI_FORMAT=X to override these defaults
	sets := maps.Collect(xiter.MapKeys(maps.All(opts.Set), strings.ToUpper))
	if _, ok := sets["CLI_FORMAT"]; !ok {
		formatMode := getFormatFromOptions(opts)
		if formatMode != enums.DisplayModeUnspecified {
			// Set directly on the struct to avoid converting back to string
			// and re-parsing inside SetFromSimple.
			sysVars.Display.CLIFormat = formatMode
		}
	}
	for k, v := range sets {
		if err := sysVars.SetFromSimple(k, v); err != nil {
			return fmt.Errorf("failed to set system variable. name: %v, value: %v: %w", k, v, err)
		}
	}
	return nil
}

func applyEmbeddedRuntimeDefaults(sysVars *systemVariables, opts *spannerOptions) error {
	if opts.usesEmbeddedRuntime() {
		// When using an embedded runtime, insecure connection is required.
		sysVars.Connection.Insecure = true
		// sysVars.Connection.Host/Port and authentication-related settings are set in run() after the runtime starts.

		// Set default values for embedded runtime if not specified by user.
		if sysVars.Connection.Project == "" {
			if opts.EmbeddedOmni {
				sysVars.Connection.Project = defaultEmbeddedOmniProjectID
			} else {
				sysVars.Connection.Project = spanemuboost.DefaultProjectID
			}
		}
		if sysVars.Connection.Instance == "" {
			if opts.EmbeddedOmni {
				sysVars.Connection.Instance = defaultEmbeddedOmniInstanceID
			} else {
				sysVars.Connection.Instance = spanemuboost.DefaultInstanceID
			}
		}
		// For database, respect --detached mode (empty database)
		if sysVars.Connection.Database == "" && !opts.Detached {
			sysVars.Connection.Database = spanemuboost.DefaultDatabaseID
		}
	}
	return nil
}

// initializeSystemVariables initializes the systemVariables struct based on spannerOptions.
// It extracts the logic for setting default values and applying flag values.
func initializeSystemVariables(opts *spannerOptions) (*systemVariables, error) {
	sysVars, err := createSystemVariablesFromOptions(opts)
	if err != nil {
		return nil, err
	}

	// DefaultAnalyzeColumns is a hardcoded constant that cannot fail.
	lo.Must0(sysVars.SetFromSimple("CLI_ANALYZE_COLUMNS", DefaultAnalyzeColumns))

	if err := applyOptionMappings(sysVars, []optionMapping{
		{"CLI_DATABASE_DIALECT", string(lo.FromPtr(opts.DatabaseDialect)), "--database-dialect"},
		{"AUTOCOMMIT_DML_MODE", lo.Ternary(opts.EnablePartitionedDML, "PARTITIONED_NON_ATOMIC", ""), "--enable-partitioned-dml"},
		{"STATEMENT_TIMEOUT", opts.Timeout, "--timeout"},
		{"RPC_PRIORITY", cmp.Or(opts.Priority, "MEDIUM"), "--priority"},
		{"CLI_QUERY_MODE", string(lo.FromPtr(opts.QueryMode)), "--query-mode"},
		{"CLI_TRY_PARTITION_QUERY", lo.Ternary(opts.TryPartitionQuery, "TRUE", ""), "--try-partition-query"},
		{"CLI_STREAMING", lo.Ternary(opts.Streaming != "" && opts.Streaming != "AUTO", opts.Streaming, ""), "--streaming"},
		{"CLI_STYLED_OUTPUT", lo.Ternary(opts.Color != "" && opts.Color != "AUTO", opts.Color, ""), "--color"},
	}); err != nil {
		return nil, err
	}

	for _, apply := range []func(*systemVariables, *spannerOptions) error{
		applyOutputTemplate,
		applyStalenessOptions,
		applyProtoDescriptors,
		applyDirectedRead,
		applyFormatAndSetOptions,
		applyEmbeddedRuntimeDefaults,
	} {
		if err := apply(sysVars, opts); err != nil {
			return nil, err
		}
	}

	return sysVars, nil
}

func parseFlags(installFrom string) (globalOptions, *kong.Kong, error) {
	return parseFlagsArgs(os.Args[1:], installFrom, defaultConfigFiles(), os.Stdout, os.Stderr)
}

func parseFlagsArgs(args []string, installFrom string, configFiles []string, stdout, stderr io.Writer) (globalOptions, *kong.Kong, error) {
	gopts := newGlobalOptions()
	parser, err := newFlagParser(&gopts, installFrom, configFiles, stdout, stderr)
	if err != nil {
		return globalOptions{}, nil, err
	}
	_, err = parser.Parse(args)
	return gopts, parser, err
}

func newGlobalOptions() globalOptions {
	var gopts globalOptions
	gopts.Spanner.EmulatorImage = spanemuboost.DefaultEmulatorImage
	return gopts
}

const configFileName = ".spanner_mycli.toml"

func defaultConfigFiles() []string {
	var files []string
	if currentUser, err := user.Current(); err == nil {
		files = append(files, filepath.Join(currentUser.HomeDir, configFileName))
	}

	cwd, _ := os.Getwd() // ignore err
	files = append(files, filepath.Join(cwd, configFileName))
	return files
}

func newFlagParser(gopts *globalOptions, installFrom string, configFiles []string, stdout, stderr io.Writer) (*kong.Kong, error) {
	options := []kong.Option{
		kong.Name("spanner-mycli"),
		kong.NoDefaultHelp(),
		kong.Writers(stdout, stderr),
		kong.Vars{
			"version":                 getVersion(),
			"installFrom":             installFrom,
			"defaultPromptQuoted":     strconv.Quote(defaultPrompt),
			"defaultPrompt2Quoted":    strconv.Quote(defaultPrompt2),
			"defaultHistoryFile":      defaultHistoryFile,
			"defaultVertexAIModel":    defaultVertexAIModel,
			"defaultVertexAILocation": defaultVertexAILocation,
		},
	}

	if len(configFiles) > 0 {
		// Keep kong-toml's normal hyphenated lookup while also accepting
		// snake_case aliases such as vertexai_project for user convenience.
		options = append(options, kong.Configuration(underscoreCompatibleTOMLLoader, configFiles...))
	}
	// Context.Resolve keeps the last non-nil resolver result, so appending the
	// SPANNER_* resolver after TOML preserves CLI > env > config precedence.
	options = append(options, kong.Resolvers(spannerConnectionEnvResolver()))

	parser, err := kong.New(gopts, options...)
	if err != nil {
		return nil, fmt.Errorf("build CLI parser: %w", err)
	}
	return parser, nil
}

func spannerConnectionEnvResolver() kong.Resolver {
	envByFlagName := map[string]string{
		"project":  "SPANNER_PROJECT_ID",
		"instance": "SPANNER_INSTANCE_ID",
		"database": "SPANNER_DATABASE_ID",
	}

	return kong.ResolverFunc(func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
		envName, ok := envByFlagName[flag.Name]
		if !ok {
			return nil, nil
		}
		value, ok := os.LookupEnv(envName)
		if !ok {
			return nil, nil
		}
		return value, nil
	})
}

func readCredentialFile(filepath string) ([]byte, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// determineInputAndMode decides whether to run in interactive or batch mode
// and returns the input string, interactive flag, and any error
func determineInputAndMode(opts *spannerOptions, stdin io.Reader) (input string, interactive bool, err error) {
	// Handle alias flags - aliases have lower precedence
	// Note: This precedence may override normal flag/env/TOML precedence
	if opts.Execute != "" && opts.SQL != "" {
		slog.Warn("Both --execute and --sql are specified. Using --execute (alias has lower precedence)")
	}
	if opts.File != "" && opts.Source != "" {
		slog.Warn("Both --file and --source are specified. Using --file (alias has lower precedence)")
	}

	// Determine the file to read from
	// Note: File takes precedence over Source (alias)
	fileToRead := opts.File
	if fileToRead == "" && opts.Source != "" {
		fileToRead = opts.Source
	}

	// Check command line options first
	// Note: Execute takes precedence over SQL (alias)
	switch {
	case opts.Execute != "":
		return opts.Execute, false, nil
	case opts.SQL != "":
		return opts.SQL, false, nil
	case fileToRead == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", false, fmt.Errorf("read from stdin failed: %w", err)
		}
		return string(b), false, nil
	case fileToRead != "":
		b, err := os.ReadFile(fileToRead)
		if err != nil {
			return "", false, fmt.Errorf("read from file %v failed: %w", fileToRead, err)
		}
		return string(b), false, nil
	default:
		// No command line options - check if terminal is available
		// Check if stdin is an *os.File to determine if it's a terminal
		if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			// Terminal available - run interactively
			return "", true, nil
		}

		// No terminal - read from stdin for batch mode
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", false, fmt.Errorf("read from stdin failed: %w", err)
		}
		return string(b), false, nil
	}
}

// Functions used by multiple files. e.g. system variable, command line flags, client side statements.

func parsePriority(priority string) (sppb.RequestOptions_Priority, error) {
	if priority == "" {
		return sppb.RequestOptions_PRIORITY_UNSPECIFIED, nil
	}

	upper := strings.ToUpper(priority)

	var value string
	if !strings.HasPrefix(upper, "PRIORITY_") {
		value = "PRIORITY_" + upper
	} else {
		value = upper
	}

	p, ok := sppb.RequestOptions_Priority_value[value]
	if !ok {
		return sppb.RequestOptions_PRIORITY_UNSPECIFIED, fmt.Errorf("invalid priority: %q", value)
	}
	return sppb.RequestOptions_Priority(p), nil
}
