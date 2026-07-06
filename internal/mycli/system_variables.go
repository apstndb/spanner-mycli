// System Variables Naming Convention:
// - Variables that correspond to java-spanner JDBC properties use the same names
// - Variables that are spanner-mycli specific MUST use CLI_ prefix
// - This ensures compatibility with existing JDBC tooling while clearly identifying custom extensions
package mycli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/filesafety"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	planref "github.com/apstndb/spannerplan/plantree/reference"
	"github.com/bufbuild/protocompile"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/option"

	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

type LastQueryCache struct {
	QueryPlan       *sppb.QueryPlan
	QueryStats      map[string]any
	ReadTimestamp   time.Time
	CommitTimestamp time.Time
}

// StartupConfig holds configuration that is immutable after startup.
// It is populated by parseFlags/createSystemVariablesFromOptions (and, for the
// embedded runtime, by setup code in app.go before any session is created) and
// must never be written afterwards: every corresponding system variable is
// registered read-only, and USE/DETACH do not touch it.
// Exception: EnableADCPlus is session-init-only; its custom setter rejects
// writes once a session exists (see var_registry.go).
type StartupConfig struct {
	Host                      string // CLI_HOST
	Port                      int    // CLI_PORT
	Insecure                  bool   // CLI_INSECURE
	WithoutAuthentication     bool
	ImpersonateServiceAccount string // CLI_IMPERSONATE_SERVICE_ACCOUNT
	EnableADCPlus             bool   // CLI_ENABLE_ADC_PLUS
	EmulatorPlatform          string // CLI_EMULATOR_PLATFORM
	LogGrpc                   bool   // CLI_LOG_GRPC
	MCP                       bool   // CLI_MCP
	SkipSystemCommand         bool   // CLI_SKIP_SYSTEM_COMMAND

	// Embedded runtime overrides used by tests and the embedded emulator.
	EmbeddedClientOptions []option.ClientOption
	EmbeddedClientConfig  *spanner.ClientConfig
}

// ConnectionVars holds the connection identity. Project and Instance are fixed
// at startup; Database and Role are mutated in place only by USE/DETACH
// (SessionHandler.switchSession).
type ConnectionVars struct {
	Project  string // CLI_PROJECT
	Instance string // CLI_INSTANCE
	Database string // CLI_DATABASE
	Role     string // CLI_ROLE
}

// LastResult holds outputs of the most recently executed statement or
// transaction. It is written by statement execution and read back through
// read-only system variables (READ_TIMESTAMP, COMMIT_TIMESTAMP,
// COMMIT_RESPONSE) and last-query features such as CLI_INLINE_STATS.
// Unlike the *Vars groups, nothing here is user-settable.
type LastResult struct {
	ReadTimestamp   time.Time            // READ_TIMESTAMP
	CommitTimestamp time.Time            // COMMIT_TIMESTAMP
	CommitResponse  *sppb.CommitResponse // COMMIT_RESPONSE
	QueryCache      *LastQueryCache
}

