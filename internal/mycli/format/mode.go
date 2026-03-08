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
