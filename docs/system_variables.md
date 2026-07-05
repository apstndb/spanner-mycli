# System variables

spanner-mycli behavior can be inspected and customized through system variables.
There are two families:

- **Spanner JDBC inspired variables** (no prefix, e.g. `READONLY`, `STATEMENT_TIMEOUT`):
  they have almost the same semantics as the corresponding
  [Spanner JDBC connection properties](https://cloud.google.com/spanner/docs/jdbc-session-mgmt-commands).
- **spanner-mycli original variables** (`CLI_` prefix, e.g. `CLI_FORMAT`).

They can be used with the following statements and flags:

```sql
SHOW VARIABLES;                 -- List all variables with their current values
SHOW VARIABLE CLI_FORMAT;       -- Show a single variable
SET CLI_FORMAT = 'VERTICAL';    -- Set a variable
SET LOCAL CLI_FORMAT = 'TAB';   -- Set a variable only for the current transaction
HELP VARIABLES;                 -- Show the reference table below interactively
```

Variables can also be set at startup with `--set NAME=VALUE` command-line flags.

## Reference

The table below lists all system variables. The `operations` column shows
whether a variable can be read (`SHOW`), written (`SET`), or appended to
(`SET ... += ...`).

<!-- The table between the markers below is generated from the variable
     registry by `make docs-update` (via the hidden --sysvars-help flag).
     Do not edit it by hand. -->
<!-- sysvars-help begin -->
| Name                              | Operations     | Description                                                                                                                                                                                                                                                                                                                                                                                                                         |
|:----------------------------------|:---------------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `AUTOCOMMIT`                      | read,write     | A boolean indicating whether or not the connection is in autocommit mode. The default is true.                                                                                                                                                                                                                                                                                                                                      |
| `AUTOCOMMIT_DML_MODE`             | read,write     | A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements.                                                                                                                                                                                                                                                                                                                                   |
| `AUTO_BATCH_DML`                  | read,write     | A property of type BOOL indicating whether the DML is executed immediately or begins a batch DML. The default is false.                                                                                                                                                                                                                                                                                                             |
| `AUTO_PARTITION_MODE`             | read,write     | A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.                                                                                                                                                                                                                                                                                                 |
| `CLI_ANALYZE_COLUMNS`             | read,write     | Go template for analyzing column data.                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_ASYNC_DDL`                   | read,write     | A boolean indicating whether DDL statements should be executed asynchronously. The default is false.                                                                                                                                                                                                                                                                                                                                |
| `CLI_AUTOWRAP`                    | read,write     | Enable automatic line wrapping.                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_AUTO_CONNECT_AFTER_CREATE`   | read,write     | A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.                                                                                                                                                                                                                                                                                                                    |
| `CLI_BIGQUERY_LOCATION`           | read,write     | BigQuery location for queries (e.g. US, EU).                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_BIGQUERY_MAX_BYTES_BILLED`   | read,write     | Maximum bytes billed per BigQuery query.                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_BIGQUERY_PROJECT`            | read,write     | GCP project for BigQuery queries. Defaults to CLI_PROJECT when empty.                                                                                                                                                                                                                                                                                                                                                               |
| `CLI_CURRENT_WIDTH`               | read           | Current terminal width. Returns NULL if not connected to a terminal.                                                                                                                                                                                                                                                                                                                                                                |
| `CLI_DATABASE`                    | read           | Cloud Spanner database ID.                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_DATABASE_DIALECT`            | read,write     | Database dialect for the session.                                                                                                                                                                                                                                                                                                                                                                                                   |
| `CLI_DIRECT_READ`                 | read           | Directed read options for read-only operations, in replica_location:replica_type format. Set by the --directed-read flag.                                                                                                                                                                                                                                                                                                           |
| `CLI_ECHO_EXECUTED_DDL`           | read,write     | Echo executed DDL statements.                                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_ECHO_INPUT`                  | read,write     | Echo input statements.                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_EMULATOR_PLATFORM`           | read           | Container platform used by embedded emulator.                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_ENABLE_ADC_PLUS`             | read,write     | A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.                                                                                                                                                                                                                                                                                          |
| `CLI_ENABLE_HIGHLIGHT`            | read,write     | Enable syntax highlighting.                                                                                                                                                                                                                                                                                                                                                                                                         |
| `CLI_ENABLE_PROGRESS_BAR`         | read,write     | A boolean indicating whether to display progress bars during operations. The default is false.                                                                                                                                                                                                                                                                                                                                      |
| `CLI_ENDPOINT`                    | read           | Host and port for connections (host:port format).                                                                                                                                                                                                                                                                                                                                                                                   |
| `CLI_EXPLAIN_FORMAT`              | read,write     | Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.                                                                                                                                                                                                                                                                                              |
| `CLI_EXPLAIN_HANGING_INDENT`      | read,write     | Use hanging indent for wrapped query plan lines in EXPLAIN, EXPLAIN ANALYZE, and query profile rendering. Only affects output when CLI_EXPLAIN_WRAP_WIDTH or WIDTH is set.                                                                                                                                                                                                                                                          |
| `CLI_EXPLAIN_PRINT_SECTIONS`      | read,write     | Query plan appendix preset or comma-separated sections to print. Presets: basic, enhanced, full, none. Sections: predicates, ordering, aggregate, typed, full. Empty string suppresses appendices.                                                                                                                                                                                                                                  |
| `CLI_EXPLAIN_WRAP_WIDTH`          | read,write     | Controls query plan wrap width. It effects only operators column contents                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_FIXED_WIDTH`                 | read,write     | If set, limits output width to the specified number of characters. NULL means automatic width detection.                                                                                                                                                                                                                                                                                                                            |
| `CLI_FORMAT`                      | read,write     | Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated, raw values), TSV (tab-separated with tab/newline/carriage-return/backslash escaping), HTML (HTML table), XML (XML format), CSV (comma-separated values).                                                                                   |
| `CLI_FUZZY_FINDER_KEY`            | read,write     | Key binding for fuzzy finder. Uses go-readline-ny key names (e.g., C_T, M_F, F1). Set to empty string to disable. The default is C_T (Ctrl+T).                                                                                                                                                                                                                                                                                      |
| `CLI_FUZZY_FINDER_OPTIONS`        | read,write     | Additional fzf options passed to the fuzzy finder. Appended after built-in defaults, so user options take precedence. Example: --color=dark --no-select-1                                                                                                                                                                                                                                                                           |
| `CLI_HISTORY_FILE`                | read           | Path to the history file.                                                                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_HOST`                        | read           | Host on which Spanner server is located                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_IMPERSONATE_SERVICE_ACCOUNT` | read           | Service account to impersonate.                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_INLINE_STATS`                | read,write     | \<name\>:\<template\>, ...                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_INSECURE`                    | read           | Skip TLS certificate verification (insecure).                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_INSTANCE`                    | read           | Cloud Spanner instance ID.                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_LINT_PLAN`                   | read,write     | Enable query plan linting.                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_LOG_GRPC`                    | read           | Enable gRPC logging.                                                                                                                                                                                                                                                                                                                                                                                                                |
| `CLI_LOG_LEVEL`                   | read,write     | Log level for the CLI.                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_MARKDOWN_CODEBLOCK`          | read,write     | Enable markdown codeblock output.                                                                                                                                                                                                                                                                                                                                                                                                   |
| `CLI_MCP`                         | read           | A read-only boolean indicating whether the connection is running as an MCP server.                                                                                                                                                                                                                                                                                                                                                  |
| `CLI_OUTPUT_TEMPLATE_FILE`        | read,write     | Go text/template for formatting the output of the CLI.                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_PARSE_MODE`                  | read,write     | Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED                                                                                                                                                                                                                                                                                                                                     |
| `CLI_PORT`                        | read           | Port number for connections.                                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_PROFILE`                     | read,write     | Enable performance profiling (memory and timing metrics).                                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_PROJECT`                     | read           | GCP Project ID.                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_PROMPT`                      | read,write     | Custom prompt for spanner-mycli.                                                                                                                                                                                                                                                                                                                                                                                                    |
| `CLI_PROMPT2`                     | read,write     | Custom continuation prompt for spanner-mycli.                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_PROTOTEXT_MULTILINE`         | read,write     | Enable multiline prototext output.                                                                                                                                                                                                                                                                                                                                                                                                  |
| `CLI_PROTO_DESCRIPTOR_FILE`       | read,write,add | Comma-separated list of proto descriptor files. Supports ADD to append files.                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_QUERY_MODE`                  | read,write     | Query execution mode.                                                                                                                                                                                                                                                                                                                                                                                                               |
| `CLI_ROLE`                        | read           | Cloud Spanner database role.                                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_SKIP_COLUMN_NAMES`           | read,write     | A boolean indicating whether to suppress column headers in output. The default is false.                                                                                                                                                                                                                                                                                                                                            |
| `CLI_SKIP_SYSTEM_COMMAND`         | read           | A read-only boolean indicating whether system commands are disabled. Set by --skip-system-command or --system-command=OFF.                                                                                                                                                                                                                                                                                                          |
| `CLI_SQL_BATCH_SIZE`              | read,write     | Number of VALUES per INSERT statement for SQL export. 0 (default): single-row INSERT statements. 2+: multi-row INSERT with up to N rows per statement.                                                                                                                                                                                                                                                                              |
| `CLI_SQL_TABLE_NAME`              | read,write     | Table name for generated SQL statements. Required for SQL export formats. Supports both simple names (e.g., 'Users') and schema-qualified names (e.g., 'myschema.Users').                                                                                                                                                                                                                                                           |
| `CLI_STYLED_OUTPUT`               | read,write     | Controls ANSI styling in table output: AUTO (styled if TTY), TRUE (always styled), FALSE (never styled). Default is AUTO.                                                                                                                                                                                                                                                                                                           |
| `CLI_SUPPRESS_RESULT_LINES`       | read,write     | Suppress result lines like 'rows in set' for clean output. Useful for scripting and dump operations.                                                                                                                                                                                                                                                                                                                                |
| `CLI_TABLE_PREVIEW_ROWS`          | read,write     | Number of rows to preview for table width calculation in streaming mode. 0 means use header widths only. Positive values use that many rows for preview (default: 50). -1 means collect all rows (non-streaming).                                                                                                                                                                                                                   |
| `CLI_TABLE_STREAMING`             | read,write     | Controls table streaming output mode: AUTO/FALSE buffer table output for layout quality, TRUE streams table output. Non-table formats always stream. Default is AUTO.                                                                                                                                                                                                                                                               |
| `CLI_TAB_VISUALIZE`               | read,write     | Visualize tab characters with arrow symbol in table output.                                                                                                                                                                                                                                                                                                                                                                         |
| `CLI_TAB_WIDTH`                   | read,write     | Tab width. It is used for expanding tabs.                                                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_TRY_PARTITION_QUERY`         | read,write     | A boolean indicating whether to test query for partition compatibility instead of executing it.                                                                                                                                                                                                                                                                                                                                     |
| `CLI_TYPE_STYLES`                 | read,write     | Type-based ANSI styling for query results. Format: colon-separated TYPE=STYLE pairs (e.g., 'STRING=green:INT64=bold:NULL=dim'). Supports named colors (red, green, yellow, blue, magenta, cyan, white, black), attributes (bold, dim, italic, underline, reverse, strikethrough), and raw SGR numbers (e.g., 38;5;214 for 256-color). NULL key overrides the default dim style for NULL values. Empty string disables type styling. |
| `CLI_USE_PAGER`                   | read,write     | Enable pager for output.                                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_VERBOSE`                     | read,write     | Display verbose output.                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_VERSION`                     | read           | The version of spanner-mycli.                                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_VERTEXAI_LOCATION`           | read,write     | Vertex AI location for natural language features.                                                                                                                                                                                                                                                                                                                                                                                   |
| `CLI_VERTEXAI_MODEL`              | read,write     | Vertex AI model for natural language features.                                                                                                                                                                                                                                                                                                                                                                                      |
| `CLI_VERTEXAI_PROJECT`            | read,write     | Vertex AI project for natural language features.                                                                                                                                                                                                                                                                                                                                                                                    |
| `CLI_WIDTH_STRATEGY`              | read,write     | Controls column width allocation algorithm: GREEDY_FREQUENCY (default, frequency-based greedy), PROPORTIONAL (proportional to natural width), MARGINAL_COST (wrap-line minimization via max-heap).                                                                                                                                                                                                                                  |
| `COMMIT_RESPONSE`                 | read           | The most recent response for a read-write transaction. This is a virtual variable: it can be used in SHOW COMMIT_RESPONSE and SHOW COMMIT_RESPONSE.COMMIT_TIMESTAMP and SHOW COMMIT_RESPONSE.MUTATION_COUNT, but attempting to read its value directly will give an error. Instead use the sub-fields COMMIT_TIMESTAMP and MUTATION_COUNT.                                                                                          |
| `COMMIT_TIMESTAMP`                | read           | The commit timestamp of the last read-write transaction that Spanner committed.                                                                                                                                                                                                                                                                                                                                                     |
| `DATA_BOOST_ENABLED`              | read,write     | A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.                                                                                                                                                                                                                                                                                                     |
| `DEFAULT_ISOLATION_LEVEL`         | read,write     | The transaction isolation level that is used by default for read/write transactions.                                                                                                                                                                                                                                                                                                                                                |
| `EXCLUDE_TXN_FROM_CHANGE_STREAMS` | read,write     | Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).                                                                                                                                                                                                                                                                      |
| `MAX_COMMIT_DELAY`                | read,write     | The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.                                                                                                                                                                                                                                                                                       |
| `MAX_PARTITIONED_PARALLELISM`     | read,write     | A property of type INT64 indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for AUTO_PARTITION_MODE=TRUE and RUN PARTITIONED QUERY                                                                                                                                                                                                                                            |
| `OPTIMIZER_STATISTICS_PACKAGE`    | read,write     | A property of type STRING indicating the current optimizer statistics package that is used by this connection.                                                                                                                                                                                                                                                                                                                      |
| `OPTIMIZER_VERSION`               | read,write     | A property of type STRING indicating the optimizer version. The version is either an integer string or LATEST.                                                                                                                                                                                                                                                                                                                      |
| `READONLY`                        | read,write     | A boolean indicating whether or not the connection is in read-only mode                                                                                                                                                                                                                                                                                                                                                             |
| `READ_LOCK_MODE`                  | read,write     | The read lock mode for read/write transactions. OPTIMISTIC uses optimistic concurrency control; PESSIMISTIC uses pessimistic locking. Default is UNSPECIFIED (server default).                                                                                                                                                                                                                                                      |
| `READ_ONLY_STALENESS`             | read,write     | A property of type STRING for read-only transactions with flexible staleness.                                                                                                                                                                                                                                                                                                                                                       |
| `READ_TIMESTAMP`                  | read           | The read timestamp of the most recent read-only transaction.                                                                                                                                                                                                                                                                                                                                                                        |
| `RETRY_ABORTS_INTERNALLY`         | read,write     | A boolean indicating whether the connection automatically retries aborted transactions. The default is true.                                                                                                                                                                                                                                                                                                                        |
| `RETURN_COMMIT_STATS`             | read,write     | A property of type BOOL indicating whether statistics should be returned for transactions on this connection.                                                                                                                                                                                                                                                                                                                       |
| `RPC_PRIORITY`                    | read,write     | A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.                                                                                                                                                                                                                                                     |
| `STATEMENT_TAG`                   | read,write     | A property of type STRING that contains the request tag for the next statement.                                                                                                                                                                                                                                                                                                                                                     |
| `STATEMENT_TIMEOUT`               | read,write     | A property of type STRING indicating the current timeout value for statements (e.g., 10s, 5m, 1h). Default is 10m.                                                                                                                                                                                                                                                                                                                  |
| `TRANSACTION_TAG`                 | read,write     | A property of type STRING that contains the transaction tag for the next transaction.                                                                                                                                                                                                                                                                                                                                               |
<!-- sysvars-help end -->