// DisplayVars holds display and output formatting configuration.
type DisplayVars struct {
	CLIFormat                  enums.DisplayMode   // CLI_FORMAT
	Verbose                    bool                // CLI_VERBOSE
	Prompt                     string              // CLI_PROMPT
	Prompt2                    string              // CLI_PROMPT2
	HistoryFile                string              // CLI_HISTORY_FILE
	TabWidth                   int64               // CLI_TAB_WIDTH
	TabVisualize               bool                // CLI_TAB_VISUALIZE
	EnableHighlight            bool                // CLI_ENABLE_HIGHLIGHT
	UsePager                   bool                // CLI_USE_PAGER
	AutoWrap                   bool                // CLI_AUTOWRAP
	FixedWidth                 *int64              // CLI_FIXED_WIDTH
	MultilineProtoText         bool                // CLI_PROTOTEXT_MULTILINE
	MarkdownCodeblock          bool                // CLI_MARKDOWN_CODEBLOCK
	SkipColumnNames            bool                // CLI_SKIP_COLUMN_NAMES
	SuppressResultLines        bool                // CLI_SUPPRESS_RESULT_LINES
	ExplainFormat              enums.ExplainFormat // CLI_EXPLAIN_FORMAT
	ExplainWrapWidth           int64               // CLI_EXPLAIN_WRAP_WIDTH
	ExplainHangingIndent       bool                // CLI_EXPLAIN_HANGING_INDENT
	ExplainPrintSections       string              // CLI_EXPLAIN_PRINT_SECTIONS
	ParsedExplainPrintSections planref.PrintSections
	OutputTemplateFile         string // CLI_OUTPUT_TEMPLATE_FILE (computed getter/setter)
	OutputTemplate             *template.Template
	AnalyzeColumns             string // CLI_ANALYZE_COLUMNS
	ParsedAnalyzeColumns       []columnRenderDef
	InlineStats                string // CLI_INLINE_STATS
	ParsedInlineStats          []inlineStatsDef
	SQLTableName               string              // CLI_SQL_TABLE_NAME
	SQLBatchSize               int64               // CLI_SQL_BATCH_SIZE
	EnableProgressBar          bool                // CLI_ENABLE_PROGRESS_BAR
	StyledOutput               enums.StyledMode    // CLI_STYLED_OUTPUT
	WidthStrategy              enums.WidthStrategy // CLI_WIDTH_STRATEGY
	TypeStylesRaw              string              // CLI_TYPE_STYLES (raw string, parsed into systemVariables.typeStyles/nullStyle)
}

// QueryVars holds query execution configuration.
type QueryVars struct {
	StatementTimeout           *time.Duration                    // STATEMENT_TIMEOUT
	RPCPriority                sppb.RequestOptions_Priority      // RPC_PRIORITY
	ReadOnlyStaleness          *spanner.TimestampBound           // READ_ONLY_STALENESS
	OptimizerVersion           string                            // OPTIMIZER_VERSION
	OptimizerStatisticsPackage string                            // OPTIMIZER_STATISTICS_PACKAGE
	AutoPartitionMode          bool                              // AUTO_PARTITION_MODE
	DataBoostEnabled           bool                              // DATA_BOOST_ENABLED
	MaxPartitionedParallelism  int64                             // MAX_PARTITIONED_PARALLELISM
	QueryMode                  *sppb.ExecuteSqlRequest_QueryMode // CLI_QUERY_MODE
	TryPartitionQuery          bool                              // CLI_TRY_PARTITION_QUERY
	DirectedRead               *sppb.DirectedReadOptions         // CLI_DIRECT_READ
	StreamingMode              enums.StreamingMode               // CLI_TABLE_STREAMING
	TablePreviewRows           int64                             // CLI_TABLE_PREVIEW_ROWS
	BuildStatementMode         enums.ParseMode                   // CLI_PARSE_MODE
	Profile                    bool                              // CLI_PROFILE
	LintPlan                   bool                              // CLI_LINT_PLAN
}

// TransactionVars holds transaction-related configuration.
type TransactionVars struct {
	TransactionTag              string                                         // TRANSACTION_TAG
	RequestTag                  string                                         // STATEMENT_TAG
	ReadOnly                    bool                                           // READONLY
	ExcludeTxnFromChangeStreams bool                                           // EXCLUDE_TXN_FROM_CHANGE_STREAMS
	MaxCommitDelay              *time.Duration                                 // MAX_COMMIT_DELAY
	AutoBatchDML                bool                                           // AUTO_BATCH_DML
	AutocommitDMLMode           enums.AutocommitDMLMode                        // AUTOCOMMIT_DML_MODE
	ReturnCommitStats           bool                                           // RETURN_COMMIT_STATS
	DefaultIsolationLevel       sppb.TransactionOptions_IsolationLevel         // DEFAULT_ISOLATION_LEVEL
	ReadLockMode                sppb.TransactionOptions_ReadWrite_ReadLockMode // READ_LOCK_MODE

	// Unimplemented variables (kept for compatibility)
	Autocommit            bool // AUTOCOMMIT (unimplemented)
	RetryAbortsInternally bool // RETRY_ABORTS_INTERNALLY (unimplemented)
}

