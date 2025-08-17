# Regex Optimization Patterns

## When to Precompile Regex

### DO Precompile

- **Hot paths**: Functions called frequently (e.g., statement parsing in every command)
- **Static patterns**: Known at compile time and used multiple times

### DON'T Precompile

- **Rare operations**: Code paths executed infrequently (e.g., SHOW CREATE commands)
- **Test code**: Readability > micro-optimization in tests
- **Dynamic patterns**: Runtime-constructed patterns (caching complexity often exceeds benefit)

## Decision Checklist

1. Is this in a hot path (loop or frequently called)?
2. Is the pattern static?
3. Does profiling show it as a bottleneck?

**If any answer is "no" → Don't precompile**

## Examples

```go
// GOOD: Hot path, static pattern
var whitespaceRe = regexp.MustCompile(`\s+`)
func parseStatement(s string) {
    normalized := whitespaceRe.ReplaceAllString(s, " ")
}

// BAD: Rare operation
func showCreate() { 
    // Only called for SHOW CREATE commands
    re := regexp.MustCompile(dynamicPattern)
}

// BAD: Test code
func TestSomething(t *testing.T) {
    // Keep tests readable
    if regexp.MustCompile(`pattern`).MatchString(s) {
    }
}
```