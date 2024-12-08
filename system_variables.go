package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cloudspannerecosystem/memefish/ast"

	"github.com/samber/lo"

	"spheric.cloud/xiter"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type systemVariables struct {
	RPCPriority                 sppb.RequestOptions_Priority
	ReadOnlyStaleness           *spanner.TimestampBound
	ReadTimestamp               time.Time
	OptimizerVersion            string
	OptimizerStatisticsPackage  string
	CommitResponse              *sppb.CommitResponse
	CommitTimestamp             time.Time
	CLIFormat                   DisplayMode
	Project, Instance, Database string
	Verbose                     bool
	Prompt                      string
	Prompt2                     string
	HistoryFile                 string
	Role                        string
	Endpoint                    string
	DirectedRead                *sppb.DirectedReadOptions
	ProtoDescriptorFile         []string
	BuildStatementMode          parseMode
	LogGrpc                     bool
	QueryMode                   *sppb.ExecuteSqlRequest_QueryMode
	LintPlan                    bool
	TransactionTag              string
	RequestTag                  string

	// it is internal variable and hidden from system variable statements
	ProtoDescriptor *descriptorpb.FileDescriptorSet
	Insecure        bool
	Debug           bool
	Params          map[string]ast.Node

	// link to session
	CurrentSession *Session
}

var errIgnored = errors.New("ignored")

type setter = func(this *systemVariables, name, value string) error

type adder = func(this *systemVariables, name, value string) error

type getter = func(this *systemVariables, name string) (map[string]string, error)

type accessor struct {
	Setter setter
	Getter getter
	Adder  adder
}

func (sv *systemVariables) InstancePath() string {
	return instancePath(sv.Project, sv.Instance)
}

func (sv *systemVariables) DatabasePath() string {
	return databasePath(sv.Project, sv.Instance, sv.Database)
}

func (sv *systemVariables) ProjectPath() string {
	return projectPath(sv.Project)
}

func (sv *systemVariables) Set(name string, value string) error {
	upperName := strings.ToUpper(name)
	a, ok := accessorMap[upperName]
	if !ok {
		return fmt.Errorf("unknown variable name: %v", name)
	}
	if a.Setter == nil {
		return fmt.Errorf("setter unimplemented: %v", name)
	}

	return a.Setter(sv, upperName, value)
}

func (sv *systemVariables) Add(name string, value string) error {
	upperName := strings.ToUpper(name)
	a, ok := accessorMap[upperName]
	if !ok {
		return fmt.Errorf("unknown variable name: %v", name)
	}
	if a.Adder == nil {
		return fmt.Errorf("adder unimplemented: %v", name)
	}

	return a.Adder(sv, upperName, value)
}

func (sv *systemVariables) Get(name string) (map[string]string, error) {
	upperName := strings.ToUpper(name)
	a, ok := accessorMap[upperName]
	if !ok {
		return nil, fmt.Errorf("unknown variable name: %v", name)
	}
	if a.Getter == nil {
		return nil, fmt.Errorf("getter unimplemented: %v", name)
	}

	value, err := a.Getter(sv, name)
	if err != nil && !errors.Is(err, errIgnored) {
		return nil, err
	}
	return value, nil
}

func unquoteString(s string) string {
	return strings.Trim(s, `"'`)
}

func sliceOf[V any](vs ...V) []V {
	return vs
}

func singletonMap[K comparable, V any](k K, v V) map[K]V {
	return map[K]V{k: v}
}

func parseTimeString(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", s)
}

