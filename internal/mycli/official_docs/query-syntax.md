GoogleSQL is the new name for Google Standard SQL\! New name, same great SQL dialect.

Query statements scan one or more tables or expressions and return the computed result rows. This topic describes the syntax for SQL queries in GoogleSQL for Spanner.

## SQL syntax notation rules

The following table lists and describes the syntax notation rules that GoogleSQL documentation commonly uses.

Notation

Example

Description

Square brackets

`  [ ]  `

Optional clauses

Parentheses

`  ( )  `

Literal parentheses

Vertical bar

`  |  `

Logical `  XOR  ` (exclusive `  OR  ` )

Curly braces

`  { }  `

A set of options, such as `  { a | b | c }  ` . Select one option.

Ellipsis

`  ...  `

The preceding item can repeat.

Comma

`  ,  `

Literal comma

Comma followed by an ellipsis

`  , ...  `

The preceding item can repeat in a comma-separated list.

Item list

`  item [, ...]  `

One or more items

`  [item, ...]  `

Zero or more items

Double quotes

`  ""  `

The enclosed syntax characters (for example, `  "{"..."}"  ` ) are literal and required.

Angle brackets

`  <>  `

Literal angle brackets

## SQL syntax

``` text
query_statement:
  [ statement_hint_expr ]
  [ table_hint_expr ]
  [ group_hint_expr ]
  [ join_hint_expr ]
  query_expr

query_expr:
  [ WITH cte[, ...] ]
  { select | ( query_expr ) | set_operation }
  [ ORDER BY expression [{ ASC | DESC }] [, ...] ]
  [ LIMIT count [ OFFSET skip_rows ] ]
  [ FOR UPDATE ]

select:
  SELECT
    [ { ALL | DISTINCT } ]
    [ AS { typename | STRUCT | VALUE } ]
    select_list
  [ FROM from_clause[, ...] ]
  [ WHERE bool_expression ]
  [ GROUP [  group_hint_expr ] BY group_by_specification ]
  [ HAVING bool_expression ]
```

## `     SELECT    ` statement

``` text
SELECT
  [ { ALL | DISTINCT } ]
  [ AS { typename | STRUCT | VALUE } ]
  select_list

select_list:
  { select_all | select_expression } [, ...]

select_all:
  [ expression. ]*
  [ EXCEPT ( column_name [, ...] ) ]
  [ REPLACE ( expression AS column_name [, ...] ) ]

select_expression:
  expression [ [ AS ] alias ]
```

The `  SELECT  ` list defines the columns that the query will return. Expressions in the `  SELECT  ` list can refer to columns in any of the `  from_item  ` s in its corresponding `  FROM  ` clause.

Each item in the `  SELECT  ` list is one of:

  - `  *  `
  - `  expression  `
  - `  expression.*  `

### `     SELECT *    `

`  SELECT *  ` , often referred to as *select star* , produces one output column for each column that's visible after executing the full query.

``` text
SELECT * FROM (SELECT "apple" AS fruit, "carrot" AS vegetable);

/*-------+-----------+
 | fruit | vegetable |
 +-------+-----------+
 | apple | carrot    |
 +-------+-----------*/
```

### `     SELECT expression    `

Items in a `  SELECT  ` list can be expressions. These expressions evaluate to a single value and produce one output column, with an optional explicit `  alias  ` .

If the expression doesn't have an explicit alias, it receives an implicit alias according to the rules for [implicit aliases](#implicit_aliases) , if possible. Otherwise, the column is anonymous and you can't refer to it by name elsewhere in the query.

### `     SELECT expression.*    `

An item in a `  SELECT  ` list can also take the form of `  expression.*  ` . This produces one output column for each column or top-level field of `  expression  ` . The expression must either be a table alias or evaluate to a single value of a data type with fields, such as a STRUCT.

The following query produces one output column for each column in the table `  groceries  ` , aliased as `  g  ` .

``` text
WITH groceries AS
  (SELECT "milk" AS dairy,
   "eggs" AS protein,
   "bread" AS grain)
SELECT g.*
FROM groceries AS g;

/*-------+---------+-------+
 | dairy | protein | grain |
 +-------+---------+-------+
 | milk  | eggs    | bread |
 +-------+---------+-------*/
```

More examples:

``` text
WITH locations AS
  (SELECT STRUCT("Seattle" AS city, "Washington" AS state) AS location
  UNION ALL
  SELECT STRUCT("Phoenix" AS city, "Arizona" AS state) AS location)
SELECT l.location.*
FROM locations l;

/*---------+------------+
 | city    | state      |
 +---------+------------+
 | Seattle | Washington |
 | Phoenix | Arizona    |
 +---------+------------*/
```

``` text
WITH locations AS
  (SELECT ARRAY<STRUCT<city STRING, state STRING>>[("Seattle", "Washington"),
    ("Phoenix", "Arizona")] AS location)
SELECT l.LOCATION[offset(0)].*
FROM locations l;

/*---------+------------+
 | city    | state      |
 +---------+------------+
 | Seattle | Washington |
 +---------+------------*/
```

### `     SELECT * EXCEPT    `

A `  SELECT * EXCEPT  ` statement specifies the names of one or more columns to exclude from the result. All matching column names are omitted from the output.

``` text
WITH orders AS
  (SELECT 5 as order_id,
  "sprocket" as item_name,
  200 as quantity)
SELECT * EXCEPT (order_id)
FROM orders;

/*-----------+----------+
 | item_name | quantity |
 +-----------+----------+
 | sprocket  | 200      |
 +-----------+----------*/
```

**Note:** `  SELECT * EXCEPT  ` doesn't exclude columns that don't have names.

### `     SELECT * REPLACE    `

A `  SELECT * REPLACE  ` statement specifies one or more `  expression AS identifier  ` clauses. Each identifier must match a column name from the `  SELECT *  ` statement. In the output column list, the column that matches the identifier in a `  REPLACE  ` clause is replaced by the expression in that `  REPLACE  ` clause.

A `  SELECT * REPLACE  ` statement doesn't change the names or order of columns. However, it can change the value and the value type.

``` text
WITH orders AS
  (SELECT 5 as order_id,
  "sprocket" as item_name,
  200 as quantity)
SELECT * REPLACE ("widget" AS item_name)
FROM orders;

/*----------+-----------+----------+
 | order_id | item_name | quantity |
 +----------+-----------+----------+
 | 5        | widget    | 200      |
 +----------+-----------+----------*/

WITH orders AS
  (SELECT 5 as order_id,
  "sprocket" as item_name,
  200 as quantity)
SELECT * REPLACE (quantity/2 AS quantity)
FROM orders;

/*----------+-----------+----------+
 | order_id | item_name | quantity |
 +----------+-----------+----------+
 | 5        | sprocket  | 100      |
 +----------+-----------+----------*/
```

**Note:** `  SELECT * REPLACE  ` doesn't replace columns that don't have names.

### `     SELECT DISTINCT    `

A `  SELECT DISTINCT  ` statement discards duplicate rows and returns only the remaining rows. `  SELECT DISTINCT  ` can't return columns of the following types:

  - `  PROTO  `
  - `  STRUCT  `
  - `  ARRAY  `
  - `  GRAPH_ELEMENT  `
  - `  GRAPH_PATH  `

### `     SELECT ALL    `

A `  SELECT ALL  ` statement returns all rows, including duplicate rows. `  SELECT ALL  ` is the default behavior of `  SELECT  ` .

### Using STRUCTs with SELECT

  - Queries that return a `  STRUCT  ` at the root of the return type aren't supported in Spanner APIs. For example, the following query is supported **only as a subquery** :
    
    ``` text
    SELECT STRUCT(1, 2) FROM Users;
    ```

  - Returning an array of structs is supported. For example, the following queries **are** supported in Spanner APIs:
    
    ``` text
    SELECT ARRAY(SELECT STRUCT(1 AS A, 2 AS B)) FROM Users;
    ```
    
    ``` text
    SELECT ARRAY(SELECT AS STRUCT 1 AS a, 2 AS b) FROM Users;
    ```

  - However, query shapes that can return an `  ARRAY<STRUCT<...>>  ` typed `  NULL  ` value or an `  ARRAY<STRUCT<...>>  ` typed value with an element that's `  NULL  ` aren't supported in Spanner APIs, so the following query is supported **only as a subquery** :
    
    ``` text
    SELECT ARRAY(SELECT IF(STARTS_WITH(Users.username, "a"), NULL, STRUCT(1, 2)))
    FROM Users;
    ```

**Note:** The logic inside Spanner that decides whether or not a query can return a `  NULL  ` array of structs or `  NULL  ` array of struct elements isn't *complete* (in the logic sense of *complete* ). That means some queries that clearly can't return `  NULL  ` s are still rejected and fine-tuning is sometimes necessary to get a query shape that's supported. The least troublesome query shapes use the `  ARRAY(SELECT AS STRUCT ... )  ` subquery to construct the array of struct values.

