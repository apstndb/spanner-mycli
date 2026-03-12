Protocol buffers are a flexible mechanism for serializing structured data. They are small in size and efficient to send over RPCs. Protocol buffers are represented in Spanner using the `  PROTO  ` data type. A column can contain `  PROTO  ` values the same way it can contain `  INT32  ` or `  STRING  ` values.

A protocol buffer contains zero or more fields inside it. Each field inside a protocol buffer has its own type. All SQL data types except `  STRUCT  ` can be contained inside a `  PROTO  ` . Repeated fields in a protocol buffer are represented as `  ARRAY  ` s. To learn more about the `  PROTO  ` data type, see [Protocol buffer type](/spanner/docs/reference/standard-sql/data-types#protocol_buffer_type) . For related functions, see [Protocol buffer functions](/spanner/docs/reference/standard-sql/protocol_buffer_functions) .

To query protocol buffers, you need to understand how they are represented, what features they support, and what data they can contain. To learn more about protocol buffers, see the [Protocol Buffers Developer Guide](https://developers.google.com/protocol-buffers) .

## Limitations

The maximum size of a protocol buffer bundle is 5 MiB. For performance reasons, we recommend keeping protocol buffer bundles below 500 KiB.

## Construct a protocol buffer

You can construct a protocol buffer with the [`  NEW  `](/spanner/docs/reference/standard-sql/operators#new_operator) operator or the [`  SELECT AS typename  `](/spanner/docs/reference/standard-sql/query-syntax#select_as_typename) statement. To learn more about constructing protocol buffers, see [Protocol buffer type](/spanner/docs/reference/standard-sql/data-types#constructing_a_proto) .

### Cast to or from a protocol buffer

You can use the [`  CAST AS PROTO  `](/spanner/docs/reference/standard-sql/conversion_functions#cast-as-proto) function to cast `  PROTO  ` to or from `  BYTES  ` , `  STRING  ` , or `  PROTO  ` .

``` text
SELECT CAST('first_name: "Alana", last_name: "Yah", customer_no: 1234'
  AS example.CustomerInfo);
```

Casting to or from `  BYTES  ` produces or parses [proto2 wire format bytes](https://protobuf.dev/programming-guides/encoding) . If there is a failure during the serialization or deserialization process, Spanner throws an error. This can happen, for example, if no value is specified for a required field.

Casting to or from `  STRING  ` produces or parses the [proto2 text format](https://protobuf.dev/reference/protobuf/textformat-spec) . When casting from `  STRING  ` , unknown field names aren't parseable. This means you need to be cautious, because round-tripping from `  PROTO  ` to `  STRING  ` back to `  PROTO  ` might result in unexpected results.

`  STRING  ` literals used where a `  PROTO  ` value is expected will be implicitly cast to `  PROTO  ` . If the literal value can't be parsed using the expected `  PROTO  ` type, an error is raised. To return `  NULL  ` instead of raising an error, use [`  SAFE_CAST  `](/spanner/docs/reference/standard-sql/conversion_functions#safe_casting) .

### Create a protocol buffer

To create a protocol buffer, import the protocol messages from a protocol buffer with the [`  CREATE PROTO BUNDLE  `](/spanner/docs/reference/standard-sql/data-definition-language#create-proto-bundle) statement. Only the protocol message type names in a `  CREATE PROTO BUNDLE  ` statement are stored in your database schema. For example:

First, create a protocol buffer bundle with the `  examples.shipping.Order  ` message:

``` text
syntax = "proto2";
package examples.shipping;

message Order {
  optional string order_number = 1;
  optional int64 date = 2;

  message Address {
    optional string street = 1;
    optional string city = 2;
    optional string state = 3;
    optional string country = 4;
  }

  optional Address shipping_address = 3;

  message Item {
    optional string product_name = 1;
    optional int32 quantity = 2;
  }

  repeated Item line_item = 4;

  

}

message OrderHistory {
  optional string order_number = 1;
  optional int64 date = 2;
}
```

Next, compile the protocol buffer. Use the [`  protoc  ` compiler](https://github.com/protocolbuffers/protobuf#protobuf-compiler-installation) to generate the proto descriptor file along with the corresponding language-specific classes from your `  .proto  ` file. To generate the `  order_descriptors.pb  ` proto descriptor:

``` text
protoc --include_imports --descriptor_set_out=order_descriptors.pb order_protos.proto
```

Use the `  CREATE PROTO BUNDLE  ` DDL statement to load types available from proto files into your database schema.

For example:

``` text
CREATE PROTO BUNDLE (
  examples.shipping.Order,
  examples.shipping.Order.Address,
  examples.shipping.Order.Item
)
```

Then, upload the proto descriptor using the `  gcloud spanner databases ddl update  ` command:

``` text
gcloud spanner databases ddl update DATABASE_NAME --instance=INSTANCE_NAME
  --ddl='CREATE PROTO BUNDLE (`examples.shipping.Order`, `examples.shipping.Order.Address`, `examples.shipping.Order.Item`);'
  --proto-descriptors-file=order_descriptors.pb
```

If you include nested message types within a protocol message in your DML statement, the nested message types must be included in the `  CREATE PROTO BUNDLE  ` DDL statement.

Finally, create an `  Orders  ` table with the `  Order  ` message:

``` text
CREATE TABLE Orders (
  Id INT64,
  OrderInfo `examples.shipping.Order`,
) PRIMARY KEY(Id);
```

### Add a message to a protocol buffer

You can add messages to your protocol buffer with the [`  ALTER PROTO BUNDLE  `](/spanner/docs/reference/standard-sql/data-definition-language#alter-proto-bundle) statement. For example:

``` text
ALTER PROTO BUNDLE INSERT (
  examples.shipping.OrderHistory
)
```

Update the proto bundle with the added message using the `  gcloud spanner databases ddl update  ` command:

``` text
gcloud spanner databases ddl update DATABASE_NAME --instance=INSTANCE_NAME
  --ddl='ALTER PROTO BUNDLE INSERT (`examples.shipping.OrderHistory`);'
  --proto-descriptors-file=order_descriptors.pb
```

### Use a protocol buffer message in a column definition

You can include a protocol buffer message when creating new tables by declaring a column's type as a protocol message. For example:

``` text
CREATE TABLE OrderHistory (
  Id INT64,
  OrderHistoryInfo examples.shipping.OrderHistory,
) PRIMARY KEY(Id);
```

### Update a message for a protocol buffer

You can update and replace existing proto messages with the [`  ALTER PROTO BUNDLE  `](/spanner/docs/reference/standard-sql/data-definition-language#alter-proto-bundle) statement. For example:

``` text
ALTER PROTO BUNDLE UPDATE (
  examples.shipping.Order,
)
```

Update the proto bundle using the `  gcloud spanner databases ddl update  ` command:

``` text
gcloud spanner databases ddl update DATABASE_NAME --instance=INSTANCE_NAME
  --ddl='ALTER PROTO BUNDLE UPDATE(`examples.shipping.Order`);'
  --proto-descriptors-file=order_descriptors.pb
```

### Delete a message from a protocol buffer

If you're no longer using a message, you can delete it from the protocol buffer with the [`  ALTER PROTO BUNDLE  `](/spanner/docs/reference/standard-sql/data-definition-language#alter-proto-bundle) statement. For example:

``` text
ALTER PROTO BUNDLE DELETE (
  examples.shipping.OrderHistory,
)
```

Update the proto bundle using the `  gcloud spanner databases ddl update  ` command:

``` text
gcloud spanner databases ddl update DATABASE_NAME --instance=INSTANCE_NAME
  --ddl='ALTER PROTO BUNDLE DELETE(`examples.shipping.OrderHistory`);'
  --proto-descriptors-file=order_descriptors.pb
```

## Query a protocol buffer

Use the dot operator to access the fields contained within a protocol buffer. You can't use this operator to get values of fields that use the [`  Oneof  `](https://protobuf.dev/programming-guides/proto3/#oneof) , [`  Any  `](https://protobuf.dev/programming-guides/proto3/#any) , or [`  Unknown Fields  `](https://protobuf.dev/programming-guides/proto3/#unknowns) type.

### Example protocol buffer message

The following example queries for a table called `  Customers  ` . This table contains a column `  Orders  ` of type `  PROTO  ` .

``` text
CREATE TABLE Customers (
  Id INT64,
  Orders examples.shipping.Order,
) PRIMARY KEY(Id);
```

The proto stored in `  Orders  ` contains fields such as the items ordered and the shipping address. The `  .proto  ` file that defines this protocol buffer might look like this:

``` text
syntax = "proto2";
package examples.shipping;

message Order {
  optional string order_number = 1;
  optional int64 date = 2;

  message Address {
    optional string street = 1;
    optional string city = 2;
    optional string state = 3;
    optional string country = 4;
  }

  optional Address shipping_address = 3;

  message Item {
    optional string product_name = 1;
    optional int32 quantity = 2;
  }

  repeated Item line_item = 4;

  

}
```

An instance of this message might be:

``` text
{
  order_number: 1234567
  date: 16242
  shipping_address: {
      street: "1234 Main St"
      city: "AnyCity"
      state: "AnyState"
      country: "United States"
  }
  line_item: [
    {
      product_name: "Foo"
      quantity: 10
    },
    {
      product_name: "Bar"
      quantity: 5
    }
  ]
}
```

### Query top-level fields

You can write a query to return an entire protocol buffer message, or to return a top-level or nested field of the message.

Using our example protocol buffer message, the following query returns all protocol buffer values from the `  Orders  ` column:

``` text
SELECT
  c.Orders
FROM
  Customers AS c;
```

This query returns the top-level field `  order_number  ` from all protocol buffer messages in the `  Orders  ` column using the [dot operator](/spanner/docs/reference/standard-sql/operators#field_access_operator) :

``` text
SELECT
  c.Orders.order_number
FROM
  Customers AS c;
```

### Query nested paths

You can write a query to return a nested field of a protocol buffer message.

In the previous example, the `  Order  ` protocol buffer contains another protocol buffer message, `  Address  ` , in the `  shipping_address  ` field. You can create a query that returns all orders that have a shipping address in the United States:

``` text
SELECT
  c.Orders.order_number,
  c.Orders.shipping_address
FROM
  Customers AS c
WHERE
  c.Orders.shipping_address.country = "United States";
```

### Return repeated fields

A protocol buffer message can contain repeated fields. When referenced in a SQL statement, the repeated fields return as `  ARRAY  ` values. For example, our protocol buffer message contains a repeated field, `  line_item  ` .

The following query returns a set of `  ARRAY  ` s containing the line items, each holding all the line items for one order:

``` text
SELECT
  c.Orders.line_item
FROM
  Customers AS c;
```

For more information about arrays, see [Work with arrays](/spanner/docs/reference/standard-sql/arrays#working_with_arrays) .

For more information about default field values, see [Default values and `  NULL  ` s](#default_values_and_nulls) .

### Return the number of elements in an array

You can return the number of values in a repeated fields in a protocol buffer using the `  ARRAY_LENGTH  ` function.

``` text
SELECT
  c.Orders.order_number,
  ARRAY_LENGTH(c.Orders.line_item)
FROM
  Customers AS c;
```

## Type mapping

The following table gives examples of the mapping between various protocol buffer field types and the resulting GoogleSQL types.

<table>
<thead>
<tr class="header">
<th>Protocol buffer field type</th>
<th>GoogleSQL type</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       bool      </code></td>
<td><code dir="ltr" translate="no">       BOOL      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       bytes      </code></td>
<td><code dir="ltr" translate="no">       BYTES      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       double      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       int64      </code></td>
<td><code dir="ltr" translate="no">       INT64      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       sint64      </code></td>
<td><code dir="ltr" translate="no">       INT64      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       sfixed64      </code></td>
<td><code dir="ltr" translate="no">       INT64      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       string      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       message      </code></td>
<td><code dir="ltr" translate="no">       PROTO      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       enum      </code></td>
<td><code dir="ltr" translate="no">       ENUM      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       repeated      </code></td>
<td><code dir="ltr" translate="no">       ARRAY      </code></td>
</tr>
</tbody>
</table>

The following protocol buffer field types can appear in the protocol buffer definition and are written to the protocol buffer column data, but they can't be used in DML statements or queries with the field access operator. Otherwise, an unknown type error appears. They can only be accessed by reading the entire protocol buffer file:

  - `  float  `
  - `  int32  `
  - `  sint32  `
  - `  sfixed32  `
  - `  uint32  `
  - `  fixed32  `
  - `  uint64  `
  - `  fixed64  `

### Default values and `     NULL    ` s

Protocol buffer messages themselves don't have a default value; only the non-repeated fields contained inside a protocol buffer have defaults. When the result of a query returns a full protocol buffer value, it's returned as a `  blob  ` (a data structure that's used to store large binary data). The result preserves all fields in the protocol buffer as they were stored, including unset fields. This means that you can run a query that returns a protocol buffer, and then extract fields or check field presence in your client code with normal protocol buffer default behavior.

`  NULL  ` values aren't typically returned when accessing non-repeated leaf fields contained in a `  PROTO  ` from within a SQL statement. However, `  NULL  ` values can result in some cases, for example if a containing parent message is `  NULL  ` .

This behavior is consistent with the standards for handling protocol buffers messages. If the field value isn't explicitly set on a non-repeated leaf field, the `  PROTO  ` default value for the field is returned. A change to the default value for a `  PROTO  ` field affects all future reads of that field using the new `  PROTO  ` definition for records where the value is unset.

For example, consider the following protocol buffer:

``` text
message SimpleMessage {
  optional SubMessage message_field_x = 1;
  optional SubMessage message_field_y = 2;
}

message SubMessage {
  optional string field_a = 1;
  optional string field_b = 2;
  optional string field_c = 3;
  repeated string field_d = 4;
  repeated string field_e = 5;
}
```

Assume the following field values:

  - `  message_field_x  ` isn't set.
  - `  message_field_y.field_a  ` is set to `  "a"  ` .
  - `  message_field_y.field_b  ` isn't set.
  - `  message_field_y.field_c  ` isn't set.
  - `  message_field_y.field_d  ` is set to `  ["d"]  ` .
  - `  message_field_y.field_e  ` isn't set.

For this example, the following table summarizes the values produced from various accessed fields:

<table>
<thead>
<tr class="header">
<th>Accessed field</th>
<th>Value produced</th>
<th>Reason</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       message_field_x      </code></td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
<td>Message isn't set.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       message_field_x.field_a      </code> through <code dir="ltr" translate="no">       message_field_x.field_e      </code></td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
<td>Parent message isn't set.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       message_field_y      </code></td>
<td><code dir="ltr" translate="no">       PROTO&lt;SubMessage&gt;{ field_a: "a" field_d: ["d"]}      </code></td>
<td>Parent message and child fields are set.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       message_field_y.field_a      </code></td>
<td><code dir="ltr" translate="no">       "a"      </code></td>
<td>Field is set.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       message_field_y.field_b      </code></td>
<td><code dir="ltr" translate="no">       ""      </code> (empty string)</td>
<td>Field isn't set but parent message is set, so default value (empty string) is produced.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       message_field_y.field_c      </code></td>
<td><code dir="ltr" translate="no">       NULL      </code></td>
<td>Field isn't set and annotation indicates to not use defaults.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       message_field_y.field_d      </code></td>
<td><code dir="ltr" translate="no">       ["d"]      </code></td>
<td>Field is set.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       message_field_y.field_e      </code></td>
<td><code dir="ltr" translate="no">       [ ]      </code> (empty array)</td>
<td>Repeated field isn't set.</td>
</tr>
</tbody>
</table>

### Check if a non-repeated field has a value

You can detect whether `  optional  ` fields are set using a virtual field, `  has_X  ` , where `  X  ` is the name of the field being checked. The type of the `  has_X  ` field is `  BOOL  ` . The `  has_  ` field is available for any non-repeated field of a `  PROTO  ` value. This field equals true if the value `  X  ` is explicitly set in the message.

This field is useful for determining if a protocol buffer field has an explicit value, or if reads will return a default value.

Consider the following protocol buffer example, which has a field `  country  ` . You can construct a query to determine if a `  Customer  ` protocol buffer message has a value for the country field by using the virtual field `  has_country  ` :

``` text
message ShippingAddress {
  optional string name = 1;
  optional string address = 2;
  optional string country = 3;
}
```

``` text
SELECT
  c.Orders.shipping_address.has_country
FROM
  Customer AS c;
```

If `  has_country  ` is `  TRUE  ` , that means the value for the `  country  ` field has been explicitly set. If `  has_country  ` is `  FALSE  ` , that means the parent message `  c.Orders.shipping_address  ` isn't `  NULL  ` , but the `  country  ` field hasn't been explicitly set. If `  has_country  ` is `  NULL  ` , that means the parent message `  c.Orders.shipping_address  ` is `  NULL  ` , and therefore `  country  ` can't be considered either set or unset.

Some scalar proto fields are defined to have implicit presence, so you can't distinguish whether the field is unset or set-to-default. To reliably distinguish when a proto field is unset or set-to-default, enable explicit presence on the field.

For more information about default field values, see [Default values and `  NULL  ` s](#default_values_and_nulls) .

### Check for a repeated value

You can use an `  EXISTS  ` subquery to scan inside a repeated field and check if any value exists with some desired property. For example, the following query returns the name of every customer who has placed an order for the product "Foo".

``` text
SELECT
  C.Id
FROM
  Customers AS C
WHERE
  EXISTS(
    SELECT
      *
    FROM
      C.Orders.line_item AS item
    WHERE
      item.product_name = 'Foo'
  );
```

### Nullness and nested fields

A `  PROTO  ` value may contain fields which are themselves `  PROTO  ` s. When this happens, it's possible for the nested `  PROTO  ` to be `  NULL  ` . In such a case, the fields contained within that nested field are also `  NULL  ` .

Consider this example proto:

``` text
syntax = "proto2";

package examples.package;

message NestedMessage {
  optional int64 value = 1;
}

message OuterMessage {
  optional NestedMessage nested = 1;
}
```

Running the following query returns a `  5  ` for `  value  ` because it is explicitly defined.

``` text
SELECT
  proto_field.nested.value
FROM
  (SELECT
     CAST("nested { value: 5 }" AS examples.package.OuterMessage) AS proto_field);
```

If `  value  ` isn't explicitly defined but `  nested  ` is, you get a `  0  ` because the annotation on the protocol buffer definition says to use default values.

``` text
SELECT
  proto_field.nested.value
FROM
  (SELECT
     CAST("nested { }" AS examples.package.OuterMessage) AS proto_field);
```

However, if `  nested  ` isn't explicitly defined, you get a `  NULL  ` even though the annotation says to use default values for the `  value  ` field. This is because the containing message is `  NULL  ` . This behavior applies to both repeated and non-repeated fields within a nested message.

``` text
SELECT
  proto_field.nested.value
FROM
  (SELECT
     CAST("" AS examples.package.OuterMessage) AS proto_field);
```

## Protocol buffer fields as primary key

Using [generated columns](/spanner/docs/generated-column/how-to) , you can define a primary key on a field in the protocol message.

If you define a primary key on a protocol message field, you can't modify or remove that field from the proto schema. For more information, see [Update schemas that contain a primary key or index on a proto field](/spanner/docs/reference/standard-sql/protocol-buffers#updates_schemas_pk_index_proto) .

The following is an example of a `  Singers  ` table with a `  SingerInfo  ` proto message column. You can define a primary key on the `  id  ` field of the `  PROTO  ` column by creating a stored generated column:

``` text
CREATE PROTO BUNDLE (example.music.SingerInfo, example.music.SingerInfo.Residence);
CREATE TABLE Singers (
  SingerId INT64 NOT NULL AS (SingerInfo.id) STORED,
  SingerInfo example.music.SingerInfo,
  ...
) PRIMARY KEY (SingerId);
```

It has the following definition of the `  SingerInfo  ` proto type:

``` text
package example.music;
message SingerInfo {
  optional int64     id          = 1;
  optional string    name        = 2;
  optional string    nationality = 3;
  repeated Residence residence   = 4;

  message Residence {
    required int64  start_year   = 1;
    optional int64  end_year     = 2;
    optional string city         = 3;
    optional string country      = 4;
  }
}
```

The following SQL query reads data using the previous example:

``` text
SELECT s.SingerId, s.SingerInfo.name
FROM Singers AS s
WHERE s.SingerId = 34;
```

## Index protocol buffer fields

Using [generated columns](/spanner/docs/generated-column/how-to) , you can index the fields stored in a `  PROTO  ` column, as long as the fields being indexed are of a primitive or `  ENUM  ` data type. For more information, see [Index proto fields](/spanner/docs/secondary-indexes#proto-indexing) .

## Update schemas that contain a primary key or index on a protocol buffer field

If you define a primary key or an index on a protocol message field, you can't modify or remove that field from the proto schema. This is because after you make this definition, type checking is performed every time the schema is updated. The database captures the type information for all fields in the path that are used in the primary key or index definition.

## Coercion

Protocol buffers can be coerced into other data types. For more information, see [Conversion rules](/spanner/docs/reference/standard-sql/conversion_rules) .
