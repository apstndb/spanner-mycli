spanner-mycli [![run-tests](https://github.com/apstndb/spanner-my-cli/actions/workflows/run-tests.yaml/badge.svg)](https://github.com/cloudspannerecosystem/spanner-my-cli/actions/workflows/run-tests.yaml)
===

My personal fork of spanner-cli, interactive command line tool for Cloud Spanner.

## Description

`spanner-mycli` is an interactive command line tool for [Google Cloud Spanner](https://cloud.google.com/spanner/).  
You can control your Spanner databases with idiomatic SQL commands.

## Differences from original spanner-cli

* Respects my minor use cases
  * `SHOW LOCAL PROTO` and `SHOW REMOTE PROTO` statement
  * Can use embedded emulator (`--embedded-emulator`)
  * Support [query parameters](#query-parameter-support)
* Respects batch use cases as well as interactive use cases
* More `gcloud spanner databases execute-sql` compatibilities
  * Support compatible flags (`--sql`)
* More `gcloud spanner databases ddl update` compatibilities
  * Support [`--proto-descriptor-file`](#protocol-buffers-support) flag
* Generalized concepts to extend without a lot of original syntax
  * Generalized system variables concept inspired by Spanner JDBC properties
    * `SET <name> = <value>` statement
    * `SHOW VARIABLES` statement
    * `SHOW VARIABLE <name>` statement
    * `--set <name>=<value>` flag
* Improved interactive experience
  * Use [`hymkor/go-multiline-ny`](https://github.com/hymkor/go-multiline-ny) instead of [`chzyer/readline"`](https://github.com/chzyer/readline)
    * Native multi-line editing
  * Improved prompt
    * Use `%` for prompt expansion, instead of `\` to avoid escaping
    * Allow newlines in prompt using `%n`
    * System variables expansion
    * Prompt2 with margin and waiting status
  * Autowrap and auto adjust column width to fit within terminal width.
  * Progress bar of DDL execution.
* Utilize other libraries
  * Dogfooding [`cloudspannerecosystem/memefish`](https://github.com/cloudspannerecosystem/memefish)
    * Spin out memefish logic as [`apstndb/gsqlutils`](https://github.com/apstndb/gsqlutils).
  * Utilize [`apstndb/spantype`](https://github.com/apstndb/spantype) and [`apstndb/spanvalue`](https://github.com/apstndb/spanvalue)


## Install

[Install Go](https://go.dev/doc/install) and run the following command.

```
# For Go 1.16+
go install github.com/apstndb/spanner-mycli@latest
```

Or you can build a docker image.

```
git clone https://github.com/apstndb/spanner-mycli.git
cd spanner-mycli
docker build -t spanner-mycli .
```

## Usage

```
Usage:
  spanner-mycli [OPTIONS]

spanner:
  -p, --project=               (required) GCP Project ID. [$SPANNER_PROJECT_ID]
  -i, --instance=              (required) Cloud Spanner Instance ID [$SPANNER_INSTANCE_ID]
  -d, --database=              (required) Cloud Spanner Database ID. [$SPANNER_DATABASE_ID]
  -e, --execute=               Execute SQL statement and quit. --sql is an alias.
  -f, --file=                  Execute SQL statement from file and quit.
  -t, --table                  Display output in table format for batch mode.
  -v, --verbose                Display verbose output.
      --credential=            Use the specific credential file
      --prompt=                Set the prompt to the specified format (default: spanner%t> )
      --prompt2=               Set the prompt2 to the specified format (default: %P%R> )
      --log-memefish           Emit SQL parse log using memefish
      --history=               Set the history file to the specified path (default: /tmp/spanner_mycli_readline.tmp)
      --priority=              Set default request priority (HIGH|MEDIUM|LOW)
      --role=                  Use the specific database role
      --endpoint=              Set the Spanner API endpoint (host:port)
      --directed-read=         Directed read option (replica_location:replica_type). The replicat_type is optional and either READ_ONLY or READ_WRITE
      --set=                   Set system variables e.g. --set=name1=value1 --set=name2=value2
      --proto-descriptor-file= Path of a file that contains a protobuf-serialized google.protobuf.FileDescriptorSet message.
      --insecure               Skip TLS verification and permit plaintext gRPC. --skip-tls-verify is an alias.
      --embedded-emulator      Use embedded Cloud Spanner Emulator. --project, --instance, --database, --endpoint, --insecure will be automatically configured.
      --emulator-image=        container image for --embedded-emulator (default: gcr.io/cloud-spanner-emulator/emulator:1.5.25)
```

### Authentication

Unless you specify a credential file with `--credential`, this tool uses [Application Default Credentials](https://cloud.google.com/docs/authentication/production?hl=en#providing_credentials_to_your_application) as credential source to connect to Spanner databases.  

Please make sure to prepare your credential by `gcloud auth application-default login`.

If you're running spanner-mycli in docker container on your local machine, you have to pass local credentials to the container with the following command.

```
docker run -it \
  -e GOOGLE_APPLICATION_CREDENTIALS=/tmp/credentials.json \
  -v $HOME/.config/gcloud/application_default_credentials.json:/tmp/credentials.json:ro \
  spanner-mycli --help
```

## Example

### Interactive mode

```
$ spanner-mycli -p myproject -i myinstance -d mydb
Connected.
spanner> CREATE TABLE users (
      ->   id INT64 NOT NULL,
      ->   name STRING(16) NOT NULL,
      ->   active BOOL NOT NULL
      -> ) PRIMARY KEY (id);
Query OK, 0 rows affected (30.60 sec)

spanner> SHOW TABLES;
+----------------+
| Tables_in_mydb |
+----------------+
| users          |
+----------------+
1 rows in set (18.66 msecs)

spanner> INSERT INTO users (id, name, active) VALUES (1, "foo", true), (2, "bar", false);
Query OK, 2 rows affected (5.08 sec)

spanner> SELECT * FROM users ORDER BY id ASC;
+----+------+--------+
| id | name | active |
+----+------+--------+
| 1  | foo  | true   |
| 2  | bar  | false  |
+----+------+--------+
2 rows in set (3.09 msecs)

spanner> BEGIN;
Query OK, 0 rows affected (0.02 sec)

spanner(rw txn)> DELETE FROM users WHERE active = false;
Query OK, 1 rows affected (0.61 sec)

spanner(rw txn)> COMMIT;
Query OK, 0 rows affected (0.20 sec)

spanner> SELECT * FROM users ORDER BY id ASC;
+----+------+--------+
| id | name | active |
+----+------+--------+
| 1  | foo  | true   |
+----+------+--------+
1 rows in set (2.58 msecs)

spanner> DROP TABLE users;
Query OK, 0 rows affected (25.20 sec)

spanner> SHOW TABLES;
Empty set (2.02 msecs)

spanner> EXIT;
Bye
```

### Batch mode

By passing SQL from standard input, `spanner-mycli` runs in batch mode.

```
$ echo 'SELECT * FROM users;' | spanner-mycli -p myproject -i myinstance -d mydb
id      name    active
1       foo     true
2       bar     false
```

You can also pass SQL with command line option `-e`.

```
$ spanner-mycli -p myproject -i myinstance -d mydb -e 'SELECT * FROM users;'
id      name    active
1       foo     true
2       bar     false
```

With `-t` option, results are displayed in table format.

```
$ spanner-mycli -p myproject -i myinstance -d mydb -e 'SELECT * FROM users;' -t
+----+------+--------+
| id | name | active |
+----+------+--------+
| 1  | foo  | true   |
| 2  | bar  | false  |
+----+------+--------+
```

### Directed reads mode

spanner-mycli now supports directed reads, a feature that allows you to read data from a specific replica of a Spanner database. 
To use directed reads with spanner-mycli, you need to specify the `--directed-read` flag.
The `--directed-read` flag takes a single argument, which is the name of the replica that you want to read from.
The replica name can be specified in one of the following formats:

- `<replica_location>`
- `<replica_location>:<replica_type>`

The `<replica_location>` specifies the region where the replica is located such as `us-central1`, `asia-northeast2`.  
The `<replica_type>` specifies the type of the replica either `READ_WRITE` or `READ_ONLY`. 
 
```
$ spanner-mycli -p myproject -i myinstance -d mydb --directed-read us-central1

$ spanner-mycli -p myproject -i myinstance -d mydb --directed-read us-central1:READ_ONLY

$ spanner-mycli -p myproject -i myinstance -d mydb --directed-read asia-northeast2:READ_WRITE
```

Directed reads are only effective for single queries or queries within a read-only transaction.
Please note that directed read options do not apply to queries within a read-write transaction.

> [!NOTE]
> If you specify an incorrect region or type for directed reads, directed reads will not be enabled and [your requsts won't be routed as expected](https://cloud.google.com/spanner/docs/directed-reads#parameters). For example, in a multi-region configuration `nam3`, if you mistype `us-east1` as `us-east-1`, the connection will succeed, but directed reads will not be enabled. 
>
> To perform directed reads to `asia-northeast2` in a multi-region configuration `asia1`, you need to specify `asia-northeast2` or `asia-northeast2:READ_WRITE`.
> Since the replicas placed in `asia-northeast2` are READ_WRITE replicas, directed reads will not be enabled if you specify `asia-northeast2:READ_ONLY`.
> 
> Please refer to [the Spanner documentation](https://cloud.google.com/spanner/docs/instance-configurations#available-configurations-multi-region) to verify the valid configurations.

## Syntax

In the following syntax, we use `<>` for a placeholder, `[]` for an optional keyword,
and `{}` for a mutually exclusive keyword.

* The syntax is case-insensitive.
* `\G` delimiter is also supported for displaying results vertically.

| Usage | Syntax                                                                                         | Note |
| --- |------------------------------------------------------------------------------------------------| --- |
| List databases | `SHOW DATABASES;`                                                                              | |
| Switch database | `USE <database> [ROLE <role>];`                                                                | The role you set is used for accessing with [fine-grained access control](https://cloud.google.com/spanner/docs/fgac-about). |
| Create database | `CREATE DATABSE <database>;`                                                                   | |
| Drop database | `DROP DATABASE <database>;`                                                                    | |
| List tables | `SHOW TABLES [<schema>];`                                                                      | If schema is not provided, default schema is used |
| Show table schema | `SHOW CREATE TABLE <table>;`                                                                   | The table can be a FQN.|
| Show columns | `SHOW COLUMNS FROM <table>;`                                                                   | The table can be a FQN.|
| Show indexes | `SHOW INDEX FROM <table>;`                                                                     | The table can be a FQN.|
| Create table | `CREATE TABLE ...;`                                                                            | |
| Change table schema | `ALTER TABLE ...;`                                                                             | |
| Delete table | `DROP TABLE ...;`                                                                              | |
| Truncate table | `TRUNCATE TABLE <table>;`                                                                      | Only rows are deleted. Note: Non-atomically because executed as a [partitioned DML statement](https://cloud.google.com/spanner/docs/dml-partitioned?hl=en). |
| Create index | `CREATE INDEX ...;`                                                                            | |
| Delete index | `DROP INDEX ...;`                                                                              | |
| Create role | `CREATE ROLE ...;`                                                                             | |
| Drop role | `DROP ROLE ...;`                                                                               | |
| Grant | `GRANT ...;`                                                                                   | |
| Revoke | `REVOKE ...;`                                                                                  | |
| Query | `SELECT ...;`                                                                                  | |
| DML | `{INSERT\|UPDATE\|DELETE} ...;`                                                                | |
| Partitioned DML | `PARTITIONED {UPDATE\|DELETE} ...;`                                                            | |
| Show local proto descriptors | `SHOW LOCAL PROTO;`                                                                            | |
| Show remote proto bundle | `SHOW REMOTE PROTO;`                                                                           | |
| Show Query Execution Plan | `EXPLAIN SELECT ...;`                                                                          | |
| Show DML Execution Plan | `EXPLAIN {INSERT\|UPDATE\|DELETE} ...;`                                                        | |
| Show Query Execution Plan with Stats | `EXPLAIN ANALYZE SELECT ...;`                                                                  | |
| Show DML Execution Plan with Stats | `EXPLAIN ANALYZE {INSERT\|UPDATE\|DELETE} ...;`                                                | |
| Show Query Result Shape | `DESCRIBE SELECT ...;`                                                                         | |
| Show DML Result Shape | `DESCRIBE {INSERT\|UPDATE\|DELETE} ... THEN RETURN ...;`                                       | |
| Start a new query optimizer statistics package construction | `ANALYZE;`                                                                                     | |
| Start Read-Write Transaction | `BEGIN [RW] [PRIORITY {HIGH\|MEDIUM\|LOW}] [TAG <tag>];`                                       | See [Request Priority](#request-priority) for details on the priority. The tag you set is used as both transaction tag and request tag. See also [Transaction Tags and Request Tags](#transaction-tags-and-request-tags).|
| Commit Read-Write Transaction | `COMMIT;`                                                                                      | |
| Rollback Read-Write Transaction | `ROLLBACK;`                                                                                    | |
| Start Read-Only Transaction | `BEGIN RO [{<seconds>\|<RFC3339-formatted time>}] [PRIORITY {HIGH\|MEDIUM\|LOW}] [TAG <tag>];` | `<seconds>` and `<RFC3339-formatted time>` is used for stale read. See [Request Priority](#request-priority) for details on the priority. The tag you set is used as request tag. See also [Transaction Tags and Request Tags](#transaction-tags-and-request-tags).|
| End Read-Only Transaction | `CLOSE;`                                                                                       | |
| Exit CLI | `EXIT;`                                                                                        | |
| Show variable | `SHOW VARIABLE <name>;`                                                                        | |
| Set variable | `SET <name> = <value>;`                                                                        | |
| Show variables | `SHOW VARIABLES;`                                                                              | |
| Set type query parameter | `SET PARAM <name> <type>;`                                                                        | |
| Set value query parameter | `SET PARAM <name> = <value>;`                                                                        | |
| Show variables | `SHOW PARAMS;`                                                                              | |

## System Variables

### Spanner JDBC inspired variables

They have almost same semantics with [Spanner JDBC properties](https://cloud.google.com/spanner/docs/jdbc-session-mgmt-commands?hl=en)

| Name                         | Type       | Example                              |
|------------------------------|------------|--------------------------------------|
| READ_ONLY_STALENESS          | READ_WRITE | `"analyze_20241017_15_59_17UTC"`     |
| OPTIMIZER_VERSION            | READ_WRITE | `"7"`                                |
| OPTIMIZER_STATISTICS_PACKAGE | READ_WRITE | `"7"`                                |
| RPC_PRIORITY                 | READ_WRITE | `"MEDIUM"`                           |
| READ_TIMESTAMP               | READ_ONLY  | `"2024-11-01T05:28:58.943332+09:00"` |
| COMMIT_RESPONSE              | READ_ONLY  | `"2024-11-01T05:31:11.311894+09:00"` |

### spanner-mycli original variables

| Name                      | READ/WRITE | Example                                        |
|---------------------------|------------|------------------------------------------------|
| CLI_PROJECT               | READ_ONLY  | `"myproject"`                                  |
| CLI_INSTANCE              | READ_ONLY  | `"myinstance"`                                 |
| CLI_DATABASE              | READ_ONLY  | `"mydb"`                                       |
| CLI_DIRECT_READ           | READ_ONLY  | `"asia-northeast:READ_ONLY"`                   |
| CLI_ENDPOINT              | READ_ONLY  | `"spanner.me-central2.rep.googleapis.com:443"` |
| CLI_FORMAT                | READ_WRITE | `"TABLE"`                                      |
| CLI_HISTORY_FILE          | READ_ONLY  | `"/tmp/spanner_mycli_readline.tmp"`            |
| CLI_PROMPT                | READ_WRITE | `"spanner%t> "`                                |
| CLI_PROMPT2               | READ_WRITE | `"%P%R> "`                                     |
| CLI_ROLE                  | READ_ONLY  | `"spanner_info_reader"`                        |
| CLI_VERBOSE               | READ_WRITE | `TRUE`                                         |
| CLI_PROTO_DESCRIPTOR_FILE | READ_WRITE | `"order_descriptors.pb"`                       |
| CLI_PARSE_MODE            | READ_WRITE | `"FALLBACK"`                                   |
| CLI_INSECURE              | READ_WRITE | `"FALSE"`                                      |

## Customize prompt

You can customize the prompt by `--prompt` option or `CLI_PROMPT` system variable.  
There are some escape sequences for being used in prompt.

Escape sequences:

* `%p` : GCP Project ID
* `%i` : Cloud Spanner Instance ID
* `%d` : Cloud Spanner Database ID
* `%t` : In transaction mode `(ro txn)` or `(rw txn)`
* `%n` : Newline
* `%%` : Character `%`
* `%{VAR_NAME}` : System variable expansion

Example:

```
$ spanner-mycli -p myproject -i myinstance -d mydb --prompt='[%p:%i:%d]%n\t%% '
Connected.
[myproject:myinstance:mydb]
%
[myproject:myinstance:mydb]
% SELECT * FROM users ORDER BY id ASC;
+----+------+--------+
| id | name | active |
+----+------+--------+
| 1  | foo  | true   |
| 2  | bar  | false  |
+----+------+--------+
2 rows in set (3.09 msecs)

[myproject:myinstance:mydb]
% begin;
Query OK, 0 rows affected (0.08 sec)

[myproject:myinstance:mydb]
(rw txn)% ...
```

The default prompt is `spanner%t> `.

### Prompt2

* `%P` is substituted with padding to align the primary prompt.
* `%R` is substituted with the current waiting status.

| Prompt | Meaning                                                                  |
|--------|--------------------------------------------------------------------------|
| `  -`  | Waiting for terminating multi-line query by `;`                          |
| `'''`  | Waiting for terminating multi-line single-quoted string literal by `'''` |
| `"""`  | Waiting for terminating multi-line double-quoted string literal by `"""` |
| ` /*`  | Waiting for terminating multi-line comment by `*/`                       |

```
spanner> SELECT """
    """> test
    """> """ AS s,
      -> '''
    '''> test
    '''> ''' AS s2,
      -> /*
     /*> 
     /*> */
      -> ;
```

The default prompt2 is `%P%R> `.

## Config file

This tool supports a configuration file called `spanner_mycli.cnf`, similar to `my.cnf`.  
The config file path must be `~/.spanner_mycli.cnf`.
In the config file, you can set default option values for command line options.

Example:

```conf
[spanner]
project = myproject
instance = myinstance
prompt = "[%p:%i:%d]%t> "
```

## Configuration Precedence

1. Command line flags(highest)
2. Environment variables
3. `.spanner_mycli.cnf` in current directory
4. `.spanner_mycli.cnf` in home directory(lowest)

## Request Priority

You can set [request priority](https://cloud.google.com/spanner/docs/reference/rest/v1/RequestOptions#Priority) for command level or transaction level.
By default `MEDIUM` priority is used for every request.

To set a priority for command line level, you can use `--priority={HIGH|MEDIUM|LOW}` command line option or `CLI_PRIORITY` system variable.

To set a priority for transaction level, you can use `PRIORITY {HIGH|MEDIUM|LOW}` keyword.

Here are some examples for transaction-level priority.

```
# Read-write transaction with low priority
BEGIN PRIORITY LOW;

# Read-only transaction with low priority
BEGIN RO PRIORITY LOW;

# Read-only transaction with 60s stale read and medium priority
BEGIN RO 60 PRIORITY MEDIUM;

# Read-only transaction with exact timestamp and medium priority
BEGIN RO 2021-04-01T23:47:44+00:00 PRIORITY MEDIUM;
```

Note that transaction-level priority takes precedence over command-level priority.

## Transaction Tags and Request Tags

In a read-write transaction, you can add a tag following `BEGIN RW TAG <tag>`.
spanner-mycli adds the tag set in `BEGIN RW TAG` as a transaction tag.
The tag will also be used as request tags within the transaction.

```
# Read-write transaction
# transaction_tag = tx1
+--------------------+
| BEGIN RW TAG tx1;  |
|                    |
| SELECT val         |
| FROM tab1      +-----request_tag = tx1
| WHERE id = 1;      |
|                    |
| UPDATE tab1        |
| SET val = 10   +-----request_tag = tx1
| WHERE id = 1;      |
|                    |
| COMMIT;            |
+--------------------+
```

In a read-only transaction, you can add a tag following `BEGIN RO TAG <tag>`.
Since read-only transaction doesn't support transaction tag, spanner-mycli adds the tag set in `BEGIN RO TAG` as request tags.
```
# Read-only transaction
# transaction_tag = N/A
+--------------------+
| BEGIN RO TAG tx2;  |
|                    |
| SELECT SUM(val)    |
| FROM tab1      +-----request_tag = tx2
| WHERE id = 1;      |
|                    |
| CLOSE;             |
+--------------------+
```

## Protocol Buffers support

You can use `--proto-descriptor-file` option to specify proto descriptor file.

```
$ spanner-mycli --proto-descriptor-file=testdata/protos/order_descriptors.pb 
Connected.
spanner> SHOW LOCAL PROTO;
+---------------------------------+-------------------+--------------------+
| full_name                       | package           | file               |
+---------------------------------+-------------------+--------------------+
| examples.shipping.Order         | examples.shipping | order_protos.proto |
| examples.shipping.Order.Address | examples.shipping | order_protos.proto |
| examples.shipping.Order.Item    | examples.shipping | order_protos.proto |
| examples.shipping.OrderHistory  | examples.shipping | order_protos.proto |
+---------------------------------+-------------------+--------------------+
4 rows in set (0.00 sec)

spanner> SET CLI_PROTO_DESCRIPTOR_FILE += "testdata/protos/query_plan_descriptors.pb";
Empty set (0.00 sec)

spanner> SHOW LOCAL PROTO;
+----------------------------------------------------------------+-------------------+-----------------------------------------------+
| full_name                                                      | package           | file                                          |
+----------------------------------------------------------------+-------------------+-----------------------------------------------+
| examples.shipping.Order                                        | examples.shipping | order_protos.proto                            |
| examples.shipping.Order.Address                                | examples.shipping | order_protos.proto                            |
| examples.shipping.Order.Item                                   | examples.shipping | order_protos.proto                            |
| examples.shipping.OrderHistory                                 | examples.shipping | order_protos.proto                            |
| google.protobuf.Struct                                         | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.Struct.FieldsEntry                             | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.Value                                          | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.ListValue                                      | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.NullValue                                      | google.protobuf   | google/protobuf/struct.proto                  |
| google.spanner.v1.PlanNode                                     | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.Kind                                | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.ChildLink                           | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.ShortRepresentation                 | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.ShortRepresentation.SubqueriesEntry | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.QueryPlan                                    | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
+----------------------------------------------------------------+-------------------+-----------------------------------------------+
15 rows in set (0.00 sec)

spanner> CREATE PROTO BUNDLE (`examples.shipping.Order`);
Query OK, 0 rows affected (6.34 sec)

spanner> SHOW REMOTE PROTO;
+-------------------------+-------------------+
| full_name               | package           |
+-------------------------+-------------------+
| examples.shipping.Order | examples.shipping |
+-------------------------+-------------------+
1 rows in set (0.94 sec)

spanner> ALTER PROTO BUNDLE INSERT (`examples.shipping.Order.Item`);
Query OK, 0 rows affected (9.25 sec)

spanner> SHOW REMOTE PROTO;
+------------------------------+-------------------+
| full_name                    | package           |
+------------------------------+-------------------+
| examples.shipping.Order      | examples.shipping |
| examples.shipping.Order.Item | examples.shipping |
+------------------------------+-------------------+
2 rows in set (0.82 sec)

spanner> ALTER PROTO BUNDLE UPDATE (`examples.shipping.Order`);
Query OK, 0 rows affected (8.68 sec)

spanner> ALTER PROTO BUNDLE DELETE (`examples.shipping.Order.Item`);
Query OK, 0 rows affected (9.55 sec)

spanner> SHOW REMOTE PROTO;
+-------------------------+-------------------+
| full_name               | package           |
+-------------------------+-------------------+
| examples.shipping.Order | examples.shipping |
+-------------------------+-------------------+
1 rows in set (0.79 sec)
```

You can also use `CLI_PROTO_DESCRIPTOR_FILE` system variable to update or read the current proto descriptor file setting.

```
spanner> SET CLI_PROTO_DESCRIPTOR_FILE = "./other_descriptors.pb";
Empty set (0.00 sec)

spanner> SHOW VARIABLE CLI_PROTO_DESCRIPTOR_FILE;
+---------------------------+
| CLI_PROTO_DESCRIPTOR_FILE |
+---------------------------+
| ./other_descriptors.pb    |
+---------------------------+
Empty set (0.00 sec)
```

## memefish integration

spanner-mycli utilizes [memefish](https://github.com/cloudspannerecosystem/memefish) as:

* statement type detector
* comment stripper
* statement separator

Statement type detector behavior can be controlled by `CLI_PARSE_MODE` system variable.

| CLI_PARSE_MODE | Description                        |
|----------------|------------------------------------|
| FALLBACK       | Use memefish but fallback if error |
| NO_MEMEFISH    | Don't use memefish                 |
| MEMEFISH_ONLY  | Use memefish and don't fallback    |

```
spanner> SET CLI_PARSE_MODE = "MEMEFISH_ONLY";
Empty set (0.00 sec)

spanner> SELECT * FRM 1;
ERROR: invalid statement: syntax error: :1:10: expected token: <eof>, but: <ident>

  1:  SELECT * FRM 1
               ^~~

spanner> SET CLI_PARSE_MODE = "FALLBACK";
Empty set (0.00 sec)

spanner> SELECT * FRM 1;
2024/11/02 00:22:57 ignore memefish parse error, err: syntax error: :1:10: expected token: <eof>, but: <ident>

  1:  SELECT * FRM 1
               ^~~

ERROR: spanner: code = "InvalidArgument", desc = "Syntax error: Expected end of input but got identifier \\\"FRM\\\" [at 1:10]\\nSELECT * FRM 1\\n         ^"
spanner> SET CLI_PARSE_MODE = "NO_MEMEFISH";
Empty set (0.00 sec)

spanner> SELECT * FRM 1;
ERROR: spanner: code = "InvalidArgument", desc = "Syntax error: Expected end of input but got identifier \\\"FRM\\\" [at 1:10]\\nSELECT * FRM 1\\n         ^"
```

## Query parameter support

Many Cloud Spanner clients don't support query parameters.

If you do not modify the query, you will not be able to execute queries that contain query parameters,
and you will not be able to view the query plan for queries with parameter types `STRUCT` or `ARRAY`.

spanner-mycli solves this problem by supporting query parameters.

You can define query parameters using command line option `--param` or `SET` commands.
It supports type notation or literal value notation in GoogleSQL.

Note: They are supported on the best effort basis, and type conversions are not supported.

### Query parameters definition using `--param` option
```
$ spanner-mycli \
                --param='array_type=ARRAY<STRUCT<FirstName STRING, LastName STRING>>' \
                --param='array_value=[STRUCT("John" AS FirstName, "Doe" AS LastName), ("Mary", "Sue")]'

```

You can see defined query parameters using `SHOW PARAMS;` command.

```
> SHOW PARAMS;
+-------------+------------+-------------------------------------------------------------------+
| Param_Name  | Param_Kind | Param_Value                                                       |
+-------------+------------+-------------------------------------------------------------------+
| array_value | VALUE      | [STRUCT("John" AS FirstName, "Doe" AS LastName), ("Mary", "Sue")] |
| array_type  | TYPE       | ARRAY<STRUCT<FirstName STRING, LastName STRING>>                  |
+-------------+------------+-------------------------------------------------------------------+
Empty set (0.00 sec)
```

You can use value query parameters in any statement.
```
> SELECT * FROM Singers WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) IN UNNEST(@array_value);
```

You can use type query parameters only in `EXPLAIN` or `DESCRIBE` without value.

```
> EXPLAIN SELECT * FROM Singers WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) IN UNNEST(@array_type);
+-----+----------------------------------------------------------------------------------+
| ID  | Query_Execution_Plan                                                             |
+-----+----------------------------------------------------------------------------------+
|   0 | Distributed Cross Apply                                                          |
|   1 | +- [Input] Create Batch                                                          |
|   2 | |  +- Compute Struct                                                             |
|   3 | |     +- Hash Aggregate                                                          |
|   4 | |        +- Compute                                                              |
|   5 | |           +- Array Unnest                                                      |
|  10 | |              +- [Scalar] Array Subquery                                        |
|  11 | |                 +- Array Unnest                                                |
|  35 | +- [Map] Serialize Result                                                        |
|  36 |    +- Cross Apply                                                                |
|  37 |       +- [Input] Batch Scan (Batch: $v14, scan_method: Scalar)                   |
|  40 |       +- [Map] Local Distributed Union                                           |
| *41 |          +- Filter Scan (seekable_key_size: 0)                                   |
|  42 |             +- Table Scan (Full scan: true, Table: Singers, scan_method: Scalar) |
+-----+----------------------------------------------------------------------------------+
Predicates(identified by ID):
 41: Residual Condition: (($FirstName = $batched_v8) AND ($LastName = $batched_v9))


> DESCRIBE SELECT * FROM Singers WHERE STRUCT<FirstName STRING, LastName STRING>(FirstName, LastName) IN UNNEST(@array_type);
+-------------+-------------+
| Column_Name | Column_Type |
+-------------+-------------+
| SingerId    | INT64       |
| FirstName   | STRING      |
| LastName    | STRING      |
| SingerInfo  | BYTES       |
| BirthDate   | DATE        |
+-------------+-------------+
5 rows in set (0.18 sec)
```
### Interactive definition of query parameters using `SET` commands

You can define type query parameters using `SET PARAM param_name type;` command.

```
> SET PARAM string_type STRING;
Empty set (0.00 sec)

> SHOW PARAMS;
+-------------+------------+-------------+
| Param_Name  | Param_Kind | Param_Value |
+-------------+------------+-------------+
| string_type | TYPE       | STRING      |
+-------------+------------+-------------+
Empty set (0.00 sec)
```

You can define type query parameters using `SET PARAM param_name = value;` command.

```
> SET PARAM bytes_value = b"foo";
Empty set (0.00 sec)

> SHOW PARAMS;
+-------------+------------+-------------+
| Param_Name  | Param_Kind | Param_Value |
+-------------+------------+-------------+
| bytes_value | VALUE      | B"foo"      |
+-------------+------------+-------------+
Empty set (0.00 sec)
```



## Using with the Cloud Spanner Emulator

This tool supports the [Cloud Spanner Emulator](https://cloud.google.com/spanner/docs/emulator) via the [`SPANNER_EMULATOR_HOST` environment variable](https://cloud.google.com/spanner/docs/emulator#client-libraries).

```sh
$ export SPANNER_EMULATOR_HOST=localhost:9010
# Or with gcloud env-init:
$ $(gcloud emulators spanner env-init)

$ spanner-mycli -p myproject -i myinstance -d mydb

# Or use --endpoint with --insecure
$ unset SPANNER_EMULATOR_HOST
$ spanner-mycli -p myproject -i myinstance -d mydb --endpoint=localhost:9010 --insecure
```

### Embedded Cloud Spanner Emulator

spanner-mycli can launch Cloud Spanner Emulator with empty database, powered by testcontainers.

```
$ spanner-mycli --embedded-emulator [--emulator-image= gcr.io/cloud-spanner-emulator/emulator:${VERSION}]
> SET CLI_PROMPT="%p:%i:%d%n> ";
Empty set (0.00 sec)

emulator-project:emulator-instance:emulator-database
> SELECT * FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA NOT IN ("INFORMATION_SCHEMA", "SPANNER_SYS");
+---------------+--------------+------------+-------------------+------------------+------------+---------------+-----------------+--------------------------------+
| TABLE_CATALOG | TABLE_SCHEMA | TABLE_NAME | PARENT_TABLE_NAME | ON_DELETE_ACTION | TABLE_TYPE | SPANNER_STATE | INTERLEAVE_TYPE | ROW_DELETION_POLICY_EXPRESSION |
| STRING        | STRING       | STRING     | STRING            | STRING           | STRING     | STRING        | STRING          | STRING                         |
+---------------+--------------+------------+-------------------+------------------+------------+---------------+-----------------+--------------------------------+
+---------------+--------------+------------+-------------------+------------------+------------+---------------+-----------------+--------------------------------+
Empty set (8.763167ms)
```

## How to develop

Run unit tests.

```
$ make test
```

Note: It requires Docker because integration tests using [testcontainers](https://testcontainers.com/).

Or run test except integration tests.

```
$ make fasttest
```

## TODO

* Show secondary index by "SHOW CREATE TABLE"

## Disclaimer

Do not use this tool for production databases as the tool is still alpha quality.

Generally, this project don't accept feature requests, please contact the original [spanner-cli](https://github.com/cloudspannerecosystem/spanner-cli).
