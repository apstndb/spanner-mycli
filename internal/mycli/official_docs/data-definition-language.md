Use the GoogleSQL data definition language (DDL) to do the following:

  - Create and alter a database.
  - Create and drop a placement.
  - Create, alter, or drop a locality group.
  - Create, alter, or drop tables in a database.
  - Add, alter, or drop columns in a table.
  - Create, alter, or drop indexes in a database.
  - Create, alter, or drop search indexes in a database.
  - Create, replace, or drop views in a database.
  - Create, alter, or drop change streams in a database.
  - Create or drop database roles.
  - Grant privileges to database roles.
  - Grant database roles to other database roles.
  - Create, alter, or drop ML models in a database.

## Notation

  - Square brackets "\[ \]" indicate optional clauses.
  - Parentheses "( )" indicate literal parentheses.
  - The vertical bar "|" indicates a logical OR.
  - Curly braces "{ }" enclose a set of options.
  - A comma followed by an ellipsis indicates that the preceding item can repeat in a comma-separated list. `  item [, ...]  ` indicates one or more items, and `  [item, ...]  ` indicates zero or more items.
  - A comma "," indicates the literal comma.
  - Angle brackets "\<\>" indicate literal angle brackets.
  - An mdash "—" indicates a range of values between the items on either side of it.
  - The plus sign "+" indicates that the preceding item can repeat.

## Reserved keywords

Some words have special meaning in the GoogleSQL language and are reserved in its DDL. To use a reserved keyword as an identifier in your schema, enclose it in backticks ( ``  `  `` ). For the full list of reserved keywords in GoogleSQL, see [GoogleSQL lexical structure and syntax](/spanner/docs/reference/standard-sql/lexical#reserved_keywords) .

For example:

``` text
CREATE TABLE MyTable (
  RowId INT64 NOT NULL PRIMARY KEY,
  `Order` INT64
);
```

## Names

The following rules apply to **database IDs** .

  - Must start with a lowercase letter.
  - Can contain lowercase letters, numbers, underscores, and hyphens, but not uppercase letters.
  - Cannot end with an underscore or hyphen.
  - Must be enclosed in backticks ( ``  `  `` ) if it's a reserved word or contains a hyphen.
  - Can be between 2-30 characters long.
  - Cannot be changed after you create it.

The following rules apply to names for **schemas** , **tables** , **change streams** , **columns** , **constraints** , **indexes** , **roles** , **sequences** , and **views** :

  - Must be at least one character long.

  - Can contain a maximum of 128 characters.

  - Must start with an uppercase or lowercase letter.

  - Can contain uppercase and lowercase letters, numbers, and underscores, but not hyphens.

  - Spanner objects can't be created with the same name as another object in the same database, including names that only differ in capitalization. For example, the second statement in the following snippet fails because the table names differ only by case.
    
    ``` text
    CREATE TABLE MyTable (col1 INT64 PRIMARY KEY);
    CREATE TABLE MYTABLE (col1 INT64 PRIMARY KEY);
    ```
    
    The following snippet fails because two different objects use the same name:
    
    ``` text
    CREATE TABLE MyTable (col1 INT64 PRIMARY KEY);
    CREATE SCHEMA MyTable;
    ```

  - When referring to other schema objects in a DDL statement (for example, a column name for a primary key, or table and column names in an index), make sure to use the original case for the name of each entity. As an example, consider the table `  Singers  ` created with the following statement.
    
    ``` text
    CREATE TABLE Singers (
      SingerId   INT64 NOT NULL PRIMARY KEY,
      FirstName  STRING(1024),
      LastName   STRING(1024),
      SingerInfo BYTES(MAX),
      BirthDate  DATE
    );
    ```
    
    The following command fails with the message `  Table not found: singers  ` because it uses a different case for the `  Singers  ` table.
    
    ``` text
    CREATE INDEX SingersByFirstLastName ON singers(FirstName, LastName)
    ```

  - Schema object names are case insensitive in SQL queries. As an example, consider the table `  MyTable2  ` created with the following statement.
    
    ``` text
    CREATE TABLE MyTable2 (col1 INT64 PRIMARY KEY);
    ```
    
    The following queries all succeed because schema object names are case-insensitive for queries.
    
    ``` text
    SELECT col1 FROM MyTable2 LIMIT 1;
    SELECT COL1 FROM MYTABLE2 LIMIT 1;
    SELECT COL1 FROM mytable2 LIMIT 1;
    INSERT INTO MYTABLE2 (col1) VALUES(1);
    ```

  - When a column name in a table is identical to the table name, the table must use an alias for the query to work. As an example, consider the table `  Singer  ` created with the following statement.
    
    ``` text
    CREATE TABLE Singer (
      Singer     INT64 NOT NULL PRIMARY KEY,
      FirstName  STRING(1024),
      LastName   STRING(1024),
      BirthDate  DATE
    );
    ```
    
    The following query succeeds because the table uses an alias when the table name is identical to the column name.
    
    ``` text
    SELECT S.FirstName, S.Singer FROM Singer S;
    ```

## Data types

The following are the data types used in GoogleSQL.

### Scalars

The syntax for using a scalar type in DDL is:

``` text
{
  BOOL
  | INT64
  | FLOAT32
  | FLOAT64
  | NUMERIC
  | STRING( length )
  | JSON
  | BYTES( length )
  | DATE
  | TIMESTAMP
}

length:
    { int64_value | MAX }

int64_value:
    { decimal_value | hex_value }

decimal_value:
    [-]0—9+

hex_value:
    [-]0x{0—9|a—f|A—F}+
```

An `  int64_value  ` must correspond to an integer from -9,223,372,036,854,775,808 (-2 <sup>63</sup> ) to 9,223,372,036,854,775,807 (2 <sup>63</sup> − 1). It can be specified with decimal or hexadecimal notation. The hexadecimal form requires a `  0x  ` prefix, with a lowercase `  x  ` .

#### STRING

`  STRING  ` is a variable length Unicode character string. Its value must be a valid Unicode string. Length is required, and represents the maximum number of Unicode characters (not bytes) that can be stored in the field.

