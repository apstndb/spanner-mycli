# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

spanner-mycli is a personal fork of spanner-cli, designed as an interactive command-line tool for Google Cloud Spanner. The project philosophy is "by me, for me" - a continuously evolving tool that prioritizes the author's specific needs over stability. It embraces experimental features and follows a "ZeroVer" approach (will never reach v1.0.0).

## Common Development Commands

### Building and Running
```bash
make build          # Build the application
make run PROJECT=myproject INSTANCE=myinstance DATABASE=mydatabase
go run . -p PROJECT -i INSTANCE -d DATABASE  # Run directly with Go
```

### Testing
```bash
make test           # Run all tests
make test-verbose   # Run all tests with verbose output
make fasttest       # Run tests excluding slow tests (skip_slow_test build tag)
make fasttest-verbose  # Run fast tests with verbose output
```

### Development
```bash
make lint           # Run golangci-lint
make clean          # Clean build artifacts and test cache
```

## Architecture

### Core Components
- **main.go**: Entry point, CLI argument parsing, configuration management
- **cli.go**: Main interactive CLI interface and batch processing
- **session.go**: Database session management and Spanner client connections
- **statements.go**: Core SQL statement processing and execution
- **statements_*.go**: Specialized statement handlers (mutations, schema, explain, llm, proto)
- **client_side_statement_def.go**: **CRITICAL** - Defines all client-side statement patterns and handlers

### Client-Side Statement System
The `client_side_statement_def.go` file is the heart of spanner-mycli's extended SQL syntax. It contains:

- **clientSideStatementDef**: Structure defining regex patterns and handlers for custom statements
- **clientSideStatementDescription**: Human-readable documentation for each statement
- **Pattern Matching**: Uses compiled regex patterns for case-insensitive statement matching
- **Handler Functions**: Convert regex matches to structured Statement objects

Key client-side statement categories:
- **Database Operations**: `USE`, `DROP DATABASE`, `SHOW DATABASES`
- **Schema Operations**: `SHOW CREATE`, `SHOW TABLES`, `SHOW COLUMNS`, `SHOW INDEX`, `SHOW DDLS`
- **Protocol Buffers**: `SHOW LOCAL PROTO`, `SHOW REMOTE PROTO`, `SYNC PROTO BUNDLE`
- **Query Analysis**: `EXPLAIN`, `EXPLAIN ANALYZE`, `DESCRIBE`, `SHOW PLAN NODE`
- **Partitioned Operations**: `PARTITION`, `RUN PARTITIONED QUERY`, `TRY PARTITIONED QUERY`
- **Transaction Control**: `BEGIN RW/RO`, `COMMIT`, `ROLLBACK`, `SET TRANSACTION`
- **System Variables**: `SET`, `SHOW VARIABLES`, `SHOW VARIABLE`
- **Query Parameters**: `SET PARAM`, `SHOW PARAMS`
- **Mutations**: `MUTATE` statements for direct table modifications
- **Batching**: `START BATCH`, `RUN BATCH`, `ABORT BATCH`
- **Split Points**: `ADD SPLIT POINTS`, `DROP SPLIT POINTS`, `SHOW SPLIT POINTS`
- **GenAI**: `GEMINI` statements for AI-assisted query generation
- **Cassandra**: `CQL` statements for Cassandra interface (experimental)

### Key Features
- **System Variables**: Generalized configuration system inspired by Spanner JDBC properties
- **Enhanced SQL Support**: Built on memefish parser with Spanner-specific extensions
- **Protocol Buffers**: Full proto support for advanced use cases
- **GenAI Integration**: Gemini support for query composition
- **Advanced Query Planning**: Comprehensive query plan analysis and visualization
  - Configurable EXPLAIN ANALYZE with customizable columns and inline stats
  - Query plan investigation with EXPLAIN/EXPLAIN ANALYZE LAST QUERY and SHOW PLAN NODE
  - Compact format and wrapped plans for constrained display environments
  - Query plan linter (experimental) and query profiles (experimental)
- **Partitioned Operations**: Support for partitioned queries and DML
- **Improved Interactive Experience**: Multi-line editing, syntax highlighting, progress bars, paging

### Testing Approach
- Unit tests in `*_test.go` files
- Integration tests in `integration_test.go`
- Slow tests separated with `skip_slow_test` build tag
- Uses testcontainers for Spanner emulator testing
- Test data in `testdata/` directory

### Dependencies
- Cloud Spanner SDK: `cloud.google.com/go/spanner`
- SQL Parser: `github.com/cloudspannerecosystem/memefish`
- CLI: `github.com/jessevdk/go-flags`, `github.com/nyaosorg/go-readline-ny`
- Output: `github.com/olekukonko/tablewriter`
- GenAI: `google.golang.org/genai`

## Development Notes

### Philosophy
This project embraces continuous change and experimentation. It prioritizes the author's specific needs and allows testing of new features that might be rejected in more conservative projects. The tool is intentionally experimental and follows a "ZeroVer" version policy.

### Issue Management
- Issues are managed through GitHub Issues with comprehensive labeling system
- All fixes must go through Pull Requests - never close issues manually
- Use "claude-code" label for issues identified by automated code analysis
- Additional labels: bug, enhancement, documentation, testing, tech-debt, performance, blocked, concurrency

### File Structure
- Statement processing follows modular pattern in `statements_*.go` files
- Output formatting centralized in `cli_output.go`
- System variables managed in `system_variables.go`
- Error handling standardized in `errors.go`
- **Client-side statements defined in `client_side_statement_def.go`**

