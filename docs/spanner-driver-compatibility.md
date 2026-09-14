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
| `autocommit` | no explicit property (`database/sql` native) | JDBC `autocommit` | `AUTOCOMMIT` placeholder (`UnimplementedVar`), tracked #83 |
| `autocommit_dml_mode` (`Transactional`/`PartitionedNonAtomic`) | yes | yes | `AUTOCOMMIT_DML_MODE` implemented |
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
| `default_sequence_kind` + auto-set on DDL failure | yes (v1.26.0) | JDBC/PGAdapter auto-set v6.88.0; `CREATE SEQUENCE` v6.102.0 | not implemented (candidate gap; both reference drivers converged) |
| `max_commit_delay` | yes | yes | `MAX_COMMIT_DELAY` implemented |
| `commit_priority` (`HIGH`/`MEDIUM`/`LOW`/`UNSPECIFIED`) | yes | n/a | `COMMIT_PRIORITY` implemented. Default `UNSPECIFIED` inherits the resolved transaction RPC priority (existing mycli behavior). go-sql-spanner's default `UNSPECIFIED` is the Go driver's `CommitPriority` default and does not inherit `RPC_PRIORITY`. The effective value is frozen in the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. `SET LOCAL` is not supported. Not applied to query, DML, heartbeat, partitioned DML, read-only, or Admin RPCs. |
| `keep_transaction_alive` | n/a | yes (`KEEP_TRANSACTION_ALIVE`, default `false`) | `KEEP_TRANSACTION_ALIVE` implemented. Default `TRUE` preserves existing mycli heartbeat after the first user SQL on an explicit read-write owner. Java defaults to `false`; this CLI default is intentionally `TRUE`. The policy is frozen on the logical owner with the constructor snapshot reused across physical attempts, including SAVEPOINT reconstruction. `SET LOCAL` is not supported. Idle-deadline (#357) is not implemented. `TRANSACTION_TIMEOUT` is a separate logical-owner budget and is not implied by this variable. |
| `proto_descriptors` / `proto_descriptors_file_path` | via properties | java-spanner properties | `PROTO_DESCRIPTORS` (inline base64 graph) and `PROTO_DESCRIPTORS_FILE_PATH` (SET/SHOW plus ADD, source compilation and HTTP(S) extensions) implemented; session-persistent graph, not full Java lifecycle parity. Neither supports SET LOCAL. |
| `ddlInTransactionMode` | — | java-spanner property | not implemented, tracked #402 |
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
| `RUN PARTITION '<token>'` | not at SQL level in the go driver | JDBC `RUN PARTITION '<token>'` | token form tracked #45 (see note below) |

> Note on `RUN PARTITION '<token>'`: a matching pattern and a
> `RunPartitionStatement` handler exist in
> `internal/mycli/client_side_statement_def.go`, but the statement's help/usage
> is commented out and annotated "This statement is currently unimplemented", so
> it is not exposed as a supported statement. Completing it is tracked in #45.

## Candidate gaps (no issue yet)

These are deltas that look like plausible additions but do not yet have a
tracking issue. They are listed here so the gap is not lost:

- `SHOW TRANSACTION` PostgreSQL-only aliases (`DEFERRABLE`, `transaction_isolation`,
  arbitrary `SHOW TRANSACTION <var>`) — isolation level and read-only inspection
  landed in #959; remaining aliases are tracked with #230.
- `default_sequence_kind` (with auto-set on DDL failure) — both reference drivers
  converged on it.
- `max_partitions` connection property.
- `transaction_isolation` PG alias for `isolation_level`.

## Intentionally not tracked / out of scope

- go-sql-spanner statement-cache knobs — implementation detail of the
  `database/sql` driver, not a spanner-mycli concern.
- DSN-level connection concerns such as `connect_timeout` — spanner-mycli manages
  its own connection lifecycle rather than exposing a DSN.
- `begin_transaction_option` — driver-internal transaction bootstrapping detail.
