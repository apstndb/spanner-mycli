package main

import (
	"errors"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
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

func TestSystemVariablesSetGet(t *testing.T) {
	// Should cover normal cases of all system variables
	tests := []struct {
		desc                               string
		sysVars                            *systemVariables
		name                               string
		value                              string
		want                               map[string]string
		unimplementedSet, unimplementedGet bool
	}{
		// Java-spanner compatible variables
		{desc: "READ_TIMESTAMP", name: "READ_TIMESTAMP", unimplementedSet: true,
			sysVars: &systemVariables{ReadTimestamp: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)},
			want:    singletonMap("READ_TIMESTAMP", "1970-01-01T00:00:00Z")},
		{desc: "COMMIT_TIMESTAMP", name: "COMMIT_TIMESTAMP", unimplementedSet: true,
			sysVars: &systemVariables{CommitTimestamp: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)},
			want:    singletonMap("COMMIT_TIMESTAMP", "1970-01-01T00:00:00Z")},
		{desc: "COMMIT_RESPONSE", name: "COMMIT_RESPONSE", unimplementedSet: true,
			sysVars: &systemVariables{
				CommitTimestamp: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC),
				CommitResponse: &sppb.CommitResponse{CommitStats: &sppb.CommitResponse_CommitStats{
					MutationCount: 10,
				}},
			},
			want: map[string]string{"COMMIT_TIMESTAMP": "1970-01-01T00:00:00Z", "MUTATION_COUNT": "10"}},

		// CLI_* variables
		{desc: "CLI_VERSION", name: "CLI_VERSION", unimplementedSet: true,
			want: singletonMap("CLI_VERSION", getVersion())},
		{desc: "CLI_PROJECT", name: "CLI_PROJECT", unimplementedSet: true,
			sysVars: &systemVariables{Project: "test-project"},
			want:    singletonMap("CLI_PROJECT", "test-project")},
		{desc: "CLI_INSTANCE", name: "CLI_INSTANCE", unimplementedSet: true,
			sysVars: &systemVariables{Instance: "test-instance"},
			want:    singletonMap("CLI_INSTANCE", "test-instance")},
		{desc: "CLI_DATABASE", name: "CLI_DATABASE", unimplementedSet: true,
			sysVars: &systemVariables{Database: "test-database"},
			want:    singletonMap("CLI_DATABASE", "test-database")},
		{desc: "CLI_HISTORY_FILE", name: "CLI_HISTORY_FILE", unimplementedSet: true,
			sysVars: &systemVariables{HistoryFile: "/tmp/spanner_mycli_readline.tmp"},
			want:    singletonMap("CLI_HISTORY_FILE", "/tmp/spanner_mycli_readline.tmp")},
		{desc: "CLI_ENDPOINT", name: "CLI_ENDPOINT", unimplementedSet: true,
			sysVars: &systemVariables{Endpoint: "localhost:9010"},
			want:    singletonMap("CLI_ENDPOINT", "localhost:9010")},
		{desc: "CLI_DIRECT_READ", name: "CLI_DIRECT_READ", unimplementedSet: true,
			sysVars: &systemVariables{DirectedRead: &sppb.DirectedReadOptions{Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
				IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{Type: sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE, Location: "asia-northeast2"}}}}}},
			want: singletonMap("CLI_DIRECT_READ", "asia-northeast2:READ_WRITE")},
		// Java-spanner compatible boolean variables
		{desc: "READONLY", name: "READONLY", value: "TRUE",
			want: singletonMap("READONLY", "TRUE")},
		{desc: "AUTO_PARTITION_MODE", name: "AUTO_PARTITION_MODE", value: "TRUE",
			want: singletonMap("AUTO_PARTITION_MODE", "TRUE")},
		{desc: "AUTOCOMMIT", name: "AUTOCOMMIT", unimplementedSet: true, unimplementedGet: true,
			value: "FALSE",
			want:  singletonMap("AUTOCOMMIT", "FALSE")},
		{desc: "RETRY_ABORTS_INTERNALLY", name: "RETRY_ABORTS_INTERNALLY",
			unimplementedSet: true, unimplementedGet: true},
		{desc: "EXCLUDE_TXN_FROM_CHANGE_STREAMS", name: "EXCLUDE_TXN_FROM_CHANGE_STREAMS", value: "TRUE",
			want: singletonMap("EXCLUDE_TXN_FROM_CHANGE_STREAMS", "TRUE")},
		{desc: "AUTO_BATCH_DML", name: "AUTO_BATCH_DML", value: "TRUE",
			want: singletonMap("AUTO_BATCH_DML", "TRUE")},
		{desc: "DATA_BOOST_ENABLED", name: "DATA_BOOST_ENABLED", value: "TRUE",
			want: singletonMap("DATA_BOOST_ENABLED", "TRUE")},

		// CLI_* boolean variables
		{desc: "CLI_VERBOSE", name: "CLI_VERBOSE", value: "TRUE",
			want: singletonMap("CLI_VERBOSE", "TRUE")},
		{desc: "CLI_ECHO_EXECUTED_DDL", name: "CLI_ECHO_EXECUTED_DDL", value: "TRUE",
			want: singletonMap("CLI_ECHO_EXECUTED_DDL", "TRUE")},
		{desc: "CLI_ECHO_INPUT", name: "CLI_ECHO_INPUT", value: "TRUE",
			want: singletonMap("CLI_ECHO_INPUT", "TRUE")},
		{desc: "CLI_EXPLAIN_FORMAT", name: "CLI_EXPLAIN_FORMAT", value: "CURRENT",
			want: singletonMap("CLI_EXPLAIN_FORMAT", "CURRENT")},
		{desc: "CLI_USE_PAGER", name: "CLI_USE_PAGER", value: "TRUE",
			want: singletonMap("CLI_USE_PAGER", "TRUE")},
		{desc: "CLI_AUTOWRAP", name: "CLI_AUTOWRAP", value: "TRUE",
			want: singletonMap("CLI_AUTOWRAP", "TRUE")},
		{desc: "CLI_ENABLE_HIGHLIGHT", name: "CLI_ENABLE_HIGHLIGHT", value: "TRUE",
			want: singletonMap("CLI_ENABLE_HIGHLIGHT", "TRUE")},
		{desc: "CLI_PROTOTEXT_MULTILINE", name: "CLI_PROTOTEXT_MULTILINE", value: "TRUE",
			want: singletonMap("CLI_PROTOTEXT_MULTILINE", "TRUE")},
		{desc: "CLI_MARKDOWN_CODEBLOCK", name: "CLI_MARKDOWN_CODEBLOCK", value: "TRUE",
			want: singletonMap("CLI_MARKDOWN_CODEBLOCK", "TRUE")},
		{desc: "CLI_LINT_PLAN", name: "CLI_LINT_PLAN", value: "TRUE",
			want: singletonMap("CLI_LINT_PLAN", "TRUE")},
		{desc: "CLI_INSECURE", name: "CLI_INSECURE", unimplementedSet: true,
			sysVars: &systemVariables{Insecure: true},
			want:    singletonMap("CLI_INSECURE", "TRUE")},
		{desc: "CLI_LOG_GRPC", name: "CLI_LOG_GRPC", unimplementedSet: true,
			sysVars: &systemVariables{LogGrpc: true},
			want:    singletonMap("CLI_LOG_GRPC", "TRUE")},

		// Java-spanner compatible string variables
		{desc: "MAX_COMMIT_DELAY", name: "MAX_COMMIT_DELAY", value: "100ms",
			want: singletonMap("MAX_COMMIT_DELAY", "100ms")},
		{desc: "READ_ONLY_STALENESS", name: "READ_ONLY_STALENESS", value: "STRONG",
			want: singletonMap("READ_ONLY_STALENESS", "STRONG")},
		{desc: "OPTIMIZER_VERSION", name: "OPTIMIZER_VERSION", value: "LATEST",
			want: singletonMap("OPTIMIZER_VERSION", "LATEST")},
		{desc: "OPTIMIZER_STATISTICS_PACKAGE", name: "OPTIMIZER_STATISTICS_PACKAGE", value: "test-package",
			want: singletonMap("OPTIMIZER_STATISTICS_PACKAGE", "test-package")},
		{desc: "RPC_PRIORITY", name: "RPC_PRIORITY", value: "HIGH",
			want: singletonMap("RPC_PRIORITY", "HIGH")},
		{desc: "STATEMENT_TAG", name: "STATEMENT_TAG", value: "test-statement",
			want: singletonMap("STATEMENT_TAG", "test-statement")},
		{desc: "TRANSACTION_TAG", name: "TRANSACTION_TAG", value: "test-tag",
			sysVars: &systemVariables{CurrentSession: &Session{tc: &transactionContext{
				mode: transactionModePending,
			}}},
			want: singletonMap("TRANSACTION_TAG", "test-tag")},

		// CLI_* string variables
		{desc: "CLI_OUTPUT_TEMPLATE_FILE", name: "CLI_OUTPUT_TEMPLATE_FILE", value: "output_default.tmpl",
			want: singletonMap("CLI_OUTPUT_TEMPLATE_FILE", "output_default.tmpl")},
		{desc: "CLI_ROLE", name: "CLI_ROLE",
			unimplementedSet: true, sysVars: &systemVariables{Role: "test-role"},
			want: singletonMap("CLI_ROLE", "test-role")},
		{desc: "CLI_PROMPT", name: "CLI_PROMPT", value: "test-prompt",
			want: singletonMap("CLI_PROMPT", "test-prompt")},
		{desc: "CLI_PROMPT2", name: "CLI_PROMPT2", value: "test-prompt2",
			want: singletonMap("CLI_PROMPT2", "test-prompt2")},
		{desc: "CLI_ANALYZE_COLUMNS", name: "CLI_ANALYZE_COLUMNS", value: "name:{{.template}}:LEFT",
			want: singletonMap("CLI_ANALYZE_COLUMNS", "name:{{.template}}:LEFT")},
		{desc: "CLI_INLINE_STATS", name: "CLI_INLINE_STATS", value: "name:{{.template}}",
			want: singletonMap("CLI_INLINE_STATS", "name:{{.template}}")},
		{desc: "CLI_PARSE_MODE", name: "CLI_PARSE_MODE", value: "FALLBACK",
			want: singletonMap("CLI_PARSE_MODE", "FALLBACK")},
		{desc: "CLI_LOG_LEVEL", name: "CLI_LOG_LEVEL", value: "INFO",
			want: singletonMap("CLI_LOG_LEVEL", "INFO")},
		{desc: "CLI_VERTEXAI_MODEL", name: "CLI_VERTEXAI_MODEL", value: "test",
			want: singletonMap("CLI_VERTEXAI_MODEL", "test")},
		{desc: "CLI_VERTEXAI_PROJECT", name: "CLI_VERTEXAI_PROJECT", value: "example-project",
			want: singletonMap("CLI_VERTEXAI_PROJECT", "example-project")},
		{desc: "CLI_PROTO_DESCRIPTOR_FILE", name: "CLI_PROTO_DESCRIPTOR_FILE", value: "testdata/protos/order_descriptors.pb",
			want: singletonMap("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/order_descriptors.pb")},

		// Java-spanner compatible integer variables
		{desc: "MAX_PARTITIONED_PARALLELISM", name: "MAX_PARTITIONED_PARALLELISM", value: "10",
			want: singletonMap("MAX_PARTITIONED_PARALLELISM", "10")},

		// CLI_* integer variables
		{desc: "CLI_TAB_WIDTH", name: "CLI_TAB_WIDTH", value: "4",
			want: singletonMap("CLI_TAB_WIDTH", "4")},

		// Java-spanner compatible enum variables
		{desc: "AUTOCOMMIT_DML_MODE", name: "AUTOCOMMIT_DML_MODE", value: "TRANSACTIONAL",
			want: singletonMap("AUTOCOMMIT_DML_MODE", "TRANSACTIONAL")},
		{desc: "DEFAULT_ISOLATION_LEVEL", name: "DEFAULT_ISOLATION_LEVEL", value: "SERIALIZABLE",
			want: singletonMap("DEFAULT_ISOLATION_LEVEL", "SERIALIZABLE")},

		// CLI_* enum variables
		{desc: "CLI_FORMAT", name: "CLI_FORMAT", value: "TABLE",
			want: singletonMap("CLI_FORMAT", "TABLE")},
		{desc: "CLI_DATABASE_DIALECT", name: "CLI_DATABASE_DIALECT",
			value: "GOOGLE_STANDARD_SQL",
			want:  singletonMap("CLI_DATABASE_DIALECT", "GOOGLE_STANDARD_SQL")},
		{desc: "CLI_QUERY_MODE", name: "CLI_QUERY_MODE", value: "PROFILE",
			want: singletonMap("CLI_QUERY_MODE", "PROFILE")},

		// New CLI_* variables added for Issue #243
		{desc: "CLI_ENABLE_PROGRESS_BAR", name: "CLI_ENABLE_PROGRESS_BAR", value: "TRUE",
			want: singletonMap("CLI_ENABLE_PROGRESS_BAR", "TRUE")},
		{desc: "CLI_IMPERSONATE_SERVICE_ACCOUNT", name: "CLI_IMPERSONATE_SERVICE_ACCOUNT", unimplementedSet: true,
			sysVars: &systemVariables{ImpersonateServiceAccount: "test@example.com"},
			want:    singletonMap("CLI_IMPERSONATE_SERVICE_ACCOUNT", "test@example.com")},
		{desc: "CLI_ENABLE_ADC_PLUS", name: "CLI_ENABLE_ADC_PLUS", unimplementedSet: true,
			sysVars: &systemVariables{EnableADCPlus: true},
			want:    singletonMap("CLI_ENABLE_ADC_PLUS", "TRUE")},
		{desc: "CLI_MCP", name: "CLI_MCP", unimplementedSet: true,
			sysVars: &systemVariables{MCP: true},
			want:    singletonMap("CLI_MCP", "TRUE")},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			test := test
			sysVars := test.sysVars
			if sysVars == nil {
				sysVars = &systemVariables{}
			}

			err := sysVars.Set(test.name, test.value)
			if !test.unimplementedSet {
				if err != nil {
					t.Errorf("sysVars.Set should success, but failed: %v", err)
				}
			} else {
				var e errSetterUnimplemented
				if !errors.As(err, &e) {
					t.Errorf("sysVars.Set is skipped, but implemented: %v", err)
				}
			}

			got, err := sysVars.Get(test.name)
			if !test.unimplementedGet {
				if err != nil {
					t.Errorf("sysVars.Get should success, but failed: %v", err)
				}

				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("sysVars.Get() mismatch (-want +got):\n%s", diff)
				}
			} else {
				var e errGetterUnimplemented
				if !errors.As(err, &e) {
					t.Errorf("sysVars.Get is skipped, but implemented: %v", err)
				}
			}
		})
	}
}
