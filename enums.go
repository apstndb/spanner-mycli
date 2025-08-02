package main

// DisplayMode represents different output display formats
//
//go:generate enumer -type=DisplayMode -trimprefix=DisplayMode -transform=snake_upper
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

// AutocommitDMLMode represents the DML autocommit behavior
//
//go:generate enumer -type=AutocommitDMLMode -trimprefix=AutocommitDMLMode -transform=snake_upper
type AutocommitDMLMode int

const (
	AutocommitDMLModeTransactional AutocommitDMLMode = iota
	AutocommitDMLModePartitionedNonAtomic
)

// parseMode represents statement parsing behavior
//
//go:generate enumer -type=parseMode -trimprefix=parseMode -transform=snake_upper
type parseMode int

const (
	parseModeUnspecified parseMode = iota
	parseModeFallback
	parseModeNoMemefish
	parseModeMemefishOnly
)

// explainFormat represents EXPLAIN output format
//
//go:generate enumer -type=explainFormat -trimprefix=explainFormat -transform=snake_upper
type explainFormat int

const (
	explainFormatUnspecified explainFormat = iota
	explainFormatCurrent
	explainFormatTraditional
	explainFormatCompact
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
