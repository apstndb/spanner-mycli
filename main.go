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
	"fmt"
	"io"
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
	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/olekukonko/tablewriter"

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
type spannerOptions struct {
	ProjectId                 string            `long:"project" short:"p" env:"SPANNER_PROJECT_ID"  description:"(required) GCP Project ID."`
	InstanceId                string            `long:"instance" short:"i" env:"SPANNER_INSTANCE_ID" description:"(required) Cloud Spanner Instance ID"`
	DatabaseId                string            `long:"database" short:"d" env:"SPANNER_DATABASE_ID" description:"(required) Cloud Spanner Database ID."`
	Execute                   string            `long:"execute" short:"e" description:"Execute SQL statement and quit. --sql is an alias."`
	File                      string            `long:"file" short:"f" description:"Execute SQL statement from file and quit."`
	Table                     bool              `long:"table" short:"t" description:"Display output in table format for batch mode."`
	Verbose                   bool              `long:"verbose" short:"v" description:"Display verbose output."`
	Credential                string            `long:"credential" description:"Use the specific credential file"`
	Prompt                    *string           `long:"prompt" description:"Set the prompt to the specified format" default-mask:"spanner%t> "`
	Prompt2                   *string           `long:"prompt2" description:"Set the prompt2 to the specified format" default-mask:"%P%R> "`
	LogMemefish               bool              `long:"log-memefish" description:"Emit SQL parse log using memefish"`
	HistoryFile               *string           `long:"history" description:"Set the history file to the specified path" default-mask:"/tmp/spanner_mycli_readline.tmp"`
	Priority                  string            `long:"priority" description:"Set default request priority (HIGH|MEDIUM|LOW)"`
	Role                      string            `long:"role" description:"Use the specific database role"`
	Endpoint                  string            `long:"endpoint" description:"Set the Spanner API endpoint (host:port)"`
	DirectedRead              string            `long:"directed-read" description:"Directed read option (replica_location:replica_type). The replicat_type is optional and either READ_ONLY or READ_WRITE"`
	SQL                       string            `long:"sql" hidden:"true" description:"alias of --execute"`
	Set                       map[string]string `long:"set" key-value-delimiter:"=" description:"Set system variables e.g. --set=name1=value1 --set=name2=value2"`
	Param                     map[string]string `long:"param" key-value-delimiter:"=" description:"Set query parameters, it can be literal or type(EXPLAIN/DESCRIBE only) e.g. --param=\"p1='string_value'\" --param=p2=FLOAT64"`
	ProtoDescriptorFile       string            `long:"proto-descriptor-file" description:"Path of a file that contains a protobuf-serialized google.protobuf.FileDescriptorSet message."`
	Insecure                  bool              `long:"insecure" description:"Skip TLS verification and permit plaintext gRPC. --skip-tls-verify is an alias."`
	SkipTlsVerify             bool              `long:"skip-tls-verify" description:"An alias of --insecure" hidden:"true"`
	EmbeddedEmulator          bool              `long:"embedded-emulator" description:"Use embedded Cloud Spanner Emulator. --project, --instance, --database, --endpoint, --insecure will be automatically configured."`
	EmulatorImage             string            `long:"emulator-image" description:"container image for --embedded-emulator"`
	Help                      bool              `long:"help" short:"h" hidden:"true"`
	Debug                     bool              `long:"debug" hidden:"true"`
	LogGrpc                   bool              `long:"log-grpc" description:"Show gRPC logs"`
	QueryMode                 string            `long:"query-mode" description:"Mode in which the query must be processed." choice:"NORMAL" choice:"PLAN" choice:"PROFILE"`
	Strong                    bool              `long:"strong" description:"Perform a strong query."`
	ReadTimestamp             string            `long:"read-timestamp" description:"Perform a query at the given timestamp."`
	VertexAIProject           string            `long:"vertexai-project" description:"VertexAI project" ini-name:"vertexai_project"`
	VertexAIModel             *string           `long:"vertexai-model" description:"VertexAI project" ini-name:"vertexai_model" default-mask:"gemini-2.0-flash"`
	DatabaseDialect           string            `long:"database-dialect" description:"The SQL dialect of the Cloud Spanner Database." choice:"POSTGRESQL" choice:"GOOGLE_STANDARD_SQL"`
	ImpersonateServiceAccount string            `long:"impersonate-service-account" description:"Impersonate service account email"`
	Version                   bool              `long:"version" description:"Show version string."`
	StatementHelp             bool              `long:"statement-help" description:"Show statement help." hidden:"true"`
}

func addEmulatorImageOption(parser *flags.Parser) {
	parser.Groups()[0].Find("spanner").FindOptionByLongName("emulator-image").DefaultMask = spanemuboost.DefaultEmulatorImage
}

const (
	defaultPrompt        = "spanner%t> "
	defaultPrompt2       = "%P%R> "
	defaultHistoryFile   = "/tmp/spanner_mycli_readline.tmp"
	defaultVertexAIModel = "gemini-2.0-flash"
)

