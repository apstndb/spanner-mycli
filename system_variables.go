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
			s := unquoteString(value)

			var staleness spanner.TimestampBound
			switch {
			case s == "STRONG":
				staleness = spanner.StrongRead()
			case strings.HasPrefix(s, "MIN_READ_TIMESTAMP ") || strings.HasPrefix(s, "READ_TIMESTAMP "):
				var minReadTimestamp bool
				if strings.HasPrefix(s, "MIN_") {
					minReadTimestamp = true
					s = strings.TrimPrefix(s, "MIN_")
				}
				ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(strings.TrimPrefix(s, "READ_TIMESTAMP ")))
				if err != nil {
					return err
				}
				if minReadTimestamp {
					staleness = spanner.MinReadTimestamp(ts)
				} else {
					staleness = spanner.ReadTimestamp(ts)
				}
			case strings.HasPrefix(s, "MAX_STALENESS ") || strings.HasPrefix(s, "MIN_STALENESS "):
				var maxStaleness bool
				if strings.HasPrefix(s, "MAX_STALENESS ") {
					maxStaleness = true
					s = strings.TrimPrefix(s, "MAX_STALENESS ")
				} else {
					s = strings.TrimPrefix(s, "EXACT_STALENESS ")
				}
				ts, err := time.ParseDuration(strings.TrimSpace(s))
				if err != nil {
					return err
				}
				if maxStaleness {
					staleness = spanner.MaxStaleness(ts)
				} else {
					staleness = spanner.ExactStaleness(ts)
				}
			}
			this.ReadOnlyStaleness = &staleness
			return nil
		},
		func(this *systemVariables, name string) (map[string]string, error) {
			if this.ReadOnlyStaleness == nil {
				return singletonMap(name, "NULL"), nil
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
			ts := this.ReadTimestamp
			if ts.IsZero() {
				return nil, errIgnored
			}
			return singletonMap(name, ts.Format(time.RFC3339Nano)), nil
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
