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

TODO: Document other CLI_* variables