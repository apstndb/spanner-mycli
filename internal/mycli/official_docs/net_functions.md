GoogleSQL for Spanner supports the following Net functions.

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
<td><a href="/spanner/docs/reference/standard-sql/net_functions#nethost"><code dir="ltr" translate="no">        NET.HOST       </code></a></td>
<td>Gets the hostname from a URL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_from_string"><code dir="ltr" translate="no">        NET.IP_FROM_STRING       </code></a></td>
<td>Converts an IPv4 or IPv6 address from a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_net_mask"><code dir="ltr" translate="no">        NET.IP_NET_MASK       </code></a></td>
<td>Gets a network mask.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_to_string"><code dir="ltr" translate="no">        NET.IP_TO_STRING       </code></a></td>
<td>Converts an IPv4 or IPv6 address from a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order to a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_trunc"><code dir="ltr" translate="no">        NET.IP_TRUNC       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> IPv4 or IPv6 address in network byte order to a <code dir="ltr" translate="no">       BYTES      </code> subnet address.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netipv4_from_int64"><code dir="ltr" translate="no">        NET.IPV4_FROM_INT64       </code></a></td>
<td>Converts an IPv4 address from an <code dir="ltr" translate="no">       INT64      </code> value to a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netipv4_to_int64"><code dir="ltr" translate="no">        NET.IPV4_TO_INT64       </code></a></td>
<td>Converts an IPv4 address from a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order to an <code dir="ltr" translate="no">       INT64      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netpublic_suffix"><code dir="ltr" translate="no">        NET.PUBLIC_SUFFIX       </code></a></td>
<td>Gets the public suffix from a URL.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netreg_domain"><code dir="ltr" translate="no">        NET.REG_DOMAIN       </code></a></td>
<td>Gets the registered or registrable domain from a URL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netsafe_ip_from_string"><code dir="ltr" translate="no">        NET.SAFE_IP_FROM_STRING       </code></a></td>
<td>Similar to the <code dir="ltr" translate="no">       NET.IP_FROM_STRING      </code> , but returns <code dir="ltr" translate="no">       NULL      </code> instead of producing an error if the input is invalid.</td>
</tr>
</tbody>
</table>

## `     NET.HOST    `

``` text
NET.HOST(url)
```

**Description**

