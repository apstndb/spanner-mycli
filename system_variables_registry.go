package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Types for variable registration helpers
type simpleVar[T any] struct {
	name  string
	desc  string
	field *T
}

type readOnlyVar[T any] struct {
	name   string
	desc   string
	getter func() T
}

// registerSimpleVariables registers read-write variables with simple field pointers.
// This is intentionally separate from registerReadOnlyVariables to maintain a clear
// distinction between mutable and immutable system variables at the type level.
func registerSimpleVariables[T any](
	registry *sysvar.Registry,
	vars []simpleVar[T],
	dualModeParser sysvar.DualModeParser[T],
	formatter func(T) string,
) {
	for _, v := range vars {
		parser := sysvar.NewTypedVariableParser(
			v.name,
			v.desc,
			dualModeParser,
			func() T { return *v.field },
			func(val T) error {
				*v.field = val
				return nil
			},
			formatter,
		)
		if err := registry.Register(parser); err != nil {
			panic(fmt.Sprintf("Failed to register %s: %v", v.name, err))
		}
	}
}

// registerReadOnlyVariables registers read-only variables that only have getters.
// Keeping this separate from registerSimpleVariables ensures compile-time safety
// and makes the read-only nature of these variables explicit in the code structure.
func registerReadOnlyVariables[T any](
	registry *sysvar.Registry,
	vars []readOnlyVar[T],
	dualModeParser sysvar.DualModeParser[T],
	formatter func(T) string,
) {
	for _, v := range vars {
		parser := sysvar.NewTypedVariableParser(
			v.name,
			v.desc,
			dualModeParser,
			v.getter,
			nil, // Read-only, no setter
			formatter,
		)
		if err := registry.Register(parser); err != nil {
			panic(fmt.Sprintf("Failed to register %s: %v", v.name, err))
		}
	}
}

// createSystemVariableRegistry creates and configures the parser registry for system variables.
// This function sets up all the system variable parsers with their getters and setters.
func createSystemVariableRegistry(sv *systemVariables) *sysvar.Registry {
	registry := sysvar.NewRegistry()

	// Register java-spanner compatible variables for compatibility with Java client
	registerJavaSpannerCompatibleVariables(registry, sv)

	// Register spanner-mycli specific CLI variables
	registerSpannerMyCLIVariables(registry, sv)

	return registry
}

// mustRegister registers a variable parser with the registry and panics on error.
// This is used during initialization where registration failures are fatal.
func mustRegister(registry *sysvar.Registry, parser sysvar.VariableParser) {
	if err := registry.Register(parser); err != nil {
		panic(fmt.Sprintf("Failed to register %s: %v", parser.Name(), err))
	}
}

