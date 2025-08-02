// System Variables Naming Convention:
// - Variables that correspond to java-spanner JDBC properties use the same names
// - Variables that are spanner-mycli specific MUST use CLI_ prefix
// - This ensures compatibility with existing JDBC tooling while clearly identifying custom extensions
package main

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
	"github.com/bufbuild/protocompile"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/samber/lo"

	"spheric.cloud/xiter"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

type AutocommitDMLMode bool

const (
	AutocommitDMLModeTransactional        AutocommitDMLMode = false
	AutocommitDMLModePartitionedNonAtomic AutocommitDMLMode = true
)

type LastQueryCache struct {
	QueryPlan  *sppb.QueryPlan
	QueryStats map[string]any
	Timestamp  time.Time
}

// systemVariables holds configuration state for spanner-mycli sessions.
// IMPORTANT: This struct is designed to be read-only after creation for session safety.
// SessionHandler depends on this read-only property when creating new sessions with
// modified copies of systemVariables (e.g., for USE/DETACH operations).
type systemVariables struct {
	// java-spanner compatible
	AutoPartitionMode bool                         `sysvar:"name=AUTO_PARTITION_MODE,desc='A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.'"`
	RPCPriority       sppb.RequestOptions_Priority `sysvar:"name=RPC_PRIORITY,desc=\"A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.\",type=proto_enum,proto=sppb.RequestOptions_Priority,prefix=PRIORITY_"`
	// Complex type - requires custom parser
	ReadOnlyStaleness *spanner.TimestampBound `sysvar:"name=READ_ONLY_STALENESS,desc='A property of type STRING for read-only transactions with flexible staleness.',type=manual"`
	// Read-only with custom formatter
	ReadTimestamp              time.Time `sysvar:"name=READ_TIMESTAMP,desc='The read timestamp of the most recent read-only transaction.',readonly,getter=formatReadTimestamp"`
	OptimizerVersion           string    `sysvar:"name=OPTIMIZER_VERSION,desc='A property of type STRING indicating the optimizer version. The version is either an integer string or LATEST.'"`
	OptimizerStatisticsPackage string    `sysvar:"name=OPTIMIZER_STATISTICS_PACKAGE,desc='A property of type STRING indicating the current optimizer statistics package that is used by this connection.'"`
	// Complex type - not exposed in registry
	CommitResponse *sppb.CommitResponse `sysvar:"name=COMMIT_RESPONSE,desc='The most recent commit response from Spanner.',type=manual"`
	// Read-only with custom formatter
	CommitTimestamp             time.Time      `sysvar:"name=COMMIT_TIMESTAMP,desc='The commit timestamp of the last read-write transaction that Spanner committed.',readonly,getter=formatCommitTimestamp"`
	TransactionTag              string         `sysvar:"name=TRANSACTION_TAG,desc='A property of type STRING that contains the transaction tag for the next transaction.'"`
	RequestTag                  string         `sysvar:"name=STATEMENT_TAG,desc='A property of type STRING that contains the request tag for the next statement.'"`
	ReadOnly                    bool           `sysvar:"name=READONLY,desc='A boolean indicating whether or not the connection is in read-only mode. The default is false.',setter=setReadOnly"`
	DataBoostEnabled            bool           `sysvar:"name=DATA_BOOST_ENABLED,desc='A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.'"`
	AutoBatchDML                bool           `sysvar:"name=AUTO_BATCH_DML,desc='A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false.'"`
	ExcludeTxnFromChangeStreams bool           `sysvar:"name=EXCLUDE_TXN_FROM_CHANGE_STREAMS,desc='Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).'"`
	MaxCommitDelay              *time.Duration `sysvar:"name=MAX_COMMIT_DELAY,desc='The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.',type=nullable_duration,min=time.Duration(0),max=500 * time.Millisecond"`
	MaxPartitionedParallelism   int64          `sysvar:"name=MAX_PARTITIONED_PARALLELISM,desc='A property of type INT64 indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for AUTO_PARTITION_MODE=TRUE and RUN PARTITIONED QUERY'"`
	// Custom enum type
	AutocommitDMLMode AutocommitDMLMode `sysvar:"name=AUTOCOMMIT_DML_MODE,desc='A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements.',type=manual"`
	StatementTimeout  *time.Duration    `sysvar:"name=STATEMENT_TIMEOUT,desc='A property of type STRING indicating the current timeout value for statements (e.g., 10s, 5m, 1h). Default is 10m.',type=nullable_duration,min=time.Duration(0)"`
	ReturnCommitStats bool              `sysvar:"name=RETURN_COMMIT_STATS,desc='A property of type BOOL indicating whether statistics should be returned for transactions on this connection.'"`

	DefaultIsolationLevel sppb.TransactionOptions_IsolationLevel `sysvar:"name=DEFAULT_ISOLATION_LEVEL,desc='The transaction isolation level that is used by default for read/write transactions.',type=proto_enum,proto=sppb.TransactionOptions_IsolationLevel,prefix=ISOLATION_LEVEL_,aliases=sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED:UNSPECIFIED"`

	// CLI_* variables

	// Custom enum type
	CLIFormat   DisplayMode `sysvar:"name=CLI_FORMAT,desc='Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated), HTML (HTML table), XML (XML format), CSV (comma-separated values).',type=manual"`
	Project     string      `sysvar:"name=CLI_PROJECT,desc='GCP Project ID.',readonly"`
	Instance    string      `sysvar:"name=CLI_INSTANCE,desc='Cloud Spanner instance ID.',readonly"`
	Database    string      `sysvar:"name=CLI_DATABASE,desc='Cloud Spanner database ID.',readonly"`
	Verbose     bool        `sysvar:"name=CLI_VERBOSE,desc='Display verbose output.'"`
	Prompt      string      `sysvar:"name=CLI_PROMPT,desc='Custom prompt for spanner-mycli.'"`
	Prompt2     string      `sysvar:"name=CLI_PROMPT2,desc='Custom continuation prompt for spanner-mycli.',setter=setPrompt2"`
	HistoryFile string      `sysvar:"name=CLI_HISTORY_FILE,desc='Path to the history file.',readonly"`

	// Complex type - custom parser
	DirectedRead *sppb.DirectedReadOptions `sysvar:"name=CLI_DIRECT_READ,desc='Configuration for directed reads.',type=manual"`

	// Complex parser with ADD support
	ProtoDescriptorFile []string `sysvar:"name=CLI_PROTO_DESCRIPTOR_FILE,desc='Comma-separated list of proto descriptor files. Supports ADD to append files.',type=manual"`
	// Custom enum with aliases
	BuildStatementMode parseMode `sysvar:"name=CLI_PARSE_MODE,desc='Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED',type=manual"`
	Insecure           bool      `sysvar:"name=CLI_INSECURE,desc='Skip TLS certificate verification (insecure).',readonly"`
	LogGrpc            bool      `sysvar:"name=CLI_LOG_GRPC,desc='Enable gRPC logging.',readonly"`
	LintPlan           bool      `sysvar:"name=CLI_LINT_PLAN,desc='Enable query plan linting.'"`
	UsePager           bool      `sysvar:"name=CLI_USE_PAGER,desc='Enable pager for output.'"`
	AutoWrap           bool      `sysvar:"name=CLI_AUTOWRAP,desc='Enable automatic line wrapping.'"`
	FixedWidth         *int64    `sysvar:"name=CLI_FIXED_WIDTH,desc='If set, limits output width to the specified number of characters. NULL means automatic width detection.',type=nullable_int"`
	EnableHighlight    bool      `sysvar:"name=CLI_ENABLE_HIGHLIGHT,desc='Enable syntax highlighting.'"`
	MultilineProtoText bool      `sysvar:"name=CLI_PROTOTEXT_MULTILINE,desc='Enable multiline prototext output.'"`
	MarkdownCodeblock  bool      `sysvar:"name=CLI_MARKDOWN_CODEBLOCK,desc='Enable markdown codeblock output.'"`

	QueryMode         *sppb.ExecuteSqlRequest_QueryMode `sysvar:"name=CLI_QUERY_MODE,desc='Query execution mode.',type=proto_enum,proto=sppb.ExecuteSqlRequest_QueryMode,getter=getQueryMode,setter=setQueryMode"`
	TryPartitionQuery bool                              `sysvar:"name=CLI_TRY_PARTITION_QUERY,desc='A boolean indicating whether to test query for partition compatibility instead of executing it.'"`

	VertexAIProject    string                     `sysvar:"name=CLI_VERTEXAI_PROJECT,desc='Vertex AI project for natural language features.'"`
	VertexAIModel      string                     `sysvar:"name=CLI_VERTEXAI_MODEL,desc='Vertex AI model for natural language features.'"`
	DatabaseDialect    databasepb.DatabaseDialect `sysvar:"name=CLI_DATABASE_DIALECT,desc='Database dialect for the session.',type=proto_enum,proto=databasepb.DatabaseDialect,aliases=databasepb.DatabaseDialect_DATABASE_DIALECT_UNSPECIFIED:"`
	EchoExecutedDDL    bool                       `sysvar:"name=CLI_ECHO_EXECUTED_DDL,desc='Echo executed DDL statements.'"`
	Role               string                     `sysvar:"name=CLI_ROLE,desc='Cloud Spanner database role.',readonly"`
	EchoInput          bool                       `sysvar:"name=CLI_ECHO_INPUT,desc='Echo input statements.'"`
	Host               string                     `sysvar:"name=CLI_HOST,desc='Host on which Spanner server is located',readonly"`
	Port               int                        `sysvar:"name=CLI_PORT,desc='Port number for connections.',readonly,getter=getPortAsInt64"`
	EmulatorPlatform   string                     `sysvar:"name=CLI_EMULATOR_PLATFORM,desc='Container platform used by embedded emulator.',readonly"`
	OutputTemplateFile string                     // Not exposed as system variable
	TabWidth           int64                      `sysvar:"name=CLI_TAB_WIDTH,desc='Tab width. It is used for expanding tabs.'"`
	// Custom enum with setter
	LogLevel slog.Level `sysvar:"name=CLI_LOG_LEVEL,desc='Log level for the CLI.',type=manual,setter=setLogLevel"`

	// Custom template parser with side effects
	AnalyzeColumns string `sysvar:"name=CLI_ANALYZE_COLUMNS,desc='Go template for analyzing column data.',type=manual"`
	// Custom template parser
	InlineStats string `sysvar:"name=CLI_INLINE_STATS,desc='<name>:<template>, ...',type=manual"`

	// Custom enum type
	ExplainFormat          explainFormat `sysvar:"name=CLI_EXPLAIN_FORMAT,desc='Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.',type=manual"`
	ExplainWrapWidth       int64         `sysvar:"name=CLI_EXPLAIN_WRAP_WIDTH,desc='Controls query plan wrap width. It effects only operators column contents'"`
	AutoConnectAfterCreate bool          `sysvar:"name=CLI_AUTO_CONNECT_AFTER_CREATE,desc='A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.'"`

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
	StreamManager *StreamManager

	// TODO: Expose as CLI_*
	EnableProgressBar         bool   `sysvar:"name=CLI_ENABLE_PROGRESS_BAR,desc='A boolean indicating whether to display progress bars during operations. The default is false.'"`
	ImpersonateServiceAccount string `sysvar:"name=CLI_IMPERSONATE_SERVICE_ACCOUNT,desc='Service account to impersonate.',readonly"`
	EnableADCPlus             bool   `sysvar:"name=CLI_ENABLE_ADC_PLUS,desc='A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.',setter=setEnableADCPlus"`
	MCP                       bool   `sysvar:"name=CLI_MCP,desc='A read-only boolean indicating whether the connection is running as an MCP server.',readonly"`                // CLI_MCP (read-only)
	AsyncDDL                  bool   `sysvar:"name=CLI_ASYNC_DDL,desc='A boolean indicating whether DDL statements should be executed asynchronously. The default is false.'"` // CLI_ASYNC_DDL
	SkipSystemCommand         bool   `sysvar:"name=CLI_SKIP_SYSTEM_COMMAND,desc='Controls whether system commands are disabled.'"`                                             // CLI_SKIP_SYSTEM_COMMAND
	SkipColumnNames           bool   `sysvar:"name=CLI_SKIP_COLUMN_NAMES,desc='A boolean indicating whether to suppress column headers in output. The default is false.'"`     // CLI_SKIP_COLUMN_NAMES

	// Registry holds the parser registry for system variables
	Registry *sysvar.Registry

	// Computed variables (using underscore fields with struct tags)
	_ struct{} `sysvar:"name=CLI_VERSION,desc='The version of spanner-mycli.',readonly,getter=getCLIVersion"`
	_ struct{} `sysvar:"name=CLI_CURRENT_WIDTH,desc='Current terminal width. Returns NULL if not connected to a terminal.',readonly,getter=getCLICurrentWidth"`
	_ struct{} `sysvar:"name=CLI_ENDPOINT,desc='Host and port for connections (host:port format).',getter=getCLIEndpoint,setter=setCLIEndpoint"`

	// Unimplemented variables (kept for compatibility)
	Autocommit            bool `sysvar:"name=AUTOCOMMIT,desc='A boolean indicating whether or not the connection is in autocommit mode. The default is true.',type=unimplemented"`
	RetryAbortsInternally bool `sysvar:"name=RETRY_ABORTS_INTERNALLY,desc='A boolean indicating whether the connection automatically retries aborted transactions. The default is true.',type=unimplemented"`

	// Complex setter with side effects
	_ struct{} `sysvar:"name=CLI_OUTPUT_TEMPLATE_FILE,desc='Go text/template for formatting the output of the CLI.',getter=getCLIOutputTemplateFile,setter=setCLIOutputTemplateFile"`

	// Multi-value getters (special handling needed)
	_ struct{} `sysvar:"name=COMMIT_RESPONSE,desc='Returns a result set with one row and two columns, COMMIT_TIMESTAMP and MUTATION_COUNT.',type=manual"`
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

type setter = func(this *systemVariables, name, value string) error

type adder = func(this *systemVariables, name, value string) error

type getter = func(this *systemVariables, name string) (map[string]string, error)

type accessor struct {
	Setter setter
	Getter getter
	Adder  adder
}

type systemVariableDef struct {
	Accessor    accessor
	Description string
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
		EnableADCPlus:        true,
		AnalyzeColumns:       DefaultAnalyzeColumns,
		ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
		Prompt:               defaultPrompt,
		Prompt2:              defaultPrompt2,
		HistoryFile:          defaultHistoryFile,
		VertexAIModel:        defaultVertexAIModel,
		OutputTemplate:       defaultOutputFormat,
	}

	// Don't initialize registry here - it will be done after the struct is assigned
	// to avoid closures capturing a pointer to the local copy
	return sv
}

