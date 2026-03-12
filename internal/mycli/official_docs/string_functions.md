GoogleSQL for Spanner supports string functions. These string functions work on two different values: `  STRING  ` and `  BYTES  ` data types. `  STRING  ` values must be well-formed UTF-8.

Functions that return position values, such as [STRPOS](#strpos) , encode those positions as `  INT64  ` . The value `  1  ` refers to the first character (or byte), `  2  ` refers to the second, and so on. The value `  0  ` indicates an invalid position. When working on `  STRING  ` types, the returned positions refer to character positions.

All string comparisons are done byte-by-byte, without regard to Unicode canonical equivalence.

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
<td><a href="/spanner/docs/reference/standard-sql/string_functions#byte_length"><code dir="ltr" translate="no">        BYTE_LENGTH       </code></a></td>
<td>Gets the number of <code dir="ltr" translate="no">       BYTES      </code> in a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#char_length"><code dir="ltr" translate="no">        CHAR_LENGTH       </code></a></td>
<td>Gets the number of characters in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#character_length"><code dir="ltr" translate="no">        CHARACTER_LENGTH       </code></a></td>
<td>Synonym for <code dir="ltr" translate="no">       CHAR_LENGTH      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#code_points_to_bytes"><code dir="ltr" translate="no">        CODE_POINTS_TO_BYTES       </code></a></td>
<td>Converts an array of extended ASCII code points to a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#code_points_to_string"><code dir="ltr" translate="no">        CODE_POINTS_TO_STRING       </code></a></td>
<td>Converts an array of extended ASCII code points to a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#concat"><code dir="ltr" translate="no">        CONCAT       </code></a></td>
<td>Concatenates one or more <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> values into a single result.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#ends_with"><code dir="ltr" translate="no">        ENDS_WITH       </code></a></td>
<td>Checks if a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value is the suffix of another value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#format_string"><code dir="ltr" translate="no">        FORMAT       </code></a></td>
<td>Formats data and produces the results as a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_base32"><code dir="ltr" translate="no">        FROM_BASE32       </code></a></td>
<td>Converts a base32-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_base64"><code dir="ltr" translate="no">        FROM_BASE64       </code></a></td>
<td>Converts a base64-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_hex"><code dir="ltr" translate="no">        FROM_HEX       </code></a></td>
<td>Converts a hexadecimal-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_string"><code dir="ltr" translate="no">        LAX_STRING       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       STRING      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#lcase"><code dir="ltr" translate="no">        LCASE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       LOWER      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#length"><code dir="ltr" translate="no">        LENGTH       </code></a></td>
<td>Gets the length of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#lower"><code dir="ltr" translate="no">        LOWER       </code></a></td>
<td>Formats alphabetic characters in a <code dir="ltr" translate="no">       STRING      </code> value as lowercase.<br />
<br />
Formats ASCII characters in a <code dir="ltr" translate="no">       BYTES      </code> value as lowercase.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#lpad"><code dir="ltr" translate="no">        LPAD       </code></a></td>
<td>Prepends a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value with a pattern.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#ltrim"><code dir="ltr" translate="no">        LTRIM       </code></a></td>
<td>Identical to the <code dir="ltr" translate="no">       TRIM      </code> function, but only removes leading characters.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#normalize"><code dir="ltr" translate="no">        NORMALIZE       </code></a></td>
<td>Case-sensitively normalizes the characters in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#normalize_and_casefold"><code dir="ltr" translate="no">        NORMALIZE_AND_CASEFOLD       </code></a></td>
<td>Case-insensitively normalizes the characters in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#octet_length"><code dir="ltr" translate="no">        OCTET_LENGTH       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       BYTE_LENGTH      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_contains"><code dir="ltr" translate="no">        REGEXP_CONTAINS       </code></a></td>
<td>Checks if a value is a partial match for a regular expression.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_extract"><code dir="ltr" translate="no">        REGEXP_EXTRACT       </code></a></td>
<td>Produces a substring that matches a regular expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_extract_all"><code dir="ltr" translate="no">        REGEXP_EXTRACT_ALL       </code></a></td>
<td>Produces an array of all substrings that match a regular expression.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_replace"><code dir="ltr" translate="no">        REGEXP_REPLACE       </code></a></td>
<td>Produces a <code dir="ltr" translate="no">       STRING      </code> value where all substrings that match a regular expression are replaced with a specified value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#repeat"><code dir="ltr" translate="no">        REPEAT       </code></a></td>
<td>Produces a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value that consists of an original value, repeated.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#replace"><code dir="ltr" translate="no">        REPLACE       </code></a></td>
<td>Replaces all occurrences of a pattern with another pattern in a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#reverse"><code dir="ltr" translate="no">        REVERSE       </code></a></td>
<td>Reverses a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#rpad"><code dir="ltr" translate="no">        RPAD       </code></a></td>
<td>Appends a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value with a pattern.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#rtrim"><code dir="ltr" translate="no">        RTRIM       </code></a></td>
<td>Identical to the <code dir="ltr" translate="no">       TRIM      </code> function, but only removes trailing characters.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#safe_convert_bytes_to_string"><code dir="ltr" translate="no">        SAFE_CONVERT_BYTES_TO_STRING       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a <code dir="ltr" translate="no">       STRING      </code> value and replace any invalid UTF-8 characters with the Unicode replacement character, <code dir="ltr" translate="no">       U+FFFD      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#soundex"><code dir="ltr" translate="no">        SOUNDEX       </code></a></td>
<td>Gets the Soundex codes for words in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#split"><code dir="ltr" translate="no">        SPLIT       </code></a></td>
<td>Splits a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using a delimiter.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#split_substr"><code dir="ltr" translate="no">        SPLIT_SUBSTR       </code></a></td>
<td>Returns the substring from an input string that's determined by a delimiter, a location that indicates the first split of the substring to return, and the number of splits to include.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#starts_with"><code dir="ltr" translate="no">        STARTS_WITH       </code></a></td>
<td>Checks if a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value is a prefix of another value.</td>
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
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#string_agg"><code dir="ltr" translate="no">        STRING_AGG       </code></a></td>
<td>Concatenates non- <code dir="ltr" translate="no">       NULL      </code> <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> values.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/aggregate_functions">Aggregate functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#strpos"><code dir="ltr" translate="no">        STRPOS       </code></a></td>
<td>Finds the position of the first occurrence of a subvalue inside another value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#substr"><code dir="ltr" translate="no">        SUBSTR       </code></a></td>
<td>Gets a portion of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#substring"><code dir="ltr" translate="no">        SUBSTRING       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       SUBSTR      </code></td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_base32"><code dir="ltr" translate="no">        TO_BASE32       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a base32-encoded <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_base64"><code dir="ltr" translate="no">        TO_BASE64       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a base64-encoded <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_code_points"><code dir="ltr" translate="no">        TO_CODE_POINTS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value into an array of extended ASCII code points.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_hex"><code dir="ltr" translate="no">        TO_HEX       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a hexadecimal <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#trim"><code dir="ltr" translate="no">        TRIM       </code></a></td>
<td>Removes the specified leading and trailing Unicode code points or bytes from a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#ucase"><code dir="ltr" translate="no">        UCASE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       UPPER      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#upper"><code dir="ltr" translate="no">        UPPER       </code></a></td>
<td>Formats alphabetic characters in a <code dir="ltr" translate="no">       STRING      </code> value as uppercase.<br />
<br />
Formats ASCII characters in a <code dir="ltr" translate="no">       BYTES      </code> value as uppercase.</td>
</tr>
</tbody>
</table>

## `     BYTE_LENGTH    `

``` text
BYTE_LENGTH(value)
```

**Description**

Gets the number of `  BYTES  ` in a `  STRING  ` or `  BYTES  ` value, regardless of whether the value is a `  STRING  ` or `  BYTES  ` type.

**Return type**

`  INT64  `

**Examples**

``` text
SELECT BYTE_LENGTH('абвгд') AS string_example;

/*----------------+
 | string_example |
 +----------------+
 | 10             |
 +----------------*/
```

``` text
SELECT BYTE_LENGTH(b'абвгд') AS bytes_example;

/*----------------+
 | bytes_example  |
 +----------------+
 | 10             |
 +----------------*/
```

## `     CHAR_LENGTH    `

``` text
CHAR_LENGTH(value)
```

**Description**

Gets the number of characters in a `  STRING  ` value.

**Return type**

`  INT64  `

**Examples**

``` text
SELECT CHAR_LENGTH('абвгд') AS char_length;

/*-------------+
 | char_length |
 +-------------+
 | 5           |
 +------------ */
```

## `     CHARACTER_LENGTH    `

``` text
CHARACTER_LENGTH(value)
```

**Description**

Synonym for [CHAR\_LENGTH](#char_length) .

**Return type**

`  INT64  `

**Examples**

``` text
SELECT
  'абвгд' AS characters,
  CHARACTER_LENGTH('абвгд') AS char_length_example

/*------------+---------------------+
 | characters | char_length_example |
 +------------+---------------------+
 | абвгд      |                   5 |
 +------------+---------------------*/
```

## `     CODE_POINTS_TO_BYTES    `

``` text
CODE_POINTS_TO_BYTES(ascii_code_points)
```

**Description**

Takes an array of extended ASCII [code points](https://en.wikipedia.org/wiki/Code_point) as `  ARRAY<INT64>  ` and returns `  BYTES  ` .

To convert from `  BYTES  ` to an array of code points, see [TO\_CODE\_POINTS](#to_code_points) .

**Return type**

`  BYTES  `

**Examples**

The following is a basic example using `  CODE_POINTS_TO_BYTES  ` .

``` text
SELECT CODE_POINTS_TO_BYTES([65, 98, 67, 100]) AS bytes;

-- Note that the result of CODE_POINTS_TO_BYTES is of type BYTES, displayed as a base64-encoded string.
-- In BYTES format, b'AbCd' is the result.
/*----------+
 | bytes    |
 +----------+
 | QWJDZA== |
 +----------*/
```

The following example uses a rotate-by-13 places (ROT13) algorithm to encode a string.

``` text
SELECT CODE_POINTS_TO_BYTES(ARRAY_AGG(
  (SELECT
      CASE
        WHEN chr BETWEEN b'a' and b'z'
          THEN TO_CODE_POINTS(b'a')[offset(0)] +
            MOD(code+13-TO_CODE_POINTS(b'a')[offset(0)],26)
        WHEN chr BETWEEN b'A' and b'Z'
          THEN TO_CODE_POINTS(b'A')[offset(0)] +
            MOD(code+13-TO_CODE_POINTS(b'A')[offset(0)],26)
        ELSE code
      END
   FROM
     (SELECT code, CODE_POINTS_TO_BYTES([code]) chr)
  ) ORDER BY OFFSET)) AS encoded_string
FROM UNNEST(TO_CODE_POINTS(b'Test String!')) code WITH OFFSET;

-- Note that the result of CODE_POINTS_TO_BYTES is of type BYTES, displayed as a base64-encoded string.
-- In BYTES format, b'Grfg Fgevat!' is the result.
/*------------------+
 | encoded_string   |
 +------------------+
 | R3JmZyBGZ2V2YXQh |
 +------------------*/
```

## `     CODE_POINTS_TO_STRING    `

``` text
CODE_POINTS_TO_STRING(unicode_code_points)
```

**Description**

Takes an array of Unicode [code points](https://en.wikipedia.org/wiki/Code_point) as `  ARRAY<INT64>  ` and returns a `  STRING  ` .

To convert from a string to an array of code points, see [TO\_CODE\_POINTS](#to_code_points) .

**Return type**

`  STRING  `

**Examples**

The following are basic examples using `  CODE_POINTS_TO_STRING  ` .

``` text
SELECT CODE_POINTS_TO_STRING([65, 255, 513, 1024]) AS string;

/*--------+
 | string |
 +--------+
 | AÿȁЀ   |
 +--------*/
```

``` text
SELECT CODE_POINTS_TO_STRING([97, 0, 0xF9B5]) AS string;

/*--------+
 | string |
 +--------+
 | a例    |
 +--------*/
```

``` text
SELECT CODE_POINTS_TO_STRING([65, 255, NULL, 1024]) AS string;

/*--------+
 | string |
 +--------+
 | NULL   |
 +--------*/
```

The following example computes the frequency of letters in a set of words.

``` text
WITH Words AS (
  SELECT word
  FROM UNNEST(['foo', 'bar', 'baz', 'giraffe', 'llama']) AS word
)
SELECT
  CODE_POINTS_TO_STRING([code_point]) AS letter,
  COUNT(*) AS letter_count
FROM Words,
  UNNEST(TO_CODE_POINTS(word)) AS code_point
GROUP BY 1
ORDER BY 2 DESC;

/*--------+--------------+
 | letter | letter_count |
 +--------+--------------+
 | a      | 5            |
 | f      | 3            |
 | r      | 2            |
 | b      | 2            |
 | l      | 2            |
 | o      | 2            |
 | g      | 1            |
 | z      | 1            |
 | e      | 1            |
 | m      | 1            |
 | i      | 1            |
 +--------+--------------*/
```

## `     CONCAT    `

``` text
CONCAT(value1[, ...])
```

**Description**

Concatenates one or more `  STRING  ` or `  BYTE  ` values into a single result.

The function returns `  NULL  ` if any input argument is `  NULL  ` .

**Note:** You can also use the [|| concatenation operator](/spanner/docs/reference/standard-sql/operators) to concatenate values into a string.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT CONCAT('T.P.', ' ', 'Bar') as author;

/*---------------------+
 | author              |
 +---------------------+
 | T.P. Bar            |
 +---------------------*/
```

``` text
With Employees AS
  (SELECT
    'John' AS first_name,
    'Doe' AS last_name
  UNION ALL
  SELECT
    'Jane' AS first_name,
    'Smith' AS last_name
  UNION ALL
  SELECT
    'Joe' AS first_name,
    'Jackson' AS last_name)

SELECT
  CONCAT(first_name, ' ', last_name)
  AS full_name
FROM Employees;

/*---------------------+
 | full_name           |
 +---------------------+
 | John Doe            |
 | Jane Smith          |
 | Joe Jackson         |
 +---------------------*/
```

## `     ENDS_WITH    `

``` text
ENDS_WITH(value, suffix)
```

**Description**

Takes two `  STRING  ` or `  BYTES  ` values. Returns `  TRUE  ` if `  suffix  ` is a suffix of `  value  ` .

**Return type**

`  BOOL  `

**Examples**

``` text
SELECT ENDS_WITH('apple', 'e') as example

/*---------+
 | example |
 +---------+
 |    True |
 +---------*/
```

## `     FORMAT    `

``` text
FORMAT(format_string_expression, data_type_expression[, ...])
```

**Description**

`  FORMAT  ` formats a data type expression as a string.

  - `  format_string_expression  ` : Can contain zero or more [format specifiers](#format_specifiers) . Each format specifier is introduced by the `  %  ` symbol, and must map to one or more of the remaining arguments. In general, this is a one-to-one mapping, except when the `  *  ` specifier is present. For example, `  %.*i  ` maps to two arguments—a length argument and a signed integer argument. If the number of arguments related to the format specifiers isn't the same as the number of arguments, an error occurs.
  - `  data_type_expression  ` : The value to format as a string. This can be any GoogleSQL data type.

**Return type**

`  STRING  `

**Examples**

<table>
<thead>
<tr class="header">
<th>Description</th>
<th>Statement</th>
<th>Result</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Simple integer</td>
<td>FORMAT('%d', 10)</td>
<td>10</td>
</tr>
<tr class="even">
<td>Integer with left blank padding</td>
<td>FORMAT('|%10d|', 11)</td>
<td>| 11|</td>
</tr>
<tr class="odd">
<td>Integer with left zero padding</td>
<td>FORMAT('+%010d+', 12)</td>
<td>+0000000012+</td>
</tr>
<tr class="even">
<td>Integer with commas</td>
<td>FORMAT("%'d", 123456789)</td>
<td>123,456,789</td>
</tr>
<tr class="odd">
<td>STRING</td>
<td>FORMAT('-%s-', 'abcd efg')</td>
<td>-abcd efg-</td>
</tr>
<tr class="even">
<td>FLOAT64</td>
<td>FORMAT('%f %E', 1.1, 2.2)</td>
<td>1.100000 2.200000E+00</td>
</tr>
<tr class="odd">
<td>DATE</td>
<td>FORMAT('%t', date '2015-09-01')</td>
<td>2015-09-01</td>
</tr>
<tr class="even">
<td>TIMESTAMP</td>
<td>FORMAT('%t', timestamp '2015-09-01 12:34:56 America/Los_Angeles')</td>
<td>2015‑09‑01 19:34:56+00</td>
</tr>
</tbody>
</table>

The `  FORMAT()  ` function doesn't provide fully customizable formatting for all types and values, nor formatting that's sensitive to locale.

If custom formatting is necessary for a type, you must first format it using type-specific format functions, such as `  FORMAT_DATE()  ` or `  FORMAT_TIMESTAMP()  ` . For example:

``` text
SELECT FORMAT('date: %s!', FORMAT_DATE('%B %d, %Y', date '2015-01-02'));
```

Returns

``` text
date: January 02, 2015!
```

#### Supported format specifiers

``` text
%[flags][width][.precision]specifier
```

A [format specifier](#format_specifier_list) adds formatting when casting a value to a string. It can optionally contain these sub-specifiers:

  - [Flags](#flags)
  - [Width](#width)
  - [Precision](#precision)

Additional information about format specifiers:

  - [%g and %G behavior](#g_and_g_behavior)
  - [%p and %P behavior](#p_and_p_behavior)
  - [%t and %T behavior](#t_and_t_behavior)
  - [Error conditions](#error_format_specifiers)
  - [NULL argument handling](#null_format_specifiers)
  - [Additional semantic rules](#rules_format_specifiers)

##### Format specifiers

<table>
<colgroup>
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
</colgroup>
<tbody>
<tr class="odd">
<td>Specifier</td>
<td>Description</td>
<td>Examples</td>
<td>Types</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       d      </code> or <code dir="ltr" translate="no">       i      </code></td>
<td>Decimal integer</td>
<td>392</td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       o      </code></td>
<td>Octal<br />
<br />
Note: If an <code dir="ltr" translate="no">       INT64      </code> value is negative, an error is produced.</td>
<td>610</td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       x      </code></td>
<td>Hexadecimal integer<br />
<br />
Note: If an <code dir="ltr" translate="no">       INT64      </code> value is negative, an error is produced.</td>
<td>7fa</td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       X      </code></td>
<td>Hexadecimal integer (uppercase)<br />
<br />
Note: If an <code dir="ltr" translate="no">       INT64      </code> value is negative, an error is produced.</td>
<td>7FA</td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       f      </code></td>
<td>Decimal notation, in [-](integer part).(fractional part) for finite values, and in lowercase for non-finite values</td>
<td>392.650000<br />
inf<br />
nan</td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       F      </code></td>
<td>Decimal notation, in [-](integer part).(fractional part) for finite values, and in uppercase for non-finite values</td>
<td>392.650000<br />
INF<br />
NAN</td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       e      </code></td>
<td>Scientific notation (mantissa/exponent), lowercase</td>
<td>3.926500e+02<br />
inf<br />
nan</td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       E      </code></td>
<td>Scientific notation (mantissa/exponent), uppercase</td>
<td>3.926500E+02<br />
INF<br />
NAN</td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       g      </code></td>
<td>Either decimal notation or scientific notation, depending on the input value's exponent and the specified precision. Lowercase. See <a href="#g_and_g_behavior">%g and %G behavior</a> for details.</td>
<td>392.65<br />
3.9265e+07<br />
inf<br />
nan</td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       G      </code></td>
<td>Either decimal notation or scientific notation, depending on the input value's exponent and the specified precision. Uppercase. See <a href="#g_and_g_behavior">%g and %G behavior</a> for details.</td>
<td>392.65<br />
3.9265E+07<br />
INF<br />
NAN</td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       p      </code></td>
<td>Produces a one-line printable string representing a protocol buffer or JSON. See <a href="#p_and_p_behavior">%p and %P behavior</a> .</td>
<td><pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>year: 2019 month: 10</code></pre>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>{&quot;month&quot;:10,&quot;year&quot;:2019}</code></pre></td>
<td><code dir="ltr" translate="no">        JSON       </code><br />
<code dir="ltr" translate="no">        PROTO       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       P      </code></td>
<td>Produces a multi-line printable string representing a protocol buffer or JSON. See <a href="#p_and_p_behavior">%p and %P behavior</a> .</td>
<td><pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>year: 2019
month: 10</code></pre>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>{
  &quot;month&quot;: 10,
  &quot;year&quot;: 2019
}</code></pre></td>
<td><code dir="ltr" translate="no">        JSON       </code><br />
<code dir="ltr" translate="no">        PROTO       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       s      </code></td>
<td>String of characters</td>
<td>sample</td>
<td><code dir="ltr" translate="no">        STRING       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       t      </code></td>
<td>Returns a printable string representing the value. Often looks similar to casting the argument to <code dir="ltr" translate="no">       STRING      </code> . See <a href="#t_and_t_behavior">%t and %T behavior</a> .</td>
<td>sample<br />
2014‑01‑01</td>
<td>Any type</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       T      </code></td>
<td>Produces a string that's a valid GoogleSQL constant with a similar type to the value's type (maybe wider, or maybe string). See <a href="#t_and_t_behavior">%t and %T behavior</a> .</td>
<td>'sample'<br />
b'bytes sample'<br />
1234<br />
2.3<br />
date '2014‑01‑01'</td>
<td>Any type</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %      </code></td>
<td>'%%' produces a single '%'</td>
<td>%</td>
<td>n/a</td>
</tr>
</tbody>
</table>

The format specifier can optionally contain the sub-specifiers identified above in the specifier prototype.

These sub-specifiers must comply with the following specifications.

##### Flags

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<tbody>
<tr class="odd">
<td>Flags</td>
<td>Description</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       -      </code></td>
<td>Left-justify within the given field width; Right justification is the default (see width sub-specifier)</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       +      </code></td>
<td>Forces to precede the result with a plus or minus sign ( <code dir="ltr" translate="no">       +      </code> or <code dir="ltr" translate="no">       -      </code> ) even for positive numbers. By default, only negative numbers are preceded with a <code dir="ltr" translate="no">       -      </code> sign</td>
</tr>
<tr class="even">
<td>&lt;space&gt;</td>
<td>If no sign is going to be written, a blank space is inserted before the value</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       #      </code></td>
<td><ul>
<li>For `%o`, `%x`, and `%X`, this flag means to precede the value with 0, 0x or 0X respectively for values different than zero.</li>
<li>For `%f`, `%F`, `%e`, and `%E`, this flag means to add the decimal point even when there is no fractional part, unless the value is non-finite.</li>
<li>For `%g` and `%G`, this flag means to add the decimal point even when there is no fractional part unless the value is non-finite, and never remove the trailing zeros after the decimal point.</li>
</ul></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       0      </code></td>
<td>Left-pads the number with zeroes (0) instead of spaces when padding is specified (see width sub-specifier)</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       '      </code></td>
<td><p>Formats integers using the appropriating grouping character. For example:</p>
<ul>
<li><code dir="ltr" translate="no">         FORMAT("%'d", 12345678)        </code> returns <code dir="ltr" translate="no">         12,345,678        </code></li>
<li><code dir="ltr" translate="no">         FORMAT("%'x", 12345678)        </code> returns <code dir="ltr" translate="no">         bc:614e        </code></li>
<li><code dir="ltr" translate="no">         FORMAT("%'o", 55555)        </code> returns <code dir="ltr" translate="no">         15,4403        </code></li>
</ul></td>
</tr>
</tbody>
</table>

Flags may be specified in any order. Duplicate flags aren't an error. When flags aren't relevant for some element type, they are ignored.

##### Width

<table>
<tbody>
<tr class="odd">
<td>Width</td>
<td>Description</td>
</tr>
<tr class="even">
<td>&lt;number&gt;</td>
<td>Minimum number of characters to be printed. If the value to be printed is shorter than this number, the result is padded with blank spaces. The value isn't truncated even if the result is larger</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       *      </code></td>
<td>The width isn't specified in the format string, but as an additional integer value argument preceding the argument that has to be formatted</td>
</tr>
</tbody>
</table>

##### Precision

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<tbody>
<tr class="odd">
<td>Precision</td>
<td>Description</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       .      </code> &lt;number&gt;</td>
<td><ul>
<li>For integer specifiers `%d`, `%i`, `%o`, `%u`, `%x`, and `%X`: precision specifies the minimum number of digits to be written. If the value to be written is shorter than this number, the result is padded with trailing zeros. The value isn't truncated even if the result is longer. A precision of 0 means that no character is written for the value 0.</li>
<li>For specifiers `%a`, `%A`, `%e`, `%E`, `%f`, and `%F`: this is the number of digits to be printed after the decimal point. The default value is 6.</li>
<li>For specifiers `%g` and `%G`: this is the number of significant digits to be printed, before the removal of the trailing zeros after the decimal point. The default value is 6.</li>
</ul></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       .*      </code></td>
<td>The precision isn't specified in the format string, but as an additional integer value argument preceding the argument that has to be formatted</td>
</tr>
</tbody>
</table>

##### %g and %G behavior

The `  %g  ` and `  %G  ` format specifiers choose either the decimal notation (like the `  %f  ` and `  %F  ` specifiers) or the scientific notation (like the `  %e  ` and `  %E  ` specifiers), depending on the input value's exponent and the specified [precision](#precision) .

Let p stand for the specified [precision](#precision) (defaults to 6; 1 if the specified precision is less than 1). The input value is first converted to scientific notation with precision = (p - 1). If the resulting exponent part x is less than -4 or no less than p, the scientific notation with precision = (p - 1) is used; otherwise the decimal notation with precision = (p - 1 - x) is used.

Unless [`  #  ` flag](#flags) is present, the trailing zeros after the decimal point are removed, and the decimal point is also removed if there is no digit after it.

##### %p and %P behavior

The `  %p  ` format specifier produces a one-line printable string. The `  %P  ` format specifier produces a multi-line printable string. You can use these format specifiers with the following data types:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<tbody>
<tr class="odd">
<td><strong>Type</strong></td>
<td><strong>%p</strong></td>
<td><strong>%P</strong></td>
</tr>
<tr class="even">
<td>PROTO</td>
<td><p>PROTO input:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>message ReleaseDate {
 required int32 year = 1 [default=2019];
 required int32 month = 2 [default=10];
}</code></pre>
<p>Produces a one-line printable string representing a protocol buffer:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>year: 2019 month: 10</code></pre></td>
<td><p>PROTO input:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>message ReleaseDate {
 required int32 year = 1 [default=2019];
 required int32 month = 2 [default=10];
}</code></pre>
<p>Produces a multi-line printable string representing a protocol buffer:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>year: 2019
month: 10</code></pre></td>
</tr>
<tr class="odd">
<td>JSON</td>
<td><p>JSON input:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>JSON &#39;
{
  &quot;month&quot;: 10,
  &quot;year&quot;: 2019
}
&#39;</code></pre>
<p>Produces a one-line printable string representing JSON:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>{&quot;month&quot;:10,&quot;year&quot;:2019}</code></pre></td>
<td><p>JSON input:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>JSON &#39;
{
  &quot;month&quot;: 10,
  &quot;year&quot;: 2019
}
&#39;</code></pre>
<p>Produces a multi-line printable string representing JSON:</p>
<pre class="text" dir="ltr" data-is-upgraded="" translate="no"><code>{
  &quot;month&quot;: 10,
  &quot;year&quot;: 2019
}</code></pre></td>
</tr>
</tbody>
</table>

##### %t and %T behavior

The `  %t  ` and `  %T  ` format specifiers are defined for all types. The [width](#width) , [precision](#precision) , and [flags](#flags) act as they do for `  %s  ` : the [width](#width) is the minimum width and the `  STRING  ` will be padded to that size, and [precision](#precision) is the maximum width of content to show and the `  STRING  ` will be truncated to that size, prior to padding to width.

The `  %t  ` specifier is always meant to be a readable form of the value.

The `  %T  ` specifier is always a valid SQL literal of a similar type, such as a wider numeric type. The literal will not include casts or a type name, except for the special case of non-finite floating point values.

The `  STRING  ` is formatted as follows:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<tbody>
<tr class="odd">
<td><strong>Type</strong></td>
<td><strong>%t</strong></td>
<td><strong>%T</strong></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NULL      </code> of any type</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">        INT64       </code><br />
</td>
<td>123</td>
<td>123</td>
</tr>
<tr class="even">
<td>NUMERIC</td>
<td>123.0 <em>(always with .0)</em></td>
<td>NUMERIC "123.0"</td>
</tr>
<tr class="odd">
<td>FLOAT32, FLOAT64</td>
<td>123.0 <em>(always with .0)</em><br />
123e+10<br />
<code dir="ltr" translate="no">       inf      </code><br />
<code dir="ltr" translate="no">       -inf      </code><br />
<code dir="ltr" translate="no">       NaN      </code></td>
<td>123.0 <em>(always with .0)</em><br />
123e+10<br />
CAST("inf" AS &lt;type&gt;)<br />
CAST("-inf" AS &lt;type&gt;)<br />
CAST("nan" AS &lt;type&gt;)</td>
</tr>
<tr class="even">
<td>STRING</td>
<td>unquoted string value</td>
<td>quoted string literal</td>
</tr>
<tr class="odd">
<td>BYTES</td>
<td>unquoted escaped bytes<br />
e.g., abc\x01\x02</td>
<td>quoted bytes literal<br />
e.g., b"abc\x01\x02"</td>
</tr>
<tr class="even">
<td>BOOL</td>
<td>boolean value</td>
<td>boolean value</td>
</tr>
<tr class="odd">
<td>ENUM</td>
<td>EnumName</td>
<td>"EnumName"</td>
</tr>
<tr class="even">
<td>DATE</td>
<td>2011-02-03</td>
<td>DATE "2011-02-03"</td>
</tr>
<tr class="odd">
<td>TIMESTAMP</td>
<td>2011-02-03 04:05:06+00</td>
<td>TIMESTAMP "2011-02-03 04:05:06+00"</td>
</tr>
<tr class="even">
<td>INTERVAL</td>
<td>1-2 3 4:5:6.789</td>
<td>INTERVAL "1-2 3 4:5:6.789" YEAR TO SECOND</td>
</tr>
<tr class="odd">
<td>PROTO</td>
<td>one-line printable string representing a protocol buffer.</td>
<td>quoted string literal with one-line printable string representing a protocol buffer.</td>
</tr>
<tr class="even">
<td>ARRAY</td>
<td>[value, value, ...]<br />
where values are formatted with %t</td>
<td>[value, value, ...]<br />
where values are formatted with %T</td>
</tr>
<tr class="odd">
<td>JSON</td>
<td>one-line printable string representing JSON.<br />

<pre class="text" dir="ltr" data-is-upgraded="" data-syntax="JSON" translate="no"><code>{&quot;name&quot;:&quot;apple&quot;,&quot;stock&quot;:3}</code></pre></td>
<td>one-line printable string representing a JSON literal.<br />

<pre class="text" dir="ltr" data-is-upgraded="" data-syntax="SQL" translate="no"><code>JSON &#39;{&quot;name&quot;:&quot;apple&quot;,&quot;stock&quot;:3}&#39;</code></pre></td>
</tr>
</tbody>
</table>

##### Error conditions

If a format specifier is invalid, or isn't compatible with the related argument type, or the wrong number or arguments are provided, then an error is produced. For example, the following `  <format_string>  ` expressions are invalid:

``` text
FORMAT('%s', 1)
```

``` text
FORMAT('%')
```

##### NULL argument handling

A `  NULL  ` format string results in a `  NULL  ` output `  STRING  ` . Any other arguments are ignored in this case.

The function generally produces a `  NULL  ` value if a `  NULL  ` argument is present. For example, `  FORMAT('%i', NULL_expression)  ` produces a `  NULL STRING  ` as output.

However, there are some exceptions: if the format specifier is %t or %T (both of which produce `  STRING  ` s that effectively match CAST and literal value semantics), a `  NULL  ` value produces 'NULL' (without the quotes) in the result `  STRING  ` . For example, the function:

``` text
FORMAT('00-%t-00', NULL_expression);
```

Returns

``` text
00-NULL-00
```

##### Additional semantic rules

`  FLOAT64  ` and `  FLOAT32  ` values can be `  +/-inf  ` or `  NaN  ` . When an argument has one of those values, the result of the format specifiers `  %f  ` , `  %F  ` , `  %e  ` , `  %E  ` , `  %g  ` , `  %G  ` , and `  %t  ` are `  inf  ` , `  -inf  ` , or `  nan  ` (or the same in uppercase) as appropriate. This is consistent with how GoogleSQL casts these values to `  STRING  ` . For `  %T  ` , GoogleSQL returns quoted strings for `  FLOAT64  ` values that don't have non-string literal representations.

## `     FROM_BASE32    `

``` text
FROM_BASE32(string_expr)
```

**Description**

Converts the base32-encoded input `  string_expr  ` into `  BYTES  ` format. To convert `  BYTES  ` to a base32-encoded `  STRING  ` , use [TO\_BASE32](#to_base32) .

**Return type**

`  BYTES  `

**Example**

``` text
SELECT FROM_BASE32('MFRGGZDF74======') AS byte_data;

-- Note that the result of FROM_BASE32 is of type BYTES, displayed as a base64-encoded string.
/*-----------+
 | byte_data |
 +-----------+
 | YWJjZGX/  |
 +-----------*/
```

## `     FROM_BASE64    `

``` text
FROM_BASE64(string_expr)
```

**Description**

Converts the base64-encoded input `  string_expr  ` into `  BYTES  ` format. To convert `  BYTES  ` to a base64-encoded `  STRING  ` , use [TO\_BASE64](#to_base64) .

There are several base64 encodings in common use that vary in exactly which alphabet of 65 ASCII characters are used to encode the 64 digits and padding. See [RFC 4648](https://tools.ietf.org/html/rfc4648#section-4) for details. This function expects the alphabet `  [A-Za-z0-9+/=]  ` .

**Return type**

`  BYTES  `

**Example**

``` text
SELECT FROM_BASE64('/+A=') AS byte_data;

-- Note that the result of FROM_BASE64 is of type BYTES, displayed as a base64-encoded string.
/*-----------+
 | byte_data |
 +-----------+
 | /+A=      |
 +-----------*/
```

To work with an encoding using a different base64 alphabet, you might need to compose `  FROM_BASE64  ` with the `  REPLACE  ` function. For instance, the `  base64url  ` url-safe and filename-safe encoding commonly used in web programming uses `  -_=  ` as the last characters rather than `  +/=  ` . To decode a `  base64url  ` -encoded string, replace `  -  ` and `  _  ` with `  +  ` and `  /  ` respectively.

``` text
SELECT FROM_BASE64(REPLACE(REPLACE('_-A=', '-', '+'), '_', '/')) AS binary;

-- Note that the result of FROM_BASE64 is of type BYTES, displayed as a base64-encoded string.
/*--------+
 | binary |
 +--------+
 | /+A=   |
 +--------*/
```

## `     FROM_HEX    `

``` text
FROM_HEX(string)
```

**Description**

Converts a hexadecimal-encoded `  STRING  ` into `  BYTES  ` format. Returns an error if the input `  STRING  ` contains characters outside the range `  (0..9, A..F, a..f)  ` . The lettercase of the characters doesn't matter. If the input `  STRING  ` has an odd number of characters, the function acts as if the input has an additional leading `  0  ` . To convert `  BYTES  ` to a hexadecimal-encoded `  STRING  ` , use [TO\_HEX](#to_hex) .

**Return type**

`  BYTES  `

**Example**

``` text
WITH Input AS (
  SELECT '00010203aaeeefff' AS hex_str UNION ALL
  SELECT '0AF' UNION ALL
  SELECT '666f6f626172'
)
SELECT hex_str, FROM_HEX(hex_str) AS bytes_str
FROM Input;

-- Note that the result of FROM_HEX is of type BYTES, displayed as a base64-encoded string.
/*------------------+--------------+
 | hex_str          | bytes_str    |
 +------------------+--------------+
 | 0AF              | AK8=         |
 | 00010203aaeeefff | AAECA6ru7/8= |
 | 666f6f626172     | Zm9vYmFy     |
 +------------------+--------------*/
```

## `     LCASE    `

``` text
LCASE(val)
```

Alias for [`  LOWER  `](#lower) .

## `     LENGTH    `

``` text
LENGTH(value)
```

**Description**

Returns the length of the `  STRING  ` or `  BYTES  ` value. The returned value is in characters for `  STRING  ` arguments and in bytes for the `  BYTES  ` argument.

**Return type**

`  INT64  `

**Examples**

``` text
SELECT
  LENGTH('абвгд') AS string_example,
  LENGTH(CAST('абвгд' AS BYTES)) AS bytes_example;

/*----------------+---------------+
 | string_example | bytes_example |
 +----------------+---------------+
 | 5              | 10            |
 +----------------+---------------*/
```

## `     LOWER    `

``` text
LOWER(value)
```

**Description**

For `  STRING  ` arguments, returns the original string with all alphabetic characters in lowercase. Mapping between lowercase and uppercase is done according to the [Unicode Character Database](http://unicode.org/ucd/) without taking into account language-specific mappings.

For `  BYTES  ` arguments, the argument is treated as ASCII text, with all bytes greater than 127 left intact.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT
  LOWER('FOO BAR BAZ') AS example
FROM items;

/*-------------+
 | example     |
 +-------------+
 | foo bar baz |
 +-------------*/
```

## `     LPAD    `

``` text
LPAD(original_value, return_length[, pattern])
```

**Description**

Returns a `  STRING  ` or `  BYTES  ` value that consists of `  original_value  ` prepended with `  pattern  ` . The `  return_length  ` is an `  INT64  ` that specifies the length of the returned value. If `  original_value  ` is of type `  BYTES  ` , `  return_length  ` is the number of bytes. If `  original_value  ` is of type `  STRING  ` , `  return_length  ` is the number of characters.

The default value of `  pattern  ` is a blank space.

Both `  original_value  ` and `  pattern  ` must be the same data type.

If `  return_length  ` is less than or equal to the `  original_value  ` length, this function returns the `  original_value  ` value, truncated to the value of `  return_length  ` . For example, `  LPAD('hello world', 7);  ` returns `  'hello w'  ` .

If `  original_value  ` , `  return_length  ` , or `  pattern  ` is `  NULL  ` , this function returns `  NULL  ` .

This function returns an error if:

  - `  return_length  ` is negative
  - `  pattern  ` is empty

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT FORMAT('%T', LPAD('c', 5)) AS results

/*---------+
 | results |
 +---------+
 | "    c" |
 +---------*/
```

``` text
SELECT LPAD('b', 5, 'a') AS results

/*---------+
 | results |
 +---------+
 | aaaab   |
 +---------*/
```

``` text
SELECT LPAD('abc', 10, 'ghd') AS results

/*------------+
 | results    |
 +------------+
 | ghdghdgabc |
 +------------*/
```

``` text
SELECT LPAD('abc', 2, 'd') AS results

/*---------+
 | results |
 +---------+
 | ab      |
 +---------*/
```

``` text
SELECT FORMAT('%T', LPAD(b'abc', 10, b'ghd')) AS results

/*---------------+
 | results       |
 +---------------+
 | b"ghdghdgabc" |
 +---------------*/
```

## `     LTRIM    `

``` text
LTRIM(value1[, value2])
```

**Description**

Identical to [TRIM](#trim) , but only removes leading characters.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT CONCAT('#', LTRIM('   apple   '), '#') AS example

/*-------------+
 | example     |
 +-------------+
 | #apple   #  |
 +-------------*/
```

``` text
SELECT LTRIM('***apple***', '*') AS example

/*-----------+
 | example   |
 +-----------+
 | apple***  |
 +-----------*/
```

``` text
SELECT LTRIM('xxxapplexxx', 'xyz') AS example

/*-----------+
 | example   |
 +-----------+
 | applexxx  |
 +-----------*/
```

## `     NORMALIZE    `

``` text
NORMALIZE(value[, normalization_mode])
```

**Description**

Takes a string value and returns it as a normalized string. If you don't provide a normalization mode, `  NFC  ` is used.

[Normalization](https://en.wikipedia.org/wiki/Unicode_equivalence#Normalization) is used to ensure that two strings are equivalent. Normalization is often used in situations in which two strings render the same on the screen but have different Unicode code points.

`  NORMALIZE  ` supports four optional normalization modes:

<table>
<thead>
<tr class="header">
<th>Value</th>
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       NFC      </code></td>
<td>Normalization Form Canonical Composition</td>
<td>Decomposes and recomposes characters by canonical equivalence.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NFKC      </code></td>
<td>Normalization Form Compatibility Composition</td>
<td>Decomposes characters by compatibility, then recomposes them by canonical equivalence.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       NFD      </code></td>
<td>Normalization Form Canonical Decomposition</td>
<td>Decomposes characters by canonical equivalence, and multiple combining characters are arranged in a specific order.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NFKD      </code></td>
<td>Normalization Form Compatibility Decomposition</td>
<td>Decomposes characters by compatibility, and multiple combining characters are arranged in a specific order.</td>
</tr>
</tbody>
</table>

**Return type**

`  STRING  `

**Examples**

The following example normalizes different language characters:

``` text
SELECT
  NORMALIZE('\u00ea') as a,
  NORMALIZE('\u0065\u0302') as b,
  NORMALIZE('\u00ea') = NORMALIZE('\u0065\u0302') as normalized;

/*---+---+------------+
 | a | b | normalized |
 +---+---+------------+
 | ê | ê | TRUE       |
 +---+---+------------*/
```

The following examples normalize different space characters:

``` text
SELECT NORMALIZE('Raha\u2004Mahan', NFKC) AS normalized_name

/*-----------------+
 | normalized_name |
 +-----------------+
 | Raha Mahan      |
 +-----------------*/
```

``` text
SELECT NORMALIZE('Raha\u2005Mahan', NFKC) AS normalized_name

/*-----------------+
 | normalized_name |
 +-----------------+
 | Raha Mahan      |
 +-----------------*/
```

``` text
SELECT NORMALIZE('Raha\u2006Mahan', NFKC) AS normalized_name

/*-----------------+
 | normalized_name |
 +-----------------+
 | Raha Mahan      |
 +-----------------*/
```

``` text
SELECT NORMALIZE('Raha Mahan', NFKC) AS normalized_name

/*-----------------+
 | normalized_name |
 +-----------------+
 | Raha Mahan      |
 +-----------------*/
```

## `     NORMALIZE_AND_CASEFOLD    `

``` text
NORMALIZE_AND_CASEFOLD(value[, normalization_mode])
```

**Description**

Takes a string value and returns it as a normalized string. If you don't provide a normalization mode, `  NFC  ` is used.

[Normalization](https://en.wikipedia.org/wiki/Unicode_equivalence#Normalization) is used to ensure that two strings are equivalent. Normalization is often used in situations in which two strings render the same on the screen but have different Unicode code points.

[Case folding](https://en.wikipedia.org/wiki/Letter_case#Case_folding) is used for the caseless comparison of strings. If you need to compare strings and case shouldn't be considered, use `  NORMALIZE_AND_CASEFOLD  ` , otherwise use [`  NORMALIZE  `](#normalize) .

`  NORMALIZE_AND_CASEFOLD  ` supports four optional normalization modes:

<table>
<thead>
<tr class="header">
<th>Value</th>
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       NFC      </code></td>
<td>Normalization Form Canonical Composition</td>
<td>Decomposes and recomposes characters by canonical equivalence.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NFKC      </code></td>
<td>Normalization Form Compatibility Composition</td>
<td>Decomposes characters by compatibility, then recomposes them by canonical equivalence.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       NFD      </code></td>
<td>Normalization Form Canonical Decomposition</td>
<td>Decomposes characters by canonical equivalence, and multiple combining characters are arranged in a specific order.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NFKD      </code></td>
<td>Normalization Form Compatibility Decomposition</td>
<td>Decomposes characters by compatibility, and multiple combining characters are arranged in a specific order.</td>
</tr>
</tbody>
</table>

**Return type**

`  STRING  `

**Examples**

``` text
SELECT
  NORMALIZE('The red barn') = NORMALIZE('The Red Barn') AS normalized,
  NORMALIZE_AND_CASEFOLD('The red barn')
    = NORMALIZE_AND_CASEFOLD('The Red Barn') AS normalized_with_case_folding;

/*------------+------------------------------+
 | normalized | normalized_with_case_folding |
 +------------+------------------------------+
 | FALSE      | TRUE                         |
 +------------+------------------------------*/
```

``` text
SELECT
  '\u2168' AS a,
  'IX' AS b,
  NORMALIZE_AND_CASEFOLD('\u2168', NFD)=NORMALIZE_AND_CASEFOLD('IX', NFD) AS nfd,
  NORMALIZE_AND_CASEFOLD('\u2168', NFC)=NORMALIZE_AND_CASEFOLD('IX', NFC) AS nfc,
  NORMALIZE_AND_CASEFOLD('\u2168', NFKD)=NORMALIZE_AND_CASEFOLD('IX', NFKD) AS nfkd,
  NORMALIZE_AND_CASEFOLD('\u2168', NFKC)=NORMALIZE_AND_CASEFOLD('IX', NFKC) AS nfkc;

/*---+----+-------+-------+------+------+
 | a | b  | nfd   | nfc   | nfkd | nfkc |
 +---+----+-------+-------+------+------+
 | Ⅸ | IX | false | false | true | true |
 +---+----+-------+-------+------+------*/
```

``` text
SELECT
  '\u0041\u030A' AS a,
  '\u00C5' AS b,
  NORMALIZE_AND_CASEFOLD('\u0041\u030A', NFD)=NORMALIZE_AND_CASEFOLD('\u00C5', NFD) AS nfd,
  NORMALIZE_AND_CASEFOLD('\u0041\u030A', NFC)=NORMALIZE_AND_CASEFOLD('\u00C5', NFC) AS nfc,
  NORMALIZE_AND_CASEFOLD('\u0041\u030A', NFKD)=NORMALIZE_AND_CASEFOLD('\u00C5', NFKD) AS nkfd,
  NORMALIZE_AND_CASEFOLD('\u0041\u030A', NFKC)=NORMALIZE_AND_CASEFOLD('\u00C5', NFKC) AS nkfc;

/*---+----+-------+-------+------+------+
 | a | b  | nfd   | nfc   | nkfd | nkfc |
 +---+----+-------+-------+------+------+
 | Å | Å  | true  | true  | true | true |
 +---+----+-------+-------+------+------*/
```

## `     OCTET_LENGTH    `

``` text
OCTET_LENGTH(value)
```

Alias for [`  BYTE_LENGTH  `](#byte_length) .

## `     REGEXP_CONTAINS    `

``` text
REGEXP_CONTAINS(value, regexp)
```

**Description**

Returns `  TRUE  ` if `  value  ` is a partial match for the regular expression, `  regexp  ` .

If the `  regexp  ` argument is invalid, the function returns an error.

You can search for a full match by using `  ^  ` (beginning of text) and `  $  ` (end of text). Due to regular expression operator precedence, it's good practice to use parentheses around everything between `  ^  ` and `  $  ` .

**Note:** GoogleSQL provides regular expression support using the [re2](https://github.com/google/re2/wiki/Syntax) library; see that documentation for its regular expression syntax.

**Return type**

`  BOOL  `

**Examples**

The following queries check to see if an email is valid:

```` text
SELECT
  'foo@example.com' AS email,
  REGEXP_CONTAINS('foo@example.com', r'@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+') AS is_valid

/*-----------------+----------+
 | email           | is_valid |
 +-----------------+----------+
 | foo@example.com | TRUE     |
 +-----------------+----------*/
 ```

 ```googlesql
SELECT
  'www.example.net' AS email,
  REGEXP_CONTAINS('www.example.net', r'@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+') AS is_valid

/*-----------------+----------+
 | email           | is_valid |
 +-----------------+----------+
 | www.example.net | FALSE    |
 +-----------------+----------*/
 ```

The following queries check to see if an email is valid. They
perform a full match, using `^` and `$`. Due to regular expression operator
precedence, it's good practice to use parentheses around everything between `^`
and `$`.

```googlesql
SELECT
  'a@foo.com' AS email,
  REGEXP_CONTAINS('a@foo.com', r'^([\w.+-]+@foo\.com|[\w.+-]+@bar\.org)$') AS valid_email_address,
  REGEXP_CONTAINS('a@foo.com', r'^[\w.+-]+@foo\.com|[\w.+-]+@bar\.org$') AS without_parentheses;

/*----------------+---------------------+---------------------+
 | email          | valid_email_address | without_parentheses |
 +----------------+---------------------+---------------------+
 | a@foo.com      | true                | true                |
 +----------------+---------------------+---------------------*/
````

``` text
SELECT
  'a@foo.computer' AS email,
  REGEXP_CONTAINS('a@foo.computer', r'^([\w.+-]+@foo\.com|[\w.+-]+@bar\.org)$') AS valid_email_address,
  REGEXP_CONTAINS('a@foo.computer', r'^[\w.+-]+@foo\.com|[\w.+-]+@bar\.org$') AS without_parentheses;

/*----------------+---------------------+---------------------+
 | email          | valid_email_address | without_parentheses |
 +----------------+---------------------+---------------------+
 | a@foo.computer | false               | true                |
 +----------------+---------------------+---------------------*/
```

``` text
SELECT
  'b@bar.org' AS email,
  REGEXP_CONTAINS('b@bar.org', r'^([\w.+-]+@foo\.com|[\w.+-]+@bar\.org)$') AS valid_email_address,
  REGEXP_CONTAINS('b@bar.org', r'^[\w.+-]+@foo\.com|[\w.+-]+@bar\.org$') AS without_parentheses;

/*----------------+---------------------+---------------------+
 | email          | valid_email_address | without_parentheses |
 +----------------+---------------------+---------------------+
 | b@bar.org      | true                | true                |
 +----------------+---------------------+---------------------*/
```

``` text
SELECT
  '!b@bar.org' AS email,
  REGEXP_CONTAINS('!b@bar.org', r'^([\w.+-]+@foo\.com|[\w.+-]+@bar\.org)$') AS valid_email_address,
  REGEXP_CONTAINS('!b@bar.org', r'^[\w.+-]+@foo\.com|[\w.+-]+@bar\.org$') AS without_parentheses;

/*----------------+---------------------+---------------------+
 | email          | valid_email_address | without_parentheses |
 +----------------+---------------------+---------------------+
 | !b@bar.org     | false               | true                |
 +----------------+---------------------+---------------------*/
```

``` text
SELECT
  'c@buz.net' AS email,
  REGEXP_CONTAINS('c@buz.net', r'^([\w.+-]+@foo\.com|[\w.+-]+@bar\.org)$') AS valid_email_address,
  REGEXP_CONTAINS('c@buz.net', r'^[\w.+-]+@foo\.com|[\w.+-]+@bar\.org$') AS without_parentheses;

/*----------------+---------------------+---------------------+
 | email          | valid_email_address | without_parentheses |
 +----------------+---------------------+---------------------+
 | c@buz.net      | false               | false               |
 +----------------+---------------------+---------------------*/
```

## `     REGEXP_EXTRACT    `

``` text
REGEXP_EXTRACT(value, regexp)
```

**Description**

Returns the first substring in `  value  ` that matches the [re2 regular expression](https://github.com/google/re2/wiki/Syntax) , `  regexp  ` . Returns `  NULL  ` if there is no match.

If the regular expression contains a capturing group ( `  (...)  ` ), and there is a match for that capturing group, that match is returned. If there are multiple matches for a capturing group, the first match is returned.

Returns an error if:

  - The regular expression is invalid
  - The regular expression has more than one capturing group

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT REGEXP_EXTRACT('foo@example.com', r'^[a-zA-Z0-9_.+-]+') AS user_name

/*-----------+
 | user_name |
 +-----------+
 | foo       |
 +-----------*/
```

``` text
SELECT REGEXP_EXTRACT('foo@example.com', r'^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.([a-zA-Z0-9-.]+$)')

/*------------------+
 | top_level_domain |
 +------------------+
 | com              |
 +------------------*/
```

``` text
SELECT
  REGEXP_EXTRACT('ab', '.b') AS result_a,
  REGEXP_EXTRACT('ab', '(.)b') AS result_b,
  REGEXP_EXTRACT('xyztb', '(.)+b') AS result_c,
  REGEXP_EXTRACT('ab', '(z)?b') AS result_d

/*-------------------------------------------+
 | result_a | result_b | result_c | result_d |
 +-------------------------------------------+
 | ab       | a        | t        | NULL     |
 +-------------------------------------------*/
```

## `     REGEXP_EXTRACT_ALL    `

``` text
REGEXP_EXTRACT_ALL(value, regexp)
```

**Description**

Returns an array of all substrings of `  value  ` that match the [re2 regular expression](https://github.com/google/re2/wiki/Syntax) , `  regexp  ` . Returns an empty array if there is no match.

If the regular expression contains a capturing group ( `  (...)  ` ), the function returns an array of substrings that are matched by the capturing group.

The `  REGEXP_EXTRACT_ALL  ` function only returns non-overlapping matches. For example, using this function to extract `  ana  ` from `  banana  ` returns only one substring, not two.

When a capturing group is present, this non-overlapping rule applies to the *entire substring* matched by the whole regular expression, not just the part within the capturing group. The search for any subsequent match begins *after* the end of the entire substring that satisfied the previous match. In the examples that follow, the second example illustrates this behavior with the pattern `  r'\d(\d)\d'  ` .

Returns an error if:

  - The regular expression is invalid
  - The regular expression has more than one capturing group

**Return type**

`  ARRAY<STRING>  ` or `  ARRAY<BYTES>  `

**Examples**

``` text
SELECT REGEXP_EXTRACT_ALL('Try `func(x)` or `func(y)`', '`(.+?)`') AS example

/*--------------------+
 | example            |
 +--------------------+
 | [func(x), func(y)] |
 +--------------------*/
```

The following example demonstrates non-overlapping matches with a capturing group:

``` text
SELECT REGEXP_EXTRACT_ALL('123456', r'\d(\d)\d') AS example;

/*-----------+
 | example   |
 +-----------+
 | ['2', '5'] |
 +-----------*/
```

The pattern `  r'\d(\d)\d'  ` matches `  '123'  ` and captures `  '2'  ` . The next search starts after `  '3'  ` , and then it matches `  '456'  ` and captures `  '5'  ` .

## `     REGEXP_REPLACE    `

``` text
REGEXP_REPLACE(value, regexp, replacement)
```

**Description**

Returns a `  STRING  ` where all substrings of `  value  ` that match regular expression `  regexp  ` are replaced with `  replacement  ` .

You can use backslashed-escaped digits (\\1 to \\9) within the `  replacement  ` argument to insert text matching the corresponding parenthesized group in the `  regexp  ` pattern. Use \\0 to refer to the entire matching text.

To add a backslash in your regular expression, you must first escape it. For example, `  SELECT REGEXP_REPLACE('abc', 'b(.)', 'X\\1');  ` returns `  aXc  ` . You can also use [raw strings](/spanner/docs/reference/standard-sql/lexical#string_and_bytes_literals) to remove one layer of escaping, for example `  SELECT REGEXP_REPLACE('abc', 'b(.)', r'X\1');  ` .

The `  REGEXP_REPLACE  ` function only replaces non-overlapping matches. For example, replacing `  ana  ` within `  banana  ` results in only one replacement, not two.

If the `  regexp  ` argument isn't a valid regular expression, this function returns an error.

**Note:** GoogleSQL provides regular expression support using the [re2](https://github.com/google/re2/wiki/Syntax) library; see that documentation for its regular expression syntax.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT REGEXP_REPLACE('# Heading', r'^# ([a-zA-Z0-9\s]+$)', '<h1>\\1</h1>') AS html

/*--------------------------+
 | html                     |
 +--------------------------+
 | <h1>Heading</h1>         |
 +--------------------------*/
```

## `     REPEAT    `

``` text
REPEAT(original_value, repetitions)
```

**Description**

Returns a `  STRING  ` or `  BYTES  ` value that consists of `  original_value  ` , repeated. The `  repetitions  ` parameter specifies the number of times to repeat `  original_value  ` . Returns `  NULL  ` if either `  original_value  ` or `  repetitions  ` are `  NULL  ` .

This function returns an error if the `  repetitions  ` value is negative.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT REPEAT('abc', 3) AS results

/*-----------+
 | results   |
 |-----------|
 | abcabcabc |
 +-----------*/
```

``` text
SELECT REPEAT('abc', NULL) AS results

/*---------+
 | results |
 |---------|
 | NULL    |
 +---------*/
```

``` text
SELECT REPEAT(NULL, 3) AS results

/*---------+
 | results |
 |---------|
 | NULL    |
 +---------*/
```

## `     REPLACE    `

``` text
REPLACE(original_value, from_pattern, to_pattern)
```

**Description**

Replaces all occurrences of `  from_pattern  ` with `  to_pattern  ` in `  original_value  ` . If `  from_pattern  ` is empty, no replacement is made.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
WITH desserts AS
  (SELECT 'apple pie' as dessert
  UNION ALL
  SELECT 'blackberry pie' as dessert
  UNION ALL
  SELECT 'cherry pie' as dessert)

SELECT
  REPLACE (dessert, 'pie', 'cobbler') as example
FROM desserts;

/*--------------------+
 | example            |
 +--------------------+
 | apple cobbler      |
 | blackberry cobbler |
 | cherry cobbler     |
 +--------------------*/
```

## `     REVERSE    `

``` text
REVERSE(value)
```

**Description**

Returns the reverse of the input `  STRING  ` or `  BYTES  ` .

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT REVERSE('abc') AS results

/*---------+
 | results |
 +---------+
 | cba     |
 +---------*/
```

``` text
SELECT FORMAT('%T', REVERSE(b'1a3')) AS results

/*---------+
 | results |
 +---------+
 | b"3a1"  |
 +---------*/
```

## `     RPAD    `

``` text
RPAD(original_value, return_length[, pattern])
```

**Description**

Returns a `  STRING  ` or `  BYTES  ` value that consists of `  original_value  ` appended with `  pattern  ` . The `  return_length  ` parameter is an `  INT64  ` that specifies the length of the returned value. If `  original_value  ` is `  BYTES  ` , `  return_length  ` is the number of bytes. If `  original_value  ` is `  STRING  ` , `  return_length  ` is the number of characters.

The default value of `  pattern  ` is a blank space.

Both `  original_value  ` and `  pattern  ` must be the same data type.

If `  return_length  ` is less than or equal to the `  original_value  ` length, this function returns the `  original_value  ` value, truncated to the value of `  return_length  ` . For example, `  RPAD('hello world', 7);  ` returns `  'hello w'  ` .

If `  original_value  ` , `  return_length  ` , or `  pattern  ` is `  NULL  ` , this function returns `  NULL  ` .

This function returns an error if:

  - `  return_length  ` is negative
  - `  pattern  ` is empty

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT FORMAT('%T', RPAD('c', 5)) AS results

/*---------+
 | results |
 +---------+
 | "c    " |
 +---------*/
```

``` text
SELECT RPAD('b', 5, 'a') AS results

/*---------+
 | results |
 +---------+
 | baaaa   |
 +---------*/
```

``` text
SELECT RPAD('abc', 10, 'ghd') AS results

/*------------+
 | results    |
 +------------+
 | abcghdghdg |
 +------------*/
```

``` text
SELECT RPAD('abc', 2, 'd') AS results

/*---------+
 | results |
 +---------+
 | ab      |
 +---------*/
```

``` text
SELECT FORMAT('%T', RPAD(b'abc', 10, b'ghd')) AS results

/*---------------+
 | results       |
 +---------------+
 | b"abcghdghdg" |
 +---------------*/
```

## `     RTRIM    `

``` text
RTRIM(value1[, value2])
```

**Description**

Identical to [TRIM](#trim) , but only removes trailing characters.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT RTRIM('***apple***', '*') AS example

/*-----------+
 | example   |
 +-----------+
 | ***apple  |
 +-----------*/
```

``` text
SELECT RTRIM('applexxz', 'xyz') AS example

/*---------+
 | example |
 +---------+
 | apple   |
 +---------*/
```

## `     SAFE_CONVERT_BYTES_TO_STRING    `

``` text
SAFE_CONVERT_BYTES_TO_STRING(value)
```

**Description**

Converts a sequence of `  BYTES  ` to a `  STRING  ` . Any invalid UTF-8 characters are replaced with the Unicode replacement character, `  U+FFFD  ` .

**Return type**

`  STRING  `

**Examples**

The following statement returns the Unicode replacement character, �.

``` text
SELECT SAFE_CONVERT_BYTES_TO_STRING(b'\xc2') as safe_convert;
```

## `     SOUNDEX    `

``` text
SOUNDEX(value)
```

**Description**

Returns a `  STRING  ` that represents the [Soundex](https://en.wikipedia.org/wiki/Soundex) code for `  value  ` .

SOUNDEX produces a phonetic representation of a string. It indexes words by sound, as pronounced in English. It's typically used to help determine whether two strings, such as the family names *Levine* and *Lavine* , or the words *to* and *too* , have similar English-language pronunciation.

The result of the SOUNDEX consists of a letter followed by 3 digits. Non-latin characters are ignored. If the remaining string is empty after removing non-Latin characters, an empty `  STRING  ` is returned.

**Return type**

`  STRING  `

**Examples**

``` text
SELECT 'Ashcraft' AS value, SOUNDEX('Ashcraft') AS soundex

/*----------------------+---------+
 | value                | soundex |
 +----------------------+---------+
 | Ashcraft             | A261    |
 +----------------------+---------*/
```

## `     SPLIT    `

``` text
SPLIT(value[, delimiter])
```

**Description**

Splits a `  STRING  ` or `  BYTES  ` value, using a delimiter. The `  delimiter  ` argument must be a literal character or sequence of characters. You can't split with a regular expression.

For `  STRING  ` , the default delimiter is the comma `  ,  ` .

For `  BYTES  ` , you must specify a delimiter.

Splitting on an empty delimiter produces an array of UTF-8 characters for `  STRING  ` values, and an array of `  BYTES  ` for `  BYTES  ` values.

Splitting an empty `  STRING  ` returns an `  ARRAY  ` with a single empty `  STRING  ` .

**Return type**

`  ARRAY<STRING>  ` or `  ARRAY<BYTES>  `

**Examples**

``` text
WITH letters AS
  (SELECT '' as letter_group
  UNION ALL
  SELECT 'a' as letter_group
  UNION ALL
  SELECT 'b c d' as letter_group)

SELECT SPLIT(letter_group, ' ') as example
FROM letters;

/*----------------------+
 | example              |
 +----------------------+
 | []                   |
 | [a]                  |
 | [b, c, d]            |
 +----------------------*/
```

## `     SPLIT_SUBSTR    `

``` text
SPLIT_SUBSTR(value, delimiter, start_split[, count])
```

**Description**

Returns a substring from an input `  STRING  ` that's determined by a delimiter, a location that indicates the first split of the substring to return, and the number of splits to include in the returned substring.

The `  value  ` argument is the supplied `  STRING  ` value from which a substring is returned.

The `  delimiter  ` argument is the delimiter used to split the input `  STRING  ` . It must be a literal character or sequence of characters.

  - The `  delimiter  ` argument can't be a regular expression.
  - Delimiter matching is from left to right.
  - If the delimiter is a sequence of characters, then two instances of the delimiter in the input string can't overlap. For example, if the delimiter is `  **  ` , then the delimiters in the string `  aa***bb***cc  ` are:
      - The first two asterisks after `  aa  ` .
      - The first two asterisks after `  bb  ` .

The `  start_split  ` argument is an integer that specifies the first split of the substring to return.

  - If `  start_split  ` is `  1  ` , then the returned substring starts from the first split.
  - If `  start_split  ` is `  0  ` or less than the negative of the number of splits, then `  start_split  ` is treated as if it's `  1  ` and returns a substring that starts with the first split.
  - If `  start_split  ` is greater than the number of splits, then an empty string is returned.
  - If `  start_split  ` is negative, then the splits are counted from the end of the input string. If `  start_split  ` is `  -1  ` , then the last split in the input string is returned.

The optional `  count  ` argument is an integer that specifies the maximum number of splits to include in the returned substring.

  - If `  count  ` isn't specified, then the substring from the `  start_split  ` position to the end of the input string is returned.
  - If `  count  ` is `  0  ` , an empty string is returned.
  - If `  count  ` is negative, an error is returned.
  - If the sum of `  count  ` plus `  start_split  ` is greater than the number of splits, then a substring from `  start_split  ` to the end of the input string is returned.

**Return type**

`  STRING  `

**Examples**

The following example returns an empty string because `  count  ` is `  0  ` :

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", 1, 0) AS example

/*---------+
 | example |
 +---------+
 |         |
 +---------*/
```

The following example returns two splits starting with the first split:

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", 1, 2) AS example

/*---------+
 | example |
 +---------+
 | www.abc |
 +---------*/
```

The following example returns one split starting with the first split:

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", 1, 1) AS example

/*---------+
 | example |
 +---------+
 | www     |
 +---------*/
```

The following example returns splits from the right because `  start_split  ` is a negative value:

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", -1, 1) AS example

/*---------+
 | example |
 +---------+
 | com     |
 +---------*/
```

The following example returns a substring with three splits, starting with the first split:

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", 1, 3) AS example

/*-------------+
 | example     |
 +-------------+
 | www.abc.xyz |
 +------------*/
```

If `  start_split  ` is zero, then it's treated as if it's `  1  ` . The following example returns three substrings starting with the first split:

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", 0, 3) AS example

/*-------------+
 | example     |
 +-------------+
 | www.abc.xyz |
 +------------*/
```

If `  start_split  ` is greater than the number of splits, then an empty string is returned:

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", 5, 3) AS example

/*---------+
 | example |
 +---------+
 |         |
 +--------*/
```

In the following example, the `  start_split  ` value ( `  -5  ` ) is less than the negative of the number of splits ( `  -4  ` ), so `  start_split  ` is treated as `  1  ` :

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", -5, 3) AS example

/*-------------+
 | example     |
 +-------------+
 | www.abc.xyz |
 +------------*/
```

In the following example, the substring from `  start_split  ` to the end of the string is returned because `  count  ` isn't specified:

``` text
SELECT SPLIT_SUBSTR("www.abc.xyz.com", ".", 3) AS example

/*---------+
 | example |
 +---------+
 | xyz.com |
 +--------*/
```

The following two examples demonstrate how `  SPLIT_SUBSTR  ` works with a multi-character delimiter that has overlapping matches in the input string. In each example, the input string contains instances of three asterisks in a row ( `  ***  ` ) and the delimiter is two asterisks ( `  **  ` ).

``` text
SELECT SPLIT_SUBSTR('aaa***bbb***ccc', '**', 1, 2) AS example

/*-----------+
 | example   |
 +-----------+
 | aaa***bbb |
 +----------*/
```

``` text
SELECT SPLIT_SUBSTR('aaa***bbb***ccc', '**', 2, 2) AS example

/*------------+
 | example    |
 +------------+
 | *bbb***ccc |
 +-----------*/
```

## `     STARTS_WITH    `

``` text
STARTS_WITH(value, prefix)
```

**Description**

Takes two `  STRING  ` or `  BYTES  ` values. Returns `  TRUE  ` if `  prefix  ` is a prefix of `  value  ` .

**Return type**

`  BOOL  `

**Examples**

``` text
SELECT STARTS_WITH('bar', 'b') AS example

/*---------+
 | example |
 +---------+
 |    True |
 +---------*/
```

## `     STRPOS    `

``` text
STRPOS(value, subvalue)
```

**Description**

Takes two `  STRING  ` or `  BYTES  ` values. Returns the 1-based position of the first occurrence of `  subvalue  ` inside `  value  ` . Returns `  0  ` if `  subvalue  ` isn't found.

**Return type**

`  INT64  `

**Examples**

``` text
SELECT STRPOS('foo@example.com', '@') AS example

/*---------+
 | example |
 +---------+
 |       4 |
 +---------*/
```

## `     SUBSTR    `

``` text
SUBSTR(value, position[, length])
```

**Description**

Gets a portion (substring) of the supplied `  STRING  ` or `  BYTES  ` value.

The `  position  ` argument is an integer specifying the starting position of the substring.

  - If `  position  ` is `  1  ` , the substring starts from the first character or byte.
  - If `  position  ` is `  0  ` or less than `  -LENGTH(value)  ` , `  position  ` is set to `  1  ` , and the substring starts from the first character or byte.
  - If `  position  ` is greater than the length of `  value  ` , the function produces an empty substring.
  - If `  position  ` is negative, the function counts from the end of `  value  ` , with `  -1  ` indicating the last character or byte.

The `  length  ` argument specifies the maximum number of characters or bytes to return.

  - If `  length  ` isn't specified, the function produces a substring that starts at the specified position and ends at the last character or byte of `  value  ` .
  - If `  length  ` is `  0  ` , the function produces an empty substring.
  - If `  length  ` is negative, the function produces an error.
  - The returned substring may be shorter than `  length  ` , for example, when `  length  ` exceeds the length of `  value  ` , or when the starting position of the substring plus `  length  ` is greater than the length of `  value  ` .

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT SUBSTR('apple', 2) AS example

/*---------+
 | example |
 +---------+
 | pple    |
 +---------*/
```

``` text
SELECT SUBSTR('apple', 2, 2) AS example

/*---------+
 | example |
 +---------+
 | pp      |
 +---------*/
```

``` text
SELECT SUBSTR('apple', -2) AS example

/*---------+
 | example |
 +---------+
 | le      |
 +---------*/
```

``` text
SELECT SUBSTR('apple', 1, 123) AS example

/*---------+
 | example |
 +---------+
 | apple   |
 +---------*/
```

``` text
SELECT SUBSTR('apple', 123) AS example

/*---------+
 | example |
 +---------+
 |         |
 +---------*/
```

``` text
SELECT SUBSTR('apple', 123, 5) AS example

/*---------+
 | example |
 +---------+
 |         |
 +---------*/
```

## `     SUBSTRING    `

``` text
SUBSTRING(value, position[, length])
```

Alias for [`  SUBSTR  `](#substr) .

## `     TO_BASE32    `

``` text
TO_BASE32(bytes_expr)
```

**Description**

Converts a sequence of `  BYTES  ` into a base32-encoded `  STRING  ` . To convert a base32-encoded `  STRING  ` into `  BYTES  ` , use [FROM\_BASE32](#from_base32) .

**Return type**

`  STRING  `

**Example**

``` text
SELECT TO_BASE32(b'abcde\xFF') AS base32_string;

/*------------------+
 | base32_string    |
 +------------------+
 | MFRGGZDF74====== |
 +------------------*/
```

## `     TO_BASE64    `

``` text
TO_BASE64(bytes_expr)
```

**Description**

Converts a sequence of `  BYTES  ` into a base64-encoded `  STRING  ` . To convert a base64-encoded `  STRING  ` into `  BYTES  ` , use [FROM\_BASE64](#from_base64) .

There are several base64 encodings in common use that vary in exactly which alphabet of 65 ASCII characters are used to encode the 64 digits and padding. See [RFC 4648](https://tools.ietf.org/html/rfc4648#section-4) for details. This function adds padding and uses the alphabet `  [A-Za-z0-9+/=]  ` .

**Return type**

`  STRING  `

**Example**

``` text
SELECT TO_BASE64(b'\377\340') AS base64_string;

/*---------------+
 | base64_string |
 +---------------+
 | /+A=          |
 +---------------*/
```

To work with an encoding using a different base64 alphabet, you might need to compose `  TO_BASE64  ` with the `  REPLACE  ` function. For instance, the `  base64url  ` url-safe and filename-safe encoding commonly used in web programming uses `  -_=  ` as the last characters rather than `  +/=  ` . To encode a `  base64url  ` -encoded string, replace `  +  ` and `  /  ` with `  -  ` and `  _  ` respectively.

``` text
SELECT REPLACE(REPLACE(TO_BASE64(b'\377\340'), '+', '-'), '/', '_') as websafe_base64;

/*----------------+
 | websafe_base64 |
 +----------------+
 | _-A=           |
 +----------------*/
```

## `     TO_CODE_POINTS    `

``` text
TO_CODE_POINTS(value)
```

**Description**

Takes a `  STRING  ` or `  BYTES  ` value and returns an array of `  INT64  ` values that represent code points or extended ASCII character values.

  - If `  value  ` is a `  STRING  ` , each element in the returned array represents a [code point](https://en.wikipedia.org/wiki/Code_point) . Each code point falls within the range of \[0, 0xD7FF\] and \[0xE000, 0x10FFFF\].
  - If `  value  ` is `  BYTES  ` , each element in the array is an extended ASCII character value in the range of \[0, 255\].

To convert from an array of code points to a `  STRING  ` or `  BYTES  ` , see [CODE\_POINTS\_TO\_STRING](#code_points_to_string) or [CODE\_POINTS\_TO\_BYTES](#code_points_to_bytes) .

**Return type**

`  ARRAY<INT64>  `

**Examples**

The following examples get the code points for each element in an array of words.

``` text
SELECT
  'foo' AS word,
  TO_CODE_POINTS('foo') AS code_points

/*---------+------------------------------------+
 | word    | code_points                        |
 +---------+------------------------------------+
 | foo     | [102, 111, 111]                    |
 +---------+------------------------------------*/
```

``` text
SELECT
  'bar' AS word,
  TO_CODE_POINTS('bar') AS code_points

/*---------+------------------------------------+
 | word    | code_points                        |
 +---------+------------------------------------+
 | bar     | [98, 97, 114]                      |
 +---------+------------------------------------*/
```

``` text
SELECT
  'baz' AS word,
  TO_CODE_POINTS('baz') AS code_points

/*---------+------------------------------------+
 | word    | code_points                        |
 +---------+------------------------------------+
 | baz     | [98, 97, 122]                      |
 +---------+------------------------------------*/
```

``` text
SELECT
  'giraffe' AS word,
  TO_CODE_POINTS('giraffe') AS code_points

/*---------+------------------------------------+
 | word    | code_points                        |
 +---------+------------------------------------+
 | giraffe | [103, 105, 114, 97, 102, 102, 101] |
 +---------+------------------------------------*/
```

``` text
SELECT
  'llama' AS word,
  TO_CODE_POINTS('llama') AS code_points

/*---------+------------------------------------+
 | word    | code_points                        |
 +---------+------------------------------------+
 | llama   | [108, 108, 97, 109, 97]            |
 +---------+------------------------------------*/
```

The following examples convert integer representations of `  BYTES  ` to their corresponding ASCII character values.

``` text
SELECT
  b'\x66\x6f\x6f' AS bytes_value,
  TO_CODE_POINTS(b'\x66\x6f\x6f') AS bytes_value_as_integer

/*------------------+------------------------+
 | bytes_value      | bytes_value_as_integer |
 +------------------+------------------------+
 | foo              | [102, 111, 111]        |
 +------------------+------------------------*/
```

``` text
SELECT
  b'\x00\x01\x10\xff' AS bytes_value,
  TO_CODE_POINTS(b'\x00\x01\x10\xff') AS bytes_value_as_integer

/*------------------+------------------------+
 | bytes_value      | bytes_value_as_integer |
 +------------------+------------------------+
 | \x00\x01\x10\xff | [0, 1, 16, 255]        |
 +------------------+------------------------*/
```

The following example demonstrates the difference between a `  BYTES  ` result and a `  STRING  ` result. Notice that the character `  Ā  ` is represented as a two-byte Unicode sequence. As a result, the `  BYTES  ` version of `  TO_CODE_POINTS  ` returns an array with two elements, while the `  STRING  ` version returns an array with a single element.

``` text
SELECT TO_CODE_POINTS(b'Ā') AS b_result, TO_CODE_POINTS('Ā') AS s_result;

/*------------+----------+
 | b_result   | s_result |
 +------------+----------+
 | [196, 128] | [256]    |
 +------------+----------*/
```

## `     TO_HEX    `

``` text
TO_HEX(bytes)
```

**Description**

Converts a sequence of `  BYTES  ` into a hexadecimal `  STRING  ` . Converts each byte in the `  STRING  ` as two hexadecimal characters in the range `  (0..9, a..f)  ` . To convert a hexadecimal-encoded `  STRING  ` to `  BYTES  ` , use [FROM\_HEX](#from_hex) .

**Return type**

`  STRING  `

**Example**

``` text
SELECT
  b'\x00\x01\x02\x03\xAA\xEE\xEF\xFF' AS byte_string,
  TO_HEX(b'\x00\x01\x02\x03\xAA\xEE\xEF\xFF') AS hex_string

/*----------------------------------+------------------+
 | byte_string                      | hex_string       |
 +----------------------------------+------------------+
 | \x00\x01\x02\x03\xaa\xee\xef\xff | 00010203aaeeefff |
 +----------------------------------+------------------*/
```

## `     TRIM    `

``` text
TRIM(value_to_trim[, set_of_characters_to_remove])
```

**Description**

Takes a `  STRING  ` or `  BYTES  ` value to trim.

If the value to trim is a `  STRING  ` , removes from this value all leading and trailing Unicode code points in `  set_of_characters_to_remove  ` . The set of code points is optional. If it isn't specified, all whitespace characters are removed from the beginning and end of the value to trim.

If the value to trim is `  BYTES  ` , removes from this value all leading and trailing bytes in `  set_of_characters_to_remove  ` . The set of bytes is required.

**Return type**

  - `  STRING  ` if `  value_to_trim  ` is a `  STRING  ` value.
  - `  BYTES  ` if `  value_to_trim  ` is a `  BYTES  ` value.

**Examples**

In the following example, all leading and trailing whitespace characters are removed from `  item  ` because `  set_of_characters_to_remove  ` isn't specified.

``` text
SELECT CONCAT('#', TRIM( '   apple   '), '#') AS example

/*----------+
 | example  |
 +----------+
 | #apple#  |
 +----------*/
```

In the following example, all leading and trailing `  *  ` characters are removed from ' ***apple*** '.

``` text
SELECT TRIM('***apple***', '*') AS example

/*---------+
 | example |
 +---------+
 | apple   |
 +---------*/
```

In the following example, all leading and trailing `  x  ` , `  y  ` , and `  z  ` characters are removed from 'xzxapplexxy'.

``` text
SELECT TRIM('xzxapplexxy', 'xyz') as example

/*---------+
 | example |
 +---------+
 | apple   |
 +---------*/
```

In the following example, examine how `  TRIM  ` interprets characters as Unicode code-points. If your trailing character set contains a combining diacritic mark over a particular letter, `  TRIM  ` might strip the same diacritic mark from a different letter.

``` text
SELECT
  TRIM('abaW̊', 'Y̊') AS a,
  TRIM('W̊aba', 'Y̊') AS b,
  TRIM('abaŪ̊', 'Y̊') AS c,
  TRIM('Ū̊aba', 'Y̊') AS d

/*------+------+------+------+
 | a    | b    | c    | d    |
 +------+------+------+------+
 | abaW | W̊aba | abaŪ | Ūaba |
 +------+------+------+------*/
```

In the following example, all leading and trailing `  b'n'  ` , `  b'a'  ` , `  b'\xab'  ` bytes are removed from `  item  ` .

``` text
SELECT b'apple', TRIM(b'apple', b'na\xab') AS example

-- Note that the result of TRIM is of type BYTES, displayed as a base64-encoded string.
/*----------------------+------------------+
 | item                 | example          |
 +----------------------+------------------+
 | YXBwbGU=             | cHBsZQ==         |
 +----------------------+------------------*/
```

## `     UCASE    `

``` text
UCASE(val)
```

Alias for [`  UPPER  `](#upper) .

## `     UPPER    `

``` text
UPPER(value)
```

**Description**

For `  STRING  ` arguments, returns the original string with all alphabetic characters in uppercase. Mapping between uppercase and lowercase is done according to the [Unicode Character Database](http://unicode.org/ucd/) without taking into account language-specific mappings.

For `  BYTES  ` arguments, the argument is treated as ASCII text, with all bytes greater than 127 left intact.

**Return type**

`  STRING  ` or `  BYTES  `

**Examples**

``` text
SELECT UPPER('foo') AS example

/*---------+
 | example |
 +---------+
 | FOO     |
 +---------*/
```
