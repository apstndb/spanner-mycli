package sysvar

import (
	"fmt"
	"strings"
	"time"
	
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/parser"
)

// DisplayMode represents the output format for query results.
type DisplayMode int

const (
	DisplayModeTable DisplayMode = iota
	DisplayModeTableComment
	DisplayModeTableDetailComment
	DisplayModeVertical
	DisplayModeTab
	DisplayModeHTML
	DisplayModeXML
	DisplayModeCSV
)

// DisplayModeParser parses display mode values.
var DisplayModeParser = parser.NewEnumParser(map[string]DisplayMode{
	"TABLE":               DisplayModeTable,
	"TABLE_COMMENT":       DisplayModeTableComment,
	"TABLE_DETAIL_COMMENT": DisplayModeTableDetailComment,
	"VERTICAL":            DisplayModeVertical,
	"TAB":                 DisplayModeTab,
	"HTML":                DisplayModeHTML,
	"XML":                 DisplayModeXML,
	"CSV":                 DisplayModeCSV,
})

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

// AutocommitDMLMode represents the autocommit mode for DML statements.
type AutocommitDMLMode bool

const (
	AutocommitDMLModeTransactional        AutocommitDMLMode = false
	AutocommitDMLModePartitionedNonAtomic AutocommitDMLMode = true
)

// AutocommitDMLModeParser parses autocommit DML mode values.
var AutocommitDMLModeParser = parser.NewEnumParser(map[string]AutocommitDMLMode{
	"TRANSACTIONAL":         AutocommitDMLModeTransactional,
	"PARTITIONED_NON_ATOMIC": AutocommitDMLModePartitionedNonAtomic,
})

// IsolationLevelParser parses transaction isolation level values.
var IsolationLevelParser = parser.NewEnumParser(map[string]sppb.TransactionOptions_IsolationLevel{
	"ISOLATION_LEVEL_UNSPECIFIED": sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
	"UNSPECIFIED":                 sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
	"SNAPSHOT":                    sppb.TransactionOptions_SNAPSHOT,
	"SERIALIZABLE":                sppb.TransactionOptions_SERIALIZABLE,
})

// QueryModeParser parses query mode values.
var QueryModeParser = parser.NewEnumParser(map[string]sppb.ExecuteSqlRequest_QueryMode{
	"NORMAL": sppb.ExecuteSqlRequest_NORMAL,
	"PLAN":   sppb.ExecuteSqlRequest_PLAN,
	"PROFILE": sppb.ExecuteSqlRequest_PROFILE,
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

// PortParser parses port numbers (1-65535).
var PortParser = parser.NewIntParser().WithRange(1, 65535)

// TabWidthParser parses tab width values (1-100).
var TabWidthParser = parser.NewIntParser().WithRange(1, 100)

// PaginationLimitParser parses pagination limit values (1-100000).
var PaginationLimitParser = parser.NewIntParser().WithRange(1, 100000)

// DurationParser is a standard duration parser.
var DurationParser = parser.NewDurationParser()

// TimeoutParser parses timeout values (0 to 1 hour).
var TimeoutParser = parser.NewDurationParser().WithRange(0, time.Hour)

// CommitDelayParser parses commit delay values (0 to 500ms).
var CommitDelayParser = parser.NewDurationParser().WithRange(0, 500*time.Millisecond)

// StringParser is a standard string parser.
var StringParser = parser.NewStringParser()

// OptionalStringParser parses optional string values (can be NULL).
type OptionalStringParser struct {
	parser.BaseParser[*string]
}

// NewOptionalStringParser creates a parser for optional string values.
func NewOptionalStringParser() *OptionalStringParser {
	p := &OptionalStringParser{}
	p.BaseParser = parser.BaseParser[*string]{
		ParseFunc: func(value string) (*string, error) {
			upper := strings.ToUpper(strings.TrimSpace(value))
			if upper == "NULL" || upper == "" {
				return nil, nil
			}
			// Use the standard string parser
			parsed, err := StringParser.Parse(value)
			if err != nil {
				return nil, err
			}
			return &parsed, nil
		},
	}
	return p
}

// OptionalIntParser parses optional integer values (can be NULL).
type OptionalIntParser struct {
	parser.BaseParser[*int64]
	min *int64
	max *int64
}

// NewOptionalIntParser creates a parser for optional integer values.
func NewOptionalIntParser() *OptionalIntParser {
	p := &OptionalIntParser{}
	p.BaseParser = parser.BaseParser[*int64]{
		ParseFunc: func(value string) (*int64, error) {
			upper := strings.ToUpper(strings.TrimSpace(value))
			if upper == "NULL" || upper == "" {
				return nil, nil
			}
			// Use the standard int parser
			parsed, err := IntParser.Parse(value)
			if err != nil {
				return nil, err
			}
			return &parsed, nil
		},
	}
	return p
}

// WithRange adds range validation to the optional integer parser.
func (p *OptionalIntParser) WithRange(min, max int64) *OptionalIntParser {
	p.min = &min
	p.max = &max
	p.ValidateFunc = func(value *int64) error {
		if value == nil {
			return nil
		}
		if p.min != nil && *value < *p.min {
			return fmt.Errorf("value %d is less than minimum %d", *value, *p.min)
		}
		if p.max != nil && *value > *p.max {
			return fmt.Errorf("value %d is greater than maximum %d", *value, *p.max)
		}
		return nil
	}
	return p
}

// OptionalDurationParser parses optional duration values (can be NULL).
type OptionalDurationParser struct {
	parser.BaseParser[*time.Duration]
	min *time.Duration
	max *time.Duration
}

// NewOptionalDurationParser creates a parser for optional duration values.
func NewOptionalDurationParser() *OptionalDurationParser {
	p := &OptionalDurationParser{}
	p.BaseParser = parser.BaseParser[*time.Duration]{
		ParseFunc: func(value string) (*time.Duration, error) {
			upper := strings.ToUpper(strings.TrimSpace(value))
			if upper == "NULL" || upper == "" {
				return nil, nil
			}
			// Use the standard duration parser
			parsed, err := DurationParser.Parse(value)
			if err != nil {
				return nil, err
			}
			return &parsed, nil
		},
	}
	return p
}

// WithRange adds range validation to the optional duration parser.
func (p *OptionalDurationParser) WithRange(min, max time.Duration) *OptionalDurationParser {
	p.min = &min
	p.max = &max
	p.ValidateFunc = func(value *time.Duration) error {
		if value == nil {
			return nil
		}
		if p.min != nil && *value < *p.min {
			return fmt.Errorf("duration %v is less than minimum %v", *value, *p.min)
		}
		if p.max != nil && *value > *p.max {
			return fmt.Errorf("duration %v is greater than maximum %v", *value, *p.max)
		}
		return nil
	}
	return p
}