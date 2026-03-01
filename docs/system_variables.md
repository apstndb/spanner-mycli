## System variables reference

### Spanner JDBC inspired variables

TODO

### spanner-mycli original variables

#### Output Formatting Variables

##### CLI_SKIP_COLUMN_NAMES
- **Type**: BOOL
- **Default**: FALSE
- **Description**: Suppresses column headers in query result output
- **Access**: Read/Write
- **Usage**: 
  ```sql
  SET CLI_SKIP_COLUMN_NAMES = TRUE;
  SELECT * FROM users;  -- Output without column headers
  ```
- **Notes**:
  - Affects table format (`CLI_FORMAT='TABLE'`), tab format (`CLI_FORMAT='TAB'`), CSV format (`CLI_FORMAT='CSV'`), HTML format (`CLI_FORMAT='HTML'`), and XML format (`CLI_FORMAT='XML'`)
  - Headers are always preserved in vertical format (`CLI_FORMAT='VERTICAL'`) as they are integral to the format
  - Can be set via `--skip-column-names` command-line flag
  - Useful for scripting and data processing where only raw data is needed

##### CLI_SKIP_SYSTEM_COMMAND
- **Type**: BOOL
- **Default**: FALSE
- **Description**: Indicates whether system commands are disabled
- **Access**: Read-only
- **Usage**: 
  ```sql
  SHOW CLI_SKIP_SYSTEM_COMMAND;  -- Check if system commands are disabled
  ```
- **Notes**:
  - This is a read-only variable that reflects the state set by command-line flags
  - Can be set via `--skip-system-command` flag or `--system-command=OFF`
  - When set to TRUE, the `\!` meta command is disabled
  - When both flags are used, `--skip-system-command` takes precedence
  - Security feature to prevent shell command execution in restricted environments

##### CLI_FORMAT
- **Type**: STRING
- **Default**: TABLE (interactive mode) or TAB (batch mode)
- **Description**: Controls output format for query results
- **Access**: Read/Write
- **Valid Values**:
  - `TABLE` - ASCII table with borders (default for interactive mode)
  - `TABLE_COMMENT` - Table wrapped in `/* */` comments
  - `TABLE_DETAIL_COMMENT` - Table and execution details wrapped in `/* */` comments (useful for embedding results in SQL code blocks)
  - `VERTICAL` - Vertical format with column:value pairs
  - `TAB` - Tab-separated values (default for batch mode)
  - `HTML` - HTML table format
  - `XML` - XML format
  - `CSV` - Comma-separated values (RFC 4180 compliant)
  - `SQL_INSERT` - INSERT statements
  - `SQL_INSERT_OR_IGNORE` - INSERT OR IGNORE statements
  - `SQL_INSERT_OR_UPDATE` - INSERT OR UPDATE statements
- **Usage**: 
  ```sql
  SET CLI_FORMAT = 'VERTICAL';
  SELECT * FROM users;  -- Output in vertical format
  
  SET CLI_FORMAT = 'HTML';
  SELECT * FROM users;  -- Output as HTML table
  
  SET CLI_FORMAT = 'XML';
  SELECT * FROM users;  -- Output as XML
  
  SET CLI_FORMAT = 'CSV';
  SELECT * FROM users;  -- Output as CSV (comma-separated values)
  
  SET CLI_FORMAT = 'TABLE_DETAIL_COMMENT';
  SELECT * FROM users;  -- Output as table with execution stats, all wrapped in /* */ comments
  ```
- **Notes**:
  - Can be set via `--html` flag (sets to HTML format)
  - Can be set via `--xml` flag (sets to XML format)
  - Can be set via `--csv` flag (sets to CSV format)
  - Can be set via `--table` flag (sets to TABLE format in batch mode)
  - HTML and XML formats are compatible with Google Cloud Spanner CLI
  - All special characters are properly escaped in HTML, XML, and CSV formats for security
  - CSV format follows RFC 4180 standard with automatic escaping of commas, quotes, and newlines
  - The format affects how query results are displayed, not how they are executed
  - `TABLE_DETAIL_COMMENT` is particularly useful with `CLI_ECHO_INPUT=TRUE` and `CLI_MARKDOWN_CODEBLOCK=TRUE` for documentation

