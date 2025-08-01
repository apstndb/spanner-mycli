// Package sysvar provides system variable parsers for spanner-mycli.
// This file contains predefined parsers and examples.
//
// Most parsers are created inline using builder functions from builder.go.
// For examples of creating parsers, see:
// - CreateProtobufEnumVariableParserWithAutoFormatter for protobuf enums with prefix stripping
// - CreateStringEnumVariableParser for string-based enums
// - CreateIntRangeParser for integers with range validation
// - CreateDurationRangeParser for durations with range validation
package sysvar
