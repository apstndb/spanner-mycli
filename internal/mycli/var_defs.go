package mycli

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

// varScope classifies where a system variable's value lives and, together with
// readOnly, whether SET may change it.
type varScope int

const (
	// scopeSession is the SET-able surface. Resettable session vars participate
	// in RESET / RESET ALL when they have explicit prepare/commit support.
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

	scope      varScope
	readOnly   bool // for scopeSession exceptions only; other scopes are implicitly read-only
	initOnly   bool // settable only before session creation
	txnGuard   bool // SET rejected while a transaction is active
	batchGuard bool // SET rejected while a manual START BATCH is open
	noLocal    bool // opt-out of SET LOCAL for otherwise-eligible vars
	noReset    bool // opt-out of RESET / RESET ALL for otherwise-eligible vars

	// aliases are additional (typically deprecated) names accepted by
	// SET/SHOW/ADD. They resolve to the same handler as name but are excluded
	// from listings and completion. Used for one-release renames.
	aliases []string

	// bind constructs the live handler bound to the given systemVariables.
	bind func(sv *systemVariables) Variable
	// bindAdd, when non-nil, constructs the variable's ADD handler.
	bindAdd func(sv *systemVariables) func(string) error
}

// settable reports whether SET may change this variable. Non-session scopes are
// implicitly read-only; scopeSession vars are settable unless readOnly is set.
func (d *varDef) settable() bool { return d.scope == scopeSession && !d.readOnly }

// localAllowed reports whether SET LOCAL may target this variable. It excludes
// read-only, session-init-only, transaction-guarded, and explicitly opted-out
// (noLocal) variables, because none of those can be safely set inside a
// transaction and reverted when it ends.
func (d *varDef) localAllowed() bool {
	return d.settable() && !d.initOnly && !d.txnGuard && !d.noLocal
}

// resettable reports whether RESET / RESET ALL should restore this variable to
// its captured startup snapshot. Session-init-only, read-only, and explicitly
// opted-out (noReset) variables are excluded, including file-backed reloads,
// opaque descriptor graphs, unimplemented placeholders, and connection identity.
func (d *varDef) resettable() bool {
	return d.settable() && !d.initOnly && !d.noReset
}

// parseExplainOperatorHeader trims surrounding whitespace at assignment.
// Whitespace-only becomes empty (keep the WIDTH-dependent Operator header).
// Nonempty values reject embedded control characters before any write.
func parseExplainOperatorHeader(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if strings.ContainsFunc(trimmed, unicode.IsControl) {
		return "", fmt.Errorf("CLI_EXPLAIN_OPERATOR_HEADER cannot contain control characters")
	}
	return trimmed, nil
}

