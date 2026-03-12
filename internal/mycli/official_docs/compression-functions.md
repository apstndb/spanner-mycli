GoogleSQL for Spanner supports compression functions.

Compression functions compress or decompress bytes or string values using the Zstandard (Zstd) lossless data compression algorithm.

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
<td><a href="/spanner/docs/reference/standard-sql/compression-functions#zstd_compress"><code dir="ltr" translate="no">        ZSTD_COMPRESS       </code></a></td>
<td>Compresses <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> input into <code dir="ltr" translate="no">       BYTES      </code> output using the <a href="https://en.wikipedia.org/wiki/Zstd">Zstandard (Zstd)</a> lossless data compression algorithm.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/compression-functions#zstd_decompress_to_bytes"><code dir="ltr" translate="no">        ZSTD_DECOMPRESS_TO_BYTES       </code></a></td>
<td>Decompresses <code dir="ltr" translate="no">       BYTES      </code> input into <code dir="ltr" translate="no">       BYTES      </code> output using the <a href="https://en.wikipedia.org/wiki/Zstd">Zstandard (Zstd)</a> lossless data compression algorithm.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/compression-functions#zstd_decompress_to_string"><code dir="ltr" translate="no">        ZSTD_DECOMPRESS_TO_STRING       </code></a></td>
<td>Decompress <code dir="ltr" translate="no">       BYTES      </code> input into <code dir="ltr" translate="no">       STRING      </code> output using the <a href="https://en.wikipedia.org/wiki/Zstd">Zstandard (Zstd)</a> lossless data compression algorithm.</td>
</tr>
</tbody>
</table>

## `     ZSTD_COMPRESS    `

``` text
ZSTD_COMPRESS(string_or_bytes_value, level => 3)
```

**Description**

Compresses `  STRING  ` or `  BYTES  ` input into `  BYTES  ` output using the Zstandard (Zstd) lossless data compression algorithm.

Arguments:

  - `  string_or_bytes_value  ` : The SQL value to compress.
  - `  level  ` : Optional. The Zstd compression level. The default is 3. You can set `  level  ` to an integer value between -5 and 22. A higher value results in a better compression ratio at the cost of slower performance.

**Return type**

`  BYTES  ` : Base64-encoded bytes.

**Example**

``` text
SELECT ZSTD_COMPRESS('string_value') AS result;

/*------------------------------+
 | result                       |
 +------------------------------+
 | KLUv/SAMYQAAc3RyaW5nX3ZhbHVl |
 +------------------------------*/
```

``` text
SELECT ZSTD_COMPRESS(b'bytes_value', level => 1);

/*------------------------------+
 | result                       |
 +------------------------------+
 | KLUv/SALWQAAYnl0ZXNfdmFsdWU= |
 +------------------------------*/
```

This function returns `  NULL  ` if the input is `  NULL  ` :

``` text
SELECT ZSTD_COMPRESS(NULL) AS result;

/*------------+
 | result     |
 +------------+
 | NULL       |
 +------------*/
```

## `     ZSTD_DECOMPRESS_TO_BYTES    `

``` text
ZSTD_DECOMPRESS_TO_BYTES(bytes_value, size_limit => 1024 * 1024 * 1024)
```

**Description**

Decompresses `  BYTES  ` input into `  BYTES  ` using the Zstandard (Zstd) lossless data compression algorithm.

Arguments:

  - `  bytes_value  ` : The bytes to decompress.
  - `  size_limit  ` : Optional. The size limit of returned decompressed bytes. The default value is one GiB. You can set this limit to a lower value to minimize the risk of `  ZSTD_DECOMPRESS_TO_BYTES  ` causing server memory issues.

**Return type**

`  BYTES  ` : Base64-encoded bytes.

**Example**

``` text
SELECT ZSTD_DECOMPRESS_TO_BYTES(ZSTD_COMPRESS(b'bytes')) AS result;

/*------------+
 | result     |
 +------------+
 | Ynl0ZXM=   |
 +------------*/
```

If compressed bytes exceed the `  size_limit  ` value, `  ZSTD_DECOMPRESS_TO_BYTES  ` returns an error:

``` text
SELECT ZSTD_DECOMPRESS_TO_BYTES(ZSTD_COMPRESS(b'bytes'), size_limit => 1) AS result;

Statement failed: ZSTD output is too large: (5 bytes) > limit (1 bytes)
```

This function returns `  NULL  ` if the input is `  NULL  ` :

``` text
SELECT ZSTD_DECOMPRESS_TO_BYTES(NULL) AS result;

/*------------+
 | result     |
 +------------+
 | NULL       |
 +------------*/
```

## `     ZSTD_DECOMPRESS_TO_STRING    `

``` text
ZSTD_DECOMPRESS_TO_STRING(bytes_value, size_limit => 1024 * 1024 * 1024)
```

**Description**

Decompress `  BYTES  ` input into `  STRING  ` output using the Zstandard (Zstd) lossless data compression algorithm.

Arguments:

  - `  bytes_value  ` : The bytes to decompress.
  - `  size_limit  ` : Optional. The size limit of returned decompressed string. The default value is one GiB. You can set this limit to a lower value to minimize the risk of `  ZSTD_DECOMPRESS_TO_STRING  ` causing server memory issues.

**Return type**

`  STRING  `

**Example**

``` text
SELECT ZSTD_DECOMPRESS_TO_STRING(ZSTD_COMPRESS('zstd')) AS result;

/*----------+
 | result   |
 +----------+
 | "zstd"   |
 +----------*/
```

If compressed bytes exceed the `  size_limit  ` value, `  ZSTD_DECOMPRESS_TO_STRING  ` returns an error:

``` text
SELECT ZSTD_DECOMPRESS_TO_STRING(ZSTD_COMPRESS('zstd'), size_limit => 1) AS result;

Statement failed: ZSTD output is too large: (4 bytes) > limit (1 bytes)
```

This function returns `  NULL  ` if the input is `  NULL  ` :

``` text
SELECT ZSTD_DECOMPRESS_TO_STRING(NULL) AS result;

/*------------+
 | result     |
 +------------+
 | NULL       |
 +------------*/
```
