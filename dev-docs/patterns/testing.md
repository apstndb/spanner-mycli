# Testing Best Practices

## Coverage Analysis

- Use `go tool cover -func` for a quick, readable summary of function-level coverage
- Use `go tool cover -html` for detailed line-by-line analysis
- Before and after test refactoring, always verify coverage doesn't decrease:

```bash
# Before changes
go test ./... -coverprofile=tmp/coverage_before.out
go tool cover -func=tmp/coverage_before.out | tail -1  # Total coverage

# After changes
go test ./... -coverprofile=tmp/coverage_after.out
go tool cover -func=tmp/coverage_after.out | tail -1   # Compare with before

# Detailed comparison for specific files
go tool cover -func=tmp/coverage_before.out | grep "filename.go"
go tool cover -func=tmp/coverage_after.out | grep "filename.go"
```

## Test File Organization

- Each production code file (`*.go`) should have a corresponding test file (`*_test.go`)
- Integration tests should be consolidated into existing `integration_*_test.go` files when possible
- Avoid creating new `integration_*_test.go` files unless testing a completely new feature area

## Test Refactoring

- Create helper functions ONLY when they can be reused across multiple test cases
- Use table-driven tests with descriptive test names
- Group related tests using `t.Run()` subtests
- Focus on improving signal-to-noise ratio - make test intent clearer
- Increase test density by removing unnecessary boilerplate while preserving clarity
- Always run full test suite after refactoring to ensure no breakage

## Test Code Principles

### Readability
Avoid boolean parameters - use descriptive wrapper functions:
- Bad: `testStatement(t, input, want, true)` - unclear what `true` means
- Good: `testStatementTypeOnly(t, input, want)` - intent is clear

### Signal-to-Noise Ratio
- Signal: Clear test names, obvious assertions, easy-to-understand failures
- Noise: Boilerplate code, repetitive setup, unclear names

### Helper Functions
Must be genuinely reusable (used in multiple places).

### Pointer-Based Expectations
Use nil to skip checks in test structs:
```go
type expectation struct {
    Name  *string  // nil = don't check
    Count *int     // nil = don't check
}
```