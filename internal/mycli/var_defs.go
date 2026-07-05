package mycli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

// varScope classifies where a system variable's value lives and, together with
// readOnly, whether SET may change it.
type varScope int

const (
	// scopeSession is the SET-able surface. (Later: participates in RESET ALL
	// and SET LOCAL.)
	scopeSession varScope = iota
	// scopeStartup is StartupConfig-backed: read-only via SET, written only by
	// config.go/app.go before session creation.
	scopeStartup
	// scopeConnection is connection identity: read-only via SET, mutated only by
	// USE/DETACH.
	scopeConnection
	// scopeResult is last-statement output (LastResult): read-only via SET.
	scopeResult
)

// varDef is the declarative metadata for one system variable. The varDefs
// table is the single source of truth for a variable's name, description,
// read-only/scope policy, and how to construct its live handler; registerAll
// iterates it to populate the registry.
type varDef struct {
	name string
	desc string

	scope    varScope
	readOnly bool // for scopeSession exceptions only; other scopes are implicitly read-only

	// bind constructs the live handler bound to the given systemVariables.
	bind func(sv *systemVariables) Variable
	// bindAdd, when non-nil, constructs the variable's ADD handler.
	bindAdd func(sv *systemVariables) func(string) error
}

// settable reports whether SET may change this variable. Non-session scopes are
// implicitly read-only; scopeSession vars are settable unless readOnly is set.
func (d *varDef) settable() bool { return d.scope == scopeSession && !d.readOnly }

