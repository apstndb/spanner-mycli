GoogleSQL for Spanner supports the following functions, which can retrieve and transform JSON data.

## Categories

The JSON functions are grouped into the following categories based on their behavior:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Category</th>
<th>Functions</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Extractors</td>
<td><a href="#json_query"><code dir="ltr" translate="no">        JSON_QUERY       </code></a><br />
<a href="#json_value"><code dir="ltr" translate="no">        JSON_VALUE       </code></a><br />
<a href="#json_query_array"><code dir="ltr" translate="no">        JSON_QUERY_ARRAY       </code></a><br />
<a href="#json_value_array"><code dir="ltr" translate="no">        JSON_VALUE_ARRAY       </code></a><br />
</td>
<td>Functions that extract JSON data.</td>
</tr>
<tr class="even">
<td>Lax converters</td>
<td><a href="#lax_bool"><code dir="ltr" translate="no">        LAX_BOOL       </code></a><br />
<a href="#lax_double"><code dir="ltr" translate="no">        LAX_FLOAT64       </code></a><br />
<a href="#lax_int64"><code dir="ltr" translate="no">        LAX_INT64       </code></a><br />
<a href="#lax_string"><code dir="ltr" translate="no">        LAX_STRING       </code></a><br />
</td>
<td>Functions that flexibly convert a JSON value to a SQL value without returning errors.</td>
</tr>
<tr class="odd">
<td>Converters</td>
<td><a href="#bool_for_json"><code dir="ltr" translate="no">        BOOL       </code></a><br />
<a href="#bool_array_for_json"><code dir="ltr" translate="no">        BOOL_ARRAY       </code></a><br />
<a href="#double_for_json"><code dir="ltr" translate="no">        FLOAT64       </code></a><br />
<a href="#double_array_for_json"><code dir="ltr" translate="no">        FLOAT64_ARRAY       </code></a><br />
<a href="#float_for_json"><code dir="ltr" translate="no">        FLOAT32       </code></a><br />
<a href="#float_array_for_json"><code dir="ltr" translate="no">        FLOAT32_ARRAY       </code></a><br />
<a href="#int64_for_json"><code dir="ltr" translate="no">        INT64       </code></a><br />
<a href="#int64_array_for_json"><code dir="ltr" translate="no">        INT64_ARRAY       </code></a><br />
<a href="#string_for_json"><code dir="ltr" translate="no">        STRING       </code></a><br />
<a href="#string_array_for_json"><code dir="ltr" translate="no">        STRING_ARRAY       </code></a><br />
</td>
<td>Functions that convert a JSON value to a SQL value.</td>
</tr>
<tr class="even">
<td>Other converters</td>
<td><a href="#parse_json"><code dir="ltr" translate="no">        PARSE_JSON       </code></a><br />
<a href="#to_json"><code dir="ltr" translate="no">        TO_JSON       </code></a><br />
<a href="#safe_to_json"><code dir="ltr" translate="no">        SAFE_TO_JSON       </code></a><br />
<a href="#to_json_string"><code dir="ltr" translate="no">        TO_JSON_STRING       </code></a><br />
</td>
<td>Other conversion functions from or to JSON.</td>
</tr>
<tr class="odd">
<td>Constructors</td>
<td><a href="#json_array"><code dir="ltr" translate="no">        JSON_ARRAY       </code></a><br />
<a href="#json_object"><code dir="ltr" translate="no">        JSON_OBJECT       </code></a><br />
</td>
<td>Functions that create JSON.</td>
</tr>
<tr class="even">
<td>Mutators</td>
<td><a href="#json_array_append"><code dir="ltr" translate="no">        JSON_ARRAY_APPEND       </code></a><br />
<a href="#json_array_insert"><code dir="ltr" translate="no">        JSON_ARRAY_INSERT       </code></a><br />
<a href="#json_remove"><code dir="ltr" translate="no">        JSON_REMOVE       </code></a><br />
<a href="#json_set"><code dir="ltr" translate="no">        JSON_SET       </code></a><br />
<a href="#json_strip_nulls"><code dir="ltr" translate="no">        JSON_STRIP_NULLS       </code></a><br />
</td>
<td>Functions that mutate existing JSON.</td>
</tr>
<tr class="odd">
<td>Accessors</td>
<td><a href="#json_keys"><code dir="ltr" translate="no">        JSON_KEYS       </code></a><br />
<a href="#json_type"><code dir="ltr" translate="no">        JSON_TYPE       </code></a><br />
</td>
<td>Functions that provide access to JSON properties.</td>
</tr>
<tr class="even">
<td>Predicates</td>
<td><a href="#json_contains"><code dir="ltr" translate="no">        JSON_CONTAINS       </code></a><br />
</td>
<td>Functions that return <code dir="ltr" translate="no">       BOOL      </code> when checking JSON documents for certain properties.</td>
</tr>
</tbody>
</table>

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
<td><a href="/spanner/docs/reference/standard-sql/json_functions#bool_for_json"><code dir="ltr" translate="no">        BOOL       </code></a></td>
<td>Converts a JSON boolean to a SQL <code dir="ltr" translate="no">       BOOL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#bool_array_for_json"><code dir="ltr" translate="no">        BOOL_ARRAY       </code></a></td>
<td>Converts a JSON array of booleans to a SQL <code dir="ltr" translate="no">       ARRAY&lt;BOOL&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#double_for_json"><code dir="ltr" translate="no">        FLOAT64       </code></a></td>
<td>Converts a JSON number to a SQL <code dir="ltr" translate="no">       FLOAT64      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#double_array_for_json"><code dir="ltr" translate="no">        FLOAT64_ARRAY       </code></a></td>
<td>Converts a JSON array of numbers to a SQL <code dir="ltr" translate="no">       ARRAY&lt;FLOAT64&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#float_for_json"><code dir="ltr" translate="no">        FLOAT32       </code></a></td>
<td>Converts a JSON number to a SQL <code dir="ltr" translate="no">       FLOAT32      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#float_array_for_json"><code dir="ltr" translate="no">        FLOAT32_ARRAY       </code></a></td>
<td>Converts a JSON array of numbers to a SQL <code dir="ltr" translate="no">       ARRAY&lt;FLOAT32&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#int64_for_json"><code dir="ltr" translate="no">        INT64       </code></a></td>
<td>Converts a JSON number to a SQL <code dir="ltr" translate="no">       INT64      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#int64_array_for_json"><code dir="ltr" translate="no">        INT64_ARRAY       </code></a></td>
<td>Converts a JSON array of numbers to a SQL <code dir="ltr" translate="no">       ARRAY&lt;INT64&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array"><code dir="ltr" translate="no">        JSON_ARRAY       </code></a></td>
<td>Creates a JSON array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array_append"><code dir="ltr" translate="no">        JSON_ARRAY_APPEND       </code></a></td>
<td>Appends JSON data to the end of a JSON array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array_insert"><code dir="ltr" translate="no">        JSON_ARRAY_INSERT       </code></a></td>
<td>Inserts JSON data into a JSON array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_contains"><code dir="ltr" translate="no">        JSON_CONTAINS       </code></a></td>
<td>Checks if a JSON document contains another JSON document.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_keys"><code dir="ltr" translate="no">        JSON_KEYS       </code></a></td>
<td>Extracts unique JSON keys from a JSON expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_object"><code dir="ltr" translate="no">        JSON_OBJECT       </code></a></td>
<td>Creates a JSON object.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_query"><code dir="ltr" translate="no">        JSON_QUERY       </code></a></td>
<td>Extracts a JSON value and converts it to a SQL JSON-formatted <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       JSON      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_query_array"><code dir="ltr" translate="no">        JSON_QUERY_ARRAY       </code></a></td>
<td>Extracts a JSON array and converts it to a SQL <code dir="ltr" translate="no">       ARRAY&lt;JSON-formatted STRING&gt;      </code> or <code dir="ltr" translate="no">       ARRAY&lt;JSON&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_remove"><code dir="ltr" translate="no">        JSON_REMOVE       </code></a></td>
<td>Produces JSON with the specified JSON data removed.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_set"><code dir="ltr" translate="no">        JSON_SET       </code></a></td>
<td>Inserts or replaces JSON data.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_strip_nulls"><code dir="ltr" translate="no">        JSON_STRIP_NULLS       </code></a></td>
<td>Removes JSON nulls from JSON objects and JSON arrays.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_type"><code dir="ltr" translate="no">        JSON_TYPE       </code></a></td>
<td>Gets the JSON type of the outermost JSON value and converts the name of this type to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_value"><code dir="ltr" translate="no">        JSON_VALUE       </code></a></td>
<td>Extracts a JSON scalar value and converts it to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_value_array"><code dir="ltr" translate="no">        JSON_VALUE_ARRAY       </code></a></td>
<td>Extracts a JSON array of scalar values and converts it to a SQL <code dir="ltr" translate="no">       ARRAY&lt;STRING&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_bool"><code dir="ltr" translate="no">        LAX_BOOL       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       BOOL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_double"><code dir="ltr" translate="no">        LAX_FLOAT64       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       FLOAT64      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_int64"><code dir="ltr" translate="no">        LAX_INT64       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       INT64      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_string"><code dir="ltr" translate="no">        LAX_STRING       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#parse_json"><code dir="ltr" translate="no">        PARSE_JSON       </code></a></td>
<td>Converts a JSON-formatted <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       JSON      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#safe_to_json"><code dir="ltr" translate="no">        SAFE_TO_JSON       </code></a></td>
<td>Similar to the `TO_JSON` function, but for each unsupported field in the input argument, produces a JSON null instead of an error.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#string_for_json"><code dir="ltr" translate="no">        STRING       </code> (JSON)</a></td>
<td>Converts a JSON string to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#string_array_for_json"><code dir="ltr" translate="no">        STRING_ARRAY       </code></a></td>
<td>Converts a JSON array of strings to a SQL <code dir="ltr" translate="no">       ARRAY&lt;STRING&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#to_json"><code dir="ltr" translate="no">        TO_JSON       </code></a></td>
<td>Converts a SQL value to a JSON value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#to_json_string"><code dir="ltr" translate="no">        TO_JSON_STRING       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       JSON      </code> value to a SQL JSON-formatted <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
</tbody>
</table>

## `     BOOL    `

``` text
BOOL(json_expr)
```

**Description**

Converts a JSON boolean to a SQL `  BOOL  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON 'true'
    ```
    
    If the JSON value isn't a boolean, an error is produced. If the expression is SQL `  NULL  ` , the function returns SQL `  NULL  ` .

**Return type**

`  BOOL  `

**Examples**

``` text
SELECT BOOL(JSON 'true') AS vacancy;

/*---------+
 | vacancy |
 +---------+
 | true    |
 +---------*/
```

``` text
SELECT BOOL(JSON_QUERY(JSON '{"hotel class": "5-star", "vacancy": true}', "$.vacancy")) AS vacancy;

/*---------+
 | vacancy |
 +---------+
 | true    |
 +---------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if JSON isn't of type bool.
SELECT BOOL(JSON '123') AS result; -- Throws an error
SELECT BOOL(JSON 'null') AS result; -- Throws an error
SELECT SAFE.BOOL(JSON '123') AS result; -- Returns a SQL NULL
```

## `     BOOL_ARRAY    `

``` text
BOOL_ARRAY(json_expr)
```

**Description**

Converts a JSON array of booleans to a SQL `  ARRAY<BOOL>  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '[true]'
    ```
    
    If the JSON value isn't an array of booleans, an error is produced. If the expression is SQL `  NULL  ` , the function returns SQL `  NULL  ` .

**Return type**

`  ARRAY<BOOL>  `

**Examples**

``` text
SELECT BOOL_ARRAY(JSON '[true, false]') AS vacancies;

/*---------------+
 | vacancies     |
 +---------------+
 | [true, false] |
 +---------------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if the JSON isn't an array of booleans.
SELECT BOOL_ARRAY(JSON '[123]') AS result; -- Throws an error
SELECT BOOL_ARRAY(JSON '[null]') AS result; -- Throws an error
SELECT BOOL_ARRAY(JSON 'null') AS result; -- Throws an error
```

## `     FLOAT64    `

``` text
FLOAT64(
  json_expr
  [, wide_number_mode => { 'exact' | 'round' } ]
)
```

**Description**

Converts a JSON number to a SQL `  FLOAT64  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '9.8'
    ```
    
    If the JSON value isn't a number, an error is produced. If the expression is a SQL `  NULL  ` , the function returns SQL `  NULL  ` .

  - `  wide_number_mode  ` : A named argument with a `  STRING  ` value. Defines what happens with a number that can't be represented as a `  FLOAT64  ` without loss of precision. This argument accepts one of the two case-sensitive values:
    
      - `  exact  ` : The function fails if the result can't be represented as a `  FLOAT64  ` without loss of precision.
      - `  round  ` (default): The numeric value stored in JSON will be rounded to `  FLOAT64  ` . If such rounding isn't possible, the function fails.

**Return type**

`  FLOAT64  `

**Examples**

``` text
SELECT FLOAT64(JSON '9.8') AS velocity;

/*----------+
 | velocity |
 +----------+
 | 9.8      |
 +----------*/
```

``` text
SELECT FLOAT64(JSON_QUERY(JSON '{"vo2_max": 39.1, "age": 18}', "$.vo2_max")) AS vo2_max;

/*---------+
 | vo2_max |
 +---------+
 | 39.1    |
 +---------*/
```

``` text
SELECT FLOAT64(JSON '18446744073709551615', wide_number_mode=>'round') as result;

/*------------------------+
 | result                 |
 +------------------------+
 | 1.8446744073709552e+19 |
 +------------------------*/
```

``` text
SELECT FLOAT64(JSON '18446744073709551615') as result;

/*------------------------+
 | result                 |
 +------------------------+
 | 1.8446744073709552e+19 |
 +------------------------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if JSON isn't of type FLOAT64.
SELECT FLOAT64(JSON '"strawberry"') AS result;
SELECT FLOAT64(JSON 'null') AS result;

-- An error is thrown because `wide_number_mode` is case-sensitive and not "exact" or "round".
SELECT FLOAT64(JSON '123.4', wide_number_mode=>'EXACT') as result;
SELECT FLOAT64(JSON '123.4', wide_number_mode=>'exac') as result;

-- An error is thrown because the number can't be converted to DOUBLE without loss of precision
SELECT FLOAT64(JSON '18446744073709551615', wide_number_mode=>'exact') as result;

-- Returns a SQL NULL
SELECT SAFE.FLOAT64(JSON '"strawberry"') AS result;
```

## `     FLOAT64_ARRAY    `

``` text
FLOAT64_ARRAY(
  json_expr
  [, wide_number_mode => { 'exact' | 'round' } ]
)
```

**Description**

Converts a JSON array of numbers to a SQL `  ARRAY<FLOAT64>  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '[9.8]'
    ```
    
    If the JSON value isn't an array of numbers, an error is produced. If the expression is a SQL `  NULL  ` , the function returns SQL `  NULL  ` .

  - `  wide_number_mode  ` : A named argument that takes a `  STRING  ` value. Defines what happens with a number that can't be represented as a `  FLOAT64  ` without loss of precision. This argument accepts one of the two case-sensitive values:
    
      - `  exact  ` : The function fails if the result can't be represented as a `  FLOAT64  ` without loss of precision.
      - `  round  ` (default): The numeric value stored in JSON will be rounded to `  FLOAT64  ` . If such rounding isn't possible, the function fails.

**Return type**

`  ARRAY<FLOAT64>  `

**Examples**

``` text
SELECT FLOAT64_ARRAY(JSON '[9, 9.8]') AS velocities;

/*-------------+
 | velocities  |
 +-------------+
 | [9.0, 9.8]  |
 +-------------*/
```

``` text
SELECT FLOAT64_ARRAY(JSON '[18446744073709551615]', wide_number_mode=>'round') as result;

/*--------------------------+
 | result                   |
 +--------------------------+
 | [1.8446744073709552e+19] |
 +--------------------------*/
```

