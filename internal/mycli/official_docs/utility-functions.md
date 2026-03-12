GoogleSQL for Spanner supports the following utility functions.

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
<td><a href="/spanner/docs/reference/standard-sql/utility-functions#generate_uuid"><code dir="ltr" translate="no">        GENERATE_UUID       </code></a></td>
<td>Produces a random universally unique identifier (UUID) as a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/utility-functions#new_uuid"><code dir="ltr" translate="no">        NEW_UUID       </code></a></td>
<td>Produces a random universally unique identifier (UUID) as a <code dir="ltr" translate="no">       UUID      </code> value.</td>
</tr>
</tbody>
</table>

## `     GENERATE_UUID    `

``` text
GENERATE_UUID()
```

**Description**

Returns a random universally unique identifier (UUID) as a `  STRING  ` . The returned `  STRING  ` consists of 32 hexadecimal digits in five groups separated by hyphens in the form 8-4-4-4-12. The hexadecimal digits represent 122 random bits and 6 fixed bits, in compliance with [RFC 4122 section 4.4](https://tools.ietf.org/html/rfc4122#section-4.4) . The returned `  STRING  ` is lowercase.

**Return Data Type**

STRING

**Example**

The following query generates a random UUID.

``` text
SELECT GENERATE_UUID() AS uuid;

/*--------------------------------------+
 | uuid                                 |
 +--------------------------------------+
 | 4192bff0-e1e0-43ce-a4db-912808c32493 |
 +--------------------------------------*/
```

## `     NEW_UUID    `

``` text
NEW_UUID()
```

**Description**

Returns a random universally unique identifier (UUID) as a `  UUID  ` . The returned `  UUID  ` consists of 32 hexadecimal digits in five groups separated by hyphens in the form 8-4-4-4-12. The hexadecimal digits represent 122 random bits and 6 fixed bits, in compliance with [RFC 4122 section 4.4](https://tools.ietf.org/html/rfc4122#section-4.4) .

GoogleSQL accepts any number of hyphens with some caveats:

1.  Use single hyphens between hexadecimal numbers, no consecutive hyphens.
2.  Don't start or end a UUID with a hyphen.

**Return Data Type**

UUID

**Example**

The following query generates a random UUID.

``` text
SELECT NEW_UUID() AS uuid;

/*--------------------------------------+
 | uuid                                 |
 +--------------------------------------+
 | 4192bff0-e1e0-43ce-a4db-912808c32493 |
 +--------------------------------------*/
```