var logMemefish bool

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

// Overwrite genkit/logger's init
func init() {
	/*
		h := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
		slog.SetDefault(h)
	*/
}

func main() {
	gopts, parserForHelp, err := parseFlags()

	if flags.WroteHelp(err) {
		// exit successfully
		return
	} else if err != nil {
		parserForHelp.WriteHelp(os.Stderr)
		exitf("Invalid options\n")
	} else if gopts.Spanner.Help {
		parserForHelp.WriteHelp(os.Stderr)
		return
	} else if gopts.Spanner.Version {
		fmt.Printf("%v\n%v\n", getVersion(), installFrom)
		return
	}

	exitCode := run(context.Background(), &gopts.Spanner)
	os.Exit(exitCode)
}

func run(ctx context.Context, opts *spannerOptions) (exitCode int) {
	logMemefish = opts.LogMemefish

	if opts.StatementHelp {
		fmt.Print(renderClientStatementHelp(clientSideStatementDefs))
		return exitCodeSuccess
	}

	if opts.Insecure && opts.SkipTlsVerify {
		exitf("invalid parameters: --insecure and --skip-tls-verify are mutually exclusive\n")
	}

	if opts.Strong && opts.ReadTimestamp != "" {
		exitf("invalid parameters: --strong and --read-timestamp are mutually exclusive\n")
	}

	if !opts.EmbeddedEmulator && (opts.ProjectId == "" || opts.InstanceId == "" || opts.DatabaseId == "") {
		exitf("Missing parameters: -p, -i, -d are required\n")
	}

	params := make(map[string]ast.Node)
	for k, v := range opts.Param {
		if typ, err := memefish.ParseType("", v); err == nil {
			params[k] = typ
			continue
		}

		// ignore ParseType error
		if expr, err := memefish.ParseExpr("", v); err != nil {
			exitf("error on parsing --param=%v=%v, err: %v", k, v, err)
		} else {
			params[k] = expr
		}
	}

	sysVars := systemVariables{
		Project:                   opts.ProjectId,
		Instance:                  opts.InstanceId,
		Database:                  opts.DatabaseId,
		Verbose:                   opts.Verbose,
		Prompt:                    lo.FromPtrOr(opts.Prompt, defaultPrompt),
		Prompt2:                   lo.FromPtrOr(opts.Prompt2, defaultPrompt2),
		HistoryFile:               lo.FromPtrOr(opts.HistoryFile, defaultHistoryFile),
		Role:                      opts.Role,
		Endpoint:                  opts.Endpoint,
		Insecure:                  opts.Insecure || opts.SkipTlsVerify,
		Debug:                     opts.Debug,
		Params:                    params,
		LogGrpc:                   opts.LogGrpc,
		ImpersonateServiceAccount: opts.ImpersonateServiceAccount,
		VertexAIProject:           opts.VertexAIProject,
		VertexAIModel:             lo.FromPtrOr(opts.VertexAIModel, defaultVertexAIModel),
		EnableADCPlus:             true,
		ExecutionMethodFormat:     queryplan.ExecutionMethodFormatAngle,
	}

	if opts.Strong {
		sysVars.ReadOnlyStaleness = lo.ToPtr(spanner.StrongRead())
	}

	if opts.ReadTimestamp != "" {
		ts, err := time.Parse(time.RFC3339Nano, opts.ReadTimestamp)
		if err != nil {
			exitf("error on parsing --read-timestamp=%v, err: %v\n", opts.ReadTimestamp, err)
		}
		sysVars.ReadOnlyStaleness = lo.ToPtr(spanner.ReadTimestamp(ts))
	}

	if opts.DatabaseDialect != "" {
		if err := sysVars.Set("CLI_DATABASE_DIALECT", opts.DatabaseDialect); err != nil {
			exitf("invalid value of --database-dialect: %v, err: %v\n", opts.DatabaseDialect, err)
		}
	}

	ss := lo.Ternary(opts.ProtoDescriptorFile != "", strings.Split(opts.ProtoDescriptorFile, ","), nil)
	for _, s := range ss {
		if err := sysVars.Add("CLI_PROTO_DESCRIPTOR_FILE", strconv.Quote(s)); err != nil {
			exitf("error on --proto-descriptor-file, file: %v, err: %v\n", s, err)
		}
	}

	if nonEmptyInputCount := xiter.Count(xiter.Of(opts.File, opts.Execute, opts.SQL), lo.IsNotEmpty); nonEmptyInputCount > 1 {
		exitf("Invalid combination: -e, -f, --sql are exclusive\n")
	}

	var cred []byte
	if opts.Credential != "" {
		var err error
		if cred, err = readCredentialFile(opts.Credential); err != nil {
			exitf("Failed to read the credential file: %v\n", err)
		}
	}

	if opts.Priority != "" {
		priority, err := parsePriority(opts.Priority)
		if err != nil {
			exitf("priority must be either HIGH, MEDIUM, or LOW\n")
		}
		sysVars.RPCPriority = priority
	} else {
		sysVars.RPCPriority = defaultPriority
	}

	if opts.QueryMode != "" {
		if err := sysVars.Set("CLI_QUERY_MODE", opts.QueryMode); err != nil {
			exitf("invalid value of --query-mode: %v, err: %v\n", opts.QueryMode, err)
		}
	}

	var directedRead *sppb.DirectedReadOptions
	if opts.DirectedRead != "" {
		var err error
		directedRead, err = parseDirectedReadOption(opts.DirectedRead)
		if err != nil {
			exitf("Invalid directed read option: %v\n", err)
		}
		sysVars.DirectedRead = directedRead
	}

	sets := maps.Collect(xiter.MapKeys(maps.All(opts.Set), strings.ToUpper))

	for k, v := range sets {
		if err := sysVars.Set(k, v); err != nil {
			exitf("failed to set system variable. name: %v, value: %v, err: %v\n", k, v, err)
		}
	}

	if opts.EmbeddedEmulator {
		sysVars.Project = "emulator-project"
		sysVars.Instance = "emulator-instance"
		sysVars.Database = "emulator-database"
		sysVars.Insecure = true

		container, teardown, err := spanemuboost.NewEmulator(ctx,
			spanemuboost.WithProjectID(sysVars.Project),
			spanemuboost.WithInstanceID(sysVars.Instance),
			spanemuboost.WithDatabaseID(sysVars.Database),
			spanemuboost.WithEmulatorImage(cmp.Or(opts.EmulatorImage, spanemuboost.DefaultEmulatorImage)),
			spanemuboost.WithDatabaseDialect(sysVars.DatabaseDialect),
		)
		if err != nil {
			exitf("failed to start Cloud Spanner Emulator: %v\n", err)
		}
		defer teardown()

		sysVars.Endpoint = container.URI
		sysVars.WithoutAuthentication = true
	}

	cli, err := NewCli(ctx, cred, os.Stdin, os.Stdout, os.Stderr, &sysVars)
	if err != nil {
		exitf("Failed to connect to Spanner: %v", err)
	}

	if opts.Execute != "" && opts.SQL != "" {
		exitf("--execute and --sql are mutually exclusive\n")
	}

	var input string
	if opts.Execute != "" {
		input = opts.Execute
	} else if opts.SQL != "" {
		input = opts.SQL
	} else if opts.File == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			exitf("Read from stdin failed: %v", err)
		}
		input = string(b)
	} else if opts.File != "" {
		b, err := os.ReadFile(opts.File)
		if err != nil {
			exitf("Read from file %v failed: %v", opts.File, err)
		}
		input = string(b)
	} else {
		input, err = readStdin()
		if err != nil {
			exitf("Read from stdin failed: %v", err)
		}
	}

	interactive := input == ""

	if _, ok := sets["CLI_FORMAT"]; !ok {
		sysVars.CLIFormat = lo.Ternary(interactive || opts.Table, DisplayModeTable, DisplayModeTab)
	}

	exitCode = lo.TernaryF(interactive,
		func() int {
			sysVars.EnableProgressBar = true
			return cli.RunInteractive(ctx)
		},
		func() int {
			return cli.RunBatch(ctx, input)
		})

	return exitCode
}

