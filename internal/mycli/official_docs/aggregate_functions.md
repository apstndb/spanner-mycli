GoogleSQL for Spanner supports the following general aggregate functions. To learn about the syntax for aggregate function calls, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

## Function list

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<thead>
<tr class="header">
<th>Name</th>
<th>Summary</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#any_value"><code dir="ltr" translate="no">        ANY_VALUE       </code></a></td>
<td>Gets an expression for some row.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#array_agg"><code dir="ltr" translate="no">        ARRAY_AGG       </code></a></td>
<td>Gets an array of values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#array_concat_agg"><code dir="ltr" translate="no">        ARRAY_CONCAT_AGG       </code></a></td>
<td>Concatenates arrays and returns a single array as a result.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#avg"><code dir="ltr" translate="no">        AVG       </code></a></td>
<td>Gets the average of non- <code dir="ltr" translate="no">       NULL      </code> values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_and"><code dir="ltr" translate="no">        BIT_AND       </code></a></td>
<td>Performs a bitwise AND operation on an expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_or"><code dir="ltr" translate="no">        BIT_OR       </code></a></td>
<td>Performs a bitwise OR operation on an expression.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_xor"><code dir="ltr" translate="no">        BIT_XOR       </code></a></td>
<td>Performs a bitwise XOR operation on an expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#count"><code dir="ltr" translate="no">        COUNT       </code></a></td>
<td>Gets the number of rows in the input, or the number of rows with an expression evaluated to any value other than <code dir="ltr" translate="no">       NULL      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#countif"><code dir="ltr" translate="no">        COUNTIF       </code></a></td>
<td>Gets the number of <code dir="ltr" translate="no">       TRUE      </code> values for an expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#logical_and"><code dir="ltr" translate="no">        LOGICAL_AND       </code></a></td>
<td>Gets the logical AND of all non- <code dir="ltr" translate="no">       NULL      </code> expressions.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#logical_or"><code dir="ltr" translate="no">        LOGICAL_OR       </code></a></td>
<td>Gets the logical OR of all non- <code dir="ltr" translate="no">       NULL      </code> expressions.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#max"><code dir="ltr" translate="no">        MAX       </code></a></td>
<td>Gets the maximum non- <code dir="ltr" translate="no">       NULL      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#min"><code dir="ltr" translate="no">        MIN       </code></a></td>
<td>Gets the minimum non- <code dir="ltr" translate="no">       NULL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#stddev"><code dir="ltr" translate="no">        STDDEV       </code></a></td>
<td>An alias of the <code dir="ltr" translate="no">       STDDEV_SAMP      </code> function.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions">Statistical aggregate functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#stddev_samp"><code dir="ltr" translate="no">        STDDEV_SAMP       </code></a></td>
<td>Computes the sample (unbiased) standard deviation of the values.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions">Statistical aggregate functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#string_agg"><code dir="ltr" translate="no">        STRING_AGG       </code></a></td>
<td>Concatenates non- <code dir="ltr" translate="no">       NULL      </code> <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#sum"><code dir="ltr" translate="no">        SUM       </code></a></td>
<td>Gets the sum of non- <code dir="ltr" translate="no">       NULL      </code> values.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#var_samp"><code dir="ltr" translate="no">        VAR_SAMP       </code></a></td>
<td>Computes the sample (unbiased) variance of the values.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions">Statistical aggregate functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#variance"><code dir="ltr" translate="no">        VARIANCE       </code></a></td>
<td>An alias of <code dir="ltr" translate="no">       VAR_SAMP      </code> .<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions">Statistical aggregate functions</a> .</td>
</tr>
</tbody>
</table>

## `     ANY_VALUE    `

