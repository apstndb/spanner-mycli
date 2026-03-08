package format

// Mode represents the output format mode as a string.
// Built-in modes are defined as constants below.
// Custom modes can be registered via RegisterFormatFunc and RegisterStreamingFormatter.
//
// Mode values use UPPER_SNAKE_CASE to match the enumer-generated strings
// of enums.DisplayMode, enabling conversion via format.Mode(dm.String()).
type Mode string

const (
	ModeTable              Mode = "TABLE"
	ModeTableComment       Mode = "TABLE_COMMENT"
	ModeTableDetailComment Mode = "TABLE_DETAIL_COMMENT"
	ModeVertical           Mode = "VERTICAL"
	ModeTab                Mode = "TAB"
	ModeHTML               Mode = "HTML"
	ModeXML                Mode = "XML"
	ModeCSV                Mode = "CSV"
)

// IsTableMode returns true if the mode is one of the table display modes.
func (m Mode) IsTableMode() bool {
	return m == ModeTable || m == ModeTableComment || m == ModeTableDetailComment
}

// ValueFormatMode specifies how row values should be formatted before being
// passed to a formatter. This allows formatters to declare their value formatting
// requirements, decoupling the value formatting decision from mode-name checks
// in execute_sql.go.
type ValueFormatMode int

const (
	// DisplayValues formats values for human display (e.g., NULL as "NULL" text,
	// timestamps in readable format). This is the default for built-in modes.
	DisplayValues ValueFormatMode = iota

	// SQLLiteralValues formats values as valid SQL literals (e.g., NULL keyword,
	// strings quoted, bytes as hex). Used by SQL export modes.
	SQLLiteralValues
)

// ValueFormatModeFor returns the ValueFormatMode declared for the given Mode.
// Built-in modes return DisplayValues. Registered modes return their declared value.
func ValueFormatModeFor(mode Mode) ValueFormatMode {
	if vfm, ok := lookupValueFormatMode(mode); ok {
		return vfm
	}
	return DisplayValues
}
