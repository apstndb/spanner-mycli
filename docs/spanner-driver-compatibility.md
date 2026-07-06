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
| `auto_batch_dml_update_count` / `auto_batch_dml_update_count_verification` | yes (v1.11.0) | yes | not implemented, tracked #401 |
| `ddl_execution_mode` (`SYNC`/`ASYNC`/`ASYNC_WAIT`) + `ddl_async_wait_timeout` | yes (v1.24.0) | n/a | `CLI_ASYNC_DDL` (bool) approximates; enum rename tracked #485 |
| `directed_read` | yes (v1.26.0) | Connection API Directed Read since the 6.52.x era | `CLI_DIRECT_READ` (read-only; lives outside the registry, complex proto type); rename + read/write support tracked #486 (varDef series #725 PR3b) |
| `transaction_timeout` | yes (v1.22.0) | v6.101.0 | not implemented, tracked #482 |
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
| `proto_descriptors` / `proto_descriptors_file_path` | via properties | java-spanner properties | `CLI_PROTO_DESCRIPTOR_FILE` implemented; rename tracked #487, in-memory descriptor var tracked #295 |
| `ddlInTransactionMode` | — | java-spanner property | not implemented, tracked #402 |
| Inactive-transaction action | — | java-spanner property | not implemented, tracked #403 |
| Statement-scoped connection state (`SET LOCAL`-style) | yes (v1.22.0) | JDBC `SET LOCAL` | implemented (see `SET LOCAL` below, #691) |

## Client-side statements

| Statement | go-sql-spanner | java-spanner | spanner-mycli status |
|-----------|----------------|--------------|----------------------|
| `SET LOCAL <name> = <value>` | statement-scoped state (v1.22.0) | JDBC `SET LOCAL` | implemented (#691) |
| `RESET ALL` | `RESET <property>` exists | JDBC `RESET ALL` | not implemented, tracked #484 (varDef series #725 PR5) |
| `RESET <single property>` | yes | yes | not implemented (candidate gap) |
| `SAVEPOINT` / `RELEASE` / `ROLLBACK TO` | not in the go driver | java-spanner Connection API since 2023 | not implemented, tracked #404 |
| `SHOW TRANSACTION ISOLATION LEVEL` / `SHOW TRANSACTION <var>` | yes (v1.26.0) | `SHOW DEFAULT_TRANSACTION_ISOLATION` v6.106.0 | not implemented (candidate gap; both drivers converged) |
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

- `SHOW TRANSACTION` statement family (isolation level and other transaction
  variables) — both reference drivers converged on it.
- `default_sequence_kind` (with auto-set on DDL failure) — both reference drivers
  converged on it.
- `RESET <single property>` (the non-`ALL` form).
- `max_partitions` connection property.
- Case-insensitive query parameter matching.
- `transaction_isolation` PG alias for `isolation_level`.

## Intentionally not tracked / out of scope

- go-sql-spanner statement-cache knobs — implementation detail of the
  `database/sql` driver, not a spanner-mycli concern.
- DSN-level connection concerns such as `connect_timeout` — spanner-mycli manages
  its own connection lifecycle rather than exposing a DSN.
- `begin_transaction_option` — driver-internal transaction bootstrapping detail.
