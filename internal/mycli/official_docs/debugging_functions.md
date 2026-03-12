GoogleSQL for Spanner supports the following debugging functions.

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
<td><a href="/spanner/docs/reference/standard-sql/debugging_functions#error"><code dir="ltr" translate="no">        ERROR       </code></a></td>
<td>Produces an error with a custom error message.</td>
</tr>
</tbody>
</table>

## `     ERROR    `

``` text
ERROR(error_message)
```

**Description**

Returns an error.

**Definitions**

  - `  error_message  ` : A `  STRING  ` value that represents the error message to produce. Any whitespace characters beyond a single space are trimmed from the results.

**Details**

`  ERROR  ` is treated like any other expression that may result in an error: there is no special guarantee of evaluation order.

**Return Data Type**

GoogleSQL infers the return type in context.

**Examples**

In the following example, the query produces an error message:

``` text
-- ERROR: Show this error message (while evaluating error("Show this error message"))
SELECT ERROR('Show this error message')
```

In the following example, the query returns an error message if the value of the row doesn't match one of two defined values.

``` text
SELECT
  CASE
    WHEN value = 'foo' THEN 'Value is foo.'
    WHEN value = 'bar' THEN 'Value is bar.'
    ELSE ERROR(CONCAT('Found unexpected value: ', value))
  END AS new_value
FROM (
  SELECT 'foo' AS value UNION ALL
  SELECT 'bar' AS value UNION ALL
  SELECT 'baz' AS value);

-- Found unexpected value: baz
```

The following example demonstrates bad usage of the `  ERROR  ` function. In this example, GoogleSQL might evaluate the `  ERROR  ` function before or after the `  x > 0  ` condition, because GoogleSQL doesn't guarantee ordering between `  WHERE  ` clause conditions. Therefore, the results with the `  ERROR  ` function might vary.

``` text
SELECT *
FROM (SELECT -1 AS x)
WHERE x > 0 AND ERROR('Example error');
```

In the next example, the `  WHERE  ` clause evaluates an `  IF  ` condition, which ensures that GoogleSQL only evaluates the `  ERROR  ` function if the condition fails.

``` text
SELECT *
FROM (SELECT -1 AS x)
WHERE IF(x > 0, true, ERROR(FORMAT('Error: x must be positive but is %t', x)));

-- Error: x must be positive but is -1
```
