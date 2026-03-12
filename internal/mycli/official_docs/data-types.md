This page provides an overview of all GoogleSQL for Spanner data types, including information about their value domains. For information on data type literals and constructors, see [Lexical Structure and Syntax](/spanner/docs/reference/standard-sql/lexical#literals) .

## Data type list

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
<td><a href="#array_type">Array type</a></td>
<td>An ordered list of zero or more elements of non-array values.<br />
SQL type name: <code dir="ltr" translate="no">       ARRAY      </code></td>
</tr>
<tr class="even">
<td><a href="#boolean_type">Boolean type</a></td>
<td>A value that can be either <code dir="ltr" translate="no">       TRUE      </code> or <code dir="ltr" translate="no">       FALSE      </code> .<br />
SQL type name: <code dir="ltr" translate="no">       BOOL      </code><br />
SQL aliases: <code dir="ltr" translate="no">       BOOLEAN      </code></td>
</tr>
<tr class="odd">
<td><a href="#bytes_type">Bytes type</a></td>
<td>Variable-length binary data.<br />
SQL type name: <code dir="ltr" translate="no">       BYTES      </code></td>
</tr>
<tr class="even">
<td><a href="#date_type">Date type</a></td>
<td>A Gregorian calendar date, independent of time zone.<br />
SQL type name: <code dir="ltr" translate="no">       DATE      </code></td>
</tr>
<tr class="odd">
<td><a href="#enum_type">Enum type</a></td>
<td>Named type that enumerates a list of possible values.<br />
SQL type name: <code dir="ltr" translate="no">       ENUM      </code></td>
</tr>
<tr class="even">
<td><a href="#graph_element_type">Graph element type</a></td>
<td>An element in a property graph.<br />
SQL type name: <code dir="ltr" translate="no">       GRAPH_ELEMENT      </code></td>
</tr>
<tr class="odd">
<td><a href="#graph_path_type">Graph path type</a></td>
<td>A path in a property graph.<br />
SQL type name: <code dir="ltr" translate="no">       GRAPH_PATH      </code></td>
</tr>
<tr class="even">
<td><a href="#interval_type">Interval type</a></td>
<td>A duration of time, without referring to any specific point in time.<br />
SQL type name: <code dir="ltr" translate="no">       INTERVAL      </code></td>
</tr>
<tr class="odd">
<td><a href="#json_type">JSON type</a></td>
<td>Represents JSON, a lightweight data-interchange format.<br />
SQL type name: <code dir="ltr" translate="no">       JSON      </code></td>
</tr>
<tr class="even">
<td><a href="#numeric_types">Numeric types</a></td>
<td><p>A numeric value. Several types are supported.</p>
<p>A 64-bit integer.<br />
SQL type name: <code dir="ltr" translate="no">        INT64       </code></p>
<p>A decimal value with precision of 38 digits.<br />
SQL type name: <code dir="ltr" translate="no">        NUMERIC       </code></p>
<p>An approximate single precision numeric value.<br />
SQL type name: <code dir="ltr" translate="no">        FLOAT32       </code></p>
<p>An approximate double precision numeric value.<br />
SQL type name: <code dir="ltr" translate="no">        FLOAT64       </code></p></td>
</tr>
<tr class="odd">
<td><a href="#protocol_buffer_type">Protocol buffer type</a></td>
<td>A protocol buffer.<br />
SQL type name: <code dir="ltr" translate="no">       PROTO      </code></td>
</tr>
<tr class="even">
<td><a href="#string_type">String type</a></td>
<td>Variable-length character data.<br />
SQL type name: <code dir="ltr" translate="no">       STRING      </code></td>
</tr>
<tr class="odd">
<td><a href="#struct_type">Struct type</a></td>
<td>Container of ordered fields.<br />
SQL type name: <code dir="ltr" translate="no">       STRUCT      </code></td>
</tr>
<tr class="even">
<td><a href="#timestamp_type">Timestamp type</a></td>
<td>A timestamp value represents an absolute point in time, independent of any time zone or convention such as daylight saving time (DST).<br />
SQL type name: <code dir="ltr" translate="no">       TIMESTAMP      </code></td>
</tr>
<tr class="odd">
<td><a href="#uuid_type">UUID type</a></td>
<td>A universally unique identifier (UUID) represented as a 128-bit number.</td>
</tr>
</tbody>
</table>

## Data type properties

When storing and querying data, it's helpful to keep the following data type properties in mind:

### Valid column types

All data types are valid column types, except for:

  - `  STRUCT  `
  - `  INTERVAL  `

### Valid key column types

All data types are valid key column types for primary keys, foreign keys, and secondary indexes, except for:

  - `  FLOAT32  `
  - `  ARRAY  `
  - `  JSON  `
  - `  STRUCT  `

### Storage size for data types

Each data type includes 8 bytes of storage overhead, in addition to the following values:

  - `  ARRAY  ` : The sum of the size of its elements.
  - `  BOOL  ` : 1 byte.
  - `  BYTES  ` : The number of bytes.
  - `  DATE  ` : 4 bytes.
  - `  FLOAT32  ` : 4 bytes.
  - `  FLOAT64  ` : 8 bytes.
  - `  INT64  ` : 8 bytes.
  - `  JSON  ` : The number of bytes in UTF-8 encoding of the JSON-formatted string equivalent after canonicalization.
  - `  NUMERIC  ` : A function of both the precision and scale of the value being stored. The value `  0  ` is stored as 1 byte. The storage size for all other values varies between 6 and 22 bytes.
  - `  STRING  ` : The number of bytes in its UTF-8 encoding.
  - `  STRUCT  ` : The sum of its parts.
  - `  TIMESTAMP  ` : 12 bytes.

### Nullable data types

For nullable data types, `  NULL  ` is a valid value. Currently, all existing data types are nullable.

### Orderable data types

Expressions of orderable data types can be used in an `  ORDER BY  ` clause. Applies to all data types except for:

  - `  ARRAY  `
  - `  PROTO  `
  - `  STRUCT  `
  - `  JSON  `
  - `  GRAPH_ELEMENT  `
  - `  GRAPH_PATH  `

#### Ordering `     NULL    ` s

In the context of the `  ORDER BY  ` clause, `  NULL  ` s are the minimum possible value; that is, `  NULL  ` s appear first in `  ASC  ` sorts and last in `  DESC  ` sorts.

To learn more about using `  ASC  ` and `  DESC  ` , see the [`  ORDER BY  ` clause](/spanner/docs/reference/standard-sql/query-syntax#order_by_clause) .

#### Ordering floating points

Floating point values are sorted in this order, from least to greatest:

1.  `  NULL  `
2.  `  NaN  ` — All `  NaN  ` values are considered equal when sorting.
3.  `  -inf  `
4.  Negative numbers
5.  0 or -0 — All zero values are considered equal when sorting.
6.  Positive numbers
7.  `  +inf  `

### Groupable data types

Can generally appear in an expression following `  GROUP BY  ` and `  DISTINCT  ` . All data types are supported except for:

  - `  PROTO  `
  - `  JSON  `
  - `  ARRAY  `
  - `  STRUCT  `
  - `  GRAPH_PATH  `

#### Grouping with floating point types

Groupable floating point types can appear in an expression following `  GROUP BY  ` and `  DISTINCT  ` .

Special floating point values are grouped in the following way, including both grouping done by a `  GROUP BY  ` clause and grouping done by the `  DISTINCT  ` keyword:

  - `  NULL  `
  - `  NaN  ` — All `  NaN  ` values are considered equal when grouping.
  - `  -inf  `
  - 0 or -0 — All zero values are considered equal when grouping.
  - `  +inf  `

### Comparable data types

Values of the same comparable data type can be compared to each other. All data types are supported except for:

  - `  PROTO  `
  - `  JSON  `

Notes:

  - Equality comparisons for array data types are supported as long as the element types are the same, and the element types are comparable. Less than and greater than comparisons aren't supported.
  - Equality comparisons for structs are supported field by field, in field order. Field names are ignored. Less than and greater than comparisons aren't supported.
  - All types that support comparisons can be used in a `  JOIN  ` condition. See [JOIN Types](/spanner/docs/reference/standard-sql/query-syntax#join_types) for an explanation of join conditions.

## Array type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       ARRAY      </code></td>
<td>Ordered list of zero or more elements of any non-array type.</td>
</tr>
</tbody>
</table>

An array is an ordered list of zero or more elements of non-array values. Elements in an array must share the same type.

Arrays of arrays aren't allowed. Queries that would produce an array of arrays return an error. Instead, a struct must be inserted between the arrays using the `  SELECT AS STRUCT  ` construct.

To learn more about the literal representation of an array type, see [Array literals](/spanner/docs/reference/standard-sql/lexical#array_literals) .

To learn more about using arrays in GoogleSQL, see [Work with arrays](/spanner/docs/reference/standard-sql/arrays#constructing_arrays) .

### `     NULL    ` s and the array type

An empty array and a `  NULL  ` array are two distinct values. Arrays can contain `  NULL  ` elements.

### Declaring an array type

``` text
ARRAY<T>
```

Array types are declared using the angle brackets ( `  <  ` and `  >  ` ). The type of the elements of an array can be arbitrarily complex with the exception that an array can't directly contain another array.

**Examples**

Type Declaration

Meaning

`  ARRAY<INT64>  `

Simple array of 64-bit integers.

`  ARRAY<STRUCT<INT64, INT64>>  `

An array of structs, each of which contains two 64-bit integers.

`  ARRAY<ARRAY<INT64>>  `  
(not supported)

This is an **invalid** type declaration which is included here just in case you came looking for how to create a multi-level array. Arrays can't contain arrays directly. Instead see the next example.

`  ARRAY<STRUCT<ARRAY<INT64>>>  `

An array of arrays of 64-bit integers. Notice that there is a struct between the two arrays because arrays can't hold other arrays directly.

### Constructing an array

You can construct an array using array literals or array functions.

#### Using array literals

You can build an array literal in GoogleSQL using brackets ( `  [  ` and `  ]  ` ). Each element in an array is separated by a comma.

``` text
SELECT [1, 2, 3] AS numbers;

SELECT ["apple", "pear", "orange"] AS fruit;

SELECT [true, false, true] AS booleans;
```

You can also create arrays from any expressions that have compatible types. For example:

``` text
SELECT [a, b, c]
FROM
  (SELECT 5 AS a,
          37 AS b,
          406 AS c);

SELECT [a, b, c]
FROM
  (SELECT CAST(5 AS INT64) AS a,
          CAST(37 AS FLOAT64) AS b,
          406 AS c);
```

Notice that the second example contains three expressions: one that returns an `  INT64  ` , one that returns a `  FLOAT64  ` , and one that declares a literal. This expression works because all three expressions share `  FLOAT64  ` as a supertype.

To declare a specific data type for an array, use angle brackets ( `  <  ` and `  >  ` ). For example:

``` text
SELECT ARRAY<FLOAT64>[1, 2, 3] AS floats;
```

Arrays of most data types, such as `  INT64  ` or `  STRING  ` , don't require that you declare them first.

``` text
SELECT [1, 2, 3] AS numbers;
```

You can write an empty array of a specific type using `  ARRAY<type>[]  ` . You can also write an untyped empty array using `  []  ` , in which case GoogleSQL attempts to infer the array type from the surrounding context. If GoogleSQL can't infer a type, the default type `  ARRAY<INT64>  ` is used.

#### Using generated values

You can also construct an `  ARRAY  ` with generated values.

##### Generating arrays of integers

[`  GENERATE_ARRAY  `](/spanner/docs/reference/standard-sql/array_functions#generate_array) generates an array of values from a starting and ending value and a step value. For example, the following query generates an array that contains all of the odd integers from 11 to 33, inclusive:

``` text
SELECT GENERATE_ARRAY(11, 33, 2) AS odds;

/*--------------------------------------------------+
 | odds                                             |
 +--------------------------------------------------+
 | [11, 13, 15, 17, 19, 21, 23, 25, 27, 29, 31, 33] |
 +--------------------------------------------------*/
```

You can also generate an array of values in descending order by giving a negative step value:

``` text
SELECT GENERATE_ARRAY(21, 14, -1) AS countdown;

/*----------------------------------+
 | countdown                        |
 +----------------------------------+
 | [21, 20, 19, 18, 17, 16, 15, 14] |
 +----------------------------------*/
```

##### Generating arrays of dates

[`  GENERATE_DATE_ARRAY  `](/spanner/docs/reference/standard-sql/array_functions#generate_date_array) generates an array of `  DATE  ` s from a starting and ending `  DATE  ` and a step `  INTERVAL  ` .

You can generate a set of `  DATE  ` values using `  GENERATE_DATE_ARRAY  ` . For example, this query returns the current `  DATE  ` and the following `  DATE  ` s at 1 `  WEEK  ` intervals up to and including a later `  DATE  ` :

``` text
SELECT
  GENERATE_DATE_ARRAY('2017-11-21', '2017-12-31', INTERVAL 1 WEEK)
    AS date_array;

/*--------------------------------------------------------------------------+
 | date_array                                                               |
 +--------------------------------------------------------------------------+
 | [2017-11-21, 2017-11-28, 2017-12-05, 2017-12-12, 2017-12-19, 2017-12-26] |
 +--------------------------------------------------------------------------*/
```

## Boolean type

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       BOOL      </code><br />
<code dir="ltr" translate="no">       BOOLEAN      </code></td>
<td>Boolean values are represented by the keywords <code dir="ltr" translate="no">       TRUE      </code> and <code dir="ltr" translate="no">       FALSE      </code> (case-insensitive).</td>
</tr>
</tbody>
</table>

`  BOOLEAN  ` is an alias for `  BOOL  ` .

Boolean values are sorted in this order, from least to greatest:

1.  `  NULL  `
2.  `  FALSE  `
3.  `  TRUE  `

## Bytes type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       BYTES      </code></td>
<td>Variable-length binary data.</td>
</tr>
</tbody>
</table>

String and bytes are separate types that can't be used interchangeably. Most functions on strings are also defined on bytes. The bytes version operates on raw bytes rather than Unicode characters. Casts between string and bytes enforce that the bytes are encoded using UTF-8.

You can convert a base64-encoded `  STRING  ` expression into the `  BYTES  ` format using the [`  FROM_BASE64  ` function](/spanner/docs/reference/standard-sql/string_functions#from_base64) . You can also convert a sequence of `  BYTES  ` into a base64-encoded `  STRING  ` expression using the [`  TO_BASE64  ` function](/spanner/docs/reference/standard-sql/string_functions#to_base64) .

To learn more about the literal representation of a bytes type, see [Bytes literals](/spanner/docs/reference/standard-sql/lexical#string_and_bytes_literals) .

## Date type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Range</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       DATE      </code></td>
<td>0001-01-01 to 9999-12-31.</td>
</tr>
</tbody>
</table>

The date type represents a Gregorian calendar date, independent of time zone. A date value doesn't represent a specific 24-hour time period. Rather, a given date value represents a different 24-hour period when interpreted in different time zones, and may represent a shorter or longer day during daylight saving time (DST) transitions. To represent an absolute point in time, use a [timestamp](#timestamp_type) .

##### Canonical format

``` text
YYYY-[M]M-[D]D
```

  - `  YYYY  ` : Four-digit year.
  - `  [M]M  ` : One or two digit month.
  - `  [D]D  ` : One or two digit day.

To learn more about the literal representation of a date type, see [Date literals](/spanner/docs/reference/standard-sql/lexical#date_literals) .

## Enum type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       ENUM      </code></td>
<td>Named type that maps string constants to integer constants.</td>
</tr>
</tbody>
</table>

An enum is a named type that enumerates a list of possible values, each of which contains:

  - An integer value: Integers are used for comparison and ordering enum values. There is no requirement that these integers start at zero or that they be contiguous.
  - A string value for its name: Strings are case sensitive. In the case of protocol buffer open enums, this name is optional.
  - Optional alias values: One or more additional string values that act as aliases.

Enum values are referenced using their integer value or their string value. You reference an enum type, such as when using CAST, by using its fully qualified name.

You must define the `  ENUM  ` type in a protocol buffer file and declare it in the `  CREATE PROTO BUNDLE  ` statement.

You can't create new enum types using GoogleSQL.

To learn more about the literal representation of an enum type, see [Enum literals](/spanner/docs/reference/standard-sql/lexical#enum_literals) .

## Graph element type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       GRAPH_ELEMENT      </code></td>
<td>An element in a property graph.</td>
</tr>
</tbody>
</table>

A variable with a `  GRAPH_ELEMENT  ` type is produced by a graph query. The generated type has this format:

``` text
GRAPH_ELEMENT<T>
```

A graph element is either a node or an edge, representing data from a matching node or edge table based on its label. Each graph element holds a set of properties that can be accessed with a case-insensitive name, similar to fields of a struct.

Graph elements with dynamic properties enabled can store properties beyond those defined in the schema. A schema change isn't needed to manage dynamic properties because the property names and values are based on the input column's values. You can access dynamic properties with their names in the same way as defined properties. For information about how to model dynamic properties, see [dynamic properties definition](/spanner/docs/reference/standard-sql/graph-schema-statements#dynamic_properties_definition) .

If a property isn't defined in the schema, accessing it through the [field-access-operator](/spanner/docs/reference/standard-sql/operators#field_access_operator) returns the `  JSON  ` type if the dynamic property exists, or `  NULL  ` if the property doesn't exist.

**Note:** Names uniquely identify all properties in a graph element, case-insensitively. A defined property takes precedence over any dynamic property when their names conflict.

**Example**

In the following example, `  n  ` represents a graph element in the [`  FinGraph  `](/spanner/docs/reference/standard-sql/graph-schema-statements#fin_graph) property graph:

``` text
GRAPH FinGraph
MATCH (n:Person)
RETURN n.name
```

## Graph path type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       GRAPH_PATH      </code></td>
<td>A path in a property graph.</td>
</tr>
</tbody>
</table>

The graph path data type represents a sequence of nodes interleaved with edges and has this format:

``` text
GRAPH_PATH<NODE_TYPE, EDGE_TYPE>
```

You can construct a graph path with the [`  PATH  `](/spanner/docs/reference/standard-sql/graph-gql-functions#path) function or when you create a [path variable](/spanner/docs/reference/standard-sql/graph-patterns#graph_pattern_definition) in a graph pattern.

## Interval type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Range</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       INTERVAL      </code></td>
<td>-10000-0 -3660000 -87840000:0:0 to 10000-0 3660000 87840000:0:0</td>
</tr>
</tbody>
</table>

An `  INTERVAL  ` object represents duration or amount of time, without referring to any specific point in time.

##### Canonical format

``` text
[sign]Y-M [sign]D [sign]H:M:S[.F]
```

  - `  sign  ` : `  +  ` or `  -  `
  - `  Y  ` : Year
  - `  M  ` : Month
  - `  D  ` : Day
  - `  H  ` : Hour
  - `  M  ` : Minute
  - `  S  ` : Second
  - `  [.F]  ` : Up to nine fractional digits (nanosecond precision)

To learn more about the literal representation of an interval type, see [Interval literals](/spanner/docs/reference/standard-sql/lexical#interval_literals) .

### Constructing an interval

You can construct an interval with an interval literal that supports a [single datetime part](#single_datetime_part_interval) or a [datetime part range](#range_datetime_part_interval) .

#### Construct an interval with a single datetime part

``` text
INTERVAL int64_expression datetime_part
```

You can construct an `  INTERVAL  ` object with an `  INT64  ` expression and one [interval-supported datetime part](#interval_datetime_parts) . For example:

``` text
-- 1 year, 0 months, 0 days, 0 hours, 0 minutes, and 0 seconds (1-0 0 0:0:0)
INTERVAL 1 YEAR
INTERVAL 4 QUARTER
INTERVAL 12 MONTH

-- 0 years, 3 months, 0 days, 0 hours, 0 minutes, and 0 seconds (0-3 0 0:0:0)
INTERVAL 1 QUARTER
INTERVAL 3 MONTH

-- 0 years, 0 months, 42 days, 0 hours, 0 minutes, and 0 seconds (0-0 42 0:0:0)
INTERVAL 6 WEEK
INTERVAL 42 DAY

-- 0 years, 0 months, 0 days, 25 hours, 0 minutes, and 0 seconds (0-0 0 25:0:0)
INTERVAL 25 HOUR
INTERVAL 1500 MINUTE
INTERVAL 90000 SECOND

-- 0 years, 0 months, 0 days, 1 hours, 30 minutes, and 0 seconds (0-0 0 1:30:0)
INTERVAL 90 MINUTE

-- 0 years, 0 months, 0 days, 0 hours, 1 minutes, and 30 seconds (0-0 0 0:1:30)
INTERVAL 90 SECOND

-- 0 years, 0 months, -5 days, 0 hours, 0 minutes, and 0 seconds (0-0 -5 0:0:0)
INTERVAL -5 DAY
```

For additional examples, see [Interval literals](/spanner/docs/reference/standard-sql/lexical#interval_literal_single) .

#### Construct an interval with a datetime part range

``` text
INTERVAL datetime_parts_string starting_datetime_part TO ending_datetime_part
```

You can construct an `  INTERVAL  ` object with a `  STRING  ` that contains the datetime parts that you want to include, a starting datetime part, and an ending datetime part. The resulting `  INTERVAL  ` object only includes datetime parts in the specified range.

You can use one of the following formats with the [interval-supported datetime parts](#interval_datetime_parts) :

<table>
<thead>
<tr class="header">
<th>Datetime part string</th>
<th>Datetime parts</th>
<th>Example</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       Y-M      </code></td>
<td><code dir="ltr" translate="no">       YEAR TO MONTH      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '2-11' YEAR TO MONTH      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       Y-M D      </code></td>
<td><code dir="ltr" translate="no">       YEAR TO DAY      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '2-11 28' YEAR TO DAY      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       Y-M D H      </code></td>
<td><code dir="ltr" translate="no">       YEAR TO HOUR      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '2-11 28 16' YEAR TO HOUR      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       Y-M D H:M      </code></td>
<td><code dir="ltr" translate="no">       YEAR TO MINUTE      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '2-11 28 16:15' YEAR TO MINUTE      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       Y-M D H:M:S      </code></td>
<td><code dir="ltr" translate="no">       YEAR TO SECOND      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '2-11 28 16:15:14' YEAR TO SECOND      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       M D      </code></td>
<td><code dir="ltr" translate="no">       MONTH TO DAY      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '11 28' MONTH TO DAY      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       M D H      </code></td>
<td><code dir="ltr" translate="no">       MONTH TO HOUR      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '11 28 16' MONTH TO HOUR      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       M D H:M      </code></td>
<td><code dir="ltr" translate="no">       MONTH TO MINUTE      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '11 28 16:15' MONTH TO MINUTE      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       M D H:M:S      </code></td>
<td><code dir="ltr" translate="no">       MONTH TO SECOND      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '11 28 16:15:14' MONTH TO SECOND      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       D H      </code></td>
<td><code dir="ltr" translate="no">       DAY TO HOUR      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '28 16' DAY TO HOUR      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       D H:M      </code></td>
<td><code dir="ltr" translate="no">       DAY TO MINUTE      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '28 16:15' DAY TO MINUTE      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       D H:M:S      </code></td>
<td><code dir="ltr" translate="no">       DAY TO SECOND      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '28 16:15:14' DAY TO SECOND      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       H:M      </code></td>
<td><code dir="ltr" translate="no">       HOUR TO MINUTE      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '16:15' HOUR TO MINUTE      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       H:M:S      </code></td>
<td><code dir="ltr" translate="no">       HOUR TO SECOND      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '16:15:14' HOUR TO SECOND      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       M:S      </code></td>
<td><code dir="ltr" translate="no">       MINUTE TO SECOND      </code></td>
<td><code dir="ltr" translate="no">       INTERVAL '15:14' MINUTE TO SECOND      </code></td>
</tr>
</tbody>
</table>

For example:

``` text
-- 0 years, 8 months, 20 days, 17 hours, 0 minutes, and 0 seconds (0-8 20 17:0:0)
INTERVAL '8 20 17' MONTH TO HOUR

-- 0 years, 8 months, -20 days, 17 hours, 0 minutes, and 0 seconds (0-8 -20 17:0:0)
INTERVAL '8 -20 17' MONTH TO HOUR
```

For additional examples, see [Interval literals](/spanner/docs/reference/standard-sql/lexical#interval_literal_range) .

#### Interval-supported date and time parts

You can use the following date parts to construct an interval:

  - `  YEAR  ` : Number of years, `  Y  ` .
  - `  QUARTER  ` : Number of quarters; each quarter is converted to `  3  ` months, `  M  ` .
  - `  MONTH  ` : Number of months, `  M  ` . Each `  12  ` months is converted to `  1  ` year.
  - `  WEEK  ` : Number of weeks; Each week is converted to `  7  ` days, `  D  ` .
  - `  DAY  ` : Number of days, `  D  ` .

You can use the following time parts to construct an interval:

  - `  HOUR  ` : Number of hours, `  H  ` .
  - `  MINUTE  ` : Number of minutes, `  M  ` . Each `  60  ` minutes is converted to `  1  ` hour.
  - `  SECOND  ` : Number of seconds, `  S  ` . Each `  60  ` seconds is converted to `  1  ` minute. Can include up to nine fractional digits (nanosecond precision).
  - `  MILLISECOND  ` : Number of milliseconds.
  - `  MICROSECOND  ` : Number of microseconds.
  - `  NANOSECOND  ` : Number of nanoseconds.

## JSON type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       JSON      </code></td>
<td>Represents JSON, a lightweight data-interchange format.</td>
</tr>
</tbody>
</table>

Expect these canonicalization behaviors when creating a value of JSON type:

  - Booleans, strings, and nulls are preserved exactly.
  - Whitespace characters aren't preserved.
  - A JSON value can store integers in the range of -9,223,372,036,854,775,808 (minimum signed 64-bit integer) to 18,446,744,073,709,551,615 (maximum unsigned 64-bit integer) and floating point numbers within a domain of `  FLOAT64  ` .
  - The order of elements in an array is preserved exactly.
  - The order of the members of an object is lexicographically ordered.
  - If an object has duplicate keys, the first key that's found is preserved.
  - Up to 80 levels can be nested.
  - The format of the original string representation of a JSON number may not be preserved.

To learn more about the literal representation of a JSON type, see [JSON literals](/spanner/docs/reference/standard-sql/lexical#json_literals) .

## Numeric types

Numeric types include the following types:

  - `  INT64  `
  - `  NUMERIC  `
  - `  FLOAT32  `
  - `  FLOAT64  `

### Integer type

Integers are numeric values that don't have fractional components.

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Range</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       INT64      </code></td>
<td>-9,223,372,036,854,775,808 to 9,223,372,036,854,775,807</td>
</tr>
</tbody>
</table>

To learn more about the literal representation of an integer type, see [Integer literals](/spanner/docs/reference/standard-sql/lexical#integer_literals) .

### Decimal type

Decimal type values are numeric values with fixed decimal precision and scale. Precision is the number of digits that the number contains. Scale is how many of these digits appear after the decimal point.

This type can represent decimal fractions exactly, and is suitable for financial calculations.

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<thead>
<tr class="header">
<th>Name</th>
<th>Precision, Scale, and Range</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td>Precision: 38<br />
Scale: 9<br />
Minimum value greater than 0 that can be handled: 1e-9<br />
Min: -9.9999999999999999999999999999999999999E+28<br />
Max: 9.9999999999999999999999999999999999999E+28<br />
</td>
</tr>
</tbody>
</table>

To learn more about the literal representation of a `  NUMERIC  ` type, see [`  NUMERIC  ` literals](/spanner/docs/reference/standard-sql/lexical#numeric_literals) .

### Floating point types

Floating point values are approximate numeric values with fractional components.

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       FLOAT32      </code></td>
<td>Single precision (approximate) numeric values.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td>Double precision (approximate) numeric values.</td>
</tr>
</tbody>
</table>

To learn more about the literal representation of a floating point type, see [Floating point literals](/spanner/docs/reference/standard-sql/lexical#floating_point_literals) .

#### Floating point semantics

When working with floating point numbers, there are special non-numeric values that need to be considered: `  NaN  ` and `  +/-inf  `

**Note:** Format the floating point special values as `  Infinity  ` , `  -Infinity  ` , and `  NaN  ` when using the Spanner REST and RPC APIs, as documented in [TypeCode (REST)](../rest/v1/ResultSetMetadata#TypeCode) and [TypeCode (RPC)](../rpc/google.spanner.v1#google.spanner.v1.TypeCode) . The literals `  +inf  ` , `  -inf  ` , and `  nan  ` aren't supported in the Spanner REST and RPC APIs.

Arithmetic operators provide standard IEEE-754 behavior for all finite input values that produce finite output and for all operations for which at least one input is non-finite.

Function calls and operators return an overflow error if the input is finite but the output would be non-finite. If the input contains non-finite values, the output can be non-finite. In general functions don't introduce `  NaN  ` s or `  +/-inf  ` . However, specific functions like `  IEEE_DIVIDE  ` can return non-finite values on finite input. All such cases are noted explicitly in [Mathematical functions](/spanner/docs/reference/standard-sql/mathematical_functions) .

Floating point values are approximations.

  - The binary format used to represent floating point values can only represent a subset of the numbers between the most positive number and most negative number in the value range. This enables efficient handling of a much larger range than would be possible otherwise. Numbers that aren't exactly representable are approximated by utilizing a close value instead. For example, `  0.1  ` can't be represented as an integer scaled by a power of `  2  ` . When this value is displayed as a string, it's rounded to a limited number of digits, and the value approximating `  0.1  ` might appear as `  "0.1"  ` , hiding the fact that the value isn't precise. In other situations, the approximation can be visible.
  - Summation of floating point values might produce surprising results because of [limited precision](https://en.wikipedia.org/wiki/Floating-point_arithmetic#Accuracy_problems) . For example, `  (1e30 + 1) - 1e30 = 0  ` , while `  (1e30 - 1e30) + 1 = 1.0  ` . This is because the floating point value doesn't have enough precision to represent `  (1e30 + 1)  ` , and the result is rounded to `  1e30  ` . This example also shows that the result of the `  SUM  ` aggregate function of floating points values depends on the order in which the values are accumulated. In general, this order isn't deterministic and therefore the result isn't deterministic. Thus, the resulting `  SUM  ` of floating point values might not be deterministic and two executions of the same query on the same tables might produce different results.
  - If the above points are concerning, use a [decimal type](#decimal_types) instead.

##### Mathematical function examples

<table>
<thead>
<tr class="header">
<th>Left Term</th>
<th>Operator</th>
<th>Right Term</th>
<th>Returns</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Any value</td>
<td><code dir="ltr" translate="no">       +      </code></td>
<td><code dir="ltr" translate="no">       NaN      </code></td>
<td><code dir="ltr" translate="no">       NaN      </code></td>
</tr>
<tr class="even">
<td>1.0</td>
<td><code dir="ltr" translate="no">       +      </code></td>
<td><code dir="ltr" translate="no">       +inf      </code></td>
<td><code dir="ltr" translate="no">       +inf      </code></td>
</tr>
<tr class="odd">
<td>1.0</td>
<td><code dir="ltr" translate="no">       +      </code></td>
<td><code dir="ltr" translate="no">       -inf      </code></td>
<td><code dir="ltr" translate="no">       -inf      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       -inf      </code></td>
<td><code dir="ltr" translate="no">       +      </code></td>
<td><code dir="ltr" translate="no">       +inf      </code></td>
<td><code dir="ltr" translate="no">       NaN      </code></td>
</tr>
<tr class="odd">
<td>Maximum <code dir="ltr" translate="no">       FLOAT64      </code> value</td>
<td><code dir="ltr" translate="no">       +      </code></td>
<td>Maximum <code dir="ltr" translate="no">       FLOAT64      </code> value</td>
<td>Overflow error</td>
</tr>
<tr class="even">
<td>Minimum <code dir="ltr" translate="no">       FLOAT64      </code> value</td>
<td><code dir="ltr" translate="no">       /      </code></td>
<td>2.0</td>
<td>0.0</td>
</tr>
<tr class="odd">
<td>1.0</td>
<td><code dir="ltr" translate="no">       /      </code></td>
<td><code dir="ltr" translate="no">       0.0      </code></td>
<td>"Divide by zero" error</td>
</tr>
</tbody>
</table>

Comparison operators provide standard IEEE-754 behavior for floating point input.

##### Comparison operator examples

<table>
<thead>
<tr class="header">
<th>Left Term</th>
<th>Operator</th>
<th>Right Term</th>
<th>Returns</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       NaN      </code></td>
<td><code dir="ltr" translate="no">       =      </code></td>
<td>Any value</td>
<td><code dir="ltr" translate="no">       FALSE      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NaN      </code></td>
<td><code dir="ltr" translate="no">       &lt;      </code></td>
<td>Any value</td>
<td><code dir="ltr" translate="no">       FALSE      </code></td>
</tr>
<tr class="odd">
<td>Any value</td>
<td><code dir="ltr" translate="no">       &lt;      </code></td>
<td><code dir="ltr" translate="no">       NaN      </code></td>
<td><code dir="ltr" translate="no">       FALSE      </code></td>
</tr>
<tr class="even">
<td>-0.0</td>
<td><code dir="ltr" translate="no">       =      </code></td>
<td>0.0</td>
<td><code dir="ltr" translate="no">       TRUE      </code></td>
</tr>
<tr class="odd">
<td>-0.0</td>
<td><code dir="ltr" translate="no">       &lt;      </code></td>
<td>0.0</td>
<td><code dir="ltr" translate="no">       FALSE      </code></td>
</tr>
</tbody>
</table>

For more information on how these values are ordered and grouped so they can be compared, see [Ordering floating point values](#orderable_floating_points) .

## Protocol buffer type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td>An instance of protocol buffer.</td>
</tr>
</tbody>
</table>

Protocol buffers provide structured data types with a defined serialization format and cross-language support libraries. Protocol buffer message types can contain optional, required, or repeated fields, including nested messages. For more information, see the [Protocol Buffers Developer Guide](https://developers.google.com/protocol-buffers/docs/overview) .

Protocol buffer message types behave similarly to [struct types](#struct_type) , and support similar operations like reading field values by name. Protocol buffer types are always named types, and can be referred to by their fully-qualified protocol buffer name (i.e. `  package.ProtoName  ` ). Protocol buffers support some additional behavior beyond structs, like default field values, defining a column type, and checking for the presence of optional fields.

Protocol buffer [enum types](#enum_type) are also available and can be referenced using the fully-qualified enum type name.

To learn more about using protocol buffers in GoogleSQL, see [Work with protocol buffers](/spanner/docs/reference/standard-sql/protocol-buffers) .

### Constructing a protocol buffer

You can construct a protocol buffer using the [`  NEW  `](/spanner/docs/reference/standard-sql/operators#new_operator) operator or the [`  SELECT AS typename  `](/spanner/docs/reference/standard-sql/query-syntax#select_as_typename) statement. Regardless of the method that you choose, the resulting protocol buffer is the same.

#### `     NEW protocol_buffer {...}    `

You can create a protocol buffer using the [`  NEW  `](/spanner/docs/reference/standard-sql/operators#new_operator) operator with a map constructor:

``` text
NEW protocol_buffer {
  field_name: literal_or_expression
  field_name { ... }
  repeated_field_name: [literal_or_expression, ... ]
}
```

Where:

  - `  protocol_buffer  ` : The full protocol buffer name including the package name.
  - `  field_name  ` : The name of a field.
  - `  literal_or_expression  ` : The field value.

**Example**

``` text
NEW googlesql.examples.astronomy.Planet {
  planet_name: 'Jupiter'
  facts: {
    length_of_day: 9.93
    distance_to_sun: 5.2 * ASTRONOMICAL_UNIT
    has_rings: TRUE
  }
  major_moons: [
    { moon_name: 'Io' },
    { moon_name: 'Europa' },
    { moon_name: 'Ganymede' },
    { moon_name: 'Callisto'}
  ]
  minor_moons: (
    SELECT ARRAY_AGG(moon_name)
    FROM SolarSystemMoons
    WHERE
      planet_name = 'Jupiter'
      AND circumference < 3121
  )
  count_of_space_probe_photos: (
    GALILEO_PHOTOS
    + JUNO_PHOTOS
    + NEW_HORIZONS_PHOTOS
    + CASSINI_PHOTOS
    + ULYSSES_PHOTOS
    + VOYAGER_1_PHOTOS
    + VOYAGER_2_PHOTOS
    + PIONEER_10_PHOTOS
    + PIONEER_11_PHOTOS
  )
}
```

When using this syntax, the following rules apply:

  - The field values must be expressions that are implicitly coercible or literal-coercible to the type of the corresponding protocol buffer field.
  - Commas between fields are optional.
  - A colon is required between field name and values unless the value is a map constructor.
  - The `  NEW protocol_buffer  ` prefix is optional if the protocol buffer type can be inferred from the context.
  - The type of submessages inside the map constructor can be inferred.

**Examples**

Simple:

``` text
SELECT
  key,
  name,
  NEW googlesql.examples.music.Chart { rank: 1 chart_name: '2' }
```

Nested messages and arrays:

``` text
SELECT
  NEW googlesql.examples.music.Album {
    album_name: 'New Moon'
    singer {
      nationality: 'Canadian'
      residence: [ { city: 'Victoria' }, { city: 'Toronto' } ]
    }
    song: ['Sandstorm', 'Wait']
  }
```

Non-literal expressions as values:

``` text
SELECT
  NEW googlesql.examples.music.Chart {
    rank: (SELECT COUNT(*) FROM TableName WHERE foo = 'bar')
    chart_name: CONCAT('best', 'hits')
  }
```

The following examples infers the protocol buffer data type from context:

  - From `  ARRAY  ` constructor:
    
    ``` text
    SELECT
      ARRAY<googlesql.examples.music.Chart>[
        { rank: 1 chart_name: '2' },
        { rank: 2 chart_name: '3' }]
    ```

  - From `  STRUCT  ` constructor:
    
    ``` text
    SELECT
      STRUCT<STRING, googlesql.examples.music.Chart, INT64>(
        'foo', { rank: 1 chart_name: '2' }, 7)[1]
    ```

  - From column names through `  SET  ` :
    
      - Simple column:
    
    <!-- end list -->
    
    ``` text
    UPDATE TableName SET proto_column = { rank: 1 chart_name: '2' }
    ```
    
      - Array column:
    
    <!-- end list -->
    
    ``` text
    UPDATE TableName
    SET proto_array_column = [
      { rank: 1 chart_name: '2' }, { rank: 2 chart_name: '3' }]
    ```

  - From generated column names in `  CREATE  ` :
    
    ``` text
    CREATE TABLE TableName (
      proto_column googlesql.examples.music.Chart  AS (
        { rank: 1 chart_name: '2' }))
    ```

  - From column names in default values in `  CREATE  ` :
    
    ``` text
    CREATE TABLE TableName(
      proto_column googlesql.examples.music.Chart DEFAULT (
        { rank: 1 chart_name: '2' }))
    ```

#### `     NEW protocol_buffer (...)    `

You can create a protocol buffer using the [`  NEW  `](/spanner/docs/reference/standard-sql/operators#new_operator) operator with a parenthesized list of arguments and aliases to specify field names:

``` text
NEW protocol_buffer(field [AS alias], ...)
```

**Example**

``` text
SELECT
  key,
  name,
  NEW googlesql.examples.music.Chart(key AS rank, name AS chart_name)
FROM
  (SELECT 1 AS key, "2" AS name);
```

When using this syntax, the following rules apply:

  - All field expressions must have an [explicit alias](/spanner/docs/reference/standard-sql/query-syntax#explicit_alias_syntax) or end with an identifier. For example, the expression `  a.b.c  ` has the [implicit alias](/spanner/docs/reference/standard-sql/query-syntax#implicit_aliases) `  c  ` .
  - `  NEW  ` matches fields by alias to the field names of the protocol buffer. Aliases must be unique.
  - The expressions must be implicitly coercible or literal-coercible to the type of the corresponding protocol buffer field.

#### `     SELECT AS typename    `

The [`  SELECT AS typename  `](/spanner/docs/reference/standard-sql/query-syntax#select_as_typename) statement can produce a value table where the row type is a specific named protocol buffer type.

### Limited comparisons for protocol buffer values

Direct comparison of protocol buffers isn't supported. There are a few alternative solutions:

  - One way to compare protocol buffers is to do a pair-wise comparison between the fields of the protocol buffers. This can also be used to `  GROUP BY  ` or `  ORDER BY  ` protocol buffer fields.
  - To get a simple approximation comparison, cast protocol buffer to string. This applies lexicographical ordering for numeric fields.

## String type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Variable-length character (Unicode) data.</td>
</tr>
</tbody>
</table>

Input string values must be UTF-8 encoded and output string values will be UTF-8 encoded. Alternate encodings like CESU-8 and Modified UTF-8 aren't treated as valid UTF-8.

All functions and operators that act on string values operate on Unicode characters rather than bytes. For example, functions like `  SUBSTR  ` and `  LENGTH  ` applied to string input count the number of characters, not bytes.

Each Unicode character has a numeric value called a code point assigned to it. Lower code points are assigned to lower characters. When characters are compared, the code points determine which characters are less than or greater than other characters.

Most functions on strings are also defined on bytes. The bytes version operates on raw bytes rather than Unicode characters. Strings and bytes are separate types that can't be used interchangeably. There is no implicit casting in either direction. Explicit casting between string and bytes does UTF-8 encoding and decoding. Casting bytes to string returns an error if the bytes aren't valid UTF-8.

To learn more about the literal representation of a string type, see [String literals](/spanner/docs/reference/standard-sql/lexical#string_and_bytes_literals) .

## Struct type

**Note:** See details about using `  STRUCT  ` s in the [SELECT statement](/spanner/docs/reference/standard-sql/query-syntax#using_structs_with_select) and in [subqueries](/spanner/docs/reference/standard-sql/subqueries) .

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT      </code></td>
<td>Container of ordered fields each with a type (required) and field name (optional).</td>
</tr>
</tbody>
</table>

To learn more about the literal representation of a struct type, see [Struct literals](/spanner/docs/reference/standard-sql/lexical#struct_literals) .

### Declaring a struct type

``` text
STRUCT<T>
```

Struct types are declared using the angle brackets ( `  <  ` and `  >  ` ). The type of the elements of a struct can be arbitrarily complex.

**Examples**

Type Declaration

Meaning

`  STRUCT<INT64>  `

Simple struct with a single unnamed 64-bit integer field.

`  STRUCT<x STRUCT<y INT64, z INT64>>  `

A struct with a nested struct named `  x  ` inside it. The struct `  x  ` has two fields, `  y  ` and `  z  ` , both of which are 64-bit integers.

`  STRUCT<inner_array ARRAY<INT64>>  `

A struct containing an array named `  inner_array  ` that holds 64-bit integer elements.

### Constructing a struct

#### Tuple syntax

``` text
(expr1, expr2 [, ... ])
```

The output type is an anonymous struct type with anonymous fields with types matching the types of the input expressions. There must be at least two expressions specified. Otherwise this syntax is indistinguishable from an expression wrapped with parentheses.

**Examples**

<table>
<thead>
<tr class="header">
<th>Syntax</th>
<th>Output Type</th>
<th>Notes</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       (x, x+y)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;?,?&gt;      </code></td>
<td>If column names are used (unquoted strings), the struct field data type is derived from the column data type. <code dir="ltr" translate="no">       x      </code> and <code dir="ltr" translate="no">       y      </code> are columns, so the data types of the struct fields are derived from the column types and the output type of the addition operator.</td>
</tr>
</tbody>
</table>

This syntax can also be used with struct comparison for comparison expressions using multi-part keys, e.g., in a `  WHERE  ` clause:

``` text
WHERE (Key1,Key2) IN ( (12,34), (56,78) )
```

#### Typeless struct syntax

``` text
STRUCT( expr1 [AS field_name] [, ... ])
```

Duplicate field names are allowed. Fields without names are considered anonymous fields and can't be referenced by name. struct values can be `  NULL  ` , or can have `  NULL  ` field values.

**Examples**

<table>
<thead>
<tr class="header">
<th>Syntax</th>
<th>Output Type</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT(1,2,3)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;int64,int64,int64&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT()      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;&gt;      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT('abc')      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;string&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT(1, t.str_col)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;int64, str_col string&gt;      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT(1 AS a, 'abc' AS b)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;a int64, b string&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT(str_col AS abc)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;abc string&gt;      </code></td>
</tr>
</tbody>
</table>

#### Typed struct syntax

``` text
STRUCT<[field_name] field_type, ...>( expr1 [, ... ])
```

Typed syntax allows constructing structs with an explicit struct data type. The output type is exactly the `  field_type  ` provided. The input expression is coerced to `  field_type  ` if the two types aren't the same, and an error is produced if the types aren't compatible. `  AS alias  ` isn't allowed on the input expressions. The number of expressions must match the number of fields in the type, and the expression types must be coercible or literal-coercible to the field types.

**Examples**

<table>
<thead>
<tr class="header">
<th>Syntax</th>
<th>Output Type</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT&lt;int64&gt;(5)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;int64&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT&lt;date&gt;("2011-05-05")      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;date&gt;      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT&lt;x int64, y string&gt;(1, t.str_col)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;x int64, y string&gt;      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT&lt;int64&gt;(int_col)      </code></td>
<td><code dir="ltr" translate="no">       STRUCT&lt;int64&gt;      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT&lt;x int64&gt;(5 AS x)      </code></td>
<td>Error - Typed syntax doesn't allow <code dir="ltr" translate="no">       AS      </code></td>
</tr>
</tbody>
</table>

### Limited comparisons for structs

Structs can be directly compared using equality operators:

  - Equal ( `  =  ` )
  - Not Equal ( `  !=  ` or `  <>  ` )
  - \[ `  NOT  ` \] `  IN  `

Notice, though, that these direct equality comparisons compare the fields of the struct pairwise in ordinal order ignoring any field names. If instead you want to compare identically named fields of a struct, you can compare the individual fields directly.

## Timestamp type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Range</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td>0001-01-01 00:00:00 to 9999-12-31 23:59:59.999999999 UTC.</td>
</tr>
</tbody>
</table>

A timestamp value represents an absolute point in time, independent of any time zone or convention such as daylight saving time (DST), with nanosecond precision.

A timestamp is typically represented internally as the number of elapsed nanoseconds since a fixed initial point in time.

Note that a timestamp itself doesn't have a time zone; it represents the same instant in time globally. However, the *display* of a timestamp for human readability usually includes a Gregorian date, a time, and a time zone, in an implementation-dependent format. For example, the displayed values "2020-01-01 00:00:00 UTC", "2019-12-31 19:00:00 America/New\_York", and "2020-01-01 05:30:00 Asia/Kolkata" all represent the same instant in time and therefore represent the same timestamp value.

  - To represent a Gregorian date as it might appear on a calendar (a civil date), use a [date](#date_type) value.

##### Canonical format

#### For Rest and RPC APIs

Follow the rules for encoding to and decoding from JSON values as described in [TypeCode (RPC)](../rest/v1/ResultSetMetadata#TypeCode) and [TypeCode (REST)](../rpc/google.spanner.v1#google.spanner.v1.TypeCode) . In particular, the timestamp value must end with an uppercase literal "Z" to specify Zulu time (UTC-0).

For example:

``` text
2014-09-27T12:30:00.45Z
```

Timestamp values must be expressed in Zulu time and can't include a UTC offset. For example, the following timestamp isn't supported:

``` text
-- NOT SUPPORTED! TIMESTAMPS CANNOT INCLUDE A UTC OFFSET WHEN USED WITH THE REST AND RPC APIS
2014-09-27 12:30:00.45-8:00
```

#### For client libraries

Use the language-specific timestamp format.

#### For SQL queries

The canonical format for a timestamp literal has the following parts:

``` text
{
  civil_date_part[time_part [time_zone]] |
  civil_date_part[time_part[time_zone_offset]] |
  civil_date_part[time_part[utc_time_zone]]
}

civil_date_part:
    YYYY-[M]M-[D]D

time_part:
    { |T|t}[H]H:[M]M:[S]S[.F]
```

  - `  YYYY  ` : Four-digit year.
  - `  [M]M  ` : One or two digit month.
  - `  [D]D  ` : One or two digit day.
  - `  { |T|t}  ` : A space or a `  T  ` or `  t  ` separator. The `  T  ` and `  t  ` separators are flags for time.
  - `  [H]H  ` : One or two digit hour (valid values from 00 to 23).
  - `  [M]M  ` : One or two digit minutes (valid values from 00 to 59).
  - `  [S]S  ` : One or two digit seconds (valid values from 00 to 60).
  - `  [.F]  ` : Up to nine fractional digits (nanosecond precision).
  - `  [time_zone]  ` : String representing the time zone. When a time zone isn't explicitly specified, the default time zone, America/Los\_Angeles, is used. For details, see [time zones](#time_zones) .
  - `  [time_zone_offset]  ` : String representing the offset from the Coordinated Universal Time (UTC) time zone. For details, see [time zones](#time_zones) .
  - `  [utc_time_zone]  ` : String representing the Coordinated Universal Time (UTC), usually the letter `  Z  ` or `  z  ` . For details, see [time zones](#time_zones) .

To learn more about the literal representation of a timestamp type, see [Timestamp literals](/spanner/docs/reference/standard-sql/lexical#timestamp_literals) .

### Time zones

A time zone is used when converting from a civil date or time (as might appear on a calendar or clock) to a timestamp (an absolute time), or vice versa. This includes the operation of parsing a string containing a civil date and time like "2020-01-01 00:00:00" and converting it to a timestamp. The resulting timestamp value itself doesn't store a specific time zone, because it represents one instant in time globally.

Time zones are represented by strings in one of these canonical formats:

  - Offset from Coordinated Universal Time (UTC), or the letter `  Z  ` or `  z  ` for UTC.
  - Time zone name from the [tz database](http://www.iana.org/time-zones) .

The following timestamps are identical because the time zone offset for `  America/Los_Angeles  ` is `  -08  ` for the specified date and time.

``` text
SELECT UNIX_MILLIS(TIMESTAMP '2008-12-25 15:30:00 America/Los_Angeles') AS millis;
```

``` text
SELECT UNIX_MILLIS(TIMESTAMP '2008-12-25 15:30:00-08:00') AS millis;
```

#### Specify Coordinated Universal Time (UTC)

You can specify UTC using the following suffix:

``` text
{Z|z}
```

You can also specify UTC using the following time zone name:

``` text
{Etc/UTC}
```

The `  Z  ` suffix is a placeholder that implies UTC when converting an [RFC 3339-format](https://datatracker.ietf.org/doc/html/rfc3339#page-10) value to a `  TIMESTAMP  ` value. The value `  Z  ` isn't a valid time zone for functions that accept a time zone. If you're specifying a time zone, or you're unsure of the format to use to specify UTC, we recommend using the `  Etc/UTC  ` time zone name.

The `  Z  ` suffix isn't case sensitive. When using the `  Z  ` suffix, no space is allowed between the `  Z  ` and the rest of the timestamp. The following are examples of using the `  Z  ` suffix and the `  Etc/UTC  ` time zone name:

``` text
SELECT TIMESTAMP '2014-09-27T12:30:00.45Z'
SELECT TIMESTAMP '2014-09-27 12:30:00.45z'
SELECT TIMESTAMP '2014-09-27T12:30:00.45 Etc/UTC'
```

#### Specify an offset from Coordinated Universal Time (UTC)

You can specify the offset from UTC using the following format:

``` text
{+|-}H[H][:M[M]]
```

Examples:

``` text
-08:00
-8:15
+3:00
+07:30
-7
```

When using this format, no space is allowed between the time zone and the rest of the timestamp.

``` text
2014-09-27 12:30:00.45-8:00
```

#### Time zone name

Format:

``` text
tz_identifier
```

A time zone name is a tz identifier from the [tz database](http://www.iana.org/time-zones) . For a less comprehensive but simpler reference, see the [List of tz database time zones](http://en.wikipedia.org/wiki/List_of_tz_database_time_zones) on Wikipedia.

Examples:

``` text
America/Los_Angeles
America/Argentina/Buenos_Aires
Etc/UTC
Pacific/Auckland
```

When using a time zone name, a space is required between the name and the rest of the timestamp:

``` text
2014-09-27 12:30:00.45 America/Los_Angeles
```

Note that not all time zone names are interchangeable even if they do happen to report the same time during a given part of the year. For example, `  America/Los_Angeles  ` reports the same time as `  UTC-7:00  ` during daylight saving time (DST), but reports the same time as `  UTC-8:00  ` outside of DST.

If a time zone isn't specified, the default time zone value is used.

#### Leap seconds

A timestamp is simply an offset from 1970-01-01 00:00:00 UTC, assuming there are exactly 60 seconds per minute. Leap seconds aren't represented as part of a stored timestamp.

If the input contains values that use ":60" in the seconds field to represent a leap second, that leap second isn't preserved when converting to a timestamp value. Instead that value is interpreted as a timestamp with ":00" in the seconds field of the following minute.

Leap seconds don't affect timestamp computations. All timestamp computations are done using Unix-style timestamps, which don't reflect leap seconds. Leap seconds are only observable through functions that measure real-world time. In these functions, it's possible for a timestamp second to be skipped or repeated when there is a leap second.

#### Daylight saving time

A timestamp is unaffected by daylight saving time (DST) because it represents a point in time. When you display a timestamp as a civil time, with a timezone that observes DST, the following rules apply:

  - During the transition from standard time to DST, one hour is skipped. A civil time from the skipped hour is treated the same as if it were written an hour later. For example, in the `  America/Los_Angeles  ` time zone, the hour between 2 AM and 3 AM on March 10, 2024 is skipped on a clock. The times 2:30 AM and 3:30 AM on that date are treated as the same point in time:
    
    ``` text
    SELECT
    FORMAT_TIMESTAMP("%c %Z", "2024-03-10 02:30:00 America/Los_Angeles", "UTC") AS two_thirty,
    FORMAT_TIMESTAMP("%c %Z", "2024-03-10 03:30:00 America/Los_Angeles", "UTC") AS three_thirty;
    
    /*------------------------------+------------------------------+
     | two_thirty                   | three_thirty                 |
     +------------------------------+------------------------------+
     | Sun Mar 10 10:30:00 2024 UTC | Sun Mar 10 10:30:00 2024 UTC |
     +------------------------------+------------------------------*/
    ```

  - When there's ambiguity in how to represent a civil time in a particular timezone because of DST, the later time is chosen:
    
    ``` text
    SELECT
    FORMAT_TIMESTAMP("%c %Z", "2024-03-10 10:30:00 UTC", "America/Los_Angeles") as ten_thirty;
    
    /*--------------------------------+
     | ten_thirty                     |
     +--------------------------------+
     | Sun Mar 10 03:30:00 2024 UTC-7 |
     +--------------------------------*/
    ```

  - During the transition from DST to standard time, one hour is repeated. A civil time that shows a time during that hour is treated as if it's the earlier instance of that time. For example, in the `  America/Los_Angeles  ` time zone, the hour between 1 AM and 2 AM on November 3, 2024, is repeated on a clock. The time 1:30 AM on that date is treated as the earlier (DST) instance of that time.
    
    ``` text
    SELECT
    FORMAT_TIMESTAMP("%c %Z", "2024-11-03 01:30:00 America/Los_Angeles", "UTC") as one_thirty,
    FORMAT_TIMESTAMP("%c %Z", "2024-11-03 02:30:00 America/Los_Angeles", "UTC") as two_thirty;
    
    /*------------------------------+------------------------------+
     | one_thirty                   | two_thirty                   |
     +------------------------------+------------------------------+
     | Sun Nov 3 08:30:00 2024 UTC  | Sun Nov 3 10:30:00 2024 UTC  |
     +------------------------------+------------------------------*/
    ```

## UUID type

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       UUID      </code></td>
<td>A universally unique identifier (UUID) represented as a 128-bit number.</td>
</tr>
</tbody>
</table>

The following ASCII string format of lowercase hexadecimal digits is used to represent a UUID:

`  [8 digits]-[4 digits]-[4 digits]-[4 digits]-[12 digits]  `

**Example**

`  f81d4fae-7dec-11d0-a765-00a0c91e6bf6  `

### Cast a UUID to a string

You can cast a UUID to a string by using the following syntax:

``` text
  SELECT CAST(NEW_UUID() AS STRING) AS UUID_STR;
```

You can also cast a string to a UUID, either explicitly or by using an implicit coercion of a literal or parameter.

**Examples**

``` text
  SELECT UUID_id >= CAST("00000000-0000-0000-0000-000000000000" AS UUID) FROM T1;
```

``` text
  SELECT UUID_id >= "00000000-0000-0000-0000-000000000000" FROM T1;
```

### Cast a UUID to bytes

You can cast a UUID to bytes by using the following syntax:

``` text
  SELECT CAST(NEW_UUID() AS BYTES) AS UUID_BYTES;
```

You can also explicitly cast bytes to a UUID. Unlike strings, bytes can't be implicitly coerced to a UUID.

##### Comparison operator examples

The comparison operator compares UUIDs using their internal representation. However, the result is presented as if the comparison were performed on the 36-character lowercase ASCII string representation of the UUIDs, using lexicographical order.

<table>
<thead>
<tr class="header">
<th>Left term</th>
<th>Operator</th>
<th>Right term</th>
<th>Returns</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Any value</td>
<td><code dir="ltr" translate="no">       =      </code></td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NULL      </code></td>
<td><code dir="ltr" translate="no">       &lt;      </code></td>
<td>Any value</td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
</tr>
<tr class="odd">
<td>00000000-0000-0000-0000-000000000000</td>
<td><code dir="ltr" translate="no">       &lt;      </code></td>
<td>ffffffff-ffff-ffff-ffff-ffffffffffff</td>
<td><code dir="ltr" translate="no">       TRUE      </code></td>
</tr>
<tr class="even">
<td>00000000-0000-0000-0000-000000000000</td>
<td><code dir="ltr" translate="no">       =      </code></td>
<td>00000000-0000-0000-0000-000000000000</td>
<td><code dir="ltr" translate="no">       TRUE      </code></td>
</tr>
<tr class="odd">
<td>00000000-0000-0000-0000-000000000000</td>
<td><code dir="ltr" translate="no">       &gt;      </code></td>
<td>ffffffff-ffff-ffff-ffff-ffffffffffff</td>
<td><code dir="ltr" translate="no">       FALSE      </code></td>
</tr>
</tbody>
</table>

**Example**

``` text
  SELECT NEW_UUID() >= "00000000-0000-0000-0000-000000000000" AS Is_GE;

/*-------+
 | Is_GE |
 +-------+
 | true  |
 +-------*/
```
