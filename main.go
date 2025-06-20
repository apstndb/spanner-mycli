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

// Package main is a command line tool for Cloud Spanner
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanemuboost"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"

	"github.com/cloudspannerecosystem/memefish"

	"github.com/cloudspannerecosystem/memefish/ast"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/jessevdk/go-flags"
	"github.com/samber/lo"
	"spheric.cloud/xiter"
)

type globalOptions struct {
	Spanner spannerOptions `group:"spanner"`
}

// We can't use `default` because spanner-mycli uses multiple flags.NewParser() to process config files and flags.
// All fields use `default-mask:"-"` to prevent struct field values from appearing in help output.
// This is necessary because go-flags displays any non-zero struct field values as defaults in help text.
type spannerOptions struct {
	ProjectId                 string            `long:"project" short:"p" env:"SPANNER_PROJECT_ID" default-mask:"-" description:"(required) GCP Project ID."`
	InstanceId                string            `long:"instance" short:"i" env:"SPANNER_INSTANCE_ID" default-mask:"-" description:"(required) Cloud Spanner Instance ID"`
	DatabaseId                string            `long:"database" short:"d" env:"SPANNER_DATABASE_ID" default-mask:"-" description:"Cloud Spanner Database ID. Optional when --detached is used."`
	Detached                  bool              `long:"detached" description:"Start in detached mode, ignoring database env var/flag" default-mask:"-"`
	Execute                   string            `long:"execute" short:"e" description:"Execute SQL statement and quit. --sql is an alias." default-mask:"-"`
	File                      string            `long:"file" short:"f" description:"Execute SQL statement from file and quit." default-mask:"-"`
	Table                     bool              `long:"table" short:"t" description:"Display output in table format for batch mode." default-mask:"-"`
	Verbose                   bool              `long:"verbose" short:"v" description:"Display verbose output." default-mask:"-"`
	Credential                string            `long:"credential" description:"Use the specific credential file" default-mask:"-"`
	Prompt                    *string           `long:"prompt" description:"Set the prompt to the specified format" default-mask:"spanner%t> "`
	Prompt2                   *string           `long:"prompt2" description:"Set the prompt2 to the specified format" default-mask:"%P%R> "`
	HistoryFile               *string           `long:"history" description:"Set the history file to the specified path" default-mask:"/tmp/spanner_mycli_readline.tmp"`
	Priority                  string            `long:"priority" description:"Set default request priority (HIGH|MEDIUM|LOW)" default-mask:"-"`
	Role                      string            `long:"role" description:"Use the specific database role. --database-role is an alias." default-mask:"-"`
	Endpoint                  string            `long:"endpoint" description:"Set the Spanner API endpoint (host:port)" default-mask:"-"`
	DirectedRead              string            `long:"directed-read" description:"Directed read option (replica_location:replica_type). The replicat_type is optional and either READ_ONLY or READ_WRITE" default-mask:"-"`
	SQL                       string            `long:"sql" hidden:"true" description:"alias of --execute" default-mask:"-"`
	Set                       map[string]string `long:"set" key-value-delimiter:"=" description:"Set system variables e.g. --set=name1=value1 --set=name2=value2" default-mask:"-"`
	Param                     map[string]string `long:"param" key-value-delimiter:"=" description:"Set query parameters, it can be literal or type(EXPLAIN/DESCRIBE only) e.g. --param=\"p1='string_value'\" --param=p2=FLOAT64" default-mask:"-"`
	ProtoDescriptorFile       string            `long:"proto-descriptor-file" description:"Path of a file that contains a protobuf-serialized google.protobuf.FileDescriptorSet message." default-mask:"-"`
	Insecure                  bool              `long:"insecure" description:"Skip TLS verification and permit plaintext gRPC. --skip-tls-verify is an alias." default-mask:"-"`
	SkipTlsVerify             bool              `long:"skip-tls-verify" description:"An alias of --insecure" hidden:"true" default-mask:"-"`
	EmbeddedEmulator          bool              `long:"embedded-emulator" description:"Use embedded Cloud Spanner Emulator. --project, --instance, --database, --endpoint, --insecure will be automatically configured." default-mask:"-"`
	EmulatorImage             string            `long:"emulator-image" description:"container image for --embedded-emulator" default-mask:"-"`
	OutputTemplate            string            `long:"output-template" description:"Filepath of output template. (EXPERIMENTAL)" default-mask:"-"`
	LogLevel                  string            `long:"log-level" default-mask:"-"`
	LogGrpc                   bool              `long:"log-grpc" description:"Show gRPC logs" default-mask:"-"`
	QueryMode                 string            `long:"query-mode" description:"Mode in which the query must be processed." choice:"NORMAL" choice:"PLAN" choice:"PROFILE" default-mask:"-"`
	Strong                    bool              `long:"strong" description:"Perform a strong query." default-mask:"-"`
	ReadTimestamp             string            `long:"read-timestamp" description:"Perform a query at the given timestamp." default-mask:"-"`
	VertexAIProject           string            `long:"vertexai-project" description:"Vertex AI project" ini-name:"vertexai_project" default-mask:"-"`
	VertexAIModel             *string           `long:"vertexai-model" description:"Vertex AI model" ini-name:"vertexai_model" default-mask:"gemini-2.0-flash"`
	DatabaseDialect           string            `long:"database-dialect" description:"The SQL dialect of the Cloud Spanner Database." choice:"POSTGRESQL" choice:"GOOGLE_STANDARD_SQL" default-mask:"-"`
	ImpersonateServiceAccount string            `long:"impersonate-service-account" description:"Impersonate service account email" default-mask:"-"`
	Version                   bool              `long:"version" description:"Show version string." default-mask:"-"`
	StatementHelp             bool              `long:"statement-help" description:"Show statement help." hidden:"true" default-mask:"-"`
	DatabaseRole              string            `long:"database-role" description:"alias of --role" hidden:"true" default-mask:"-"`
	EnablePartitionedDML      bool              `long:"enable-partitioned-dml" description:"Partitioned DML as default (AUTOCOMMIT_DML_MODE=PARTITIONED_NON_ATOMIC)" default-mask:"-"`
	Timeout                   string            `long:"timeout" description:"Statement timeout (e.g., '10s', '5m', '1h')" default:"10m"`
	Async                     bool              `long:"async" description:"Return immediately, without waiting for the operation in progress to complete" default-mask:"-"`
	TryPartitionQuery         bool              `long:"try-partition-query" description:"Test whether the query can be executed as partition query without execution" default-mask:"-"`
	MCP                       bool              `long:"mcp" description:"Run as MCP server" default-mask:"-"`
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


const (
	defaultPrompt         = "spanner%t> "
	defaultPrompt2        = "%P%R> "
	defaultHistoryFile    = "/tmp/spanner_mycli_readline.tmp"
	defaultVertexAIModel  = "gemini-2.0-flash"
	DefaultAnalyzeColumns = "Rows:{{.Rows.Total}},Exec.:{{.ExecutionSummary.NumExecutions}},Total Latency:{{.Latency}}"
)

var DefaultParsedAnalyzeColumns = lo.Must(customListToTableRenderDefs(DefaultAnalyzeColumns))

var (
	// https://rhysd.hatenablog.com/entry/2021/06/27/222254
	version     = ""
	installFrom = "built from source"
)

func getVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	return info.Main.Version
}

