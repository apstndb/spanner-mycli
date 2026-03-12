GoogleSQL for Spanner supports statistical aggregate functions. To learn about the syntax for aggregate function calls, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

## Function list

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Summary</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#stddev"><code dir="ltr" translate="no">        STDDEV       </code></a></td>
<td>An alias of the <code dir="ltr" translate="no">       STDDEV_SAMP      </code> function.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#stddev_samp"><code dir="ltr" translate="no">        STDDEV_SAMP       </code></a></td>
<td>Computes the sample (unbiased) standard deviation of the values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#var_samp"><code dir="ltr" translate="no">        VAR_SAMP       </code></a></td>
<td>Computes the sample (unbiased) variance of the values.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#variance"><code dir="ltr" translate="no">        VARIANCE       </code></a></td>
<td>An alias of <code dir="ltr" translate="no">       VAR_SAMP      </code> .</td>
</tr>
</tbody>
</table>

## `     STDDEV    `

``` text
STDDEV(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

An alias of [STDDEV\_SAMP](#stddev_samp) .

## `     STDDEV_SAMP    `

``` text
STDDEV_SAMP(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the sample (unbiased) standard deviation of the values. The return result is between `  0  ` and `  +Inf  ` .

All numeric types are supported. If the input is `  NUMERIC  ` then the internal aggregation is stable with the final output converted to a `  FLOAT64  ` . Otherwise the input is converted to a `  FLOAT64  ` before aggregation, resulting in a potentially unstable result.

This function ignores any `  NULL  ` inputs. If there are fewer than two non- `  NULL  ` inputs, this function returns `  NULL  ` .

`  NaN  ` is produced if:

  - Any input value is `  NaN  `
  - Any input value is positive infinity or negative infinity.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Return Data Type**

`  FLOAT64  `

**Examples**

``` text
SELECT STDDEV_SAMP(x) AS results FROM UNNEST([10, 14, 18]) AS x

/*---------+
 | results |
 +---------+
 | 4       |
 +---------*/
```

``` text
SELECT STDDEV_SAMP(x) AS results FROM UNNEST([10, 14, NULL]) AS x

/*--------------------+
 | results            |
 +--------------------+
 | 2.8284271247461903 |
 +--------------------*/
```

``` text
SELECT STDDEV_SAMP(x) AS results FROM UNNEST([10, NULL]) AS x

/*---------+
 | results |
 +---------+
 | NULL    |
 +---------*/
```

``` text
SELECT STDDEV_SAMP(x) AS results FROM UNNEST([NULL]) AS x

/*---------+
 | results |
 +---------+
 | NULL    |
 +---------*/
```

``` text
SELECT STDDEV_SAMP(x) AS results FROM UNNEST([10, 14, CAST('Infinity' as FLOAT64)]) AS x

/*---------+
 | results |
 +---------+
 | NaN     |
 +---------*/
```

## `     VAR_SAMP    `

``` text
VAR_SAMP(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

Returns the sample (unbiased) variance of the values. The return result is between `  0  ` and `  +Inf  ` .

All numeric types are supported. If the input is `  NUMERIC  ` then the internal aggregation is stable with the final output converted to a `  FLOAT64  ` . Otherwise the input is converted to a `  FLOAT64  ` before aggregation, resulting in a potentially unstable result.

This function ignores any `  NULL  ` inputs. If there are fewer than two non- `  NULL  ` inputs, this function returns `  NULL  ` .

`  NaN  ` is produced if:

  - Any input value is `  NaN  `
  - Any input value is positive infinity or negative infinity.

To learn more about the optional aggregate clauses that you can pass into this function, see [Aggregate function calls](/spanner/docs/reference/standard-sql/aggregate-function-calls) .

**Return Data Type**

`  FLOAT64  `

**Examples**

``` text
SELECT VAR_SAMP(x) AS results FROM UNNEST([10, 14, 18]) AS x

/*---------+
 | results |
 +---------+
 | 16      |
 +---------*/
```

``` text
SELECT VAR_SAMP(x) AS results FROM UNNEST([10, 14, NULL]) AS x

/*---------+
 | results |
 +---------+
 | 8       |
 +---------*/
```

``` text
SELECT VAR_SAMP(x) AS results FROM UNNEST([10, NULL]) AS x

/*---------+
 | results |
 +---------+
 | NULL    |
 +---------*/
```

``` text
SELECT VAR_SAMP(x) AS results FROM UNNEST([NULL]) AS x

/*---------+
 | results |
 +---------+
 | NULL    |
 +---------*/
```

``` text
SELECT VAR_SAMP(x) AS results FROM UNNEST([10, 14, CAST('Infinity' as FLOAT64)]) AS x

/*---------+
 | results |
 +---------+
 | NaN     |
 +---------*/
```

## `     VARIANCE    `

``` text
VARIANCE(
  [ DISTINCT ]
  expression
  [ HAVING { MAX | MIN } having_expression ]
)
```

**Description**

An alias of [VAR\_SAMP](#var_samp) .