// ensureRegistry initializes the registry if it hasn't been initialized yet.
// This allows lazy initialization on first use, eliminating the need for
// explicit initialization calls.
func (sv *systemVariables) ensureRegistry() {
	if sv.Registry == nil {
		sv.Registry = createSystemVariableRegistry(sv)
	}
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

// sessionInitOnlyVariables contains variables that can only be set before session creation.
// These variables typically control client initialization behavior that cannot be changed
// after the session is established. Attempting to change these after session creation
// will return an error showing the current value.
//
// To add a new session-init-only variable:
// 1. Add the variable name to this slice
// 2. Ensure the variable has a proper Setter in systemVariableDefMap
// 3. Document in the variable's Description that it must be set before session creation
var sessionInitOnlyVariables = []string{
	"CLI_ENABLE_ADC_PLUS",
	// Add more variables here as needed in the future
	// For example:
	// "CLI_ENABLE_TRACING",
	// "CLI_ENABLE_CLIENT_METRICS",
}

// SetFromGoogleSQL sets a system variable using GoogleSQL parsing mode.
// This is used for SET statements in REPL and SQL scripts where values
// are parsed as GoogleSQL expressions (e.g., TRUE, 'string value').
//
// This method acts as a facade during the migration from systemVariableDefMap
// to the new sysvar.Registry. It first attempts to use the new registry,
// then falls back to the legacy system for unmigrated variables.
// TODO: Remove the fallback logic once all variables are migrated to sysvar.Registry
func (sv *systemVariables) SetFromGoogleSQL(name string, value string) error {
	upperName := strings.ToUpper(name)

	// Ensure registry is initialized
	sv.ensureRegistry()

	// First check if the variable is in the new registry
	if sv.Registry.Has(upperName) {
		err := sv.Registry.SetFromGoogleSQL(upperName, value)
		// Convert unimplemented errors to legacy error types for compatibility
		if err != nil && strings.Contains(err.Error(), "is not implemented") {
			return errSetterUnimplemented{name}
		}
		return err
	}

	// Fall back to the old system
	a, ok := systemVariableDefMap[upperName]
	if !ok {
		return fmt.Errorf("unknown variable name: %v", name)
	}
	if a.Accessor.Setter == nil {
		return errSetterUnimplemented{name}
	}

	// Check if this is a session-init-only variable
	if slices.Contains(sessionInitOnlyVariables, upperName) {
		// Check if session is already initialized.
		// We check only CurrentSession != nil, not client != nil, because these variables
		// configure client options at CLI startup. Changes after session creation have no effect,
		// even in detached mode when the client will be created later via USE command.
		// For example, CLI_ENABLE_ADC_PLUS affects adminClient creation which happens
		// during initial session setup, not during database connection.
		if sv.CurrentSession != nil {
			// NOTE: We intentionally do not include the current value in the error message
			// or allow idempotent sets (setting to the same value). This is a deliberate
			// design choice: session-init-only variables should reject ALL SET operations
			// after session creation to make it crystal clear that these settings cannot
			// be changed once the session is established.
			return fmt.Errorf("%s cannot be changed after session creation", upperName)
		}
	}

	return a.Accessor.Setter(sv, upperName, value)
}

// SetFromSimple sets a system variable using Simple parsing mode.
// This is used for command-line flags and config files where values
// don't follow GoogleSQL syntax rules.
//
// This method checks the new registry first before falling back to systemVariableDefMap
// for legacy compatibility. Most variables are now handled by the unified registry system.
func (sv *systemVariables) SetFromSimple(name string, value string) error {
	upperName := strings.ToUpper(name)

	// Ensure registry is initialized
	sv.ensureRegistry()

	// First check if the variable is in the new registry
	if sv.Registry.Has(upperName) {
		err := sv.Registry.SetFromSimple(upperName, value)
		// Convert unimplemented errors to legacy error types for compatibility
		if err != nil && strings.Contains(err.Error(), "is not implemented") {
			return errSetterUnimplemented{name}
		}
		return err
	}

	// Fall back to SetFromGoogleSQL for old system (which doesn't distinguish modes)
	return sv.SetFromGoogleSQL(name, value)
}

func (sv *systemVariables) Add(name string, value string) error {
	// Add method is called from REPL/SQL scripts, so it uses GoogleSQL mode
	return sv.AddFromGoogleSQL(name, value)
}

func (sv *systemVariables) AddFromGoogleSQL(name string, value string) error {
	upperName := strings.ToUpper(name)

	// Ensure registry is initialized
	sv.ensureRegistry()

	// First check if the variable is in the new registry
	if sv.Registry.Has(upperName) {
		if sv.Registry.HasAppendSupport(upperName) {
			return sv.Registry.AppendFromGoogleSQL(upperName, value)
		}
		return fmt.Errorf("variable %s does not support ADD operation", upperName)
	}

	// Fall back to the old system
	a, ok := systemVariableDefMap[upperName]
	if !ok {
		return fmt.Errorf("unknown variable name: %v", name)
	}
	if a.Accessor.Adder == nil {
		return errAdderUnimplemented{name}
	}

	return a.Accessor.Adder(sv, upperName, value)
}

func (sv *systemVariables) AddFromSimple(name string, value string) error {
	upperName := strings.ToUpper(name)

	// Ensure registry is initialized
	sv.ensureRegistry()

	// First check if the variable is in the new registry
	if sv.Registry.Has(upperName) {
		if sv.Registry.HasAppendSupport(upperName) {
			return sv.Registry.AppendFromSimple(upperName, value)
		}
		return fmt.Errorf("variable %s does not support ADD operation", upperName)
	}

	// Fall back to the old system - the old system doesn't distinguish modes
	// so we just call the adder as before
	a, ok := systemVariableDefMap[upperName]
	if !ok {
		return fmt.Errorf("unknown variable name: %v", name)
	}
	if a.Accessor.Adder == nil {
		return errAdderUnimplemented{name}
	}

	return a.Accessor.Adder(sv, upperName, value)
}

func (sv *systemVariables) Get(name string) (map[string]string, error) {
	upperName := strings.ToUpper(name)

	// Ensure registry is initialized
	sv.ensureRegistry()

	// First check if the variable is in the new registry
	if sv.Registry.Has(upperName) {
		value, err := sv.Registry.Get(upperName)
		if err != nil {
			// Convert unimplemented errors to legacy error types for compatibility
			if strings.Contains(err.Error(), "is not implemented") {
				return nil, errGetterUnimplemented{name}
			}
			return nil, err
		}
		return singletonMap(name, value), nil
	}

	// Fall back to the old system
	a, ok := systemVariableDefMap[upperName]
	if !ok {
		return nil, fmt.Errorf("unknown variable name: %v", name)
	}
	if a.Accessor.Getter == nil {
		return nil, errGetterUnimplemented{name}
	}

	value, err := a.Accessor.Getter(sv, name)
	if err != nil && !errors.Is(err, errIgnored) {
		return nil, err
	}
	return value, nil
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

// systemVariableDefMap contains legacy variables that haven't been migrated to the new system yet.
// TODO: Migrate COMMIT_RESPONSE and CLI_DIRECT_READ to the new registry system,
// then remove this map and all fallback logic.
var systemVariableDefMap = map[string]systemVariableDef{
	"READONLY": {
		Description: "A boolean indicating whether or not the connection is in read-only mode. The default is false.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				if this.CurrentSession != nil && (this.CurrentSession.InReadOnlyTransaction() || this.CurrentSession.InReadWriteTransaction()) {
					return errors.New("can't change READONLY when there is a active transaction")
				}

				b, err := strconv.ParseBool(value)
				if err != nil {
					return err
				}

				this.ReadOnly = b
				return nil
			},
			Getter: boolGetter(func(sysVars *systemVariables) *bool { return &sysVars.ReadOnly }),
		},
	},
	"AUTOCOMMIT": {Description: "A boolean indicating whether or not the connection is in autocommit mode. The default is true."},
	"CLI_OUTPUT_TEMPLATE_FILE": {
		Description: "Go text/template for formatting the output of the CLI.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				filename := unquoteString(value)
				if strings.TrimSpace(strings.ToUpper(value)) == "NULL" || filename == "" {
					setDefaultOutputTemplate(this)
					return nil
				}

				return setOutputTemplateFile(this, filename)
			},
			Getter: stringGetter(func(sysVars *systemVariables) *string {
				return &sysVars.OutputTemplateFile
			}),
		},
	},
	"RETRY_ABORTS_INTERNALLY": {Description: "A boolean indicating whether the connection automatically retries aborted transactions. The default is true."},
	"READ_ONLY_STALENESS": {
		Description: "A property of type `STRING` indicating the current read-only staleness setting that Spanner uses for read-only transactions and single read-only queries.",
		Accessor: accessor{
			func(this *systemVariables, name, value string) error {
				staleness, err := parseTimestampBound(unquoteString(value))
				if err != nil {
					return err
				}

				this.ReadOnlyStaleness = &staleness
				return nil
			},
			func(this *systemVariables, name string) (map[string]string, error) {
				if this.ReadOnlyStaleness == nil {
					return nil, errIgnored
				}
				s := this.ReadOnlyStaleness.String()
				stalenessRe := regexp.MustCompile(`^\(([^:]+)(?:: (.+))?\)$`)
				matches := stalenessRe.FindStringSubmatch(s)
				if matches == nil {
					return singletonMap(name, s), nil
				}
				switch matches[1] {
				case "strong":
					return singletonMap(name, "STRONG"), nil

				case "exactStaleness":
					return singletonMap(name, fmt.Sprintf("EXACT_STALENESS %v", matches[2])), nil
				case "maxStaleness":
					return singletonMap(name, fmt.Sprintf("MAX_STALENESS %v", matches[2])), nil
				case "readTimestamp":
					ts, err := parseTimeString(matches[2])
					if err != nil {
						return nil, err
					}
					return singletonMap(name, fmt.Sprintf("READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))), nil
				case "minReadTimestamp":
					ts, err := parseTimeString(matches[2])
					if err != nil {
						return nil, err
					}
					return singletonMap(name, fmt.Sprintf("MIN_READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))), nil
				default:
					return singletonMap(name, s), nil
				}
			},
			nil,
		},
	},
	"COMMIT_RESPONSE": {
		Description: "Returns a result set with one row and two columns, COMMIT_TIMESTAMP and MUTATION_COUNT.",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.CommitResponse == nil {
					return nil, errIgnored
				}
				mutationCount := this.CommitResponse.GetCommitStats().GetMutationCount()
				return map[string]string{
					"COMMIT_TIMESTAMP": this.CommitTimestamp.Format(time.RFC3339Nano),
					"MUTATION_COUNT":   strconv.FormatInt(mutationCount, 10),
				}, nil
			},
		},
	},
	"CLI_FORMAT": {
		Description: "Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated), HTML (HTML table), XML (XML format), CSV (comma-separated values).",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				// Set the output format for query results.
				// Valid values:
				//   TABLE              - ASCII table with borders (default for interactive mode)
				//   TABLE_COMMENT      - Table wrapped in /* */ comments
				//   TABLE_DETAIL_COMMENT - Table with opening /* comment only
				//   VERTICAL           - Vertical format (column: value pairs)
				//   TAB                - Tab-separated values (default for batch mode)
				//   HTML               - HTML table format (--html flag)
				//   XML                - XML format (--xml flag)
				//   CSV                - Comma-separated values (RFC 4180)
				mode, err := parseDisplayMode(unquoteString(value))
				if err != nil {
					return fmt.Errorf("invalid CLI_FORMAT value: %v", value)
				}
				this.CLIFormat = mode
				return nil
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				// Return the current output format as a string.
				// This maps the internal DisplayMode enum to user-visible string values.
				var formatStr string
				switch this.CLIFormat {
				case DisplayModeTable:
					formatStr = "TABLE"
				case DisplayModeTableComment:
					formatStr = "TABLE_COMMENT"
				case DisplayModeTableDetailComment:
					formatStr = "TABLE_DETAIL_COMMENT"
				case DisplayModeVertical:
					formatStr = "VERTICAL"
				case DisplayModeTab:
					formatStr = "TAB"
				case DisplayModeHTML:
					formatStr = "HTML"
				case DisplayModeXML:
					formatStr = "XML"
				case DisplayModeCSV:
					formatStr = "CSV"
				default:
					// This should never happen as we validate on setter
					formatStr = "TABLE"
				}
				return singletonMap(name, formatStr), nil
			},
		},
	},
	"CLI_VERBOSE": {
		Description: "",
		Accessor: accessor{
			Getter: boolGetter(func(sysVars *systemVariables) *bool { return &sysVars.Verbose }),
			Setter: func(this *systemVariables, name, value string) error {
				b, err := strconv.ParseBool(value)
				if err != nil {
					return err
				}
				this.Verbose = b
				return nil
			},
		},
	},
	"CLI_DATABASE_DIALECT": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, this.DatabaseDialect.String()), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				n, ok := databasepb.DatabaseDialect_value[strings.ToUpper(unquoteString(value))]
				if !ok {
					return fmt.Errorf("invalid value: %v", value)
				}
				this.DatabaseDialect = databasepb.DatabaseDialect(n)
				return nil
			},
		},
	},
	"CLI_ENDPOINT": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				// Construct endpoint from host and port
				var endpoint string
				if this.Host != "" && this.Port != 0 {
					endpoint = net.JoinHostPort(this.Host, strconv.Itoa(this.Port))
				}
				return singletonMap(name, endpoint), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				// Parse endpoint and update host and port
				host, port, err := parseEndpoint(unquoteString(value))
				if err != nil {
					return err
				}
				this.Host = host
				this.Port = port
				return nil
			},
		},
	},
	"CLI_PORT": {
		Description: "Port number for Spanner connection",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, strconv.Itoa(this.Port)), nil
			},
		},
	},
	"CLI_EMULATOR_PLATFORM": {
		Description: "Container platform used by embedded emulator",
		Accessor: accessor{
			Getter: stringGetter(func(sysVars *systemVariables) *string { return &sysVars.EmulatorPlatform }),
		},
	},
	"CLI_DIRECT_READ": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, xiter.Join(xiter.Map(
					slices.Values(this.DirectedRead.GetIncludeReplicas().GetReplicaSelections()),
					func(rs *sppb.DirectedReadOptions_ReplicaSelection) string {
						return fmt.Sprintf("%v:%v", rs.GetLocation(), rs.GetType())
					},
				), ",")), nil
			},
		},
	},
	"CLI_PROMPT2": {
		Description: "",
		Accessor:    stringAccessor(func(sysVars *systemVariables) *string { return &sysVars.Prompt2 }),
	},
	"CLI_PROTO_DESCRIPTOR_FILE": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, strings.Join(this.ProtoDescriptorFile, ",")), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				filenames := strings.Split(unquoteString(value), ",")
				if len(filenames) == 0 {
					return nil
				}

				var fileDescriptorSet *descriptorpb.FileDescriptorSet
				for _, filename := range filenames {
					fds, err := readFileDescriptorProtoFromFile(filename)
					if err != nil {
						return err
					}
					fileDescriptorSet = mergeFDS(fileDescriptorSet, fds)
				}

				this.ProtoDescriptorFile = filenames
				this.ProtoDescriptor = fileDescriptorSet
				return nil
			},
			Adder: func(this *systemVariables, name, value string) error {
				filename := unquoteString(value)

				fds, err := readFileDescriptorProtoFromFile(filename)
				if err != nil {
					return err
				}

				if !slices.Contains(this.ProtoDescriptorFile, filename) {
					this.ProtoDescriptorFile = slices.Concat(this.ProtoDescriptorFile, sliceOf(filename))
					this.ProtoDescriptor = &descriptorpb.FileDescriptorSet{File: slices.Concat(this.ProtoDescriptor.GetFile(), fds.GetFile())}
				} else {
					this.ProtoDescriptor = mergeFDS(this.ProtoDescriptor, fds)
				}
				return nil
			},
		},
	},
	"CLI_EXPLAIN_WRAP_WIDTH": {
		Description: "Controls query plan wrap width. It effects only operators column contents",
		Accessor: int64Accessor(func(variables *systemVariables) *int64 {
			return &variables.ExplainWrapWidth
		}),
	},
	"CLI_INLINE_STATS": {
		Description: "<name>:<template>, ...",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				unquoted := unquoteString(value)
				defs, err := parseInlineStatsDefs(unquoted)
				if err != nil {
					return err
				}

				this.InlineStats = unquoted
				this.ParsedInlineStats = defs
				return nil
			},
			Getter: stringGetter(func(sysVars *systemVariables) *string {
				return &sysVars.InlineStats
			}),
		},
	},
	"CLI_LOG_LEVEL": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, this.LogLevel.String()), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				s := unquoteString(value)
				l, err := SetLogLevel(s)
				if err != nil {
					return err
				}
				this.LogLevel = l
				return nil
			},
		},
	},
	"CLI_LOG_GRPC": {
		Description: "",
		Accessor: accessor{
			Getter: boolGetter(func(sysVars *systemVariables) *bool {
				return lo.Ternary(sysVars.LogGrpc, &sysVars.LogGrpc, nil)
			}),
		},
	},
	"CLI_CURRENT_WIDTH": {
		Description: "Get the current screen width in spanner-mycli client-side statement.",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				// Terminal width detection is now handled by StreamManager
				// which properly uses the TTY stream even when output is tee'd
				if this.StreamManager != nil {
					return singletonMap(name, this.StreamManager.GetTerminalWidthString()), nil
				}

				// This should not happen in normal operation
				slog.Warn("StreamManager not available for terminal width detection")
				return singletonMap(name, "NULL"), nil
			},
		},
	},
	"CLI_PROTOTEXT_MULTILINE": {
		Description: "",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.MultilineProtoText
		}),
	},
	"CLI_MARKDOWN_CODEBLOCK": {
		Description: "",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.MarkdownCodeblock
		}),
	},
	"CLI_QUERY_MODE": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.QueryMode == nil {
					return nil, errIgnored
				}
				return singletonMap(name, this.QueryMode.String()), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				s := unquoteString(value)
				mode, ok := sppb.ExecuteSqlRequest_QueryMode_value[strings.ToUpper(s)]
				if !ok {
					return fmt.Errorf("invalid value: %v", s)
				}
				this.QueryMode = sppb.ExecuteSqlRequest_QueryMode(mode).Enum()
				return nil
			},
		},
	},
	"CLI_TRY_PARTITION_QUERY": {
		Description: "A boolean indicating whether to test query for partition compatibility instead of executing it.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.TryPartitionQuery
		}),
	},
	"CLI_VERSION": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, getVersion()), nil
			},
		},
	},
	// Session behavior variables - Getter only to avoid runtime session state changes
	"CLI_IMPERSONATE_SERVICE_ACCOUNT": {
		Description: "Service account email for impersonation.",
		Accessor: accessor{
			Getter: stringGetter(func(variables *systemVariables) *string {
				return &variables.ImpersonateServiceAccount
			}),
		},
	},
	"CLI_ENABLE_ADC_PLUS": {
		Description: "A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.EnableADCPlus
		}),
	},
	"CLI_ASYNC_DDL": {
		Description: "A boolean indicating whether DDL statements should be executed asynchronously. The default is false.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.AsyncDDL
		}),
	},
	"CLI_SKIP_SYSTEM_COMMAND": {
		Description: "A read-only boolean indicating whether system commands are disabled. Set via --skip-system-command flag or --system-command=OFF. When both are used, --skip-system-command takes precedence.",
		Accessor: accessor{
			Getter: boolGetter(func(variables *systemVariables) *bool {
				return &variables.SkipSystemCommand
			}),
		},
	},
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