func init() {
	h := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	slog.SetDefault(h)
}

func SetLogLevel(logLevel string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return 0, err
	}

	h := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(h)
	return level, nil
}

func main() {
	gopts, parser, err := parseFlags()

	if flags.WroteHelp(err) {
		// exit successfully
		return
	} else if err != nil {
		parser.WriteHelp(os.Stderr)
		fmt.Fprintf(os.Stderr, "Invalid options: %v\n", err)
		os.Exit(exitCodeError)
		return
	} else if gopts.Spanner.Version {
		fmt.Printf("%v\n%v\n", getVersion(), installFrom)
		return
	}

	// Using the run function that returns only an error
	// and determining the exit code based on the error type using
	// the GetExitCode function in errors.go.

	if err := run(context.Background(), &gopts.Spanner); err != nil {
		var exitCodeErr *ExitCodeError
		if !errors.As(err, &exitCodeErr) {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}

		os.Exit(GetExitCode(err))
		return
	}
	os.Exit(exitCodeSuccess)
}

// run executes the main functionality of the application.
// It returns an error that may contain an exit code.
// Use GetExitCode(err) to determine the appropriate exit code.
func run(ctx context.Context, opts *spannerOptions) error {
	if opts.StatementHelp {
		fmt.Print(renderClientStatementHelp(clientSideStatementDefs))
		return nil
	}

	if err := ValidateSpannerOptions(opts); err != nil {
		return err
	}

	sysVars, err := initializeSystemVariables(opts)
	if err != nil {
		return err
	}

	var cred []byte
	if opts.Credential != "" {
		var err error
		if cred, err = readCredentialFile(opts.Credential); err != nil {
			return fmt.Errorf("failed to read the credential file: %w", err)
		}
	}

	if opts.EmbeddedEmulator {
		container, teardown, err := spanemuboost.NewEmulator(ctx,
			spanemuboost.WithProjectID(sysVars.Project),
			spanemuboost.WithInstanceID(sysVars.Instance),
			spanemuboost.WithDatabaseID(sysVars.Database),
			spanemuboost.WithEmulatorImage(cmp.Or(opts.EmulatorImage, spanemuboost.DefaultEmulatorImage)),
			spanemuboost.WithDatabaseDialect(sysVars.DatabaseDialect),
		)
		if err != nil {
			return fmt.Errorf("failed to start Cloud Spanner Emulator: %w", err)
		}
		defer teardown()

		sysVars.Endpoint = container.URI()
		sysVars.WithoutAuthentication = true
	}

	cli, err := NewCli(ctx, cred, os.Stdin, os.Stdout, os.Stderr, &sysVars)
	if err != nil {
		return fmt.Errorf("failed to connect to Spanner: %w", err)
	}

	if opts.MCP {
		return cli.RunMCP(ctx)
	}

	var input string
	if opts.Execute != "" {
		input = opts.Execute
	} else if opts.SQL != "" {
		input = opts.SQL
	} else if opts.File == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read from stdin failed: %w", err)
		}
		input = string(b)
	} else if opts.File != "" {
		b, err := os.ReadFile(opts.File)
		if err != nil {
			return fmt.Errorf("read from file %v failed: %w", opts.File, err)
		}
		input = string(b)
	} else {
		input, err = readStdin()
		if err != nil {
			return fmt.Errorf("read from stdin failed: %w", err)
		}
	}

	interactive := input == ""

	// CLI_FORMAT is set in initializeSystemVariables from opts.Set,
	// but if not set there, it's set here based on interactive mode or --table flag.
	// This logic needs to be after sysVars is initialized.
	sets := maps.Collect(xiter.MapKeys(maps.All(opts.Set), strings.ToUpper))
	if _, ok := sets["CLI_FORMAT"]; !ok {
		sysVars.CLIFormat = lo.Ternary(interactive || opts.Table, DisplayModeTable, DisplayModeTab)
	}

	switch {
	case interactive:
		sysVars.EnableProgressBar = true
		return cli.RunInteractive(ctx)
	default:
		return cli.RunBatch(ctx, input)
	}
}