// FeatureVars holds feature flags and experimental configuration.
type FeatureVars struct {
	FuzzyFinderKey         string                     // CLI_FUZZY_FINDER_KEY (empty = disabled)
	FuzzyFinderOptions     string                     // CLI_FUZZY_FINDER_OPTIONS
	MCP                    bool                       // CLI_MCP
	VertexAIProject        string                     // CLI_VERTEXAI_PROJECT
	VertexAIModel          string                     // CLI_VERTEXAI_MODEL
	VertexAILocation       string                     // CLI_VERTEXAI_LOCATION
	BigQueryProject        string                     // CLI_BIGQUERY_PROJECT (defaults to CLI_PROJECT when empty)
	BigQueryLocation       string                     // CLI_BIGQUERY_LOCATION
	BigQueryMaxBytesBilled *int64                     // CLI_BIGQUERY_MAX_BYTES_BILLED
	EchoExecutedDDL        bool                       // CLI_ECHO_EXECUTED_DDL
	EchoInput              bool                       // CLI_ECHO_INPUT
	AsyncDDL               bool                       // CLI_ASYNC_DDL
	AutoConnectAfterCreate bool                       // CLI_AUTO_CONNECT_AFTER_CREATE
	LogLevel               slog.Level                 // CLI_LOG_LEVEL
	DatabaseDialect        databasepb.DatabaseDialect // CLI_DATABASE_DIALECT
}

// InternalVars holds internal state not directly exposed as system variables.
type InternalVars struct {
	ProtoDescriptorFile []string // CLI_PROTO_DESCRIPTOR_FILE
	ProtoDescriptor     *descriptorpb.FileDescriptorSet
}

// systemVariables holds the session state of spanner-mycli, layered by
// ownership and mutability:
//
//   - Config: immutable after startup (see StartupConfig)
//   - Connection: connection identity; Database/Role mutated only by USE/DETACH
//   - LastResult: per-statement outputs, written by statement execution
//   - Display/Query/Transaction/Feature/Internal: the SET-able surface,
//     mutated through the Registry (and transaction-scoped via SET LOCAL)
//
// There is exactly one live instance per process: the Registry captures
// pointers into this struct, so it must never be copied (USE/DETACH mutate it
// in place; see SessionHandler.switchSession).
type systemVariables struct {
	Config      StartupConfig
	Connection  ConnectionVars
	LastResult  LastResult
	Display     DisplayVars
	Query       QueryVars
	Transaction TransactionVars
	Feature     FeatureVars
	Internal    InternalVars

	// Params is intentionally top-level, not inside QueryVars.
	// Unlike grouped fields which use VarHandler[T]/Registry, Params is a dynamic map
	// managed via dedicated SET/UNSET PARAM statements (statements_params.go).
	Params map[string]ast.Node

	// inTransaction reports whether there is an active transaction.
	// nil means no session has been created yet.
	inTransaction func() bool

	// StreamManager manages tee output functionality
	StreamManager *streamio.StreamManager

	// Registry holds the system variable registry
	Registry *VarRegistry

	// typeStyles maps Spanner type codes to ANSI SGR sequences for styled output.
	// When nil or empty, all non-NULL values use PlainCell (default behavior).
	// Populated by parsing CLI_TYPE_STYLES system variable.
	typeStyles map[sppb.TypeCode]string

	// nullStyle is the ANSI SGR sequence for NULL values.
	// Empty string means no styling. Populated by the NULL key in CLI_TYPE_STYLES.
	nullStyle string
}

// toFormatConfig converts the formatter-relevant fields of systemVariables into a format.FormatConfig.
func (sv *systemVariables) toFormatConfig() format.FormatConfig {
	var styled bool
	switch sv.Display.StyledOutput {
	case enums.StyledModeTrue:
		styled = true
	case enums.StyledModeFalse:
		styled = false
	default: // StyledModeAuto
		styled = sv.StreamManager != nil && sv.StreamManager.IsTerminal()
	}

	return format.FormatConfig{
		TabWidth:        int(sv.Display.TabWidth),
		Verbose:         sv.Display.Verbose,
		SkipColumnNames: sv.Display.SkipColumnNames,
		PreviewRows:     sv.Query.TablePreviewRows,
		Styled:          styled,
		WidthStrategy:   sv.Display.WidthStrategy,
		TabVisualize:    sv.Display.TabVisualize,
	}
}

