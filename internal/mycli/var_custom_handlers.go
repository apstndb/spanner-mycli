package mycli

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/filesafety"
	"github.com/samber/lo"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

var stalenessRe = regexp.MustCompile(`^\(([^:]+)(?:: (.+))?\)$`)

// transactionTagVar is the dedicated TRANSACTION_TAG handler. Get reports the
// applied physical RW tag when one exists, otherwise the next-owner slot.
// Set writes the slot unless a physical RW owner exists.
type transactionTagVar struct {
	sv *systemVariables
}

func (v *transactionTagVar) Get() (string, error) {
	if v.sv == nil {
		return "", fmt.Errorf("variable not initialized")
	}
	if v.sv.transactionTagView != nil {
		return v.sv.transactionTagView(), nil
	}
	return v.sv.Transaction.TransactionTag, nil
}

func (v *transactionTagVar) Set(value string) error {
	if v.sv == nil {
		return fmt.Errorf("variable not initialized")
	}
	if v.sv.setTransactionTagSlot != nil {
		return v.sv.setTransactionTagSlot(value)
	}
	v.sv.Transaction.TransactionTag = value
	return nil
}

// ResetSnapshot returns the writable next-owner slot. RESET capture and
// unchanged-equality use this. Get/SHOW still report the applied RW tag.
func (v *transactionTagVar) ResetSnapshot() (string, error) {
	if v.sv == nil {
		return "", fmt.Errorf("variable not initialized")
	}
	if v.sv.transactionTagSlot != nil {
		return v.sv.transactionTagSlot(), nil
	}
	return v.sv.Transaction.TransactionTag, nil
}

// PrepareReset checks the TRANSACTION_TAG slot can be written without mutating it.
func (v *transactionTagVar) PrepareReset(string) error {
	if v.sv == nil {
		return fmt.Errorf("variable not initialized")
	}
	if v.sv.transactionTagWritable != nil {
		return v.sv.transactionTagWritable()
	}
	return nil
}

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

// PrepareReset parses a timestamp bound without assigning it.
func (t *TimestampBoundVar) PrepareReset(value string) error {
	if value == "" {
		return nil
	}
	_, err := parseTimestampBound(value)
	return err
}

// ProtoDescriptorVar handles PROTO_DESCRIPTORS_FILE_PATH with ADD support
type ProtoDescriptorVar struct {
	filesPtr      *[]string
	descriptorPtr **descriptorpb.FileDescriptorSet
}

func (p *ProtoDescriptorVar) Get() (string, error) {
	return strings.Join(*p.filesPtr, ","), nil
}

func (p *ProtoDescriptorVar) Set(value string) error {
	// SET has no session context. Remote HTTP/GCS fetches still apply the
	// existing 30s bound inside the loader; do not redesign Variable.Set.
	return installProtoDescriptorsFromFilePath(context.Background(), p.filesPtr, p.descriptorPtr, value)
}

func installProtoDescriptorsFromFilePath(ctx context.Context, filesPtr *[]string, descriptorPtr **descriptorpb.FileDescriptorSet, value string) error {
	if value == "" {
		*filesPtr = []string{}
		*descriptorPtr = nil
		return nil
	}

	files := strings.Split(value, ",")
	var fileDescriptorSet *descriptorpb.FileDescriptorSet

	for _, filename := range files {
		filename = strings.TrimSpace(filename)
		fds, err := readFileDescriptorProtoFromFileContext(ctx, filename)
		if err != nil {
			return err
		}
		fileDescriptorSet = mergeFDS(fileDescriptorSet, fds)
	}
	// Individual binary inputs may be fragments of one graph. Validate only
	// after the complete candidate has been assembled, before changing state.
	if _, err := protodesc.NewFiles(fileDescriptorSet); err != nil {
		return fmt.Errorf("invalid proto descriptor set: %w", err)
	}

	*filesPtr = files
	*descriptorPtr = fileDescriptorSet
	return nil
}

// ProtoDescriptorsVar handles PROTO_DESCRIPTORS (base64 FileDescriptorSet).
type ProtoDescriptorsVar struct {
	filesPtr      *[]string
	descriptorPtr **descriptorpb.FileDescriptorSet
}

func (p *ProtoDescriptorsVar) Get() (string, error) {
	if p.descriptorPtr == nil || *p.descriptorPtr == nil {
		return "", nil
	}
	return encodeProtoDescriptors(*p.descriptorPtr)
}

