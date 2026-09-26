# Calling Spanner queue functions

Queue availability depends on the Spanner/Omni server build. This guide describes
existing SQL execution and output controls; it does not establish that every
Omni build supports queues.

## Receive messages without waiting for query completion

For an existing queue named `Tasks`, outside an active transaction:

```sql
SET CLI_FORMAT = 'JSONL';
SET READ_ONLY_STALENESS = 'STRONG';
SET STATEMENT_TIMEOUT = '1m';
SELECT * FROM RECEIVE_Tasks(max_duration => '30s');
```

`RECEIVE_` is a long-running streaming query. Strong reads are required.
`max_duration` limits the server's receive window, while `STATEMENT_TIMEOUT`
independently limits the client statement. Choose both deliberately: an unset
statement timeout still has a 10-minute execution fallback for ordinary queries.
Use another connection to send, renew, or acknowledge while this query runs.

JSONL, CSV, TAB, and VERTICAL emit rows as the SDK delivers them. TABLE buffers
results by default. To use a streaming table:

```sql
SET CLI_FORMAT = 'TABLE';
SET CLI_TABLE_STREAMING = 'TRUE';
SET CLI_TABLE_PREVIEW_ROWS = 1;
```

The default preview of 50 rows waits for 50 rows or query completion, even with
streaming enabled. A preview of 1 uses the first row for widths; 0 uses only
header widths. Later values can be wrapped to fit those widths. `-1` buffers all
rows. A downstream pager or pipe consumer can also delay visible output.

## Queue semantics

RECEIVE acquires message leases; use `SELECT * FROM Tasks` to inspect stored
messages without receiving them. Printing a row does not acknowledge it.
Use ordinary SQL to call `RENEWLEASE_<queue>` and acknowledge messages with
DML or native mutations.

For queue definitions, function arguments, lease tokens, acknowledgement,
and transaction error handling, see the official
[queue guide](https://docs.cloud.google.com/spanner/docs/queues/queues-using)
and [processing guarantees](https://docs.cloud.google.com/spanner/docs/queues/queues-at-most-once).

## Parser compatibility

Queue TVF calls use ordinary query syntax. The bundled memefish version can
still reject queue DDL, queue privileges, or `ASSERT_ROWS_MODIFIED` in strict
`MEMEFISH_ONLY` mode. Native SQL can use the existing lexical path:

```sql
SET CLI_PARSE_MODE = 'NO_MEMEFISH';
```

This is the default parser mode. It changes local statement classification,
not server capabilities, and does not add queue AST support to parser-dependent
features. No dependency replacement is needed for this setting.

## Restore settings

These settings persist in the session. RESET restores startup values, which may
differ from built-in defaults:

```sql
RESET CLI_FORMAT;
RESET CLI_TABLE_STREAMING;
RESET CLI_TABLE_PREVIEW_ROWS;
RESET READ_ONLY_STALENESS;
RESET STATEMENT_TIMEOUT;
RESET CLI_PARSE_MODE;
```