// varDefs is the declarative table of all system variables. Order is not
// significant (listings sort by name); it mirrors the historical grouping for
// readability.
//
// Note: COMMIT_RESPONSE and CLI_DIRECT_READ require special handling outside
// the registry (see system_variables_registry.go).
var varDefs = []varDef{
	// === Simple boolean variables ===
	{
		name:  "READONLY",
		desc:  "A boolean indicating whether or not the connection is in read-only mode",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: BoolVar(&sv.Transaction.ReadOnly),
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
			}
		},
	},
	{
		name:  "AUTO_PARTITION_MODE",
		desc:  "A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Query.AutoPartitionMode) },
	},
	{
		name:  "DATA_BOOST_ENABLED",
		desc:  "A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Query.DataBoostEnabled) },
	},
	{
		name:  "AUTO_BATCH_DML",
		desc:  "A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Transaction.AutoBatchDML) },
	},
	{
		name:  "EXCLUDE_TXN_FROM_CHANGE_STREAMS",
		desc:  "Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Transaction.ExcludeTxnFromChangeStreams) },
	},
	{
		name:  "RETURN_COMMIT_STATS",
		desc:  "A property of type BOOL indicating whether statistics should be returned for transactions on this connection.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Transaction.ReturnCommitStats) },
	},
	{
		name:  "CLI_VERBOSE",
		desc:  "Display verbose output.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.Verbose) },
	},
	{
		name:  "CLI_PROFILE",
		desc:  "Enable performance profiling (memory and timing metrics).",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Query.Profile) },
	},
	{
		name:  "CLI_LINT_PLAN",
		desc:  "Enable query plan linting.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Query.LintPlan) },
	},
	{
		name:  "CLI_USE_PAGER",
		desc:  "Enable pager for output.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.UsePager) },
	},
	{
		name:  "CLI_AUTOWRAP",
		desc:  "Enable automatic line wrapping.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.AutoWrap) },
	},
	{
		name:  "CLI_ENABLE_HIGHLIGHT",
		desc:  "Enable syntax highlighting.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.EnableHighlight) },
	},
	{
		name:  "CLI_PROTOTEXT_MULTILINE",
		desc:  "Enable multiline prototext output.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.MultilineProtoText) },
	},
	{
		name:  "CLI_MARKDOWN_CODEBLOCK",
		desc:  "Enable markdown codeblock output.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.MarkdownCodeblock) },
	},
	{
		name:  "CLI_TRY_PARTITION_QUERY",
		desc:  "A boolean indicating whether to test query for partition compatibility instead of executing it.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Query.TryPartitionQuery) },
	},
	{
		name:  "CLI_ECHO_EXECUTED_DDL",
		desc:  "Echo executed DDL statements.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Feature.EchoExecutedDDL) },
	},
	{
		name:  "CLI_ECHO_INPUT",
		desc:  "Echo input statements.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Feature.EchoInput) },
	},
	{
		name:  "CLI_AUTO_CONNECT_AFTER_CREATE",
		desc:  "A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Feature.AutoConnectAfterCreate) },
	},
	{
		name:  "CLI_ENABLE_PROGRESS_BAR",
		desc:  "A boolean indicating whether to display progress bars during operations. The default is false.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.EnableProgressBar) },
	},
	{
		name:  "CLI_ASYNC_DDL",
		desc:  "A boolean indicating whether DDL statements should be executed asynchronously. The default is false.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Feature.AsyncDDL) },
	},
	{
		// Read-only: this is a security feature (--skip-system-command /
		// --system-command=OFF); if it were settable, a user in a restricted
		// environment could re-enable shell access with a single SET.
		name:  "CLI_SKIP_SYSTEM_COMMAND",
		desc:  "A read-only boolean indicating whether system commands are disabled. Set by --skip-system-command or --system-command=OFF.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.SkipSystemCommand) },
	},
	{
		name:  "CLI_TAB_VISUALIZE",
		desc:  "Visualize tab characters with arrow symbol in table output.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.TabVisualize) },
	},
	{
		name:  "CLI_SKIP_COLUMN_NAMES",
		desc:  "A boolean indicating whether to suppress column headers in output. The default is false.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.SkipColumnNames) },
	},
	{
		name:  "CLI_EXPLAIN_HANGING_INDENT",
		desc:  "Use hanging indent for wrapped query plan lines in EXPLAIN, EXPLAIN ANALYZE, and query profile rendering. Only affects output when CLI_EXPLAIN_WRAP_WIDTH or WIDTH is set.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.ExplainHangingIndent) },
	},
	{
		name:  "CLI_FUZZY_FINDER_KEY",
		desc:  "Key binding for fuzzy finder. Uses go-readline-ny key names (e.g., C_T, M_F, F1). Set to empty string to disable. The default is C_T (Ctrl+T).",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Feature.FuzzyFinderKey) },
	},
	{
		name:  "CLI_FUZZY_FINDER_OPTIONS",
		desc:  "Additional fzf options passed to the fuzzy finder. Appended after built-in defaults, so user options take precedence. Example: --color=dark --no-select-1",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Feature.FuzzyFinderOptions) },
	},
	{
		name:  "CLI_TABLE_STREAMING",
		desc:  "Controls table streaming output mode: AUTO/FALSE buffer table output for layout quality, TRUE streams table output. Non-table formats always stream. Default is AUTO.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StreamingModeVar(&sv.Query.StreamingMode) },
	},
	{
		name:  "CLI_STYLED_OUTPUT",
		desc:  "Controls ANSI styling in table output: AUTO (styled if TTY), TRUE (always styled), FALSE (never styled). Default is AUTO.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StyledModeVar(&sv.Display.StyledOutput) },
	},
	{
		name:  "CLI_WIDTH_STRATEGY",
		desc:  "Controls column width allocation algorithm: GREEDY_FREQUENCY (default, frequency-based greedy), PROPORTIONAL (proportional to natural width), MARGINAL_COST (wrap-line minimization via max-heap).",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return WidthStrategyVar(&sv.Display.WidthStrategy) },
	},

	// === String variables ===
	{
		name:  "OPTIMIZER_VERSION",
		desc:  "A property of type STRING indicating the optimizer version. The version is either an integer string or LATEST.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Query.OptimizerVersion) },
	},
	{
		name:  "OPTIMIZER_STATISTICS_PACKAGE",
		desc:  "A property of type STRING indicating the current optimizer statistics package that is used by this connection.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Query.OptimizerStatisticsPackage) },
	},
	{
		name:  "TRANSACTION_TAG",
		desc:  "A property of type STRING that contains the transaction tag for the next transaction.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Transaction.TransactionTag) },
	},
	{
		name:  "STATEMENT_TAG",
		desc:  "A property of type STRING that contains the request tag for the next statement.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Transaction.RequestTag) },
	},
	{
		name:  "CLI_PROJECT",
		desc:  "GCP Project ID.",
		scope: scopeConnection,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Connection.Project) },
	},
	{
		name:  "CLI_INSTANCE",
		desc:  "Cloud Spanner instance ID.",
		scope: scopeConnection,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Connection.Instance) },
	},
	{
		name:  "CLI_DATABASE",
		desc:  "Cloud Spanner database ID.",
		scope: scopeConnection,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Connection.Database) },
	},
	{
		name:  "CLI_PROMPT",
		desc:  "Custom prompt for spanner-mycli.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Display.Prompt) },
	},
	{
		name:  "CLI_PROMPT2",
		desc:  "Custom continuation prompt for spanner-mycli.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: StringVar(&sv.Display.Prompt2),
				customSetter: func(value string) error {
					if value == "" {
						return fmt.Errorf("CLI_PROMPT2 cannot be empty")
					}
					sv.Display.Prompt2 = value
					return nil
				},
			}
		},
	},
	{
		name:  "CLI_HISTORY_FILE",
		desc:  "Path to the history file.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Display.HistoryFile) },
	},
	{
		name:  "CLI_VERTEXAI_PROJECT",
		desc:  "Vertex AI project for natural language features.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Feature.VertexAIProject) },
	},
	{
		name:  "CLI_VERTEXAI_MODEL",
		desc:  "Vertex AI model for natural language features.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Feature.VertexAIModel) },
	},
	{
		name:  "CLI_VERTEXAI_LOCATION",
		desc:  "Vertex AI location for natural language features.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Feature.VertexAILocation) },
	},
	{
		name:  "CLI_ROLE",
		desc:  "Cloud Spanner database role.",
		scope: scopeConnection,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Connection.Role) },
	},
	{
		name:  "CLI_HOST",
		desc:  "Host on which Spanner server is located",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.Host) },
	},
	{
		name:  "CLI_EMULATOR_PLATFORM",
		desc:  "Container platform used by embedded emulator.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.EmulatorPlatform) },
	},
	{
		name:  "CLI_IMPERSONATE_SERVICE_ACCOUNT",
		desc:  "Service account to impersonate.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.ImpersonateServiceAccount) },
	},

	// === Integer variables ===
	{
		name:  "MAX_PARTITIONED_PARALLELISM",
		desc:  "A property of type INT64 indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for AUTO_PARTITION_MODE=TRUE and RUN PARTITIONED QUERY",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return IntVar(&sv.Query.MaxPartitionedParallelism) },
	},
	{
		name:  "CLI_TAB_WIDTH",
		desc:  "Tab width. It is used for expanding tabs.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return IntVar(&sv.Display.TabWidth) },
	},
	{
		name:  "CLI_EXPLAIN_WRAP_WIDTH",
		desc:  "Controls query plan wrap width. It effects only operators column contents",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return IntVar(&sv.Display.ExplainWrapWidth) },
	},
	{
		name:  "CLI_TABLE_PREVIEW_ROWS",
		desc:  "Number of rows to preview for table width calculation in streaming mode. 0 means use header widths only. Positive values use that many rows for preview (default: 50). -1 means collect all rows (non-streaming).",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return IntVar(&sv.Query.TablePreviewRows) },
	},
	{
		name:  "CLI_SQL_TABLE_NAME",
		desc:  "Table name for generated SQL statements. Required for SQL export formats. Supports both simple names (e.g., 'Users') and schema-qualified names (e.g., 'myschema.Users').",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Display.SQLTableName) },
	},
	{
		name:  "CLI_SQL_BATCH_SIZE",
		desc:  "Number of VALUES per INSERT statement for SQL export. 0 (default): single-row INSERT statements. 2+: multi-row INSERT with up to N rows per statement.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return IntVar(&sv.Display.SQLBatchSize) },
	},
	{
		name:  "CLI_SUPPRESS_RESULT_LINES",
		desc:  "Suppress result lines like 'rows in set' for clean output. Useful for scripting and dump operations.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Display.SuppressResultLines) },
	},
	{
		name:  "CLI_PORT",
		desc:  "Port number for connections.",
		scope: scopeStartup,
		bind: func(sv *systemVariables) Variable {
			return &IntGetterVar{getter: func() int64 { return int64(sv.Config.Port) }}
		},
	},

	// === Nullable types ===
	{
		name:  "MAX_COMMIT_DELAY",
		desc:  "The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return NullableDurationVar(&sv.Transaction.MaxCommitDelay).
				WithValidator(durationValidator(durationPtr(0), durationPtr(500*time.Millisecond)))
		},
	},
	{
		name:  "STATEMENT_TIMEOUT",
		desc:  "A property of type STRING indicating the current timeout value for statements (e.g., 10s, 5m, 1h). Default is 10m.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return NullableDurationVar(&sv.Query.StatementTimeout).
				WithValidator(durationValidator(durationPtr(0), nil))
		},
	},
	{
		name:  "CLI_FIXED_WIDTH",
		desc:  "If set, limits output width to the specified number of characters. NULL means automatic width detection.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return NullableIntVar(&sv.Display.FixedWidth) },
	},

	// === Proto Enum types ===
	{
		name:  "RPC_PRIORITY",
		desc:  "A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return RPCPriorityVar(&sv.Query.RPCPriority) },
	},
	{
		name:  "DEFAULT_ISOLATION_LEVEL",
		desc:  "The transaction isolation level that is used by default for read/write transactions.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return IsolationLevelVar(&sv.Transaction.DefaultIsolationLevel) },
	},
	{
		name:  "READ_LOCK_MODE",
		desc:  "The read lock mode for read/write transactions. OPTIMISTIC uses optimistic concurrency control; PESSIMISTIC uses pessimistic locking. Default is UNSPECIFIED (server default).",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return ReadLockModeVar(&sv.Transaction.ReadLockMode) },
	},
	{
		name:  "CLI_DATABASE_DIALECT",
		desc:  "Database dialect for the session.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return DatabaseDialectVar(&sv.Feature.DatabaseDialect) },
	},
	{
		name:  "CLI_QUERY_MODE",
		desc:  "Query execution mode.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: QueryModeVar(func() *sppb.ExecuteSqlRequest_QueryMode {
					// Use a zero-value pointer just for ValidValues() / description;
					// actual get/set are handled by custom getter/setter below.
					var mode sppb.ExecuteSqlRequest_QueryMode
					return &mode
				}()),
				customGetter: func() (string, error) {
					if sv.Query.QueryMode == nil {
						return "NULL", nil
					}
					mode := *sv.Query.QueryMode
					return QueryModeVar(&mode).Get()
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
					err := QueryModeVar(&mode).Set(value)
					if err != nil {
						return err
					}
					*sv.Query.QueryMode = mode
					return nil
				},
			}
		},
	},

	// === Custom enum types ===
	{
		name:  "AUTOCOMMIT_DML_MODE",
		desc:  "A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return AutocommitDMLModeVar(&sv.Transaction.AutocommitDMLMode) },
	},
	{
		name:  "CLI_FORMAT",
		desc:  "Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated, raw values), TSV (tab-separated with tab/newline/carriage-return/backslash escaping), HTML (HTML table), XML (XML format), CSV (comma-separated values).",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return DisplayModeVar(&sv.Display.CLIFormat) },
	},
	{
		name:  "CLI_PARSE_MODE",
		desc:  "Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return ParseModeVar(&sv.Query.BuildStatementMode) },
	},
	{
		name:  "CLI_EXPLAIN_FORMAT",
		desc:  "Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: ExplainFormatVar(&sv.Display.ExplainFormat),
				customSetter: func(value string) error {
					if value == "" {
						sv.Display.ExplainFormat = enums.ExplainFormatUnspecified
						return nil
					}
					return ExplainFormatVar(&sv.Display.ExplainFormat).Set(value)
				},
			}
		},
	},
	{
		name:  "CLI_EXPLAIN_PRINT_SECTIONS",
		desc:  "Query plan appendix preset or comma-separated sections to print. Presets: basic, enhanced, full, none. Sections: predicates, ordering, aggregate, typed, full. Empty string suppresses appendices.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: StringVar(&sv.Display.ExplainPrintSections),
				customSetter: func(value string) error {
					sections, err := parseExplainPrintSections(value)
					if err != nil {
						return err
					}
					sv.Display.ExplainPrintSections = value
					sv.Display.ParsedExplainPrintSections = sections
					return nil
				},
			}
		},
	},
	{
		name:  "CLI_LOG_LEVEL",
		desc:  "Log level for the CLI.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return &LogLevelVar{ptr: &sv.Feature.LogLevel} },
	},

	// === Computed/Read-only variables ===
	{
		name:  "CLI_VERSION",
		desc:  "The version of spanner-mycli.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return NewReadOnlyVar(getVersion) },
	},
	{
		name:  "CLI_CURRENT_WIDTH",
		desc:  "Current terminal width. Returns NULL if not connected to a terminal.",
		scope: scopeStartup,
		bind: func(sv *systemVariables) Variable {
			return NewReadOnlyVar(func() string {
				if sv.StreamManager != nil {
					return sv.StreamManager.GetTerminalWidthString()
				}
				return "NULL"
			})
		},
	},
	{
		name:  "READ_TIMESTAMP",
		desc:  "The read timestamp of the most recent read-only transaction.",
		scope: scopeResult,
		bind:  func(sv *systemVariables) Variable { return &TimestampVar{ptr: &sv.LastResult.ReadTimestamp} },
	},
	{
		name:  "COMMIT_TIMESTAMP",
		desc:  "The commit timestamp of the last read-write transaction that Spanner committed.",
		scope: scopeResult,
		bind:  func(sv *systemVariables) Variable { return &TimestampVar{ptr: &sv.LastResult.CommitTimestamp} },
	},

	// === Complex variables ===
	{
		name:  "READ_ONLY_STALENESS",
		desc:  "A property of type STRING for read-only transactions with flexible staleness.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return &TimestampBoundVar{ptr: &sv.Query.ReadOnlyStaleness} },
	},
	{
		name:  "CLI_PROTO_DESCRIPTOR_FILE",
		desc:  "Comma-separated list of proto descriptor files. Supports ADD to append files.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &ProtoDescriptorVar{
				filesPtr:      &sv.Internal.ProtoDescriptorFile,
				descriptorPtr: &sv.Internal.ProtoDescriptor,
			}
		},
		bindAdd: func(sv *systemVariables) func(string) error {
			return (&ProtoDescriptorVar{
				filesPtr:      &sv.Internal.ProtoDescriptorFile,
				descriptorPtr: &sv.Internal.ProtoDescriptor,
			}).Add
		},
	},
	{
		name: "CLI_TYPE_STYLES",
		desc: "Type-based ANSI styling for query results. Format: colon-separated TYPE=STYLE pairs (e.g., 'STRING=green:INT64=bold:NULL=dim'). " +
			"Supports named colors (red, green, yellow, blue, magenta, cyan, white, black), " +
			"attributes (bold, dim, italic, underline, reverse, strikethrough), " +
			"and raw SGR numbers (e.g., 38;5;214 for 256-color). " +
			"NULL key overrides the default dim style for NULL values. Empty string disables type styling.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: StringVar(&sv.Display.TypeStylesRaw),
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
			}
		},
	},
	{
		name:  "CLI_ENDPOINT",
		desc:  "Host and port for connections (host:port format).",
		scope: scopeStartup,
		bind: func(sv *systemVariables) Variable {
			return &EndpointVar{
				hostPtr: &sv.Config.Host,
				portPtr: &sv.Config.Port,
			}
		},
	},
	{
		name:  "CLI_OUTPUT_TEMPLATE_FILE",
		desc:  "Go text/template for formatting the output of the CLI.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: StringVar(&sv.Display.OutputTemplateFile),
				customGetter: func() (string, error) {
					return sv.Display.OutputTemplateFile, nil
				},
				customSetter: func(value string) error {
					// Parse and set template.
					// An empty value (or NULL) restores the built-in default template
					// (defaultOutputFormat), matching the startup default when no
					// --output-template flag is given. This keeps SET and startup in
					// sync so Get/Set round-trips (relied on by RESET ALL) hold.
					if value == "" || strings.EqualFold(value, "NULL") {
						sv.Display.OutputTemplateFile = ""
						sv.Display.OutputTemplate = defaultOutputFormat
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
			}
		},
	},
	{
		name:  "CLI_ANALYZE_COLUMNS",
		desc:  "Go template for analyzing column data.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &TemplateVar{
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
			}
		},
	},
	{
		name:  "CLI_INLINE_STATS",
		desc:  "<name>:<template>, ...",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &TemplateVar{
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
			}
		},
	},
	{
		// CLI_ENABLE_ADC_PLUS is a session-init-only variable.
		// Currently implemented using a custom setter that checks inTransaction != nil
		// to detect whether a session has been created. Could use native support
		// for session-init-only behavior if added to the varDef metadata.
		name:  "CLI_ENABLE_ADC_PLUS",
		desc:  "A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				base: BoolVar(&sv.Config.EnableADCPlus),
				customSetter: func(value string) error {
					if sv.inTransaction != nil {
						return fmt.Errorf("CLI_ENABLE_ADC_PLUS cannot be changed after session creation")
					}
					b, err := strconv.ParseBool(value)
					if err != nil {
						return err
					}
					sv.Config.EnableADCPlus = b
					return nil
				},
			}
		},
	},
	{
		name:  "CLI_MCP",
		desc:  "A read-only boolean indicating whether the connection is running as an MCP server.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.MCP) },
	},
	{
		name:  "CLI_INSECURE",
		desc:  "Skip TLS certificate verification (insecure).",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.Insecure) },
	},
	{
		name:  "CLI_LOG_GRPC",
		desc:  "Enable gRPC logging.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.LogGrpc) },
	},

	// === Unimplemented variables ===
	{
		name:  "AUTOCOMMIT",
		desc:  "A boolean indicating whether or not the connection is in autocommit mode. The default is true.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return &UnimplementedVar{name: "AUTOCOMMIT"} },
	},
	{
		name:  "RETRY_ABORTS_INTERNALLY",
		desc:  "A boolean indicating whether the connection automatically retries aborted transactions. The default is true.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return &UnimplementedVar{name: "RETRY_ABORTS_INTERNALLY"} },
	},
}
