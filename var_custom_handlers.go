package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/descriptorpb"
)

var stalenessRe = regexp.MustCompile(`^\(([^:]+)(?:: (.+))?\)$`)

// formatTimestampBound formats a TimestampBound for display
func formatTimestampBound(tb *spanner.TimestampBound) string {
	if tb == nil {
		return ""
	}

	s := tb.String()
	matches := stalenessRe.FindStringSubmatch(s)
	if matches == nil {
		return s
	}

	switch matches[1] {
	case "strong":
		return "STRONG"
	case "exactStaleness":
		return fmt.Sprintf("EXACT_STALENESS %v", matches[2])
	case "maxStaleness":
		return fmt.Sprintf("MAX_STALENESS %v", matches[2])
	case "readTimestamp":
		ts, err := parseTimeString(matches[2])
		if err != nil {
			return s
		}
		return fmt.Sprintf("READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))
	case "minReadTimestamp":
		ts, err := parseTimeString(matches[2])
		if err != nil {
			return s
		}
		return fmt.Sprintf("MIN_READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))
	default:
		return s
	}
}

// TimestampBoundVar handles READ_ONLY_STALENESS variable
type TimestampBoundVar struct {
	ptr         **spanner.TimestampBound
	description string
}

func (t *TimestampBoundVar) Get() (string, error) {
	if *t.ptr == nil {
		return "", nil
	}
	return formatTimestampBound(*t.ptr), nil
}

func (t *TimestampBoundVar) Set(value string) error {
	if value == "" {
		*t.ptr = nil
		return nil
	}

	staleness, err := parseTimestampBound(value)
	if err != nil {
		return err
	}
	*t.ptr = &staleness
	return nil
}

func (t *TimestampBoundVar) Description() string {
	return t.description
}

func (t *TimestampBoundVar) IsReadOnly() bool {
	return false
}

// ProtoDescriptorVar handles CLI_PROTO_DESCRIPTOR_FILE with ADD support
type ProtoDescriptorVar struct {
	filesPtr      *[]string
	descriptorPtr **descriptorpb.FileDescriptorSet
	description   string
}

func (p *ProtoDescriptorVar) Get() (string, error) {
	return strings.Join(*p.filesPtr, ","), nil
}

func (p *ProtoDescriptorVar) Set(value string) error {
	if value == "" {
		*p.filesPtr = []string{}
		*p.descriptorPtr = nil
		return nil
	}

	files := strings.Split(value, ",")
	var fileDescriptorSet *descriptorpb.FileDescriptorSet

	for _, filename := range files {
		filename = strings.TrimSpace(filename)
		fds, err := readFileDescriptorProtoFromFile(filename)
		if err != nil {
			return err
		}
		fileDescriptorSet = mergeFDS(fileDescriptorSet, fds)
	}

	*p.filesPtr = files
	*p.descriptorPtr = fileDescriptorSet
	return nil
}

func (p *ProtoDescriptorVar) Add(value string) error {
	value = strings.TrimSpace(value)

	// Check if already exists
	if lo.Contains(*p.filesPtr, value) {
		return nil
	}

	fds, err := readFileDescriptorProtoFromFile(value)
	if err != nil {
		return err
	}

	*p.filesPtr = append(*p.filesPtr, value)
	*p.descriptorPtr = mergeFDS(*p.descriptorPtr, fds)
	return nil
}

func (p *ProtoDescriptorVar) Description() string {
	return p.description
}

func (p *ProtoDescriptorVar) IsReadOnly() bool {
	return false
}

// EndpointVar handles CLI_ENDPOINT (host:port)
type EndpointVar struct {
	hostPtr     *string
	portPtr     *int
	description string
}

func (e *EndpointVar) Get() (string, error) {
	if *e.hostPtr == "" || *e.portPtr == 0 {
		return "", nil
	}
	return net.JoinHostPort(*e.hostPtr, strconv.Itoa(*e.portPtr)), nil
}

func (e *EndpointVar) Set(value string) error {
	if value == "" {
		*e.hostPtr = ""
		*e.portPtr = 0
		return nil
	}

	host, port, err := parseEndpoint(value)
	if err != nil {
		return err
	}
	*e.hostPtr = host
	*e.portPtr = port
	return nil
}

func (e *EndpointVar) Description() string {
	return e.description
}

func (e *EndpointVar) IsReadOnly() bool {
	return false
}

// parseOutputTemplate parses output template file
func parseOutputTemplate(filename string) (*template.Template, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("").Funcs(sproutFuncMap()).Parse(string(b))
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}

// parseAnalyzeColumns parses analyze columns definition
func parseAnalyzeColumns(value string) ([]columnRenderDef, error) {
	return customListToTableRenderDefs(value)
}

// parseInlineStats parses inline stats definition
func parseInlineStats(value string) ([]inlineStatsDef, error) {
	return parseInlineStatsDefs(value)
}

// TemplateVar handles template variables like CLI_ANALYZE_COLUMNS
type TemplateVar struct {
	stringPtr   *string
	parsedPtr   interface{} // Will be type-asserted based on usage
	parseFunc   func(string) error
	description string
}

func (t *TemplateVar) Get() (string, error) {
	return *t.stringPtr, nil
}

func (t *TemplateVar) Set(value string) error {
	if t.parseFunc != nil {
		if err := t.parseFunc(value); err != nil {
			return err
		}
	}
	*t.stringPtr = value
	return nil
}

func (t *TemplateVar) Description() string {
	return t.description
}

func (t *TemplateVar) IsReadOnly() bool {
	return false
}

// AutocommitDMLModeVar handles AUTOCOMMIT_DML_MODE using enumer-generated methods
func AutocommitDMLModeVar(ptr *enums.AutocommitDMLMode, desc string) *EnumVar[enums.AutocommitDMLMode] {
	return &EnumVar[enums.AutocommitDMLMode]{
		ptr:         ptr,
		values:      enumerValues(enums.AutocommitDMLModeValues()),
		description: desc,
	}
}

// LogLevelVar handles CLI_LOG_LEVEL
type LogLevelVar struct {
	ptr         *slog.Level
	description string
}

func (l *LogLevelVar) Get() (string, error) {
	return l.ptr.String(), nil
}

func (l *LogLevelVar) Set(value string) error {
	// Special handling for "WARNING" alias which slog doesn't recognize
	if strings.EqualFold(value, "WARNING") {
		*l.ptr = slog.LevelWarn
		return nil
	}

	// Use slog.Level's built-in UnmarshalText for everything else
	// This handles: DEBUG, INFO, WARN, ERROR (case-insensitive)
	// and numeric offsets like "DEBUG+4", "INFO-8"
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return fmt.Errorf("invalid log level: %s", value)
	}
	*l.ptr = level
	return nil
}

func (l *LogLevelVar) Description() string {
	return l.description
}

func (l *LogLevelVar) IsReadOnly() bool {
	return false
}

// UnimplementedVar handles unimplemented variables
type UnimplementedVar struct {
	name        string
	description string
}

func (u *UnimplementedVar) Get() (string, error) {
	return "", errGetterUnimplemented{u.name}
}

func (u *UnimplementedVar) Set(value string) error {
	return errSetterUnimplemented{u.name}
}

func (u *UnimplementedVar) Description() string {
	return u.description
}

func (u *UnimplementedVar) IsReadOnly() bool {
	return false
}

// TimestampVar handles timestamp formatting for read-only timestamp variables
type TimestampVar struct {
	ptr         *time.Time
	description string
}

func (t *TimestampVar) Get() (string, error) {
	if t.ptr == nil {
		return "", fmt.Errorf("invalid state: TimestampVar ptr is nil")
	}
	return formatTimestamp(*t.ptr, ""), nil
}

func (t *TimestampVar) Set(value string) error {
	return errSetterReadOnly
}

func (t *TimestampVar) Description() string {
	return t.description
}

func (t *TimestampVar) IsReadOnly() bool {
	return true
}

// IntGetterVar handles integer variables with custom getters
type IntGetterVar struct {
	getter      func() int64
	description string
}

func (i *IntGetterVar) Get() (string, error) {
	return strconv.FormatInt(i.getter(), 10), nil
}

func (i *IntGetterVar) Set(value string) error {
	return errSetterReadOnly
}

func (i *IntGetterVar) Description() string {
	return i.description
}

func (i *IntGetterVar) IsReadOnly() bool {
	return true
}