var accessorMap = map[string]accessor{
	"AUTOCOMMIT":              {},
	"RETRY_ABORTS_INTERNALLY": {},
	"AUTOCOMMIT_DML_MODE":     {},
	"STATEMENT_TIMEOUT":       {},
	"READ_ONLY_STALENESS": {
		func(this *systemVariables, name, value string) error {
			staleness, err := parseTimestampBound(unquoteString(value))
			if err != nil {
				return err
			}

			this.ReadOnlyStaleness = &staleness
			return nil
		},
		func(this *systemVariables, name string) (map[string]string, error) {
			if this.ReadOnlyStaleness == nil {
				return nil, errIgnored
			}
			s := this.ReadOnlyStaleness.String()
			stalenessRe := regexp.MustCompile(`^\(([^:]+)(?:: (.+))?\)$`)
			matches := stalenessRe.FindStringSubmatch(s)
			if matches == nil {
				return singletonMap(name, s), nil
			}
			switch matches[1] {
			case "strong":
				return singletonMap(name, "STRONG"), nil

			case "exactStaleness":
				return singletonMap(name, fmt.Sprintf("EXACT_STALENESS %v", matches[2])), nil
			case "maxStaleness":
				return singletonMap(name, fmt.Sprintf("MAX_STALENESS %v", matches[2])), nil
			// TODO: re-format timestamp as RFC3339
			case "readTimestamp":
				ts, err := parseTimeString(matches[2])
				if err != nil {
					return nil, err
				}
				return singletonMap(name, fmt.Sprintf("READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))), nil
			case "minReadTimestamp":
				ts, err := parseTimeString(matches[2])
				if err != nil {
					return nil, err
				}
				return singletonMap(name, fmt.Sprintf("MIN_READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))), nil
			default:
				return singletonMap(name, s), nil
			}
		},
		nil,
	},
	"OPTIMIZER_VERSION": stringAccessor(func(sysVars *systemVariables) *string {
		return &sysVars.OptimizerVersion
	}),
	"OPTIMIZER_STATISTICS_PACKAGE": stringAccessor(func(sysVars *systemVariables) *string {
		return &sysVars.OptimizerStatisticsPackage
	}),
	"RETURN_COMMIT_STATS": {},
	"RPC_PRIORITY": {
		Setter: func(this *systemVariables, name, value string) error {
			s := unquoteString(value)

			p, err := parsePriority(s)
			if err != nil {
				return err
			}

			this.RPCPriority = p
			return nil
		},
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, strings.TrimPrefix(this.RPCPriority.String(), "PRIORITY_")), nil
		},
	},
	"TRANSACTION_TAG": {
		Setter: func(this *systemVariables, name, value string) error {
			if this.CurrentSession == nil {
				return errors.New("invalid state: current session is not populated")
			}

			if this.CurrentSession.InReadOnlyTransaction() {
				return errors.New("can't set transaction tag in read-only transaction")
			}

			if this.CurrentSession.InReadWriteTransaction() {
				return errors.New("can't set transaction tag in read-write transaction")
			}

			this.TransactionTag = unquoteString(value)
			return nil
		},
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			if this.TransactionTag == "" {
				return nil, errIgnored
			}

			return singletonMap(name, this.TransactionTag), nil
		},
	},
	"STATEMENT_TAG": {
		Setter: func(this *systemVariables, name, value string) error {
			this.RequestTag = unquoteString(value)
			return nil
		},
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			if this.RequestTag == "" {
				return nil, errIgnored
			}

			return singletonMap(name, this.RequestTag), nil
		},
	},
	"READ_TIMESTAMP": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			if this.ReadTimestamp.IsZero() {
				return nil, errIgnored
			}
			return singletonMap(name, this.ReadTimestamp.Format(time.RFC3339Nano)), nil
		},
	},
	"COMMIT_TIMESTAMP": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			if this.CommitTimestamp.IsZero() {
				return nil, errIgnored
			}
			s := this.CommitTimestamp.Format(time.RFC3339Nano)
			return singletonMap(name, s), nil
		},
	},
	"COMMIT_RESPONSE": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			if this.CommitResponse == nil {
				return nil, errIgnored
			}
			mutationCount := this.CommitResponse.GetCommitStats().GetMutationCount()
			return map[string]string{
				"COMMIT_TIMESTAMP": this.CommitTimestamp.Format(time.RFC3339Nano),
				"MUTATION_COUNT":   strconv.FormatInt(mutationCount, 10),
			}, nil
		},
	},
	"CLI_FORMAT": {
		Setter: func(this *systemVariables, name, value string) error {
			switch strings.ToUpper(unquoteString(value)) {
			case "TABLE":
				this.CLIFormat = DisplayModeTable
			case "VERTICAL":
				this.CLIFormat = DisplayModeVertical
			case "TAB":
				this.CLIFormat = DisplayModeTab
			}
			return nil
		},
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			var formatStr string
			switch this.CLIFormat {
			case DisplayModeTable:
				formatStr = "TABLE"
			case DisplayModeVertical:
				formatStr = "VERTICAL"
			case DisplayModeTab:
				formatStr = "TAB"
			}
			return singletonMap(name, formatStr), nil
		},
	},
	"CLI_VERBOSE": {
		Getter: boolGetter(func(sysVars *systemVariables) *bool { return &sysVars.Verbose }),
		Setter: func(this *systemVariables, name, value string) error {
			b, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			this.Verbose = b
			return nil
		},
	},
	"CLI_ROLE": {
		Getter: stringGetter(func(sysVars *systemVariables) *string { return &sysVars.Role }),
	},
	"CLI_ENDPOINT": {
		Getter: stringGetter(func(sysVars *systemVariables) *string { return &sysVars.Endpoint }),
	},
	"CLI_DIRECT_READ": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, xiter.Join(xiter.Map(
				slices.Values(this.DirectedRead.GetIncludeReplicas().GetReplicaSelections()),
				func(rs *sppb.DirectedReadOptions_ReplicaSelection) string {
					return fmt.Sprintf("%v:%v", rs.GetLocation(), rs.GetType())
				},
			), ",")), nil
		},
	},
	"CLI_HISTORY_FILE": {
		Getter: stringGetter(
			func(sysVars *systemVariables) *string { return &sysVars.HistoryFile },
		),
	},
	"CLI_PROMPT":  stringAccessor(func(sysVars *systemVariables) *string { return &sysVars.Prompt }),
	"CLI_PROMPT2": stringAccessor(func(sysVars *systemVariables) *string { return &sysVars.Prompt2 }),
	"CLI_PROJECT": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, this.Project), nil
		},
	},
	"CLI_INSTANCE": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, this.Instance), nil
		},
	},
	"CLI_DATABASE": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, this.Database), nil
		},
	},
	"CLI_PROTO_DESCRIPTOR_FILE": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, strings.Join(this.ProtoDescriptorFile, ",")), nil
		},
		Setter: func(this *systemVariables, name, value string) error {
			filenames := strings.Split(unquoteString(value), ",")
			if len(filenames) == 0 {
				return nil
			}

			var fileDescriptorSet *descriptorpb.FileDescriptorSet
			for _, filename := range filenames {
				fds, err := readFileDescriptorProtoFromFile(filename)
				if err != nil {
					return err
				}
				fileDescriptorSet = mergeFDS(fileDescriptorSet, fds)
			}

			this.ProtoDescriptorFile = filenames
			this.ProtoDescriptor = fileDescriptorSet
			return nil
		},
		Adder: func(this *systemVariables, name, value string) error {
			filename := unquoteString(value)

			fds, err := readFileDescriptorProtoFromFile(filename)
			if err != nil {
				return err
			}

			if !slices.Contains(this.ProtoDescriptorFile, filename) {
				this.ProtoDescriptorFile = slices.Concat(this.ProtoDescriptorFile, sliceOf(filename))
				this.ProtoDescriptor = &descriptorpb.FileDescriptorSet{File: slices.Concat(this.ProtoDescriptor.GetFile(), fds.GetFile())}
			} else {
				this.ProtoDescriptor = mergeFDS(this.ProtoDescriptor, fds)
			}
			return nil
		},
	},
	"CLI_PARSE_MODE": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			if this.BuildStatementMode == parseModeUnspecified {
				return nil, errIgnored
			}
			return singletonMap(name, string(this.BuildStatementMode)), nil
		},
		Setter: func(this *systemVariables, name, value string) error {
			s := strings.ToUpper(unquoteString(value))
			switch s {
			case string(parseModeFallback),
				string(parseMemefishOnly),
				string(parseModeNoMemefish),
				string(parseModeUnspecified):

				this.BuildStatementMode = parseMode(s)
				return nil
			default:
				return fmt.Errorf("invalid value: %v", s)
			}
		},
	},
	"CLI_INSECURE": {
		Getter: boolGetter(func(sysVars *systemVariables) *bool { return &sysVars.Insecure }),
	},
	"CLI_DEBUG": {
		Getter: boolGetter(func(sysVars *systemVariables) *bool { return lo.Ternary(sysVars.Debug, lo.ToPtr(sysVars.Debug), nil) }),
		Setter: boolSetter(func(sysVars *systemVariables) *bool { return &sysVars.Debug }),
	},
	"CLI_LOG_GRPC": {
		Getter: boolGetter(func(sysVars *systemVariables) *bool {
			return lo.Ternary(sysVars.LogGrpc, &sysVars.LogGrpc, nil)
		}),
	},
	"CLI_LINT_PLAN": {
		Getter: boolGetter(func(sysVars *systemVariables) *bool {
			return lo.Ternary(sysVars.LintPlan, lo.ToPtr(sysVars.LintPlan), nil)
		}),
		Setter: boolSetter(func(sysVars *systemVariables) *bool { return &sysVars.LintPlan }),
	},
	"CLI_QUERY_MODE": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			if this.QueryMode == nil {
				return nil, errIgnored
			}
			return singletonMap(name, this.QueryMode.String()), nil
		},
		Setter: func(this *systemVariables, name, value string) error {
			s := unquoteString(value)
			mode, ok := sppb.ExecuteSqlRequest_QueryMode_value[strings.ToUpper(s)]
			if !ok {
				return fmt.Errorf("invalid value: %v", s)
			}
			this.QueryMode = sppb.ExecuteSqlRequest_QueryMode(mode).Enum()
			return nil
		},
	},
	"CLI_VERSION": {
		Getter: func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, getVersion()), nil
		},
	},
}

