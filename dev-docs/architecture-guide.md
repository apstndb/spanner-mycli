# Architecture and Code Organization

This document provides detailed architectural information for spanner-mycli development.

## Core Components

### Entry Point and Configuration
- **main.go**: Entry point, CLI argument parsing, configuration management
- **session.go**: Database session management and Spanner client connections

### Interactive Interface
- **cli.go**: Main interactive CLI interface and batch processing
- **cli_output.go**: Output formatting and display logic
- **cli_readline.go**: Terminal input handling and readline integration
- **cli_mcp.go**: MCP (Model Context Protocol) server integration

### SQL Processing
- **statements.go**: Core SQL statement processing and execution
- **statements_*.go**: Specialized statement handlers:
  - `statements_mutations.go`: DML and mutation operations
  - `statements_schema.go`: DDL and schema operations
  - `statements_explain_describe.go`: Query analysis and introspection
  - `statements_llm.go`: GenAI integration
  - `statements_proto.go`: Protocol Buffers support
  - `statements_partitioned_query.go`: Partitioned operations
  - `statements_query_profile.go`: Query profiling and performance analysis

### Configuration and Variables
- **system_variables.go**: System variable definitions and management
- **client_side_statement_def.go**: **CRITICAL** - Defines all client-side statement patterns and handlers

## Client-Side Statement System

The `client_side_statement_def.go` file is the heart of spanner-mycli's extended SQL syntax.

### Core Components

- **clientSideStatementDef**: Structure defining regex patterns and handlers for custom statements
- **clientSideStatementDescription**: Human-readable documentation for each statement
- **Pattern Matching**: Uses compiled regex patterns for case-insensitive statement matching
- **Handler Functions**: Convert regex matches to structured Statement objects

### Statement Categories

#### Database Operations
- `USE` - Switch database context
- `DROP DATABASE` - Database deletion
- `SHOW DATABASES` - List available databases
- `DETACH` - Disconnect from current database

#### Schema Operations
- `SHOW CREATE` - Display DDL for objects
- `SHOW TABLES` - List tables in database
- `SHOW COLUMNS` - Display table structure
- `SHOW INDEX` - Show index information
- `SHOW DDLS` - Display all DDL statements

#### Query Analysis
- `EXPLAIN` - Show query execution plan
- `EXPLAIN ANALYZE` - Show execution plan with statistics
- `DESCRIBE` - Describe table or query structure
- `SHOW PLAN NODE` - Display specific plan node details

#### Transaction Control
- `BEGIN RW/RO` - Start read-write or read-only transactions
- `COMMIT` - Commit current transaction
- `ROLLBACK` - Rollback current transaction
- `SET TRANSACTION` - Configure transaction properties

#### System Variables
- `SET` - Set system variable values
- `SHOW VARIABLES` - Display all system variables
- `SHOW VARIABLE` - Display specific system variable

#### Advanced Features
- **Protocol Buffers**: Proto type management and operations
- **GenAI**: AI-powered query assistance
- **Partitioned Operations**: Large-scale data processing
- **Batching**: Batch operation management
- **Mutations**: DML operation handling

## Adding New Client-Side Statements

### Step-by-Step Process

1. **Add Definition**: Add new entry to `clientSideStatementDefs` slice in `client_side_statement_def.go`
   ```go
   {
       Regex: regexp.MustCompile(`(?is)^SHOW\s+MY_FEATURE(?:\s+(.*))?$`),
       Handler: func(matches []string) (Statement, error) {
           return &ShowMyFeatureStatement{
               Object: strings.TrimSpace(matches[1]),
           }, nil
       },
   }
   ```

2. **Define Regex Pattern**: Use `(?is)` flags for case-insensitive matching
   - `(?i)` - case-insensitive
   - `(?s)` - allow `.` to match newlines

3. **Create Statement Struct**: Define corresponding Statement struct
   ```go
   type ShowMyFeatureStatement struct {
       Object string
   }
   
   func (s *ShowMyFeatureStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
       // Implementation here
   }
   ```

4. **Add Implementation**: Create implementation in appropriate `statements_*.go` file

5. **Update Tests**: Add comprehensive test coverage

6. **Update Documentation**: Add to statement help and user documentation

### Pattern Guidelines

- **Naming**: Use clear, descriptive statement names
- **Regex**: Capture groups for parameters, handle optional elements
- **Error Handling**: Provide clear error messages for invalid syntax
- **Consistency**: Follow existing patterns for similar statements

