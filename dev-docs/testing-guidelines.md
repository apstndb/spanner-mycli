# Testing Guidelines

This document outlines the testing standards and coverage goals for spanner-mycli.

## Coverage Goals

### Checking Current Status
To check the current coverage status, run:
```bash
make test-coverage
```

This will display the overall coverage percentage and generate a detailed HTML report.

### Target Coverage
- **Short-term Goal (3 months)**: 70% overall coverage
- **Medium-term Goal (6 months)**: 75% overall coverage
- **Long-term Goal**: 80% overall coverage

We aim for gradual, sustainable improvement rather than rushing to meet arbitrary numbers.

## Running Tests Locally

### Basic Test Commands
```bash
# Run all tests
make test

# Run quick tests (with -short flag)
make test-quick

# Run tests with coverage report
make test-coverage

# Run coverage and open HTML report in browser
make test-coverage-open
```

### Understanding Coverage Reports

After running `make test-coverage`, you'll find:
- **Coverage profile**: `tmp/coverage.out` - Raw coverage data
- **HTML report**: `tmp/coverage.html` - Visual coverage report
- **Terminal summary**: Shows overall coverage percentage

The HTML report provides:
- Line-by-line coverage visualization
- Red highlighting for uncovered code
- Green highlighting for covered code
- Per-file and per-package statistics

## Writing Effective Tests

### Test Organization
- Place tests in `*_test.go` files alongside the code they test
- Use table-driven tests for multiple similar test cases
- Group related tests using subtests (`t.Run()`)

### Test Naming Conventions
```go
// Function tests: Test<FunctionName>
func TestParseQuery(t *testing.T) { }

// Method tests: Test<Type>_<MethodName>
func TestSession_ExecuteQuery(t *testing.T) { }

// Scenario tests: Test<Type>_<Scenario>
func TestSession_ConcurrentQueries(t *testing.T) { }
```

### Comparison and Assertions
- **Prefer `google/go-cmp`** over `reflect.DeepEqual` for comparing complex structures
  - Provides better diff output on failures
  - Handles unexported fields and custom comparisons
  - Example:
    ```go
    import "github.com/google/go-cmp/cmp"
    
    if diff := cmp.Diff(want, got); diff != "" {
        t.Errorf("ParseQuery() mismatch (-want +got):\n%s", diff)
    }
    ```
- Use `reflect.DeepEqual` only for simple comparisons
- Consider `testify/assert` for more readable assertions

### Mock Guidelines
- Use interfaces for external dependencies
- Create test doubles in `*_test.go` files
- Consider using `testify/mock` for complex mocks

### Integration Tests
- Integration tests are skipped when using `go test -short` (no build tags needed)
- Mock Google Cloud Spanner client for most tests
- Use emulator for integration tests when necessary
- Mark integration tests with `t.Skip()` when `testing.Short()` is true:
  ```go
  func TestIntegration(t *testing.T) {
      if testing.Short() {
          t.Skip("skipping integration test in short mode")
      }
      // integration test code here
  }
  ```

## Priority Areas for Coverage Improvement

### High Priority
1. **Core SQL Processing** (`statements.go`)
   - Statement parsing and execution
   - Error handling paths
   - Edge cases in query processing

2. **Session Management** (`session.go`)
   - Connection lifecycle
   - Transaction handling
   - Concurrent operations

3. **Internal Packages**
   - `internal/protostruct` - Protocol buffer utilities
   - Other internal utilities (excluding generated code)

4. **System Variables** (`system_variables.go`)
   - Variable validation
   - Type conversions
   - Default value handling

### Medium Priority
1. **Client-side Statements** (`client_side_statement_def.go`)
   - Pattern matching
   - Command execution
   - Error scenarios

2. **Interactive Features**
   - Readline integration
   - Command completion
   - History management

### Low Priority
1. **Generated Code** (`internal/proto/zetasql`)
   - Generally excluded from coverage goals
   - Focus on integration tests instead

2. **Utility Functions**
   - Simple helpers with obvious behavior
   - Consider cost/benefit of testing

## CI/CD Integration

### Pull Request Coverage
- Octocov automatically comments on PRs with coverage changes
- Coverage report artifacts are available for 7 days
- Use the artifact link in PR comments to view detailed HTML reports

### Coverage Enforcement
Currently, we don't enforce minimum coverage thresholds. Instead, we:
- Monitor coverage trends over time
- Review coverage changes in PRs
- Encourage incremental improvements

## Best Practices

1. **Write tests alongside new features** - Don't accumulate test debt
2. **Test edge cases** - Empty inputs, nil values, concurrent access
3. **Test error paths** - Ensure errors are handled gracefully
4. **Keep tests fast** - Use mocks instead of real services
5. **Make tests deterministic** - Avoid time-dependent or random behavior
6. **Document test intentions** - Use descriptive test names and comments

### Test Safety and Isolation

#### Global State Management
- **Always use `t.Setenv()`** instead of `os.Setenv()` for environment variables
- **Avoid `os.Chdir()`** - use absolute paths or parser methods instead
- **Use `t.TempDir()`** for temporary files - automatic cleanup
- **Consider `t.Parallel()`** compatibility even if not using it yet

