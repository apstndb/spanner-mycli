package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// VarRegistry is a registry for system variables
type VarRegistry struct {
	vars        map[string]Variable
	addHandlers map[string]func(string) error // For ADD operations
	sv          *systemVariables
}

// NewVarRegistry creates a new variable registry
func NewVarRegistry(sv *systemVariables) *VarRegistry {
	r := &VarRegistry{
		vars:        make(map[string]Variable),
		addHandlers: make(map[string]func(string) error),
		sv:          sv,
	}
	r.registerAll()
	return r
}

// Register adds a variable to the registry
func (r *VarRegistry) Register(name string, v Variable) {
	r.vars[strings.ToUpper(name)] = v
}

// RegisterWithAdd adds a variable with ADD support
func (r *VarRegistry) RegisterWithAdd(name string, v Variable, addFunc func(string) error) {
	r.Register(name, v)
	r.addHandlers[strings.ToUpper(name)] = addFunc
}

// Get retrieves a variable value
func (r *VarRegistry) Get(name string) (string, error) {
	v, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return "", &ErrUnknownVariable{Name: name}
	}
	value, err := v.Get()
	if err != nil {
		return "", err
	}
	return value, nil
}

// Set sets a variable value
func (r *VarRegistry) Set(name, value string, isGoogleSQL bool) error {
	v, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return &ErrUnknownVariable{Name: name}
	}

	// Parse GoogleSQL value if needed
	if isGoogleSQL {
		value = parseGoogleSQLValue(value)
	}

	return v.Set(value)
}

// Add performs ADD operation on a variable
func (r *VarRegistry) Add(name, value string) error {
	upperName := strings.ToUpper(name)

	// First check if the variable exists
	if _, ok := r.vars[upperName]; !ok {
		return &ErrUnknownVariable{Name: name}
	}

	// Then check if it supports ADD
	addFunc, ok := r.addHandlers[upperName]
	if !ok {
		return &ErrAddNotSupported{Name: name}
	}
	return addFunc(value)
}

// GetDescription returns variable description
func (r *VarRegistry) GetDescription(name string) (string, error) {
	v, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return "", &ErrUnknownVariable{Name: name}
	}
	return v.Description(), nil
}

// IsReadOnly checks if a variable is read-only
func (r *VarRegistry) IsReadOnly(name string) (bool, error) {
	v, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return false, &ErrUnknownVariable{Name: name}
	}
	return v.IsReadOnly(), nil
}

// ListVariables returns a map of all variables with their current values
func (r *VarRegistry) ListVariables() map[string]string {
	result := make(map[string]string)
	for name, v := range r.vars {
		value, err := v.Get()
		if err == nil {
			result[name] = value
		}
	}
	return result
}

// ListVariableInfo returns information about all variables
func (r *VarRegistry) ListVariableInfo() map[string]struct {
	Description string
	ReadOnly    bool
	CanAdd      bool
} {
	result := make(map[string]struct {
		Description string
		ReadOnly    bool
		CanAdd      bool
	})

	for name, v := range r.vars {
		_, hasAdd := r.addHandlers[name]
		result[name] = struct {
			Description string
			ReadOnly    bool
			CanAdd      bool
		}{
			Description: v.Description(),
			ReadOnly:    v.IsReadOnly(),
			CanAdd:      hasAdd,
		}
	}

	return result
}

