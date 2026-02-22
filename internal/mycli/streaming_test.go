package mycli

import (
	"bytes"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/stretchr/testify/assert"
)

func TestStreamingCSV(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		rows     []Row
		sysVars  *systemVariables
		expected string
	}{
		{
			name: "basic CSV with headers",
			rows: []Row{
				{"1", "Alice", "30"},
				{"2", "Bob", "25"},
			},
			sysVars: &systemVariables{
				SkipColumnNames: false,
			},
			expected: "ID,Name,Age\n1,Alice,30\n2,Bob,25\n",
		},
		{
			name: "CSV without headers",
			rows: []Row{
				{"1", "Alice", "30"},
				{"2", "Bob", "25"},
			},
			sysVars: &systemVariables{
				SkipColumnNames: true,
			},
			expected: "1,Alice,30\n2,Bob,25\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Create streaming processor for CSV
			processor := NewStreamingProcessorForMode(enums.DisplayModeCSV, &buf, tt.sysVars, 0)
			assert.NotNil(t, processor)

			// Create mock metadata
			metadata := &sppb.ResultSetMetadata{
				RowType: &sppb.StructType{
					Fields: []*sppb.StructType_Field{
						{Name: "ID"},
						{Name: "Name"},
						{Name: "Age"},
					},
				},
			}

			// Initialize processor
			err := processor.Init(metadata, tt.sysVars)
			assert.NoError(t, err)

			// Process rows
			for _, row := range tt.rows {
				err := processor.ProcessRow(row)
				assert.NoError(t, err)
			}

			// Finish processing
			err = processor.Finish(QueryStats{}, int64(len(tt.rows)))
			assert.NoError(t, err)

			// Check output
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestStreamingTab(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// Create streaming processor for TAB
	sysVars := &systemVariables{
		SkipColumnNames: false,
	}
	processor := NewStreamingProcessorForMode(enums.DisplayModeTab, &buf, sysVars, 0)
	assert.NotNil(t, processor)

	// Create mock metadata
	metadata := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "ID"},
				{Name: "Name"},
			},
		},
	}

	// Initialize processor
	err := processor.Init(metadata, sysVars)
	assert.NoError(t, err)

	// Process rows
	rows := []Row{
		{"1", "Alice"},
		{"2", "Bob"},
	}

	for _, row := range rows {
		err := processor.ProcessRow(row)
		assert.NoError(t, err)
	}

	// Finish processing
	err = processor.Finish(QueryStats{}, int64(len(rows)))
	assert.NoError(t, err)

	// Check output
	expected := "ID\tName\n1\tAlice\n2\tBob\n"
	assert.Equal(t, expected, buf.String())
}

func TestStreamingVertical(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// Create streaming processor for VERTICAL
	sysVars := &systemVariables{}
	processor := NewStreamingProcessorForMode(enums.DisplayModeVertical, &buf, sysVars, 0)
	assert.NotNil(t, processor)

	// Create mock metadata
	metadata := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "ID"},
				{Name: "Name"},
			},
		},
	}

	// Initialize processor
	err := processor.Init(metadata, sysVars)
	assert.NoError(t, err)

	// Process one row
	err = processor.ProcessRow(Row{"1", "Alice"})
	assert.NoError(t, err)

	// Finish processing
	err = processor.Finish(QueryStats{}, 1)
	assert.NoError(t, err)

	// Check output contains expected patterns
	output := buf.String()
	assert.Contains(t, output, "1. row")
	assert.Contains(t, output, "ID: 1")
	assert.Contains(t, output, "Name: Alice")
}

func TestBufferedVsStreaming(t *testing.T) {
	t.Parallel()
	// Test that buffered and streaming produce identical output for CSV
	rows := []Row{
		{"1", "Alice", "30"},
		{"2", "Bob", "25"},
		{"3", "Charlie", "35"},
	}

	columnNames := []string{"ID", "Name", "Age"}
	sysVars := &systemVariables{
		SkipColumnNames: false,
	}

	// Buffered output
	var bufBuffered bytes.Buffer
	config := sysVars.toFormatConfig()
	csvFormatter, err := format.NewFormatter(enums.DisplayModeCSV)
	assert.NoError(t, err)
	err = csvFormatter(&bufBuffered, rows, columnNames, config, 0)
	assert.NoError(t, err)

	// Streaming output
	var bufStreaming bytes.Buffer
	formatter := format.NewCSVFormatter(&bufStreaming, sysVars.SkipColumnNames)
	err = formatter.InitFormat(columnNames, config, nil)
	assert.NoError(t, err)

	for _, row := range rows {
		err = formatter.WriteRow(row)
		assert.NoError(t, err)
	}

	err = formatter.FinishFormat()
	assert.NoError(t, err)

	// Compare outputs
	assert.Equal(t, bufBuffered.String(), bufStreaming.String(),
		"Buffered and streaming outputs should be identical")
}

func TestStreamingHTML(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// Create streaming processor for HTML
	sysVars := &systemVariables{
		SkipColumnNames: false,
	}
	processor := NewStreamingProcessorForMode(enums.DisplayModeHTML, &buf, sysVars, 0)
	assert.NotNil(t, processor)

	// Create mock metadata
	metadata := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "ID"},
				{Name: "Name"},
			},
		},
	}

	// Initialize processor
	err := processor.Init(metadata, sysVars)
	assert.NoError(t, err)

	// Process row with special characters
	err = processor.ProcessRow(Row{"1", "Alice & Bob <test>"})
	assert.NoError(t, err)

	// Finish processing
	err = processor.Finish(QueryStats{}, 1)
	assert.NoError(t, err)

	// Check output
	output := buf.String()
	assert.Contains(t, output, "<TABLE BORDER='1'>")
	assert.Contains(t, output, "<TH>ID</TH>")
	assert.Contains(t, output, "<TH>Name</TH>")
	assert.Contains(t, output, "<TD>1</TD>")
	assert.Contains(t, output, "<TD>Alice &amp; Bob &lt;test&gt;</TD>")
	assert.Contains(t, output, "</TABLE>")
}

func TestStreamingXML(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// Create streaming processor for XML
	sysVars := &systemVariables{
		SkipColumnNames: false,
	}
	processor := NewStreamingProcessorForMode(enums.DisplayModeXML, &buf, sysVars, 0)
	assert.NotNil(t, processor)

	// Create mock metadata
	metadata := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "ID"},
				{Name: "Name"},
			},
		},
	}

	// Initialize processor
	err := processor.Init(metadata, sysVars)
	assert.NoError(t, err)

	// Process row
	err = processor.ProcessRow(Row{"1", "Alice"})
	assert.NoError(t, err)

	// Finish processing
	err = processor.Finish(QueryStats{}, 1)
	assert.NoError(t, err)

	// Check output
	output := buf.String()
	assert.Contains(t, output, "<?xml version")
	assert.Contains(t, output, "<resultset")
	assert.Contains(t, output, "<header>")
	assert.Contains(t, output, "<field>ID</field>")
	assert.Contains(t, output, "<field>Name</field>")
	assert.Contains(t, output, "<row>")
	assert.Contains(t, output, "</resultset>")
}