// parseEndpoint parses an endpoint string into host and port components.
// It returns an error if the endpoint is invalid.
func parseEndpoint(endpoint string) (host string, port int, err error) {
	if endpoint == "" {
		return "", 0, nil
	}

	h, pStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		// net.SplitHostPort returns an error for inputs that do not contain a port,
		// which is the desired behavior for an endpoint string that is expected to have one.
		// This also correctly handles bare IPv6 addresses by failing to parse them as host-port pairs.
		return "", 0, fmt.Errorf("invalid endpoint format (expected host:port): %w", err)
	}

	p, err := strconv.Atoi(pStr)
	if err != nil {
		// The port is not a valid number.
		return "", 0, fmt.Errorf("invalid port in endpoint: %q", pStr)
	}

	// For IPv6, net.SplitHostPort keeps brackets, which we need to remove
	// for consistent storage and for net.JoinHostPort to work correctly.
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = h[1 : len(h)-1]
	}

	return h, p, nil
}

var errIgnored = errors.New("ignored")

func projectPath(projectID string) string {
	return fmt.Sprintf("projects/%v", projectID)
}

func instancePath(projectID, instanceID string) string {
	return fmt.Sprintf("projects/%v/instances/%v", projectID, instanceID)
}

func databasePath(projectID, instanceID, databaseID string) string {
	return fmt.Sprintf("projects/%v/instances/%v/databases/%v", projectID, instanceID, databaseID)
}

func (sv *systemVariables) InstancePath() string {
	return instancePath(sv.Connection.Project, sv.Connection.Instance)
}

func (sv *systemVariables) DatabasePath() string {
	return databasePath(sv.Connection.Project, sv.Connection.Instance, sv.Connection.Database)
}

func (sv *systemVariables) ProjectPath() string {
	return projectPath(sv.Connection.Project)
}

// newSystemVariablesWithDefaults creates a new systemVariables instance with default values.
// This function ensures consistency between initialization and test expectations.
func newSystemVariablesWithDefaults() systemVariables {
	sv := systemVariables{
		Config: StartupConfig{
			EnableADCPlus: true,
		},
		Display: DisplayVars{
			CLIFormat:                  enums.DisplayModeTable, // Default to TABLE format
			AnalyzeColumns:             DefaultAnalyzeColumns,
			ParsedAnalyzeColumns:       DefaultParsedAnalyzeColumns,
			ExplainHangingIndent:       true,
			ExplainPrintSections:       DefaultExplainPrintSections,
			ParsedExplainPrintSections: DefaultParsedExplainPrintSections,
			Prompt:                     defaultPrompt,
			Prompt2:                    defaultPrompt2,
			HistoryFile:                defaultHistoryFile(),
			OutputTemplate:             defaultOutputFormat,
			TypeStylesRaw:              defaultTypeStyles,
		},
		Query: QueryVars{
			RPCPriority:      defaultPriority,
			StreamingMode:    enums.StreamingModeAuto, // Default to automatic selection based on format
			TablePreviewRows: 50,                      // Default to 50 rows - enough to fit on one screen while prioritizing proper table formatting
		},
		Transaction: TransactionVars{
			ReturnCommitStats: true,
		},
		Feature: FeatureVars{
			VertexAIModel:    defaultVertexAIModel,
			VertexAILocation: defaultVertexAILocation,
			LogLevel:         slog.LevelWarn,
			FuzzyFinderKey:   "C_T",
		},

		// Initialize empty maps to avoid nil
		Params: make(map[string]ast.Node),
	}

	// Parse the default type styles to populate typeStyles/nullStyle
	if config, err := parseTypeStyles(sv.Display.TypeStylesRaw); err == nil {
		sv.typeStyles = config.typeStyles
		sv.nullStyle = config.nullStyle
	}

	// Don't initialize registry here - it will be done after the struct is assigned
	// to avoid closures capturing a pointer to the local copy
	return sv
}

