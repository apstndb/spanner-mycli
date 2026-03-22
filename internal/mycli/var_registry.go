package mycli

import (
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
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

// GetVariable retrieves the Variable handler by name, or nil if not found.
func (r *VarRegistry) GetVariable(name string) Variable {
	return r.vars[strings.ToUpper(name)]
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
	upperName := strings.ToUpper(name)
	v, ok := r.vars[upperName]
	if !ok {
		slog.Debug("Variable not found in registry", "name", upperName, "availableVars", maps.Keys(r.vars))
		return &ErrUnknownVariable{Name: name}
	}

	// Parse GoogleSQL value if needed
	originalValue := value
	if isGoogleSQL {
		value = parseGoogleSQLValue(value)
	}

	slog.Debug("Registry.Set", "name", upperName, "originalValue", originalValue, "parsedValue", value, "isGoogleSQL", isGoogleSQL)

	err := v.Set(value)
	slog.Debug("Registry.Set result", "name", upperName, "err", err)
	return err
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
		base: BoolVar(&sv.Transaction.ReadOnly, "A boolean indicating whether or not the connection is in read-only mode"),
		customSetter: func(value string) error {
			// Custom validation for READONLY
			if sv.inTransaction != nil && sv.inTransaction() {
				return errSetterInTransaction
			}
			b, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			sv.Transaction.ReadOnly = b
			return nil
		},
	})

	r.Register("AUTO_PARTITION_MODE", BoolVar(&sv.Query.AutoPartitionMode,
		"A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed."))
	r.Register("DATA_BOOST_ENABLED", BoolVar(&sv.Query.DataBoostEnabled,
		"A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false."))
	r.Register("AUTO_BATCH_DML", BoolVar(&sv.Transaction.AutoBatchDML,
		"A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false."))
	r.Register("EXCLUDE_TXN_FROM_CHANGE_STREAMS", BoolVar(&sv.Transaction.ExcludeTxnFromChangeStreams,
		"Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true)."))
	r.Register("RETURN_COMMIT_STATS", BoolVar(&sv.Transaction.ReturnCommitStats,
		"A property of type BOOL indicating whether statistics should be returned for transactions on this connection."))
	r.Register("CLI_VERBOSE", BoolVar(&sv.Display.Verbose, "Display verbose output."))
	r.Register("CLI_PROFILE", BoolVar(&sv.Query.Profile, "Enable performance profiling (memory and timing metrics)."))
	r.Register("CLI_LINT_PLAN", BoolVar(&sv.Query.LintPlan, "Enable query plan linting."))
	r.Register("CLI_USE_PAGER", BoolVar(&sv.Display.UsePager, "Enable pager for output."))
	r.Register("CLI_AUTOWRAP", BoolVar(&sv.Display.AutoWrap, "Enable automatic line wrapping."))
	r.Register("CLI_ENABLE_HIGHLIGHT", BoolVar(&sv.Display.EnableHighlight, "Enable syntax highlighting."))
	r.Register("CLI_PROTOTEXT_MULTILINE", BoolVar(&sv.Display.MultilineProtoText, "Enable multiline prototext output."))
	r.Register("CLI_MARKDOWN_CODEBLOCK", BoolVar(&sv.Display.MarkdownCodeblock, "Enable markdown codeblock output."))
	r.Register("CLI_TRY_PARTITION_QUERY", BoolVar(&sv.Query.TryPartitionQuery,
		"A boolean indicating whether to test query for partition compatibility instead of executing it."))
	r.Register("CLI_ECHO_EXECUTED_DDL", BoolVar(&sv.Feature.EchoExecutedDDL, "Echo executed DDL statements."))
	r.Register("CLI_ECHO_INPUT", BoolVar(&sv.Feature.EchoInput, "Echo input statements."))
	r.Register("CLI_AUTO_CONNECT_AFTER_CREATE", BoolVar(&sv.Feature.AutoConnectAfterCreate,
		"A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false."))
	r.Register("CLI_ENABLE_PROGRESS_BAR", BoolVar(&sv.Display.EnableProgressBar,
		"A boolean indicating whether to display progress bars during operations. The default is false."))
	r.Register("CLI_ASYNC_DDL", BoolVar(&sv.Feature.AsyncDDL,
		"A boolean indicating whether DDL statements should be executed asynchronously. The default is false."))
	r.Register("CLI_SKIP_SYSTEM_COMMAND", BoolVar(&sv.Feature.SkipSystemCommand,
		"Controls whether system commands are disabled."))
	r.Register("CLI_SKIP_COLUMN_NAMES", BoolVar(&sv.Display.SkipColumnNames,
		"A boolean indicating whether to suppress column headers in output. The default is false."))
	r.Register("CLI_FUZZY_FINDER_KEY", StringVar(&sv.Feature.FuzzyFinderKey,
		"Key binding for fuzzy finder. Uses go-readline-ny key names (e.g., C_T, M_F, F1). Set to empty string to disable. The default is C_T (Ctrl+T)."))
	r.Register("CLI_FUZZY_FINDER_OPTIONS", StringVar(&sv.Feature.FuzzyFinderOptions,
		"Additional fzf options passed to the fuzzy finder. Appended after built-in defaults, so user options take precedence. Example: --color=dark --no-select-1"))
	r.Register("CLI_STREAMING", StreamingModeVar(&sv.Query.StreamingMode,
		"Controls streaming output mode: AUTO (format-dependent), TRUE (always stream), FALSE (never stream). Default is AUTO."))
	r.Register("CLI_STYLED_OUTPUT", StyledModeVar(&sv.Display.StyledOutput,
		"Controls ANSI styling in table output: AUTO (styled if TTY), TRUE (always styled), FALSE (never styled). Default is AUTO."))
	r.Register("CLI_WIDTH_STRATEGY", WidthStrategyVar(&sv.Display.WidthStrategy,
		"Controls column width allocation algorithm: GREEDY_FREQUENCY (default, frequency-based greedy), PROPORTIONAL (proportional to natural width), MARGINAL_COST (wrap-line minimization via max-heap)."))

	// === String variables (15+) ===
	r.Register("OPTIMIZER_VERSION", StringVar(&sv.Query.OptimizerVersion,
		"A property of type STRING indicating the optimizer version. The version is either an integer string or LATEST."))
	r.Register("OPTIMIZER_STATISTICS_PACKAGE", StringVar(&sv.Query.OptimizerStatisticsPackage,
		"A property of type STRING indicating the current optimizer statistics package that is used by this connection."))
	r.Register("TRANSACTION_TAG", StringVar(&sv.Transaction.TransactionTag,
		"A property of type STRING that contains the transaction tag for the next transaction."))
	r.Register("STATEMENT_TAG", StringVar(&sv.Transaction.RequestTag,
		"A property of type STRING that contains the request tag for the next statement."))
	r.Register("CLI_PROJECT", StringVar(&sv.Connection.Project, "GCP Project ID.").AsReadOnly())
	r.Register("CLI_INSTANCE", StringVar(&sv.Connection.Instance, "Cloud Spanner instance ID.").AsReadOnly())
	r.Register("CLI_DATABASE", StringVar(&sv.Connection.Database, "Cloud Spanner database ID.").AsReadOnly())
	r.Register("CLI_PROMPT", StringVar(&sv.Display.Prompt, "Custom prompt for spanner-mycli."))
	r.Register("CLI_PROMPT2", &CustomVar{
		base: StringVar(&sv.Display.Prompt2, "Custom continuation prompt for spanner-mycli."),
		customSetter: func(value string) error {
			if value == "" {
				return fmt.Errorf("CLI_PROMPT2 cannot be empty")
			}
			sv.Display.Prompt2 = value
			return nil
		},
	})
	r.Register("CLI_HISTORY_FILE", StringVar(&sv.Display.HistoryFile, "Path to the history file.").AsReadOnly())
	r.Register("CLI_VERTEXAI_PROJECT", StringVar(&sv.Feature.VertexAIProject, "Vertex AI project for natural language features."))
	r.Register("CLI_VERTEXAI_MODEL", StringVar(&sv.Feature.VertexAIModel, "Vertex AI model for natural language features."))
	r.Register("CLI_VERTEXAI_LOCATION", StringVar(&sv.Feature.VertexAILocation, "Vertex AI location for natural language features."))
	r.Register("CLI_ROLE", StringVar(&sv.Connection.Role, "Cloud Spanner database role.").AsReadOnly())
	r.Register("CLI_HOST", StringVar(&sv.Connection.Host, "Host on which Spanner server is located").AsReadOnly())
	r.Register("CLI_EMULATOR_PLATFORM", StringVar(&sv.Connection.EmulatorPlatform, "Container platform used by embedded emulator.").AsReadOnly())
	r.Register("CLI_IMPERSONATE_SERVICE_ACCOUNT", StringVar(&sv.Connection.ImpersonateServiceAccount, "Service account to impersonate.").AsReadOnly())

	// === Integer variables (10+) ===
	r.Register("MAX_PARTITIONED_PARALLELISM", IntVar(&sv.Query.MaxPartitionedParallelism,
		"A property of type INT64 indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for AUTO_PARTITION_MODE=TRUE and RUN PARTITIONED QUERY"))
	r.Register("CLI_TAB_WIDTH", IntVar(&sv.Display.TabWidth, "Tab width. It is used for expanding tabs."))
	r.Register("CLI_TAB_VISUALIZE", BoolVar(&sv.Display.TabVisualize, "Visualize tab characters with arrow symbol in table output."))
	r.Register("CLI_EXPLAIN_WRAP_WIDTH", IntVar(&sv.Display.ExplainWrapWidth,
		"Controls query plan wrap width. It effects only operators column contents"))
	r.Register("CLI_TABLE_PREVIEW_ROWS", IntVar(&sv.Query.TablePreviewRows,
		"Number of rows to preview for table width calculation in streaming mode. 0 means use header widths only. Positive values use that many rows for preview (default: 50). -1 means collect all rows (non-streaming)."))
	r.Register("CLI_SQL_TABLE_NAME", StringVar(&sv.Display.SQLTableName,
		"Table name for generated SQL statements. Required for SQL export formats. Supports both simple names (e.g., 'Users') and schema-qualified names (e.g., 'myschema.Users')."))
	r.Register("CLI_SQL_BATCH_SIZE", IntVar(&sv.Display.SQLBatchSize,
		"Number of VALUES per INSERT statement for SQL export. 0 (default): single-row INSERT statements. 2+: multi-row INSERT with up to N rows per statement."))
	r.Register("CLI_SUPPRESS_RESULT_LINES", BoolVar(&sv.Display.SuppressResultLines,
		"Suppress result lines like 'rows in set' for clean output. Useful for scripting and dump operations."))
	r.Register("CLI_PORT", &IntGetterVar{
		getter:      func() int64 { return int64(sv.Connection.Port) },
		description: "Port number for connections.",
	})

	// === Nullable types (5+) ===
	r.Register("MAX_COMMIT_DELAY", NullableDurationVar(&sv.Transaction.MaxCommitDelay,
		"The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.").
		WithValidator(durationValidator(durationPtr(0), durationPtr(500*time.Millisecond))))
	r.Register("STATEMENT_TIMEOUT", NullableDurationVar(&sv.Query.StatementTimeout,
		"A property of type STRING indicating the current timeout value for statements (e.g., 10s, 5m, 1h). Default is 10m.").
		WithValidator(durationValidator(durationPtr(0), nil)))
	r.Register("CLI_FIXED_WIDTH", NullableIntVar(&sv.Display.FixedWidth,
		"If set, limits output width to the specified number of characters. NULL means automatic width detection."))

	// === Proto Enum types (5) ===
	r.Register("RPC_PRIORITY", RPCPriorityVar(&sv.Query.RPCPriority,
		"A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution."))
	r.Register("DEFAULT_ISOLATION_LEVEL", IsolationLevelVar(&sv.Transaction.DefaultIsolationLevel,
		"The transaction isolation level that is used by default for read/write transactions."))
	r.Register("READ_LOCK_MODE", ReadLockModeVar(&sv.Transaction.ReadLockMode,
		"The read lock mode for read/write transactions. OPTIMISTIC uses optimistic concurrency control; PESSIMISTIC uses pessimistic locking. Default is UNSPECIFIED (server default)."))

	r.Register("CLI_DATABASE_DIALECT", DatabaseDialectVar(&sv.Feature.DatabaseDialect,
		"Database dialect for the session."))
	r.Register("CLI_QUERY_MODE", &CustomVar{
		base: QueryModeVar(func() *sppb.ExecuteSqlRequest_QueryMode {
			// Use a zero-value pointer just for ValidValues() / description;
			// actual get/set are handled by custom getter/setter below.
			var mode sppb.ExecuteSqlRequest_QueryMode
			return &mode
		}(), "Query execution mode."),
		customGetter: func() (string, error) {
			if sv.Query.QueryMode == nil {
				return "NULL", nil
			}
			mode := *sv.Query.QueryMode
			return QueryModeVar(&mode, "").Get()
		},
		customSetter: func(value string) error {
			if strings.EqualFold(value, "NULL") {
				sv.Query.QueryMode = nil
				return nil
			}
			if sv.Query.QueryMode == nil {
				sv.Query.QueryMode = new(sppb.ExecuteSqlRequest_QueryMode)
			}
			mode := *sv.Query.QueryMode
			err := QueryModeVar(&mode, "").Set(value)
			if err != nil {
				return err
			}
			*sv.Query.QueryMode = mode
			return nil
		},
	})

	// === Custom enum types (4+) ===
	r.Register("AUTOCOMMIT_DML_MODE", AutocommitDMLModeVar(&sv.Transaction.AutocommitDMLMode,
		"A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements."))
	r.Register("CLI_FORMAT", DisplayModeVar(&sv.Display.CLIFormat,
		"Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated), HTML (HTML table), XML (XML format), CSV (comma-separated values)."))
	r.Register("CLI_PARSE_MODE", ParseModeVar(&sv.Query.BuildStatementMode,
		"Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED"))
	r.Register("CLI_EXPLAIN_FORMAT", &CustomVar{
		base: ExplainFormatVar(&sv.Display.ExplainFormat,
			"Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation."),
		customSetter: func(value string) error {
			if value == "" {
				sv.Display.ExplainFormat = enums.ExplainFormatUnspecified
				return nil
			}
			return ExplainFormatVar(&sv.Display.ExplainFormat, "").Set(value)
		},
	})
	r.Register("CLI_LOG_LEVEL", &LogLevelVar{
		ptr:         &sv.Feature.LogLevel,
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
		ptr:         &sv.Query.ReadTimestamp,
		description: "The read timestamp of the most recent read-only transaction.",
	})
	r.Register("COMMIT_TIMESTAMP", &TimestampVar{
		ptr:         &sv.Transaction.CommitTimestamp,
		description: "The commit timestamp of the last read-write transaction that Spanner committed.",
	})

	// === Complex variables (10+) ===
	r.Register("READ_ONLY_STALENESS", &TimestampBoundVar{
		ptr:         &sv.Query.ReadOnlyStaleness,
		description: "A property of type STRING for read-only transactions with flexible staleness.",
	})

	protoDescVar := &ProtoDescriptorVar{
		filesPtr:      &sv.Internal.ProtoDescriptorFile,
		descriptorPtr: &sv.Internal.ProtoDescriptor,
		description:   "Comma-separated list of proto descriptor files. Supports ADD to append files.",
	}
	r.RegisterWithAdd("CLI_PROTO_DESCRIPTOR_FILE", protoDescVar, protoDescVar.Add)

	r.Register("CLI_TYPE_STYLES", &CustomVar{
		base: StringVar(&sv.Display.TypeStylesRaw,
			"Type-based ANSI styling for query results. Format: colon-separated TYPE=STYLE pairs (e.g., 'STRING=green:INT64=bold:NULL=dim'). "+
				"Supports named colors (red, green, yellow, blue, magenta, cyan, white, black), "+
				"attributes (bold, dim, italic, underline, reverse, strikethrough), "+
				"and raw SGR numbers (e.g., 38;5;214 for 256-color). "+
				"NULL key overrides the default dim style for NULL values. Empty string disables type styling."),
		customGetter: func() (string, error) {
			return sv.Display.TypeStylesRaw, nil
		},
		customSetter: func(value string) error {
			if strings.EqualFold(value, "NULL") {
				value = ""
			}
			config, err := parseTypeStyles(value)
			if err != nil {
				return err
			}
			sv.Display.TypeStylesRaw = value
			sv.typeStyles = config.typeStyles
			sv.nullStyle = config.nullStyle
			return nil
		},
	})

	r.Register("CLI_ENDPOINT", &EndpointVar{
		hostPtr:     &sv.Connection.Host,
		portPtr:     &sv.Connection.Port,
		description: "Host and port for connections (host:port format).",
	})

	r.Register("CLI_OUTPUT_TEMPLATE_FILE", &CustomVar{
		base: StringVar(&sv.Display.OutputTemplateFile, "Go text/template for formatting the output of the CLI."),
		customGetter: func() (string, error) {
			return sv.Display.OutputTemplateFile, nil
		},
		customSetter: func(value string) error {
			// Parse and set template
			if value == "" || strings.EqualFold(value, "NULL") {
				sv.Display.OutputTemplateFile = ""
				sv.Display.OutputTemplate = nil
				return nil
			}

			tmpl, err := parseOutputTemplate(value)
			if err != nil {
				return err
			}
			sv.Display.OutputTemplateFile = value
			sv.Display.OutputTemplate = tmpl
			return nil
		},
	})

	r.Register("CLI_ANALYZE_COLUMNS", &TemplateVar{
		stringPtr: &sv.Display.AnalyzeColumns,
		parsedPtr: &sv.Display.ParsedAnalyzeColumns,
		parseFunc: func(value string) error {
			parsed, err := parseAnalyzeColumns(value)
			if err != nil {
				return err
			}
			sv.Display.ParsedAnalyzeColumns = parsed
			return nil
		},
		description: "Go template for analyzing column data.",
	})

	r.Register("CLI_INLINE_STATS", &TemplateVar{
		stringPtr: &sv.Display.InlineStats,
		parsedPtr: &sv.Display.ParsedInlineStats,
		parseFunc: func(value string) error {
			parsed, err := parseInlineStats(value)
			if err != nil {
				return err
			}
			sv.Display.ParsedInlineStats = parsed
			return nil
		},
		description: "<name>:<template>, ...",
	})

	// CLI_ENABLE_ADC_PLUS is a session-init-only variable.
	// Currently implemented using a custom setter that checks inTransaction != nil
	// to detect whether a session has been created. Could use native support
	// for session-init-only behavior if added to VarHandler (see TODO in var_handler.go).
	r.Register("CLI_ENABLE_ADC_PLUS", &CustomVar{
		base: BoolVar(&sv.Connection.EnableADCPlus, "A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true."),
		customSetter: func(value string) error {
			if sv.inTransaction != nil {
				return fmt.Errorf("CLI_ENABLE_ADC_PLUS cannot be changed after session creation")
			}
			b, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			sv.Connection.EnableADCPlus = b
			return nil
		},
	})

	r.Register("CLI_MCP", BoolVar(&sv.Feature.MCP, "A read-only boolean indicating whether the connection is running as an MCP server.").AsReadOnly())
	r.Register("CLI_INSECURE", BoolVar(&sv.Connection.Insecure, "Skip TLS certificate verification (insecure).").AsReadOnly())
	r.Register("CLI_LOG_GRPC", BoolVar(&sv.Feature.LogGrpc, "Enable gRPC logging.").AsReadOnly())

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