func mergeFDS(left, right *descriptorpb.FileDescriptorSet) *descriptorpb.FileDescriptorSet {
	result := slices.Clone(left.GetFile())
	for _, fd := range right.GetFile() {
		idx := slices.IndexFunc(result, func(descriptorProto *descriptorpb.FileDescriptorProto) bool {
			return descriptorProto.GetPackage() == fd.GetPackage() && descriptorProto.GetName() == fd.GetName()
		})
		if idx != -1 {
			result[idx] = fd
		} else {
			result = append(result, fd)
		}
	}
	return &descriptorpb.FileDescriptorSet{File: result}
}

func readFileDescriptorProtoFromFile(filename string) (*descriptorpb.FileDescriptorSet, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error on read proto descriptor-file %v: %w", filename, err)
	}
	var fds descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(b, &fds)
	if err != nil {
		return nil, fmt.Errorf("error on unmarshal proto descriptor-file %v: %w", filename, err)
	}
	return &fds, nil
}

func stringGetter(f func(sysVars *systemVariables) *string) getter {
	return func(this *systemVariables, name string) (map[string]string, error) {
		ref := f(this)
		if ref == nil {
			return nil, errIgnored
		}
		return singletonMap(name, *ref), nil
	}
}

func boolGetter(f func(sysVars *systemVariables) *bool) getter {
	return func(this *systemVariables, name string) (map[string]string, error) {
		ref := f(this)
		if ref == nil {
			return nil, errIgnored
		}

		return singletonMap(name, strings.ToUpper(strconv.FormatBool(*ref))), nil
	}
}

