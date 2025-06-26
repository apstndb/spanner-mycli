package main

import (
	"cloud.google.com/go/spanner"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
)

func TestSystemVariables_AddCLIProtoDescriptorFile(t *testing.T) {
	tests := []struct {
		desc      string
		values    []string
		wantError bool
		errorMsg  string
	}{
		{
			desc:   "single valid descriptor",
			values: []string{"testdata/protos/order_descriptors.pb"},
		},
		{
			desc:   "repeated same descriptor",
			values: []string{"testdata/protos/order_descriptors.pb", "testdata/protos/order_descriptors.pb"},
		},
		{
			desc:   "multiple different descriptors",
			values: []string{"testdata/protos/order_descriptors.pb", "testdata/protos/query_plan_descriptors.pb"},
		},
		{
			desc:      "non-existent file",
			values:    []string{"testdata/protos/non_existent.pb"},
			wantError: true,
			errorMsg:  "no such file or directory",
		},
		{
			desc:      "invalid proto file",
			values:    []string{"testdata/invalid_protos/invalid.txt"},
			wantError: true,
			errorMsg:  "error on unmarshal proto descriptor-file",
		},
		{
			desc:   "empty file",
			values: []string{"testdata/invalid_protos/empty.pb"},
			// Empty files unmarshal successfully to empty FileDescriptorSet
		},
		{
			desc:   "proto source file",
			values: []string{"testdata/protos/singer.proto"},
		},
		{
			desc:      "invalid proto source file",
			values:    []string{"testdata/invalid_protos/invalid_proto.proto"},
			wantError: true,
			errorMsg:  "invalid_proto.proto:",
		},
		{
			desc:      "mix of valid and invalid files",
			values:    []string{"testdata/protos/order_descriptors.pb", "testdata/protos/non_existent.pb"},
			wantError: true,
			errorMsg:  "no such file or directory",
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var sysVars systemVariables
			var lastErr error
			for _, value := range test.values {
				if err := sysVars.Add("CLI_PROTO_DESCRIPTOR_FILE", value); err != nil {
					lastErr = err
					if !test.wantError {
						t.Errorf("unexpected error: value: %v, err: %v", value, err)
					}
				}
			}
			
			if test.wantError {
				if lastErr == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(lastErr.Error(), test.errorMsg) {
					t.Errorf("expected error containing %q, got %v", test.errorMsg, lastErr)
				}
			}
		})
	}
}

func TestReadFileDescriptorProtoFromFile(t *testing.T) {
	// Ensure test fixtures directory exists for dynamic files
	if err := os.MkdirAll("testdata/test_fixtures", 0755); err != nil {
		t.Fatalf("Failed to create test fixtures directory: %v", err)
	}
	
	// Create a test file with permission issues
	permissionTestFile := "testdata/test_fixtures/permission_test.pb"
	if err := os.WriteFile(permissionTestFile, []byte("test"), 0000); err == nil {
		defer func() {
			err := os.Chmod(permissionTestFile, 0644) // Reset permissions
			if err != nil {
				t.Errorf("failed to reset permissions for %s: %v", permissionTestFile, err)
			}
			err = os.Remove(permissionTestFile)
			if err != nil {
				t.Errorf("failed to remove %s: %v", permissionTestFile, err)
			}
		}()
	}

	// Create a large descriptor file for testing
	largeFile := "testdata/test_fixtures/large_test.pb"
	if err := os.WriteFile(largeFile, make([]byte, 1024*1024), 0644); err == nil {
		defer func() {
			err := os.Remove(largeFile)
			if err != nil {
				t.Errorf("failed to remove %s: %v", largeFile, err)
			}
		}()
	}

	tests := []struct {
		desc      string
		filename  string
		wantError bool
		errorMsg  string
	}{
		{
			desc:     "valid descriptor file",
			filename: "testdata/protos/order_descriptors.pb",
		},
		{
			desc:     "valid proto source file",
			filename: "testdata/protos/singer.proto",
		},
		{
			desc:      "non-existent file",
			filename:  "testdata/protos/non_existent.pb",
			wantError: true,
			errorMsg:  "no such file or directory",
		},
		{
			desc:      "permission denied file",
			filename:  permissionTestFile,
			wantError: true,
			errorMsg:  "permission denied",
		},
		{
			desc:      "invalid proto binary file",
			filename:  "testdata/invalid_protos/invalid.txt",
			wantError: true,
			errorMsg:  "error on unmarshal proto descriptor-file",
		},
		{
			desc:     "empty file",
			filename: "testdata/invalid_protos/empty.pb",
			// Empty files unmarshal successfully to empty FileDescriptorSet
		},
		{
			desc:      "invalid proto source file",
			filename:  "testdata/invalid_protos/invalid_proto.proto",
			wantError: true,
			errorMsg:  "invalid_proto.proto:",
		},
		{
			desc:      "directory instead of file",
			filename:  "testdata/protos/",
			wantError: true,
			errorMsg:  "is a directory",
		},
		{
			desc:      "large invalid binary file",
			filename:  largeFile,
			wantError: true,
			errorMsg:  "error on unmarshal proto descriptor-file",
		},
		{
			desc:      "HTTP URL - non-existent",
			filename:  "http://example.com/non_existent.pb",
			wantError: true,
			errorMsg:  "error on unmarshal proto descriptor-file",
		},
		{
			desc:      "HTTPS URL - non-existent",
			filename:  "https://example.com/non_existent.pb",
			wantError: true,
			errorMsg:  "error on unmarshal proto descriptor-file",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			fds, err := readFileDescriptorProtoFromFile(test.filename)

			if test.wantError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), test.errorMsg) {
					t.Errorf("expected error containing %q, got %v", test.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if fds == nil {
					t.Errorf("expected non-nil FileDescriptorSet")
				}
			}
		})
	}
}

