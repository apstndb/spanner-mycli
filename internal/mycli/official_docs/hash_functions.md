GoogleSQL for Spanner supports the following hash functions.

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
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#farm_fingerprint"><code dir="ltr" translate="no">        FARM_FINGERPRINT       </code></a></td>
<td>Computes the fingerprint of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the FarmHash Fingerprint64 algorithm.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#sha1"><code dir="ltr" translate="no">        SHA1       </code></a></td>
<td>Computes the hash of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the SHA-1 algorithm.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#sha256"><code dir="ltr" translate="no">        SHA256       </code></a></td>
<td>Computes the hash of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the SHA-256 algorithm.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#sha512"><code dir="ltr" translate="no">        SHA512       </code></a></td>
<td>Computes the hash of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the SHA-512 algorithm.</td>
</tr>
</tbody>
</table>

## `     FARM_FINGERPRINT    `

``` text
FARM_FINGERPRINT(value)
```

**Description**

Computes the fingerprint of the `  STRING  ` or `  BYTES  ` input using the `  Fingerprint64  ` function from the [open-source FarmHash library](https://github.com/google/farmhash) . The output of this function for a particular input will never change.

**Return type**

INT64

**Examples**

``` text
WITH example AS (
  SELECT 1 AS x, "foo" AS y, true AS z UNION ALL
  SELECT 2 AS x, "apple" AS y, false AS z UNION ALL
  SELECT 3 AS x, "" AS y, true AS z
)
SELECT
  *,
  FARM_FINGERPRINT(CONCAT(CAST(x AS STRING), y, CAST(z AS STRING)))
    AS row_fingerprint
FROM example;
/*---+-------+-------+----------------------+
 | x | y     | z     | row_fingerprint      |
 +---+-------+-------+----------------------+
 | 1 | foo   | true  | -1541654101129638711 |
 | 2 | apple | false | 2794438866806483259  |
 | 3 |       | true  | -4880158226897771312 |
 +---+-------+-------+----------------------*/
```

## `     SHA1    `

``` text
SHA1(input)
```

**Description**

Computes the hash of the input using the [SHA-1 algorithm](https://en.wikipedia.org/wiki/SHA-1) . The input can either be `  STRING  ` or `  BYTES  ` . The string version treats the input as an array of bytes.

This function returns 20 bytes.

**Warning:** SHA1 is no longer considered secure. For increased security, use another hashing function.

**Return type**

`  BYTES  `

**Example**

``` text
SELECT SHA1("Hello World") as sha1;

-- Note that the result of SHA1 is of type BYTES, displayed as a base64-encoded string.
/*------------------------------+
 | sha1                         |
 +------------------------------+
 | Ck1VqNd45QIvq3AZd8XYQLvEhtA= |
 +------------------------------*/
```

## `     SHA256    `

``` text
SHA256(input)
```

**Description**

Computes the hash of the input using the [SHA-256 algorithm](https://en.wikipedia.org/wiki/SHA-2) . The input can either be `  STRING  ` or `  BYTES  ` . The string version treats the input as an array of bytes.

This function returns 32 bytes.

**Return type**

`  BYTES  `

**Example**

``` text
SELECT SHA256("Hello World") as sha256;
```

## `     SHA512    `

``` text
SHA512(input)
```

**Description**

Computes the hash of the input using the [SHA-512 algorithm](https://en.wikipedia.org/wiki/SHA-2) . The input can either be `  STRING  ` or `  BYTES  ` . The string version treats the input as an array of bytes.

This function returns 64 bytes.

**Return type**

`  BYTES  `

**Example**

``` text
SELECT SHA512("Hello World") as sha512;
```
