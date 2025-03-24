spanner-mycli
===

My personal fork of spanner-cli, interactive command line tool for Cloud Spanner.

## Description

`spanner-mycli` is an interactive command line tool for [Google Cloud Spanner](https://cloud.google.com/spanner/).  
You can control your Spanner databases with idiomatic SQL commands.

## Differences from original spanner-cli

* Respects my minor use cases
  * Protocol Buffere support as `SHOW LOCAL PROTO`, `SHOW REMOTE PROTO`, `SYNC PROTO BUNDLE` statement
  * Can use embedded emulator (`--embedded-emulator`)
  * Support [query parameters](#query-parameter-support)
  * Test root-partitionable with [`TRY PARTITIONED QUERY <sql>` command](#test-root-partitionable)
  * Experimental Partitioned Query and Data Boost support.
  * GenAI support(`GEMINI` statement).
  * Interactive DDL batching
* Respects training and verification use-cases.
  * gRPC logging(`--log-grpc`)
  * Support mutations
* Respects batch use cases as well as interactive use cases
* More `gcloud spanner databases execute-sql` compatibilities
  * Support compatible flags (`--sql`, `--query-mode`, `--strong`, `--read-timestamp`)
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
  * Autowrap and auto adjust column width to fit within terminal width when `CLI_AUTOWRAP = TRUE`).
  * Pager support when `CLI_USE_PAGER = TRUE`
  * Progress bar of DDL execution.
  * Syntax highlight when `CLI_ENABLE_HIGHLIGHT = TRUE`
* Utilize other libraries
  * Dogfooding [`cloudspannerecosystem/memefish`](https://github.com/cloudspannerecosystem/memefish)
    * Spin out memefish logic as [`apstndb/gsqlutils`](https://github.com/apstndb/gsqlutils).
  * Utilize [`apstndb/spantype`](https://github.com/apstndb/spantype) and [`apstndb/spanvalue`](https://github.com/apstndb/spanvalue)

## Disclaimer

Do not use this tool for production databases as the tool is experimental/alpha quality forever.

## Version policy

This software will not have a stable release. In other words, v1.0.0 will never be released. It will be operated as a kind of [ZeroVer](https://0ver.org/).

v0.X.Y will be operated as follows:

- The initial release version is v0.1.0, forked from spanner-cli v0.10.6.
- The patch version Y will be incremented for changes that include only bug fixes.
- The minor version X will always be incremented when there are new features or changes related to compatibility.
- As a general rule, unreleased updates to the main branch will be released within one week.

## Install

[Install Go](https://go.dev/doc/install) and run the following command.

```
# For Go 1.23+
go install github.com/apstndb/spanner-mycli@latest
```

Or you can use a container image.

https://github.com/apstndb/spanner-mycli/pkgs/container/spanner-mycli

## Usage

```
Usage:
  spanner-mycli [OPTIONS]

spanner:
  -p, --project=                                          (required) GCP Project ID. [$SPANNER_PROJECT_ID]
  -i, --instance=                                         (required) Cloud Spanner Instance ID [$SPANNER_INSTANCE_ID]
  -d, --database=                                         (required) Cloud Spanner Database ID. [$SPANNER_DATABASE_ID]
  -e, --execute=                                          Execute SQL statement and quit. --sql is an alias.
  -f, --file=                                             Execute SQL statement from file and quit.
  -t, --table                                             Display output in table format for batch mode.
  -v, --verbose                                           Display verbose output.
      --credential=                                       Use the specific credential file
      --prompt=                                           Set the prompt to the specified format (default: spanner%t> )
      --prompt2=                                          Set the prompt2 to the specified format (default: %P%R> )
      --log-memefish                                      Emit SQL parse log using memefish
      --history=                                          Set the history file to the specified path (default: /tmp/spanner_mycli_readline.tmp)
      --priority=                                         Set default request priority (HIGH|MEDIUM|LOW)
      --role=                                             Use the specific database role
      --endpoint=                                         Set the Spanner API endpoint (host:port)
      --directed-read=                                    Directed read option (replica_location:replica_type). The replicat_type is optional and either READ_ONLY or READ_WRITE
      --set=                                              Set system variables e.g. --set=name1=value1 --set=name2=value2
      --param=                                            Set query parameters, it can be literal or type(EXPLAIN/DESCRIBE only) e.g. --param="p1='string_value'" --param=p2=FLOAT64
      --proto-descriptor-file=                            Path of a file that contains a protobuf-serialized google.protobuf.FileDescriptorSet message.
      --insecure                                          Skip TLS verification and permit plaintext gRPC. --skip-tls-verify is an alias.
      --embedded-emulator                                 Use embedded Cloud Spanner Emulator. --project, --instance, --database, --endpoint, --insecure will be automatically configured.
      --emulator-image=                                   container image for --embedded-emulator (default: gcr.io/cloud-spanner-emulator/emulator:1.5.28)
      --log-grpc                                          Show gRPC logs
      --query-mode=[NORMAL|PLAN|PROFILE]                  Mode in which the query must be processed.
      --strong                                            Perform a strong query.
      --read-timestamp=                                   Perform a query at the given timestamp.
      --vertexai-project=                                 VertexAI project
      --database-dialect=[POSTGRESQL|GOOGLE_STANDARD_SQL] The SQL dialect of the Cloud Spanner Database.
      --impersonate-service-account=                      Impersonate service account email
      --version                                           Show version string.
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
$ docker run -it \
    -v $HOME/.config/gcloud/application_default_credentials.json:/home/nonroot/.config/gcloud/application_default_credentials.json:ro \
    ghcr.io/apstndb/spanner-mycli --help
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

## Client-Side Statement Syntax

spanner-mycli supports all Spanner GoogleSQL and Spanner Graph statements, as well as several client-side statements.

Note: If any valid Spanner statement can't be executed, it is a bug.

In the following syntax, we use `<>` for a placeholder, `[]` for an optional keyword,
and `{A|B|...}` for a mutually exclusive keyword.

* The syntax is case-insensitive.

<!-- spanner-mycli --statement-help -->
|                       Usage                        |                                              Syntax                                              |                                                                            Note                                                                             |
|----------------------------------------------------|--------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Switch database                                    | `USE <database> [ROLE <role>];`                                                                  | The role you set is used for accessing with [fine-grained access control](https://cloud.google.com/spanner/docs/fgac-about).                                |
| Drop database                                      | `DROP DATABASE <database>;`                                                                      |                                                                                                                                                             |
| List databases                                     | `SHOW DATABASES;`                                                                                |                                                                                                                                                             |
| Show DDL of the schema object                      | `SHOW CREATE <type> <fqn>;`                                                                      |                                                                                                                                                             |
| List tables                                        | `SHOW TABLES [<schema>];`                                                                        | If schema is not provided, the default schema is used                                                                                                       |
| Show columns                                       | `SHOW COLUMNS FROM <table_fqn>;`                                                                 |                                                                                                                                                             |
| Show indexes                                       | `SHOW INDEX FROM <table_fqn>;`                                                                   |                                                                                                                                                             |
| SHOW DDLs                                          | `SHOW DDLS;`                                                                                     |                                                                                                                                                             |
| Show local proto descriptors                       | `SHOW LOCAL PROTO;`                                                                              |                                                                                                                                                             |
| Show remote proto bundle                           | `SHOW REMOTE PROTO;`                                                                             |                                                                                                                                                             |
| Manipulate PROTO BUNDLE                            | `SYNC PROTO BUNDLE [{UPSERT\|DELETE} (<type> ...)];`                                             |                                                                                                                                                             |
| Truncate table                                     | `TRUNCATE TABLE <table>;`                                                                        | Only rows are deleted. Note: Non-atomically because executed as a [partitioned DML statement](https://cloud.google.com/spanner/docs/dml-partitioned?hl=en). |
| Show execution plan without execution              | `EXPLAIN <sql>;`                                                                                 |                                                                                                                                                             |
| Execute query and show execution plan with profile | `EXPLAIN ANALYZE <sql>;`                                                                         |                                                                                                                                                             |
| Show result shape without execution                | `DESCRIBE <sql>;`                                                                                |                                                                                                                                                             |
| Partitioned DML                                    | `PARTITIONED {UPDATE\|DELETE} ...;`                                                              |                                                                                                                                                             |
| Show partition tokens of partition query           | `PARTITION <sql>;`                                                                               |                                                                                                                                                             |
| Run partitioned query                              | `RUN PARTITIONED QUERY <sql>;`                                                                   |                                                                                                                                                             |
| Test root-partitionable                            | `TRY PARTITIONED QUERY <sql>;`                                                                   |                                                                                                                                                             |
| Start R/W transaction                              | `BEGIN RW [TRANSACTION] [PRIORITY {HIGH\|MEDIUM\|LOW}];`                                         | (spanner-cli style);  See [Request Priority](#request-priority) for details on the priority.                                                                |
| Start R/O transaction                              | `BEGIN RO [TRANSACTION] [{<seconds>\|<RFC3339-formatted time>}] [PRIORITY {HIGH\|MEDIUM\|LOW}];` | `<seconds>` and `<RFC3339-formatted time>` is used for stale read. See [Request Priority](#request-priority) for details on the priority.                   |
| Start transaction                                  | `BEGIN [TRANSACTION] [PRIORITY {HIGH\|MEDIUM\|LOW}];`                                            | (Spanner JDBC driver style); It respects `READONLY` system variable. See [Request Priority](#request-priority) for details on the priority.                 |
| Commit R/W transaction or end R/O Transaction      | `COMMIT [TRANSACTION];`                                                                          |                                                                                                                                                             |
| Rollback R/W transaction or end R/O transaction    | `ROLLBACK [TRANSACTION];`                                                                        | `CLOSE` can be used as a synonym of `ROLLBACK`.                                                                                                             |
| Set transaction mode                               | `SET TRANSACTION {READ ONLY\|READ WRITE};`                                                       | (Spanner JDBC driver style); Set transaction mode for the current transaction.                                                                              |
| Start DDL batching                                 | `START BATCH DDL;`                                                                               |                                                                                                                                                             |
| Start DML batching                                 | `START BATCH DML;`                                                                               |                                                                                                                                                             |
| Run active batch                                   | `RUN BATCH;`                                                                                     |                                                                                                                                                             |
| Abort active batch                                 | `ABORT BATCH [TRANSACTION];`                                                                     |                                                                                                                                                             |
| Set variable                                       | `SET <name> = <value>;`                                                                          |                                                                                                                                                             |
| Add value to variable                              | `SET <name> += <value>;`                                                                         |                                                                                                                                                             |
| Show variables                                     | `SHOW VARIABLES;`                                                                                |                                                                                                                                                             |
| Show variable                                      | `SHOW VARIABLE <name>;`                                                                          |                                                                                                                                                             |
| Set type query parameter                           | `SET PARAM <name> <type>;`                                                                       |                                                                                                                                                             |
| Set value query parameter                          | `SET PARAM <name> = <value>;`                                                                    |                                                                                                                                                             |
| Show query parameters                              | `SHOW PARAMS;`                                                                                   |                                                                                                                                                             |
| Perform write mutations                            | `MUTATE <table_fqn> {INSERT\|UPDATE\|REPLACE\|INSERT_OR_UPDATE} ...;`                            |                                                                                                                                                             |
| Perform delete mutations                           | `MUTATE <table_fqn> DELETE ...;`                                                                 |                                                                                                                                                             |
| Show sampled query plans                           | `SHOW QUERY PROFILES;`                                                                           | EARLY EXPERIMENTAL                                                                                                                                          |
| Show the single sampled query plan                 | `SHOW QUERY PROFILE <fingerprint>;`                                                              | EARLY EXPERIMENTAL                                                                                                                                          |
| Compose query using LLM                            | `GEMINI "<prompt>";`                                                                             |                                                                                                                                                             |
| Show help                                          | `HELP;`                                                                                          |                                                                                                                                                             |
| Show help for variables                            | `HELP VARIABLES;`                                                                                |                                                                                                                                                             |
| Exit CLI                                           | `EXIT;`                                                                                          |                                                                                                                                                             |

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

If you set only `%P` to `prompt2`, continuation prompt is only indentations.
It is convenient because of copy-and-paste friendly.

```
spanner> SET CLI_PROMPT2 = "%P";
Empty set (0.00 sec)

spanner> SELECT """
         test
         """ AS s;
```
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

You can set transaction tag using `SET TRANSACTION_TAG = "<tag>"`, and request tag using `SET STATEMENT_TAG = "<tag>"`.
Note: `TRANSACTION_TAG` is effective only in the current transaction and `STATEMENT_TAG` is effective only in the next query.

```
+------------------------------+
| BEGIN;                       |
| SET TRANSACTION_TAG = "tx1"; |
|                              |
| SET STATEMENT_TAG = "req1";  |
| SELECT val                   |
| FROM tab1      +--------------transaction_tag = tx1, request_tag = req1
| WHERE id = 1;                |
|                              |
| SELECT *                     |
| FROM tab1;     +--------------transaction_tag = tx1
|                              |
| SET STATEMENT_TAG = "req2";  |
|                              |
| UPDATE tab1                  |
| SET val = 10   +--------------transaction_tag = tx1, request_tag = req2
| WHERE id = 1;                |
|                              |
| COMMIT;        +--------------transaction_tag = tx1
|                              |
| BEGIN;                       |
| SELECT 1;                    |
| COMMIT;        +--------------no transaction_tag
+------------------------------+
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

## Notable features of spanner-mycli

This section describes some notable features of spanner-mycli, they are not appeared in original spanner-cli.

### System Variables

#### Spanner JDBC inspired variables

They have almost same semantics with [Spanner JDBC properties](https://cloud.google.com/spanner/docs/jdbc-session-mgmt-commands?hl=en)

| Name                            | Type       | Example                                             |
|---------------------------------|------------|-----------------------------------------------------|
| READONLY                        | READ_WRITE | `TRUE`                                              |
| READ_ONLY_STALENESS             | READ_WRITE | `"analyze_20241017_15_59_17UTC"`                    |
| OPTIMIZER_VERSION               | READ_WRITE | `"7"`                                               |
| OPTIMIZER_STATISTICS_PACKAGE    | READ_WRITE | `"7"`                                               |
| RPC_PRIORITY                    | READ_WRITE | `"MEDIUM"`                                          |
| READ_TIMESTAMP                  | READ_ONLY  | `"2024-11-01T05:28:58.943332+09:00"`                |
| COMMIT_RESPONSE                 | READ_ONLY  | `"2024-11-01T05:31:11.311894+09:00"`                |
| TRANSACTION_TAG                 | READ_WRITE | `"app=concert,env=dev,action=update"`               |
| STATEMENT_TAG                   | READ_WRITE | `"app=concert,env=dev,action=update,request=fetch"` |
| DATA_BOOST_ENABLED              | READ_WRITE | `TRUE`                                              |
| AUTO_BATCH_DML                  | READ_WRITE | `TRUE`                                              |
| EXCLUDE_TXN_FROM_CHANGE_STREAMS | READ_WRITE | `TRUE`                                              |
| MAX_COMMIT_DELAY                | READ_WRITE | `"500ms"`                                           |
| AUTOCOMMIT_DML_MODE             | READ_WRITE | `"PARTITIONED_NON_ATOMIC"`                          |
| MAX_PARTITIONED_PARALLELISM     | READ_WRITE | `4`                                                 |
| DEFAULT_ISOLATION_LEVEL         | READ_WRITE | `REPEATABLE_READ`                                    |

#### spanner-mycli original variables

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
| CLI_QUERY_MODE            | READ_WRITE | `"PROFILE"`                                    |
| CLI_LINT_PLAN             | READ_WRITE | `"TRUE"`                                       |
| CLI_USE_PAGER             | READ_WRITE | `"TRUE"`                                       |
| CLI_AUTOWRAP              | READ_WRITE | `"TRUE"`                                       |
| CLI_DATABASE_DIALECT      | READ_WRITE | `"TRUE"`                                       |
| CLI_ENABLE_HIGHLIGHT      | READ_WRITE | `"TRUE"`                                       |
| CLI_PROTOTEXT_MULTILINE   | READ_WRITE | `"TRUE"`                                       |

### Batch statements

#### DDL batching

You can issue [a batch DDL statements](https://cloud.google.com/spanner/docs/schema-updates#order_of_execution_of_statements_in_batches).

- `START BATCH DDL` starts DDL batching. Consecutive DDL statements are batched.
- `RUN BATCH` runs the batch.
- or `ABORT BATCH` aborts the batch.

Note: `SET CLI_ECHO_EXECUTED_DDL = TRUE` enables echo back of actual executed DDLs. It is useful to observe multiple schema versions in batch DDL.

```
spanner> START BATCH DDL;
Empty set (0.00 sec)

spanner> DROP INDEX IF EXISTS BatchExampleByCol;
Query OK, 0 rows affected (0.00 sec) (1 DDL in batch)

spanner> DROP TABLE IF EXISTS BatchExample;
Query OK, 0 rows affected (0.00 sec) (2 DDLs in batch)

spanner> CREATE TABLE BatchExample (PK INT64 PRIMARY KEY, Col INT64);
Query OK, 0 rows affected (0.00 sec) (3 DDLs in batch)

spanner> CREATE INDEX ExistingTableByCol ON ExistingTable(Col);
Query OK, 0 rows affected (0.00 sec) (4 DDLs in batch)

spanner> CREATE INDEX BatchExampleByCol ON BatchExample(Col);
Query OK, 0 rows affected (0.00 sec) (5 DDLs in batch)

spanner> RUN BATCH;
+--------------------------------------------------------------+-----------------------------+
| Executed                                                     | Commit Timestamp            |
+--------------------------------------------------------------+-----------------------------+
| DROP INDEX IF EXISTS BatchExampleByCol;                      | 2024-12-29T09:09:00.404097Z |
| DROP TABLE IF EXISTS BatchExample;                           | 2024-12-29T09:09:00.404097Z |
| CREATE TABLE BatchExample (PK INT64 PRIMARY KEY, Col INT64); | 2024-12-29T09:09:00.404097Z |
| CREATE INDEX ExistingTableByCol ON ExistingTable(Col);       | 2024-12-29T09:09:18.278025Z |
| CREATE INDEX BatchExampleByCol ON BatchExample(Col);         | 2024-12-29T09:09:41.061034Z |
+--------------------------------------------------------------+-----------------------------+
Query OK, 0 rows affected (51.00 sec)
timestamp:      2024-12-29T09:09:41.061034Z


# or abort using ABORT BATCH.
```

#### DML 

You can use [batch DML](https://cloud.google.com/spanner/docs/dml-best-practices#batch-dml).

- `START BATCH DML` starts DML batching. Consecutive DML statements are batched.
- `RUN BATCH` runs the batch.
- or `ABORT BATCH` aborts the batch.

```
spanner> START BATCH DML;
Empty set (0.00 sec)

spanner> DELETE FROM BatchExample WHERE TRUE;
Query OK, 0 rows affected (0.00 sec) (1 DML in batch)

spanner> INSERT INTO BatchExample (PK, Col) VALUES(1, 2);
Query OK, 0 rows affected (0.00 sec) (2 DMLs in batch)

spanner> UPDATE BatchExample SET Col = Col + 1 WHERE TRUE;
Query OK, 0 rows affected (0.00 sec) (3 DMLs in batch)

spanner> RUN BATCH;
+--------------------------------------------------+------+
| DML                                              | Rows |
+--------------------------------------------------+------+
| DELETE FROM BatchExample WHERE TRUE              | 1    |
| INSERT INTO BatchExample (PK, Col) VALUES(1, 2)  | 1    |
| UPDATE BatchExample SET Col = Col + 1 WHERE TRUE | 1    |
+--------------------------------------------------+------+
Query OK, at most 3 rows affected (0.59 sec)
timestamp:      2024-12-29T17:25:48.616395+09:00
mutation_count: 9

# or abort using ABORT BATCH.
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

### Protocol Buffers support

You can use `--proto-descriptor-file` option to specify proto descriptor file.

```
$ spanner-mycli --proto-descriptor-file=testdata/protos/order_descriptors.pb 
Connected.
spanner> SHOW LOCAL PROTO;
+---------------------------------+-------+-------------------+--------------------+
| full_name                       | kind  | package           | file               |
+---------------------------------+-------+-------------------+--------------------+
| examples.shipping.Order         | PROTO | examples.shipping | order_protos.proto |
| examples.shipping.Order.Address | PROTO | examples.shipping | order_protos.proto |
| examples.shipping.Order.Item    | PROTO | examples.shipping | order_protos.proto |
| examples.shipping.OrderHistory  | PROTO | examples.shipping | order_protos.proto |
+---------------------------------+-------+-------------------+--------------------+
4 rows in set (0.00 sec)

spanner> SET CLI_PROTO_DESCRIPTOR_FILE += "testdata/protos/query_plan_descriptors.pb";
Empty set (0.00 sec)

spanner> SHOW LOCAL PROTO;
+----------------------------------------------------------------+-------+-------------------+-----------------------------------------------+
| full_name                                                      | kind  | package           | file                                          |
+----------------------------------------------------------------+-------+-------------------+-----------------------------------------------+
| examples.shipping.Order                                        | PROTO | examples.shipping | order_protos.proto                            |
| examples.shipping.Order.Address                                | PROTO | examples.shipping | order_protos.proto                            |
| examples.shipping.Order.Item                                   | PROTO | examples.shipping | order_protos.proto                            |
| examples.shipping.OrderHistory                                 | PROTO | examples.shipping | order_protos.proto                            |
| google.protobuf.Struct                                         | PROTO | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.Struct.FieldsEntry                             | PROTO | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.Value                                          | PROTO | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.ListValue                                      | PROTO | google.protobuf   | google/protobuf/struct.proto                  |
| google.protobuf.NullValue                                      | ENUM  | google.protobuf   | google/protobuf/struct.proto                  |
| google.spanner.v1.PlanNode                                     | PROTO | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.ChildLink                           | PROTO | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.ShortRepresentation                 | PROTO | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.ShortRepresentation.SubqueriesEntry | PROTO | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.PlanNode.Kind                                | ENUM  | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
| google.spanner.v1.QueryPlan                                    | PROTO | google.spanner.v1 | googleapis/google/spanner/v1/query_plan.proto |
+----------------------------------------------------------------+-------+-------------------+-----------------------------------------------+
15 rows in set (0.00 sec)

spanner> CREATE PROTO BUNDLE (`examples.shipping.Order`);
Query OK, 0 rows affected (6.34 sec)

spanner> SHOW REMOTE PROTO;
+-------------------------+-------+-------------------+
| full_name               | kind  | package           |
+-------------------------+-------+-------------------+
| examples.shipping.Order | PROTO | examples.shipping |
+-------------------------+-------+-------------------+
1 rows in set (0.94 sec)

spanner> ALTER PROTO BUNDLE INSERT (`examples.shipping.Order.Item`);
Query OK, 0 rows affected (9.25 sec)

spanner> SHOW REMOTE PROTO;
+------------------------------+-------+-------------------+
| full_name                    | kind  | package           |
+------------------------------+-------+-------------------+
| examples.shipping.Order      | PROTO | examples.shipping |
| examples.shipping.Order.Item | PROTO | examples.shipping |
+------------------------------+-------+-------------------+
2 rows in set (0.82 sec)

spanner> ALTER PROTO BUNDLE UPDATE (`examples.shipping.Order`);
Query OK, 0 rows affected (8.68 sec)

spanner> ALTER PROTO BUNDLE DELETE (`examples.shipping.Order.Item`);
Query OK, 0 rows affected (9.55 sec)

spanner> SHOW REMOTE PROTO;
+-------------------------+-------+-------------------+
| full_name               | kind  | package           |
+-------------------------+-------+-------------------+
| examples.shipping.Order | PROTO | examples.shipping |
+-------------------------+-------+-------------------+
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

(EXPERIMENTAL) It also supports non-compiled `.proto` file.

```
spanner> SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/order_protos.proto";
Empty set (0.00 sec)

spanner> SHOW LOCAL PROTO;
+---------------------------------+-------+-------------------+------------------------------------+
| full_name                       | kind  | package           | file                               |
+---------------------------------+-------+-------------------+------------------------------------+
| examples.shipping.Order         | PROTO | examples.shipping | testdata/protos/order_protos.proto |
| examples.shipping.Order.Address | PROTO | examples.shipping | testdata/protos/order_protos.proto |
| examples.shipping.Order.Item    | PROTO | examples.shipping | testdata/protos/order_protos.proto |
| examples.shipping.OrderHistory  | PROTO | examples.shipping | testdata/protos/order_protos.proto |
+---------------------------------+-------+-------------------+------------------------------------+
4 rows in set (0.00 sec)
```

This feature is powered by [bufbuild/protocompile](https://github.com/bufbuild/protocompile).

(EXPERIMENTAL) `.pb` and `.proto` files can be loaded from URL.

```
spanner> SET CLI_PROTO_DESCRIPTOR_FILE = "https://github.com/apstndb/spanner-mycli/raw/refs/heads/main/testdata/protos/order_descriptors.pb";
Empty set (0.68 sec)

[gcpug-public-spanner:merpay-sponsored-instance:apstndb-sampledb3]
spanner> SHOW LOCAL PROTO;
+---------------------------------+-------+-------------------+--------------------+
| full_name                       | kind  | package           | file               |
+---------------------------------+-------+-------------------+--------------------+
| examples.shipping.Order         | PROTO | examples.shipping | order_protos.proto |
| examples.shipping.Order.Address | PROTO | examples.shipping | order_protos.proto |
| examples.shipping.Order.Item    | PROTO | examples.shipping | order_protos.proto |
| examples.shipping.OrderHistory  | PROTO | examples.shipping | order_protos.proto |
+---------------------------------+-------+-------------------+--------------------+
4 rows in set (0.00 sec)
```

#### `SYNC PROTO BUNDLE` statement


Note: `SET CLI_ECHO_EXECUTED_DDL = TRUE` enables echo back of actual executed DDLs.

```
spanner> SET CLI_ECHO_EXECUTED_DDL = TRUE;
Empty set (0.00 sec)
```

##### `SYNC PROTO BUNDLE UPSERT`

`SYNC PROTO BUNDLE UPSERT (FullName, ...)` simplifies use of `CREATE PROTO BUNDLE` and `ALTER PROTO BUNDLE {INSERT|UPDATE}`.

`SYNC PROTO BUNDLE` executes `ALTER PROTO BUNDLE INSERT` or `ALTER PROTO BUNDLE UPDATE` in `UPSERT` semantics.

```
spanner> SHOW REMOTE PROTO;
+-------------------------+-------+-------------------+
| full_name               | kind  | package           |
+-------------------------+-------+-------------------+
| examples.shipping.Order | PROTO | examples.shipping |
+-------------------------+-------+-------------------+
1 rows in set (0.87 sec)

spanner> SYNC PROTO BUNDLE UPSERT (`examples.shipping.Order`, examples.shipping.OrderHistory);
+------------------------------------------------------------------------------------------------+
| executed                                                                                       |
+------------------------------------------------------------------------------------------------+
| ALTER PROTO BUNDLE INSERT (examples.shipping.OrderHistory) UPDATE (examples.shipping.`Order`); |
+------------------------------------------------------------------------------------------------+
Query OK, 0 rows affected (8.57 sec)

```

If `SYNC PROTO BUNDLE UPSERT` is executed on empty proto bundle,
`CREATE PROTO BUNDLE` will be executed instead of `ALTER PROTO BUNDLE`.

```
spanner> SHOW REMOTE PROTO;
Empty set (0.89 sec)

spanner> SYNC PROTO BUNDLE UPSERT (`examples.shipping.Order`);
+--------------------------------------------------+
| executed                                         |
+--------------------------------------------------+
| CREATE PROTO BUNDLE (examples.shipping.`Order`); |
+--------------------------------------------------+
Query OK, 0 rows affected (8.27 sec)
```

##### `SYNC PROTO BUNDLE DELETE`

`SYNC PROTO BUNDLE DELETE (<full_name>, ...)` executes `PROTO BUNDLE DELETE` only existing types(`DELETE IF EXISTS` semantics).
```
spanner> SHOW REMOTE PROTO;
+--------------------------------+-------+-------------------+
| full_name                      | kind  | package           |
+--------------------------------+-------+-------------------+
| examples.shipping.Order        | PROTO | examples.shipping |
| examples.shipping.OrderHistory | PROTO | examples.shipping |
+--------------------------------+-------+-------------------+
2 rows in set (0.87 sec)

spanner> SYNC PROTO BUNDLE DELETE (`examples.shipping.Order`, examples.UnknownType);
+--------------------------------------------------------+
| executed                                               |
+--------------------------------------------------------+
| ALTER PROTO BUNDLE DELETE (examples.shipping.`Order`); |
+--------------------------------------------------------+
Query OK, 0 rows affected (8.86 sec)

spanner> SYNC PROTO BUNDLE DELETE (examples.UnknownType);
Query OK, 0 rows affected (0.79 sec)
```

If `SYNC PROTO BUNDLE DELETE` will delete the last Protocol Buffers type in the proto bundle,
`DROP PROTO BUNDLE` will be executed instead of `ALTER PROTO BUNDLE`.

```
spanner> SHOW REMOTE PROTO;

+-------------------------+-------+-------------------+
| full_name               | kind  | package           |
+-------------------------+-------+-------------------+
| examples.shipping.Order | PROTO | examples.shipping |
+-------------------------+-------+-------------------+
1 rows in set (0.87 sec)

spanner> SYNC PROTO BUNDLE DELETE (`examples.shipping.OrderHistory`);
+--------------------+
| executed           |
+--------------------+
| DROP PROTO BUNDLE; |
+--------------------+
Query OK, 0 rows affected (8.24 sec)
```

#### `PROTO`/`ENUM` value formatting

Loaded proto descriptors are used for formatting `PROTO` and `ENUM` column values.

```
spanner> SELECT p, p.*
         FROM (
           SELECT AS VALUE NEW examples.spanner.music.SingerInfo{singer_id: 1, birth_date: "1970-01-01", nationality: "Japan", genre: "POP"}
         ) AS p;
+----------------------------------+-----------+------------+-------------+-------------+
| p                                | singer_id | birth_date | nationality | genre       |
| PROTO<SingerInfo>                | INT64     | STRING     | STRING      | ENUM<Genre> |
+----------------------------------+-----------+------------+-------------+-------------+
| CAESCjE5NzAtMDEtMDEaBUphcGFuIAA= | 1         | 1970-01-01 | Japan       | 0           |
+----------------------------------+-----------+------------+-------------+-------------+
1 rows in set (0.33 msecs)

spanner> SET CLI_PROTO_DESCRIPTOR_FILE += "testdata/protos/singer.proto";
Empty set (0.00 sec)

spanner> SHOW LOCAL PROTO;
+-----------------------------------------+-------+------------------------+------------------------------+
| full_name                               | kind  | package                | file                         |
+-----------------------------------------+-------+------------------------+------------------------------+
| examples.spanner.music.SingerInfo       | PROTO | examples.spanner.music | testdata/protos/singer.proto |
| examples.spanner.music.CustomSingerInfo | PROTO | examples.spanner.music | testdata/protos/singer.proto |
| examples.spanner.music.Genre            | ENUM  | examples.spanner.music | testdata/protos/singer.proto |
| examples.spanner.music.CustomGenre      | ENUM  | examples.spanner.music | testdata/protos/singer.proto |
+-----------------------------------------+-------+------------------------+------------------------------+
4 rows in set (0.00 sec)

spanner> SELECT p, p.*
         FROM (
           SELECT AS VALUE NEW examples.spanner.music.SingerInfo{singer_id: 1, birth_date: "1970-01-01", nationality: "Japan", genre: "POP"}
         ) AS p;
+-------------------------------------------------------------------+-----------+------------+-------------+-------------+
| p                                                                 | singer_id | birth_date | nationality | genre       |
| PROTO<SingerInfo>                                                 | INT64     | STRING     | STRING      | ENUM<Genre> |
+-------------------------------------------------------------------+-----------+------------+-------------+-------------+
| singer_id:1 birth_date:"1970-01-01" nationality:"Japan" genre:POP | 1         | 1970-01-01 | Japan       | POP         |
+-------------------------------------------------------------------+-----------+------------+-------------+-------------+
1 rows in set (0.87 msecs)
```

You can enable multiline format.

```
spanner> SET CLI_PROTOTEXT_MULTILINE=TRUE;
Empty set (0.00 sec)

spanner> SELECT p, p.*
         FROM (
           SELECT AS VALUE NEW examples.spanner.music.SingerInfo{singer_id: 1, birth_date: "1970-01-01", nationality: "Japan", genre: "POP"}
         ) AS p;
+--------------------------+-----------+------------+-------------+-------------+
| p                        | singer_id | birth_date | nationality | genre       |
| PROTO<SingerInfo>        | INT64     | STRING     | STRING      | ENUM<Genre> |
+--------------------------+-----------+------------+-------------+-------------+
| singer_id: 1             | 1         | 1970-01-01 | Japan       | POP         |
| birth_date: "1970-01-01" |           |            |             |             |
| nationality: "Japan"     |           |            |             |             |
| genre: POP               |           |            |             |             |
+--------------------------+-----------+------------+-------------+-------------+
1 rows in set (3.37 msecs)
```
### memefish integration

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

### Mutations support

spanner-mycli supports mutations.

Mutations are buffered in read-write transaction, or immediately commit outside explicit transaction.

#### Write mutations

```
MUTATE <table_fqn> {INSERT|UPDATE|REPLACE|INSERT_OR_UPDATE} {<struct_literal> | <array_of_struct_literal>};
```

#### Delete mutations

```
MUTATE <table_fqn> DELETE ALL;
MUTATE <table_fqn> DELETE {<tuple_struct_literal> | <array_of_tuple_struct_literal>};
MUTATE <table_fqn> DELETE KEY_RANGE({start_closed | start_open} => <tuple_struct_literal>,
                                    {end_closed | end_open} => <tuple_struct_literal>);
```

Note: In this context, parenthesized expression and some simple literals are treated as a single field struct literal.

#### Examples of mutations

Example schema

```
CREATE TABLE MutationTest (PK INT64, Col INT64) PRIMARY KEY(PK);
CREATE TABLE MutationTest2 (PK1 INT64, PK2 STRING(MAX), Col INT64) PRIMARY KEY(PK1, PK2);
```

##### Examples of Write mutations

Insert a single row with key(`1`).

```
MUTATE MutationTest INSERT STRUCT(1 AS PK);
```

Insert or update four rows with keys(`1, "foo"`, `1, "n"`, `1, "m"`, `50, "foobar"`).

```
MUTATE MutationTest2 INSERT_OR_UPDATE [STRUCT(1 AS PK1, "foo" AS PK2, 0 AS Col), (1, "n", 1), (1, "m", 3), (50, "foobar", 4)];
```

You can set commit timestamps using `PENDING_COMMIT_TIMESTAMP()`.
```
spanner> MUTATE Performances INSERT_OR_UPDATE STRUCT(
                1 AS SingerId, 1 AS VenueId,
                DATE "2024-12-25" AS EventDate,
                PENDING_COMMIT_TIMESTAMP() AS LastUpdateTime);
Query OK, 0 rows affected (1.18 sec)
timestamp:      2024-12-13T02:40:46.253698+09:00
mutation_count: 4

spanner> SELECT * FROM Performances;
+----------+---------+------------+---------+-----------------------------+
| SingerId | VenueId | EventDate  | Revenue | LastUpdateTime              |
| INT64    | INT64   | DATE       | INT64   | TIMESTAMP                   |
+----------+---------+------------+---------+-----------------------------+
| 1        | 1       | 2024-12-25 | NULL    | 2024-12-12T17:40:46.253698Z |
+----------+---------+------------+---------+-----------------------------+
1 rows in set (14.89 msecs)
timestamp:            2024-12-13T02:40:57.15133+09:00
cpu time:             13.35 msecs
rows scanned:         1 rows
deleted rows scanned: 0 rows
optimizer version:    7
optimizer statistics: auto_20241212_12_01_00UTC
```

##### Examples of Delete mutations

Delete all rows in `MutationTest` table.

```
MUTATE MutationTest DELETE ALL;
```

Delete rows with PK (`1`) in `MutationTest` table.

```
MUTATE MutationTest DELETE (1);
```

Delete a single row with PK (`1, "foo"`) in `MutationTest2` table.

```
MUTATE MutationTest2 DELETE (1, "foo");
```

Delete two rows with PK (`1, "foo"`, `2, "bar"`) in `MutationTest2` table.

```
MUTATE MutationTest2 DELETE [(1, "foo"), (2, "bar")];
```

Delete rows between `(1, "a") <= PK < (1, "n")` in `MutationTest2` table

```
MUTATE MutationTest2 DELETE KEY_RANGE(start_closed => (1, "a"), end_open => (1, "n"));
```

### Query parameter support

Many Cloud Spanner clients don't support query parameters.

If you do not modify the query, you will not be able to execute queries that contain query parameters,
and you will not be able to view the query plan for queries with parameter types `STRUCT` or `ARRAY`.

spanner-mycli solves this problem by supporting query parameters.

You can define query parameters using command line option `--param` or `SET` commands.
It supports type notation or literal value notation in GoogleSQL.

Note: They are supported on the best effort basis, and type conversions are not supported.

#### Query parameters definition using `--param` option
```
$ spanner-mycli \
                --param='array_type=ARRAY<STRUCT<FirstName STRING, LastName STRING>>' \
                --param='array_value=[STRUCT("Marc" AS FirstName, "Richards" AS LastName), ("Catalina", "Smith")]'

```

You can see defined query parameters using `SHOW PARAMS;` command.

```
> SHOW PARAMS;
+-------------+------------+-------------------------------------------------------------------+
| Param_Name  | Param_Kind | Param_Value                                                       |
+-------------+------------+-------------------------------------------------------------------+
| array_value | VALUE      | [STRUCT("Marc" AS FirstName, "Richards" AS LastName), ("Catalina", "Smith")] |
| array_type  | TYPE       | ARRAY<STRUCT<FirstName STRING, LastName STRING>>                  |
+-------------+------------+-------------------------------------------------------------------+
Empty set (0.00 sec)
```

You can use value query parameters in any statement.
```
> SELECT * FROM Singers WHERE STRUCT(FirstName, LastName) IN UNNEST(@array_value);
+----------+-----------+----------+------------+------------+
| SingerId | FirstName | LastName | SingerInfo | BirthDate  |
+----------+-----------+----------+------------+------------+
| 2        | Catalina  | Smith    | NULL       | 1990-08-17 |
| 1        | Marc      | Richards | NULL       | 1970-09-03 |
+----------+-----------+----------+------------+------------+
2 rows in set (7.8 msecs)
```

You can use type query parameters only in `EXPLAIN` or `DESCRIBE` without value.

```
> EXPLAIN SELECT * FROM Singers WHERE STRUCT(FirstName, LastName) IN UNNEST(@array_type);
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

14 rows in set (0.18 sec)

> DESCRIBE SELECT * FROM Singers WHERE STRUCT(FirstName, LastName) IN UNNEST(@array_type);
+-------------+-------------+
| Column_Name | Column_Type |
+-------------+-------------+
| SingerId    | INT64       |
| FirstName   | STRING      |
| LastName    | STRING      |
| SingerInfo  | BYTES       |
| BirthDate   | DATE        |
+-------------+-------------+
5 rows in set (0.17 sec)
```

#### Interactive definition of query parameters using `SET` commands

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

### Partition Queries

spanner-mycli have some partition queries functionality.

#### Test root-partitionable

You can test whether the query is root-partitionable using `TRY PARTITIONED QUERY` command.

```
spanner> TRY PARTITIONED QUERY SELECT * FROM Singers;
+--------------------+
| Root_Partitionable |
+--------------------+
| TRUE               |
+--------------------+
1 rows in set (0.78 sec)

spanner> TRY PARTITIONED QUERY SELECT * FROM Singers ORDER BY SingerId;
ERROR: query can't be a partition query: rpc error: code = InvalidArgument desc = Query is not root partitionable since it does not have a DistributedUnion at the root. Please check the conditions for a query to be root-partitionable.
error details: name = Help desc = Conditions for a query to be root-partitionable. url = https://cloud.google.com/spanner/docs/reads#read_data_in_parallel
```

#### Run partitioned query (EXPERIMENTAL)

You can execute partitioned query using `RUN PARTITIONED QUERY` command.


```
spanner> RUN PARTITIONED QUERY SELECT * FROM Singers;
+----------+-----------+----------+------------+------------+
| SingerId | FirstName | LastName | SingerInfo | BirthDate  |
+----------+-----------+----------+------------+------------+
| 1        | Marc      | Richards | NULL       | 1970-09-03 |
| 2        | Catalina  | Smith    | NULL       | 1990-08-17 |
| 3        | Alice     | Trentor  | NULL       | 1991-10-02 |
| 4        | Lea       | Martin   | NULL       | 1991-11-09 |
| 5        | David     | Lomond   | NULL       | 1977-01-29 |
+----------+-----------+----------+------------+------------+
5 rows in set from 3 partitions (1.40 sec)
```

Or you can use `SET AUTO_PARTITION_MODE`.

```
spanner> SET AUTO_PARTITION_MODE = TRUE;

Empty set (0.00 sec)

spanner> SELECT * FROM Singers;
+----------+-----------+----------+------------+------------+
| SingerId | FirstName | LastName | SingerInfo | BirthDate  |
+----------+-----------+----------+------------+------------+
| 1        | Marc      | Richards | NULL       | 1970-09-03 |
| 2        | Catalina  | Smith    | NULL       | 1990-08-17 |
| 3        | Alice     | Trentor  | NULL       | 1991-10-02 |
| 4        | Lea       | Martin   | NULL       | 1991-11-09 |
| 5        | David     | Lomond   | NULL       | 1977-01-29 |
+----------+-----------+----------+------------+------------+
5 rows in set from 3 partitions (1.40 sec)
```

Note: Any stats are not available in partitioned query.

##### Isolation Level

You can set session-level default isolation level and transaction-level isolation level.

```
spanner> SET DEFAULT_ISOLATION_LEVEL = "REPEATABLE_READ";
spanner> BEGIN ISOLATION LEVEL REPEATABLE READ;
```

##### Enable Data Boost

You can enable Data Boost using `DATA_BOOST_ENABLED`.

```
spanner> SET DATA_BOOST_ENABLED = TRUE;
```

##### Parallelism setting

In default, the number of worker goroutines for partitioned query is the value of `GOMAXPROCS`.

You can change it using `MAX_PARTITIONED_PARALLELISM`.

```
spanner> SET MAX_PARTITIONED_PARALLELISM = 1;
```

Note: There is no streaming output, so result won't be printed unless all work is done.

#### Show partition tokens.

You can show partition tokens using `PARTITION` command.

Note: spanner-mycli does not clean up batch read-only transactions, which may prevent resources from being freed until they time out.

```
spanner> PARTITION SELECT * FROM Singers;
+-------------------------------------------------------------------------------------------------+
| Partition_Token                                                                                 |
+-------------------------------------------------------------------------------------------------+
| QUw0MGxyRjJDZENocXc0TkZnR3NxVHN1QnFCMy1yWkxWZmlIdFhhc2U4T2lWOGJRVFhJRkgydU1URmZUb2dBLXVvZE1O... |
| QUw0MGxyRzhPdmZUR04xclFQVTJKQXlVMEJjZFBUWTV3NUFsT2x0VGxTNHZWSExsejQweXNUWUFtUXd5ZjBzWEhmQ0Fa... |
| QUw0MGxyR3paSk96WUNqdjFsQW9tc2UwOFFoNlA4SzhHUzNWQVltNzVlRHZxdjZpUmFVSFN2UmtBanozc0hEaE9Iem9x... |
+-------------------------------------------------------------------------------------------------+
3 rows in set (0.65 sec)
```

### Query plan linter (EARLY EXPERIMANTAL)

`CLI_LINT_PLAN` system variable enables heuristic query plan linter in `EXPLAIN` and `EXPLAIN ANALYZE`.

```
spanner> SET CLI_LINT_PLAN = TRUE;
Empty set (0.00 sec)

spanner> EXPLAIN SELECT * FROM Singers WHERE FirstName LIKE "%Hoge%";
+----+----------------------------------------------------------------------------------+
| ID | Query_Execution_Plan                                                             |
+----+----------------------------------------------------------------------------------+
|  0 | Distributed Union (distribution_table: Singers, split_ranges_aligned: false)     |
|  1 | +- Local Distributed Union                                                       |
|  2 |    +- Serialize Result                                                           |
| *3 |       +- Filter Scan (seekable_key_size: 0)                                      |
|  4 |          +- Table Scan (Full scan: true, Table: Singers, scan_method: Automatic) |
+----+----------------------------------------------------------------------------------+
Predicates(identified by ID):
 3: Residual Condition: ($FirstName LIKE '%Hoge%')

Experimental Lint Result:
 3: Filter Scan (seekable_key_size: 0)
     Residual Condition: Potentially expensive Residual Condition: Maybe better to modify it to Scan Condition
 4: Table Scan (Full scan: true, Table: Singers, scan_method: Automatic)
     Full scan=true: Potentially expensive execution full scan: Do you really want full scan?
```

### GenAI support

You can use `GEMINI` statement by setting `vertexai_project` in config file.

```:.spanner_mycli.cnf
[spanner]
vertexai_project = example-project
```

The generated query is automatically filled in the prompt.

```
spanner> GEMINI "Generate query to show all table with table_schema concat by dot if table_schema is not empty string.";
+---------------------+------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
| Column              | Value                                                                                                                                                                        |
+---------------------+------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
| text                | SELECT IF(table_schema != '', CONCAT(table_schema, '.', table_name), table_name) AS table_name_with_schema FROM INFORMATION_SCHEMA.TABLES;                                   |
| semanticDescription | This query retrieves all tables and their schemas, concatenating the schema name with the table name if a schema exists, effectively listing all tables with their schema na |
|                     | mes when applicable.                                                                                                                                                         |
| syntaxDescription   | The query selects from the INFORMATION_SCHEMA.TABLES view. It uses IF() to check if table_schema is not empty, and if so, concatenates table_schema and table_name with a do |
|                     | t ('.'). Otherwise, it just returns the table_name. The result is aliased as table_name_with_schema.                                                                         |
+---------------------+------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
Empty set (5.72 sec)

spanner> SELECT IF(table_schema != '', CONCAT(table_schema, '.', table_name), table_name) AS table_name_with_schema FROM INFORMATION_SCHEMA.TABLES;
```

### Show query profiles (EARLY EXPERIMENTAL)

spanner-mycli can render undocumented underlying table of [sampled query plans](https://cloud.google.com/spanner/docs/query-execution-plans#sampled-plans).
These features are early experimental state so it will be changed.

#### Show query profiles(It will be very long outputs).

```
spanner> SHOW QUERY PROFILES;
+-----------------------------------------------------------------------------------------------------------------------------------------------------------+
| Plan                                                                                                                                                      |
+-----------------------------------------------------------------------------------------------------------------------------------------------------------+
| SELECT INTERVAL_END, QUERY_PROFILE FROM SPANNER_SYS.QUERY_PROFILES_TOP_HOUR                                                                               |
| ID | Plan                                                                                                                                                 |
| *0 | Distributed Union (distribution_table: _TopNQueryProfiles, split_ranges_aligned: false)                                                              |
|  1 | +- Local Distributed Union                                                                                                                           |
|  2 |    +- Serialize Result                                                                                                                               |
| *3 |       +- Filter Scan (seekable_key_size: 0)                                                                                                          |
| *4 |          +- Table Scan (Table: _TopNQueryProfiles, scan_method: Scalar)                                                                              |
| Predicates:                                                                                                                                               |
| 0: Split Range: (($interval_seconds = <scrubbed>) AND ($source = <scrubbed>) AND ($call_type = <scrubbed>))                                               |
| 3: Residual Condition: (($source = <scrubbed>) AND ($call_type = <scrubbed>))                                                                             |
| 4: Seek Condition: ($interval_seconds = <scrubbed>)                                                                                                       |
|                                                                                                                                                           |
| interval_end:                 2024-11-29 16:00:00 +0000 UTC                                                                                               |
| text_fingerprint:             1603015871075919821                                                                                                         |
| elapsed_time:                 8.52 msecs                                                                                                                  |
| cpu_time:                     7.48 msecs                                                                                                                  |
| rows_returned:                24                                                                                                                          |
| deleted_rows_scanned:         3                                                                                                                           |
| optimizer_version:            7                                                                                                                           |
| optimizer_statistics_package: auto_20241128_05_46_13UTC                                                                                                   |
|                                                                                                                                                           |
| SELECT @_p0_INT64                                                                                                                                         |
| ID | Plan                                                                                                                                                 |
|  0 | Serialize Result                                                                                                                                     |
|  1 | +- Unit Relation                                                                                                                                     |
|                                                                                                                                                           |
| interval_end:                 2024-11-23 17:00:00 +0000 UTC                                                                                               |
| text_fingerprint:             -773118905674708524                                                                                                         |
| elapsed_time:                 2.25 msecs                                                                                                                  |
| cpu_time:                     1.26 msecs                                                                                                                  |
| rows_returned:                1                                                                                                                           |
| deleted_rows_scanned:         0                                                                                                                           |
| optimizer_version:            7                                                                                                                           |
| optimizer_statistics_package: auto_20241122_05_36_46UTC                                                                                                   |
|
```

Render a latest profile for a `TEXT_FINGERPRINT`. It is compatible with plan linter(`CLI_LINT_PLAN`).

```
spanner> SHOW QUERY PROFILE 1603015871075919821;
+----+-----------------------------------------------------------------------------------------+---------------+------------+---------------+
| ID | Query_Execution_Plan                                                                    | Rows_Returned | Executions | Total_Latency |
+----+-----------------------------------------------------------------------------------------+---------------+------------+---------------+
| *0 | Distributed Union (distribution_table: _TopNQueryProfiles, split_ranges_aligned: false) | 24            | 1          | 3.31 msecs    |
|  1 | +- Local Distributed Union                                                              | 24            | 1          | 3.29 msecs    |
|  2 |    +- Serialize Result                                                                  | 24            | 1          | 3.28 msecs    |
| *3 |       +- Filter Scan (seekable_key_size: 0)                                             |               |            |               |
| *4 |          +- Table Scan (Table: _TopNQueryProfiles, scan_method: Scalar)                 | 24            | 1          | 0.27 msecs    |
+----+-----------------------------------------------------------------------------------------+---------------+------------+---------------+
Predicates(identified by ID):
 0: Split Range: (($interval_seconds = <scrubbed>) AND ($source = <scrubbed>) AND ($call_type = <scrubbed>))
 3: Residual Condition: (($source = <scrubbed>) AND ($call_type = <scrubbed>))
 4: Seek Condition: ($interval_seconds = <scrubbed>)

Experimental Lint Result:
 3: Filter Scan (seekable_key_size: 0)
     Residual Condition: Potentially expensive Residual Condition: Maybe better to modify it to Scan Condition

5 rows in set (8.52 msecs)
cpu time:             7.48 msecs
rows scanned:         24 rows
deleted rows scanned: 3 rows
optimizer version:    7
optimizer statistics: auto_20241128_05_46_13UTC
```

### Markdown output

spanner-mycli can emit input and output in Markdown.

TODO: More description

````
$ spanner-mycli -v --set 'CLI_FORMAT=TABLE_DETAIL_COMMENT' --set 'CLI_MARKDOWN_CODEBLOCK=TRUE' --set 'CLI_ECHO_INPUT=TRUE'

spanner> GRAPH FinGraph
      -> MATCH (n:Account)
      -> RETURN LABELS(n) AS labels, PROPERTY_NAMES(n) AS props, n.id;
```sql
GRAPH FinGraph
MATCH (n:Account)
RETURN LABELS(n) AS labels, PROPERTY_NAMES(n) AS props, n.id;
/*---------------+------------------------------------------+-------+
 | labels        | props                                    | id    |
 | ARRAY<STRING> | ARRAY<STRING>                            | INT64 |
 +---------------+------------------------------------------+-------+
 | [Account]     | [create_time, id, is_blocked, nick_name] | 7     |
 | [Account]     | [create_time, id, is_blocked, nick_name] | 16    |
 | [Account]     | [create_time, id, is_blocked, nick_name] | 20    |
 +---------------+------------------------------------------+-------+
3 rows in set (6.26 msecs)
timestamp:            2025-02-09T23:28:26.088524+09:00
cpu time:             4.45 msecs
rows scanned:         3 rows
deleted rows scanned: 0 rows
optimizer version:    7
optimizer statistics: auto_20250207_09_19_31UTC
*/
```
````

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

## Incompatibilities from spanner-cli

In principle, spanner-mycli accepts the same input as spanner-cli, but some compatibility is intentionally not maintained.

- `BEGIN RW TAG <tag>` and `BEGIN RO TAG <tag>` are no longer supported.
  - Use `SET TRANSACTION TAG = "<tag>"` and `SET STATEMENT_TAG = "<tag>"`.
  - Rationale: spanner-cli are broken. https://github.com/cloudspannerecosystem/spanner-cli/issues/132
- `\G` is no longer supported.
  - Use `SET CLI_FORMAT = "VERTICAL"`.
  - Rationale: `\G` is not compatible with [GoogleSQL lexical structure](https://cloud.google.com/spanner/docs/reference/standard-sql/lexical) and [memefish](https://github.com/cloudspannerecosystem/memefish).
- `\` is no longer used for prompt expansions.
  - Use `%` instead.
  - Rationale: `\` is needed to be escaped in ini files of [jassevdk/go-flags](https://github.com/jessevdk/go-flags).