``` text
SELECT FLOAT64_ARRAY(JSON '[18446744073709551615]') as result;

/*--------------------------+
 | result                   |
 +--------------------------+
 | [1.8446744073709552e+19] |
 +--------------------------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if the JSON isn't an array of numbers.
SELECT FLOAT64_ARRAY(JSON '["strawberry"]') AS result;
SELECT FLOAT64_ARRAY(JSON '[null]') AS result;
SELECT FLOAT64_ARRAY(JSON 'null') AS result;

-- An error is thrown because `wide_number_mode` is case-sensitive and not "exact" or "round".
SELECT FLOAT64_ARRAY(JSON '[123.4]', wide_number_mode=>'EXACT') as result;
SELECT FLOAT64_ARRAY(JSON '[123.4]', wide_number_mode=>'exac') as result;

-- An error is thrown because the number can't be converted to DOUBLE without loss of precision
SELECT FLOAT64_ARRAY(JSON '[18446744073709551615]', wide_number_mode=>'exact') as result;
```

## `     FLOAT32    `

``` text
FLOAT32(
  json_expr
  [, [ wide_number_mode => ] { 'exact' | 'round' } ]
)
```

**Description**

Converts a JSON number to a SQL `  FLOAT32  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '9.8'
    ```
    
    If the JSON value isn't a number, an error is produced. If the expression is a SQL `  NULL  ` , the function returns SQL `  NULL  ` .

  - `  wide_number_mode  ` : A named argument with a `  STRING  ` value. Defines what happens with a number that can't be represented as a `  FLOAT32  ` without loss of precision. This argument accepts one of the two case-sensitive values:
    
      - `  exact  ` : The function fails if the result can't be represented as a `  FLOAT32  ` without loss of precision.
      - `  round  ` (default): The numeric value stored in JSON will be rounded to `  FLOAT32  ` . If such rounding isn't possible, the function fails.

**Return type**

`  FLOAT32  `

**Examples**

``` text
SELECT FLOAT32(JSON '9.8') AS velocity;

/*----------+
 | velocity |
 +----------+
 | 9.8      |
 +----------*/
```

``` text
SELECT FLOAT32(JSON_QUERY(JSON '{"vo2_max": 39.1, "age": 18}', "$.vo2_max")) AS vo2_max;

/*---------+
 | vo2_max |
 +---------+
 | 39.1    |
 +---------*/
```

``` text
SELECT FLOAT32(JSON '16777217', wide_number_mode=>'round') as result;

/*------------+
 | result     |
 +------------+
 | 16777216.0 |
 +------------*/
```

``` text
SELECT FLOAT32(JSON '16777216') as result;

/*------------+
 | result     |
 +------------+
 | 16777216.0 |
 +------------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if JSON isn't of type FLOAT32.
SELECT FLOAT32(JSON '"strawberry"') AS result;
SELECT FLOAT32(JSON 'null') AS result;

-- An error is thrown because `wide_number_mode` is case-sensitive and not "exact" or "round".
SELECT FLOAT32(JSON '123.4', wide_number_mode=>'EXACT') as result;
SELECT FLOAT32(JSON '123.4', wide_number_mode=>'exac') as result;

-- An error is thrown because the number can't be converted to FLOAT without loss of precision
SELECT FLOAT32(JSON '16777217', wide_number_mode=>'exact') as result;

-- Returns a SQL NULL
SELECT SAFE.FLOAT32(JSON '"strawberry"') AS result;
```

## `     FLOAT32_ARRAY    `

``` text
FLOAT32_ARRAY(
  json_expr
  [, wide_number_mode => { 'exact' | 'round' } ]
)
```

**Description**

Converts a JSON array of numbers to a SQL `  ARRAY<FLOAT32>  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '[9.8]'
    ```
    
    If the JSON value isn't an array of numbers, an error is produced. If the expression is a SQL `  NULL  ` , the function returns SQL `  NULL  ` .

  - `  wide_number_mode  ` : A named argument with a `  STRING  ` value. Defines what happens with a number that can't be represented as a `  FLOAT32  ` without loss of precision. This argument accepts one of the two case-sensitive values:
    
      - `  exact  ` : The function fails if the result can't be represented as a `  FLOAT32  ` without loss of precision.
      - `  round  ` (default): The numeric value stored in JSON will be rounded to `  FLOAT32  ` . If such rounding isn't possible, the function fails.

**Return type**

`  ARRAY<FLOAT32>  `

**Examples**

``` text
SELECT FLOAT32_ARRAY(JSON '[9, 9.8]') AS velocities;

/*-------------+
 | velocities  |
 +-------------+
 | [9.0, 9.8]  |
 +-------------*/
```

``` text
SELECT FLOAT32_ARRAY(JSON '[16777217]', wide_number_mode=>'round') as result;

/*--------------+
 | result       |
 +--------------+
 | [16777216.0] |
 +--------------*/
```

``` text
SELECT FLOAT32_ARRAY(JSON '[16777216]') as result;

/*--------------+
 | result       |
 +--------------+
 | [16777216.0] |
 +--------------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if the JSON isn't an array of numbers in FLOAT32 domain.
SELECT FLOAT32_ARRAY(JSON '["strawberry"]') AS result;
SELECT FLOAT32_ARRAY(JSON '[null]') AS result;
SELECT FLOAT32_ARRAY(JSON 'null') AS result;

-- An error is thrown because `wide_number_mode` is case-sensitive and not "exact" or "round".
SELECT FLOAT32_ARRAY(JSON '[123.4]', wide_number_mode=>'EXACT') as result;
SELECT FLOAT32_ARRAY(JSON '[123.4]', wide_number_mode=>'exac') as result;

-- An error is thrown because the number can't be converted to FLOAT without loss of precision
SELECT FLOAT32_ARRAY(JSON '[16777217]', wide_number_mode=>'exact') as result;
```

## `     INT64    `

``` text
INT64(json_expr)
```

**Description**

Converts a JSON number to a SQL `  INT64  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '999'
    ```
    
    If the JSON value isn't a number, or the JSON number isn't in the SQL `  INT64  ` domain, an error is produced. If the expression is SQL `  NULL  ` , the function returns SQL `  NULL  ` .

**Return type**

`  INT64  `

**Examples**

``` text
SELECT INT64(JSON '2005') AS flight_number;

/*---------------+
 | flight_number |
 +---------------+
 | 2005          |
 +---------------*/
```

``` text
SELECT INT64(JSON_QUERY(JSON '{"gate": "A4", "flight_number": 2005}', "$.flight_number")) AS flight_number;

/*---------------+
 | flight_number |
 +---------------+
 | 2005          |
 +---------------*/
```

``` text
SELECT INT64(JSON '10.0') AS score;

/*-------+
 | score |
 +-------+
 | 10    |
 +-------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if JSON isn't a number or can't be converted to a 64-bit integer.
SELECT INT64(JSON '10.1') AS result;  -- Throws an error
SELECT INT64(JSON '"strawberry"') AS result; -- Throws an error
SELECT INT64(JSON 'null') AS result; -- Throws an error
SELECT SAFE.INT64(JSON '"strawberry"') AS result;  -- Returns a SQL NULL
```

## `     INT64_ARRAY    `

``` text
INT64_ARRAY(json_expr)
```

**Description**

Converts a JSON array of numbers to a SQL `  INT64_ARRAY  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '[999]'
    ```
    
    If the JSON value isn't an array of numbers, or the JSON numbers aren't in the SQL `  INT64  ` domain, an error is produced. If the expression is SQL `  NULL  ` , the function returns SQL `  NULL  ` .

**Return type**

`  ARRAY<INT64>  `

**Examples**

``` text
SELECT INT64_ARRAY(JSON '[2005, 2003]') AS flight_numbers;

/*----------------+
 | flight_numbers |
 +----------------+
 | [2005, 2003]   |
 +----------------*/
```

``` text
SELECT INT64_ARRAY(JSON '[10.0]') AS scores;

/*--------+
 | scores |
 +--------+
 | [10]   |
 +--------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if the JSON isn't an array of numbers in INT64 domain.
SELECT INT64_ARRAY(JSON '[10.1]') AS result;  -- Throws an error
SELECT INT64_ARRAY(JSON '["strawberry"]') AS result; -- Throws an error
SELECT INT64_ARRAY(JSON '[null]') AS result; -- Throws an error
SELECT INT64_ARRAY(JSON 'null') AS result; -- Throws an error
```

## `     JSON_ARRAY    `

``` text
JSON_ARRAY([value][, ...])
```

**Description**

Creates a JSON array from zero or more SQL values.

Arguments:

  - `  value  ` : A [JSON encoding-supported](#json_encodings) value to add to a JSON array.

**Return type**

`  JSON  `

**Examples**

The following query creates a JSON array with one value in it:

``` text
SELECT JSON_ARRAY(10) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | [10]      |
 +-----------*/
```

You can create a JSON array with an empty JSON array in it. For example:

``` text
SELECT JSON_ARRAY([]) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | [[]]      |
 +-----------*/
```

``` text
SELECT JSON_ARRAY(10, 'foo', NULL) AS json_data

/*-----------------+
 | json_data       |
 +-----------------+
 | [10,"foo",null] |
 +-----------------*/
```

``` text
SELECT JSON_ARRAY(STRUCT(10 AS a, 'foo' AS b)) AS json_data

/*----------------------+
 | json_data            |
 +----------------------+
 | [{"a":10,"b":"foo"}] |
 +----------------------*/
```

``` text
SELECT JSON_ARRAY(10, ['foo', 'bar'], [20, 30]) AS json_data

/*----------------------------+
 | json_data                  |
 +----------------------------+
 | [10,["foo","bar"],[20,30]] |
 +----------------------------*/
```

``` text
SELECT JSON_ARRAY(10, [JSON '20', JSON '"foo"']) AS json_data

/*-----------------+
 | json_data       |
 +-----------------+
 | [10,[20,"foo"]] |
 +-----------------*/
```

You can create an empty JSON array. For example:

``` text
SELECT JSON_ARRAY() AS json_data

/*-----------+
 | json_data |
 +-----------+
 | []        |
 +-----------*/
```

## `     JSON_ARRAY_APPEND    `

``` text
JSON_ARRAY_APPEND(
  json_expr,
  json_path_value_pair[, ...]
  [, append_each_element => { TRUE | FALSE } ]
)

json_path_value_pair:
  json_path, value
```

Appends JSON data to the end of a JSON array.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '["a", "b", "c"]'
    ```

  - `  json_path_value_pair  ` : A value and the [JSONPath](#JSONPath_format) for that value. This includes:
    
      - `  json_path  ` : Append `  value  ` at this [JSONPath](#JSONPath_format) in `  json_expr  ` .
    
      - `  value  ` : A [JSON encoding-supported](#json_encodings) value to append.

  - `  append_each_element  ` : A named argument with a `  BOOL  ` value.
    
      - If `  TRUE  ` (default), and `  value  ` is a SQL array, appends each element individually.
    
      - If `  FALSE,  ` and `  value  ` is a SQL array, appends the array as one element.

Details:

  - Path value pairs are evaluated left to right. The JSON produced by evaluating one pair becomes the JSON against which the next pair is evaluated.
  - The operation is ignored if the path points to a JSON non-array value that isn't a JSON null.
  - If `  json_path  ` points to a JSON null, the JSON null is replaced by a JSON array that contains `  value  ` .
  - If the path exists but has an incompatible type at any given path token, the path value pair operation is ignored.
  - The function applies all path value pair append operations even if an individual path value pair operation is invalid. For invalid operations, the operation is ignored and the function continues to process the rest of the path value pairs.
  - If any `  json_path  ` is an invalid [JSONPath](#JSONPath_format) , an error is produced.
  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - If `  append_each_element  ` is SQL `  NULL  ` , the function returns `  json_expr  ` .
  - If `  json_path  ` is SQL `  NULL  ` , the `  json_path_value_pair  ` operation is ignored.

**Return type**

`  JSON  `

**Examples**

In the following example, path `  $  ` is matched and appends `  1  ` .

``` text
SELECT JSON_ARRAY_APPEND(JSON '["a", "b", "c"]', '$', 1) AS json_data

/*-----------------+
 | json_data       |
 +-----------------+
 | ["a","b","c",1] |
 +-----------------*/
```

In the following example, `  append_each_element  ` defaults to `  TRUE  ` , so `  [1, 2]  ` is appended as individual elements.

``` text
SELECT JSON_ARRAY_APPEND(JSON '["a", "b", "c"]', '$', [1, 2]) AS json_data

/*-------------------+
 | json_data         |
 +-------------------+
 | ["a","b","c",1,2] |
 +-------------------*/
```

In the following example, `  append_each_element  ` is `  FALSE  ` , so `  [1, 2]  ` is appended as one element.

``` text
SELECT JSON_ARRAY_APPEND(
  JSON '["a", "b", "c"]',
  '$', [1, 2],
  append_each_element=>FALSE) AS json_data

/*---------------------+
 | json_data           |
 +---------------------+
 | ["a","b","c",[1,2]] |
 +---------------------*/
```

In the following example, `  append_each_element  ` is `  FALSE  ` , so `  [1, 2]  ` and `  [3, 4]  ` are each appended as one element.

``` text
SELECT JSON_ARRAY_APPEND(
  JSON '["a", ["b"], "c"]',
  '$[1]', [1, 2],
  '$[1][1]', [3, 4],
  append_each_element=>FALSE) AS json_data

/*-----------------------------+
 | json_data                   |
 +-----------------------------+
 | ["a",["b",[1,2,[3,4]]],"c"] |
 +-----------------------------*/
```

In the following example, the first path `  $[1]  ` appends `  [1, 2]  ` as single elements, and then the second path `  $[1][1]  ` isn't a valid path to an array, so the second operation is ignored.

``` text
SELECT JSON_ARRAY_APPEND(
  JSON '["a", ["b"], "c"]',
  '$[1]', [1, 2],
  '$[1][1]', [3, 4]) AS json_data

/*---------------------+
 | json_data           |
 +---------------------+
 | ["a",["b",1,2],"c"] |
 +---------------------*/
```

In the following example, path `  $.a  ` is matched and appends `  2  ` .

``` text
SELECT JSON_ARRAY_APPEND(JSON '{"a": [1]}', '$.a', 2) AS json_data

/*-------------+
 | json_data   |
 +-------------+
 | {"a":[1,2]} |
 +-------------*/
```

In the following example, a value is appended into a JSON null.

``` text
SELECT JSON_ARRAY_APPEND(JSON '{"a": null}', '$.a', 10)

/*------------+
 | json_data  |
 +------------+
 | {"a":[10]} |
 +------------*/
```

In the following example, path `  $.a  ` isn't an array, so the operation is ignored.

``` text
SELECT JSON_ARRAY_APPEND(JSON '{"a": 1}', '$.a', 2) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"a":1}   |
 +-----------*/
```

In the following example, path `  $.b  ` doesn't exist, so the operation is ignored.

``` text
SELECT JSON_ARRAY_APPEND(JSON '{"a": 1}', '$.b', 2) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"a":1}   |
 +-----------*/
```

## `     JSON_ARRAY_INSERT    `

``` text
JSON_ARRAY_INSERT(
  json_expr,
  json_path_value_pair[, ...]
  [, insert_each_element => { TRUE | FALSE } ]
)

json_path_value_pair:
  json_path, value
