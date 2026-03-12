The GoogleSQL data manipulation language (DML) lets you update, insert, and delete data in GoogleSQL tables.

For information about how to use DML statements, see [Inserting, updating, and deleting data using Data Manipulation Language](/spanner/docs/dml-tasks) . You can also modify data [using mutations](/spanner/docs/modify-mutation-api) .

## Tables used in examples

``` text
CREATE TABLE Singers (
  SingerId    INT64 NOT NULL,
  FirstName   STRING(1024),
  LastName    STRING(1024),
  BirthDate   DATE,
  Status      STRING(1024),
  LastUpdated TIMESTAMP DEFAULT (PENDING_COMMIT_TIMESTAMP())
    ON UPDATE (PENDING_COMMIT_TIMESTAMP())
    OPTIONS (allow_commit_timestamp = true),
  SingerInfo  googlesql.example.SingerInfo,
  AlbumInfo   googlesql.example.Album,
) PRIMARY KEY(SingerId);

CREATE TABLE AlbumInfo (
  SingerId        INT64 NOT NULL,
  AlbumId         INT64 NOT NULL,
  AlbumTitle      STRING(MAX),
  MarketingBudget INT64,
) PRIMARY KEY(SingerId, AlbumId),
  INTERLEAVE IN PARENT Singers ON DELETE CASCADE;

CREATE TABLE Songs (
  SingerId  INT64 NOT NULL,
  AlbumId   INT64 NOT NULL,
  TrackId   INT64 NOT NULL,
  SongName  STRING(MAX),
  Duration  INT64,
  SongGenre STRING(25),
) PRIMARY KEY(SingerId, AlbumId, TrackId),
  INTERLEAVE IN PARENT AlbumInfo ON DELETE CASCADE;

CREATE TABLE Concerts (
  VenueId      INT64 NOT NULL,
  SingerId     INT64 NOT NULL,
  ConcertDate  DATE NOT NULL,
  BeginTime    TIMESTAMP,
  EndTime      TIMESTAMP,
  TicketPrices ARRAY<INT64>,
) PRIMARY KEY(VenueId, SingerId, ConcertDate);

CREATE TABLE AckworthSingers (
  SingerId   INT64 NOT NULL,
  FirstName  STRING(1024),
  LastName   STRING(1024),
  BirthDate  DATE,
) PRIMARY KEY(SingerId);

CREATE TABLE Fans (
  FanId     STRING(36) DEFAULT (GENERATE_UUID()),
  FirstName STRING(1024),
  LastName  STRING(1024),
) PRIMARY KEY(FanId);
```

### Definitions for protocol buffers used in examples

``` text
package googlesql.example;

message SingerInfo {
  optional string    nationality = 1;
  repeated Residence residence   = 2;

  message Residence {
    required int64  start_year   = 1;
    optional int64  end_year     = 2;
    optional string city         = 3;
    optional string country      = 4;
  }
}

message Album {
  optional string title = 1;
  optional int64 tracks = 2;
  repeated string comments = 3;
  repeated Song song = 4;

  message Song {
    optional string songtitle = 1;
    optional int64 length = 2;
    repeated Chart chart = 3;

    message Chart {
      optional string chartname = 1;
      optional int64 rank = 2;
    }
  }
}
```

## Notation used in the syntax

  - Square brackets `  [ ]  ` indicate optional clauses.
  - Parentheses `  ( )  ` indicate literal parentheses.
  - The vertical bar `  |  ` indicates a logical OR.
  - Curly braces `  { }  ` enclose a set of options.
  - A comma followed by an ellipsis indicates that the preceding item can repeat in a comma-separated list. `  item [, ...]  ` indicates one or more items, and `  [item, ...]  ` indicates zero or more items.
  - A comma `  ,  ` indicates the literal comma.
  - Angle brackets `  <>  ` indicate literal angle brackets.
  - A colon `  :  ` indicates a definition.
  - Uppercase words, such as `  INSERT  ` , are keywords.

## INSERT statement

Use the `  INSERT  ` statement to add new rows to a table. The `  INSERT  ` statement can insert one or more rows specified by value expressions, or zero or more rows produced by a query. The statement by default returns the number of rows inserted into the table.

``` text
INSERT [[OR] IGNORE | UPDATE]
[INTO] table_name
 (column_name_1 [, ..., column_name_n] )
 input [ASSERT_ROWS_MODIFIED number_rows] [return_clause]

ON CONFLICT syntax:

 INSERT [INTO] target_name [AS table-alias]
 (column[, ...])
 on_conflict_input
 ON CONFLICT [conflict_target]
 conflict_action [ASSERT_ROWS_MODIFIED number_rows]
 [return_clause]

input:
  {
    value_list | SELECT_QUERY
  }

on_conflict_input:
{
    value_list | (SELECT_QUERY)
}

value_list:
 VALUES (row_1_column_1_expr [, ..., row_1_column_n_expr ] )
        [, ..., (row_k_column_1_expr [, ..., row_k_column_n_expr ] ) ]

expr: value_expression | DEFAULT

conflict_target:
  {
    (column_name [,..]) | ON UNIQUE CONSTRAINT index_constraint_name
  }

conflict_action:
  {
    DO NOTHING | DO UPDATE SET set_clause [ WHERE update_condition ]
  }

set_clause:
  update_item[, ...]

update_item:
  {
    path_expression = expression
    | path_expression = DEFAULT
  }

update_condition:
  bool_expression

return_clause:
  THEN RETURN [ WITH ACTION [ AS alias ]  ] { select_all | expression [  [ AS ] alias ] } [, ...]

select_all:
    [ table_name. ]*
    [ EXCEPT ( column_name [, ...] ) ]
    [ REPLACE ( expression [ AS ] column_name [, ...] ) ]
```

`  INSERT  ` statements must comply with these rules:

  - The column names can be in any order.
  - Duplicate names are not allowed in the list of columns.
  - The number of columns must match the number of values.
  - GoogleSQL matches the values in the `  VALUES  ` clause or the select query positionally with the column list.
  - Each value must be type compatible with its associated column.
  - The values must comply with any constraints in the schema, for example, unique secondary indexes.
  - All non-null columns must appear in the column list, and have a non-null value specified.
  - `  INSERT OR  ` and `  ON CONFLICT  ` clauses aren't allowed in the same statement.

If a statement does not comply with the rules, Spanner raises an error and the entire statement fails.

If the statement attempts to insert a duplicate row, as determined by the primary key, then the entire statement fails.

**Note:** `  INSERT  ` is not supported in [Partitioned DML](/spanner/docs/dml-partitioned) .

### Value type compatibility