// registerJavaSpannerCompatibleVariables registers variables that maintain compatibility
// with the java-spanner client library. These variables follow the same naming conventions
// and behavior as the Java implementation.
func registerJavaSpannerCompatibleVariables(registry *sysvar.Registry, sv *systemVariables) {
	// Connection control
	mustRegister(registry, sysvar.NewBooleanParser(
		"READONLY",
		"A boolean indicating whether or not the connection is in read-only mode. The default is false.",
		sysvar.GetValue(&sv.ReadOnly),
		func(v bool) error {
			if sv.CurrentSession != nil && (sv.CurrentSession.InReadOnlyTransaction() || sv.CurrentSession.InReadWriteTransaction()) {
				return errors.New("can't change READONLY when there is a active transaction")
			}
			sv.ReadOnly = v
			return nil
		},
	))

	// Java-Spanner boolean variables
	registerSimpleVariables(registry, []simpleVar[bool]{
		{"AUTO_PARTITION_MODE", "A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.", &sv.AutoPartitionMode},
		{"RETURN_COMMIT_STATS", "A property of type BOOL indicating whether statistics should be returned for transactions on this connection.", &sv.ReturnCommitStats},
		{"AUTO_BATCH_DML", "A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false.", &sv.AutoBatchDML},
		{"DATA_BOOST_ENABLED", "A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.", &sv.DataBoostEnabled},
		{"EXCLUDE_TXN_FROM_CHANGE_STREAMS", "Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).", &sv.ExcludeTxnFromChangeStreams},
	}, sysvar.DualModeBoolParser, sysvar.FormatBool)

	// Integer configuration
	registerSimpleVariables(registry, []simpleVar[int64]{
		{"MAX_PARTITIONED_PARALLELISM", "A property of type `INT64` indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for `AUTO_PARTITION_MODE=TRUE` and `RUN PARTITIONED QUERY`", &sv.MaxPartitionedParallelism},
	}, sysvar.DualModeIntParser, sysvar.FormatInt)

	// DEFAULT_ISOLATION_LEVEL
	sysvar.RegisterProtobufEnumWithAliases(registry,
		"DEFAULT_ISOLATION_LEVEL",
		"The transaction isolation level that is used by default for read/write transactions.",
		sppb.TransactionOptions_IsolationLevel_value,
		"ISOLATION_LEVEL_",
		sysvar.GetValue(&sv.DefaultIsolationLevel),
		sysvar.SetValue(&sv.DefaultIsolationLevel),
		map[sppb.TransactionOptions_IsolationLevel][]string{
			sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED: {"UNSPECIFIED"},
		},
	)

	// Transaction tagging
	registerSimpleVariables(registry, []simpleVar[string]{
		{"TRANSACTION_TAG", "A property of type STRING that contains the transaction tag for the next transaction.", &sv.TransactionTag},
		{"STATEMENT_TAG", "A property of type STRING that contains the request tag for the next statement.", &sv.RequestTag},
	}, sysvar.DualModeStringParser, sysvar.FormatString)

	// MAX_COMMIT_DELAY
	minDelay := time.Duration(0)
	maxDelay := 500 * time.Millisecond
	mustRegister(registry, sysvar.NewNullableDurationVariableParser(
		"MAX_COMMIT_DELAY",
		"The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.",
		sysvar.GetValue(&sv.MaxCommitDelay),
		sysvar.SetValue(&sv.MaxCommitDelay),
		&minDelay,
		&maxDelay,
	))

	// AUTOCOMMIT_DML_MODE
	autocommitDMLModeValues := map[string]AutocommitDMLMode{
		"TRANSACTIONAL":          AutocommitDMLModeTransactional,
		"PARTITIONED_NON_ATOMIC": AutocommitDMLModePartitionedNonAtomic,
	}
	mustRegister(registry, sysvar.NewEnumVariableParser(
		"AUTOCOMMIT_DML_MODE",
		"A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements.",
		autocommitDMLModeValues,
		sysvar.GetValue(&sv.AutocommitDMLMode),
		sysvar.SetValue(&sv.AutocommitDMLMode),
		sysvar.FormatEnumFromMap("AutocommitDMLMode", autocommitDMLModeValues),
	))

	// Optimizer configuration
	registerSimpleVariables(registry, []simpleVar[string]{
		{"OPTIMIZER_VERSION", "A property of type `STRING` indicating the optimizer version. The version is either an integer string or 'LATEST'.", &sv.OptimizerVersion},
		{"OPTIMIZER_STATISTICS_PACKAGE", "A property of type STRING indicating the current optimizer statistics package that is used by this connection.", &sv.OptimizerStatisticsPackage},
	}, sysvar.DualModeStringParser, sysvar.FormatString)

	// RPC configuration
	sysvar.RegisterProtobufEnum(registry,
		"RPC_PRIORITY",
		"A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.",
		sppb.RequestOptions_Priority_value,
		"PRIORITY_",
		sysvar.GetValue(&sv.RPCPriority),
		sysvar.SetValue(&sv.RPCPriority),
	)

	// Statement timeout
	mustRegister(registry, sysvar.NewNullableDurationVariableParser(
		"STATEMENT_TIMEOUT",
		"A property of type STRING indicating the current timeout value for statements (e.g., '10s', '5m', '1h'). Default is '10m'.",
		sysvar.GetValue(&sv.StatementTimeout),
		sysvar.SetValue(&sv.StatementTimeout),
		lo.ToPtr(time.Duration(0)), nil, // Min: 0, no max
	))

	// Read-only timestamps (java-spanner compatible)
	mustRegister(registry, sysvar.NewReadOnlyStringParser(
		"READ_TIMESTAMP",
		"The read timestamp of the most recent read-only transaction.",
		sysvar.FormatTimestamp(&sv.ReadTimestamp),
	))

	mustRegister(registry, sysvar.NewReadOnlyStringParser(
		"COMMIT_TIMESTAMP",
		"The commit timestamp of the last read-write transaction that Spanner committed.",
		sysvar.FormatTimestamp(&sv.CommitTimestamp),
	))
}

