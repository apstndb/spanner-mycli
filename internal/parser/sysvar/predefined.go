package sysvar

import (
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/parser"
)

// PriorityParser parses RPC priority values.
var PriorityParser = parser.NewEnumParser(map[string]sppb.RequestOptions_Priority{
	"PRIORITY_UNSPECIFIED": sppb.RequestOptions_PRIORITY_UNSPECIFIED,
	"UNSPECIFIED":          sppb.RequestOptions_PRIORITY_UNSPECIFIED,
	"PRIORITY_LOW":         sppb.RequestOptions_PRIORITY_LOW,
	"LOW":                  sppb.RequestOptions_PRIORITY_LOW,
	"PRIORITY_MEDIUM":      sppb.RequestOptions_PRIORITY_MEDIUM,
	"MEDIUM":               sppb.RequestOptions_PRIORITY_MEDIUM,
	"PRIORITY_HIGH":        sppb.RequestOptions_PRIORITY_HIGH,
	"HIGH":                 sppb.RequestOptions_PRIORITY_HIGH,
})

// IsolationLevelParser parses transaction isolation level values.
var IsolationLevelParser = parser.NewEnumParser(map[string]sppb.TransactionOptions_IsolationLevel{
	"ISOLATION_LEVEL_UNSPECIFIED": sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
	"UNSPECIFIED":                 sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
	"SERIALIZABLE":                sppb.TransactionOptions_SERIALIZABLE,
	"REPEATABLE_READ":             sppb.TransactionOptions_REPEATABLE_READ,
})

// QueryModeParser parses query mode values.
var QueryModeParser = parser.NewEnumParser(map[string]sppb.ExecuteSqlRequest_QueryMode{
	"NORMAL":              sppb.ExecuteSqlRequest_NORMAL,
	"PLAN":                sppb.ExecuteSqlRequest_PLAN,
	"PROFILE":             sppb.ExecuteSqlRequest_PROFILE,
	"WITH_PLAN_AND_STATS": sppb.ExecuteSqlRequest_PROFILE, // Alias for compatibility
})

// ExplainFormat represents the format for EXPLAIN output.
type ExplainFormat string

const (
	ExplainFormatCurrent     ExplainFormat = "CURRENT"
	ExplainFormatTraditional ExplainFormat = "TRADITIONAL"
	ExplainFormatCompact     ExplainFormat = "COMPACT"
)

// ExplainFormatParser parses explain format values.
var ExplainFormatParser = parser.NewEnumParser(map[string]ExplainFormat{
	"CURRENT":     ExplainFormatCurrent,
	"TRADITIONAL": ExplainFormatTraditional,
	"COMPACT":     ExplainFormatCompact,
})

// Common parser instances with validation

// BoolParser is a standard boolean parser.
var BoolParser = parser.NewBoolParser()

// IntParser is a standard integer parser.
var IntParser = parser.NewIntParser()

// PositiveIntParser parses positive integers.
var PositiveIntParser = parser.NewIntParser().WithMin(1)

// PortParser parses port numbers (0-65535).
var PortParser = parser.NewIntParser().WithRange(0, 65535)

// TabWidthParser parses tab width values (1-100).
var TabWidthParser = parser.NewIntParser().WithRange(1, 100)

// PaginationLimitParser parses pagination limit values (1-100000).
var PaginationLimitParser = parser.NewIntParser().WithRange(1, 100000)

// DurationParser is a standard duration parser.
var DurationParser = parser.NewDurationParser()

// TimeoutParser parses timeout values (0 or positive).
var TimeoutParser = parser.NewDurationParser().WithMin(0)

// CommitDelayParser parses commit delay values (0 to 500ms).
var CommitDelayParser = parser.NewDurationParser().WithRange(0, 500*time.Millisecond)

// StringParser is a standard string parser.
var StringParser = parser.NewStringParser()

// OptionalStringParser parses optional string values (can be NULL).
var OptionalStringParser = parser.NewOptionalParser(StringParser)

// OptionalIntParser creates a parser for optional integer values.
func NewOptionalIntParser() parser.Parser[*int64] {
	return parser.NewOptionalParser(IntParser)
}

// NewOptionalIntParserWithRange creates a parser for optional integer values with range validation.
func NewOptionalIntParserWithRange(min, max int64) parser.Parser[*int64] {
	return parser.WithRangeValidation(parser.NewOptionalParser(IntParser), min, max)
}

// NewOptionalDurationParser creates a parser for optional duration values.
func NewOptionalDurationParser() parser.Parser[*time.Duration] {
	return parser.NewOptionalParser(DurationParser)
}

// NewOptionalDurationParserWithRange creates a parser for optional duration values with range validation.
func NewOptionalDurationParserWithRange(min, max time.Duration) parser.Parser[*time.Duration] {
	return parser.WithRangeValidation(parser.NewOptionalParser(DurationParser), min, max)
}
