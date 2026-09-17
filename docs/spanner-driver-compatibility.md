# Spanner driver compatibility matrix

## Purpose

spanner-mycli borrows its connection-level configuration surface (system
variables and client-side statements) from the official Cloud Spanner drivers.
This document tracks which driver-defined connection properties and client-side
statements spanner-mycli supports, and which deltas are tracked as work items or
intentionally skipped.

The authoritative list of supported drivers per SQL dialect is the
[Cloud Spanner drivers overview](https://docs.cloud.google.com/spanner/docs/drivers-overview).
For spanner-mycli the two reference drivers are:

- **Spanner JDBC** — the `java-spanner` Connection API. `java-spanner` now lives
  in the [`googleapis/google-cloud-java`](https://github.com/googleapis/google-cloud-java)
  monorepo; changelogs are at `java-spanner/CHANGELOG.md` and
  `java-spanner-jdbc/CHANGELOG.md`.
- **go-sql-spanner** — the [`googleapis/go-sql-spanner`](https://github.com/googleapis/go-sql-spanner)
  `database/sql` driver.

Version numbers below refer to `go-sql-spanner` releases unless the column header
says otherwise; `java-spanner` versions are given where known.

## Maintenance policy

- This matrix is maintained through pull requests. When a reference driver ships
  a connection property or client-side statement relevant to spanner-mycli,
  update the matrix in the same spirit as a code change (PR-reviewable).
- The spanner-mycli column is currently hand-verified against
  `internal/mycli/var_defs.go` and `internal/mycli/client_side_statement_def.go`.
  It may later be generated from the varDef registry (see #725 PR6) so the column
  cannot drift from the code.
- Work items are tracked as sub-issues of umbrella issue #47, which is slimmed to
  work-tracking only. Issue references such as #482 auto-link on GitHub.

## Connection properties

| Property | go-sql-spanner (version) | java-spanner (version) | spanner-mycli status |
|----------|--------------------------|------------------------|----------------------|
| `retry_aborts_internally` | yes (pre-v1.0; default `true`) | yes; `SET LOCAL retry_aborts_internally` support in v6.83.0 (2024-12-13) | [RETRY_ABORTS_INTERNALLY](savepoint.md#explicit-aborted-retry) supports implicit and explicit read-write ABORTED retry. Opt-in default `FALSE` differs from Java/Go `TRUE`; maximum 50 physical attempts per logical owner, within the original timeout budget. Client-emulated replay has result-validation and visibility limits; it is not a stable driver contract. |
| `autocommit` | no explicit property (`database/sql` native) | JDBC `autocommit` | [AUTOCOMMIT](system_variables.md#autocommit) implemented (#83), default `TRUE`. `FALSE` starts transactions lazily; changing back to `TRUE`, EOF, EXIT, and Close never commit unfinished work. `SET LOCAL` is not supported. |
| `autocommit_dml_mode` (`Transactional`/`PartitionedNonAtomic`/`TransactionalWithFallbackToPartitionedNonAtomic`) | yes (two modes) | yes (three modes) | [AUTOCOMMIT_DML_MODE](system_variables.md#autocommit_dml_mode) supports three modes (#964). Fallback is opt-in and narrower than Java: only eligible implicit UPDATE/DELETE SQL-phase mutation-limit failures; not the weaker resource-limit classifier or Commit failures. Not a stable driver-parity claim. |
| `auto_batch_dml` | yes | yes | `AUTO_BATCH_DML` implemented |
| `auto_batch_dml_update_count` / `auto_batch_dml_update_count_verification` | yes (v1.11.0) | yes | [Update-count policy](system_variables.md#auto_batch_dml_update_count) implemented. Expected count defaults to 1; verification defaults `FALSE`, unlike go-sql-spanner. Checks each enabled queued entry and reports actual server counts; manual and partitioned DML are unchanged. |
| `ddl_execution_mode` (`SYNC`/`ASYNC`/`ASYNC_WAIT`) + `ddl_async_wait_timeout` | yes (v1.24.0) | n/a | [DDL_EXECUTION_MODE](system_variables.md#ddl_execution_mode) and [DDL_ASYNC_WAIT_TIMEOUT](system_variables.md#ddl_async_wait_timeout) implemented. Defaults `SYNC` and 10s; `--async` selects `ASYNC`. Wait expiry hands off the operation ID successfully; cancellation and failed operations remain errors. Replaces `CLI_ASYNC_DDL` (#485). |
| `directed_read` | yes (v1.26.0) | Connection API Directed Read since the 6.52.x era | `DIRECTED_READ` (session SET/SHOW, location[:READ_ONLY\|READ_WRITE] shorthand plus DirectedReadOptions protobuf JSON, empty clears). SHOW uses shorthand when lossless. SET rejected while a transaction is pending or active. Not applied to RW/DML/heartbeat/PDML. |
| `transaction_timeout` | yes (v1.22.0) | v6.101.0 | [TRANSACTION_TIMEOUT](system_variables.md#transaction_timeout) implemented. Default `NULL` adds no logical deadline. One budget starts at the first database RPC and survives SAVEPOINT reconstruction and ABORTED retry. Distinct from statement and user-idle timeouts. |
| `statement_timeout` | yes (v1.22.0) | connection URL support v6.102.0 | `STATEMENT_TIMEOUT` implemented |
| `read_lock_mode` (`PESSIMISTIC`/`OPTIMISTIC`) | yes (v1.18.0) | v6.100.0 | `READ_LOCK_MODE` implemented |
| `exclude_txn_from_change_streams` | yes (v1.4.0) | yes | `EXCLUDE_TXN_FROM_CHANGE_STREAMS` implemented |
| `isolation_level` (PG alias `transaction_isolation`, alias since v1.26.0) | yes | `default_isolation_level` v6.90.0; per-txn isolation; PG isolation statements | `DEFAULT_ISOLATION_LEVEL` implemented (alias not implemented — see remaining tracked gaps) |
| `auto_partition_mode` | yes | yes | `AUTO_PARTITION_MODE` implemented |
| `data_boost_enabled` | yes | yes | `DATA_BOOST_ENABLED` implemented |
| `max_partitioned_parallelism` | yes | yes | `MAX_PARTITIONED_PARALLELISM` implemented |
| `max_partitions` | yes | yes | `MAX_PARTITIONS` implemented (#957). Nonnegative INT64; default 0 means no hint and preserves existing service behavior. Applied once at the shared generation path for PARTITION, TRY PARTITIONED QUERY, RUN PARTITIONED QUERY, and AUTO_PARTITION_MODE. Not applied to RUN PARTITION token execution. Independent of `MAX_PARTITIONED_PARALLELISM`; returned partitions are never truncated. The official PartitionOptions API currently ignores this hint; the advertised service default is 10,000 and the advertised maximum is 200,000, but the actual returned count may be smaller or larger. The client does not cap at 200,000. A fake-RPC harness forwards requested 0/1/10/10000/200000 on the wire. Emulator 1.5.56 returned 2 partitions for a 20-row query at every tested setting, including requested 1; that is not managed-service proof. Managed Spanner remains unverified. |
| `default_sequence_kind` + auto-set on DDL failure | yes (v1.26.0) | JDBC/PGAdapter auto-set v6.88.0; `CREATE SEQUENCE` v6.102.0 | [DEFAULT_SEQUENCE_KIND](system_variables.md#default_sequence_kind) implemented, disabled by default. Only `bit_reversed_positive`; repair is SYNC-only, for a narrowly classified missing-kind error and metadata-proven unfinished suffix. |
| `max_commit_delay` | yes | yes | `MAX_COMMIT_DELAY` implemented |
| `commit_priority` (`HIGH`/`MEDIUM`/`LOW`/`UNSPECIFIED`) | yes | n/a | [COMMIT_PRIORITY](system_variables.md#commit_priority) implemented. Default `UNSPECIFIED` inherits transaction RPC priority; go-sql-spanner does not inherit `RPC_PRIORITY`. Frozen across physical attempts; no `SET LOCAL`. |
| `keep_transaction_alive` | n/a | yes (`KEEP_TRANSACTION_ALIVE`, default `false`) | [KEEP_TRANSACTION_ALIVE](system_variables.md#keep_transaction_alive) implemented. Default `TRUE` intentionally differs from Java `false`. Explicit RW heartbeats are independent of transaction and user-idle timeouts; no `SET LOCAL`. |
| `proto_descriptors` / `proto_descriptors_file_path` | via properties | java-spanner properties | `PROTO_DESCRIPTORS` (inline base64 graph) and `PROTO_DESCRIPTORS_FILE_PATH` (SET/SHOW plus ADD, source compilation and HTTP(S) extensions) implemented; session-persistent graph, not full Java lifecycle parity. Neither supports SET LOCAL. |
| `ca_cert_file` / `client_cert_file` / `client_cert_key` | yes (experimental/Omni host) | client-certificate / key connection properties | Startup-only `--ca-cert-file` / `--client-cert-file` / `--client-cert-key` (`CLI_CA_CERT_FILE`, `CLI_CLIENT_CERT_FILE`, `CLI_CLIENT_CERT_KEY`). Requires an explicit `--endpoint` or `--host`. Custom CA replaces system roots. Client cert and key are a mandatory pair. Transport is `omni.ConnectionOptions` applied to data, database admin, instance admin, USE/DETACH, and RecreateClient. Not SET-able. Does not set `ClientConfig.Type=OMNI` or Omni username/password. `--without-authentication` (`CLI_WITHOUT_AUTHENTICATION`) is an explicit opt-out of Google bearer credentials for the Spanner endpoint only; it is not inferred from TLS files, is not plaintext, and does not apply to feature AuthOptions (BigQuery/Gemini). |
| `disable_native_metrics` / native Cloud Monitoring | yes (`disable_native_metrics`) | yes | mycli keeps native Cloud Monitoring disabled (`DisableNativeMetrics: true`) and does not expose the JDBC property name. Opt-in caller-owned SDK `spanner/client/*` metrics use startup-only `--spanner-metrics-exporter=off\|otlp` (default `off`) and `--spanner-metrics-endpoint` (`CLI_SPANNER_METRICS_EXPORTER` / `CLI_SPANNER_METRICS_ENDPOINT`, #663). One OTLP HTTP/protobuf sink, no global MeterProvider, reused across USE/DETACH/`RecreateClient`. `OTEL_*` env vars alone do not enable export. `SPANNER_EMULATOR_HOST` suppresses SDK caller metrics. No `SHOW METRICS` or legacy pool statistics. Client traces are independently opt-in (#967). |
| `enableEndToEndTracing` / OpenTelemetry client traces | yes (`enableEndToEndTracing`) | yes (`enableEndToEndTracing`) | Default off. Startup-only `--spanner-traces-exporter=off\|otlp`, `--spanner-traces-endpoint`, `--spanner-traces-sample-ratio` in `[0,1]` (default `0.01`, ParentBased). `CLI_SPANNER_TRACES_*` are not SET-able. Opt-in installs one process-owned global TracerProvider and official `otlptracehttp` v1.44.0; a small allowlist/privacy wrapper keeps pinned go-spanner v1.95.0 operation names plus IDs/timing/status code and drops SQL/params/rows/errors/tags/unknown names. Off mode does not Set the global provider or mutate `OTEL_*` / `SPANNER_ENABLE_END_TO_END_TRACING`. The SDK may still honor that env var independently. Requested `x-goog-spanner-end-to-end-tracing: true` is a client header only. This is not a managed-server-span proof, and the emulator cannot establish server-trace behavior. Previously acquired tracers from OTel's initial proxy cannot be restored after the first Set. |
| `ddlInTransactionMode` | — | java-spanner property (`FAIL` / `ALLOW_IN_EMPTY_TRANSACTION` / `AUTO_COMMIT_TRANSACTION`) | [CLI_DDL_IN_TRANSACTION_MODE](system_variables.md#cli_ddl_in_transaction_mode) implemented (#402). Default `FAIL` differs from Java `ALLOW_IN_EMPTY_TRANSACTION`. Opt-in auto-commit can commit RW work before non-transactional DDL; later DDL failure cannot undo it. RO, manual DML batches, and SAVEPOINT recovery remain restricted. EOF/EXIT/Close never auto-commit. |
| Inactive-transaction action | — | java-spanner property | not implemented, tracked #403. Do not share an implementation with CLI-owned `CLI_IDLE_TRANSACTION_TIMEOUT` (#357). |
| `CLI_IDLE_TRANSACTION_TIMEOUT` | n/a (CLI-owned) | n/a (not a JDBC property) | [CLI_IDLE_TRANSACTION_TIMEOUT](system_variables.md#cli_idle_transaction_timeout) implemented (#357). CLI-owned sliding user-idle interval, default disabled (`NULL`/0), not an RPC deadline or Java inactive-transaction policy. Heartbeats and inspection do not reset it. |
| Statement-scoped connection state (`SET LOCAL`-style) | yes (v1.22.0) | JDBC `SET LOCAL` | implemented (see `SET LOCAL` below, #691) |

## Client-side statements

| Statement | go-sql-spanner | java-spanner | spanner-mycli status |
|-----------|----------------|--------------|----------------------|
| `SET LOCAL <name> = <value>` | statement-scoped state (v1.22.0) | JDBC `SET LOCAL` | implemented (#691) |
| Named query parameters (`SET PARAM` / `--param`) | case-insensitive name matching on bind | JDBC `PreparedStatement` names | case-insensitive logical identity (#958). Sequential `SET PARAM` updates one binding and keeps the first stored spelling. Binding uses the first SQL occurrence's spelling without rewriting SQL; Spanner matches later case variants in the same statement. `--param` is a `map[string]string` (no retained order for differently cased keys); conflicting case aliases are rejected. Identical aliases (same kind and memefish `SQL()` rendering) collapse to one stored spelling (lexicographically first) before `SHOW PARAMS`. |
| `RESET ALL` | `RESET <property>` exists | JDBC `RESET ALL` | [RESET rules](system_variables.md#stored-values-snapshots-and-effective-behavior) implemented (#484). Restores the startup snapshot before initialization commands, with exclusions for external resources and identity. Successful resets retire targeted LOCAL undo entries; rejection changes neither state nor undo. |
| `RESET <single property>` | yes | yes | [RESET rules](system_variables.md#stored-values-snapshots-and-effective-behavior) implemented (#960), scoped to one canonical name or alias. `RESET LOCAL` and `SET x=DEFAULT` are unsupported. |
| `SAVEPOINT` / `RELEASE` / `ROLLBACK TO` | not in the go driver | java-spanner Connection API since 2023 | `CLI_SAVEPOINT_SUPPORT` (`DISABLED` default, `ENABLED`). Client-emulated replay with result validation; not native Spanner savepoints. Deltas vs Java: disabled by default, synchronous reconstruction, no `FAIL_AFTER_ROLLBACK`, exact-case identifier names, 128 code-point CLI limit. See [docs/savepoint.md](savepoint.md). |
| `SHOW TRANSACTION ISOLATION LEVEL` / `SHOW TRANSACTION READ ONLY` | yes (v1.26.0) | `SHOW DEFAULT_TRANSACTION_ISOLATION` v6.106.0 | `SHOW TRANSACTION ISOLATION LEVEL` and `SHOW TRANSACTION READ ONLY` implemented (#959). Side-effect-free inspection of the current logical owner (pending, active RO/RW, SAVEPOINT recovery). Idle sessions report next-transaction `DEFAULT_ISOLATION_LEVEL` / `READONLY`. `UNSPECIFIED` isolation means the database default; it is not a guessed server isolation. PostgreSQL `DEFERRABLE` / `SHOW TRANSACTION <var>` aliases are tracked with #230. |
| `RUN PARTITIONED QUERY <select>` | yes (v1.24.0) | — | `RUN PARTITIONED QUERY` implemented |
| `RUN PARTITION '<token>'` | not at SQL level in the go driver | JDBC `RUN PARTITION '<token>'` | experimental native Go envelope (#45). Not JDBC/Java wire compatible. |

> Note on `RUN PARTITION '<token>'`: `PARTITION` now exports
> `smycli-part/1/<base64url(JSON)>` complete native tokens (pinned
> `cloud.google.com/go/spanner` v1.95.0 `MarshalBinary` payloads plus database
> and RFC3339Nano UTC timestamps). Older bare `GetPartitionToken` values are
> unsupported. Client-side validity is one hour with a one-minute
> forward-clock tolerance; this is not authentication or remote cleanup.
> `RUN PARTITION` requires an idle session (no live logical owner or manual
> batch). A decoder-only inspector of the pinned gob/protobuf layout checks
> native consistency before SDK Unmarshal/Execute. `Cleanup`/`Close` in
> v1.95.0 are local and do not `DeleteSession`. Managed-service retention and
> cross-principal behavior are unverified. `RUN PARTITIONED QUERY` is unchanged.

## Remaining tracked gaps

PostgreSQL-only aliases remain tracked with #230:

- `SHOW TRANSACTION` PostgreSQL-only aliases (`DEFERRABLE`, `transaction_isolation`,
  arbitrary `SHOW TRANSACTION <var>`) — isolation level and read-only inspection
  landed in #959; remaining aliases are tracked with #230.
- `transaction_isolation` PG alias for `isolation_level`.

## Intentionally not tracked / out of scope

- go-sql-spanner statement-cache knobs — implementation detail of the
  `database/sql` driver, not a spanner-mycli concern.
- DSN-level connection concerns such as `connect_timeout` — spanner-mycli manages
  its own connection lifecycle rather than exposing a DSN.
- `begin_transaction_option` — driver-internal transaction bootstrapping detail.
