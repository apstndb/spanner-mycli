GoogleSQL for Spanner supports conversion functions. These data type conversions are explicit, but some conversions can happen implicitly. You can learn more about implicit and explicit conversion [here](/spanner/docs/reference/standard-sql/conversion_rules) .

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
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_to_string"><code dir="ltr" translate="no">        ARRAY_TO_STRING       </code></a></td>
<td>Produces a concatenation of the elements in an array as a <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/array_functions">Array functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#bool_for_json"><code dir="ltr" translate="no">        BOOL       </code></a></td>
<td>Converts a JSON boolean to a SQL <code dir="ltr" translate="no">       BOOL      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#bool_array_for_json"><code dir="ltr" translate="no">        BOOL_ARRAY       </code></a></td>
<td>Converts a JSON array of booleans to a SQL <code dir="ltr" translate="no">       ARRAY&lt;BOOL&gt;      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/conversion_functions#cast"><code dir="ltr" translate="no">        CAST       </code></a></td>
<td>Convert the results of an expression to the given type.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#code_points_to_bytes"><code dir="ltr" translate="no">        CODE_POINTS_TO_BYTES       </code></a></td>
<td>Converts an array of extended ASCII code points to a <code dir="ltr" translate="no">       BYTES      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String aggregate functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#code_points_to_string"><code dir="ltr" translate="no">        CODE_POINTS_TO_STRING       </code></a></td>
<td>Converts an array of extended ASCII code points to a <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String aggregate functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_from_unix_date"><code dir="ltr" translate="no">        DATE_FROM_UNIX_DATE       </code></a></td>
<td>Interprets an <code dir="ltr" translate="no">       INT64      </code> expression as the number of days since 1970-01-01.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/date_functions">Date functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_base32"><code dir="ltr" translate="no">        FROM_BASE32       </code></a></td>
<td>Converts a base32-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_base64"><code dir="ltr" translate="no">        FROM_BASE64       </code></a></td>
<td>Converts a base64-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_hex"><code dir="ltr" translate="no">        FROM_HEX       </code></a></td>
<td>Converts a hexadecimal-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#int64_for_json"><code dir="ltr" translate="no">        INT64       </code></a></td>
<td>Converts a JSON number to a SQL <code dir="ltr" translate="no">       INT64      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#int64_array_for_json"><code dir="ltr" translate="no">        INT64_ARRAY       </code></a></td>
<td>Converts a JSON array of numbers to a SQL <code dir="ltr" translate="no">       ARRAY&lt;INT64&gt;      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_bool"><code dir="ltr" translate="no">        LAX_BOOL       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       BOOL      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_double"><code dir="ltr" translate="no">        LAX_FLOAT64       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       FLOAT64      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_int64"><code dir="ltr" translate="no">        LAX_INT64       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       INT64      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_string"><code dir="ltr" translate="no">        LAX_STRING       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#parse_date"><code dir="ltr" translate="no">        PARSE_DATE       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       DATE      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/date_functions">Date functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#parse_json"><code dir="ltr" translate="no">        PARSE_JSON       </code></a></td>
<td>Converts a JSON-formatted <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       JSON      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#parse_timestamp"><code dir="ltr" translate="no">        PARSE_TIMESTAMP       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/conversion_functions#safe_casting"><code dir="ltr" translate="no">        SAFE_CAST       </code></a></td>
<td>Similar to the <code dir="ltr" translate="no">       CAST      </code> function, but returns <code dir="ltr" translate="no">       NULL      </code> when a runtime error is produced.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#safe_convert_bytes_to_string"><code dir="ltr" translate="no">        SAFE_CONVERT_BYTES_TO_STRING       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a <code dir="ltr" translate="no">       STRING      </code> value and replace any invalid UTF-8 characters with the Unicode replacement character, <code dir="ltr" translate="no">       U+FFFD      </code> .<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#string_for_json"><code dir="ltr" translate="no">        STRING       </code> (JSON)</a></td>
<td>Converts a JSON string to a SQL <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#string_array_for_json"><code dir="ltr" translate="no">        STRING_ARRAY       </code></a></td>
<td>Converts a JSON array of strings to a SQL <code dir="ltr" translate="no">       ARRAY&lt;STRING&gt;      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#string"><code dir="ltr" translate="no">        STRING       </code> (Timestamp)</a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to a <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_micros"><code dir="ltr" translate="no">        TIMESTAMP_MICROS       </code></a></td>
<td>Converts the number of microseconds since 1970-01-01 00:00:00 UTC to a <code dir="ltr" translate="no">       TIMESTAMP      </code> .<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_millis"><code dir="ltr" translate="no">        TIMESTAMP_MILLIS       </code></a></td>
<td>Converts the number of milliseconds since 1970-01-01 00:00:00 UTC to a <code dir="ltr" translate="no">       TIMESTAMP      </code> .<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_seconds"><code dir="ltr" translate="no">        TIMESTAMP_SECONDS       </code></a></td>
<td>Converts the number of seconds since 1970-01-01 00:00:00 UTC to a <code dir="ltr" translate="no">       TIMESTAMP      </code> .<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_base32"><code dir="ltr" translate="no">        TO_BASE32       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a base32-encoded <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_base64"><code dir="ltr" translate="no">        TO_BASE64       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a base64-encoded <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_code_points"><code dir="ltr" translate="no">        TO_CODE_POINTS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value into an array of extended ASCII code points.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_hex"><code dir="ltr" translate="no">        TO_HEX       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a hexadecimal <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/string_functions">String functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#to_json"><code dir="ltr" translate="no">        TO_JSON       </code></a></td>
<td>Converts a SQL value to a JSON value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#to_json_string"><code dir="ltr" translate="no">        TO_JSON_STRING       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       JSON      </code> value to a SQL JSON-formatted <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#unix_date"><code dir="ltr" translate="no">        UNIX_DATE       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       DATE      </code> value to the number of days since 1970-01-01.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/date_functions">Date functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#unix_micros"><code dir="ltr" translate="no">        UNIX_MICROS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to the number of microseconds since 1970-01-01 00:00:00 UTC.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#unix_millis"><code dir="ltr" translate="no">        UNIX_MILLIS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to the number of milliseconds since 1970-01-01 00:00:00 UTC.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#unix_seconds"><code dir="ltr" translate="no">        UNIX_SECONDS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to the number of seconds since 1970-01-01 00:00:00 UTC.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/timestamp_functions">Timestamp functions</a> .</td>
</tr>
</tbody>
</table>