## Configuration Management

### Configuration Sources (Priority Order)

1. Command-line flags
2. Environment variables
3. Configuration files (`.spanner_mycli.cnf`)
4. System defaults

### Configuration File

**Config file**: `.spanner_mycli.cnf` (searched in home directory, then current directory)

### Configuration File Format

```ini
[default]
project = myproject
instance = myinstance
database = mydatabase

[profile_name]
project = other-project
instance = other-instance
database = other-database
```

### Environment Variables

- `SPANNER_PROJECT_ID` - Default project ID
- `SPANNER_INSTANCE_ID` - Default instance ID
- `SPANNER_DATABASE_ID` - Default database ID

## Backward Compatibility

**spanner-mycli does not require traditional backward compatibility** since it's not used as an external library:

- **Clean refactoring over compatibility**: Prefer clear, well-named interfaces
- **Direct removal of old interfaces**: No need to maintain deprecated versions
- **Cleaner codebase**: No accumulation of deprecated interfaces or methods

## Testing Strategy

### Test Categories

- **Unit Tests**: `*_test.go` files alongside source code
- **Integration Tests**: `integration_test.go` with Spanner emulator
- **Slow Tests**: Separated with `skip_slow_test` build tag
- **MCP Tests**: `integration_mcp_test.go` for MCP server functionality

### Test Infrastructure

- **testcontainers**: Spanner emulator testing
- **Test Data**: `testdata/` directory with fixtures
- **Emulator Integration**: Automated emulator lifecycle management

### Test Execution

```bash
# Unit tests only
go test -short ./...

# All tests including integration
make test

# Slow tests (CI/local comprehensive testing)
go test -tags slow ./...

# Lint and style checks
make lint
```

## Dependencies

### Core Dependencies

- **Cloud Spanner SDK**: `cloud.google.com/go/spanner` - Primary Spanner client
- **SQL Parser**: `github.com/cloudspannerecosystem/memefish` - GoogleSQL parsing
- **CLI Framework**: `github.com/jessevdk/go-flags` - Command-line argument parsing
- **Terminal Interface**: `github.com/nyaosorg/go-readline-ny` - Interactive input
- **Table Output**: `github.com/olekukonko/tablewriter` - Formatted table display
- **GenAI**: `google.golang.org/genai` - AI functionality integration

### Dependency Behavior Notes

#### go-flags Library (Issue #251 Insights)

**Discovery**: go-flags library uses struct field values as defaults in help text, not just `default` tags

- **Default Display Control**: Use `default-mask:"-"` struct tag to hide config/env values from help text defaults
- **Architecture Pattern**: Avoid creating multiple parser instances with shared structs to prevent config value leakage into help display
- **Testing Requirement**: Help text output verification important when modifying flag parsing logic

```go
type Options struct {
    Project string `long:"project" env:"SPANNER_PROJECT_ID" default-mask:"-"`
}
```

## Build and Development

### Build System

- **Makefile**: Primary build interface
- **Go Modules**: Dependency management via `go.mod`
- **Cross-platform**: Supports macOS, Linux, Windows

### Development Commands

```bash
# Build application
make build

# Run with parameters
make run PROJECT=myproject INSTANCE=myinstance DATABASE=mydatabase

# Alternative direct execution
go run . -p PROJECT -i INSTANCE -d DATABASE

# Clean build artifacts
make clean
```

## File Organization

```
spanner-mycli/
├── main.go                          # Entry point
├── cli*.go                          # CLI interface components
├── session.go                       # Session management
├── statements*.go                   # Statement processing
├── system_variables.go              # System variable management
├── client_side_statement_def.go     # Statement definitions (CRITICAL)
├── execute_sql.go                   # SQL execution logic
├── internal/                        # Internal packages
├── testdata/                        # Test fixtures
├── docs/                            # User documentation
├── dev-docs/                        # Developer documentation
├── scripts/                         # Automation scripts
└── official_docs/                   # Upstream documentation
```

## Related Documentation

- [Development Insights](development-insights.md) - Development patterns and best practices
- [System Variable Patterns](patterns/system-variables.md) - System variable implementation
- [Issue Management](issue-management.md) - GitHub workflow and processes
- [Testing Guide](testing-guide.md) - Testing strategies and execution