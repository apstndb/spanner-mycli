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
	"github.com/apstndb/spanemuboost"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/jessevdk/go-flags"
	"github.com/samber/lo"
	"golang.org/x/term"
	"spheric.cloud/xiter"

	"github.com/apstndb/spanner-mycli/enums"
)

type globalOptions struct {
	Spanner spannerOptions `group:"spanner"`
}

// We can't use `default` because spanner-mycli uses multiple flags.NewParser() to process config files and flags.
// All fields use `default-mask:"-"` to prevent struct field values from appearing in help output.
// This is necessary because go-flags displays any non-zero struct field values as defaults in help text.
//
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
// This precedence behavior may override normal flag/env/ini precedence.
type spannerOptions struct {
	ProjectId                 string            `long:"project" short:"p" env:"SPANNER_PROJECT_ID" default-mask:"-" description:"(required) GCP Project ID."`
	InstanceId                string            `long:"instance" short:"i" env:"SPANNER_INSTANCE_ID" default-mask:"-" description:"(required) Cloud Spanner Instance ID"`
	DatabaseId                string            `long:"database" short:"d" env:"SPANNER_DATABASE_ID" default-mask:"-" description:"Cloud Spanner Database ID. Optional when --detached is used."`
	Detached                  bool              `long:"detached" description:"Start in detached mode, ignoring database env var/flag" default-mask:"-"`
	Execute                   string            `long:"execute" short:"e" description:"Execute SQL statement and quit. --sql is an alias." default-mask:"-"`
	File                      string            `long:"file" short:"f" description:"Execute SQL statement from file and quit. --source is an alias." default-mask:"-"`
	Source                    string            `long:"source" hidden:"true" description:"Hidden alias of --file for Google Cloud Spanner CLI compatibility" default-mask:"-"`
	Table                     bool              `long:"table" short:"t" description:"Display output in table format for batch mode." default-mask:"-"`
	HTML                      bool              `long:"html" description:"Display output in HTML format." default-mask:"-"`
	XML                       bool              `long:"xml" description:"Display output in XML format." default-mask:"-"`
	CSV                       bool              `long:"csv" description:"Display output in CSV format." default-mask:"-"`
	Format                    string            `long:"format" description:"Output format (table, tab, vertical, html, xml, csv)" default-mask:"-"`
	Verbose                   bool              `long:"verbose" short:"v" description:"Display verbose output." default-mask:"-"`
	Credential                string            `long:"credential" description:"Use the specific credential file" default-mask:"-"`
	Prompt                    *string           `long:"prompt" description:"Set the prompt to the specified format" default-mask:"spanner%t> "`
	Prompt2                   *string           `long:"prompt2" description:"Set the prompt2 to the specified format" default-mask:"%P%R> "`
	HistoryFile               *string           `long:"history" description:"Set the history file to the specified path" default-mask:"/tmp/spanner_mycli_readline.tmp"`
	Priority                  string            `long:"priority" description:"Set default request priority (HIGH|MEDIUM|LOW)" default-mask:"-"`
	Role                      string            `long:"role" description:"Use the specific database role. --database-role is an alias." default-mask:"-"`
	Endpoint                  string            `long:"endpoint" description:"Set the Spanner API endpoint (host:port)" default-mask:"-"`
	Host                      string            `long:"host" description:"Host on which Spanner server is located" default-mask:"-"`
	Port                      int               `long:"port" description:"Port number for Spanner connection" default-mask:"-"`
	DirectedRead              string            `long:"directed-read" description:"Directed read option (replica_location:replica_type). The replicat_type is optional and either READ_ONLY or READ_WRITE" default-mask:"-"`
	SQL                       string            `long:"sql" hidden:"true" description:"Hidden alias of --execute for gcloud spanner databases execute-sql compatibility" default-mask:"-"`
	Set                       map[string]string `long:"set" key-value-delimiter:"=" description:"Set system variables e.g. --set=name1=value1 --set=name2=value2" default-mask:"-"`
	Param                     map[string]string `long:"param" key-value-delimiter:"=" description:"Set query parameters, it can be literal or type(EXPLAIN/DESCRIBE only) e.g. --param=\"p1='string_value'\" --param=p2=FLOAT64" default-mask:"-"`
	ProtoDescriptorFile       string            `long:"proto-descriptor-file" description:"Path of a file that contains a protobuf-serialized google.protobuf.FileDescriptorSet message." default-mask:"-"`
	Insecure                  *bool             `long:"insecure" description:"Skip TLS verification and permit plaintext gRPC. --skip-tls-verify is an alias." default-mask:"-"`
	SkipTlsVerify             *bool             `long:"skip-tls-verify" description:"Hidden alias of --insecure from original spanner-cli" hidden:"true" default-mask:"-"`
	EmbeddedEmulator          bool              `long:"embedded-emulator" description:"Use embedded Cloud Spanner Emulator. --project, --instance, --database, --endpoint, --insecure will be automatically configured." default-mask:"-"`
	EmulatorImage             string            `long:"emulator-image" description:"container image for --embedded-emulator" default-mask:"-"`
	EmulatorPlatform          string            `long:"emulator-platform" description:"Container platform (e.g. linux/amd64, linux/arm64) for embedded emulator" default-mask:"-"`
	SampleDatabase            string            `long:"sample-database" description:"Initialize emulator with built-in sample (e.g. fingraph, singers, banking) or path to metadata.json file. Requires --embedded-emulator." default-mask:"-"`
	ListSamples               bool              `long:"list-samples" description:"List available sample databases and exit" default-mask:"-"`
	OutputTemplate            string            `long:"output-template" description:"Filepath of output template. (EXPERIMENTAL)" default-mask:"-"`
	LogLevel                  string            `long:"log-level" default-mask:"-"`
	LogGrpc                   bool              `long:"log-grpc" description:"Show gRPC logs" default-mask:"-"`
	QueryMode                 string            `long:"query-mode" description:"Mode in which the query must be processed." choice:"NORMAL" choice:"PLAN" choice:"PROFILE" default-mask:"-"`
	Strong                    bool              `long:"strong" description:"Perform a strong query." default-mask:"-"`
	ReadTimestamp             string            `long:"read-timestamp" description:"Perform a query at the given timestamp." default-mask:"-"`
	VertexAIProject           string            `long:"vertexai-project" description:"Vertex AI project" ini-name:"vertexai_project" default-mask:"-"`
	VertexAIModel             *string           `long:"vertexai-model" description:"Vertex AI model" ini-name:"vertexai_model" default-mask:"gemini-3-flash-preview"`
	DatabaseDialect           string            `long:"database-dialect" description:"The SQL dialect of the Cloud Spanner Database." choice:"POSTGRESQL" choice:"GOOGLE_STANDARD_SQL" default-mask:"-"`
	ImpersonateServiceAccount string            `long:"impersonate-service-account" description:"Impersonate service account email" default-mask:"-"`
	Version                   bool              `long:"version" description:"Show version string." default-mask:"-"`
	StatementHelp             bool              `long:"statement-help" description:"Show statement help." hidden:"true" default-mask:"-"`
	DatabaseRole              string            `long:"database-role" description:"Hidden alias of --role for gcloud spanner databases execute-sql compatibility" hidden:"true" default-mask:"-"`
	DeploymentEndpoint        string            `long:"deployment-endpoint" hidden:"true" description:"Hidden alias of --endpoint for Google Cloud Spanner CLI compatibility" default-mask:"-"`
	EnablePartitionedDML      bool              `long:"enable-partitioned-dml" description:"Partitioned DML as default (AUTOCOMMIT_DML_MODE=PARTITIONED_NON_ATOMIC)" default-mask:"-"`
	Timeout                   string            `long:"timeout" description:"Statement timeout (e.g., '10s', '5m', '1h')" default:"10m"`
	Async                     bool              `long:"async" description:"Return immediately, without waiting for the operation in progress to complete" default-mask:"-"`
	TryPartitionQuery         bool              `long:"try-partition-query" description:"Test whether the query can be executed as partition query without execution" default-mask:"-"`
	MCP                       bool              `long:"mcp" description:"Run as MCP server" default-mask:"-"`
	// SkipSystemCommand is kept for compatibility with official Spanner CLI.
	// The official implementation uses --skip-system-command to disable shell commands,
	// so we maintain the same flag name and behavior for consistency.
	SkipSystemCommand bool `long:"skip-system-command" description:"Do not allow system commands" default-mask:"-"`
	// SystemCommand provides an alternative way to control system command execution.
	// It accepts ON/OFF values and is maintained for compatibility with Google Cloud Spanner CLI.
	// When both --skip-system-command and --system-command are used, --skip-system-command takes precedence.
	SystemCommand   *string `long:"system-command" description:"Enable or disable system commands (ON/OFF)" choice:"ON" choice:"OFF" default-mask:"ON"`
	Tee             string  `long:"tee" description:"Append a copy of output to the specified file (both screen and file)" default-mask:"-"`
	Output          string  `long:"output" short:"o" description:"Redirect output to file (file only, no screen output)" default-mask:"-"`
	SkipColumnNames bool    `long:"skip-column-names" description:"Suppress column headers in output" default-mask:"-"`
	Streaming       string  `long:"streaming" description:"Streaming output mode: AUTO (format-dependent default), TRUE (always stream), FALSE (never stream)" choice:"AUTO" choice:"TRUE" choice:"FALSE" default:"AUTO"`
	Color           string  `long:"color" description:"ANSI styling in output: AUTO (styled if TTY), TRUE (always styled), FALSE (never styled)" choice:"AUTO" choice:"TRUE" choice:"FALSE" default:"AUTO"`
	Quiet           bool    `long:"quiet" short:"q" description:"Suppress result lines like 'rows in set' for clean output" default-mask:"-"`
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

// ValidateSpannerOptions validates the spannerOptions struct.
func ValidateSpannerOptions(opts *spannerOptions) error {
	if opts.Strong && opts.ReadTimestamp != "" {
		return fmt.Errorf("invalid parameters: --strong and --read-timestamp are mutually exclusive")
	}

	// Check for mutual exclusivity between --endpoint and (--host or --port)
	if opts.Endpoint != "" && (opts.Host != "" || opts.Port != 0) {
		return fmt.Errorf("invalid combination: --endpoint and (--host or --port) are mutually exclusive")
	}

	if !opts.EmbeddedEmulator && (opts.ProjectId == "" || opts.InstanceId == "") {
		return fmt.Errorf("missing parameters: -p, -i are required")
	}

	if !opts.EmbeddedEmulator && !opts.Detached && opts.DatabaseId == "" {
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

func applyEmbeddedEmulatorDefaults(sysVars *systemVariables, opts *spannerOptions) error {
	if opts.EmbeddedEmulator {
		// When using embedded emulator, insecure connection is required
		sysVars.Connection.Insecure = true
		// sysVars.Connection.Host/Port and sysVars.Connection.WithoutAuthentication will be set in run() after emulator starts

		// Set default values for embedded emulator if not specified by user
		if sysVars.Connection.Project == "" {
			sysVars.Connection.Project = spanemuboost.DefaultProjectID
		}
		if sysVars.Connection.Instance == "" {
			sysVars.Connection.Instance = spanemuboost.DefaultInstanceID
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
		{"CLI_DATABASE_DIALECT", opts.DatabaseDialect, "--database-dialect"},
		{"AUTOCOMMIT_DML_MODE", lo.Ternary(opts.EnablePartitionedDML, "PARTITIONED_NON_ATOMIC", ""), "--enable-partitioned-dml"},
		{"STATEMENT_TIMEOUT", opts.Timeout, "--timeout"},
		{"RPC_PRIORITY", cmp.Or(opts.Priority, "MEDIUM"), "--priority"},
		{"CLI_QUERY_MODE", opts.QueryMode, "--query-mode"},
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
		applyEmbeddedEmulatorDefaults,
	} {
		if err := apply(sysVars, opts); err != nil {
			return nil, err
		}
	}

	return sysVars, nil
}

func parseFlags() (globalOptions, *flags.Parser, error) {
	// Initialize globalOptions with default values that should appear in help text.
	// go-flags uses struct field values as defaults when displaying help.
	// This is why we set EmulatorImage here instead of using default-mask.
	var gopts globalOptions
	gopts.Spanner.EmulatorImage = spanemuboost.DefaultEmulatorImage

	// process config files at first
	configFileParser := flags.NewParser(&gopts, flags.Default)

	if err := readConfigFile(configFileParser); err != nil {
		return globalOptions{}, nil, fmt.Errorf("invalid config file format: %w", err)
	}

	// then, process environment variables and command line options
	// use another parser to process environment variables with higher precedence than configuration files
	flagParser := flags.NewParser(&gopts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)

	_, err := flagParser.Parse()
	return gopts, flagParser, err
}

const cnfFileName = ".spanner_mycli.cnf"

func readConfigFile(parser *flags.Parser) error {
	var cnfFiles []string
	if currentUser, err := user.Current(); err == nil {
		cnfFiles = append(cnfFiles, filepath.Join(currentUser.HomeDir, cnfFileName))
	}

	cwd, _ := os.Getwd() // ignore err
	cwdCnfFile := filepath.Join(cwd, cnfFileName)
	cnfFiles = append(cnfFiles, cwdCnfFile)

	iniParser := flags.NewIniParser(parser)
	for _, cnfFile := range cnfFiles {
		// skip if missing
		if _, err := os.Stat(cnfFile); err != nil {
			continue
		}
		if err := iniParser.ParseFile(cnfFile); err != nil {
			return err
		}
	}

	return nil
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
	// Note: This precedence may override normal flag/env/ini precedence
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