#### Test Data Setup
```go
// Good - using t.TempDir() for test files
func TestWithTempFile(t *testing.T) {
    tempDir := t.TempDir() // Automatically cleaned up
    tempFile := filepath.Join(tempDir, "test.txt")
    if err := os.WriteFile(tempFile, []byte("test data"), 0644); err != nil {
        t.Fatal(err)
    }
    // Use tempFile in test...
}

// Good - using cleanup functions for resources
func TestWithResource(t *testing.T) {
    resource, err := createResource()
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { 
        if err := resource.Close(); err != nil {
            t.Logf("cleanup failed: %v", err)
        }
    })
    // Use resource in test...
}
```

## Common Testing Patterns

### Flag Testing
When testing CLI flags, consider the following patterns implemented in `main_flags_test.go`:

#### Multi-Stage Validation
```go
// Flags are validated at different stages:
// 1. Parsing (go-flags)
// 2. ValidateSpannerOptions()
// 3. initializeSystemVariables()

// Test accordingly:
_, parseErr := parser.ParseArgs(tt.args)
if parseErr == nil {
    err = ValidateSpannerOptions(&opts)
    if err == nil {
        _, err = initializeSystemVariables(&opts)
    }
}
```

#### Environment Variable Management
Use `t.Setenv()` for safe parallel test execution:
```go
// Good - automatically cleaned up, safe for parallel tests
t.Setenv("SPANNER_DATABASE_ID", "test-db")

// Avoid - requires manual cleanup, not safe for parallel tests
os.Setenv("SPANNER_DATABASE_ID", "test-db")
defer os.Unsetenv("SPANNER_DATABASE_ID")
```

**Key Benefits of t.Setenv():**
- **Thread-safe**: Works correctly even when `t.Parallel()` is added later
- **Automatic cleanup**: No need for defer statements or manual restoration
- **Test isolation**: Each test gets its own environment snapshot
- **Future-proof**: Prevents test flakiness when tests are parallelized

**When testing multiple environment variables:**
```go
// Set multiple environment variables in tests
envVars := map[string]string{
    "SPANNER_PROJECT_ID": "test-project",
    "SPANNER_INSTANCE_ID": "test-instance",
    "SPANNER_DATABASE_ID": "test-database",
}
for k, v := range envVars {
    t.Setenv(k, v)
}
```

#### Terminal Testing with PTY
For accurate terminal detection, use PTY helpers:
```go
// Helper types for stdin simulation
type stdinProvider func() (io.Reader, func(), error)

func ptyStdin() stdinProvider {
    return func() (io.Reader, func(), error) {
        pty, tty, err := pty.Open()
        if err != nil {
            return nil, nil, err
        }
        cleanup := func() {
            pty.Close()
            tty.Close()
        }
        return tty, cleanup, nil
    }
}

// Use in tests to simulate interactive mode
stdin, cleanup, err := ptyStdin()()
defer cleanup()
interactive := term.IsTerminal(int(stdin.(*os.File).Fd()))
```

#### Config File Testing
Avoid `os.Chdir()` in tests; parse config files directly:
```go
// Good - no global state change
iniParser := flags.NewIniParser(parser)
err := iniParser.ParseFile(configFile)

// Avoid - changes global working directory
os.Chdir(configDir)
defer os.Chdir(oldDir)
```

**Why avoiding global state matters:**
- **Parallel test execution**: `os.Chdir()` affects all running tests
- **Test isolation**: Each test should be independent
- **Predictable behavior**: Tests should not depend on execution order
- **CI/CD compatibility**: Works reliably in different environments

### Table-Driven Tests
```go
import "github.com/google/go-cmp/cmp"

func TestParseQuery(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Query
        wantErr bool
    }{
        {
            name:  "simple select",
            input: "SELECT * FROM users",
            want:  &Query{Type: Select, Table: "users"},
        },
        {
            name:    "invalid syntax",
            input:   "INVALID QUERY",
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseQuery(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ParseQuery() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !tt.wantErr {
                if diff := cmp.Diff(tt.want, got); diff != "" {
                    t.Errorf("ParseQuery() mismatch (-want +got):\n%s", diff)
                }
            }
        })
    }
}
```

### Testing with Mocks
```go
type mockSpannerClient struct {
    mock.Mock
}

func (m *mockSpannerClient) ExecuteQuery(ctx context.Context, query string) (*Result, error) {
    args := m.Called(ctx, query)
    return args.Get(0).(*Result), args.Error(1)
}

func TestSession_ExecuteQuery(t *testing.T) {
    client := new(mockSpannerClient)
    client.On("ExecuteQuery", mock.Anything, "SELECT 1").Return(&Result{Value: 1}, nil)
    
    session := &Session{client: client}
    result, err := session.ExecuteQuery(context.Background(), "SELECT 1")
    
    assert.NoError(t, err)
    assert.Equal(t, 1, result.Value)
    client.AssertExpectations(t)
}
```

## Troubleshooting

### Common Issues

1. **Tests timing out**
   - Check for deadlocks or infinite loops
   - Use `context.WithTimeout()` for operations that might hang
   - Add `-timeout` flag to go test command

2. **Flaky tests**
   - Remove time dependencies
   - Use deterministic random seeds
   - Mock external services

3. **Coverage not improving**
   - Check if code is actually reachable
   - Look for dead code that can be removed
   - Consider if 100% coverage is realistic for the code

### Getting Help

- Check existing test examples in the codebase
- Refer to Go testing documentation: https://golang.org/pkg/testing/
- Ask in PR comments for specific testing advice