func TestSystemVariables_CLIProtoDescriptorFile_Integration(t *testing.T) {
	tests := []struct {
		desc             string
		descriptorFiles  []string
		verifyDescriptor bool
	}{
		{
			desc:             "verify descriptors are loaded and usable",
			descriptorFiles:  []string{"testdata/protos/order_descriptors.pb", "testdata/protos/query_plan_descriptors.pb"},
			verifyDescriptor: true,
		},
		{
			desc:             "verify proto source files are compiled and loaded",
			descriptorFiles:  []string{"testdata/protos/singer.proto"},
			verifyDescriptor: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var sysVars systemVariables
			
			// Add descriptor files
			for _, file := range test.descriptorFiles {
				if err := sysVars.Add("CLI_PROTO_DESCRIPTOR_FILE", file); err != nil {
					t.Fatalf("Failed to add descriptor file %s: %v", file, err)
				}
			}
			
			// Verify descriptors were loaded
			if test.verifyDescriptor {
				if sysVars.ProtoDescriptor == nil {
					t.Errorf("Expected ProtoDescriptor to be set")
				}
				if sysVars.ProtoDescriptor.GetFile() == nil {
					t.Errorf("Expected FileDescriptorSet to contain files")
				}
				if len(sysVars.ProtoDescriptor.GetFile()) == 0 {
					t.Errorf("Expected at least one file descriptor")
				}
			}
		})
	}
}

