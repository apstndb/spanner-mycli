# Working with Spanner queues

Queue availability depends on the Spanner/Omni server build. This guide describes
existing SQL execution and output controls; it does not establish that every
Omni build supports queues. `HELP QUEUES;` shows the essential settings offline.

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

## Receive is not a passive peek

RECEIVE acquires message leases and delivery is at least once. Printing a row
does not acknowledge it. spanner-mycli does not automatically renew leases,
acknowledge messages, or loop to restart the receive query.

Use `RENEWLEASE_<queue>` explicitly if processing requires more time, and
acknowledge only after processing succeeds. Inspect the returned lease tokens
and expiration times. Cancelling a query or failing to write its output must
not be interpreted as acknowledgement or rollback of earlier operations.

For transactional acknowledgement with `DELETE ... ASSERT_ROWS_MODIFIED 1`,
a failed assertion fails that statement; it does not automatically abort the
transaction. Explicitly roll back when the surrounding transaction must not
commit earlier work.

See the official [queue guide](https://docs.cloud.google.com/spanner/docs/queues/queues-using)
and [delivery model](https://docs.cloud.google.com/spanner/docs/queues/queues-overview)
for lease, acknowledgement, and retry semantics.

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
