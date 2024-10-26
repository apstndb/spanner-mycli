package main

import (
	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type systemVariables struct {
	RPCPriority                sppb.RequestOptions_Priority
	ReadOnlyStaleness          *spanner.TimestampBound
	ReadTimestamp              time.Time
	OptimizerVersion           string
	OptimizerStatisticsPackage string
	CommitResponse             *sppb.CommitResponse
	CommitTimestamp            time.Time
}

var errIgnored = errors.New("ignored")

type setter = func(this *systemVariables, name, value string) error

type getter = func(this *systemVariables, name string) (map[string]string, error)

type accessor struct {
	Setter setter
	Getter getter
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
				return singletonMap(name, fmt.Sprintf("READ_TIMESTAMP %v", matches[2])), nil
			case "minReadTimestamp":
				return singletonMap(name, fmt.Sprintf("MIN_READ_TIMESTAMP %v", matches[2])), nil
			default:
				return singletonMap(name, s), nil
			}
		},
	},
	"OPTIMIZER_VERSION": {
		func(this *systemVariables, name, value string) error {
			this.OptimizerVersion = unquoteString(value)
			return nil
		},
		func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, this.OptimizerVersion), nil
		},
	},
	"OPTIMIZER_STATISTICS_PACKAGE": {
		func(this *systemVariables, name, value string) error {
			this.OptimizerStatisticsPackage = unquoteString(value)
			return nil
		},
		func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, this.OptimizerStatisticsPackage), nil
		},
	},
	"RETURN_COMMIT_STATS": {},
	"RPC_PRIORITY": {
		func(this *systemVariables, name, value string) error {
			s := unquoteString(value)

			p, err := parsePriority(s)
			if err != nil {
				return err
			}

			this.RPCPriority = p
			return nil
		},
		func(this *systemVariables, name string) (map[string]string, error) {
			return singletonMap(name, strings.TrimPrefix(this.RPCPriority.String(), "PRIORITY_")), nil
		},
	},
	"STATEMENT_TAG":   {},
	"TRANSACTION_TAG": {},
	"READ_TIMESTAMP": {
		nil,
		func(this *systemVariables, name string) (map[string]string, error) {
			if this.ReadTimestamp.IsZero() {
				return nil, errIgnored
			}
			return singletonMap(name, this.ReadTimestamp.Format(time.RFC3339Nano)), nil
		},
	},
	"COMMIT_TIMESTAMP": {
		nil,
		func(this *systemVariables, name string) (map[string]string, error) {
			if this.CommitTimestamp.IsZero() {
				return nil, errIgnored
			}
			s := this.CommitTimestamp.Format(time.RFC3339Nano)
			return singletonMap(name, s), nil
		},
	},
	"COMMIT_RESPONSE": {
		nil,
		func(this *systemVariables, name string) (map[string]string, error) {
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
