# Client-emulated SAVEPOINT

`CLI_SAVEPOINT_SUPPORT` enables a client-side SAVEPOINT for explicit
transactions. This is replay with validation of previously observed results, not
a native Cloud Spanner savepoint. Locks, transaction IDs, commit timestamps, and
unreturned writes are not preserved.

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
- Change the setting only while idle. `SET LOCAL` is not supported.
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
manual `START BATCH` is open.

## What is guaranteed

A successful `ROLLBACK TO` means the prefix before that marker has been
re-executed on a new physical read-write attempt and the newly observed results
matched the fingerprints recorded the first time: typed rows, end-of-stream,
DML affected-row counts, Batch DML count vectors, and returned DML rows.

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

## Recovery

If a statement fails after a completed marker in an enabled explicit RW
transaction, the physical attempt is discarded and the logical transaction
requires `ROLLBACK TO SAVEPOINT`. `COMMIT` and new SQL are rejected until then.
Failed reconstruction ends the logical transaction.

See also [system variables](system_variables.md) and the driver
[compatibility matrix](spanner-driver-compatibility.md).
