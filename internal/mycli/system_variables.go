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
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/bufbuild/protocompile"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

type LastQueryCache struct {
	QueryPlan       *sppb.QueryPlan
	QueryStats      map[string]any
	ReadTimestamp   time.Time
	CommitTimestamp time.Time
}

// systemVariables holds configuration state for spanner-mycli sessions.
// IMPORTANT: This struct is designed to be read-only after creation for session safety.
// SessionHandler depends on this read-only property when creating new sessions with
// modified copies of systemVariables (e.g., for USE/DETACH operations).
type systemVariables struct {
	// java-spanner compatible
	AutoPartitionMode           bool                         // AUTO_PARTITION_MODE
	RPCPriority                 sppb.RequestOptions_Priority // RPC_PRIORITY
	ReadOnlyStaleness           *spanner.TimestampBound      // READ_ONLY_STALENESS
	ReadTimestamp               time.Time                    // READ_TIMESTAMP
	OptimizerVersion            string                       // OPTIMIZER_VERSION
	OptimizerStatisticsPackage  string                       // OPTIMIZER_STATISTICS_PACKAGE
	CommitResponse              *sppb.CommitResponse         // COMMIT_RESPONSE
	CommitTimestamp             time.Time                    // COMMIT_TIMESTAMP
	TransactionTag              string                       // TRANSACTION_TAG
	RequestTag                  string                       // STATEMENT_TAG
	ReadOnly                    bool                         // READONLY
	DataBoostEnabled            bool                         // DATA_BOOST_ENABLED
	AutoBatchDML                bool                         // AUTO_BATCH_DML
	ExcludeTxnFromChangeStreams bool                         // EXCLUDE_TXN_FROM_CHANGE_STREAMS
	MaxCommitDelay              *time.Duration               // MAX_COMMIT_DELAY
	MaxPartitionedParallelism   int64                        // MAX_PARTITIONED_PARALLELISM
	AutocommitDMLMode           enums.AutocommitDMLMode      // AUTOCOMMIT_DML_MODE
	StatementTimeout            *time.Duration               // STATEMENT_TIMEOUT
	ReturnCommitStats           bool                         // RETURN_COMMIT_STATS

	DefaultIsolationLevel sppb.TransactionOptions_IsolationLevel         // DEFAULT_ISOLATION_LEVEL
	ReadLockMode          sppb.TransactionOptions_ReadWrite_ReadLockMode // READ_LOCK_MODE

	// CLI_* variables

	CLIFormat   enums.DisplayMode // CLI_FORMAT
	Project     string            // CLI_PROJECT
	Instance    string            // CLI_INSTANCE
	Database    string            // CLI_DATABASE
	Verbose     bool              // CLI_VERBOSE
	Profile     bool              // CLI_PROFILE - enables performance profiling (memory, timing)
	Prompt      string            // CLI_PROMPT
	Prompt2     string            // CLI_PROMPT2
	HistoryFile string            // CLI_HISTORY_FILE

	DirectedRead *sppb.DirectedReadOptions // CLI_DIRECT_READ

	ProtoDescriptorFile []string        // CLI_PROTO_DESCRIPTOR_FILE
	BuildStatementMode  enums.ParseMode // CLI_PARSE_MODE
	Insecure            bool            // CLI_INSECURE
	LogGrpc             bool            // CLI_LOG_GRPC
	LintPlan            bool            // CLI_LINT_PLAN
	UsePager            bool            // CLI_USE_PAGER
	AutoWrap            bool            // CLI_AUTOWRAP
	FixedWidth          *int64          // CLI_FIXED_WIDTH
	EnableHighlight     bool            // CLI_ENABLE_HIGHLIGHT
	MultilineProtoText  bool            // CLI_PROTOTEXT_MULTILINE
	MarkdownCodeblock   bool            // CLI_MARKDOWN_CODEBLOCK

	QueryMode         *sppb.ExecuteSqlRequest_QueryMode // CLI_QUERY_MODE
	TryPartitionQuery bool                              // CLI_TRY_PARTITION_QUERY

	VertexAIProject    string                     // CLI_VERTEXAI_PROJECT
	VertexAIModel      string                     // CLI_VERTEXAI_MODEL
	DatabaseDialect    databasepb.DatabaseDialect // CLI_DATABASE_DIALECT
	EchoExecutedDDL    bool                       // CLI_ECHO_EXECUTED_DDL
	Role               string                     // CLI_ROLE
	EchoInput          bool                       // CLI_ECHO_INPUT
	Host               string                     // CLI_HOST
	Port               int                        // CLI_PORT
	EmulatorPlatform   string                     // CLI_EMULATOR_PLATFORM
	OutputTemplateFile string                     // Not exposed as system variable
	TabWidth           int64                      // CLI_TAB_WIDTH
	LogLevel           slog.Level                 // CLI_LOG_LEVEL

	AnalyzeColumns string // CLI_ANALYZE_COLUMNS
	InlineStats    string // CLI_INLINE_STATS

	ExplainFormat          enums.ExplainFormat // CLI_EXPLAIN_FORMAT
	ExplainWrapWidth       int64               // CLI_EXPLAIN_WRAP_WIDTH
	AutoConnectAfterCreate bool                // CLI_AUTO_CONNECT_AFTER_CREATE

	// They are internal variables and hidden from system variable statements
	ProtoDescriptor      *descriptorpb.FileDescriptorSet
	ParsedAnalyzeColumns []columnRenderDef
	ParsedInlineStats    []inlineStatsDef
	OutputTemplate       *template.Template
	LastQueryCache       *LastQueryCache

	WithoutAuthentication bool
	Params                map[string]ast.Node

	// link to session
	CurrentSession *Session

	// TtyOutStream has been moved to StreamManager.
	// Use StreamManager.GetTtyStream() instead.

	// StreamManager manages tee output functionality
	StreamManager *streamio.StreamManager

	EnableProgressBar         bool   // CLI_ENABLE_PROGRESS_BAR
	ImpersonateServiceAccount string // CLI_IMPERSONATE_SERVICE_ACCOUNT
	EnableADCPlus             bool   // CLI_ENABLE_ADC_PLUS
	MCP                       bool   // CLI_MCP
	AsyncDDL                  bool   // CLI_ASYNC_DDL
	SkipSystemCommand         bool   // CLI_SKIP_SYSTEM_COMMAND
	SkipColumnNames           bool   // CLI_SKIP_COLUMN_NAMES

	// Streaming output configuration
	StreamingMode    enums.StreamingMode // CLI_STREAMING
	TablePreviewRows int64               // CLI_TABLE_PREVIEW_ROWS

	// SQL export configuration
	SQLTableName string // CLI_SQL_TABLE_NAME
	SQLBatchSize int64  // CLI_SQL_BATCH_SIZE

	// Result output configuration
	SuppressResultLines bool // CLI_SUPPRESS_RESULT_LINES

	// Registry holds the system variable registry
	Registry *VarRegistry

	// Computed variables (handled by registry)
	// CLI_VERSION - computed
	// CLI_CURRENT_WIDTH - computed
	// CLI_ENDPOINT - computed getter/setter
	// CLI_OUTPUT_TEMPLATE_FILE - computed getter/setter

	// Unimplemented variables (kept for compatibility)
	Autocommit            bool // AUTOCOMMIT (unimplemented)
	RetryAbortsInternally bool // RETRY_ABORTS_INTERNALLY (unimplemented)
}