See [Querying STRUCT elements in an ARRAY](/spanner/docs/reference/standard-sql/arrays#query_structs_in_an_array) for more examples on how to query `  STRUCTs  ` inside an `  ARRAY  ` .

Also see notes about using `  STRUCTs  ` in [subqueries](/spanner/docs/reference/standard-sql/subqueries) .

### `     SELECT AS STRUCT    `

``` text
SELECT AS STRUCT expr [[AS] struct_field_name1] [,...]
```

This produces a [value table](#value_tables) with a STRUCT row type, where the STRUCT field names and types match the column names and types produced in the `  SELECT  ` list.

Example:

``` text
SELECT ARRAY(SELECT AS STRUCT 1 a, 2 b)
```

`  SELECT AS STRUCT  ` can be used in a scalar or array subquery to produce a single STRUCT type grouping multiple values together. Scalar and array subqueries (see [Subqueries](/spanner/docs/reference/standard-sql/subqueries) ) are normally not allowed to return multiple columns, but can return a single column with STRUCT type.

Anonymous columns are allowed.

Example:

``` text
SELECT AS STRUCT 1 x, 2, 3
```

The query above produces STRUCT values of type `  STRUCT<int64 x, int64, int64>.  ` The first field has the name `  x  ` while the second and third fields are anonymous.

The example above produces the same result as this `  SELECT AS VALUE  ` query using a struct constructor:

``` text
SELECT AS VALUE STRUCT(1 AS x, 2, 3)
```

Duplicate columns are allowed.

Example:

``` text
SELECT AS STRUCT 1 x, 2 y, 3 x
```

The query above produces STRUCT values of type `  STRUCT<int64 x, int64 y, int64 x>.  ` The first and third fields have the same name `  x  ` while the second field has the name `  y  ` .

The example above produces the same result as this `  SELECT AS VALUE  ` query using a struct constructor:

``` text
SELECT AS VALUE STRUCT(1 AS x, 2 AS y, 3 AS x)
```

### `     SELECT AS typename    `

``` text
SELECT AS typename
  expr [[AS] field]
  [, ...]
```

A `  SELECT AS typename  ` statement produces a value table where the row type is a specific named type. Currently, [protocol buffers](/spanner/docs/reference/standard-sql/protocol-buffers) are the only supported type that can be used with this syntax.

When selecting as a type that has fields, such as a proto message type, the `  SELECT  ` list may produce multiple columns. Each produced column must have an explicit or [implicit](#implicit_aliases) alias that matches a unique field of the named type.

When used with `  SELECT DISTINCT  ` , or `  GROUP BY  ` or `  ORDER BY  ` using column ordinals, these operators are first applied on the columns in the `  SELECT  ` list. The value construction happens last. This means that `  DISTINCT  ` can be applied on the input columns to the value construction, including in cases where `  DISTINCT  ` wouldn't be allowed after value construction because grouping isn't supported on the constructed type.

The following is an example of a `  SELECT AS typename  ` query.

``` text
SELECT AS tests.TestProtocolBuffer mytable.key int64_val, mytable.name string_val
FROM mytable;
```

The query returns the output as a `  tests.TestProtocolBuffer  ` protocol buffer. `  mytable.key int64_val  ` means that values from the `  key  ` column are stored in the `  int64_val  ` field in the protocol buffer. Similarly, values from the `  mytable.name  ` column are stored in the `  string_val  ` protocol buffer field.

To learn more about protocol buffers, see [Work with protocol buffers](/spanner/docs/reference/standard-sql/protocol-buffers) .

### `     SELECT AS VALUE    `

`  SELECT AS VALUE  ` produces a [value table](#value_tables) from any `  SELECT  ` list that produces exactly one column. Instead of producing an output table with one column, possibly with a name, the output will be a value table where the row type is just the value type that was produced in the one `  SELECT  ` column. Any alias the column had will be discarded in the value table.

Example:

``` text
SELECT AS VALUE 1
```

The query above produces a table with row type INT64.

Example:

``` text
SELECT AS VALUE STRUCT(1 AS a, 2 AS b) xyz
```

The query above produces a table with row type `  STRUCT<a int64, b int64>  ` .

Example:

``` text
SELECT AS VALUE v FROM (SELECT AS STRUCT 1 a, true b) v WHERE v.b
```

Given a value table `  v  ` as input, the query above filters out certain values in the `  WHERE  ` clause, and then produces a value table using the exact same value that was in the input table. If the query above didn't use `  SELECT AS VALUE  ` , then the output table schema would differ from the input table schema because the output table would be a regular table with a column named `  v  ` containing the input value.

## `     FROM    ` clause

``` text
FROM from_clause[, ...]

from_clause:
  from_item
  [ tablesample_operator ]

from_item:
  {
    table_name [ table_hint_expr ] [ as_alias ]
    | { join_operation | ( join_operation ) }
    | ( query_expr ) [ table_hint_expr ] [ as_alias ]
    | field_path
    | unnest_operator
    | cte_name [ table_hint_expr ] [ as_alias ]
    | graph_table_operator [ as_alias ]
  }

as_alias:
  [ AS ] alias
```

The `  FROM  ` clause indicates the table or tables from which to retrieve rows, and specifies how to join those rows together to produce a single stream of rows for processing in the rest of the query.

#### `     tablesample_operator    `

See [TABLESAMPLE operator](#tablesample_operator) .

#### `     graph_table_operator    `

See [GRAPH\_TABLE operator](/spanner/docs/reference/standard-sql/graph-sql-queries#graph_table_operator) .

#### `     table_name    `

The name of an existing table.

``` text
SELECT * FROM Roster;
```

#### `     join_operation    `

See [Join operation](#join_types) .

#### `     query_expr    `

`  ( query_expr ) [ [ AS ] alias ]  ` is a [table subquery](/spanner/docs/reference/standard-sql/subqueries#table_subquery_concepts) .

#### `     field_path    `

In the `  FROM  ` clause, `  field_path  ` is any path that resolves to a field within a data type. `  field_path  ` can go arbitrarily deep into a nested data structure.

Some examples of valid `  field_path  ` values include:

``` text
SELECT * FROM T1 t1, t1.array_column;

SELECT * FROM T1 t1, t1.struct_column.array_field;

SELECT (SELECT ARRAY_AGG(c) FROM t1.array_column c) FROM T1 t1;

SELECT a.struct_field1 FROM T1 t1, t1.array_of_structs a;

SELECT (SELECT STRING_AGG(a.struct_field1) FROM t1.array_of_structs a) FROM T1 t1;
```

Field paths in the `  FROM  ` clause must end in an array or a repeated field. In addition, field paths can't contain arrays or repeated fields before the end of the path. For example, the path `  array_column.some_array.some_array_field  ` is invalid because it contains an array before the end of the path.

**Note:** If a path has only one name, it's interpreted as a table. To work around this, wrap the path using `  UNNEST  ` , or use the fully-qualified path.

**Note:** If a path has more than one name, and it matches a field name, it's interpreted as a field name. To force the path to be interpreted as a table name, wrap the path using ``  `  `` .

#### `     unnest_operator    `

See [UNNEST operator](#unnest_operator) .

#### `     cte_name    `

Common table expressions (CTEs) in a [`  WITH  ` Clause](#with_clause) act like temporary tables that you can reference anywhere in the `  FROM  ` clause. In the example below, `  subQ1  ` and `  subQ2  ` are CTEs.

Example:

``` text
WITH
  subQ1 AS (SELECT * FROM Roster WHERE SchoolID = 52),
  subQ2 AS (SELECT SchoolID FROM subQ1)
SELECT DISTINCT * FROM subQ2;
```

## `     UNNEST    ` operator

``` text
unnest_operator:
  {
    UNNEST( array ) [ as_alias ]
    | array_path [ as_alias ]
  }
  [ table_hint_expr ]
  [ WITH OFFSET [ as_alias ] ]

array:
  { array_expression | array_path }

as_alias:
  [AS] alias
```

The `  UNNEST  ` operator takes an array and returns a table with one row for each element in the array. The output of `  UNNEST  ` is one [value table](#value_tables) column. For these `  ARRAY  ` element types, `  SELECT *  ` against the value table column returns multiple columns:

  - `  STRUCT  `
  - `  PROTO  `

Input values:

  - `  array_expression  ` : An expression that produces an array and that's not an array path.

  - `  array_path  ` : The path to an `  ARRAY  ` type.
    
      - In an implicit `  UNNEST  ` operation, the path must start with a [range variable](#range_variables) name.
      - In an explicit `  UNNEST  ` operation, the path can optionally start with a [range variable](#range_variables) name.
    
    The `  UNNEST  ` operation with any [correlated](#correlated_join) `  array_path  ` must be on the right side of a `  CROSS JOIN  ` , `  LEFT JOIN  ` , or `  INNER JOIN  ` operation.

  - `  as_alias  ` : If specified, defines the explicit name of the value table column containing the array element values. It can be used to refer to the column elsewhere in the query.

  - `  WITH OFFSET  ` : `  UNNEST  ` destroys the order of elements in the input array. Use this optional clause to return an additional column with the array element indexes, or *offsets* . Offset counting starts at zero for each row produced by the `  UNNEST  ` operation. This column has an optional alias; If the optional alias isn't used, the default column name is `  offset  ` .
    
    Example:
    
    ``` text
    SELECT * FROM UNNEST ([10,20,30]) as numbers WITH OFFSET;
    
    /*---------+--------+
     | numbers | offset |
     +---------+--------+
     | 10      | 0      |
     | 20      | 1      |
     | 30      | 2      |
     +---------+--------*/
    ```

You can also use `  UNNEST  ` outside of the `  FROM  ` clause with the [`  IN  ` operator](/spanner/docs/reference/standard-sql/operators#in_operators) .

For several ways to use `  UNNEST  ` , including construction, flattening, and filtering, see [Work with arrays](/spanner/docs/reference/standard-sql/arrays) .

To learn more about the ways you can use `  UNNEST  ` explicitly and implicitly, see [Explicit and implicit `  UNNEST  `](#explicit_implicit_unnest) .

### `     UNNEST    ` and structs

For an input array of structs, `  UNNEST  ` returns a row for each struct, with a separate column for each field in the struct. The alias for each column is the name of the corresponding struct field.

Example:

``` text
SELECT *
FROM UNNEST(
  ARRAY<
    STRUCT<
      x INT64,
      y STRING,
      z ARRAY<INT64>>>[
        (1, 'foo', [10, 11]),
        (3, 'bar', [20, 21])]);

/*---+-----+----------+
 | x | y   | z        |
 +---+-----+----------+
 | 1 | foo | {10, 11} |
 | 3 | bar | {20, 21} |
 +---+-----+----------*/
```

### `     UNNEST    ` and protocol buffers

For an input array of protocol buffers, `  UNNEST  ` returns a row for each protocol buffer, with a separate column for each field in the protocol buffer. The alias for each column is the name of the corresponding protocol buffer field.

Example:

``` text
SELECT *
FROM UNNEST(
  ARRAY<googlesql.examples.music.Album>[
    NEW googlesql.examples.music.Album (
      'The Goldberg Variations' AS album_name,
      ['Aria', 'Variation 1', 'Variation 2'] AS song
    )
  ]
);

/*-------------------------+--------+----------------------------------+
 | album_name              | singer | song                             |
 +-------------------------+--------+----------------------------------+
 | The Goldberg Variations | NULL   | [Aria, Variation 1, Variation 2] |
 +-------------------------+--------+----------------------------------*/
```

As with structs, you can alias `  UNNEST  ` to define a range variable. You can reference this alias in the `  SELECT  ` list to return a value table where each row is a protocol buffer element from the array.

``` text
SELECT proto_value
FROM UNNEST(
  ARRAY<googlesql.examples.music.Album>[
    NEW googlesql.examples.music.Album (
      'The Goldberg Variations' AS album_name,
      ['Aria', 'Var. 1'] AS song
    )
  ]
) AS proto_value;

/*---------------------------------------------------------------------+
 | proto_value                                                         |
 +---------------------------------------------------------------------+
 | {album_name: "The Goldberg Variations" song: "Aria" song: "Var. 1"} |
 +---------------------------------------------------------------------*/
```

### Explicit and implicit `     UNNEST    `

Array unnesting can be either explicit or implicit. To learn more, see the following sections.

#### Explicit unnesting

The `  UNNEST  ` keyword is required in explicit unnesting. For example:

``` text
WITH Coordinates AS (SELECT [1,2] AS position)
SELECT results FROM Coordinates, UNNEST(Coordinates.position) AS results;
```

This example and the following examples use the `  array_path  ` called `  Coordinates.position  ` to illustrate unnesting.

#### Implicit unnesting

The `  UNNEST  ` keyword isn't used in implicit unnesting.

For example:

``` text
WITH Coordinates AS (SELECT [1,2] AS position)
SELECT results FROM Coordinates, Coordinates.position AS results;
```

##### Tables and implicit unnesting

When you use `  array_path  ` with implicit `  UNNEST  ` , `  array_path  ` must be prepended with the table. For example:

``` text
WITH Coordinates AS (SELECT [1,2] AS position)
SELECT results FROM Coordinates, Coordinates.position AS results;
```

### `     UNNEST    ` and `     NULL    ` values

`  UNNEST  ` treats `  NULL  ` values as follows:

  - `  NULL  ` and empty arrays produce zero rows.
  - An array containing `  NULL  ` values produces rows containing `  NULL  ` values.

## `     TABLESAMPLE    ` operator

``` text
tablesample_clause:
  TABLESAMPLE sample_method (sample_size percent_or_rows )

sample_method:
  { BERNOULLI | RESERVOIR }

sample_size:
  numeric_value_expression

percent_or_rows:
  { PERCENT | ROWS }
```

**Description**

You can use the `  TABLESAMPLE  ` operator to select a random sample of a dataset. This operator is useful when you're working with tables that have large amounts of data and you don't need precise answers.

  - `  sample_method  ` : When using the `  TABLESAMPLE  ` operator, you must specify the sampling algorithm to use:
      - `  BERNOULLI  ` : Each row is independently selected with the probability given in the `  percent  ` clause. As a result, you get approximately `  N * percent/100  ` rows.
      - `  RESERVOIR  ` : Takes as parameter an actual sample size K (expressed as a number of rows). If the input is smaller than K, it outputs the entire input relation. If the input is larger than K, reservoir sampling outputs a sample of size exactly K, where any sample of size K is equally likely.
  - `  sample_size  ` : The size of the sample.
  - `  percent_or_rows  ` : The `  TABLESAMPLE  ` operator requires that you choose either `  ROWS  ` or `  PERCENT  ` . If you choose `  PERCENT  ` , the value must be between 0 and 100. If you choose `  ROWS  ` , the value must be greater than or equal to 0.

**Examples**

The following examples illustrate the use of the `  TABLESAMPLE  ` operator.

Select from a table using the `  RESERVOIR  ` sampling method:

``` text
SELECT MessageId
FROM Messages TABLESAMPLE RESERVOIR (100 ROWS);
```

Select from a table using the `  BERNOULLI  ` sampling method:

``` text
SELECT MessageId
FROM Messages TABLESAMPLE BERNOULLI (0.1 PERCENT);
```

Use `  TABLESAMPLE  ` with a subquery:

``` text
SELECT Subject FROM
(SELECT MessageId, Subject FROM Messages WHERE ServerId="test")
TABLESAMPLE BERNOULLI(50 PERCENT)
WHERE MessageId > 3;
```

Use a `  TABLESAMPLE  ` operation with a join to another table.

``` text
SELECT S.Subject
FROM
(SELECT MessageId, ThreadId FROM Messages WHERE ServerId="test") AS R
TABLESAMPLE RESERVOIR(5 ROWS),
Threads AS S
WHERE S.ServerId="test" AND R.ThreadId = S.ThreadId;
```

## `     GRAPH_TABLE    ` operator

To learn more about this operator, see [`  GRAPH_TABLE  ` operator](/spanner/docs/reference/standard-sql/graph-sql-queries#graph_table_operator) in the Graph Query Language (GQL) reference guide.

## Join operation

``` text
join_operation:
  { cross_join_operation | condition_join_operation }

cross_join_operation:
  from_item cross_join_operator [ join_hint_expr ] from_item

condition_join_operation:
  from_item condition_join_operator [ join_hint_expr ] from_item join_condition

cross_join_operator:
  { CROSS JOIN | , }

condition_join_operator:
  {
    [INNER] [ join_method ] JOIN
    | FULL [OUTER] [ join_method ] JOIN
    | LEFT [OUTER] [ join_method ] JOIN
    | RIGHT [OUTER] [ join_method ] JOIN
  }

join_method:
  { HASH }

join_condition:
  { on_clause | using_clause }

on_clause:
  ON bool_expression

using_clause:
  USING ( column_list )
```

The `  JOIN  ` operation merges two `  from_item  ` s so that the `  SELECT  ` clause can query them as one source. The join operator and join condition specify how to combine and discard rows from the two `  from_item  ` s to form a single source.

### `     [INNER] JOIN    `

An `  INNER JOIN  ` , or simply `  JOIN  ` , effectively calculates the Cartesian product of the two `  from_item  ` s and discards all rows that don't meet the join condition. *Effectively* means that it's possible to implement an `  INNER JOIN  ` without actually calculating the Cartesian product.

``` text
FROM A INNER JOIN B ON A.w = B.y

/*
Table A       Table B       Result
+-------+     +-------+     +---------------+
| w | x |  *  | y | z |  =  | w | x | y | z |
+-------+     +-------+     +---------------+
| 1 | a |     | 2 | k |     | 2 | b | 2 | k |
| 2 | b |     | 3 | m |     | 3 | c | 3 | m |
| 3 | c |     | 3 | n |     | 3 | c | 3 | n |
| 3 | d |     | 4 | p |     | 3 | d | 3 | m |
+-------+     +-------+     | 3 | d | 3 | n |
                            +---------------+
*/
```

``` text
FROM A INNER JOIN B USING (x)

/*
Table A       Table B       Result
+-------+     +-------+     +-----------+
| x | y |  *  | x | z |  =  | x | y | z |
+-------+     +-------+     +-----------+
| 1 | a |     | 2 | k |     | 2 | b | k |
| 2 | b |     | 3 | m |     | 3 | c | m |
| 3 | c |     | 3 | n |     | 3 | c | n |
| 3 | d |     | 4 | p |     | 3 | d | m |
+-------+     +-------+     | 3 | d | n |
                            +-----------+
*/
```

**Example**

This query performs an `  INNER JOIN  ` on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) tables.

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster JOIN TeamMascot ON Roster.SchoolID = TeamMascot.SchoolID;

/*---------------------------+
 | LastName   | Mascot       |
 +---------------------------+
 | Adams      | Jaguars      |
 | Buchanan   | Lakers       |
 | Coolidge   | Lakers       |
 | Davis      | Knights      |
 +---------------------------*/
```

You can use a [correlated](#correlated_join) `  INNER JOIN  ` to flatten an array into a set of rows. To learn more, see [Convert elements in an array to rows in a table](/spanner/docs/reference/standard-sql/arrays#flattening_arrays) .

### `     CROSS JOIN    `

`  CROSS JOIN  ` returns the Cartesian product of the two `  from_item  ` s. In other words, it combines each row from the first `  from_item  ` with each row from the second `  from_item  ` .

If the rows of the two `  from_item  ` s are independent, then the result has *M \* N* rows, given *M* rows in one `  from_item  ` and *N* in the other. Note that this still holds for the case when either `  from_item  ` has zero rows.

In a `  FROM  ` clause, a `  CROSS JOIN  ` can be written like this:

``` text
FROM A CROSS JOIN B

/*
Table A       Table B       Result
+-------+     +-------+     +---------------+
| w | x |  *  | y | z |  =  | w | x | y | z |
+-------+     +-------+     +---------------+
| 1 | a |     | 2 | c |     | 1 | a | 2 | c |
| 2 | b |     | 3 | d |     | 1 | a | 3 | d |
+-------+     +-------+     | 2 | b | 2 | c |
                            | 2 | b | 3 | d |
                            +---------------+
*/
```

You can use a [correlated](#correlated_join) cross join to convert or flatten an array into a set of rows, though the (equivalent) `  INNER JOIN  ` is preferred over `  CROSS JOIN  ` for this case. To learn more, see [Convert elements in an array to rows in a table](/spanner/docs/reference/standard-sql/arrays#flattening_arrays) .

**Examples**

This query performs an `  CROSS JOIN  ` on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) tables.

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster CROSS JOIN TeamMascot;

/*---------------------------+
 | LastName   | Mascot       |
 +---------------------------+
 | Adams      | Jaguars      |
 | Adams      | Knights      |
 | Adams      | Lakers       |
 | Adams      | Mustangs     |
 | Buchanan   | Jaguars      |
 | Buchanan   | Knights      |
 | Buchanan   | Lakers       |
 | Buchanan   | Mustangs     |
 | ...                       |
 +---------------------------*/
```

### Comma cross join (,)

[`  CROSS JOIN  `](#cross_join) s can be written implicitly with a comma. This is called a comma cross join.

A comma cross join looks like this in a `  FROM  ` clause:

``` text
FROM A, B

/*
Table A       Table B       Result
+-------+     +-------+     +---------------+
| w | x |  *  | y | z |  =  | w | x | y | z |
+-------+     +-------+     +---------------+
| 1 | a |     | 2 | c |     | 1 | a | 2 | c |
| 2 | b |     | 3 | d |     | 1 | a | 3 | d |
+-------+     +-------+     | 2 | b | 2 | c |
                            | 2 | b | 3 | d |
                            +---------------+
*/
```

You can't write comma cross joins inside parentheses. To learn more, see [Join operations in a sequence](#sequences_of_joins) .

``` text
FROM (A, B)  // INVALID
```

You can use a [correlated](#correlated_join) comma cross join to convert or flatten an array into a set of rows. To learn more, see [Convert elements in an array to rows in a table](/spanner/docs/reference/standard-sql/arrays#flattening_arrays) .

**Examples**

This query performs a comma cross join on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) tables.

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster, TeamMascot;

/*---------------------------+
 | LastName   | Mascot       |
 +---------------------------+
 | Adams      | Jaguars      |
 | Adams      | Knights      |
 | Adams      | Lakers       |
 | Adams      | Mustangs     |
 | Buchanan   | Jaguars      |
 | Buchanan   | Knights      |
 | Buchanan   | Lakers       |
 | Buchanan   | Mustangs     |
 | ...                       |
 +---------------------------*/
```

### `     FULL [OUTER] JOIN    `

A `  FULL OUTER JOIN  ` (or simply `  FULL JOIN  ` ) returns all fields for all matching rows in both `  from_items  ` that meet the join condition. If a given row from one `  from_item  ` doesn't join to any row in the other `  from_item  ` , the row returns with `  NULL  ` values for all columns from the other `  from_item  ` .

``` text
FROM A FULL OUTER JOIN B ON A.w = B.y

/*
Table A       Table B       Result
+-------+     +-------+     +---------------------------+
| w | x |  *  | y | z |  =  | w    | x    | y    | z    |
+-------+     +-------+     +---------------------------+
| 1 | a |     | 2 | k |     | 1    | a    | NULL | NULL |
| 2 | b |     | 3 | m |     | 2    | b    | 2    | k    |
| 3 | c |     | 3 | n |     | 3    | c    | 3    | m    |
| 3 | d |     | 4 | p |     | 3    | c    | 3    | n    |
+-------+     +-------+     | 3    | d    | 3    | m    |
                            | 3    | d    | 3    | n    |
                            | NULL | NULL | 4    | p    |
                            +---------------------------+
*/
```

``` text
FROM A FULL OUTER JOIN B USING (x)

/*
Table A       Table B       Result
+-------+     +-------+     +--------------------+
| x | y |  *  | x | z |  =  | x    | y    | z    |
+-------+     +-------+     +--------------------+
| 1 | a |     | 2 | k |     | 1    | a    | NULL |
| 2 | b |     | 3 | m |     | 2    | b    | k    |
| 3 | c |     | 3 | n |     | 3    | c    | m    |
| 3 | d |     | 4 | p |     | 3    | c    | n    |
+-------+     +-------+     | 3    | d    | m    |
                            | 3    | d    | n    |
                            | 4    | NULL | p    |
                            +--------------------+
*/
```

**Example**

This query performs a `  FULL JOIN  ` on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) tables.

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster FULL JOIN TeamMascot ON Roster.SchoolID = TeamMascot.SchoolID;

/*---------------------------+
 | LastName   | Mascot       |
 +---------------------------+
 | Adams      | Jaguars      |
 | Buchanan   | Lakers       |
 | Coolidge   | Lakers       |
 | Davis      | Knights      |
 | Eisenhower | NULL         |
 | NULL       | Mustangs     |
 +---------------------------*/
```

### `     LEFT [OUTER] JOIN    `

The result of a `  LEFT OUTER JOIN  ` (or simply `  LEFT JOIN  ` ) for two `  from_item  ` s always retains all rows of the left `  from_item  ` in the `  JOIN  ` operation, even if no rows in the right `  from_item  ` satisfy the join predicate.

All rows from the *left* `  from_item  ` are retained; if a given row from the left `  from_item  ` doesn't join to any row in the *right* `  from_item  ` , the row will return with `  NULL  ` values for all columns exclusively from the right `  from_item  ` . Rows from the right `  from_item  ` that don't join to any row in the left `  from_item  ` are discarded.

``` text
FROM A LEFT OUTER JOIN B ON A.w = B.y

/*
Table A       Table B       Result
+-------+     +-------+     +---------------------------+
| w | x |  *  | y | z |  =  | w    | x    | y    | z    |
+-------+     +-------+     +---------------------------+
| 1 | a |     | 2 | k |     | 1    | a    | NULL | NULL |
| 2 | b |     | 3 | m |     | 2    | b    | 2    | k    |
| 3 | c |     | 3 | n |     | 3    | c    | 3    | m    |
| 3 | d |     | 4 | p |     | 3    | c    | 3    | n    |
+-------+     +-------+     | 3    | d    | 3    | m    |
                            | 3    | d    | 3    | n    |
                            +---------------------------+
*/
```

``` text
FROM A LEFT OUTER JOIN B USING (x)

/*
Table A       Table B       Result
+-------+     +-------+     +--------------------+
| x | y |  *  | x | z |  =  | x    | y    | z    |
+-------+     +-------+     +--------------------+
| 1 | a |     | 2 | k |     | 1    | a    | NULL |
| 2 | b |     | 3 | m |     | 2    | b    | k    |
| 3 | c |     | 3 | n |     | 3    | c    | m    |
| 3 | d |     | 4 | p |     | 3    | c    | n    |
+-------+     +-------+     | 3    | d    | m    |
                            | 3    | d    | n    |
                            +--------------------+
*/
```

**Example**

This query performs a `  LEFT JOIN  ` on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) tables.

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster LEFT JOIN TeamMascot ON Roster.SchoolID = TeamMascot.SchoolID;

/*---------------------------+
 | LastName   | Mascot       |
 +---------------------------+
 | Adams      | Jaguars      |
 | Buchanan   | Lakers       |
 | Coolidge   | Lakers       |
 | Davis      | Knights      |
 | Eisenhower | NULL         |
 +---------------------------*/
```

### `     RIGHT [OUTER] JOIN    `

The result of a `  RIGHT OUTER JOIN  ` (or simply `  RIGHT JOIN  ` ) for two `  from_item  ` s always retains all rows of the right `  from_item  ` in the `  JOIN  ` operation, even if no rows in the left `  from_item  ` satisfy the join predicate.

All rows from the *right* `  from_item  ` are returned; if a given row from the right `  from_item  ` doesn't join to any row in the *left* `  from_item  ` , the row will return with `  NULL  ` values for all columns exclusively from the left `  from_item  ` . Rows from the left `  from_item  ` that don't join to any row in the right `  from_item  ` are discarded.

``` text
FROM A RIGHT OUTER JOIN B ON A.w = B.y

/*
Table A       Table B       Result
+-------+     +-------+     +---------------------------+
| w | x |  *  | y | z |  =  | w    | x    | y    | z    |
+-------+     +-------+     +---------------------------+
| 1 | a |     | 2 | k |     | 2    | b    | 2    | k    |
| 2 | b |     | 3 | m |     | 3    | c    | 3    | m    |
| 3 | c |     | 3 | n |     | 3    | c    | 3    | n    |
| 3 | d |     | 4 | p |     | 3    | d    | 3    | m    |
+-------+     +-------+     | 3    | d    | 3    | n    |
                            | NULL | NULL | 4    | p    |
                            +---------------------------+
*/
```

``` text
FROM A RIGHT OUTER JOIN B USING (x)

/*
Table A       Table B       Result
+-------+     +-------+     +--------------------+
| x | y |  *  | x | z |  =  | x    | y    | z    |
+-------+     +-------+     +--------------------+
| 1 | a |     | 2 | k |     | 2    | b    | k    |
| 2 | b |     | 3 | m |     | 3    | c    | m    |
| 3 | c |     | 3 | n |     | 3    | c    | n    |
| 3 | d |     | 4 | p |     | 3    | d    | m    |
+-------+     +-------+     | 3    | d    | n    |
                            | 4    | NULL | p    |
                            +--------------------+
*/
```

**Example**

This query performs a `  RIGHT JOIN  ` on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) tables.

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster RIGHT JOIN TeamMascot ON Roster.SchoolID = TeamMascot.SchoolID;

/*---------------------------+
 | LastName   | Mascot       |
 +---------------------------+
 | Adams      | Jaguars      |
 | Buchanan   | Lakers       |
 | Coolidge   | Lakers       |
 | Davis      | Knights      |
 | NULL       | Mustangs     |
 +---------------------------*/
```

### Join conditions

In a [join operation](#join_types) , a join condition helps specify how to combine rows in two `  from_items  ` to form a single source.

The two types of join conditions are the [`  ON  ` clause](#on_clause) and [`  USING  ` clause](#using_clause) . You must use a join condition when you perform a conditional join operation. You can't use a join condition when you perform a cross join operation.

#### `     ON    ` clause

``` text
ON bool_expression
```

**Description**

Given a row from each table, if the `  ON  ` clause evaluates to `  TRUE  ` , the query generates a consolidated row with the result of combining the given rows.

Definitions:

  - `  bool_expression  ` : The boolean expression that specifies the condition for the join. This is frequently a [comparison operation](/spanner/docs/reference/standard-sql/operators#comparison_operators) or logical combination of comparison operators.

Details:

Similarly to `  CROSS JOIN  ` , `  ON  ` produces a column once for each column in each input table.

A `  NULL  ` join condition evaluation is equivalent to a `  FALSE  ` evaluation.

If a column-order sensitive operation such as `  UNION  ` or `  SELECT *  ` is used with the `  ON  ` join condition, the resulting table contains all of the columns from the left input in order, and then all of the columns from the right input in order.

**Examples**

The following examples show how to use the `  ON  ` clause:

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3),
  B AS ( SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4)
SELECT * FROM A INNER JOIN B ON A.x = B.x;

WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3),
  B AS ( SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4)
SELECT A.x, B.x FROM A INNER JOIN B ON A.x = B.x;

/*
Table A   Table B   Result (A.x, B.x)
+---+     +---+     +-------+
| x |  *  | x |  =  | x | x |
+---+     +---+     +-------+
| 1 |     | 2 |     | 2 | 2 |
| 2 |     | 3 |     | 3 | 3 |
| 3 |     | 4 |     +-------+
+---+     +---+
*/
```

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT * FROM A LEFT OUTER JOIN B ON A.x = B.x;

WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT A.x, B.x FROM A LEFT OUTER JOIN B ON A.x = B.x;

/*
Table A    Table B   Result
+------+   +---+     +-------------+
| x    | * | x |  =  | x    | x    |
+------+   +---+     +-------------+
| 1    |   | 2 |     | 1    | NULL |
| 2    |   | 3 |     | 2    | 2    |
| 3    |   | 4 |     | 3    | 3    |
| NULL |   | 5 |     | NULL | NULL |
+------+   +---+     +-------------+
*/
```

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT * FROM A FULL OUTER JOIN B ON A.x = B.x;

WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT A.x, B.x FROM A FULL OUTER JOIN B ON A.x = B.x;

/*
Table A    Table B   Result
+------+   +---+     +-------------+
| x    | * | x |  =  | x    | x    |
+------+   +---+     +-------------+
| 1    |   | 2 |     | 1    | NULL |
| 2    |   | 3 |     | 2    | 2    |
| 3    |   | 4 |     | 3    | 3    |
| NULL |   | 5 |     | NULL | NULL |
+------+   +---+     | NULL | 4    |
                     | NULL | 5    |
                     +-------------+
*/
```

#### `     USING    ` clause

``` text
USING ( column_name_list )

column_name_list:
    column_name[, ...]
```

**Description**

When you are joining two tables, `  USING  ` performs an [equality comparison operation](/spanner/docs/reference/standard-sql/operators#comparison_operators) on the columns named in `  column_name_list  ` . Each column name in `  column_name_list  ` must appear in both input tables. For each pair of rows from the input tables, if the equality comparisons all evaluate to `  TRUE  ` , one row is added to the resulting column.

Definitions:

  - `  column_name_list  ` : A list of columns to include in the join condition.
  - `  column_name  ` : The column that exists in both of the tables that you are joining.

Details:

A `  NULL  ` join condition evaluation is equivalent to a `  FALSE  ` evaluation.

If a column-order sensitive operation such as `  UNION  ` or `  SELECT *  ` is used with the `  USING  ` join condition, the resulting table contains columns in this order:

  - The columns from `  column_name_list  ` in the order they appear in the `  USING  ` clause.
  - All other columns of the left input in the order they appear in the input.
  - All other columns of the right input in the order they appear in the input.

A column name in the `  USING  ` clause must not be qualified by a table name.

If the join is an `  INNER JOIN  ` or a `  LEFT OUTER JOIN  ` , the output columns are populated from the values in the first table. If the join is a `  RIGHT OUTER JOIN  ` , the output columns are populated from the values in the second table. If the join is a `  FULL OUTER JOIN  ` , the output columns are populated by [coalescing](/spanner/docs/reference/standard-sql/conditional_expressions#coalesce) the values from the left and right tables in that order.

**Examples**

The following example shows how to use the `  USING  ` clause with one column name in the column name list:

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 9 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 9 UNION ALL SELECT 9 UNION ALL SELECT 5)
SELECT * FROM A INNER JOIN B USING (x);

/*
Table A    Table B   Result
+------+   +---+     +---+
| x    | * | x |  =  | x |
+------+   +---+     +---+
| 1    |   | 2 |     | 2 |
| 2    |   | 9 |     | 9 |
| 9    |   | 9 |     | 9 |
| NULL |   | 5 |     +---+
+------+   +---+
*/
```

The following example shows how to use the `  USING  ` clause with multiple column names in the column name list:

``` text
WITH
  A AS (
    SELECT 1 as x, 15 as y UNION ALL
    SELECT 2, 10 UNION ALL
    SELECT 9, 16 UNION ALL
    SELECT NULL, 12),
  B AS (
    SELECT 2 as x, 10 as y UNION ALL
    SELECT 9, 17 UNION ALL
    SELECT 9, 16 UNION ALL
    SELECT 5, 15)
SELECT * FROM A INNER JOIN B USING (x, y);

/*
Table A         Table B        Result
+-----------+   +---------+     +---------+
| x    | y  | * | x  | y  |  =  | x  | y  |
+-----------+   +---------+     +---------+
| 1    | 15 |   | 2  | 10 |     | 2  | 10 |
| 2    | 10 |   | 9  | 17 |     | 9  | 16 |
| 9    | 16 |   | 9  | 16 |     +---------+
| NULL | 12 |   | 5  | 15 |
+-----------+   +---------+
*/
```

The following examples show additional ways in which to use the `  USING  ` clause with one column name in the column name list:

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 9 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 9 UNION ALL SELECT 9 UNION ALL SELECT 5)
SELECT x, A.x, B.x FROM A INNER JOIN B USING (x)

/*
Table A    Table B   Result
+------+   +---+     +--------------------+
| x    | * | x |  =  | x    | A.x  | B.x  |
+------+   +---+     +--------------------+
| 1    |   | 2 |     | 2    | 2    | 2    |
| 2    |   | 9 |     | 9    | 9    | 9    |
| 9    |   | 9 |     | 9    | 9    | 9    |
| NULL |   | 5 |     +--------------------+
+------+   +---+
*/
```

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 9 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 9 UNION ALL SELECT 9 UNION ALL SELECT 5)
SELECT x, A.x, B.x FROM A LEFT OUTER JOIN B USING (x)

/*
Table A    Table B   Result
+------+   +---+     +--------------------+
| x    | * | x |  =  | x    | A.x  | B.x  |
+------+   +---+     +--------------------+
| 1    |   | 2 |     | 1    | 1    | NULL |
| 2    |   | 9 |     | 2    | 2    | 2    |
| 9    |   | 9 |     | 9    | 9    | 9    |
| NULL |   | 5 |     | 9    | 9    | 9    |
+------+   +---+     | NULL | NULL | NULL |
                     +--------------------+
*/
```

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 2 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 9 UNION ALL SELECT 9 UNION ALL SELECT 5)
SELECT x, A.x, B.x FROM A RIGHT OUTER JOIN B USING (x)

/*
Table A    Table B   Result
+------+   +---+     +--------------------+
| x    | * | x |  =  | x    | A.x  | B.x  |
+------+   +---+     +--------------------+
| 1    |   | 2 |     | 2    | 2    | 2    |
| 2    |   | 9 |     | 2    | 2    | 2    |
| 2    |   | 9 |     | 9    | NULL | 9    |
| NULL |   | 5 |     | 9    | NULL | 9    |
+------+   +---+     | 5    | NULL | 5    |
                     +--------------------+
*/
```

``` text
WITH
  A AS ( SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 2 UNION ALL SELECT NULL),
  B AS ( SELECT 2 as x UNION ALL SELECT 9 UNION ALL SELECT 9 UNION ALL SELECT 5)
SELECT x, A.x, B.x FROM A FULL OUTER JOIN B USING (x);

/*
Table A    Table B   Result
+------+   +---+     +--------------------+
| x    | * | x |  =  | x    | A.x  | B.x  |
+------+   +---+     +--------------------+
| 1    |   | 2 |     | 1    | 1    | NULL |
| 2    |   | 9 |     | 2    | 2    | 2    |
| 2    |   | 9 |     | 2    | 2    | 2    |
| NULL |   | 5 |     | NULL | NULL | NULL |
+------+   +---+     | 9    | NULL | 9    |
                     | 9    | NULL | 9    |
                     | 5    | NULL | 5    |
                     +--------------------+
*/
```

The following example shows how to use the `  USING  ` clause with only some column names in the column name list.

``` text
WITH
  A AS (
    SELECT 1 as x, 15 as y UNION ALL
    SELECT 2, 10 UNION ALL
    SELECT 9, 16 UNION ALL
    SELECT NULL, 12),
  B AS (
    SELECT 2 as x, 10 as y UNION ALL
    SELECT 9, 17 UNION ALL
    SELECT 9, 16 UNION ALL
    SELECT 5, 15)
SELECT * FROM A INNER JOIN B USING (x);

/*
Table A         Table B         Result
+-----------+   +---------+     +-----------------+
| x    | y  | * | x  | y  |  =  | x   | A.y | B.y |
+-----------+   +---------+     +-----------------+
| 1    | 15 |   | 2  | 10 |     | 2   | 10  | 10  |
| 2    | 10 |   | 9  | 17 |     | 9   | 16  | 17  |
| 9    | 16 |   | 9  | 16 |     | 9   | 16  | 16  |
| NULL | 12 |   | 5  | 15 |     +-----------------+
+-----------+   +---------+
*/
```

The following query performs an `  INNER JOIN  ` on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) table. The query returns the rows from `  Roster  ` and `  TeamMascot  ` where `  Roster.SchoolID  ` is the same as `  TeamMascot.SchoolID  ` . The results include a single `  SchoolID  ` column.

``` text
SELECT * FROM Roster INNER JOIN TeamMascot USING (SchoolID);

/*----------------------------------------+
 | SchoolID   | LastName   | Mascot       |
 +----------------------------------------+
 | 50         | Adams      | Jaguars      |
 | 52         | Buchanan   | Lakers       |
 | 52         | Coolidge   | Lakers       |
 | 51         | Davis      | Knights      |
 +----------------------------------------*/
```

#### `     ON    ` and `     USING    ` equivalency

The [`  ON  `](#on_clause) and [`  USING  `](#using_clause) join conditions aren't equivalent, but they share some rules and sometimes they can produce similar results.

In the following examples, observe what is returned when all rows are produced for inner and outer joins. Also, look at how each join condition handles `  NULL  ` values.

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4)
SELECT * FROM A INNER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4)
SELECT * FROM A INNER JOIN B USING (x);

/*
Table A   Table B   Result ON     Result USING
+---+     +---+     +-------+     +---+
| x |  *  | x |  =  | x | x |     | x |
+---+     +---+     +-------+     +---+
| 1 |     | 2 |     | 2 | 2 |     | 2 |
| 2 |     | 3 |     | 3 | 3 |     | 3 |
| 3 |     | 4 |     +-------+     +---+
+---+     +---+
*/
```

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT * FROM A LEFT OUTER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT * FROM A LEFT OUTER JOIN B USING (x);

/*
Table A    Table B   Result ON           Result USING
+------+   +---+     +-------------+     +------+
| x    | * | x |  =  | x    | x    |     | x    |
+------+   +---+     +-------------+     +------+
| 1    |   | 2 |     | 1    | NULL |     | 1    |
| 2    |   | 3 |     | 2    | 2    |     | 2    |
| 3    |   | 4 |     | 3    | 3    |     | 3    |
| NULL |   | 5 |     | NULL | NULL |     | NULL |
+------+   +---+     +-------------+     +------+
*/
```

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4)
SELECT * FROM A FULL OUTER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4)
SELECT * FROM A FULL OUTER JOIN B USING (x);

/*
Table A   Table B   Result ON           Result USING
+---+     +---+     +-------------+     +---+
| x |  *  | x |  =  | x    | x    |     | x |
+---+     +---+     +-------------+     +---+
| 1 |     | 2 |     | 1    | NULL |     | 1 |
| 2 |     | 3 |     | 2    | 2    |     | 2 |
| 3 |     | 4 |     | 3    | 3    |     | 3 |
+---+     +---+     | NULL | 4    |     | 4 |
                    +-------------+     +---+
*/
```

Although `  ON  ` and `  USING  ` aren't equivalent, they can return the same results in some situations if you specify the columns you want to return.

In the following examples, observe what is returned when a specific row is produced for inner and outer joins. Also, look at how each join condition handles `  NULL  ` values.

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT A.x FROM A INNER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT x FROM A INNER JOIN B USING (x);

/*
Table A    Table B   Result ON     Result USING
+------+   +---+     +---+         +---+
| x    | * | x |  =  | x |         | x |
+------+   +---+     +---+         +---+
| 1    |   | 2 |     | 2 |         | 2 |
| 2    |   | 3 |     | 3 |         | 3 |
| 3    |   | 4 |     +---+         +---+
| NULL |   | 5 |
+------+   +---+
*/
```

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT A.x FROM A LEFT OUTER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT x FROM A LEFT OUTER JOIN B USING (x);

/*
Table A    Table B   Result ON    Result USING
+------+   +---+     +------+     +------+
| x    | * | x |  =  | x    |     | x    |
+------+   +---+     +------+     +------+
| 1    |   | 2 |     | 1    |     | 1    |
| 2    |   | 3 |     | 2    |     | 2    |
| 3    |   | 4 |     | 3    |     | 3    |
| NULL |   | 5 |     | NULL |     | NULL |
+------+   +---+     +------+     +------+
*/
```

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT A.x FROM A FULL OUTER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT x FROM A FULL OUTER JOIN B USING (x);

/*
Table A    Table B   Result ON    Result USING
+------+   +---+     +------+     +------+
| x    | * | x |  =  | x    |     | x    |
+------+   +---+     +------+     +------+
| 1    |   | 2 |     | 1    |     | 1    |
| 2    |   | 3 |     | 2    |     | 2    |
| 3    |   | 4 |     | 3    |     | 3    |
| NULL |   | 5 |     | NULL |     | NULL |
+------+   +---+     | NULL |     | 4    |
                     | NULL |     | 5    |
                     +------+     +------+
*/
```

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT B.x FROM A FULL OUTER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT x FROM A FULL OUTER JOIN B USING (x);

/*
Table A    Table B   Result ON    Result USING
+------+   +---+     +------+     +------+
| x    | * | x |  =  | x    |     | x    |
+------+   +---+     +------+     +------+
| 1    |   | 2 |     | 2    |     | 1    |
| 2    |   | 3 |     | 3    |     | 2    |
| 3    |   | 4 |     | NULL |     | 3    |
| NULL |   | 5 |     | NULL |     | NULL |
+------+   +---+     | 4    |     | 4    |
                     | 5    |     | 5    |
                     +------+     +------+
*/
```

In the following example, observe what is returned when `  COALESCE  ` is used with the `  ON  ` clause. It provides the same results as a query with the `  USING  ` clause.

``` text
WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT COALESCE(A.x, B.x) FROM A FULL OUTER JOIN B ON A.x = B.x;

WITH
  A AS (SELECT 1 as x UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT NULL),
  B AS (SELECT 2 as x UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5)
SELECT x FROM A FULL OUTER JOIN B USING (x);

/*
Table A    Table B   Result ON    Result USING
+------+   +---+     +------+     +------+
| x    | * | x |  =  | x    |     | x    |
+------+   +---+     +------+     +------+
| 1    |   | 2 |     | 1    |     | 1    |
| 2    |   | 3 |     | 2    |     | 2    |
| 3    |   | 4 |     | 3    |     | 3    |
| NULL |   | 5 |     | NULL |     | NULL |
+------+   +---+     | 4    |     | 4    |
                     | 5    |     | 5    |
                     +------+     +------+
*/
```

### Join operations in a sequence

The `  FROM  ` clause can contain multiple `  JOIN  ` operations in a sequence. `  JOIN  ` s are bound from left to right. For example:

``` text
FROM A JOIN B USING (x) JOIN C USING (x)

-- A JOIN B USING (x)        = result_1
-- result_1 JOIN C USING (x) = result_2
-- result_2                  = return value
```

You can also insert parentheses to group `  JOIN  ` s:

``` text
FROM ( (A JOIN B USING (x)) JOIN C USING (x) )

-- A JOIN B USING (x)        = result_1
-- result_1 JOIN C USING (x) = result_2
-- result_2                  = return value
```

With parentheses, you can group `  JOIN  ` s so that they are bound in a different order:

``` text
FROM ( A JOIN (B JOIN C USING (x)) USING (x) )

-- B JOIN C USING (x)       = result_1
-- A JOIN result_1          = result_2
-- result_2                 = return value
```

When comma cross joins are present in a query with a sequence of JOINs, they group from left to right like other `  JOIN  ` types:

``` text
FROM A JOIN B USING (x) JOIN C USING (x), D

-- A JOIN B USING (x)        = result_1
-- result_1 JOIN C USING (x) = result_2
-- result_2 CROSS JOIN D     = return value
```

There can't be a `  RIGHT JOIN  ` or `  FULL JOIN  ` after a comma cross join unless it's parenthesized:

``` text
FROM A, B RIGHT JOIN C ON TRUE // INVALID
```

``` text
FROM A, B FULL JOIN C ON TRUE  // INVALID
```

``` text
FROM A, B JOIN C ON TRUE       // VALID
```

``` text
FROM A, (B RIGHT JOIN C ON TRUE) // VALID
```

``` text
FROM A, (B FULL JOIN C ON TRUE)  // VALID
```

### Correlated join operation

A join operation is *correlated* when the right `  from_item  ` contains a reference to at least one range variable or column name introduced by the left `  from_item  ` .

In a correlated join operation, rows from the right `  from_item  ` are determined by a row from the left `  from_item  ` . Consequently, `  RIGHT OUTER  ` and `  FULL OUTER  ` joins can't be correlated because right `  from_item  ` rows can't be determined in the case when there is no row from the left `  from_item  ` .

All correlated join operations must reference an array in the right `  from_item  ` .

This is a conceptual example of a correlated join operation that includes a [correlated subquery](/spanner/docs/reference/standard-sql/subqueries#correlated_subquery_concepts) :

``` text
FROM A JOIN UNNEST(ARRAY(SELECT AS STRUCT * FROM B WHERE A.ID = B.ID)) AS C
```

  - Left `  from_item  ` : `  A  `
  - Right `  from_item  ` : `  UNNEST(...) AS C  `
  - A correlated subquery: `  (SELECT AS STRUCT * FROM B WHERE A.ID = B.ID)  `

This is another conceptual example of a correlated join operation. `  array_of_IDs  ` is part of the left `  from_item  ` but is referenced in the right `  from_item  ` .

``` text
FROM A JOIN UNNEST(A.array_of_IDs) AS C
```

The [`  UNNEST  ` operator](#unnest_operator) can be explicit or implicit. These are both allowed:

``` text
FROM A JOIN UNNEST(A.array_of_IDs) AS IDs
```

``` text
FROM A JOIN A.array_of_IDs AS IDs
```

In a correlated join operation, the right `  from_item  ` is re-evaluated against each distinct row from the left `  from_item  ` . In the following conceptual example, the correlated join operation first evaluates `  A  ` and `  B  ` , then `  A  ` and `  C  ` :

``` text
FROM
  A
  JOIN
  UNNEST(ARRAY(SELECT AS STRUCT * FROM B WHERE A.ID = B.ID)) AS C
  ON A.Name = C.Name
```

**Caveats**

  - In a correlated `  LEFT JOIN  ` , when the input table on the right side is empty for some row from the left side, it's as if no rows from the right side satisfied the join condition in a regular `  LEFT JOIN  ` . When there are no joining rows, a row with `  NULL  ` values for all columns on the right side is generated to join with the row from the left side.
  - In a correlated `  CROSS JOIN  ` , when the input table on the right side is empty for some row from the left side, it's as if no rows from the right side satisfied the join condition in a regular correlated `  INNER JOIN  ` . This means that the row is dropped from the results.

**Examples**

This is an example of a correlated join, using the [Roster](#roster_table) and [PlayerStats](#playerstats_table) tables:

``` text
SELECT *
FROM
  Roster
JOIN
  UNNEST(
    ARRAY(
      SELECT AS STRUCT *
      FROM PlayerStats
      WHERE PlayerStats.OpponentID = Roster.SchoolID
    )) AS PlayerMatches
  ON PlayerMatches.LastName = 'Buchanan'

/*------------+----------+----------+------------+--------------+
 | LastName   | SchoolID | LastName | OpponentID | PointsScored |
 +------------+----------+----------+------------+--------------+
 | Adams      | 50       | Buchanan | 50         | 13           |
 | Eisenhower | 77       | Buchanan | 77         | 0            |
 +------------+----------+----------+------------+--------------*/
```

A common pattern for a correlated `  LEFT JOIN  ` is to have an `  UNNEST  ` operation on the right side that references an array from some column introduced by input on the left side. For rows where that array is empty or `  NULL  ` , the `  UNNEST  ` operation produces no rows on the right input. In that case, a row with a `  NULL  ` entry in each column of the right input is created to join with the row from the left input. For example:

``` text
SELECT A.name, item, ARRAY_LENGTH(A.items) item_count_for_name
FROM
  UNNEST(
    [
      STRUCT(
        'first' AS name,
        [1, 2, 3, 4] AS items),
      STRUCT(
        'second' AS name,
        [] AS items)]) AS A
LEFT JOIN
  A.items AS item;

/*--------+------+---------------------+
 | name   | item | item_count_for_name |
 +--------+------+---------------------+
 | first  | 1    | 4                   |
 | first  | 2    | 4                   |
 | first  | 3    | 4                   |
 | first  | 4    | 4                   |
 | second | NULL | 0                   |
 +--------+------+---------------------*/
```

In the case of a correlated `  INNER JOIN  ` or `  CROSS JOIN  ` , when the input on the right side is empty for some row from the left side, the final row is dropped from the results. For example:

``` text
SELECT A.name, item
FROM
  UNNEST(
    [
      STRUCT(
        'first' AS name,
        [1, 2, 3, 4] AS items),
      STRUCT(
        'second' AS name,
        [] AS items)]) AS A
INNER JOIN
  A.items AS item;

/*-------+------+
 | name  | item |
 +-------+------+
 | first | 1    |
 | first | 2    |
 | first | 3    |
 | first | 4    |
 +-------+------*/
```

## `     WHERE    ` clause

``` text
WHERE bool_expression
```

The `  WHERE  ` clause filters the results of the `  FROM  ` clause.

Only rows whose `  bool_expression  ` evaluates to `  TRUE  ` are included. Rows whose `  bool_expression  ` evaluates to `  NULL  ` or `  FALSE  ` are discarded.

The evaluation of a query with a `  WHERE  ` clause is typically completed in this order:

  - `  FROM  `
  - `  WHERE  `
  - `  GROUP BY  ` and aggregation
  - `  HAVING  `
  - `  DISTINCT  `
  - `  ORDER BY  `
  - `  LIMIT  `

Evaluation order doesn't always match syntax order.

The `  WHERE  ` clause only references columns available via the `  FROM  ` clause; it can't reference `  SELECT  ` list aliases.

**Examples**

This query returns returns all rows from the [`  Roster  `](#roster_table) table where the `  SchoolID  ` column has the value `  52  ` :

``` text
SELECT * FROM Roster
WHERE SchoolID = 52;
```

The `  bool_expression  ` can contain multiple sub-conditions:

``` text
SELECT * FROM Roster
WHERE STARTS_WITH(LastName, "Mc") OR STARTS_WITH(LastName, "Mac");
```

Expressions in an `  INNER JOIN  ` have an equivalent expression in the `  WHERE  ` clause. For example, a query using `  INNER  ` `  JOIN  ` and `  ON  ` has an equivalent expression using `  CROSS JOIN  ` and `  WHERE  ` . For example, the following two queries are equivalent:

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster INNER JOIN TeamMascot
ON Roster.SchoolID = TeamMascot.SchoolID;
```

``` text
SELECT Roster.LastName, TeamMascot.Mascot
FROM Roster CROSS JOIN TeamMascot
WHERE Roster.SchoolID = TeamMascot.SchoolID;
```

## `     GROUP BY    ` clause

``` text
GROUP [ group_hint_expr ] BY groupable_items
```

**Description**

The `  GROUP BY  ` clause groups together rows in a table that share common values for certain columns. For a group of rows in the source table with non-distinct values, the `  GROUP BY  ` clause aggregates them into a single combined row. This clause is commonly used when aggregate functions are present in the `  SELECT  ` list, or to eliminate redundancy in the output.

**Definitions**

  - `  group_hint_expr  ` : Hints that you can apply to the `  GROUP BY  ` clause. To learn more, see [Group hints](#group_hints) .
  - `  groupable_items  ` : Group rows in a table that share common values for certain columns. To learn more, see [Group rows by groupable items](#group_by_grouping_item) .

### Group rows by groupable items

``` text
GROUP BY groupable_item[, ...]

groupable_item:
  {
    value
    | value_alias
    | column_ordinal
  }
```

**Description**

The `  GROUP BY  ` clause can include [groupable](/spanner/docs/reference/standard-sql/data-types#data_type_properties) expressions and their ordinals.

**Definitions**

  - `  value  ` : An expression that represents a non-distinct, groupable value. To learn more, see [Group rows by values](#group_by_values) .
  - `  value_alias  ` : An alias for `  value  ` . To learn more, see [Group rows by values](#group_by_values) .
  - `  column_ordinal  ` : An `  INT64  ` value that represents the ordinal assigned to a groupable expression in the `  SELECT  ` list. To learn more, see [Group rows by column ordinals](#group_by_col_ordinals) .

#### Group rows by values

The `  GROUP BY  ` clause can group rows in a table with non-distinct values in the `  GROUP BY  ` clause. For example:

``` text
WITH PlayerStats AS (
  SELECT 'Adams' as LastName, 'Noam' as FirstName, 3 as PointsScored UNION ALL
  SELECT 'Buchanan', 'Jie', 0 UNION ALL
  SELECT 'Coolidge', 'Kiran', 1 UNION ALL
  SELECT 'Adams', 'Noam', 4 UNION ALL
  SELECT 'Buchanan', 'Jie', 13)
SELECT SUM(PointsScored) AS total_points, LastName
FROM PlayerStats
GROUP BY LastName;

/*--------------+----------+
 | total_points | LastName |
 +--------------+----------+
 | 7            | Adams    |
 | 13           | Buchanan |
 | 1            | Coolidge |
 +--------------+----------*/
```

`  GROUP BY  ` clauses may also refer to aliases. If a query contains aliases in the `  SELECT  ` clause, those aliases override names in the corresponding `  FROM  ` clause. For example:

``` text
WITH PlayerStats AS (
  SELECT 'Adams' as LastName, 'Noam' as FirstName, 3 as PointsScored UNION ALL
  SELECT 'Buchanan', 'Jie', 0 UNION ALL
  SELECT 'Coolidge', 'Kiran', 1 UNION ALL
  SELECT 'Adams', 'Noam', 4 UNION ALL
  SELECT 'Buchanan', 'Jie', 13)
SELECT SUM(PointsScored) AS total_points, LastName AS last_name
FROM PlayerStats
GROUP BY last_name;

/*--------------+-----------+
 | total_points | last_name |
 +--------------+-----------+
 | 7            | Adams     |
 | 13           | Buchanan  |
 | 1            | Coolidge  |
 +--------------+-----------*/
```

To learn more about the data types that are supported for values in the `  GROUP BY  ` clause, see [Groupable data types](/spanner/docs/reference/standard-sql/data-types#data_type_properties) .

#### Group rows by column ordinals

The `  GROUP BY  ` clause can refer to expression names in the `  SELECT  ` list. The `  GROUP BY  ` clause also allows ordinal references to expressions in the `  SELECT  ` list, using integer values. `  1  ` refers to the first value in the `  SELECT  ` list, `  2  ` the second, and so forth. The value list can combine ordinals and value names. The following queries are equivalent:

``` text
WITH PlayerStats AS (
  SELECT 'Adams' as LastName, 'Noam' as FirstName, 3 as PointsScored UNION ALL
  SELECT 'Buchanan', 'Jie', 0 UNION ALL
  SELECT 'Coolidge', 'Kiran', 1 UNION ALL
  SELECT 'Adams', 'Noam', 4 UNION ALL
  SELECT 'Buchanan', 'Jie', 13)
SELECT SUM(PointsScored) AS total_points, LastName, FirstName
FROM PlayerStats
GROUP BY LastName, FirstName;

/*--------------+----------+-----------+
 | total_points | LastName | FirstName |
 +--------------+----------+-----------+
 | 7            | Adams    | Noam      |
 | 13           | Buchanan | Jie       |
 | 1            | Coolidge | Kiran     |
 +--------------+----------+-----------*/
```

``` text
WITH PlayerStats AS (
  SELECT 'Adams' as LastName, 'Noam' as FirstName, 3 as PointsScored UNION ALL
  SELECT 'Buchanan', 'Jie', 0 UNION ALL
  SELECT 'Coolidge', 'Kiran', 1 UNION ALL
  SELECT 'Adams', 'Noam', 4 UNION ALL
  SELECT 'Buchanan', 'Jie', 13)
SELECT SUM(PointsScored) AS total_points, LastName, FirstName
FROM PlayerStats
GROUP BY 2, 3;

/*--------------+----------+-----------+
 | total_points | LastName | FirstName |
 +--------------+----------+-----------+
 | 7            | Adams    | Noam      |
 | 13           | Buchanan | Jie       |
 | 1            | Coolidge | Kiran     |
 +--------------+----------+-----------*/
```

## `     HAVING    ` clause

``` text
HAVING bool_expression
```

The `  HAVING  ` clause filters the results produced by `  GROUP BY  ` or aggregation. `  GROUP BY  ` or aggregation must be present in the query. If aggregation is present, the `  HAVING  ` clause is evaluated once for every aggregated row in the result set.

Only rows whose `  bool_expression  ` evaluates to `  TRUE  ` are included. Rows whose `  bool_expression  ` evaluates to `  NULL  ` or `  FALSE  ` are discarded.

The evaluation of a query with a `  HAVING  ` clause is typically completed in this order:

  - `  FROM  `
  - `  WHERE  `
  - `  GROUP BY  ` and aggregation
  - `  HAVING  `
  - `  DISTINCT  `
  - `  ORDER BY  `
  - `  LIMIT  `

Evaluation order doesn't always match syntax order.

The `  HAVING  ` clause references columns available via the `  FROM  ` clause, as well as `  SELECT  ` list aliases. Expressions referenced in the `  HAVING  ` clause must either appear in the `  GROUP BY  ` clause or they must be the result of an aggregate function:

``` text
SELECT LastName
FROM Roster
GROUP BY LastName
HAVING SUM(PointsScored) > 15;
```

If a query contains aliases in the `  SELECT  ` clause, those aliases override names in a `  FROM  ` clause.

``` text
SELECT LastName, SUM(PointsScored) AS ps
FROM Roster
GROUP BY LastName
HAVING ps > 0;
```

### Mandatory aggregation

Aggregation doesn't have to be present in the `  HAVING  ` clause itself, but aggregation must be present in at least one of the following forms:

#### Aggregation function in the `     SELECT    ` list.

``` text
SELECT LastName, SUM(PointsScored) AS total
FROM PlayerStats
GROUP BY LastName
HAVING total > 15;
```

#### Aggregation function in the `     HAVING    ` clause.

``` text
SELECT LastName
FROM PlayerStats
GROUP BY LastName
HAVING SUM(PointsScored) > 15;
```

#### Aggregation in both the `     SELECT    ` list and `     HAVING    ` clause.

When aggregation functions are present in both the `  SELECT  ` list and `  HAVING  ` clause, the aggregation functions and the columns they reference don't need to be the same. In the example below, the two aggregation functions, `  COUNT()  ` and `  SUM()  ` , are different and also use different columns.

``` text
SELECT LastName, COUNT(*)
FROM PlayerStats
GROUP BY LastName
HAVING SUM(PointsScored) > 15;
```

## `     ORDER BY    ` clause

``` text
ORDER BY expression
  [COLLATE collation_specification]
  [{ ASC | DESC }]
  [, ...]

collation_specification:
  language_tag[:collation_attribute]
```

The `  ORDER BY  ` clause specifies a column or expression as the sort criterion for the result set. If an `  ORDER BY  ` clause isn't present, the order of the results of a query isn't defined. Column aliases from a `  FROM  ` clause or `  SELECT  ` list are allowed. If a query contains aliases in the `  SELECT  ` clause, those aliases override names in the corresponding `  FROM  ` clause. The data type of `  expression  ` must be [orderable](/spanner/docs/reference/standard-sql/data-types#orderable_data_types) .

**Optional Clauses**

  - `  COLLATE  ` : You can use the `  COLLATE  ` clause to refine how data is ordered by an `  ORDER BY  ` clause. *Collation* refers to a set of rules that determine how strings are compared according to the conventions and standards of a particular written language, region, or country. These rules might define the correct character sequence, with options for specifying case-insensitivity. You can use `  COLLATE  ` only on columns of type `  STRING  ` .
    
    `  collation_specification  ` represents the collation specification for the `  COLLATE  ` clause. The collation specification can be a string literal or a query parameter. To learn more see [collation specification details](/spanner/docs/reference/standard-sql/collation-concepts#collate_spec_details) .

  - `  ASC | DESC  ` : Sort the results in ascending or descending order of `  expression  ` values. `  ASC  ` is the default value.

**Examples**

Use the default sort order (ascending).

``` text
SELECT x, y
FROM (SELECT 1 AS x, true AS y UNION ALL
      SELECT 9, true)
ORDER BY x;

/*------+-------+
 | x    | y     |
 +------+-------+
 | 1    | true  |
 | 9    | true  |
 +------+-------*/
```

Use descending sort order.

``` text
SELECT x, y
FROM (SELECT 1 AS x, true AS y UNION ALL
      SELECT 9, true)
ORDER BY x DESC;

/*------+-------+
 | x    | y     |
 +------+-------+
 | 9    | true  |
 | 1    | true  |
 +------+-------*/
```

It's possible to order by multiple columns. In the example below, the result set is ordered first by `  SchoolID  ` and then by `  LastName  ` :

``` text
SELECT LastName, PointsScored, OpponentID
FROM PlayerStats
ORDER BY SchoolID, LastName;
```

When used in conjunction with [set operators](#set_operators) , the `  ORDER BY  ` clause applies to the result set of the entire query; it doesn't apply only to the closest `  SELECT  ` statement. For this reason, it can be helpful (though it isn't required) to use parentheses to show the scope of the `  ORDER BY  ` .

This query without parentheses:

``` text
SELECT * FROM Roster
UNION ALL
SELECT * FROM TeamMascot
ORDER BY SchoolID;
```

is equivalent to this query with parentheses:

``` text
( SELECT * FROM Roster
  UNION ALL
  SELECT * FROM TeamMascot )
ORDER BY SchoolID;
```

but isn't equivalent to this query, where the `  ORDER BY  ` clause applies only to the second `  SELECT  ` statement:

``` text
SELECT * FROM Roster
UNION ALL
( SELECT * FROM TeamMascot
  ORDER BY SchoolID );
```

You can also use integer literals as column references in `  ORDER BY  ` clauses. An integer literal becomes an ordinal (for example, counting starts at 1) into the `  SELECT  ` list.

Example - the following two queries are equivalent:

``` text
SELECT SUM(PointsScored), LastName
FROM PlayerStats
GROUP BY LastName
ORDER BY LastName;
```

``` text
SELECT SUM(PointsScored), LastName
FROM PlayerStats
GROUP BY 2
ORDER BY 2;
```

Collate results using English - Canada:

``` text
SELECT Place
FROM Locations
ORDER BY Place COLLATE "en_CA"
```

Collate results using a parameter:

``` text
#@collate_param = "arg_EG"
SELECT Place
FROM Locations
ORDER BY Place COLLATE @collate_param
```

Using multiple `  COLLATE  ` clauses in a statement:

``` text
SELECT APlace, BPlace, CPlace
FROM Locations
ORDER BY APlace COLLATE "en_US" ASC,
         BPlace COLLATE "ar_EG" DESC,
         CPlace COLLATE "en" DESC
```

Case insensitive collation:

``` text
SELECT Place
FROM Locations
ORDER BY Place COLLATE "en_US:ci"
```

Default Unicode case-insensitive collation:

``` text
SELECT Place
FROM Locations
ORDER BY Place COLLATE "und:ci"
```

## Set operators

``` text
  query_expr
  
  {
    UNION { ALL | DISTINCT } |
    INTERSECT { ALL | DISTINCT } |
    EXCEPT { ALL | DISTINCT }
  }
  
  query_expr
```

Set operators combine or filter results from two or more input queries into a single result set.

**Definitions**

  - `  query_expr  ` : One of two input queries whose results are combined or filtered into a single result set.
  - `  UNION  ` : Returns the combined results of the left and right input queries. Values in columns that are matched by position are concatenated vertically.
  - `  INTERSECT  ` : Returns rows that are found in the results of both the left and right input queries.
  - `  EXCEPT  ` : Returns rows from the left input query that aren't present in the right input query.
  - `  ALL  ` : Executes the set operation on all rows.
  - `  DISTINCT  ` : Excludes duplicate rows in the set operation.

**Positional column matching**

  - Columns from input queries are matched by their position in the queries. That is, the first column in the first input query is paired with the first column in the second input query and so on.
  - The input queries on each side of the operator must return the same number of columns.

**Other column-related rules**

  - For set operations other than `  UNION ALL  ` , all column types must support equality comparison.
  - The results of the set operation always use the column names from the first input query.
  - The results of the set operation always use the supertypes of input types in corresponding columns, so paired columns must also have either the same data type or a common supertype.

**Parenthesized set operators**

  - Parentheses must be used to separate different set operations. Set operations like `  UNION ALL  ` and `  UNION DISTINCT  ` are considered different.
  - Parentheses are also used to group set operations and control order of operations. In `  EXCEPT  ` set operations, for example, query results can vary depending on the operation grouping.

The following examples illustrate the use of parentheses with set operations:

``` text
-- Same set operations, no parentheses.
query1
UNION ALL
query2
UNION ALL
query3;
```

``` text
-- Different set operations, parentheses needed.
query1
UNION ALL
(
  query2
  UNION DISTINCT
  query3
);
```

``` text
-- Invalid
query1
UNION ALL
query2
UNION DISTINCT
query3;
```

``` text
-- Same set operations, no parentheses.
query1
EXCEPT ALL
query2
EXCEPT ALL
query3;

-- Equivalent query with optional parentheses, returns same results.
(
  query1
  EXCEPT ALL
  query2
)
EXCEPT ALL
query3;
```

``` text
-- Different execution order with a subquery, parentheses needed.
query1
EXCEPT ALL
(
  query2
  EXCEPT ALL
  query3
);
```

**Set operator behavior with duplicate rows**

Consider a given row `  R  ` that appears exactly `  m  ` times in the first input query and `  n  ` times in the second input query, where `  m >= 0  ` and `  n >= 0  ` :

  - For `  UNION ALL  ` , row `  R  ` appears exactly `  m + n  ` times in the result.
  - For `  INTERSECT ALL  ` , row `  R  ` appears exactly `  MIN(m, n)  ` times in the result.
  - For `  EXCEPT ALL  ` , row `  R  ` appears exactly `  MAX(m - n, 0)  ` times in the result.
  - For `  UNION DISTINCT  ` , the `  DISTINCT  ` is computed after the `  UNION  ` is computed, so row `  R  ` appears exactly one time.
  - For `  INTERSECT DISTINCT  ` , row `  R  ` appears once in the output if `  m > 0  ` and `  n > 0  ` .
  - For `  EXCEPT DISTINCT  ` , row `  R  ` appears once in the output if `  m > 0  ` and `  n = 0  ` .
  - If more than two input queries are used, the above operations generalize and the output is the same as if the input queries were combined incrementally from left to right.

### `     UNION    `

The `  UNION  ` operator returns the combined results of the left and right input queries. Columns are matched according to the rules described previously and rows are concatenated vertically.

**Examples**

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3]) AS number
UNION ALL
SELECT 1;

/*--------+
 | number |
 +--------+
 | 1      |
 | 2      |
 | 3      |
 | 1      |
 +--------*/
```

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3]) AS number
UNION DISTINCT
SELECT 1;

/*--------+
 | number |
 +--------+
 | 1      |
 | 2      |
 | 3      |
 +--------*/
```

The following example shows multiple chained operators:

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3]) AS number
UNION DISTINCT
SELECT 1
UNION DISTINCT
SELECT 2;

/*--------+
 | number |
 +--------+
 | 1      |
 | 2      |
 | 3      |
 +--------*/
```

### `     INTERSECT    `

The `  INTERSECT  ` operator returns rows that are found in the results of both the left and right input queries.

**Examples**

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3, 3, 4]) AS number
INTERSECT ALL
SELECT * FROM UNNEST(ARRAY<INT64>[2, 3, 3, 5]) AS number;

/*--------+
 | number |
 +--------+
 | 2      |
 | 3      |
 | 3      |
 +--------*/
```

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3, 3, 4]) AS number
INTERSECT DISTINCT
SELECT * FROM UNNEST(ARRAY<INT64>[2, 3, 3, 5]) AS number;

/*--------+
 | number |
 +--------+
 | 2      |
 | 3      |
 +--------*/
```

The following example shows multiple chained operations:

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3, 3, 4]) AS number
INTERSECT DISTINCT
SELECT * FROM UNNEST(ARRAY<INT64>[2, 3, 3, 5]) AS number
INTERSECT DISTINCT
SELECT * FROM UNNEST(ARRAY<INT64>[3, 3, 4, 5]) AS number;

/*--------+
 | number |
 +--------+
 | 3      |
 +--------*/
```

### `     EXCEPT    `

The `  EXCEPT  ` operator returns rows from the left input query that aren't present in the right input query.

**Examples**

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3, 3, 4]) AS number
EXCEPT ALL
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2]) AS number;

/*--------+
 | number |
 +--------+
 | 3      |
 | 3      |
 | 4      |
 +--------*/
```

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3, 3, 4]) AS number
EXCEPT DISTINCT
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2]) AS number;

/*--------+
 | number |
 +--------+
 | 3      |
 | 4      |
 +--------*/
```

The following example shows multiple chained operations:

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3, 3, 4]) AS number
EXCEPT DISTINCT
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2]) AS number
EXCEPT DISTINCT
SELECT * FROM UNNEST(ARRAY<INT64>[1, 4]) AS number;

/*--------+
 | number |
 +--------+
 | 3      |
 +--------*/
```

The following example modifies the execution behavior of the set operations. The first input query is used against the result of the last two input queries instead of the values of the last two queries individually. In this example, the `  EXCEPT  ` result of the last two input queries is `  2  ` . Therefore, the `  EXCEPT  ` results of the entire query are any values other than `  2  ` in the first input query.

``` text
SELECT * FROM UNNEST(ARRAY<INT64>[1, 2, 3, 3, 4]) AS number
EXCEPT DISTINCT
(
  SELECT * FROM UNNEST(ARRAY<INT64>[1, 2]) AS number
  EXCEPT DISTINCT
  SELECT * FROM UNNEST(ARRAY<INT64>[1, 4]) AS number
);

/*--------+
 | number |
 +--------+
 | 1      |
 | 3      |
 | 4      |
 +--------*/
```

## `     LIMIT    ` and `     OFFSET    ` clause

``` text
LIMIT count [ OFFSET skip_rows ]
```

Limits the number of rows to return in a query. Optionally includes the ability to skip over rows.

**Definitions**

  - `  LIMIT  ` : Limits the number of rows to produce.
    
    `  count  ` is an `  INT64  ` constant expression that represents the non-negative, non- `  NULL  ` limit. No more than `  count  ` rows are produced. `  LIMIT 0  ` returns 0 rows.
    
    If there is a set operation, `  LIMIT  ` is applied after the set operation is evaluated.

  - `  OFFSET  ` : Skips a specific number of rows before applying `  LIMIT  ` .
    
    `  skip_rows  ` is an `  INT64  ` constant expression that represents the non-negative, non- `  NULL  ` number of rows to skip.

**Details**

The rows that are returned by `  LIMIT  ` and `  OFFSET  ` have undefined order unless these clauses are used after `  ORDER BY  ` .

A constant expression can be represented by a general expression, literal, or parameter value.

**Note:** Although the `  LIMIT  ` clause limits the rows that a query produces, it doesn't limit the amount of data processed by that query.

**Examples**

``` text
SELECT *
FROM UNNEST(ARRAY<STRING>['a', 'b', 'c', 'd', 'e']) AS letter
ORDER BY letter ASC LIMIT 2;

/*---------+
 | letter  |
 +---------+
 | a       |
 | b       |
 +---------*/
```

``` text
SELECT *
FROM UNNEST(ARRAY<STRING>['a', 'b', 'c', 'd', 'e']) AS letter
ORDER BY letter ASC LIMIT 3 OFFSET 1;

/*---------+
 | letter  |
 +---------+
 | b       |
 | c       |
 | d       |
 +---------*/
```

## `     FOR UPDATE    ` clause

``` text
SELECT expression
FOR UPDATE;

UPDATE
expression;
```

In [serializable isolation](https://cloud.google.com/spanner/docs/isolation-levels#serializable) , when you use the `  SELECT  ` query to scan a table, add a `  FOR UPDATE  ` clause to enable exclusive locks at the row-and-column granularity level, otherwise known as cell-level. The lock remains in place for the lifetime of the read-write transaction. During this time, the `  FOR UPDATE  ` clause prevents other transactions from modifying the locked cells until the current transaction completes. For more information, see [Use SELECT FOR UPDATE in serializable isolation](https://cloud.google.com/spanner/docs/use-select-for-update-serializable) .

Unlike in serializable isolation, `  FOR UPDATE  ` doesn't acquire locks under repeatable read isolation. For more information, see [Use SELECT FOR UPDATE in repeatable read isolation](https://cloud.google.com/spanner/docs/use-select-for-update-repeatable-read) .

Example:

``` text
SELECT MarketingBudget
FROM Albums
WHERE SingerId = 1 and AlbumId = 1
FOR UPDATE;

UPDATE Albums
SET MarketingBudget = 100000
WHERE SingerId = 1 and AlbumId = 1;
```

You can't use the `  FOR UPDATE  ` clause in the following ways:

  - In combination with the [`  LOCK_SCANNED_RANGES  `](https://cloud.google.com/spanner/docs/reference/standard-sql/query-syntax#statement_hints) hint
  - In full-text search queries
  - In read-only transactions
  - Within DDL statements

## `     WITH    ` clause

``` text
WITH cte[, ...]
```

A `  WITH  ` clause contains one or more common table expressions (CTEs). A CTE acts like a temporary table that you can reference within a single query expression. Each CTE binds the results of a [subquery](/spanner/docs/reference/standard-sql/subqueries) to a table name, which can be used elsewhere in the same query expression, but [rules apply](#cte_rules) .

### CTEs

``` text
cte:
  cte_name AS ( query_expr )
```

A common table expression (CTE) contains a [subquery](/spanner/docs/reference/standard-sql/subqueries) and a name associated with the CTE.

  - A CTE can't reference itself.
  - A CTE can be referenced by the query expression that contains the `  WITH  ` clause, but [rules apply](#cte_rules) .

##### Examples

In this example, a `  WITH  ` clause defines two CTEs that are referenced in the related set operation, where one CTE is referenced by each of the set operation's input query expressions:

``` text
WITH subQ1 AS (SELECT SchoolID FROM Roster),
     subQ2 AS (SELECT OpponentID FROM PlayerStats)
SELECT * FROM subQ1
UNION ALL
SELECT * FROM subQ2
```

`  WITH  ` isn't supported *in* a subquery. This returns an error:

``` text
SELECT account
FROM (
  WITH result AS (SELECT * FROM NPCs)
  SELECT *
  FROM result)
```

You can use a `  FOR UPDATE  ` clause in a CTE subquery to lock the scanned range of the subquery.

The following query exclusively locks `  col1  ` and `  col2  ` in table `  t  ` .

``` text
WITH t1 AS (SELECT col1, col2 FROM t FOR UPDATE)
SELECT * FROM t1;
```

However, a `  FOR UPDATE  ` clause in the outer query won't propagate into the CTE. In the following example, an exclusive lock won't apply to any cells in table `  t  ` .

``` text
WITH t2 AS (SELECT col1, col2 FROM t)
SELECT * FROM t2 FOR UPDATE;
```

`  WITH  ` clause isn't supported in DML statements.

Temporary tables defined by the `  WITH  ` clause are stored in memory. Spanner dynamically allocates memory for all temporary tables created by a query. If the available resources aren't sufficient then the query will fail.

### CTE rules and constraints

Common table expressions (CTEs) can be referenced inside the query expression that contains the `  WITH  ` clause.

Here are some general rules and constraints to consider when working with CTEs:

  - Each CTE in the same `  WITH  ` clause must have a unique name.
  - A CTE defined in a `  WITH  ` clause is only visible to other CTEs in the same `  WITH  ` clause that were defined after it.
  - A local CTE overrides an outer CTE or table with the same name.
  - A CTE on a subquery may not reference correlated columns from the outer query.

### CTE visibility

References between common table expressions (CTEs) in the `  WITH  ` clause can go backward but not forward.

This is what happens when you have two CTEs that reference themselves or each other in a `  WITH  ` clause. Assume that `  A  ` is the first CTE and `  B  ` is the second CTE in the clause:

  - A references A = Invalid
  - A references B = Invalid
  - B references A = Valid
  - A references B references A = Invalid (cycles aren't allowed)

This produces an error. `  A  ` can't reference itself because self-references aren't supported:

``` text
WITH
  A AS (SELECT 1 AS n UNION ALL (SELECT n + 1 FROM A WHERE n < 3))
SELECT * FROM A

-- Error
```

This produces an error. `  A  ` can't reference `  B  ` because references between CTEs can go backwards but not forwards:

``` text
WITH
  A AS (SELECT * FROM B),
  B AS (SELECT 1 AS n)
SELECT * FROM B

-- Error
```

`  B  ` can reference `  A  ` because references between CTEs can go backwards:

``` text
WITH
  A AS (SELECT 1 AS n),
  B AS (SELECT * FROM A)
SELECT * FROM B

/*---+
 | n |
 +---+
 | 1 |
 +---*/
```

This produces an error. `  A  ` and `  B  ` reference each other, which creates a cycle:

``` text
WITH
  A AS (SELECT * FROM B),
  B AS (SELECT * FROM A)
SELECT * FROM B

-- Error
```

## Using aliases

An alias is a temporary name given to a table, column, or expression present in a query. You can introduce explicit aliases in the `  SELECT  ` list or `  FROM  ` clause, or GoogleSQL infers an implicit alias for some expressions. Expressions with neither an explicit nor implicit alias are anonymous and the query can't reference them by name.

### Explicit aliases

You can introduce explicit aliases in either the `  FROM  ` clause or the `  SELECT  ` list.

In a `  FROM  ` clause, you can introduce explicit aliases for any item, including tables, arrays, subqueries, and `  UNNEST  ` clauses, using `  [AS] alias  ` . The `  AS  ` keyword is optional.

Example:

``` text
SELECT s.FirstName, s2.SongName
FROM Singers AS s, (SELECT * FROM Songs) AS s2;
```

You can introduce explicit aliases for any expression in the `  SELECT  ` list using `  [AS] alias  ` . The `  AS  ` keyword is optional.

Example:

``` text
SELECT s.FirstName AS name, LOWER(s.FirstName) AS lname
FROM Singers s;
```

### Implicit aliases

In the `  SELECT  ` list, if there is an expression that doesn't have an explicit alias, GoogleSQL assigns an implicit alias according to the following rules. There can be multiple columns with the same alias in the `  SELECT  ` list.

  - For identifiers, the alias is the identifier. For example, `  SELECT abc  ` implies `  AS abc  ` .
  - For path expressions, the alias is the last identifier in the path. For example, `  SELECT abc.def.ghi  ` implies `  AS ghi  ` .
  - For field access using the "dot" member field access operator, the alias is the field name. For example, `  SELECT (struct_function()).fname  ` implies `  AS fname  ` .

In all other cases, there is no implicit alias, so the column is anonymous and can't be referenced by name. The data from that column will still be returned and the displayed query results may have a generated label for that column, but the label can't be used like an alias.

In a `  FROM  ` clause, `  from_item  ` s aren't required to have an alias. The following rules apply:

  - If there is an expression that doesn't have an explicit alias, GoogleSQL assigns an implicit alias in these cases:
      - For identifiers, the alias is the identifier. For example, `  FROM abc  ` implies `  AS abc  ` .
      - For path expressions, the alias is the last identifier in the path. For example, `  FROM abc.def.ghi  ` implies `  AS ghi  `
      - The column produced using `  WITH OFFSET  ` has the implicit alias `  offset  ` .
  - Table subqueries don't have implicit aliases.
  - `  FROM UNNEST(x)  ` doesn't have an implicit alias.

### Alias visibility

After you introduce an explicit alias in a query, there are restrictions on where else in the query you can reference that alias. These restrictions on alias visibility are the result of GoogleSQL name scoping rules.

#### Visibility in the `     FROM    ` clause

GoogleSQL processes aliases in a `  FROM  ` clause from left to right, and aliases are visible only to subsequent path expressions in a `  FROM  ` clause.

Example:

Assume the `  Singers  ` table had a `  Concerts  ` column of `  ARRAY  ` type.

``` text
SELECT FirstName
FROM Singers AS s, s.Concerts;
```

Invalid:

``` text
SELECT FirstName
FROM s.Concerts, Singers AS s;  // INVALID.
```

`  FROM  ` clause aliases are **not** visible to subqueries in the same `  FROM  ` clause. Subqueries in a `  FROM  ` clause can't contain correlated references to other tables in the same `  FROM  ` clause.

Invalid:

``` text
SELECT FirstName
FROM Singers AS s, (SELECT (2020 - ReleaseDate) FROM s)  // INVALID.
```

You can use any column name from a table in the `  FROM  ` as an alias anywhere in the query, with or without qualification with the table name.

Example:

``` text
SELECT FirstName, s.ReleaseDate
FROM Singers s WHERE ReleaseDate = 1975;
```

If the `  FROM  ` clause contains an explicit alias, you must use the explicit alias instead of the implicit alias for the remainder of the query (see [Implicit Aliases](#implicit_aliases) ). A table alias is useful for brevity or to eliminate ambiguity in cases such as self-joins, where the same table is scanned multiple times during query processing.

Example:

``` text
SELECT * FROM Singers as s, Songs as s2
ORDER BY s.LastName
```

Invalid — `  ORDER BY  ` doesn't use the table alias:

``` text
SELECT * FROM Singers as s, Songs as s2
ORDER BY Singers.LastName;  // INVALID.
```

#### Visibility in the `     SELECT    ` list

Aliases in the `  SELECT  ` list are visible only to the following clauses:

  - `  GROUP BY  ` clause
  - `  ORDER BY  ` clause
  - `  HAVING  ` clause

Example:

``` text
SELECT LastName AS last, SingerID
FROM Singers
ORDER BY last;
```

#### Visibility in the `     GROUP BY    ` , `     ORDER BY    ` , and `     HAVING    ` clauses

These three clauses, `  GROUP BY  ` , `  ORDER BY  ` , and `  HAVING  ` , can refer to only the following values:

  - Tables in the `  FROM  ` clause and any of their columns.
  - Aliases from the `  SELECT  ` list.

`  GROUP BY  ` and `  ORDER BY  ` can also refer to a third group:

  - Integer literals, which refer to items in the `  SELECT  ` list. The integer `  1  ` refers to the first item in the `  SELECT  ` list, `  2  ` refers to the second item, etc.

Example:

``` text
SELECT SingerID AS sid, COUNT(Songid) AS s2id
FROM Songs
GROUP BY 1
ORDER BY 2 DESC;
```

The previous query is equivalent to:

``` text
SELECT SingerID AS sid, COUNT(Songid) AS s2id
FROM Songs
GROUP BY sid
ORDER BY s2id DESC;
```

### Duplicate aliases

A `  SELECT  ` list or subquery containing multiple explicit or implicit aliases of the same name is allowed, as long as the alias name isn't referenced elsewhere in the query, since the reference would be [ambiguous](#ambiguous_aliases) .

Example:

``` text
SELECT 1 AS a, 2 AS a;

/*---+---+
 | a | a |
 +---+---+
 | 1 | 2 |
 +---+---*/
```

### Ambiguous aliases

GoogleSQL provides an error if accessing a name is ambiguous, meaning it can resolve to more than one unique object in the query or in a table schema, including the schema of a destination table.

The following query contains column names that conflict between tables, since both `  Singers  ` and `  Songs  ` have a column named `  SingerID  ` :

``` text
SELECT SingerID
FROM Singers, Songs;
```

The following query contains aliases that are ambiguous in the `  GROUP BY  ` clause because they are duplicated in the `  SELECT  ` list:

``` text
SELECT FirstName AS name, LastName AS name,
FROM Singers
GROUP BY name;
```

The following query contains aliases that are ambiguous in the `  SELECT  ` list and `  FROM  ` clause because they share a column and field with same name.

  - Assume the `  Person  ` table has three columns: `  FirstName  ` , `  LastName  ` , and `  PrimaryContact  ` .
  - Assume the `  PrimaryContact  ` column represents a struct with these fields: `  FirstName  ` and `  LastName  ` .

The alias `  P  ` is ambiguous and will produce an error because `  P.FirstName  ` in the `  GROUP BY  ` clause could refer to either `  Person.FirstName  ` or `  Person.PrimaryContact.FirstName  ` .

``` text
SELECT FirstName, LastName, PrimaryContact AS P
FROM Person AS P
GROUP BY P.FirstName;
```

A name is *not* ambiguous in `  GROUP BY  ` , `  ORDER BY  ` or `  HAVING  ` if it's both a column name and a `  SELECT  ` list alias, as long as the name resolves to the same underlying object. In the following example, the alias `  BirthYear  ` isn't ambiguous because it resolves to the same underlying column, `  Singers.BirthYear  ` .

``` text
SELECT LastName, BirthYear AS BirthYear
FROM Singers
GROUP BY BirthYear;
```

### Range variables

In GoogleSQL, a range variable is a table expression alias in the `  FROM  ` clause. Sometimes a range variable is known as a `  table alias  ` . A range variable lets you reference rows being scanned from a table expression. A table expression represents an item in the `  FROM  ` clause that returns a table. Common items that this expression can represent include tables, [value tables](#value_tables) , [subqueries](/spanner/docs/reference/standard-sql/subqueries) , [joins](#join_types) , and [parenthesized joins](#join_types) .

In general, a range variable provides a reference to the rows of a table expression. A range variable can be used to qualify a column reference and unambiguously identify the related table, for example `  range_variable.column_1  ` .

When referencing a range variable on its own without a specified column suffix, the result of a table expression is the row type of the related table. Value tables have explicit row types, so for range variables related to value tables, the result type is the value table's row type. Other tables don't have explicit row types, and for those tables, the range variable type is a dynamically defined struct that includes all of the columns in the table.

**Examples**

In these examples, the `  WITH  ` clause is used to emulate a temporary table called `  Grid  ` . This table has columns `  x  ` and `  y  ` . A range variable called `  Coordinate  ` refers to the current row as the table is scanned. `  Coordinate  ` can be used to access the entire row or columns in the row.

The following example selects column `  x  ` from range variable `  Coordinate  ` , which in effect selects column `  x  ` from table `  Grid  ` .

``` text
WITH Grid AS (SELECT 1 x, 2 y)
SELECT Coordinate.x FROM Grid AS Coordinate;

/*---+
 | x |
 +---+
 | 1 |
 +---*/
```

The following example selects all columns from range variable `  Coordinate  ` , which in effect selects all columns from table `  Grid  ` .

``` text
WITH Grid AS (SELECT 1 x, 2 y)
SELECT Coordinate.* FROM Grid AS Coordinate;

/*---+---+
 | x | y |
 +---+---+
 | 1 | 2 |
 +---+---*/
```

The following example selects the range variable `  Coordinate  ` , which is a reference to rows in table `  Grid  ` . Since `  Grid  ` isn't a value table, the result type of `  Coordinate  ` is a struct that contains all the columns from `  Grid  ` .

``` text
WITH Grid AS (SELECT 1 x, 2 y)
SELECT Coordinate FROM Grid AS Coordinate;

/*--------------+
 | Coordinate   |
 +--------------+
 | {x: 1, y: 2} |
 +--------------*/
```

## Hints

``` text
@{hint_key=hint_value[, ...]}
```

GoogleSQL supports hints, which make the query optimizer use a specific operator in the execution plan. If performance is an issue for you, a hint might be able to help by suggesting a different query execution plan shape.

**Definitions**

  - `  hint_key  ` : The name of the hint key.
  - `  hint_value  ` : The value for `  hint_key  ` .

**Examples**

``` text
@{KEY_ONE=TRUE}
```

``` text
@{KEY_TWO=10, KEY_THREE=FALSE}
```

### Statement hints

The following query statement hints are supported:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Hint key</th>
<th>Possible values</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       USE_ADDITIONAL_PARALLELISM      </code></td>
<td><code dir="ltr" translate="no">       TRUE      </code><br />
| <code dir="ltr" translate="no">       FALSE      </code> (default)</td>
<td>If <code dir="ltr" translate="no">       TRUE      </code> , the execution engine favors using more parallelism when possible. Because this can reduce resources available to other operations, you may want to avoid this hint if you run latency-sensitive operations on the same instance.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       OPTIMIZER_VERSION      </code></td>
<td><code dir="ltr" translate="no">       1      </code> to <code dir="ltr" translate="no">       N      </code><br />
| <code dir="ltr" translate="no">       latest_version      </code><br />
| <code dir="ltr" translate="no">       default_version      </code><br />
</td>
<td><p>Executes the query using the specified optimizer version. Possible values are <code dir="ltr" translate="no">        1       </code> to <code dir="ltr" translate="no">        N       </code> (the latest optimizer version), <code dir="ltr" translate="no">        default_version       </code> , or <code dir="ltr" translate="no">        latest_version       </code> . If the hint isn't set, the optimizer executes against the package that's set in database options or specified through the client API. If neither of those are set, the optimizer uses the <a href="../../query-optimizer/manage-query-optimizer#default_version">default version</a> .</p>
<p>In terms of version setting precedence, the value set by the client API takes precedence over the value in the database options and the value set by this hint takes precedence over everything else.</p>
<p>For more information, see <a href="../../query-optimizer/overview">Query optimizer</a> .</p></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       OPTIMIZER_STATISTICS_PACKAGE      </code></td>
<td><code dir="ltr" translate="no">       package_name      </code><br />
| <code dir="ltr" translate="no">       latest      </code></td>
<td><p>Executes the query using the specified optimizer statistics package. Possible values for <code dir="ltr" translate="no">          package_name        </code> can be found by running the following query:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>SELECT * FROM INFORMATION_SCHEMA.SPANNER_STATISTICS</code></pre>
<p>If the hint isn't set, the optimizer executes against the package that's set in the database option or specified through the client API. If neither of those are set, the optimizer defaults to the latest package.</p>
<p>The value set by the client API takes precedence over the value in the database options and the value set by this hint takes precedence over everything else.</p>
<p>The specified package needs to be pinned by the database option or have <code dir="ltr" translate="no">        allow_gc=false       </code> to prevent garbage collection.</p>
<p>For more information, see <a href="../../query-optimizer/overview#query_optimizer_statistics_packages">Query optimizer statistics packages</a> .</p></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       ALLOW_DISTRIBUTED_MERGE      </code></td>
<td><code dir="ltr" translate="no">       TRUE      </code> (default)<br />
| <code dir="ltr" translate="no">       FALSE      </code></td>
<td><p>If <code dir="ltr" translate="no">        TRUE       </code> (default), the engine favors using a distributed merge sort algorithm for certain ORDER BY queries. When applicable, global sorts are changed to local sorts. This gives the advantage of parallel sorting close to where the data is stored. The locally sorted data is then merged to provide globally sorted data. This allows for removal of full global sorts and potentially improved latency.</p>
<p>This feature can increase parallelism of certain ORDER BY queries. This hint has been provided so that users can experiment with turning off the distributed merge algorithm if desired.</p></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       LOCK_SCANNED_RANGES      </code></td>
<td><code dir="ltr" translate="no">       exclusive      </code><br />
| <code dir="ltr" translate="no">       shared      </code> (default)</td>
<td><p>Use this hint to request an exclusive lock on a set of ranges scanned by a transaction. Acquiring an exclusive lock helps in scenarios when you observe high write contention, that is, you notice that multiple transactions are concurrently trying to read and write to the same data, resulting in a large number of aborts.</p>
<p>Without the hint, it's possible that multiple simultaneous transactions will acquire shared locks, and then try to upgrade to exclusive locks. This will cause a deadlock, because each transaction's shared lock is preventing the other transaction(s) from upgrading to exclusive. Spanner aborts all but one of the transactions.</p>
<p>When requesting an exclusive lock using this hint, one transaction acquires the lock and proceeds to execute, while other transactions wait their turn for the lock. Throughput is still limited because the conflicting transactions can only be performed one at a time, but in this case Spanner is always making progress on one transaction, saving time that would otherwise be spent aborting and retrying transactions.</p>
<p>This hint is supported on all statement types, both query and DML.</p>
<p>Spanner always enforces <a href="../../transactions#serializability_and_external_consistency">serializability</a> Lock mode hints can affect which transactions wait or abort in contended workloads, but don't change the isolation level.</p>
<p>Because this is just a hint, it shouldn't be considered equivalent to a mutex. In other words, you shouldn't use Spanner exclusive locks as a mutual exclusion mechanism for the execution of code outside of Spanner. For more information, see <a href="../../transactions#locking">Locking</a> .</p>
<p>You can't use both the <code dir="ltr" translate="no">        FOR UPDATE       </code> clause and the <code dir="ltr" translate="no">        LOCK_SCANNED_RANGES       </code> hint in the same query. An error is returned. For more information, see <a href="../../use-select-for-update">.</a></p></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       SCAN_METHOD      </code></td>
<td><code dir="ltr" translate="no">       AUTO      </code> (default)<br />
| <code dir="ltr" translate="no">       BATCH      </code><br />
| <code dir="ltr" translate="no">       ROW      </code></td>
<td>Use this hint to enforce the query scan method.
<p>The default Spanner scan method is <code dir="ltr" translate="no">        AUTO       </code> (automatic). The <code dir="ltr" translate="no">        AUTO       </code> setting specifies that batch-oriented query processing might be used to improve query performance. If you want to change the default scanning method, you can use a statement hint to enforce the <code dir="ltr" translate="no">        BATCH       </code> -oriented or <code dir="ltr" translate="no">        ROW       </code> -oriented processing method. You can't manually set the scan method to <code dir="ltr" translate="no">        AUTO       </code> ; to do so, remove the statement hint, and Spanner will set it to the default method. For more information, see <a href="/spanner/docs/sql-best-practices#optimize-scans">Optimize scans</a> .</p></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       EXECUTION_METHOD      </code></td>
<td><code dir="ltr" translate="no">       DEFAULT      </code><br />
| <code dir="ltr" translate="no">       BATCH      </code><br />
| <code dir="ltr" translate="no">       ROW      </code></td>
<td>Use this hint to enforce the query execution method.
<p>The default Spanner query execution method is <code dir="ltr" translate="no">        DEFAULT       </code> . The <code dir="ltr" translate="no">        DEFAULT       </code> setting specifies that batch-oriented execution might be used to improve query performance, depending on the heuristics of the query. If you want to change the default execution method, you can use a statement hint to enforce the <code dir="ltr" translate="no">        BATCH       </code> -oriented or <code dir="ltr" translate="no">        ROW       </code> -oriented execution method. You can't manually set the query execution method to <code dir="ltr" translate="no">        DEFAULT       </code> ; to do so, remove the statement hint, and Spanner will set it to the default method. For more information, see <a href="/spanner/docs/sql-best-practices#optimize-query-execution">Optimize query execution</a> .</p></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       USE_UNENFORCED_FOREIGN_KEY      </code></td>
<td><code dir="ltr" translate="no">       TRUE      </code> (default)<br />
| <code dir="ltr" translate="no">       FALSE      </code></td>
<td>Use this hint to enforce the query scan method.
<p>If <code dir="ltr" translate="no">        TRUE       </code> (default), the query optimizer relies on <a href="/spanner/docs/foreign-keys/overview#informational-foreign-keys">informational ( <code dir="ltr" translate="no">         NOT ENFORCED        </code> ) foreign key</a> relationships to improve query performance. For example, if set to <code dir="ltr" translate="no">        TRUE       </code> , the optimizer can remove redundant scans, and push some <code dir="ltr" translate="no">        LIMIT       </code> operators through the join operators. <code dir="ltr" translate="no">        USE_UNENFORCED_FOREIGN_KEY       </code> overrides the value of the <code dir="ltr" translate="no">        use_unenforced_foreign_key_for_query_optimization       </code> database option for the applied statement. This hint might introduce incorrect results if the data is inconsistent with the foreign key relationships.</p>
<p>For more information, see <a href="/spanner/docs/foreign-keys/overview#informational-foreign-keys">informational foreign keys</a> .</p></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       ALLOW_TIMESTAMP_PREDICATE_PUSHDOWN      </code></td>
<td><code dir="ltr" translate="no">       TRUE      </code><br />
| <code dir="ltr" translate="no">       FALSE      </code> (default)</td>
<td>If set to <code dir="ltr" translate="no">       TRUE      </code> , the query execution engine uses the timestamp predicate pushdown optimization technique. This technique improves the efficiency of queries that use timestamps and data with an age-based tiered storage policy. For more information, see <a href="/spanner/docs/sql-best-practices#optimize-timestamp-predicate-pushdown">Optimize queries with timestamp predicate pushdown</a> .</td>
</tr>
</tbody>
</table>

### Table hints

The following table hints are supported:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Hint key</th>
<th>Possible values</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       FORCE_INDEX      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><ul>
<li>If set to the name of an index, use that index instead of the base table. If the index can't provide all needed columns, perform a back join with the base table.</li>
<li>If set to the string <code dir="ltr" translate="no">         _BASE_TABLE        </code> , use the base table for the index strategy instead of an index. Note that this is the only valid value when <code dir="ltr" translate="no">         FORCE_INDEX        </code> is used in a statement hint expression.</li>
</ul>
<p>Note: <code dir="ltr" translate="no">        FORCE_INDEX       </code> is actually a directive, not a hint, which means an error is raised if the index doesn't exist.</p></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       GROUPBY_SCAN_OPTIMIZATION      </code></td>
<td><code dir="ltr" translate="no">       TRUE      </code><br />
| <code dir="ltr" translate="no">       FALSE      </code></td>
<td><p>The group by scan optimization can make queries faster if they use <code dir="ltr" translate="no">        GROUP BY       </code> or <code dir="ltr" translate="no">        SELECT DISTINCT       </code> . It can be applied if the grouping keys can form a prefix of the underlying table or index key, and if the query requires only the first row from each group.</p>
<p>The optimization is applied if the optimizer estimates that it will make the query more efficient. The hint overrides that decision. If the hint is set to <code dir="ltr" translate="no">        FALSE       </code> , the optimization isn't considered. If the hint is set to <code dir="ltr" translate="no">        TRUE       </code> , the optimization will be applied as long as it's legal to do so.</p></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       SCAN_METHOD      </code></td>
<td><code dir="ltr" translate="no">       AUTO      </code> (default)<br />
| <code dir="ltr" translate="no">       BATCH      </code><br />
| <code dir="ltr" translate="no">       ROW      </code></td>
<td>Use this hint to enforce the query scan method.
<p>By default, Spanner sets the scan method as <code dir="ltr" translate="no">        AUTO       </code> (automatic) which means depending on the heuristics of the query, batch-oriented query processing might be used to improve query performance. If you want to change the default scanning method from <code dir="ltr" translate="no">        AUTO       </code> , you can use the hint to enforce a <code dir="ltr" translate="no">        ROW       </code> or <code dir="ltr" translate="no">        BATCH       </code> oriented processing method. For more information see <a href="/spanner/docs/sql-best-practices#optimize-scans">Optimize scans</a> .</p></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       INDEX_STRATEGY      </code></td>
<td><code dir="ltr" translate="no">       FORCE_INDEX_UNION      </code></td>
<td><p>Use the <code dir="ltr" translate="no">        INDEX_STRATEGY=FORCE_INDEX_UNION       </code> hint to access data, using the Index Union pattern (reading from two or more indexes and unioning the results). This hint is useful when the condition in the <code dir="ltr" translate="no">        WHERE       </code> clause is a disjunction. If an index union isn't possible, an error is raised.</p></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       SEEKABLE_KEY_SIZE      </code></td>
<td><code dir="ltr" translate="no">       0      </code> to <code dir="ltr" translate="no">       16      </code></td>
<td><p>Forces the seekable key size to be equal to the specified value.</p>
<p>The seekable key size is the length of the key (primary key or index key) that's used in a <a href="https://cloud.google.com/spanner/docs/query-execution-operators#filter_scan">seekable condition</a> , while the rest of the key is used in a residual condition.</p>
<p>This hint requires the <code dir="ltr" translate="no">        FORCE_INDEX       </code> hint to also be specified.</p></td>
</tr>
</tbody>
</table>

The following example shows how to use a [secondary index](../../secondary-indexes.md) when reading from a table, by appending an index directive of the form `  @{FORCE_INDEX=index_name}  ` to the table name:

``` text
SELECT s.SingerId, s.FirstName, s.LastName, s.SingerInfo
FROM Singers@{FORCE_INDEX=SingersByFirstLastName} AS s
WHERE s.FirstName = "Catalina" AND s.LastName > "M";
```

You can include multiple indexes in a query, though only a single index is supported for each distinct table reference. Example:

``` text
SELECT s.SingerId, s.FirstName, s.LastName, s.SingerInfo, c.ConcertDate
FROM Singers@{FORCE_INDEX=SingersByFirstLastName} AS s JOIN
     Concerts@{FORCE_INDEX=ConcertsBySingerId} AS c ON s.SingerId = c.SingerId
WHERE s.FirstName = "Catalina" AND s.LastName > "M";
```

Read more about index directives in [Secondary Indexes](../../secondary-indexes#index_directive) .

### Join hints

The following join hints are supported:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Hint key</th>
<th>Possible values</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       FORCE_JOIN_ORDER      </code></td>
<td><code dir="ltr" translate="no">       TRUE      </code><br />
<code dir="ltr" translate="no">       FALSE      </code> (default)</td>
<td>If set to <code dir="ltr" translate="no">       TRUE      </code> , use the join order that's specified in the query.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       JOIN_METHOD      </code></td>
<td><code dir="ltr" translate="no">       HASH_JOIN      </code><br />
<code dir="ltr" translate="no">       APPLY_JOIN      </code><br />
<code dir="ltr" translate="no">       MERGE_JOIN      </code><br />
<code dir="ltr" translate="no">       PUSH_BROADCAST_HASH_JOIN      </code></td>
<td>When implementing a logical join, choose a specific alternative to use for the underlying join method. Learn more in <a href="#join_methods">Join methods</a> . To use a HASH join, either use <code dir="ltr" translate="no">       HASH JOIN      </code> or <code dir="ltr" translate="no">       JOIN@{JOIN_METHOD=HASH_JOIN}      </code> , but not both.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       HASH_JOIN_BUILD_SIDE      </code></td>
<td><code dir="ltr" translate="no">       BUILD_LEFT      </code><br />
<code dir="ltr" translate="no">       BUILD_RIGHT      </code></td>
<td>Specifies which side of the hash join is used as the build side. Can only be used with <code dir="ltr" translate="no">       JOIN_METHOD=HASH_JOIN      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       BATCH_MODE      </code></td>
<td><code dir="ltr" translate="no">       TRUE      </code> (default)<br />
<code dir="ltr" translate="no">       FALSE      </code></td>
<td>Used to disable batched apply join in favor of row-at-a-time apply join. Can only be used with <code dir="ltr" translate="no">       JOIN_METHOD=APPLY_JOIN      </code> .</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       HASH_JOIN_EXECUTION      </code></td>
<td><code dir="ltr" translate="no">       MULTI_PASS      </code> (default)<br />
<code dir="ltr" translate="no">       ONE_PASS      </code></td>
<td>For a hash join, specifies what should be done when the hash table size reaches its memory limit. Can only be used when <code dir="ltr" translate="no">       JOIN_METHOD=HASH_JOIN      </code> . See <a href="#hash_join_execution">Hash Join Execution</a> for more details.</td>
</tr>
</tbody>
</table>

#### Join methods

Join methods are specific implementations of the various logical join types. Some join methods are available only for certain join types. The choice of which join method to use depends on the specifics of your query and of the data being queried. The best way to figure out if a particular join method helps with the performance of your query is to try the method and view the resulting [query execution plan](https://cloud.google.com/spanner/docs/query-execution-plans) . See [Query Execution Operators](https://cloud.google.com/spanner/docs/query-execution-operators) for more details.

<table>
<colgroup>
<col style="width: 20%" />
<col style="width: 40%" />
<col style="width: 40%" />
</colgroup>
<thead>
<tr class="header">
<th>Join Method</th>
<th>Description</th>
<th>Operands</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       HASH_JOIN      </code></td>
<td>The hash join operator builds a hash table out of one side (the build side), and probes in the hash table for all the elements in the other side (the probe side).</td>
<td>Different variants are used for various join types. View the query execution plan for your query to see which variant is used. Read more about the <a href="https://cloud.google.com/spanner/docs/query-execution-operators#hash-join">Hash join operator</a> .</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       APPLY_JOIN      </code></td>
<td>The apply join operator gets each item from one side (the input side), and evaluates the subquery on other side (the map side) using the values of the item from the input side.</td>
<td>Different variants are used for various join types. Cross apply is used for inner join, and outer apply is used for left joins. Read more about the <a href="https://cloud.google.com/spanner/docs/query-execution-operators#cross-apply">Cross apply operator</a> and <a href="https://cloud.google.com/spanner/docs/query-execution-operators#outer-apply">Outer apply operator</a></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       MERGE_JOIN      </code></td>
<td>The merge join operator joins two streams of sorted data. The optimizer adds Sort operators to the plan if the data isn't already providing the required sort property for the given join condition. The engine provides a distributed merge sort by default, which when coupled with merge join may allow for larger joins, potentially avoiding disk spilling and improving scale and latency.</td>
<td>Different variants are used for various join types. View the query execution plan for your query to see which variant is used. Read more about the <a href="https://cloud.google.com/spanner/docs/query-execution-operators#merge-join">Merge join operator</a> .</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       PUSH_BROADCAST_HASH_JOIN      </code></td>
<td>The push broadcast hash join operator builds a batch of data from the build side of the join. The batch is then sent in parallel to all the local splits of the probe side of the join. On each of the local servers, a hash join is executed between the batch and the local data. This join is most likely to be beneficial when the input can fit within one batch, but isn't strict. Another potential area of benefit is when operations can be distributed to the local servers, such as an aggregation that occurs after a join. A push broadcast hash join can distribute some aggregation where a traditional hash join can't.</td>
<td>Different variants are used for various join types. View the query execution plan for your query to see which variant is used. Read more about the <a href="https://cloud.google.com/spanner/docs/query-execution-operators#push-broadcast-hash-join">Push broadcast hash join operator</a> .</td>
</tr>
</tbody>
</table>

#### Hash Join Execution

To execute a hash join between two tables, Spanner first scans rows from the build side and loads them into a hash table. Then it scans rows from the probe side, while comparing them against the hash table. If the hash table reaches its memory limit, depending on the value of the `  HASH_JOIN_EXECUTION  ` query hint, the hash join has one of the following behaviors:

  - `  HASH_JOIN_EXECUTION=MULTI_PASS  ` (default): The query engine splits the build side table into partitions in a way that the size of a hash table corresponding to each partition is less than the memory size limit. For every partition of the build side table, the probe side is scanned once.
  - `  HASH_JOIN_EXECUTION=ONE_PASS  ` : The query engine writes both the build side table and the probe side table to disk in partitions in a way that the hash table of the build side table in each partition is less than the memory limit. The probe side is only scanned once.

### Group hints

The following group hints are supported:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Group hint key</th>
<th>Group hint values</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       GROUP_METHOD      </code></td>
<td><code dir="ltr" translate="no">       HASH_GROUP      </code><br />
<code dir="ltr" translate="no">       STREAM_GROUP      </code></td>
<td>Specifies an alternative to choose when implementing a <code dir="ltr" translate="no">       GROUP BY      </code> operator.</td>
</tr>
</tbody>
</table>

### Graph hints

Hints are supported for graphs. For more information, see [Graph hints](/spanner/docs/reference/standard-sql/graph-query-statements#graph_hints) .

## Value tables

In addition to standard SQL *tables* , GoogleSQL supports *value tables* . In a value table, rather than having rows made up of a list of columns, each row is a single value of a specific type, and there are no column names.

In the following example, a value table for a `  STRUCT  ` is produced with the `  SELECT AS VALUE  ` statement:

``` text
SELECT * FROM (SELECT AS VALUE STRUCT(123 AS a, FALSE AS b))

/*-----+-------+
 | a   | b     |
 +-----+-------+
 | 123 | FALSE |
 +-----+-------*/
```

Value tables are often but not exclusively used with compound data types. A value table can consist of any supported GoogleSQL data type, although value tables consisting of scalar types occur less frequently than structs.

### Return query results as a value table

Spanner doesn't support value tables as base tables in database schemas and doesn't support returning value tables in query results. As a consequence, value table producing queries aren't supported as top-level queries.

Value tables can also occur as the output of the [`  UNNEST  `](#unnest_operator) operator or a [subquery](/spanner/docs/reference/standard-sql/subqueries) . The [`  WITH  ` clause](#with_clause) introduces a value table if the subquery used produces a value table.

In contexts where a query with exactly one column is expected, a value table query can be used instead. For example, scalar and array [subqueries](/spanner/docs/reference/standard-sql/subqueries) normally require a single-column query, but in GoogleSQL, they also allow using a value table query.

### Use a set operation on a value table

In `  SET  ` operations like `  UNION ALL  ` you can combine tables with value tables, provided that the table consists of a single column with a type that matches the value table's type. The result of these operations is always a value table.

## Appendix A: examples with sample data

These examples include statements which perform queries on the [`  Roster  `](#roster_table) and [`  TeamMascot  `](#teammascot_table) , and [`  PlayerStats  `](#playerstats_table) tables.

### Sample tables

The following tables are used to illustrate the behavior of different query clauses in this reference.

#### Roster table

The `  Roster  ` table includes a list of player names ( `  LastName  ` ) and the unique ID assigned to their school ( `  SchoolID  ` ). It looks like this:

``` text
/*-----------------------+
 | LastName   | SchoolID |
 +-----------------------+
 | Adams      | 50       |
 | Buchanan   | 52       |
 | Coolidge   | 52       |
 | Davis      | 51       |
 | Eisenhower | 77       |
 +-----------------------*/
```

You can use this `  WITH  ` clause to emulate a temporary table name for the examples in this reference:

``` text
WITH Roster AS
 (SELECT 'Adams' as LastName, 50 as SchoolID UNION ALL
  SELECT 'Buchanan', 52 UNION ALL
  SELECT 'Coolidge', 52 UNION ALL
  SELECT 'Davis', 51 UNION ALL
  SELECT 'Eisenhower', 77)
SELECT * FROM Roster
```

#### PlayerStats table

The `  PlayerStats  ` table includes a list of player names ( `  LastName  ` ) and the unique ID assigned to the opponent they played in a given game ( `  OpponentID  ` ) and the number of points scored by the athlete in that game ( `  PointsScored  ` ).

``` text
/*----------------------------------------+
 | LastName   | OpponentID | PointsScored |
 +----------------------------------------+
 | Adams      | 51         | 3            |
 | Buchanan   | 77         | 0            |
 | Coolidge   | 77         | 1            |
 | Adams      | 52         | 4            |
 | Buchanan   | 50         | 13           |
 +----------------------------------------*/
```

You can use this `  WITH  ` clause to emulate a temporary table name for the examples in this reference:

``` text
WITH PlayerStats AS
 (SELECT 'Adams' as LastName, 51 as OpponentID, 3 as PointsScored UNION ALL
  SELECT 'Buchanan', 77, 0 UNION ALL
  SELECT 'Coolidge', 77, 1 UNION ALL
  SELECT 'Adams', 52, 4 UNION ALL
  SELECT 'Buchanan', 50, 13)
SELECT * FROM PlayerStats
```

#### TeamMascot table

The `  TeamMascot  ` table includes a list of unique school IDs ( `  SchoolID  ` ) and the mascot for that school ( `  Mascot  ` ).

``` text
/*---------------------+
 | SchoolID | Mascot   |
 +---------------------+
 | 50       | Jaguars  |
 | 51       | Knights  |
 | 52       | Lakers   |
 | 53       | Mustangs |
 +---------------------*/
```

You can use this `  WITH  ` clause to emulate a temporary table name for the examples in this reference:

``` text
WITH TeamMascot AS
 (SELECT 50 as SchoolID, 'Jaguars' as Mascot UNION ALL
  SELECT 51, 'Knights' UNION ALL
  SELECT 52, 'Lakers' UNION ALL
  SELECT 53, 'Mustangs')
SELECT * FROM TeamMascot
```

### `     GROUP BY    ` clause

Example:

``` text
SELECT LastName, SUM(PointsScored)
FROM PlayerStats
GROUP BY LastName;
```

<table>
<thead>
<tr class="header">
<th>LastName</th>
<th>SUM</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Adams</td>
<td>7</td>
</tr>
<tr class="even">
<td>Buchanan</td>
<td>13</td>
</tr>
<tr class="odd">
<td>Coolidge</td>
<td>1</td>
</tr>
</tbody>
</table>

### `     UNION    `

The `  UNION  ` operator combines the result sets of two or more `  SELECT  ` statements by pairing columns from the result set of each `  SELECT  ` statement and vertically concatenating them.

Example:

``` text
SELECT Mascot AS X, SchoolID AS Y
FROM TeamMascot
UNION ALL
SELECT LastName, PointsScored
FROM PlayerStats;
```

Results:

<table>
<thead>
<tr class="header">
<th>X</th>
<th>Y</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Jaguars</td>
<td>50</td>
</tr>
<tr class="even">
<td>Knights</td>
<td>51</td>
</tr>
<tr class="odd">
<td>Lakers</td>
<td>52</td>
</tr>
<tr class="even">
<td>Mustangs</td>
<td>53</td>
</tr>
<tr class="odd">
<td>Adams</td>
<td>3</td>
</tr>
<tr class="even">
<td>Buchanan</td>
<td>0</td>
</tr>
<tr class="odd">
<td>Coolidge</td>
<td>1</td>
</tr>
<tr class="even">
<td>Adams</td>
<td>4</td>
</tr>
<tr class="odd">
<td>Buchanan</td>
<td>13</td>
</tr>
</tbody>
</table>

### `     INTERSECT    `

This query returns the last names that are present in both Roster and PlayerStats.

``` text
SELECT LastName
FROM Roster
INTERSECT ALL
SELECT LastName
FROM PlayerStats;
```

Results:

<table>
<thead>
<tr class="header">
<th>LastName</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Adams</td>
</tr>
<tr class="even">
<td>Coolidge</td>
</tr>
<tr class="odd">
<td>Buchanan</td>
</tr>
</tbody>
</table>

### `     EXCEPT    `

The query below returns last names in Roster that are **not** present in PlayerStats.

``` text
SELECT LastName
FROM Roster
EXCEPT DISTINCT
SELECT LastName
FROM PlayerStats;
```

Results:

<table>
<thead>
<tr class="header">
<th>LastName</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Eisenhower</td>
</tr>
<tr class="even">
<td>Davis</td>
</tr>
</tbody>
</table>

Reversing the order of the `  SELECT  ` statements will return last names in PlayerStats that are **not** present in Roster:

``` text
SELECT LastName
FROM PlayerStats
EXCEPT DISTINCT
SELECT LastName
FROM Roster;
```

Results:

``` text
(empty)
```