// renderClientStatementHelp generates a table of client-side statement help.
func renderClientStatementHelp(stmts []*clientSideStatementDef) string {
	var sb strings.Builder

	table := tablewriter.NewWriter(&sb)
	table.SetHeader([]string{"Usage", "Syntax", "Note"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)
	for _, stmt := range clientSideStatementDefs {
		for _, desc := range stmt.Descriptions {
			table.Append([]string{desc.Usage, "`" + strings.NewReplacer("|", `\|`).Replace(desc.Syntax) + ";`", desc.Note})
		}
	}
	table.Render()

	return sb.String()
}

func parseFlags() (globalOptions, *flags.Parser, error) {
	var gopts globalOptions

	// process config files at first
	configFileParser := flags.NewParser(&gopts, flags.Default)
	addEmulatorImageOption(configFileParser)

	if err := readConfigFile(configFileParser); err != nil {
		exitf("Invalid config file format\n")
	}

	// then, process environment variables and command line options
	// use another parser to process environment variables with higher precedence than configuration files
	// flagParser := flags.NewParser(&gopts, flags.PrintErrors|flags.PassDoubleDash)
	flagParser := flags.NewParser(&gopts, flags.PrintErrors|flags.PassDoubleDash)
	addEmulatorImageOption(flagParser)

	// TODO: Workaround to avoid to display config value as default
	parserForHelp := flags.NewParser(&globalOptions{}, flags.Default)
	addEmulatorImageOption(parserForHelp)
	_, err := flagParser.Parse()
	return gopts, parserForHelp, err
}

func exitf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(exitCodeError)
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