## Detailed variable documentation

This section provides extended documentation for selected variables that need
more explanation than the reference table above.

### Output Formatting Variables

#### CLI_SKIP_COLUMN_NAMES
- **Type**: BOOL
- **Default**: FALSE
- **Description**: Suppresses column headers in query result output
- **Access**: Read/Write
- **Usage**: 
  ```sql
  SET CLI_SKIP_COLUMN_NAMES = TRUE;
  SELECT * FROM users;  -- Output without column headers
  ```
- **Notes**:
  - Affects table format (`CLI_FORMAT='TABLE'`), tab format (`CLI_FORMAT='TAB'`), TSV format (`CLI_FORMAT='TSV'`), CSV format (`CLI_FORMAT='CSV'`), HTML format (`CLI_FORMAT='HTML'`), and XML format (`CLI_FORMAT='XML'`)
  - Headers are always preserved in vertical format (`CLI_FORMAT='VERTICAL'`) as they are integral to the format
  - Can be set via `--skip-column-names` command-line flag
  - Useful for scripting and data processing where only raw data is needed

#### CLI_SKIP_SYSTEM_COMMAND
- **Type**: BOOL
- **Default**: FALSE
- **Description**: Indicates whether system commands are disabled
- **Access**: Read-only
- **Usage**: 
  ```sql
  SHOW CLI_SKIP_SYSTEM_COMMAND;  -- Check if system commands are disabled
  ```