``` text
ANY_VALUE(
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns `  expression  ` for some row chosen from the group. Which row is chosen is nondeterministic, not random. Returns `  NULL  ` when the input produces no rows. Returns `  NULL  ` when `  expression  ` or `  having_expression  ` is `  NULL  ` for all rows in the group.

If `  expression  ` contains any non-NULL values, then `  ANY_VALUE  ` behaves as if `  IGNORE NULLS  ` is specified; rows for which `  expression  ` is `  NULL  ` aren't considered and won't be selected.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

Any

**Returned Data Types**

Matches the input data type.

**Examples**

``` text
SELECT ANY_VALUE(fruit) as any_value
FROM UNNEST(["apple", "banana", "pear"]) as fruit;

/*-----------+
 | any_value |
 +-----------+
 | apple     |
 +-----------*/
```

``` text
WITH
  Store AS (
    SELECT 20 AS sold, "apples" AS fruit
    UNION ALL
    SELECT 30 AS sold, "pears" AS fruit
    UNION ALL
    SELECT 30 AS sold, "bananas" AS fruit
    UNION ALL
    SELECT 10 AS sold, "oranges" AS fruit
  )
SELECT ANY_VALUE(fruit HAVING MAX sold) AS a_highest_selling_fruit FROM Store;

/*-------------------------+
 | a_highest_selling_fruit |
 +-------------------------+
 | pears                   |
 +-------------------------*/
```

``` text
WITH
  Store AS (
    SELECT 20 AS sold, "apples" AS fruit
    UNION ALL
    SELECT 30 AS sold, "pears" AS fruit
    UNION ALL
    SELECT 30 AS sold, "bananas" AS fruit
    UNION ALL
    SELECT 10 AS sold, "oranges" AS fruit
  )
SELECT ANY_VALUE(fruit HAVING MIN sold) AS a_lowest_selling_fruit FROM Store;

/*-------------------------+
 | a_lowest_selling_fruit  |
 +-------------------------+
 | oranges                 |
 +-------------------------*/
```

## `     ARRAY_AGG    `

``` text
ARRAY_AGG(
  [ DISTINCT ]
  expression
  [ { IGNORE | RESPECT } NULLS ]
  [ HAVING { MAX | MIN } having_expression ]
  [ ORDER BY key [ { ASC | DESC } ] [, ... ] ]
  [ LIMIT n ]
)
```

**Description**

Returns an ARRAY of `  expression  ` values.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

All data types except ARRAY.

**Returned Data Types**

ARRAY

If there are zero input rows, this function returns `  NULL  ` .

**Examples**

``` text
SELECT ARRAY_AGG(x) AS array_agg FROM UNNEST([2, 1,-2, 3, -2, 1, 2]) AS x;

/*-------------------------+
 | array_agg               |
 +-------------------------+
 | [2, 1, -2, 3, -2, 1, 2] |
 +-------------------------*/
```

``` text
SELECT ARRAY_AGG(DISTINCT x) AS array_agg
FROM UNNEST([2, 1, -2, 3, -2, 1, 2]) AS x;

/*---------------+
 | array_agg     |
 +---------------+
 | [2, 1, -2, 3] |
 +---------------*/
```

``` text
SELECT ARRAY_AGG(x IGNORE NULLS) AS array_agg
FROM UNNEST([NULL, 1, -2, 3, -2, 1, NULL]) AS x;

/*-------------------+
 | array_agg         |
 +-------------------+
 | [1, -2, 3, -2, 1] |
 +-------------------*/
```

``` text
SELECT ARRAY_AGG(x ORDER BY ABS(x)) AS array_agg
FROM UNNEST([2, 1, -2, 3, -2, 1, 2]) AS x;

/*-------------------------+
 | array_agg               |
 +-------------------------+
 | [1, 1, 2, -2, -2, 2, 3] |
 +-------------------------*/
```

``` text
SELECT ARRAY_AGG(x LIMIT 5) AS array_agg
FROM UNNEST([2, 1, -2, 3, -2, 1, 2]) AS x;

/*-------------------+
 | array_agg         |
 +-------------------+
 | [2, 1, -2, 3, -2] |
 +-------------------*/
