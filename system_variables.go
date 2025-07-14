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
	AutocommitDMLMode           AutocommitDMLMode            // AUTOCOMMIT_DML_MODE
	StatementTimeout            *time.Duration               // STATEMENT_TIMEOUT
	ReturnCommitStats           bool                         // RETURN_COMMIT_STATS

	DefaultIsolationLevel sppb.TransactionOptions_IsolationLevel // DEFAULT_ISOLATION_LEVEL

	// CLI_* variables

	CLIFormat   DisplayMode // CLI_FORMAT
	Project     string      // CLI_PROJECT
	Instance    string      // CLI_INSTANCE
	Database    string      // CLI_DATABASE
	Verbose     bool        // CLI_VERBOSE
	Prompt      string      // CLI_PROMPT
	Prompt2     string      // CLI_PROMPT2
	HistoryFile string      // CLI_HISTORY_FILE

	DirectedRead *sppb.DirectedReadOptions // CLI_DIRECT_READ

	ProtoDescriptorFile []string  // CLI_PROTO_DESCRIPTOR_FILE
	BuildStatementMode  parseMode // CLI_PARSE_MODE
	Insecure            bool      // CLI_INSECURE
	LogGrpc             bool      // CLI_LOG_GRPC
	LintPlan            bool      // CLI_LINT_PLAN
	UsePager            bool      // CLI_USE_PAGER
	AutoWrap            bool      // CLI_AUTOWRAP
	FixedWidth          *int64    // CLI_FIXED_WIDTH
	EnableHighlight     bool      // CLI_ENABLE_HIGHLIGHT
	MultilineProtoText  bool      // CLI_PROTOTEXT_MULTILINE
	MarkdownCodeblock   bool      // CLI_MARKDOWN_CODEBLOCK

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
	OutputTemplateFile string                     // CLI_OUTPUT_TEMPLATE_FILE
	TabWidth           int64                      // CLI_TAB_WIDTH
	LogLevel           slog.Level                 // CLI_LOG_LEVEL

	AnalyzeColumns string // CLI_ANALYZE_COLUMNS
	InlineStats    string // CLI_INLINE_STATS

	ExplainFormat          explainFormat // CLI_EXPLAIN_FORMAT
	ExplainWrapWidth       int64         // CLI_EXPLAIN_WRAP_WIDTH
	AutoConnectAfterCreate bool          // CLI_AUTO_CONNECT_AFTER_CREATE

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
	EnableProgressBar         bool
	ImpersonateServiceAccount string
	EnableADCPlus             bool
	MCP                       bool // CLI_MCP (read-only)
	MCPDebug                  bool // CLI_MCP_DEBUG
	AsyncDDL                  bool // CLI_ASYNC_DDL
	SkipSystemCommand         bool // CLI_SKIP_SYSTEM_COMMAND
	SkipColumnNames           bool // CLI_SKIP_COLUMN_NAMES
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
	return systemVariables{
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

func (sv *systemVariables) Set(name string, value string) error {
	upperName := strings.ToUpper(name)
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

func (sv *systemVariables) Add(name string, value string) error {
	upperName := strings.ToUpper(name)
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
	"DEFAULT_ISOLATION_LEVEL": {
		Description: "The transaction isolation level that is used by default for read/write transactions.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				v := strings.Join(strings.Fields(strings.ToUpper(unquoteString(value))), "_")
				isolation, ok := sppb.TransactionOptions_IsolationLevel_value[v]
				if ok {
					this.DefaultIsolationLevel = sppb.TransactionOptions_IsolationLevel(isolation)
				} else {
					return fmt.Errorf("invalid isolation level: %v", v)
				}
				return nil
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, this.DefaultIsolationLevel.String()), nil
			},
		},
	},
	"AUTO_PARTITION_MODE": {
		Description: "A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.AutoPartitionMode
		}),
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
	"MAX_COMMIT_DELAY": {
		Description: "The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				if strings.ToUpper(value) == "NULL" {
					this.MaxCommitDelay = nil
					return nil
				}

				duration, err := time.ParseDuration(unquoteString(value))
				if err != nil {
					return fmt.Errorf("failed to parse duration %s: %w", value, err)
				}

				this.MaxCommitDelay = &duration
				return nil
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.MaxCommitDelay == nil {
					return singletonMap(name, "NULL"), errIgnored
				}

				return singletonMap(name, this.MaxCommitDelay.String()), nil
			},
		},
	},
	"RETRY_ABORTS_INTERNALLY": {Description: "A boolean indicating whether the connection automatically retries aborted transactions. The default is true."},
	"MAX_PARTITIONED_PARALLELISM": {
		Description: "A property of type `INT64` indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for `AUTO_PARTITION_MODE=TRUE` and `RUN PARTITIONED QUERY`",
		Accessor: int64Accessor(func(variables *systemVariables) *int64 {
			return &variables.MaxPartitionedParallelism
		}),
	},
	"AUTOCOMMIT_DML_MODE": {
		Description: "A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				switch unquoteString(value) {
				case "PARTITIONED_NON_ATOMIC":
					this.AutocommitDMLMode = AutocommitDMLModePartitionedNonAtomic
					return nil
				case "TRANSACTIONAL":
					this.AutocommitDMLMode = AutocommitDMLModeTransactional
					return nil
				default:
					return fmt.Errorf("invalid AUTOCOMMIT_DML_MODE value: %v", value)
				}
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name,
					lo.Ternary(this.AutocommitDMLMode == AutocommitDMLModePartitionedNonAtomic, "PARTITIONED_NON_ATOMIC", "TRANSACTIONAL")), nil
			},
		},
	},
	"STATEMENT_TIMEOUT": {
		Description: "A property of type STRING indicating the current timeout value for statements (e.g., '10s', '5m', '1h'). Default is '10m'.",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.StatementTimeout == nil {
					return singletonMap(name, "10m"), nil
				}
				return singletonMap(name, this.StatementTimeout.String()), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				timeout, err := time.ParseDuration(unquoteString(value))
				if err != nil {
					return fmt.Errorf("invalid timeout format: %v", err)
				}
				if timeout < 0 {
					return fmt.Errorf("timeout cannot be negative")
				}
				this.StatementTimeout = &timeout
				return nil
			},
		},
	},
	"EXCLUDE_TXN_FROM_CHANGE_STREAMS": {
		Description: "Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.ExcludeTxnFromChangeStreams
		}),
	},
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
	"OPTIMIZER_VERSION": {
		Description: "A property of type `STRING` indicating the optimizer version. The version is either an integer string or 'LATEST'.",
		Accessor: stringAccessor(func(sysVars *systemVariables) *string {
			return &sysVars.OptimizerVersion
		}),
	},
	"OPTIMIZER_STATISTICS_PACKAGE": {
		Description: "A property of type STRING indicating the current optimizer statistics package that is used by this connection.",
		Accessor: stringAccessor(func(sysVars *systemVariables) *string {
			return &sysVars.OptimizerStatisticsPackage
		}),
	},
	"RETURN_COMMIT_STATS": {
		Description: "A property of type BOOL indicating whether statistics should be returned for transactions on this connection.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.ReturnCommitStats
		}),
	},
	"AUTO_BATCH_DML": {
		Description: "A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.AutoBatchDML
		}),
	},
	"DATA_BOOST_ENABLED": {
		Description: "A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.",
		Accessor: boolAccessor(func(sysVars *systemVariables) *bool {
			return &sysVars.DataBoostEnabled
		}),
	},
	"RPC_PRIORITY": {
		Description: "A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				s := unquoteString(value)

				p, err := parsePriority(s)
				if err != nil {
					return err
				}

				this.RPCPriority = p
				return nil
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, strings.TrimPrefix(this.RPCPriority.String(), "PRIORITY_")), nil
			},
		},
	},
	"TRANSACTION_TAG": {
		Description: "A property of type STRING that contains the transaction tag for the next transaction.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				if this.CurrentSession == nil {
					return errors.New("invalid state: current session is not populated")
				}

				return this.CurrentSession.setTransactionTag(unquoteString(value))
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.CurrentSession == nil {
					return singletonMap(name, ""), errIgnored
				}

				tag := this.CurrentSession.getTransactionTag()
				if tag == "" {
					return singletonMap(name, ""), errIgnored
				}

				return singletonMap(name, tag), nil
			},
		},
	},
	"STATEMENT_TAG": {
		Description: "A property of type STRING that contains the request tag for the next statement.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				this.RequestTag = unquoteString(value)
				return nil
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.RequestTag == "" {
					return nil, errIgnored
				}

				return singletonMap(name, this.RequestTag), nil
			},
		},
	},
	"READ_TIMESTAMP": {
		Description: "The read timestamp of the most recent read-only transaction.",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.ReadTimestamp.IsZero() {
					return nil, errIgnored
				}
				return singletonMap(name, this.ReadTimestamp.Format(time.RFC3339Nano)), nil
			},
		},
	},
	"COMMIT_TIMESTAMP": {
		Description: "The commit timestamp of the last read-write transaction that Spanner committed.",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.CommitTimestamp.IsZero() {
					return nil, errIgnored
				}
				s := this.CommitTimestamp.Format(time.RFC3339Nano)
				return singletonMap(name, s), nil
			},
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
	"CLI_MCP": {
		Description: "A read-only boolean indicating whether the connection is running as an MCP server.",
		Accessor: accessor{
			Getter: boolGetter(func(sysVars *systemVariables) *bool { return &sysVars.MCP }),
		},
	},
	"CLI_MCP_DEBUG": {
		Description: "Enable debug logging for MCP requests and responses.",
		Accessor:    boolAccessor(func(sysVars *systemVariables) *bool { return &sysVars.MCPDebug }),
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
	"CLI_ECHO_EXECUTED_DDL": {
		Description: "",
		Accessor: accessor{
			Getter: boolGetter(func(sysVars *systemVariables) *bool { return &sysVars.EchoExecutedDDL }),
			Setter: func(this *systemVariables, name, value string) error {
				b, err := strconv.ParseBool(value)
				if err != nil {
					return err
				}
				this.EchoExecutedDDL = b
				return nil
			},
		},
	},
	"CLI_ROLE": {
		Description: "",
		Accessor: accessor{
			Getter: stringGetter(func(sysVars *systemVariables) *string { return &sysVars.Role }),
		},
	},
	"CLI_ECHO_INPUT": {
		Description: "",
		Accessor:    boolAccessor(func(sysVars *systemVariables) *bool { return &sysVars.EchoInput }),
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
	"CLI_HOST": {
		Description: "Host on which Spanner server is located",
		Accessor: accessor{
			Getter: stringGetter(func(sysVars *systemVariables) *string { return &sysVars.Host }),
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
	"CLI_HISTORY_FILE": {
		Description: "",
		Accessor: accessor{
			Getter: stringGetter(
				func(sysVars *systemVariables) *string { return &sysVars.HistoryFile },
			),
		},
	},
	"CLI_VERTEXAI_MODEL": {
		Description: "",
		Accessor: stringAccessor(func(sysVars *systemVariables) *string {
			return &sysVars.VertexAIModel
		}),
	},
	"CLI_VERTEXAI_PROJECT": {
		Description: "",
		Accessor: stringAccessor(func(sysVars *systemVariables) *string {
			return &sysVars.VertexAIProject
		}),
	},
	"CLI_PROMPT": {
		Description: "",
		Accessor:    stringAccessor(func(sysVars *systemVariables) *string { return &sysVars.Prompt }),
	},
	"CLI_PROMPT2": {
		Description: "",
		Accessor:    stringAccessor(func(sysVars *systemVariables) *string { return &sysVars.Prompt2 }),
	},
	"CLI_PROJECT": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, this.Project), nil
			},
		},
	},
	"CLI_INSTANCE": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, this.Instance), nil
			},
		},
	},
	"CLI_DATABASE": {
		Description: "Current database name. Empty string when in detached mode.",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				// Return empty string for detached mode, actual database name when connected
				if this.CurrentSession != nil && this.CurrentSession.IsDetached() {
					return singletonMap(name, ""), nil
				}
				return singletonMap(name, this.Database), nil
			},
		},
	},
	"CLI_TAB_WIDTH": {
		Description: "Tab width. It is used for expanding tabs.",
		Accessor: int64Accessor(func(variables *systemVariables) *int64 {
			return &variables.TabWidth
		}),
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
	"CLI_EXPLAIN_FORMAT": {
		Description: "Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				return singletonMap(name, string(this.ExplainFormat)), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				format, err := parseExplainFormat(unquoteString(value))
				if err != nil {
					return err
				}

				this.ExplainFormat = format
				return nil
			},
		},
	},
	"CLI_ANALYZE_COLUMNS": {
		Description: "<name>:<template>[:<alignment>], ...",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				def, err := customListToTableRenderDefs(unquoteString(value))
				if err != nil {
					return err
				}

				if err != nil {
					return err
				}

				this.AnalyzeColumns = value
				this.ParsedAnalyzeColumns = def
				return nil
			},
			Getter: stringGetter(func(sysVars *systemVariables) *string {
				return &sysVars.AnalyzeColumns
			}),
		},
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
	"CLI_PARSE_MODE": {
		Description: "",
		Accessor: accessor{
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.BuildStatementMode == parseModeUnspecified {
					return nil, errIgnored
				}
				return singletonMap(name, string(this.BuildStatementMode)), nil
			},
			Setter: func(this *systemVariables, name, value string) error {
				s := strings.ToUpper(unquoteString(value))
				switch s {
				case string(parseModeFallback),
					string(parseMemefishOnly),
					string(parseModeNoMemefish),
					string(parseModeUnspecified):

					this.BuildStatementMode = parseMode(s)
					return nil
				default:
					return fmt.Errorf("invalid value: %v", s)
				}
			},
		},
	},
	"CLI_INSECURE": {
		Description: "",
		Accessor: accessor{
			Getter: boolGetter(func(sysVars *systemVariables) *bool { return &sysVars.Insecure }),
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
	"CLI_LINT_PLAN": {
		Description: "",
		Accessor: accessor{
			Getter: boolGetter(func(sysVars *systemVariables) *bool {
				return lo.Ternary(sysVars.LintPlan, lo.ToPtr(sysVars.LintPlan), nil)
			}),
			Setter: boolSetter(func(sysVars *systemVariables) *bool { return &sysVars.LintPlan }),
		},
	},
	"CLI_USE_PAGER": {
		Description: "",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.UsePager
		}),
	},
	"CLI_AUTOWRAP": {
		Description: "",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.AutoWrap
		}),
	},
	"CLI_FIXED_WIDTH": {
		Description: "Set fixed width to overwrite wrap width for CLI_AUTOWRAP.",
		Accessor: accessor{
			Setter: func(this *systemVariables, name, value string) error {
				if strings.ToUpper(value) == "NULL" {
					this.FixedWidth = nil
					return nil
				}

				width, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return err
				}

				this.FixedWidth = &width
				return nil
			},
			Getter: func(this *systemVariables, name string) (map[string]string, error) {
				if this.FixedWidth == nil {
					return singletonMap(name, "NULL"), nil
				}
				return singletonMap(name, strconv.FormatInt(*this.FixedWidth, 10)), nil
			},
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
	"CLI_ENABLE_HIGHLIGHT": {
		Description: "",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.EnableHighlight
		}),
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
	"CLI_AUTO_CONNECT_AFTER_CREATE": {
		Description: "A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.AutoConnectAfterCreate
		}),
	},
	"CLI_ENABLE_PROGRESS_BAR": {
		Description: "A boolean indicating whether to display progress bars during operations. The default is false.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.EnableProgressBar
		}),
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
	"CLI_SKIP_COLUMN_NAMES": {
		Description: "A boolean indicating whether to suppress column headers in output. The default is false.",
		Accessor: boolAccessor(func(variables *systemVariables) *bool {
			return &variables.SkipColumnNames
		}),
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
