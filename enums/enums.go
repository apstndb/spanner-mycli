package enums

// DisplayMode represents different output display formats
//
//go:generate go tool enumer -type=DisplayMode -trimprefix=DisplayMode -transform=snake_upper
type DisplayMode int

const (
	DisplayModeUnspecified DisplayMode = iota
	DisplayModeTable
	DisplayModeTableComment
	DisplayModeTableDetailComment
	DisplayModeVertical
	DisplayModeTab
	DisplayModeHTML
	DisplayModeXML
	DisplayModeCSV
	DisplayModeSQLInsert
	DisplayModeSQLInsertOrIgnore
	DisplayModeSQLInsertOrUpdate
)

// AutocommitDMLMode represents the DML autocommit behavior
//
//go:generate go tool enumer -type=AutocommitDMLMode -trimprefix=AutocommitDMLMode -transform=snake_upper
type AutocommitDMLMode int

const (
	AutocommitDMLModeTransactional AutocommitDMLMode = iota
	AutocommitDMLModePartitionedNonAtomic
)

// ParseMode represents statement parsing behavior
//
//go:generate go tool enumer -type=ParseMode -trimprefix=ParseMode -transform=snake_upper
type ParseMode int

const (
	ParseModeUnspecified ParseMode = iota
	ParseModeFallback
	ParseModeNoMemefish
	ParseModeMemefishOnly
)

// ExplainFormat represents EXPLAIN output format
//
//go:generate go tool enumer -type=ExplainFormat -trimprefix=ExplainFormat -transform=snake_upper
type ExplainFormat int

const (
	ExplainFormatUnspecified ExplainFormat = iota
	ExplainFormatCurrent
	ExplainFormatTraditional
	ExplainFormatCompact
)

// StreamingMode represents the streaming output mode.
//
//go:generate go tool enumer -type=StreamingMode -trimprefix=StreamingMode -transform=snake_upper
type StreamingMode int

const (
	StreamingModeAuto  StreamingMode = iota // Smart default based on format
	StreamingModeTrue                       // Always stream
	StreamingModeFalse                      // Never stream
)

// IsSQLExport returns true if the display mode is one of the SQL export formats
func (d DisplayMode) IsSQLExport() bool {
	return d == DisplayModeSQLInsert || d == DisplayModeSQLInsertOrUpdate || d == DisplayModeSQLInsertOrIgnore
}
