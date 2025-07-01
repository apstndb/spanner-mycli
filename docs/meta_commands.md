# Meta Commands

Meta commands are special commands that start with a backslash (`\`) and are processed by the CLI itself rather than being sent to the database. They provide additional functionality for interactive sessions.

**Note**: Meta commands are only supported in interactive mode. They cannot be used in batch mode (with `--execute` or `--file` flags).

## Shell Command Execution (`\!`)

The `\!` meta command allows you to execute shell commands without leaving the CLI:

```
spanner> \! echo "Hello from shell"
Hello from shell
spanner> \! pwd
/Users/username/projects
```

**Note**: Only non-interactive shell commands are supported. Interactive commands that require user input (such as `vi`, `less`, or interactive shells) will not work properly as stdin is not connected to the executed command.

### Security

Shell command execution can be disabled using the `--skip-system-command` flag:

```bash
spanner-mycli --skip-system-command
```

When disabled, attempting to use `\!` will result in an error:

```
spanner> \! ls
ERROR: system commands are disabled
```

## SQL File Execution (`\.`)

The `\.` meta command allows you to execute SQL statements from a file:

```
spanner> \. init_database.sql
Query OK, 0 rows affected (30.60 sec)
Query OK, 5 rows affected (5.08 sec)
```

### Features

- Executes all SQL statements in the file sequentially
- Displays results after each statement (same as interactive mode)
- Stops execution on the first error
- Supports both relative and absolute file paths
- Handles filenames with spaces when quoted: `\. "file with spaces.sql"`

### Limitations

Meta commands (including `\.`) cannot be used within sourced files. Only SQL statements are allowed in the files. This is because the file contents are processed using the same parser as batch mode, which explicitly rejects meta commands.

### Example

Create a file `setup.sql`:
```sql
CREATE TABLE users (
  id INT64 NOT NULL,
  name STRING(100),
  email STRING(100)
) PRIMARY KEY (id);

INSERT INTO users (id, name, email) VALUES
  (1, 'Alice', 'alice@example.com'),
  (2, 'Bob', 'bob@example.com');
```

Execute it:
```
spanner> \. setup.sql
Query OK, 0 rows affected (1.23 sec)
Query OK, 2 rows affected (0.45 sec)
```

## Database Switching (`\u`)

The `\u` meta command allows you to switch to a different database without restarting the CLI:

```
spanner> \u mydb
Database changed
spanner> \u my-database
Database changed
```

### Features

- Quick database switching within the same instance
- Supports database names with special characters (hyphens, underscores)
- Backtick quoting for database names: `` \u `my-database` ``
- Maintains current project and instance context
- Similar to the `USE` statement but with shorter syntax

### Differences from USE Statement

While `\u` provides similar functionality to the `USE` statement, there are some differences:

- **Syntax**: `\u` is more concise (`\u mydb` vs `USE mydb`)
- **ROLE support**: Unlike `USE database ROLE role`, the `\u` command does not support specifying a role
- **Compatibility**: `\u` provides compatibility with Google Cloud Spanner CLI

### Example

```
spanner> SHOW DATABASES;
+-----------+
| Database  |
+-----------+
| mydb      |
| test-db   |
| prod-db   |
+-----------+

spanner> \u test-db
Database changed

spanner> SELECT DATABASE() as current_db;
+------------+
| current_db |
+------------+
| test-db    |
+------------+
```