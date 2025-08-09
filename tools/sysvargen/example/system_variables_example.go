//go:build example
// +build example

// Example of system variables with sysvar tags for code generation.
package main

import (
	"fmt"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// Mock types for the example
type (
	DisplayMode int
	parseMode   string
)

// systemVariables demonstrates how sysvar tags can be used to generate registration code.
// This is a proof of concept - in real implementation, these tags would be added to the actual systemVariables struct.
type systemVariables struct {
	// Boolean variables
	ReadOnly  bool `sysvar:"name=READONLY,desc='A boolean indicating whether or not the connection is in read-only mode',setter=setReadOnly"`
	AutoBatch bool `sysvar:"name=AUTO_BATCH_DML,desc='A boolean indicating whether the DML is executed immediately or begins a batch DML'"`
	Verbose   bool `sysvar:"name=CLI_VERBOSE,desc='Display verbose output'"`

	// String variables
	Project     string `sysvar:"name=CLI_PROJECT,desc='GCP Project ID',readonly"`
	Instance    string `sysvar:"name=CLI_INSTANCE,desc='Cloud Spanner instance ID',readonly"`
	Database    string `sysvar:"name=CLI_DATABASE,desc='Cloud Spanner database ID',readonly"`
	Prompt      string `sysvar:"name=CLI_PROMPT,desc='Custom prompt for spanner-mycli'"`
	HistoryFile string `sysvar:"name=CLI_HISTORY_FILE,desc='Path to the history file',readonly"`

	// Integer variables
	TabWidth         int64 `sysvar:"name=CLI_TAB_WIDTH,desc='Tab width for expanding tabs'"`
	ExplainWrapWidth int64 `sysvar:"name=CLI_EXPLAIN_WRAP_WIDTH,desc='Controls query plan wrap width'"`

	// Nullable types (would need special handling)
	StatementTimeout *time.Duration `sysvar:"name=STATEMENT_TIMEOUT,desc='Timeout value for statements',type=nullable_duration"`
	FixedWidth       *int64         `sysvar:"name=CLI_FIXED_WIDTH,desc='Fixed output width',type=nullable_int"`

	// Enum types (would need enum values specified)
	CLIFormat DisplayMode `sysvar:"name=CLI_FORMAT,desc='Controls output format',type=enum,enum='TABLE,VERTICAL,TAB,HTML,XML,CSV'"`
	ParseMode parseMode   `sysvar:"name=CLI_PARSE_MODE,desc='Controls statement parsing mode',type=enum,enum='FALLBACK,NO_MEMEFISH,MEMEFISH_ONLY'"`

	// Proto enum types
	RPCPriority sppb.RequestOptions_Priority `sysvar:"name=RPC_PRIORITY,desc='Request priority',type=proto_enum,proto='sppb.RequestOptions_Priority',prefix=PRIORITY_"`

	// Fields without sysvar tags are ignored
	InternalField string

	// Fields with sysvar:"-" are explicitly ignored
	IgnoredField bool `sysvar:"-"`
}

// Custom setter example
func (sv *systemVariables) setReadOnly(v bool) error {
	// Custom validation logic
	if sv.InTransaction() {
		return fmt.Errorf("cannot change READONLY during transaction")
	}
	sv.ReadOnly = v
	return nil
}

// Mock method for example
func (sv *systemVariables) InTransaction() bool {
	return false
}
