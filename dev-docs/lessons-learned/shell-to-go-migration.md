# Go Development Tools: Current Design (Issue #301)

## Overview

AI-friendly development tools built with Go to replace scattered shell scripts. Focus on GitHub API optimization, unified review analysis, and structured AI assistant workflows.

## Current Architecture

### Unified Module Design

**Structure**: Single Go module with shared utilities
```
dev-tools/
├── go.mod                    # Unified dependency management
├── shared/                   # Optimized GitHub API client
├── gh-helper/               # Generic GitHub operations
└── spanner-mycli-dev/       # Project-specific workflows
```

**Key improvements from shell scripts**:
- 88% reduction in GitHub API calls (17 → 2 for review workflows)
- HTTP/2 optimization with connection pooling
- Unified review analysis preventing feedback oversight

## Current Technical Design

### GitHub API Optimization Strategy

**Core Architecture** (`dev-tools/shared/github.go:125-146`):
```go
// 1. HTTP/2 with Connection Reuse
transport := &http.Transport{
    MaxIdleConns:        100,  // Global connection pool
    MaxIdleConnsPerHost: 10,   // GitHub API specific
    IdleConnTimeout:     90 * time.Second,
    TLSClientConfig: &tls.Config{
        NextProtos: []string{"h2", "http/1.1"}, // Prefer HTTP/2
    },
}

// 2. Single Command Lifecycle Design
func getToken() (string, error) {
    // Direct gh auth token calls - no caching complexity
    cmd := exec.Command("gh", "auth", "token")
    // ... 
}
```

**Measured Performance Impact**:
- Header compression (HPACK) reduces Authorization header overhead
- Connection multiplexing eliminates TCP handshake per request
- 88% API call reduction: 17 individual `gh` commands → 2 GraphQL queries

### GraphQL Query Patterns

**Fragment-based approach eliminates redundancy**:
```go
// Before: Duplicated field definitions across 3 queries
// After: Single fragment definition, ~60% reduction in query size
const ReviewFragment = `
fragment ReviewFields on PullRequestReview {
  id
  author { login }
  createdAt
  state
  body @include(if: $includeReviewBodies)
  // ...
}`
```

**Safety and optimization patterns**:
- Use GraphQL variables instead of `fmt.Sprintf` (injection-safe) 
- Conditional directives (@include/@skip) for dynamic field selection
- Separate queries per pagination direction vs complex conditionals
- Batch operations eliminate N+1 query problems (threads, reviews)

### Instance-level vs Global Caching

**Decision**: Instance-level repository ID caching
```go
type GitHubClient struct {
    Owner        string
    Repo         string
    repositoryID string // Simple field vs sync.Map
}
```

**Rationale**:
- Typical usage: one repository per client instance
- Repository IDs are immutable (no TTL complexity)
- Simpler than global cache with sync.Map and type assertions
- Follows YAGNI principle for current usage patterns

## Workflow-Specific Learnings

### Unified Review Analysis Pattern

**Critical insight**: Review bodies vs inline threads contain different types of feedback
- **Inline threads**: Specific code implementation issues
- **Review bodies**: Architecture concerns, high-level design feedback, critical system behavior

**Problem identified**: Tools focusing only on threaded comments miss critical feedback
```bash
# Incomplete - only inline comments
gh-helper reviews fetch <PR> --list-threads
```

**Solution implemented**: Comprehensive review analysis
```bash
# Complete - reviews + threads + severity detection  
gh-helper reviews analyze <PR>
```

**Technical implementation**: `shared/unified_review.go` + `shared/review_monitor.go`
- Single GraphQL query fetches both review bodies and threads
- Severity detection extracts critical/high-priority items
- Actionable item extraction with keyword analysis

### Timeout Optimization

**Data-driven approach**: Measured actual operation times
- **Gemini reviews**: 1-3 minutes (95th percentile: 4 minutes)
- **CI checks**: 2-5 minutes (95th percentile: 6 minutes)
- **Combined workflow**: 3-6 minutes

