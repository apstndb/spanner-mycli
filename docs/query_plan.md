# Query Plan Features

This document describes the query plan analysis and visualization features in spanner-mycli.

## EXPLAIN

You can see query plan without query execution using the `EXPLAIN` client side statement.

```
spanner> EXPLAIN
         SELECT SingerId, FirstName FROM Singers WHERE FirstName LIKE "A%";
+----+-------------------------------------------------------------------------------------------+
| ID | Query_Execution_Plan                                                                      |
+----+-------------------------------------------------------------------------------------------+
| *0 | Distributed Union <Row> (distribution_table: indexOnSingers, split_ranges_aligned: false) |
|  1 | +- Local Distributed Union <Row>                                                          |
|  2 |    +- Serialize Result <Row>                                                              |
|  3 |       +- Filter Scan <Row> (seekable_key_size: 1)                                         |
| *4 |          +- Index Scan <Row> (Index: indexOnSingers, scan_method: Row)                    |
+----+-------------------------------------------------------------------------------------------+
Predicates(identified by ID):
 0: Split Range: STARTS_WITH($FirstName, 'A')
 4: Seek Condition: STARTS_WITH($FirstName, 'A')

5 rows in set (0.86 sec)
```

Note: `<Row>` or `<Batch>` after the operator name mean [execution method](https://cloud.google.com/spanner/docs/sql-best-practices#optimize-query-execution) of the operator node.

## EXPLAIN ANALYZE

You can see query plan and execution profile using the `EXPLAIN ANALYZE` client side statement.
You should know that it requires executing the query.

```
spanner> EXPLAIN ANALYZE
         SELECT SingerId, FirstName FROM Singers WHERE FirstName LIKE "A%";
+----+-------------------------------------------------------------------------------------------+---------------+------------+---------------+
| ID | Query_Execution_Plan                                                                      | Rows_Returned | Executions | Total_Latency |
+----+-------------------------------------------------------------------------------------------+---------------+------------+---------------+
| *0 | Distributed Union <Row> (distribution_table: indexOnSingers, split_ranges_aligned: false) | 235           | 1          | 1.17 msecs    |
|  1 | +- Local Distributed Union <Row>                                                          | 235           | 1          | 1.12 msecs    |
|  2 |    +- Serialize Result <Row>                                                              | 235           | 1          | 1.1 msecs     |
|  3 |       +- Filter Scan <Row> (seekable_key_size: 1)                                         | 235           | 1          | 1.05 msecs    |
| *4 |          +- Index Scan <Row> (Index: indexOnSingers, scan_method: Row)                    | 235           | 1          | 1.02 msecs    |
+----+-------------------------------------------------------------------------------------------+---------------+------------+---------------+
Predicates(identified by ID):
 0: Split Range: STARTS_WITH($FirstName, 'A')
 4: Seek Condition: STARTS_WITH($FirstName, 'A')

5 rows in set (4.49 msecs)
timestamp:            2025-04-16T01:07:59.137819+09:00
cpu time:             3.73 msecs
rows scanned:         235 rows
deleted rows scanned: 0 rows
optimizer version:    7
optimizer statistics: auto_20250413_15_34_23UTC
```

## Configurable EXPLAIN ANALYZE

spanner-mycli supports to configure execution stats columns of `EXPLAIN ANALYZE` using `CLI_ANALYZE_COLUMNS`.

Note: `CLI_ANALYZE_COLUMNS` is formatted string like `<name>:<template>[:<alignment>]`. `<template>` is needed to be written in [`text/template`](https://pkg.go.dev/text/template) format and it is bounded with [`ExecutionStats`](https://pkg.go.dev/github.com/apstndb/spannerplanviz/stats#ExecutionStats).

```
spanner> SET CLI_ANALYZE_COLUMNS='Rows:{{if ne .Rows.Total ""}}{{.Rows.Total}}{{end}},Scanned:{{.ScannedRows.Total}},Filtered:{{.FilteredRows.Total}}';

Empty set (0.00 sec)

spanner> EXPLAIN ANALYZE
         SELECT * FROM Singers
         JOIN Albums USING (SingerId)
         WHERE FirstName LIKE "M%c%";
+-----+--------------------------------------------------------------------------------+------+---------+----------+
| ID  | Query_Execution_Plan <execution_method> (metadata, ...)                        | Rows | Scanned | Filtered |
+-----+--------------------------------------------------------------------------------+------+---------+----------+
|   0 | Distributed Union on Singers <Row> (split_ranges_aligned: true)                |  864 |         |          |
|   1 | +- Local Distributed Union <Row>                                               |  864 |         |          |
|   2 |    +- Serialize Result <Row>                                                   |  864 |         |          |
|   3 |       +- Cross Apply <Row>                                                     |  864 |         |          |
|  *4 |          +- [Input] Filter Scan <Row> (seekable_key_size: 0)                   |      |         |          |
|   5 |          |  +- Table Scan on Singers <Row> (Full scan, scan_method: Automatic) |   27 |    1000 |      973 |
|  15 |          +- [Map] Local Distributed Union <Row>                                |  864 |         |          |
|  16 |             +- Filter Scan <Row> (seekable_key_size: 0)                        |      |         |          |
| *17 |                +- Table Scan on Albums <Row> (scan_method: Row)                |  864 |     864 |        0 |
+-----+--------------------------------------------------------------------------------+------+---------+----------+
Predicates(identified by ID):
  4: Residual Condition: ($FirstName LIKE 'M%c%')
 17: Seek Condition: ($SingerId_1 = $SingerId)
```

### Inline stats

You can define inline stats using `CLI_INLINE_STATS`.
Inline stats are rendered in Operator column, and it is good for sparse stats which are not appeared in all operators.

```
spanner> SET CLI_INLINE_STATS='Scanned:{{.ScannedRows.Total}},Filtered:{{.FilteredRows.Total}}';
Empty set (0.00 sec)

spanner> EXPLAIN ANALYZE WIDTH=70
         SELECT * FROM Singers
         JOIN Albums USING (SingerId)
         WHERE FirstName LIKE "M%c%";
+-----+-----------------------------------------------------------------------+------+-------+---------------+
| ID  | Operator <execution_method> (metadata, ...)                           | Rows | Exec. | Total Latency |
+-----+-----------------------------------------------------------------------+------+-------+---------------+
|   0 | Distributed Union on Albums <Row>                                     |  864 |     1 |  105.05 msecs |
|   1 | +- Serialize Result <Row>                                             |  864 |     1 |   104.9 msecs |
|   2 |    +- Cross Apply <Row>                                               |  864 |     1 |  104.52 msecs |
|   3 |       +- [Input] Distributed Union on Singers <Row>                   |   27 |     1 |   93.77 msecs |
|   4 |       |  +- Local Distributed Union <Row>                             |   27 |     1 |   93.74 msecs |
|  *5 |       |     +- Filter Scan <Row> (seekable_key_size: 0)               |      |       |               |
|   6 |       |        +- Table Scan on Singers <Row> (Full scan, scan_method |   27 |     1 |   93.71 msecs |
|     |       |           : Automatic, Scanned=1000, Filtered=973)            |      |       |               |
|  17 |       +- [Map] Local Distributed Union <Row>                          |  864 |    27 |   10.64 msecs |
|  18 |          +- Filter Scan <Row> (seekable_key_size: 0)                  |      |       |               |
| *19 |             +- Table Scan on Albums <Row> (scan_method: Row, Scanned= |  864 |    27 |   10.51 msecs |
|     |                864, Filtered=0)                                       |      |       |               |
+-----+-----------------------------------------------------------------------+------+-------+---------------+
Predicates(identified by ID):
  5: Residual Condition: ($FirstName LIKE 'M%c%')
 19: Seek Condition: ($SingerId_1 = $SingerId)

10 rows in set (120.95 msecs)
timestamp:            2025-06-05T05:43:23.912275+09:00
cpu time:             45.08 msecs
rows scanned:         1864 rows
deleted rows scanned: 0 rows
optimizer version:    8
optimizer statistics: auto_20250604_03_26_04UTC
```

## Investigate the last query

Query plan is cached after executing a query or a DML, or `EXPLAIN ANALYZE`.

### `EXPLAIN [ANALYZE]` for the last query

You can render `EXPLAIN` or `EXPLAIN ANALYZE` without executing the query again.
It is good to do trial and error loop to find a better format for your query plan.

```
spanner> SELECT * FROM Singers
         JOIN Albums USING (SingerId)
         WHERE AlbumTitle LIKE "%e";
+----------+-----------+----------+------------+------------+---------+-------------------------+-----------------+
| SingerId | FirstName | LastName | SingerInfo | BirthDate  | AlbumId | AlbumTitle              | MarketingBudget |
+----------+-----------+----------+------------+------------+---------+-------------------------+-----------------+
| 2        | Catalina  | Smith    | NULL       | 1990-08-17 | 2       | Forever Hold Your Peace | NULL            |
| 3        | Alice     | Trentor  | NULL       | 1991-10-02 | 1       | Nothing To Do With Me   | NULL            |
+----------+-----------+----------+------------+------------+---------+-------------------------+-----------------+
2 rows in set (14.11 msecs)

spanner> EXPLAIN LAST QUERY;
+-----+----------------------------------------------------------------------------------+
| ID  | Operator <execution_method> (metadata, ...)                                      |
+-----+----------------------------------------------------------------------------------+
|   0 | Distributed Union on Singers <Row>                                               |
|   1 | +- Serialize Result <Row>                                                        |
|   2 |    +- Cross Apply <Row>                                                          |
|   3 |       +- [Input] Distributed Union on Albums <Row>                               |
|   4 |       |  +- Local Distributed Union <Row>                                        |
|  *5 |       |     +- Filter Scan <Row> (seekable_key_size: 0)                          |
|   6 |       |        +- Table Scan on Albums <Row> (Full scan, scan_method: Automatic) |
|  16 |       +- [Map] Local Distributed Union <Row>                                     |
|  17 |          +- Filter Scan <Row> (seekable_key_size: 0)                             |
| *18 |             +- Table Scan on Singers <Row> (scan_method: Row)                    |
+-----+----------------------------------------------------------------------------------+
Predicates(identified by ID):
  5: Residual Condition: ($AlbumTitle LIKE '%e')
 18: Seek Condition: ($SingerId = $SingerId_1)

10 rows in set (0.00 sec)

spanner> EXPLAIN ANALYZE WIDTH=50 LAST QUERY;
+-----+---------------------------------------------------+------+-------+---------------+
| ID  | Operator <execution_method> (metadata, ...)       | Rows | Exec. | Total Latency |
+-----+---------------------------------------------------+------+-------+---------------+
|   0 | Distributed Union on Singers <Row>                |    2 |     1 |     5.5 msecs |
|   1 | +- Serialize Result <Row>                         |    2 |     3 |    3.03 msecs |
|   2 |    +- Cross Apply <Row>                           |    2 |     3 |    3.02 msecs |
|   3 |       +- [Input] Distributed Union on Albums <Row |    2 |     3 |    2.95 msecs |
|     |       |  >                                        |      |       |               |
|   4 |       |  +- Local Distributed Union <Row>         |    2 |     4 |    0.26 msecs |
|  *5 |       |     +- Filter Scan <Row> (seekable_key_si |      |       |               |
|     |       |        ze: 0)                             |      |       |               |
|   6 |       |        +- Table Scan on Albums <Row> (Ful |    2 |     4 |    0.22 msecs |
|     |       |           l scan, scan_method: Automatic) |      |       |               |
|  16 |       +- [Map] Local Distributed Union <Row>      |    2 |     2 |    0.06 msecs |
|  17 |          +- Filter Scan <Row> (seekable_key_size: |      |       |               |
|     |              0)                                   |      |       |               |
| *18 |             +- Table Scan on Singers <Row> (scan_ |    2 |     2 |    0.05 msecs |
|     |                method: Row)                       |      |       |               |
+-----+---------------------------------------------------+------+-------+---------------+
Predicates(identified by ID):
  5: Residual Condition: ($AlbumTitle LIKE '%e')
 18: Seek Condition: ($SingerId = $SingerId_1)

10 rows in set (14.11 msecs)
cpu time:             12.29 msecs
rows scanned:         9 rows
deleted rows scanned: 0 rows
optimizer version:    8
optimizer statistics: auto_20250605_10_15_54UTC
```

#### `SHOW PLAN NODE`

You can also print the raw plan node in the last query using `SHOW PLAN NODE <node_id>` client-side statement.

```
spanner> SHOW PLAN NODE 18;
+-------------------------------------------------------------------------------------------------+
| Content of Node 18                                                                              |
+-------------------------------------------------------------------------------------------------+
| index: 18                                                                                       |
| kind: 1                                                                                         |
| display_name: Scan                                                                              |
| child_links:                                                                                    |
| - {child_index: 19, variable: SingerId}                                                         |
| - {child_index: 20, variable: FirstName}                                                        |
| - {child_index: 21, variable: LastName}                                                         |
| - {child_index: 22, variable: SingerInfo}                                                       |
| - {child_index: 23, variable: BirthDate}                                                        |
| - {child_index: 27, type: Seek Condition}                                                       |
| metadata: {execution_method: Row, scan_method: Row, scan_target: Singers, scan_type: TableScan} |
| execution_stats:                                                                                |
|   cpu_time: {mean: "0.04", std_deviation: "0.03", total: "0.08", unit: msecs}                   |
|   deleted_rows: {mean: "0", std_deviation: "0", total: "0", unit: rows}                         |
|   execution_summary: {num_executions: "2"}                                                      |
|   filesystem_delay_seconds: {mean: "0", std_deviation: "0", total: "0", unit: msecs}            |
|   filtered_rows: {mean: "0", std_deviation: "0", total: "0", unit: rows}                        |
|   latency: {mean: "0.04", std_deviation: "0.03", total: "0.08", unit: msecs}                    |
|   rows: {mean: "1", std_deviation: "0", total: "2", unit: rows}                                 |
|   scanned_rows:                                                                                 |
|     histogram:                                                                                  |
|     - {count: "3", lower_bound: "0", percentage: "75", upper_bound: "1"}                        |
|     - {count: "1", lower_bound: "1", percentage: "25", upper_bound: "4"}                        |
|     mean: "0.5"                                                                                 |
|     std_deviation: "0.87"                                                                       |
|     total: "2"                                                                                  |
|     unit: rows                                                                                  |
+-------------------------------------------------------------------------------------------------+
1 rows in set (0.00 sec)
```

## Query plan linter (EARLY EXPERIMANTAL)

`CLI_LINT_PLAN` system variable enables heuristic query plan linter in `EXPLAIN` and `EXPLAIN ANALYZE`.

```
spanner> SET CLI_LINT_PLAN = TRUE;
Empty set (0.00 sec)

spanner> EXPLAIN SELECT * FROM Singers WHERE FirstName LIKE "%Hoge%";
+----+----------------------------------------------------------------------------------+
| ID | Query_Execution_Plan                                                             |
+----+----------------------------------------------------------------------------------+
|  0 | Distributed Union (distribution_table: Singers, split_ranges_aligned: false)     |
|  1 | +- Local Distributed Union                                                       |
|  2 |    +- Serialize Result                                                           |
| *3 |       +- Filter Scan (seekable_key_size: 0)                                      |
|  4 |          +- Table Scan (Full scan: true, Table: Singers, scan_method: Automatic) |
+----+----------------------------------------------------------------------------------+
Predicates(identified by ID):
 3: Residual Condition: ($FirstName LIKE '%Hoge%')

Experimental Lint Result:
 3: Filter Scan (seekable_key_size: 0)
     Residual Condition: Potentially expensive Residual Condition: Maybe better to modify it to Scan Condition
 4: Table Scan (Full scan: true, Table: Singers, scan_method: Automatic)
     Full scan=true: Potentially expensive execution full scan: Do you really want full scan?
```

## Show query profiles (EARLY EXPERIMENTAL)

spanner-mycli can render undocumented underlying table of [sampled query plans](https://cloud.google.com/spanner/docs/query-execution-plans#sampled-plans).
These features are early experimental state so it will be changed.

### Show query profiles(It will be very long outputs).

```
spanner> SHOW QUERY PROFILES;
+-------------------------------------------------------------------------------------------+
| Plan                                                                                      |
+-------------------------------------------------------------------------------------------+
| SELECT * FROM Singers JOIN Albums USING (SingerId)                                        |
| ID  | Plan                                                                                |
|   0 | Distributed Union on Singers <Row> (split_ranges_aligned)                           |
|   1 | +- Local Distributed Union <Row>                                                    |
|   2 |    +- Serialize Result <Row>                                                        |
|   3 |       +- Cross Apply <Row>                                                          |
|   4 |          +- [Input] Table Scan on Singers <Row> (Full scan, scan_method: Automatic) |
|  10 |          +- [Map] Local Distributed Union <Row>                                     |
|  11 |             +- Filter Scan <Row> (seekable_key_size: 0)                             |
| *12 |                +- Table Scan on Albums <Row> (scan_method: Row)                     |
| Predicates:                                                                               |
| 12: Seek Condition: ($SingerId_1 = $SingerId)                                             |
| interval_end:                 2025-05-29 08:00:00 +0000 UTC                               |
| text_fingerprint:             -6422424748333414178                                        |
| elapsed_time:                 11.09 msecs                                                 |
| cpu_time:                     9.2 msecs                                                   |
| rows_returned:                7                                                           |
| deleted_rows_scanned:         0                                                           |
| optimizer_version:            7                                                           |
| optimizer_statistics_package: auto_20250527_16_21_42UTC                                   |
| SELECT @_p0_INT64                                                                         |
| ID | Plan                                                                                 |
|  0 | Serialize Result <Row>                                                               |
|  1 | +- Unit Relation <Row>                                                               |
| interval_end:                 2025-05-22 05:00:00 +0000 UTC                               |
| text_fingerprint:             -773118905674708524                                         |
| elapsed_time:                 19.78 msecs                                                 |
| cpu_time:                     19.73 msecs                                                 |
| rows_returned:                1                                                           |
| deleted_rows_scanned:         0                                                           |
| optimizer_version:            7                                                           |
| optimizer_statistics_package: auto_20250521_18_02_30UTC                                   |
+-------------------------------------------------------------------------------------------+
24 rows in set (0.62 sec)
```

Render a latest profile for a `TEXT_FINGERPRINT`. It is compatible with plan linter(`CLI_LINT_PLAN`).

```
spanner> SHOW QUERY PROFILE -6422424748333414178;
+-----+-------------------------------------------------------------------------------------+------+-------+---------------+
| ID  | Operator <execution_method> (metadata, ...)                                         | Rows | Exec. | Total Latency |
+-----+-------------------------------------------------------------------------------------+------+-------+---------------+
|   0 | Distributed Union on Singers <Row> (split_ranges_aligned)                           |    7 |     1 |    5.09 msecs |
|   1 | +- Local Distributed Union <Row>                                                    |    7 |     3 |    2.85 msecs |
|   2 |    +- Serialize Result <Row>                                                        |    7 |     4 |    0.18 msecs |
|   3 |       +- Cross Apply <Row>                                                          |    7 |     4 |    0.17 msecs |
|   4 |          +- [Input] Table Scan on Singers <Row> (Full scan, scan_method: Automatic) |    5 |     4 |     0.1 msecs |
|  10 |          +- [Map] Local Distributed Union <Row>                                     |    7 |     5 |    0.06 msecs |
|  11 |             +- Filter Scan <Row> (seekable_key_size: 0)                             |      |       |               |
| *12 |                +- Table Scan on Albums <Row> (scan_method: Row)                     |    7 |     5 |    0.05 msecs |
+-----+-------------------------------------------------------------------------------------+------+-------+---------------+
Predicates(identified by ID):
 12: Seek Condition: ($SingerId_1 = $SingerId)

8 rows in set (11.09 msecs)
cpu time:             9.2 msecs
rows scanned:         12 rows
deleted rows scanned: 0 rows
optimizer version:    7
optimizer statistics: auto_20250527_16_21_42UTC
```

## More concise format of `EXPLAIN` and `EXPLAIN ANALYZE`

spanner-mycli prioritizes the density of `EXPLAIN` and `EXPLAIN ANALYZE` information, and may sometimes change to a more concise format, even if it involves breaking changes.
This is because the results of `EXPLAIN` and `EXPLAIN ANALYZE` sometimes need to be displayed in limited spaces, such as:

- Unmaximized terminals in small notebooks
- Code blocks within GitHub issues or pull requests for review purposes
- Code blocks on technical information sharing sites like Medium, Zenn, and Qiita

In particular, since code blocks often have a fixed display area, they do not expand even if the browser window size is increased.
The differences in `EXPLAIN ANALYZE` output between `spanner-cli` and `spanner-mycli` are explained below.

```sql
EXPLAIN ANALYZE
SELECT * FROM Singers
WHERE LastName LIKE "%son";
```

spanner-cli
```text
+----+---------------------------------------------------------------------------------------------------------+---------------+------------+---------------+
| ID | Query_Execution_Plan                                                                                    | Rows_Returned | Executions | Total_Latency |
+----+---------------------------------------------------------------------------------------------------------+---------------+------------+---------------+
|  0 | Distributed Union (distribution_table: Singers, execution_method: Row, split_ranges_aligned: false)     | 0             | 1          | 3.46 msecs    |
|  1 | +- Local Distributed Union (execution_method: Row)                                                      | 0             | 3          | 0.29 msecs    |
|  2 |    +- Serialize Result (execution_method: Row)                                                          | 0             | 3          | 0.26 msecs    |
| *3 |       +- Filter Scan (execution_method: Row, seekable_key_size: 0)                                      |               |            |               |
|  4 |          +- Table Scan (Full scan: true, Table: Singers, execution_method: Row, scan_method: Automatic) | 0             | 3          | 0.26 msecs    |
+----+---------------------------------------------------------------------------------------------------------+---------------+------------+---------------+
Predicates(identified by ID):
 3: Residual Condition: ($LastName LIKE '%son')
```

spanner-mycli

```text
+----+-----------------------------------------------------------------------------+------+-------+---------------+
| ID | Operator <execution_method> (metadata, ...)                                 | Rows | Exec. | Total Latency |
+----+-----------------------------------------------------------------------------+------+-------+---------------+
|  0 | Distributed Union on Singers <Row>                                          |    0 |     1 |    7.58 msecs |
|  1 | +- Local Distributed Union <Row>                                            |    0 |     3 |    0.78 msecs |
|  2 |    +- Serialize Result <Row>                                                |    0 |     3 |    0.75 msecs |
| *3 |       +- Filter Scan <Row> (seekable_key_size: 0)                           |      |       |               |
|  4 |          +- Table Scan on Singers <Row> (Full scan, scan_method: Automatic) |    0 |     3 |    0.75 msecs |
+----+-----------------------------------------------------------------------------+------+-------+---------------+
Predicates(identified by ID):
 3: Residual Condition: ($LastName LIKE '%son')

```

- `execution_method: {Row|Batch}` metadata is simply displayed as `<Row>` or `<Batch>` after display name of operator.
- Target metadata, `distribution_table: <target>`, `scan_target: <target>`, and `table: <target>` are displayed as `on <target>` after display name of operator.
- Some metadata with boolean value is printed as label.
  - For example, `Full scan: true` and `split_ranges_aligned: true` are shortened as `Full scan` and `split_ranges_aligned`, and they are not printed if the value is `false`.
- Column names of `EXPLAIN ANALYZE` are shorter than before.

Note: These changes except column names can be controlled using `EXPLAIN [ANALYZE] FORMAT=TRADITIONAL` or `SET CLI_EXPLAIN_FORMAT=TRADITIONAL`.

To support more limited width environment, there are further options.
### Compact format

`FORMAT=COMPACT` option removes characters that use width for readability, as long as it doesn't compromise information.

- Whitespace is generally not inserted.
- The characters used for the tree will only be one character wide per level.

```
spanner> EXPLAIN FORMAT=COMPACT
         SELECT * FROM Singers
         JOIN Albums USING (SingerId)
         WHERE LastName LIKE "%son";
+-----+-------------------------------------------------------------------+
| ID  | Operator <execution_method> (metadata, ...)                       |
+-----+-------------------------------------------------------------------+
|   0 | Distributed Union on Albums<Row>                                  |
|   1 | +Serialize Result<Row>                                            |
|   2 |  +Cross Apply<Row>                                                |
|   3 |   +[Input]Distributed Union on Singers<Row>                       |
|   4 |   |+Local Distributed Union<Row>                                  |
|  *5 |   | +Filter Scan<Row>(seekable_key_size:0)                        |
|   6 |   |  +Table Scan on Singers<Row>(Full scan,scan_method:Automatic) |
|  17 |   +[Map]Local Distributed Union<Row>                              |
|  18 |    +Filter Scan<Row>(seekable_key_size:0)                         |
| *19 |     +Table Scan on Albums<Row>(scan_method:Row)                   |
+-----+-------------------------------------------------------------------+
Predicates(identified by ID):
  5: Residual Condition: ($LastName LIKE '%son')
 19: Seek Condition: ($SingerId_1 = $SingerId)
```

### Wrapped plans

When you want to adjust the width, such as when displaying an execution plan on media where horizontal scrolling is not possible and content might be truncated or wrapped,
you can use the `WIDTH=<width>` option to wrap the content of the `Operator` column at the specified width.

For example, in the illustration below, by setting the width of the ASCII tree drawn in the Operator column to 39 characters,
the entire output, including the ASCII table, can fit within 80 characters.

```
spanner> EXPLAIN ANALYZE FORMAT=COMPACT WIDTH=39
         SELECT * FROM Singers
         JOIN Albums USING (SingerId)
         WHERE LastName LIKE "%r";
+-----+-----------------------------------------+------+-------+---------------+
| ID  | Operator                                | Rows | Exec. | Total Latency |
+-----+-----------------------------------------+------+-------+---------------+
|   0 | Distributed Union on Albums<Row>        |    1 |     1 |    3.82 msecs |
|   1 | +Serialize Result<Row>                  |    1 |     3 |     2.2 msecs |
|   2 |  +Cross Apply<Row>                      |    1 |     3 |    2.19 msecs |
|   3 |   +[Input]Distributed Union on Singers< |    1 |     3 |    2.16 msecs |
|     |   |Row>                                 |      |       |               |
|   4 |   |+Local Distributed Union<Row>        |    1 |     4 |    0.25 msecs |
|  *5 |   | +Filter Scan<Row>(seekable_key_size |      |       |               |
|     |   |  :0)                                |      |       |               |
|   6 |   |  +Table Scan on Singers<Row>(Full s |    1 |     4 |    0.21 msecs |
|     |   |   can,scan_method:Automatic)        |      |       |               |
|  17 |   +[Map]Local Distributed Union<Row>    |    1 |     1 |    0.03 msecs |
|  18 |    +Filter Scan<Row>(seekable_key_size: |      |       |               |
|     |     0)                                  |      |       |               |
| *19 |     +Table Scan on Albums<Row>(scan_met |    1 |     1 |    0.03 msecs |
|     |      hod:Row)                           |      |       |               |
+-----+-----------------------------------------+------+-------+---------------+
Predicates(identified by ID):
  5: Residual Condition: ($LastName LIKE '%r')
 19: Seek Condition: ($SingerId_1 = $SingerId)
```

Note: Since the widths of columns other than the Operator column are not deterministic, there may be cases where the output does not fit within 80 characters even with the same settings, such as when the number of Rows is large.

## Configuration Options

### System Variables
- `CLI_ANALYZE_COLUMNS`: Customize EXPLAIN ANALYZE columns using text/template format
- `CLI_INLINE_STATS`: Define inline statistics display within operator column
- `CLI_LINT_PLAN`: Enable heuristic query plan linter for EXPLAIN and EXPLAIN ANALYZE
- `CLI_EXPLAIN_FORMAT`: Control EXPLAIN format (TRADITIONAL vs default concise format)

### Statement Options
- `FORMAT`: Control output format
  - `FORMAT=COMPACT`: Remove whitespace and use single-character tree drawing
  - `FORMAT=TRADITIONAL`: Use spanner-cli compatible format
- `WIDTH=<width>`: Control content wrapping for fixed-width displays