// newSystemVariablesWithDefaultsForTest creates a new systemVariables instance
// with defaults for testing.
func newSystemVariablesWithDefaultsForTest() *systemVariables {
	sv := newSystemVariablesWithDefaults()
	return &sv
}

type errSetterUnimplemented struct {
	Name string
}

func (e errSetterUnimplemented) Error() string {
	return fmt.Sprintf("unimplemented setter: %v", e.Name)
}

type errGetterUnimplemented struct {
	Name string
}

func (e errGetterUnimplemented) Error() string {
	return fmt.Sprintf("unimplemented getter: %v", e.Name)
}

type errAdderUnimplemented struct {
	Name string
}

func (e errAdderUnimplemented) Error() string {
	return fmt.Sprintf("unimplemented adder: %v", e.Name)
}

var (
	_ error = &errSetterUnimplemented{}
	_ error = &errGetterUnimplemented{}
	_ error = &errAdderUnimplemented{}
)

// SetFromGoogleSQL sets a system variable using GoogleSQL parsing mode.
// This is used for SET statements in REPL and SQL scripts where values
// are parsed as GoogleSQL expressions (e.g., TRUE, 'string value').
func (sv *systemVariables) SetFromGoogleSQL(name string, value string) error {
	sv.ensureRegistry()
	return sv.setFrom(name, value, true)
}

// SetFromSimple sets a system variable using Simple parsing mode.
// This is used for command-line flags and config files where values
// don't follow GoogleSQL syntax rules.
func (sv *systemVariables) SetFromSimple(name string, value string) error {
	sv.ensureRegistry()
	return sv.setFrom(name, value, false)
}

func (sv *systemVariables) AddFromGoogleSQL(name string, value string) error {
	sv.ensureRegistry()
	return sv.addFrom(name, value, true)
}

func (sv *systemVariables) AddFromSimple(name string, value string) error {
	sv.ensureRegistry()
	return sv.addFrom(name, value, false)
}

func (sv *systemVariables) Get(name string) (map[string]string, error) {
	sv.ensureRegistry()
	return sv.get(name)
}

// ListVariables returns all variables with their current values
func (sv *systemVariables) ListVariables() map[string]string {
	sv.ensureRegistry()
	return sv.Registry.ListVariables()
}

// ListVariableInfo returns information about all variables
func (sv *systemVariables) ListVariableInfo() map[string]struct {
	Description   string
	ReadOnly      bool
	CanAdd        bool
	Unimplemented bool
} {
	sv.ensureRegistry()
	return sv.Registry.ListVariableInfo()
}

func unquoteString(s string) string {
	return strings.Trim(s, `"'`)
}

func singletonMap[K comparable, V any](k K, v V) map[K]V {
	return map[K]V{k: v}
}

// parseTimeString parses timestamp strings from spanner.TimestampBound.String() output.
// This is NOT for parsing user input - user input is handled by parseTimestampBound.
// The format matches time.Time.String() default format, which is what TimestampBound.String()
// uses internally for readTimestamp and minReadTimestamp modes.
func parseTimeString(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", s)
}

var defaultOutputFormat = template.Must(template.New("").Funcs(sproutFuncMap()).Parse(outputTemplateStr))

func mergeFDS(left, right *descriptorpb.FileDescriptorSet) *descriptorpb.FileDescriptorSet {
	result := slices.Clone(left.GetFile())
	for _, fd := range right.GetFile() {
		idx := slices.IndexFunc(result, func(descriptorProto *descriptorpb.FileDescriptorProto) bool {
			return descriptorProto.GetPackage() == fd.GetPackage() && descriptorProto.GetName() == fd.GetName()
		})
		if idx != -1 {
			result[idx] = fd
		} else {
			result = append(result, fd)
		}
	}
	return &descriptorpb.FileDescriptorSet{File: result}
}

var httpResolver = protocompile.ResolverFunc(httpResolveFunc)

var httpOrHTTPSRe = regexp.MustCompile("^https?://")

func httpResolveFunc(path string) (protocompile.SearchResult, error) {
	if !httpOrHTTPSRe.MatchString(path) {
		return protocompile.SearchResult{}, protoregistry.NotFound
	}

	b, err := loadFromHTTPWithLimit(context.Background(), path, filesafety.DefaultMaxFileSize)
	if err != nil {
		return protocompile.SearchResult{}, err
	}

	return protocompile.SearchResult{Source: bytes.NewReader(b)}, nil
}

