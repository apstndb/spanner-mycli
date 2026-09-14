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
RESET ALL;                      -- Restore resettable variables to their startup snapshots
RESET CLI_FORMAT;               -- Restore one variable (canonical name or alias)
-- Ordinary SET is session-durable through COMMIT and ROLLBACK, including after
-- SET LOCAL. This is not PostgreSQL transactional SET: ROLLBACK does not undo
-- a successful SET. A later SET LOCAL in the same transaction saves the new
-- session value and restores that value when the transaction ends.
-- RESET and RESET ALL restore values captured after defaults, config, flags,
-- and --set, before --init-command / --init-command-add. Init-command
-- assignments are ordinary SQL and can be reset. File-backed
-- descriptors/templates, opaque graphs, connection identity, stream handles,
-- and unimplemented placeholders are excluded. After a successful RESET,
-- targeted LOCAL undo is retired (including equal-value resets); a rejected
-- RESET changes neither values nor undo. RESET LOCAL and SET x=DEFAULT are
-- not supported.
HELP VARIABLES;                 -- Show the reference table below interactively
```

Variables can also be set at startup with `--set NAME=VALUE` command-line flags.

## Reference

The table below lists all system variables. The `operations` column shows
whether a variable can be read (`SHOW`), written (`SET`), or appended to
(`SET ... += ...`). A variable marked `unimplemented` is recognized for
compatibility with Spanner JDBC connection properties but currently rejects
both `SHOW` and `SET`.

<!-- The table between the markers below is generated from the variable
     registry by `make docs-update` (via the hidden --sysvars-help flag).
     Do not edit it by hand. -->
<!-- sysvars-help begin -->
| Name                                       | Operations     | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
|:-------------------------------------------|:---------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `AUTOCOMMIT`                               | unimplemented  | A boolean indicating whether or not the connection is in autocommit mode. The default is true.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `AUTOCOMMIT_DML_MODE`                      | read,write     | A STRING property indicating the autocommit mode for Data Manipulation Language (DML) statements. TRANSACTIONAL (default) commits each implicit DML atomically. PARTITIONED_NON_ATOMIC uses partitioned DML for implicit UPDATE/DELETE. TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC retries one eligible implicit UPDATE/DELETE as partitioned DML only after a SQL-phase mutation-limit failure that matches the pinned InvalidArgument + exact mutation-limit sentence + Cloud Spanner limits Help classifier. The fallback is non-atomic, returns a lower-bound count, and can partially commit if the partitioned attempt later fails. INSERT, THEN RETURN, explicit/pending/RO/SAVEPOINT owners, batches, EXPLAIN/analysis, Commit-phase failures, and weaker resource-limit errors are not retried. Not a Java-complete or stable driver-parity claim. |
| `AUTO_BATCH_DML`                           | read,write     | A BOOL indicating whether DML in an explicit read-write transaction is buffered until COMMIT, a later execute-now statement, or RUN BATCH. SET only changes future buffering. The default is false.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `AUTO_BATCH_DML_UPDATE_COUNT`              | read,write     | A nonnegative INT64 expected update count captured for each newly buffered automatic DML statement. The default is 1. Zero is a valid explicit expectation. This value is an expectation only; it is never reported as observed affected rows. SET, SET LOCAL, RESET, and transaction-end restoration change the policy for future enqueues only.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `AUTO_BATCH_DML_UPDATE_COUNT_VERIFICATION` | read,write     | A BOOL indicating whether flush compares each captured expected count with the actual BatchUpdate count before accepting a successful automatic DML receipt. The default is false so existing arbitrary UPDATE/DELETE statements keep succeeding until verification is enabled. SET, SET LOCAL, RESET, and transaction-end restoration change the policy for future enqueues only. Replay still validates journaled actual counts when this is false.                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `AUTO_PARTITION_MODE`                      | read,write     | A property of type BOOL indicating whether the connection automatically uses partitioned queries for all queries that are executed.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `CLI_ANALYZE_COLUMNS`                      | read,write     | Go template for analyzing column data.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `CLI_AUTOWRAP`                             | read,write     | Enable automatic line wrapping.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_AUTO_CONNECT_AFTER_CREATE`            | read,write     | A boolean indicating whether to automatically connect to a database after CREATE DATABASE. The default is false.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_BIGQUERY_LOCATION`                    | read,write     | BigQuery location for queries (e.g. US, EU).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_BIGQUERY_MAX_BYTES_BILLED`            | read,write     | Maximum bytes billed per BigQuery query.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `CLI_BIGQUERY_PROJECT`                     | read,write     | GCP project for BigQuery queries. Defaults to CLI_PROJECT when empty.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `CLI_CA_CERT_FILE`                         | read           | Path to the PEM CA certificate file used as the TLS trust bundle. Empty when unset. Replaces system roots when set. Startup-only.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `CLI_CLIENT_CERT_FILE`                     | read           | Path to the PEM client certificate file for mTLS. Empty when unset. Startup-only.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `CLI_CLIENT_CERT_KEY`                      | read           | Path to the PEM client private-key file for mTLS. SHOW reports the path only and never the key bytes. Empty when unset. Startup-only.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `CLI_CURRENT_WIDTH`                        | read           | Current terminal width. Returns NULL if not connected to a terminal.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `CLI_DATABASE`                             | read           | Cloud Spanner database ID.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_DATABASE_DIALECT`                     | read,write     | Database dialect for the session.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `CLI_DUMP_CYCLIC_MAX_BYTES`                | read,write     | Positive aggregate encoded cyclic-text retention cap for DUMP MUTATE mode. Default 67108864 (64 MiB). Not a hard heap bound or Spanner commit-size/mutation-count estimate.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_DUMP_CYCLIC_MODE`                     | read,write     | DUMP cyclic data policy: REJECT (default) or opt-in MUTATE (one transaction per cyclic table group). No service-quota prediction; later restore failure can leave earlier groups committed.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_ECHO_EXECUTED_DDL`                    | read,write     | Echo executed DDL statements.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_ECHO_INPUT`                           | read,write     | Echo input statements.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `CLI_EMULATOR_PLATFORM`                    | read           | Container platform used by embedded emulator.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_ENABLE_ADC_PLUS`                      | read,write     | A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_ENABLE_HIGHLIGHT`                     | read,write     | Enable syntax highlighting.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_ENABLE_PROGRESS_BAR`                  | read,write     | A boolean indicating whether to display progress bars during operations. The default is false.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `CLI_ENDPOINT`                             | read           | Host and port for connections (host:port format).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `CLI_EXPLAIN_FORMAT`                       | read,write     | Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `CLI_EXPLAIN_HANGING_INDENT`               | read,write     | Use hanging indent for wrapped query plan lines in EXPLAIN, EXPLAIN ANALYZE, and query profile rendering. Only affects output when CLI_EXPLAIN_WRAP_WIDTH or WIDTH is set.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_EXPLAIN_PRINT_SECTIONS`               | read,write     | Query plan appendix preset or comma-separated sections to print. Presets: basic, enhanced, full, none. Sections: predicates, ordering, aggregate, typed, full. Empty string suppresses appendices.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_EXPLAIN_WRAP_WIDTH`                   | read,write     | Controls query plan wrap width. It effects only operators column contents                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_FIXED_WIDTH`                          | read,write     | If set, limits output width to the specified number of characters. NULL means automatic width detection.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `CLI_FORMAT`                               | read,write     | Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated, raw values), TSV (tab-separated with tab/newline/carriage-return/backslash escaping), HTML (HTML table), XML (XML format), CSV (comma-separated values), JSONL (newline-delimited JSON), SQL_INSERT (INSERT statements), SQL_INSERT_OR_IGNORE (INSERT OR IGNORE statements), SQL_INSERT_OR_UPDATE (INSERT OR UPDATE statements).                                                                                                                                                                                                                                                                                                                                              |
| `CLI_FUZZY_FINDER_KEY`                     | read,write     | Key binding for fuzzy finder. Uses go-readline-ny key names (e.g., C_T, M_F, F1). Set to empty string to disable. The default is C_T (Ctrl+T).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `CLI_FUZZY_FINDER_OPTIONS`                 | read,write     | Additional fzf options passed to the fuzzy finder. Appended after built-in defaults, so user options take precedence. Example: --color=dark --no-select-1                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_GENAI_BACKEND`                        | read,write     | GenAI backend: GEMINI_ENTERPRISE or GEMINI_API. VERTEX_AI is accepted as an alias for GEMINI_ENTERPRISE.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `CLI_GENAI_THINKING_LEVEL`                 | read,write     | Gemini thinking level. UNSPECIFIED lets the model choose its default.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `CLI_HISTORY_FILE`                         | read           | Path to the history file.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_HOST`                                 | read           | Host on which Spanner server is located                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `CLI_IMPERSONATE_SERVICE_ACCOUNT`          | read           | Service account to impersonate.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_INLINE_STATS`                         | read,write     | \<name\>:\<template\>, ...                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_INSECURE`                             | read           | Permit plaintext gRPC (no TLS). Set by --insecure or --skip-tls-verify.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `CLI_INSTANCE`                             | read           | Cloud Spanner instance ID.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_LINT_PLAN`                            | read,write     | Enable query plan linting.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_LOG_GRPC`                             | read           | Enable gRPC logging.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `CLI_LOG_LEVEL`                            | read,write     | Log level for the CLI slog logger (DEBUG, INFO, WARN, ERROR; WARNING is accepted as WARN). SET and --set change the process threshold. Embedded container lifecycle logs follow the startup --log-level snapshot, not later SET.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_MARKDOWN_CODEBLOCK`                   | read,write     | Enable markdown codeblock output.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `CLI_MCP`                                  | read           | A read-only boolean indicating whether the connection is running as an MCP server.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_OUTPUT_TEMPLATE_FILE`                 | read,write     | Go text/template for formatting the output of the CLI.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `CLI_PARSE_MODE`                           | read,write     | Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_PORT`                                 | read           | Port number for connections.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_PROFILE`                              | read,write     | Enable performance profiling (memory and timing metrics).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_PROJECT`                              | read           | GCP Project ID.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_PROMPT`                               | read,write     | Custom prompt for spanner-mycli.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `CLI_PROMPT2`                              | read,write     | Custom continuation prompt for spanner-mycli.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_PROTOTEXT_MULTILINE`                  | read,write     | Enable multiline prototext output.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_QUERY_MODE`                           | read,write     | Query execution mode.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `CLI_ROLE`                                 | read           | Cloud Spanner database role.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_SAVEPOINT_SUPPORT`                    | read,write     | Enable client-emulated SAVEPOINT for explicit transactions. DISABLED (default) preserves current behavior. ENABLED records a journal from BEGIN and reconstructs RW prefixes on ROLLBACK TO. This is replay with result validation, not a native Spanner savepoint. SET is rejected while a transaction is pending or active and while a manual batch is open; SET LOCAL is not supported.                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_SKIP_COLUMN_NAMES`                    | read,write     | A boolean indicating whether to suppress column headers in output. The default is false.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `CLI_SKIP_SYSTEM_COMMAND`                  | read           | A read-only boolean indicating whether system commands are disabled. Set by --skip-system-command or --system-command=OFF.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CLI_SPANNER_METRICS_ENDPOINT`             | read           | Startup-only absolute http/https URL of the OTLP metrics collector used when CLI_SPANNER_METRICS_EXPORTER=otlp. Host required; no userinfo, query, or fragment. Missing or root path is /v1/metrics. Empty when export is off. Not SET-able.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `CLI_SPANNER_METRICS_EXPORTER`             | read           | Startup-only caller-owned Spanner client metrics exporter: off (default) or otlp. otlp uses OTLP HTTP/protobuf to CLI_SPANNER_METRICS_ENDPOINT. Does not enable native Cloud Monitoring or a global MeterProvider. OTEL_* environment variables alone do not initialize export. SPANNER_EMULATOR_HOST suppresses SDK caller metrics. Not SET-able.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_SPANNER_TRACES_ENDPOINT`              | read           | Startup-only absolute http/https URL of the OTLP traces collector used when CLI_SPANNER_TRACES_EXPORTER=otlp. Host required; no userinfo, query, or fragment. Missing or root path is /v1/traces. Empty when export is off. Not SET-able.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_SPANNER_TRACES_EXPORTER`              | read           | Startup-only process-owned Spanner client traces exporter: off (default) or otlp. otlp uses official OTLP HTTP/protobuf to CLI_SPANNER_TRACES_ENDPOINT and installs one CLI-owned global TracerProvider. Off does not change the global provider or environment. Does not set a global MeterProvider. OTEL_* environment variables alone do not initialize export. SPANNER_ENABLE_END_TO_END_TRACING is still honored by the SDK when CLI traces are off. Shutdown of the pinned initial OTel proxy uses a no-op substitute; previously acquired proxy tracers cannot be restored. Not SET-able.                                                                                                                                                                                                                                                                       |
| `CLI_SPANNER_TRACES_SAMPLE_RATIO`          | read           | Startup-only root sampling ratio in [0,1] for CLI-owned traces when CLI_SPANNER_TRACES_EXPORTER=otlp. ParentBased: sampled parents are honored. Default 0.01. Not SET-able.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_SQL_BATCH_SIZE`                       | read,write     | Number of VALUES per INSERT statement for SQL export. 0 (default): single-row INSERT statements. 2+: multi-row INSERT with up to N rows per statement.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `CLI_SQL_TABLE_NAME`                       | read,write     | Table name for generated SQL statements. Required for SQL export formats. Supports both simple names (e.g., 'Users') and schema-qualified names (e.g., 'myschema.Users').                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_STYLED_OUTPUT`                        | read,write     | Controls ANSI styling in table output: AUTO (styled if TTY), TRUE (always styled), FALSE (never styled). Default is AUTO.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_SUPPRESS_RESULT_LINES`                | read,write     | Suppress result lines like 'rows in set' for clean output. Useful for scripting and dump operations.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `CLI_TABLE_PREVIEW_ROWS`                   | read,write     | Number of rows to preview for table width calculation in streaming mode. 0 means use header widths only. Positive values use that many rows for preview (default: 50). -1 means collect all rows (non-streaming).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `CLI_TABLE_STREAMING`                      | read,write     | Controls table streaming output mode: AUTO/FALSE buffer table output for layout quality, TRUE streams table output. Non-table formats always stream. Default is AUTO.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `CLI_TAB_VISUALIZE`                        | read,write     | Visualize tab characters with arrow symbol in table output.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_TAB_WIDTH`                            | read,write     | Tab width. It is used for expanding tabs.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_TRY_PARTITION_QUERY`                  | read,write     | A boolean indicating whether to test query for partition compatibility instead of executing it.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `CLI_TYPE_STYLES`                          | read,write     | Type-based ANSI styling for query results. Format: colon-separated TYPE=STYLE pairs (e.g., 'STRING=green:INT64=bold:NULL=dim'). Supports named colors (red, green, yellow, blue, magenta, cyan, white, black), attributes (bold, dim, italic, underline, reverse, strikethrough), and raw SGR numbers (e.g., 38;5;214 for 256-color). NULL key overrides the default dim style for NULL values. Empty string disables type styling.                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `CLI_USE_PAGER`                            | read,write     | Enable pager for output.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `CLI_VERBOSE`                              | read,write     | Display verbose output.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `CLI_VERSION`                              | read           | The version of spanner-mycli.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CLI_VERTEXAI_LOCATION`                    | read,write     | Gemini Enterprise location for natural language features.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `CLI_VERTEXAI_MODEL`                       | read,write     | Gemini model for natural language features.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `CLI_VERTEXAI_PROJECT`                     | read,write     | Gemini Enterprise project override. Defaults to CLI_PROJECT when empty.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `CLI_WIDTH_STRATEGY`                       | read,write     | Controls column width allocation algorithm: GREEDY_FREQUENCY (default, frequency-based greedy), PROPORTIONAL (proportional to natural width), MARGINAL_COST (wrap-line minimization via max-heap).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `CLI_WITHOUT_AUTHENTICATION`               | read           | Do not send Google bearer credentials to the Spanner endpoint. Requires an explicit endpoint and at least one custom TLS file. Does not enable plaintext or skip certificate verification.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `COMMIT_PRIORITY`                          | read,write     | Commit RPC priority for read-write transactions (HIGH, MEDIUM, LOW). UNSPECIFIED (default) inherits the resolved transaction RPC priority, which is the existing mycli behavior. That differs from go-sql-spanner, where default UNSPECIFIED is the Go driver's CommitPriority default and does not inherit RPC_PRIORITY. The effective value is frozen in the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. SET LOCAL is not supported. Not applied to query, DML, heartbeat, partitioned DML, read-only, or Admin RPCs.                                                                                                                                                                                                                                                                                                  |
| `COMMIT_RESPONSE`                          | read           | The most recent response for a read-write transaction. SHOW VARIABLE COMMIT_RESPONSE returns COMMIT_TIMESTAMP and MUTATION_COUNT columns; SHOW VARIABLES includes those values as COMMIT_TIMESTAMP and MUTATION_COUNT.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `COMMIT_TIMESTAMP`                         | read           | The commit timestamp of the last read-write transaction that Spanner committed.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `DATA_BOOST_ENABLED`                       | read,write     | A property of type BOOL indicating whether this connection should use Data Boost for partitioned queries. The default is false.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `DDL_ASYNC_WAIT_TIMEOUT`                   | read,write     | Maximum time ASYNC_WAIT spends waiting for a DDL operation before returning the still-running operation ID as a successful asynchronous submission. The remaining budget bounds in-flight GetOperation polls as well as the time between polls. Expiry cancels only the polling RPC and does not cancel the server operation. The default is 10s. Unused in SYNC and ASYNC modes.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `DDL_EXECUTION_MODE`                       | read,write     | How DDL statements wait for the Admin long-running operation. SYNC (default) waits for the actual result. ASYNC returns the accepted operation ID immediately. ASYNC_WAIT waits up to DDL_ASYNC_WAIT_TIMEOUT and, on wait-budget expiry, returns the still-running operation ID as a successful asynchronous submission without canceling the server operation. --async selects ASYNC. Replaces CLI_ASYNC_DDL.                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `DEFAULT_ISOLATION_LEVEL`                  | read,write     | The transaction isolation level that is used by default for read/write transactions.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `DEFAULT_SEQUENCE_KIND`                    | read,write     | Opt-in default sequence kind used only to repair a precise missing-kind SYNC DDL failure. Empty/NULL (default) disables repair. The only accepted non-empty value is bit_reversed_positive. When enabled, a matching InvalidArgument failure may submit one extra ALTER DATABASE to set the database option default_sequence_kind (a database-wide schema mutation; requires the existing Spanner DDL update permission) and then retry only the metadata-proven unfinished suffix. Repair does not run in ASYNC, ASYNC_WAIT, or SHOW OPERATION.                                                                                                                                                                                                                                                                                                                       |
| `DIRECTED_READ`                            | read,write     | Directed read options for supported read-only queries. Accepts replica_location or replica_location:READ_ONLY\|READ_WRITE shorthand, or DirectedReadOptions protobuf JSON. SHOW uses shorthand when that form is lossless; otherwise protobuf JSON. Empty string clears. SET is rejected while a transaction is pending or active; SET LOCAL is not supported. Not applied to read-write queries, DML, heartbeat, or partitioned DML.                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `EXCLUDE_TXN_FROM_CHANGE_STREAMS`          | read,write     | Controls whether to exclude recording modifications in current transaction from the allowed tracking change streams(with DDL option allow_txn_exclusion=true).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `KEEP_TRANSACTION_ALIVE`                   | read,write     | Whether an explicit read-write owner schedules keepalive heartbeats after the first user SQL. TRUE (default) preserves existing mycli behavior. FALSE prevents heartbeat scheduling for that owner without changing user SQL, COMMIT, ROLLBACK, or cancellation. Java KEEP_TRANSACTION_ALIVE defaults to false; this CLI default is intentionally TRUE. The policy is frozen on the logical owner with the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. Changing the session default does not alter an active owner. SET LOCAL is not supported. Idle-deadline (#357) is not implemented. TRANSACTION_TIMEOUT is a separate logical-owner budget and is not implied by this variable.                                                                                                                                     |
| `MAX_COMMIT_DELAY`                         | read,write     | The amount of latency this request is configured to incur in order to improve throughput. You can specify it as duration between 0 and 500ms.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `MAX_PARTITIONED_PARALLELISM`              | read,write     | A property of type INT64 indicating the number of worker threads the spanner-mycli uses to execute partitions. This value is used for AUTO_PARTITION_MODE=TRUE and RUN PARTITIONED QUERY                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `OPTIMIZER_STATISTICS_PACKAGE`             | read,write     | A property of type STRING indicating the current optimizer statistics package that is used by this connection.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `OPTIMIZER_VERSION`                        | read,write     | A property of type STRING indicating the optimizer version. The version is either an integer string or LATEST.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `PROTO_DESCRIPTORS`                        | read,write     | Base64 FileDescriptorSet for the session proto graph. DUMP SCHEMA/DATABASE emit SET PROTO_DESCRIPTORS so replay is self-contained. SET LOCAL is not supported. Cannot be changed while a manual batch is active.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `PROTO_DESCRIPTORS_FILE_PATH`              | read,write,add | Comma-separated list of proto descriptor files. Supports ADD to append files. HTTP(S) source vs binary is classified from the URL path, not query or fragment.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `READONLY`                                 | read,write     | A boolean indicating whether or not the connection is in read-only mode                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `READ_LOCK_MODE`                           | read,write     | The read lock mode for read/write transactions. OPTIMISTIC uses optimistic concurrency control; PESSIMISTIC uses pessimistic locking. Default is UNSPECIFIED (server default).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `READ_ONLY_STALENESS`                      | read,write     | A property of type STRING for read-only transactions with flexible staleness.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `READ_TIMESTAMP`                           | read           | The read timestamp of the most recent read-only transaction.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `RETRY_ABORTS_INTERNALLY`                  | unimplemented  | A boolean indicating whether the connection automatically retries aborted transactions. The default is true.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `RETURN_COMMIT_STATS`                      | read,write     | A property of type BOOL indicating whether statistics should be returned for transactions on this connection.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `RPC_PRIORITY`                             | read,write     | A property of type STRING indicating the relative priority for Spanner requests. The priority acts as a hint to the Spanner scheduler and doesn't guarantee order of execution.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `STATEMENT_TAG`                            | read,write     | A property of type STRING that contains the request tag for the next statement.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `STATEMENT_TIMEOUT`                        | read,write     | A property of type STRING indicating the current timeout value for statements (e.g., 10s, 5m, 1h). NULL (the omitted-flag default) uses 10m for ordinary statements and 24h for partitioned DML. This is a CLI policy, not a server-required deadline.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `TRANSACTION_TAG`                          | read,write     | Transaction tag for the next physical read-write transaction. After that owner starts, SHOW reports the applied tag. The consumed slot is then empty unless a SET LOCAL baseline restores. Ordinary SET after SET LOCAL supersedes LOCAL. SET and SET LOCAL are rejected while a read-write transaction is active. Read-only transactions do not consume this tag; SET LOCAL during RO still restores. Partitioned DML does not currently send a transaction tag.                                                                                                                                                                                                                                                                                                                                                                                                      |
| `TRANSACTION_TIMEOUT`                      | read,write     | Logical read/write transaction deadline (duration or NULL). NULL or 0 means no additional transaction deadline. The duration is captured for the logical owner; the single total budget starts at the first real database RPC (including constructor BeginTransaction) and is preserved across physical reconstruction. Pending SET LOCAL may select the duration before the first RPC; changing it after first real database use is rejected, including when the selected duration is NULL or 0 and no timer exists. Session SET after BEGIN applies to a later owner. Distinct from STATEMENT_TIMEOUT and from unimplemented user-idle expiry (#357). ABORTED retries (#293) are not implemented; a later retry path must reuse the remaining budget.                                                                                                                |
<!-- sysvars-help end -->

## Detailed variable documentation

This section provides extended documentation for selected variables that need
more explanation than the reference table above.

### JDBC-inspired variables

#### DIRECTED_READ
- **Type**: STRING (`location`, `location:READ_ONLY|READ_WRITE`, or DirectedReadOptions protobuf JSON)
- **Default**: empty (no directed read)
- **Access**: Read/Write. `SET` is rejected while a transaction is pending or active. `SET LOCAL` is not supported.
- **Description**: Routes supported read-only queries using DirectedReadOptions. The location[:type] shorthand is the same grammar as `--directed-read` and always sets one include replica with autoFailoverDisabled=true. Protobuf JSON accepts the complete message (include or exclude, multiple replicaSelections, autoFailoverDisabled). An empty value clears the option. SHOW prints shorthand when that form round-trips without loss; otherwise it prints protobuf JSON.
- **Notes**:
  - Compatible with java-spanner / go-sql-spanner DirectedReadOptions JSON. Unknown or malformed JSON fields are rejected and leave the current value unchanged.
  - Applied to autocommit SELECT, explicit read-only transactions (including the initialization `SELECT 1`), partition query execution, DUMP data-plane catalog/column/row reads, `DatabaseExists`, and metadata completions.
  - Not applied to read-write SELECT or DML, heartbeat, or partitioned DML.
  - `--directed-read` then `--set` keeps existing flag-then-`--set` precedence.

#### COMMIT_PRIORITY
- **Type**: STRING (`HIGH`, `MEDIUM`, `LOW`, `UNSPECIFIED`)
- **Default**: `UNSPECIFIED` (inherit the resolved transaction RPC priority)
- **Access**: Read/Write. `SET LOCAL` is not supported.
- **Description**: Overrides the Commit RPC priority for read-write transactions. Query, DML, heartbeat, partitioned DML, read-only, and Admin RPCs keep using their existing priority fields.
- **Notes**:
  - Default `UNSPECIFIED` inherits the already-resolved transaction RPC priority. This preserves the previous mycli behavior where Commit used the same priority as the transaction.
  - go-sql-spanner's `commit_priority` default is also `UNSPECIFIED`, but that value is the Go driver's `CommitPriority` default and does **not** inherit `RPC_PRIORITY`.
  - The effective value is frozen in the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. Changing the session default does not alter an active or reconstructed attempt.
  - `SET` during a pending transaction is applied when the owner is constructed. `SET LOCAL` is not supported because existing guards cannot apply a local override only before activation without mutating a frozen attempt.

#### KEEP_TRANSACTION_ALIVE
- **Type**: BOOL
- **Default**: TRUE (preserve existing mycli heartbeat after the first explicit read-write user SQL)
- **Access**: Read/Write. `SET LOCAL` is not supported.
- **Description**: Controls whether an explicit read-write owner schedules keepalive `SELECT 1` heartbeats after the first user SQL. `FALSE` prevents heartbeat scheduling for that owner without changing user SQL, COMMIT, ROLLBACK, or cancellation.
- **Notes**:
  - Java `KEEP_TRANSACTION_ALIVE` defaults to `false`. This CLI default is intentionally `TRUE` so existing sessions keep the current heartbeat behavior.
  - The policy is frozen on the logical owner with the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. Changing the session default does not alter an active or reconstructed owner.
  - `SET` during a pending transaction is applied when the owner is constructed.
  - Idle-deadline (#357) is not implemented. `TRANSACTION_TIMEOUT` is a separate logical-owner budget and is not implied by this variable.
  - The 5-second interval is unchanged; there is no interval-tuning surface.

#### TRANSACTION_TIMEOUT
- **Type**: STRING (duration or `NULL`)
- **Default**: `NULL` (no additional transaction deadline)
- **Access**: Read/Write. `SET LOCAL` is supported before the first real database RPC.
- **Description**: Logical read/write transaction deadline, distinct from `STATEMENT_TIMEOUT` and from unimplemented user-idle expiry (#357).
- **Notes**:
  - `NULL` or `0` means no additional transaction deadline.
  - The duration is captured for the logical owner. Session `SET` or `RESET` after `BEGIN` applies to a later owner, not the current one.
  - A pending `SET LOCAL` may select the duration before the first database RPC. Changing the budget after first real database use is rejected, including when the selected duration is `NULL` or `0` and no timer exists.
  - The single total budget starts at the first real database RPC, including constructor `BeginTransaction` for `ReadWriteStmtBasedTransaction`. Client-only `BEGIN`/`SHOW` and buffering automatic DML without a transaction RPC do not start it. First-use is tracked even when the duration is `NULL` or `0`.
  - The deadline is preserved across physical reconstruction (`ROLLBACK TO`) and is not restarted by later statements.
  - SQL, Batch DML, commit, and replay RPCs receive the minimum of the caller context, `STATEMENT_TIMEOUT`, and the remaining transaction budget.
  - Expiry cancels in-flight RPCs without waiting for the transaction mutex, retires only the matching logical owner, stops its heartbeat, and restores `SET LOCAL` at the serialized session safe point (start and end of `ExecuteStatement`, and `Close`). Timer goroutines never call `Registry.Set`.
  - ABORTED retries (#293) are not implemented. A later retry path must reuse the remaining budget.

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

### DDL_EXECUTION_MODE

How DDL statements wait for the Admin long-running operation. This is a
type/behavior replacement for the removed boolean `CLI_ASYNC_DDL`.

- **Type**: STRING (`SYNC` / `ASYNC` / `ASYNC_WAIT`)
- **Default**: `SYNC`
- **Access**: Read/Write
- **Values**:
  - `SYNC` waits until the LRO completes and reports its actual result
    (including a completed failing LRO).
  - `ASYNC` returns the accepted operation ID immediately. Later DDL failure
    remains visible through `SHOW OPERATION`. `--async` selects this mode.
  - `ASYNC_WAIT` waits until completion or `DDL_ASYNC_WAIT_TIMEOUT`. The
    remaining wait budget bounds the initial GetOperation poll, later polls,
    and the time between polls. When that separate wait budget expires, the
    still-running operation ID is returned as a successful asynchronous
    submission. Expiry cancels only the polling RPC; the server operation is
    not canceled. Caller or `STATEMENT_TIMEOUT` cancellation remains an error
    that includes the operation ID.
- **Migration**: `SET CLI_ASYNC_DDL = TRUE` becomes
  `SET DDL_EXECUTION_MODE = 'ASYNC'`. `FALSE` is the `SYNC` default.

### DDL_ASYNC_WAIT_TIMEOUT

- **Type**: duration string (for example `10s`, `1m`)
- **Default**: `10s`
- **Access**: Read/Write
- **Description**: Maximum time `ASYNC_WAIT` spends waiting before handing off
  the still-running operation ID. Unused in `SYNC` and `ASYNC`. Must be >= 0.
  The remaining budget applies to in-flight GetOperation polls as well as the
  between-poll wait. Zero expires the wait budget immediately for a
  still-pending operation, including before the first GetOperation poll. A
  terminal result already received from UpdateDatabaseDdl or a preceding poll
  is reported as-is.

### AUTOCOMMIT_DML_MODE

Autocommit DML routing. `TRANSACTIONAL` (default) commits each implicit DML
atomically. `PARTITIONED_NON_ATOMIC` uses partitioned DML for implicit
`UPDATE`/`DELETE`. `TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC`
keeps the transactional attempt first and retries **one** eligible implicit
`UPDATE` or `DELETE` as partitioned DML only when that attempt fails at
`ExecuteSql`/`ExecuteStreamingSql` before any `Commit`, with
`InvalidArgument`, the exact sentence `The transaction contains too many
mutations.`, and exactly one Help link
(`Cloud Spanner limits documentation.` /
`https://cloud.google.com/spanner/docs/limits`) on the unwrapped API error.

