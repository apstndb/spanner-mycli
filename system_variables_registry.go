package main

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/descriptorpb"
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
	mustRegister(sysvar.NewSimpleBooleanParser(
		"AUTO_PARTITION_MODE",
		"A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.",
		&sv.AutoPartitionMode,
	))

	// RETURN_COMMIT_STATS
	mustRegister(sysvar.NewSimpleBooleanParser(
		"RETURN_COMMIT_STATS",
		"A property of type BOOL indicating whether statistics should be returned for transactions on this connection.",
		&sv.ReturnCommitStats,
	))

	// AUTO_BATCH_DML
	mustRegister(sysvar.NewSimpleBooleanParser(
		"AUTO_BATCH_DML",
		"A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false.",
		&sv.AutoBatchDML,
	))

	// DATA_BOOST_ENABLED
	mustRegister(sysvar.NewSimpleBooleanParser(
		"DATA_BOOST_ENABLED",
		"A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.",
		&sv.DataBoostEnabled,
	))

	// EXCLUDE_TXN_FROM_CHANGE_STREAMS
	mustRegister(sysvar.NewSimpleBooleanParser(
		"EXCLUDE_TXN_FROM_CHANGE_STREAMS",
		"Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).",
		&sv.ExcludeTxnFromChangeStreams,
	))

	// CLI variables - boolean

	// CLI_VERBOSE
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_VERBOSE",
		"Display verbose output.",
		&sv.Verbose,
	))

	// CLI_ECHO_EXECUTED_DDL
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_ECHO_EXECUTED_DDL",
		"Echo executed DDL statements.",
		&sv.EchoExecutedDDL,
	))

	// CLI_ECHO_INPUT
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_ECHO_INPUT",
		"Echo input statements.",
		&sv.EchoInput,
	))

	// CLI_USE_PAGER
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_USE_PAGER",
		"Enable pager for output.",
		&sv.UsePager,
	))

	// CLI_AUTOWRAP
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_AUTOWRAP",
		"Enable automatic line wrapping.",
		&sv.AutoWrap,
	))

	// CLI_ENABLE_HIGHLIGHT
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_ENABLE_HIGHLIGHT",
		"Enable syntax highlighting.",
		&sv.EnableHighlight,
	))

	// CLI_PROTOTEXT_MULTILINE
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_PROTOTEXT_MULTILINE",
		"Enable multiline prototext output.",
		&sv.MultilineProtoText,
	))

	// CLI_MARKDOWN_CODEBLOCK
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_MARKDOWN_CODEBLOCK",
		"Enable markdown codeblock output.",
		&sv.MarkdownCodeblock,
	))

	// CLI_TRY_PARTITION_QUERY
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_TRY_PARTITION_QUERY",
		"A boolean indicating whether to test query for partition compatibility instead of executing it.",
		&sv.TryPartitionQuery,
	))

	// CLI_AUTO_CONNECT_AFTER_CREATE
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_AUTO_CONNECT_AFTER_CREATE",
		"A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.",
		&sv.AutoConnectAfterCreate,
	))

	// CLI_ENABLE_PROGRESS_BAR
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_ENABLE_PROGRESS_BAR",
		"A boolean indicating whether to display progress bars during operations. The default is false.",
		&sv.EnableProgressBar,
	))

	// CLI_ENABLE_ADC_PLUS
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_ENABLE_ADC_PLUS",
		"A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.",
		&sv.EnableADCPlus,
	))

	// CLI_ASYNC_DDL
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_ASYNC_DDL",
		"A boolean indicating whether DDL statements should be executed asynchronously. The default is false.",
		&sv.AsyncDDL,
	))

	// CLI_SKIP_COLUMN_NAMES
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_SKIP_COLUMN_NAMES",
		"A boolean indicating whether to suppress column headers in output. The default is false.",
		&sv.SkipColumnNames,
	))

	// CLI_LINT_PLAN (special case with conditional getter)
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_LINT_PLAN",
		"Enable query plan linting.",
		&sv.LintPlan,
	))

	// Integer variables

	// MAX_PARTITIONED_PARALLELISM
	mustRegister(sysvar.NewSimpleIntegerParser(
		"MAX_PARTITIONED_PARALLELISM",
		"A property of type `INT64` indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for `AUTO_PARTITION_MODE=TRUE` and `RUN PARTITIONED QUERY`",
		&sv.MaxPartitionedParallelism,
	))

	// CLI_TAB_WIDTH
	mustRegister(sysvar.NewSimpleIntegerParser(
		"CLI_TAB_WIDTH",
		"Tab width. It is used for expanding tabs.",
		&sv.TabWidth,
	))

	// CLI_EXPLAIN_WRAP_WIDTH
	mustRegister(sysvar.NewSimpleIntegerParser(
		"CLI_EXPLAIN_WRAP_WIDTH",
		"Controls query plan wrap width. It effects only operators column contents",
		&sv.ExplainWrapWidth,
	))

	// String variables

	// OPTIMIZER_VERSION
	mustRegister(sysvar.NewSimpleStringParser(
		"OPTIMIZER_VERSION",
		"A property of type `STRING` indicating the optimizer version. The version is either an integer string or 'LATEST'.",
		&sv.OptimizerVersion,
	))

	// OPTIMIZER_STATISTICS_PACKAGE
	mustRegister(sysvar.NewSimpleStringParser(
		"OPTIMIZER_STATISTICS_PACKAGE",
		"A property of type STRING indicating the current optimizer statistics package that is used by this connection.",
		&sv.OptimizerStatisticsPackage,
	))

	// CLI_PROMPT
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_PROMPT",
		"Custom prompt for spanner-mycli.",
		&sv.Prompt,
	))

	// CLI_PROMPT2
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_PROMPT2",
		"Custom continuation prompt for spanner-mycli.",
		&sv.Prompt2,
	))

	// CLI_VERTEXAI_MODEL
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_VERTEXAI_MODEL",
		"Vertex AI model for natural language features.",
		&sv.VertexAIModel,
	))

	// CLI_VERTEXAI_PROJECT
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_VERTEXAI_PROJECT",
		"Vertex AI project for natural language features.",
		&sv.VertexAIProject,
	))

	// Enum variables

	// RPC_PRIORITY
	mustRegister(sysvar.NewEnumParser(
		"RPC_PRIORITY",
		"A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.",
		map[string]sppb.RequestOptions_Priority{
			"UNSPECIFIED": sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			"LOW":         sppb.RequestOptions_PRIORITY_LOW,
			"MEDIUM":      sppb.RequestOptions_PRIORITY_MEDIUM,
			"HIGH":        sppb.RequestOptions_PRIORITY_HIGH,
		},
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
			// Reverse map lookup for DisplayMode
			for name, value := range formatValues {
				if value == v {
					return name
				}
			}
			return fmt.Sprintf("DisplayMode(%d)", v)
		},
	))

	// CLI_EXPLAIN_FORMAT
	mustRegister(sysvar.NewSimpleEnumParser(
		"CLI_EXPLAIN_FORMAT",
		"Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.",
		map[string]explainFormat{
			"":            explainFormatUnspecified,
			"CURRENT":     explainFormatCurrent,
			"TRADITIONAL": explainFormatTraditional,
			"COMPACT":     explainFormatCompact,
		},
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
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_SKIP_SYSTEM_COMMAND",
		"Controls whether system commands are disabled.",
		&sv.SkipSystemCommand,
	))

	// CLI_LOG_GRPC
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_LOG_GRPC",
		"Enable gRPC logging.",
		&sv.LogGrpc,
	))

	// CLI_INSECURE
	mustRegister(sysvar.NewSimpleBooleanParser(
		"CLI_INSECURE",
		"Skip TLS certificate verification (insecure).",
		&sv.Insecure,
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
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_HISTORY_FILE",
		"Path to the history file.",
		&sv.HistoryFile,
	))

	// CLI_PROJECT
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_PROJECT",
		"GCP Project ID.",
		&sv.Project,
	))

	// CLI_INSTANCE
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_INSTANCE",
		"Cloud Spanner instance ID.",
		&sv.Instance,
	))

	// CLI_DATABASE
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_DATABASE",
		"Cloud Spanner database ID.",
		&sv.Database,
	))

	// CLI_ROLE
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_ROLE",
		"Cloud Spanner database role.",
		&sv.Role,
	))

	// Note: CLI_ENDPOINT is handled specially in the old system (parses to host/port)

	// CLI_IMPERSONATE_SERVICE_ACCOUNT
	mustRegister(sysvar.NewSimpleStringParser(
		"CLI_IMPERSONATE_SERVICE_ACCOUNT",
		"Service account to impersonate.",
		&sv.ImpersonateServiceAccount,
	))

	// TRANSACTION_TAG
	mustRegister(sysvar.NewSimpleStringParser(
		"TRANSACTION_TAG",
		"A property of type STRING that contains the transaction tag for the next transaction.",
		&sv.TransactionTag,
	))

	// STATEMENT_TAG
	mustRegister(sysvar.NewSimpleStringParser(
		"STATEMENT_TAG",
		"A property of type STRING that contains the request tag for the next statement.",
		&sv.RequestTag,
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
			// Reverse map lookup for AutocommitDMLMode
			for name, value := range autocommitDMLModeValues {
				if value == v {
					return name
				}
			}
			return fmt.Sprintf("AutocommitDMLMode(%v)", v)
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
			// Use protobuf String() method to get full name, then extract short form
			fullName := v.String()
			if strings.HasPrefix(fullName, "ISOLATION_LEVEL_") {
				return strings.TrimPrefix(fullName, "ISOLATION_LEVEL_")
			}
			return fullName
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

	// CLI_PROTO_DESCRIPTOR_FILE - supports both Set and Add operations
	mustRegister(sysvar.NewProtoDescriptorFileParser(
		"CLI_PROTO_DESCRIPTOR_FILE",
		"Comma-separated list of proto descriptor files. Supports ADD to append files.",
		func() []string { return sv.ProtoDescriptorFile },
		func(files []string) error {
			// Set operation - replace all files
			if len(files) == 0 {
				sv.ProtoDescriptorFile = []string{}
				sv.ProtoDescriptor = nil
				return nil
			}

			var fileDescriptorSet *descriptorpb.FileDescriptorSet
			for _, filename := range files {
				fds, err := readFileDescriptorProtoFromFile(filename)
				if err != nil {
					return err
				}
				fileDescriptorSet = mergeFDS(fileDescriptorSet, fds)
			}

			sv.ProtoDescriptorFile = files
			sv.ProtoDescriptor = fileDescriptorSet
			return nil
		},
		func(filename string) error {
			// Add operation - append a file
			fds, err := readFileDescriptorProtoFromFile(filename)
			if err != nil {
				return err
			}

			if !slices.Contains(sv.ProtoDescriptorFile, filename) {
				sv.ProtoDescriptorFile = slices.Concat(sv.ProtoDescriptorFile, sliceOf(filename))
				sv.ProtoDescriptor = &descriptorpb.FileDescriptorSet{File: slices.Concat(sv.ProtoDescriptor.GetFile(), fds.GetFile())}
			} else {
				sv.ProtoDescriptor = mergeFDS(sv.ProtoDescriptor, fds)
			}
			return nil
		},
		nil, // No additional validation needed - readFileDescriptorProtoFromFile does validation
	))

	// CLI_PARSE_MODE
	mustRegister(sysvar.NewSimpleEnumParser(
		"CLI_PARSE_MODE",
		"Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED",
		map[string]parseMode{
			"FALLBACK":      parseModeFallback,
			"NO_MEMEFISH":   parseModeNoMemefish,
			"MEMEFISH_ONLY": parseMemefishOnly,
			"UNSPECIFIED":   parseModeUnspecified,
			"":              parseModeUnspecified, // Allow empty string
		},
		func() parseMode { return sv.BuildStatementMode },
		func(v parseMode) error {
			sv.BuildStatementMode = v
			return nil
		},
	))

	// Special variables with complex handling
	// These remain in the old system for now as they require special parsing logic:
	// - READ_ONLY_STALENESS (complex parsing logic)
	// - CLI_OUTPUT_TEMPLATE_FILE (file handling)
	// - CLI_DIRECT_READ (complex parsing)
	// - CLI_EMULATOR_PLATFORM (architecture detection)
	// - CLI_HOST (special parsing)
	// - COMMIT_RESPONSE (complex result set)

	return registry
}
