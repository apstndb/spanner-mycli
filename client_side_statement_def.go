package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/samber/lo"
	scxiter "spheric.cloud/xiter"
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

type clientSideStatementDef struct {
	// Descriptions represents human-readable descriptions.
	// It can be multiple because some clientSideStatementDef represents multiple statements in single pattern.
	Descriptions []clientSideStatementDescription

	// Pattern is a compiled regular expression for the statement.
	// It must be matched on the whole statement without semicolon, and case-insensitive.
	Pattern *regexp.Regexp

	// HandleSubmatch holds a handler which converts the result of (*regexp.Regexp).FindStringSubmatch() to Statement.
	HandleSubmatch func(matched []string) (Statement, error)
}

var schemaObjectsReStr = stringsiter.Join("|", xiter.Map(func(s string) string {
	return strings.ReplaceAll(s, " ", `\s+`)
}, slices.Values([]string{
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
})))

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
		Pattern: regexp.MustCompile(`(?is)^USE\s+([^\s]+)(?:\s+ROLE\s+(.+))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &UseStatement{Database: unquoteIdentifier(matched[1]), Role: unquoteIdentifier(matched[2])}, nil
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		Pattern: regexp.MustCompile(`(?is)^DROP\s+DATABASE\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &DropDatabaseStatement{DatabaseId: unquoteIdentifier(matched[1])}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `List databases`,
				Syntax: `SHOW DATABASES`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+DATABASES$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		Pattern: regexp.MustCompile(fmt.Sprintf(`(?is)^SHOW\s+CREATE\s+(%s)\s+(.+)$`, schemaObjectsReStr)),
		HandleSubmatch: func(matched []string) (Statement, error) {
			objectType := strings.ToUpper(regexp.MustCompile(`\s+`).ReplaceAllString(matched[1], " "))
			schema, name := extractSchemaAndName(unquoteIdentifier(matched[2]))
			return &ShowCreateStatement{ObjectType: objectType, Schema: schema, Name: name}, nil
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
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+TABLES(?:\s+(.+))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowTablesStatement{Schema: unquoteIdentifier(matched[1])}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show columns`,
				Syntax: `SHOW COLUMNS FROM <table_fqn>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^(?:SHOW\s+COLUMNS\s+FROM)\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(matched[1]))
			return &ShowColumnsStatement{Schema: schema, Table: table}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Show indexes`,
				Syntax: `SHOW INDEX FROM <table_fqn>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(matched[1]))
			return &ShowIndexStatement{Schema: schema, Table: table}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `SHOW DDLs`,
				Syntax: `SHOW DDLS`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+DDLS$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowDdlsStatement{}, nil
		},
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+OPERATION\s+(.+?)(?:\s+(SYNC|ASYNC))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			operationId := unquoteString(matched[1])
			mode := strings.ToUpper(matched[2])
			if mode == "" {
				mode = "ASYNC" // Default to ASYNC mode
			}
			return &ShowOperationStatement{OperationId: operationId, Mode: mode}, nil
		},
	},
	// Split Points
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  "Add split points",
				Syntax: "ADD SPLIT POINTS [EXPIRED AT <timestamp>] <type> <fqn> (<key>, ...) [TableKey (<key>, ...)] ...",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^ADD\s+SPLIT\s+POINTS\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			points, err := parseAddSplitPointsBody(matched[1])
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
		Pattern: regexp.MustCompile(`(?is)^DROP\s+SPLIT\s+POINTS\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			points, err := parseDropSplitPointsBody(matched[1])
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		HandleSubmatch: func(matched []string) (Statement, error) {
			return parseSyncProtoBundle(matched[1])
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
		Pattern: regexp.MustCompile(`(?is)^TRUNCATE\s+TABLE\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(matched[1]))
			return &TruncateTableStatement{Schema: schema, Table: table}, nil
		},
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
		HandleSubmatch: func(matched []string) (Statement, error) {
			isAnalyze := matched[1] != ""
			options, err := parseExplainOptions(matched[2])
			if err != nil {
				return nil, fmt.Errorf("invalid EXPLAIN%s: %w", lo.Ternary(isAnalyze, " ANALYZE", ""), err)
			}

			format, err := parseExplainFormat(lo.FromPtr(options["FORMAT"]))
			if err != nil {
				return nil, fmt.Errorf("invalid EXPLAIN%s: %w", lo.Ternary(isAnalyze, " ANALYZE", ""), err)
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

			query := matched[3]
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
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+PLAN\s+NODE\s+(\d+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			nodeIDStr := matched[1]
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
		Pattern: regexp.MustCompile(`(?is)^DESCRIBE\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			isDML := stmtkind.IsDMLLexical(matched[1])
			switch {
			case isDML:
				return &DescribeStatement{Statement: matched[1], IsDML: true}, nil
			default:
				return &DescribeStatement{Statement: matched[1]}, nil
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
		Pattern: regexp.MustCompile(`(?is)^PARTITIONED\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &PartitionedDmlStatement{Dml: matched[1]}, nil
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
		Pattern: regexp.MustCompile(`(?is)^PARTITION\s(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &PartitionStatement{SQL: matched[1]}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Run partitioned query`,
				Syntax: `RUN PARTITIONED QUERY <sql>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^RUN\s+PARTITIONED\s+QUERY\s(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &RunPartitionedQueryStatement{SQL: matched[1]}, nil
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
		Pattern: regexp.MustCompile(`(?is)^RUN\s+PARTITION\s+('[^']*'|"[^"]*")$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &RunPartitionStatement{Token: unquoteString(matched[1])}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Test root-partitionable`,
				Syntax: `TRY PARTITIONED QUERY <sql>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^TRY\s+PARTITIONED\s+QUERY\s(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &TryPartitionedQueryStatement{SQL: matched[1]}, nil
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
		Pattern: regexp.MustCompile(`(?is)^BEGIN\s+RW(?:\s+TRANSACTION)?(?:\s+ISOLATION\s+LEVEL\s+(SERIALIZABLE|REPEATABLE\s+READ))?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			isolationLevel, err := parseIsolationLevel(matched[1])
			if err != nil {
				return nil, err
			}

			priority, err := parsePriority(matched[2])
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
		Pattern: regexp.MustCompile(`(?is)^BEGIN\s+RO(?:\s+TRANSACTION)?(?:\s+([^\s]+))?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			stmt := &BeginRoStatement{
				TimestampBoundType: timestampBoundUnspecified,
			}

			if matched[1] != "" {
				if t, err := time.Parse(time.RFC3339Nano, unquoteString(matched[1])); err == nil {
					stmt = &BeginRoStatement{
						TimestampBoundType: readTimestamp,
						Timestamp:          t,
					}
				}
				if i, err := strconv.Atoi(matched[1]); err == nil {
					stmt = &BeginRoStatement{
						TimestampBoundType: exactStaleness,
						Staleness:          time.Duration(i) * time.Second,
					}
				}
			}

			priority, err := parsePriority(matched[2])
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
		Pattern: regexp.MustCompile(`(?is)^BEGIN(?:\s+TRANSACTION)?(?:\s+ISOLATION\s+LEVEL\s+(SERIALIZABLE|REPEATABLE\s+READ))?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			isolationLevel, err := parseIsolationLevel(matched[1])
			if err != nil {
				return nil, err
			}

			priority, err := parsePriority(matched[2])
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		Pattern: regexp.MustCompile(`(?is)^SET\s+TRANSACTION\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			isReadOnly, err := parseTransaction(matched[1])
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
		Pattern: regexp.MustCompile(`(?is)^START\s+BATCH\s+(DDL|DML)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &StartBatchStatement{Mode: lo.Ternary(strings.ToUpper(matched[1]) == "DDL", batchModeDDL, batchModeDML)}, nil
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		Pattern: regexp.MustCompile(`(?is)^SET\s+([^\s=]+)\s*=\s*(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetStatement{VarName: matched[1], Value: matched[2]}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Add value to variable`,
				Syntax: `SET <name> += <value>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+([^\s+=]+)\s*\+=\s*(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetAddStatement{VarName: matched[1], Value: matched[2]}, nil
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+VARIABLE\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowVariableStatement{VarName: matched[1]}, nil
		},
	},
	// Query Parameter
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Set type query parameter`,
				Syntax: `SET PARAM <name> <type>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*([^=]*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetParamTypeStatement{Name: matched[1], Type: matched[2]}, nil
		},
	},
	{
		Descriptions: []clientSideStatementDescription{
			{
				Usage:  `Set value query parameter`,
				Syntax: `SET PARAM <name> = <value>`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*=\s*(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetParamValueStatement{Name: matched[1], Value: matched[2]}, nil
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
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowParamsStatement{}, nil
		},
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
		Pattern: regexp.MustCompile(`(?is)^MUTATE\s+(\S+)\s+(INSERT|UPDATE|INSERT_OR_UPDATE|REPLACE|DELETE)\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &MutateStatement{Table: unquoteIdentifier(matched[1]), Operation: matched[2], Body: matched[3]}, nil
		},
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+QUERY\s+PROFILE\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			fprint, err := strconv.ParseInt(strings.TrimSpace(matched[1]), 10, 64)
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

		Pattern: regexp.MustCompile(`(?is)^GEMINI\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &GeminiStatement{Text: unquoteString(matched[1])}, nil
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
		Pattern: regexp.MustCompile(`(?is)^CQL\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &CQLStatement{CQL: matched[1]}, nil
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		HandleSubmatch: func(matched []string) (Statement, error) {
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
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ExitStatement{}, nil
		},
	},
}

type explainFormat string

const (
	explainFormatUnspecified explainFormat = ""
	explainFormatCurrent     explainFormat = "CURRENT"
	explainFormatTraditional explainFormat = "TRADITIONAL"
	explainFormatCompact     explainFormat = "COMPACT"
)

func parseExplainFormat(s string) (explainFormat, error) {
	switch strings.ToUpper(s) {
	case "COMPACT":
		return explainFormatCompact, nil
	case "CURRENT":
		return explainFormatCurrent, nil
	case "TRADITIONAL":
		return explainFormatTraditional, nil
	case "":
		return explainFormatUnspecified, nil
	default:
		return "", fmt.Errorf("parse error: unknown explain format: %s", s)
	}

}

// Helper functions for HandleSubmatch implementations

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
		names, err := scxiter.TryCollect(scxiter.MapErr(
			slices.Values(e.Values),
			exprToFullName))
		if err != nil {
			return nil, err
		}

		return names, err
	default:
		return nil, fmt.Errorf("must be paren expr or tuple of path, but: %T", expr)
	}
}

func exprToFullName(expr ast.Expr) (string, error) {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name, nil
	case *ast.Path:
		return scxiter.Join(xiter.Map(func(ident *ast.Ident) string { return ident.Name }, slices.Values(e.Idents)), "."), nil
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