- **Type**: STRING enum
- **Default**: `TRANSACTIONAL`
- **Access**: Read/Write (SET, SET LOCAL, RESET ALL)
- **Notes**:
  - Fallback is non-atomic. Success reports a lower-bound row count and
    `(non-atomic mutation-limit fallback)` in the status line. Failure
    preserves both causes and warns that partitioned DML may have partially
    committed; that phase is not rolled back.
  - INSERT, `THEN RETURN`, explicit/pending/RO/SAVEPOINT owners, manual or
    automatic batches, and EXPLAIN/analysis paths are not retried.
  - Commit-phase failures, `ResourceExhausted`, `Aborted`, cancellations,
    deadlines, and the weaker Java `Transaction resource limits exceeded`
    branch are not retried.
  - SQL, bound parameters, priority, request tag, and optimizer settings are
    frozen before the implicit owner is created. Transaction tags and
    `LastStatement` are not sent on the partitioned attempt.
  - One caller/`STATEMENT_TIMEOUT` budget covers both phases. The logical
    owner's absolute `TRANSACTION_TIMEOUT` deadline is reused; exhausted
    budgets skip partitioned DML. The 24h partitioned-DML default timeout is
    not re-applied.
  - This is not a Java-complete or stable driver-parity claim. See
    [spanner-driver-compatibility.md](spanner-driver-compatibility.md) and
    #964.