### Adding New Client-Side Statements
When adding new client-side statements:
1. Add new entry to `clientSideStatementDefs` slice in `client_side_statement_def.go`
2. Define regex pattern with `(?is)` flags for case-insensitive matching
3. Create corresponding Statement struct and handler
4. Add implementation in appropriate `statements_*.go` file
5. Update tests and documentation

### Build Tags
- Use `skip_slow_test` tag to exclude slow integration tests
- Separate slow tests in files like `main_slow_test.go`

### Configuration
- Config file: `.spanner_mycli.cnf` (home directory or current working directory)
- Environment variables: `SPANNER_PROJECT_ID`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`

## Code Review Guidelines

### Pull Request Process
- All changes must go through Pull Requests
- PRs should include clear description of changes and test plan
- Link PRs to relevant issues using "Fixes #issue-number"
- Use descriptive commit messages following conventional format

### Code Quality Standards
- Follow existing code patterns and conventions
- Add tests for new functionality
- Ensure `make test` passes
- Run `make lint` for code quality (note: local linter config may differ from CI)
- Keep changes focused and minimal when fixing bugs

### Review Criteria
- Code correctness and logic
- Following project conventions
- Performance implications
- Test coverage adequacy
- Security considerations
- Error handling completeness

## Documentation Structure

### Overview
- **README.md**: High-level feature overview with basic examples and project differences
- **docs/query_plan.md**: Comprehensive query plan feature documentation with detailed examples

### Recent Documentation Updates (2025-06)
- Split query plan documentation from README.md into dedicated docs/query_plan.md
- Reorganized README.md to position query plan features as top-level (not minor use cases)
- Improved documentation structure separating overview from detailed technical documentation
- Enhanced emphasis on constrained display environment support (narrow terminals, code blocks, documentation sites)
- Added comprehensive issue tracking and code review process documentation

### Code Review Process

#### Review Comment Management
1. **Extract Review Comments**:
   ```bash
   # Get all PR-level comments
   gh pr view <PR_NUMBER> -R <OWNER>/<REPO> --json 'comments'
   
   # Get specific reviewer's comments (e.g., Gemini Code Assist)
   gh pr view <PR_NUMBER> -R <OWNER>/<REPO> --json 'comments' | jq '.comments[] | select(.author.login == "gemini-code-assist")'
   
   # Get all line-level review comments
   gh api graphql -f query='{ repository(owner: "<OWNER>", name: "<REPO>") { pullRequest(number: <PR_NUMBER>) { reviews(first: 100) { nodes { author { login } comments(first: 25) { edges { node { id body path line originalLine } } } } } } } }'
   
   # Get specific reviewer's line comments
   gh api graphql -f query='{ repository(owner: "<OWNER>", name: "<REPO>") { pullRequest(number: <PR_NUMBER>) { reviews(first: 100) { nodes { author { login } comments(first: 25) { edges { node { id body path line originalLine } } } } } } } }' | jq '.data.repository.pullRequest.reviews.nodes[] | select(.author.login == "<REVIEWER_LOGIN>") | .comments.edges[] | .node'
   ```

2. **Efficient Review Tracking** (Automated Script):
   ```bash
   # Use the provided script for robust incremental review checking
   ./scripts/check-pr-reviews.sh <PR_NUMBER> [OWNER] [REPO]
   
   # Example usage:
   ./scripts/check-pr-reviews.sh 259                    # Uses default apstndb/spanner-mycli
   ./scripts/check-pr-reviews.sh 259 owner repo-name   # Custom owner/repo
   
   # The script automatically:
   # 1. Fetches latest reviews with timestamps and IDs
   # 2. Compares with previous state (stored in .cache/pr-reviews/)
   # 3. Verifies data integrity by checking if previous review still exists
   # 4. Shows only new reviews since last check
   # 5. Updates state for next incremental check
   
   # First run shows all recent reviews and creates baseline state
   # Subsequent runs show only new reviews, preventing duplicates and missed reviews
   ```

   **Script Advantages vs GitHub MCP Server:**
   - **Incremental Efficiency**: Only fetches new comments since last check, reducing API calls
   - **Built-in State Management**: Automatically maintains state in `.cache/pr-reviews/` (git-ignored)
   - **Data Integrity Verification**: Checks if previously seen reviews still exist to detect data corruption
   - **Optimized GitHub API Usage**: Uses direct `gh` CLI and GraphQL for minimal data transfer
   - **Robust Duplicate Prevention**: Timestamp and ID-based comparison prevents missing or duplicate reviews
   - **Single Command Simplicity**: No need to implement client-side filtering or caching logic
   
   **MCP Alternative Trade-offs:**
   - MCP functions require full data retrieval on each check (less efficient)
   - Need to implement state management and incremental logic manually
   - Client-side filtering required instead of API-level filtering
   - Multiple API calls may be needed vs single GraphQL query

3. **Address Comments Individually**:
   - Handle each review comment as a separate commit
   - Use clear commit messages describing the specific fix
   - Prioritize by severity: critical → high → medium → low (for automated reviews)
   - Prioritize by reviewer feedback and discussion for human reviews
   - One commit per review comment for better traceability

4. **Follow-up Actions**:
   - For Gemini Code Assist: Comment `/gemini review` to trigger re-review
   - For human reviewers: Tag reviewer or request re-review through GitHub UI
   - Mark conversations as resolved after addressing each comment