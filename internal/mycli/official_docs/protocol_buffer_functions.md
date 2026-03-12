GoogleSQL for Spanner supports the following protocol buffer functions.

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
<td><a href="/spanner/docs/reference/standard-sql/protocol_buffer_functions#replace_fields"><code dir="ltr" translate="no">        REPLACE_FIELDS       </code></a></td>
<td>Replaces the values in one or more protocol buffer fields.</td>
</tr>
</tbody>
</table>

## `     REPLACE_FIELDS    `

``` text
REPLACE_FIELDS(proto_expression, value AS field_path [, ... ])
```

**Description**

Returns a copy of a protocol buffer, replacing the values in one or more fields. `  field_path  ` is a delimited path to the protocol buffer field that's replaced. When using `  replace_fields  ` , the following limitations apply:

  - If `  value  ` is `  NULL  ` , it un-sets `  field_path  ` or returns an error if the last component of `  field_path  ` is a required field.
  - Replacing subfields will succeed only if the message containing the field is set.
  - Replacing subfields of repeated field isn't allowed.
  - A repeated field can be replaced with an `  ARRAY  ` value.

**Return type**

Type of `  proto_expression  `

**Examples**

The following example uses protocol buffer messages `  Book  ` and `  BookDetails  ` .

``` text
message Book {
  required string title = 1;
  repeated string reviews = 2;
  optional BookDetails details = 3;
};

message BookDetails {
  optional string author = 1;
  optional int32 chapters = 2;
};
```

This statement replaces the values of the field `  title  ` and subfield `  chapters  ` of proto type `  Book  ` . Note that field `  details  ` must be set for the statement to succeed.

``` text
SELECT REPLACE_FIELDS(
  NEW Book(
    "The Hummingbird" AS title,
    NEW BookDetails(10 AS chapters) AS details),
  "The Hummingbird II" AS title,
  11 AS details.chapters)
AS proto;

/*-----------------------------------------------------------------------------+
 | proto                                                                       |
 +-----------------------------------------------------------------------------+
 |{title: "The Hummingbird II" details: {chapters: 11 }}                       |
 +-----------------------------------------------------------------------------*/
```

The function can replace value of repeated fields.

``` text
SELECT REPLACE_FIELDS(
  NEW Book("The Hummingbird" AS title,
    NEW BookDetails(10 AS chapters) AS details),
  ["A good read!", "Highly recommended."] AS reviews)
AS proto;

/*-----------------------------------------------------------------------------+
 | proto                                                                       |
 +-----------------------------------------------------------------------------+
 |{title: "The Hummingbird" review: "A good read" review: "Highly recommended."|
 | details: {chapters: 10 }}                                                   |
 +-----------------------------------------------------------------------------*/
```

The function can also set a field to `  NULL  ` .

``` text
SELECT REPLACE_FIELDS(
  NEW Book("The Hummingbird" AS title,
    NEW BookDetails(10 AS chapters) AS details),
  NULL AS details)
AS proto;

/*-----------------------------------------------------------------------------+
 | proto                                                                       |
 +-----------------------------------------------------------------------------+
 |{title: "The Hummingbird" }                                                  |
 +-----------------------------------------------------------------------------*/
```