- **Notes**:
  - This is a read-only variable that reflects the state set by command-line flags
  - Can be set via `--skip-system-command` flag or `--system-command=OFF`
  - When set to TRUE, the `\!` meta command is disabled
  - When both flags are used, `--skip-system-command` takes precedence
  - Security feature to prevent shell command execution in restricted environments

#### CLI_TABLE_STREAMING
- **Type**: ENUM
- **Default**: AUTO
- **Description**: Controls streaming behavior for table-oriented output formats.
- **Access**: Read/Write
- **Valid Values**:
  - `AUTO` (default) - Buffer table output to calculate widths for stable layout.
  - `TRUE` - Stream table output as rows are produced for faster time-to-first-byte.
  - `FALSE` - Never stream table output; always buffer table output for full layout.
- **Usage**:
  ```sql
  SET CLI_TABLE_STREAMING = 'FALSE';
  SELECT * FROM users;  -- Table output is buffered for width stability

  SET CLI_TABLE_STREAMING = 'TRUE';
  SELECT * FROM users;  -- Table output is streamed
  ```
- **Notes**:
  - This setting only affects table formats. Non-table formats (CSV, JSONL, etc.) continue to stream regardless.
  - `CLI_TABLE_PREVIEW_ROWS` controls how many rows are used to estimate column width before streaming.
  - `AUTO` is the safe default for balanced memory usage and result formatting.