var resolver = protocompile.CompositeResolver{&protocompile.SourceResolver{}, httpResolver}

func readFileDescriptorProtoFromFile(filename string) (*descriptorpb.FileDescriptorSet, error) {
	if filepath.Ext(filename) == ".proto" {
		compiler := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(resolver),
		}

		files, err := compiler.Compile(context.Background(), filename)
		if err != nil {
			return nil, err
		}

		return &descriptorpb.FileDescriptorSet{
			File: sliceOf(protodesc.ToFileDescriptorProto(files.FindFileByPath(filename))),
		}, nil
	}

	var b []byte
	var err error
	if httpOrHTTPSRe.MatchString(filename) {
		b, err = loadFromHTTPWithLimit(context.Background(), filename, filesafety.DefaultMaxFileSize)
		if err != nil {
			return nil, fmt.Errorf("error on fetch proto descriptor from %v: %w", filename, err)
		}
	} else {
		// nil options = regular-file check with the 100MB default cap.
		b, err = filesafety.SafeReadFile(filename, nil)
		if err != nil {
			return nil, fmt.Errorf("error on read proto descriptor-file %v: %w", filename, err)
		}
	}

	var fds descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(b, &fds)
	if err != nil {
		return nil, fmt.Errorf("error on unmarshal proto descriptor-file %v: %w", filename, err)
	}
	return &fds, nil
}

func parseTimestampBound(s string) (spanner.TimestampBound, error) {
	// Use strings.Fields for more robust whitespace handling
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return spanner.StrongRead(), fmt.Errorf("unknown staleness: %q", s)
	}

	first := fields[0]

	// All timestamp bounds accept at most 2 fields (type + parameter)
	if len(fields) > 2 {
		return spanner.StrongRead(), fmt.Errorf("%s accepts at most one parameter", first)
	}
	var second string
	if len(fields) > 1 {
		second = fields[1]
	}

	// only for error result
	nilStaleness := spanner.StrongRead()

	switch strings.ToUpper(first) {
	case "STRONG":
		if len(fields) > 1 {
			return nilStaleness, fmt.Errorf("STRONG does not accept any parameters")
		}
		return spanner.StrongRead(), nil
	case "MIN_READ_TIMESTAMP":
		if len(fields) < 2 {
			return nilStaleness, fmt.Errorf("MIN_READ_TIMESTAMP requires a timestamp parameter")
		}
		ts, err := time.Parse(time.RFC3339Nano, second)
		if err != nil {
			return nilStaleness, err
		}
		return spanner.MinReadTimestamp(ts), nil
	case "READ_TIMESTAMP":
		if len(fields) < 2 {
			return nilStaleness, fmt.Errorf("READ_TIMESTAMP requires a timestamp parameter")
		}
		ts, err := time.Parse(time.RFC3339Nano, second)
		if err != nil {
			return nilStaleness, err
		}
		return spanner.ReadTimestamp(ts), nil
	case "MAX_STALENESS":
		if len(fields) < 2 {
			return nilStaleness, fmt.Errorf("MAX_STALENESS requires a duration parameter")
		}
		ts, err := time.ParseDuration(second)
		if err != nil {
			return nilStaleness, err
		}
		if ts < 0 {
			return nilStaleness, fmt.Errorf("staleness duration %q must be non-negative", second)
		}
		return spanner.MaxStaleness(ts), nil
	case "EXACT_STALENESS":
		if len(fields) < 2 {
			return nilStaleness, fmt.Errorf("EXACT_STALENESS requires a duration parameter")
		}
		ts, err := time.ParseDuration(second)
		if err != nil {
			return nilStaleness, err
		}
		if ts < 0 {
			return nilStaleness, fmt.Errorf("staleness duration %q must be non-negative", second)
		}
		return spanner.ExactStaleness(ts), nil
	default:
		return nilStaleness, fmt.Errorf("unknown staleness: %v", first)
	}
}
