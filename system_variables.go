package main

import (
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"strconv"
	"strings"
)

type systemVariables struct {
	RPCPriority sppb.RequestOptions_Priority
}

type setter func(this *systemVariables, value string) error

type getter func(this *systemVariables) (string, error)

type accessor struct {
	Setter setter
	Getter getter
}

var accessorMap = map[string]accessor{
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
}
