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
  - Affects table format (`CLI_FORMAT='TABLE'`) and tab format (`CLI_FORMAT='TAB'`)
  - Headers are always preserved in vertical format (`CLI_FORMAT='VERTICAL'`) as they are integral to the format
  - Can be set via `--skip-column-names` command-line flag
  - Useful for scripting and data processing where only raw data is needed

TODO: Document other CLI_* variables