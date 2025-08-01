// Package sysvar provides a type-safe system variable parsing and validation framework.
//
// This package implements a generics-based approach to handle system variables with
// different types while maintaining type safety and consistent validation across
// different input modes (GoogleSQL and Simple).
//
// # Design Principles
//
// System variables in spanner-mycli should only be registered from system_variables_registry.go
// in the main package. This package provides the building blocks for that registration,
// but does not define any concrete system variables itself.
//
// The package uses Go generics extensively to provide type-safe parsing without
// code duplication. Concrete helper functions (like NewSimpleBooleanParser) have been
// intentionally removed in favor of more generic approaches that encourage proper
// type handling.
//
// # Core Components
//
//   - TypedVariableParser[T]: Generic parser that handles any type T
//   - Registry: Central registry for all system variables
//   - DualModeParser: Supports both GoogleSQL and Simple parsing modes
//   - Various builder functions for creating parsers with specific behaviors
//
// # Usage
//
// System variables should be registered using the generic NewTypedVariableParser
// or specific parser constructors (NewBooleanParser, NewStringParser, etc.) that
// explicitly require getter and setter functions. This ensures proper encapsulation
// and makes the data flow explicit.
package sysvar