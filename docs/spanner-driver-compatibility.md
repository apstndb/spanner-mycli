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
| `retry_aborts_internally` | yes (pre-v1.0; default `true`) | yes; `SET LOCAL retry_aborts_internally` support in v6.83.0 (2024-12-13) | `RETRY_ABORTS_INTERNALLY` placeholder (`UnimplementedVar`), tracked #293 |
| `autocommit` | no explicit property (`database/sql` native) | JDBC `autocommit` | `AUTOCOMMIT` implemented (#83). Default `TRUE` keeps single-use reads and implicit RW DML commits. `FALSE` lazily starts a pending logical owner at the next eligible database operation (ordinary SELECT/DML/MUTATE, PROFILE DML even during a manual batch, EXPLAIN ANALYZE, automatic DML enqueue, nonempty RUN BATCH DML, or first enabled SAVEPOINT) and stays idle after COMMIT/ROLLBACK. Same-value SET/RESET is idempotent even with an owner or manual batch; a real change is rejected with the existing transaction/batch setter errors. SET LOCAL is not supported. `false`→`true` never implicit-Commits. EXPLAIN PLAN, ordinary manual-batch DML enqueue, DDL (#402), PDML/TRUNCATE, partition, admin, and inspection commands do not create a new owner. EOF/EXIT/Close never commit unfinished work. Idle expiry (#357), abort retry (#293), and delayed first write (#966) stay separate. |
| `autocommit_dml_mode` (`Transactional`/`PartitionedNonAtomic`/`TransactionalWithFallbackToPartitionedNonAtomic`) | yes (two modes) | yes (three modes) | `AUTOCOMMIT_DML_MODE` implemented: `TRANSACTIONAL` (default), `PARTITIONED_NON_ATOMIC`, and opt-in `TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC`. The third value retries one eligible implicit UPDATE/DELETE as partitioned DML only after a SQL-phase mutation-limit failure that matches the pinned InvalidArgument + exact mutation-limit sentence + Cloud Spanner limits Help classifier (#964). It is not Java-complete (no weaker `Transaction resource limits exceeded` branch) and is not a stable driver-parity claim. INSERT, THEN RETURN, explicit/pending/RO/SAVEPOINT owners, batches, EXPLAIN/analysis, and Commit-phase failures are not retried. |
| `auto_batch_dml` | yes | yes | `AUTO_BATCH_DML` implemented |
| `auto_batch_dml_update_count` / `auto_batch_dml_update_count_verification` | yes (v1.11.0) | yes | `AUTO_BATCH_DML_UPDATE_COUNT` (nonnegative INT64, default 1) and `AUTO_BATCH_DML_UPDATE_COUNT_VERIFICATION` (BOOL, default FALSE). Policy is frozen per automatic queue entry at enqueue. A successful BatchUpdate compares each enabled entry's actual count before a journal receipt; aggregate equality is not enough. CLI verification defaults off so existing arbitrary UPDATE/DELETE statements keep succeeding (go-sql-spanner defaults verification on). A matching flush reports actual server counts; the expected count is never presented as observed affected rows. RPC/partial BatchUpdate failures keep their original cause. Replay still checks journaled actual counts when verification is later disabled. Manual, THEN RETURN, unbuffered, and partitioned DML are unchanged. |
| `ddl_execution_mode` (`SYNC`/`ASYNC`/`ASYNC_WAIT`) + `ddl_async_wait_timeout` | yes (v1.24.0) | n/a | `DDL_EXECUTION_MODE` (`SYNC` default / `ASYNC` / `ASYNC_WAIT`) + `DDL_ASYNC_WAIT_TIMEOUT` (default 10s). `--async` selects `ASYNC`. The remaining wait budget bounds in-flight GetOperation polls as well as the between-poll wait. Wait-budget expiry is a successful handoff of the still-running operation ID and cancels only the polling RPC, not the server operation; caller/statement cancellation remains an error with that ID; a completed failing LRO remains a failure. `CLI_ASYNC_DDL` was removed (#485). |
| `directed_read` | yes (v1.26.0) | Connection API Directed Read since the 6.52.x era | `DIRECTED_READ` (session SET/SHOW, location[:READ_ONLY\|READ_WRITE] shorthand plus DirectedReadOptions protobuf JSON, empty clears). SHOW uses shorthand when lossless. SET rejected while a transaction is pending or active. Not applied to RW/DML/heartbeat/PDML. |
| `transaction_timeout` | yes (v1.22.0) | v6.101.0 | `TRANSACTION_TIMEOUT` implemented. Duration or `NULL`; `NULL`/0 means no additional logical read/write deadline. The duration is captured for the logical owner and the single total budget starts at the first real database RPC (including constructor `BeginTransaction`), not client-only `BEGIN`/`SHOW` or DML buffering. The budget is preserved across physical reconstruction. Pending `SET LOCAL` may select the duration before the first RPC; changing it after activation is rejected. Session `SET` after `BEGIN` applies to a later owner. Distinct from `STATEMENT_TIMEOUT` and unimplemented user-idle expiry (#357). ABORTED retries (#293) are not implemented. |
| `statement_timeout` | yes (v1.22.0) | connection URL support v6.102.0 | `STATEMENT_TIMEOUT` implemented |
| `read_lock_mode` (`PESSIMISTIC`/`OPTIMISTIC`) | yes (v1.18.0) | v6.100.0 | `READ_LOCK_MODE` implemented |
| `exclude_txn_from_change_streams` | yes (v1.4.0) | yes | `EXCLUDE_TXN_FROM_CHANGE_STREAMS` implemented |
| `isolation_level` (PG alias `transaction_isolation`, alias since v1.26.0) | yes | `default_isolation_level` v6.90.0; per-txn isolation; PG isolation statements | `DEFAULT_ISOLATION_LEVEL` implemented (alias not implemented — see candidate gaps) |
| `auto_partition_mode` | yes | yes | `AUTO_PARTITION_MODE` implemented |
| `data_boost_enabled` | yes | yes | `DATA_BOOST_ENABLED` implemented |
| `max_partitioned_parallelism` | yes | yes | `MAX_PARTITIONED_PARALLELISM` implemented |
| `max_partitions` | yes | yes | not implemented (candidate gap) |
| `default_sequence_kind` + auto-set on DDL failure | yes (v1.26.0) | JDBC/PGAdapter auto-set v6.88.0; `CREATE SEQUENCE` v6.102.0 | `DEFAULT_SEQUENCE_KIND` (empty/NULL default = disabled; only `bit_reversed_positive` is accepted). SYNC-only: after `InvalidArgument` plus the pinned missing-kind sentence, one `ALTER DATABASE` sets the database option, then only a metadata-proven unfinished suffix is retried. Any ALTER failure stops without a suffix retry. ASYNC, ASYNC_WAIT, and SHOW OPERATION do not repair. |
| `max_commit_delay` | yes | yes | `MAX_COMMIT_DELAY` implemented |
| `commit_priority` (`HIGH`/`MEDIUM`/`LOW`/`UNSPECIFIED`) | yes | n/a | `COMMIT_PRIORITY` implemented. Default `UNSPECIFIED` inherits the resolved transaction RPC priority (existing mycli behavior). go-sql-spanner's default `UNSPECIFIED` is the Go driver's `CommitPriority` default and does not inherit `RPC_PRIORITY`. The effective value is frozen in the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. `SET LOCAL` is not supported. Not applied to query, DML, heartbeat, partitioned DML, read-only, or Admin RPCs. |
| `keep_transaction_alive` | n/a | yes (`KEEP_TRANSACTION_ALIVE`, default `false`) | `KEEP_TRANSACTION_ALIVE` implemented. Default `TRUE` preserves existing mycli heartbeat after the first user SQL on an explicit read-write owner. Java defaults to `false`; this CLI default is intentionally `TRUE`. The policy is frozen on the logical owner with the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. `SET LOCAL` is not supported. Idle-deadline (#357) is not implemented. `TRANSACTION_TIMEOUT` is a separate logical-owner budget and is not implied by this variable. |
| `proto_descriptors` / `proto_descriptors_file_path` | via properties | java-spanner properties | `PROTO_DESCRIPTORS` (inline base64 graph) and `PROTO_DESCRIPTORS_FILE_PATH` (SET/SHOW plus ADD, source compilation and HTTP(S) extensions) implemented; session-persistent graph, not full Java lifecycle parity. Neither supports SET LOCAL. |
| `ca_cert_file` / `client_cert_file` / `client_cert_key` | yes (experimental/Omni host) | client-certificate / key connection properties | Startup-only `--ca-cert-file` / `--client-cert-file` / `--client-cert-key` (`CLI_CA_CERT_FILE`, `CLI_CLIENT_CERT_FILE`, `CLI_CLIENT_CERT_KEY`). Requires an explicit `--endpoint` or `--host`. Custom CA replaces system roots. Client cert and key are a mandatory pair. Transport is `omni.ConnectionOptions` applied to data, database admin, instance admin, USE/DETACH, and RecreateClient. Not SET-able. Does not set `ClientConfig.Type=OMNI` or Omni username/password. `--without-authentication` (`CLI_WITHOUT_AUTHENTICATION`) is an explicit opt-out of Google bearer credentials for the Spanner endpoint only; it is not inferred from TLS files, is not plaintext, and does not apply to feature AuthOptions (BigQuery/Gemini). |
| `disable_native_metrics` / native Cloud Monitoring | yes (`disable_native_metrics`) | yes | mycli keeps native Cloud Monitoring disabled (`DisableNativeMetrics: true`) and does not expose the JDBC property name. Opt-in caller-owned SDK `spanner/client/*` metrics use startup-only `--spanner-metrics-exporter=off\|otlp` (default `off`) and `--spanner-metrics-endpoint` (`CLI_SPANNER_METRICS_EXPORTER` / `CLI_SPANNER_METRICS_ENDPOINT`, #663). One OTLP HTTP/protobuf sink, no global MeterProvider, reused across USE/DETACH/`RecreateClient`. `OTEL_*` env vars alone do not enable export. `SPANNER_EMULATOR_HOST` suppresses SDK caller metrics. No `SHOW METRICS` or legacy pool statistics. Client traces are independently opt-in (#967). |
| `enableEndToEndTracing` / OpenTelemetry client traces | yes (`enableEndToEndTracing`) | yes (`enableEndToEndTracing`) | Default off. Startup-only `--spanner-traces-exporter=off\|otlp`, `--spanner-traces-endpoint`, `--spanner-traces-sample-ratio` in `[0,1]` (default `0.01`, ParentBased). `CLI_SPANNER_TRACES_*` are not SET-able. Opt-in installs one process-owned global TracerProvider and official `otlptracehttp` v1.44.0; a small allowlist/privacy wrapper keeps pinned go-spanner v1.95.0 operation names plus IDs/timing/status code and drops SQL/params/rows/errors/tags/unknown names. Off mode does not Set the global provider or mutate `OTEL_*` / `SPANNER_ENABLE_END_TO_END_TRACING`. The SDK may still honor that env var independently. Requested `x-goog-spanner-end-to-end-tracing: true` is a client header only. This is not a managed-server-span proof, and the emulator cannot establish server-trace behavior. Previously acquired tracers from OTel's initial proxy cannot be restored after the first Set. |
| `ddlInTransactionMode` | — | java-spanner property (`FAIL` / `ALLOW_IN_EMPTY_TRANSACTION` / `AUTO_COMMIT_TRANSACTION`) | `CLI_DDL_IN_TRANSACTION_MODE` implemented (#402). Default `FAIL` intentionally differs from Java `ALLOW_IN_EMPTY_TRANSACTION`. Policy is captured on the logical owner at creation, including pending `BEGIN`. Session `SET` after `BEGIN` applies to a later owner. `SET LOCAL` may change this owner only before user work. Empty pending is retired without an SDK constructor/`Commit`. Constructor-only empty RW is rolled back (`ALLOW`) or committed (`AUTO_COMMIT`) before DDL. Nonempty RW is rejected except `AUTO_COMMIT`, which flushes eligible automatic DML and `Commit`s first. Manual DML batch, RO, and SAVEPOINT recovery reject with zero Admin. Empty BulkDdl is a no-op and does not commit. `START BATCH DDL` is admitted before batch state changes; `RUN BATCH` validates descriptors and rechecks admission before `Commit`, then carries that preparation receipt through Admin. `CreateDatabase` is out of scope. EOF/EXIT/Close never auto-commit because of this variable. SYNC default-sequence repair is `DEFAULT_SEQUENCE_KIND` (#984), not this variable. |
| Inactive-transaction action | — | java-spanner property | not implemented, tracked #403 |
| Statement-scoped connection state (`SET LOCAL`-style) | yes (v1.22.0) | JDBC `SET LOCAL` | implemented (see `SET LOCAL` below, #691) |

## Client-side statements

| Statement | go-sql-spanner | java-spanner | spanner-mycli status |
|-----------|----------------|--------------|----------------------|
| `SET LOCAL <name> = <value>` | statement-scoped state (v1.22.0) | JDBC `SET LOCAL` | implemented (#691) |
| Named query parameters (`SET PARAM` / `--param`) | case-insensitive name matching on bind | JDBC `PreparedStatement` names | case-insensitive logical identity (#958). Sequential `SET PARAM` updates one binding and keeps the first stored spelling. Binding uses the first SQL occurrence's spelling without rewriting SQL; Spanner matches later case variants in the same statement. `--param` is a `map[string]string` (no retained order for differently cased keys); conflicting case aliases are rejected. Identical aliases (same kind and memefish `SQL()` rendering) collapse to one stored spelling (lexicographically first) before `SHOW PARAMS`. |
| `RESET ALL` | `RESET <property>` exists | JDBC `RESET ALL` | implemented (#484). Restores values captured after defaults/config/flags/--set, before `--init-command` / `--init-command-add`. File-backed descriptors/templates, opaque graphs, connection identity, stream/output handles, and unimplemented placeholders are excluded. Ordinary RESET is persistent like SET: after the whole operation succeeds it retires only the targeted LOCAL undo entries, including equal-value resets. A rejected RESET ALL changes neither values nor undo. |
| `RESET <single property>` | yes | yes | implemented (#960). Same startup snapshot, exclusions, guards, and LOCAL undo rules as RESET ALL, scoped to one canonical name or alias. RESET LOCAL and SET x=DEFAULT are not supported. |
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

## Candidate gaps (no issue yet)

These are deltas that look like plausible additions but do not yet have a
tracking issue. They are listed here so the gap is not lost:

- `SHOW TRANSACTION` PostgreSQL-only aliases (`DEFERRABLE`, `transaction_isolation`,
  arbitrary `SHOW TRANSACTION <var>`) — isolation level and read-only inspection
  landed in #959; remaining aliases are tracked with #230.
- `max_partitions` connection property.
- `transaction_isolation` PG alias for `isolation_level`.

## Intentionally not tracked / out of scope

- go-sql-spanner statement-cache knobs — implementation detail of the
  `database/sql` driver, not a spanner-mycli concern.
- DSN-level connection concerns such as `connect_timeout` — spanner-mycli manages
  its own connection lifecycle rather than exposing a DSN.
- `begin_transaction_option` — driver-internal transaction bootstrapping detail.