Takes a URL as a `  STRING  ` value and returns the host. For best results, URL values should comply with the format as defined by [RFC 3986](https://tools.ietf.org/html/rfc3986#appendix-A) . If the URL value doesn't comply with RFC 3986 formatting, this function makes a best effort to parse the input and return a relevant result. If the function can't parse the input, it returns `  NULL  ` .

**Note:** The function doesn't perform any normalization.

**Return Data Type**

`  STRING  `

**Example**

``` text
SELECT
  FORMAT("%T", input) AS input,
  description,
  FORMAT("%T", NET.HOST(input)) AS host,
  FORMAT("%T", NET.PUBLIC_SUFFIX(input)) AS suffix,
  FORMAT("%T", NET.REG_DOMAIN(input)) AS domain
FROM (
  SELECT "" AS input, "invalid input" AS description
  UNION ALL SELECT "http://abc.xyz", "standard URL"
  UNION ALL SELECT "//user:password@a.b:80/path?query",
                   "standard URL with relative scheme, port, path and query, but no public suffix"
  UNION ALL SELECT "https://[::1]:80", "standard URL with IPv6 host"
  UNION ALL SELECT "http://例子.卷筒纸.中国", "standard URL with internationalized domain name"
  UNION ALL SELECT "    www.Example.Co.UK    ",
                   "non-standard URL with spaces, upper case letters, and without scheme"
  UNION ALL SELECT "mailto:?to=&subject=&body=", "URI rather than URL--unsupported"
);
```

<table>
<thead>
<tr class="header">
<th>input</th>
<th>description</th>
<th>host</th>
<th>suffix</th>
<th>domain</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>""</td>
<td>invalid input</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>"http://abc.xyz"</td>
<td>standard URL</td>
<td>"abc.xyz"</td>
<td>"xyz"</td>
<td>"abc.xyz"</td>
</tr>
<tr class="odd">
<td>"//user:password@a.b:80/path?query"</td>
<td>standard URL with relative scheme, port, path and query, but no public suffix</td>
<td>"a.b"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>"https://[::1]:80"</td>
<td>standard URL with IPv6 host</td>
<td>"[::1]"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>"http://例子.卷筒纸.中国"</td>
<td>standard URL with internationalized domain name</td>
<td>"例子.卷筒纸.中国"</td>
<td>"中国"</td>
<td>"卷筒纸.中国"</td>
</tr>
<tr class="even">
<td>" www.Example.Co.UK "</td>
<td>non-standard URL with spaces, upper case letters, and without scheme</td>
<td>"www.Example.Co.UK"</td>
<td>"Co.UK"</td>
<td>"Example.Co.UK"</td>
</tr>
<tr class="odd">
<td>"mailto:?to=&amp;subject=&amp;body="</td>
<td>URI rather than URL--unsupported</td>
<td>"mailto"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

## `     NET.IP_FROM_STRING    `

``` text
NET.IP_FROM_STRING(addr_str)
```

**Description**

Converts an IPv4 or IPv6 address from text (STRING) format to binary (BYTES) format in network byte order.

This function supports the following formats for `  addr_str  ` :

  - IPv4: Dotted-quad format. For example, `  10.1.2.3  ` .
  - IPv6: Colon-separated format. For example, `  1234:5678:90ab:cdef:1234:5678:90ab:cdef  ` . For more examples, see the [IP Version 6 Addressing Architecture](http://www.ietf.org/rfc/rfc2373.txt) .

This function doesn't support [CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing) , such as `  10.1.2.3/32  ` .

If this function receives a `  NULL  ` input, it returns `  NULL  ` . If the input is considered invalid, an `  OUT_OF_RANGE  ` error occurs.

**Return Data Type**

BYTES

**Example**

``` text
SELECT
  addr_str, FORMAT("%T", NET.IP_FROM_STRING(addr_str)) AS ip_from_string
FROM UNNEST([
  '48.49.50.51',
  '::1',
  '3031:3233:3435:3637:3839:4041:4243:4445',
  '::ffff:192.0.2.128'
]) AS addr_str;

/*---------------------------------------------------------------------------------------------------------------+
 | addr_str                                | ip_from_string                                                      |
 +---------------------------------------------------------------------------------------------------------------+
 | 48.49.50.51                             | b"0123"                                                             |
 | ::1                                     | b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01" |
 | 3031:3233:3435:3637:3839:4041:4243:4445 | b"0123456789@ABCDE"                                                 |
 | ::ffff:192.0.2.128                      | b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xc0\x00\x02\x80" |
 +---------------------------------------------------------------------------------------------------------------*/
```

## `     NET.IP_NET_MASK    `

``` text
NET.IP_NET_MASK(num_output_bytes, prefix_length)
```

**Description**

Returns a network mask: a byte sequence with length equal to `  num_output_bytes  ` , where the first `  prefix_length  ` bits are set to 1 and the other bits are set to 0. `  num_output_bytes  ` and `  prefix_length  ` are INT64. This function throws an error if `  num_output_bytes  ` isn't 4 (for IPv4) or 16 (for IPv6). It also throws an error if `  prefix_length  ` is negative or greater than `  8 * num_output_bytes  ` .

**Return Data Type**

BYTES

**Example**

``` text
SELECT x, y, FORMAT("%T", NET.IP_NET_MASK(x, y)) AS ip_net_mask
FROM UNNEST([
  STRUCT(4 as x, 0 as y),
  (4, 20),
  (4, 32),
  (16, 0),
  (16, 1),
  (16, 128)
]);

/*--------------------------------------------------------------------------------+
 | x  | y   | ip_net_mask                                                         |
 +--------------------------------------------------------------------------------+
 | 4  | 0   | b"\x00\x00\x00\x00"                                                 |
 | 4  | 20  | b"\xff\xff\xf0\x00"                                                 |
 | 4  | 32  | b"\xff\xff\xff\xff"                                                 |
 | 16 | 0   | b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" |
 | 16 | 1   | b"\x80\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" |
 | 16 | 128 | b"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" |
 +--------------------------------------------------------------------------------*/
```

## `     NET.IP_TO_STRING    `

``` text
NET.IP_TO_STRING(addr_bin)
```

**Description** Converts an IPv4 or IPv6 address from binary (BYTES) format in network byte order to text (STRING) format.

If the input is 4 bytes, this function returns an IPv4 address as a STRING. If the input is 16 bytes, it returns an IPv6 address as a STRING.

If this function receives a `  NULL  ` input, it returns `  NULL  ` . If the input has a length different from 4 or 16, an `  OUT_OF_RANGE  ` error occurs.

**Return Data Type**

STRING

**Example**

``` text
SELECT FORMAT("%T", x) AS addr_bin, NET.IP_TO_STRING(x) AS ip_to_string
FROM UNNEST([
  b"0123",
  b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01",
  b"0123456789@ABCDE",
  b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xc0\x00\x02\x80"
]) AS x;

/*---------------------------------------------------------------------------------------------------------------+
 | addr_bin                                                            | ip_to_string                            |
 +---------------------------------------------------------------------------------------------------------------+
 | b"0123"                                                             | 48.49.50.51                             |
 | b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01" | ::1                                     |
 | b"0123456789@ABCDE"                                                 | 3031:3233:3435:3637:3839:4041:4243:4445 |
 | b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xc0\x00\x02\x80" | ::ffff:192.0.2.128                      |
 +---------------------------------------------------------------------------------------------------------------*/
```

## `     NET.IP_TRUNC    `

``` text
NET.IP_TRUNC(addr_bin, prefix_length)
```

**Description** Takes `  addr_bin  ` , an IPv4 or IPv6 address in binary (BYTES) format in network byte order, and returns a subnet address in the same format. The result has the same length as `  addr_bin  ` , where the first `  prefix_length  ` bits are equal to those in `  addr_bin  ` and the remaining bits are 0.

This function throws an error if `  LENGTH(addr_bin)  ` isn't 4 or 16, or if `  prefix_len  ` is negative or greater than `  LENGTH(addr_bin) * 8  ` .

**Return Data Type**

BYTES

**Example**

``` text
SELECT
  FORMAT("%T", x) as addr_bin, prefix_length,
  FORMAT("%T", NET.IP_TRUNC(x, prefix_length)) AS ip_trunc
FROM UNNEST([
  STRUCT(b"\xAA\xBB\xCC\xDD" as x, 0 as prefix_length),
  (b"\xAA\xBB\xCC\xDD", 11), (b"\xAA\xBB\xCC\xDD", 12),
  (b"\xAA\xBB\xCC\xDD", 24), (b"\xAA\xBB\xCC\xDD", 32),
  (b'0123456789@ABCDE', 80)
]);

/*-----------------------------------------------------------------------------+
 | addr_bin            | prefix_length | ip_trunc                              |
 +-----------------------------------------------------------------------------+
 | b"\xaa\xbb\xcc\xdd" | 0             | b"\x00\x00\x00\x00"                   |
 | b"\xaa\xbb\xcc\xdd" | 11            | b"\xaa\xa0\x00\x00"                   |
 | b"\xaa\xbb\xcc\xdd" | 12            | b"\xaa\xb0\x00\x00"                   |
 | b"\xaa\xbb\xcc\xdd" | 24            | b"\xaa\xbb\xcc\x00"                   |
 | b"\xaa\xbb\xcc\xdd" | 32            | b"\xaa\xbb\xcc\xdd"                   |
 | b"0123456789@ABCDE" | 80            | b"0123456789\x00\x00\x00\x00\x00\x00" |
 +-----------------------------------------------------------------------------*/
```

## `     NET.IPV4_FROM_INT64    `

``` text
NET.IPV4_FROM_INT64(integer_value)
```

**Description**

Converts an IPv4 address from integer format to binary (BYTES) format in network byte order. In the integer input, the least significant bit of the IP address is stored in the least significant bit of the integer, regardless of host or client architecture. For example, `  1  ` means `  0.0.0.1  ` , and `  0x1FF  ` means `  0.0.1.255  ` .

This function checks that either all the most significant 32 bits are 0, or all the most significant 33 bits are 1 (sign-extended from a 32-bit integer). In other words, the input should be in the range `  [-0x80000000, 0xFFFFFFFF]  ` ; otherwise, this function throws an error.

This function doesn't support IPv6.

**Return Data Type**

BYTES

**Example**

``` text
SELECT x, x_hex, FORMAT("%T", NET.IPV4_FROM_INT64(x)) AS ipv4_from_int64
FROM (
  SELECT CAST(x_hex AS INT64) x, x_hex
  FROM UNNEST(["0x0", "0xABCDEF", "0xFFFFFFFF", "-0x1", "-0x2"]) AS x_hex
);

/*-----------------------------------------------+
 | x          | x_hex      | ipv4_from_int64     |
 +-----------------------------------------------+
 | 0          | 0x0        | b"\x00\x00\x00\x00" |
 | 11259375   | 0xABCDEF   | b"\x00\xab\xcd\xef" |
 | 4294967295 | 0xFFFFFFFF | b"\xff\xff\xff\xff" |
 | -1         | -0x1       | b"\xff\xff\xff\xff" |
 | -2         | -0x2       | b"\xff\xff\xff\xfe" |
 +-----------------------------------------------*/
```

## `     NET.IPV4_TO_INT64    `

``` text
NET.IPV4_TO_INT64(addr_bin)
```

**Description**

Converts an IPv4 address from binary (BYTES) format in network byte order to integer format. In the integer output, the least significant bit of the IP address is stored in the least significant bit of the integer, regardless of host or client architecture. For example, `  1  ` means `  0.0.0.1  ` , and `  0x1FF  ` means `  0.0.1.255  ` . The output is in the range `  [0, 0xFFFFFFFF]  ` .

If the input length isn't 4, this function throws an error.

This function doesn't support IPv6.

**Return Data Type**

INT64

**Example**

``` text
SELECT
  FORMAT("%T", x) AS addr_bin,
  FORMAT("0x%X", NET.IPV4_TO_INT64(x)) AS ipv4_to_int64
FROM
UNNEST([b"\x00\x00\x00\x00", b"\x00\xab\xcd\xef", b"\xff\xff\xff\xff"]) AS x;

/*-------------------------------------+
 | addr_bin            | ipv4_to_int64 |
 +-------------------------------------+
 | b"\x00\x00\x00\x00" | 0x0           |
 | b"\x00\xab\xcd\xef" | 0xABCDEF      |
 | b"\xff\xff\xff\xff" | 0xFFFFFFFF    |
 +-------------------------------------*/
```

## `     NET.PUBLIC_SUFFIX    `

``` text
NET.PUBLIC_SUFFIX(url)
```

**Description**

Takes a URL as a `  STRING  ` value and returns the public suffix (such as `  com  ` , `  org  ` , or `  net  ` ). A public suffix is an ICANN domain registered at [publicsuffix.org](https://publicsuffix.org/list/) . For best results, URL values should comply with the format as defined by [RFC 3986](https://tools.ietf.org/html/rfc3986#appendix-A) . If the URL value doesn't comply with RFC 3986 formatting, this function makes a best effort to parse the input and return a relevant result.

This function returns `  NULL  ` if any of the following is true:

  - It can't parse the host from the input;
  - The parsed host contains adjacent dots in the middle (not leading or trailing);
  - The parsed host doesn't contain any public suffix.

Before looking up the public suffix, this function temporarily normalizes the host by converting uppercase English letters to lowercase and encoding all non-ASCII characters with [Punycode](https://en.wikipedia.org/wiki/Punycode) . The function then returns the public suffix as part of the original host instead of the normalized host.

**Note:** The function doesn't perform [Unicode normalization](https://en.wikipedia.org/wiki/Unicode_equivalence) .

**Note:** The public suffix data at [publicsuffix.org](https://publicsuffix.org/list/) also contains private domains. This function ignores the private domains.

**Note:** The public suffix data may change over time. Consequently, input that produces a `  NULL  ` result now may produce a non- `  NULL  ` value in the future.

**Return Data Type**

`  STRING  `

**Example**

``` text
SELECT
  FORMAT("%T", input) AS input,
  description,
  FORMAT("%T", NET.HOST(input)) AS host,
  FORMAT("%T", NET.PUBLIC_SUFFIX(input)) AS suffix,
  FORMAT("%T", NET.REG_DOMAIN(input)) AS domain
FROM (
  SELECT "" AS input, "invalid input" AS description
  UNION ALL SELECT "http://abc.xyz", "standard URL"
  UNION ALL SELECT "//user:password@a.b:80/path?query",
                   "standard URL with relative scheme, port, path and query, but no public suffix"
  UNION ALL SELECT "https://[::1]:80", "standard URL with IPv6 host"
  UNION ALL SELECT "http://例子.卷筒纸.中国", "standard URL with internationalized domain name"
  UNION ALL SELECT "    www.Example.Co.UK    ",
                   "non-standard URL with spaces, upper case letters, and without scheme"
  UNION ALL SELECT "mailto:?to=&subject=&body=", "URI rather than URL--unsupported"
);
```

<table>
<thead>
<tr class="header">
<th>input</th>
<th>description</th>
<th>host</th>
<th>suffix</th>
<th>domain</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>""</td>
<td>invalid input</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>"http://abc.xyz"</td>
<td>standard URL</td>
<td>"abc.xyz"</td>
<td>"xyz"</td>
<td>"abc.xyz"</td>
</tr>
<tr class="odd">
<td>"//user:password@a.b:80/path?query"</td>
<td>standard URL with relative scheme, port, path and query, but no public suffix</td>
<td>"a.b"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>"https://[::1]:80"</td>
<td>standard URL with IPv6 host</td>
<td>"[::1]"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>"http://例子.卷筒纸.中国"</td>
<td>standard URL with internationalized domain name</td>
<td>"例子.卷筒纸.中国"</td>
<td>"中国"</td>
<td>"卷筒纸.中国"</td>
</tr>
<tr class="even">
<td>" www.Example.Co.UK "</td>
<td>non-standard URL with spaces, upper case letters, and without scheme</td>
<td>"www.Example.Co.UK"</td>
<td>"Co.UK"</td>
<td>"Example.Co.UK</td>
</tr>
<tr class="odd">
<td>"mailto:?to=&amp;subject=&amp;body="</td>
<td>URI rather than URL--unsupported</td>
<td>"mailto"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

## `     NET.REG_DOMAIN    `

``` text
NET.REG_DOMAIN(url)
```

**Description**

Takes a URL as a string and returns the registered or registrable domain (the [public suffix](#netpublic_suffix) plus one preceding label), as a string. For best results, URL values should comply with the format as defined by [RFC 3986](https://tools.ietf.org/html/rfc3986#appendix-A) . If the URL value doesn't comply with RFC 3986 formatting, this function makes a best effort to parse the input and return a relevant result.

This function returns `  NULL  ` if any of the following is true:

  - It can't parse the host from the input;
  - The parsed host contains adjacent dots in the middle (not leading or trailing);
  - The parsed host doesn't contain any public suffix;
  - The parsed host contains only a public suffix without any preceding label.

Before looking up the public suffix, this function temporarily normalizes the host by converting uppercase English letters to lowercase and encoding all non-ASCII characters with [Punycode](https://en.wikipedia.org/wiki/Punycode) . The function then returns the registered or registerable domain as part of the original host instead of the normalized host.

**Note:** The function doesn't perform [Unicode normalization](https://en.wikipedia.org/wiki/Unicode_equivalence) .

**Note:** The public suffix data at [publicsuffix.org](https://publicsuffix.org/list/) also contains private domains. This function doesn't treat a private domain as a public suffix. For example, if `  us.com  ` is a private domain in the public suffix data, `  NET.REG_DOMAIN("foo.us.com")  ` returns `  us.com  ` (the public suffix `  com  ` plus the preceding label `  us  ` ) rather than `  foo.us.com  ` (the private domain `  us.com  ` plus the preceding label `  foo  ` ).

**Note:** The public suffix data may change over time. Consequently, input that produces a `  NULL  ` result now may produce a non- `  NULL  ` value in the future.

**Return Data Type**

`  STRING  `

**Example**

``` text
SELECT
  FORMAT("%T", input) AS input,
  description,
  FORMAT("%T", NET.HOST(input)) AS host,
  FORMAT("%T", NET.PUBLIC_SUFFIX(input)) AS suffix,
  FORMAT("%T", NET.REG_DOMAIN(input)) AS domain
FROM (
  SELECT "" AS input, "invalid input" AS description
  UNION ALL SELECT "http://abc.xyz", "standard URL"
  UNION ALL SELECT "//user:password@a.b:80/path?query",
                   "standard URL with relative scheme, port, path and query, but no public suffix"
  UNION ALL SELECT "https://[::1]:80", "standard URL with IPv6 host"
  UNION ALL SELECT "http://例子.卷筒纸.中国", "standard URL with internationalized domain name"
  UNION ALL SELECT "    www.Example.Co.UK    ",
                   "non-standard URL with spaces, upper case letters, and without scheme"
  UNION ALL SELECT "mailto:?to=&subject=&body=", "URI rather than URL--unsupported"
);
```

<table>
<thead>
<tr class="header">
<th>input</th>
<th>description</th>
<th>host</th>
<th>suffix</th>
<th>domain</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>""</td>
<td>invalid input</td>
<td>NULL</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>"http://abc.xyz"</td>
<td>standard URL</td>
<td>"abc.xyz"</td>
<td>"xyz"</td>
<td>"abc.xyz"</td>
</tr>
<tr class="odd">
<td>"//user:password@a.b:80/path?query"</td>
<td>standard URL with relative scheme, port, path and query, but no public suffix</td>
<td>"a.b"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="even">
<td>"https://[::1]:80"</td>
<td>standard URL with IPv6 host</td>
<td>"[::1]"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
<tr class="odd">
<td>"http://例子.卷筒纸.中国"</td>
<td>standard URL with internationalized domain name</td>
<td>"例子.卷筒纸.中国"</td>
<td>"中国"</td>
<td>"卷筒纸.中国"</td>
</tr>
<tr class="even">
<td>" www.Example.Co.UK "</td>
<td>non-standard URL with spaces, upper case letters, and without scheme</td>
<td>"www.Example.Co.UK"</td>
<td>"Co.UK"</td>
<td>"Example.Co.UK"</td>
</tr>
<tr class="odd">
<td>"mailto:?to=&amp;subject=&amp;body="</td>
<td>URI rather than URL--unsupported</td>
<td>"mailto"</td>
<td>NULL</td>
<td>NULL</td>
</tr>
</tbody>
</table>

## `     NET.SAFE_IP_FROM_STRING    `

``` text
NET.SAFE_IP_FROM_STRING(addr_str)
```

**Description**

Similar to [`  NET.IP_FROM_STRING  `](#netip_from_string) , but returns `  NULL  ` instead of throwing an error if the input is invalid.

**Return Data Type**

BYTES

**Example**

``` text
SELECT
  addr_str,
  FORMAT("%T", NET.SAFE_IP_FROM_STRING(addr_str)) AS safe_ip_from_string
FROM UNNEST([
  '48.49.50.51',
  '::1',
  '3031:3233:3435:3637:3839:4041:4243:4445',
  '::ffff:192.0.2.128',
  '48.49.50.51/32',
  '48.49.50',
  '::wxyz'
]) AS addr_str;

/*---------------------------------------------------------------------------------------------------------------+
 | addr_str                                | safe_ip_from_string                                                 |
 +---------------------------------------------------------------------------------------------------------------+
 | 48.49.50.51                             | b"0123"                                                             |
 | ::1                                     | b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01" |
 | 3031:3233:3435:3637:3839:4041:4243:4445 | b"0123456789@ABCDE"                                                 |
 | ::ffff:192.0.2.128                      | b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xc0\x00\x02\x80" |
 | 48.49.50.51/32                          | NULL                                                                |
 | 48.49.50                                | NULL                                                                |
 | ::wxyz                                  | NULL                                                                |
 +---------------------------------------------------------------------------------------------------------------*/
```
