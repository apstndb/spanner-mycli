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
	"github.com/testcontainers/testcontainers-go"
	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"

	"github.com/cloudspannerecosystem/memefish"

	"github.com/cloudspannerecosystem/memefish/ast"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/jessevdk/go-flags"
	"github.com/samber/lo"
	"golang.org/x/term"
	"spheric.cloud/xiter"
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
	Tee             string  `long:"tee" description:"Append a copy of output to the specified file" default-mask:"-"`
	SkipColumnNames bool    `long:"skip-column-names" description:"Suppress column headers in output" default-mask:"-"`
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

// withPlatform creates a ContainerCustomizer that sets the container platform
func withPlatform(platform string) testcontainers.ContainerCustomizer {
	return testcontainers.CustomizeRequest(
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				ImagePlatform: platform,
			},
		},
	)
}

// detectContainerPlatform detects the platform of a running container
// It tries multiple methods in order of preference:
// 1. ImageManifestDescriptor.Platform (most accurate but not always available)
// 2. Docker API image inspect (reliable when Docker socket is accessible)
// 3. Basic Platform field from container (usually just OS, e.g., "linux")
// Returns "unknown" if platform detection fails - this is intentional as the function
// must return a string value for the system variable, and "unknown" accurately
// represents that the platform could not be determined.
func detectContainerPlatform(ctx context.Context, container *tcspanner.Container) string {
	inspectResult, err := container.Inspect(ctx)
	if err != nil {
		slog.Warn("Failed to inspect container", "error", err)
		// Return "unknown" rather than empty string to indicate detection attempted but failed
		return "unknown"
	}

	// Debug log the inspect result
	slog.Debug("Container inspect result",
		"Platform", inspectResult.Platform,
		"ImageManifestDescriptor", inspectResult.ImageManifestDescriptor != nil,
		"Image", inspectResult.Image,
		"ConfigNotNil", inspectResult.Config != nil,
	)

	// Log config details if available
	if inspectResult.Config != nil {
		slog.Debug("Container config details",
			"Image", inspectResult.Config.Image,
			"Labels", inspectResult.Config.Labels,
		)
	}

	// Method 1: Try ImageManifestDescriptor (OCI standard)
	if inspectResult.ImageManifestDescriptor != nil && inspectResult.ImageManifestDescriptor.Platform != nil {
		p := inspectResult.ImageManifestDescriptor.Platform
		slog.Debug("ImageManifestDescriptor.Platform details",
			"OS", p.OS,
			"Architecture", p.Architecture,
			"Variant", p.Variant,
			"OSVersion", p.OSVersion,
			"OSFeatures", p.OSFeatures,
		)
		platform := p.OS + "/" + p.Architecture
		if p.Variant != "" {
			platform += "/" + p.Variant
		}
		return platform
	}

	// Method 2: Try Docker API image inspect
	slog.Debug("ImageManifestDescriptor not available, trying image inspect")
	if inspectResult.Config != nil && inspectResult.Config.Image != "" {
		// Get provider for image inspection.
		// IMPORTANT: GetProvider() creates a NEW provider instance each time, not a singleton.
		// We must close it to avoid resource leaks.
		// See: https://github.com/testcontainers/testcontainers-go/blob/main/provider.go
		genericProvider, err := testcontainers.ProviderDefault.GetProvider()
		if err != nil {
			slog.Debug("Failed to get testcontainers provider", "error", err)
		} else {
			defer func() {
				if err := genericProvider.Close(); err != nil {
					slog.Debug("Failed to close testcontainers provider", "error", err)
				}
			}()

			platform := inspectImagePlatform(ctx, inspectResult.Config.Image, genericProvider)
			if platform != "" {
				return platform
			}
		}
	}

	// Method 3: Fallback to basic platform field
	if inspectResult.Platform != "" {
		slog.Debug("Using basic Platform field", "Platform", inspectResult.Platform)
		return inspectResult.Platform
	}

	// All detection methods failed - return "unknown" to indicate this state
	return "unknown"
}