```

Produces a new JSON value that's created by inserting JSON data into a JSON array.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '["a", "b", "c"]'
    ```

  - `  json_path_value_pair  ` : A value and the [JSONPath](#JSONPath_format) for that value. This includes:
    
      - `  json_path  ` : Insert `  value  ` at this [JSONPath](#JSONPath_format) in `  json_expr  ` .
    
      - `  value  ` : A [JSON encoding-supported](#json_encodings) value to insert.

  - `  insert_each_element  ` : A named argument with a `  BOOL  ` value.
    
      - If `  TRUE  ` (default), and `  value  ` is a SQL array, inserts each element individually.
    
      - If `  FALSE,  ` and `  value  ` is a SQL array, inserts the array as one element.

Details:

  - Path value pairs are evaluated left to right. The JSON produced by evaluating one pair becomes the JSON against which the next pair is evaluated.
  - The operation is ignored if the path points to a JSON non-array value that isn't a JSON null.
  - If `  json_path  ` points to a JSON null, the JSON null is replaced by a JSON array of the appropriate size and padded on the left with JSON nulls.
  - If the path exists but has an incompatible type at any given path token, the path value pair operator is ignored.
  - The function applies all path value pair append operations even if an individual path value pair operation is invalid. For invalid operations, the operation is ignored and the function continues to process the rest of the path value pairs.
  - If the array index in `  json_path  ` is larger than the size of the array, the function extends the length of the array to the index, fills in the array with JSON nulls, then adds `  value  ` at the index.
  - If any `  json_path  ` is an invalid [JSONPath](#JSONPath_format) , an error is produced.
  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - If `  insert_each_element  ` is SQL `  NULL  ` , the function returns `  json_expr  ` .
  - If `  json_path  ` is SQL `  NULL  ` , the `  json_path_value_pair  ` operation is ignored.

**Return type**

`  JSON  `

**Examples**

In the following example, path `  $[1]  ` is matched and inserts `  1  ` .

``` text
SELECT JSON_ARRAY_INSERT(JSON '["a", ["b", "c"], "d"]', '$[1]', 1) AS json_data

/*-----------------------+
 | json_data             |
 +-----------------------+
 | ["a",1,["b","c"],"d"] |
 +-----------------------*/
```

In the following example, path `  $[1][0]  ` is matched and inserts `  1  ` .

``` text
SELECT JSON_ARRAY_INSERT(JSON '["a", ["b", "c"], "d"]', '$[1][0]', 1) AS json_data

/*-----------------------+
 | json_data             |
 +-----------------------+
 | ["a",[1,"b","c"],"d"] |
 +-----------------------*/
```

In the following example, `  insert_each_element  ` defaults to `  TRUE  ` , so `  [1, 2]  ` is inserted as individual elements.

``` text
SELECT JSON_ARRAY_INSERT(JSON '["a", "b", "c"]', '$[1]', [1, 2]) AS json_data

/*-------------------+
 | json_data         |
 +-------------------+
 | ["a",1,2,"b","c"] |
 +-------------------*/
```

In the following example, `  insert_each_element  ` is `  FALSE  ` , so `  [1, 2]  ` is inserted as one element.

``` text
SELECT JSON_ARRAY_INSERT(
  JSON '["a", "b", "c"]',
  '$[1]', [1, 2],
  insert_each_element=>FALSE) AS json_data

/*---------------------+
 | json_data           |
 +---------------------+
 | ["a",[1,2],"b","c"] |
 +---------------------*/
```

In the following example, path `  $[7]  ` is larger than the length of the matched array, so the array is extended with JSON nulls and `  "e"  ` is inserted at the end of the array.

``` text
SELECT JSON_ARRAY_INSERT(JSON '["a", "b", "c", "d"]', '$[7]', "e") AS json_data

/*--------------------------------------+
 | json_data                            |
 +--------------------------------------+
 | ["a","b","c","d",null,null,null,"e"] |
 +--------------------------------------*/
```

In the following example, path `  $.a  ` is an object, so the operation is ignored.

``` text
SELECT JSON_ARRAY_INSERT(JSON '{"a": {}}', '$.a[0]', 2) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"a":{}}  |
 +-----------*/
```

In the following example, path `  $  ` doesn't specify a valid array position, so the operation is ignored.

``` text
SELECT JSON_ARRAY_INSERT(JSON '[1, 2]', '$', 3) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | [1,2]     |
 +-----------*/
```

In the following example, a value is inserted into a JSON null.

``` text
SELECT JSON_ARRAY_INSERT(JSON '{"a": null}', '$.a[2]', 10) AS json_data

/*----------------------+
 | json_data            |
 +----------------------+
 | {"a":[null,null,10]} |
 +----------------------*/
```

In the following example, the operation is ignored because you can't insert data into a JSON number.

``` text
SELECT JSON_ARRAY_INSERT(JSON '1', '$[0]', 'r1') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | 1         |
 +-----------*/
```

## `     JSON_CONTAINS    `

``` text
JSON_CONTAINS(json_expr, json_expr)
```

**Description**

Checks if a JSON document contains another JSON document. This function returns `  true  ` if the first parameter JSON document contains the second parameter JSON document; otherwise the function returns `  false  ` . If any input argument is `  NULL  ` , a `  NULL  ` value is returned.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"class": {"students": [{"name": "Jane"}]}}'
    ```

Details:

  - The structure and data of the contained document must match a portion of the containing document. This function determines if the smaller JSON document is part of the larger JSON document.

  - JSON scalars: A JSON scalar value (like a string, number, bool, or JSON null ) contains only itself.

  - JSON objects:
    
      - An object contains another object if the first object contains all the key-value pairs present in the second JSON object.
      - When checking for object containment, extra key-value pairs in the containing object don't prevent a match.
      - Any JSON object can contain an empty object.

  - JSON arrays:
    
      - An array contains another array if every element of the second array is contained by some element of the first.
      - Duplicate elements in arrays are treated as if they appear only once.
      - The order of elements within JSON arrays isn't significant for containment checks.
      - Any array can contain an empty array.
      - As a special case, a top-level array can contain a scalar value.

**Return type**

`  BOOL  `

**Examples**

In the following example, a JSON scalar value (a string) contains only itself:

``` text
SELECT JSON_CONTAINS(JSON '"a"', JSON '"a"') AS result;

/*----------+
 |  result  |
 +----------+
 |   true   |
 +----------*/
```

The following examples check if a JSON object contains another JSON object:

``` text
SELECT
    JSON_CONTAINS(JSON '{"a": {"b": 1}, "c": 2}', JSON '{"b": 1}') AS result1,
    JSON_CONTAINS(JSON '{"a": {"b": 1}, "c": 2}', JSON '{"a": {"b": 1}}') AS result2,
    JSON_CONTAINS(JSON '{"a": {"b": 1, "d": 3}, "c": 2}', JSON '{"a": {"b": 1}}') AS result3;

/*----------*----------*----------+
 |  result1 |  result2 |  result3 |
 +----------+----------+----------+
 |   false  |   true   |   true   |
 +----------*----------*----------*/
```

The following examples check if a JSON array contains another JSON array. An array contains another array if the first JSON array contains all the elements present in the second array. The order of elements doesn't matter.

Also, if the array is a top-level array, it can contain a scalar value.

``` text
SELECT
    JSON_CONTAINS(JSON '[1, 2, 3]', JSON '[2]') AS result1,
    JSON_CONTAINS(JSON '[1, 2, 3]', JSON '2') AS result2;

/*----------*----------+
 |  result1 |  result2 |
 +----------+----------+
 |   true   |   true   |
 +----------*----------*/
```

``` text
SELECT
    JSON_CONTAINS(JSON '[[1, 2, 3]]', JSON '2') AS result1,
    JSON_CONTAINS(JSON '[[1, 2, 3]]', JSON '[2]') AS result2,
    JSON_CONTAINS(JSON '[[1, 2, 3]]', JSON '[[2]]') AS result3;

/*----------*----------*----------+
 |  result1 |  result2 |  result3 |
 +----------+----------+----------+
 |   false  |   false  |   true   |
 +----------*----------*----------*/
```

The following examples check if a JSON array contains a JSON object:

``` text
SELECT
    JSON_CONTAINS(JSON '[{"a":0}, {"b":1, "c":2}]', JSON '[{"b":1}]') AS result1,
    JSON_CONTAINS(JSON '[{"a":0}, {"b":1, "c":2}]', JSON '{"b":1}') AS results2,
    JSON_CONTAINS(JSON '[{"a":0}, {"b":1, "c":2}]', JSON '[{"a":0, "b":1}]') AS results3;

/*----------*----------*----------+
 |  result1 |  result2 |  result3 |
 +----------+----------+----------+
 |   true   |   false  |   false  |
 +----------*----------*----------*/
```

## `     JSON_KEYS    `

``` text
JSON_KEYS(
  json_expr
  [, max_depth ]
  [, mode => { 'strict' | 'lax' | 'lax recursive' } ]
)
```

**Description**

Extracts unique JSON keys from a JSON expression.

Arguments:

  - `  json_expr  ` : `  JSON  ` . For example:
    
    ``` text
    JSON '{"class": {"students": [{"name": "Jane"}]}}'
    ```

  - `  max_depth  ` : An `  INT64  ` value that represents the maximum depth of nested fields to search in `  json_expr  ` . If not set, the function searches the entire JSON document.

  - `  mode  ` : A named argument with a `  STRING  ` value that can be one of the following:
    
      - `  strict  ` (default): Ignore any key that appears in an array.
      - `  lax  ` : Also include keys contained in non-consecutively nested arrays.
      - `  lax recursive  ` : Return all keys.

Details:

  - Keys are de-duplicated and returned in alphabetical order.
  - Keys don't include array indices.
  - Keys containing special characters are escaped using double quotes.
  - Keys are case sensitive and not normalized.
  - If `  json_expr  ` or `  mode  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - If `  max_depth  ` is SQL `  NULL  ` , the function ignores the argument.
  - If `  max_depth  ` is less than or equal to 0, then an error is returned.

**Return type**

`  ARRAY<STRING>  `

**Examples**

In the following example, there are no arrays, so all keys are returned.

``` text
SELECT JSON_KEYS(JSON '{"a": {"b":1}}') AS json_keys

/*-----------+
 | json_keys |
 +-----------+
 | [a, a.b]  |
 +-----------*/
```

In the following example, `  max_depth  ` is set to 1 so "a.b" isn't included.

``` text
SELECT JSON_KEYS(JSON '{"a": {"b":1}}', 1) AS json_keys

/*-----------+
 | json_keys |
 +-----------+
 | [a]       |
 +-----------*/
```

In the following example, the `  json_expr  ` argument contains an array. Because the mode is `  strict  ` , keys inside the array are excluded.

``` text
SELECT JSON_KEYS(JSON '{"a":[{"b":1}, {"c":2}], "d":3}') AS json_keys

/*-----------+
 | json_keys |
 +-----------+
 | [a, d]    |
 +-----------*/
```

In the following example, the `  json_expr  ` argument contains an array. Because the mode is `  lax  ` , keys inside the array are included.

``` text
SELECT JSON_KEYS(
  JSON '{"a":[{"b":1}, {"c":2}], "d":3}',
  mode => "lax") as json_keys

/*------------------+
 | json_keys        |
 +------------------+
 | [a, a.b, a.c, d] |
 +------------------*/
```

In the following example, the `  json_expr  ` argument contains consecutively nested arrays. Because the mode is `  lax  ` , keys inside the consecutively nested arrays aren't included.

``` text
SELECT JSON_KEYS(JSON '{"a":[[{"b":1}]]}', mode => "lax") as json_keys

/*-----------+
 | json_keys |
 +-----------+
 | [a]       |
 +-----------*/
```

In the following example, the `  json_expr  ` argument contains consecutively nested arrays. Because the mode is `  lax recursive  ` , every key is returned.

``` text
SELECT JSON_KEYS(JSON '{"a":[[{"b":1}]]}', mode => "lax recursive") as json_keys

/*-----------+
 | json_keys |
 +-----------+
 | [a, a.b]  |
 +-----------*/
```

In the following example, the `  json_expr  ` argument contains multiple arrays. Because the arrays aren't consecutively nested and the mode is `  lax  ` , keys inside the arrays are included.

``` text
SELECT JSON_KEYS(JSON '{"a":[{"b":[{"c":1}]}]}', mode => "lax") as json_keys

/*-----------------+
 | json_keys       |
 +-----------------+
 | [a, a.b, a.b.c] |
 +-----------------*/
```

In the following example, the `  json_expr  ` argument contains both consecutively nested and single arrays. Because the mode is `  lax  ` , keys inside the consecutively nested arrays are excluded.

``` text
SELECT JSON_KEYS(JSON '{"a":[{"b":[[{"c":1}]]}]}', mode => "lax") as json_keys

/*-----------+
 | json_keys |
 +-----------+
 | [a, a.b]  |
 +-----------*/
```

In the following example, the `  json_expr  ` argument contains both consecutively nested and single arrays. Because the mode is `  lax recursive  ` , all keys are included.

``` text
SELECT JSON_KEYS(
  JSON '{"a":[{"b":[[{"c":1}]]}]}', mode => "lax recursive") as json_keys

/*-----------------+
 | json_keys       |
 +-----------------+
 | [a, a.b, a.b.c] |
 +-----------------*/
```

## `     JSON_OBJECT    `

  - [Signature 1](#json_object_signature1) : `  JSON_OBJECT([json_key, json_value][, ...])  `
  - [Signature 2](#json_object_signature2) : `  JSON_OBJECT(json_key_array, json_value_array)  `

#### Signature 1

``` text
JSON_OBJECT([json_key, json_value][, ...])
```

**Description**

Creates a JSON object, using key-value pairs.

Arguments:

  - `  json_key  ` : A `  STRING  ` value that represents a key.
  - `  json_value  ` : A [JSON encoding-supported](#json_encodings) value.

Details:

  - If two keys are passed in with the same name, only the first key-value pair is preserved.
  - The order of key-value pairs isn't preserved.
  - If `  json_key  ` is `  NULL  ` , an error is produced.

**Return type**

`  JSON  `

**Examples**

You can create an empty JSON object by passing in no JSON keys and values. For example:

``` text
SELECT JSON_OBJECT() AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {}        |
 +-----------*/
```

You can create a JSON object by passing in key-value pairs. For example:

``` text
SELECT JSON_OBJECT('foo', 10, 'bar', TRUE) AS json_data

/*-----------------------+
 | json_data             |
 +-----------------------+
 | {"bar":true,"foo":10} |
 +-----------------------*/
```

``` text
SELECT JSON_OBJECT('foo', 10, 'bar', ['a', 'b']) AS json_data

/*----------------------------+
 | json_data                  |
 +----------------------------+
 | {"bar":["a","b"],"foo":10} |
 +----------------------------*/
```

``` text
SELECT JSON_OBJECT('a', NULL, 'b', JSON 'null') AS json_data

/*---------------------+
 | json_data           |
 +---------------------+
 | {"a":null,"b":null} |
 +---------------------*/
```

``` text
SELECT JSON_OBJECT('a', 10, 'a', 'foo') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"a":10}  |
 +-----------*/
```

``` text
WITH Items AS (SELECT 'hello' AS key, 'world' AS value)
SELECT JSON_OBJECT(key, value) AS json_data FROM Items

/*-------------------+
 | json_data         |
 +-------------------+
 | {"hello":"world"} |
 +-------------------*/
```

An error is produced if a SQL `  NULL  ` is passed in for a JSON key.

``` text
-- Error: A key can't be NULL.
SELECT JSON_OBJECT(NULL, 1) AS json_data
```

An error is produced if the number of JSON keys and JSON values don't match:

``` text
-- Error: No matching signature for function JSON_OBJECT for argument types:
-- STRING, INT64, STRING
SELECT JSON_OBJECT('a', 1, 'b') AS json_data
```

#### Signature 2

``` text
JSON_OBJECT(json_key_array, json_value_array)
```

Creates a JSON object, using an array of keys and values.

Arguments:

  - `  json_key_array  ` : An array of zero or more `  STRING  ` keys.
  - `  json_value_array  ` : An array of zero or more [JSON encoding-supported](#json_encodings) values.

Details:

  - If two keys are passed in with the same name, only the first key-value pair is preserved.
  - The order of key-value pairs isn't preserved.
  - The number of keys must match the number of values, otherwise an error is produced.
  - If any argument is `  NULL  ` , an error is produced.
  - If a key in `  json_key_array  ` is `  NULL  ` , an error is produced.

**Return type**

`  JSON  `

**Examples**

You can create an empty JSON object by passing in an empty array of keys and values. For example:

``` text
SELECT JSON_OBJECT(CAST([] AS ARRAY<STRING>), []) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {}        |
 +-----------*/
```

You can create a JSON object by passing in an array of keys and an array of values. For example:

``` text
SELECT JSON_OBJECT(['a', 'b'], [10, NULL]) AS json_data

/*-------------------+
 | json_data         |
 +-------------------+
 | {"a":10,"b":null} |
 +-------------------*/
```

``` text
SELECT JSON_OBJECT(['a', 'b'], [JSON '10', JSON '"foo"']) AS json_data

/*--------------------+
 | json_data          |
 +--------------------+
 | {"a":10,"b":"foo"} |
 +--------------------*/
```

``` text
SELECT
  JSON_OBJECT(
    ['a', 'b'],
    [STRUCT(10 AS id, 'Red' AS color), STRUCT(20 AS id, 'Blue' AS color)])
    AS json_data

/*------------------------------------------------------------+
 | json_data                                                  |
 +------------------------------------------------------------+
 | {"a":{"color":"Red","id":10},"b":{"color":"Blue","id":20}} |
 +------------------------------------------------------------*/
```

``` text
SELECT
  JSON_OBJECT(
    ['a', 'b'],
    [TO_JSON(10), TO_JSON(['foo', 'bar'])])
    AS json_data

/*----------------------------+
 | json_data                  |
 +----------------------------+
 | {"a":10,"b":["foo","bar"]} |
 +----------------------------*/
```

The following query groups by `  id  ` and then creates an array of keys and values from the rows with the same `  id  ` :

``` text
WITH
  Fruits AS (
    SELECT 0 AS id, 'color' AS json_key, 'red' AS json_value UNION ALL
    SELECT 0, 'fruit', 'apple' UNION ALL
    SELECT 1, 'fruit', 'banana' UNION ALL
    SELECT 1, 'ripe', 'true'
  )
SELECT JSON_OBJECT(ARRAY_AGG(json_key), ARRAY_AGG(json_value)) AS json_data
FROM Fruits
GROUP BY id

/*----------------------------------+
 | json_data                        |
 +----------------------------------+
 | {"color":"red","fruit":"apple"}  |
 | {"fruit":"banana","ripe":"true"} |
 +----------------------------------*/
```

An error is produced if the size of the JSON keys and values arrays don't match:

``` text
-- Error: The number of keys and values must match.
SELECT JSON_OBJECT(['a', 'b'], [10]) AS json_data
```

An error is produced if the array of JSON keys or JSON values is a SQL `  NULL  ` .

``` text
-- Error: The keys array can't be NULL.
SELECT JSON_OBJECT(CAST(NULL AS ARRAY<STRING>), [10, 20]) AS json_data
```

``` text
-- Error: The values array can't be NULL.
SELECT JSON_OBJECT(['a', 'b'], CAST(NULL AS ARRAY<INT64>)) AS json_data
```

## `     JSON_QUERY    `

``` text
JSON_QUERY(json_string_expr, json_path)
```

``` text
JSON_QUERY(json_expr, json_path)
```

**Description**

Extracts a JSON value and converts it to a SQL JSON-formatted `  STRING  ` or `  JSON  ` value. This function uses double quotes to escape invalid [JSONPath](#JSONPath_format) characters in JSON keys. For example: `  "a.b"  ` .

Arguments:

  - `  json_string_expr  ` : A JSON-formatted string. For example:
    
    ``` text
    '{"class": {"students": [{"name": "Jane"}]}}'
    ```
    
    Extracts a SQL `  NULL  ` when a JSON-formatted string `  null  ` is encountered. For example:
    
    ``` text
    SELECT JSON_QUERY("null", "$") -- Returns a SQL NULL
    ```

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"class": {"students": [{"name": "Jane"}]}}'
    ```
    
    Extracts a JSON `  null  ` when a JSON `  null  ` is encountered.
    
    ``` text
    SELECT JSON_QUERY(JSON 'null', "$") -- Returns a JSON 'null'
    ```

  - `  json_path  ` : The [JSONPath](#JSONPath_format) . This identifies the data that you want to obtain from the input.

There are differences between the JSON-formatted string and JSON input types. For details, see [Differences between the JSON and JSON-formatted STRING types](#differences_json_and_string) .

**Return type**

  - `  json_string_expr  ` : A JSON-formatted `  STRING  `
  - `  json_expr  ` : `  JSON  `

**Examples**

In the following example, JSON data is extracted and returned as JSON.

``` text
SELECT
  JSON_QUERY(
    JSON '{"class": {"students": [{"id": 5}, {"id": 12}]}}',
    '$.class') AS json_data;

/*-----------------------------------+
 | json_data                         |
 +-----------------------------------+
 | {"students":[{"id":5},{"id":12}]} |
 +-----------------------------------*/
```

In the following examples, JSON data is extracted and returned as JSON-formatted strings.

``` text
SELECT
  JSON_QUERY('{"class": {"students": [{"name": "Jane"}]}}', '$') AS json_text_string;

/*-----------------------------------------------------------+
 | json_text_string                                          |
 +-----------------------------------------------------------+
 | {"class":{"students":[{"name":"Jane"}]}}                  |
 +-----------------------------------------------------------*/
```

``` text
SELECT JSON_QUERY('{"class": {"students": []}}', '$') AS json_text_string;

/*-----------------------------------------------------------+
 | json_text_string                                          |
 +-----------------------------------------------------------+
 | {"class":{"students":[]}}                                 |
 +-----------------------------------------------------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "John"},{"name": "Jamie"}]}}',
    '$') AS json_text_string;

/*-----------------------------------------------------------+
 | json_text_string                                          |
 +-----------------------------------------------------------+
 | {"class":{"students":[{"name":"John"},{"name":"Jamie"}]}} |
 +-----------------------------------------------------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "Jane"}]}}',
    '$.class.students[0]') AS first_student;

/*-----------------+
 | first_student   |
 +-----------------+
 | {"name":"Jane"} |
 +-----------------*/
```

``` text
SELECT
  JSON_QUERY('{"class": {"students": []}}', '$.class.students[0]') AS first_student;

/*-----------------+
 | first_student   |
 +-----------------+
 | NULL            |
 +-----------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "John"}, {"name": "Jamie"}]}}',
    '$.class.students[0]') AS first_student;

/*-----------------+
 | first_student   |
 +-----------------+
 | {"name":"John"} |
 +-----------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "Jane"}]}}',
    '$.class.students[1].name') AS second_student;

/*----------------+
 | second_student |
 +----------------+
 | NULL           |
 +----------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": []}}',
    '$.class.students[1].name') AS second_student;

/*----------------+
 | second_student |
 +----------------+
 | NULL           |
 +----------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "John"}, {"name": null}]}}',
    '$.class.students[1].name') AS second_student;

/*----------------+
 | second_student |
 +----------------+
 | NULL           |
 +----------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "John"}, {"name": "Jamie"}]}}',
    '$.class.students[1].name') AS second_student;

/*----------------+
 | second_student |
 +----------------+
 | "Jamie"        |
 +----------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "Jane"}]}}',
    '$.class."students"') AS student_names;

/*------------------------------------+
 | student_names                      |
 +------------------------------------+
 | [{"name":"Jane"}]                  |
 +------------------------------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": []}}',
    '$.class."students"') AS student_names;

/*------------------------------------+
 | student_names                      |
 +------------------------------------+
 | []                                 |
 +------------------------------------*/
```

``` text
SELECT
  JSON_QUERY(
    '{"class": {"students": [{"name": "John"}, {"name": "Jamie"}]}}',
    '$.class."students"') AS student_names;

/*------------------------------------+
 | student_names                      |
 +------------------------------------+
 | [{"name":"John"},{"name":"Jamie"}] |
 +------------------------------------*/
```

``` text
SELECT JSON_QUERY('{"a": null}', "$.a"); -- Returns a SQL NULL
SELECT JSON_QUERY('{"a": null}', "$.b"); -- Returns a SQL NULL
```

``` text
SELECT JSON_QUERY(JSON '{"a": null}', "$.a"); -- Returns a JSON 'null'
SELECT JSON_QUERY(JSON '{"a": null}', "$.b"); -- Returns a SQL NULL
```

## `     JSON_QUERY_ARRAY    `

``` text
JSON_QUERY_ARRAY(json_string_expr[, json_path])
```

``` text
JSON_QUERY_ARRAY(json_expr[, json_path])
```

**Description**

Extracts a JSON array and converts it to a SQL `  ARRAY<JSON-formatted STRING>  ` or `  ARRAY<JSON>  ` value. In addition, this function uses double quotes to escape invalid [JSONPath](#JSONPath_format) characters in JSON keys. For example: `  "a.b"  ` .

Arguments:

  - `  json_string_expr  ` : A JSON-formatted string. For example:
    
    ``` text
    '["a", "b", {"key": "c"}]'
    ```

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '["a", "b", {"key": "c"}]'
    ```

  - `  json_path  ` : The [JSONPath](#JSONPath_format) . This identifies the data that you want to obtain from the input. If this optional parameter isn't provided, then the JSONPath `  $  ` symbol is applied, which means that all of the data is analyzed.

There are differences between the JSON-formatted string and JSON input types. For details, see [Differences between the JSON and JSON-formatted STRING types](#differences_json_and_string) .

**Return type**

  - `  json_string_expr  ` : `  ARRAY<JSON-formatted STRING>  `
  - `  json_expr  ` : `  ARRAY<JSON>  `

**Examples**

This extracts items in JSON to an array of `  JSON  ` values:

``` text
SELECT JSON_QUERY_ARRAY(
  JSON '{"fruits": ["apples", "oranges", "grapes"]}', '$.fruits'
  ) AS json_array;

/*---------------------------------+
 | json_array                      |
 +---------------------------------+
 | ["apples", "oranges", "grapes"] |
 +---------------------------------*/
```

This extracts the items in a JSON-formatted string to a string array:

``` text
SELECT JSON_QUERY_ARRAY('[1, 2, 3]') AS string_array;

/*--------------+
 | string_array |
 +--------------+
 | [1, 2, 3]    |
 +--------------*/
```

This extracts a string array and converts it to an integer array:

``` text
SELECT ARRAY(
  SELECT CAST(integer_element AS INT64)
  FROM UNNEST(
    JSON_QUERY_ARRAY('[1, 2, 3]','$')
  ) AS integer_element
) AS integer_array;

/*---------------+
 | integer_array |
 +---------------+
 | [1, 2, 3]     |
 +---------------*/
```

This extracts string values in a JSON-formatted string to an array:

``` text
-- Doesn't strip the double quotes
SELECT JSON_QUERY_ARRAY('["apples", "oranges", "grapes"]', '$') AS string_array;

/*---------------------------------+
 | string_array                    |
 +---------------------------------+
 | ["apples", "oranges", "grapes"] |
 +---------------------------------*/
```

``` text
-- Strips the double quotes
SELECT ARRAY(
  SELECT JSON_VALUE(string_element, '$')
  FROM UNNEST(JSON_QUERY_ARRAY('["apples", "oranges", "grapes"]', '$')) AS string_element
) AS string_array;

/*---------------------------+
 | string_array              |
 +---------------------------+
 | [apples, oranges, grapes] |
 +---------------------------*/
```

This extracts only the items in the `  fruit  ` property to an array:

``` text
SELECT JSON_QUERY_ARRAY(
  '{"fruit": [{"apples": 5, "oranges": 10}, {"apples": 2, "oranges": 4}], "vegetables": [{"lettuce": 7, "kale": 8}]}',
  '$.fruit'
) AS string_array;

/*-------------------------------------------------------+
 | string_array                                          |
 +-------------------------------------------------------+
 | [{"apples":5,"oranges":10}, {"apples":2,"oranges":4}] |
 +-------------------------------------------------------*/
```

These are equivalent:

``` text
SELECT JSON_QUERY_ARRAY('{"fruits": ["apples", "oranges", "grapes"]}', '$.fruits') AS string_array;

SELECT JSON_QUERY_ARRAY('{"fruits": ["apples", "oranges", "grapes"]}', '$."fruits"') AS string_array;

-- The queries above produce the following result:
/*---------------------------------+
 | string_array                    |
 +---------------------------------+
 | ["apples", "oranges", "grapes"] |
 +---------------------------------*/
```

In cases where a JSON key uses invalid JSONPath characters, you can escape those characters using double quotes: `  " "  ` . For example:

``` text
SELECT JSON_QUERY_ARRAY('{"a.b": {"c": ["world"]}}', '$."a.b".c') AS hello;

/*-----------+
 | hello     |
 +-----------+
 | ["world"] |
 +-----------*/
```

The following examples show how invalid requests and empty arrays are handled:

``` text
-- An error is returned if you provide an invalid JSONPath.
SELECT JSON_QUERY_ARRAY('["foo", "bar", "baz"]', 'INVALID_JSONPath') AS result;

-- If the JSONPath doesn't refer to an array, then NULL is returned.
SELECT JSON_QUERY_ARRAY('{"a": "foo"}', '$.a') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- If a key that doesn't exist is specified, then the result is NULL.
SELECT JSON_QUERY_ARRAY('{"a": "foo"}', '$.b') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- Empty arrays in JSON-formatted strings are supported.
SELECT JSON_QUERY_ARRAY('{"a": "foo", "b": []}', '$.b') AS result;

/*--------+
 | result |
 +--------+
 | []     |
 +--------*/
```

## `     JSON_REMOVE    `

``` text
JSON_REMOVE(json_expr, json_path[, ...])
```

Produces a new SQL `  JSON  ` value with the specified JSON data removed.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"class": {"students": [{"name": "Jane"}]}}'
    ```

  - `  json_path  ` : Remove data at this [JSONPath](#JSONPath_format) in `  json_expr  ` .

Details:

  - Paths are evaluated left to right. The JSON produced by evaluating the first path is the JSON for the next path.
  - The operation ignores non-existent paths and continue processing the rest of the paths.
  - For each path, the entire matched JSON subtree is deleted.
  - If the path matches a JSON object key, this function deletes the key-value pair.
  - If the path matches an array element, this function deletes the specific element from the matched array.
  - If removing the path results in an empty JSON object or empty JSON array, the empty structure is preserved.
  - If `  json_path  ` is `  $  ` or an invalid [JSONPath](#JSONPath_format) , an error is produced.
  - If `  json_path  ` is SQL `  NULL  ` , the path operation is ignored.

**Return type**

`  JSON  `

**Examples**

In the following example, the path `  $[1]  ` is matched and removes `  ["b", "c"]  ` .

``` text
SELECT JSON_REMOVE(JSON '["a", ["b", "c"], "d"]', '$[1]') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | ["a","d"] |
 +-----------*/
```

You can use the field access operator to pass JSON data into this function. For example:

``` text
WITH T AS (SELECT JSON '{"a": {"b": 10, "c": 20}}' AS data)
SELECT JSON_REMOVE(data.a, '$.b') AS json_data FROM T

/*-----------+
 | json_data |
 +-----------+
 | {"c":20}  |
 +-----------*/
```

In the following example, the first path `  $[1]  ` is matched and removes `  ["b", "c"]  ` . Then, the second path `  $[1]  ` is matched and removes `  "d"  ` .

``` text
SELECT JSON_REMOVE(JSON '["a", ["b", "c"], "d"]', '$[1]', '$[1]') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | ["a"]     |
 +-----------*/
```

The structure of an empty array is preserved when all elements are deleted from it. For example:

``` text
SELECT JSON_REMOVE(JSON '["a", ["b", "c"], "d"]', '$[1]', '$[1]', '$[0]') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | []        |
 +-----------*/
```

In the following example, the path `  $.a.b.c  ` is matched and removes the `  "c":"d"  ` key-value pair from the JSON object.

``` text
SELECT JSON_REMOVE(JSON '{"a": {"b": {"c": "d"}}}', '$.a.b.c') AS json_data

/*----------------+
 | json_data      |
 +----------------+
 | {"a":{"b":{}}} |
 +----------------*/
```

In the following example, the path `  $.a.b  ` is matched and removes the `  "b": {"c":"d"}  ` key-value pair from the JSON object.

``` text
SELECT JSON_REMOVE(JSON '{"a": {"b": {"c": "d"}}}', '$.a.b') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"a":{}}  |
 +-----------*/
```

In the following example, the path `  $.b  ` isn't valid, so the operation makes no changes.

``` text
SELECT JSON_REMOVE(JSON '{"a": 1}', '$.b') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"a":1}   |
 +-----------*/
```

In the following example, path `  $.a.b  ` and `  $.b  ` don't exist, so those operations are ignored, but the others are processed.

``` text
SELECT JSON_REMOVE(JSON '{"a": [1, 2, 3]}', '$.a[0]', '$.a.b', '$.b', '$.a[0]') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"a":[3]} |
 +-----------*/
```

If you pass in `  $  ` as the path, an error is produced. For example:

``` text
-- Error: The JSONPath can't be '$'
SELECT JSON_REMOVE(JSON '{}', '$') AS json_data
```

In the following example, the operation is ignored because you can't remove data from a JSON null.

``` text
SELECT JSON_REMOVE(JSON 'null', '$.a.b') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | null      |
 +-----------*/
```

## `     JSON_SET    `

``` text
JSON_SET(
  json_expr,
  json_path_value_pair[, ...]
  [, create_if_missing => { TRUE | FALSE } ]
)

json_path_value_pair:
  json_path, value
```

Produces a new SQL `  JSON  ` value with the specified JSON data inserted or replaced.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"class": {"students": [{"name": "Jane"}]}}'
    ```

  - `  json_path_value_pair  ` : A value and the [JSONPath](#JSONPath_format) for that value. This includes:
    
      - `  json_path  ` : Insert or replace `  value  ` at this [JSONPath](#JSONPath_format) in `  json_expr  ` .
    
      - `  value  ` : A [JSON encoding-supported](#json_encodings) value to insert.

  - `  create_if_missing  ` : A named argument that takes a `  BOOL  ` value.
    
      - If `  TRUE  ` (default), replaces or inserts data if the path doesn't exist.
    
      - If `  FALSE  ` , only existing JSONPath values are replaced. If the path doesn't exist, the set operation is ignored.

Details:

  - Path value pairs are evaluated left to right. The JSON produced by evaluating one pair becomes the JSON against which the next pair is evaluated.

  - If a matched path has an existing value, it overwrites the existing data with `  value  ` .

  - If `  create_if_missing  ` is `  TRUE  ` :
    
      - If a path doesn't exist, the remainder of the path is recursively created.
      - If the matched path prefix points to a JSON null, the remainder of the path is recursively created, and `  value  ` is inserted.
      - If a path token points to a JSON array and the specified index is *larger* than the size of the array, pads the JSON array with JSON nulls, recursively creates the remainder of the path at the specified index, and inserts the path value pair.

  - This function applies all path value pair set operations even if an individual path value pair operation is invalid. For invalid operations, the operation is ignored and the function continues to process the rest of the path value pairs.

  - If the path exists but has an incompatible type at any given path token, no update happens for that specific path value pair.

  - If any `  json_path  ` is an invalid [JSONPath](#JSONPath_format) , an error is produced.

  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .

  - If `  json_path  ` is SQL `  NULL  ` , the `  json_path_value_pair  ` operation is ignored.

  - If `  create_if_missing  ` is SQL `  NULL  ` , the set operation is ignored.

**Return type**

`  JSON  `

**Examples**

In the following example, the path `  $  ` matches the entire `  JSON  ` value and replaces it with `  {"b": 2, "c": 3}  ` .

``` text
SELECT JSON_SET(JSON '{"a": 1}', '$', JSON '{"b": 2, "c": 3}') AS json_data

/*---------------+
 | json_data     |
 +---------------+
 | {"b":2,"c":3} |
 +---------------*/
```

In the following example, `  create_if_missing  ` is `  FALSE  ` and the path `  $.b  ` doesn't exist, so the set operation is ignored.

``` text
SELECT JSON_SET(
  JSON '{"a": 1}',
  "$.b", 999,
  create_if_missing => false) AS json_data

/*------------+
 | json_data  |
 +------------+
 | '{"a": 1}' |
 +------------*/
```

In the following example, `  create_if_missing  ` is `  TRUE  ` and the path `  $.a  ` exists, so the value is replaced.

``` text
SELECT JSON_SET(
  JSON '{"a": 1}',
  "$.a", 999,
  create_if_missing => false) AS json_data

/*--------------+
 | json_data    |
 +--------------+
 | '{"a": 999}' |
 +--------------*/
```

In the following example, the path `  $.a  ` is matched, but `  $.a.b  ` doesn't exist, so the new path and the value are inserted.

``` text
SELECT JSON_SET(JSON '{"a": {}}', '$.a.b', 100) AS json_data

/*-----------------+
 | json_data       |
 +-----------------+
 | {"a":{"b":100}} |
 +-----------------*/
```

In the following example, the path prefix `  $  ` points to a JSON null, so the remainder of the path is created for the value `  100  ` .

``` text
SELECT JSON_SET(JSON 'null', '$.a.b', 100) AS json_data

/*-----------------+
 | json_data       |
 +-----------------+
 | {"a":{"b":100}} |
 +-----------------*/
```

In the following example, the path `  $.a.c  ` implies that the value at `  $.a  ` is a JSON object but it's not. This part of the operation is ignored, but the other parts of the operation are completed successfully.

``` text
SELECT JSON_SET(
  JSON '{"a": 1}',
  '$.b', 2,
  '$.a.c', 100,
  '$.d', 3) AS json_data

/*---------------------+
 | json_data           |
 +---------------------+
 | {"a":1,"b":2,"d":3} |
 +---------------------*/
```

In the following example, the path `  $.a[2]  ` implies that the value for `  $.a  ` is an array, but it's not, so the operation is ignored for that value.

``` text
SELECT JSON_SET(
  JSON '{"a": 1}',
  '$.a[2]', 100,
  '$.b', 2) AS json_data

/*---------------+
 | json_data     |
 +---------------+
 | {"a":1,"b":2} |
 +---------------*/
```

In the following example, the path `  $[1]  ` is matched and replaces the array element value with `  foo  ` .

``` text
SELECT JSON_SET(JSON '["a", ["b", "c"], "d"]', '$[1]', "foo") AS json_data

/*-----------------+
 | json_data       |
 +-----------------+
 | ["a","foo","d"] |
 +-----------------*/
```

In the following example, the path `  $[1][0]  ` is matched and replaces the array element value with `  foo  ` .

``` text
SELECT JSON_SET(JSON '["a", ["b", "c"], "d"]', '$[1][0]', "foo") AS json_data

/*-----------------------+
 | json_data             |
 +-----------------------+
 | ["a",["foo","c"],"d"] |
 +-----------------------*/
```

In the following example, the path prefix `  $  ` points to a JSON null, so the remainder of the path is created. The resulting array is padded with JSON nulls and appended with `  foo  ` .

``` text
SELECT JSON_SET(JSON 'null', '$[0][3]', "foo")

/*--------------------------+
 | json_data                |
 +--------------------------+
 | [[null,null,null,"foo"]] |
 +--------------------------*/
```

In the following example, the path `  $[1]  ` is matched, the matched array is extended since `  $[1][4]  ` is larger than the existing array, and then `  foo  ` is inserted in the array.

``` text
SELECT JSON_SET(JSON '["a", ["b", "c"], "d"]', '$[1][4]', "foo") AS json_data

/*-------------------------------------+
 | json_data                           |
 +-------------------------------------+
 | ["a",["b","c",null,null,"foo"],"d"] |
 +-------------------------------------*/
```

In the following example, the path `  $[1][0][0]  ` implies that the value of `  $[1][0]  ` is an array, but it isn't, so the operation is ignored.

``` text
SELECT JSON_SET(JSON '["a", ["b", "c"], "d"]', '$[1][0][0]', "foo") AS json_data

/*---------------------+
 | json_data           |
 +---------------------+
 | ["a",["b","c"],"d"] |
 +---------------------*/
```

In the following example, the path `  $[1][2]  ` is larger than the length of the matched array. The array length is extended and the remainder of the path is recursively created. The operation continues to the path `  $[1][2][1]  ` and inserts `  foo  ` .

``` text
SELECT JSON_SET(JSON '["a", ["b", "c"], "d"]', '$[1][2][1]', "foo") AS json_data

/*----------------------------------+
 | json_data                        |
 +----------------------------------+
 | ["a",["b","c",[null,"foo"]],"d"] |
 +----------------------------------*/
```

In the following example, because the `  JSON  ` object is empty, key `  b  ` is inserted, and the remainder of the path is recursively created.

``` text
SELECT JSON_SET(JSON '{}', '$.b[2].d', 100) AS json_data

/*-----------------------------+
 | json_data                   |
 +-----------------------------+
 | {"b":[null,null,{"d":100}]} |
 +-----------------------------*/
```

In the following example, multiple values are set.

``` text
SELECT JSON_SET(
  JSON '{"a": 1, "b": {"c":3}, "d": [4]}',
  '$.a', 'v1',
  '$.b.e', 'v2',
  '$.d[2]', 'v3') AS json_data

/*---------------------------------------------------+
 | json_data                                         |
 +---------------------------------------------------+
 | {"a":"v1","b":{"c":3,"e":"v2"},"d":[4,null,"v3"]} |
 +---------------------------------------------------*/
```

## `     JSON_STRIP_NULLS    `

``` text
JSON_STRIP_NULLS(
  json_expr
  [, json_path ]
  [, include_arrays => { TRUE | FALSE } ]
  [, remove_empty => { TRUE | FALSE } ]
)
```

Recursively removes JSON nulls from JSON objects and JSON arrays.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"a": null, "b": "c"}'
    ```

  - `  json_path  ` : Remove JSON nulls at this [JSONPath](#JSONPath_format) for `  json_expr  ` .

  - `  include_arrays  ` : A named argument that's either `  TRUE  ` (default) or `  FALSE  ` . If `  TRUE  ` or omitted, the function removes JSON nulls from JSON arrays. If `  FALSE  ` , doesn't.

  - `  remove_empty  ` : A named argument that's either `  TRUE  ` or `  FALSE  ` (default). If `  TRUE  ` , the function removes empty JSON objects after JSON nulls are removed. If `  FALSE  ` or omitted, doesn't.
    
    If `  remove_empty  ` is `  TRUE  ` and `  include_arrays  ` is `  TRUE  ` or omitted, the function additionally removes empty JSON arrays.

Details:

  - If a value is a JSON null, the associated key-value pair is removed.
  - If `  remove_empty  ` is set to `  TRUE  ` , the function recursively removes empty containers after JSON nulls are removed.
  - If the function generates JSON with nothing in it, the function returns a JSON null.
  - If `  json_path  ` is an invalid [JSONPath](#JSONPath_format) , an error is produced.
  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - If `  json_path  ` , `  include_arrays  ` , or `  remove_empty  ` is SQL `  NULL  ` , the function returns `  json_expr  ` .

**Return type**

`  JSON  `

**Examples**

In the following example, all JSON nulls are removed.

``` text
SELECT JSON_STRIP_NULLS(JSON '{"a": null, "b": "c"}') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"b":"c"} |
 +-----------*/
```

In the following example, all JSON nulls are removed from a JSON array.

``` text
SELECT JSON_STRIP_NULLS(JSON '[1, null, 2, null]') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | [1,2]     |
 +-----------*/
```

In the following example, `  include_arrays  ` is set as `  FALSE  ` so that JSON nulls aren't removed from JSON arrays.

``` text
SELECT JSON_STRIP_NULLS(JSON '[1, null, 2, null]', include_arrays=>FALSE) AS json_data

/*-----------------+
 | json_data       |
 +-----------------+
 | [1,null,2,null] |
 +-----------------*/
```

In the following example, `  remove_empty  ` is omitted and defaults to `  FALSE  ` , and the empty structures are retained.

``` text
SELECT JSON_STRIP_NULLS(JSON '[1, null, 2, null, [null]]') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | [1,2,[]]  |
 +-----------*/
```

In the following example, `  remove_empty  ` is set as `  TRUE  ` , and the empty structures are removed.

``` text
SELECT JSON_STRIP_NULLS(
  JSON '[1, null, 2, null, [null]]',
  remove_empty=>TRUE) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | [1,2]     |
 +-----------*/
```

In the following examples, `  remove_empty  ` is set as `  TRUE  ` , and the empty structures are removed. Because no JSON data is left the function returns JSON null.

``` text
SELECT JSON_STRIP_NULLS(JSON '{"a": null}', remove_empty=>TRUE) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | null      |
 +-----------*/
```

``` text
SELECT JSON_STRIP_NULLS(JSON '{"a": [null]}', remove_empty=>TRUE) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | null      |
 +-----------*/
```

In the following example, empty structures are removed for JSON objects, but not JSON arrays.

``` text
SELECT JSON_STRIP_NULLS(
  JSON '{"a": {"b": {"c": null}}, "d": [null], "e": [], "f": 1}',
  include_arrays=>FALSE,
  remove_empty=>TRUE) AS json_data

/*---------------------------+
 | json_data                 |
 +---------------------------+
 | {"d":[null],"e":[],"f":1} |
 +---------------------------*/
```

In the following example, empty structures are removed for both JSON objects, and JSON arrays.

``` text
SELECT JSON_STRIP_NULLS(
  JSON '{"a": {"b": {"c": null}}, "d": [null], "e": [], "f": 1}',
  remove_empty=>TRUE) AS json_data

/*-----------+
 | json_data |
 +-----------+
 | {"f":1}   |
 +-----------*/
```

In the following example, because no JSON data is left, the function returns a JSON null.

``` text
SELECT JSON_STRIP_NULLS(JSON 'null') AS json_data

/*-----------+
 | json_data |
 +-----------+
 | null      |
 +-----------*/
```

## `     JSON_TYPE    `

``` text
JSON_TYPE(json_expr)
```

**Description**

Gets the JSON type of the outermost JSON value and converts the name of this type to a SQL `  STRING  ` value. The names of these JSON types can be returned: `  object  ` , `  array  ` , `  string  ` , `  number  ` , `  boolean  ` , `  null  `

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"name": "sky", "color": "blue"}'
    ```
    
    If this expression is SQL `  NULL  ` , the function returns SQL `  NULL  ` . If the extracted JSON value isn't a valid JSON type, an error is produced.

**Return type**

`  STRING  `

**Examples**

``` text
SELECT json_val, JSON_TYPE(json_val) AS type
FROM
  UNNEST(
    [
      JSON '"apple"',
      JSON '10',
      JSON '3.14',
      JSON 'null',
      JSON '{"city": "New York", "State": "NY"}',
      JSON '["apple", "banana"]',
      JSON 'false'
    ]
  ) AS json_val;

/*----------------------------------+---------+
 | json_val                         | type    |
 +----------------------------------+---------+
 | "apple"                          | string  |
 | 10                               | number  |
 | 3.14                             | number  |
 | null                             | null    |
 | {"State":"NY","city":"New York"} | object  |
 | ["apple","banana"]               | array   |
 | false                            | boolean |
 +----------------------------------+---------*/
```

## `     JSON_VALUE    `

``` text
JSON_VALUE(json_string_expr[, json_path])
```

``` text
JSON_VALUE(json_expr[, json_path])
```

**Description**

Extracts a JSON scalar value and converts it to a SQL `  STRING  ` value. In addition, this function:

  - Removes the outermost quotes and unescapes the values.
  - Returns a SQL `  NULL  ` if a non-scalar value is selected.
  - Uses double quotes to escape invalid [JSONPath](#JSONPath_format) characters in JSON keys. For example: `  "a.b"  ` .

Arguments:

  - `  json_string_expr  ` : A JSON-formatted string. For example:
    
    ``` text
    '{"name": "Jakob", "age": "6"}'
    ```

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"name": "Jane", "age": "6"}'
    ```

  - `  json_path  ` : The [JSONPath](#JSONPath_format) . This identifies the data that you want to obtain from the input. If this optional parameter isn't provided, then the JSONPath `  $  ` symbol is applied, which means that all of the data is analyzed.
    
    If `  json_path  ` returns a JSON `  null  ` or a non-scalar value (in other words, if `  json_path  ` refers to an object or an array), then a SQL `  NULL  ` is returned.

There are differences between the JSON-formatted string and JSON input types. For details, see [Differences between the JSON and JSON-formatted STRING types](#differences_json_and_string) .

**Return type**

`  STRING  `

**Examples**

In the following example, JSON data is extracted and returned as a scalar value.

``` text
SELECT JSON_VALUE(JSON '{"name": "Jakob", "age": "6" }', '$.age') AS scalar_age;

/*------------+
 | scalar_age |
 +------------+
 | 6          |
 +------------*/
```

The following example compares how results are returned for the `  JSON_QUERY  ` and `  JSON_VALUE  ` functions.

``` text
SELECT JSON_QUERY('{"name": "Jakob", "age": "6"}', '$.name') AS json_name,
  JSON_VALUE('{"name": "Jakob", "age": "6"}', '$.name') AS scalar_name,
  JSON_QUERY('{"name": "Jakob", "age": "6"}', '$.age') AS json_age,
  JSON_VALUE('{"name": "Jakob", "age": "6"}', '$.age') AS scalar_age;

/*-----------+-------------+----------+------------+
 | json_name | scalar_name | json_age | scalar_age |
 +-----------+-------------+----------+------------+
 | "Jakob"   | Jakob       | "6"      | 6          |
 +-----------+-------------+----------+------------*/
```

``` text
SELECT JSON_QUERY('{"fruits": ["apple", "banana"]}', '$.fruits') AS json_query,
  JSON_VALUE('{"fruits": ["apple", "banana"]}', '$.fruits') AS json_value;

/*--------------------+------------+
 | json_query         | json_value |
 +--------------------+------------+
 | ["apple","banana"] | NULL       |
 +--------------------+------------*/
```

In cases where a JSON key uses invalid JSONPath characters, you can escape those characters using double quotes. For example:

``` text
SELECT JSON_VALUE('{"a.b": {"c": "world"}}', '$."a.b".c') AS hello;

/*-------+
 | hello |
 +-------+
 | world |
 +-------*/
```

## `     JSON_VALUE_ARRAY    `

``` text
JSON_VALUE_ARRAY(json_string_expr[, json_path])
```

``` text
JSON_VALUE_ARRAY(json_expr[, json_path])
```

**Description**

Extracts a JSON array of scalar values and converts it to a SQL `  ARRAY<STRING>  ` value. In addition, this function:

  - Removes the outermost quotes and unescapes the values.
  - Returns a SQL `  NULL  ` if the selected value isn't an array or not an array containing only scalar values.
  - Uses double quotes to escape invalid [JSONPath](#JSONPath_format) characters in JSON keys. For example: `  "a.b"  ` .

Arguments:

  - `  json_string_expr  ` : A JSON-formatted string. For example:
    
    ``` text
    '["apples", "oranges", "grapes"]'
    ```

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '["apples", "oranges", "grapes"]'
    ```

  - `  json_path  ` : The [JSONPath](#JSONPath_format) . This identifies the data that you want to obtain from the input. If this optional parameter isn't provided, then the JSONPath `  $  ` symbol is applied, which means that all of the data is analyzed.

There are differences between the JSON-formatted string and JSON input types. For details, see [Differences between the JSON and JSON-formatted STRING types](#differences_json_and_string) .

Caveats:

  - A JSON `  null  ` in the input array produces a SQL `  NULL  ` as the output for that JSON `  null  ` .
  - If a JSONPath matches an array that contains scalar objects and a JSON `  null  ` , then the output is an array of the scalar objects and a SQL `  NULL  ` .

**Return type**

`  ARRAY<STRING>  `

**Examples**

This extracts items in JSON to a string array:

``` text
SELECT JSON_VALUE_ARRAY(
  JSON '{"fruits": ["apples", "oranges", "grapes"]}', '$.fruits'
  ) AS string_array;

/*---------------------------+
 | string_array              |
 +---------------------------+
 | [apples, oranges, grapes] |
 +---------------------------*/
```

The following example compares how results are returned for the `  JSON_QUERY_ARRAY  ` and `  JSON_VALUE_ARRAY  ` functions.

``` text
SELECT JSON_QUERY_ARRAY('["apples", "oranges"]') AS json_array,
       JSON_VALUE_ARRAY('["apples", "oranges"]') AS string_array;

/*-----------------------+-------------------+
 | json_array            | string_array      |
 +-----------------------+-------------------+
 | ["apples", "oranges"] | [apples, oranges] |
 +-----------------------+-------------------*/
```

This extracts the items in a JSON-formatted string to a string array:

``` text
-- Strips the double quotes
SELECT JSON_VALUE_ARRAY('["foo", "bar", "baz"]', '$') AS string_array;

/*-----------------+
 | string_array    |
 +-----------------+
 | [foo, bar, baz] |
 +-----------------*/
```

This extracts a string array and converts it to an integer array:

``` text
SELECT ARRAY(
  SELECT CAST(integer_element AS INT64)
  FROM UNNEST(
    JSON_VALUE_ARRAY('[1, 2, 3]', '$')
  ) AS integer_element
) AS integer_array;

/*---------------+
 | integer_array |
 +---------------+
 | [1, 2, 3]     |
 +---------------*/
```

These are equivalent:

``` text
SELECT JSON_VALUE_ARRAY('{"fruits": ["apples", "oranges", "grapes"]}', '$.fruits') AS string_array;
SELECT JSON_VALUE_ARRAY('{"fruits": ["apples", "oranges", "grapes"]}', '$."fruits"') AS string_array;

-- The queries above produce the following result:
/*---------------------------+
 | string_array              |
 +---------------------------+
 | [apples, oranges, grapes] |
 +---------------------------*/
```

In cases where a JSON key uses invalid JSONPath characters, you can escape those characters using double quotes: `  " "  ` . For example:

``` text
SELECT JSON_VALUE_ARRAY('{"a.b": {"c": ["world"]}}', '$."a.b".c') AS hello;

/*---------+
 | hello   |
 +---------+
 | [world] |
 +---------*/
```

The following examples explore how invalid requests and empty arrays are handled:

``` text
-- An error is thrown if you provide an invalid JSONPath.
SELECT JSON_VALUE_ARRAY('["foo", "bar", "baz"]', 'INVALID_JSONPath') AS result;

-- If the JSON-formatted string is invalid, then NULL is returned.
SELECT JSON_VALUE_ARRAY('}}', '$') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- If the JSON document is NULL, then NULL is returned.
SELECT JSON_VALUE_ARRAY(NULL, '$') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- If a JSONPath doesn't match anything, then the output is NULL.
SELECT JSON_VALUE_ARRAY('{"a": ["foo", "bar", "baz"]}', '$.b') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- If a JSONPath matches an object that isn't an array, then the output is NULL.
SELECT JSON_VALUE_ARRAY('{"a": "foo"}', '$') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- If a JSONPath matches an array of non-scalar objects, then the output is NULL.
SELECT JSON_VALUE_ARRAY('{"a": [{"b": "foo", "c": 1}, {"b": "bar", "c": 2}], "d": "baz"}', '$.a') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- If a JSONPath matches an array of mixed scalar and non-scalar objects,
-- then the output is NULL.
SELECT JSON_VALUE_ARRAY('{"a": [10, {"b": 20}]', '$.a') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/

-- If a JSONPath matches an empty JSON array, then the output is an empty array instead of NULL.
SELECT JSON_VALUE_ARRAY('{"a": "foo", "b": []}', '$.b') AS result;

/*--------+
 | result |
 +--------+
 | []     |
 +--------*/

-- In the following query, the JSON null input is returned as a
-- SQL NULL in the output.
SELECT JSON_VALUE_ARRAY('["world", null, 1]') AS result;

/*------------------+
 | result           |
 +------------------+
 | [world, NULL, 1] |
 +------------------*/
```

## `     LAX_BOOL    `

``` text
LAX_BOOL(json_expr)
```

**Description**

Attempts to convert a JSON value to a SQL `  BOOL  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON 'true'
    ```

Details:

  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - See the conversion rules in the next section for additional `  NULL  ` handling.

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From JSON type</th>
<th>To SQL <code dir="ltr" translate="no">       BOOL      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>boolean</td>
<td>If the JSON boolean is <code dir="ltr" translate="no">       true      </code> , returns <code dir="ltr" translate="no">       TRUE      </code> . Otherwise, returns <code dir="ltr" translate="no">       FALSE      </code> .</td>
</tr>
<tr class="even">
<td>string</td>
<td>If the JSON string is <code dir="ltr" translate="no">       'true'      </code> , returns <code dir="ltr" translate="no">       TRUE      </code> . If the JSON string is <code dir="ltr" translate="no">       'false'      </code> , returns <code dir="ltr" translate="no">       FALSE      </code> . If the JSON string is any other value or has whitespace in it, returns <code dir="ltr" translate="no">       NULL      </code> . This conversion is case-insensitive.</td>
</tr>
<tr class="odd">
<td>number</td>
<td>If the JSON number is a representation of <code dir="ltr" translate="no">       0      </code> , returns <code dir="ltr" translate="no">       FALSE      </code> . Otherwise, returns <code dir="ltr" translate="no">       TRUE      </code> .</td>
</tr>
<tr class="even">
<td>other type or null</td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
</tr>
</tbody>
</table>

**Return type**

`  BOOL  `

**Examples**

Example with input that's a JSON boolean:

``` text
SELECT LAX_BOOL(JSON 'true') AS result;

/*--------+
 | result |
 +--------+
 | true   |
 +--------*/
```

Examples with inputs that are JSON strings:

``` text
SELECT LAX_BOOL(JSON '"true"') AS result;

/*--------+
 | result |
 +--------+
 | TRUE   |
 +--------*/
```

``` text
SELECT LAX_BOOL(JSON '"true "') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

``` text
SELECT LAX_BOOL(JSON '"foo"') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

Examples with inputs that are JSON numbers:

``` text
SELECT LAX_BOOL(JSON '10') AS result;

/*--------+
 | result |
 +--------+
 | TRUE   |
 +--------*/
```

``` text
SELECT LAX_BOOL(JSON '0') AS result;

/*--------+
 | result |
 +--------+
 | FALSE  |
 +--------*/
```

``` text
SELECT LAX_BOOL(JSON '0.0') AS result;

/*--------+
 | result |
 +--------+
 | FALSE  |
 +--------*/
```

``` text
SELECT LAX_BOOL(JSON '-1.1') AS result;

/*--------+
 | result |
 +--------+
 | TRUE   |
 +--------*/
```

## `     LAX_FLOAT64    `

``` text
LAX_FLOAT64(json_expr)
```

**Description**

Attempts to convert a JSON value to a SQL `  FLOAT64  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '9.8'
    ```

Details:

  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - See the conversion rules in the next section for additional `  NULL  ` handling.

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From JSON type</th>
<th>To SQL <code dir="ltr" translate="no">       FLOAT64      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>boolean</td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
</tr>
<tr class="even">
<td>string</td>
<td>If the JSON string represents a JSON number, parses it as a JSON number, and then safe casts the result as a <code dir="ltr" translate="no">       FLOAT64      </code> value. If the JSON string can't be converted, returns <code dir="ltr" translate="no">       NULL      </code> .</td>
</tr>
<tr class="odd">
<td>number</td>
<td>Casts the JSON number as a <code dir="ltr" translate="no">       FLOAT64      </code> value. Large JSON numbers are rounded.</td>
</tr>
<tr class="even">
<td>other type or null</td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
</tr>
</tbody>
</table>

**Return type**

`  FLOAT64  `

**Examples**

Examples with inputs that are JSON numbers:

``` text
SELECT LAX_FLOAT64(JSON '9.8') AS result;

/*--------+
 | result |
 +--------+
 | 9.8    |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON '9') AS result;

/*--------+
 | result |
 +--------+
 | 9.0    |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON '9007199254740993') AS result;

/*--------------------+
 | result             |
 +--------------------+
 | 9007199254740992.0 |
 +--------------------*/
```

``` text
SELECT LAX_FLOAT64(JSON '1e100') AS result;

/*--------+
 | result |
 +--------+
 | 1e+100 |
 +--------*/
```

Examples with inputs that are JSON booleans:

``` text
SELECT LAX_FLOAT64(JSON 'true') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON 'false') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

Examples with inputs that are JSON strings:

``` text
SELECT LAX_FLOAT64(JSON '"10"') AS result;

/*--------+
 | result |
 +--------+
 | 10.0   |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"1.1"') AS result;

/*--------+
 | result |
 +--------+
 | 1.1    |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"1.1e2"') AS result;

/*--------+
 | result |
 +--------+
 | 110.0  |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"9007199254740993"') AS result;

/*--------------------+
 | result             |
 +--------------------+
 | 9007199254740992.0 |
 +--------------------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"+1.5"') AS result;

/*--------+
 | result |
 +--------+
 | 1.5    |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"NaN"') AS result;

/*--------+
 | result |
 +--------+
 | NaN    |
 +--------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"Inf"') AS result;

/*----------+
 | result   |
 +----------+
 | Infinity |
 +----------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"-InfiNiTY"') AS result;

/*-----------+
 | result    |
 +-----------+
 | -Infinity |
 +-----------*/
```

``` text
SELECT LAX_FLOAT64(JSON '"foo"') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

## `     LAX_INT64    `

``` text
LAX_INT64(json_expr)
```

**Description**

Attempts to convert a JSON value to a SQL `  INT64  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '999'
    ```

Details:

  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - See the conversion rules in the next section for additional `  NULL  ` handling.

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From JSON type</th>
<th>To SQL <code dir="ltr" translate="no">       INT64      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>boolean</td>
<td>If the JSON boolean is <code dir="ltr" translate="no">       true      </code> , returns <code dir="ltr" translate="no">       1      </code> . If <code dir="ltr" translate="no">       false      </code> , returns <code dir="ltr" translate="no">       0      </code> .</td>
</tr>
<tr class="even">
<td>string</td>
<td>If the JSON string represents a JSON number, parses it as a JSON number, and then safe casts the results as an <code dir="ltr" translate="no">       INT64      </code> value. If the JSON string can't be converted, returns <code dir="ltr" translate="no">       NULL      </code> .</td>
</tr>
<tr class="odd">
<td>number</td>
<td>Casts the JSON number as an <code dir="ltr" translate="no">       INT64      </code> value. If the JSON number can't be converted, returns <code dir="ltr" translate="no">       NULL      </code> .</td>
</tr>
<tr class="even">
<td>other type or null</td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
</tr>
</tbody>
</table>

**Return type**

`  INT64  `

**Examples**

Examples with inputs that are JSON numbers:

``` text
SELECT LAX_INT64(JSON '10') AS result;

/*--------+
 | result |
 +--------+
 | 10     |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '10.0') AS result;

/*--------+
 | result |
 +--------+
 | 10     |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '1.1') AS result;

/*--------+
 | result |
 +--------+
 | 1      |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '3.5') AS result;

/*--------+
 | result |
 +--------+
 | 4      |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '1.1e2') AS result;

/*--------+
 | result |
 +--------+
 | 110    |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '1e100') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

Examples with inputs that are JSON booleans:

``` text
SELECT LAX_INT64(JSON 'true') AS result;

/*--------+
 | result |
 +--------+
 | 1      |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON 'false') AS result;

/*--------+
 | result |
 +--------+
 | 0      |
 +--------*/
```

Examples with inputs that are JSON strings:

``` text
SELECT LAX_INT64(JSON '"10"') AS result;

/*--------+
 | result |
 +--------+
 | 10     |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '"1.1"') AS result;

/*--------+
 | result |
 +--------+
 | 1      |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '"1.1e2"') AS result;

/*--------+
 | result |
 +--------+
 | 110    |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '"+1.5"') AS result;

/*--------+
 | result |
 +--------+
 | 2      |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '"1e100"') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

``` text
SELECT LAX_INT64(JSON '"foo"') AS result;

/*--------+
 | result |
 +--------+
 | NULL   |
 +--------*/
```

## `     LAX_STRING    `

``` text
LAX_STRING(json_expr)
```

**Description**

Attempts to convert a JSON value to a SQL `  STRING  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '"name"'
    ```

Details:

  - If `  json_expr  ` is SQL `  NULL  ` , the function returns SQL `  NULL  ` .
  - See the conversion rules in the next section for additional `  NULL  ` handling.

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From JSON type</th>
<th>To SQL <code dir="ltr" translate="no">       STRING      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>boolean</td>
<td>If the JSON boolean is <code dir="ltr" translate="no">       true      </code> , returns <code dir="ltr" translate="no">       'true'      </code> . If <code dir="ltr" translate="no">       false      </code> , returns <code dir="ltr" translate="no">       'false'      </code> .</td>
</tr>
<tr class="even">
<td>string</td>
<td>Returns the JSON string as a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td>number</td>
<td>Returns the JSON number as a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td>other type or null</td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
</tr>
</tbody>
</table>

**Return type**

`  STRING  `

**Examples**

Examples with inputs that are JSON strings:

``` text
SELECT LAX_STRING(JSON '"purple"') AS result;

/*--------+
 | result |
 +--------+
 | purple |
 +--------*/
```

``` text
SELECT LAX_STRING(JSON '"10"') AS result;

/*--------+
 | result |
 +--------+
 | 10     |
 +--------*/
```

Examples with inputs that are JSON booleans:

``` text
SELECT LAX_STRING(JSON 'true') AS result;

/*--------+
 | result |
 +--------+
 | true   |
 +--------*/
```

``` text
SELECT LAX_STRING(JSON 'false') AS result;

/*--------+
 | result |
 +--------+
 | false  |
 +--------*/
```

Examples with inputs that are JSON numbers:

``` text
SELECT LAX_STRING(JSON '10.0') AS result;

/*--------+
 | result |
 +--------+
 | 10     |
 +--------*/
```

``` text
SELECT LAX_STRING(JSON '10') AS result;

/*--------+
 | result |
 +--------+
 | 10     |
 +--------*/
```

``` text
SELECT LAX_STRING(JSON '1e100') AS result;

/*--------+
 | result |
 +--------+
 | 1e+100 |
 +--------*/
```

## `     PARSE_JSON    `

``` text
PARSE_JSON(
  json_string_expr
  [, wide_number_mode => { 'exact' | 'round' } ]
)
```

**Description**

Converts a JSON-formatted `  STRING  ` value to a [`  JSON  ` value](https://www.json.org/json-en.html) .

Arguments:

  - `  json_string_expr  ` : A JSON-formatted string. For example:
    
    ``` text
    '{"class": {"students": [{"name": "Jane"}]}}'
    ```

  - `  wide_number_mode  ` : A named argument with a `  STRING  ` value. Determines how to handle numbers that can't be stored in a `  JSON  ` value without the loss of precision. If used, `  wide_number_mode  ` must include one of the following values:
    
      - `  exact  ` (default): Only accept numbers that can be stored without loss of precision. If a number that can't be stored without loss of precision is encountered, the function throws an error.
      - `  round  ` : If a number that can't be stored without loss of precision is encountered, attempt to round it to a number that can be stored without loss of precision. If the number can't be rounded, the function throws an error.
    
    If a number appears in a JSON object or array, the `  wide_number_mode  ` argument is applied to the number in the object or array.

Numbers from the following domains can be stored in JSON without loss of precision:

  - 64-bit signed/unsigned integers, such as `  INT64  `
  - `  FLOAT64  `

**Return type**

`  JSON  `

**Examples**

In the following example, a JSON-formatted string is converted to `  JSON  ` .

``` text
SELECT PARSE_JSON('{"coordinates": [10, 20], "id": 1}') AS json_data;

/*--------------------------------+
 | json_data                      |
 +--------------------------------+
 | {"coordinates":[10,20],"id":1} |
 +--------------------------------*/
```

The following queries fail because:

  - The number that was passed in can't be stored without loss of precision.
  - `  wide_number_mode=>'exact'  ` is used implicitly in the first query and explicitly in the second query.

<!-- end list -->

``` text
SELECT PARSE_JSON('{"id": 922337203685477580701}') AS json_data; -- fails
SELECT PARSE_JSON('{"id": 922337203685477580701}', wide_number_mode=>'exact') AS json_data; -- fails
```

The following query rounds the number to a number that can be stored in JSON.

``` text
SELECT PARSE_JSON('{"id": 922337203685477580701}', wide_number_mode=>'round') AS json_data;

/*------------------------------+
 | json_data                    |
 +------------------------------+
 | {"id":9.223372036854776e+20} |
 +------------------------------*/
```

You can also use valid JSON-formatted strings that don't represent name/value pairs. For example:

``` text
SELECT PARSE_JSON('6') AS json_data;

/*------------------------------+
 | json_data                    |
 +------------------------------+
 | 6                            |
 +------------------------------*/
```

``` text
SELECT PARSE_JSON('"red"') AS json_data;

/*------------------------------+
 | json_data                    |
 +------------------------------+
 | "red"                        |
 +------------------------------*/
```

## `     SAFE_TO_JSON    `

``` text
SAFE_TO_JSON(sql_value)
```

**Description**

Similar to the `  TO_JSON  ` function, but for each unsupported field in the input argument, produces a JSON null instead of an error.

Arguments:

  - `  sql_value  ` : The SQL value to convert to a JSON value. You can review the GoogleSQL data types that this function supports and their [JSON encodings](#json_encodings) .

**Return type**

`  JSON  `

**Example**

The following queries are functionally the same, except that `  SAFE_TO_JSON  ` produces a JSON null instead of an error when a hypothetical unsupported data type is encountered:

``` text
-- Produces a JSON null.
SELECT SAFE_TO_JSON(CAST(b'' AS UNSUPPORTED_TYPE)) as result;
```

``` text
-- Produces an error.
SELECT TO_JSON(CAST(b'' AS UNSUPPORTED_TYPE), stringify_wide_numbers=>TRUE) as result;
```

In the following query, the value for `  ut  ` is ignored because the value is an unsupported type:

``` text
SELECT SAFE_TO_JSON(STRUCT(CAST(b'' AS UNSUPPORTED_TYPE) AS ut) AS result;

/*--------------+
 | result       |
 +--------------+
 | {"ut": null} |
 +--------------*/
```

The following array produces a JSON null instead of an error because the data type for the array isn't supported.

``` text
SELECT SAFE_TO_JSON([
        CAST(b'' AS UNSUPPORTED_TYPE),
        CAST(b'' AS UNSUPPORTED_TYPE),
        CAST(b'' AS UNSUPPORTED_TYPE),
    ]) AS result;

/*------------+
 | result     |
 +------------+
 | null       |
 +------------*/
```

## `     STRING    `

``` text
STRING(json_expr)
```

**Description**

Converts a JSON string to a SQL `  STRING  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '"purple"'
    ```
    
    If the JSON value isn't a string, an error is produced. If the expression is SQL `  NULL  ` , the function returns SQL `  NULL  ` .

**Return type**

`  STRING  `

**Examples**

``` text
SELECT STRING(JSON '"purple"') AS color;

/*--------+
 | color  |
 +--------+
 | purple |
 +--------*/
```

``` text
SELECT STRING(JSON_QUERY(JSON '{"name": "sky", "color": "blue"}', "$.color")) AS color;

/*-------+
 | color |
 +-------+
 | blue  |
 +-------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if the JSON isn't of type string.
SELECT STRING(JSON '123') AS result; -- Throws an error
SELECT STRING(JSON 'null') AS result; -- Throws an error
SELECT SAFE.STRING(JSON '123') AS result; -- Returns a SQL NULL
```

## `     STRING_ARRAY    `

``` text
STRING_ARRAY(json_expr)
```

**Description**

Converts a JSON array of strings to a SQL `  ARRAY<STRING>  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '["purple", "blue"]'
    ```
    
    If the JSON value isn't an array of strings, an error is produced. If the expression is SQL `  NULL  ` , the function returns SQL `  NULL  ` .

**Return type**

`  ARRAY<STRING>  `

**Examples**

``` text
SELECT STRING_ARRAY(JSON '["purple", "blue"]') AS colors;

/*----------------+
 | colors         |
 +----------------+
 | [purple, blue] |
 +----------------*/
```

The following examples show how invalid requests are handled:

``` text
-- An error is thrown if the JSON isn't an array of strings.
SELECT STRING_ARRAY(JSON '[123]') AS result; -- Throws an error
SELECT STRING_ARRAY(JSON '[null]') AS result; -- Throws an error
SELECT STRING_ARRAY(JSON 'null') AS result; -- Throws an error
```

## `     TO_JSON    `

``` text
TO_JSON(
  sql_value
  [, stringify_wide_numbers => { TRUE | FALSE } ]
)
```

**Description**

Converts a SQL value to a JSON value.

Arguments:

  - `  sql_value  ` : The SQL value to convert to a JSON value. You can review the GoogleSQL data types that this function supports and their JSON encodings [here](#json_encodings) .

  - `  stringify_wide_numbers  ` : A named argument that's either `  TRUE  ` or `  FALSE  ` (default).
    
      - If `  TRUE  ` , numeric values outside of the `  FLOAT64  ` type domain are encoded as strings.
      - If `  FALSE  ` (default), numeric values outside of the `  FLOAT64  ` type domain aren't encoded as strings, but are stored as JSON numbers. If a numerical value can't be stored in JSON without loss of precision, an error is thrown.
    
    The following numerical data types are affected by the `  stringify_wide_numbers  ` argument:

  - `  INT64  `

  - `  NUMERIC  `
    
    If one of these numerical data types appears in a container data type such as an `  ARRAY  ` or `  STRUCT  ` , the `  stringify_wide_numbers  ` argument is applied to the numerical data types in the container data type.

**Return type**

`  JSON  `

**Examples**

In the following example, the query converts rows in a table to JSON values.

``` text
With CoordinatesTable AS (
    (SELECT 1 AS id, [10, 20] AS coordinates) UNION ALL
    (SELECT 2 AS id, [30, 40] AS coordinates) UNION ALL
    (SELECT 3 AS id, [50, 60] AS coordinates))
SELECT TO_JSON(t) AS json_objects
FROM CoordinatesTable AS t;

/*--------------------------------+
 | json_objects                   |
 +--------------------------------+
 | {"coordinates":[10,20],"id":1} |
 | {"coordinates":[30,40],"id":2} |
 | {"coordinates":[50,60],"id":3} |
 +--------------------------------*/
```

In the following example, the query returns a large numerical value as a JSON string.

``` text
SELECT TO_JSON(9007199254740993, stringify_wide_numbers=>TRUE) as stringify_on;

/*--------------------+
 | stringify_on       |
 +--------------------+
 | "9007199254740993" |
 +--------------------*/
```

In the following example, both queries return a large numerical value as a JSON number.

``` text
SELECT TO_JSON(9007199254740993, stringify_wide_numbers=>FALSE) as stringify_off;
SELECT TO_JSON(9007199254740993) as stringify_off;

/*------------------+
 | stringify_off    |
 +------------------+
 | 9007199254740993 |
 +------------------*/
```

In the following example, only large numeric values are converted to JSON strings.

``` text
With T1 AS (
  (SELECT 9007199254740993 AS id) UNION ALL
  (SELECT 2 AS id))
SELECT TO_JSON(t, stringify_wide_numbers=>TRUE) AS json_objects
FROM T1 AS t;

/*---------------------------+
 | json_objects              |
 +---------------------------+
 | {"id":"9007199254740993"} |
 | {"id":2}                  |
 +---------------------------*/
```

In this example, the values `  9007199254740993  ` ( `  INT64  ` ) and `  2.1  ` ( `  FLOAT64  ` ) are converted to the common supertype `  FLOAT64  ` , which isn't affected by the `  stringify_wide_numbers  ` argument.

``` text
With T1 AS (
  (SELECT 9007199254740993 AS id) UNION ALL
  (SELECT 2.1 AS id))
SELECT TO_JSON(t, stringify_wide_numbers=>TRUE) AS json_objects
FROM T1 AS t;

/*------------------------------+
 | json_objects                 |
 +------------------------------+
 | {"id":9.007199254740992e+15} |
 | {"id":2.1}                   |
 +------------------------------*/
```

In the following example, a graph path is converted into a JSON array.

``` text
GRAPH FinGraph
MATCH p=(src:Account)-[t1:Transfers]->(dst:Account)
RETURN TO_JSON(p) AS json_array

/*--------------------------------------------------------------------+
 | json_array                                                         |
 +--------------------------------------------------------------------+
 | [{                                                                 |
 |    "identifier":"mUZpbkdyYXBoLkFjY291bnQAeJEg",                    |
 |    "kind":"node",                                                  |
 |    "labels":["Account"],                                           |
 |    "properties":{                                                  |
 |      "create_time":"2020-01-28T01:55:09.206Z",                     |
 |      "id":16,                                                      |
 |      "is_blocked":true,                                            |
 |      "nick_name":"Vacation Fund"                                   |
 |    }                                                               |
 |  },                                                                |
 |  {                                                                 |
 |    "destination_node_identifier":"mUZpbkdyYXBoLkFjY291bnQAeJEo",   |
 |    "identifier":"mUZpbkdyYXBoLkFjY291...",                         |
 |    "kind":"edge",                                                  |
 |    "labels":["Transfers"],                                         |
 |    "properties":{                                                  |
 |      "amount":300.0,                                               |
 |      "create_time":"2020-09-25T09:36:14.926Z",                     |
 |      "id":16,                                                      |
 |      "order_number":"103650009791820",                             |
 |      "to_id":20                                                    |
 |    },                                                              |
 |    "source_node_identifier":"mUZpbkdyYXBoLkFjY291bnQAeJEg"         |
 |  },                                                                |
 |  {                                                                 |
 |    "identifier":"mUZpbkdyYXBoLkFjY291bnQAeJEo",                    |
 |    "kind":"node",                                                  |
 |    "labels":["Account"],                                           |
 |    "properties":{                                                  |
 |      "create_time":"2020-02-18T13:44:20.655Z",                     |
 |      "id":20,                                                      |
 |      "is_blocked":false,                                           |
 |      "nick_name":"Vacation Fund"                                   |
 |    }                                                               |
 |  }                                                                 |
 |  ...                                                               |
 | ]                                                                  |
 +--------------------------------------------------------------------/*
```

## `     TO_JSON_STRING    `

``` text
TO_JSON_STRING(json_expr)
```

**Description**

Converts a JSON value to a SQL JSON-formatted `  STRING  ` value.

Arguments:

  - `  json_expr  ` : JSON. For example:
    
    ``` text
    JSON '{"class": {"students": [{"name": "Jane"}]}}'
    ```

**Return type**

A JSON-formatted `  STRING  `

**Example**

Convert a JSON value to a JSON-formatted `  STRING  ` value.

``` text
SELECT TO_JSON_STRING(JSON '{"id": 1, "coordinates": [10, 20]}') AS json_string

/*--------------------------------+
 | json_string                    |
 +--------------------------------+
 | {"coordinates":[10,20],"id":1} |
 +--------------------------------*/
```

## Supplemental materials

### Differences between the JSON and JSON-formatted STRING types

Many JSON functions accept two input types:

  - [`  JSON  `](/spanner/docs/reference/standard-sql/data-types#json_type) type
  - `  STRING  ` type

The `  STRING  ` version of the extraction functions behaves differently than the `  JSON  ` version, mainly because `  JSON  ` type values are always validated whereas JSON-formatted `  STRING  ` type values aren't.

#### Non-validation of `     STRING    ` inputs

The following `  STRING  ` is invalid JSON because it's missing a trailing `  }  ` :

``` text
{"hello": "world"
```

The JSON function reads the input from the beginning and stops as soon as the field to extract is found, without reading the remainder of the input. A parsing error isn't produced.

With the `  JSON  ` type, however, `  JSON '{"hello": "world"'  ` returns a parsing error.

For example:

``` text
SELECT JSON_VALUE('{"hello": "world"', "$.hello") AS hello;

/*-------+
 | hello |
 +-------+
 | world |
 +-------*/
```

``` text
SELECT JSON_VALUE(JSON '{"hello": "world"', "$.hello") AS hello;
-- An error is returned: Invalid JSON literal: syntax error while parsing
-- object - unexpected end of input; expected '}'
```

#### No strict validation of extracted values

In the following examples, duplicated keys aren't removed when using a JSON-formatted string. Similarly, keys order is preserved. For the `  JSON  ` type, `  JSON '{"key": 1, "key": 2}'  ` will result in `  JSON '{"key":1}'  ` during parsing.

``` text
SELECT JSON_QUERY('{"key": 1, "key": 2}', "$") AS string;

/*-------------------+
 | string            |
 +-------------------+
 | {"key":1,"key":2} |
 +-------------------*/
```

``` text
SELECT JSON_QUERY(JSON '{"key": 1, "key": 2}', "$") AS json;

/*-----------+
 | json      |
 +-----------+
 | {"key":1} |
 +-----------*/
```

#### JSON `     null    `

When using a JSON-formatted `  STRING  ` type in a JSON function, a JSON `  null  ` value is extracted as a SQL `  NULL  ` value.

When using a JSON type in a JSON function, a JSON `  null  ` value returns a JSON `  null  ` value.

``` text
WITH t AS (
  SELECT '{"name": null}' AS json_string, JSON '{"name": null}' AS json)
SELECT JSON_QUERY(json_string, "$.name") AS name_string,
  JSON_QUERY(json_string, "$.name") IS NULL AS name_string_is_null,
  JSON_QUERY(json, "$.name") AS name_json,
  JSON_QUERY(json, "$.name") IS NULL AS name_json_is_null
FROM t;

/*-------------+---------------------+-----------+-------------------+
 | name_string | name_string_is_null | name_json | name_json_is_null |
 +-------------+---------------------+-----------+-------------------+
 | NULL        | true                | null      | false             |
 +-------------+---------------------+-----------+-------------------*/
```

### JSON encodings

You can encode a SQL value as a JSON value with the following functions:

  - `  TO_JSON  `
  - `  JSON_SET  ` (uses `  TO_JSON  ` encoding)
  - `  JSON_ARRAY  ` (uses `  TO_JSON  ` encoding)
  - `  JSON_ARRAY_APPEND  ` (uses `  TO_JSON  ` encoding)
  - `  JSON_ARRAY_INSERT  ` (uses `  TO_JSON  ` encoding)
  - `  JSON_OBJECT  ` (uses `  TO_JSON  ` encoding)

The following SQL to JSON encodings are supported:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From SQL</th>
<th>To JSON</th>
<th>Examples</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>NULL</td>
<td><p>null</p></td>
<td>SQL input: <code dir="ltr" translate="no">       NULL      </code><br />
JSON output: <code dir="ltr" translate="no">       null      </code></td>
</tr>
<tr class="even">
<td>BOOL</td>
<td>boolean</td>
<td>SQL input: <code dir="ltr" translate="no">       TRUE      </code><br />
JSON output: <code dir="ltr" translate="no">       true      </code><br />
SQL input: <code dir="ltr" translate="no">       FALSE      </code><br />
JSON output: <code dir="ltr" translate="no">       false      </code><br />
</td>
</tr>
<tr class="odd">
<td>INT64</td>
<td><p>number or string</p>
<p>If the <code dir="ltr" translate="no">        stringify_wide_numbers       </code> argument is <code dir="ltr" translate="no">        TRUE       </code> and the value is outside of the FLOAT64 type domain, the value is encoded as a string. If the value can't be stored in JSON without loss of precision, the function fails. Otherwise, the value is encoded as a number.</p>
<p>If the <code dir="ltr" translate="no">        stringify_wide_numbers       </code> isn't used or is <code dir="ltr" translate="no">        FALSE       </code> , numeric values outside of the `FLOAT64` type domain aren't encoded as strings, but are stored as JSON numbers. If a numerical value can't be stored in JSON without loss of precision, an error is thrown.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       9007199254740992      </code><br />
JSON output: <code dir="ltr" translate="no">       9007199254740992      </code><br />
SQL input: <code dir="ltr" translate="no">       9007199254740993      </code><br />
JSON output: <code dir="ltr" translate="no">       9007199254740993      </code><br />
SQL input with stringify_wide_numbers=&gt;TRUE: <code dir="ltr" translate="no">       9007199254740992      </code><br />
JSON output: <code dir="ltr" translate="no">       9007199254740992      </code><br />
SQL input with stringify_wide_numbers=&gt;TRUE: <code dir="ltr" translate="no">       9007199254740993      </code><br />
JSON output: <code dir="ltr" translate="no">       "9007199254740993"      </code><br />
</td>
</tr>
<tr class="even">
<td>INTERVAL</td>
<td>string</td>
<td>SQL input: <code dir="ltr" translate="no">       INTERVAL '10:20:30.52' HOUR TO SECOND      </code><br />
JSON output: <code dir="ltr" translate="no">       "PT10H20M30.52S"      </code><br />
SQL input: <code dir="ltr" translate="no">       INTERVAL 1 SECOND      </code><br />
JSON output: <code dir="ltr" translate="no">       "PT1S"      </code><br />
<code dir="ltr" translate="no">       INTERVAL -25 MONTH      </code><br />
JSON output: <code dir="ltr" translate="no">       "P-2Y-1M"      </code><br />
<code dir="ltr" translate="no">       INTERVAL '1 5:30' DAY TO MINUTE      </code><br />
JSON output: <code dir="ltr" translate="no">       "P1DT5H30M"      </code><br />
</td>
</tr>
<tr class="odd">
<td>NUMERIC</td>
<td><p>number or string</p>
<p>If the <code dir="ltr" translate="no">        stringify_wide_numbers       </code> argument is <code dir="ltr" translate="no">        TRUE       </code> and the value is outside of the FLOAT64 type domain, it's encoded as a string. Otherwise, it's encoded as a number.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       -1      </code><br />
JSON output: <code dir="ltr" translate="no">       -1      </code><br />
SQL input: <code dir="ltr" translate="no">       0      </code><br />
JSON output: <code dir="ltr" translate="no">       0      </code><br />
SQL input: <code dir="ltr" translate="no">       9007199254740993      </code><br />
JSON output: <code dir="ltr" translate="no">       9007199254740993      </code><br />
SQL input: <code dir="ltr" translate="no">       123.56      </code><br />
JSON output: <code dir="ltr" translate="no">       123.56      </code><br />
SQL input with stringify_wide_numbers=&gt;TRUE: <code dir="ltr" translate="no">       9007199254740993      </code><br />
JSON output: <code dir="ltr" translate="no">       "9007199254740993"      </code><br />
SQL input with stringify_wide_numbers=&gt;TRUE: <code dir="ltr" translate="no">       123.56      </code><br />
JSON output: <code dir="ltr" translate="no">       123.56      </code><br />
</td>
</tr>
<tr class="even">
<td>FLOAT64</td>
<td><p>number or string</p>
<p><code dir="ltr" translate="no">        +/-inf       </code> and <code dir="ltr" translate="no">        NaN       </code> are encoded as <code dir="ltr" translate="no">        Infinity       </code> , <code dir="ltr" translate="no">        -Infinity       </code> , and <code dir="ltr" translate="no">        NaN       </code> . Otherwise, this value is encoded as a number.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       1.0      </code><br />
JSON output: <code dir="ltr" translate="no">       1      </code><br />
SQL input: <code dir="ltr" translate="no">       9007199254740993      </code><br />
JSON output: <code dir="ltr" translate="no">       9007199254740993      </code><br />
SQL input: <code dir="ltr" translate="no">       "+inf"      </code><br />
JSON output: <code dir="ltr" translate="no">       "Infinity"      </code><br />
SQL input: <code dir="ltr" translate="no">       "-inf"      </code><br />
JSON output: <code dir="ltr" translate="no">       "-Infinity"      </code><br />
SQL input: <code dir="ltr" translate="no">       "NaN"      </code><br />
JSON output: <code dir="ltr" translate="no">       "NaN"      </code><br />
</td>
</tr>
<tr class="odd">
<td>STRING</td>
<td><p>string</p>
<p>Encoded as a string, escaped according to the JSON standard. Specifically, <code dir="ltr" translate="no">        "       </code> , <code dir="ltr" translate="no">        \,       </code> and the control characters from <code dir="ltr" translate="no">        U+0000       </code> to <code dir="ltr" translate="no">        U+001F       </code> are escaped.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       "abc"      </code><br />
JSON output: <code dir="ltr" translate="no">       "abc"      </code><br />
SQL input: <code dir="ltr" translate="no">       "\"abc\""      </code><br />
JSON output: <code dir="ltr" translate="no">       "\"abc\""      </code><br />
</td>
</tr>
<tr class="even">
<td>BYTES</td>
<td><p>string</p>
<p>Uses RFC 4648 Base64 data encoding.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       b"Google"      </code><br />
JSON output: <code dir="ltr" translate="no">       "R29vZ2xl"      </code><br />
</td>
</tr>
<tr class="odd">
<td>ENUM</td>
<td><p>string</p>
<p>Invalid enum values are encoded as their number, such as 0 or 42.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       Color.Red      </code><br />
JSON output: <code dir="ltr" translate="no">       "Red"      </code><br />
</td>
</tr>
<tr class="even">
<td>DATE</td>
<td>string</td>
<td>SQL input: <code dir="ltr" translate="no">       DATE '2017-03-06'      </code><br />
JSON output: <code dir="ltr" translate="no">       "2017-03-06"      </code><br />
</td>
</tr>
<tr class="odd">
<td>TIMESTAMP</td>
<td><p>string</p>
<p>Encoded as ISO 8601 date and time, where T separates the date and time and Z (Zulu/UTC) represents the time zone.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       TIMESTAMP '2017-03-06 12:34:56.789012'      </code><br />
JSON output: <code dir="ltr" translate="no">       "2017-03-06T12:34:56.789012Z"      </code><br />
</td>
</tr>
<tr class="even">
<td>UUID</td>
<td><p>string</p>
<p>Encoded as lowercase hexadecimal format as specified in <a href="https://www.rfc-editor.org/rfc/rfc9562#name-uuid-format">RFC 9562</a> .</p></td>
<td>SQL input: <code dir="ltr" translate="no">       CAST('f81d4fae-7dec-11d0-a765-00a0c91e6bf6' AS UUID)      </code><br />
JSON output: <code dir="ltr" translate="no">       "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"      </code><br />
</td>
</tr>
<tr class="odd">
<td>JSON</td>
<td><p>data of the input JSON</p></td>
<td>SQL input: <code dir="ltr" translate="no">       JSON '{"item": "pen", "price": 10}'      </code><br />
JSON output: <code dir="ltr" translate="no">       {"item":"pen", "price":10}      </code><br />
SQL input: <code dir="ltr" translate="no">       [1, 2, 3]      </code><br />
JSON output: <code dir="ltr" translate="no">       [1, 2, 3]      </code><br />
</td>
</tr>
<tr class="even">
<td>ARRAY</td>
<td><p>array</p>
<p>Can contain zero or more elements.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       ["red", "blue", "green"]      </code><br />
JSON output: <code dir="ltr" translate="no">       ["red","blue","green"]      </code><br />
SQL input: <code dir="ltr" translate="no">       [1, 2, 3]      </code><br />
JSON output: <code dir="ltr" translate="no">       [1,2,3]      </code><br />
</td>
</tr>
<tr class="odd">
<td>STRUCT</td>
<td><p>object</p>
<p>The object can contain zero or more key-value pairs. Each value is formatted according to its type.</p>
<p>For <code dir="ltr" translate="no">        TO_JSON       </code> , a field is included in the output string and any duplicates of this field are omitted.</p>
<p>Anonymous fields are represented with <code dir="ltr" translate="no">        ""       </code> .</p>
<p>Invalid UTF-8 field names might result in unparseable JSON. String values are escaped according to the JSON standard. Specifically, <code dir="ltr" translate="no">        "       </code> , <code dir="ltr" translate="no">        \,       </code> and the control characters from <code dir="ltr" translate="no">        U+0000       </code> to <code dir="ltr" translate="no">        U+001F       </code> are escaped.</p></td>
<td>SQL input: <code dir="ltr" translate="no">       STRUCT(12 AS purchases, TRUE AS inStock)      </code><br />
JSON output: <code dir="ltr" translate="no">       {"inStock": true,"purchases":12}      </code><br />
</td>
</tr>
<tr class="even">
<td>PROTO</td>
<td><p>object</p>
<p>The object can contain zero or more key-value pairs. Each value is formatted according to its type.</p>
<p>Field names with underscores are converted to camel case in accordance with <a href="https://developers.google.com/protocol-buffers/docs/proto3#json">protobuf json conversion</a> . Field values are formatted according to <a href="https://developers.google.com/protocol-buffers/docs/proto3#json">protobuf json conversion</a> . If a <code dir="ltr" translate="no">        field_value       </code> is a non-empty repeated field or submessage, the elements and fields are indented to the appropriate level.</p>
<ul>
<li>Field names that aren't valid UTF-8 might result in unparseable JSON.</li>
<li>Field annotations are ignored.</li>
<li>Repeated fields are represented as arrays.</li>
<li>Submessages are formatted as values of PROTO type.</li>
<li>Extension fields are included in the output, where the extension field name is enclosed in brackets and prefixed with the full name of the extension type.</li>
</ul></td>
<td>SQL input: <code dir="ltr" translate="no">       NEW Item(12 AS purchases,TRUE AS in_Stock)      </code><br />
JSON output: <code dir="ltr" translate="no">       {"purchases":12,"inStock": true}      </code><br />
</td>
</tr>
<tr class="odd">
<td>GRAPH_ELEMENT</td>
<td><p>object</p>
<p>The object can contain zero or more key-value pairs. Each value is formatted according to its type.</p>
<p>For <code dir="ltr" translate="no">        TO_JSON       </code> , graph element (node or edge) objects are supported.</p>
<ul>
<li>The graph element identifier is only valid within the scope of the same query response and can't be used to correlate entities across different queries.</li>
<li>Field names that aren't valid UTF-8 might result in unparseable JSON.</li>
<li>The result may include internal key-value pairs that aren't defined by the users.</li>
<li>The conversion can fail if the object contains values of unsupported types.</li>
</ul></td>
<td>SQL:<br />

<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>GRAPH FinGraph
MATCH (p:Person WHERE p.name = &#39;Dana&#39;)
RETURN TO_JSON(p) AS dana_json;</code></pre>
<br />
JSON output (truncated):<br />

<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>{&quot;identifier&quot;:&quot;ZGFuYQ==&quot;,&quot;kind&quot;:&quot;node&quot;,&quot;labels&quot;:[&quot;Person&quot;],&quot;properties&quot;:{&quot;id&quot;:2,&quot;name&quot;:&quot;Dana&quot;}}</code></pre></td>
</tr>
<tr class="even">
<td>GRAPH_PATH</td>
<td><p>array</p>
<p>The array can contain one or more objects that represent graph elements in a graph path.</p></td>
<td>SQL:<br />

<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>GRAPH FinGraph
MATCH account_ownership = (p:Person)-[o:Owns]-&gt;(a:Account)
RETURN TO_JSON(account_ownership) AS results</code></pre>
<br />
JSON output for <code dir="ltr" translate="no">       account_ownership      </code> (truncated):<br />

<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>[
  {&quot;identifier&quot;:&quot;ZGFuYQ==&quot;,&quot;kind&quot;:&quot;node&quot;,&quot;labels&quot;:[&quot;Person&quot;], ...},
  {&quot;identifier&quot;:&quot;TPZuYM==&quot;,&quot;kind&quot;:&quot;edge&quot;,&quot;labels&quot;:[&quot;Owns&quot;], ...},
  {&quot;identifier&quot;:&quot;PRTuMI==&quot;,&quot;kind&quot;:&quot;node&quot;,&quot;labels&quot;:[&quot;Account&quot;], ...}
]</code></pre></td>
</tr>
</tbody>
</table>

### JSONPath format

With the JSONPath format, you can identify the values you want to obtain from a JSON-formatted string.

If a key in a JSON functions contains a JSON format operator, refer to each JSON function for how to escape them.

A JSON function returns `  NULL  ` if the JSONPath format doesn't match a value in a JSON-formatted string. If the selected value for a scalar function isn't scalar, such as an object or an array, the function returns `  NULL  ` . If the JSONPath format is invalid, an error is produced.

#### Operators for JSONPath

The JSONPath format supports these operators:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Operator</th>
<th>Description</th>
<th>Examples</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       $      </code></td>
<td>Root object or element. The JSONPath format must start with this operator, which refers to the outermost level of the JSON-formatted string.</td>
<td><p>JSON-formatted string:<br />
<code dir="ltr" translate="no">        '{"class" : {"students" : [{"name" : "Jane"}]}}'       </code></p>
<p>JSON path:<br />
<code dir="ltr" translate="no">        "$"       </code></p>
<p>JSON result:<br />
<code dir="ltr" translate="no">        {"class":{"students":[{"name":"Jane"}]}}       </code><br />
</p></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       .      </code></td>
<td>Child operator. You can identify child values using dot-notation.</td>
<td><p>JSON-formatted string:<br />
<code dir="ltr" translate="no">        '{"class" : {"students" : [{"name" : "Jane"}]}}'       </code></p>
<p>JSON path:<br />
<code dir="ltr" translate="no">        "$.class.students"       </code></p>
<p>JSON result:<br />
<code dir="ltr" translate="no">        [{"name":"Jane"}]       </code></p></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       []      </code></td>
<td>Subscript operator. If the object is a JSON array, you can use brackets to specify the array index.</td>
<td><p>JSON-formatted string:<br />
<code dir="ltr" translate="no">        '{"class" : {"students" : [{"name" : "Jane"}]}}'       </code></p>
<p>JSON path:<br />
<code dir="ltr" translate="no">        "$.class.students[0]"       </code></p>
<p>JSON result:<br />
<code dir="ltr" translate="no">        {"name":"Jane"}       </code></p></td>
</tr>
</tbody>
</table>