```

``` text
WITH vals AS
  (
    SELECT 1 x UNION ALL
    SELECT -2 x UNION ALL
    SELECT 3 x UNION ALL
    SELECT -2 x UNION ALL
    SELECT 1 x
  )
SELECT ARRAY_AGG(DISTINCT x ORDER BY x) as array_agg
FROM vals;

/*------------+
 | array_agg  |
 +------------+
 | [-2, 1, 3] |
 +------------*/
```

``` text
WITH vals AS
  (
    SELECT 1 x, 'a' y UNION ALL
    SELECT 1 x, 'b' y UNION ALL
    SELECT 2 x, 'a' y UNION ALL
    SELECT 2 x, 'c' y
  )
SELECT x, ARRAY_AGG(y) as array_agg
FROM vals
GROUP BY x;

/*---------------+
 | x | array_agg |
 +---------------+
 | 1 | [a, b]    |
 | 2 | [a, c]    |
 +---------------*/
```

## `     ARRAY_CONCAT_AGG    `

``` text
ARRAY_CONCAT_AGG(
  expression
  [ HAVING { MAX | MIN } having_expression ]
  [ ORDER BY key [ { ASC | DESC } ] [, ... ] ]
  [ LIMIT n ]
)
```

**Description**

Concatenates elements from `  expression  ` of type `  ARRAY  ` , returning a single array as a result.

This function ignores `  NULL  ` input arrays, but respects the `  NULL  ` elements in non- `  NULL  ` input arrays. Returns `  NULL  ` if there are zero input rows or `  expression  ` evaluates to `  NULL  ` for all rows.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

`  ARRAY  `

**Returned Data Types**

`  ARRAY  `

**Examples**

``` text
SELECT ARRAY_CONCAT_AGG(x) AS array_concat_agg FROM (
  SELECT [NULL, 1, 2, 3, 4] AS x
  UNION ALL SELECT NULL
  UNION ALL SELECT [5, 6]
  UNION ALL SELECT [7, 8, 9]
);

/*-----------------------------------+
 | array_concat_agg                  |
 +-----------------------------------+
 | [NULL, 1, 2, 3, 4, 5, 6, 7, 8, 9] |
 +-----------------------------------*/
```

``` text
SELECT ARRAY_CONCAT_AGG(x ORDER BY ARRAY_LENGTH(x)) AS array_concat_agg FROM (
  SELECT [1, 2, 3, 4] AS x
  UNION ALL SELECT [5, 6]
  UNION ALL SELECT [7, 8, 9]
);

/*-----------------------------------+
 | array_concat_agg                  |
 +-----------------------------------+
 | [5, 6, 7, 8, 9, 1, 2, 3, 4]       |
 +-----------------------------------*/
```

``` text
SELECT ARRAY_CONCAT_AGG(x LIMIT 2) AS array_concat_agg FROM (
  SELECT [1, 2, 3, 4] AS x
  UNION ALL SELECT [5, 6]
  UNION ALL SELECT [7, 8, 9]
);

/*--------------------------+
 | array_concat_agg         |
 +--------------------------+
 | [1, 2, 3, 4, 5, 6]       |
 +--------------------------*/
```

``` text
SELECT ARRAY_CONCAT_AGG(x ORDER BY ARRAY_LENGTH(x) LIMIT 2) AS array_concat_agg FROM (
  SELECT [1, 2, 3, 4] AS x
  UNION ALL SELECT [5, 6]
  UNION ALL SELECT [7, 8, 9]
);

/*------------------+
 | array_concat_agg |
 +------------------+
 | [5, 6, 7, 8, 9]  |
 +------------------*/