// varDefs is the declarative table of all system variables. Order is not
// significant (listings sort by name); it mirrors the historical grouping for
// readability.
//
// Note: COMMIT_RESPONSE is a multi-valued registry def (GetMulti).
var varDefs = []varDef{
	{
		name:  "CLI_DUMP_CYCLIC_MODE",
		desc:  "DUMP cyclic data policy: REJECT (default) or opt-in MUTATE (one transaction per cyclic table group). No service-quota prediction; later restore failure can leave earlier groups committed.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return DumpCyclicModeVar(&sv.Display.DumpCyclicMode) },
	},
	{
		name:  "CLI_DUMP_CYCLIC_MAX_BYTES",
		desc:  "Positive aggregate encoded cyclic-text retention cap for DUMP MUTATE mode. Default 67108864 (64 MiB). Not a hard heap bound or Spanner commit-size/mutation-count estimate.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return IntVar(&sv.Display.DumpCyclicMaxBytes).WithValidator(func(n int64) error {
				if n <= 0 {
					return fmt.Errorf("CLI_DUMP_CYCLIC_MAX_BYTES must be positive")
				}
				return nil
			})
		},
	},
	// === Simple boolean variables ===
	{
		// txnGuard: READONLY switches the transaction mode, which is meaningless
		// (and unsafe) while a transaction is already open. The check is enforced
		// centrally in VarRegistry.Set.
		name:     "READONLY",
		desc:     "A boolean indicating whether or not the connection is in read-only mode",
		scope:    scopeSession,
		txnGuard: true,
		bind:     func(sv *systemVariables) Variable { return BoolVar(&sv.Transaction.ReadOnly) },
	},
	{
		name:     "DIRECTED_READ",
		desc:     "Directed read options for supported read-only queries. Accepts replica_location or replica_location:READ_ONLY|READ_WRITE shorthand, or DirectedReadOptions protobuf JSON. SHOW uses shorthand when that form is lossless; otherwise protobuf JSON. Empty string clears. SET is rejected while a transaction is pending or active; SET LOCAL is not supported. Not applied to read-write queries, DML, heartbeat, or partitioned DML.",
		scope:    scopeSession,
		txnGuard: true,
		noLocal:  true,
		bind: func(sv *systemVariables) Variable {
			return &CustomVar{
				customGetter: func() (string, error) {
					return formatDirectedReadOption(sv.Query.DirectedRead), nil
				},
				customSetter: func(value string) error {
					if strings.TrimSpace(value) == "" {
						sv.Query.DirectedRead = nil
						return nil
					}
					parsed, err := parseDirectedReadOption(value)
					if err != nil {
						return err
					}
					sv.Query.DirectedRead = parsed
					return nil
				},
				prepareReset: func(value string) error {
					if strings.TrimSpace(value) == "" {
						return nil
					}
					_, err := parseDirectedReadOption(value)
					return err
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
		desc:  "A BOOL indicating whether DML in an explicit read-write transaction is buffered until COMMIT, a later execute-now statement, or RUN BATCH. SET only changes future buffering. The default is false.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Transaction.AutoBatchDML) },
	},
	{
		name:  "AUTO_BATCH_DML_UPDATE_COUNT",
		desc:  "A nonnegative INT64 expected update count captured for each newly buffered automatic DML statement. The default is 1. Zero is a valid explicit expectation. This value is an expectation only; it is never reported as observed affected rows. SET, SET LOCAL, RESET, and transaction-end restoration change the policy for future enqueues only.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return IntVar(&sv.Transaction.AutoBatchDMLUpdateCount).
				WithValidator(func(value int64) error {
					if value < 0 {
						return fmt.Errorf("AUTO_BATCH_DML_UPDATE_COUNT must be non-negative, got %d", value)
					}
					return nil
				})
		},
	},
	{
		name:  "AUTO_BATCH_DML_UPDATE_COUNT_VERIFICATION",
		desc:  "A BOOL indicating whether flush compares each captured expected count with the actual BatchUpdate count before accepting a successful automatic DML receipt. The default is false so existing arbitrary UPDATE/DELETE statements keep succeeding until verification is enabled. SET, SET LOCAL, RESET, and transaction-end restoration change the policy for future enqueues only. Replay still validates journaled actual counts when this is false.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return BoolVar(&sv.Transaction.AutoBatchDMLUpdateCountVerification)
		},
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
		name:  "DDL_EXECUTION_MODE",
		desc:  "How DDL statements wait for the Admin long-running operation. SYNC (default) waits for the actual result. ASYNC returns the accepted operation ID immediately. ASYNC_WAIT waits up to DDL_ASYNC_WAIT_TIMEOUT and, on wait-budget expiry, returns the still-running operation ID as a successful asynchronous submission without canceling the server operation. --async selects ASYNC. Replaces CLI_ASYNC_DDL.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return DDLExecutionModeVar(&sv.Feature.DDLExecutionMode) },
	},
	{
		name:  "DDL_ASYNC_WAIT_TIMEOUT",
		desc:  "Maximum time ASYNC_WAIT spends waiting for a DDL operation before returning the still-running operation ID as a successful asynchronous submission. The remaining budget bounds in-flight GetOperation polls as well as the time between polls. Expiry cancels only the polling RPC and does not cancel the server operation. The default is 10s. Unused in SYNC and ASYNC modes.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return DurationVar(&sv.Feature.DDLAsyncWaitTimeout).
				WithValidator(durationValueValidator(durationPtr(0), nil))
		},
	},
	{
		name:  "DEFAULT_SEQUENCE_KIND",
		desc:  "Opt-in default sequence kind used only to repair a precise missing-kind SYNC DDL failure. Empty/NULL (default) disables repair. The only accepted non-empty value is bit_reversed_positive. When enabled, a matching InvalidArgument failure may submit one extra ALTER DATABASE to set the database option default_sequence_kind (a database-wide schema mutation; requires the existing Spanner DDL update permission) and then retry only the metadata-proven unfinished suffix. Repair does not run in ASYNC, ASYNC_WAIT, or SHOW OPERATION.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return DefaultSequenceKindVar(&sv.Feature.DefaultSequenceKind) },
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
		desc:  "Transaction tag for the next physical read-write transaction. After that owner starts, SHOW reports the applied tag. The consumed slot is then empty unless a SET LOCAL baseline restores. Ordinary SET after SET LOCAL supersedes LOCAL. SET and SET LOCAL are rejected while a read-write transaction is active. Read-only transactions do not consume this tag; SET LOCAL during RO still restores. Partitioned DML does not currently send a transaction tag.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return &transactionTagVar{sv: sv} },
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
				prepareReset: func(value string) error {
					if value == "" {
						return fmt.Errorf("CLI_PROMPT2 cannot be empty")
					}
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
	// CLI_VERTEXAI_PROJECT/MODEL/LOCATION moved to internal/mycli/feature/llm as
	// feature-contributed variables (#778); they are registered through the
	// Feature seam in the full variant.
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
		bind: func(sv *systemVariables) Variable {
			return IntVar(&sv.Query.MaxPartitionedParallelism).
				WithValidator(func(value int64) error {
					if value < 0 {
						return fmt.Errorf("MAX_PARTITIONED_PARALLELISM must be non-negative, got %d", value)
					}
					return nil
				})
		},
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
		desc:  "A property of type STRING indicating the current timeout value for statements (e.g., 10s, 5m, 1h). NULL (the omitted-flag default) uses 10m for ordinary statements and 24h for partitioned DML. This is a CLI policy, not a server-required deadline.",
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
		name:    "COMMIT_PRIORITY",
		desc:    "Commit RPC priority for read-write transactions (HIGH, MEDIUM, LOW). UNSPECIFIED (default) inherits the resolved transaction RPC priority, which is the existing mycli behavior. That differs from go-sql-spanner, where default UNSPECIFIED is the Go driver's CommitPriority default and does not inherit RPC_PRIORITY. The effective value is frozen in the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. SET LOCAL is not supported. Not applied to query, DML, heartbeat, partitioned DML, read-only, or Admin RPCs.",
		scope:   scopeSession,
		noLocal: true,
		bind:    func(sv *systemVariables) Variable { return RPCPriorityVar(&sv.Transaction.CommitPriority) },
	},
	{
		name:    "KEEP_TRANSACTION_ALIVE",
		desc:    "Whether an explicit read-write owner schedules keepalive heartbeats after the first user SQL. TRUE (default) preserves existing mycli behavior. FALSE prevents heartbeat scheduling for that owner without changing user SQL, COMMIT, ROLLBACK, or cancellation. Java KEEP_TRANSACTION_ALIVE defaults to false; this CLI default is intentionally TRUE. The policy is frozen on the logical owner with the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. Changing the session default does not alter an active owner. SET LOCAL is not supported. CLI_IDLE_TRANSACTION_TIMEOUT is an independent user-idle quiet interval and still expires when keepalive is disabled. TRANSACTION_TIMEOUT is a separate logical-owner budget and is not implied by this variable.",
		scope:   scopeSession,
		noLocal: true,
		bind:    func(sv *systemVariables) Variable { return BoolVar(&sv.Transaction.KeepTransactionAlive) },
	},
	{
		name:  "TRANSACTION_TIMEOUT",
		desc:  "Logical read/write transaction deadline (duration or NULL). NULL or 0 means no additional transaction deadline. The duration is captured for the logical owner; the single total budget starts at the first real database RPC (including constructor BeginTransaction) and is preserved across physical reconstruction. Pending SET LOCAL may select the duration before the first RPC; changing it after first real database use is rejected, including when the selected duration is NULL or 0 and no timer exists. Session SET after BEGIN applies to a later owner. Distinct from STATEMENT_TIMEOUT and from CLI_IDLE_TRANSACTION_TIMEOUT (#357). ABORTED retries (#293) are not implemented; a later retry path must reuse the remaining budget.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return NullableDurationVar(&sv.Transaction.TransactionTimeout).
				WithValidator(durationValidator(durationPtr(0), nil))
		},
	},
	{
		name:  "CLI_IDLE_TRANSACTION_TIMEOUT",
		desc:  "Sliding user-idle quiet interval on the logical owner (duration or NULL). Units are Go duration strings (for example 60s or 5m). NULL or 0 disables expiry; there is no default 60-second timeout. The duration is captured at BEGIN. Session SET or RESET after BEGIN applies to a later owner. SET LOCAL may change the captured duration before admitted user or database work and then freezes (identical values remain accepted). Successful explicit BEGIN RW or BEGIN RO that acquired a server transaction starts the first quiet interval; a resource-free pending BEGIN does not arm until work. Completed admitted work rearms, including successful buffered automatic or manual DML, buffered MUTATE, and successful SAVEPOINT, RELEASE, or ROLLBACK TO. Heartbeat SELECT 1, client-only SHOW, polling, SET, RESET, invalid syntax, and rejected admission do not reset idle. Expiry holds through the user RPC, iterator consumption, and CLI result rendering or pager. Idle is not an RPC context deadline; TRANSACTION_TIMEOUT still cancels in-flight work. Expiry rolls back and retires only the matching owner. The next ordinary command reports a one-shot error without executing; ROLLBACK, CLOSE, BEGIN, USE, and DETACH acknowledge the notice. Independent of KEEP_TRANSACTION_ALIVE.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return NullableDurationVar(&sv.Transaction.IdleTransactionTimeout).
				WithValidator(durationValidator(durationPtr(0), nil))
		},
	},
	{
		name:  "CLI_DDL_IN_TRANSACTION_MODE",
		desc:  "How DDL interacts with an existing logical transaction. FAIL (default) rejects DDL while any owner exists. ALLOW_IN_EMPTY_TRANSACTION retires an empty pending or constructor-only RW owner without Commit, then runs DDL; nonempty RW, RO, recovery, and manual DML batch are rejected. AUTO_COMMIT_TRANSACTION no-op-retires empty pending, Commits constructor-only or nonempty RW (flushing eligible automatic DML first), then runs DDL; RO, recovery, and manual DML batch are rejected. The policy is captured on the logical owner at creation, including pending BEGIN. Session SET after BEGIN applies to a later owner. SET LOCAL may change this owner's captured policy only before user work. DDL is never SAVEPOINT-rollbackable. Empty BulkDdl is a no-op and does not commit. START BATCH DDL is admitted before batch state changes; RUN BATCH validates descriptors and rechecks admission before Commit, then carries that preparation receipt through Admin. CreateDatabase is out of scope. EOF/EXIT/Close never auto-commit because of this variable. Default FAIL intentionally differs from Java ALLOW_IN_EMPTY_TRANSACTION. SYNC default-sequence repair is DEFAULT_SEQUENCE_KIND (#984), not this variable.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return DdlInTransactionModeVar(&sv.Transaction.DdlInTransactionMode)
		},
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
					var mode sppb.ExecuteSqlRequest_QueryMode
					if sv.Query.QueryMode != nil {
						mode = *sv.Query.QueryMode
					}
					if err := QueryModeVar(&mode).Set(value); err != nil {
						return err
					}
					sv.Query.QueryMode = &mode
					return nil
				},
				prepareReset: func(value string) error {
					if strings.EqualFold(value, "NULL") {
						return nil
					}
					var mode sppb.ExecuteSqlRequest_QueryMode
					return QueryModeVar(&mode).Set(value)
				},
			}
		},
	},

	{
		name:       "CLI_SAVEPOINT_SUPPORT",
		desc:       "Enable client-emulated SAVEPOINT for explicit transactions. DISABLED (default) preserves current behavior. ENABLED records a journal from BEGIN and reconstructs RW prefixes on ROLLBACK TO. This is replay with result validation, not a native Spanner savepoint. SET is rejected while a transaction is pending or active and while a manual batch is open; SET LOCAL is not supported.",
		scope:      scopeSession,
		txnGuard:   true,
		batchGuard: true,
		noLocal:    true,
		bind:       func(sv *systemVariables) Variable { return SavepointSupportVar(&sv.Transaction.SavepointSupport) },
	},
	{
		name:  "AUTOCOMMIT_DML_MODE",
		desc:  "A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements. TRANSACTIONAL (default) commits each implicit DML atomically. PARTITIONED_NON_ATOMIC uses partitioned DML for implicit UPDATE/DELETE. TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC retries one eligible implicit UPDATE/DELETE as partitioned DML only after a SQL-phase mutation-limit failure that matches the pinned InvalidArgument + exact mutation-limit sentence + Cloud Spanner limits Help classifier. The fallback is non-atomic, returns a lower-bound count, and can partially commit if the partitioned attempt later fails. INSERT, THEN RETURN, explicit/pending/RO/SAVEPOINT owners, batches, EXPLAIN/analysis, Commit-phase failures, and weaker resource-limit errors are not retried. Not a Java-complete or stable driver-parity claim.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return AutocommitDMLModeVar(&sv.Transaction.AutocommitDMLMode) },
	},
	{
		name:  "CLI_FORMAT",
		desc:  "Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated, raw values), TSV (tab-separated with tab/newline/carriage-return/backslash escaping), HTML (HTML table), XML (XML format), CSV (comma-separated values), JSONL (newline-delimited JSON), SQL_INSERT (INSERT statements), SQL_INSERT_OR_IGNORE (INSERT OR IGNORE statements), SQL_INSERT_OR_UPDATE (INSERT OR UPDATE statements).",
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
				prepareReset: func(value string) error {
					if value == "" {
						return nil
					}
					var tmp enums.ExplainFormat
					return ExplainFormatVar(&tmp).Set(value)
				},
			}
		},
	},
	{
		name:  "CLI_EXPLAIN_OPERATOR_HEADER",
		desc:  "Literal Operator column header for EXPLAIN, EXPLAIN ANALYZE, LAST QUERY, and query-profile plan tables. Empty or whitespace-only keeps the existing WIDTH-dependent header. A nonempty value is trimmed of surrounding whitespace and used at every width. WIDTH wraps operator cell text only and does not truncate a deliberately long header. Ordinary Unicode is accepted. Embedded control characters (including NUL, CR, LF, tabs, and terminal escapes) are rejected.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &VarHandler[string]{
				ptr:    &sv.Display.ExplainOperatorHeader,
				format: func(s string) string { return s },
				parse:  parseExplainOperatorHeader,
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
				prepareReset: func(value string) error {
					_, err := parseExplainPrintSections(value)
					return err
				},
			}
		},
	},
	{
		name:  "CLI_LOG_LEVEL",
		desc:  "Log level for the CLI slog logger (DEBUG, INFO, WARN, ERROR; WARNING is accepted as WARN). SET and --set change the process threshold. Embedded container lifecycle logs follow the startup --log-level snapshot, not later SET.",
		scope: scopeSession,
		bind: func(sv *systemVariables) Variable {
			return &LogLevelVar{ptr: &sv.Feature.LogLevel, runtime: sv.runtimeLogLevel}
		},
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
	{
		// COMMIT_RESPONSE is multi-valued: SHOW VARIABLE / SHOW VARIABLES expose
		// its COMMIT_TIMESTAMP and MUTATION_COUNT columns via the MultiValueVar
		// capability (commitResponseVar.GetMulti). It is read-only (scopeResult).
		name:  "COMMIT_RESPONSE",
		desc:  "The most recent response for a read-write transaction. SHOW VARIABLE COMMIT_RESPONSE returns COMMIT_TIMESTAMP and MUTATION_COUNT columns; SHOW VARIABLES includes those values as COMMIT_TIMESTAMP and MUTATION_COUNT.",
		scope: scopeResult,
		bind:  func(sv *systemVariables) Variable { return &commitResponseVar{sv: sv} },
	},

	// === Complex variables ===
	{
		name:  "READ_ONLY_STALENESS",
		desc:  "A property of type STRING for read-only transactions with flexible staleness.",
		scope: scopeSession,
		bind:  func(sv *systemVariables) Variable { return &TimestampBoundVar{ptr: &sv.Query.ReadOnlyStaleness} },
	},
	{
		// noLocal/noReset: this setter reads files from disk as a side effect.
		// RESET ALL does not turn a displayed path into a resource-loading reset.
		name:    "PROTO_DESCRIPTORS_FILE_PATH",
		noReset: true,
		desc:    "Comma-separated list of proto descriptor files. Supports ADD to append files. HTTP(S) source vs binary is classified from the URL path, not query or fragment.",
		scope:   scopeSession,
		noLocal: true,
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
		// noLocal/noReset: the displayed value is an opaque graph; RESET ALL
		// does not serialize or reload descriptor state.
		name:    protoDescriptorsVarName,
		noReset: true,
		desc:    "Base64 FileDescriptorSet for the session proto graph. DUMP SCHEMA/DATABASE emit SET PROTO_DESCRIPTORS so replay is self-contained. SET LOCAL is not supported. Cannot be changed while a manual batch is active.",
		scope:   scopeSession,
		noLocal: true,
		bind: func(sv *systemVariables) Variable {
			return &ProtoDescriptorsVar{
				filesPtr:      &sv.Internal.ProtoDescriptorFile,
				descriptorPtr: &sv.Internal.ProtoDescriptor,
			}
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
				prepareReset: func(value string) error {
					if strings.EqualFold(value, "NULL") {
						value = ""
					}
					_, err := parseTypeStyles(value)
					return err
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
		// noLocal/noReset: this setter reads a template file from disk. RESET ALL
		// does not turn a displayed path into a resource-loading reset.
		name:    "CLI_OUTPUT_TEMPLATE_FILE",
		desc:    "Go text/template for formatting the output of the CLI.",
		scope:   scopeSession,
		noLocal: true,
		noReset: true,
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
					// sync so Get/Set empty-path round-trips hold.
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
				prepareFunc: func(value string) error {
					_, err := parseAnalyzeColumns(value)
					return err
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
					// Empty means "no inline stats". parseInlineStats would reject
					// "" (it requires "<name>:<template>"), which would make the
					// default value fail to Set back and break the SET LOCAL
					// Get->Set round-trip; treat it as clearing instead.
					if value == "" {
						sv.Display.ParsedInlineStats = nil
						return nil
					}
					parsed, err := parseInlineStats(value)
					if err != nil {
						return err
					}
					sv.Display.ParsedInlineStats = parsed
					return nil
				},
				prepareFunc: func(value string) error {
					if value == "" {
						return nil
					}
					_, err := parseInlineStats(value)
					return err
				},
			}
		},
	},
	{
		// initOnly: CLI_ENABLE_ADC_PLUS must be fixed before session creation so
		// authentication behavior stays consistent for the session's lifetime.
		// The "session exists" check is enforced centrally in VarRegistry.Set.
		name:     "CLI_ENABLE_ADC_PLUS",
		desc:     "A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.",
		scope:    scopeSession,
		initOnly: true,
		bind:     func(sv *systemVariables) Variable { return BoolVar(&sv.Config.EnableADCPlus) },
	},
	{
		name:  "CLI_MCP",
		desc:  "A read-only boolean indicating whether the connection is running as an MCP server.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.MCP) },
	},
	{
		name:  "CLI_INSECURE",
		desc:  "Permit plaintext gRPC (no TLS). Set by --insecure or --skip-tls-verify.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.Insecure) },
	},
	{
		name:  "CLI_CA_CERT_FILE",
		desc:  "Path to the PEM CA certificate file used as the TLS trust bundle. Empty when unset. Replaces system roots when set. Startup-only.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.CaCertFile) },
	},
	{
		name:  "CLI_CLIENT_CERT_FILE",
		desc:  "Path to the PEM client certificate file for mTLS. Empty when unset. Startup-only.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.ClientCertFile) },
	},
	{
		name:  "CLI_CLIENT_CERT_KEY",
		desc:  "Path to the PEM client private-key file for mTLS. SHOW reports the path only and never the key bytes. Empty when unset. Startup-only.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.ClientCertKey) },
	},
	{
		name:  "CLI_WITHOUT_AUTHENTICATION",
		desc:  "Do not send Google bearer credentials to the Spanner endpoint. Requires an explicit endpoint and at least one custom TLS file. Does not enable plaintext or skip certificate verification.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.WithoutAuthentication) },
	},
	{
		name:  "CLI_LOG_GRPC",
		desc:  "Enable gRPC logging.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.LogGrpc) },
	},
	{
		name: "CLI_SPANNER_METRICS_EXPORTER",
		desc: "Startup-only caller-owned Spanner client metrics exporter: off (default) or otlp. " +
			"otlp uses OTLP HTTP/protobuf to CLI_SPANNER_METRICS_ENDPOINT. " +
			"Does not enable native Cloud Monitoring or a global MeterProvider. " +
			"OTEL_* environment variables alone do not initialize export. " +
			"SPANNER_EMULATOR_HOST suppresses SDK caller metrics. Not SET-able.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.SpannerMetricsExporter) },
	},
	{
		name: "CLI_SPANNER_METRICS_ENDPOINT",
		desc: "Startup-only absolute http/https URL of the OTLP metrics collector used when " +
			"CLI_SPANNER_METRICS_EXPORTER=otlp. Host required; no userinfo, query, or fragment. " +
			"Missing or root path is /v1/metrics. Empty when export is off. Not SET-able.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.SpannerMetricsEndpoint) },
	},
	{
		name: "CLI_SPANNER_TRACES_EXPORTER",
		desc: "Startup-only process-owned Spanner client traces exporter: off (default) or otlp. " +
			"otlp uses official OTLP HTTP/protobuf to CLI_SPANNER_TRACES_ENDPOINT and installs one " +
			"CLI-owned global TracerProvider. Off does not change the global provider or environment. " +
			"Does not set a global MeterProvider. " +
			"OTEL_* environment variables alone do not initialize export. " +
			"SPANNER_ENABLE_END_TO_END_TRACING is still honored by the SDK when CLI traces are off. " +
			"Shutdown of the pinned initial OTel proxy uses a no-op substitute; previously acquired " +
			"proxy tracers cannot be restored. Not SET-able.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.SpannerTracesExporter) },
	},
	{
		name: "CLI_SPANNER_TRACES_ENDPOINT",
		desc: "Startup-only absolute http/https URL of the OTLP traces collector used when " +
			"CLI_SPANNER_TRACES_EXPORTER=otlp. Host required; no userinfo, query, or fragment. " +
			"Missing or root path is /v1/traces. Empty when export is off. Not SET-able.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return StringVar(&sv.Config.SpannerTracesEndpoint) },
	},
	{
		name: "CLI_SPANNER_TRACES_SAMPLE_RATIO",
		desc: "Startup-only root sampling ratio in [0,1] for CLI-owned traces when " +
			"CLI_SPANNER_TRACES_EXPORTER=otlp. ParentBased: sampled parents are honored. " +
			"Default 0.01. Not SET-able.",
		scope: scopeStartup,
		bind:  func(sv *systemVariables) Variable { return Float64Var(&sv.Config.SpannerTracesSampleRatio) },
	},

	// === Unimplemented variables ===
	{
		name:    "AUTOCOMMIT",
		desc:    "A boolean indicating whether or not the connection is in autocommit mode. The default is true.",
		scope:   scopeSession,
		noReset: true,
		bind:    func(sv *systemVariables) Variable { return &UnimplementedVar{name: "AUTOCOMMIT"} },
	},
	{
		name:    "RETRY_ABORTS_INTERNALLY",
		desc:    "A boolean indicating whether the connection automatically retries aborted transactions. The default is true.",
		scope:   scopeSession,
		noReset: true,
		bind:    func(sv *systemVariables) Variable { return &UnimplementedVar{name: "RETRY_ABORTS_INTERNALLY"} },
	},
}
