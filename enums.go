package main

// DisplayMode represents different output display formats
//
//go:generate enumer -type=DisplayMode -linecomment
type DisplayMode int

const (
	DisplayModeTable              DisplayMode = iota // TABLE
	DisplayModeTableComment                          // TABLE_COMMENT
	DisplayModeTableDetailComment                    // TABLE_DETAIL_COMMENT
	DisplayModeVertical                              // VERTICAL
	DisplayModeTab                                   // TAB
	DisplayModeHTML                                  // HTML
	DisplayModeXML                                   // XML
	DisplayModeCSV                                   // CSV
)

// AutocommitDMLMode represents the DML autocommit behavior
//
//go:generate enumer -type=AutocommitDMLMode -linecomment
type AutocommitDMLMode int

const (
	AutocommitDMLModeTransactional        AutocommitDMLMode = iota // TRANSACTIONAL
	AutocommitDMLModePartitionedNonAtomic                          // PARTITIONED_NON_ATOMIC
)

// parseMode represents statement parsing behavior
//
//go:generate enumer -type=parseMode -linecomment
type parseMode int

const (
	parseModeUnspecified parseMode = iota // UNSPECIFIED
	parseModeFallback                     // FALLBACK
	parseModeNoMemefish                   // NO_MEMEFISH
	parseMemefishOnly                     // MEMEFISH_ONLY
)

// explainFormat represents EXPLAIN output format
//
//go:generate enumer -type=explainFormat -linecomment
type explainFormat int

const (
	explainFormatUnspecified explainFormat = iota //
	explainFormatCurrent                          // CURRENT
	explainFormatTraditional                      // TRADITIONAL
	explainFormatCompact                          // COMPACT
)

// Wrapper functions to maintain compatibility

// parseDisplayMode converts a string format name to DisplayMode.
// It accepts both uppercase and lowercase format names.
// Returns an error if the format name is invalid.
func parseDisplayMode(format string) (DisplayMode, error) {
	return DisplayModeString(format)
}

// parseExplainFormat converts a string to explainFormat
func parseExplainFormat(s string) (explainFormat, error) {
	if s == "" {
		return explainFormatUnspecified, nil
	}
	return explainFormatString(s)
}
