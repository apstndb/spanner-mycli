package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/gsqlutils/stmtkind"
	"github.com/samber/lo"
)

var clientStatementHandlers = []*clientStatementHandler{
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Manipulate PROTO BUNDLE`,
				Syntax: `SYNC PROTO BUNDLE [{UPSERT|DELETE} (<type> ...)]`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SYNC\s+PROTO\s+BUNDLE(?:\s+(?P<args>.*))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return parseSyncProtoBundle(matched[1])
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Exit CLI`,
				Syntax: `EXIT`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^EXIT$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ExitStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
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
		// DROP DATABASE is not native Cloud Spanner statement
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Drop database`,
				Syntax: `DROP DATABASE <database>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^DROP\s+DATABASE\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &DropDatabaseStatement{DatabaseId: unquoteIdentifier(matched[1])}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Truncate table`,
				Syntax: `TRUNCATE TABLE <table>`,
				Note:   `Only rows are deleted. Note: Non-atomically because executed as a [partitioned DML statement](https://cloud.google.com/spanner/docs/dml-partitioned?hl=en).`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^TRUNCATE\s+TABLE\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &TruncateTableStatement{Table: unquoteIdentifier(matched[1])}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show local proto descriptors`,
				Syntax: `SHOW LOCAL PROTO`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+LOCAL\s+PROTO$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowLocalProtoStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show remote proto bundle`,
				Syntax: `SHOW REMOTE PROTO`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+REMOTE\s+PROTO$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowRemoteProtoStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `List databases`,
				Syntax: `SHOW DATABASES`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+DATABASES$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowDatabasesStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show DDL of the schema object`,
				Syntax: `SHOW CREATE <type> <fqn>`,
				Note:   ``,
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
		Descriptions: []clientStatementDescription{
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
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show result shape`,
				Syntax: `DESCRIBE <sql>`,
				Note:   ``,
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
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show query execution plan with stats`,
				Syntax: `EXPLAIN ANALYZE <sql>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^EXPLAIN\s+(ANALYZE\s+)?(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			isAnalyze := matched[1] != ""
			isDML := stmtkind.IsDMLLexical(matched[2])
			switch {
			case isAnalyze && isDML:
				return &ExplainAnalyzeDmlStatement{Dml: matched[2]}, nil
			case isAnalyze:
				return &ExplainAnalyzeStatement{Query: matched[2]}, nil
			default:
				return &ExplainStatement{Explain: matched[2], IsDML: isDML}, nil
			}
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show columns`,
				Syntax: `SHOW COLUMNS FROM <table_fqn>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^(?:SHOW\s+COLUMNS\s+FROM)\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(matched[1]))
			return &ShowColumnsStatement{Schema: schema, Table: table}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show indexes`,
				Syntax: `SHOW INDEX FROM <table_fqn>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			schema, table := extractSchemaAndName(unquoteIdentifier(matched[1]))
			return &ShowIndexStatement{Schema: schema, Table: table}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Partitioned DML`,
				Syntax: `PARTITIONED {UPDATE|DELETE} ...`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^PARTITIONED\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &PartitionedDmlStatement{Dml: matched[1]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Start R/W transaction`,
				Syntax: `BEGIN RW [TRANSACTION] [PRIORITY {HIGH|MEDIUM|LOW}]`,
				Note:   `(spanner-cli style);  See [Request Priority](#request-priority) for details on the priority.`,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^BEGIN\s+RW(?:\s+TRANSACTION)?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			priority, err := parsePriority(matched[1])
			if err != nil {
				return nil, err
			}

			return &BeginRwStatement{Priority: priority}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Start R/O transaction`,
				Syntax: `BEGIN RO [TRANSACTION] [{<seconds>|<RFC3339-formatted time>}] [PRIORITY {HIGH|MEDIUM|LOW}]`,
				Note:   "`<seconds>` and `<RFC3339-formatted time>` is used for stale read. See [Request Priority](#request-priority) for details on the priority.",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^BEGIN\s+RO(?:\s+TRANSACTION)?(?:\s+([^\s]+))?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			stmt := &BeginRoStatement{
				TimestampBoundType: timestampBoundUnspecified,
			}

			if matched[1] != "" {
				if t, err := time.Parse(time.RFC3339Nano, matched[1]); err == nil {
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
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Start transaction`,
				Syntax: `BEGIN [TRANSACTION] [PRIORITY {HIGH|MEDIUM|LOW}]`,
				Note:   "(Spanner JDBC driver style); It respects `READONLY` system variable. See [Request Priority](#request-priority) for details on the priority.",
			},
		},
		Pattern: regexp.MustCompile(`(?is)^BEGIN(?:\s+TRANSACTION)?(?:\s+PRIORITY\s+(HIGH|MEDIUM|LOW))?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			priority, err := parsePriority(matched[1])
			if err != nil {
				return nil, err
			}

			return &BeginStatement{Priority: priority}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Commit R/W transaction or end R/O Transaction`,
				Syntax: `COMMIT [TRANSACTION]`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^COMMIT(?:\s+TRANSACTION)?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &CommitStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
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
		Descriptions: []clientStatementDescription{
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
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show variable`,
				Syntax: `SHOW VARIABLE <name>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+VARIABLE\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowVariableStatement{VarName: matched[1]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Set type query parameter`,
				Syntax: `SET PARAM <name> <type>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*([^=]*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetParamTypeStatement{Name: matched[1], Type: matched[2]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Set value query parameter`,
				Syntax: `SET PARAM <name> = <value>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+PARAM\s+([^\s=]+)\s*=\s*(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetParamValueStatement{Name: matched[1], Value: matched[2]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Set variable`,
				Syntax: `SET <name> = <value>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+([^\s=]+)\s*=\s*(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetStatement{VarName: matched[1], Value: matched[2]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Add value to variable`,
				Syntax: `SET <name> += <value>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SET\s+([^\s+=]+)\s*\+=\s*(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &SetAddStatement{VarName: matched[1], Value: matched[2]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show query parameters`,
				Syntax: `SHOW PARAMS`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+PARAMS$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowParamsStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show variables`,
				Syntax: `SHOW VARIABLES`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+VARIABLES$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowVariablesStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show partition tokens of partition query`,
				Syntax: `PARTITION <sql>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^PARTITION\s(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &PartitionStatement{SQL: matched[1]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Run partitioned query`,
				Syntax: `RUN PARTITIONED QUERY <sql>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^RUN\s+PARTITIONED\s+QUERY\s(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &RunPartitionedQueryStatement{SQL: matched[1]}, nil
		},
	},
	{
		// unimplemented
		Descriptions: []clientStatementDescription{},
		Pattern:      regexp.MustCompile(`(?is)^RUN\s+PARTITION\s+('[^']*'|"[^"]*")$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &RunPartitionStatement{Token: unquoteString(matched[1])}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Test root-partitionable`,
				Syntax: `TRY PARTITIONED QUERY <sql>`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^TRY\s+PARTITIONED\s+QUERY\s(\S.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &TryPartitionedQueryStatement{SQL: matched[1]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Perform write mutations`,
				Syntax: `MUTATE <table_fqn> {INSERT|UPDATE|REPLACE|INSERT_OR_UPDATE} ...`,
				Note:   ``,
			},
			{
				Usage:  `Perform delete mutations`,
				Syntax: `MUTATE <table_fqn> DELETE ...`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^MUTATE\s+(\S+)\s+(INSERT|UPDATE|INSERT_OR_UPDATE|REPLACE|DELETE)\s+(.+)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &MutateStatement{Table: unquoteIdentifier(matched[1]), Operation: matched[2], Body: matched[3]}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
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
		Descriptions: []clientStatementDescription{
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
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `SHOW DDLs`,
				Syntax: `SHOW DDLS`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^SHOW\s+DDLS$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &ShowDdlsStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Compose query using LLM`,
				Syntax: `GEMINI "<prompt>"`,
				Note:   ``,
			},
		},

		Pattern: regexp.MustCompile(`(?is)^GEMINI\s+(.*)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &GeminiStatement{Text: unquoteString(matched[1])}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Start DDL batching`,
				Syntax: `START BATCH DDL`,
				Note:   ``,
			},
			{
				Usage:  `Start DML batching`,
				Syntax: `START BATCH DML`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^START\s+BATCH\s+(DDL|DML)$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &StartBatchStatement{Mode: lo.Ternary(strings.ToUpper(matched[1]) == "DDL", batchModeDDL, batchModeDML)}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Run active batch`,
				Syntax: `RUN BATCH`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^RUN\s+BATCH$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &RunBatchStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Abort active batch`,
				Syntax: `ABORT BATCH [TRANSACTION]`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^ABORT\s+BATCH(?:\s+TRANSACTION)?$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &AbortBatchStatement{}, nil
		},
	},
	{
		Descriptions: []clientStatementDescription{
			{
				Usage:  `Show help`,
				Syntax: `HELP`,
				Note:   ``,
			},
		},
		Pattern: regexp.MustCompile(`(?is)^HELP$`),
		HandleSubmatch: func(matched []string) (Statement, error) {
			return &HelpStatement{}, nil
		},
	},
}
