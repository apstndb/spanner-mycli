package main

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
	"github.com/samber/lo"
)

// createSystemVariableRegistry creates and configures the parser registry for system variables.
// This function sets up all the system variable parsers with their getters and setters.
func createSystemVariableRegistry(sv *systemVariables) *sysvar.Registry {
	registry := sysvar.NewRegistry()

	// Helper function to register variables with panic on error
	mustRegister := func(parser sysvar.VariableParser) {
		if err := registry.Register(parser); err != nil {
			panic(fmt.Sprintf("Failed to register %s: %v", parser.Name(), err))
		}
	}

	// Migrate simple boolean variables first

	// READONLY - Connection read-only mode
	mustRegister(sysvar.NewBooleanParser(
		"READONLY",
		"A boolean indicating whether or not the connection is in read-only mode. The default is false.",
		func() bool { return sv.ReadOnly },
		func(v bool) error {
			if sv.CurrentSession != nil && (sv.CurrentSession.InReadOnlyTransaction() || sv.CurrentSession.InReadWriteTransaction()) {
				return errors.New("can't change READONLY when there is a active transaction")
			}
			sv.ReadOnly = v
			return nil
		},
	))

	// AUTO_PARTITION_MODE
	mustRegister(sysvar.NewBooleanParser(
		"AUTO_PARTITION_MODE",
		"A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.",
		func() bool { return sv.AutoPartitionMode },
		func(v bool) error {
			sv.AutoPartitionMode = v
			return nil
		},
	))

	// RETURN_COMMIT_STATS
	mustRegister(sysvar.NewBooleanParser(
		"RETURN_COMMIT_STATS",
		"A property of type BOOL indicating whether statistics should be returned for transactions on this connection.",
		func() bool { return sv.ReturnCommitStats },
		func(v bool) error {
			sv.ReturnCommitStats = v
			return nil
		},
	))

	// AUTO_BATCH_DML
	mustRegister(sysvar.NewBooleanParser(
		"AUTO_BATCH_DML",
		"A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false.",
		func() bool { return sv.AutoBatchDML },
		func(v bool) error {
			sv.AutoBatchDML = v
			return nil
		},
	))

	// DATA_BOOST_ENABLED
	mustRegister(sysvar.NewBooleanParser(
		"DATA_BOOST_ENABLED",
		"A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.",
		func() bool { return sv.DataBoostEnabled },
		func(v bool) error {
			sv.DataBoostEnabled = v
			return nil
		},
	))

	// EXCLUDE_TXN_FROM_CHANGE_STREAMS
	mustRegister(sysvar.NewBooleanParser(
		"EXCLUDE_TXN_FROM_CHANGE_STREAMS",
		"Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).",
		func() bool { return sv.ExcludeTxnFromChangeStreams },
		func(v bool) error {
			sv.ExcludeTxnFromChangeStreams = v
			return nil
		},
	))

	// CLI variables - boolean

	// CLI_VERBOSE
	mustRegister(sysvar.NewBooleanParser(
		"CLI_VERBOSE",
		"Display verbose output.",
		func() bool { return sv.Verbose },
		func(v bool) error {
			sv.Verbose = v
			return nil
		},
	))

	// CLI_ECHO_EXECUTED_DDL
	mustRegister(sysvar.NewBooleanParser(
		"CLI_ECHO_EXECUTED_DDL",
		"Echo executed DDL statements.",
		func() bool { return sv.EchoExecutedDDL },
		func(v bool) error {
			sv.EchoExecutedDDL = v
			return nil
		},
	))

	// CLI_ECHO_INPUT
	mustRegister(sysvar.NewBooleanParser(
		"CLI_ECHO_INPUT",
		"Echo input statements.",
		func() bool { return sv.EchoInput },
		func(v bool) error {
			sv.EchoInput = v
			return nil
		},
	))

	// CLI_USE_PAGER
	mustRegister(sysvar.NewBooleanParser(
		"CLI_USE_PAGER",
		"Enable pager for output.",
		func() bool { return sv.UsePager },
		func(v bool) error {
			sv.UsePager = v
			return nil
		},
	))

	// CLI_AUTOWRAP
	mustRegister(sysvar.NewBooleanParser(
		"CLI_AUTOWRAP",
		"Enable automatic line wrapping.",
		func() bool { return sv.AutoWrap },
		func(v bool) error {
			sv.AutoWrap = v
			return nil
		},
	))

	// CLI_ENABLE_HIGHLIGHT
	mustRegister(sysvar.NewBooleanParser(
		"CLI_ENABLE_HIGHLIGHT",
		"Enable syntax highlighting.",
		func() bool { return sv.EnableHighlight },
		func(v bool) error {
			sv.EnableHighlight = v
			return nil
		},
	))

	// CLI_PROTOTEXT_MULTILINE
	mustRegister(sysvar.NewBooleanParser(
		"CLI_PROTOTEXT_MULTILINE",
		"Enable multiline prototext output.",
		func() bool { return sv.MultilineProtoText },
		func(v bool) error {
			sv.MultilineProtoText = v
			return nil
		},
	))

	// CLI_MARKDOWN_CODEBLOCK
	mustRegister(sysvar.NewBooleanParser(
		"CLI_MARKDOWN_CODEBLOCK",
		"Enable markdown codeblock output.",
		func() bool { return sv.MarkdownCodeblock },
		func(v bool) error {
			sv.MarkdownCodeblock = v
			return nil
		},
	))

	// CLI_TRY_PARTITION_QUERY
	mustRegister(sysvar.NewBooleanParser(
		"CLI_TRY_PARTITION_QUERY",
		"A boolean indicating whether to test query for partition compatibility instead of executing it.",
		func() bool { return sv.TryPartitionQuery },
		func(v bool) error {
			sv.TryPartitionQuery = v
			return nil
		},
	))

	// CLI_AUTO_CONNECT_AFTER_CREATE
	mustRegister(sysvar.NewBooleanParser(
		"CLI_AUTO_CONNECT_AFTER_CREATE",
		"A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.",
		func() bool { return sv.AutoConnectAfterCreate },
		func(v bool) error {
			sv.AutoConnectAfterCreate = v
			return nil
		},
	))

	// CLI_ENABLE_PROGRESS_BAR
	mustRegister(sysvar.NewBooleanParser(
		"CLI_ENABLE_PROGRESS_BAR",
		"A boolean indicating whether to display progress bars during operations. The default is false.",
		func() bool { return sv.EnableProgressBar },
		func(v bool) error {
			sv.EnableProgressBar = v
			return nil
		},
	))

	// CLI_ENABLE_ADC_PLUS
	mustRegister(sysvar.NewBooleanParser(
		"CLI_ENABLE_ADC_PLUS",
		"A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.",
		func() bool { return sv.EnableADCPlus },
		func(v bool) error {
			sv.EnableADCPlus = v
			return nil
		},
	))

	// CLI_ASYNC_DDL
	mustRegister(sysvar.NewBooleanParser(
		"CLI_ASYNC_DDL",
		"A boolean indicating whether DDL statements should be executed asynchronously. The default is false.",
		func() bool { return sv.AsyncDDL },
		func(v bool) error {
			sv.AsyncDDL = v
			return nil
		},
	))

	// CLI_SKIP_COLUMN_NAMES
	mustRegister(sysvar.NewBooleanParser(
		"CLI_SKIP_COLUMN_NAMES",
		"A boolean indicating whether to suppress column headers in output. The default is false.",
		func() bool { return sv.SkipColumnNames },
		func(v bool) error {
			sv.SkipColumnNames = v
			return nil
		},
	))

	// CLI_LINT_PLAN (special case with conditional getter)
	mustRegister(sysvar.NewBooleanParser(
		"CLI_LINT_PLAN",
		"Enable query plan linting.",
		func() bool { return sv.LintPlan },
		func(v bool) error {
			sv.LintPlan = v
			return nil
		},
	))

	// Integer variables

	// MAX_PARTITIONED_PARALLELISM
	mustRegister(sysvar.NewIntegerParser(
		"MAX_PARTITIONED_PARALLELISM",
		"A property of type `INT64` indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for `AUTO_PARTITION_MODE=TRUE` and `RUN PARTITIONED QUERY`",
		func() int64 { return sv.MaxPartitionedParallelism },
		func(v int64) error {
			sv.MaxPartitionedParallelism = v
			return nil
		},
		nil, nil, // No min/max constraints
	))

	// CLI_TAB_WIDTH
	mustRegister(sysvar.NewIntegerParser(
		"CLI_TAB_WIDTH",
		"Tab width. It is used for expanding tabs.",
		func() int64 { return sv.TabWidth },
		func(v int64) error {
			sv.TabWidth = v
			return nil
		},
		nil, nil, // No min/max constraints
	))

	// CLI_EXPLAIN_WRAP_WIDTH
	mustRegister(sysvar.NewIntegerParser(
		"CLI_EXPLAIN_WRAP_WIDTH",
		"Controls query plan wrap width. It effects only operators column contents",
		func() int64 { return sv.ExplainWrapWidth },
		func(v int64) error {
			sv.ExplainWrapWidth = v
			return nil
		},
		nil, nil, // No min/max constraints
	))

	// String variables

	// OPTIMIZER_VERSION
	mustRegister(sysvar.NewStringParser(
		"OPTIMIZER_VERSION",
		"A property of type `STRING` indicating the optimizer version. The version is either an integer string or 'LATEST'.",
		func() string { return sv.OptimizerVersion },
		func(v string) error {
			sv.OptimizerVersion = v
			return nil
		},
	))

	// OPTIMIZER_STATISTICS_PACKAGE
	mustRegister(sysvar.NewStringParser(
		"OPTIMIZER_STATISTICS_PACKAGE",
		"A property of type STRING indicating the current optimizer statistics package that is used by this connection.",
		func() string { return sv.OptimizerStatisticsPackage },
		func(v string) error {
			sv.OptimizerStatisticsPackage = v
			return nil
		},
	))

	// CLI_PROMPT
	mustRegister(sysvar.NewStringParser(
		"CLI_PROMPT",
		"Custom prompt for spanner-mycli.",
		func() string { return sv.Prompt },
		func(v string) error {
			sv.Prompt = v
			return nil
		},
	))

	// CLI_PROMPT2
	mustRegister(sysvar.NewStringParser(
		"CLI_PROMPT2",
		"Custom continuation prompt for spanner-mycli.",
		func() string { return sv.Prompt2 },
		func(v string) error {
			sv.Prompt2 = v
			return nil
		},
	))

	// CLI_VERTEXAI_MODEL
	mustRegister(sysvar.NewStringParser(
		"CLI_VERTEXAI_MODEL",
		"Vertex AI model for natural language features.",
		func() string { return sv.VertexAIModel },
		func(v string) error {
			sv.VertexAIModel = v
			return nil
		},
	))

	// CLI_VERTEXAI_PROJECT
	mustRegister(sysvar.NewStringParser(
		"CLI_VERTEXAI_PROJECT",
		"Vertex AI project for natural language features.",
		func() string { return sv.VertexAIProject },
		func(v string) error {
			sv.VertexAIProject = v
			return nil
		},
	))

	// Enum variables

	// RPC_PRIORITY
	priorityValues := map[string]sppb.RequestOptions_Priority{
		"UNSPECIFIED": sppb.RequestOptions_PRIORITY_UNSPECIFIED,
		"LOW":         sppb.RequestOptions_PRIORITY_LOW,
		"MEDIUM":      sppb.RequestOptions_PRIORITY_MEDIUM,
		"HIGH":        sppb.RequestOptions_PRIORITY_HIGH,
	}
	mustRegister(sysvar.NewEnumParser(
		"RPC_PRIORITY",
		"A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.",
		priorityValues,
		func() sppb.RequestOptions_Priority { return sv.RPCPriority },
		func(v sppb.RequestOptions_Priority) error {
			sv.RPCPriority = v
			return nil
		},
		func(v sppb.RequestOptions_Priority) string {
			// Strip PRIORITY_ prefix for display to match user expectations
			// v.String() returns "PRIORITY_HIGH" but users expect just "HIGH"
			return strings.TrimPrefix(v.String(), "PRIORITY_")
		},
	))

	// CLI_FORMAT
	formatValues := map[string]DisplayMode{
		"TABLE":                DisplayModeTable,
		"TABLE_COMMENT":        DisplayModeTableComment,
		"TABLE_DETAIL_COMMENT": DisplayModeTableDetailComment,
		"VERTICAL":             DisplayModeVertical,
		"TAB":                  DisplayModeTab,
		"HTML":                 DisplayModeHTML,
		"XML":                  DisplayModeXML,
		"CSV":                  DisplayModeCSV,
	}
	mustRegister(sysvar.NewEnumParser(
		"CLI_FORMAT",
		"Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated), HTML (HTML table), XML (XML format), CSV (comma-separated values).",
		formatValues,
		func() DisplayMode { return sv.CLIFormat },
		func(v DisplayMode) error {
			sv.CLIFormat = v
			return nil
		},
		func(v DisplayMode) string {
			// Map DisplayMode to string
			switch v {
			case DisplayModeTable:
				return "TABLE"
			case DisplayModeTableComment:
				return "TABLE_COMMENT"
			case DisplayModeTableDetailComment:
				return "TABLE_DETAIL_COMMENT"
			case DisplayModeVertical:
				return "VERTICAL"
			case DisplayModeTab:
				return "TAB"
			case DisplayModeHTML:
				return "HTML"
			case DisplayModeXML:
				return "XML"
			case DisplayModeCSV:
				return "CSV"
			default:
				return fmt.Sprintf("DisplayMode(%d)", v)
			}
		},
	))

	// CLI_EXPLAIN_FORMAT
	explainFormatValues := map[string]explainFormat{
		"":            explainFormatUnspecified,
		"CURRENT":     explainFormatCurrent,
		"TRADITIONAL": explainFormatTraditional,
		"COMPACT":     explainFormatCompact,
	}
	mustRegister(sysvar.NewStringEnumParser(
		"CLI_EXPLAIN_FORMAT",
		"Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.",
		explainFormatValues,
		func() explainFormat { return sv.ExplainFormat },
		func(v explainFormat) error {
			sv.ExplainFormat = v
			return nil
		},
	))

	// More boolean variables

	// Note: AUTOCOMMIT is handled in the old system (no direct field in systemVariables)
	// Note: RETRY_ABORTS_INTERNALLY is handled in the old system (no direct field in systemVariables)

	// Note: CLI_FIXED_WIDTH is nullable int64, not boolean - remains in old system
	// Note: CLI_INLINE_STATS is a string with complex parsing - remains in old system

	// CLI_SKIP_SYSTEM_COMMAND
	mustRegister(sysvar.NewBooleanParser(
		"CLI_SKIP_SYSTEM_COMMAND",
		"Controls whether system commands are disabled.",
		func() bool { return sv.SkipSystemCommand },
		func(v bool) error {
			sv.SkipSystemCommand = v
			return nil
		},
	))

	// CLI_LOG_GRPC
	mustRegister(sysvar.NewBooleanParser(
		"CLI_LOG_GRPC",
		"Enable gRPC logging.",
		func() bool { return sv.LogGrpc },
		func(v bool) error {
			sv.LogGrpc = v
			return nil
		},
	))

	// CLI_INSECURE
	mustRegister(sysvar.NewBooleanParser(
		"CLI_INSECURE",
		"Skip TLS certificate verification (insecure).",
		func() bool { return sv.Insecure },
		func(v bool) error {
			sv.Insecure = v
			return nil
		},
	))

	// More string variables

	// CLI_ANALYZE_COLUMNS
	mustRegister(sysvar.NewStringParser(
		"CLI_ANALYZE_COLUMNS",
		"Go template for analyzing column data.",
		func() string { return sv.AnalyzeColumns },
		func(v string) error {
			sv.AnalyzeColumns = v
			// TODO: Also update ParsedAnalyzeColumns
			return nil
		},
	))

	// CLI_HISTORY_FILE
	mustRegister(sysvar.NewStringParser(
		"CLI_HISTORY_FILE",
		"Path to the history file.",
		func() string { return sv.HistoryFile },
		func(v string) error {
			sv.HistoryFile = v
			return nil
		},
	))

	// CLI_PROJECT
	mustRegister(sysvar.NewStringParser(
		"CLI_PROJECT",
		"GCP Project ID.",
		func() string { return sv.Project },
		func(v string) error {
			sv.Project = v
			return nil
		},
	))

	// CLI_INSTANCE
	mustRegister(sysvar.NewStringParser(
		"CLI_INSTANCE",
		"Cloud Spanner instance ID.",
		func() string { return sv.Instance },
		func(v string) error {
			sv.Instance = v
			return nil
		},
	))

	// CLI_DATABASE
	mustRegister(sysvar.NewStringParser(
		"CLI_DATABASE",
		"Cloud Spanner database ID.",
		func() string { return sv.Database },
		func(v string) error {
			sv.Database = v
			return nil
		},
	))

	// CLI_ROLE
	mustRegister(sysvar.NewStringParser(
		"CLI_ROLE",
		"Cloud Spanner database role.",
		func() string { return sv.Role },
		func(v string) error {
			sv.Role = v
			return nil
		},
	))

	// Note: CLI_ENDPOINT is handled specially in the old system (parses to host/port)

	// CLI_IMPERSONATE_SERVICE_ACCOUNT
	mustRegister(sysvar.NewStringParser(
		"CLI_IMPERSONATE_SERVICE_ACCOUNT",
		"Service account to impersonate.",
		func() string { return sv.ImpersonateServiceAccount },
		func(v string) error {
			sv.ImpersonateServiceAccount = v
			return nil
		},
	))

	// TRANSACTION_TAG
	mustRegister(sysvar.NewStringParser(
		"TRANSACTION_TAG",
		"A property of type STRING that contains the transaction tag for the next transaction.",
		func() string { return sv.TransactionTag },
		func(v string) error {
			sv.TransactionTag = v
			return nil
		},
	))

	// STATEMENT_TAG
	mustRegister(sysvar.NewStringParser(
		"STATEMENT_TAG",
		"A property of type STRING that contains the request tag for the next statement.",
		func() string { return sv.RequestTag },
		func(v string) error {
			sv.RequestTag = v
			return nil
		},
	))

	// Duration variables

	// MAX_COMMIT_DELAY
	minDelay := time.Duration(0)
	maxDelay := 500 * time.Millisecond
	mustRegister(sysvar.NewNullableDurationParser(
		"MAX_COMMIT_DELAY",
		"The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.",
		func() *time.Duration { return sv.MaxCommitDelay },
		func(v *time.Duration) error {
			sv.MaxCommitDelay = v
			return nil
		},
		&minDelay,
		&maxDelay,
	))

	// STATEMENT_TIMEOUT
	mustRegister(sysvar.NewNullableDurationParser(
		"STATEMENT_TIMEOUT",
		"A property of type STRING indicating the current timeout value for statements (e.g., '10s', '5m', '1h'). Default is '10m'.",
		func() *time.Duration { return sv.StatementTimeout },
		func(v *time.Duration) error {
			sv.StatementTimeout = v
			return nil
		},
		nil, nil, // No min/max constraints
	))

	// More enum variables

	// AUTOCOMMIT_DML_MODE
	autocommitDMLModeValues := map[string]AutocommitDMLMode{
		"TRANSACTIONAL":          AutocommitDMLModeTransactional,
		"PARTITIONED_NON_ATOMIC": AutocommitDMLModePartitionedNonAtomic,
	}
	mustRegister(sysvar.NewEnumParser(
		"AUTOCOMMIT_DML_MODE",
		"A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements.",
		autocommitDMLModeValues,
		func() AutocommitDMLMode { return sv.AutocommitDMLMode },
		func(v AutocommitDMLMode) error {
			sv.AutocommitDMLMode = v
			return nil
		},
		func(v AutocommitDMLMode) string {
			switch v {
			case AutocommitDMLModeTransactional:
				return "TRANSACTIONAL"
			case AutocommitDMLModePartitionedNonAtomic:
				return "PARTITIONED_NON_ATOMIC"
			default:
				return fmt.Sprintf("AutocommitDMLMode(%v)", v)
			}
		},
	))

	// DEFAULT_ISOLATION_LEVEL
	isolationValues := map[string]sppb.TransactionOptions_IsolationLevel{
		"ISOLATION_LEVEL_UNSPECIFIED": sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
	}
	mustRegister(sysvar.NewEnumParser(
		"DEFAULT_ISOLATION_LEVEL",
		"The transaction isolation level that is used by default for read/write transactions.",
		isolationValues,
		func() sppb.TransactionOptions_IsolationLevel { return sv.DefaultIsolationLevel },
		func(v sppb.TransactionOptions_IsolationLevel) error {
			sv.DefaultIsolationLevel = v
			return nil
		},
		func(v sppb.TransactionOptions_IsolationLevel) string {
			switch v {
			case sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED:
				return "ISOLATION_LEVEL_UNSPECIFIED"
			default:
				return fmt.Sprintf("IsolationLevel(%d)", v)
			}
		},
	))

	// CLI_DATABASE_DIALECT
	dialectValues := map[string]databasepb.DatabaseDialect{
		"GOOGLE_STANDARD_SQL": databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"POSTGRESQL":          databasepb.DatabaseDialect_POSTGRESQL,
		"":                    databasepb.DatabaseDialect_DATABASE_DIALECT_UNSPECIFIED,
	}
	mustRegister(sysvar.NewEnumParser(
		"CLI_DATABASE_DIALECT",
		"Database dialect for the session.",
		dialectValues,
		func() databasepb.DatabaseDialect { return sv.DatabaseDialect },
		func(v databasepb.DatabaseDialect) error {
			sv.DatabaseDialect = v
			return nil
		},
		func(v databasepb.DatabaseDialect) string {
			switch v {
			case databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL:
				return "GOOGLE_STANDARD_SQL"
			case databasepb.DatabaseDialect_POSTGRESQL:
				return "POSTGRESQL"
			case databasepb.DatabaseDialect_DATABASE_DIALECT_UNSPECIFIED:
				return ""
			default:
				return fmt.Sprintf("DatabaseDialect(%d)", v)
			}
		},
	))

	// CLI_QUERY_MODE
	queryModeValues := map[string]sppb.ExecuteSqlRequest_QueryMode{
		"NORMAL":     sppb.ExecuteSqlRequest_NORMAL,
		"PLAN":       sppb.ExecuteSqlRequest_PLAN,
		"PROFILE":    sppb.ExecuteSqlRequest_PROFILE,
		"WITH_STATS": sppb.ExecuteSqlRequest_PROFILE, // Alias
	}
	mustRegister(sysvar.NewEnumParser(
		"CLI_QUERY_MODE",
		"Query execution mode.",
		queryModeValues,
		func() sppb.ExecuteSqlRequest_QueryMode {
			if sv.QueryMode == nil {
				return sppb.ExecuteSqlRequest_NORMAL
			}
			return *sv.QueryMode
		},
		func(v sppb.ExecuteSqlRequest_QueryMode) error {
			sv.QueryMode = &v
			return nil
		},
		func(v sppb.ExecuteSqlRequest_QueryMode) string {
			switch v {
			case sppb.ExecuteSqlRequest_NORMAL:
				return "NORMAL"
			case sppb.ExecuteSqlRequest_PLAN:
				return "PLAN"
			case sppb.ExecuteSqlRequest_PROFILE:
				return "PROFILE"
			default:
				return fmt.Sprintf("QueryMode(%d)", v)
			}
		},
	))

	// CLI_LOG_LEVEL
	logLevelValues := map[string]string{
		"DEBUG": "DEBUG",
		"INFO":  "INFO",
		"WARN":  "WARN",
		"ERROR": "ERROR",
	}
	mustRegister(sysvar.NewStringEnumParser(
		"CLI_LOG_LEVEL",
		"Log level for the CLI.",
		logLevelValues,
		func() string {
			switch sv.LogLevel {
			case slog.LevelDebug:
				return "DEBUG"
			case slog.LevelInfo:
				return "INFO"
			case slog.LevelWarn:
				return "WARN"
			case slog.LevelError:
				return "ERROR"
			default:
				return "WARN"
			}
		},
		func(v string) error {
			level, err := SetLogLevel(v)
			if err != nil {
				return err
			}
			sv.LogLevel = level
			return nil
		},
	))

	// More integer variables

	// Note: CLI_CURRENT_WIDTH is handled in the old system (no field in systemVariables)

	// CLI_PORT
	mustRegister(sysvar.NewIntegerParser(
		"CLI_PORT",
		"Port number for connections.",
		func() int64 { return int64(sv.Port) },
		func(v int64) error {
			if v < 0 || v > 65535 {
				return fmt.Errorf("port must be between 0 and 65535")
			}
			sv.Port = int(v)
			return nil
		},
		lo.ToPtr(int64(0)), lo.ToPtr(int64(65535)),
	))

	// Read-only variables

	// READ_TIMESTAMP
	mustRegister(sysvar.NewReadOnlyStringParser(
		"READ_TIMESTAMP",
		"The read timestamp of the most recent read-only transaction.",
		func() string {
			if sv.ReadTimestamp.IsZero() {
				return "NULL"
			}
			return sv.ReadTimestamp.Format(time.RFC3339Nano)
		},
	))

	// COMMIT_TIMESTAMP
	mustRegister(sysvar.NewReadOnlyStringParser(
		"COMMIT_TIMESTAMP",
		"The commit timestamp of the last read-write transaction that Spanner committed.",
		func() string {
			if sv.CommitTimestamp.IsZero() {
				return "NULL"
			}
			return sv.CommitTimestamp.Format(time.RFC3339Nano)
		},
	))

	// CLI_MCP
	mustRegister(sysvar.NewReadOnlyBooleanParser(
		"CLI_MCP",
		"A read-only boolean indicating whether the connection is running as an MCP server.",
		func() bool { return sv.MCP },
	))

	// CLI_VERSION
	mustRegister(sysvar.NewReadOnlyStringParser(
		"CLI_VERSION",
		"The version of spanner-mycli.",
		func() string { return getVersion() },
	))

	// Special variables with complex handling
	// These remain in the old system for now as they require special parsing logic:
	// - READ_ONLY_STALENESS (complex parsing logic)
	// - CLI_OUTPUT_TEMPLATE_FILE (file handling)
	// - CLI_PROTO_DESCRIPTOR_FILE (file handling)
	// - CLI_DIRECT_READ (complex parsing)
	// - CLI_PARSE_MODE (special enum)
	// - CLI_EMULATOR_PLATFORM (architecture detection)
	// - CLI_HOST (special parsing)
	// - COMMIT_RESPONSE (complex result set)

	return registry
}