// registerSpannerMyCLIVariables registers variables specific to spanner-mycli.
// These variables use the CLI_ prefix and provide additional functionality
// beyond what the java-spanner client offers.
func registerSpannerMyCLIVariables(registry *sysvar.Registry, sv *systemVariables) {
	// Output formatting variables
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
	mustRegister(registry, sysvar.NewEnumVariableParser(
		"CLI_FORMAT",
		"Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated), HTML (HTML table), XML (XML format), CSV (comma-separated values).",
		formatValues,
		sysvar.GetValue(&sv.CLIFormat),
		sysvar.SetValue(&sv.CLIFormat),
		sysvar.FormatEnumFromMap("DisplayMode", formatValues),
	))

	// Output formatting boolean variables
	registerSimpleVariables(registry, []simpleVar[bool]{
		{"CLI_SKIP_COLUMN_NAMES", "A boolean indicating whether to suppress column headers in output. The default is false.", &sv.SkipColumnNames},
		{"CLI_AUTOWRAP", "Enable automatic line wrapping.", &sv.AutoWrap},
		{"CLI_ENABLE_HIGHLIGHT", "Enable syntax highlighting.", &sv.EnableHighlight},
		{"CLI_PROTOTEXT_MULTILINE", "Enable multiline prototext output.", &sv.MultilineProtoText},
		{"CLI_MARKDOWN_CODEBLOCK", "Enable markdown codeblock output.", &sv.MarkdownCodeblock},
	}, sysvar.DualModeBoolParser, sysvar.FormatBool)

	mustRegister(registry, sysvar.NewNullableIntVariableParser(
		"CLI_FIXED_WIDTH",
		"If set, limits output width to the specified number of characters. NULL means automatic width detection.",
		sysvar.GetValue(&sv.FixedWidth),
		sysvar.SetValue(&sv.FixedWidth),
		nil, nil, // No min/max constraints
	))

	// Display and formatting integers
	registerSimpleVariables(registry, []simpleVar[int64]{
		{"CLI_TAB_WIDTH", "Tab width. It is used for expanding tabs.", &sv.TabWidth},
		{"CLI_EXPLAIN_WRAP_WIDTH", "Controls query plan wrap width. It effects only operators column contents", &sv.ExplainWrapWidth},
	}, sysvar.DualModeIntParser, sysvar.FormatInt)

	// CLI_EXPLAIN_FORMAT
	mustRegister(registry, sysvar.NewSimpleEnumParser(
		"CLI_EXPLAIN_FORMAT",
		"Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.",
		sysvar.SliceToEnumMap([]explainFormat{
			explainFormatUnspecified,
			explainFormatCurrent,
			explainFormatTraditional,
			explainFormatCompact,
		}),
		sysvar.GetValue(&sv.ExplainFormat),
		sysvar.SetValue(&sv.ExplainFormat),
	))

	mustRegister(registry, sysvar.NewStringVariableParser(
		"CLI_ANALYZE_COLUMNS",
		"Go template for analyzing column data.",
		sysvar.GetValue(&sv.AnalyzeColumns),
		func(v string) error {
			def, err := customListToTableRenderDefs(unquoteString(v))
			if err != nil {
				return err
			}

			sv.AnalyzeColumns = v
			sv.ParsedAnalyzeColumns = def
			return nil
		},
	))

	mustRegister(registry, sysvar.NewStringVariableParser(
		"CLI_INLINE_STATS",
		"<name>:<template>, ...",
		sysvar.GetValue(&sv.InlineStats),
		func(v string) error {
			unquoted := unquoteString(v)
			defs, err := parseInlineStatsDefs(unquoted)
			if err != nil {
				return err
			}

			sv.InlineStats = unquoted
			sv.ParsedInlineStats = defs
			return nil
		},
	))

	// User interface and interaction variables
	registerSimpleVariables(registry, []simpleVar[bool]{
		{"CLI_USE_PAGER", "Enable pager for output.", &sv.UsePager},
		{"CLI_ENABLE_PROGRESS_BAR", "A boolean indicating whether to display progress bars during operations. The default is false.", &sv.EnableProgressBar},
		{"CLI_SKIP_SYSTEM_COMMAND", "Controls whether system commands are disabled.", &sv.SkipSystemCommand},
	}, sysvar.DualModeBoolParser, sysvar.FormatBool)

	// Prompt configuration
	registerSimpleVariables(registry, []simpleVar[string]{
		{"CLI_PROMPT", "Custom prompt for spanner-mycli.", &sv.Prompt},
	}, sysvar.DualModeStringParser, sysvar.FormatString)

	mustRegister(registry, sysvar.NewStringVariableParser(
		"CLI_PROMPT2",
		"Custom continuation prompt for spanner-mycli.",
		sysvar.GetValue(&sv.Prompt2),
		sysvar.SetValue(&sv.Prompt2),
	))

	mustRegister(registry, sysvar.NewReadOnlyStringParser(
		"CLI_HISTORY_FILE",
		"Path to the history file.",
		sysvar.GetValue(&sv.HistoryFile),
	))

	// Debug and logging variables
	registerSimpleVariables(registry, []simpleVar[bool]{
		{"CLI_VERBOSE", "Display verbose output.", &sv.Verbose},
		{"CLI_ECHO_EXECUTED_DDL", "Echo executed DDL statements.", &sv.EchoExecutedDDL},
		{"CLI_ECHO_INPUT", "Echo input statements.", &sv.EchoInput},
		{"CLI_LINT_PLAN", "Enable query plan linting.", &sv.LintPlan},
	}, sysvar.DualModeBoolParser, sysvar.FormatBool)

	// CLI_LOG_LEVEL
	// Use enum parser with slog.Level's String() values
	logLevelValues := map[string]slog.Level{
		"DEBUG": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"WARN":  slog.LevelWarn,
		"ERROR": slog.LevelError,
	}
	mustRegister(registry, sysvar.NewEnumVariableParser(
		"CLI_LOG_LEVEL",
		"Log level for the CLI.",
		logLevelValues,
		sysvar.GetValue(&sv.LogLevel),
		func(v slog.Level) error {
			sv.LogLevel = v
			// Re-initialize the logger with the new level
			h := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: v,
			}))
			slog.SetDefault(h)
			return nil
		},
		slog.Level.String,
	))

	// Read-only debug variables
	registerReadOnlyVariables(registry, []readOnlyVar[bool]{
		{"CLI_LOG_GRPC", "Enable gRPC logging.", sysvar.GetValue(&sv.LogGrpc)},
		{"CLI_INSECURE", "Skip TLS certificate verification (insecure).", sysvar.GetValue(&sv.Insecure)},
	}, sysvar.DualModeBoolParser, sysvar.FormatBool)

	// Session and connection information (read-only)
	registerReadOnlyVariables(registry, []readOnlyVar[string]{
		{"CLI_PROJECT", "GCP Project ID.", sysvar.GetValue(&sv.Project)},
		{"CLI_INSTANCE", "Cloud Spanner instance ID.", sysvar.GetValue(&sv.Instance)},
		{"CLI_DATABASE", "Cloud Spanner database ID.", sysvar.GetValue(&sv.Database)},
		{"CLI_ROLE", "Cloud Spanner database role.", sysvar.GetValue(&sv.Role)},
		{"CLI_HOST", "Host on which Spanner server is located", sysvar.GetValue(&sv.Host)},
		{"CLI_IMPERSONATE_SERVICE_ACCOUNT", "Service account to impersonate.", sysvar.GetValue(&sv.ImpersonateServiceAccount)},
	}, sysvar.DualModeStringParser, sysvar.FormatString)

	mustRegister(registry, sysvar.NewIntegerVariableParser(
		"CLI_PORT",
		"Port number for connections.",
		sysvar.GetConvertedValue(&sv.Port, func(p int) int64 { return int64(p) }),
		nil,      // No setter - read-only
		nil, nil, // No min/max validation needed for read-only
	))

	// Session-init-only variable
	mustRegister(registry, sysvar.NewBooleanParser(
		"CLI_ENABLE_ADC_PLUS",
		"A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.",
		sysvar.GetValue(&sv.EnableADCPlus),
		sysvar.SetSessionInitOnly(&sv.EnableADCPlus, "CLI_ENABLE_ADC_PLUS", &sv.CurrentSession),
	))

	// CLI_MCP
	mustRegister(registry, sysvar.NewReadOnlyBooleanParser(
		"CLI_MCP",
		"A read-only boolean indicating whether the connection is running as an MCP server.",
		sysvar.GetValue(&sv.MCP),
	))

	// CLI_VERSION
	mustRegister(registry, sysvar.NewReadOnlyStringParser(
		"CLI_VERSION",
		"The version of spanner-mycli.",
		getVersion,
	))

	// External integrations
	// Vertex AI integration
	registerSimpleVariables(registry, []simpleVar[string]{
		{"CLI_VERTEXAI_MODEL", "Vertex AI model for natural language features.", &sv.VertexAIModel},
		{"CLI_VERTEXAI_PROJECT", "Vertex AI project for natural language features.", &sv.VertexAIProject},
	}, sysvar.DualModeStringParser, sysvar.FormatString)

	// Proto descriptor files
	mustRegister(registry, sysvar.NewProtoDescriptorFileParser(
		"CLI_PROTO_DESCRIPTOR_FILE",
		"Comma-separated list of proto descriptor files. Supports ADD to append files.",
		sysvar.GetValue(&sv.ProtoDescriptorFile),
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

	// Query execution features
	registerSimpleVariables(registry, []simpleVar[bool]{
		{"CLI_TRY_PARTITION_QUERY", "A boolean indicating whether to test query for partition compatibility instead of executing it.", &sv.TryPartitionQuery},
		{"CLI_AUTO_CONNECT_AFTER_CREATE", "A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.", &sv.AutoConnectAfterCreate},
		{"CLI_ASYNC_DDL", "A boolean indicating whether DDL statements should be executed asynchronously. The default is false.", &sv.AsyncDDL},
	}, sysvar.DualModeBoolParser, sysvar.FormatBool)

	// CLI_DATABASE_DIALECT
	sysvar.RegisterProtobufEnumWithAliases(registry,
		"CLI_DATABASE_DIALECT",
		"Database dialect for the session.",
		databasepb.DatabaseDialect_value,
		"", // No prefix to strip
		sysvar.GetValue(&sv.DatabaseDialect),
		sysvar.SetValue(&sv.DatabaseDialect),
		map[databasepb.DatabaseDialect][]string{
			databasepb.DatabaseDialect_DATABASE_DIALECT_UNSPECIFIED: {""},
		},
	)

	// CLI_QUERY_MODE
	mustRegister(registry, sysvar.CreateProtobufEnumParser(
		"CLI_QUERY_MODE",
		"Query execution mode.",
		sppb.ExecuteSqlRequest_QueryMode_value,
		"", // No prefix to strip
		sysvar.GetValueOrDefault(&sv.QueryMode, sppb.ExecuteSqlRequest_NORMAL),
		sysvar.SetPointerValue(&sv.QueryMode),
	))

	// CLI_PARSE_MODE
	mustRegister(registry, sysvar.NewSimpleEnumParser(
		"CLI_PARSE_MODE",
		"Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED",
		sysvar.BuildEnumMapWithAliases(
			[]parseMode{
				parseModeUnspecified, // ""
				parseModeFallback,    // "FALLBACK"
				parseModeNoMemefish,  // "NO_MEMEFISH"
				parseMemefishOnly,    // "MEMEFISH_ONLY"
			},
			map[parseMode][]string{
				parseModeUnspecified: {"UNSPECIFIED"}, // Alias for ""
			},
		),
		sysvar.GetValue(&sv.BuildStatementMode),
		sysvar.SetValue(&sv.BuildStatementMode),
	))
}
