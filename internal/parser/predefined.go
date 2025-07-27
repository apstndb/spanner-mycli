package parser

import (
	"time"
)

// GoogleSQLDurationParser parses duration from GoogleSQL string literals.
var GoogleSQLDurationParser = WithTransform(
	GoogleSQLStringLiteralParser,
	func(s string) (time.Duration, error) {
		return time.ParseDuration(s)
	},
)

// DualModeDurationParser provides dual-mode parsing for duration values.
var DualModeDurationParser = NewDualModeParser[time.Duration](
	GoogleSQLDurationParser,
	NewDurationParser(),
)

// NullableDualModeDurationParser provides nullable dual-mode parsing for duration values.
var NullableDualModeDurationParser = NewNullableParser(DualModeDurationParser)