package mycli

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
)

// clientSideStatementDescription is a human-readable part of clientSideStatementDef.
type clientSideStatementDescription struct {
	// Usage is a purpose of the statement.
	Usage string

	// Syntax is human-readable statement syntax.
	// In the following syntax, we use `<>` for a placeholder, `[]` for an optional keyword, and `{A|B|...}` for a mutually exclusive keyword.
	Syntax string

	// Note is additional information to be printed by --statement-hint, only for README.md.
	Note string
}

// fuzzyCompletionType represents what kind of candidates to provide for argument completion.
type fuzzyCompletionType int

const (
	fuzzyCompleteDatabase fuzzyCompletionType = iota + 1
	fuzzyCompleteVariable
	fuzzyCompleteTable
	fuzzyCompleteVariableValue
	fuzzyCompleteRole
	fuzzyCompleteOperation
	fuzzyCompleteView
	fuzzyCompleteIndex
	fuzzyCompleteChangeStream
	fuzzyCompleteSequence
	fuzzyCompleteModel
	fuzzyCompleteSchema
	fuzzyCompleteParam
)

func (t fuzzyCompletionType) String() string {
	switch t {
	case fuzzyCompleteDatabase:
		return "database"
	case fuzzyCompleteVariable:
		return "variable"
	case fuzzyCompleteTable:
		return "table"
	case fuzzyCompleteVariableValue:
		return "variable_value"
	case fuzzyCompleteRole:
		return "role"
	case fuzzyCompleteOperation:
		return "operation"
	case fuzzyCompleteView:
		return "view"
	case fuzzyCompleteIndex:
		return "index"
	case fuzzyCompleteChangeStream:
		return "change_stream"
	case fuzzyCompleteSequence:
		return "sequence"
	case fuzzyCompleteModel:
		return "model"
	case fuzzyCompleteSchema:
		return "schema"
	case fuzzyCompleteParam:
		return "param"
	default:
		return fmt.Sprintf("unhandled fuzzyCompletionType: %d", t)
	}
}

// fuzzyArgCompletion defines how to detect and complete an argument for a client-side statement.
type fuzzyArgCompletion struct {
	// PrefixPattern matches partial input and captures the argument being typed in group 1.
	PrefixPattern *regexp.Regexp

	// CompletionType specifies what candidates to fetch.
	CompletionType fuzzyCompletionType

	// Suffix is appended to the selected candidate after insertion (e.g., " = " for SET variable name).
	Suffix string
}

type clientSideStatementDef struct {
	// Descriptions represents human-readable descriptions.
	// It can be multiple because some clientSideStatementDef represents multiple statements in single pattern.
	Descriptions []clientSideStatementDescription

	// Pattern is a compiled regular expression for the statement.
	// It must be matched on the whole statement without semicolon, and case-insensitive.
	Pattern *regexp.Regexp

	// HandleGroups holds a handler which converts named capture groups to Statement.
	// Named groups are extracted from Pattern using namedGroups().
	HandleGroups func(groups map[string]string) (Statement, error)

	// Completion defines optional fuzzy argument completion for this statement.
	// When non-nil, each entry's PrefixPattern is tried against partial input to detect argument completion.
	Completion []fuzzyArgCompletion
}

// namedGroups extracts named capture groups from a regexp match into a map.
func namedGroups(re *regexp.Regexp, match []string) map[string]string {
	groups := make(map[string]string, len(match)-1)
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" {
			groups[name] = match[i]
		}
	}
	return groups
}

var schemaObjectsReStr = strings.Join(slices.Collect(loi.Map(slices.Values([]string{
	"SCHEMA",
	"DATABASE",
	"PLACEMENT",
	"PROTO BUNDLE",
	"TABLE",
	"INDEX",
	"SEARCH INDEX",
	"VIEW",
	"CHANGE STREAM",
	"ROLE",
	"SEQUENCE",
	"MODEL",
	"VECTOR INDEX",
	"PROPERTY GRAPH",
}), func(s string) string {
	return strings.ReplaceAll(s, " ", `\s+`)
})), "|")

var whitespaceRe = regexp.MustCompile(`\s+`)