#### CLI_FORMAT
- **Type**: STRING
- **Default**: TABLE
- **Description**: Controls output format for query results
- **Access**: Read/Write
- **Valid Values**:
  - `TABLE` - ASCII table with borders (default for both interactive and batch modes)
  - `TABLE_COMMENT` - Table wrapped in `/* */` comments
  - `TABLE_DETAIL_COMMENT` - Table and execution details wrapped in `/* */` comments (useful for embedding results in SQL code blocks)
  - `VERTICAL` - Vertical format with column:value pairs
  - `TAB` - Tab-separated values (raw; values are joined with tabs as-is, so values containing tabs or newlines break the row/column structure)
  - `TSV` - Tab-separated values with escaping (tab, newline, carriage return, and backslash inside values are escaped as `\t`, `\n`, `\r`, and `\\`, guaranteeing one row per line and one field per column)
  - `HTML` - HTML table format
  - `XML` - XML format
  - `CSV` - Comma-separated values (RFC 4180 compliant)
  - `JSONL` - JSON Lines (one JSON object per row with type-aware values)
  - `SQL_INSERT` - INSERT statements
  - `SQL_INSERT_OR_IGNORE` - INSERT OR IGNORE statements
  - `SQL_INSERT_OR_UPDATE` - INSERT OR UPDATE statements
