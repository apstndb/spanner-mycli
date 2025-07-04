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

Shell command execution can be controlled using two options:

1. `--skip-system-command` flag - Disables system commands
2. `--system-command=ON/OFF` option - Enables (ON) or disables (OFF) system commands (default: ON)

```bash
# Disable using --skip-system-command
spanner-mycli --skip-system-command

# Disable using --system-command
spanner-mycli --system-command=OFF

# Explicitly enable (default behavior)
spanner-mycli --system-command=ON
```

Note: When both options are used, `--skip-system-command` takes precedence.

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

## Prompt Change (`\R`)

The `\R` meta command allows you to change the prompt string during your session:

```
spanner> \R mycli> 
mycli> 
```

### Features

- Changes the prompt immediately for the current session
- Updates the `CLI_PROMPT` system variable
- Supports the same percent expansion patterns as the `--prompt` flag (see README.md for details)
- The change persists for the duration of the session

### Examples

```
spanner> \R [%p/%i/%d]> 
[myproject/myinstance/mydatabase]> 

spanner> \R custom> 
custom> SHOW VARIABLE CLI_PROMPT;
+---------------+----------+
| Variable_name | Value    |
+---------------+----------+
| CLI_PROMPT    | custom>  |
+---------------+----------+
```

### Notes

- A trailing space is automatically added to the prompt for better separation between prompt and user input
- The input prompt string has trailing spaces trimmed, but a space is always added when setting the prompt
- An empty `\R` command (without a prompt string) will show an error
- The prompt change only affects the current session
- This is similar to `SET CLI_PROMPT = 'prompt> '` but more convenient for interactive use

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

## Output Tee Control (`\T` and `\t`)

The `\T` and `\t` meta commands provide dynamic control over output logging during an interactive session, complementing the `--tee` command-line option.

### Usage

- `\T <filename>` - Start appending output to the specified file
- `\t` - Stop output logging

```
spanner> \T session.log
spanner> SELECT * FROM users;  -- This query and result will be logged
spanner> \t
spanner> SELECT * FROM sensitive_data;  -- This won't be logged
```

For detailed information about tee functionality (what gets logged, file handling, error handling), see [Output logging (tee functionality)](../README.md#output-logging-tee-functionality) in the README.