Notes:

  - Writes to the column are rejected if the new value is not a valid Unicode string or exceeds the specified length.

  - `  length  ` can be an integer in the range \[1, 2621440\].

  - For a field whose length is unpredictable or does not need to be constrained, you can set `  length  ` to the convenience value `  MAX  ` , which is equivalent to 2621440 for validation purposes.
    
    Only the actual length of the stored string impacts storage costs; specifying `  MAX  ` does not use any additional storage capacity.

  - GoogleSQL requires Unicode strings to be UTF-8 encoded on receipt at the server.

  - Collation is done by Unicode character numerical value (technically by [code point](//en.wikipedia.org/wiki/Code_point) , which is subtly different due to [combining characters](//en.wikipedia.org/wiki/Combining_character) ). For ASCII strings, this is the standard lexicographical sort order.

  - You can reduce the length of a column after the table has been created, but doing so requires Spanner to [validate](/spanner/docs/schema-updates#updates-that-require-validation) that the existing data is within the length constraint.

#### JSON

`  JSON  ` is a variable length Unicode character string representing a JSON object. The string must be UTF-8 encoded on receipt at the server. The maximum length of the JSON value is 10 MB.

See [Working with JSON](/spanner/docs/working-with-json) and [Data types](/spanner/docs/reference/standard-sql/data-types#json_type) For more information.

#### BYTES

`  BYTES  ` is a variable length binary string. Length is required, and represents the maximum number of bytes that can be stored in the field.

Notes:

  - Writes to the column are rejected if the new value exceeds the specified length.

  - `  length  ` can be an integer in the range \[1, 10485760\] or the convenience value `  MAX  ` , which is equivalent to 10485760 for validation purposes.
    
    Only the actual stored bytes impact storage costs; specifying `  MAX  ` does not use any additional storage capacity.

  - You can reduce the length of a column after the table has been created, but doing so requires Spanner to [validate](/spanner/docs/schema-updates#updates-that-require-validation) that the existing data is within the length constraint.

#### DATE

  - A timezone-independent date.
  - The range \[ `  0001-01-01, 9999-12-31  ` \] is the legal interval for dates. A write to a date column is rejected if the value is outside of that interval.
  - For more information and to see the canonical format, see [Data Types](/spanner/docs/reference/standard-sql/data-types#date_type) .

#### TIMESTAMP

  - A timestamp with nanosecond precision.
  - Timezone-independent, over the range \[ `  0001-01-01 00:00:00  ` to `  10000-01-01 00:00:00  ` \].
  - For more information and to see the canonical format, see [Data Types](/spanner/docs/reference/standard-sql/data-types#timestamp_type) .

### Arrays

The syntax for using the `  ARRAY  ` type in DDL is:

``` text
ARRAY<scalar_type> [(vector_length=>vector_length_value)]
```

GoogleSQL supports arrays of scalars. The primary purpose of arrays is to store a collection of values in a space efficient way. Arrays are not designed to provide access to individual elements; to read or write a single element, you must read or write the entire array.

If your application uses data structures like vectors or repeated fields, you can persist their state in a GoogleSQL array.

Here's an example of an alternate definition of `  Singers  ` that uses multiple columns of `  ARRAY  ` type:

``` text
CREATE TABLE Singers (
  SingerId INT64,
  FeaturedSingerIds ARRAY<INT64>,
  SongNames ARRAY<STRING(MAX)>
) PRIMARY KEY (SingerId) ...;
```

Notes:

  - Arrays with subtype `  ARRAY  ` (nested arrays) are not supported.

  - Arrays, like scalar values, can never be larger than 10 MiB total.

  - Arrays can't be used as key columns.

  - In a `  CREATE TABLE  ` statement, you can create columns of `  ARRAY  ` type with a `  NOT NULL  ` annotation.
    
    After you create the table, you cannot add a column of `  ARRAY  ` type with a `  NOT NULL  ` annotation, and you cannot add a `  NOT NULL  ` annotation to an existing column of `  ARRAY  ` type.

  - `  vector_length  ` sets an array column to a fixed size for use in a vector search. The value must be an integer greater than or equal to zero. You can only use this parameter with an array that uses the `  FLOAT32  ` or `  FLOAT64  ` data types. That is, `  ARRAY<FLOAT32> (vector_length=>INT)  ` or `  ARRAY<FLOAT64> (vector_length=>INT)  ` . Setting an array with `  vector_length  ` is required to perform [approximate nearest neighbors](/spanner/docs/find-approximate-nearest-neighbors) vector search. It can also provide performance benefits when performing [K-nearest neighbors](/spanner/docs/find-k-nearest-neighbors) vector search. You can create this array column and set its value using [`  CREATE TABLE  `](#create_table) . You can alter this array column using [`  ALTER TABLE  `](#alter_table) .

### Protocol buffers

The syntax for using the protocol buffers ( `  PROTO  ` ) data type in DDL is:

``` text
proto_type_name;
```

GoogleSQL supports `  PROTO  ` and arrays of `  PROTO  ` . Protocol buffers are a flexible, efficient mechanism for serializing structured data. For more information, see [Work with protocol buffers in GoogleSQL](/spanner/docs/reference/standard-sql/protocol-buffers) .

The following is an example of a table named `  Singers  ` with a `  SingerInfo  ` proto message column and an `  SingerInfoArray  ` proto message array column:

``` text
CREATE TABLE Singers (
 SingerId   INT64 NOT NULL PRIMARY KEY,
 FirstName  STRING(1024),
 LastName   STRING(1024),
 SingerInfo googlesql.example.SingerInfo,
 SingerInfoArray ARRAY<googlesql.example.SingerInfo>,
);
```

It has the following definition of the `  SingerInfo  ` proto type:

``` text
  package googlesql.example;
  message SingerInfo {
  optional string    nationality = 1;
  repeated Residence residence   = 2;

    message Residence {
      required int64  start_year   = 1;
      optional int64  end_year     = 2;
      optional string city         = 3;
      optional string country      = 4;
    }
  }
```

## SCHEMA statements

This section has information about the `  CREATE SCHEMA  ` and `  DROP SCHEMA  ` statements.

### CREATE SCHEMA

Creates a new schema and assigns a name.

``` text
CREATE SCHEMA [schema_name]
```

#### Parameters

`  schema_name  `

  - Contains a name for a schema.
  - When querying data, use fully qualified names (FQNs) to specify objects that belong to a specific schema. FQNs combine the schema name and the object name to identify database objects. For example, `  products.albums  ` for the `  products  ` schema and `  albums  ` table. For more information, see [Named schemas](/spanner/docs/schema-and-data-model#named-schemas) .

### DROP SCHEMA

Removes a named schema.

``` text
DROP SCHEMA schema_name
```

#### Parameters

`  schema_name  `

  - Contains the name for the schema to drop.

#### Parameters

`  schema_name  `

  - Contains the name of the schema that you want to drop.

## DATABASE statements

This section has information about the `  CREATE DATABASE  ` and `  ALTER DATABASE  ` statements.

### CREATE DATABASE

When creating a GoogleSQL database, you must provide a `  CREATE DATABASE  ` statement, which defines the ID of the database:

``` text
CREATE DATABASE database_id

where database_id
    {a—z}[{a—z|0—9|_|-}+]{a—z|0—9}
```

#### Parameters

`  database_id  `

  - The name of the database to create. [See Names](#database-id-names) .

### ALTER DATABASE

Changes the definition of a database.

#### Syntax

``` text
ALTER DATABASE database_id
    action

where database_id is:
    {a—z}[{a—z|0—9|_|-}+]{a—z|0—9}

and action is:
    SET OPTIONS ( options_def [, ... ] )

and options_def is:
    { default_leader = { 'region' | null } |
      optimizer_version = { 1 ... 8 | null } |
      optimizer_statistics_package = { 'package_name' | null } |
      version_retention_period = { 'duration' | null } |
      default_sequence_kind = { 'bit_reversed_positive' | null } |
      default_time_zone = { 'time_zone_name' | null } |
      read_lease_regions = {'read_lease_region_name[, ... ]' | null } }
```

#### Description

`  ALTER DATABASE  ` changes the definition of an existing database.

`  SET OPTIONS  `

  - Use this clause to set an option at the database level of the schema hierarchy.

#### Parameters

`  database_id  `

  - The name of the database whose attributes are to be altered. If the name is a reserved word or contains a hyphen, enclose it in backticks ( ``  `  `` ). For information on database naming rules, see [Names](#database-id-names) .

`  options_def  `

  - The `  optimizer_version = { 1 ... 8 | null }  ` option lets you specify the query optimizer version to use. Setting this option to `  null  ` is equivalent to setting it to the default version. For more information, see [Query Optimizer](/spanner/docs/query-optimizer/overview) .

  - The `  optimizer_statistics_package = { ' package_name ' | null }  ` option lets you specify the query optimizer statistics package name to use. By default, this is the latest collected statistics package, but you can specify any available statistics package version. Setting this option to `  null  ` is equivalent to setting it to the latest version. For more information, see [Query statistics package versioning](/spanner/docs/query-optimizer/manage-query-optimizer) .

  - The `  version_retention_period = { 'duration' | null }  ` is the period for which Spanner retains all versions of data and schema for the database. The duration must be in the range `  [1h, 7d]  ` and can be specified in days, hours, minutes, or seconds. For example, the values `  1d  ` , `  24h  ` , `  1440m  ` , and `  86400s  ` are equivalent. Setting the value to `  null  ` resets the retention period to the default, which is 1 hour. This option can be used for point-in-time recovery. For more information, see [Point-in-time Recovery](/spanner/docs/pitr) .

  - The `  default_leader = { 'region' | null }  ` sets the leader region for your database. You can only use this parameter for databases that use a multi-region configuration. `  default_leader  ` must be set to `  null  ` , or one of the read-write replicas in your multi-region configuration. `  null  ` resets the leader region to the default leader region for your database's multi-region configuration. For more information, see [Configuring the default leader region](/spanner/docs/instance-configurations#config-default-leader-region) .

  - The `  default_sequence_kind = { 'bit_reversed_positive' | null }  ` sets the default sequence kind for your database. `  bit_reversed_positive  ` is the only valid sequence kind. The `  bit_reversed_positive  ` option specifies that the values generated by the sequence are of type `  INT64  ` , are greater than zero, and aren't sequential. You don't need to specify a sequence type when using `  default_sequence_kind  ` . When you use `  default_sequence_kind  ` for a sequence or identity column, you can't change the sequence kind later. For more information, see [Primary key default values management](/spanner/docs/primary-key-default-value#serial-auto-increment) .

  - The `  use_unenforced_foreign_key_for_query_optimization = { true | false | null }  ` lets you specify whether the query optimizer can rely on [informational foreign key relationships](/spanner/docs/foreign-keys/overview#informational-foreign-keys) to improve query performance. For example, the optimizer can remove redundant scans, and push some `  LIMIT  ` operators through the join operators. Setting `  use_unenforced_foreign_key_for_query_optimization  ` to `  null  ` is equivalent to setting it to `  true  ` . Note that enabling this might lead to incorrect results if the data is inconsistent with the foreign key relationships.

  - The `  default_time_zone = { 'time_zone_name' | null }  ` option sets the default time zone for your database. If set to `  NULL  ` , the system defaults to `  America/Los_Angeles  ` . Specifying a time zone within a `  DATE  ` or `  TIMESTAMP  ` function overrides this setting. The `  time_zone_name  ` must be a valid entry from the [IANA Time Zone Database](https://www.iana.org/time-zones) . This option can only be set on empty databases without any tables.

  - The `  read_lease_regions = {'read_lease_region_name' | null }  ` option sets the [read lease](/spanner/docs/read-lease) region for your database. By default, or when set to `  NULL  ` , the database doesn't use any read lease regions. If you set one or more read lease regions for your database, Spanner gives the right to serve reads locally to one or more non-leader, read-write, or read-only regions. This allows the non-leader regions directly serve strong reads and reduce strong read latency.

## LOCALITY GROUP statements

This section has information about the `  CREATE LOCALITY GROUP  ` , `  ALTER LOCALITY GROUP  ` , and `  DROP LOCALITY GROUP  ` statements.

### CREATE LOCALITY GROUP

Use the `  CREATE LOCALITY GROUP  ` statement to define a locality group to store some columns separately or to use tiered storage. For more information, see [Locality groups](/spanner/docs/schema-and-data-model#locality-groups) and [Tiered storage overview](/spanner/docs/tiered-storage) .

#### Syntax

``` text
CREATE LOCALITY GROUP locality_group_name [ storage_def ]

where storage_def is:
    { OPTIONS ( storage = '{ ssd | hdd }' [, ssd_to_hdd_spill_timespan='duration' ] ) }
```

#### Description

`  CREATE LOCALITY GROUP  ` defines a new locality group in the current database.

#### Parameters

`  locality_group_name  `

  - The name of the locality group.

`  OPTIONS  `

  - Use `  storage  ` to define the storage type of the locality group. You can set the storage type as 'ssd' or 'hdd'.

  - Use `  ssd_to_hdd_spill_timespan  ` to define the amount of time that data is stored in SSD storage before it moves to HDD storage. After the specified time passes, Spanner migrates the data to HDD storage during its normal compaction cycle, which typically occurs over the course of seven days from the specified time. The duration must be at least one hour ( `  1h  ` ) and at most 365 days ( `  365d  ` ) long. It can be specified in days, hours, minutes, or seconds. For example, the values `  1d  ` , `  24h  ` , `  1440m  ` , and `  86400s  ` are equivalent.

### ALTER LOCALITY GROUP

Use the `  ALTER LOCALITY GROUP  ` statement to change the storage option or age-based policy of a locality group.

#### Syntax

``` text
ALTER LOCALITY GROUP locality_group_name [ storage_def ]

where storage_def is:
    { SET OPTIONS ( [ storage = '{ ssd | hdd }' ssd_to_hdd_spill_timespan='duration' ] ) }
```

#### Description

`  ALTER LOCALITY GROUP  ` changes the storage option or age-based policy of a locality group. You can change these options together or individually.

#### Parameters

`  locality_group_name  `

  - The name of the locality group. When updating the `  default  ` locality group, `  default  ` must be within backticks ( ``  `default`  `` ). You only need to include the backticks for the `  default  ` locality group.

`  OPTIONS  `

  - Use `  storage  ` to define the new storage type of the locality group.

  - Use the `  ssd_to_hdd_spill_timespan = 'duration'  ` option to set the new age-based policy of the locality group. The duration must be at least one hour ( `  1h  ` ) and at most 365 days ( `  365d  ` ) long. It can be specified in days, hours, minutes, or seconds. For example, the values `  1d  ` , `  24h  ` , `  1440m  ` , and `  86400s  ` are equivalent.

### DROP LOCALITY GROUP

Use the `  DROP LOCALITY GROUP  ` statement to drop the locality group. You can't drop a locality group if it contains data. You must first move all data that's in the locality group to another locality group.

#### Syntax

``` text
DROP LOCALITY GROUP locality_group_name
```

#### Description

`  DROP LOCALITY GROUP  ` drops the locality group.

## PLACEMENT statements

**Preview — [Geo-partitioning](/spanner/docs/geo-partitioning)**

This feature is subject to the "Pre-GA Offerings Terms" in the General Service Terms section of the [Service Specific Terms](/terms/service-terms#1) . Pre-GA features are available "as is" and might have limited support. For more information, see the [launch stage descriptions](https://cloud.google.com/products/#product-launch-stages) .

This section has information about `  PLACEMENT  ` statements.

### CREATE PLACEMENT

Use the `  CREATE PLACEMENT  ` statement to define a placement to partition row data in your database. For more information, see the [Geo-partitioning overview](/spanner/docs/geo-partitioning) .

#### Syntax

``` text
CREATE PLACEMENT placement_name [ partition_def ]

where partition_def is:
    { OPTIONS ( instance_partition="partition_id" [, default_leader="leader_region_id" ]
      [, read_lease_regions = {'read_lease_region_name[, ... ]' | null } ] ) }
```

#### Description

`  CREATE PLACEMENT  ` defines a new placement in the current database.

#### Parameters

`  placement_name  `

  - The name of the placement.

`  partition_id  `

  - The unique identifier of the user-created partition associated with the placement.

`  leader_region_id  `

  - This optional parameter sets the default leader region for the partition. Similar to [setting the default leader](/spanner/docs/instance-configurations#configure-leader-region) at the database level. However, this only applies to the partition.

  - The `  read_lease_regions = {'read_lease_region_name' | null }  ` option sets one or more [read lease](/spanner/docs/read-lease) regions for your placement. By default, or when set to `  NULL  ` , the placement doesn't use any read lease regions. If you set one or more read lease regions for your placement, Spanner gives the right to serve reads locally to one or more non-leader, read-write, or read-only regions. This lets the non-leader regions directly serve strong reads and reduce strong read latency.

### DROP PLACEMENT

Use the `  DROP PLACEMENT  ` statement to delete a placement.

#### Syntax

``` text
DROP PLACEMENT placement_name
```

#### Description

`  DROP PLACEMENT  ` drops a placement.

#### Parameters

`  placement_name  `

  - The name of the placement to drop.

## PROTO BUNDLE statements

The `  PROTO  ` files you create need to be loaded into your database schema using `  PROTO BUNDLE  ` , making the `  PROTO  ` files available for use by tables and queries keyed by `  PROTO  ` and `  ENUM  ` fields.

### CREATE PROTO BUNDLE

Use the `  CREATE PROTO BUNDLE  ` statement to load types available from imported proto files into the schema.

#### Syntax

``` text
CREATE PROTO BUNDLE ("
                      (<proto_type_name>) ("," <proto_type_name>)*
                    ")
```

#### Description

`  CREATE PROTO BUNDLE  ` loads types available from imported proto files.

#### Parameters

*`  proto_type_name  `*

  - The proto types included in your `  PROTO BUNDLE  ` .

Notes:

  - Spanner requires some proto types to be included in your `  PROTO BUNDLE  ` . In particular:
      - Any message type that is used as the type of a `  PROTO  ` column.
      - Any enum type that is used by an `  ENUM  ` column.
      - Any type needed to resolve a proto field path.
      - Any enum type that is referenced by a message type in the `  PROTO BUNDLE  ` .
      - Any message type that nests a message or enum type already in the `  PROTO BUNDLE  ` .
      - Any nested message type that is used as the type of a `  PROTO  ` column.
  - If you're using a protocol buffer type and any part of the type name is a Spanner reserved keyword, enclose the entire protocol buffer type name in backticks. For example, if you created a message named `  Bytes  ` in the package `  my.awesome.proto  ` , and you wanted to create a column of that type, you can use the column definition: `  MyColumn my.awesome.proto.Bytes  ` .

### ALTER PROTO BUNDLE

The `  ALTER PROTO BUNDLE  ` statement is used to update the proto information stored in the schema.

#### Syntax

``` text
ALTER PROTO BUNDLE
[ INSERT ( <proto_type_name> , .... ) ]
[ UPDATE ( <proto_type_name> , .... ) ]
[ DELETE ( <proto_type_name> , .... ) ]
```

#### Description

`  ALTER PROTO BUNDLE  ` updates the proto information already stored in the schema.

#### Parameters

*`  proto_type_name  `*

  - The proto types included in your `  PROTO BUNDLE  ` .

Notes:

  - All the same notes that apply to [`  CREATE PROTO BUNDLE  `](#create-proto-bundle) apply to `  ALTER PROTO BUNDLE  ` , but they apply to the final proto bundle, not the alteration itself.
  - `  INSERT  ` , `  UPDATE  ` , and `  DELETE  ` clauses all execute atomically as a single change to your database's type information.

### DROP PROTO BUNDLE

The DROP PROTO BUNDLE statement is used to drop all proto type information stored in the schema.

#### Syntax

``` text
DROP PROTO BUNDLE
```

#### Description

`  DROP PROTO BUNDLE  ` drops all proto type information stored in the schema.

Notes:

  - All the same notes that apply to `  CREATE PROTO BUNDLE  ` apply to `  DROP PROTO BUNDLE  ` . You can't drop a proto bundle if your database uses types in the proto bundle.

## TABLE statements

This section has information about the `  CREATE TABLE  ` , `  ALTER TABLE  ` , `  DROP TABLE  ` , AND `  RENAME TABLE  ` statements.

### CREATE TABLE

Defines a new table.

#### Syntax

``` text
CREATE TABLE [ IF NOT EXISTS ] table_name ( [
   { column_name data_type [NOT NULL]
     [ { DEFAULT ( expression ) [ ON UPDATE ( expression )]
       | AS ( expression ) [ STORED ]
       | GENERATED BY DEFAULT AS IDENTITY [ ( sequence_option_clause ... ) ]
       | AUTO_INCREMENT } ]
     [ HIDDEN ]
     [ PRIMARY KEY ]
     [ options_def ]
   | location_name STRING(MAX) NOT NULL PLACEMENT KEY
   | table_constraint
   | synonym_definition }
   [, ... ]
] ) [ PRIMARY KEY ( [column_name [ { ASC | DESC } ], ...] ) ]
[, INTERLEAVE IN [PARENT] table_name [ ON DELETE { CASCADE | NO ACTION } ] ]
[, ROW DELETION POLICY ( OLDER_THAN ( timestamp_column, INTERVAL num_days DAY ) ) ]
[, OPTIONS ( locality_group = 'locality_group_name' ) ]

where data_type is:
    { scalar_type | array_type | proto_type_name }

and options_def is:
    { OPTIONS ( allow_commit_timestamp = { true | null } |
                locality_group = 'locality_group_name' ) }

and table_constraint is:
    [ CONSTRAINT constraint_name ]
    { CHECK ( expression ) |
      FOREIGN KEY ( column_name [, ... ] ) REFERENCES  ref_table  ( ref_column [, ... ] )
        [ ON DELETE { CASCADE | NO ACTION } ] [ { ENFORCED | NOT ENFORCED } ]
    }

and synonym_definition is:
    [ SYNONYM (synonym) ]

and sequence_option_clause is:
    { BIT_REVERSED_POSITIVE
    | SKIP RANGE skip_range_min, skip_range_max
    | START COUNTER WITH start_with_counter }
```

#### Description

`  CREATE TABLE  ` defines a new table in the current database.

#### Parameters

`  IF NOT EXISTS  `

  - If a table exists with the same name, the `  CREATE  ` statement has no effect and no error is generated.

`  table_name  `

  - The name of the table to be created. For naming rules, see [Names](#names) .

`  column_name  `

  - The name of a column to be created. For naming rules, see [Names](#names) .

`  data_type  `

  - The data type of the column, which can be a [***Scalar***](#scalars) or an [***Array***](#arrays) type.

`  vector_length  `

  - `  vector_length  ` sets an array column to a fixed size for use in a vector search. The value must be an integer greater than or equal to zero. You can only use this parameter with an array that uses the `  FLOAT32  ` or `  FLOAT64  ` data types. That is, `  ARRAY<FLOAT32> (vector_length=>INT)  ` or `  ARRAY<FLOAT64> (vector_length=>INT)  ` . It isn't supported for a `  DEFAULT  ` or generated column.

`  timestamp_column  `

  - The name of a column of type `  TIMESTAMP  ` , that is also specified in the CREATE TABLE statement.

`  num_days  `

  - The number of days after the date in the specified `  timestamp_column  ` , after which the row is marked for deletion. Valid values are non-negative integers.

`  NOT NULL  `

  - This optional column annotation specifies that the column is required for all mutations that insert a new row.

  - You cannot add a NOT NULL column to an existing table. For most column types, you can work around this limitation:
    
      - For columns of `  ARRAY  ` type, the only time you can use a NOT NULL annotation is when you create the table. After that, you cannot add a NOT NULL annotation to a column of `  ARRAY  ` type.
    
      - For all other column types, you can add a nullable column; fill that column by writing values to all rows; and update your schema with a NOT NULL annotation on that column.

`  HIDDEN  `

Hides a column if it shouldn't appear in `  SELECT *  ` statements. If the column is hidden, you can still select it using its name. For example, `  SELECT Id, Name, ColHidden FROM TableWithHiddenColumn  ` .

The primary use case for `  HIDDEN  ` columns is to omit `  TOKENLIST  ` columns from a `  SELECT *  ` statement.

`  DEFAULT (  ` `  expression  ` `  )  `

  - This clause sets a default value for the column.

  - A column with a default value can be a key or non-key column.

  - A column can't have a default value and also be a generated column.

  - You can insert your own value into a column that has a default value, overriding the default value. You can also reset a non-key column to its default value by using `  UPDATE ... SET  ` `  column-name  ` `  = DEFAULT  ` .

  - A generated column or a check constraint can depend on a column with a  
    default value.

  - A column can only use `  PENDING_COMMIT_TIMESTAMP  ` as a default value if it has the `  ALLOW_COMMIT_TIMESTAMP  ` type (this is the only default value allowed for this type).

  - `  expression  ` can be a literal or any valid SQL expression that is assignable to the column data type, with the following properties and restrictions:
    
      - The expression can be non-deterministic.
      - The expression can't reference other columns.
      - The expression can't contain subqueries, query parameters, aggregates, or analytic functions.

`  ON UPDATE (  ` `  expression  ` `  )  `

  - This clause configures a column to automatically update its value whenever a row is modified. This is typically used to maintain "last updated" timestamps without requiring manual input in every `  UPDATE  ` statement.

  - The column is set to the result of the expression whenever an update occurs on any non-key column in the row.

  - The expression is triggered even if the update statement sets a column to its current value (that is, no actual data change occurs).

  - You can bypass the automated value by explicitly providing a value for the column within your [`  UPDATE  `](/spanner/docs/reference/standard-sql/dml-syntax#update-statement) or [`  INSERT  `](/spanner/docs/reference/standard-sql/dml-syntax#insert-statement) statement.

  - To use the `  ON UPDATE  ` clause, the column must satisfy these conditions:
    
      - Must not be part of the table’s `  PRIMARY KEY  ` .
    
      - Must have a `  DEFAULT  ` expression that is identical to the `  ON UPDATE  ` expression.
    
      - Must be a commit timestamp column, and the expression must be one of the following, depending on the column's data type:
        
          - `  PENDING_COMMIT_TIMESTAMP()  ` (for `  TIMESTAMP  ` columns)
          - `  PENDING_COMMIT_TIMESTAMP_INT64()  ` (for `  INT64  ` columns)

`  GENERATED BY DEFAULT AS IDENTITY [ (  ` ***sequence\_option\_clause*** `  ... )]  `

  - This clause auto-generates integer values for the column.
  - `  BIT_REVERSED_POSITIVE  ` is the only valid type.
  - An identity column can be a key or non-key column.
  - An identity column can't have a default value or be a generated column.
  - You can insert your own value into an identity column. You can also reset a non-key column to use generated value by using `  UPDATE ... SET  ` `  column-name  ` `  = DEFAULT  ` .
  - A generated column or a check constraint can depend on an identity column.
  - An identity column accepts the following option clauses:
      - `  BIT_REVERSED_POSITIVE  ` indicates the type of identity column.
      - `  SKIP RANGE  ` `  skip_range_min  ` , `  skip_range_max  ` allows the underlying sequence to skip the numbers in this range when calling `  GET_NEXT_SEQUENCE_VALUE  ` . The skipped range is an integer value and inclusive. The accepted values for `  skip_range_min  ` is any value that is less than or equal to `  skip_range_max  ` . The accepted values for `  skip_range_max  ` is any value that is greater than or equal to `  skip_range_min  ` .
      - `  START COUNTER WITH  ` `  start_with_counter  ` is a positive `  INT64  ` value that Spanner uses to set the next value for the internal sequence counter. For example, when Spanner obtains a value from the bit-reversed sequence, it begins with `  start_with_counter  ` . Spanner bit reverses this value before returning it. The default value is `  1  ` .

`  AS (  ` `  expression  ` `  ) [STORED]  `

  - This clause creates a column as a *generated column* , which is a column whose value is defined as a function of other columns in the same row.

  - `  expression  ` can be any valid SQL expression that's assignable to the column data type with the following restrictions.
    
      - The expression can only reference columns in the same table.
    
      - The expression can't contain [subqueries](/spanner/docs/reference/standard-sql/subqueries) .
    
      - Expressions with non-deterministic functions such as [`  PENDING_COMMIT_TIMESTAMP()  `](/spanner/docs/reference/standard-sql/timestamp_functions#pending_commit_timestamp) , [`  CURRENT_DATE()  `](/spanner/docs/reference/standard-sql/date_functions#current_date) , and [`  CURRENT_TIMESTAMP()  `](/spanner/docs/reference/standard-sql/timestamp_functions#current_timestamp) can't be made into a `  STORED  ` generated column or a generated column that is indexed.
    
      - You can't modify the expression of a `  STORED  ` or indexed generated column.

  - For GoogleSQL-dialect databases, a non-stored generated column of type `  STRING  ` or `  BYTES  ` must have a length of `  MAX  ` .

  - For PostgreSQL-dialect databases, a non-stored, or virtual, generated column of type `  VARCHAR  ` must have a length of `  MAX  ` .

  - The `  STORED  ` attribute that follows the expression stores the result of the expression along with other columns of the table. Subsequent updates to any of the referenced columns cause Spanner to re-evaluate and store the expression.

  - Generated columns that are not `  STORED  ` can't be marked as `  NOT NULL  ` .

  - Direct writes to generated columns aren't allowed.

  - Column option `  allow_commit_timestamp  ` isn't allowed on generated columns or any columns that generated columns reference.

  - For `  STORED  ` or generated columns that are indexed, you can't change the data type of the column, or of any columns that the generated column references.

  - You can't drop a column a generated column references.

  - You can use a generated column as a primary key with the following additional restrictions:
    
      - The generated primary key can't reference other generated columns.
    
      - The generated primary key can reference, at most, one non-key column.
    
      - The generated primary key can't depend on a non-key column with a `  DEFAULT  ` clause.

  - The following rules apply when using generated key columns:
    
      - Read APIs: You must fully specify the key columns, including the generated key columns.
      - Mutation APIs: For `  INSERT  ` , `  INSERT_OR_UPDATE  ` , and `  REPLACE  ` , Spanner doesn't allow you to specify generated key columns. For `  UPDATE  ` , you can optionally specify generated key columns. For `  DELETE  ` , you need to fully specify the key columns including the generated keys.
      - DML: You can't explicitly write to generated keys in `  INSERT  ` or `  UPDATE  ` statements.
      - Query: In general, we recommend that you use the generated key column as a filter in your query. Optionally, if the expression for the generated key column uses only one column as a reference, the query can apply an equality ( `  =  ` ) or `  IN  ` condition to the referenced column. For more information and an example, see [Create a unique key derived from a value column](/spanner/docs/generated-column/how-to#primary-key-generated-column) .

For examples on how to work with generated columns, see [Creating and managing generated columns](/spanner/docs/generated-column/how-to) .

`  AUTO_INCREMENT  `

  - This clause creates a column as an *identity column* , which is a column whose value is generated by a sequence. To use `  AUTO_INCREMENT  ` , the database option `  default_sequence_kind  ` must be explicitly set.

For examples of how to work with `  AUTO_INCREMENT  ` , see [Primary key default values management](/spanner/docs/primary-key-default-value) .

`  location_name  ` `  STRING(MAX) NOT NULL PLACEMENT KEY  `

  - `  location_name  ` : The name of the column.
  - `  PLACEMENT KEY  ` is the required attribute that defines this column as the column that contains the placement information for rows in this table.

`  PRIMARY KEY  ` in column definition or `  PRIMARY KEY ( [  ` `  column_name  ` `  [ { ASC | DESC } ], ...]  ` in table definition

  - Every table must have a primary key and that primary key can be composed of zero or more columns of that table.

  - A single-column primary key can be defined either inline within the column definition or at the table-level.

  - A zero or multi-column primary key must be defined at the table-level with the `  PRIMARY KEY ( [  ` `  column_name  ` `  [ { ASC | DESC } ], ...]  ` syntax.

  - A primary key can't be defined at both the column and table-level.

  - Adding the `  DESC  ` annotation on a primary key column name changes the physical layout of data from ascending order (default) to descending order. The `  ASC  ` or `  DESC  ` option can be specified only when defining the primary key at the table-level.
    
    For more details, see [Schema and data model](/spanner/docs/schema-and-data-model) .

`  [, INTERLEAVE IN PARENT  ` `  table_name  ` `  [ ON DELETE { CASCADE | NO ACTION } ] ]  `

  - `  INTERLEAVE IN PARENT  ` defines a child-to-parent table relationship, which results in a physical interleaving of parent and child rows. The primary-key columns of a parent must positionally match, both in name and type, a prefix of the primary-key columns of any child. Adding rows to the child table fails if the corresponding parent row does not exist. The parent row can either exist in the database or be inserted before the insertion of the child rows in the same transaction.

  - The optional `  ON DELETE  ` clause is only allowed for `  INTERLEAVE IN PARENT  ` . `  ON DELETE  ` defines the behavior of rows in `  ChildTable  ` when a mutation attempts to delete the parent row. The supported options are:
    
      - `  CASCADE  ` : the child rows are deleted.
    
      - `  NO ACTION  ` : the child rows are not deleted. If deleting a parent would leave behind child rows, thus violating parent-child referential integrity, the write will fail.
    
    You can omit the `  ON DELETE  ` clause, in which case the default of `  ON DELETE NO ACTION  ` is used.

For more details, see [Schema and data model](/spanner/docs/schema-and-data-model) .

`  INTERLEAVE IN parent_table_name  `

  - `  INTERLEAVE IN  ` defines the same parent-child relationship and physical interleaving of parent and child rows as `  INTERLEAVE IN PARENT  ` , but the parent-child referential integrity constraint isn't enforced. Rows in the child table can be inserted before the corresponding rows in the parent table. Like with `  IN PARENT  ` , the primary-key columns of a parent must positionally match, both in name and type, a prefix of the primary-key columns of any child.

`  CONSTRAINT  ` `  constraint_name  `

  - An optional name for a table constraint. If a name is not specified, Spanner generates a name for the constraint. Constraints names, including generated names, can be queried from the Spanner [information schema](/spanner/docs/information-schema) .

`  CHECK (  ` `  expression  ` `  )  `

  - A `  CHECK  ` constraint lets you specify that the values of one or more columns must satisfy a boolean expression.

  - `  expression  ` can be any valid SQL expression that evaluates to a `  BOOL  ` .

  - The following restrictions apply to a check constraint `  expression  ` term.
    
      - The expression can only reference columns in the same table.
    
      - The expression must reference at least one non-generated column, whether directly or through a generated column which references a non-generated column.
    
      - The expression can't reference columns that have set the `  allow_commit_timestamp  ` option.
    
      - The expression can't contain [subqueries](/spanner/docs/reference/standard-sql/subqueries) .
    
      - The expression can't contain non-deterministic functions, such as [`  CURRENT_DATE()  `](/spanner/docs/reference/standard-sql/date_functions#current_date) and [`  CURRENT_TIMESTAMP()  `](/spanner/docs/reference/standard-sql/timestamp_functions#current_timestamp) .

  - For more information, see [Creating and managing check constraints](/spanner/docs/check-constraint/how-to) .

`  FOREIGN KEY (  ` `  column_name  ` `  [, ... ] ) REFERENCES  ` `  ref_table  ` `  (  ` `  ref_column  ` `  [, ... ] [ ON DELETE { CASCADE | NO ACTION } ] [ { ENFORCED | NOT ENFORCED } ] )  `

  - Use this clause to define a foreign key constraint. A foreign key is defined on the *referencing* table of the relationship, and it references the *referenced* table. The foreign key columns of the two tables are called the *referencing* and *referenced* columns, and their row values are the keys.

  - Foreign key constraints can be declared with or without the enforcement clause. If you don't specify an enforcement clause, the foreign key constraint defaults to enforced.

  - An enforced foreign key constraint requires that one or more columns of this table must contain only values that are in the referenced columns of the referenced table. A [informational ( `  NOT ENFORCED  ` ) foreign key](/spanner/docs/foreign-keys/overview#informational-foreign-keys) constraint doesn't require this.

  - When creating a foreign key, a unique constraint is automatically created on the referenced table, unless the entire primary key is referenced. If the unique constraint can't be satisfied, the entire schema change will fail.

  - The number of referencing and referenced columns must be the same. Order is also significant. That is, the first referencing column refers to the first referenced column, and the second to the second.

  - The referencing and referenced columns must have matching types and they must support the equality operator ('='). The columns must also be indexable. Columns of type `  ARRAY  ` are not allowed.

  - When you create a foreign key with the ON DELETE CASCADE action, deleting a row in the referenced table atomically deletes all rows from the referencing table that references the deleted row in the same transaction.

  - If you don't specify a foreign key action, the default action is NO ACTION.

  - Foreign keys can't be created on columns with the `  allow_commit_timestamp=true  ` option.

For more information, see [Foreign keys](/spanner/docs/foreign-keys/overview) .

`  OPTIONS ( allow_commit_timestamp = { true | null } )  `

  - The `  allow_commit_timestamp  ` option allows insert and update operations to request that Spanner write the commit timestamp of the transaction into the column. For more information, see [Commit timestamps in GoogleSQL-dialect databases](/spanner/docs/commit-timestamp) .

`  [, ROW DELETION POLICY ( OLDER_THAN (  ` `  timestamp_column  ` `  , INTERVAL  ` `  num_days  ` `  DAY ) ) ]  `

  - Use this clause to set a row deletion policy for this table. For more information, see [Time to live (TTL)](/spanner/docs/ttl) .

`  OPTIONS ( locality_group = '<code><b><i>locality_group_name</b></i></code>' )  `

  - Use this clause to store columns together or to set a tiered storage policy. For more information, see [Locality groups](/spanner/docs/schema-and-data-model#locality-groups) and [Tiered storage overview](/spanner/docs/tiered-storage) .

`  SYNONYM ( synonym )  `

  - Defines a [synonym](/spanner/docs/table-name-synonym#add-synonym) for a table, which is an additional name that an application can use to access the table. A table can have one synonym. You can only use a synonym for queries and DML. You can't use the synonym for DDL or schema changes. You can see the synonym in the DDL representation of the table.

### ALTER TABLE

Changes the definition of a table.

#### Syntax

``` text
ALTER TABLE table_name
    action

where action is:
    ADD SYNONYM synonym
    DROP SYNONYM synonym
    RENAME TO new_table_name [, ADD SYNONYM synonym]
    ADD [ COLUMN ] [ IF NOT EXISTS] column_name data_type [ column_expression ] [ options_def ]
    DROP [ COLUMN ] column_name
    ADD table_constraint
    DROP CONSTRAINT constraint_name
    SET ON DELETE { CASCADE | NO ACTION }
    SET INTERLEAVE IN [ PARENT ] parent_table_name [ ON DELETE { CASCADE | NO ACTION } ]
    ALTER [ COLUMN ] column_name
      {
        data_type  [ NOT NULL ]
          [ {
              DEFAULT ( expression ) [ ON UPDATE ( expression )]
              | AS ( expression )
              | GENERATED BY DEFAULT AS IDENTITY [ ( sequence_option_clause ... ) ]
          } ]
        | SET OPTIONS ( options_def )
        | SET DEFAULT ( expression )
        | DROP DEFAULT
        | SET ON UPDATE ( expression )
        | DROP ON UPDATE
        | ALTER IDENTITY
          {
            SET { SKIP RANGE skip_range_min, skip_range_max | NO SKIP RANGE }
            | RESTART COUNTER WITH counter_restart
          }
      }
    ADD ROW DELETION POLICY ( OLDER_THAN ( timestamp_column, INTERVAL num_days DAY ))
    DROP ROW DELETION POLICY
    REPLACE ROW DELETION POLICY ( OLDER_THAN ( timestamp_column, INTERVAL num_days DAY ))
    OPTIONS ( locality_group = 'locality_group_name' )

and data_type is:
    { scalar_type | array_type }

and column_expression is:
    [ NOT NULL ] [ { DEFAULT ( expression ) [ ON UPDATE ( expression )]
    | AS ( expression ) STORED
    | GENERATED BY DEFAULT AS IDENTITY [ ( sequence_option_clause ) ] } ]

and options_def is:
    allow_commit_timestamp = { true | null } |
    locality_group = 'locality_group_name'

and table_constraint is:
    [ CONSTRAINT constraint_name ]
    { CHECK ( expression ) |
      FOREIGN KEY ( column_name [, ... ] ) REFERENCES ref_table ( ref_column [, ... ] )
        [ ON DELETE { CASCADE | NO ACTION } ] [ { ENFORCED | NOT ENFORCED } ]
    }
```

#### Description

`  ALTER TABLE  ` changes the definition of an existing table.

`  ADD SYNONYM synonym  `

  - Adds a synonym to a table to give it an alternate name. You can use the synonym for reads, writes, queries, and for use with DML. You can't use `  ADD SYNONYM  ` with DDL, such as to create an index. A table can have one synonym. For more information, see [Add a table name synonym](/spanner/docs/table-name-synonym#add-synonym) .

`  DROP SYNONYM synonym  `

  - Removes a synonym from a table. For more information, see [Remove a synonym](/spanner/docs/table-name-synonym#remove-synonym) .

`  RENAME TO new_table_name  `

  - Renames a table, for example, if the table name is misspelled. For more information, see [Rename a table](/spanner/docs/table-name-synonym#rename-table) .

`  RENAME TO new_table_name [, ADD SYNONYM synonym ]  `

  - Adds a synonym to a table so that when you rename the table, you can add the old table name to the synonym. This gives you time to update applications with the new table name while still allowing them to access the table with the old name. For more information, see [Rename a table and add a synonym](/spanner/docs/table-name-synonym#rename-add-synonym) .

`  ADD COLUMN  `

  - Adds a new column to the table, using the same syntax as `  CREATE TABLE  ` .

  - If you specify `  IF NOT EXISTS  ` and a column of the same name already exists, the statement has no effect and no error is generated.

  - You can specify `  NOT NULL  ` in an `  ALTER TABLE...ADD COLUMN  ` statement if you specify `  DEFAULT (  ` `  expression  ` `  )  ` or `  AS (  ` `  expression  ` `  ) STORED  ` for the column.

  - If you include `  DEFAULT (  ` `  expression  ` `  )  ` or `  AS (  ` `  expression  ` `  ) STORED  ` , the expression is evaluated and the computed value is backfilled for existing rows. The backfill operation is asynchronous. This backfill operation happens only when an `  ADD COLUMN  ` statement is issued. There's no backfill on `  ALTER COLUMN  ` .

  - The `  DEFAULT  ` clause has restrictions. See the description of this clause in `  CREATE TABLE  ` .

`  DROP COLUMN  `

  - Drops a column from a table.

  - You can't drop a column referenced by a generated column.

  - Dropping a column referenced by a `  CHECK  ` constraint is not allowed.

`  ADD  ` `  table_constraint  `

  - Adds a new constraint to a table using the same syntax as `  CREATE TABLE  ` .

  - For foreign keys, the existing data is validated before the foreign key is added. If any existing constrained key doesn't have a corresponding referenced key for an enforced foreign key, or the referenced key isn't unique for a foreign key, the foreign key constraint is violated, and the `  ALTER  ` statement fails.

  - Changing the enforcement or adding a foreign key action on an existing foreign key constraint isn't supported. Instead, you need to add a new foreign key constraint with the enforcement or action.

  - If you don't specify a foreign key action, the default action is NO ACTION.

  - If you don't specify the type of a foreign key, it defaults to an enforced foreign key.

  - For `  CHECK  ` constraints, new data is validated immediately against the constraint. A long-running process is also started to validate the existing data against the constraint. If any existing data does not conform to the constraint, the check constraint is rolled back.

  - The following restrictions apply to a check constraint `  expression  ` term.
    
      - The expression can only reference columns in the same table.
    
      - The expression must reference at least one non-generated column, whether directly or through a generated column which references a non-generated column.
    
      - The expression can't reference columns that have set the `  allow_commit_timestamp  ` option.
    
      - The expression can't contain [subqueries](/spanner/docs/reference/standard-sql/subqueries) .
    
      - The expression can't contain non-deterministic functions, such as [`  CURRENT_DATE()  `](/spanner/docs/reference/standard-sql/date_functions#current_date) and [`  CURRENT_TIMESTAMP()  `](/spanner/docs/reference/standard-sql/timestamp_functions#current_timestamp) .

`  DROP CONSTRAINT  ` `  constraint_name  `

  - Drops the specified constraint on a table, along with any associated index, if applicable.

`  SET ON DELETE { CASCADE | NO ACTION }  `

  - This alteration can be applied only on child tables of parent-child, interleaved tables relationships. For more information, see [Schema and data model](/spanner/docs/schema-and-data-model) .

  - The `  ON DELETE CASCADE  ` clause signifies that when a row from the parent table is deleted, its child rows in this table will automatically be deleted as well. Child rows are all rows that start with the same primary key. If a child table does not have this annotation, or the annotation is `  ON DELETE NO ACTION  ` , then you must delete the child rows before you can delete the parent row.

`  SET INTERLEAVE IN [ PARENT ] parent_table_name [ ON DELETE { CASCADE | NO ACTION } ]  `

  - `  SET INTERLEAVE IN PARENT  ` migrates an interleaved table to use `  IN PARENT  ` semantics, which require that the parent row exist for each child row. While executing this schema change, the child rows are validated to ensure there are no referential integrity violations. If there are, the schema change fails. If no `  ON DELETE  ` clause is specified, `  NO ACTION  ` is the default. Note that directly migrating from an `  INTERLEAVE IN  ` table to `  IN PARENT ON DELETE CASCADE  ` is not supported. This must be done in two steps. The first step is to migrate `  INTERLEAVE IN  ` to `  INTERLEAVE IN PARENT T [ON DELETE NO ACTION]  ` and the second step is to migrate to `  INTERLEAVE IN PARENT T ON DELETE CASCADE  ` . If referential integrity validation fails, use a query like the following to identify missing parent rows.
    
    ``` text
        SELECT pk1, pk2 FROM child
        EXCEPT DISTINCT
        SELECT pk1, pk2 FROM parent;
    ```
    
      - `  SET INTERLEAVE IN  ` , like `  SET INTERLEAVE IN PARENT  ` , migrates an `  INTERLEAVE IN PARENT  ` interleaved table to `  INTERLEAVE IN  ` , thus removing the parent-child enforcement between the two tables.
    
      - The `  ON DELETE  ` clause is only supported when migrating to `  INTERLEAVE IN PARENT  ` .

`  ALTER COLUMN  `

  - Changes the definition of an existing column on a table.

  - `  data_type  ` `  [ NOT NULL ] [ DEFAULT (  ` `  expression  ` `  ) [ ON UPDATE (  ` `  expression  ` `  )] | AS (  ` `  expression  ` `  ) ]  `
    
      - This clause changes the data type of the column.
    
      - The `  DEFAULT  ` clause has restrictions. See the description of this clause in `  CREATE TABLE  ` .
    
      - The `  ON UPDATE  ` clause has restrictions. See the description of this clause in `  CREATE TABLE  ` .
    
      - Statements to set, change, or drop the default value or `  ON UPDATE  ` value of an existing column don't affect existing rows.
    
      - If the column has data and is altered to have the `  NOT NULL  ` constraint, the statement might fail if there is at least one existing row with a `  NULL  ` value. This is true even when a `  NOT NULL DEFAULT (...)  ` is specified, because there is no backfill operation for `  ALTER COLUMN  ` .
    
      - If `  DEFAULT  ` , `  ON UPDATE  ` , or `  NOT NULL  ` are unspecified, these properties are removed from the column.
    
      - The `  AS  ` clause is used to [Modify a generated column expression](/spanner/docs/generated-column/how-to#modify-generated-column) .
    
      - `  ARRAY (vector_length=> vector_length_value )  ` : You can use this clause to update the vector length of an array column for vector embeddings. The value of the vector length annotation indicates the dimension of the vectors in the column. The value must be an integer greater than or equal to zero. You can only use this parameter with an array that uses the `  FLOAT32  ` or `  FLOAT64  ` data types. That is, `  ARRAY<FLOAT32> (vector_length=>INT)  ` or `  ARRAY<FLOAT64> (vector_length=>INT)  ` . All values in the column must have the same array dimensions as defined by `  vector_length  ` . It isn't supported for a `  DEFAULT  ` or generated column.

  - `  SET OPTIONS  ` `  ( options_def )  `
    
      - Use this clause to set an option at the column level of the schema hierarchy.

  - `  SET DEFAULT  ` `  ( expression )  `
    
      - Sets or changes a default value for the column. Only the metadata is affected. Existing data is not changed.
    
      - This clause has restrictions. See the description of this clause in `  CREATE TABLE  ` .
    
      - When you use this clause, the result of the expression must be assignable to the current column type. To change the column type and default value in a single statement, use the following:
        
        `  ALTER TABLE  ` `  table-name  ` `  ALTER COLUMN  ` `  column-name data_type  ` `  DEFAULT  ` `  expression  `

  - `  DROP DEFAULT  `
    
      - Drops the column default value. Only metadata is affected. Existing data is not changed. You can't use `  DROP DEFAULT  ` on a column that has an `  ON UPDATE  ` expression.

  - `  SET ON UPDATE  ` `  ( expression )  `
    
      - Sets or changes the `  ON UPDATE  ` attribute for an existing column. Only metadata is affected. Existing data isn't changed.
    
      - This clause has restrictions. See the description of this clause in `  CREATE TABLE  ` .
    
      - The result of the expression must be compatible with the column's data type.
    
      - This clause is specifically used for commit timestamp columns. To apply it, the column must meet one of the following conditions:
        
          - It already has an identical `  SET DEFAULT  ` expression and is already configured as a commit timestamp column.
          - You are simultaneously updating the column definition (including the default value and commit timestamp option) within the same `  ALTER COLUMN  ` statement.

  - `  DROP ON UPDATE  `
    
      - Drops the `  ON UPDATE  ` expression on the column. Only metadata is affected. Existing data isn't changed.

  - `  ALTER IDENTITY  `
    
      - Sets or unsets the skipped range using `  SET { SKIP RANGE  ` `  skip_range_min  ` , `  skip_range_max  ` `  | NO SKIP RANGE }  ` .
    
      - Restarts the internal counter with a specific value using `  RESTART COUNTER WITH  ` `  counter_restart  ` .
    
      - These clauses are similar to [`  Identity Columns in CREATE TABLE  `](#spanner-identity-column) .

`  ADD ROW DELETION POLICY ( OLDER_THAN (  ` `  timestamp_column  ` `  , INTERVAL  ` `  num_days  ` `  DAY ) )  `

  - Adds a row deletion policy to the table defining the amount of time after a specific date after which to delete a row. See [Time to live](/spanner/docs/ttl) . Only one row deletion policy can exist on a table at a time.

`  DROP ROW DELETION POLICY  `

  - Drops the row deletion policy on a table.

`  REPLACE ROW DELETION POLICY ( OLDER_THAN (  ` `  timestamp_column  ` `  , INTERVAL  ` `  num_days  ` `  DAY ) )  `

  - Replaces the existing row deletion policy with a new policy.

`  SET OPTIONS ( locality_group = '<code><b><i>locality_group_name</b></i></code>' )  `

  - Alters the locality group used by the table.

#### Parameters

`  table_name  `

  - The name of an existing table to alter.

`  column_name  `

  - The name of a new or existing column. You can't change the key columns of a table.

`  data_type  `

  - Data type of the new column, or new data type for an existing column.

  - You can't change the data type of a generated column, or any columns referenced by the generated column.

  - Changing the data type is not allowed on any columns referenced in a `  CHECK  ` constraint. `  options_def  `

  - The `  (allow_commit_timestamp=true)  ` option allows insert and update operations to request that Spanner write the commit timestamp of the transaction into the column. For more information, see [Commit timestamps in GoogleSQL-dialect databases](/spanner/docs/commit-timestamp) .

`  options_def  `

  - The `  allow_commit_timestamp = { true | null }  ` clause is the only allowed option. If `  true  ` , a commit timestamp can be stored into the column.
    
    To learn about commit timestamps, see [Commit timestamps in GoogleSQL-dialect databases](/spanner/docs/commit-timestamp) .

`  table_constraint  `

  - New table constraint for the table.

`  constraint_name  `

  - The name of a new or existing constraint.

`  ref_table  `

  - The *referenced* table in a foreign key constraint.

`  ref_column  `

  - The *referenced* column in a foreign key constraint.

### DROP TABLE

Removes a table.

#### Syntax

``` text
DROP TABLE [ IF EXISTS ] table_name
```

#### Description

Use the `  DROP TABLE  ` statement to remove a table from the database.

  - `  DROP TABLE  ` is not recoverable.

  - You can't drop a table if there are indexes over it, or if there are any tables or indexes interleaved within it.

  - A `  DROP TABLE  ` statement automatically drops the foreign keys and foreign keys backing indexes of a table.

#### Parameters

`  IF EXISTS  `

  - If a table of the specified name doesn't exist, then the `  DROP  ` statement has no effect and no error is generated.

`  table_name  `

  - The name of the table to drop.

### RENAME TABLE

Renames a table or multiple tables at once.

#### Syntax

``` text
RENAME TABLE old_table_name TO new_table_name ...
   [, old_table_name2 TO new_table_name2 ...]
```

#### Description

Renames a table or multiple tables simultaneously, for example, if the table name is misspelled. For more information, see [Rename a table](/spanner/docs/table-name-synonym#rename-table) .

#### Parameters

`  old_table_name  `

  - The old name of the table.

`  new_table_name  `

  - The new name for the table.

#### Example

This example shows how to change the names of multiple tables atomically.

``` text
RENAME TABLE Singers TO Artists, Albums TO Recordings;
```

## INDEX statements

This section has information about the `  CREATE INDEX  ` , `  ALTER INDEX  ` , and `  DROP INDEX  ` statements.

### CREATE INDEX

Use the `  CREATE INDEX  ` statement to define [secondary indexes](/spanner/docs/secondary-indexes) .

#### Syntax

``` text
CREATE [ UNIQUE ] [ NULL_FILTERED ] INDEX [ IF NOT EXISTS ] index_name
ON table_name ( key_part [, ...] ) [ storing_clause ]
[ where_clause ] [ , interleave_clause ]
[ OPTIONS ( locality_group = 'locality_group_name' ) ]

where index_name is:
    {a—z|A—Z}[{a—z|A—Z|0—9|_}+]

and key_part is:
    column_name [ { ASC | DESC } ]

and storing_clause is:
    STORING ( column_name [, ...] )

and where_clause is:
    WHERE column_name IS NOT NULL [AND ...]

and interleave_clause is:
    INTERLEAVE IN table_name
```

#### Description

Spanner automatically indexes the primary key columns of each table.

You can use `  CREATE INDEX  ` to create secondary indexes for other columns. Adding a secondary index on a column makes it more efficient to look up data in that column. For more details, see [secondary indexes](/spanner/docs/secondary-indexes) .

**Note:** Spanner has a hard size limit of 8 KB for the total size of a table key or an index key. To work around this limitation, you can create a stored [generated column](/spanner/docs/generated-column/how-to) with expressions and an index column.

#### Parameters

`  UNIQUE  `

  - Indicates that this secondary index enforces a `  UNIQUE  ` constraint on the data being indexed. The `  UNIQUE  ` constraint causes any transaction that would result in a duplicate index key to be rejected. See [Unique Indexes](/spanner/docs/secondary-indexes#unique-indexes) for more information.

`  NULL_FILTERED  `

  - Indicates that this secondary index does not index `  NULL  ` values. For more information, see [Indexing of NULL values](/spanner/docs/secondary-indexes#null-indexing) .

`  IF NOT EXISTS  `

  - If an index already exists with the same name, then the `  CREATE  ` statement has no effect and no error is generated.

`  index_name  `

  - The name of the index to be created. For information about naming rules, see [Names](#names) .

`  table_name  `

  - The name of the table to be indexed.

`  WHERE IS NOT NULL  `

  - Rows that contain NULL in any of the columns listed in this clause aren't included in the index. The columns must be stored in the index, including key columns and columns present in the `  STORING  ` clause.

`  INTERLEAVE IN  `

  - Defines a table to interleave the index in. If `  T  ` is the table into which the index is interleaved, then the primary key of `  T  ` must be the key prefix of the index, with each key matching in type, sort order, and nullability. Matching by name is not required.
    
    If the index key that you want to use for index operations matches the key of a table, you might want to interleave the index in that table if the row in the table should have a data locality relationship with the corresponding indexed rows.
    
    For example, if you want to index all rows of `  Songs  ` for a particular row of `  Singers  ` , your index keys would contain `  SingerId  ` and `  SongName  ` and your index would be a good candidate for interleaving in `  Singers  ` if you frequently fetch information about a singer as you fetch that singer's songs from the index. The definition of `  SongsBySingerSongName  ` in [Creating a Secondary Index](/spanner/docs/secondary-indexes#creating_a_secondary_index) is an example of creating such an interleaved index.
    
    Like interleaved tables, entries in interleaved indexes are stored with the corresponding row of the parent table. See [database splits](/spanner/docs/schema-and-data-model#database-splits) for more details.

`  DESC  `

  - Defines descending scan order for the corresponding index column. When scanning a table using an index column marked `  DESC  ` , the scanned rows appear in the descending order with respect to this index column. If you don't specify a sort order, the default is ascending ( `  ASC  ` ).

`  STORING  `

  - Provides a mechanism for duplicating data from the table into one or more secondary indexes on that table. At the cost of extra storage, this can reduce read latency when looking up data using a secondary index, because it eliminates the need to retrieve data from the main table after having found the selected entries in the index. See [STORING clause](/spanner/docs/secondary-indexes#storing_clause) for an example.

`  [ OPTIONS ( locality_group = ' locality_group_name ' ) ]  `

  - Use this clause to set a secondary index-level locality group override. For more information, see [Locality groups](/spanner/docs/schema-and-data-model#locality-groups) and [Tiered storage overview](/spanner/docs/tiered-storage) .

### ALTER INDEX

Use the `  ALTER INDEX  ` statement to add additional columns or remove stored columns from the [secondary indexes](/spanner/docs/secondary-indexes) .

#### Syntax

``` text
ALTER INDEX index_name {ADD|DROP} STORED COLUMN column_name
```

#### Description

Add an additional column into an index or remove a column from an index.

#### Parameters

`  index_name  `

  - The name of the index to alter.

`  column_name  `

  - The name of the column to add into the index or to remove from the index.

### DROP INDEX

Removes a secondary index.

#### Syntax

``` text
DROP INDEX [ IF EXISTS ] index_name
```

#### Description

Use the `  DROP INDEX  ` statement to drop a secondary index.

#### Parameters

`  IF EXISTS  `

  - If an index of the specified name doesn't exist, then the `  DROP  ` statement has no effect and no error is generated.

`  index_name  `

  - The name of the index to drop.

## SEARCH INDEX statements

This section has information about the `  CREATE SEARCH INDEX  ` , `  ALTER SEARCH INDEX  ` , and `  DROP SEARCH INDEX  ` statements.

### CREATE SEARCH INDEX

Use the `  CREATE SEARCH INDEX  ` statement to define search indexes. For more information, see [Search indexes](/spanner/docs/full-text-search/search-indexes) .

#### Syntax

``` text
CREATE SEARCH INDEX index_name
ON table_name ( token_column_list )
[ storing_clause ] [ partition_clause ]
[ orderby_clause ] [ where_clause ]
[ interleave_clause ] [ options_clause ]

where index_name is:
    {a—z|A—Z}[{a—z|A—Z|0—9|_}+]

and token_column_list is:
    column_name [, ...]

and storing_clause is:
    STORING ( column_name [, ...] )

and partition_clause is:
    PARTITION BY column_name [, ...]

and orderby_clause is:
    ORDER BY column_name [ {ASC | DESC} ]

and where_clause is:
    WHERE column_name IS NOT NULL [AND ...]

and interleave_clause is:
    , INTERLEAVE IN table_name

and options_clause is:
    OPTIONS ( option_name=option_value [, ...] )
```

#### Description

You can use `  CREATE SEARCH INDEX  ` to create search indexes for `  TOKENLIST  ` columns. Adding a search index on a column makes it more efficient to search data in the source column of the `  TOKENLIST  ` .

#### Parameters

`  index_name  `

  - The name of the search index to be created. For naming rules, see [Names](#names) .

`  table_name  `

  - The name of the table to be indexed for search.

`  token_column_list  `

  - A list of `  TOKENLIST  ` columns to be indexed for search.

`  STORING  `

  - Provides a mechanism for duplicating data from the table into the search index. This is the same as `  STORING  ` in secondary indexes. For more information, see [`  STORING  ` clause](/spanner/docs/secondary-indexes#storing_clause) .

`  PARTITION BY  `

  - A list of columns to partition the search index by. Partition columns subdivide the index into smaller units, one for each unique partition. Queries can only search within a single partition at a time. Queries against partitioned indexes are generally more efficient than queries against unpartitioned indexes because only splits from a single partition need to be read.

`  ORDER BY  `

  - A list of `  INT64  ` columns that the search index will store rows in that order within a partition. The column must be `  NOT NULL  ` , or the index must define `  WHERE IS NOT NULL  ` . This property can support at most one column.

`  WHERE IS NOT NULL  `

  - Rows that contain NULL in any of the columns listed in this clause aren't included in the index. The columns must be stored in the index, including key columns and columns present in the `  STORING  ` clause.

`  INTERLEAVE IN  `

  - Similarly to secondary indexes [INTERLEAVE IN](#create-index-interleave) , search indexes can be interleaved in an ancestor table of the base table. The primary reason to use interleaved search indexes is to colocate base table data with index data for small partitions.

  - Interleaved search indexes have three restrictions:
    
      - Only sort-order sharded indexes can be interleaved.
      - Search indexes can only be interleaved in top-level tables (and not in child tables).
      - Like interleaved tables and secondary indexes, the key of the parent table must be a prefix of the interleaved search index's `  PARTITION BY  ` columns.

`  OPTIONS  `

  - A list of key value pairs that overrides the default settings of the search index.
    
      - `  sort_order_sharding  ` When `  true  ` , the search index will be sharded by one or more columns specified in the `  ORDER BY  ` clause. When `  false  ` , the search index is sharded uniformly. Default value is `  false  ` . See [search index sharding](/spanner/docs/full-text-search/search-indexes#search_index_sharding) for more details.

### ALTER SEARCH INDEX

Use the `  ALTER SEARCH INDEX  ` statement to add or remove columns from the search indexes.

#### Syntax

``` text
ALTER SEARCH INDEX index_name {ADD|DROP} [STORED] COLUMN column_name
```

#### Description

Add a `  TOKENLIST  ` column into a search index or remove an existing `  TOKENLIST  ` column from a search index. Use `  STORED COLUMN  ` to add or remove stored columns from a search index.

#### Parameters

`  index_name  `

  - The name of the search index to alter.

`  column_name  `

  - The name of the column to add into the index or to remove from the search index.

### DROP SEARCH INDEX

Removes a search index.

#### Syntax

``` text
DROP SEARCH INDEX [ IF EXISTS ] index_name
```

#### Description

Use the `  DROP SEARCH INDEX  ` statement to drop a search index.

#### Parameters

`  IF EXISTS  `

  - If a search index with the specified name doesn't exist, then the `  DROP  ` statement has no effect and no error is generated.

`  index_name  `

  - The name of the search index to drop.

## VIEW statements

This section has information about the `  CREATE VIEW  ` , `  CREATE OR REPLACE VIEW  ` , and `  DROP VIEW  ` statements.

### CREATE VIEW and CREATE OR REPLACE VIEW

Use the `  CREATE VIEW  ` or `  CREATE OR REPLACE VIEW  ` statement to define a [view](/spanner/docs/views) .

#### Syntax

``` text
{ CREATE VIEW | CREATE OR REPLACE VIEW } view_name
SQL SECURITY { INVOKER | DEFINER }
AS query
```

#### Description

`  CREATE VIEW  ` defines a new view in the current database. If a view named `  view_name  ` exists, the `  CREATE VIEW  ` statement fails.

`  CREATE OR REPLACE VIEW  ` defines a new view in the current database. If a view named `  view_name  ` exists, its definition is replaced. Use this statement to replace the security type of a view.

#### Parameters

`  view_name  `

  - The name of the view to be created. For naming rules, see [Names](#names) .

`  SQL SECURITY  `

  - The security type can be either `  INVOKER  ` or `  DEFINER  ` . Depending on the security type of the view, Spanner may or may not access check the objects referenced in the view against the database role of the principal who invoked the query. For more information, see [About views](/spanner/docs/views) .

AS `  query  `

  - The query that defines the view content.
    
      - The query must specify a name for each item in the [SELECT list](/spanner/docs/query-syntax#select_list) .
    
      - The query cannot include [query parameters](/spanner/docs/lexical#query_parameters) .
    
      - GoogleSQL disregards any [ORDER BY clause](/spanner/docs/query-syntax#order_by_clause) in this query that isn't paired with a [LIMIT clause](/spanner/docs/query-syntax#limit_and_offset_clause) .
    
    See [Query syntax](/spanner/docs/query-syntax) for information on constructing a query.

### DROP VIEW

Removes a view.

#### Syntax

``` text
DROP VIEW [ IF EXISTS ] view_name
```

#### Description

Use the `  DROP VIEW  ` statement to remove a view from the database. Unless the `  IF EXISTS  ` clause is specified, the statement fails if the view doesn't exist.

#### Parameters

`  IF EXISTS  `

  - If the view doesn't exist, the `  DROP  ` statement has no effect and doesn't generate an error.

`  view_name  `

  - The name of the view to drop.

## CHANGE STREAM statements

This section has information about the `  CREATE CHANGE STREAM  ` , `  ALTER CHANGE STREAM  ` , and `  DROP CHANGE STREAM  ` statements.

### CREATE CHANGE STREAM

Defines a new [change stream](/spanner/docs/change-streams) .

#### Syntax

``` text
CREATE CHANGE STREAM [ IF NOT EXISTS ] change_stream_name
[ FOR { table_columns [, ... ] | ALL } ]
[ OPTIONS ( change_stream_option [, ... ] ) ]

where table_columns is:
    table_name [ ( [ column_name, ... ] ) ]

and change_stream_option is:
    { retention_period = 'duration' |
      value_capture_type = { 'OLD_AND_NEW_VALUES' | 'NEW_ROW' |'NEW_VALUES' | 'NEW_ROW_AND_OLD_VALUES' } |
      exclude_ttl_deletes = { false | true } |
      exclude_insert = { false | true } |
      exclude_update = { false | true } |
      exclude_delete = { false | true } |
      allow_txn_exclusion = { false | true } }
```

#### Description

`  CREATE CHANGE STREAM  ` defines a new change stream in the current database. For more information, see [Create a change stream](/spanner/docs/change-streams/manage#create) .

#### Parameters

`  IF NOT EXISTS  `

  - If a change stream exists with the same name, the `  CREATE  ` statement has no effect and doesn't generate an error.

`  change_stream_name  `

  - The name of the change stream to be created. The maximum number of characters of a change stream name is 128. However, the name you provide is prepended with the 10 character prefix, `  READ_JSON_  ` . Because of this, the maximum number of characters you can assign to `  change_stream_name  ` \` is 118. For further naming rules, see [Names](#names) .

`  FOR {  ` `  table_columns  ` `  [, ... ] | ALL }  `

  - The `  FOR  ` clause defines the tables and columns that are watched by the change stream.

  - You can specify a list of `  table_columns  ` to watch, where `  table_columns  ` can be either of the following:
    
      - `  table_name  ` : This watches the entire table, including all of the future columns when they are added to this table.
    
      - `  table_name  ` `  ( [  ` `  column_name  ` `  , ... ] )  ` : You can optionally specify a list of zero or more non-key columns following the table name. This watches only the primary key and the listed non-key columns of the table. With an empty list of non-key columns, `  table_name  ` `  ()  ` watches only the primary key.
    
    **Note:** Primary key columns are always watched by the change stream. You only need to list the non-key columns. Listing any primary key columns is not allowed.

  - `  ALL  ` lets you watch all tables and columns in the entire database, including all of the future tables and columns as soon as they are created.

  - When the `  FOR  ` clause is omitted, the change stream watches nothing.

`  OPTIONS (  ` `  change_stream_option  ` `  [, ... ] )  `

  - The `  retention_period = 'duration'  ` option lets you specify how long a change stream retains its data. The duration must be in the range `  [1d, 7d]  ` and can be specified in days, hours, minutes, or seconds. For example, the values `  1d  ` , `  24h  ` , `  1440m  ` , and `  86400s  ` are equivalent. The default is 1 day. For more information, see [Data retention](/spanner/docs/change-streams#data-retention) .

  - The `  value_capture_type  ` option controls which values are captured for a changed row. It can be `  OLD_AND_NEW_VALUES  ` (default), `  NEW_VALUES  ` , `  NEW_ROW  ` , or `  NEW_ROW_AND_OLD_VALUES  ` . For more information, see [Value capture type](/spanner/docs/change-streams#value-capture-type) .

  - The `  exclude_ttl_deletes  ` configuration parameter lets you filter out [time to live based deletes](/spanner/docs#ttl-filter) from your change stream. When you set this filter, only future TTL-based deletes are removed. It can be set to `  false  ` (default) or `  true  ` . For more information, see [TTL-based deletes filter](/spanner/docs/change-streams#ttl-filter) .

  - The `  exclude_insert  ` configuration parameter lets you filter out all `  INSERT  ` table modifications from your change stream. It can be set to `  false  ` (default) or `  true  ` . For more information, see [Table modification type filters](/spanner/docs/change-streams#mod-type-filter) .

  - The `  exclude_update  ` configuration parameter lets you filter out all `  UPDATE  ` table modifications from your change stream. It can be set to `  false  ` (default) or `  true  ` . For more information, see [Table modification type filters](/spanner/docs/change-streams#mod-type-filter) .

  - The `  exclude_delete  ` configuration parameter lets you filter out all `  DELETE  ` table modifications from your change stream. It can be set to `  false  ` (default) or `  true  ` . For more information, see [Table modification type filters](/spanner/docs/change-streams#mod-type-filter) .

  - The `  allow_txn_exclusion  ` configuration parameter lets you enable transaction-level records exclusion. It can be set to `  false  ` (default) or `  true  ` . For more information, see [Transaction-level records exclusion](/spanner/docs/change-streams#transaction-exclusion) .

### ALTER CHANGE STREAM

Changes the definition of a change stream.

#### Syntax

``` text
ALTER CHANGE STREAM change_stream_name
    action

where action is:
    { SET FOR { table_columns [, ... ] | ALL } |
      DROP FOR ALL |
      SET OPTIONS ( change_stream_option [, ... ] ) }

and table_columns is:
    table_name [ ( [ column_name, ... ] ) ]

and change_stream_option is:
    { retention_period = { 'duration' | null } |
      value_capture_type = { 'OLD_AND_NEW_VALUES' | 'NEW_ROW' | 'NEW_VALUES' | 'NEW_ROW_AND_OLD_VALUES' | null } |
      exclude_ttl_deletes = { false | true | null } |
      exclude_insert = { false | true | null } |
      exclude_update = { false | true | null } |
      exclude_delete = { false | true | null } |
      allow_txn_exclusion = { false | true | null } }
```

#### Description

`  ALTER CHANGE STREAM  ` changes the definition of an existing change stream. For more information, see [Modify a change stream](/spanner/docs/change-streams/manage#modify) .

#### Parameters

`  change_stream_name  `

  - The name of an existing change stream to alter.

`  SET FOR {  ` `  table_columns  ` `  [, ... ] | ALL }  `

  - Sets a new `  FOR  ` clause to modify what the change stream watches, using the same syntax as `  CREATE CHANGE STREAM  ` .

`  DROP FOR ALL  `

  - [Suspends a change stream](/spanner/docs/change-streams/manage#suspend) to watch nothing.

`  SET OPTIONS  `

  - Sets options on the change stream (such as `  retention_period  ` , `  value_capture_type  ` , `  exclude_ttl_deletes  ` , `  exclude_insert  ` , `  exclude_update  ` , `  exclude_delete  ` , and `  allow_txn_exclusion  ` ), using the same syntax as `  CREATE CHANGE STREAM  ` .

  - Setting an option to `  null  ` is equivalent to setting it to the default value.

### DROP CHANGE STREAM

Removes a change stream.

#### Syntax

``` text
DROP CHANGE STREAM [ IF EXISTS ] change_stream_name
```

#### Description

Use the `  DROP CHANGE STREAM  ` statement to remove a change stream from the database and delete its data change records.

#### Parameters

`  IF EXISTS  `

  - If a change stream of the specified name doesn't exist, the `  DROP  ` statement has no effect and doesn't generate an error.

`  change_stream_name  `

  - The name of the change stream to drop.

## ROLE statements

This section has information about the `  CREATE ROLE  ` and `  DROP ROLE  ` statements.

### CREATE ROLE

Defines a new database role.

#### Syntax

``` text
CREATE ROLE database_role_name
```

#### Description

`  CREATE ROLE  ` defines a new database role. Database roles are collections of [fine-grained access control](/spanner/docs/fgac-about) privileges. You can create only one role with this statement.

#### Parameters

`  database_role_name  `

  - The name of the database role to create. The role name `  public  ` and role names starting with `  spanner_  ` are reserved for [system roles](/spanner/docs/fgac-system-roles) . See also [Names](#names) .

#### Example

This example creates the database role `  hr_manager  ` .

`  CREATE ROLE hr_manager  `

### DROP ROLE

Drops a database role.

#### Syntax

``` text
DROP ROLE database_role_name
```

#### Description

`  DROP ROLE  ` drops a database role. You can drop only one role with this statement.

You can't drop a database role if it has any privileges granted to it. All privileges granted to a database role must be revoked before the role can be dropped. You can drop a database role whether or not access to it is granted to IAM principals.

Dropping a role automatically revokes its membership in other roles and revokes the membership of its members.

You can't drop [system roles](/spanner/docs/fgac-system-roles) .

#### Parameters

`  database_role_name  `

  - The name of the database role to drop.

#### Example

This example drops the database role `  hr_manager  ` .

`  DROP ROLE hr_manager  `

## GRANT and REVOKE statements

This section has information about the `  GRANT  ` and `  REVOKE  ` statements.

### GRANT

Grants privileges that allow database roles to access database objects.

#### Syntax

``` text
GRANT { SELECT | INSERT | UPDATE | DELETE } ON TABLE table_list TO ROLE role_list

GRANT { SELECT | INSERT | UPDATE }(column_list)
   ON TABLE table_list | ON ALL TABLES IN SCHEMA schema_name [, ...]
   TO ROLE role_list

GRANT SELECT
    ON CHANGE STREAM change_stream_list
        | ON ALL CHANGE STREAMS IN SCHEMA schema_name [, ...] }
    TO ROLE role_list

GRANT SELECT ON VIEW view_list | ON ALL VIEWS IN SCHEMA schema_name [, ...]
    TO ROLE role_list

GRANT EXECUTE ON TABLE FUNCTION function_list
    TO ROLE role_list.

GRANT ROLE role_list
    TO ROLE role_list

GRANT USAGE ON SCHEMA [DEFAULT | schema_name_list] TO ROLE role_list

where table_list is:
      table_name [, ...]

and column_list is:
    column_name [,...]

and view_list is:
    view_name [, ...]

and change_stream_list is:
    change_stream_name [, ...]

and function_list is:
    change_stream_read_function_name [, ...]

and schema_name_list is:
    schema_name [, ...]

and role_list is:
    database_role_name [, ...]
```

#### Description

For [fine-grained access control](/spanner/docs/fgac-about) , grants privileges on one or more tables, views, change streams, or change stream read functions to database roles. Also grants database roles to other database roles to create a database role hierarchy with inheritance. When granting `  SELECT  ` , `  INSERT  ` , or `  UPDATE  ` on a table, optionally grants privileges on only a subset of table columns.

#### Parameters

`  table_name  `

  - The name of an existing table.

`  column_name  `

  - The name of an existing column in the specified table.

`  view_name  `

  - The name of an existing view.

`  change_stream_name  `

  - The name of an existing change stream.

`  change_stream_read_function_name  `

  - The name of an existing read function for a change stream. For more information, see [Change stream read functions and query syntax](../../change-streams/details#change_stream_query_syntax) .

`  schema_name  `

  - The name of the schema.

`  database_role_name  `

  - The name of an existing database role.

#### Notes and restrictions

  - Identifiers for database objects named in the `  GRANT  ` statement must use the case that was specified when the object was created. For example, if you created a table with a name that is in all lower case with a capitalized first letter, you must use that same case in the `  GRANT  ` statement. Table-valued functions (TVFs) get automatically created with a prefix added to the change stream name, so ensure that you use the proper case for both the prefix and the change stream name. For more information about TVFs, see [Change stream query syntax](../../change-streams/details#change_stream_query_syntax) . created a table with a name that is in all lower case with a capitalized first letter, you must use that same case in the `  GRANT  ` statement. For each change stream, GoogleSQL automatically creates a change stream read function with a name that consists of a prefix added to the change stream name, so ensure that you use the proper case for both the prefix and the change stream name. For more information about change stream read functions, see [Change stream query syntax](../../change-streams/details#change_stream_query_syntax) .

  - When granting column-level privileges on multiple tables, each table must contain the named columns.

  - If a table contains a column that is marked `  NOT NULL  ` and has no default value, you can't insert into the table unless you have the `  INSERT  ` privilege on that column.

  - After granting `  SELECT  ` on a change stream to a role, grant `  EXECUTE  ` to that role on the read function for the change stream. For information about change stream read functions, see [Change stream read functions and query syntax](../../change-streams/details#change_stream_query_syntax) .

  - Granting `  SELECT  ` on a table doesn't grant `  SELECT  ` on the change stream that tracks it. You must make a separate grant for the change stream.

  - `  ALL TABLES IN SCHEMA  ` , `  ALL CHANGE STREAMS IN SCHEMA  ` , and `  ALL VIEWS IN SCHEMA  ` performs a one-time bulk grant for a role to all those database objects that use the schema, but not to future objects that use the schema.

#### Examples

The following example grants `  SELECT  ` on the `  employees  ` table to the `  hr_rep  ` role. Grantees of the `  hr_rep  ` role can read all columns of `  employees  ` .

``` text
GRANT SELECT ON TABLE employees TO ROLE hr_rep;
```

The next example grants `  SELECT  ` on a subset of columns of the `  contractors  ` table to the `  hr_rep  ` role. Grantees of the `  hr_rep  ` role can read-only the named columns.

``` text
GRANT SELECT(name, address, phone) ON TABLE contractors TO ROLE hr_rep;
```

The next example mixes table-level and column-level grants. `  hr_manager  ` can read all table columns, but can update only the `  location  ` column.

``` text
GRANT SELECT, UPDATE(location) ON TABLE employees TO ROLE hr_manager;
```

The next example makes column-level grants on two tables. Both tables must contain the `  name  ` , `  level  ` , and `  location  ` columns.

``` text
GRANT SELECT(name, level, location), UPDATE(location) ON TABLE employees, contractors TO ROLE hr_manager;
```

The next example grants `  INSERT  ` on a subset of columns of the `  employees  ` table.

``` text
GRANT INSERT(name, cost_center, location, manager) ON TABLE employees TO ROLE hr_manager;
```

The next example grants the database role `  pii_access  ` to the roles `  hr_manager  ` and `  hr_director  ` . The `  hr_manager  ` and `  hr_director  ` roles are *members* of `  pii_access  ` and inherit the privileges that were granted to `  pii_access  ` . For more information, see [Database role hierarchies and inheritance](../../fgac-about#role_hierarchy) .

``` text
GRANT ROLE pii_access TO ROLE hr_manager, hr_director;
```

### REVOKE

Revokes privileges that allow database roles access to database objects.

#### Syntax

``` text
REVOKE { SELECT | INSERT | UPDATE | DELETE } ON TABLE table_list FROM ROLE role_list

REVOKE { SELECT | INSERT | UPDATE }(column_list) ON TABLE table_list FROM ROLE role_list

REVOKE SELECT ON VIEW view_list FROM ROLE role_list

REVOKE SELECT ON CHANGE STREAM change_stream_list FROM ROLE role_list

REVOKE EXECUTE ON TABLE FUNCTION function_list FROM ROLE role_list

REVOKE ROLE role_list FROM ROLE role_list

and table_list is:
    table_name [, ...]

and column_list is:
    column_name [,...]

and view_list is:
    view_name [, ...]

and change_stream_list is:
    change_stream_name [, ...]

and function_list is:
    change_stream_read_function_name [, ...]

and role_list is:
    database_role_name [, ...]
```

#### Description

For [fine-grained access control](/spanner/docs/fgac-about) , revokes privileges on one or more tables, views, change streams, or change stream read functions from database roles. Also revokes database roles from other database roles. When revoking `  SELECT  ` , `  INSERT  ` , or `  UPDATE  ` on a table, optionally revokes privileges on only a subset of table columns.

#### Parameters

`  table_name  `

  - The name of an existing table.

`  column_name  `

  - The name of an existing column in the previously specified table.

`  view_name  `

  - The name of an existing view.

`  change_stream_name  `

  - The name of an existing change stream.

`  change_stream_read_function_name  `

  - The name of an existing read function for a change stream. For more information, see [Change stream read functions and query syntax](../../change-streams/details#change_stream_query_syntax) .

`  database_role_name  `

  - The name of an existing database role.

#### Notes and restrictions

  - Identifiers for database objects named in the `  REVOKE  ` statement must use the case that was specified when the object was created. For example, if you created a table with a name that is in all lower case with a capitalized first letter, you must use that same case in the `  REVOKE  ` statement. For each change stream, GoogleSQL automatically creates a change stream read function with a name that consists of a prefix added to the change stream name, so ensure that you use the proper case for both the prefix and the change stream name. For more information about change stream read functions, see [Change stream query syntax](../../change-streams/details#change_stream_query_syntax) .

  - When revoking column-level privileges on multiple tables, each table must contain the named columns.

  - A `  REVOKE  ` statement at the column level has no effect if privileges were granted at the table level.

  - After revoking `  SELECT  ` on a change stream from a role, revoke `  EXECUTE  ` on the change stream's read function from that role.

  - Revoking `  SELECT  ` on a change stream doesn't revoke any privileges on the table that it tracks.

#### Examples

The following example revokes `  SELECT  ` on the `  employees  ` table from the role `  hr_rep  ` .

`  REVOKE SELECT ON TABLE employees FROM ROLE hr_rep;  `

The next example revokes `  SELECT  ` on a subset of columns of the `  contractors  ` table from the role `  hr_rep  ` .

`  REVOKE SELECT(name, address, phone) ON TABLE contractors FROM ROLE hr_rep;  `

The next example shows revoking both table-level and column-level privileges in a single statement.

`  REVOKE SELECT, UPDATE(location) ON TABLE employees FROM ROLE hr_manager;  `

The next example revokes column-level grants on two tables. Both tables must contain the `  name  ` , `  level  ` , and `  location  ` columns.

`  REVOKE SELECT(name, level, location), UPDATE(location) ON TABLE employees, contractors FROM ROLE hr_manager;  `

The next example revokes `  INSERT  ` on a subset of columns.

`  REVOKE INSERT(name, cost_center, location, manager) ON TABLE employees FROM ROLE hr_manager;  `

The following example revokes the database role `  pii_access  ` from the `  hr_manager  ` and `  hr_director  ` database roles. The `  hr_manager  ` and `  hr_director  ` roles lose any privileges that they inherited from `  pii_access  ` .

`  REVOKE ROLE pii_access FROM ROLE hr_manager, hr_director;  `

## SEQUENCE statements

This section has information about the `  CREATE SEQUENCE,  ` ALTER SEQUENCE `  , and  ` DROP SEQUENCE\` statements.

### CREATE SEQUENCE

Creates a sequence object.

#### Syntax

``` text
CREATE SEQUENCE
    [ IF NOT EXISTS ] sequence_name
    [ sequence_option_clause ... ]
    [ OPTIONS ( sequence_options ) ]

where sequence_option_clause is:
    BIT_REVERSED_POSITIVE
    | SKIP RANGE skip_range_min, skip_range_max
    | START COUNTER WITH start_with_counter
```

#### Description

When you use a `  CREATE SEQUENCE  ` statement, Spanner creates a schema object that you can poll for values using the `  GET_NEXT_SEQUENCE_VALUE  ` function.

#### Parameters

`  IF NOT EXISTS  `

  - If a sequence already exists with the same name, then the CREATE statement has no effect and no error is generated.

`  sequence_name  `

  - The name of the sequence to create. For naming rules, see [Names](#names) .

`  OPTIONS ( sequence_options )  `

  - Use this clause to set an option on the specified sequence. Each sequence option uses a `  key=value  ` pair, where key is the option name, and value is a literal. Multiple options are separated by commas. Options use the following syntax:
    
    ``` text
    OPTIONS (option_name = value [,...])
    ```
    
    A sequence accepts the following options:
    
      - The `  sequence_kind  ` option accepts a `  STRING  ` to indicate the type of sequence to use. At this time, `  bit_reversed_positive  ` is the only valid type and it's a required option.
      - <span id="skip-range">The `  skip_range_min  ` and `  skip_range_max  ` parameters cause the sequence to skip the numbers in this range when calling `  GET_NEXT_SEQUENCE_VALUE  ` . The skipped range is inclusive. These parameters are both integers that have a default value of NULL. The accepted values for `  skip_range_min  ` is any value that is less than or equal to `  skip_range_max  ` . The accepted values for `  skip_range_max  ` is any value that is more than or equal to `  skip_range_min  ` .</span>
      - The `  start_with_counter  ` option is a positive `  INT64  ` value that Spanner uses to set the next value for the internal sequence counter. For example, the next time that Spanner obtains a value from the bit-reversed sequence, it begins with `  start_with_counter  ` . Spanner bit reverses this value before returning it to the client. The default value is `  1  ` .

#### Examples

``` text
# Create a positive bit-reversed sequence to use in a primary key.

CREATE SEQUENCE MySequence OPTIONS (
    sequence_kind='bit_reversed_positive',
    skip_range_min = 1,
    skip_range_max = 1000,
    start_with_counter = 50);

# Create a table that uses the sequence for a key column.
CREATE TABLE Singers (
  SingerId INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(SEQUENCE MySequence)),
  FirstName  STRING(1024),
  LastName   STRING(1024),
  SingerInfo googlesql.example.SingerInfo,
  BirthDate  DATE
) PRIMARY KEY (SingerId);
```

Use the following SQL to query information about sequences.

``` text
SELECT * FROM information_schema.sequences;
SELECT * FROM information_schema.sequence_options;
```

### ALTER SEQUENCE

Makes changes to the sequence object.

#### Syntax

``` text
ALTER SEQUENCE sequence_name
    { SET OPTIONS sequence_options | sequence_option_clause ...  }

where sequence_option_clause is:
    { { SKIP RANGE skip_range_min, skip_range_max | NO SKIP RANGE }
      | RESTART COUNTER WITH counter_restart }
```

#### Description

`  ALTER SEQUENCE  ` makes changes to the specified sequence schema object. Executing this statement doesn't affect values the sequence already generated. If the `  ALTER SEQUENCE  ` statement doesn't include an option, the current value of the option remains the same.

#### Parameters

`  sequence_name  `

  - The name of an existing sequence to alter. `  sequence_name  ` is case sensitive. Don't include the path in the `  sequence_name  ` .

`  SET OPTIONS ( sequence_options )  `

  - Use this clause to set an option on the specified sequence. Each sequence option uses a `  key=value  ` pair, where key is the option name, and value is a literal. Multiple options are separated by commas. Options use the following syntax:
    
    ``` text
    SET OPTIONS (option_name = value [,...])
    ```
    
    This parameter offers the same options as [`  CREATE SEQUENCE  `](#create-sequence) .

#### Examples

``` text
# Alter the sequence to include a skipped range. This is useful when you are
# migrating from a regular sequence with sequential data
ALTER SEQUENCE MySequence
SET OPTIONS (skip_range_min=1, skip_range_max=1234567);
```

### DROP SEQUENCE

Drops a specific sequence.

#### Syntax

``` text
DROP SEQUENCE [IF EXISTS] sequence_name
```

#### Description

`  DROP SEQUENCE  ` drops a specific sequence. Spanner can't drop a sequence if its name appears in a sequence function that is used in a column default value or a view.

#### Parameters

`  sequence_name  `

  - The name of the existing sequence to drop. `  IF EXISTS  `

  - If a sequence of the specified name doesn't exist, then the `  DROP  ` statement has no effect and no error is generated.

## STATISTICS statements

This section has information about the `  ALTER STATISTICS  ` and `  ANALYZE  ` statements.

### ALTER STATISTICS

Changes the definition of a query optimizer statistics package.

#### Syntax

``` text
ALTER STATISTICS package_name
    action

where package_name is:
    {a—z}[{a—z|0—9|_|-}+]{a—z|0—9}

and action is:
    SET OPTIONS ( options_def )

and options_def is:
    { allow_gc = { true | false } }
```

#### Description

`  ALTER STATISTICS  ` changes the definition of a query optimizer statistics package.

`  SET OPTIONS  `

  - Use this clause to set an option on the specified statistics package.

#### Parameters

`  package_name  `

  - The name of an existing query optimizer statistics package whose attributes are to be altered.
    
    To fetch existing statistics packages, run the following query:
    
    ``` text
    SELECT s.package_name AS package_name, s.allow_gc AS allow_gc FROM INFORMATION_SCHEMA.SPANNER_STATISTICS s;
    ```

`  options_def  `

  - The `  allow_gc = { true | false }  ` option lets you specify whether a given statistics package is garbage collected. A package must be set as `  allow_gc=false  ` if it is used in a query hint. For more information, see [Garbage collection of statistics packages](/spanner/docs/query-optimizer/overview#statistics-gc) .

### ANALYZE

Start a new query optimizer statistics package construction.

#### Syntax

``` text
ANALYZE
```

#### Description

`  ANALYZE  ` starts a new query optimizer statistics package construction.

## MODEL statements

This section has information about the `  CREATE MODEL  ` , `  ALTER MODEL  ` , and `  DROP MODEL  ` statements.

### CREATE MODEL and CREATE OR REPLACE MODEL

Use the `  CREATE MODEL  ` or `  CREATE OR REPLACE MODEL  ` statement to define an ML model.

**Note:** Spanner Vertex AI integration supports only classifier, regression, and text ML models.

#### Syntax

``` text
{ CREATE MODEL | CREATE OR REPLACE MODEL | CREATE MODEL IF NOT EXISTS } model_name
[INPUT ( column_list ) OUTPUT ( column_list )]
REMOTE
[OPTIONS ( model_options )]

where column_list is:
   { column_name data_type [OPTIONS ( model_column_options )] [, ... ] }

and model_column_options is:
    {
      required = { true | false }
    }

and model_options is:
    {
      endpoint = '{endpoint_address}',
      endpoints = [ '{endpoint_address}' [, ...] ],
      default_batch_size = int64_value
    }
```

#### Description

`  CREATE MODEL  ` registers a reference to the Vertex AI ML model in the current database. If a model named `  model_name  ` already exists, the `  CREATE MODEL  ` statement fails.

`  CREATE OR REPLACE MODEL  ` registers a reference to the Vertex AI ML model in the current database. If a model named `  model_name  ` already exists, its definition is replaced.

`  CREATE MODEL IF NOT EXISTS  ` registers a reference to the Vertex AI ML model in the current database. If a model named `  model_name  ` already exists, the `  CREATE MODEL IF NOT EXISTS  ` statement does not have any effect and no error is generated.

As soon as the model reference is registered in a database, it can be used from queries that use the [ML.Predict](/spanner/docs/reference/standard-sql/ml-functions#mlpredict) function.

Model registration doesn't result in copying a model from the Vertex AI to a database, but only in creation of a reference to this models' endpoint hosted in the Vertex AI. If the model's endpoint gets removed from the Vertex AI, Spanner queries referencing this model fail.

#### Model endpoint access control

To be able to access a registered Vertex AI model endpoint from Spanner, you need to grant access permission to Spanner's [service agent](/iam/docs/service-agents) account.

Spanner creates the service agent and grants the necessary permissions when Spanner executes the first `  MODEL  ` DDL statement. If both the Spanner database and the Vertex AI endpoint are in the same project, then no additional setup is required.

If the Spanner service agent account doesn't exist for your Spanner project, [create](/sdk/gcloud/reference/beta/services/identity/create) it by running the following command:

``` text
gcloud beta services identity create --service=spanner.googleapis.com --project={PROJECT}`
```

Follow the steps described in the [following tutorial](/iam/docs/manage-access-service-accounts#grant-single-role) to grant the [`  Spanner API Service Agent  `](/iam/docs/roles-permissions/spanner#spanner.serviceAgent) role to the Spanner [service agent](/iam/docs/service-agents) account `  service-{PROJECT}@gcp-sa-spanner.iam.gserviceaccount.com  ` on your Vertex AI project.

#### Parameters

`  model_name  `

  - The name of the model to be created. See [Names](#names) .

`  INPUT ( column_list ) OUTPUT ( column_list )  `

  - Lists of columns that define model inputs (that is, features) and outputs (that is, labels). The following [types](/spanner/docs/reference/standard-sql/data-types) (used in the `  type  ` field of `  column_list  ` ) are supported: `  BOOL  ` , `  BYTES  ` , `  FLOAT32  ` , `  FLOAT64  ` , `  INT64  ` , `  STRING  ` , and `  ARRAY  ` of listed types.
    
      - Map the model's input or output columns with 32-bit integer types to `  INT64  ` .

  - If the Vertex AI endpoint has [instance and prediction schemas](/vertex-ai/docs/reference/rest/v1/PredictSchemata) , Spanner validates the provided `  INPUT  ` and `  OUTPUT  ` clauses against those remote schemas. You can also omit `  INPUT  ` and `  OUTPUT  ` clauses, letting Spanner automatically discover the endpoint schema.

  - If the Vertex AI endpoint does not have [instance and prediction schemas](/vertex-ai/docs/reference/rest/v1/PredictSchemata) , `  INPUT  ` and `  OUTPUT  ` clauses must be provided. Spanner doesn't perform validation and mismatches result in runtime errors. We strongly recommend providing instance and prediction schemas, especially when using custom models.

`  model_column_options  `

  - ***required*** lets you mark input or output columns as optional to match your Vertex AI schema.
      - Input columns cannot be declared optional if the instance field is required.
      - Optional input columns can be omitted in ML function calls.
      - Required input columns must be provided to ML function calls.
      - Output columns cannot be declared as required if the prediction field is optional.
      - Optional outputs columns can return NULL if the endpoint does not produce them.
      - Required outputs columns must be produced by the endpoint.

`  model_options  `

  - ***endpoint*** is the address of the Vertex AI endpoint to connect to. Mutually exclusive with endpoints option. Supported formats:
    
      - `  //aiplatform.googleapis.com/projects/{project}/locations/{location}/endpoints/{endpoint}  ` .
      - `  //aiplatform.googleapis.com/projects/{project}/locations/{location}/publishers/{publisher}/models/{endpoint}  ` .
      - `  https://{location}-aiplatform.googleapis.com/v1/projects/{project}/locations/{location}/endpoints/{endpoint}  ` .
      - `  https://{location}-aiplatform.googleapis.com/v1/projects/{project}/locations/{location}/publishers/{publisher}/models/{endpoint}  ` .

  - ***endpoints*** is a list of addresses of Vertex AI endpoints to connect to. Mutually exclusive with endpoint option. Prediction starts with the first endpoint on the list and fails over in the specified order. Endpoints can host different models as long as their schemas can be merged together:
    
      - Each column's name must use the same case across all endpoints
      - Each column's type must be the same across all endpoints.
      - Each input column is considered required if at least one endpoint requires it
      - Each output column is considered required only if all endpoints require it

  - ***default\_batch\_size*** specifies the maximum number of rows per remote inference call. The value must be between 1 and 10. For models that don't support batching, you must set the value to 1. This default value can be overridden with per-query hints.

### ALTER MODEL

Changes the definition of a model.

**Note:** Spanner Vertex AI integration supports only classifier, regression, and text ML models.

#### Syntax

``` text
ALTER MODEL [ IF EXISTS ] model_name
SET OPTIONS ( model_options )

where model_options is:
    {
      endpoint = '{endpoint_address}',
      endpoints = [ '{endpoint_address}' [, ...] ],
      default_batch_size = int64_value
    }
```

#### Description

`  ALTER MODEL  ` changes the definition of an existing table.

#### Parameters

`  model_name  `

  - The name of an existing model whose attributes are to be altered.

`  SET OPTIONS  `

  - Sets options on the model, using the same syntax as `  CREATE MODEL  ` .

  - Setting an option to `  null  ` is equivalent to setting it to the default value.

  - The following list of options which can be updated:
    
      - ***endpoint*** is the address of the Vertex AI endpoint to connect to.
      - ***endpoints*** is a list of addresses of Vertex AI endpoints to connect to. Mutually exclusive with endpoint option.
      - ***default\_batch\_size*** specifies the maximum number of rows per remote inference call. The value must be between 1 and 10. For models that don't support batching, you must set the value to 1. This default value can be overridden with per-query hints.

### DROP MODEL

Removes a model.

#### Syntax

``` text
DROP MODEL [ IF EXISTS ] model_name
```

#### Description

Use the `  DROP MODEL  ` statement to remove a model definition from the database. Unless the `  IF EXISTS  ` clause is specified, the statement fails if the model doesn't exist.

After you delete a model definition, all SQL queries referencing the deleted model fail. Dropping a model definition does not affect the underlying the Vertex AI endpoint that this model is attached to.

#### Parameters

`  model_name  `

  - The name of the model to drop.

## VECTOR INDEX statements

If you have a table with a large amount of vector data, you can use a vector index to perform similarity searches and [approximate nearest neighbor (ANN)](/spanner/docs/find-approximate-nearest-neighbors) queries efficiently, with the trade-off of reduced [recall](https://developers.google.com/machine-learning/crash-course/classification/precision-and-recall#recall) and more approximate results.

### CREATE VECTOR INDEX

Creates a new vector index on a column of a table.

#### Syntax

``` text
CREATE VECTOR INDEX [ IF NOT EXISTS ] index_name
ON table_name (column_name [, extra_key_column_name, ...] )
[ STORING ( column_name [, ...] ) ]
[ WHERE column_name IS NOT NULL ]
OPTIONS(index_option_list)
```

#### Parameters

`  IF NOT EXISTS  `

  - If there is already a vector index with that name in the table, do nothing.

`  index_name  `

  - The name of the vector index you're creating. This name must be unique for each database.

`  table_name  `

  - The name of the table.

`  column_name  `

  - The name of an embedding column with a type of `  ARRAY<FLOAT64>(vector_length=>INT)  ` or `  ARRAY<FLOAT32>(vector_length=>INT)  ` . The column can't have any child fields. All elements in the array must be non- `  NULL  ` , and all values in the column must have the same array dimensions as defined by `  vector_length  ` . If the embedding column isn't defined as `  NOT NULL  ` , then use the `  WHERE column_name IS NOT NULL  ` clause when creating the vector index. If you include additional `  extra_key_column_name  ` in the vector index, the embedding column must be the first column listed.

`  extra_key_column_name  `

  - The name of one or more non-embedding columns that you use as keys in the index. These columns must appear after `  column_name  ` . Extra keys are arranged as actual keys within the underlying Spanner data structure supporting the index. These extra keys help the query engine speed up ANN queries, similar to how keys are used in [secondary indexes](/spanner/docs/secondary-indexes) . Compared to using `  STORING  ` columns, key columns have the following characteristics:
    
      - They must be [valid key types](/spanner/docs/reference/standard-sql/data-types#valid_key_column_types) .
      - They incur slightly more processing cost than using storing columns.
      - You can't add or drop key columns.

`  WHERE IS NOT NULL  `

  - Rows that contain NULL in any of the columns listed in this clause aren't included in the index. The columns must be present in the indexed columns or `  STORING  ` clause.

`  STORING  `

  - Provides a mechanism for duplicating data from the table into the vector index. This is the same as `  STORING  ` in a secondary index. For more information, see [`  STORING  ` clause](/spanner/docs/secondary-indexes#storing_clause) .

[`  index_option_list  `](#vector_index_option_list)

  - The list of options to set on the vector index.

#### Description

You can only create a new vector index on a column of a table.

#### `     index_option_list    `

The index option list specifies options for the vector index. Spanner creates tree-based vector indexes which use a tree-like structure to partition vector data. Using `  index_option_list  ` , you can define the specific distance metric and search tree specification used to create the vector index. Specify the options in the following format: `  NAME=VALUE, ...  ` .

The following index options are supported:

<table>
<thead>
<tr class="header">
<th><code dir="ltr" translate="no">       NAME      </code></th>
<th><code dir="ltr" translate="no">       VALUE      </code></th>
<th>Details</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       distance_type      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Required. The distance metric used to build the vector index. This value can be <a href="/spanner/docs/reference/standard-sql/mathematical_functions#approx_cosine_distance"><code dir="ltr" translate="no">        COSINE       </code></a> , <a href="/spanner/docs/reference/standard-sql/mathematical_functions#approx_dot_product"><code dir="ltr" translate="no">        DOT_PRODUCT       </code></a> , or <a href="/spanner/docs/reference/standard-sql/mathematical_functions#approx_euclidean_distance"><code dir="ltr" translate="no">        EUCLIDEAN       </code></a> .</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       tree_depth      </code></td>
<td><code dir="ltr" translate="no">       INT      </code></td>
<td>The tree depth (level). This value can be either <code dir="ltr" translate="no">       2      </code> or <code dir="ltr" translate="no">       3      </code> . A tree with 2 levels only has leaves ( <code dir="ltr" translate="no">       num_leaves      </code> ) as nodes. If the dataset has more than 100 million rows, then you can use a tree with 3 levels and add branches ( <code dir="ltr" translate="no">       num_branches      </code> ) to further partition the dataset.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       num_leaves      </code></td>
<td><code dir="ltr" translate="no">       INT      </code></td>
<td>The number of leaves (i.e. potential partitions) for the vector data. You can designate <code dir="ltr" translate="no">       num_leaves      </code> for trees with 2 or 3 levels. We recommend that the number of leaves is <code dir="ltr" translate="no">       number_of_rows_in_dataset/1000      </code> .</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       num_branches      </code></td>
<td><code dir="ltr" translate="no">       INT      </code></td>
<td>The number of branches to further parititon the vector data. You can only designate <code dir="ltr" translate="no">       num_branches      </code> for trees with 3 levels. The number of branches must be fewer than the number of leaves. We recommend that the number of leaves is between <code dir="ltr" translate="no">       1000      </code> and <code dir="ltr" translate="no">       sqrt(number_of_rows_in_dataset)      </code> .</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       disable_search      </code></td>
<td><code dir="ltr" translate="no">       BOOL      </code></td>
<td>The <code dir="ltr" translate="no">       disable_search = true      </code> option prevents Spanner from using a vector index in your database. If you use the <code dir="ltr" translate="no">       FORCE_INDEX      </code> hint to specify a vector index which has the <code dir="ltr" translate="no">       disable_search      </code> option set to <code dir="ltr" translate="no">       true      </code> , the query fails.</td>
</tr>
</tbody>
</table>

#### Examples

The following example creates a vector index `  Singer_vector_index  ` on the `  embedding  ` column of the `  Singers  ` table and defines the distance type:

``` text
CREATE TABLE Singers(id INT64, genre STRING, embedding ARRAY<FLOAT32>(vector_length=>128))
PRIMARY KEY(id);

CREATE VECTOR INDEX Singer_vector_index ON Singers(embedding)
STORING (genre)
WHERE embedding IS NOT NULL
OPTIONS(distance_type = 'COSINE');
```

The following example creates a vector index `  Singer_vector_index  ` on the `  embedding  ` column of the `  Singers  ` table and defines the distance type and search tree specifications, which are optional:

``` text
CREATE TABLE Singers(id INT64, embedding ARRAY<FLOAT32>(vector_length=>128))
PRIMARY KEY(id);

CREATE VECTOR INDEX Singer_vector_index ON Singers(embedding)
STORING (genre)
WHERE embedding IS NOT NULL
OPTIONS(distance_type = 'COSINE', tree_depth = 3, num_branches = 1000, num_leaves = 1000000);
```

### `     ALTER VECTOR INDEX    ` statement

Use the `  ALTER VECTOR INDEX  ` statement to add additional stored columns or remove stored columns from the vector index.

#### Syntax

``` text
ALTER VECTOR INDEX index_name
    action

where action is:
    { ADD STORED COLUMN column_name |
      DROP STORED COLUMN column_name |
      SET OPTIONS ( options_def ) }

and options_def is:
    { disable_search = { true | false | null } }
```

#### Parameters

`  index_name  `

  - The name of the vector index to alter.

`  column_name  `

  - The name of the stored column to add or remove from the vector index.

`  options_def  `

  - The `  disable_search = true  ` option prevents Spanner from using a vector index in your database. If you use the `  FORCE_INDEX  ` hint to specify a vector index which has the `  disable_search  ` option set to `  true  ` , the query fails.

#### Description

Add an additional stored column into a vector index or remove a stored column from the index.

#### Examples

The following `  ALTER VECTOR INDEX  ` statement modifies the vector index by removing the stored column `  genre  ` :

``` text
ALTER VECTOR INDEX Singer_vector_index
DROP STORED COLUMN genre;
```

### `     DROP VECTOR INDEX    ` statement

Deletes a vector index on a table.

#### Syntax

``` text
DROP [ VECTOR ] INDEX index_name;
```

#### Parameters

  - `  index_name  ` : The name of the vector index to be deleted.

#### Example

The following example deletes the vector index `  Singer_vector_index  ` :

``` text
DROP VECTOR INDEX Singer_vector_index;
```
