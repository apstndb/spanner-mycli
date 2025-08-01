package parser

import (
	"time"
)

// GoogleSQLDurationParser parses duration from GoogleSQL string literals.
// It extracts the string literal and delegates to the standard duration parser.
var GoogleSQLDurationParser = NewDelegatingGoogleSQLParser(NewDurationParser())

// DualModeDurationParser provides dual-mode parsing for duration values.
var DualModeDurationParser = NewDualModeParser[time.Duration](
	GoogleSQLDurationParser,
	NewDurationParser(),
)

// NullableDualModeDurationParser provides nullable dual-mode parsing for duration values.
var NullableDualModeDurationParser = NewNullableParser(DualModeDurationParser)