func stringGetter(f func(sysVars *systemVariables) *string) getter {
	return func(this *systemVariables, name string) (map[string]string, error) {
		ref := f(this)
		if ref == nil {
			return nil, errIgnored
		}
		return singletonMap(name, *ref), nil
	}
}

func int64Getter(f func(sysVars *systemVariables) *int64) getter {
	return func(this *systemVariables, name string) (map[string]string, error) {
		ref := f(this)
		if ref == nil {
			return nil, errIgnored
		}

		return singletonMap(name, strings.ToUpper(strconv.FormatInt(*ref, 10))), nil
	}
}

func int64Setter(f func(sysVars *systemVariables) *int64) setter {
	return func(this *systemVariables, name string, value string) error {
		ref := f(this)

		b, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}

		*ref = b
		return nil
	}
}

func int64Accessor(f func(variables *systemVariables) *int64) accessor {
	return accessor{
		Setter: int64Setter(f),
		Getter: int64Getter(f),
	}
}

func boolGetter(f func(sysVars *systemVariables) *bool) getter {
	return func(this *systemVariables, name string) (map[string]string, error) {
		ref := f(this)
		if ref == nil {
			return nil, errIgnored
		}

		return singletonMap(name, strings.ToUpper(strconv.FormatBool(*ref))), nil
	}
}

func boolSetter(f func(sysVars *systemVariables) *bool) setter {
	return func(this *systemVariables, name string, value string) error {
		ref := f(this)

		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}

		*ref = b
		return nil
	}
}

func stringSetter(f func(sysVars *systemVariables) *string) setter {
	return func(this *systemVariables, name, value string) error {
		ref := f(this)
		s := unquoteString(value)
		*ref = s
		return nil
	}
}

func boolAccessor(f func(variables *systemVariables) *bool) accessor {
	return accessor{
		Setter: boolSetter(f),
		Getter: boolGetter(f),
	}
}

func stringAccessor(f func(sysVars *systemVariables) *string) accessor {
	return accessor{
		Setter: stringSetter(f),
		Getter: stringGetter(f),
	}
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