// ValidateSpannerOptions validates the spannerOptions struct.
func ValidateSpannerOptions(opts *spannerOptions) error {
	if opts.Insecure && opts.SkipTlsVerify {
		return fmt.Errorf("invalid parameters: --insecure and --skip-tls-verify are mutually exclusive")
	}

	if opts.Strong && opts.ReadTimestamp != "" {
		return fmt.Errorf("invalid parameters: --strong and --read-timestamp are mutually exclusive")
	}

	if !opts.EmbeddedEmulator && (opts.ProjectId == "" || opts.InstanceId == "") {
		return fmt.Errorf("missing parameters: -p, -i are required")
	}
	
	if !opts.EmbeddedEmulator && !opts.Detached && opts.DatabaseId == "" {
		return fmt.Errorf("missing parameter: -d is required (or use --detached for detached mode)")
	}

	if nonEmptyInputCount := xiter.Count(xiter.Of(opts.File, opts.Execute, opts.SQL), lo.IsNotEmpty); nonEmptyInputCount > 1 {
		return fmt.Errorf("invalid combination: -e, -f, --sql are exclusive")
	}

	// This check is redundant with the above, but kept for consistency with original code.
	if opts.Execute != "" && opts.SQL != "" {
		return fmt.Errorf("--execute and --sql are mutually exclusive")
	}

	if opts.TryPartitionQuery {
		hasInput := opts.Execute != "" || opts.SQL != "" || opts.File != ""
		if !hasInput {
			return fmt.Errorf("--try-partition-query requires SQL input via --execute, --file, or --sql")
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

// createSystemVariablesFromOptions creates a systemVariables instance from spannerOptions.
func createSystemVariablesFromOptions(opts *spannerOptions) (systemVariables, error) {
	params, err := parseParams(opts.Param)
	if err != nil {
		return systemVariables{}, err
	}

	l, err := SetLogLevel(cmp.Or(opts.LogLevel, "WARN"))
	if err != nil {
		return systemVariables{}, fmt.Errorf("error on parsing --log-level=%v", opts.LogLevel)
	}

	// Start with defaults and override with options
	sysVars := newSystemVariablesWithDefaults()
	
	// Override with command-line options (only when explicitly provided)
	sysVars.Project = opts.ProjectId
	sysVars.Instance = opts.InstanceId
	sysVars.Database = determineInitialDatabase(opts)
	sysVars.Verbose = opts.Verbose || opts.MCP // Set Verbose to true when MCP is true
	sysVars.MCP = opts.MCP                     // Set MCP field for CLI_MCP system variable
	
	// Override defaults only if explicitly provided
	if opts.Prompt != nil {
		sysVars.Prompt = *opts.Prompt
	}
	if opts.Prompt2 != nil {
		sysVars.Prompt2 = *opts.Prompt2
	}
	if opts.HistoryFile != nil {
		sysVars.HistoryFile = *opts.HistoryFile
	}
	if opts.VertexAIModel != nil {
		sysVars.VertexAIModel = *opts.VertexAIModel
	}
	
	sysVars.Role = cmp.Or(opts.Role, opts.DatabaseRole)
	sysVars.Endpoint = opts.Endpoint
	sysVars.Insecure = opts.Insecure || opts.SkipTlsVerify
	sysVars.Params = params
	sysVars.LogGrpc = opts.LogGrpc
	sysVars.LogLevel = l
	sysVars.ImpersonateServiceAccount = opts.ImpersonateServiceAccount
	sysVars.VertexAIProject = opts.VertexAIProject
	sysVars.AsyncDDL = opts.Async

	return sysVars, nil
}

// initializeSystemVariables initializes the systemVariables struct based on spannerOptions.
// It extracts the logic for setting default values and applying flag values.
func initializeSystemVariables(opts *spannerOptions) (systemVariables, error) {
	// Create basic system variables from options
	sysVars, err := createSystemVariablesFromOptions(opts)
	if err != nil {
		return systemVariables{}, err
	}

	// initialize default value
	if err := sysVars.Set("CLI_ANALYZE_COLUMNS", DefaultAnalyzeColumns); err != nil {
		return systemVariables{}, fmt.Errorf("parse error: %w", err)
	}

	if opts.OutputTemplate == "" {
		setDefaultOutputTemplate(&sysVars)
	} else {
		if err := setOutputTemplateFile(&sysVars, opts.OutputTemplate); err != nil {
			return systemVariables{}, fmt.Errorf("parse error of output template: %w", err)
		}
	}

	if opts.Strong {
		sysVars.ReadOnlyStaleness = lo.ToPtr(spanner.StrongRead())
	}

	if opts.ReadTimestamp != "" {
		ts, err := time.Parse(time.RFC3339Nano, opts.ReadTimestamp)
		if err != nil {
			return systemVariables{}, fmt.Errorf("error on parsing --read-timestamp=%v: %w", opts.ReadTimestamp, err)
		}
		sysVars.ReadOnlyStaleness = lo.ToPtr(spanner.ReadTimestamp(ts))
	}

	if opts.DatabaseDialect != "" {
		if err := sysVars.Set("CLI_DATABASE_DIALECT", opts.DatabaseDialect); err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --database-dialect: %v: %w", opts.DatabaseDialect, err)
		}
	}

	if opts.EnablePartitionedDML {
		if err := sysVars.Set("AUTOCOMMIT_DML_MODE", "PARTITIONED_NON_ATOMIC"); err != nil {
			return systemVariables{}, fmt.Errorf("unknown error on --enable-partitioned-dml: %w", err)
		}
	}

	if opts.Timeout != "" {
		// Validate timeout format before setting system variable for better error reporting
		_, err := time.ParseDuration(opts.Timeout)
		if err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --timeout: %v: %w", opts.Timeout, err)
		}
		if err := sysVars.Set("STATEMENT_TIMEOUT", opts.Timeout); err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --timeout: %v: %w", opts.Timeout, err)
		}
	}

	ss := lo.Ternary(opts.ProtoDescriptorFile != "", strings.Split(opts.ProtoDescriptorFile, ","), nil)
	for _, s := range ss {
		if err := sysVars.Add("CLI_PROTO_DESCRIPTOR_FILE", strconv.Quote(s)); err != nil {
			return systemVariables{}, fmt.Errorf("error on --proto-descriptor-file, file: %v: %w", s, err)
		}
	}

	if opts.Priority != "" {
		priority, err := parsePriority(opts.Priority)
		if err != nil {
			return systemVariables{}, fmt.Errorf("priority must be either HIGH, MEDIUM, or LOW: %w", err)
		}
		sysVars.RPCPriority = priority
	} else {
		sysVars.RPCPriority = defaultPriority
	}

	if opts.QueryMode != "" {
		if err := sysVars.Set("CLI_QUERY_MODE", opts.QueryMode); err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --query-mode: %v: %w", opts.QueryMode, err)
		}
	}

	if opts.TryPartitionQuery {
		if err := sysVars.Set("CLI_TRY_PARTITION_QUERY", "TRUE"); err != nil {
			return systemVariables{}, fmt.Errorf("failed to set CLI_TRY_PARTITION_QUERY: %w", err)
		}
	}

	if opts.DirectedRead != "" {
		directedRead, err := parseDirectedReadOption(opts.DirectedRead)
		if err != nil {
			return systemVariables{}, fmt.Errorf("invalid directed read option: %w", err)
		}
		sysVars.DirectedRead = directedRead
	}

	sets := maps.Collect(xiter.MapKeys(maps.All(opts.Set), strings.ToUpper))
	for k, v := range sets {
		if err := sysVars.Set(k, v); err != nil {
			return systemVariables{}, fmt.Errorf("failed to set system variable. name: %v, value: %v: %w", k, v, err)
		}
	}

	if opts.EmbeddedEmulator {
		sysVars.Project = "emulator-project"
		sysVars.Instance = "emulator-instance"
		sysVars.Database = "emulator-database"
		sysVars.Insecure = true
		// sysVars.Endpoint and sysVars.WithoutAuthentication will be set in run() after emulator starts
	}

	return sysVars, nil
}

// renderClientStatementHelp generates a table of client-side statement help.
func renderClientStatementHelp(stmts []*clientSideStatementDef) string {
	var sb strings.Builder

	table := tablewriter.NewTable(&sb,
		tablewriter.WithRenderer(renderer.NewMarkdown()),
		tablewriter.WithHeaderAutoFormat(tw.Off),
		tablewriter.WithHeaderAlignment(tw.AlignLeft)).
		Configure(func(config *tablewriter.Config) {
		})

	table.Header([]string{"Usage", "Syntax", "Note"})

	for _, stmt := range stmts {
		for _, desc := range stmt.Descriptions {
			err := table.Append([]string{desc.Usage, "`" + strings.NewReplacer("|", `\|`).Replace(desc.Syntax) + ";`", desc.Note})
			if err != nil {
				slog.Error("tablewriter.Table.Append() failed", "err", err)
			}
		}
	}

	if err := table.Render(); err != nil {
		slog.Error("tablewriter.Table.Render() failed", "err", err)
	}

	return sb.String()
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
	return io.ReadAll(f)
}

func readStdin() (string, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(b), nil
	} else {
		return "", nil
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