### DEFAULT_SEQUENCE_KIND

Opt-in repair for SYNC DDL that failed because the database has no default
sequence kind. This is not a server session option; it authorizes one extra
database-wide `ALTER DATABASE`.

- **Type**: STRING (`NULL` / empty, or `bit_reversed_positive`)
- **Default**: `NULL` (disabled)
- **Access**: Read/Write, including `SET LOCAL`
- **Behavior**:
  - Empty/`NULL` performs no extra DDL.
  - `bit_reversed_positive` is the only accepted non-empty value.
  - Repair runs only for `DDL_EXECUTION_MODE=SYNC` after `InvalidArgument`
    plus the complete missing-default-sequence-kind sentence used by the
    pinned Go/Java drivers.
  - After a matching failure, mycli submits one dialect-correct
    `ALTER DATABASE` (GoogleSQL `SET OPTIONS (default_sequence_kind = …)` or
    PostgreSQL `SET spanner.default_sequence_kind = …`) and, only if that
    ALTER succeeds, retries the unfinished suffix proven by matching terminal
    `UpdateDatabaseDdl` metadata. Any ALTER create/wait error stops the
    attempt.
  - The extra mutation requires the existing Spanner DDL update permission
    (`spanner.databases.update`). See
    [primary-key defaults](https://docs.cloud.google.com/spanner/docs/primary-key-default-value#serial-auto-increment).
  - `ASYNC`, `ASYNC_WAIT`, and `SHOW OPERATION` never repair.

### AUTO_BATCH_DML_UPDATE_COUNT

Expected update count captured for each newly buffered automatic DML
statement. This is an expectation only; buffered commands never report it as
observed affected rows.

- **Type**: INT64 (nonnegative)
- **Default**: `1`
- **Access**: Read/Write (SET, SET LOCAL, RESET ALL)
- **Notes**:
  - Zero is a valid explicit expectation. Negative values are rejected.
  - SET, SET LOCAL, RESET, and transaction-end restoration change the policy
    for future enqueues only. Already queued statements keep the value
    captured at enqueue.
  - A matching flush reports the actual server counts through the existing
    batch result path.

### AUTO_BATCH_DML_UPDATE_COUNT_VERIFICATION

Whether flush compares each captured expected count with the actual
BatchUpdate count before accepting a successful automatic DML receipt.

- **Type**: BOOL
- **Default**: `FALSE`
- **Access**: Read/Write (SET, SET LOCAL, RESET ALL)
- **Notes**:
  - The default is off so existing arbitrary UPDATE/DELETE statements keep
    succeeding until verification is enabled. This differs from
    go-sql-spanner, which defaults verification on for ORM provisional counts.
  - Comparison is per statement. Expected `[1,1]` with actual `[0,2]` fails
    at statement 1 even though the totals match.
  - A disabled entry is not checked even if a later SET enables verification.
  - RPC or partial BatchUpdate failures keep their original cause; missing
    counts are not fabricated.
  - A mismatch uses the existing SAVEPOINT statement-failure/recovery
    boundary. A following COMMIT cannot commit the mismatched attempt.
  - After a successful journaled flush, replay still validates the recorded
    actual counts even if this flag is later set to FALSE.
  - Manual batches, THEN RETURN/row-producing DML, ordinary unbuffered DML,
    and partitioned DML are not affected.

### CLI_SAVEPOINT_SUPPORT

Client-emulated SAVEPOINT for explicit transactions. `DISABLED` (default) leaves
behavior unchanged. `ENABLED` records a journal from `BEGIN` and reconstructs
read-write prefixes on `ROLLBACK TO`. This is replay with result validation, not
a native Spanner savepoint. SET is rejected while a transaction is pending or
active and while a manual batch is open; SET LOCAL is not supported.

See [savepoint.md](savepoint.md) for syntax, recovery, and volatile-write
limits.

### CLI_SPANNER_METRICS_EXPORTER / CLI_SPANNER_METRICS_ENDPOINT

Startup-only caller-owned Spanner client metrics. These are not JDBC
`disable_native_metrics` and do not turn native Cloud Monitoring back on.

- **Type**: STRING / STRING
- **Default**: `off` / empty
- **Access**: Read-only after connect. SQL `SET`, `SET LOCAL`, and `RESET` are
  rejected. Flags/TOML: `--spanner-metrics-exporter=off|otlp` and
  `--spanner-metrics-endpoint`.
- **Description**: `off` constructs no exporter or MeterProvider and leaves any
  preexisting injected `ClientMetricsProvider` untouched. `otlp` starts one
  dedicated MeterProvider plus PeriodicReader before the first Spanner client
  and exports SDK `spanner/client/*` instruments over OTLP HTTP/protobuf to the
  explicit endpoint. The process owner is reused across USE/DETACH/reconnect/
  `RecreateClient` and is flushed/shut down once from `runWithOutput` with a
  single five-second budget.
- **Notes**:
  - Endpoint must be an absolute `http` or `https` URL with a host. Userinfo,
    query, and fragment are rejected. A missing or root path becomes
    `/v1/metrics`; any other path is kept. `http` is plaintext collector
    transport; `https` uses the collector's normal TLS.
  - `off` plus a nonempty endpoint, and `otlp` without an endpoint, are
    rejected before an exporter or client is constructed.
  - `OTEL_*` environment variables alone do not initialize export or choose a
    destination. After explicit `otlp` opt-in, other standard OTLP HTTP
    exporter settings (timeouts, headers, certificate files) may still apply;
    destination, path, and scheme come only from the CLI URL.
  - The collector does not reuse the Spanner endpoint, Google credentials, or
    custom Spanner certificate options. A global MeterProvider is not
    installed.
  - `SPANNER_EMULATOR_HOST` still suppresses SDK caller-owned client metrics.
    Native Cloud Monitoring stays disabled (`DisableNativeMetrics: true`).
  - There is no `SHOW METRICS` snapshot. Client traces are independently
    opt-in via `CLI_SPANNER_TRACES_*` (#967) and are not enabled by these
    variables.

### CLI_SPANNER_TRACES_EXPORTER / CLI_SPANNER_TRACES_ENDPOINT / CLI_SPANNER_TRACES_SAMPLE_RATIO

Startup-only process-owned Spanner client traces. This is not a private
per-client tracer API: the pinned Go SDK records through the process-global
`TracerProvider`.

- **Type**: STRING / STRING / FLOAT64
- **Default**: `off` / empty / `0.01`
- **Access**: Read-only after connect. SQL `SET`, `SET LOCAL`, and `RESET` are
  rejected. Flags/TOML: `--spanner-traces-exporter=off|otlp`,
  `--spanner-traces-endpoint`, and `--spanner-traces-sample-ratio`.
- **Description**: `off` constructs no exporter or TracerProvider and does not
  call `SetTracerProvider` or mutate environment variables. `otlp` starts one
  official OTLP HTTP/protobuf exporter (`otlptracehttp` v1.44.0) plus a
  CLI-owned `TracerProvider` before the first owned Spanner client. Sampling
  is `ParentBased(TraceIDRatioBased(ratio))`: a sampled parent is honored, an
  unsampled parent drops the child, and ratio `0` never samples roots. The
  owner is reused across USE/DETACH/reconnect/`RecreateClient` and is flushed
  with caller-owned metrics under one fresh five-second budget.
- **Notes**:
  - Endpoint rules match metrics except the default path is `/v1/traces`.
    `off` plus a nonempty endpoint, and `otlp` without an endpoint, are
    rejected. Ratio must be in `[0,1]` when export is `otlp`.
  - `OTEL_*` environment variables alone do not initialize export. After
    explicit `otlp` opt-in, destination/path/scheme come only from the CLI
    URL. A global MeterProvider is never installed.
  - When enabled, `EnableEndToEndTracing` is set only on a copied client
    config. That requests `x-goog-spanner-end-to-end-tracing: true`. It is
    not evidence of managed server spans. The emulator cannot establish
    server-trace behavior. `SPANNER_ENABLE_END_TO_END_TRACING` may still be
    honored by the SDK when CLI traces are off.
  - Privacy: only the pinned `cloud.google.com/go/spanner` v1.95.0
    instrumentation scope and known `startSpan` names are exported
    (`NewClient`, `CreateSession`, `Read`, `Query`, `Update`, `BatchUpdate`,
    `RowIterator`, `PartitionedUpdate`, `ReadWriteTransaction`,
    `ReadWriteTransactionWithOptions`, `Apply`, `BatchWrite`,
    `BatchWriteResponseIterator`). Resource/scope metadata are replaced with
    fixed values. Attributes, events, links, TraceState, status descriptions,
    and request/transaction tags are dropped. Span/trace/parent IDs, timing,
    and status code are kept. Review the allowlist on every go-spanner
    upgrade.
  - A second simultaneous CLI traces owner is rejected. A later external
    global provider is not overwritten on shutdown. OTel v1.44.0's initial
    proxy delegates irreversibly on the first `SetTracerProvider`; shutdown
    substitutes a no-op provider for that pinned proxy and does not restore
    previous global recording semantics. Tracers obtained from the proxy
    before CLI setup cannot be magically restored. That composition is
    unsupported.
  - Multiplexed `CreateSession` is started from `context.Background()` in the
    pinned SDK, so it is a root under the sample ratio rather than a child of
    the user statement.
  - There is no `SHOW TRACE` command.

Variables not covered in this section are described by the generated
[reference table](#reference) above.