```

## `     AVG    `

``` text
AVG(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the average of non- `  NULL  ` values in an aggregated group.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

Caveats:

  - If the aggregated group is empty or the argument is `  NULL  ` for all rows in the group, returns `  NULL  ` .
  - If the argument is `  NaN  ` for any row in the group, returns `  NaN  ` .
  - If the argument is `  [+|-]Infinity  ` for any row in the group, returns either `  [+|-]Infinity  ` or `  NaN  ` .
  - If there is numeric overflow, produces an error.
  - If a [floating-point type](/spanner/docs/reference/standard-sql/data-types#floating_point_types) is returned, the result is [non-deterministic](/spanner/docs/reference/standard-sql/data-types#floating_point_semantics) , which means you might receive a different result each time you use this function.

**Supported Argument Types**

  - Any numeric input type
  - `  INTERVAL  `

**Returned Data Types**

<table>
<thead>
<tr class="header">
<th>INPUT</th>
<th><code dir="ltr" translate="no">       INT64      </code></th>
<th><code dir="ltr" translate="no">       NUMERIC      </code></th>
<th><code dir="ltr" translate="no">       FLOAT32      </code></th>
<th><code dir="ltr" translate="no">       FLOAT64      </code></th>
<th><code dir="ltr" translate="no">       INTERVAL      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>OUTPUT</td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL      </code></td>
</tr>
</tbody>
</table>

**Examples**

``` text
SELECT AVG(x) as avg
FROM UNNEST([0, 2, 4, 4, 5]) as x;

/*-----+
 | avg |
 +-----+
 | 3   |
 +-----*/
```

``` text
SELECT AVG(DISTINCT x) AS avg
FROM UNNEST([0, 2, 4, 4, 5]) AS x;

/*------+
 | avg  |
 +------+
 | 2.75 |
 +------*/
```

## `     BIT_AND    `

``` text
BIT_AND(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Performs a bitwise AND operation on `  expression  ` and returns the result.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

  - INT64

**Returned Data Types**

INT64

**Examples**

``` text
SELECT BIT_AND(x) as bit_and FROM UNNEST([0xF001, 0x00A1]) as x;

/*---------+
 | bit_and |
 +---------+
 | 1       |
 +---------*/
```

## `     BIT_OR    `

``` text
BIT_OR(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Performs a bitwise OR operation on `  expression  ` and returns the result.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

  - INT64

**Returned Data Types**

INT64

**Examples**

``` text
SELECT BIT_OR(x) as bit_or FROM UNNEST([0xF001, 0x00A1]) as x;

/*--------+
 | bit_or |
 +--------+
 | 61601  |
 +--------*/
```

## `     BIT_XOR    `

``` text
BIT_XOR(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Performs a bitwise XOR operation on `  expression  ` and returns the result.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

  - INT64

**Returned Data Types**

INT64

**Examples**

``` text
SELECT BIT_XOR(x) AS bit_xor FROM UNNEST([5678, 1234]) AS x;

/*---------+
 | bit_xor |
 +---------+
 | 4860    |
 +---------*/
```

``` text
SELECT BIT_XOR(x) AS bit_xor FROM UNNEST([1234, 5678, 1234]) AS x;

/*---------+
 | bit_xor |
 +---------+
 | 5678    |
 +---------*/
```

``` text
SELECT BIT_XOR(DISTINCT x) AS bit_xor FROM UNNEST([1234, 5678, 1234]) AS x;

/*---------+
 | bit_xor |
 +---------+
 | 4860    |
 +---------*/
```

## `     COUNT    `

``` text
COUNT(*)
```

``` text
COUNT(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Gets the number of rows in the input or the number of rows with an expression evaluated to any value other than `  NULL  ` .

**Definitions**

  - `  *  ` : Use this value to get the number of all rows in the input.
  - `  expression  ` : A value of any data type that represents the expression to evaluate. If `  DISTINCT  ` is present, `  expression  ` can only be a data type that is [groupable](/spanner/docs/reference/standard-sql/data-types#groupable_data_types) .
  - `  DISTINCT  ` : To learn more, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .
  - `  HAVING { MAX | MIN }  ` : To learn more, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Details**

To count the number of distinct values of an expression for which a certain condition is satisfied, you can use the following recipe:

``` text
COUNT(DISTINCT IF(condition, expression, NULL))
```

`  IF  ` returns the value of `  expression  ` if `  condition  ` is `  TRUE  ` , or `  NULL  ` otherwise. The surrounding `  COUNT(DISTINCT ...)  ` ignores the `  NULL  ` values, so it counts only the distinct values of `  expression  ` for which `  condition  ` is `  TRUE  ` .

To count the number of non-distinct values of an expression for which a certain condition is satisfied, consider using the [`  COUNTIF  `](/spanner/docs/reference/standard-sql/aggregate_functions#countif) function.

**Return type**

`  INT64  `

**Examples**

You can use the `  COUNT  ` function to return the number of rows in a table or the number of distinct values of an expression. For example:

``` text
SELECT
  COUNT(*) AS count_star,
  COUNT(DISTINCT x) AS count_dist_x
FROM UNNEST([1, 4, 4, 5]) AS x;

/*------------+--------------+
 | count_star | count_dist_x |
 +------------+--------------+
 | 4          | 3            |
 +------------+--------------*/
```

``` text
SELECT COUNT(*) AS count_star, COUNT(x) AS count_x
FROM UNNEST([1, 4, NULL, 4, 5]) AS x;

/*------------+---------+
 | count_star | count_x |
 +------------+---------+
 | 5          | 4       |
 +------------+---------*/
```

The following query counts the number of distinct positive values of `  x  ` :

``` text
SELECT COUNT(DISTINCT IF(x > 0, x, NULL)) AS distinct_positive
FROM UNNEST([1, -2, 4, 1, -5, 4, 1, 3, -6, 1]) AS x;

/*-------------------+
 | distinct_positive |
 +-------------------+
 | 3                 |
 +-------------------*/
```

The following query counts the number of distinct dates on which a certain kind of event occurred:

``` text
WITH Events AS (
  SELECT DATE '2021-01-01' AS event_date, 'SUCCESS' AS event_type
  UNION ALL
  SELECT DATE '2021-01-02' AS event_date, 'SUCCESS' AS event_type
  UNION ALL
  SELECT DATE '2021-01-02' AS event_date, 'FAILURE' AS event_type
  UNION ALL
  SELECT DATE '2021-01-03' AS event_date, 'SUCCESS' AS event_type
  UNION ALL
  SELECT DATE '2021-01-04' AS event_date, 'FAILURE' AS event_type
  UNION ALL
  SELECT DATE '2021-01-04' AS event_date, 'FAILURE' AS event_type
)
SELECT
  COUNT(DISTINCT IF(event_type = 'FAILURE', event_date, NULL))
    AS distinct_dates_with_failures
FROM Events;

/*------------------------------+
 | distinct_dates_with_failures |
 +------------------------------+
 | 2                            |
 +------------------------------*/
```

The following query counts the number of distinct `  id  ` s that exist in both the `  customers  ` and `  vendor  ` tables:

``` text
WITH
  customers AS (
    SELECT 1934 AS id, 'a' AS team UNION ALL
    SELECT 2991, 'b' UNION ALL
    SELECT 3988, 'c'),
  vendors AS (
    SELECT 1934 AS id, 'd' AS team UNION ALL
    SELECT 2991, 'e' UNION ALL
    SELECT 4366, 'f')
SELECT
  COUNT(DISTINCT IF(id IN (SELECT id FROM customers), id, NULL)) AS result
FROM vendors;

/*--------+
 | result |
 +--------+
 | 2      |
 +--------*/
```

## `     COUNTIF    `

``` text
COUNTIF(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Gets the number of `  TRUE  ` values for an expression.

**Definitions**

  - `  expression  ` : A `  BOOL  ` value that represents the expression to evaluate.
  - `  DISTINCT  ` : To learn more, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .
  - `  HAVING { MAX | MIN }  ` : To learn more, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Details**

The function signature `  COUNTIF(DISTINCT ...)  ` is generally not useful. If you would like to use `  DISTINCT  ` , use `  COUNT  ` with `  DISTINCT IF  ` . For more information, see the [`  COUNT  `](/spanner/docs/reference/standard-sql/aggregate_functions#count) function.

**Return type**

`  INT64  `

**Examples**

``` text
SELECT COUNTIF(x<0) AS num_negative, COUNTIF(x>0) AS num_positive
FROM UNNEST([5, -2, 3, 6, -10, -7, 4, 0]) AS x;

/*--------------+--------------+
 | num_negative | num_positive |
 +--------------+--------------+
 | 3            | 4            |
 +--------------+--------------*/
```

## `     LOGICAL_AND    `

``` text
LOGICAL_AND(
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the logical AND of all non- `  NULL  ` expressions. Returns `  NULL  ` if there are zero input rows or `  expression  ` evaluates to `  NULL  ` for all rows.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

`  BOOL  `

**Return Data Types**

`  BOOL  `

**Examples**

`  LOGICAL_AND  ` returns `  FALSE  ` because not all of the values in the array are less than 3.

``` text
SELECT LOGICAL_AND(x < 3) AS logical_and FROM UNNEST([1, 2, 4]) AS x;

/*-------------+
 | logical_and |
 +-------------+
 | FALSE       |
 +-------------*/
```

## `     LOGICAL_OR    `

``` text
LOGICAL_OR(
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the logical OR of all non- `  NULL  ` expressions. Returns `  NULL  ` if there are zero input rows or `  expression  ` evaluates to `  NULL  ` for all rows.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

`  BOOL  `

**Return Data Types**

`  BOOL  `

**Examples**

`  LOGICAL_OR  ` returns `  TRUE  ` because at least one of the values in the array is less than 3.

``` text
SELECT LOGICAL_OR(x < 3) AS logical_or FROM UNNEST([1, 2, 4]) AS x;

/*------------+
 | logical_or |
 +------------+
 | TRUE       |
 +------------*/
```

## `     MAX    `

``` text
MAX(
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the maximum non- `  NULL  ` value in an aggregated group.

Caveats:

  - If the aggregated group is empty or the argument is `  NULL  ` for all rows in the group, returns `  NULL  ` .
  - If the argument is `  NaN  ` for any row in the group, returns `  NaN  ` .

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

Any [orderable data type](/spanner/docs/reference/standard-sql/data-types#data_type_properties) except for `  ARRAY  ` .

**Return Data Types**

The data type of the input values.

**Examples**

``` text
SELECT MAX(x) AS max
FROM UNNEST([8, 37, 55, 4]) AS x;

/*-----+
 | max |
 +-----+
 | 55  |
 +-----*/
```

## `     MIN    `

``` text
MIN(
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the minimum non- `  NULL  ` value in an aggregated group.

Caveats:

  - If the aggregated group is empty or the argument is `  NULL  ` for all rows in the group, returns `  NULL  ` .
  - If the argument is `  NaN  ` for any row in the group, returns `  NaN  ` .

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

Any [orderable data type](/spanner/docs/reference/standard-sql/data-types#data_type_properties) except for `  ARRAY  ` .

**Return Data Types**

The data type of the input values.

**Examples**

``` text
SELECT MIN(x) AS min
FROM UNNEST([8, 37, 4, 55]) AS x;

/*-----+
 | min |
 +-----+
 | 4   |
 +-----*/
```

## `     STRING_AGG    `

``` text
STRING_AGG(
  [ DISTINCT ]
  expression [, delimiter]
  [ HAVING { MAX | MIN } having_expression ]
  [ ORDER BY key [ { ASC | DESC } ] [, ... ] ]
  [ LIMIT n ]
)
```

**Description**

Returns a value (either `  STRING  ` or `  BYTES  ` ) obtained by concatenating non- `  NULL  ` values. Returns `  NULL  ` if there are zero input rows or `  expression  ` evaluates to `  NULL  ` for all rows.

If a `  delimiter  ` is specified, concatenated values are separated by that delimiter; otherwise, a comma is used as a delimiter.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Supported Argument Types**

Either `  STRING  ` or `  BYTES  ` .

**Return Data Types**

Either `  STRING  ` or `  BYTES  ` .

**Examples**

``` text
SELECT STRING_AGG(fruit) AS string_agg
FROM UNNEST(["apple", NULL, "pear", "banana", "pear"]) AS fruit;

/*------------------------+
 | string_agg             |
 +------------------------+
 | apple,pear,banana,pear |
 +------------------------*/
```

``` text
SELECT STRING_AGG(fruit, " & ") AS string_agg
FROM UNNEST(["apple", "pear", "banana", "pear"]) AS fruit;

/*------------------------------+
 | string_agg                   |
 +------------------------------+
 | apple & pear & banana & pear |
 +------------------------------*/
```

``` text
SELECT STRING_AGG(DISTINCT fruit, " & ") AS string_agg
FROM UNNEST(["apple", "pear", "banana", "pear"]) AS fruit;

/*-----------------------+
 | string_agg            |
 +-----------------------+
 | apple & pear & banana |
 +-----------------------*/
```

``` text
SELECT STRING_AGG(fruit, " & " ORDER BY LENGTH(fruit)) AS string_agg
FROM UNNEST(["apple", "pear", "banana", "pear"]) AS fruit;

/*------------------------------+
 | string_agg                   |
 +------------------------------+
 | pear & pear & apple & banana |
 +------------------------------*/
```

``` text
SELECT STRING_AGG(fruit, " & " LIMIT 2) AS string_agg
FROM UNNEST(["apple", "pear", "banana", "pear"]) AS fruit;

/*--------------+
 | string_agg   |
 +--------------+
 | apple & pear |
 +--------------*/
```

``` text
SELECT STRING_AGG(DISTINCT fruit, " & " ORDER BY fruit DESC LIMIT 2) AS string_agg
FROM UNNEST(["apple", "pear", "banana", "pear"]) AS fruit;

/*---------------+
 | string_agg    |
 +---------------+
 | pear & banana |
 +---------------*/
```

## `     SUM    `

``` text
SUM(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the sum of non- `  NULL  ` values in an aggregated group.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

Caveats:

  - If the aggregated group is empty or the argument is `  NULL  ` for all rows in the group, returns `  NULL  ` .
  - If the argument is `  NaN  ` for any row in the group, returns `  NaN  ` .
  - If the argument is `  [+|-]Infinity  ` for any row in the group, returns either `  [+|-]Infinity  ` or `  NaN  ` .
  - If there is numeric overflow, produces an error.
  - If a [floating-point type](/spanner/docs/reference/standard-sql/data-types#floating_point_types) is returned, the result is [non-deterministic](/spanner/docs/reference/standard-sql/data-types#floating_point_semantics) , which means you might receive a different result each time you use this function.

**Supported Argument Types**

  - Any supported numeric data type
  - `  INTERVAL  `

**Return Data Types**

<table>
<thead>
<tr class="header">
<th>INPUT</th>
<th><code dir="ltr" translate="no">       INT64      </code></th>
<th><code dir="ltr" translate="no">       NUMERIC      </code></th>
<th><code dir="ltr" translate="no">       FLOAT32      </code></th>
<th><code dir="ltr" translate="no">       FLOAT64      </code></th>
<th><code dir="ltr" translate="no">       INTERVAL      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>OUTPUT</td>
<td><code dir="ltr" translate="no">       INT64      </code></td>
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL      </code></td>
</tr>
</tbody>
</table>

**Examples**

``` text
SELECT SUM(x) AS sum
FROM UNNEST([1, 2, 3, 4, 5, 4, 3, 2, 1]) AS x;

/*-----+
 | sum |
 +-----+
 | 25  |
 +-----*/
```

``` text
SELECT SUM(DISTINCT x) AS sum
FROM UNNEST([1, 2, 3, 4, 5, 4, 3, 2, 1]) AS x;

/*-----+
 | sum |
 +-----+
 | 15  |
 +-----*/
```

``` text
SELECT SUM(x) AS sum
FROM UNNEST([]) AS x;

/*------+
 | sum  |
 +------+
 | NULL |
 +------*/
```
