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
- A retry-required explicit owner can share this same journal. That does
  not enable `SAVEPOINT` / `ROLLBACK TO` / `RELEASE` while
  `CLI_SAVEPOINT_SUPPORT` is `DISABLED`.

## Explicit ABORTED retry

`RETRY_ABORTS_INTERNALLY=TRUE` (opt-in, default `FALSE`; Java/Go drivers
default `TRUE`) lets an explicit read-write owner recover from a real
Spanner `ABORTED` by silently replaying the fully observed journal prefix
and retrying the current frozen operation. This is the same typed ordered
fingerprint comparison as `ROLLBACK TO`: row types/metadata, values,
order, count (including zero rows), DML affected-row counts, and Batch DML
count vectors. A changed replay result or reconstruction failure ends the
logical transaction.

The lifetime budget is **49 automatic physical reconstructions** (50
attempts including the original) per logical owner. Prefix-replay
`ABORTED` errors consume that budget. Successful statements and manual
`ROLLBACK TO` do not reset it. Reconstruction reuses the original caller
cancellation and `TRANSACTION_TIMEOUT` budget and is not new user-idle
activity.

Bytes already delivered by the **current** failed operation, including
headers, prohibit automatic re-execution of that operation.
Buffered/unpublished current results may be discarded and retried. A
writer failure is neither a successful receipt nor a retryable database
failure. Prior fully completed output is replayed silently and must not
appear twice.

Automatic full-prefix retry is attempted before SAVEPOINT
recovery-required. Successful retry keeps markers and `SET LOCAL` undo.
Partial current output or an exhausted attempt budget fall back to the
existing valid-marker recovery behavior; without a marker the owner is
retired. Manual `ROLLBACK TO` never silently substitutes an earlier
marker for full-prefix retry.

`SET LOCAL RETRY_ABORTS_INTERNALLY` is allowed only on an unused pending
read-write owner, before the first RPC, queued work, or SAVEPOINT marker.
Direct `BEGIN RW` and implicit RW have no such window.

Limitations are the same as SAVEPOINT replay: hidden reads and unreturned
volatile writes (generated UUIDs, sequences, defaults) are not preserved.
PLAN-only operations, PDML, read-only transactions, and non-`ABORTED`
Commit failures are not retried.

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

When `RETRY_ABORTS_INTERNALLY` is enabled and the current operation has
not already delivered output, a real `ABORTED` tries full-prefix
automatic recovery before this recovery-required state. If retry is
forbidden by partial current output or the attempt budget is exhausted,
the existing valid-marker recovery behavior remains: the physical attempt
is discarded, the original abort error is preserved, and `ROLLBACK TO`
reuses the same client and logical owner. Client recreation still runs
for terminal aborts: capture off, no completed marker, fingerprint
mismatch/reconstruction failure, cancellation, owner expiry, or after
the owner has already ended.

See also [system variables](system_variables.md) and the driver
[compatibility matrix](spanner-driver-compatibility.md).