// registerAll registers all system variables
func (r *VarRegistry) registerAll() {
	sv := r.sv

	// === Simple boolean variables (30+) ===
	r.Register("READONLY", &CustomVar{
		base: BoolVar(&sv.ReadOnly, "A boolean indicating whether or not the connection is in read-only mode"),
		customSetter: func(value string) error {
			// Custom validation for READONLY
			if sv.CurrentSession != nil &&
				(sv.CurrentSession.InReadOnlyTransaction() || sv.CurrentSession.InReadWriteTransaction()) {
				return errSetterInTransaction
			}
			b, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			sv.ReadOnly = b
			return nil
		},
	})

	r.Register("AUTO_PARTITION_MODE", BoolVar(&sv.AutoPartitionMode,
		"A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed."))
	r.Register("DATA_BOOST_ENABLED", BoolVar(&sv.DataBoostEnabled,
		"A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false."))
	r.Register("AUTO_BATCH_DML", BoolVar(&sv.AutoBatchDML,
		"A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false."))
	r.Register("EXCLUDE_TXN_FROM_CHANGE_STREAMS", BoolVar(&sv.ExcludeTxnFromChangeStreams,
		"Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true)."))
	r.Register("RETURN_COMMIT_STATS", BoolVar(&sv.ReturnCommitStats,
		"A property of type BOOL indicating whether statistics should be returned for transactions on this connection."))
	r.Register("CLI_VERBOSE", BoolVar(&sv.Verbose, "Display verbose output."))
	r.Register("CLI_LINT_PLAN", BoolVar(&sv.LintPlan, "Enable query plan linting."))
	r.Register("CLI_USE_PAGER", BoolVar(&sv.UsePager, "Enable pager for output."))
	r.Register("CLI_AUTOWRAP", BoolVar(&sv.AutoWrap, "Enable automatic line wrapping."))
	r.Register("CLI_ENABLE_HIGHLIGHT", BoolVar(&sv.EnableHighlight, "Enable syntax highlighting."))
	r.Register("CLI_PROTOTEXT_MULTILINE", BoolVar(&sv.MultilineProtoText, "Enable multiline prototext output."))
	r.Register("CLI_MARKDOWN_CODEBLOCK", BoolVar(&sv.MarkdownCodeblock, "Enable markdown codeblock output."))
	r.Register("CLI_TRY_PARTITION_QUERY", BoolVar(&sv.TryPartitionQuery,
		"A boolean indicating whether to test query for partition compatibility instead of executing it."))
	r.Register("CLI_ECHO_EXECUTED_DDL", BoolVar(&sv.EchoExecutedDDL, "Echo executed DDL statements."))
	r.Register("CLI_ECHO_INPUT", BoolVar(&sv.EchoInput, "Echo input statements."))
	r.Register("CLI_AUTO_CONNECT_AFTER_CREATE", BoolVar(&sv.AutoConnectAfterCreate,
		"A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false."))
	r.Register("CLI_ENABLE_PROGRESS_BAR", BoolVar(&sv.EnableProgressBar,
		"A boolean indicating whether to display progress bars during operations. The default is false."))
	r.Register("CLI_ASYNC_DDL", BoolVar(&sv.AsyncDDL,
		"A boolean indicating whether DDL statements should be executed asynchronously. The default is false."))
	r.Register("CLI_SKIP_SYSTEM_COMMAND", BoolVar(&sv.SkipSystemCommand,
		"Controls whether system commands are disabled."))
	r.Register("CLI_SKIP_COLUMN_NAMES", BoolVar(&sv.SkipColumnNames,
		"A boolean indicating whether to suppress column headers in output. The default is false."))

	// === String variables (15+) ===
	r.Register("OPTIMIZER_VERSION", StringVar(&sv.OptimizerVersion,
		"A property of type STRING indicating the optimizer version. The version is either an integer string or LATEST."))
	r.Register("OPTIMIZER_STATISTICS_PACKAGE", StringVar(&sv.OptimizerStatisticsPackage,
		"A property of type STRING indicating the current optimizer statistics package that is used by this connection."))
	r.Register("TRANSACTION_TAG", StringVar(&sv.TransactionTag,
		"A property of type STRING that contains the transaction tag for the next transaction."))
	r.Register("STATEMENT_TAG", StringVar(&sv.RequestTag,
		"A property of type STRING that contains the request tag for the next statement."))
	r.Register("CLI_PROJECT", StringVar(&sv.Project, "GCP Project ID.").AsReadOnly())
	r.Register("CLI_INSTANCE", StringVar(&sv.Instance, "Cloud Spanner instance ID.").AsReadOnly())
	r.Register("CLI_DATABASE", StringVar(&sv.Database, "Cloud Spanner database ID.").AsReadOnly())
	r.Register("CLI_PROMPT", StringVar(&sv.Prompt, "Custom prompt for spanner-mycli."))
	r.Register("CLI_PROMPT2", &CustomVar{
		base: StringVar(&sv.Prompt2, "Custom continuation prompt for spanner-mycli."),
		customSetter: func(value string) error {
			if value == "" {
				return fmt.Errorf("CLI_PROMPT2 cannot be empty")
			}
			sv.Prompt2 = value
			return nil
		},
	})
	r.Register("CLI_HISTORY_FILE", StringVar(&sv.HistoryFile, "Path to the history file.").AsReadOnly())
	r.Register("CLI_VERTEXAI_PROJECT", StringVar(&sv.VertexAIProject, "Vertex AI project for natural language features."))
	r.Register("CLI_VERTEXAI_MODEL", StringVar(&sv.VertexAIModel, "Vertex AI model for natural language features."))
	r.Register("CLI_ROLE", StringVar(&sv.Role, "Cloud Spanner database role.").AsReadOnly())
	r.Register("CLI_HOST", StringVar(&sv.Host, "Host on which Spanner server is located").AsReadOnly())
	r.Register("CLI_EMULATOR_PLATFORM", StringVar(&sv.EmulatorPlatform, "Container platform used by embedded emulator.").AsReadOnly())
	r.Register("CLI_IMPERSONATE_SERVICE_ACCOUNT", StringVar(&sv.ImpersonateServiceAccount, "Service account to impersonate.").AsReadOnly())

	// === Integer variables (10+) ===
	r.Register("MAX_PARTITIONED_PARALLELISM", IntVar(&sv.MaxPartitionedParallelism,
		"A property of type INT64 indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for AUTO_PARTITION_MODE=TRUE and RUN PARTITIONED QUERY"))
	r.Register("CLI_TAB_WIDTH", IntVar(&sv.TabWidth, "Tab width. It is used for expanding tabs."))
	r.Register("CLI_EXPLAIN_WRAP_WIDTH", IntVar(&sv.ExplainWrapWidth,
		"Controls query plan wrap width. It effects only operators column contents"))
	r.Register("CLI_PORT", &IntGetterVar{
		getter:      func() int64 { return int64(sv.Port) },
		description: "Port number for connections.",
	})

	// === Nullable types (5+) ===
	r.Register("MAX_COMMIT_DELAY", NullableDurationVar(&sv.MaxCommitDelay,
		"The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.").
		WithValidator(durationValidator(durationPtr(0), durationPtr(500*time.Millisecond))))
	r.Register("STATEMENT_TIMEOUT", NullableDurationVar(&sv.StatementTimeout,
		"A property of type STRING indicating the current timeout value for statements (e.g., 10s, 5m, 1h). Default is 10m.").
		WithValidator(durationValidator(durationPtr(0), nil)))
	r.Register("CLI_FIXED_WIDTH", NullableIntVar(&sv.FixedWidth,
		"If set, limits output width to the specified number of characters. NULL means automatic width detection."))

	// === Proto Enum types (5) ===
	r.Register("RPC_PRIORITY", RPCPriorityVar(&sv.RPCPriority,
		"A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution."))
	r.Register("DEFAULT_ISOLATION_LEVEL", IsolationLevelVar(&sv.DefaultIsolationLevel,
		"The transaction isolation level that is used by default for read/write transactions."))
	r.Register("CLI_DATABASE_DIALECT", DatabaseDialectVar(&sv.DatabaseDialect,
		"Database dialect for the session."))
	r.Register("CLI_QUERY_MODE", &CustomVar{
		base: StringVar(&sv.AnalyzeColumns, "Query execution mode."), // dummy for description
		customGetter: func() (string, error) {
			if sv.QueryMode == nil {
				return "NULL", nil
			}
			// Create temporary variable for QueryModeVar
			mode := *sv.QueryMode
			return QueryModeVar(&mode, "").Get()
		},
		customSetter: func(value string) error {
			if strings.EqualFold(value, "NULL") {
				sv.QueryMode = nil
				return nil
			}
			if sv.QueryMode == nil {
				sv.QueryMode = new(sppb.ExecuteSqlRequest_QueryMode)
			}
			// Create temporary variable for QueryModeVar
			mode := *sv.QueryMode
			err := QueryModeVar(&mode, "").Set(value)
			if err != nil {
				return err
			}
			*sv.QueryMode = mode
			return nil
		},
	})

	// === Custom enum types (4+) ===
	r.Register("AUTOCOMMIT_DML_MODE", AutocommitDMLModeVar(&sv.AutocommitDMLMode,
		"A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements."))
	r.Register("CLI_FORMAT", DisplayModeVar(&sv.CLIFormat,
		"Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated), HTML (HTML table), XML (XML format), CSV (comma-separated values)."))
	r.Register("CLI_PARSE_MODE", ParseModeVar(&sv.BuildStatementMode,
		"Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED"))
	r.Register("CLI_EXPLAIN_FORMAT", &CustomVar{
		base: ExplainFormatVar(&sv.ExplainFormat,
			"Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation."),
		customSetter: func(value string) error {
			if value == "" {
				sv.ExplainFormat = explainFormatUnspecified
				return nil
			}
			return ExplainFormatVar(&sv.ExplainFormat, "").Set(value)
		},
	})
	r.Register("CLI_LOG_LEVEL", &LogLevelVar{
		ptr:         &sv.LogLevel,
		description: "Log level for the CLI.",
	})

	// === Computed/Read-only variables (3+) ===
	r.Register("CLI_VERSION", NewReadOnlyVar(getVersion, "The version of spanner-mycli."))
	r.Register("CLI_CURRENT_WIDTH", NewReadOnlyVar(func() string {
		if sv.StreamManager != nil {
			return sv.StreamManager.GetTerminalWidthString()
		}
		return "NULL"
	}, "Current terminal width. Returns NULL if not connected to a terminal."))
	r.Register("READ_TIMESTAMP", &TimestampVar{
		ptr:         &sv.ReadTimestamp,
		description: "The read timestamp of the most recent read-only transaction.",
	})
	r.Register("COMMIT_TIMESTAMP", &TimestampVar{
		ptr:         &sv.CommitTimestamp,
		description: "The commit timestamp of the last read-write transaction that Spanner committed.",
	})

	// === Complex variables (10+) ===
	r.Register("READ_ONLY_STALENESS", &TimestampBoundVar{
		ptr:         &sv.ReadOnlyStaleness,
		description: "A property of type STRING for read-only transactions with flexible staleness.",
	})

	protoDescVar := &ProtoDescriptorVar{
		filesPtr:      &sv.ProtoDescriptorFile,
		descriptorPtr: &sv.ProtoDescriptor,
		description:   "Comma-separated list of proto descriptor files. Supports ADD to append files.",
	}
	r.RegisterWithAdd("CLI_PROTO_DESCRIPTOR_FILE", protoDescVar, protoDescVar.Add)

	r.Register("CLI_ENDPOINT", &EndpointVar{
		hostPtr:     &sv.Host,
		portPtr:     &sv.Port,
		description: "Host and port for connections (host:port format).",
	})

	r.Register("CLI_OUTPUT_TEMPLATE_FILE", &CustomVar{
		base: StringVar(&sv.OutputTemplateFile, "Go text/template for formatting the output of the CLI."),
		customGetter: func() (string, error) {
			return sv.OutputTemplateFile, nil
		},
		customSetter: func(value string) error {
			// Parse and set template
			if value == "" || strings.EqualFold(value, "NULL") {
				sv.OutputTemplateFile = ""
				sv.OutputTemplate = nil
				return nil
			}

			tmpl, err := parseOutputTemplate(value)
			if err != nil {
				return err
			}
			sv.OutputTemplateFile = value
			sv.OutputTemplate = tmpl
			return nil
		},
	})

	r.Register("CLI_ANALYZE_COLUMNS", &TemplateVar{
		stringPtr: &sv.AnalyzeColumns,
		parsedPtr: &sv.ParsedAnalyzeColumns,
		parseFunc: func(value string) error {
			parsed, err := parseAnalyzeColumns(value)
			if err != nil {
				return err
			}
			sv.ParsedAnalyzeColumns = parsed
			return nil
		},
		description: "Go template for analyzing column data.",
	})

	r.Register("CLI_INLINE_STATS", &TemplateVar{
		stringPtr: &sv.InlineStats,
		parsedPtr: &sv.ParsedInlineStats,
		parseFunc: func(value string) error {
			parsed, err := parseInlineStats(value)
			if err != nil {
				return err
			}
			sv.ParsedInlineStats = parsed
			return nil
		},
		description: "<name>:<template>, ...",
	})

	r.Register("CLI_ENABLE_ADC_PLUS", &CustomVar{
		base: BoolVar(&sv.EnableADCPlus, "A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true."),
		customSetter: func(value string) error {
			if sv.CurrentSession != nil {
				return fmt.Errorf("CLI_ENABLE_ADC_PLUS cannot be changed after session creation")
			}
			b, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			sv.EnableADCPlus = b
			return nil
		},
	})

	r.Register("CLI_MCP", BoolVar(&sv.MCP, "A read-only boolean indicating whether the connection is running as an MCP server.").AsReadOnly())
	r.Register("CLI_INSECURE", BoolVar(&sv.Insecure, "Skip TLS certificate verification (insecure).").AsReadOnly())
	r.Register("CLI_LOG_GRPC", BoolVar(&sv.LogGrpc, "Enable gRPC logging.").AsReadOnly())

	// === Unimplemented variables ===
	r.Register("AUTOCOMMIT", &UnimplementedVar{
		name:        "AUTOCOMMIT",
		description: "A boolean indicating whether or not the connection is in autocommit mode. The default is true.",
	})
	r.Register("RETRY_ABORTS_INTERNALLY", &UnimplementedVar{
		name:        "RETRY_ABORTS_INTERNALLY",
		description: "A boolean indicating whether the connection automatically retries aborted transactions. The default is true.",
	})

	// Note: COMMIT_RESPONSE and CLI_DIRECT_READ require special handling outside the registry
}

// parseGoogleSQLValue parses GoogleSQL-style values using memefish
func parseGoogleSQLValue(value string) (result string) {
	value = strings.TrimSpace(value)

	// Protect against panics from memefish.
	// While memefish.ParseExpr normally returns errors, it panics in some cases:
	// - Unclosed string literals (e.g., 'hello or "world)
	// - Unclosed triple-quoted strings (e.g., ''')
	// Without this recovery, entering an unclosed string in SET statements
	// would crash spanner-mycli entirely.
	defer func() {
		if r := recover(); r != nil {
			// If memefish panics, return the original value
			result = value
		}
	}()

	// Try to parse as an expression using memefish
	expr, err := memefish.ParseExpr("", value)
	if err != nil {
		// If parsing fails, return the original value
		return value
	}

	// Handle different literal types
	switch lit := expr.(type) {
	case *ast.StringLiteral:
		return lit.Value
	case *ast.BoolLiteral:
		return strconv.FormatBool(lit.Value)
	default:
		// For other expressions, return the original value
		return value
	}
}