// toFormatConfig converts the formatter-relevant fields of systemVariables into a format.FormatConfig.
func (sv *systemVariables) toFormatConfig() format.FormatConfig {
	return format.FormatConfig{
		TabWidth:        int(sv.TabWidth),
		Verbose:         sv.Verbose,
		SkipColumnNames: sv.SkipColumnNames,
		SQLTableName:    sv.SQLTableName,
		SQLBatchSize:    sv.SQLBatchSize,
		PreviewRows:     sv.TablePreviewRows,
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
	return instancePath(sv.Project, sv.Instance)
}

func (sv *systemVariables) DatabasePath() string {
	return databasePath(sv.Project, sv.Instance, sv.Database)
}

func (sv *systemVariables) ProjectPath() string {
	return projectPath(sv.Project)
}

// newSystemVariablesWithDefaults creates a new systemVariables instance with default values.
// This function ensures consistency between initialization and test expectations.
func newSystemVariablesWithDefaults() systemVariables {
	sv := systemVariables{
		// Java-spanner compatible defaults
		ReturnCommitStats: true,
		RPCPriority:       defaultPriority,

		// CLI defaults
		CLIFormat:            enums.DisplayModeTable, // Default to TABLE format
		EnableADCPlus:        true,
		AnalyzeColumns:       DefaultAnalyzeColumns,
		ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
		Prompt:               defaultPrompt,
		Prompt2:              defaultPrompt2,
		HistoryFile:          defaultHistoryFile,
		VertexAIModel:        defaultVertexAIModel,
		OutputTemplate:       defaultOutputFormat,
		LogLevel:             slog.LevelWarn,

		// Streaming defaults
		StreamingMode:    enums.StreamingModeAuto, // Default to automatic selection based on format
		TablePreviewRows: 50,                      // Default to 50 rows - enough to fit on one screen while prioritizing proper table formatting

		// Initialize empty maps to avoid nil
		Params: make(map[string]ast.Node),
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
	return sv.setFromGoogleSQL(name, value)
}

// SetFromSimple sets a system variable using Simple parsing mode.
// This is used for command-line flags and config files where values
// don't follow GoogleSQL syntax rules.
func (sv *systemVariables) SetFromSimple(name string, value string) error {
	sv.ensureRegistry()
	return sv.setFromSimple(name, value)
}

func (sv *systemVariables) AddFromGoogleSQL(name string, value string) error {
	sv.ensureRegistry()
	return sv.addFromGoogleSQL(name, value)
}

func (sv *systemVariables) AddFromSimple(name string, value string) error {
	sv.ensureRegistry()
	return sv.addFromSimple(name, value)
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
	Description string
	ReadOnly    bool
	CanAdd      bool
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

func setDefaultOutputTemplate(sysVars *systemVariables) {
	sysVars.OutputTemplateFile = ""
	sysVars.OutputTemplate = defaultOutputFormat
}

func setOutputTemplateFile(sysVars *systemVariables, filename string) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	tmpl, err := template.New("").Funcs(sproutFuncMap()).Parse(string(b))
	if err != nil {
		return err
	}

	sysVars.OutputTemplateFile = filename
	sysVars.OutputTemplate = tmpl
	return nil
}

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

	resp, err := http.Get(path)
	if err != nil {
		return protocompile.SearchResult{}, err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return protocompile.SearchResult{}, err
	}

	return protocompile.SearchResult{Source: &buf}, nil
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
		resp, err := http.Get(filename)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to fetch proto descriptor from %v: HTTP %d", filename, resp.StatusCode)
		}

		b, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	} else {
		b, err = os.ReadFile(filename)
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