func boolSetter(f func(sysVars *systemVariables) *bool) setter {
	return func(this *systemVariables, name string, value string) error {
		ref := f(this)

		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}

		*ref = b
		return nil
	}
}

func stringSetter(f func(sysVars *systemVariables) *string) setter {
	return func(this *systemVariables, name, value string) error {
		ref := f(this)
		s := unquoteString(value)
		*ref = s
		return nil
	}
}

func stringAccessor(f func(variables *systemVariables) *string) accessor {
	return accessor{
		Setter: stringSetter(f),
		Getter: stringGetter(f),
	}
}

func parseTimestampBound(s string) (spanner.TimestampBound, error) {
	first, second, _ := strings.Cut(s, " ")

	// only for error result
	nilStaleness := spanner.StrongRead()

	switch strings.ToUpper(first) {
	case "STRONG":
		return spanner.StrongRead(), nil
	case "MIN_READ_TIMESTAMP":
		ts, err := time.Parse(time.RFC3339Nano, second)
		if err != nil {
			return nilStaleness, err
		}
		return spanner.MinReadTimestamp(ts), nil
	case "READ_TIMESTAMP":
		ts, err := time.Parse(time.RFC3339Nano, second)
		if err != nil {
			return nilStaleness, err
		}
		return spanner.ReadTimestamp(ts), nil
	case "MAX_STALENESS":
		ts, err := time.ParseDuration(second)
		if err != nil {
			return nilStaleness, err
		}
		return spanner.MaxStaleness(ts), nil
	case "EXACT_STALENESS":
		ts, err := time.ParseDuration(second)
		if err != nil {
			return nilStaleness, err
		}
		return spanner.ExactStaleness(ts), nil
	default:
		return nilStaleness, fmt.Errorf("unknown staleness: %v", first)
	}
}
