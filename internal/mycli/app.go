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
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/apstndb/spanemuboost"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/samber/lo"
	"golang.org/x/term"

	_ "github.com/apstndb/spanner-mycli/internal/mycli/formatsql" // Register SQL export formatters
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

const (
	defaultPrompt           = "spanner%t> "
	defaultPrompt2          = "%P%R> "
	defaultHistoryFile      = "/tmp/spanner_mycli_readline.tmp"
	defaultVertexAIModel    = "gemini-3-flash-preview"
	defaultVertexAILocation = "global"
	DefaultAnalyzeColumns   = "Rows:{{.Rows.Total}},Exec.:{{.ExecutionSummary.NumExecutions}},Total Latency:{{.Latency}}"
)

var DefaultParsedAnalyzeColumns = lo.Must(customListToTableRenderDefs(DefaultAnalyzeColumns))

// buildVersion is set by Main() from ldflags-injected values in the root main package.
var buildVersion string

func getVersion() string {
	if buildVersion != "" {
		return buildVersion
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

	// Disable multiplexed session for r/w transaction,
	// workaround for https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/issues/282
	if err := os.Setenv("GOOGLE_CLOUD_SPANNER_MULTIPLEXED_SESSIONS_FOR_RW", "false"); err != nil {
		slog.Error("failed to set required environment variable for spanner emulator workaround", "error", err)
	}
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

// Main is the entry point called from the root main package.
// version and installFrom are passed from main via ldflags.
func Main(version, installFrom string) {
	buildVersion = version

	gopts, parser, err := parseFlags(installFrom)

	if errors.Is(err, helpRequestedError{}) || errors.Is(err, versionRequestedError{}) {
		// exit successfully
		return
	} else if err != nil {
		var parseErr *kong.ParseError
		if errors.As(err, &parseErr) {
			writeUsageTo(parseErr.Context, parser, os.Stderr)
		}
		fmt.Fprintf(os.Stderr, "Invalid options: %v\n", err)
		os.Exit(exitCodeError)
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

func writeUsageTo(ctx *kong.Context, _ *kong.Kong, w io.Writer) {
	if ctx == nil {
		return
	}
	stdout := ctx.Stdout
	ctx.Stdout = w
	defer func() {
		ctx.Stdout = stdout
	}()
	if err := ctx.PrintUsage(false); err != nil {
		slog.Error("failed to print usage", "error", err)
	}
}

// run executes the main functionality of the application.
// It returns an error that may contain an exit code.
// Use GetExitCode(err) to determine the appropriate exit code.
func run(ctx context.Context, opts *spannerOptions) error {
	if opts.StatementHelp {
		fmt.Print(renderClientStatementHelp(clientSideStatementDefs))
		return nil
	}

	if opts.ListSamples {
		fmt.Print(ListAvailableSamples())
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
		// Log when user provides custom values with embedded emulator
		if opts.ProjectId != "" || opts.InstanceId != "" || opts.DatabaseId != "" {
			attrs := []any{}
			if opts.ProjectId != "" {
				attrs = append(attrs, "project", opts.ProjectId)
			}
			if opts.InstanceId != "" {
				attrs = append(attrs, "instance", opts.InstanceId)
			}
			if opts.DatabaseId != "" {
				attrs = append(attrs, "database", opts.DatabaseId)
			}
			slog.Warn("Using embedded emulator with user-provided values", attrs...)
		}

		// Use values from sysVars (which includes defaults set in initializeSystemVariables)
		emulatorOpts := []spanemuboost.Option{
			spanemuboost.WithProjectID(sysVars.Connection.Project),
			spanemuboost.WithInstanceID(sysVars.Connection.Instance),
			spanemuboost.WithDatabaseID(sysVars.Connection.Database),
			spanemuboost.WithContainerImage(cmp.Or(opts.EmulatorImage, spanemuboost.DefaultEmulatorImage)),
			spanemuboost.WithDatabaseDialect(sysVars.Feature.DatabaseDialect),
		}

		// In detached mode, only create instance without database
		if opts.Detached {
			emulatorOpts = append(emulatorOpts, spanemuboost.EnableInstanceAutoConfigOnly())
		}

		// Add platform customizer if specified
		if opts.EmulatorPlatform != "" {
			emulatorOpts = append(emulatorOpts, spanemuboost.WithContainerCustomizers(withPlatform(opts.EmulatorPlatform)))
		}

		// Load sample database if specified
		if opts.SampleDatabase != "" {
			var sample *SampleDatabase
			var err error

			// Check if it's a path to metadata file (.json, .yaml, .yml)
			if metadataFileRegex.MatchString(opts.SampleDatabase) {
				// Load from metadata file
				sample, err = loadSampleFromMetadata(opts.SampleDatabase)
				if err != nil {
					return fmt.Errorf("failed to load sample metadata: %w", err)
				}
			} else {
				// Look for built-in sample by name
				samples := discoverBuiltinSamples()
				if s, ok := samples[opts.SampleDatabase]; ok {
					sample = &s
				} else {
					return fmt.Errorf("sample database not found: %s", opts.SampleDatabase)
				}
			}

			// Use the sample's dialect
			emulatorOpts = append(emulatorOpts, spanemuboost.WithDatabaseDialect(sample.ParsedDialect))

			// Prepare URIs for batch loading
			var uris []string
			if sample.SchemaURI != "" {
				uris = append(uris, sample.SchemaURI)
			}
			if sample.DataURI != "" {
				uris = append(uris, sample.DataURI)
			}

			// Batch load all URIs efficiently (handles mixed sources)
			results, err := loadMultipleFromURIs(ctx, uris)
			if err != nil {
				return fmt.Errorf("failed to load sample database %q: %w", opts.SampleDatabase, err)
			}

			// Helper function to parse and add statements to emulator options
			addStatements := func(uri string, setupFunc func([]string) spanemuboost.Option, stmtType string) error {
				if content, ok := results[uri]; ok && len(content) > 0 {
					statements, err := ParseStatements(content, uri)
					if err != nil {
						return fmt.Errorf("failed to parse %s statements: %w", stmtType, err)
					}
					emulatorOpts = append(emulatorOpts, setupFunc(statements))
				}
				return nil
			}

			// Parse and add DDL statements
			if err := addStatements(sample.SchemaURI, spanemuboost.WithSetupDDLs, "schema"); err != nil {
				return err
			}

			// Parse and add DML statements
			if err := addStatements(sample.DataURI, spanemuboost.WithSetupRawDMLs, "data"); err != nil {
				return err
			}
		}

		embeddedRuntime, err := spanemuboost.Run(ctx, spanemuboost.BackendEmulator, emulatorOpts...)
		if err != nil {
			return fmt.Errorf("failed to start Cloud Spanner Emulator: %w", err)
		}
		defer func() {
			if err := embeddedRuntime.Close(); err != nil {
				slog.Warn("failed to close emulator container", "error", err)
			}
		}()
		sysVars.Connection.Project = embeddedRuntime.ProjectID()
		sysVars.Connection.Instance = embeddedRuntime.InstanceID()
		sysVars.Connection.Database = embeddedRuntime.DatabaseID()

		// Always detect the actual platform the container is running on
		// The --emulator-platform flag only controls what platform is requested,
		// but we want to show the actual platform in CLI_EMULATOR_PLATFORM
		runtimePlatform, err := spanemuboost.RuntimePlatform(ctx, embeddedRuntime)
		if err != nil {
			slog.Warn("failed to detect embedded runtime platform", "error", err)
			sysVars.Connection.EmulatorPlatform = "unknown"
		} else {
			sysVars.Connection.EmulatorPlatform = runtimePlatform
		}
		slog.Debug("Detected container platform",
			"requested", opts.EmulatorPlatform,
			"actual", sysVars.Connection.EmulatorPlatform)

		// Parse container URI into host and port
		host, port, err := parseEndpoint(embeddedRuntime.URI())
		if err != nil {
			// This should not happen with a valid URI from testcontainers, but handle defensively.
			return fmt.Errorf("failed to parse emulator endpoint URI %q: %w", embeddedRuntime.URI(), err)
		}
		sysVars.Connection.Host, sysVars.Connection.Port = host, port
		sysVars.Connection.WithoutAuthentication = true
	}

	// TTY detection has been moved to StreamManager.

	// Setup output streams
	var errStream io.Writer = os.Stderr // Error stream is not affected by --tee

	// Determine the original output stream
	// Always use os.Stdout as the original output for actual data
	var originalOut io.Writer = os.Stdout

	// Create StreamManager for managing all input/output streams
	streamManager := streamio.NewStreamManager(os.Stdin, originalOut, errStream)
	// StreamManager will automatically detect if os.Stdout is a TTY
	if term.IsTerminal(int(os.Stdout.Fd())) {
		streamManager.SetTtyStream(os.Stdout)
	}
	sysVars.StreamManager = streamManager
	defer streamManager.Close()

	// Handle output redirection options
	if opts.Tee != "" && opts.Output != "" {
		return errors.New("cannot use both --tee and --output flags simultaneously")
	}

	// If --tee is specified, enable tee output (normal mode: both screen and file)
	if opts.Tee != "" {
		if err := streamManager.EnableTee(opts.Tee, false); err != nil {
			return err
		}
	}

	// If --output is specified, enable output redirect (silent mode: file only)
	if opts.Output != "" {
		if err := streamManager.EnableTee(opts.Output, true); err != nil {
			return err
		}
	}

	cli, err := NewCli(ctx, cred, sysVars)
	if err != nil {
		return fmt.Errorf("failed to connect to Spanner: %w", err)
	}

	if opts.MCP {
		return cli.RunMCP(ctx)
	}

	input, interactive, err := determineInputAndMode(opts, os.Stdin)
	if err != nil {
		return err
	}

	switch {
	case interactive:
		sysVars.Display.EnableProgressBar = true
		return cli.RunInteractive(ctx)
	default:
		return cli.RunBatch(ctx, input)
	}
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
