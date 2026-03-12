A GoogleSQL statement comprises a series of tokens. Tokens include identifiers, quoted identifiers, literals, keywords, operators, and special characters. You can separate tokens with comments or whitespace such as spaces, backspaces, tabs, or newlines.

## Identifiers

Identifiers are names that are associated with columns, tables, fields, path expressions, and more. They can be [unquoted](#unquoted_identifiers) or [quoted](#quoted_identifiers) and some are [case-sensitive](#case_sensitivity) .

### Unquoted identifiers

  - Must begin with a letter or an underscore (\_) character.
  - Subsequent characters can be letters, numbers, or underscores (\_).

### Quoted identifiers

  - Must be enclosed by backtick (\`) characters.
  - Can contain any characters, including spaces and symbols.
  - Can't be empty.
  - Have the same escape sequences as [string literals](#string_and_bytes_literals) .
  - If an identifier is the same as a [reserved keyword](#reserved_keywords) , the identifier must be quoted. For example, the identifier `  FROM  ` must be quoted. Additional rules apply for [path expressions](#path_expressions) and [field names](#field_names) .

### Identifier examples

Path expression examples:

``` text
-- Valid. _5abc and dataField are valid identifiers.
_5abc.dataField

-- Valid. `5abc` and dataField are valid identifiers.
`5abc`.dataField

-- Invalid. 5abc is an invalid identifier because it's unquoted and starts
-- with a number rather than a letter or underscore.
5abc.dataField

-- Valid. abc5 and dataField are valid identifiers.
abc5.dataField

-- Invalid. abc5! is an invalid identifier because it's unquoted and contains
-- a character that isn't a letter, number, or underscore.
abc5!.dataField

-- Valid. `GROUP` and dataField are valid identifiers.
`GROUP`.dataField

-- Invalid. GROUP is an invalid identifier because it's unquoted and is a
-- stand-alone reserved keyword.
GROUP.dataField

-- Valid. abc5 and GROUP are valid identifiers.
abc5.GROUP
```

Function examples:

``` text
-- Valid. dataField is a valid identifier in a function called foo().
foo().dataField
```

Array access operation examples:

``` text
-- Valid. dataField is a valid identifier in an array called items.
items[OFFSET(3)].dataField
```

Named query parameter examples:

``` text
-- Valid. param and dataField are valid identifiers.
@param.dataField
```

Protocol buffer examples:

``` text
-- Valid. dataField is a valid identifier in a protocol buffer called foo.
(foo).dataField
```

## Path expressions

A path expression describes how to navigate to an object in a graph of objects and generally follows this structure:

``` text
path:
  [path_expression][. ...]

path_expression:
  [first_part]/subsequent_part[ { / | : | - } subsequent_part ][...]

first_part:
  { unquoted_identifier | quoted_identifier }

subsequent_part:
  { unquoted_identifier | quoted_identifier | number }
```

  - `  path  ` : A graph of one or more objects.
  - `  path_expression  ` : An object in a graph of objects.
  - `  first_part  ` : A path expression can start with a quoted or unquoted identifier. If the path expressions starts with a [reserved keyword](#reserved_keywords) , it must be a quoted identifier.
  - `  subsequent_part  ` : Subsequent parts of a path expression can include non-identifiers, such as reserved keywords. If a subsequent part of a path expressions starts with a [reserved keyword](#reserved_keywords) , it may be quoted or unquoted.

Examples:

``` text
foo.bar
foo.bar/25
foo/bar:25
foo/bar/25-31
/foo/bar
/25/foo/bar
```

## Field names

A field name represents the name of a field inside a complex data type such as a struct, protocol buffer message, or JSON object.

  - A field name can be a quoted identifier or an unquoted identifier.

## Literals

A literal represents a constant value of a built-in data type. Some, but not all, data types can be expressed as literals.

### String and bytes literals

A string literal represents a constant value of the [string data type](/spanner/docs/reference/standard-sql/data-types#string_type) . A bytes literal represents a constant value of the [bytes data type](/spanner/docs/reference/standard-sql/data-types#bytes_type) .

Both string and bytes literals must be *quoted* , either with single ( `  '  ` ) or double ( `  "  ` ) quotation marks, or *triple-quoted* with groups of three single ( `  '''  ` ) or three double ( `  """  ` ) quotation marks.

#### Formats for quoted literals

The following table lists all of the ways you can format a quoted literal.

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Literal</th>
<th>Examples</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Quoted string</td>
<td><ul>
<li><code dir="ltr" translate="no">         "abc"        </code></li>
<li><code dir="ltr" translate="no">         "it's"        </code></li>
<li><code dir="ltr" translate="no">         'it\'s'        </code></li>
<li><code dir="ltr" translate="no">         'Title: "Boy"'        </code></li>
</ul></td>
<td>Quoted strings enclosed by single ( <code dir="ltr" translate="no">       '      </code> ) quotes can contain unescaped double ( <code dir="ltr" translate="no">       "      </code> ) quotes, as well as the inverse.<br />
Backslashes ( <code dir="ltr" translate="no">       \      </code> ) introduce escape sequences. See the Escape Sequences table below.<br />
Quoted strings can't contain newlines, even when preceded by a backslash ( <code dir="ltr" translate="no">       \      </code> ).</td>
</tr>
<tr class="even">
<td>Triple-quoted string</td>
<td><ul>
<li><code dir="ltr" translate="no">         """abc"""        </code></li>
<li><code dir="ltr" translate="no">         '''it's'''        </code></li>
<li><code dir="ltr" translate="no">         '''Title:"Boy"'''        </code></li>
<li><code dir="ltr" translate="no">         '''two                  lines'''        </code></li>
<li><code dir="ltr" translate="no">         '''why\?'''        </code></li>
</ul></td>
<td>Embedded newlines and quotes are allowed without escaping - see fourth example.<br />
Backslashes ( <code dir="ltr" translate="no">       \      </code> ) introduce escape sequences. See Escape Sequences table below.<br />
A trailing unescaped backslash ( <code dir="ltr" translate="no">       \      </code> ) at the end of a line isn't allowed.<br />
End the string with three unescaped quotes in a row that match the starting quotes.</td>
</tr>
<tr class="odd">
<td>Raw string</td>
<td><ul>
<li><code dir="ltr" translate="no">         r"abc+"        </code></li>
<li><code dir="ltr" translate="no">         r'''abc+'''        </code></li>
<li><code dir="ltr" translate="no">         r"""abc+"""        </code></li>
<li><code dir="ltr" translate="no">         r'f\(abc,(.*),def\)'        </code></li>
</ul></td>
<td>Quoted or triple-quoted literals that have the raw string literal prefix ( <code dir="ltr" translate="no">       r      </code> or <code dir="ltr" translate="no">       R      </code> ) are interpreted as raw strings (sometimes described as regex strings).<br />
Backslash characters ( <code dir="ltr" translate="no">       \      </code> ) don't act as escape characters. If a backslash followed by another character occurs inside the string literal, both characters are preserved.<br />
A raw string can't end with an odd number of backslashes.<br />
Raw strings are useful for constructing regular expressions. The prefix is case-insensitive.</td>
</tr>
<tr class="even">
<td>Bytes</td>
<td><ul>
<li><code dir="ltr" translate="no">         B"abc"        </code></li>
<li><code dir="ltr" translate="no">         B'''abc'''        </code></li>
<li><code dir="ltr" translate="no">         b"""abc"""        </code></li>
</ul></td>
<td>Quoted or triple-quoted literals that have the bytes literal prefix ( <code dir="ltr" translate="no">       b      </code> or <code dir="ltr" translate="no">       B      </code> ) are interpreted as bytes.</td>
</tr>
<tr class="odd">
<td>Raw bytes</td>
<td><ul>
<li><code dir="ltr" translate="no">         br'abc+'        </code></li>
<li><code dir="ltr" translate="no">         RB"abc+"        </code></li>
<li><code dir="ltr" translate="no">         RB'''abc'''        </code></li>
</ul></td>
<td>A bytes literal can be interpreted as raw bytes if both the <code dir="ltr" translate="no">       r      </code> and <code dir="ltr" translate="no">       b      </code> prefixes are present. These prefixes can be combined in any order and are case-insensitive. For example, <code dir="ltr" translate="no">       rb'abc*'      </code> and <code dir="ltr" translate="no">       rB'abc*'      </code> and <code dir="ltr" translate="no">       br'abc*'      </code> are all equivalent. See the description for raw string to learn more about what you can do with a raw literal.</td>
</tr>
</tbody>
</table>

#### Escape sequences for string and bytes literals

The following table lists all valid escape sequences for representing non-alphanumeric characters in string and bytes literals. Any sequence not in this table produces an error.

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<thead>
<tr class="header">
<th>Escape Sequence</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       \a      </code></td>
<td>Bell</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \b      </code></td>
<td>Backspace</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       \f      </code></td>
<td>Formfeed</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \n      </code></td>
<td>Newline</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       \r      </code></td>
<td>Carriage Return</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \t      </code></td>
<td>Tab</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       \v      </code></td>
<td>Vertical Tab</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \\      </code></td>
<td>Backslash ( <code dir="ltr" translate="no">       \      </code> )</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       \?      </code></td>
<td>Question Mark ( <code dir="ltr" translate="no">       ?      </code> )</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \"      </code></td>
<td>Double Quote ( <code dir="ltr" translate="no">       "      </code> )</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       \'      </code></td>
<td>Single Quote ( <code dir="ltr" translate="no">       '      </code> )</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \`      </code></td>
<td>Backtick ( <code dir="ltr" translate="no">       `      </code> )</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       \ooo      </code></td>
<td>Octal escape, with exactly 3 digits (in the range 0–7). Decodes to a single Unicode character (in string literals) or byte (in bytes literals).</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \xhh      </code> or <code dir="ltr" translate="no">       \Xhh      </code></td>
<td>Hex escape, with exactly 2 hex digits (0–9 or A–F or a–f). Decodes to a single Unicode character (in string literals) or byte (in bytes literals). Examples:
<ul>
<li><code dir="ltr" translate="no">         '\x41'        </code> == <code dir="ltr" translate="no">         'A'        </code></li>
<li><code dir="ltr" translate="no">         '\x41B'        </code> is <code dir="ltr" translate="no">         'AB'        </code></li>
<li><code dir="ltr" translate="no">         '\x4'        </code> is an error</li>
</ul></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       \uhhhh      </code></td>
<td>Unicode escape, with lowercase 'u' and exactly 4 hex digits. Valid only in string literals or identifiers.<br />
Note that the range D800-DFFF isn't allowed, as these are surrogate unicode values.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       \Uhhhhhhhh      </code></td>
<td>Unicode escape, with uppercase 'U' and exactly 8 hex digits. Valid only in string literals or identifiers.<br />
The range D800-DFFF isn't allowed, as these values are surrogate unicode values. Also, values greater than 10FFFF aren't allowed.</td>
</tr>
</tbody>
</table>

### Integer literals

Integer literals are either a sequence of decimal digits (0–9) or a hexadecimal value that's prefixed with " `  0x  ` " or " `  0X  ` ". Integers can be prefixed by " `  +  ` " or " `  -  ` " to represent positive and negative values, respectively. Examples:

``` text
123
0xABC
-123
```

An integer literal is interpreted as an `  INT64  ` .

A integer literal represents a constant value of the [integer data type](/spanner/docs/reference/standard-sql/data-types#integer_types) .

### `     NUMERIC    ` literals

You can construct `  NUMERIC  ` literals using the `  NUMERIC  ` keyword followed by a floating point value in quotes.

Examples:

``` text
SELECT NUMERIC '0';
SELECT NUMERIC '123456';
SELECT NUMERIC '-3.14';
SELECT NUMERIC '-0.54321';
SELECT NUMERIC '1.23456e05';
SELECT NUMERIC '-9.876e-3';
```

A `  NUMERIC  ` literal represents a constant value of the [`  NUMERIC  ` data type](/spanner/docs/reference/standard-sql/data-types#decimal_types) .

### Floating point literals

Syntax options:

``` text
[+-]DIGITS.[DIGITS][e[+-]DIGITS]
[+-][DIGITS].DIGITS[e[+-]DIGITS]
DIGITSe[+-]DIGITS
```

`  DIGITS  ` represents one or more decimal numbers (0 through 9) and `  e  ` represents the exponent marker (e or E).

Examples:

``` text
123.456e-67
.1E4
58.
4e2
```

Numeric literals that contain either a decimal point or an exponent marker are presumed to be type double.

Implicit coercion of floating point literals to float type is possible if the value is within the valid float range.

There is no literal representation of NaN or infinity, but the following case-insensitive strings can be explicitly cast to float:

  - "NaN"
  - "inf" or "+inf"
  - "-inf"

A floating-point literal represents a constant value of the [floating-point data type](/spanner/docs/reference/standard-sql/data-types#floating_point_types) .

### Array literals

Array literals are comma-separated lists of elements enclosed in square brackets. The `  ARRAY  ` keyword is optional, and an explicit element type T is also optional.

Examples:

``` text
[1, 2, 3]
['x', 'y', 'xy']
ARRAY[1, 2, 3]
ARRAY<string>['x', 'y', 'xy']
ARRAY<int64>[]
```

An array literal represents a constant value of the [array data type](/spanner/docs/reference/standard-sql/data-types#array_type) .

### Struct literals

A struct literal is a struct whose fields are all literals. Struct literals can be written using any of the syntaxes for [constructing a struct](/spanner/docs/reference/standard-sql/data-types#constructing_a_struct) (tuple syntax, typeless struct syntax, or typed struct syntax).

Note that tuple syntax requires at least two fields, in order to distinguish it from an ordinary parenthesized expression. To write a struct literal with a single field, use typeless struct syntax or typed struct syntax.

<table>
<thead>
<tr class="header">
<th>Example</th>
<th>Output Type</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       (1, 2, 3)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;INT64, INT64, INT64&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       (1, 'abc')      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;INT64, STRING&gt;      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT(1 AS foo, 'abc' AS bar)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;foo INT64, bar STRING&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT&lt;INT64, STRING&gt;(1, 'abc')      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;INT64, STRING&gt;      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT(1)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;INT64&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT&lt;INT64&gt;(1)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;INT64&gt;      </code></td>
</tr>
</tbody>
</table>

A struct literal represents a constant value of the [struct data type](/spanner/docs/reference/standard-sql/data-types#struct_type) .

### Date literals

Syntax:

``` text
DATE 'date_canonical_format'
```

Date literals contain the `  DATE  ` keyword followed by [`  date_canonical_format  `](/spanner/docs/reference/standard-sql/data-types#canonical_format_for_date_literals) , a string literal that conforms to the canonical date format, enclosed in single quotation marks. Date literals support a range between the years 1 and 9999, inclusive. Dates outside of this range are invalid.

For example, the following date literal represents September 27, 2014:

``` text
DATE '2014-09-27'
```

String literals in canonical date format also implicitly coerce to DATE type when used where a DATE-type expression is expected. For example, in the query

``` text
SELECT * FROM foo WHERE date_col = "2014-09-27"
```

the string literal `  "2014-09-27"  ` will be coerced to a date literal.

A date literal represents a constant value of the [date data type](/spanner/docs/reference/standard-sql/data-types#date_type) .

### Timestamp literals

Syntax:

``` text
TIMESTAMP 'timestamp_canonical_format'
```

Timestamp literals contain the `  TIMESTAMP  ` keyword and [`  timestamp_canonical_format  `](/spanner/docs/reference/standard-sql/data-types#canonical_format_for_timestamp_literals) , a string literal that conforms to the canonical timestamp format, enclosed in single quotation marks.

Timestamp literals support a range between the years 1 and 9999, inclusive. Timestamps outside of this range are invalid.

A timestamp literal can include a numerical suffix to indicate the time zone:

``` text
TIMESTAMP '2014-09-27 12:30:00.45-08'
```

If this suffix is absent, the default time zone, America/Los\_Angeles, is used.

For example, the following timestamp represents 12:30 p.m. on September 27, 2014 in the default time zone, America/Los\_Angeles:

``` text
TIMESTAMP '2014-09-27 12:30:00.45'
```

For more information about time zones, see [Time zone](#timezone) .

String literals with the canonical timestamp format, including those with time zone names, implicitly coerce to a timestamp literal when used where a timestamp expression is expected. For example, in the following query, the string literal `  "2014-09-27 12:30:00.45 America/Los_Angeles"  ` is coerced to a timestamp literal.

``` text
SELECT * FROM foo
WHERE timestamp_col = "2014-09-27 12:30:00.45 America/Los_Angeles"
```

A timestamp literal can include these optional characters:

  - `  T  ` or `  t  `
  - `  Z  ` or `  z  `

If you use one of these characters, a space can't be included before or after it. These are valid:

``` text
TIMESTAMP '2017-01-18T12:34:56.123456Z'
TIMESTAMP '2017-01-18t12:34:56.123456'
TIMESTAMP '2017-01-18 12:34:56.123456z'
TIMESTAMP '2017-01-18 12:34:56.123456Z'
```

A timestamp literal represents a constant value of the [timestamp data type](/spanner/docs/reference/standard-sql/data-types#timestamp_type) .

#### Time zone

Since timestamp literals must be mapped to a specific point in time, a time zone is necessary to correctly interpret a literal. If a time zone isn't specified as part of the literal itself, then GoogleSQL uses the default time zone value, which the GoogleSQL implementation sets.

GoogleSQL can represent a time zones using a string, which represents the [offset from Coordinated Universal Time (UTC)](/spanner/docs/reference/standard-sql/data-types#utc_offset) .

Examples:

``` text
'-08:00'
'-8:15'
'+3:00'
'+07:30'
'-7'
```

Time zones can also be expressed using string [time zone names](/spanner/docs/reference/standard-sql/data-types#time_zone_name) .

Examples:

``` text
TIMESTAMP '2014-09-27 12:30:00 America/Los_Angeles'
TIMESTAMP '2014-09-27 12:30:00 America/Argentina/Buenos_Aires'
```

### Interval literals

An interval literal represents a constant value of the [interval data type](/spanner/docs/reference/standard-sql/data-types#interval_type) . There are two types of interval literals:

  - [Interval literal with a single datetime part](#interval_literal_single)
  - [Interval literal with a datetime part range](#interval_literal_range)

An interval literal can be used directly inside of the `  SELECT  ` statement and as an argument in some functions that support the interval data type.

#### Interval literal with a single datetime part

Syntax:

``` text
INTERVAL int64_expression datetime_part
```

The single datetime part syntax includes an `  INT64  ` expression and a single [interval-supported datetime part](/spanner/docs/reference/standard-sql/data-types#interval_datetime_parts) . For example:

``` text
-- 0 years, 0 months, 5 days, 0 hours, 0 minutes, 0 seconds (0-0 5 0:0:0)
INTERVAL 5 DAY

-- 0 years, 0 months, -5 days, 0 hours, 0 minutes, 0 seconds (0-0 -5 0:0:0)
INTERVAL -5 DAY

-- 0 years, 0 months, 0 days, 0 hours, 0 minutes, 1 seconds (0-0 0 0:0:1)
INTERVAL 1 SECOND
```

When a negative sign precedes the year or month part in an interval literal, the negative sign distributes over the years and months. Or, when a negative sign precedes the time part in an interval literal, the negative sign distributes over the hours, minutes, and seconds. For example:

``` text
-- -2 years, -1 months, 0 days, 0 hours, 0 minutes, and 0 seconds (-2-1 0 0:0:0)
INTERVAL -25 MONTH

-- 0 years, 0 months, 0 days, -1 hours, -30 minutes, and 0 seconds (0-0 0 -1:30:0)
INTERVAL -90 MINUTE
```

For more information on how to construct interval with a single datetime part, see [Construct an interval with a single datetime part](/spanner/docs/reference/standard-sql/data-types#single_datetime_part_interval) .

#### Interval literal with a datetime part range

Syntax:

``` text
INTERVAL datetime_parts_string starting_datetime_part TO ending_datetime_part
```

The range datetime part syntax includes a [datetime parts string](/spanner/docs/reference/standard-sql/data-types#range_datetime_part_interval) , a [starting datetime part](/spanner/docs/reference/standard-sql/data-types#interval_datetime_parts) , and an [ending datetime part](/spanner/docs/reference/standard-sql/data-types#interval_datetime_parts) .

For example:

``` text
-- 0 years, 0 months, 0 days, 10 hours, 20 minutes, 30 seconds (0-0 0 10:20:30.520)
INTERVAL '10:20:30.52' HOUR TO SECOND

-- 1 year, 2 months, 0 days, 0 hours, 0 minutes, 0 seconds (1-2 0 0:0:0)
INTERVAL '1-2' YEAR TO MONTH

-- 0 years, 1 month, -15 days, 0 hours, 0 minutes, 0 seconds (0-1 -15 0:0:0)
INTERVAL '1 -15' MONTH TO DAY

-- 0 years, 0 months, 1 day, 5 hours, 30 minutes, 0 seconds (0-0 1 5:30:0)
INTERVAL '1 5:30' DAY TO MINUTE
```

When a negative sign precedes the year or month part in an interval literal, the negative sign distributes over the years and months. Or, when a negative sign precedes the time part in an interval literal, the negative sign distributes over the hours, minutes, and seconds. For example:

``` text
-- -23 years, -2 months, 10 days, -12 hours, -30 minutes, and 0 seconds (-23-2 10 -12:30:0)
INTERVAL '-23-2 10 -12:30' YEAR TO MINUTE

-- -23 years, -2 months, 10 days, 0 hours, -30 minutes, and 0 seconds (-23-2 10 -0:30:0)
SELECT INTERVAL '-23-2 10 -0:30' YEAR TO MINUTE

-- Produces an error because the negative sign for minutes must come before the hour.
SELECT INTERVAL '-23-2 10 0:-30' YEAR TO MINUTE

-- Produces an error because the negative sign for months must come before the year.
SELECT INTERVAL '23--2 10 0:30' YEAR TO MINUTE

-- 0 years, -2 months, 10 days, 0 hours, 30 minutes, and 0 seconds (-0-2 10 0:30:0)
SELECT INTERVAL '-2 10 0:30' MONTH TO MINUTE

-- 0 years, 0 months, 0 days, 0 hours, -30 minutes, and -10 seconds (0-0 0 -0:30:10)
SELECT INTERVAL '-30:10' MINUTE TO SECOND
```

For more information on how to construct interval with a datetime part range, see [Construct an interval with a datetime part range](/spanner/docs/reference/standard-sql/data-types#single_datetime_part_interval) .

### Enum literals

There is no syntax for enum literals. Integer or string literals are coerced to the enum type when necessary, or explicitly cast to a specific enum type name. For more information, see [Literal coercion](/spanner/docs/reference/standard-sql/conversion_rules#coercion) .

An enum literal represents a constant value of the [enum data type](/spanner/docs/reference/standard-sql/data-types#enum_type) .

### JSON literals

Syntax:

``` text
JSON 'json_formatted_data'
```

A JSON literal represents [JSON](https://en.wikipedia.org/wiki/JSON) -formatted data.

Example:

``` text
JSON '
{
  "id": 10,
  "type": "fruit",
  "name": "apple",
  "on_menu": true,
  "recipes":
    {
      "salads":
      [
        { "id": 2001, "type": "Walnut Apple Salad" },
        { "id": 2002, "type": "Apple Spinach Salad" }
      ],
      "desserts":
      [
        { "id": 3001, "type": "Apple Pie" },
        { "id": 3002, "type": "Apple Scones" },
        { "id": 3003, "type": "Apple Crumble" }
      ]
    }
}
'
```

A JSON literal represents a constant value of the [JSON data type](/spanner/docs/reference/standard-sql/data-types#json_type) .

## Case sensitivity

GoogleSQL follows these rules for case sensitivity:

<table>
<thead>
<tr class="header">
<th>Category</th>
<th>Case-sensitive?</th>
<th>Notes</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Keywords</td>
<td>No</td>
<td></td>
</tr>
<tr class="even">
<td>Function names</td>
<td>No</td>
<td></td>
</tr>
<tr class="odd">
<td>Table names</td>
<td>See Notes</td>
<td>Table names are usually case-insensitive, but they might be case-sensitive when querying a database that uses case-sensitive table names.</td>
</tr>
<tr class="even">
<td>Column names</td>
<td>No</td>
<td></td>
</tr>
<tr class="odd">
<td>Field names</td>
<td>No</td>
<td></td>
</tr>
<tr class="even">
<td>All type names except for protocol buffer type names</td>
<td>No</td>
<td></td>
</tr>
<tr class="odd">
<td>Protocol buffer type names</td>
<td>Yes</td>
<td></td>
</tr>
<tr class="even">
<td>Enum type names</td>
<td>Yes</td>
<td></td>
</tr>
<tr class="odd">
<td>String values</td>
<td>Yes</td>
<td>Any value of type <code dir="ltr" translate="no">       STRING      </code> preserves its case. For example, the result of an expression that produces a <code dir="ltr" translate="no">       STRING      </code> value or a column value that's of type <code dir="ltr" translate="no">       STRING      </code> .</td>
</tr>
<tr class="even">
<td>String comparisons</td>
<td>Yes</td>
<td>However, string comparisons are case-insensitive in <a href="/spanner/docs/reference/standard-sql/collation-concepts">collations</a> that are case-insensitive. This behavior also applies to operations affected by collation, such as <code dir="ltr" translate="no">       GROUP BY      </code> and <code dir="ltr" translate="no">       DISTINCT      </code> clauses.</td>
</tr>
<tr class="odd">
<td>Aliases within a query</td>
<td>No</td>
<td></td>
</tr>
<tr class="even">
<td>Regular expression matching</td>
<td>See Notes</td>
<td>Regular expression matching is case-sensitive by default, unless the expression itself specifies that it should be case-insensitive.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       LIKE      </code> matching</td>
<td>Yes</td>
<td></td>
</tr>
<tr class="even">
<td>Property graph names</td>
<td>No</td>
<td></td>
</tr>
<tr class="odd">
<td>Property graph label names</td>
<td>No</td>
<td></td>
</tr>
<tr class="even">
<td>Property graph property names</td>
<td>No</td>
<td></td>
</tr>
</tbody>
</table>

## Reserved keywords

Keywords are a group of tokens that have special meaning in the GoogleSQL language, and have the following characteristics:

  - Keywords can't be used as identifiers unless enclosed by backtick (\`) characters.
  - Keywords are case-insensitive.

GoogleSQL has the following reserved keywords.

<table>
<colgroup>
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
</colgroup>
<tbody>
<tr class="odd">
<td>ALL<br />
AND<br />
ANY<br />
ARRAY<br />
AS<br />
ASC<br />
ASSERT_ROWS_MODIFIED<br />
AT<br />
BETWEEN<br />
BY<br />
CASE<br />
CAST<br />
COLLATE<br />
CONTAINS<br />
CREATE<br />
CROSS<br />
CUBE<br />
CURRENT<br />
DEFAULT<br />
DEFINE<br />
DESC<br />
DISTINCT<br />
ELSE<br />
END<br />
</td>
<td>ENUM<br />
ESCAPE<br />
EXCEPT<br />
EXCLUDE<br />
EXISTS<br />
EXTRACT<br />
FALSE<br />
FETCH<br />
FOLLOWING<br />
FOR<br />
FROM<br />
FULL<br />
GRAPH_TABLE<br />
GROUP<br />
GROUPING<br />
GROUPS<br />
HASH<br />
HAVING<br />
IF<br />
IGNORE<br />
IN<br />
INNER<br />
INTERSECT<br />
INTERVAL<br />
INTO<br />
</td>
<td>IS<br />
JOIN<br />
LATERAL<br />
LEFT<br />
LIKE<br />
LIMIT<br />
LOOKUP<br />
MERGE<br />
NATURAL<br />
NEW<br />
NO<br />
NOT<br />
NULL<br />
NULLS<br />
OF<br />
ON<br />
OR<br />
ORDER<br />
OUTER<br />
OVER<br />
PARTITION<br />
PRECEDING<br />
PROTO<br />
RANGE<br />
</td>
<td>RECURSIVE<br />
RESPECT<br />
RIGHT<br />
ROLLUP<br />
ROWS<br />
SELECT<br />
SET<br />
SOME<br />
STRUCT<br />
TABLESAMPLE<br />
THEN<br />
TO<br />
TREAT<br />
TRUE<br />
UNBOUNDED<br />
UNION<br />
UNNEST<br />
USING<br />
WHEN<br />
WHERE<br />
WINDOW<br />
WITH<br />
WITHIN<br />
</td>
</tr>
</tbody>
</table>

## Terminating semicolons

You can optionally use a terminating semicolon ( `  ;  ` ) when you submit a query string statement through an Application Programming Interface (API).

In a request containing multiple statements, you must separate statements with semicolons, but the semicolon is generally optional after the final statement. Some interactive tools require statements to have a terminating semicolon.

## Trailing commas

You can optionally use a trailing comma ( `  ,  ` ) at the end of a column list in a `  SELECT  ` statement. You might have a trailing comma as the result of programmatically creating a column list.

**Example**

``` text
SELECT name, release_date, FROM Books
```

## Query parameters

You can use query parameters to substitute arbitrary expressions. However, query parameters can't be used to substitute identifiers, column names, table names, or other parts of the query itself. Query parameters are defined outside of the query statement.

Client APIs allow the binding of parameter names to values; the query engine substitutes a bound value for a parameter at execution time.

Parameterized queries have better [query cache](/spanner/docs/whitepapers/life-of-query#caching) hit rates resulting in lower query latency and lower overall CPU usage.

For example, instead of using a query like the following:

``` text
SELECT AlbumId FROM Albums WHERE SEARCH(AlbumTitle_Tokens, 'cat')
```

use the following syntax:

``` text
SELECT AlbumId FROM Albums WHERE SEARCH(AlbumTitle_Tokens, @p)
```

Spanner runs the query optimizer on distinct SQL. The fewer distinct SQL instances the application uses, the fewer times the query optimization is invoked.

### Named query parameters

Syntax:

``` text
@parameter_name
```

A named query parameter is denoted using an [identifier](#identifiers) preceded by the `  @  ` character.

A named query parameter can start with an identifier or a reserved keyword. An identifier can be unquoted or quoted.

**Example:**

This example returns all rows where `  LastName  ` is equal to the value of the named query parameter `  myparam  ` .

``` text
SELECT * FROM Roster WHERE LastName = @myparam
```

## Hints

``` text
@{ hint [, ...] }

hint:
  [engine_name.]hint_name = value
```

The purpose of a hint is to modify the execution strategy for a query without changing the result of the query. Hints generally don't affect query semantics, but may have performance implications. These hint types are available:

  - [GROUP hints](/spanner/docs/reference/standard-sql/query-syntax#group_hints)
  - [JOIN hints](/spanner/docs/reference/standard-sql/query-syntax#join_hints)
  - [STATEMENT hints](/spanner/docs/reference/standard-sql/query-syntax#statement_hints)
  - [TABLE hints](/spanner/docs/reference/standard-sql/query-syntax#table_hints)

Hint syntax requires the `  @  ` character followed by curly braces. You can create one hint or a group of hints. The optional `  engine_name.  ` prefix allows for multiple engines to define hints with the same `  hint_name  ` . This is important if you need to suggest different engine-specific execution strategies or different engines support different hints.

You can assign [identifiers](#identifiers) and [literals](#literals) to hints.

  - Identifiers are useful for hints that are meant to act like enums. You can use an identifier to avoid using a quoted string. In the resolved AST, identifier hints are represented as string literals, so `  @{hint="abc"}  ` is the same as `  @{hint=abc}  ` . Identifier hints can also be used for hints that take a table name or column name as a single identifier.
  - NULL literals are allowed and are inferred as integers.

Hints are meant to apply only to the node they are attached to, and not to a larger scope. For example, a hint on a `  JOIN  ` in the middle of the `  FROM  ` clause is meant to apply to that `  JOIN  ` only, and not other `  JOIN  ` s in the `  FROM  ` clause. Statement-level hints can be used for hints that modify execution of an entire statement, for example an overall memory budget or deadline.

**Examples**

In this example, a literal is assigned to a hint. This hint is only used with two database engines called `  database_engine_a  ` and `  database_engine_b  ` . The value for the hint is different for each database engine.

``` text
@{ database_engine_a.file_count=23, database_engine_b.file_count=10 }
```

In this example, an identifier is assigned to a hint. There are unique identifiers for each hint type. You can view a list of hint types at the beginning of this topic.

``` text
@{ JOIN_METHOD=HASH_JOIN }
```

## Comments

Comments are sequences of characters that the parser ignores. GoogleSQL supports the following types of comments.

### Single-line comments

Use a single-line comment if you want the comment to appear on a line by itself.

**Examples**

``` text
# this is a single-line comment
SELECT book FROM library;
```

``` text
-- this is a single-line comment
SELECT book FROM library;
```

``` text
/* this is a single-line comment */
SELECT book FROM library;
```

``` text
SELECT book FROM library
/* this is a single-line comment */
WHERE book = "Ulysses";
```

### Inline comments

Use an inline comment if you want the comment to appear on the same line as a statement. A comment that's prepended with `  #  ` or `  --  ` must appear to the right of a statement.

**Examples**

``` text
SELECT book FROM library; # this is an inline comment
```

``` text
SELECT book FROM library; -- this is an inline comment
```

``` text
SELECT book FROM library; /* this is an inline comment */
```

``` text
SELECT book FROM library /* this is an inline comment */ WHERE book = "Ulysses";
```

### Multiline comments

Use a multiline comment if you need the comment to span multiple lines. Nested multiline comments aren't supported.

**Examples**

``` text
SELECT book FROM library
/*
  This is a multiline comment
  on multiple lines
*/
WHERE book = "Ulysses";
```

``` text
SELECT book FROM library
/* this is a multiline comment
on two lines */
WHERE book = "Ulysses";
```
