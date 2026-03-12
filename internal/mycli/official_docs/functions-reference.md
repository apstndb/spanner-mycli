When you call a function, specific rules may apply. You can also add the `  SAFE.  ` prefix, which prevents functions from generating some types of errors. To learn more, see the next sections.

## Function call rules

The following rules apply to all built-in GoogleSQL functions unless explicitly indicated otherwise in the function description:

  - If an operand is `  NULL  ` , the function result is `  NULL  ` .
  - For functions that are time zone sensitive, the default time zone, America/Los\_Angeles, is used when a time zone isn't specified.

## Lambdas

**Syntax:**

``` text
(arg[, ...]) -> body_expression
```

``` text
arg -> body_expression
```

**Description**

For some functions, GoogleSQL supports lambdas as builtin function arguments. A lambda takes a list of arguments and an expression as the lambda body.

  - `  arg  ` :
      - Name of the lambda argument is defined by the user.
      - No type is specified for the lambda argument. The type is inferred from the context.
  - `  body_expression  ` :
      - The lambda body can be any valid scalar expression.

## SAFE. prefix

**Syntax:**

``` text
SAFE.function_name()
```

**Description**

If you begin a scalar function with the `  SAFE.  ` prefix, it will return `  NULL  ` instead of an error. The `  SAFE.  ` prefix only prevents errors from the prefixed function itself: it doesn't prevent errors that occur while evaluating argument expressions. The `  SAFE.  ` prefix only prevents errors that occur because of the value of the function inputs, such as "value out of range" errors; other errors, such as internal or system errors, may still occur. If the function doesn't return an error, `  SAFE.  ` has no effect on the output.

**Exclusions**

  - [Operators](/spanner/docs/reference/standard-sql/operators) , such as `  +  ` and `  =  ` , don't support the `  SAFE.  ` prefix. To prevent errors from a division operation, use [SAFE\_DIVIDE](/spanner/docs/reference/standard-sql/mathematical_functions#safe_divide) .
  - Some operators, such as `  IN  ` , `  ARRAY  ` , and `  UNNEST  ` , resemble functions but don't support the `  SAFE.  ` prefix.
  - The `  CAST  ` and `  EXTRACT  ` functions don't support the `  SAFE.  ` prefix. To prevent errors from casting, use [SAFE\_CAST](/spanner/docs/reference/standard-sql/conversion_functions#safe_casting) .
  - You can't append the `  SAFE.  ` prefix to a function that contains a [lambda](#lambdas) .

**Example**

In the following example, the first use of the `  SUBSTR  ` function would normally return an error, because the function doesn't support length arguments with negative values. However, the `  SAFE.  ` prefix causes the function to return `  NULL  ` instead. The second use of the `  SUBSTR  ` function provides the expected output: the `  SAFE.  ` prefix has no effect.

``` text
SELECT SAFE.SUBSTR('foo', 0, -2) AS safe_output UNION ALL
SELECT SAFE.SUBSTR('bar', 0, 2) AS safe_output;

/*-------------+
 | safe_output |
 +-------------+
 | NULL        |
 | ba          |
 +-------------*/
```

## Function hints

The following hints are available for GoogleSQL functions:

### DISABLE\_INLINE

``` text
function_name() @{DISABLE_INLINE = TRUE}
```

To disable other parts of a query from using the function as an inline expression, add the `  @{DISABLE_INLINE = TRUE}  ` hint after a scalar function. This allows the function to be computed once instead of each time another part of a query references it.

`  DISABLE_INLINE  ` works with top-level functions.

You can't use `  DISABLE_INLINE  ` with a few functions, including those that don't produce a scalar value and `  CAST  ` . Although you can't use `  DISABLE_INLINE  ` with the `  CAST  ` function, you can use it with the first expression inside this function.

**Examples**

In the following example, inline expressions are enabled by default for `  x  ` . `  x  ` is computed twice, once by each reference:

``` text
SELECT
  SUBSTRING(CAST(x AS STRING), 2, 5) AS w,
  SUBSTRING(CAST(x AS STRING), 3, 7) AS y
FROM (SELECT SHA512(z) AS x FROM t)
```

In the following example, inline expressions are disabled for `  x  ` . `  x  ` is computed once, and the result is used by each reference:

``` text
SELECT
  SUBSTRING(CAST(x AS STRING), 2, 5) AS w,
  SUBSTRING(CAST(x AS STRING), 3, 7) AS y
FROM (SELECT SHA512(z) @{DISABLE_INLINE = TRUE} AS x FROM t)
```
