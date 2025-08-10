package enums

// DisplayMode represents different output display formats
//
//go:generate enumer -type=DisplayMode -trimprefix=DisplayMode -transform=snake_upper
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
)

// AutocommitDMLMode represents the DML autocommit behavior
//
//go:generate enumer -type=AutocommitDMLMode -trimprefix=AutocommitDMLMode -transform=snake_upper
type AutocommitDMLMode int

const (
	AutocommitDMLModeTransactional AutocommitDMLMode = iota
	AutocommitDMLModePartitionedNonAtomic
)

// ParseMode represents statement parsing behavior
//
//go:generate enumer -type=ParseMode -trimprefix=ParseMode -transform=snake_upper
type ParseMode int

const (
	ParseModeUnspecified ParseMode = iota
	ParseModeFallback
	ParseModeNoMemefish
	ParseModeMemefishOnly
)

// ExplainFormat represents EXPLAIN output format
//
//go:generate enumer -type=ExplainFormat -trimprefix=ExplainFormat -transform=snake_upper
type ExplainFormat int

const (
	ExplainFormatUnspecified ExplainFormat = iota
	ExplainFormatCurrent
	ExplainFormatTraditional
	ExplainFormatCompact
)

// StreamingMode represents the streaming output mode.
//
//go:generate enumer -type=StreamingMode -trimprefix=StreamingMode -transform=snake_upper
type StreamingMode int

const (
	StreamingModeAuto  StreamingMode = iota // Smart default based on format
	StreamingModeTrue                       // Always stream
	StreamingModeFalse                      // Never stream
)