func (p *ProtoDescriptorsVar) Set(value string) error {
	if value == "" {
		*p.filesPtr = nil
		*p.descriptorPtr = nil
		return nil
	}
	raw, err := decodeProtoDescriptorBytes(value)
	if err != nil {
		return err
	}
	fds, err := parseProtoDescriptorsGraph(raw)
	if err != nil {
		return err
	}
	*p.filesPtr = nil
	*p.descriptorPtr = fds
	return nil
}

func (p *ProtoDescriptorVar) Add(value string) error {
	value = strings.TrimSpace(value)

	// Check if already exists
	if lo.Contains(*p.filesPtr, value) {
		return nil
	}

	fds, err := readFileDescriptorProtoFromFileContext(context.Background(), value)
	if err != nil {
		return err
	}

	candidate := mergeFDS(*p.descriptorPtr, fds)
	if _, err := protodesc.NewFiles(candidate); err != nil {
		return fmt.Errorf("invalid proto descriptor set: %w", err)
	}
	*p.filesPtr = append(*p.filesPtr, value)
	*p.descriptorPtr = candidate
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
	b, err := filesafety.SafeReadFile(filename, nil)
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
	parsedPtr   any // Will be type-asserted based on usage
	parseFunc   func(string) error
	prepareFunc func(string) error
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

// PrepareReset validates the template string without writing parsed state.
func (t *TemplateVar) PrepareReset(value string) error {
	if t.prepareFunc != nil {
		return t.prepareFunc(value)
	}
	return errResetUnsupported
}

// AutocommitDMLModeVar handles AUTOCOMMIT_DML_MODE using enumer-generated methods
func AutocommitDMLModeVar(ptr *enums.AutocommitDMLMode) *EnumVar[enums.AutocommitDMLMode] {
	return &EnumVar[enums.AutocommitDMLMode]{
		ptr:    ptr,
		values: enumerValues(enums.AutocommitDMLModeValues()),
	}
}

// autocommitVar is the AUTOCOMMIT handler. Same-value SET/RESET is a no-op
// even with a logical owner or manual batch; a real change reuses
// errSetterInTransaction / errSetterInManualBatch. No SET LOCAL.
type autocommitVar struct {
	sv *systemVariables
}

// AutocommitVar binds AUTOCOMMIT to sv.Transaction.Autocommit.
func AutocommitVar(sv *systemVariables) *autocommitVar {
	return &autocommitVar{sv: sv}
}

func (v *autocommitVar) Get() (string, error) {
	if v.sv == nil {
		return "", fmt.Errorf("variable not initialized")
	}
	return formatBool(v.sv.Transaction.Autocommit), nil
}

func (v *autocommitVar) Set(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if v.sv.Transaction.Autocommit == parsed {
		return nil
	}
	if err := v.rejectToggle(); err != nil {
		return err
	}
	v.sv.Transaction.Autocommit = parsed
	return nil
}

func (v *autocommitVar) PrepareReset(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if v.sv.Transaction.Autocommit == parsed {
		return nil
	}
	return v.rejectToggle()
}

func (v *autocommitVar) ValidValues() []string {
	return []string{"TRUE", "FALSE"}
}

func (v *autocommitVar) rejectToggle() error {
	if v.sv.inTransaction != nil && v.sv.inTransaction() {
		return errSetterInTransaction
	}
	if v.sv.inManualBatch != nil && v.sv.inManualBatch() {
		return errSetterInManualBatch
	}
	return nil
}

// parseLogLevel accepts slog names (DEBUG, INFO, WARN, ERROR), numeric
// offsets, and the WARNING alias. Unknown values must not be applied.
func parseLogLevel(value string) (slog.Level, error) {
	if strings.EqualFold(value, "WARNING") {
		value = "WARN"
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return 0, fmt.Errorf("invalid log level: %s", value)
	}
	return level, nil
}

// LogLevelVar handles CLI_LOG_LEVEL. runtime is nil for isolated fixtures.
type LogLevelVar struct {
	ptr     *slog.Level
	runtime *slog.LevelVar
}

func (l *LogLevelVar) Get() (string, error) {
	return l.ptr.String(), nil
}

func (l *LogLevelVar) Set(value string) error {
	level, err := parseLogLevel(value)
	if err != nil {
		return err
	}
	*l.ptr = level
	if l.runtime != nil {
		l.runtime.Set(level)
	}
	return nil
}

// PrepareReset parses a log level without changing the process threshold.
func (l *LogLevelVar) PrepareReset(value string) error {
	_, err := parseLogLevel(value)
	return err
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
