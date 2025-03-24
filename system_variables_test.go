package main

import (
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func TestSystemVariables_AddCLIProtoDescriptorFile(t *testing.T) {
	// TODO: More test
	tests := []struct {
		desc   string
		values []string
	}{
		{"single", []string{"testdata/protos/order_descriptors.pb"}},
		{"repeated", []string{"testdata/protos/order_descriptors.pb", "testdata/protos/order_descriptors.pb"}},
		{"multiple", []string{"testdata/protos/order_descriptors.pb", "testdata/protos/query_plan_descriptors.pb"}},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var sysVars systemVariables
			for _, value := range test.values {
				if err := sysVars.Add("CLI_PROTO_DESCRIPTOR_FILE", value); err != nil {
					t.Errorf("should success, but failed, value: %v, err: %v", value, err)
				}
			}
		})
	}
}

func TestSystemVariables_DefaultIsolationLevel(t *testing.T) {
	// TODO: More test
	tests := []struct {
		value string
		want  sppb.TransactionOptions_IsolationLevel
	}{
		{"REPEATABLE READ", sppb.TransactionOptions_REPEATABLE_READ},
		{"repeatable read", sppb.TransactionOptions_REPEATABLE_READ},
		{"REPEATABLE_READ", sppb.TransactionOptions_REPEATABLE_READ},
		{"repeatable_read", sppb.TransactionOptions_REPEATABLE_READ},
		{"serializable", sppb.TransactionOptions_SERIALIZABLE},
		{"SERIALIZABLE", sppb.TransactionOptions_SERIALIZABLE},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			var sysVars systemVariables
			if err := sysVars.Set("DEFAULT_ISOLATION_LEVEL", test.value); err != nil {
				t.Errorf("should success, but failed, value: %v, err: %v", test.value, err)
			}

			if sysVars.DefaultIsolationLevel != test.want {
				t.Errorf("DefaultIsolationLevel should be %v, but %v", test.want, sysVars.DefaultIsolationLevel)
			}
		})
	}
}