##### CLI_SQL_TABLE_NAME
- **Type**: STRING
- **Default**: (empty)
- **Description**: Table name for generated SQL statements
- **Access**: Read/Write
- **Usage**: 
  ```sql
  SET CLI_SQL_TABLE_NAME = 'DestTable';
  SET CLI_FORMAT = 'SQL_INSERT';
  SELECT * FROM SourceTable;  -- Generates INSERT INTO DestTable statements
  
  -- Schema-qualified names are supported
  SET CLI_SQL_TABLE_NAME = 'myschema.Users';
  ```
- **Notes**:
  - Required when using SQL export formats (SQL_INSERT, SQL_INSERT_OR_IGNORE, SQL_INSERT_OR_UPDATE)
  - Supports both simple names (e.g., 'Users') and schema-qualified names (e.g., 'myschema.Users')
  - Identifiers are automatically quoted when necessary using memefish's ast.Path

##### CLI_SQL_BATCH_SIZE
- **Type**: INT64
- **Default**: 0
- **Description**: Number of VALUES per INSERT statement for SQL export
- **Access**: Read/Write
- **Valid Values**:
  - `0` or `1` - Single-row INSERT statements (one per row)
  - `2` or higher - Multi-row INSERT with up to N rows per statement
- **Usage**: 
  ```sql
  -- Single-row INSERTs (default)
  SET CLI_SQL_BATCH_SIZE = 0;
  SET CLI_SQL_TABLE_NAME = 'users';
  SET CLI_FORMAT = 'SQL_INSERT';
  SELECT * FROM users LIMIT 3;
  -- Output:
  -- INSERT INTO users (id, name) VALUES (1, 'Alice');
  -- INSERT INTO users (id, name) VALUES (2, 'Bob');
  -- INSERT INTO users (id, name) VALUES (3, 'Charlie');
  
  -- Multi-row INSERTs (batch size of 100)
  SET CLI_SQL_BATCH_SIZE = 100;
  SELECT * FROM users LIMIT 200;
  -- Output:
  -- INSERT INTO users (id, name) VALUES
  --   (1, 'Alice'),
  --   (2, 'Bob'),
  --   ... (up to 100 rows);
  -- INSERT INTO users (id, name) VALUES
  --   (101, 'Dave'),
  --   ... (remaining rows);
  ```
- **Notes**:
  - Affects SQL export formats only
  - Batching can improve performance when importing large datasets
  - The last batch may contain fewer rows than the batch size

#### Interactive / Fuzzy Finder Variables

##### CLI_FUZZY_FINDER_OPTIONS
- **Type**: STRING
- **Default**: (empty)
- **Description**: Additional fzf options passed to the fuzzy finder
- **Access**: Read/Write
- **Usage**:
  ```sql
  SET CLI_FUZZY_FINDER_OPTIONS = '--color=dark';
  SET CLI_FUZZY_FINDER_OPTIONS = '--no-select-1 --no-cycle';  -- Override defaults
  SET CLI_FUZZY_FINDER_OPTIONS = '';  -- Reset to defaults only
  ```
- **Notes**:
  - Options are appended after built-in defaults, so user options take precedence (last wins)
  - Uses standard fzf option syntax (space-separated flags)
  - Built-in defaults: `--reverse`, `--no-sort`, `--height`, `--border=rounded`, `--info=inline-right`, `--select-1`, `--exit-0`, `--highlight-line`, `--cycle`
  - Useful for customizing appearance (colors, layout) or behavior (sorting, preview)
  - `--tmux` is **not supported** because the fuzzy finder runs fzf in-process via the Go library

TODO: Document other CLI_* variables