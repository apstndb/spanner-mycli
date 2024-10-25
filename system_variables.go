package main

import (
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"strconv"
	"strings"
)

type systemVariables struct {
	RPCPriority sppb.RequestOptions_Priority
}

type setter = func(this *systemVariables, value string) error

type getter = func(this *systemVariables) (string, error)

type accessor struct {
	Setter setter
	Getter getter
}

var accessorMap = map[string]accessor{
	"AUTOCOMMIT":                   {},
	"RETRY_ABORTS_INTERNALLY":      {},
	"AUTOCOMMIT_DML_MODE":          {},
	"STATEMENT_TIMEOUT":            {},
	"READ_ONLY_STALENESS":          {},
	"OPTIMIZER_VERSION":            {},
	"OPTIMIZER_STATISTICS_PACKAGE": {},
	"RETURN_COMMIT_STATS":          {},
	"RPC_PRIORITY": {
		func(this *systemVariables, value string) error {
			s, err := strconv.Unquote(value)
			if err != nil {
				return err
			}

			p, err := parsePriority(s)
			if err != nil {
				return err
			}

			this.RPCPriority = p
			return nil
		},
		func(this *systemVariables) (string, error) {
			return strings.TrimPrefix(this.RPCPriority.String(), "PRIORITY_"), nil
		},
	},
	"STATEMENT_TAG":    {},
	"TRANSACTION_TAG":  {},
	"READ_TIMESTAMP":   {},
	"COMMIT_TIMESTAMP": {},
	"COMMIT_RESPONSE":  {},
}
