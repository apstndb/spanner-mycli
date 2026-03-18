package format

import (
	"cmp"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/apstndb/go-tabwrap"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

var (
	topLeftRe     = regexp.MustCompile(`^\+`)
	bottomRightRe = regexp.MustCompile(`\+$`)
)

// TableStreamingFormatter provides table output using tablewriter.
// It supports both streaming mode (output as rows arrive) and buffered mode
// (collect all rows, then render). This is the single table rendering engine
// used by both the streaming and buffered code paths.
type TableStreamingFormatter struct {
	out          io.Writer
	config       FormatConfig
	screenWidth  int
	table        *tablewriter.Table
	columns      []string
	widths       []int
	initialized  bool
	previewRows  []Row
	previewSize  int
	rowsBuffered int

	// Unified support fields
	mode       Mode        // ModeTable, ModeTableComment, ModeTableDetailComment
	params     TableParams // verbose headers, column alignment
	streaming  bool        // true: Start/Close (immediate output), false: Render (deferred output)
	commentBuf strings.Builder
	hasRows    bool
}

// NewTableStreamingFormatter creates a streaming table formatter.
// Used by the streaming pipeline where rows are output as they arrive.
// previewSize determines how many rows to buffer for width calculation (0 = headers-only).
// mode specifies the table mode (ModeTable, ModeTableComment, ModeTableDetailComment).
func NewTableStreamingFormatter(out io.Writer, config FormatConfig, screenWidth int, previewSize int, mode Mode) *TableStreamingFormatter {
	return &TableStreamingFormatter{
		out:         out,
		config:      config,
		screenWidth: screenWidth,
		previewSize: previewSize,
		previewRows: []Row{},
		mode:        mode,
		streaming:   true,
	}
}

// NewTableFormatterForBuffered creates a table formatter for buffered rendering.
// Used by the buffered code path where all rows are available before formatting.
// Supports TableParams (verbose headers, column alignment) and all table modes
// (TABLE, TABLE_COMMENT, TABLE_DETAIL_COMMENT).
func NewTableFormatterForBuffered(out io.Writer, config FormatConfig, screenWidth int, mode Mode, params TableParams) *TableStreamingFormatter {
	return &TableStreamingFormatter{
		out:         out,
		config:      config,
		screenWidth: screenWidth,
		previewRows: []Row{},
		mode:        mode,
		params:      params,
		streaming:   false,
	}
}

// InitFormat initializes the table with column information and preview rows for width calculation.
func (f *TableStreamingFormatter) InitFormat(columnNames []string, config FormatConfig, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columnNames
	f.config = config
	f.previewRows = previewRows

	// Use verbose headers for width calculation if available
	headerForWidth := columnNames
	if f.config.Verbose && len(f.params.VerboseHeaders) > 0 {
		headerForWidth = f.params.VerboseHeaders
	}

	// Calculate optimal widths using preview rows
	f.calculateWidths(columnNames, headerForWidth, previewRows)

	// Determine output writer (buffer for comment modes)
	tableOut := f.out
	if f.mode == ModeTableComment || f.mode == ModeTableDetailComment {
		tableOut = &f.commentBuf
	}

	// Build table options
	opts := []tablewriter.Option{
		tablewriter.WithRenderer(
			renderer.NewBlueprint(tw.Rendition{Symbols: tw.NewSymbols(tw.StyleASCII)})),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithTrimSpace(tw.Off),
		tablewriter.WithHeaderAutoFormat(tw.Off),
	}
	if f.streaming {
		opts = append(opts, tablewriter.WithStreaming(tw.StreamConfig{Enable: true}))
	}

	f.table = tablewriter.NewTable(tableOut, opts...).Configure(func(twConfig *tablewriter.Config) {
		if len(f.params.ColumnAlign) > 0 {
			twConfig.Row.Alignment.PerColumn = f.params.ColumnAlign
		}
		twConfig.Row.Formatting.AutoWrap = tw.WrapNone
	})

	// Start streaming if in streaming mode
	if f.streaming {
		if err := f.table.Start(); err != nil {
			return fmt.Errorf("failed to start table streaming: %w", err)
		}
	}

	// Set headers
	if !f.config.SkipColumnNames && len(columnNames) > 0 {
		displayHeaders := columnNames
		if f.config.Verbose && len(f.params.VerboseHeaders) > 0 {
			displayHeaders = f.params.VerboseHeaders
		}
		headers := f.wrapHeaders(displayHeaders)
		f.table.Header(headers)
	}

	f.initialized = true
	return nil
}

// WriteRow writes a single table row.
func (f *TableStreamingFormatter) WriteRow(row Row) error {
	if !f.initialized {
		// Buffer rows until initialization (streaming path with preview)
		f.previewRows = append(f.previewRows, row)
		f.rowsBuffered++

		// Check if we have enough preview rows
		if f.previewSize > 0 && f.rowsBuffered >= f.previewSize {
			// Initialize with buffered rows
			return f.InitFormat(f.columns, f.config, f.previewRows)
		}
		return nil
	}

	return f.writeRowInternal(row)
}

// writeRowInternal writes a row to the table.
func (f *TableStreamingFormatter) writeRowInternal(row Row) error {
	f.hasRows = true
	row = visualizeTabsInRow(row, f.newCondition(), f.config.Styled)
	wrappedRow := f.wrapRow(row)

	// When Styled is true, pass cells as tw.Formatter so tablewriter calls Format() (with ANSI codes).
	// When false, pass plain strings so tablewriter never calls Format().
	var data []any
	if f.config.Styled {
		data = Formatters(wrappedRow)
	} else {
		data = toAnySlice(Texts(wrappedRow))
	}

	if err := f.table.Append(data); err != nil {
		return fmt.Errorf("failed to append row to table: %w", err)
	}
	return nil
}

// FinishFormat completes the table output.
func (f *TableStreamingFormatter) FinishFormat() error {
	// Initialize if not done yet (e.g., fewer rows than preview size)
	if !f.initialized && len(f.columns) > 0 {
		if err := f.InitFormat(f.columns, f.config, f.previewRows); err != nil {
			return err
		}
	}

	if f.table != nil {
		if f.streaming {
			if err := f.table.Close(); err != nil {
				return fmt.Errorf("failed to close table stream: %w", err)
			}
		} else {
			// Non-streaming: only render if there are rows or verbose mode with headers
			forceRender := f.config.Verbose && len(f.columns) > 0
			if forceRender || f.hasRows {
				if err := f.table.Render(); err != nil {
					return fmt.Errorf("failed to render table: %w", err)
				}
			}
		}
	}

	// Handle comment mode post-processing
	if (f.mode == ModeTableComment || f.mode == ModeTableDetailComment) && f.commentBuf.Len() > 0 {
		s := strings.TrimSpace(f.commentBuf.String())
		// Sanitize */ in table content to prevent premature SQL comment closure.
		s = strings.ReplaceAll(s, "*/", "* /")
		s = strings.ReplaceAll(s, "\n", "\n ")
		s = topLeftRe.ReplaceAllLiteralString(s, "/*")

		if f.mode == ModeTableComment {
			s = bottomRightRe.ReplaceAllLiteralString(s, "*/")
		}

		if s != "" {
			if _, err := fmt.Fprintln(f.out, s); err != nil {
				return err
			}
		}
	}

	return nil
}

// newCondition creates a tabwrap.Condition from the formatter's config.
func (f *TableStreamingFormatter) newCondition() *tabwrap.Condition {
	return &tabwrap.Condition{TabWidth: cmp.Or(f.config.TabWidth, 4)}
}

// calculateWidths calculates optimal column widths based on preview rows.
// columns are the plain column names, headersForWidth are the headers used for width calculation
// (may include verbose type information).
func (f *TableStreamingFormatter) calculateWidths(columns []string, headersForWidth []string, previewRows []Row) {
	wc := &widthCalculator{Condition: f.newCondition()}
	f.widths = CalculateWidthWithStrategy(f.config.WidthStrategy, columns, headersForWidth, wc, f.screenWidth, previewRows)
}

// wrapHeaders wraps headers according to calculated widths.
func (f *TableStreamingFormatter) wrapHeaders(headers []string) []string {
	if len(f.widths) == 0 {
		return headers
	}

	rw := f.newCondition()
	return slices.Collect(hiter.Unify(
		rw.Wrap,
		hiter.Pairs(slices.Values(headers), slices.Values(f.widths))))
}

// wrapRow wraps row columns according to calculated widths, preserving cell metadata.
func (f *TableStreamingFormatter) wrapRow(row Row) Row {
	if len(f.widths) == 0 {
		return row
	}

	rw := f.newCondition()
	if f.config.Styled {
		return wrapRowStyled(row, f.widths, rw)
	}
	return wrapRowPreserving(row, f.widths, rw)
}