Values that you add in an `  INSERT  ` statement must be compatible with the target column's type. A value's type is compatible with the target column's type if the value meets one of the following criteria:

  - The value type matches the column type exactly. For example, inserting a value of type `  INT64  ` in a column that has a type of `  INT64  ` is compatible.
  - GoogleSQL can [implicitly coerce](/spanner/docs/reference/standard-sql/conversion_rules) the value into the target type.

### Default values

Use the `  DEFAULT  ` keyword to insert the default value of a column. If a column is not included in the list, GoogleSQL assigns the default value of the column. If the column has no defined default value, `  NULL  ` is assigned to the column.

The use of default values is subject to current Spanner limits, including the mutation limit. If a column has a default value and it is used in an insert or update, the column is counted as one mutation. For example, assuming that table `  T  ` has three columns and that `  col_a  ` has a default value, the following inserts each result in three mutations:

``` text
INSERT INTO T (id, col_a, col_b) VALUES (1, DEFAULT, 1);
INSERT INTO T (id, col_a, col_b) VALUES (2, 200, 2);
INSERT INTO T (id, col_b) VALUES (3, 3);
```

For more information about default column values, see the `  DEFAULT ( expression )  ` clause in `  CREATE TABLE  ` .

For more information about mutations, see [What are mutations?](../../dml-versus-mutations.md#mutations-concept) .

### INSERT OR IGNORE

Use the `  INSERT OR IGNORE  ` clause to insert new rows that don't exist in the table. If the primary key of the row already exists, then the row is ignored. For an `  INSERT OR IGNORE  ` query that inserts multiple rows or inserts from a subquery, only the new rows are inserted. Rows where the primary key already exists are ignored.

For example, if the primary key is `  SingerId  ` and the table already contains a `  SingerId  ` of 7, then in the following example, `  INSERT  ` would insert the first row and ignore the second row:

``` text
INSERT OR IGNORE INTO Singers
    (SingerId, FirstName, LastName, Birthdate, Status, SingerInfo)
VALUES (5, "Zak", "Sterling", "1996-03-12", "active", "nationality:'USA'"),
       (7, "Edie", "Silver", "1998-01-23", "active", "nationality:'USA'");
```

You can use `  INSERT OR IGNORE  ` in single or batch DML requests using the [`  executeBatchDml  `](/spanner/docs/reference/rest/v1/projects.instances.databases.sessions/executeBatchDml) API.

### INSERT OR UPDATE

Use the `  INSERT OR UPDATE  ` clause to insert or update a row. If the primary key is not found, a new row is inserted. If a row with the primary key already exists in the table, then it is updated with the values that you specify in the statement. If the row is updated, any column that you don't specify remains unchanged.

An exception to these rules is columns with `  ON UPDATE  ` expressions. These columns have their value automatically set to the `  ON UPDATE  ` expression if the statement column list includes any non-key columns.

In the following statement, `  INSERT OR UPDATE  ` modifies the column value of `  Status  ` from `  active  ` to `  inactive  ` in the existing table with the primary key `  SingerId  ` of `  5  ` . It also automatically updates the column value of `  LastUpdatedTime  ` to the `  ON UPDATE  ` expression, `  PENDING_COMMIT_TIMESTAMP()  ` .

``` text
INSERT OR UPDATE INTO Singers
    (SingerId, Status)
VALUES (5, "inactive");
```

If the row does not exist, the previous statement inserts a new row with values in the specified fields and `  PENDING_COMMIT_TIMESTAMP()  ` as the default value for `  LastUpdatedTime  ` .

You can use `  INSERT OR UPDATE  ` in single or batch DML requests using the [`  executeBatchDml  `](/spanner/docs/reference/rest/v1/projects.instances.databases.sessions/executeBatchDml) API.

### `     ON CONFLICT DO NOTHING    `

Use the `  ON CONFLICT DO NOTHING  ` clause to attempt an `  INSERT  ` and if the row exists, Spanner doesn't perform the `  INSERT  ` on that row. This prevents duplicates in Spanner. The [`  conflict_target  `](#conflict-conflict_target) is optional for this clause. When used, it specifies the table constraint to use for conflicts. When not used, the row is ignored if the insert row violates any unique constraint on the table, such as the primary key or unique indexes.

The `  ON CONFLICT DO NOTHING  ` clause has the following behavior:

  - For an `  INSERT OR IGNORE  ` query that inserts multiple rows or inserts from a subquery, only the new rows are inserted.
  - If the input is ordered, Spanner inserts the first row in that order. If unordered, Spanner inserts an arbitrary row.
  - If the input uses `  DO NOTHING  ` with a `  THEN RETURN  ` clause and an insert row is ignored, Spanner doesn't return any columns for that ignored row. This behavior is consistent with `  INSERT OR IGNORE  ` .

### `     ON CONFLICT DO UPDATE    `

Use the `  ON CONFLICT DO UPDATE  ` clause to perform an `  INSERT  ` and if the unique constraint specified by the `  conflict_target  ` is violated, Spanner updates the existing row. If a `  WHERE  ` clause is present, the row is updated only if the `  WHERE  ` condition is `  true  ` or satisfied. The [`  conflict_target  `](#conflict-conflict_target) clause specifies the table constraint to use for conflicts and is required for this clause.

If there's a conflict, the `  SET  ` clause specifies the updates to apply to the existing row.

The `  SET  ` clause has the following behavior:

  - The columns specified for update in the `  SET  ` clause don't need to be present in the `  INSERT  ` column list.
  - Columns can be updated to arbitrary value expressions. These expressions can reference values from both the existing row in the table and the new values from the `  INSERT  ` statement.
  - To reference existing row column values, prefix the column with the table name as the alias.
  - For the insert row column values, prefix the column name with the `  EXCLUDED  ` alias (that is, `  EXCLUDED.column_name  ` ). This ensures that the incoming value from the attempted insert is being used.

Like expressions in the `  SET  ` clause, expressions in a `  WHERE  ` clause can also reference the existing row and the insert row in input. It follows the same rules to alias the column with a table name or with `  EXCLUDED  ` respectively to prevent ambiguity.

Any column, including generated columns, can be accessed using the `  EXCLUDED  ` alias. If a column isn't explicitly in the `  INSERT  ` column list (and thus has no value from the input), it defaults to its `  DEFAULT  ` value when referenced with `  EXCLUDED  ` . Dependent columns of generated columns use the value from the `  EXCLUDED  ` row; otherwise, they use their `  DEFAULT  ` or `  NULL  ` values if not listed.

### `     conflict_target    ` parameter

Both `  ON CONFLICT DO NOTHING  ` and `  ON CONFLICT DO UPDATE  ` use the `  conflict_target  ` parameter to specify the table constraint that the query uses to evaluate if the insert row is duplicate. If the `  conflict_target  ` key column values already exist in the table, then the insert row is considered a duplicate.

There are two primary ways to specify the `  conflict_target  ` :

  - Use a list of columns in a primary key or key columns of a `  UNIQUE  ` index.
  - Use an `  ON UNIQUE CONSTRAINT  ` clause, for example, `  ON CONFLICT ON UNIQUE CONSTRAINT unique_index_name  ` .

**Note:** Unique indexes with null filtered key columns aren't supported.

The `  conflict_target  ` must comply with the following rules:

  - Only primary key or a unique index can be specified as the `  conflict_target  ` .
  - A statement can list only one table constraint as the `  conflict_target  ` .
  - Default or generated columns can be a `  conflict_target  ` if they belong to a primary key or key columns of a `  UNIQUE  ` index.
  - A `  conflict_target  ` must include all columns covered by the chosen constraint. The set of columns in the conflict target identifies the primary key or unique constraint.
  - You must use the [`  SELECT  ` privilege ( `  spanner.databases.select  ` )](/spanner/docs/iam#roles) on the `  conflict_target  ` columns.

### THEN RETURN

Use the `  THEN RETURN  ` clause to return the results of the `  INSERT  ` operation and selected data from the newly inserted rows. This clause is especially useful for retrieving values of columns with default values, generated columns, and auto-generated keys, without having to use additional `  SELECT  ` statements.

Use the `  THEN RETURN  ` clause to capture expressions based on newly inserted rows that include the following:

  - `  WITH ACTION  ` : An optional clause that adds a string column called `  ACTION  ` to the result row set. Each value in this column represents the type of action that was applied to the column during statement execution. Values include `  INSERT  ` , `  DELETE  ` , and `  UPDATE  ` . The `  ACTION  ` column is appended as the last output column.
  - `  *  ` : Returns all columns.
  - `  table_name.*  ` : Returns all columns from the table. You cannot use the .\* expression with other expressions, including field access.
  - `  EXCEPT ( column_name [, ...] )  ` : Specifies the columns to exclude from the result. All matching column names are omitted from the output.
  - `  REPLACE ( expression [ AS ] column_name [, ...] )  ` : Specifies one or more `  expression AS identifier  ` clauses. Each identifier must match a column name from the `  table_name.*  ` statement. In the output column list, the column that matches the identifier in a `  REPLACE  ` clause is replaced by the expression in that `  REPLACE  ` clause. Note that the value that gets inserted into the table is not replaced, just the value returned by the `  THEN RETURN  ` clause.
  - `  expression  ` : Represents a column name of the table specified by `  table_name  ` or an expression that uses any combination of such column names. Column names are valid if they belong to columns of the `  table_name  ` . Excluded expressions include aggregate and analytic functions.
  - `  alias  ` : Represents a temporary name for an expression in the query.

When using `  THEN RETURN  ` with `  ON CONFLICT DO UPDATE  ` , only table columns are returned; `  EXCLUDED.column_name  ` can't be used in a `  THEN RETURN  ` clause.

For instructions and code samples, see [Modify data with the returning DML statements](/spanner/docs/dml-tasks#client-library-dml-return) .

## INSERT examples

The section provides code examples for `  INSERT  ` statements.

### INSERT using literal values examples

The following example adds two rows to the `  Singers  ` table.

``` text
INSERT INTO Singers (SingerId, FirstName, LastName, SingerInfo)
VALUES(1, 'Marc', 'Richards', "nationality: 'USA'"),
      (2, 'Catalina', 'Smith', "nationality: 'Brazil'"),
      (3, "Andrew", "Duneskipper", NULL);
```

These are the two new rows in the table:

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>nationality: USA</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>nationality: Brazil</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>3</td>
<td>Alice</td>
<td>Trentor</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

### INSERT using a SELECT statement example

The following example shows how to copy the data from one table into another table using a `  SELECT  ` statement as the input:

``` text
INSERT INTO Singers (SingerId, FirstName, LastName)
SELECT SingerId, FirstName, LastName
FROM AckworthSingers;
```

If the `  Singers  ` table had no rows, and the `  AckworthSingers  ` table had three rows, then there are now three rows in the `  Singers  ` table:

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>3</td>
<td>Alice</td>
<td>Trentor</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

The following example shows how to use `  UNNEST  ` to return a table that is the input to the `  INSERT  ` command.

``` text
INSERT INTO Singers (SingerId, FirstName, LastName)
SELECT *
FROM UNNEST ([(4, 'Lea', 'Martin'),
      (5, 'David', 'Lomond'),
      (6, 'Elena', 'Campbell')]);
```

After adding these three additional rows to the table from the previous example, there are six rows in the `  Singers  ` table:

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>3</td>
<td>Alice</td>
<td>Trentor</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>4</td>
<td>Lea</td>
<td>Martin</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>5</td>
<td>David</td>
<td>Lomond</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>6</td>
<td>Elena</td>
<td>Campbell</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

### INSERT using a subquery example

The following example shows how to insert a row into a table, where one of the values is computed using a subquery:

``` text
INSERT INTO Singers (SingerId, FirstName)
VALUES (4, (SELECT FirstName FROM AckworthSingers WHERE SingerId = 4));
```

The following tables show the data before the statement is executed.

**Singers**

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

**AckworthSingers**

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>4</td>
<td>Lea</td>
<td>Martin</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>5</td>
<td>David</td>
<td>Lomond</td>
<td>NULL</td>
</tr>
</tbody>
</table>

The following table shows the data after the statement is executed.

**Singers**

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>4</td>
<td>Lea</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

To include multiple columns, you include multiple subqueries:

``` text
INSERT INTO Singers (SingerId, FirstName, LastName)
VALUES (4,
        (SELECT FirstName FROM AckworthSingers WHERE SingerId = 4),
        (SELECT LastName  FROM AckworthSingers WHERE SingerId = 4));
```

### INSERT with THEN RETURN examples

The following query inserts two rows into a table, uses `  THEN RETURN  ` to fetch the SingerId column from these rows, and computes a new column called `  FullName  ` .

``` text
INSERT INTO Singers (SingerId, FirstName, LastName)
VALUES
    (7, 'Melissa', 'Garcia'),
    (8, 'Russell', 'Morales')
THEN RETURN SingerId, FirstName || ' ' || LastName AS FullName;
```

The following table shows the query result:

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FullName</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>7</td>
<td>Melissa Garcia</td>
</tr>
<tr class="even">
<td>8</td>
<td>Russell Morales</td>
</tr>
</tbody>
</table>

The following query inserts a row to the `  Fans  ` table. Spanner automatically generates a Version 4 UUID for the primary key `  FanId  ` , and returns it using the `  THEN RETURN  ` clause.

``` text
INSERT INTO Fans (FirstName, LastName)
VALUES ('Melissa', 'Garcia')
THEN RETURN FanId;
```

The following table shows the query result:

<table>
<thead>
<tr class="header">
<th style="text-align: left;">FanId</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td style="text-align: left;">6af91072-f009-4c15-8c42-ebe38ae83751</td>
</tr>
</tbody>
</table>

The following query tries to insert or update a row into a table. It uses `  THEN RETURN  ` to fetch the modified row and `  WITH ACTION  ` to show the modified row action type.

``` text
INSERT OR UPDATE Singers (SingerId, FirstName, LastName)
VALUES (7, 'Melissa', 'Gartner')
THEN RETURN WITH ACTION SingerId, FirstName || ' ' || LastName AS FullName;
```

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FullName</th>
<th>Action</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>7</td>
<td>Melissa Gartner</td>
<td>UPDATE</td>
</tr>
</tbody>
</table>

### INSERT OR IGNORE example

The following query inserts a row in the `  Singers  ` table for singers with an ID between 10 and 100. If an ID already exists in `  Singers  ` , it's ignored.

``` text
INSERT OR IGNORE INTO Singers (
  SingerId, FirstName, LastName, BirthDate, Status, SingerInfo)
  (SELECT id, fname, lname, dob, status, info
    FROM latest_album
    WHERE id > 10
    AND id < 100);
```

### INSERT with ON CONFLICT DO UPDATE examples

The following example checks for a UNIQUE constraint on a list of the `  (FirstName, LastName)  ` columns:

``` text
INSERT INTO Singers (SingerId, FirstName, LastName) excluded
ON CONFLICT(FirstName, LastName) DO UPDATE SET SingerId = excluded.SingerId;
```

The following example checks for a conflict on the `  UNIQUE CONSTRAINT UniqueIndex_SingerName  ` .

``` text
INSERT INTO Singers (SingerId, FirstName, LastName, BirthDate, Status, LastUpdated)
VALUES (101, 'Adele', 'Adkins', '1988-05-05', 'Active', CURRENT_TIMESTAMP())
ON CONFLICT ON UNIQUE CONSTRAINT UniqueIndex_SingerName
DO UPDATE SET Status = excluded.Status, LastUpdated = Singers.LastUpdated;
```

The following example checks for a conflict on the `  PRIMARY KEY (SingerId)  ` :

``` text
INSERT INTO Singers (SingerId, FirstName, LastName) excluded
ON CONFLICT(SingerId)
DO UPDATE SET FirstName = excluded.FirstName;
```

For the following unique index:

``` text
CREATE UNIQUE INDEX Index_AlbumInfo_SingerAlbum ON AlbumInfo (SingerId, AlbumId);
```

The following shows an example for updating a table where the primary key is `  (SingerId, AlbumId)  ` , using a unique index.

``` text
INSERT INTO AlbumInfo (SingerId, AlbumId, AlbumTitle, MarketingBudget)
VALUES (10, 1, 'My New Album', 750000)
ON CONFLICT(SingerId, AlbumId)
DO UPDATE SET AlbumTitle = EXCLUDED.AlbumTitle, MarketingBudget = EXCLUDED.MarketingBudget;
```

For the next example, imagine the `  AlbumInfo  ` table has a default value for `  LastReviewScore INT64 DEFAULT 75  ` , and a generated column `  EstimatedProfit INT64 AS (MarketingBudget * 2) STORED  ` :

``` text
INSERT INTO AlbumInfo (SingerId, AlbumId, AlbumTitle, MarketingBudget)
VALUES (10, 2, 'The Second Album', 600000)
ON CONFLICT(SingerId, AlbumId)
DO UPDATE SET LastReviewScore = EXCLUDED.LastReviewScore, EstimatedProfit = EXCLUDED.EstimatedProfit + 10000;
```

The following example updates only a subset of columns while others remain unchanged:

``` text
INSERT INTO AlbumInfo (SingerId, AlbumId, AlbumTitle, MarketingBudget)
VALUES (10, 1, 'Brand New Album Title', 800000)
ON CONFLICT(SingerId, AlbumId)
DO UPDATE SET AlbumTitle = EXCLUDED.AlbumTitle;
```

### INSERT with ON CONFLICT DO NOTHING examples

The following example ignores the row if `  SingerId  ` , `  AlbumId  ` , or `  MarketingBudget  ` has duplicates.

``` text
INSERT INTO Singers (SingerId, FirstName)
VALUES (1, 'John')
ON CONFLICT DO NOTHING;
```

The following example checks for a conflict on the `  PRIMARY KEY (SingerId)  ` :

``` text
INSERT INTO Singers (SingerId, FirstName, LastName) excluded
ON CONFLICT(SingerId) DO NOTHING;
```

## DELETE statement

Use the `  DELETE  ` statement to delete rows from a table.

``` text
[statement_hint_expr] DELETE [FROM] table_name [table_hint_expr] [[AS] alias] WHERE condition [return_clause];

statement_hint_expr: '@{' statement_hint_key = statement_hint_value '}'

table_hint_expr: '@{' table_hint_key = table_hint_value '}'

return_clause:
    THEN RETURN [ WITH ACTION [ AS alias ]  ] { select_all | expression [  [ AS ] alias ] } [, ...]

select_all:
    [ table_name. ]*
    [ EXCEPT ( column_name [, ...] ) ]
    [ REPLACE ( expression [ AS ] column_name [, ...] ) ]
```

### WHERE clause

The `  WHERE  ` clause is required. This requirement can help prevent accidentally deleting all the rows in a table. To delete all rows in a table, set the `  condition  ` to `  true  ` :

``` text
DELETE FROM table_name WHERE true;
```

The `  WHERE  ` clause can contain any valid SQL statement, including a subquery that refers to other tables.

### Aliases

The `  WHERE  ` clause has an implicit alias to `  table_name  ` . This alias lets you reference columns in `  table_name  ` without qualifying them with `  table_name  ` . For example, if your statement started with `  DELETE FROM Singers  ` , then you could access any columns of `  Singers  ` in the `  WHERE  ` clause. In this example, `  FirstName  ` is a column in the `  Singers  ` table:

``` text
DELETE FROM Singers WHERE FirstName = 'Alice';
```

You can also create an explicit alias using the optional `  AS  ` keyword. For more details on aliases, see [Query syntax](/spanner/docs/reference/standard-sql/query-syntax#aliases_2) .

### THEN RETURN

With the optional `  THEN RETURN  ` clause, you can obtain data from rows that are being deleted in a table. To learn more about the values that you can use in this clause, see [THEN RETURN](#insert-and-then-return) .

### Statement hints

`  statement_hint_expr  ` is a statement-level hint. The following hints are supported:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th><code dir="ltr" translate="no">       statement_hint_key      </code></th>
<th><code dir="ltr" translate="no">       statement_hint_value      </code></th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>PDML_MAX_PARALLELISM</td>
<td>An integer between 1 to 1000</td>
<td>Sets the maximum parallelism for <a href="/spanner/docs/dml-partitioned">Partitioned DML</a> queries.<br />
This hint is only valid with <a href="/spanner/docs/dml-partitioned">Partitioned DML</a> query execution mode.</td>
</tr>
</tbody>
</table>

### Table hints

`  table_hint_expr  ` is a hint for accessing the table. The following hints are supported:

<table>
<thead>
<tr class="header">
<th><code dir="ltr" translate="no">       table_hint_key      </code></th>
<th><code dir="ltr" translate="no">       table_hint_value      </code></th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>FORCE_INDEX</td>
<td>Index name</td>
<td>Use specified index when querying rows to be deleted.</td>
</tr>
<tr class="even">
<td>FORCE_INDEX</td>
<td>_BASE_TABLE</td>
<td>Don't use an index. Instead, scan the base table.</td>
</tr>
</tbody>
</table>

## DELETE examples

This section contains code examples for `  DELETE  ` examples.

### DELETE with WHERE clause example

The following `  DELETE  ` statement deletes all singers whose first name is `  Alice  ` .

``` text
DELETE FROM Singers WHERE FirstName = 'Alice';
```

The following table shows the data before the statement is executed.

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>3</td>
<td>Alice</td>
<td>Trentor</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

The following table shows the data after the statement is executed.

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

### DELETE with subquery example

The following statement deletes any singer in `  SINGERS  ` whose first name is not in `  AckworthSingers  ` .

``` text
DELETE FROM Singers
WHERE FirstName NOT IN (SELECT FirstName from AckworthSingers);
```

The following table shows the data before the statement is executed.

**Singers**

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>3</td>
<td>Alice</td>
<td>Trentor</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>4</td>
<td>Lea</td>
<td>Martin</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>5</td>
<td>David</td>
<td>Lomond</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>6</td>
<td>Elena</td>
<td>Campbell</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

**AckworthSingers**

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>4</td>
<td>Lea</td>
<td>Martin</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>5</td>
<td>David</td>
<td>Lomond</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>6</td>
<td>Elena</td>
<td>Campbell</td>
<td>NULL</td>
</tr>
</tbody>
</table>

The following table shows the data after the statement is executed.

**Singers**

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>4</td>
<td>Lea</td>
<td>Martin</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>5</td>
<td>David</td>
<td>Lomond</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>6</td>
<td>Elena</td>
<td>Campbell</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

### DELETE with THEN RETURN example

The following query deletes all rows in a table that contains a singer called `  Melissa  ` and returns all columns in the deleted rows except the `  LastUpdated  ` column.

``` text
DELETE FROM Singers WHERE Firstname = 'Melissa'
THEN RETURN * EXCEPT (LastUpdated);
```

The following table shows the query result:

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>7</td>
<td>Melissa</td>
<td>Garcia</td>
<td>NULL</td>
</tr>
</tbody>
</table>

## UPDATE statement

Use the `  UPDATE  ` statement to update existing rows in a table.

``` text
[statement_hint_expr] UPDATE table_name [table_hint_expr] [[AS] alias]
SET update_item [, ...]
WHERE condition [return_clause];

update_item: column_name = { expression | DEFAULT }

statement_hint_expr: '@{' statement_hint_key = statement_hint_value '}'

table_hint_expr: '@{' table_hint_key = table_hint_value '}'

return_clause:
    THEN RETURN [ WITH ACTION [ AS alias ]  ] { select_all | expression [  [ AS ] alias ] } [, ...]

select_all:
    [ table_name. ]*
    [ EXCEPT ( column_name [, ...] ) ]
    [ REPLACE ( expression [ AS ] column_name [, ...] ) ]
```

Where:

  - `  table_name  ` is the name of a table to update.

  - The `  SET  ` clause is a list of update\_items to perform on each row where the `  WHERE  ` condition is true.

  - `  expression  ` is an update expression. The expression can be a literal, a SQL expression, or a SQL subquery.

  - `  statement_hint_expr  ` is a statement-level hint. The following hints are supported:
    
    <table>
    <colgroup>
    <col style="width: 33%" />
    <col style="width: 33%" />
    <col style="width: 33%" />
    </colgroup>
    <thead>
    <tr class="header">
    <th><code dir="ltr" translate="no">         statement_hint_key        </code></th>
    <th><code dir="ltr" translate="no">         statement_hint_value        </code></th>
    <th>Description</th>
    </tr>
    </thead>
    <tbody>
    <tr class="odd">
    <td>PDML_MAX_PARALLELISM</td>
    <td>An integer between 1 to 1000</td>
    <td>Sets the maximum parallelism for <a href="/spanner/docs/dml-partitioned">Partitioned DML</a> queries.<br />
    This hint is only valid with <a href="/spanner/docs/dml-partitioned">Partitioned DML</a> query execution mode.</td>
    </tr>
    </tbody>
    </table>

  - `  table_hint_expr  ` is a hint for accessing the table. The following hints are supported:
    
    <table>
    <thead>
    <tr class="header">
    <th><code dir="ltr" translate="no">         table_hint_key        </code></th>
    <th><code dir="ltr" translate="no">         table_hint_value        </code></th>
    <th>Description</th>
    </tr>
    </thead>
    <tbody>
    <tr class="odd">
    <td>FORCE_INDEX</td>
    <td>Index name</td>
    <td>Use specified index when querying rows to be updated.</td>
    </tr>
    <tr class="even">
    <td>FORCE_INDEX</td>
    <td>_BASE_TABLE</td>
    <td>Don't use an index. Instead, scan the base table.</td>
    </tr>
    </tbody>
    </table>

`  UPDATE  ` statements must comply with the following rules:

  - A column can appear only once in the `  SET  ` clause.
  - The columns in the `  SET  ` clause can be listed in any order.
  - Each value must be type compatible with its associated column.
  - The values must comply with any constraints in the schema, such as unique secondary indexes or non-nullable columns.
  - Updates with joins are not supported.
  - You cannot update primary key columns.

If a statement does not comply with the rules, Spanner raises an error and the entire statement fails.

Columns not included in the `  SET  ` clause are not modified.

Column updates are performed simultaneously. For example, you can swap two column values using a single `  SET  ` clause:

``` text
SET x = y, y = x
```

### Value type compatibility

Values updated with an `  UPDATE  ` statement must be compatible with the target column's type. A value's type is compatible with the target column's type if the value meets one of the following criteria:

  - The value type matches the column type exactly. For example, the value type is `  INT64  ` and the column type is `  INT64  ` .
  - GoogleSQL can [implicitly coerce](/spanner/docs/reference/standard-sql/conversion_rules) the value into the target type.

### Default values

The `  DEFAULT  ` keyword sets the value of a column to its default value. If the column has no defined default value, the `  DEFAULT  ` keyword sets it to `  NULL  ` .

The use of default values is subject to current Spanner limits, including the mutation limit. If a column has a default value and it is used in an insert or update, the column is counted as one mutation. For example, assume that in table `  T  ` , `  col_a  ` has a default value. The following updates each result in two mutations. One comes from the primary key, and another comes from either the explicit value (1000) or the default value.

``` text
UPDATE T SET col_a = 1000 WHERE id=1;
UPDATE T SET col_a = DEFAULT WHERE id=3;
```

For more information about default column values, see the `  DEFAULT ( expression )  ` clause in `  CREATE TABLE  ` .

For more information about mutations, see [What are mutations?](../../dml-versus-mutations.md#mutations-concept) .

### `     ON UPDATE    `

If the target table includes an `  ON UPDATE  ` expression for a column, that column's value is set to the expression if the column isn't explicitly set in the `  UPDATE  ` statement. The column is automatically updated whether or not the values in the other columns changed.

The following example automatically sets the `  LastUpdated  ` column in the `  Singers  ` table to the commit timestamp, whether or not the value of `  Status  ` changed.

``` text
UPDATE Singers SET Status = 'inactive' WHERE SingerId = 10;
```

The following example doesn't trigger `  ON UPDATE  ` for the `  LastUpdated  ` column because an explicit value has been provided.

``` text
UPDATE Singers
SET Status = 'active', LastUpdated = TIMESTAMP ("2025-07-15 15:30:00+00")
WHERE SingerId = 10;
```

### WHERE clause

The `  WHERE  ` clause is required. This requirement can help prevent accidentally updating all the rows in a table. To update all rows in a table, set the `  condition  ` to `  true  ` .

The `  WHERE  ` clause can contain any valid SQL boolean expression, including a subquery that refers to other tables.

### THEN RETURN

With the optional `  THEN RETURN  ` clause, you can obtain data from rows that are being updated in a table. To learn more about the values that you can use in this clause, see [THEN RETURN](#insert-and-then-return) .

### Aliases

The `  WHERE  ` clause has an implicit alias to `  table_name  ` . This alias lets you reference columns in `  table_name  ` without qualifying them with `  table_name  ` . For example, if your statement starts with `  UPDATE Singers  ` , then you can access any columns of `  Singers  ` in the `  WHERE  ` clause. In this example, `  FirstName  ` and `  LastName  ` are columns in the `  Singers  ` table:

``` text
UPDATE Singers
SET BirthDate = '1990-10-10'
WHERE FirstName = 'Marc' AND LastName = 'Richards';
```

You can also create an explicit alias using the optional `  AS  ` keyword. For more details on aliases, see [Query syntax](/spanner/docs/reference/standard-sql/query-syntax#aliases_2) .

## UPDATE examples

This section contains code examples for `  UPDATE  ` statements.

### UPDATE with literal values example

The following example updates the `  Singers  ` table by updating the `  BirthDate  ` column in one of the rows.

``` text
UPDATE Singers
SET BirthDate = '1990-10-10', SingerInfo = "nationality:'USA'"
WHERE FirstName = 'Marc' AND LastName = 'Richards';
```

The following table shows the data before the statement is executed.

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>3</td>
<td>Alice</td>
<td>Trentor</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

The following table shows the data after the statement is executed.

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>FirstName</th>
<th>LastName</th>
<th>BirthDate</th>
<th>Status</th>
<th>SingerInfo</th>
<th>AlbumInfo</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>Marc</td>
<td>Richards</td>
<td>1990-10-10</td>
<td>NULL</td>
<td>nationality: USA</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>2</td>
<td>Catalina</td>
<td>Smith</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>3</td>
<td>Alice</td>
<td>Trentor</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

### UPDATE ARRAY columns example

The following example updates an `  ARRAY  ` column.

``` text
UPDATE Concerts SET TicketPrices = [25, 50, 100] WHERE VenueId = 1;
```

The following table shows the data before the statement is executed.

<table>
<thead>
<tr class="header">
<th>VenueId</th>
<th>SingerId</th>
<th>ConcertDate</th>
<th>BeginTime</th>
<th>EndTime</th>
<th>TicketPrices</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>1</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>1</td>
<td>2</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>2</td>
<td>3</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

The following table shows the data after the statement is executed.

<table>
<thead>
<tr class="header">
<th>VenueId</th>
<th>SingerId</th>
<th>ConcertDate</th>
<th>BeginTime</th>
<th>EndTime</th>
<th>TicketPrices</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>1</td>
<td>1</td>
<td>2018-01-01</td>
<td>NULL</td>
<td>NULL</td>
<td>[25, 50, 100]</td>
</tr>
<tr class="even">
<td>1</td>
<td>2</td>
<td>2018-01-01</td>
<td>NULL</td>
<td>NULL</td>
<td>[25, 50, 100]</td>
</tr>
<tr class="odd">
<td>2</td>
<td>3</td>
<td>2018-01-01</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

### UPDATE with THEN RETURN example

The following query updates all rows where the singer first name is equal to `  Russell  ` and returns the `  SingerId  ` in the updated rows. It also extracts the year from the updated `  BirthDate  ` column as a new output column called `  year  ` .

``` text
UPDATE Singers
SET BirthDate = '1990-10-10'
WHERE FirstName = 'Russell'
THEN RETURN SingerId, EXTRACT(YEAR FROM BirthDate) AS year;
```

The following table shows the query result:

<table>
<thead>
<tr class="header">
<th>SingerId</th>
<th>year</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>8</td>
<td>1990</td>
</tr>
</tbody>
</table>

## Bound STRUCT parameters

You can use [bound `  STRUCT  ` parameters](/spanner/docs/structs#using_struct_objects_as_bound_parameters_in_sql_queries) in the `  WHERE  ` clause of a DML statement. The following code example updates the `  LastName  ` in rows filtered by `  FirstName  ` and `  LastName  ` .

### C\#

``` csharp
using Google.Cloud.Spanner.Data;
using System;
using System.Threading.Tasks;

public class UpdateUsingDmlWithStructCoreAsyncSample
{
    public async Task<int> UpdateUsingDmlWithStructCoreAsync(string projectId, string instanceId, string databaseId)
    {
        var nameStruct = new SpannerStruct
        {
            { "FirstName", SpannerDbType.String, "Timothy" },
            { "LastName", SpannerDbType.String, "Campbell" }
        };
        string connectionString = $"Data Source=projects/{projectId}/instances/{instanceId}/databases/{databaseId}";

        using var connection = new SpannerConnection(connectionString);
        await connection.OpenAsync();

        using var cmd = connection.CreateDmlCommand("UPDATE Singers SET LastName = 'Grant' WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) = @name");
        cmd.Parameters.Add("name", nameStruct.GetSpannerDbType(), nameStruct);
        int rowCount = await cmd.ExecuteNonQueryAsync();

        Console.WriteLine($"{rowCount} row(s) updated...");
        return rowCount;
    }
}
```

### Go

``` go
import (
 "context"
 "fmt"
 "io"

 "cloud.google.com/go/spanner"
)

func updateUsingDMLStruct(w io.Writer, db string) error {
 ctx := context.Background()
 client, err := spanner.NewClient(ctx, db)
 if err != nil {
     return err
 }
 defer client.Close()

 _, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
     type name struct {
         FirstName string
         LastName  string
     }
     var singerInfo = name{"Timothy", "Campbell"}

     stmt := spanner.Statement{
         SQL: `Update Singers Set LastName = 'Grant'
             WHERE STRUCT<FirstName String, LastName String>(Firstname, LastName) = @name`,
         Params: map[string]interface{}{"name": singerInfo},
     }
     rowCount, err := txn.Update(ctx, stmt)
     if err != nil {
         return err
     }
     fmt.Fprintf(w, "%d record(s) inserted.\n", rowCount)
     return nil
 })
 return err
}
```

### Java

``` java
static void updateUsingDmlWithStruct(DatabaseClient dbClient) {
  Struct name =
      Struct.newBuilder().set("FirstName").to("Timothy").set("LastName").to("Campbell").build();
  Statement s =
      Statement.newBuilder(
              "UPDATE Singers SET LastName = 'Grant' "
                  + "WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) "
                  + "= @name")
          .bind("name")
          .to(name)
          .build();
  dbClient
      .readWriteTransaction()
      .run(transaction -> {
        long rowCount = transaction.executeUpdate(s);
        System.out.printf("%d record updated.\n", rowCount);
        return null;
      });
}
```

### Node.js

``` javascript
// Imports the Google Cloud client library
const {Spanner} = require('@google-cloud/spanner');

const nameStruct = Spanner.struct({
  FirstName: 'Timothy',
  LastName: 'Campbell',
});

/**
 * TODO(developer): Uncomment the following lines before running the sample.
 */
// const projectId = 'my-project-id';
// const instanceId = 'my-instance';
// const databaseId = 'my-database';

// Creates a client
const spanner = new Spanner({
  projectId: projectId,
});

// Gets a reference to a Cloud Spanner instance and database
const instance = spanner.instance(instanceId);
const database = instance.database(databaseId);

database.runTransaction(async (err, transaction) => {
  if (err) {
    console.error(err);
    return;
  }
  try {
    const [rowCount] = await transaction.runUpdate({
      sql: `UPDATE Singers SET LastName = 'Grant'
      WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) = @name`,
      params: {
        name: nameStruct,
      },
    });

    console.log(`Successfully updated ${rowCount} record.`);
    await transaction.commit();
  } catch (err) {
    console.error('ERROR:', err);
  } finally {
    // Close the database when finished.
    database.close();
  }
});
```

### PHP

```` php
use Google\Cloud\Spanner\SpannerClient;
use Google\Cloud\Spanner\Database;
use Google\Cloud\Spanner\Transaction;
use Google\Cloud\Spanner\StructType;
use Google\Cloud\Spanner\StructValue;

/**
 * Update data with a DML statement using Structs.
 *
 * The database and table must already exist and can be created using
 * `create_database`.
 * Example:
 * ```
 * insert_data($instanceId, $databaseId);
 * ```
 *
 * @param string $instanceId The Spanner instance ID.
 * @param string $databaseId The Spanner database ID.
 */
function update_data_with_dml_structs(string $instanceId, string $databaseId): void
{
    $spanner = new SpannerClient();
    $instance = $spanner->instance($instanceId);
    $database = $instance->database($databaseId);

    $database->runTransaction(function (Transaction $t) {
        $nameValue = (new StructValue)
            ->add('FirstName', 'Timothy')
            ->add('LastName', 'Campbell');
        $nameType = (new StructType)
            ->add('FirstName', Database::TYPE_STRING)
            ->add('LastName', Database::TYPE_STRING);

        $rowCount = $t->executeUpdate(
            "UPDATE Singers SET LastName = 'Grant' "
             . 'WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) '
             . '= @name',
            [
                'parameters' => [
                    'name' => $nameValue
                ],
                'types' => [
                    'name' => $nameType
                ]
            ]);
        $t->commit();
        printf('Updated %d row(s).' . PHP_EOL, $rowCount);
    });
}
````

### Python

``` python
# instance_id = "your-spanner-instance"
# database_id = "your-spanner-db-id"

spanner_client = spanner.Client()
instance = spanner_client.instance(instance_id)
database = instance.database(database_id)

record_type = param_types.Struct(
    [
        param_types.StructField("FirstName", param_types.STRING),
        param_types.StructField("LastName", param_types.STRING),
    ]
)
record_value = ("Timothy", "Campbell")

def write_with_struct(transaction):
    row_ct = transaction.execute_update(
        "UPDATE Singers SET LastName = 'Grant' "
        "WHERE STRUCT<FirstName STRING, LastName STRING>"
        "(FirstName, LastName) = @name",
        params={"name": record_value},
        param_types={"name": record_type},
    )
    print("{} record(s) updated.".format(row_ct))

database.run_in_transaction(write_with_struct)
```

### Ruby

``` ruby
# project_id  = "Your Google Cloud project ID"
# instance_id = "Your Spanner instance ID"
# database_id = "Your Spanner database ID"

require "google/cloud/spanner"

spanner = Google::Cloud::Spanner.new project: project_id
client  = spanner.client instance_id, database_id
row_count = 0
name_struct = { FirstName: "Timothy", LastName: "Campbell" }

client.transaction do |transaction|
  row_count = transaction.execute_update(
    "UPDATE Singers SET LastName = 'Grant'
     WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) = @name",
    params: { name: name_struct }
  )
end

puts "#{row_count} record updated."
```

## Commit timestamps

Use the [`  PENDING_COMMIT_TIMESTAMP  `](/spanner/docs/reference/standard-sql/timestamp_functions) function to write commit timestamps to a `  TIMESTAMP  ` column. The column must have the `  allow_commit_timestamp  ` option set to `  true  ` . The following DML statement updates the `  LastUpdated  ` column in the `  Singers  ` table with the commit timestamp:

``` text
UPDATE Singers SET LastUpdated = PENDING_COMMIT_TIMESTAMP() WHERE SingerId = 1;
```

For more information on using commit timestamps in DML, see [Commit timestamps in GoogleSQL-dialect databases](/spanner/docs/commit-timestamp#dml) and [Commit timestamps in PostgreSQL-dialect databases](/spanner/docs/commit-timestamp-postgresql#dml) .

## Update fields in protocol buffers

You can update non-repeating and repeating fields in protocol buffers. Consider the [Singers example table](/spanner/docs/reference/standard-sql/dml-syntax#tables) . It contains a column, `  AlbumInfo  ` , of type `  Albums  ` , and the `  Albums  ` column contains a non-repeating field `  tracks  ` .

The following statement updates the value of `  tracks  ` :

``` text
UPDATE Singers s
SET s.AlbumInfo.tracks = 15
WHERE s.SingerId = 5 AND s.AlbumInfo.title = "Fire is hot";
```

You can also update a repeated field using an array of values:

``` text
UPDATE Singers s
SET s.AlbumInfo.comments = ["A good album!", "Hurt my ears!", "Totally unlistenable."]
WHERE s.SingerId = 5 AND s.AlbumInfo.title = "Fire is Hot";
```

### Nested updates

You can construct DML statements inside a parent update statement that modify a repeated field of a protocol buffer or an array. These statements are called *nested updates* .

For example, the `  Album  ` message contains a repeated field called `  comments  ` . This nested update statement adds a comment to an album:

``` text
UPDATE Singers s
SET (INSERT s.AlbumInfo.comments
     VALUES ("Groovy!"))
WHERE s.SingerId = 5 AND s.AlbumInfo.title = "Fire is Hot";
```

`  Album  ` also contains a repeated protocol buffer, `  Song  ` , which provides information about a song on the album. This nested update statement updates the album with a new song:

``` text
UPDATE Singers s
SET (INSERT s.AlbumInfo.Song(Song)
     VALUES ("songtitle: 'Bonus Track', length: 180"))
WHERE s.SingerId = 5 AND s.AlbumInfo.title = "Fire is Hot";
```

If the repeated field is another protocol buffer, you can provide the protocol buffer as a string literal. For example, the following statement adds a new song to the album and updates the number of tracks.

``` text
UPDATE Singers s
SET (INSERT s.AlbumInfo.Song
     VALUES ('''songtitle: 'Bonus Track', length:180''')),
     s.Albums.tracks = 16
WHERE s.SingerId = 5 and s.AlbumInfo.title = "Fire is Hot";
```

You can also nest a nested update statement in another nested update statement. For example, the `  Song  ` protocol buffer itself has another repeated protocol buffer, `  Chart  ` , which provides information on what chart the song appears on, and what rank it has.

The following statement adds a new chart to a song:

``` text
UPDATE Singers s
SET (UPDATE s.AlbumInfo.Song so
    SET (INSERT INTO so.Chart
         VALUES ("chartname: 'Galaxy Top 100', rank: 5"))
    WHERE so.songtitle = "Bonus Track")
WHERE s.SingerId = 5;
```

This following statement updates the chart to reflect a new rank for the song:

``` text
UPDATE Singers s
SET (UPDATE s.AlbumInfo.Song so
     SET (UPDATE so.Chart c
          SET c.rank = 2
          WHERE c.chartname = "Galaxy Top 100")
     WHERE so.songtitle = "Bonus Track")
WHERE s.SingerId = 5;
```

GoogleSQL treats an array or repeated field inside a row that matches an `  UPDATE WHERE  ` clause as a table, with individual elements of the array or field treated like rows. These rows can then have nested DML statements run against them, allowing you to delete, update, and insert data as needed.

### Modify multiple fields

The previous sections demonstrates how to update a single value in a compound data type. You can also modify multiple fields in a compound data type within a single statement. For example:

``` text
UPDATE Singers s
SET
    (DELETE FROM s.SingerInfo.Residence r WHERE r.City = 'Seattle'),
    (UPDATE s.AlbumInfo.Song song SET song.songtitle = 'No, This Is Rubbish' WHERE song.songtitle = 'This Is Pretty Good'),
    (INSERT s.AlbumInfo.Song VALUES ("songtitle: 'The Second Best Song'"))
WHERE SingerId = 3 AND s.AlbumInfo.title = 'Go! Go! Go!';
```

Nested queries are processed as follows:

1.  Delete all rows that match a `  WHERE  ` clause of a `  DELETE  ` statement.
2.  Update any remaining rows that match a `  WHERE  ` clause of an `  UPDATE  ` statement. Each row must match at most one `  UPDATE WHERE  ` clause, or the statement fails due to overlapping updates.
3.  Insert all rows in `  INSERT  ` statements.

You must construct nested statements that affect the same field in the following order:

  - `  DELETE  `
  - `  UPDATE  `
  - `  INSERT  `

For example:

``` text
UPDATE Singers s
SET
    (DELETE FROM s.SingerInfo.Residence r WHERE r.City = 'Seattle'),
    (UPDATE s.SingerInfo.Residence r SET r.end_year = 2015 WHERE r.City = 'Eugene'),
    (INSERT s.AlbumInfo.Song VALUES ("songtitle: 'The Second Best Song'"))
WHERE SingerId = 3 AND s.AlbumInfo.title = 'Go! Go! Go!';
```

The following statement is invalid, because the `  UPDATE  ` statement happens after the `  INSERT  ` statement.

``` text
UPDATE Singers s
SET
    (DELETE FROM s.SingerInfo.Residence r WHERE r.City = 'Seattle'),
    (INSERT s.AlbumInfo.Song VALUES ("songtitle: 'The Second Best Song'")),
    (UPDATE s.SingerInfo.Residence r SET r.end_year = 2015 WHERE r.City = 'Eugene')
WHERE SingerId = 3 AND s.AlbumInfo.title = 'Go! Go! Go!';
```

In nested queries, you can't use `  INSERT OR REPLACE  ` statements. These types of statements don't work because arrays and other compound data types don't always have a primary key, so there is no applicable definition of duplicate rows.
