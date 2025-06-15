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
make lint           # Run golangci-lint (REQUIRED before push)
make test           # Run all tests including integration tests (REQUIRED before push)
make clean          # Clean build artifacts and test cache
```

**CRITICAL REQUIREMENTS before push**:
1. **Always run `make test`** (not `make fasttest`) to ensure all integration tests pass
2. **Always run `make lint`** to ensure code quality and style compliance
3. Fix any test failures or lint errors before pushing changes or creating pull requests

**Note**: Integration tests use testcontainers with Spanner emulator and take longer to run, but they are essential to catch issues before CI.

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

### Issue Management Best Practices

#### Creating Issues
- Use `gh issue create --body` instead of GitHub MCP tools for better readability and formatting control:

```bash
gh issue create --title "Issue title" --body "$(cat <<'EOF'
## Problem
Description of the problem...

## Expected Behavior
What should happen...

## Actual Behavior  
What actually happens...

## Steps to Reproduce
1. Step one
2. Step two

## Additional Context
Any other relevant information...
EOF
)" --label "bug,enhancement"
```

#### Updating Issues
- Use `gh issue edit <number> --body "content"` for issue updates
- Multi-line content can be handled with heredoc or escaped newlines in shell commands
- This ensures better readability than using the GitHub MCP server for issue updates

#### Why Use gh CLI Over GitHub MCP
- Better control over markdown formatting
- More readable issue content
- Proper handling of multi-line content and code blocks
- Consistent formatting with project standards
- Direct command-line control without API abstraction layers

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
   
   # Get specific review's comments by review ID (most efficient for targeted review inspection)
   gh api graphql -f query='{ node(id: "<REVIEW_ID>") { ... on PullRequestReview { comments(first: 50) { nodes { id body path line originalLine } } } } }' | jq '.data.node.comments.nodes'
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

## Development Workflow Best Practices

### Issue and Pull Request Management

#### Linking Issues and Pull Requests
When working on issues, always use GitHub's issue linking syntax in both commit messages and PR descriptions to enable automatic issue closure and better traceability:

**In commit messages:**
```bash
git commit -m "feat: implement optional --database flag with detach/attach functionality

Resolves #258
Fixes #262