## `     CAST    `

``` text
CAST(expression AS typename)
```

**Description**

Cast syntax is used in a query to indicate that the result type of an expression should be converted to some other type.

When using `  CAST  ` , a query can fail if GoogleSQL is unable to perform the cast. If you want to protect your queries from these types of errors, you can use [SAFE\_CAST](#safe_casting) .

Casts between supported types that don't successfully map from the original value to the target domain produce runtime errors. For example, casting `  BYTES  ` to `  STRING  ` where the byte sequence isn't valid UTF-8 results in a runtime error.

**Examples**

The following query results in `  "true"  ` if `  x  ` is `  1  ` , `  "false"  ` for any other non- `  NULL  ` value, and `  NULL  ` if `  x  ` is `  NULL  ` .

``` text
CAST(x=1 AS STRING)
```

### CAST AS ARRAY

``` text
CAST(expression AS ARRAY<element_type>)
```

**Description**

GoogleSQL supports [casting](#cast) to `  ARRAY  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  ARRAY  `

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       ARRAY      </code></td>
<td><code dir="ltr" translate="no">       ARRAY      </code></td>
<td>Must be the exact same array type.</td>
</tr>
</tbody>
</table>

### CAST AS BOOL

``` text
CAST(expression AS BOOL)
```

**Description**

GoogleSQL supports [casting](#cast) to `  BOOL  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  INT64  `
  - `  BOOL  `
  - `  STRING  `

**Conversion rules**

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>INT64</td>
<td><code dir="ltr" translate="no">       BOOL      </code></td>
<td>Returns <code dir="ltr" translate="no">       FALSE      </code> if <code dir="ltr" translate="no">       x      </code> is <code dir="ltr" translate="no">       0      </code> , <code dir="ltr" translate="no">       TRUE      </code> otherwise.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">       BOOL      </code></td>
<td>Returns <code dir="ltr" translate="no">       TRUE      </code> if <code dir="ltr" translate="no">       x      </code> is <code dir="ltr" translate="no">       "true"      </code> and <code dir="ltr" translate="no">       FALSE      </code> if <code dir="ltr" translate="no">       x      </code> is <code dir="ltr" translate="no">       "false"      </code><br />
All other values of <code dir="ltr" translate="no">       x      </code> are invalid and throw an error instead of casting to a boolean.<br />
A string is case-insensitive when converting to a boolean.</td>
</tr>
</tbody>
</table>

### CAST AS BYTES

``` text
CAST(expression AS BYTES)
```

**Description**

GoogleSQL supports [casting](#cast) to `  BYTES  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  BYTES  `
  - `  STRING  `
  - `  PROTO  `

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">       BYTES      </code></td>
<td>Strings are cast to bytes using UTF-8 encoding. For example, the string "©", when cast to bytes, would become a 2-byte sequence with the hex values C2 and A9.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td><code dir="ltr" translate="no">       BYTES      </code></td>
<td>Returns the proto2 wire format bytes of <code dir="ltr" translate="no">       x      </code> .</td>
</tr>
</tbody>
</table>

### CAST AS DATE

``` text
CAST(expression AS DATE)
```

**Description**

GoogleSQL supports [casting](#cast) to `  DATE  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  STRING  `
  - `  TIMESTAMP  `

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">       DATE      </code></td>
<td>When casting from string to date, the string must conform to the supported date literal format, and is independent of time zone. If the string expression is invalid or represents a date that's outside of the supported min/max range, then an error is produced.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td><code dir="ltr" translate="no">       DATE      </code></td>
<td>Casting from a timestamp to date effectively truncates the timestamp as of the default time zone.</td>
</tr>
</tbody>
</table>

### CAST AS ENUM

``` text
CAST(expression AS ENUM)
```

**Description**

GoogleSQL supports [casting](#cast) to `  ENUM  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  INT64  `
  - `  STRING  `
  - `  ENUM  `

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       ENUM      </code></td>
<td><code dir="ltr" translate="no">       ENUM      </code></td>
<td>Must have the same enum name.</td>
</tr>
</tbody>
</table>

### CAST AS Floating Point

``` text
CAST(expression AS FLOAT64)
```

``` text
CAST(expression AS FLOAT32)
```

**Description**

GoogleSQL supports [casting](#cast) to floating point types. The `  expression  ` parameter can represent an expression for these data types:

  - `  INT64  `
  - `  FLOAT32  `
  - `  FLOAT64  `
  - `  NUMERIC  `
  - `  STRING  `

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>INT64</td>
<td>Floating Point</td>
<td>Returns a close but potentially not exact floating point value.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td>Floating Point</td>
<td><code dir="ltr" translate="no">       NUMERIC      </code> will convert to the closest floating point number with a possible loss of precision.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Floating Point</td>
<td>Returns <code dir="ltr" translate="no">       x      </code> as a floating point value, interpreting it as having the same form as a valid floating point literal. Also supports casts from <code dir="ltr" translate="no">       "[+,-]inf"      </code> to <code dir="ltr" translate="no">       [,-]Infinity      </code> , <code dir="ltr" translate="no">       "[+,-]infinity"      </code> to <code dir="ltr" translate="no">       [,-]Infinity      </code> , and <code dir="ltr" translate="no">       "[+,-]nan"      </code> to <code dir="ltr" translate="no">       NaN      </code> . Conversions are case-insensitive.</td>
</tr>
</tbody>
</table>

### CAST AS INT64

``` text
CAST(expression AS INT64)
```

**Description**

GoogleSQL supports [casting](#cast) to integer types. The `  expression  ` parameter can represent an expression for these data types:

  - `  INT64  `
  - `  FLOAT32  `
  - `  FLOAT64  `
  - `  NUMERIC  `
  - `  ENUM  `
  - `  BOOL  `
  - `  STRING  `

**Conversion rules**

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Floating Point</td>
<td>INT64</td>
<td>Returns the closest integer value.<br />
Halfway cases such as 1.5 or -0.5 round away from zero.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       BOOL      </code></td>
<td>INT64</td>
<td>Returns <code dir="ltr" translate="no">       1      </code> if <code dir="ltr" translate="no">       x      </code> is <code dir="ltr" translate="no">       TRUE      </code> , <code dir="ltr" translate="no">       0      </code> otherwise.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>INT64</td>
<td>A hex string can be cast to an integer. For example, <code dir="ltr" translate="no">       0x123      </code> to <code dir="ltr" translate="no">       291      </code> or <code dir="ltr" translate="no">       -0x123      </code> to <code dir="ltr" translate="no">       -291      </code> .</td>
</tr>
</tbody>
</table>

**Examples**

If you are working with hex strings ( `  0x123  ` ), you can cast those strings as integers:

``` text
SELECT '0x123' as hex_value, CAST('0x123' as INT64) as hex_to_int;

/*-----------+------------+
 | hex_value | hex_to_int |
 +-----------+------------+
 | 0x123     | 291        |
 +-----------+------------*/
```

``` text
SELECT '-0x123' as hex_value, CAST('-0x123' as INT64) as hex_to_int;

/*-----------+------------+
 | hex_value | hex_to_int |
 +-----------+------------+
 | -0x123    | -291       |
 +-----------+------------*/
```

### CAST AS INTERVAL

``` text
CAST(expression AS INTERVAL)
```

**Description**

GoogleSQL supports [casting](#cast) to `  INTERVAL  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  STRING  `

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL      </code></td>
<td>When casting from string to interval, the string must conform to either <a href="https://en.wikipedia.org/wiki/ISO_8601#Durations">ISO 8601 Duration</a> standard or to interval literal format 'Y-M D H:M:S.F'. Partial interval literal formats are also accepted when they aren't ambiguous, for example 'H:M:S'. If the string expression is invalid or represents an interval that is outside of the supported min/max range, then an error is produced.</td>
</tr>
</tbody>
</table>

**Examples**

``` text
SELECT input, CAST(input AS INTERVAL) AS output
FROM UNNEST([
  '1-2 3 10:20:30.456',
  '1-2',
  '10:20:30',
  'P1Y2M3D',
  'PT10H20M30,456S'
]) input

/*--------------------+--------------------+
 | input              | output             |
 +--------------------+--------------------+
 | 1-2 3 10:20:30.456 | 1-2 3 10:20:30.456 |
 | 1-2                | 1-2 0 0:0:0        |
 | 10:20:30           | 0-0 0 10:20:30     |
 | P1Y2M3D            | 1-2 3 0:0:0        |
 | PT10H20M30,456S    | 0-0 0 10:20:30.456 |
 +--------------------+--------------------*/
```

### CAST AS NUMERIC

``` text
CAST(expression AS NUMERIC)
```

**Description**

GoogleSQL supports [casting](#cast) to `  NUMERIC  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  INT64  `
  - `  FLOAT32  `
  - `  FLOAT64  `
  - `  NUMERIC  `
  - `  STRING  `

**Conversion rules**

<table>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       Floating Point      </code></td>
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td>The floating point number will round <a href="https://en.wikipedia.org/wiki/Rounding#Round_half_away_from_zero">half away from zero</a> . Casting a <code dir="ltr" translate="no">       NaN      </code> , <code dir="ltr" translate="no">       +inf      </code> or <code dir="ltr" translate="no">       -inf      </code> will return an error. Casting a value outside the range of <code dir="ltr" translate="no">       NUMERIC      </code> returns an overflow error.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td>The numeric literal contained in the string must not exceed the maximum precision or range of the <code dir="ltr" translate="no">       NUMERIC      </code> type, or an error will occur. If the number of digits after the decimal point exceeds nine, then the resulting <code dir="ltr" translate="no">       NUMERIC      </code> value will round <a href="https://en.wikipedia.org/wiki/Rounding#Round_half_away_from_zero">half away from zero</a> . to have nine digits after the decimal point.</td>
</tr>
</tbody>
</table>

### CAST AS PROTO

``` text
CAST(expression AS PROTO)
```

**Description**

GoogleSQL supports [casting](#cast) to `  PROTO  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  STRING  `
  - `  BYTES  `
  - `  PROTO  `

**Conversion rules**

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td>Returns the protocol buffer that results from parsing from proto2 text format.<br />
Throws an error if parsing fails, e.g., if not all required fields are set.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       BYTES      </code></td>
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td>Returns the protocol buffer that results from parsing <code dir="ltr" translate="no">       x      </code> from the proto2 wire format.<br />
Throws an error if parsing fails, e.g., if not all required fields are set.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td>Must have the same protocol buffer name.</td>
</tr>
</tbody>
</table>

**Example**

This example references a protocol buffer called `  Award  ` .

``` text
message Award {
  required int32 year = 1;
  optional int32 month = 2;
  repeated Type type = 3;

  message Type {
    optional string award_name = 1;
    optional string category = 2;
  }
}
```

``` text
SELECT
  CAST(
    '''
    year: 2001
    month: 9
    type { award_name: 'Best Artist' category: 'Artist' }
    type { award_name: 'Best Album' category: 'Album' }
    '''
    AS googlesql.examples.music.Award)
  AS award_col

/*---------------------------------------------------------+
 | award_col                                               |
 +---------------------------------------------------------+
 | {                                                       |
 |   year: 2001                                            |
 |   month: 9                                              |
 |   type { award_name: "Best Artist" category: "Artist" } |
 |   type { award_name: "Best Album" category: "Album" }   |
 | }                                                       |
 +---------------------------------------------------------*/
```

### CAST AS STRING

``` text
CAST(expression AS STRING)
```

**Description**

GoogleSQL supports [casting](#cast) to `  STRING  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  INT64  `
  - `  FLOAT32  `
  - `  FLOAT64  `
  - `  NUMERIC  `
  - `  ENUM  `
  - `  BOOL  `
  - `  BYTES  `
  - `  PROTO  `
  - `  DATE  `
  - `  TIMESTAMP  `
  - `  INTERVAL  `
  - `  STRING  `

**Conversion rules**

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Floating Point</td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Returns an approximate string representation. A returned <code dir="ltr" translate="no">       NaN      </code> or <code dir="ltr" translate="no">       0      </code> will not be signed.<br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       BOOL      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Returns <code dir="ltr" translate="no">       "true"      </code> if <code dir="ltr" translate="no">       x      </code> is <code dir="ltr" translate="no">       TRUE      </code> , <code dir="ltr" translate="no">       "false"      </code> otherwise.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       BYTES      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Returns <code dir="ltr" translate="no">       x      </code> interpreted as a UTF-8 string.<br />
For example, the bytes literal <code dir="ltr" translate="no">       b'\xc2\xa9'      </code> , when cast to a string, is interpreted as UTF-8 and becomes the unicode character "©".<br />
An error occurs if <code dir="ltr" translate="no">       x      </code> isn't valid UTF-8.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       ENUM      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Returns the canonical enum value name of <code dir="ltr" translate="no">       x      </code> .<br />
If an enum value has multiple names (aliases), the canonical name/alias for that value is used.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Returns the proto2 text format representation of <code dir="ltr" translate="no">       x      </code> .</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       DATE      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Casting from a date type to a string is independent of time zone and is of the form <code dir="ltr" translate="no">       YYYY-MM-DD      </code> .</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>When casting from timestamp types to string, the timestamp is interpreted using the default time zone, America/Los_Angeles. The number of subsecond digits produced depends on the number of trailing zeroes in the subsecond part: the CAST function will truncate zero, three, or six digits.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       INTERVAL      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Casting from an interval to a string is of the form <code dir="ltr" translate="no">       Y-M D H:M:S      </code> .</td>
</tr>
</tbody>
</table>

**Examples**

``` text
SELECT CAST(CURRENT_DATE() AS STRING) AS current_date

/*---------------+
 | current_date  |
 +---------------+
 | 2021-03-09    |
 +---------------*/
```

``` text
SELECT CAST(INTERVAL 3 DAY AS STRING) AS interval_to_string

/*--------------------+
 | interval_to_string |
 +--------------------+
 | 0-0 3 0:0:0        |
 +--------------------*/
```

``` text
SELECT CAST(
  INTERVAL "1-2 3 4:5:6.789" YEAR TO SECOND
  AS STRING) AS interval_to_string

/*--------------------+
 | interval_to_string |
 +--------------------+
 | 1-2 3 4:5:6.789    |
 +--------------------*/
```

### CAST AS STRUCT

``` text
CAST(expression AS STRUCT)
```

**Description**

GoogleSQL supports [casting](#cast) to `  STRUCT  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  STRUCT  `

**Conversion rules**

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT      </code></td>
<td><code dir="ltr" translate="no">       STRUCT      </code></td>
<td>Allowed if the following conditions are met:<br />

<ol>
<li>The two structs have the same number of fields.</li>
<li>The original struct field types can be explicitly cast to the corresponding target struct field types (as defined by field order, not field name).</li>
</ol></td>
</tr>
</tbody>
</table>

### CAST AS TIMESTAMP

``` text
CAST(expression AS TIMESTAMP)
```

**Description**

GoogleSQL supports [casting](#cast) to `  TIMESTAMP  ` . The `  expression  ` parameter can represent an expression for these data types:

  - `  STRING  `
  - `  TIMESTAMP  `

**Conversion rules**

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From</th>
<th>To</th>
<th>Rule(s) when casting <code dir="ltr" translate="no">       x      </code></th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td>When casting from string to a timestamp, <code dir="ltr" translate="no">       string_expression      </code> must conform to the supported timestamp literal formats, or else a runtime error occurs. The <code dir="ltr" translate="no">       string_expression      </code> may itself contain a time zone.<br />
<br />
If there is a time zone in the <code dir="ltr" translate="no">       string_expression      </code> , that time zone is used for conversion, otherwise the default time zone, America/Los_Angeles, is used. If the string has fewer than six digits, then it's implicitly widened.<br />
<br />
An error is produced if the <code dir="ltr" translate="no">       string_expression      </code> is invalid, has more than six subsecond digits (i.e., precision greater than microseconds), or represents a time outside of the supported timestamp range.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       DATE      </code></td>
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td>Casting from a date to a timestamp interprets <code dir="ltr" translate="no">       date_expression      </code> as of midnight (start of the day) in the default time zone, America/Los_Angeles.</td>
</tr>
</tbody>
</table>

**Examples**

The following example casts a string-formatted timestamp as a timestamp:

``` text
SELECT CAST("2020-06-02 17:00:53.110+00:00" AS TIMESTAMP) AS as_timestamp

-- Results depend upon where this query was executed.
/*-------------------------+
 | as_timestamp            |
 +-------------------------+
 | 2020-06-03T00:00:53.11Z |
 +-------------------------*/
```

## `     SAFE_CAST    `

``` text
SAFE_CAST(expression AS typename)
```

**Description**

When using `  CAST  ` , a query can fail if GoogleSQL is unable to perform the cast. For example, the following query generates an error:

``` text
SELECT CAST("apple" AS INT64) AS not_a_number;
```

If you want to protect your queries from these types of errors, you can use `  SAFE_CAST  ` . `  SAFE_CAST  ` replaces runtime errors with `  NULL  ` s. However, during static analysis, impossible casts between two non-castable types still produce an error because the query is invalid.

``` text
SELECT SAFE_CAST("apple" AS INT64) AS not_a_number;

/*--------------+
 | not_a_number |
 +--------------+
 | NULL         |
 +--------------*/
```

If you are casting from bytes to strings, you can also use the function, [`  SAFE_CONVERT_BYTES_TO_STRING  `](/spanner/docs/reference/standard-sql/string_functions#safe_convert_bytes_to_string) . Any invalid UTF-8 characters are replaced with the unicode replacement character, `  U+FFFD  ` .
