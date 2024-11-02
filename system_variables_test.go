package main

import "testing"

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