**Decision**: 5-minute default timeout
- Covers 95% of real cases
- Fails fast for actual problems
- Replaced conservative 15-minute timeout from shell scripts

### Smart Number Resolution

**AI Assistant requirement**: Reduce cognitive load for PR/issue confusion

**Implementation**:
```go
// Unified resolution strategy
func (c *GitHubClient) ResolvePRNumber(input string) (int, string, error) {
    // 1. Empty → current branch PR
    // 2. "issues/123" → explicit issue resolution  
    // 3. "123" → auto-detect issue vs PR
    // 4. "pr/123" → explicit PR
}
```

**Result**: Eliminates manual PR number lookup for AI assistants

## AI Assistant-Specific Patterns

### Command Design

**Successful patterns**:
1. **stdin over temporary files**: Direct piping from AI assistants
2. **Self-documenting help**: Comprehensive `--help` at every level
3. **Predictable JSON output**: Structured data for AI processing
4. **No interactive prompts**: All input via flags or stdin

### Permission Boundaries

**Clear separation discovered**:
- **Read operations**: Safe for autonomous AI use (reviews wait, reviews fetch --list-threads)
- **Write operations**: Require user confirmation (threads reply, PR creation)
- **Mixed operations**: Explicit flags for write portions (--request-review)

### Error Message Design

**AI-friendly error format**:
```go
return fmt.Errorf("invalid PR number format: %w\n\nHint: Use 'pr/123' for explicit PR or just '123' for auto-detection", err)
```

**Key elements**:
- Clear error description
- Actionable hints
- Examples of correct usage

## Module Architecture Insights

### Unified Module vs Separate Modules

**Tried approaches**:
1. **Separate modules per tool** - Complex dependency management
2. **Shared module with replace directives** - Development friction  
3. **Unified module with shared package** - Final choice

**Unified module benefits**:
- Single `go.mod` for dependency management
- Natural code sharing via internal package
- Simpler testing and maintenance
- ~30 lines of duplicate code eliminated

### Shared Package Design

**Effective organization**:
```
dev-tools/shared/
├── github.go          # GitHub API optimization
├── utils.go           # File operations, command execution  
├── cache.go           # Token caching
├── threads.go         # Thread batch fetching
├── unified_review.go  # Comprehensive review analysis
├── graphql_fragments.go # Type-safe GraphQL fragments
└── review_monitor.go  # Severity detection (PR #306 lesson)
```

**Package responsibilities**:
- **github.go**: API optimization and HTTP/2 setup
- **unified_review.go**: Lesson from PR #306 - unified analysis
- **graphql_fragments.go**: Type safety for GraphQL operations

## Migration Methodology

### Incremental Approach

**Strategy**: Gradual replacement while maintaining shell script compatibility
1. Build Go tools alongside existing scripts
2. Update CLAUDE.md references progressively  
3. Remove shell scripts after Go tool validation
4. Update Makefile to use new tools

**Validation process**:
- Side-by-side testing of shell vs Go implementations
- Performance measurement with real workloads
- AI assistant testing for usability

### Backward Compatibility

**Maintained during transition**:
- Same command-line interface patterns
- Similar timeout behaviors  
- Equivalent error handling (improved)
- JSON output format compatibility

## Future Implications

### Scalability Foundations

**Architecture supports**:
- Additional tools under unified module
- Enhanced shared utilities
- Plugin system for custom commands
- Integration with other CI systems

### Performance Monitoring

**Established patterns**:
- HTTP/2 optimization techniques
- GraphQL query consolidation  
- Token caching strategies
- Batch operation design

### AI Assistant Evolution

**Patterns validated**:
- stdin/heredoc input design
- Self-documenting command structure
- Clear permission boundaries
- Structured error messaging

These learnings provide a foundation for future development tool evolution while avoiding common pitfalls discovered during the migration process.