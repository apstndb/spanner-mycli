GoogleSQL for Spanner supports the following bit functions.

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
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_and"><code dir="ltr" translate="no">        BIT_AND       </code></a></td>
<td>Performs a bitwise AND operation on an expression.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/aggregate_functions">Aggregate functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/bit_functions#bit_count"><code dir="ltr" translate="no">        BIT_COUNT       </code></a></td>
<td>Gets the number of bits that are set in an input expression.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_or"><code dir="ltr" translate="no">        BIT_OR       </code></a></td>
<td>Performs a bitwise OR operation on an expression.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/aggregate_functions">Aggregate functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/bit_functions#bit_reverse"><code dir="ltr" translate="no">        BIT_REVERSE       </code></a></td>
<td>Reverses the bits in an integer.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_xor"><code dir="ltr" translate="no">        BIT_XOR       </code></a></td>
<td>Performs a bitwise XOR operation on an expression.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/aggregate_functions">Aggregate functions</a> .</td>
</tr>
</tbody>
</table>

## `     BIT_COUNT    `

``` text
BIT_COUNT(expression)
```

**Description**

The input, `  expression  ` , must be an integer or `  BYTES  ` .

Returns the number of bits that are set in the input `  expression  ` . For signed integers, this is the number of bits in two's complement form.

**Return Data Type**

`  INT64  `

**Example**

``` text
SELECT a, BIT_COUNT(a) AS a_bits, FORMAT("%T", b) as b, BIT_COUNT(b) AS b_bits
FROM UNNEST([
  STRUCT(0 AS a, b'' AS b), (0, b'\x00'), (5, b'\x05'), (8, b'\x00\x08'),
  (0xFFFF, b'\xFF\xFF'), (-2, b'\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFE'),
  (-1, b'\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF'),
  (NULL, b'\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF')
]) AS x;

/*-------+--------+---------------------------------------------+--------+
 | a     | a_bits | b                                           | b_bits |
 +-------+--------+---------------------------------------------+--------+
 | 0     | 0      | b""                                         | 0      |
 | 0     | 0      | b"\x00"                                     | 0      |
 | 5     | 2      | b"\x05"                                     | 2      |
 | 8     | 1      | b"\x00\x08"                                 | 1      |
 | 65535 | 16     | b"\xff\xff"                                 | 16     |
 | -2    | 63     | b"\xff\xff\xff\xff\xff\xff\xff\xfe"         | 63     |
 | -1    | 64     | b"\xff\xff\xff\xff\xff\xff\xff\xff"         | 64     |
 | NULL  | NULL   | b"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" | 80     |
 +-------+--------+---------------------------------------------+--------*/
```

## `     BIT_REVERSE    `

``` text
BIT_REVERSE(value, preserve_sign)
```

**Description**

Takes an integer value and returns its bit-reversed version. When `  preserve_sign  ` is `  TRUE  ` , this function provides the same bit-reversal algorithm used in bit-reversed sequence. For more information, see [Bit-reversed sequence](/spanner/docs/primary-key-default-value#bit-reversed-sequence) .

If the input value is `  NULL  ` , the function returns `  NULL  ` .

Arguments:

  - `  value  ` : The integer to bit reverse. `  Sequence  ` only supports `  INT64  ` .
  - `  preserve_sign  ` : `  TRUE  ` to exclude the sign bit, otherwise `  FALSE  ` .

**Return Data Type**

The same data type as `  value  ` .

**Example**

``` text
SELECT BIT_REVERSE(100, true) AS results

/*---------------------+
 | Results             |
 +---------------------+
 | 1369094286720630784 |
 +---------------------*/
```

``` text
SELECT BIT_REVERSE(100, false) AS results

/*---------------------+
 | Results             |
 +---------------------+
 | 2738188573441261568 |
 +---------------------*/
```

``` text
SELECT BIT_REVERSE(-100, true) AS results

/*----------------------+
 | Results              |
 +----------------------+
 | -7133701809754865665 |
 +----------------------*/
```

``` text
SELECT BIT_REVERSE(-100, false) AS results

/*---------------------+
 | Results             |
 +---------------------+
 | 4179340454199820287 |
 +---------------------*/
```