// inspectImagePlatform uses container runtime API (Docker/Podman) to get platform information from an image.
// The provider parameter should be obtained from testcontainers.ProviderDefault.GetProvider().
// The caller is responsible for managing the provider lifecycle.
func inspectImagePlatform(ctx context.Context, imageName string, provider testcontainers.GenericProvider) string {
	slog.Debug("inspectImagePlatform called", "imageName", imageName)

	// Both Docker and Podman providers return *DockerProvider type, even for Podman.
	// This is because Podman provides Docker-compatible API.
	dockerProvider, ok := provider.(*testcontainers.DockerProvider)
	if !ok {
		slog.Debug("Provider is not a DockerProvider", "type", fmt.Sprintf("%T", provider))
		return ""
	}

	// The provider embeds *client.Client directly (not as a field), so we can access it
	// using provider.Client() which returns the embedded Docker/Podman client.
	dockerClient := dockerProvider.Client()
	slog.Debug("Using container runtime client from testcontainers provider")

	// Use a context with a timeout for Docker API calls to prevent hangs
	// if the container runtime is unresponsive
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Log Docker client info
	info, err := dockerClient.Info(inspectCtx)
	if err != nil {
		slog.Debug("Failed to get Docker info", "error", err)
	} else {
		slog.Debug("Docker client info",
			"ServerVersion", info.ServerVersion,
			"OSType", info.OSType,
			"Architecture", info.Architecture)
	}

	imageInspect, err := dockerClient.ImageInspect(inspectCtx, imageName)
	if err != nil {
		slog.Debug("Image inspect failed",
			"error", err,
			"imageName", imageName)
		return ""
	}

	slog.Debug("Image inspect successful",
		"imageName", imageName,
		"Architecture", imageInspect.Architecture,
		"Os", imageInspect.Os,
		"Variant", imageInspect.Variant,
		"ID", imageInspect.ID,
	)

	// Validate that we have the required fields
	if imageInspect.Os == "" || imageInspect.Architecture == "" {
		slog.Debug("Image inspect result missing Os or Architecture",
			"imageName", imageName,
			"Os", imageInspect.Os,
			"Architecture", imageInspect.Architecture)
		return ""
	}

	platform := imageInspect.Os + "/" + imageInspect.Architecture
	if imageInspect.Variant != "" {
		platform += "/" + imageInspect.Variant
	}
	slog.Debug("Returning platform", "platform", platform)
	return platform
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
			spanemuboost.WithProjectID(sysVars.Project),
			spanemuboost.WithInstanceID(sysVars.Instance),
			spanemuboost.WithDatabaseID(sysVars.Database),
			spanemuboost.WithEmulatorImage(cmp.Or(opts.EmulatorImage, spanemuboost.DefaultEmulatorImage)),
			spanemuboost.WithDatabaseDialect(sysVars.DatabaseDialect),
		}

		// In detached mode, only create instance without database
		if opts.Detached {
			emulatorOpts = append(emulatorOpts, spanemuboost.EnableInstanceAutoConfigOnly())
		}

		// Add platform customizer if specified
		if opts.EmulatorPlatform != "" {
			emulatorOpts = append(emulatorOpts, spanemuboost.WithContainerCustomizers(withPlatform(opts.EmulatorPlatform)))
		}

		container, teardown, err := spanemuboost.NewEmulator(ctx, emulatorOpts...)
		if err != nil {
			return fmt.Errorf("failed to start Cloud Spanner Emulator: %w", err)
		}
		defer teardown()

		// Always detect the actual platform the container is running on
		// The --emulator-platform flag only controls what platform is requested,
		// but we want to show the actual platform in CLI_EMULATOR_PLATFORM
		sysVars.EmulatorPlatform = detectContainerPlatform(ctx, container)
		slog.Debug("Detected container platform",
			"requested", opts.EmulatorPlatform,
			"actual", sysVars.EmulatorPlatform)

		// Parse container URI into host and port
		host, port, err := parseEndpoint(container.URI())
		if err != nil {
			// This should not happen with a valid URI from testcontainers, but handle defensively.
			return fmt.Errorf("failed to parse emulator endpoint URI %q: %w", container.URI(), err)
		}
		sysVars.Host, sysVars.Port = host, port
		sysVars.WithoutAuthentication = true
	}

	// TTY detection has been moved to StreamManager.

	// Setup output streams
	var errStream io.Writer = os.Stderr // Error stream is not affected by --tee

	// Determine the original output stream
	// Always use os.Stdout as the original output for actual data
	var originalOut io.Writer = os.Stdout

	// Create StreamManager for managing all input/output streams
	streamManager := NewStreamManager(os.Stdin, originalOut, errStream)
	// StreamManager will automatically detect if os.Stdout is a TTY
	if term.IsTerminal(int(os.Stdout.Fd())) {
		streamManager.SetTtyStream(os.Stdout)
	}
	sysVars.StreamManager = streamManager
	defer streamManager.Close()

	// If --tee is specified, enable tee output
	if opts.Tee != "" {
		if err := streamManager.EnableTee(opts.Tee); err != nil {
			return err
		}
	}

	cli, err := NewCli(ctx, cred, &sysVars)
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

	// Set default CLI_FORMAT for interactive/batch mode if not already set
	if sysVars.CLIFormat == 0 {
		// CLI_FORMAT was not set by flags or --set, apply defaults based on mode
		if interactive {
			if err := sysVars.SetFromSimple("CLI_FORMAT", "TABLE"); err != nil {
				return fmt.Errorf("failed to set default CLI_FORMAT: %w", err)
			}
		} else {
			if err := sysVars.SetFromSimple("CLI_FORMAT", "TAB"); err != nil {
				return fmt.Errorf("failed to set default CLI_FORMAT: %w", err)
			}
		}
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
		if _, err := parseDisplayMode(opts.Format); err != nil {
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
	// Don't initialize registry here - it needs to be done after the final
	// systemVariables is in its permanent location to avoid closure issues

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

	// Handle alias flags with precedence for non-hidden flags
	// Note: This precedence may override normal flag/env/ini precedence
	if opts.Role != "" && opts.DatabaseRole != "" {
		slog.Warn("Both --role and --database-role are specified. Using --role (alias has lower precedence)")
	}
	sysVars.Role = cmp.Or(opts.Role, opts.DatabaseRole)

	// Handle --insecure/--skip-tls-verify aliases with proper precedence
	if opts.Insecure != nil && opts.SkipTlsVerify != nil {
		slog.Warn("Both --insecure and --skip-tls-verify are specified. Using --insecure (alias has lower precedence)")
	}

	// Implement precedence: non-hidden flag (--insecure) takes precedence when both are set
	if opts.Insecure != nil {
		sysVars.Insecure = *opts.Insecure
	} else if opts.SkipTlsVerify != nil {
		sysVars.Insecure = *opts.SkipTlsVerify
	} else {
		sysVars.Insecure = false // default value
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
			return systemVariables{}, fmt.Errorf("failed to parse endpoint %q: %w", endpoint, err)
		}
		sysVars.Host, sysVars.Port = host, port
	} else if opts.Host != "" || opts.Port != 0 {
		// Handle --host and --port flags
		if opts.Port != 0 && opts.Host == "" {
			// Only port specified: use localhost for emulator
			sysVars.Host = "localhost"
			sysVars.Port = opts.Port
		} else if opts.Host != "" && opts.Port == 0 {
			// Only host specified: use standard Spanner port
			sysVars.Host = opts.Host
			sysVars.Port = 443
		} else {
			// Both specified
			sysVars.Host = opts.Host
			sysVars.Port = opts.Port
		}
	}
	sysVars.Params = params
	sysVars.LogGrpc = opts.LogGrpc
	sysVars.LogLevel = l
	sysVars.ImpersonateServiceAccount = opts.ImpersonateServiceAccount
	sysVars.VertexAIProject = opts.VertexAIProject
	sysVars.AsyncDDL = opts.Async

	// Handle system command options
	// Priority: --skip-system-command takes precedence over --system-command
	if opts.SkipSystemCommand {
		sysVars.SkipSystemCommand = true
	} else if opts.SystemCommand != nil {
		// --system-command=OFF disables system commands
		sysVars.SkipSystemCommand = *opts.SystemCommand == "OFF"
	}
	// If neither flag is set, system commands are enabled by default (SkipSystemCommand = false)

	sysVars.SkipColumnNames = opts.SkipColumnNames

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

	// Initialize registry after sysVars is in its final location
	sysVars.initializeRegistry()

	// initialize default value
	if err := sysVars.SetFromSimple("CLI_ANALYZE_COLUMNS", DefaultAnalyzeColumns); err != nil {
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
		if err := sysVars.SetFromSimple("CLI_DATABASE_DIALECT", opts.DatabaseDialect); err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --database-dialect: %v: %w", opts.DatabaseDialect, err)
		}
	}

	if opts.EnablePartitionedDML {
		if err := sysVars.SetFromSimple("AUTOCOMMIT_DML_MODE", "PARTITIONED_NON_ATOMIC"); err != nil {
			return systemVariables{}, fmt.Errorf("unknown error on --enable-partitioned-dml: %w", err)
		}
	}

	if opts.Timeout != "" {
		// Validate timeout format before setting system variable for better error reporting
		_, err := time.ParseDuration(opts.Timeout)
		if err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --timeout: %v: %w", opts.Timeout, err)
		}
		if err := sysVars.SetFromSimple("STATEMENT_TIMEOUT", opts.Timeout); err != nil {
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
		if err := sysVars.SetFromSimple("RPC_PRIORITY", opts.Priority); err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --priority: %v: %w", opts.Priority, err)
		}
	} else {
		// Only set default if not already set via registry
		if err := sysVars.SetFromSimple("RPC_PRIORITY", "MEDIUM"); err != nil {
			return systemVariables{}, fmt.Errorf("failed to set default RPC_PRIORITY: %w", err)
		}
	}

	if opts.QueryMode != "" {
		if err := sysVars.SetFromSimple("CLI_QUERY_MODE", opts.QueryMode); err != nil {
			return systemVariables{}, fmt.Errorf("invalid value of --query-mode: %v: %w", opts.QueryMode, err)
		}
	}

	if opts.TryPartitionQuery {
		if err := sysVars.SetFromSimple("CLI_TRY_PARTITION_QUERY", "TRUE"); err != nil {
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

	// Set CLI_FORMAT defaults based on flags before processing --set
	// This allows --set CLI_FORMAT=X to override these defaults
	sets := maps.Collect(xiter.MapKeys(maps.All(opts.Set), strings.ToUpper))
	// Debug: log what we have
	if len(sets) > 0 {
		slog.Debug("Processing --set values", "sets", sets)
	}
	if _, ok := sets["CLI_FORMAT"]; !ok {
		// Individual format flags take precedence over --format for backward compatibility
		switch {
		case opts.HTML:
			if err := sysVars.SetFromSimple("CLI_FORMAT", "HTML"); err != nil {
				return systemVariables{}, fmt.Errorf("failed to set CLI_FORMAT: %w", err)
			}
		case opts.XML:
			if err := sysVars.SetFromSimple("CLI_FORMAT", "XML"); err != nil {
				return systemVariables{}, fmt.Errorf("failed to set CLI_FORMAT: %w", err)
			}
		case opts.CSV:
			if err := sysVars.SetFromSimple("CLI_FORMAT", "CSV"); err != nil {
				return systemVariables{}, fmt.Errorf("failed to set CLI_FORMAT: %w", err)
			}
		case opts.Table:
			if err := sysVars.SetFromSimple("CLI_FORMAT", "TABLE"); err != nil {
				return systemVariables{}, fmt.Errorf("failed to set CLI_FORMAT: %w", err)
			}
		case opts.Format != "":
			if err := sysVars.SetFromSimple("CLI_FORMAT", opts.Format); err != nil {
				return systemVariables{}, fmt.Errorf("invalid value of --format: %v: %w", opts.Format, err)
			}
		}
		// Note: Interactive mode defaults are handled in run() since we don't know
		// if we're interactive at this point
	}
	for k, v := range sets {
		slog.Debug("Setting variable from --set", "name", k, "value", v)
		if err := sysVars.SetFromSimple(k, v); err != nil {
			return systemVariables{}, fmt.Errorf("failed to set system variable. name: %v, value: %v: %w", k, v, err)
		}
	}

	if opts.EmbeddedEmulator {
		// When using embedded emulator, insecure connection is required
		sysVars.Insecure = true
		// sysVars.Endpoint and sysVars.WithoutAuthentication will be set in run() after emulator starts

		// Set default values for embedded emulator if not specified by user
		if sysVars.Project == "" {
			sysVars.Project = spanemuboost.DefaultProjectID
		}
		if sysVars.Instance == "" {
			sysVars.Instance = spanemuboost.DefaultInstanceID
		}
		// For database, respect --detached mode (empty database)
		if sysVars.Database == "" && !opts.Detached {
			sysVars.Database = spanemuboost.DefaultDatabaseID
		}
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
