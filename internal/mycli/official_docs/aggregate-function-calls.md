An aggregate function summarizes the rows of a group into a single value.

## Aggregate function call syntax

``` text
function_name(
  [ DISTINCT ]
  function_arguments
  [ { IGNORE | RESPECT } NULLS ]
  [ HAVING { MAX | MIN } having_expression ]
  [ ORDER BY key [ { ASC | DESC } ] [, ... ] ]
  [ LIMIT n ]
)
```

**Description**

Each aggregate function supports all or a subset of the aggregate function call syntax. You can use the following syntax to build an aggregate function:

  - `  DISTINCT  ` : Each distinct value of `  expression  ` is aggregated only once into the result.

  - `  IGNORE NULLS  ` or `  RESPECT NULLS  ` : If `  IGNORE NULLS  ` is specified, the `  NULL  ` values are excluded from the result. If `  RESPECT NULLS  ` is specified, both `  NULL  ` and non- `  NULL  ` values can be included in the result.
    
    If neither `  IGNORE NULLS  ` nor `  RESPECT NULLS  ` is specified, most functions default to `  IGNORE NULLS  ` behavior but in a few cases `  NULL  ` values are respected.

  - `  HAVING MAX  ` or `  HAVING MIN  ` : Restricts the set of rows that the function aggregates by a maximum or minimum value. For details, see [HAVING MAX and HAVING MIN clause](#max_min_clause) .

  - `  ORDER BY  ` : Specifies the order of the values.
    
      - For each sort key, the default sort direction is `  ASC  ` .
    
      - `  NULL  ` is the minimum possible value, so `  NULL  ` s appear first in `  ASC  ` sorts and last in `  DESC  ` sorts.
    
      - If you're using floating point data types, see [Floating point semantics](/spanner/docs/reference/standard-sql/data-types#floating_point_semantics) on ordering and grouping.
    
      - The `  ORDER BY  ` clause is supported only for aggregate functions that depend on the order of their input. For those functions, if the `  ORDER BY  ` clause is omitted, the output is nondeterministic.
    
      - If `  DISTINCT  ` is also specified, then the sort key must be the same as `  expression  ` .

  - `  LIMIT  ` : Specifies the maximum number of `  expression  ` inputs in the result.
    
      - If the input is an `  ARRAY  ` value, the limit applies to the number of input arrays, not the number of elements in the arrays. An empty array counts as `  1  ` . A `  NULL  ` array isn't counted.
    
      - If the input is a `  STRING  ` value, the limit applies to the number of input strings, not the number of characters or bytes in the inputs. An empty string counts as `  1  ` . A `  NULL  ` string isn't counted.
    
      - The limit `  n  ` must be a constant `  INT64  ` .

**Details**

The clauses in an aggregate function call are applied in the following order:

  - `  HAVING MAX  ` / `  HAVING MIN  `
  - `  IGNORE NULLS  ` or `  RESPECT NULLS  `
  - `  DISTINCT  `
  - `  ORDER BY  `
  - `  LIMIT  `

When used in conjunction with a `  GROUP BY  ` clause, the groups summarized typically have at least one row. When the associated `  SELECT  ` statement has no `  GROUP BY  ` clause or when certain aggregate function modifiers filter rows from the group to be summarized, it's possible that the aggregate function needs to summarize an empty group.

## Restrict aggregation by a maximum or minimum value

Some aggregate functions support two optional clauses that are called `  HAVING MAX  ` and `  HAVING MIN  ` . These clauses restrict the set of rows that a function aggregates to rows that have a maximum or minimum value in a particular column.

### HAVING MAX clause

``` text
HAVING MAX having_expression
```

`  HAVING MAX  ` restricts the set of input rows that the function aggregates to only those with the maximum `  having_expression  ` value. The maximum value is computed as the result of `  MAX(having_expression)  ` across rows in the group. Only rows whose `  having_expression  ` value is equal to this maximum value (using SQL equality semantics) are included in the aggregation. All other rows are ignored in the aggregation.

This clause supports all [orderable data types](/spanner/docs/reference/standard-sql/data-types#data_type_properties) , except for `  ARRAY  ` .

**Examples**

In the following query, rows with the most inches of precipitation, `  4  ` , are added to a group, and then the `  year  ` for one of these rows is produced. Which row is produced is nondeterministic, not random.

``` text
WITH
  Precipitation AS (
    SELECT 2009 AS year, 'spring' AS season, 3 AS inches
    UNION ALL
    SELECT 2001, 'winter', 4
    UNION ALL
    SELECT 2003, 'fall', 1
    UNION ALL
    SELECT 2002, 'spring', 4
    UNION ALL
    SELECT 2005, 'summer', 1
  )
SELECT ANY_VALUE(year HAVING MAX inches) AS any_year_with_max_inches FROM Precipitation;

/*--------------------------+
 | any_year_with_max_inches |
 +--------------------------+
 | 2001                     |
 +--------------------------*/
```

In the following example, the average rainfall is returned for 2001, the most recent year specified in the query. First, the query gets the rows with the maximum value in the `  year  ` column. Finally, the query averages the values in the `  inches  ` column ( `  9  ` and `  1  ` ):

``` text
WITH
  Precipitation AS (
    SELECT 2001 AS year, 'spring' AS season, 9 AS inches
    UNION ALL
    SELECT 2001, 'winter', 1
    UNION ALL
    SELECT 2000, 'fall', 3
    UNION ALL
    SELECT 2000, 'summer', 5
    UNION ALL
    SELECT 2000, 'spring', 7
    UNION ALL
    SELECT 2000, 'winter', 2
  )
SELECT AVG(inches HAVING MAX year) AS average FROM Precipitation;

/*---------+
 | average |
 +---------+
 | 5       |
 +---------*/
```

### HAVING MIN clause

``` text
HAVING MIN having_expression
```

`  HAVING MIN  ` restricts the set of input rows that the function aggregates to only those with the minimum `  having_expression  ` value. The minimum value is computed as the result of `  MIN(having_expression)  ` across rows in the group. Only rows whose `  having_expression  ` value is equal to this minimum value (using SQL equality semantics) are included in the aggregation. All other rows are ignored in the aggregation.

This clause supports all [orderable data types](/spanner/docs/reference/standard-sql/data-types#data_type_properties) , except for `  ARRAY  ` .

**Examples**

In the following query, rows with the fewest inches of precipitation, `  1  ` , are added to a group, and then the `  year  ` for one of these rows is produced. Which row is produced is nondeterministic, not random.

``` text
WITH
  Precipitation AS (
    SELECT 2009 AS year, 'spring' AS season, 3 AS inches
    UNION ALL
    SELECT 2001, 'winter', 4
    UNION ALL
    SELECT 2003, 'fall', 1
    UNION ALL
    SELECT 2002, 'spring', 4
    UNION ALL
    SELECT 2005, 'summer', 1
  )
SELECT ANY_VALUE(year HAVING MIN inches) AS any_year_with_min_inches FROM Precipitation;

/*--------------------------+
 | any_year_with_min_inches |
 +--------------------------+
 | 2003                     |
 +--------------------------*/
```

In the following example, the average rainfall is returned for 2000, the earliest year specified in the query. First, the query gets the rows with the minimum value in the `  year  ` column, and finally, the query averages the values in the `  inches  ` column:

``` text
WITH
  Precipitation AS (
    SELECT 2001 AS year, 'spring' AS season, 9 AS inches
    UNION ALL
    SELECT 2001, 'winter', 1
    UNION ALL
    SELECT 2000, 'fall', 3
    UNION ALL
    SELECT 2000, 'summer', 5
    UNION ALL
    SELECT 2000, 'spring', 7
    UNION ALL
    SELECT 2000, 'winter', 2
  )
SELECT AVG(inches HAVING MIN year) AS average FROM Precipitation;

/*---------+
 | average |
 +---------+
 | 4.25    |
 +---------*/
```

## Aggregate function examples

A simple aggregate function call for `  COUNT  ` , `  MIN  ` , and `  MAX  ` looks like this:

``` text
SELECT
  COUNT(*) AS total_count,
  COUNT(fruit) AS non_null_count,
  MIN(fruit) AS min,
  MAX(fruit) AS max
FROM
  (
    SELECT NULL AS fruit
    UNION ALL
    SELECT 'apple' AS fruit
    UNION ALL
    SELECT 'pear' AS fruit
    UNION ALL
    SELECT 'orange' AS fruit
  )

/*-------------+----------------+-------+------+
 | total_count | non_null_count | min   | max  |
 +-------------+----------------+-------+------+
 | 4           | 3              | apple | pear |
 +-------------+----------------+-------+------*/
```