func TestSystemVariables_AddCLIProtoDescriptorFile_EdgeCases(t *testing.T) {
	tests := []struct {
		desc      string
		setup     func() *systemVariables
		varName   string
		value     string
		wantError bool
		errorMsg  string
	}{
		{
			desc: "empty string value",
			setup: func() *systemVariables {
				return &systemVariables{}
			},
			varName:   "CLI_PROTO_DESCRIPTOR_FILE",
			value:     "",
			wantError: true,
			errorMsg:  "no such file or directory",
		},
		{
			desc: "spaces only value",
			setup: func() *systemVariables {
				return &systemVariables{}
			},
			varName:   "CLI_PROTO_DESCRIPTOR_FILE",
			value:     "   ",
			wantError: true,
			errorMsg:  "no such file or directory",
		},
		{
			desc: "relative path with ..",
			setup: func() *systemVariables {
				return &systemVariables{}
			},
			varName:   "CLI_PROTO_DESCRIPTOR_FILE",
			value:     "../testdata/protos/order_descriptors.pb",
			wantError: true,
			errorMsg:  "no such file or directory",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			sysVars := test.setup()
			err := sysVars.Add(test.varName, test.value)
			
			if test.wantError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), test.errorMsg) {
					t.Errorf("expected error containing %q, got %v", test.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
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
		{desc: "STATEMENT_TIMEOUT", name: "STATEMENT_TIMEOUT", value: "30s",
			want: singletonMap("STATEMENT_TIMEOUT", "30s")},

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
		{desc: "RETURN_COMMIT_STATS true", name: "RETURN_COMMIT_STATS", value: "TRUE",
			want: singletonMap("RETURN_COMMIT_STATS", "TRUE")},
		{desc: "RETURN_COMMIT_STATS false", name: "RETURN_COMMIT_STATS", value: "FALSE",
			want: singletonMap("RETURN_COMMIT_STATS", "FALSE")},
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

func TestSystemVariables_StatementTimeout(t *testing.T) {
	tests := []struct {
		desc        string
		value       string
		want        time.Duration
		expectError bool
	}{
		{"valid_seconds", "30s", 30 * time.Second, false},
		{"valid_minutes", "5m", 5 * time.Minute, false},
		{"valid_hours", "1h", 1 * time.Hour, false},
		{"valid_mixed", "1h30m", 90 * time.Minute, false},
		{"valid_zero", "0s", 0, false},
		{"invalid_format", "invalid", 0, true},
		{"negative_value", "-30s", 0, true},
		{"empty_string", "", 0, true},
	}
	
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var sysVars systemVariables
			err := sysVars.Set("STATEMENT_TIMEOUT", test.value)
			
			if test.expectError {
				if err == nil {
					t.Errorf("expected error for value %q, but got nil", test.value)
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error for value %q: %v", test.value, err)
				return
			}
			
			if sysVars.StatementTimeout == nil || *sysVars.StatementTimeout != test.want {
				var got time.Duration
				if sysVars.StatementTimeout != nil {
					got = *sysVars.StatementTimeout
				}
				t.Errorf("expected StatementTimeout %v, got %v", test.want, got)
			}
			
			// Test getter
			result, err := sysVars.Get("STATEMENT_TIMEOUT")
			if err != nil {
				t.Errorf("unexpected error getting STATEMENT_TIMEOUT: %v", err)
				return
			}
			
			expected := map[string]string{"STATEMENT_TIMEOUT": test.want.String()}
			if diff := cmp.Diff(expected, result); diff != "" {
				t.Errorf("STATEMENT_TIMEOUT getter mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseTimestampBound(t *testing.T) {
	// Test valid timestamp bounds
	validTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	validTimeStr := "2024-01-01T00:00:00Z"
	
	tests := []struct {
		desc        string
		input       string
		want        spanner.TimestampBound
		expectError bool
		errorMsg    string
	}{
		// Valid cases - all 5 timestamp bound types
		{
			desc:  "STRONG read",
			input: "STRONG",
			want:  spanner.StrongRead(),
		},
		{
			desc:  "MIN_READ_TIMESTAMP with valid timestamp",
			input: "MIN_READ_TIMESTAMP " + validTimeStr,
			want:  spanner.MinReadTimestamp(validTime),
		},
		{
			desc:  "READ_TIMESTAMP with valid timestamp",
			input: "READ_TIMESTAMP " + validTimeStr,
			want:  spanner.ReadTimestamp(validTime),
		},
		{
			desc:  "MAX_STALENESS with valid duration",
			input: "MAX_STALENESS 30s",
			want:  spanner.MaxStaleness(30 * time.Second),
		},
		{
			desc:  "EXACT_STALENESS with valid duration",
			input: "EXACT_STALENESS 1h30m",
			want:  spanner.ExactStaleness(90 * time.Minute),
		},
		
		// Case insensitivity tests
		{
			desc:  "lowercase strong",
			input: "strong",
			want:  spanner.StrongRead(),
		},
		{
			desc:  "mixed case Strong",
			input: "Strong",
			want:  spanner.StrongRead(),
		},
		{
			desc:  "lowercase min_read_timestamp",
			input: "min_read_timestamp " + validTimeStr,
			want:  spanner.MinReadTimestamp(validTime),
		},
		{
			desc:  "mixed case Min_Read_Timestamp",
			input: "Min_Read_Timestamp " + validTimeStr,
			want:  spanner.MinReadTimestamp(validTime),
		},
		{
			desc:  "lowercase read_timestamp",
			input: "read_timestamp " + validTimeStr,
			want:  spanner.ReadTimestamp(validTime),
		},
		{
			desc:  "lowercase max_staleness",
			input: "max_staleness 15s",
			want:  spanner.MaxStaleness(15 * time.Second),
		},
		{
			desc:  "lowercase exact_staleness",
			input: "exact_staleness 45m",
			want:  spanner.ExactStaleness(45 * time.Minute),
		},
		
		// Error cases - invalid timestamps
		{
			desc:        "MIN_READ_TIMESTAMP with invalid timestamp format",
			input:       "MIN_READ_TIMESTAMP invalid-date",
			expectError: true,
		},
		{
			desc:        "MIN_READ_TIMESTAMP with invalid month",
			input:       "MIN_READ_TIMESTAMP 2024-13-01T00:00:00Z",
			expectError: true,
		},
		{
			desc:        "READ_TIMESTAMP with invalid timestamp format",
			input:       "READ_TIMESTAMP not-a-timestamp",
			expectError: true,
		},
		{
			desc:        "READ_TIMESTAMP with malformed RFC3339",
			input:       "READ_TIMESTAMP 2024-01-01",
			expectError: true,
		},
		
		// Error cases - invalid durations
		{
			desc:        "MAX_STALENESS with invalid duration",
			input:       "MAX_STALENESS invalid-duration",
			expectError: true,
		},
		{
			desc:        "MAX_STALENESS with negative duration",
			input:       "MAX_STALENESS -30s",
			expectError: true,
			errorMsg:    "staleness duration \"-30s\" must be non-negative",
		},
		{
			desc:        "EXACT_STALENESS with invalid duration",
			input:       "EXACT_STALENESS not-a-duration",
			expectError: true,
		},
		{
			desc:        "EXACT_STALENESS with negative duration",
			input:       "EXACT_STALENESS -1h",
			expectError: true,
			errorMsg:    "staleness duration \"-1h\" must be non-negative",
		},
		
		// Error cases - unknown staleness types
		{
			desc:        "unknown staleness type",
			input:       "UNKNOWN_TYPE 30s",
			expectError: true,
			errorMsg:    "unknown staleness: UNKNOWN_TYPE",
		},
		{
			desc:        "empty string",
			input:       "",
			expectError: true,
			errorMsg:    "unknown staleness: \"\"",
		},
		{
			desc:        "random text",
			input:       "some random text",
			expectError: true,
			errorMsg:    "some accepts at most one parameter",
		},
		
		// Edge cases
		{
			desc:        "STRONG with extra text should fail",
			input:       "STRONG extra text",
			expectError: true,
			errorMsg:    "STRONG accepts at most one parameter",
		},
		{
			desc:        "MIN_READ_TIMESTAMP missing timestamp",
			input:       "MIN_READ_TIMESTAMP",
			expectError: true,
			errorMsg:    "MIN_READ_TIMESTAMP requires a timestamp parameter",
		},
		{
			desc:        "READ_TIMESTAMP missing timestamp",
			input:       "READ_TIMESTAMP",
			expectError: true,
			errorMsg:    "READ_TIMESTAMP requires a timestamp parameter",
		},
		{
			desc:        "MAX_STALENESS missing duration",
			input:       "MAX_STALENESS",
			expectError: true,
			errorMsg:    "MAX_STALENESS requires a duration parameter",
		},
		{
			desc:        "EXACT_STALENESS missing duration",
			input:       "EXACT_STALENESS",
			expectError: true,
			errorMsg:    "EXACT_STALENESS requires a duration parameter",
		},
		{
			desc:  "extra whitespace before timestamp",
			input: "MIN_READ_TIMESTAMP   " + validTimeStr,
			want:  spanner.MinReadTimestamp(validTime),
		},
		{
			desc:  "tabs instead of spaces",
			input: "MAX_STALENESS	60s",
			want:  spanner.MaxStaleness(60 * time.Second),
		},
		{
			desc:  "zero duration for MAX_STALENESS",
			input: "MAX_STALENESS 0s",
			want:  spanner.MaxStaleness(0),
		},
		{
			desc:  "very large duration",
			input: "EXACT_STALENESS 999999h",
			want:  spanner.ExactStaleness(999999 * time.Hour),
		},
		
		// Extra parameter validation tests
		{
			desc:        "MIN_READ_TIMESTAMP with extra parameters",
			input:       "MIN_READ_TIMESTAMP " + validTimeStr + " extra",
			expectError: true,
			errorMsg:    "MIN_READ_TIMESTAMP accepts at most one parameter",
		},
		{
			desc:        "READ_TIMESTAMP with extra parameters",
			input:       "READ_TIMESTAMP " + validTimeStr + " extra param",
			expectError: true,
			errorMsg:    "READ_TIMESTAMP accepts at most one parameter",
		},
		{
			desc:        "MAX_STALENESS with extra parameters",
			input:       "MAX_STALENESS 30s extra",
			expectError: true,
			errorMsg:    "MAX_STALENESS accepts at most one parameter",
		},
		{
			desc:        "EXACT_STALENESS with extra parameters",
			input:       "EXACT_STALENESS 1h extra param",
			expectError: true,
			errorMsg:    "EXACT_STALENESS accepts at most one parameter",
		},
	}
	
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := parseTimestampBound(test.input)
			
			if test.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if test.errorMsg != "" && err.Error() != test.errorMsg {
					t.Errorf("expected error message %q, got %q", test.errorMsg, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			
			// Compare the timestamp bounds
			if !timestampBoundsEqual(got, test.want) {
				t.Errorf("expected %v, got %v", test.want, got)
			}
		})
	}
}

// Helper function to compare TimestampBound values
func timestampBoundsEqual(a, b spanner.TimestampBound) bool {
	// Since TimestampBound doesn't have an exported comparison method,
	// we compare their string representations as a workaround
	return a.String() == b.String()
}