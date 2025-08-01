// Package parser provides generic parsing infrastructure for spanner-mycli.
//
// This package implements type-safe parsers using Go generics, supporting
// different parsing modes and validation rules. It serves as the foundation
// for the sysvar package but can also be used for other parsing needs.
//
// # Design Philosophy
//
// The parser package emphasizes type safety and composability through generics.
// Rather than providing numerous concrete implementations, it offers building
// blocks that can be composed to create specific parsers.
//
// # Core Interfaces
//
//   - Parser[T]: Basic parsing interface for any type T
//   - DualModeParser[T]: Parser supporting both GoogleSQL and Simple modes
//   - Various concrete parsers for common types (bool, int, string, duration)
//
// # Parsing Modes
//
// The package supports two parsing modes:
//   - GoogleSQL mode: For values from SQL statements (e.g., SET variable = 'value')
//   - Simple mode: For values from CLI flags or config files
//
// This distinction is important because GoogleSQL requires proper quoting for
// strings while Simple mode accepts values as-is.
package parser