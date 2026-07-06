# Testing Best Practices

## Test Tiers and Conventions

The suite has two tiers, selected with the standard `-short` flag:

- `go test -short ./...` - unit tests only; no Docker required; runs in a few
  seconds. Integration tests self-skip via `testing.Short()`.
- `make test` (`go test ./...`) - additionally runs integration tests against
  a shared Cloud Spanner emulator (spanemuboost lazy runtime; the container
  boots once per process and each test gets an isolated random database via
  `initializeWithRandomDB`).

Conventions:

- Gate emulator-dependent tests with `testing.Short()` (usually via the
  `runStatementTests` helper). This is the ONLY tier-selection mechanism:
  `testing.Short()` is checked at runtime, so every test compiles and runs in
  the default `go test`/`make test`, and self-skips under `-short`. Do NOT
  introduce build tags (such as the former `integration` / `skip_slow_test`
  tags) to gate tests; tag-gated files silently rot because no `make` target
  or CI job ever runs them.
- For statement-level integration tests, prefer the `statementTestCase` /
  `runStatementTests` table in integration_test.go with the `sr` / `srEmpty` /
  `srKeep` / `srDML` result helpers. `compareResult` already ignores
  `ReadTimestamp`, `CommitTimestamp`, `CommitStats`, `Stats`, and `Metrics`,
  so expectations stay small.
- Logic that can be exercised without RPCs should be a unit test even if the
  feature is transactional: pending transactions (`BEGIN` before the first
  statement) perform no RPCs, which is how statements_set_local_test.go tests
  transaction-scoped variables under `-short`.

## Test Style

- Standard `testing` package plus `github.com/google/go-cmp` for comparisons
  is the primary style. Report diffs as
  `t.Errorf("mismatch (-want +got):\n%s", diff)`.
- A handful of test files use `github.com/stretchr/testify`
  (`assert`/`require`), so it is a direct dependency in go.mod. Prefer
  std `testing` + go-cmp for new tests; within a file that already uses
  testify, matching its existing style is fine.
- Table-driven tests with `t.Run()` subtests and descriptive case names.
- Naming: `Test<Function>`, `Test<Type>_<Method>`, or `Test<Type>_<Scenario>`.
- Mark helpers with `t.Helper()` so failures point at the caller.
- Use `t.Fatal` when the test cannot continue (setup failures); use `t.Error`
  to collect multiple independent failures.

## Test Isolation

Write tests as if they will run under `t.Parallel()` even when they do not:

- Always `t.Setenv()`, never `os.Setenv()`.
- Never `os.Chdir()`. For config-file behavior, `parseFlagsArgs` accepts
  explicit config file paths, so tests can point at a file in `t.TempDir()`
  without touching process state.
- Use `t.TempDir()` for files and `t.Cleanup()` for other resources.

## Flag and Startup Testing

Startup validation happens in three stages, and tests must target the right
one (see main_flags_test.go for the harness):

1. kong parsing (`parseFlags` / `parseFlagsArgs`) - syntax errors.
2. `ValidateSpannerOptions` - business rules such as mutual exclusivity.
3. `initializeSystemVariables` - value format and type conversion errors.

Behavior that depends on `term.IsTerminal()` needs a real PTY; use the
`pty.Open()` helpers in main_flags_test.go to simulate interactive stdin.

## Testing Unmockable Spanner Types

Spanner transaction types have unexported fields and cannot be mocked. Use
two tiers: unit tests for error paths and locking behavior (no real
transaction needed), and emulator integration tests for actual behavior. See
transaction_manager_test.go and
session_transaction_helpers_integration_test.go.

## Coverage Analysis

- `make test-coverage` writes `tmp/coverage.out` / `tmp/coverage.html`
  (excluding enumer-generated files) and prints the total;
  `make test-coverage-open` opens the HTML report. On PRs, octocov comments
  with the coverage delta.
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
