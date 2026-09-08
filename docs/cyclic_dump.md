# Cyclic DUMP restoration

`DUMP DATABASE` and `DUMP TABLES` reject populated cyclic table groups by
default. This includes self-references and cycles involving interleave parents
and enforced foreign keys. Classification uses table relationships, not row
values: all-NULL references or row-acyclic data do not bypass the default check.
Empty cyclic groups remain exportable without a data transaction.

For GoogleSQL databases, explicitly enable mutation output when you intend to
restore populated cyclic groups with spanner-mycli:

```sql
SET CLI_DUMP_CYCLIC_MODE = 'MUTATE';
DUMP DATABASE;
-- Or export only the named tables, without DDL:
DUMP TABLES Parent, Child;
```

To save a dump without mixing progress and diagnostics into the SQL:

```sh
spanner-mycli -p myproject -i myinstance -d source_db \
  --set CLI_DUMP_CYCLIC_MODE=MUTATE --output cyclic.sql -e 'DUMP DATABASE;'
```

`--output` overwrites an existing file. Check the exit status before replaying
the file. Restore with spanner-mycli, which understands the generated `MUTATE`
statements; the file is not plain server-side SQL. Start restoration outside an
existing transaction. Use a fresh matching target for `DUMP DATABASE`.

## Output and source snapshot

Each populated strongly connected table group is emitted as one `BEGIN RW`,
typed `MUTATE ... INSERT STRUCT<...>(...)` statements, and `COMMIT`. The group
is never automatically split across commits. Acyclic tables keep ordinary
`INSERT` output. Dependencies precede the groups that need them, and dependent
tables follow. Known `NOT ENFORCED` foreign keys do not impose a safety edge.

Before emitting any dump SQL, including DATABASE DDL, the exporter reads and
encodes **all selected cyclic groups** using the same read-only transaction as
the catalog and column metadata. It retains the encoded text and does not
rescan those groups during output. Cyclic scan, cancellation, unsupported-type,
malformed-value, and local-cap errors in this preflight produce no dump SQL in
either buffered or streaming mode. DUMP does not write to the source database.

This is not a universal zero-partial-output guarantee: later acyclic reads and
output I/O can fail after streaming has begun. The separate admin DDL read for
`DUMP DATABASE` is not timestamp-bound to the read-only data snapshot, so
concurrent schema changes remain a risk.

## Local retention cap, not a service-quota estimate

`CLI_DUMP_CYCLIC_MAX_BYTES` defaults to `67108864` (64 MiB) and must be a
positive INT64. It limits the aggregate retained encoded cyclic text across
all selected groups, including transaction framing. It is not a hard process
memory bound: transient row formatting, decoded values, and container overhead
also use memory. Streaming does not remove this cyclic preflight retention.

You may explicitly raise the limit at your own local resource cost:

```sql
SET CLI_DUMP_CYCLIC_MAX_BYTES = 134217728;
```

The exporter does **not** predict the 80,000-mutation or 100 MiB service commit
limits. Generated SQL size is not server commit size. A successful dump, or a
successful emulator replay, does not certify that a group fits service quotas.
A group may fail atomically at target COMMIT; it is not automatically chunked.
Earlier groups, DDL, and ordinary INSERTs may already be committed. The entire
restore is not atomic, and rerunning after a failure can encounter duplicate
keys. Inspect the target before choosing how to recover.

## Selection, schema, and values

`DUMP TABLES` includes only the requested tables and never changes target
constraints. Selecting only part of a cycle can leave ordinary INSERT output;
omitted referenced tables, interleave parents, and prerequisite target rows are
the caller's responsibility. Selection is not a promise of standalone replay.

Generated stored columns are omitted and recompute at the target. Defaulted
columns include their observed source values. PROTO and ENUM columns use BYTES
and INT64 wire-surrogate values; matching target descriptors must already be
available. This mode does not export descriptor files. Unsupported cyclic row
types or malformed values fail during preflight rather than being guessed.

Matching schema, permissions, descriptors, and target state remain prerequisites.
There is no guarantee for arbitrary schema drift, additional target constraints,
or nonempty targets. PostgreSQL databases, row-level cycle splitting, service
quota prediction, and globally atomic restoration are outside this mode.
