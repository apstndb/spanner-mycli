The GoogleSQL procedural language lets you execute multiple statements in one query as a multi-statement query. You can use a multi-statement query to:

  - Run multiple statements in a sequence, with shared state.
  - Automate management tasks such as creating or dropping tables.

## Transactions

### `     BEGIN TRANSACTION    `

**Syntax**

``` text
BEGIN [TRANSACTION];
```

**Description**

Begins a transaction.

The transaction ends when a [`  COMMIT TRANSACTION  `](#commit_transaction) or [`  ROLLBACK TRANSACTION  `](#rollback_transaction) statement is reached. If execution ends before reaching either of these statements, an automatic rollback occurs.

**Example**

The following example performs a transaction that selects rows from an existing table into a temporary table, deletes those rows from the original table, and merges the temporary table into another table.

``` text
BEGIN TRANSACTION;

-- Create a temporary table of new arrivals from warehouse #1
CREATE TEMP TABLE tmp AS
SELECT * FROM myschema.NewArrivals WHERE warehouse = 'warehouse #1';

-- Delete the matching records from the original table.
DELETE myschema.NewArrivals WHERE warehouse = 'warehouse #1';

-- Merge the matching records into the Inventory table.
MERGE myschema.Inventory AS I
USING tmp AS T
ON I.product = T.product
WHEN NOT MATCHED THEN
 INSERT(product, quantity, supply_constrained)
 VALUES(product, quantity, false)
WHEN MATCHED THEN
 UPDATE SET quantity = I.quantity + T.quantity;

DROP TABLE tmp;

COMMIT TRANSACTION;
```

### `     COMMIT TRANSACTION    `

**Syntax**

``` text
COMMIT [TRANSACTION];
```

**Description**

Commits an open transaction. If no open transaction is in progress, then the statement fails.

**Example**

``` text
BEGIN TRANSACTION;

-- SQL statements for the transaction go here.

COMMIT TRANSACTION;
```

### `     ROLLBACK TRANSACTION    `

**Syntax**

``` text
ROLLBACK [TRANSACTION];
```

**Description**

Rolls back an open transaction. If there is no open transaction in progress, then the statement fails.

**Example**

The following example rolls back a transaction if an error occurs during the transaction. To illustrate the logic, the example triggers a divide-by-zero error after inserting a row into a table. After these statements run, the table is unaffected.

``` text
BEGIN

  BEGIN TRANSACTION;
  INSERT INTO myschema.NewArrivals
    VALUES ('top load washer', 100, 'warehouse #1');
  -- Trigger an error.
  SELECT 1/0;
  COMMIT TRANSACTION;

EXCEPTION WHEN ERROR THEN
  -- Roll back the transaction inside the exception handler.
  SELECT @@error.message;
  ROLLBACK TRANSACTION;
END;
```

## `     CALL    `

**Syntax**

``` text
CALL procedure_name (procedure_argument[, …])
```

**Description**

Calls a [procedure](/spanner/docs/reference/standard-sql/stored-procedures) with an argument list. `  procedure_argument  ` may be a variable or an expression. Spanner doesn't support stored procedures or server-side scripts. Use `  CALL  ` with only the example procedures listed on this page.

The maximum depth of procedure calls is 50 frames.

**Examples**

The following example cancels a query with the query ID `  12345  ` .

``` text
CALL cancel_query("12345");
```
