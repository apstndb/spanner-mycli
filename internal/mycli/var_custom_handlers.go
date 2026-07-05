package mycli

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
	ptr **spanner.TimestampBound
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

// ProtoDescriptorVar handles CLI_PROTO_DESCRIPTOR_FILE with ADD support
type ProtoDescriptorVar struct {
	filesPtr      *[]string
	descriptorPtr **descriptorpb.FileDescriptorSet
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

// EndpointVar handles CLI_ENDPOINT (host:port).
// Read-only, like CLI_HOST and CLI_PORT which it is derived from: the
// endpoint is part of the immutable StartupConfig, and changing it after
// startup would not reconnect the live session.
type EndpointVar struct {
	hostPtr *string
	portPtr *int
}

func (e *EndpointVar) Get() (string, error) {
	if *e.hostPtr == "" || *e.portPtr == 0 {
		return "", nil
	}
	return net.JoinHostPort(*e.hostPtr, strconv.Itoa(*e.portPtr)), nil
}

func (e *EndpointVar) Set(value string) error {
	return errSetterReadOnly
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
	stringPtr *string
	parsedPtr interface{} // Will be type-asserted based on usage
	parseFunc func(string) error
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

// AutocommitDMLModeVar handles AUTOCOMMIT_DML_MODE using enumer-generated methods
func AutocommitDMLModeVar(ptr *enums.AutocommitDMLMode) *EnumVar[enums.AutocommitDMLMode] {
	return &EnumVar[enums.AutocommitDMLMode]{
		ptr:    ptr,
		values: enumerValues(enums.AutocommitDMLModeValues()),
	}
}

// LogLevelVar handles CLI_LOG_LEVEL
type LogLevelVar struct {
	ptr *slog.Level
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

// ValidValues returns the standard log level names as GoogleSQL string literals.
func (l *LogLevelVar) ValidValues() []string {
	return []string{"'DEBUG'", "'ERROR'", "'INFO'", "'WARN'", "'WARNING'"}
}

// UnimplementedVar handles unimplemented variables
type UnimplementedVar struct {
	name string
}

func (u *UnimplementedVar) Get() (string, error) {
	return "", errGetterUnimplemented{u.name}
}

func (u *UnimplementedVar) Set(value string) error {
	return errSetterUnimplemented{u.name}
}

// commitResponseVar is the registry handler for COMMIT_RESPONSE, the one
// genuinely multi-valued system variable. It reports the last read-write
// transaction's commit result as two columns (COMMIT_TIMESTAMP, MUTATION_COUNT)
// via GetMulti; a plain Get is unavailable, mirroring java-spanner where
// COMMIT_RESPONSE cannot be read as a single value.
type commitResponseVar struct {
	sv *systemVariables
}

// Get always reports the value as unavailable so COMMIT_RESPONSE stays out of
// the flat ListVariables()/SHOW VARIABLES rows; its columns are merged in
// separately from GetMulti. errIgnored is the established "skip me" sentinel.
func (c *commitResponseVar) Get() (string, error) {
	return "", errIgnored
}

// Set is never reached through Registry.Set (scopeResult is read-only, rejected
// centrally before the handler), but is implemented for completeness.
func (c *commitResponseVar) Set(string) error {
	return errSetterReadOnly
}

// GetMulti returns COMMIT_TIMESTAMP and MUTATION_COUNT from the last commit, or
// errIgnored when no read-write transaction has committed yet.
func (c *commitResponseVar) GetMulti() (map[string]string, error) {
	if c.sv.LastResult.CommitResponse == nil {
		return nil, errIgnored
	}
	return map[string]string{
		"COMMIT_TIMESTAMP": formatTimestamp(c.sv.LastResult.CommitTimestamp, "NULL"),
		"MUTATION_COUNT":   strconv.FormatInt(c.sv.LastResult.CommitResponse.GetCommitStats().GetMutationCount(), 10),
	}, nil
}

// TimestampVar handles timestamp formatting for read-only timestamp variables
type TimestampVar struct {
	ptr *time.Time
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

// IntGetterVar handles integer variables with custom getters
type IntGetterVar struct {
	getter func() int64
}

func (i *IntGetterVar) Get() (string, error) {
	return strconv.FormatInt(i.getter(), 10), nil
}

func (i *IntGetterVar) Set(value string) error {
	return errSetterReadOnly
}
