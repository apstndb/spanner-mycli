package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

// Custom setter methods referenced in struct tags

// setReadOnly validates that READONLY can't be changed during an active transaction
func (sv *systemVariables) setReadOnly(v bool) error {
	if sv.CurrentSession != nil && (sv.CurrentSession.InReadOnlyTransaction() || sv.CurrentSession.InReadWriteTransaction()) {
		return errors.New("can't change READONLY when there is a active transaction")
	}
	sv.ReadOnly = v
	return nil
}

// setPrompt2 sets the continuation prompt
func (sv *systemVariables) setPrompt2(v string) error {
	sv.Prompt2 = v
	return nil
}

// setLogLevel sets the log level and reinitializes the logger
func (sv *systemVariables) setLogLevel(v slog.Level) error {
	sv.LogLevel = v
	// Re-initialize the logger with the new level
	h := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: v,
	}))
	slog.SetDefault(h)
	return nil
}

// setEnableADCPlus ensures this variable can only be set before session creation
func (sv *systemVariables) setEnableADCPlus(v bool) error {
	if sv.CurrentSession != nil {
		return fmt.Errorf("CLI_ENABLE_ADC_PLUS cannot be changed after session creation")
	}
	sv.EnableADCPlus = v
	return nil
}

// Custom getter methods referenced in struct tags

// getPortAsInt64 converts the int Port field to int64 for the variable system
func (sv *systemVariables) getPortAsInt64() int64 {
	return int64(sv.Port)
}

// getQueryMode returns the query mode with a default value
func (sv *systemVariables) getQueryMode() sppb.ExecuteSqlRequest_QueryMode {
	if sv.QueryMode == nil {
		return sppb.ExecuteSqlRequest_NORMAL
	}
	return *sv.QueryMode
}

// setQueryMode sets the query mode as a pointer
func (sv *systemVariables) setQueryMode(v sppb.ExecuteSqlRequest_QueryMode) error {
	sv.QueryMode = &v
	return nil
}

// formatReadTimestamp formats ReadTimestamp for display using RFC3339Nano
func (sv *systemVariables) formatReadTimestamp() string {
	return sysvar.FormatTimestamp(&sv.ReadTimestamp)()
}

// formatCommitTimestamp formats CommitTimestamp for display using RFC3339Nano
func (sv *systemVariables) formatCommitTimestamp() string {
	return sysvar.FormatTimestamp(&sv.CommitTimestamp)()
}

// getCLIVersion returns the CLI version
func (sv *systemVariables) getCLIVersion() string {
	return getVersion()
}

// getCLICurrentWidth returns the current terminal width or "NULL" if not connected to a terminal
func (sv *systemVariables) getCLICurrentWidth() string {
	if sv.StreamManager != nil {
		return sv.StreamManager.GetTerminalWidthString()
	}
	return "NULL"
}

// getCLIEndpoint returns the current endpoint configuration
func (sv *systemVariables) getCLIEndpoint() string {
	// Construct endpoint from host and port
	if sv.Host != "" && sv.Port != 0 {
		return net.JoinHostPort(sv.Host, strconv.Itoa(sv.Port))
	}
	return ""
}

// setCLIEndpoint sets the endpoint configuration by parsing host and port
func (sv *systemVariables) setCLIEndpoint(value string) error {
	// Parse endpoint and update host and port
	host, port, err := parseEndpoint(value)
	if err != nil {
		return err
	}
	sv.Host = host
	sv.Port = port
	return nil
}

// getCLIOutputTemplateFile returns the current output template file path
func (sv *systemVariables) getCLIOutputTemplateFile() string {
	if sv.OutputTemplateFile == "" {
		return "unset"
	}
	return sv.OutputTemplateFile
}

// setCLIOutputTemplateFile sets the output template file and creates the template
func (sv *systemVariables) setCLIOutputTemplateFile(value string) error {
	if value == "unset" || value == "" || strings.ToUpper(strings.TrimSpace(value)) == "NULL" {
		sv.OutputTemplateFile = ""
		setDefaultOutputTemplate(sv)
		return nil
	}

	return setOutputTemplateFile(sv, value)
}