- **Usage**: 
  ```sql
  SET CLI_FORMAT = 'VERTICAL';
  SELECT * FROM users;  -- Output in vertical format
  
  SET CLI_FORMAT = 'HTML';
  SELECT * FROM users;  -- Output as HTML table
  
  SET CLI_FORMAT = 'XML';
  SELECT * FROM users;  -- Output as XML
  
  SET CLI_FORMAT = 'CSV';
  SELECT * FROM users;  -- Output as CSV (comma-separated values)

  SET CLI_FORMAT = 'JSONL';
  SELECT * FROM users;  -- Output as JSON Lines (one JSON object per row)

  SET CLI_FORMAT = 'TABLE_DETAIL_COMMENT';
  SELECT * FROM users;  -- Output as table with execution stats, all wrapped in /* */ comments
  ```
- **Notes**:
  - Can be set via `--html` flag (sets to HTML format)
  - Can be set via `--xml` flag (sets to XML format)
  - Can be set via `--csv` flag (sets to CSV format)
  - Can be set via `--format=jsonl` flag (sets to JSONL format)
  - Can be set via `--table` flag (explicit TABLE format; batch mode already defaults to TABLE)
  - HTML and XML formats are compatible with Google Cloud Spanner CLI
  - All special characters are properly escaped in HTML, XML, and CSV formats for security
  - CSV format follows RFC 4180 standard with automatic escaping of commas, quotes, and newlines
  - TSV format escapes `\` as `\\`, tab as `\t`, newline as `\n`, and carriage return as `\r` in both header names and values; double quotes are not special and are emitted verbatim; empty strings produce empty fields; NULL is rendered as the literal text `NULL` (same as TAB/TABLE). Use TAB if you need the historical raw (unescaped) behavior
  - JSONL format produces type-aware JSON: INT64/ENUM as numbers, BOOL as booleans, ARRAY as JSON arrays, STRUCT as JSON objects, NULL as null
  - The format affects how query results are displayed, not how they are executed
  - `TABLE_DETAIL_COMMENT` is particularly useful with `CLI_ECHO_INPUT=TRUE` and `CLI_MARKDOWN_CODEBLOCK=TRUE` for documentation

#### CLI_SQL_TABLE_NAME
- **Type**: STRING
- **Default**: (empty)
- **Description**: Table name for generated SQL statements
- **Access**: Read/Write
- **Usage**: 
  ```sql
  SET CLI_SQL_TABLE_NAME = 'DestTable';
  SET CLI_FORMAT = 'SQL_INSERT';
  SELECT * FROM SourceTable;  -- Generates INSERT INTO DestTable statements
  
  -- Schema-qualified names are supported
  SET CLI_SQL_TABLE_NAME = 'myschema.Users';
  ```
- **Notes**:
  - Required when using SQL export formats (SQL_INSERT, SQL_INSERT_OR_IGNORE, SQL_INSERT_OR_UPDATE)
  - Supports both simple names (e.g., 'Users') and schema-qualified names (e.g., 'myschema.Users')
  - Identifiers are automatically quoted when necessary using memefish's ast.Path

#### CLI_SQL_BATCH_SIZE
- **Type**: INT64
- **Default**: 0
- **Description**: Number of VALUES per INSERT statement for SQL export
- **Access**: Read/Write
- **Valid Values**:
  - `0` or `1` - Single-row INSERT statements (one per row)
  - `2` or higher - Multi-row INSERT with up to N rows per statement
- **Usage**: 
  ```sql
  -- Single-row INSERTs (default)
  SET CLI_SQL_BATCH_SIZE = 0;
  SET CLI_SQL_TABLE_NAME = 'users';
  SET CLI_FORMAT = 'SQL_INSERT';
  SELECT * FROM users LIMIT 3;
  -- Output:
  -- INSERT INTO users (id, name) VALUES (1, 'Alice');
  -- INSERT INTO users (id, name) VALUES (2, 'Bob');
  -- INSERT INTO users (id, name) VALUES (3, 'Charlie');
  
  -- Multi-row INSERTs (batch size of 100)
  SET CLI_SQL_BATCH_SIZE = 100;
  SELECT * FROM users LIMIT 200;
  -- Output:
  -- INSERT INTO users (id, name) VALUES
  --   (1, 'Alice'),
  --   (2, 'Bob'),
  --   ... (up to 100 rows);
  -- INSERT INTO users (id, name) VALUES
  --   (101, 'Dave'),
  --   ... (remaining rows);
  ```
- **Notes**:
  - Affects SQL export formats only
  - Batching can improve performance when importing large datasets
  - The last batch may contain fewer rows than the batch size

#### CLI_WIDTH_STRATEGY
- **Type**: ENUM
- **Default**: GREEDY_FREQUENCY
- **Description**: Column width allocation algorithm for table output
- **Access**: Read/Write
- **Valid Values**:
  - `GREEDY_FREQUENCY` - Frequency-based greedy expansion (default, original algorithm)
  - `PROPORTIONAL` - Allocate proportional to each column's natural width
  - `MARGINAL_COST` - Aims to minimize total wrap-lines via greedy max-heap approach
- **Usage**:
  ```sql
  SET CLI_WIDTH_STRATEGY = 'MARGINAL_COST';
  SELECT * FROM large_table;  -- Uses wrap-minimizing allocation
  ```
- **Notes**:
  - Only affects table formats with autowrap enabled (`CLI_AUTOWRAP = TRUE`)
  - `GREEDY_FREQUENCY` matches the behavior of previous versions
  - `MARGINAL_COST` often produces the fewest wrapped lines but may allocate narrower columns to infrequent wide values

### Interactive / Fuzzy Finder Variables

#### CLI_FUZZY_FINDER_OPTIONS
- **Type**: STRING
- **Default**: (empty)
- **Description**: Additional fzf options passed to the fuzzy finder
- **Access**: Read/Write
- **Usage**:
  ```sql
  SET CLI_FUZZY_FINDER_OPTIONS = '--color=dark';
  SET CLI_FUZZY_FINDER_OPTIONS = '--no-select-1 --no-cycle';  -- Override defaults
  SET CLI_FUZZY_FINDER_OPTIONS = '';  -- Reset to defaults only
  ```
- **Notes**:
  - Options are appended after built-in defaults, so user options take precedence (last wins)
  - Uses standard fzf option syntax (space-separated flags)
  - Built-in defaults: `--reverse`, `--no-sort`, `--height=<computed>`, `--border=rounded`, `--info=inline-right`, `--select-1`, `--exit-0`, `--highlight-line`, `--cycle`, and `--header-border=inline` when a header is shown
  - Useful for customizing appearance (colors, layout) or behavior (sorting, preview)
  - `--tmux` and `--popup` are **not supported** because the fuzzy finder runs fzf in-process via the Go library

#### CLI_TYPE_STYLES
- **Type**: STRING
- **Default**: `"NULL=dim"`
- **Description**: Configures ANSI styling for query result values based on their Spanner type
- **Access**: Read/Write
- **Format**: Colon-separated `TYPE=STYLE` pairs (e.g., `"STRING=green:INT64=bold:NULL=dim"`)
- **Usage**:
  ```sql
  -- Color strings green and integers bold
  SET CLI_TYPE_STYLES = 'STRING=green:INT64=bold';

  -- Use 256-color for timestamps
  SET CLI_TYPE_STYLES = 'TIMESTAMP=38;5;214';

  -- Combine attributes: bold green
  SET CLI_TYPE_STYLES = 'STRING=bold;green';

  -- Disable all type styling
  SET CLI_TYPE_STYLES = '';

  -- Check current setting
  SHOW CLI_TYPE_STYLES;
  ```
- **Supported Types**:
  `BOOL`, `INT64`, `FLOAT32`, `FLOAT64`, `NUMERIC`, `STRING`, `BYTES`, `JSON`, `DATE`, `TIMESTAMP`, `ARRAY`, `STRUCT`, `PROTO`, `ENUM`, `INTERVAL`, `UUID`, `NULL`
- **Style Values**:
  - **Named colors**: `black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`
  - **Named attributes**: `bold`, `dim`, `italic`, `underline`, `blink`, `reverse`, `hidden`, `strikethrough`
  - **Raw SGR numbers**: Any valid SGR parameter number (e.g., `31` for red, `38;5;214` for 256-color, `38;2;255;128;0` for truecolor)
  - **Combined**: Semicolon-separated (e.g., `bold;green` produces `\033[1;32m`)
- **Notes**:
  - Type names are case-insensitive (`string=green` works)
  - `NULL` is a special pseudo-type that styles NULL values regardless of their column type
  - When `CLI_TYPE_STYLES` is empty, no type-based styling is applied
  - The default `"NULL=dim"` renders NULL values in dim (faint) text
  - Styling only applies when output supports ANSI escape codes (interactive terminal with styled formats)
  - Inspired by `LS_COLORS`, `GCC_COLORS`, and `JQ_COLORS` environment variable patterns

Variables not covered in this section are described by the generated
[reference table](#reference) above.
