package mycli

import (
	"bytes"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/stretchr/testify/assert"
)

func TestStreamingTab(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// Create streaming processor for TAB
	sysVars := &systemVariables{
		Display: DisplayVars{SkipColumnNames: false},
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
		toRow("1", "Alice"),
		toRow("2", "Bob"),
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
	err = processor.ProcessRow(toRow("1", "Alice"))
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

func TestStreamingHTML(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// Create streaming processor for HTML
	sysVars := &systemVariables{
		Display: DisplayVars{SkipColumnNames: false},
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
	err = processor.ProcessRow(toRow("1", "Alice & Bob <test>"))
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
		Display: DisplayVars{SkipColumnNames: false},
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
	err = processor.ProcessRow(toRow("1", "Alice"))
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