🤖 Generated with [Claude Code](https://claude.ai/code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**In PR descriptions:**
```markdown
## Summary
This PR implements the optional --database flag functionality.

## Issues Resolved
- Resolves #258 - Make --database flag optional with detach/attach functionality  
- Fixes #262 - Integration tests cannot properly test CLI-level statements

## Changes
- Added SessionHandler for proper session management
- Implemented USE/DETACH statements with actual session switching
- Enhanced integration tests to verify CLI-level statement behavior
```

**Supported Keywords:** `Closes`, `Fixes`, `Resolves` (and their variations: `Close`, `Fix`, `Resolve`)

#### Gemini Code Assist Integration
- **Manual Review Trigger**: Gemini Code Assist does not automatically review PRs
- **Trigger Command**: Use `/gemini review` as an issue comment to request code review
- **Review Response**: Gemini typically responds within 1-2 minutes with comprehensive feedback
- **Re-review Process**: After addressing comments, use `/gemini review` again for follow-up review


#### Code Review Response Strategy
1. **Address each comment individually** with focused commits
2. **Use descriptive commit messages** that reference the specific issue being addressed
3. **Test thoroughly** before pushing changes
4. **Request re-review** using appropriate methods for each reviewer type
5. **Document architectural decisions** in code comments and CLAUDE.md updates

#### Handling Different Types of Review Comments

**For Issues Requiring Fixes:**
1. Create focused commits addressing the specific issue
2. Reference the review comment in commit message
3. Push changes and request re-review

**For Praise Comments from AI Automated Reviews (e.g., Gemini Code Assist):**
1. **Acknowledge the feedback** with a brief reply thanking the reviewer
2. **Resolve the conversation** to keep the review clean and focused on actionable items
3. **Example response**: "Thank you for highlighting this good practice! Resolving as acknowledged."

**Sample AI praise comment response workflow:**
```bash
# Step 1: Reply directly to the specific review comment using REST API
gh api repos/apstndb/spanner-mycli/pulls/263/comments \
  -f body="Thank you for highlighting this good practice! The admin-only mode check is indeed important for preventing database access errors when not connected to a specific database. Resolving as acknowledged." \
  -F in_reply_to=2148555280

# Step 2: Resolve the review thread using GraphQL mutation
gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "PRRT_kwDONC6gMM5SSFCY"}) { thread { id isResolved } } }'

# Note: You'll need to get the specific comment ID and thread ID from:
# gh api repos/OWNER/REPO/pulls/PR_NUMBER/comments | jq '.[] | {id, body: .body[0:100]}'
# gh api graphql -f query='{ repository(owner: "OWNER", name: "REPO") { pullRequest(number: PR_NUMBER) { reviewThreads(first: 10) { nodes { id comments(first: 2) { nodes { id author { login } } } } } } } }'
```

**Note**: This workflow is specifically for automated AI reviews (like Gemini Code Assist) that often include praise comments to highlight good implementation practices. For human reviewers, follow standard code review etiquette and engage in meaningful discussion about the feedback.

This approach maintains good communication with AI reviewers while keeping the review focused on actionable items and reduces noise from praise-only comments.

## Documentation Update Workflow

### Updating README.md with Current Help Output

When updating README.md documentation sections that include command-line help or statement help output:

1. **Generate help output**:
   ```bash
   # Create tmp directory if not exists
   mkdir -p ./tmp
   
   # Generate --help output with 200-column width (avoids 80-column wrapping)
   script -q ./tmp/help_output.txt sh -c "stty cols 200; go run . --help"
   
   # Generate --statement-help output (no script needed, table format doesn't wrap)
   go run . --statement-help > ./tmp/statement_help.txt
   
   # Clean up control characters if needed for --help
   sed '1s/^.\{2\}//' ./tmp/help_output.txt > ./tmp/help_clean.txt
   ```

2. **Update README.md**: Replace existing content with the exact output from the generated files
3. **Document the source**: 
   - For --help: `<!-- Generated with: script -q ./tmp/help_clean.txt sh -c "stty cols 200; go run . --help" -->`
   - For --statement-help: `<!-- Generated with: go run . --statement-help > ./tmp/statement_help.txt -->`
4. **Don't manually format**: Even if tables become long, use the actual output without manual formatting changes

**Why different approaches for --help vs --statement-help?**
- **--help**: go-flags library detects terminal width and wraps at 80 columns when redirected to files. Using `script` creates a pseudo-terminal (PTY) with 200-column width to prevent wrapping.
- **--statement-help**: Table format output doesn't have line wrapping issues, so direct redirection works fine.

**Example workflow for updating help sections:**

```bash
# Generate help outputs
mkdir -p ./tmp
script -q ./tmp/help_output.txt sh -c "stty cols 200; go run . --help"
go run . --statement-help > ./tmp/statement_help.txt

# Clean control characters for --help if present
sed '1s/^.\{2\}//' ./tmp/help_output.txt > ./tmp/help_clean.txt

# Update README.md sections with actual output
# For --help section, replace content between:
# <!-- Generated with: script -q ./tmp/help_clean.txt sh -c "stty cols 200; go run . --help" -->
# ```
# [OLD CONTENT]
# ```
# 
# With content from ./tmp/help_clean.txt

# For --statement-help section, replace content between:
# <!-- Generated with: go run . --statement-help > ./tmp/statement_help.txt -->
# | Usage | Syntax | Note |
# 
# With content from ./tmp/statement_help.txt
```

**Benefits of this approach:**
- Ensures documentation matches actual behavior
- Prevents drift between documentation and implementation
- Produces clean, unwrapped output suitable for documentation (for --help)
- Makes it clear where content comes from and how it was generated
- Avoids 80-column wrapping that makes --help documentation harder to read
- Simplifies maintenance by removing manual formatting decisions

This workflow should be followed whenever:
- Command-line flags are added, removed, or modified
- Client-side statements are added, removed, or modified
- Help text descriptions are updated
- Documentation review identifies outdated help content

**Important Git Practices:**
- Always use `git add <specific-files>` instead of `git add .` to avoid committing unintended files
- Example: `git add README.md CLAUDE.md` instead of `git add .`
- Check `git status` before committing to verify only intended files are staged
- Use `git show --name-only` to review what files were included in the last commit