var clientSideStatementDefs = []*clientSideStatementDef{
	// Database
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Switch database`,
				Syntax: `USE <database> [ROLE <role>]`,
				Note:   `The role you set is used for accessing with [fine-grained access control](https://cloud.google.com/spanner/docs/fgac-about).`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^USE\s+(?P<database>[^\s]+)(?:\s+ROLE\s+(?P<role>.+))?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &UseStatement{Database: unquoteIdentifier(groups["database"]), Role: unquoteIdentifier(groups["role"])}, nil
		},
		Completion: []fuzzyArgCompletion{
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*USE\s+(\S+)\s+ROLE\s+(\S*)$`),
				CompletionType: fuzzyCompleteRole,
			},
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*USE\s+(\S*)$`),
				CompletionType: fuzzyCompleteDatabase,
			},
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Detach from database`,
				Syntax: `DETACH`,
				Note:   `Switch to detached mode, disconnecting from the current database.`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DETACH$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &DetachStatement{}, nil
		},
	},
	{
		// DROP DATABASE is not native Cloud Spanner statement
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Drop database`,
				Syntax: `DROP DATABASE <database>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DROP\s+DATABASE\s+(?P<database>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &DropDatabaseStatement{DatabaseId: unquoteIdentifier(groups["database"])}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*DROP\s+DATABASE\s+(\S*)$`),
			CompletionType: fuzzyCompleteDatabase,
		}},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `List databases`,
				Syntax: `SHOW DATABASES`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+DATABASES$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowDatabasesStatement{}, nil
		},
	},
	// Schema
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show DDL of the schema object`,
				Syntax: `SHOW CREATE <type> <fqn>`,
			},
		},
		Pattern: regexp.MustCompile(fmt.Sprintf(`(?is)^SHOW\s+CREATE\s+(?P<type>%s)\s+(?P<fqn>.+)$`, schemaObjectsReStr)),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			objectType := strings.ToUpper(whitespaceRe.ReplaceAllString(groups["type"], " "))
			schema, name := extractSchemaAndName(unquoteIdentifier(groups["fqn"]))
			return &ShowCreateStatement{ObjectType: objectType, Schema: schema, Name: name}, nil
		},
		Completion: []fuzzyArgCompletion{
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+CREATE\s+CHANGE\s+STREAM\s+(\S*)$`),
				CompletionType: fuzzyCompleteChangeStream,
			},
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+CREATE\s+TABLE\s+(\S*)$`),
				CompletionType: fuzzyCompleteTable,
			},
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+CREATE\s+VIEW\s+(\S*)$`),
				CompletionType: fuzzyCompleteView,
			},
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+CREATE\s+INDEX\s+(\S*)$`),
				CompletionType: fuzzyCompleteIndex,
			},
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+CREATE\s+SEQUENCE\s+(\S*)$`),
				CompletionType: fuzzyCompleteSequence,
			},
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+CREATE\s+MODEL\s+(\S*)$`),
				CompletionType: fuzzyCompleteModel,
			},
			{
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+CREATE\s+SCHEMA\s+(\S*)$`),
				CompletionType: fuzzyCompleteSchema,
			},
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `List tables`,
				Syntax: `SHOW TABLES [<schema>]`,
				Note:   `If schema is not provided, the default schema is used`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+TABLES(?:\s+(?P<schema>.+))?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowTablesStatement{Schema: unquoteIdentifier(groups["schema"])}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+TABLES\s+(\S*)$`),
			CompletionType: fuzzyCompleteSchema,
		}},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show columns`,
				Syntax: `SHOW COLUMNS FROM <table_fqn>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^(?:SHOW\s+COLUMNS\s+FROM)\s+(?P<table>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(groups["table"]))
			return &ShowColumnsStatement{Schema: schema, Table: table}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+COLUMNS\s+FROM\s+(\S*)$`),
			CompletionType: fuzzyCompleteTable,
		}},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show indexes`,
				Syntax: `SHOW INDEX FROM <table_fqn>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(?P<table>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(groups["table"]))
			return &ShowIndexStatement{Schema: schema, Table: table}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(\S*)$`),
			CompletionType: fuzzyCompleteTable,
		}},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `SHOW DDLs`,
				Syntax: `SHOW DDLS`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+DDLS$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowDdlsStatement{}, nil
		},
	},
	// DUMP statements for database export
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Export database DDL and data as SQL statements`,
				Syntax: `DUMP DATABASE`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DUMP\s+DATABASE$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &DumpDatabaseStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Export database DDL only as SQL statements`,
				Syntax: `DUMP SCHEMA`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DUMP\s+SCHEMA$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &DumpSchemaStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Export specific tables as SQL INSERT statements`,
				Syntax: `DUMP TABLES <table1> [, <table2>, ...]`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DUMP\s+TABLES\s+(?P<tables>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			tables := splitTableNames(groups["tables"])
			return &DumpTablesStatement{Tables: tables}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*DUMP\s+TABLES\s+(?:.*,\s*)?(\S*)$`),
			CompletionType: fuzzyCompleteTable,
		}},
	},
	// Operations
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show schema update operations`,
				Syntax: `SHOW SCHEMA UPDATE OPERATIONS`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+SCHEMA\s+UPDATE\s+OPERATIONS$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowSchemaUpdateOperations{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show specific operation (async)`,
				Syntax: `SHOW OPERATION <operation-id-or-name> [ASYNC|SYNC]`,
				Note:   `Attach to and monitor a specific Long Running Operation by its operation ID or full operation name. ASYNC (default) returns current status, SYNC provides real-time monitoring (planned).`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+OPERATION\s+(?P<operation>.+?)(?:\s+(?P<mode>SYNC|ASYNC))?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			operationId := unquoteString(groups["operation"])
			mode := strings.ToUpper(groups["mode"])
			if mode == "" {
				mode = "ASYNC" // Default to ASYNC mode
			}
			return &ShowOperationStatement{OperationId: operationId, Mode: mode}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+OPERATION\s+(\S*)$`),
			CompletionType: fuzzyCompleteOperation,
		}},
	},
	// Split Points
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  "Add split points",
				Syntax: "ADD SPLIT POINTS [EXPIRED AT <timestamp>] <type> <fqn> (<key>, ...) [TableKey (<key>, ...)] ...",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^ADD\s+SPLIT\s+POINTS\s+(?P<body>.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			points, err := parseAddSplitPointsBody(groups["body"])
			if err != nil {
				return nil, err
			}

			return &AddSplitPointsStatement{
				SplitPoints: points,
			}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  "Drop split points",
				Syntax: "DROP SPLIT POINTS <type> <fqn> (<key>, ...) [TableKey (<key>, ...)] ...",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DROP\s+SPLIT\s+POINTS\s+(?P<body>.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			points, err := parseDropSplitPointsBody(groups["body"])
			if err != nil {
				return nil, err
			}

			return &AddSplitPointsStatement{
				SplitPoints: points,
			}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  "Show split points",
				Syntax: "SHOW SPLIT POINTS",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+SPLIT\s+POINTS$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowSplitPointsStatement{}, nil
		},
	},
	// Protocol Buffers
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show local proto descriptors`,
				Syntax: `SHOW LOCAL PROTO`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+LOCAL\s+PROTO$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowLocalProtoStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show remote proto bundle`,
				Syntax: `SHOW REMOTE PROTO`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+REMOTE\s+PROTO$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowRemoteProtoStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Manipulate PROTO BUNDLE`,
				Syntax: `SYNC PROTO BUNDLE [{UPSERT|DELETE} (<type> ...)]`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SYNC\s+PROTO\s+BUNDLE(?:\s+(?P<args>.*))?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return parseSyncProtoBundle(groups["args"])
		},
	},
	// TRUNCATE TABLE
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Truncate table`,
				Syntax: `TRUNCATE TABLE <table_fqn>`,
				Note:   `Only rows are deleted. Note: Non-atomically because executed as a [partitioned DML statement](https://cloud.google.com/spanner/docs/dml-partitioned?hl=en).`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^TRUNCATE\s+TABLE\s+(?P<table>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(groups["table"]))
			return &TruncateTableStatement{Schema: schema, Table: table}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*TRUNCATE\s+TABLE\s+(\S*)$`),
			CompletionType: fuzzyCompleteTable,
		}},
	},
	// EXPLAIN & EXPLAIN ANALYZE
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show execution plan without execution`,
				Syntax: `EXPLAIN [FORMAT=<format>] [WIDTH=<width>] <sql>`,
				Note:   "Options can be in any order. Spaces are not allowed before or after the `=`.",
			},
			{
				Usage:  `Execute query and show execution plan with profile`,
				Syntax: `EXPLAIN ANALYZE [FORMAT=<format>] [WIDTH=<width>] <sql>`,
				Note:   "Options can be in any order. Spaces are not allowed before or after the `=`.",
			},
			{
				Usage:  `Show EXPLAIN [ANALYZE] of the last query without execution`,
				Syntax: `EXPLAIN [ANALYZE] [FORMAT=<format>] [WIDTH=<width>] LAST QUERY`,
				Note:   "Options can be in any order. Spaces are not allowed before or after the `=`.",
			},
		},
		// EXPLAIN statement pattern:
		// - (?is): case-insensitive, dot matches newline
		// - ^EXPLAIN\s+: start with EXPLAIN keyword
		// - (?P<analyze>ANALYZE\s+)?: optional ANALYZE keyword
		// - (?P<options>(?:(?:FORMAT|WIDTH|LAST|QUERY)(?:|=\S+)(?:\s+|$))*)): options with format/width/last/query
		// - (?P<query>.*|): optional query text or empty string
		// - $: end of string
		Pattern: regexp.MustCompile(`(?is)^EXPLAIN\s+(?P<analyze>ANALYZE\s+)?(?P<options>(?:(?:FORMAT|WIDTH|LAST|QUERY)(?:|=\S+)(?:\s+|$))*)(?P<query>.*|)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			isAnalyze := groups["analyze"] != ""
			options, err := parseExplainOptions(groups["options"])
			if err != nil {
				return nil, fmt.Errorf("invalid EXPLAIN%s: %w", lo.Ternary(isAnalyze, " ANALYZE", ""), err)
			}

			formatStr := lo.FromPtr(options["FORMAT"])
			var format enums.ExplainFormat
			if formatStr != "" {
				format, err = enums.ExplainFormatString(formatStr)
				if err != nil {
					return nil, fmt.Errorf("invalid EXPLAIN%s: %w", lo.Ternary(isAnalyze, " ANALYZE", ""), err)
				}
			}

			var width int64
			if widthStr := lo.FromPtr(options["WIDTH"]); widthStr != "" {
				width, err = strconv.ParseInt(widthStr, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid WIDTH option value: %q, expected a positive integer. Error: %w", widthStr, err)
				}
				if width <= 0 {
					return nil, fmt.Errorf("invalid WIDTH option value: %d, expected a positive integer", width)
				}
			}

			// expectLabel enforces <name> is not appeared as <name>=<value> form.
			expectLabel := func(options map[string]*string, name string) (bool, error) {
				v, ok := options[name]
				if v != nil {
					return false, fmt.Errorf(`invalid option %s=%s, %s must be specified without a value (e.g., EXPLAIN LAST QUERY)`, name, *v, name)
				}
				return ok, nil
			}

			hasLastOption, err := expectLabel(options, "LAST")
			if err != nil {
				return nil, err
			}

			hasQueryOption, err := expectLabel(options, "QUERY")
			if err != nil {
				return nil, err
			}

			query := groups["query"]
			if hasLastOption && hasQueryOption {
				if strings.TrimSpace(query) != "" {
					return nil, fmt.Errorf(`invalid string after LAST QUERY: %q. Correct syntax: EXPLAIN [ANALYZE] [options] LAST QUERY`, query)
				}

				return &ExplainLastQueryStatement{Analyze: isAnalyze, Format: format, Width: width}, nil
			}

			if strings.TrimSpace(query) == "" && (!hasLastOption || !hasQueryOption) {
				return nil, fmt.Errorf("missing SQL query or 'LAST QUERY' for EXPLAIN%s statement", lo.Ternary(isAnalyze, " ANALYZE", ""))
			}

			isDML := stmtkind.IsDMLLexical(query)
			switch {
			case isAnalyze && isDML:
				return &ExplainAnalyzeDmlStatement{Dml: query, Format: format, Width: width}, nil
			case isAnalyze:
				return &ExplainAnalyzeStatement{Query: query, Format: format, Width: width}, nil
			default:
				return &ExplainStatement{Explain: query, IsDML: isDML, Format: format, Width: width}, nil
			}
		},
	},
	// SHOW PLAN NODE
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show the specific raw plan node from the last cached query plan`,
				Syntax: `SHOW PLAN NODE <node_id>`,
				Note:   `Requires a preceding query or EXPLAIN ANALYZE.`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+PLAN\s+NODE\s+(?P<node_id>\d+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			nodeIDStr := groups["node_id"]
			nodeID, err := strconv.ParseInt(nodeIDStr, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid node ID: %q. Node ID must be an integer", nodeIDStr)
			}
			return &ShowPlanNodeStatement{NodeID: int(nodeID)}, nil
		},
	},
	// DESCRIBE
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show result shape without execution`,
				Syntax: `DESCRIBE <sql>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DESCRIBE\s+(?P<sql>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			isDML := stmtkind.IsDMLLexical(groups["sql"])
			switch {
			case isDML:
				return &DescribeStatement{Statement: groups["sql"], IsDML: true}, nil
			default:
				return &DescribeStatement{Statement: groups["sql"]}, nil
			}
		},
	},

	// Partitioned DML
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Partitioned DML`,
				Syntax: `PARTITIONED {UPDATE|DELETE} ...`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^PARTITIONED\s+(?P<dml>.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &PartitionedDmlStatement{Dml: groups["dml"]}, nil
		},
	},

	// Partitioned Query
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show partition tokens of partition query`,
				Syntax: `PARTITION <sql>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^PARTITION\s(?P<sql>\S.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &PartitionStatement{SQL: groups["sql"]}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Run partitioned query`,
				Syntax: `RUN PARTITIONED QUERY <sql>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^RUN\s+PARTITIONED\s+QUERY\s(?P<sql>\S.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &RunPartitionedQueryStatement{SQL: groups["sql"]}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			// It is commented out because it is not implemented yet.
			/*
				{
					Usage:  `Run a specific partition`,
					Syntax: `RUN PARTITION <token>`,
					Note:   `This statement is currently unimplemented.`,
				},
			*/
		},
		Pattern: regexp.MustCompile(`(?is)^RUN\s+PARTITION\s+(?P<token>'[^']*'|"[^"]*")$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &RunPartitionStatement{Token: unquoteString(groups["token"])}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Test root-partitionable`,
				Syntax: `TRY PARTITIONED QUERY <sql>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^TRY\s+PARTITIONED\s+QUERY\s(?P<sql>\S.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &TryPartitionedQueryStatement{SQL: groups["sql"]}, nil
		},
	},
	// Transaction
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Start R/W transaction`,
				Syntax: `BEGIN RW [TRANSACTION] [ISOLATION LEVEL {SERIALIZABLE|REPEATABLE READ}] [PRIORITY {HIGH|MEDIUM|LOW}]`,
				Note:   `(spanner-cli style);  See [Request Priority](#request-priority) for details on the priority.`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^BEGIN\s+RW(?:\s+TRANSACTION)?(?:\s+ISOLATION\s+LEVEL\s+(?P<isolation>SERIALIZABLE|REPEATABLE\s+READ))?(?:\s+PRIORITY\s+(?P<priority>HIGH|MEDIUM|LOW))?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			isolationLevel, err := parseIsolationLevel(groups["isolation"])
			if err != nil {
				return nil, err
			}

			priority, err := parsePriority(groups["priority"])
			if err != nil {
				return nil, err
			}

			return &BeginRwStatement{IsolationLevel: isolationLevel, Priority: priority}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Start R/O transaction`,
				Syntax: `BEGIN RO [TRANSACTION] [{<seconds>|<rfc3339_timestamp>}] [PRIORITY {HIGH|MEDIUM|LOW}]`,
				Note:   "`<seconds>` and `<rfc3339_timestamp>` is used for stale read. `<rfc3339_timestamp>` must be quoted. See [Request Priority](#request-priority) for details on the priority.",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^BEGIN\s+RO(?:\s+TRANSACTION)?(?:\s+(?P<timestamp>[^\s]+))?(?:\s+PRIORITY\s+(?P<priority>HIGH|MEDIUM|LOW))?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			stmt := &BeginRoStatement{
				TimestampBoundType: timestampBoundUnspecified,
			}

			if groups["timestamp"] != "" {
				if t, err := time.Parse(time.RFC3339Nano, unquoteString(groups["timestamp"])); err == nil {
					stmt = &BeginRoStatement{
						TimestampBoundType: readTimestamp,
						Timestamp:          t,
					}
				}
				if i, err := strconv.Atoi(groups["timestamp"]); err == nil {
					stmt = &BeginRoStatement{
						TimestampBoundType: exactStaleness,
						Staleness:          time.Duration(i) * time.Second,
					}
				}
			}

			priority, err := parsePriority(groups["priority"])
			if err != nil {
				return nil, err
			}
			stmt.Priority = priority

			return stmt, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Start transaction`,
				Syntax: `BEGIN [TRANSACTION] [ISOLATION LEVEL {SERIALIZABLE|REPEATABLE READ}] [PRIORITY {HIGH|MEDIUM|LOW}]`,
				Note:   "(Spanner JDBC driver style); It respects `READONLY` system variable. See [Request Priority](#request-priority) for details on the priority.",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^BEGIN(?:\s+TRANSACTION)?(?:\s+ISOLATION\s+LEVEL\s+(?P<isolation>SERIALIZABLE|REPEATABLE\s+READ))?(?:\s+PRIORITY\s+(?P<priority>HIGH|MEDIUM|LOW))?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			isolationLevel, err := parseIsolationLevel(groups["isolation"])
			if err != nil {
				return nil, err
			}

			priority, err := parsePriority(groups["priority"])
			if err != nil {
				return nil, err
			}

			return &BeginStatement{IsolationLevel: isolationLevel, Priority: priority}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Commit R/W transaction or end R/O Transaction`,
				Syntax: `COMMIT [TRANSACTION]`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^COMMIT(?:\s+TRANSACTION)?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &CommitStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  "Rollback R/W transaction or end R/O transaction",
				Syntax: `ROLLBACK [TRANSACTION]`,
				Note:   "`CLOSE` can be used as a synonym of `ROLLBACK`.",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^(?:ROLLBACK|CLOSE)(?:\s+TRANSACTION)?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &RollbackStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Set transaction mode`,
				Syntax: `SET TRANSACTION {READ ONLY|READ WRITE}`,
				Note:   `(Spanner JDBC driver style); Set transaction mode for the current transaction.`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+TRANSACTION\s+(?P<mode>.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			isReadOnly, err := parseTransaction(groups["mode"])
			if err != nil {
				return nil, err
			}
			return &SetTransactionStatement{IsReadOnly: isReadOnly}, nil
		},
	},
	// Batching
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Start DDL batching`,
				Syntax: `START BATCH DDL`,
			},
			{
				Usage:  `Start DML batching`,
				Syntax: `START BATCH DML`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^START\s+BATCH\s+(?P<mode>DDL|DML)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &StartBatchStatement{Mode: lo.Ternary(strings.ToUpper(groups["mode"]) == "DDL", batchModeDDL, batchModeDML)}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Run active batch`,
				Syntax: `RUN BATCH`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^RUN\s+BATCH$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &RunBatchStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Abort active batch`,
				Syntax: `ABORT BATCH [TRANSACTION]`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^ABORT\s+BATCH(?:\s+TRANSACTION)?$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &AbortBatchStatement{}, nil
		},
	},
	// System Variable
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Set variable`,
				Syntax: `SET <name> = <value>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+(?P<name>[^\s=]+)\s*=\s*(?P<value>\S.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &SetStatement{VarName: groups["name"], Value: groups["value"]}, nil
		},
		Completion: []fuzzyArgCompletion{
			{
				// Value completion: SET <name> = <partial_value>
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SET\s+(\S+)\s*=\s*(\S*)$`),
				CompletionType: fuzzyCompleteVariableValue,
			},
			{
				// Name completion: SET <partial_name>
				PrefixPattern:  regexp.MustCompile(`(?i)^\s*SET\s+([^\s=]*)$`),
				CompletionType: fuzzyCompleteVariable,
				Suffix:         " = ",
			},
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Add value to variable`,
				Syntax: `SET <name> += <value>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+(?P<name>[^\s+=]+)\s*\+=\s*(?P<value>\S.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &SetAddStatement{VarName: groups["name"], Value: groups["value"]}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show variables`,
				Syntax: `SHOW VARIABLES`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+VARIABLES$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowVariablesStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show variable`,
				Syntax: `SHOW VARIABLE <name>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+VARIABLE\s+(?P<name>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowVariableStatement{VarName: groups["name"]}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*SHOW\s+VARIABLE\s+(\S*)$`),
			CompletionType: fuzzyCompleteVariable,
		}},
	},
	// Query Parameter
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Set type query parameter`,
				Syntax: `SET PARAM <name> <type>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+PARAM\s+(?P<name>[^\s=]+)\s*(?P<type>[^=]*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &SetParamTypeStatement{Name: groups["name"], Type: groups["type"]}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*SET\s+PARAM\s+([^\s=]*)$`),
			CompletionType: fuzzyCompleteParam,
			Suffix:         " ",
		}},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Set value query parameter`,
				Syntax: `SET PARAM <name> = <value>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+PARAM\s+(?P<name>[^\s=]+)\s*=\s*(?P<value>.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &SetParamValueStatement{Name: groups["name"], Value: groups["value"]}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show query parameters`,
				Syntax: `SHOW PARAMS`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+PARAMS$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowParamsStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Unset query parameter`,
				Syntax: `UNSET PARAM <name>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^UNSET\s+PARAM\s+(?P<name>\S+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &UnsetParamStatement{Name: groups["name"]}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*UNSET\s+PARAM\s+(\S*)$`),
			CompletionType: fuzzyCompleteParam,
		}},
	},
	// Mutation
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Perform write mutations`,
				Syntax: `MUTATE <table_fqn> {INSERT|UPDATE|REPLACE|INSERT_OR_UPDATE} ...`,
			},
			{
				Usage:  `Perform delete mutations`,
				Syntax: `MUTATE <table_fqn> DELETE ...`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^MUTATE\s+(?P<table>\S+)\s+(?P<operation>INSERT|UPDATE|INSERT_OR_UPDATE|REPLACE|DELETE)\s+(?P<body>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &MutateStatement{Table: unquoteIdentifier(groups["table"]), Operation: groups["operation"], Body: groups["body"]}, nil
		},
		Completion: []fuzzyArgCompletion{{
			PrefixPattern:  regexp.MustCompile(`(?i)^\s*MUTATE\s+(\S*)$`),
			CompletionType: fuzzyCompleteTable,
		}},
	},
	// Query Profiles
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show sampled query plans`,
				Syntax: `SHOW QUERY PROFILES`,
				Note:   `EARLY EXPERIMENTAL`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+QUERY\s+PROFILES$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ShowQueryProfilesStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show the single sampled query plan`,
				Syntax: `SHOW QUERY PROFILE <fingerprint>`,
				Note:   `EARLY EXPERIMENTAL`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+QUERY\s+PROFILE\s+(?P<fingerprint>.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			fprint, err := strconv.ParseInt(strings.TrimSpace(groups["fingerprint"]), 10, 64)
			if err != nil {
				return nil, err
			}
			return &ShowQueryProfileStatement{Fprint: fprint}, nil
		},
	},
	// LLM
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Compose query using LLM`,
				Syntax: `GEMINI "<prompt>"`,
			},
		},

		Pattern: regexp.MustCompile(`(?is)^GEMINI\s+(?P<text>.*)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &GeminiStatement{Text: unquoteString(groups["text"])}, nil
		},
	},
	// Cassandra interface
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Execute CQL`,
				Syntax: `CQL ...`,
				Note:   "EARLY EXPERIMENTAL",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^CQL\s+(?P<cql>.+)$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &CQLStatement{CQL: groups["cql"]}, nil
		},
	},
	// CLI control
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show help`,
				Syntax: `HELP`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^HELP$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &HelpStatement{}, nil
		},
	},
	{
		// HELP VARIABLES is a System Variable statement, but placed here because of ordering in HELP
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show help for variables`,
				Syntax: `HELP VARIABLES`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^HELP\s+VARIABLES$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &HelpVariablesStatement{}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Exit CLI`,
				Syntax: `EXIT`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^EXIT$`),
		HandleGroups: func(groups map[string]string) (Statement, error) {
			return &ExitStatement{}, nil
		},
	},
}

// Helper functions for HandleGroups implementations

func parseTransaction(s string) (isReadOnly bool, err error) {
	if !transactionRe.MatchString(s) {
		return false, fmt.Errorf(`must be "READ ONLY" or "READ WRITE", but: %q`, s)
	}

	submatch := transactionRe.FindStringSubmatch(s)
	return submatch[1] != "", nil
}

func parseSyncProtoBundle(s string) (Statement, error) {
	p := &memefish.Parser{Lexer: &memefish.Lexer{
		File: &token.File{
			Buffer: s,
		},
	}}
	err := p.NextToken()
	if err != nil {
		return nil, err
	}

	var upsertPaths, deletePaths []string
loop:
	for {
		switch {
		case p.Token.Kind == token.TokenEOF:
			break loop
		case p.Token.IsKeywordLike("UPSERT"):
			paths, err := parsePaths(p)
			if err != nil {
				return nil, fmt.Errorf("failed to parsePaths: %w", err)
			}
			upsertPaths = append(upsertPaths, paths...)
		case p.Token.IsKeywordLike("DELETE"):
			paths, err := parsePaths(p)
			if err != nil {
				return nil, err
			}
			deletePaths = append(deletePaths, paths...)
		default:
			return nil, fmt.Errorf("expected UPSERT or DELETE, but: %q", p.Token.AsString)
		}
	}
	return &SyncProtoStatement{UpsertPaths: upsertPaths, DeletePaths: deletePaths}, nil
}

func parsePaths(p *memefish.Parser) ([]string, error) {
	expr, err := p.ParseExpr()
	if err != nil {
		return nil, err
	}

	switch e := expr.(type) {
	case *ast.ParenExpr:
		name, err := exprToFullName(e.Expr)
		if err != nil {
			return nil, err
		}
		return sliceOf(name), nil
	case *ast.TupleStructLiteral:
		return lo.MapErr(e.Values, func(expr ast.Expr, _ int) (string, error) {
			return exprToFullName(expr)
		})
	default:
		return nil, fmt.Errorf("must be paren expr or tuple of path, but: %T", expr)
	}
}

func exprToFullName(expr ast.Expr) (string, error) {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name, nil
	case *ast.Path:
		return strings.Join(slices.Collect(loi.Map(slices.Values(e.Idents), func(ident *ast.Ident) string { return ident.Name })), "."), nil
	default:
		return "", fmt.Errorf("must be ident or path, but: %T", expr)
	}
}

func parseIsolationLevel(isolationLevel string) (sppb.TransactionOptions_IsolationLevel, error) {
	if isolationLevel == "" {
		return sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, nil
	}

	value := strings.Join(strings.Fields(strings.ToUpper(isolationLevel)), "_")

	p, ok := sppb.TransactionOptions_IsolationLevel_value[value]
	if !ok {
		return sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, fmt.Errorf("invalid isolation level: %q", value)
	}
	return sppb.TransactionOptions_IsolationLevel(p), nil
}

func parseExplainOptions(ss string) (map[string]*string, error) {
	m := make(map[string]*string)
	for s := range strings.FieldsSeq(ss) {
		before, after, found := strings.Cut(s, "=")
		if before == "" {
			return nil, fmt.Errorf("invalid EXPLAIN option, expect <key>[=<value>], but: %s", s)
		}
		m[strings.ToUpper(before)] = lo.Ternary(found, lo.ToPtr(after), nil)
	}
	return m, nil
}
