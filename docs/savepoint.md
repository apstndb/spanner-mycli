# Client-emulated SAVEPOINT

`CLI_SAVEPOINT_SUPPORT` enables a client-side SAVEPOINT for explicit
transactions. This is replay with validation of previously observed results, not
a native Cloud Spanner savepoint. Locks, transaction IDs, commit timestamps, and
unreturned writes are not preserved.

`ROLLBACK TO` is a **synchronous whole-prefix replay**. It rolls back the current
physical read-write handle, starts a new SDK transaction with the frozen constructor
options, and re-executes every journaled operation before the target marker on that
new attempt. Cost scales with prefix length (SQL, DML, Batch DML, mutations), not
with the number of later discarded statements. Failed reconstruction ends the logical
transaction.

The journal budget is **16 MiB of retained payload**, not process RSS. It counts
frozen SQL, parameters, query options, mutations, marker names, and the constant-sized
fingerprint. Observed result rows are hashed, not stored.

## Enablement

```sql
SET CLI_SAVEPOINT_SUPPORT = 'ENABLED';
BEGIN;
INSERT INTO T (id, value) VALUES (1, 'keep');
SAVEPOINT before_second_row;
INSERT INTO T (id, value) VALUES (2, 'discard');
ROLLBACK TO SAVEPOINT before_second_row;
COMMIT;
```

- Values: `DISABLED` (default), `ENABLED`.
- Change the setting only while idle **and no manual `START BATCH` is open**.
  `SET LOCAL` is not supported.
- Capture starts at `BEGIN`, not at the first `SAVEPOINT`.
- Implicit one-statement transactions are not journaled.

## Syntax

```text
SAVEPOINT name
ROLLBACK [TRANSACTION] TO [SAVEPOINT] name
RELEASE [SAVEPOINT] name
```

Names are one identifier (ordinary or backtick-quoted). Matching is exact-case
on the decoded name, limited to 128 Unicode code points. Duplicate names are
rejected. `ROLLBACK` and `CLOSE` remain full-transaction operations.

Savepoint commands require an explicit transaction and are rejected while a
manual `START BATCH` is open. Manual batch mode cannot be changed as a way to
bypass that guard: `START BATCH` / `ABORT BATCH` after recovery-required are
rejected, and `SET CLI_SAVEPOINT_SUPPORT` is rejected while a batch is open.

## Markers vs later work

`ROLLBACK TO name` keeps the named marker, drops later markers, truncates the
journal after that marker, and discards automatic queued DML that has not yet
become a journaled batch. The prefix before the marker is then replayed.

`RELEASE name` drops that marker and later markers. It does **not** truncate
journaled operations or discard automatic queued DML.

## What is guaranteed

A successful `ROLLBACK TO` means the prefix before that marker has been
re-executed on a new physical read-write attempt and the newly observed results
matched the fingerprints recorded the first time: typed rows, end-of-stream,
DML affected-row counts, Batch DML count vectors, and returned DML rows.

Fingerprints compare rows in arrival order. A query whose result order is not
stable can fail reconstruction even when the same rows are returned.

Read-only transactions keep marker names only. They do not replace the snapshot
handle or revive a failed RO transaction.

## What is not guaranteed

Result equality does not prove equality of hidden reads or unreturned writes.
Inserts that use generated UUIDs, sequences, or other volatile defaults can
affect one row again while producing a different stored value. Supply
generated values as fixed parameters. Do not treat a successful checksum comparison
as native savepoint equivalence.

Already displayed output is not undone. Replay produces no application output
and does not overwrite `LastResult` query cache or metrics.

Ordinary `SET` remains session-durable. `SET LOCAL` is scoped to the whole
logical transaction; `ROLLBACK TO` does not revert it. Full `COMMIT` / `ROLLBACK`
/ `CLOSE` restore it as today.

Formatter, pager, and other output-sink failures after a completed marker are not
checkpointable successes. The just-committed journal entry is retracted and the
transaction requires `ROLLBACK TO` when a marker exists.

## Recovery

If a statement fails after a completed marker in an enabled explicit RW
transaction, the physical attempt is discarded and the logical transaction
requires `ROLLBACK TO SAVEPOINT`. Until then, only `ROLLBACK TO`, full
`ROLLBACK`/`CLOSE`, and help / local inspection (`HELP`, `HELP VARIABLES`,
`SHOW VARIABLE`, `SHOW VARIABLES`, `SHOW PARAMS`) are admitted. `SET`,
`SET LOCAL`, `SET PARAM`, `START BATCH`, `ABORT BATCH`, `SAVEPOINT`, `RELEASE`,
`COMMIT`, and SQL are rejected without changing variables, batch state, or the
logical owner. Failed reconstruction ends the logical transaction.

An `Aborted` statement after a marker enters that same recovery path and does
not replace the Spanner client. The original abort error is preserved and
`ROLLBACK TO` reuses the same client and logical owner. Client recreation still
runs for terminal aborts: capture off, no completed marker, or after the owner
has already ended.

See also [system variables](system_variables.md) and the driver
[compatibility matrix](spanner-driver-compatibility.md).
