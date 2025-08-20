package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
)

// Helper Functions

// assertError checks if an error occurred as expected
// If errorMsg is non-empty, an error is expected and must contain the message
// If errorMsg is empty, no error should occur
func assertError(t *testing.T, err error, errorMsg string) {
	t.Helper()
	if errorMsg != "" {
		// Error is expected
		if err == nil {
			t.Errorf("expected error but got none")
		} else if !strings.Contains(err.Error(), errorMsg) {
			t.Errorf("expected error containing %q, got %v", errorMsg, err)
		}
	} else {
		// No error expected
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// assertNoError checks that no error occurred
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// testSystemVariableSetGet is a helper function to test system variable set/get operations
func testSystemVariableSetGet(t *testing.T, setFunc func(*systemVariables, string, string) error, methodName string, testCase struct {
	desc                               string
	sysVars                            *systemVariables
	name                               string
	value                              string
	want                               map[string]string
	unimplementedSet, unimplementedGet bool
},
) {
	sysVars := testCase.sysVars
	if sysVars == nil {
		sysVars = newSystemVariablesWithDefaultsForTest()
	}

	// Only call Set if value is provided or if testing unimplemented setter
	if testCase.value != "" || testCase.unimplementedSet {
		err := setFunc(sysVars, testCase.name, testCase.value)
		if !testCase.unimplementedSet {
			assertNoError(t, err)
		} else {
			var e errSetterUnimplemented
			// Accept errSetterReadOnly from implementation
			if !errors.As(err, &e) && !errors.Is(err, errSetterReadOnly) {
				t.Errorf("sysVars.%s is skipped, but implemented: %v", methodName, err)
			}
		}
	}

	got, err := sysVars.Get(testCase.name)
	if !testCase.unimplementedGet {
		assertNoError(t, err)
		if diff := cmp.Diff(testCase.want, got); diff != "" {
			t.Errorf("sysVars.Get() mismatch (-want +got):\n%s", diff)
		}
	} else {
		var e errGetterUnimplemented
		if !errors.As(err, &e) {
			t.Errorf("sysVars.Get is skipped, but implemented: %v", err)
		}
	}
}

// Helper function to compare TimestampBound values
func timestampBoundsEqual(a, b spanner.TimestampBound) bool {
	// Since TimestampBound doesn't have an exported comparison method,
	// we compare their string representations as a workaround
	return a.String() == b.String()
}

// Test system variables builder for cleaner test setup
type sysVarsBuilder struct{ sv *systemVariables }

func newTestSysVars() *sysVarsBuilder {
	return &sysVarsBuilder{sv: newSystemVariablesWithDefaultsForTest()}
}

func (b *sysVarsBuilder) withReadTimestamp(t time.Time) *sysVarsBuilder {
	b.sv.ReadTimestamp = t
	return b
}

func (b *sysVarsBuilder) withCommitTimestamp(t time.Time) *sysVarsBuilder {
	b.sv.CommitTimestamp = t
	return b
}

func (b *sysVarsBuilder) withCommitResponse(r *sppb.CommitResponse) *sysVarsBuilder {
	b.sv.CommitResponse = r
	return b
}

func (b *sysVarsBuilder) withProject(p string) *sysVarsBuilder     { b.sv.Project = p; return b }
func (b *sysVarsBuilder) withInstance(i string) *sysVarsBuilder    { b.sv.Instance = i; return b }
func (b *sysVarsBuilder) withDatabase(d string) *sysVarsBuilder    { b.sv.Database = d; return b }
func (b *sysVarsBuilder) withHistoryFile(f string) *sysVarsBuilder { b.sv.HistoryFile = f; return b }
func (b *sysVarsBuilder) withHost(h string) *sysVarsBuilder        { b.sv.Host = h; return b }
func (b *sysVarsBuilder) withPort(p int) *sysVarsBuilder           { b.sv.Port = p; return b }
func (b *sysVarsBuilder) withRole(r string) *sysVarsBuilder        { b.sv.Role = r; return b }
func (b *sysVarsBuilder) withInsecure(i bool) *sysVarsBuilder      { b.sv.Insecure = i; return b }
func (b *sysVarsBuilder) withLogGrpc(l bool) *sysVarsBuilder       { b.sv.LogGrpc = l; return b }
func (b *sysVarsBuilder) withEnableADCPlus(e bool) *sysVarsBuilder { b.sv.EnableADCPlus = e; return b }
func (b *sysVarsBuilder) withMCP(m bool) *sysVarsBuilder           { b.sv.MCP = m; return b }

func (b *sysVarsBuilder) withImpersonateServiceAccount(a string) *sysVarsBuilder {
	b.sv.ImpersonateServiceAccount = a
	return b
}

func (b *sysVarsBuilder) withDirectedRead(d *sppb.DirectedReadOptions) *sysVarsBuilder {
	b.sv.DirectedRead = d
	return b
}

func (b *sysVarsBuilder) withSession(s *Session) *sysVarsBuilder { b.sv.CurrentSession = s; return b }
func (b *sysVarsBuilder) build() *systemVariables                { return b.sv }

// Proto Descriptor File Tests (Array/List Variables)

func TestSystemVariables_ProtoDescriptorFiles(t *testing.T) {
	t.Parallel()

	t.Run("AddCLIProtoDescriptorFile", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			desc     string
			values   []string
			errorMsg string
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
				desc:     "non-existent file",
				values:   []string{"testdata/protos/non_existent.pb"},
				errorMsg: "no such file or directory",
			},
			{
				desc:     "invalid proto file",
				values:   []string{"testdata/invalid_protos/invalid.txt"},
				errorMsg: "error on unmarshal proto descriptor-file",
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
				desc:     "invalid proto source file",
				values:   []string{"testdata/invalid_protos/invalid_proto.proto"},
				errorMsg: "invalid_proto.proto:",
			},
			{
				desc:     "mix of valid and invalid files",
				values:   []string{"testdata/protos/order_descriptors.pb", "testdata/protos/non_existent.pb"},
				errorMsg: "no such file or directory",
			},
		}
		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				t.Parallel()
				var sysVars systemVariables
				var lastErr error
				for _, value := range test.values {
					if err := sysVars.AddFromSimple("CLI_PROTO_DESCRIPTOR_FILE", value); err != nil {
						lastErr = err
						if test.errorMsg == "" {
							t.Errorf("unexpected error for value %q: %v", value, err)
						}
						if test.errorMsg != "" {
							break // Exit the loop immediately when expecting an error
						}
					}
				}

				assertError(t, lastErr, test.errorMsg)
			})
		}
	})

	t.Run("ReadFileDescriptorProtoFromFile", func(t *testing.T) {
		// Don't run this test in parallel because it uses an HTTP test server
		// that needs to be available for all subtests

		// Use t.TempDir() for dynamic test files - automatically cleaned up
		tempDir := t.TempDir()

		// Create a test file with permission issues
		permissionTestFile := filepath.Join(tempDir, "permission_test.pb")
		if err := os.WriteFile(permissionTestFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("Failed to create permission test file: %v", err)
		}
		// Change permissions to 0000 after creation
		if err := os.Chmod(permissionTestFile, 0o000); err != nil {
			t.Fatalf("Failed to change permissions for test file: %v", err)
		}

		// Create a test HTTP server for URL tests
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/non_existent.pb":
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("404 Not Found"))
			default:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("this is not a valid proto"))
			}
		}))
		defer httpServer.Close()

		// Create a large descriptor file for testing
		largeFile := filepath.Join(tempDir, "large_test.pb")
		if err := os.WriteFile(largeFile, make([]byte, 1024*1024), 0o644); err != nil {
			t.Fatalf("Failed to create large test file: %v", err)
		}

		tests := []struct {
			desc     string
			filename string
			errorMsg string
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
				desc:     "non-existent file",
				filename: "testdata/protos/non_existent.pb",
				errorMsg: "no such file or directory",
			},
			{
				desc:     "permission denied file",
				filename: permissionTestFile,
				errorMsg: "permission denied",
			},
			{
				desc:     "invalid proto binary file",
				filename: "testdata/invalid_protos/invalid.txt",
				errorMsg: "error on unmarshal proto descriptor-file",
			},
			{
				desc:     "empty file",
				filename: "testdata/invalid_protos/empty.pb",
				// Empty files unmarshal successfully to empty FileDescriptorSet
			},
			{
				desc:     "invalid proto source file",
				filename: "testdata/invalid_protos/invalid_proto.proto",
				errorMsg: "invalid_proto.proto:",
			},
			{
				desc:     "directory instead of file",
				filename: "testdata/protos/",
				errorMsg: "is a directory",
			},
			{
				desc:     "large invalid binary file",
				filename: largeFile,
				errorMsg: "error on unmarshal proto descriptor-file",
			},
			{
				desc:     "HTTP URL - invalid proto",
				filename: httpServer.URL + "/invalid.pb",
				errorMsg: "error on unmarshal proto descriptor-file",
			},
			{
				desc:     "HTTP URL - non-existent (404)",
				filename: httpServer.URL + "/non_existent.pb",
				errorMsg: "failed to fetch proto descriptor",
			},
		}

		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				// Don't run in parallel - shares httpServer and tempDir with parent test
				// Skip permission test on Windows as os.Chmod is not effective
				if test.desc == "permission denied file" && runtime.GOOS == "windows" {
					t.Skip("Skipping permission test on Windows as os.Chmod is not effective")
				}

				fds, err := readFileDescriptorProtoFromFile(test.filename)

				assertError(t, err, test.errorMsg)
				if err == nil && fds == nil {
					t.Errorf("expected non-nil FileDescriptorSet")
				}
			})
		}
	})

	t.Run("Integration", func(t *testing.T) {
		t.Parallel()
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
				t.Parallel()
				var sysVars systemVariables

				// Add descriptor files
				for _, file := range test.descriptorFiles {
					if err := sysVars.AddFromSimple("CLI_PROTO_DESCRIPTOR_FILE", file); err != nil {
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
	})

	t.Run("EdgeCases", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			desc     string
			varName  string
			value    string
			errorMsg string
		}{
			{desc: "empty string value", varName: "CLI_PROTO_DESCRIPTOR_FILE", value: `""`, errorMsg: "no such file or directory"},
			{desc: "spaces only value", varName: "CLI_PROTO_DESCRIPTOR_FILE", value: `"   "`, errorMsg: "no such file or directory"},
			{desc: "non-existent path with parent directory traversal", varName: "CLI_PROTO_DESCRIPTOR_FILE", value: `"../does_not_exist/non_existent_file.pb"`, errorMsg: "no such file or directory"},
		}

		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				t.Parallel()
				sysVars := newSystemVariablesWithDefaultsForTest()
				err := sysVars.AddFromGoogleSQL(test.varName, test.value)

				assertError(t, err, test.errorMsg)
			})
		}
	})
}

// String Variable Tests

func TestSystemVariables_StringTypes(t *testing.T) {
	t.Parallel()

	t.Run("CLI_ENDPOINT_Setter", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			desc        string
			value       string
			wantHost    string
			wantPort    int
			errContains string
		}{
			{desc: "valid endpoint", value: "example.com:443", wantHost: "example.com", wantPort: 443},
			{desc: "endpoint with IPv6", value: "[2001:db8::1]:443", wantHost: "2001:db8::1", wantPort: 443},
			{desc: "invalid endpoint - no port", value: "example.com", errContains: "invalid endpoint format"},
			{desc: "empty endpoint clears host and port", value: "", wantHost: "", wantPort: 0},
			{desc: "invalid endpoint - bare IPv6 without port", value: "2001:db8::1", errContains: "invalid endpoint format"},
			{desc: "invalid endpoint - non-numeric port", value: "example.com:abc", errContains: "invalid port in endpoint"},
		}

		for _, tt := range tests {
			t.Run(tt.desc, func(t *testing.T) {
				t.Parallel()
				sysVars := newSystemVariablesWithDefaultsForTest()
				err := sysVars.SetFromSimple("CLI_ENDPOINT", tt.value)
				assertError(t, err, tt.errContains)
				if err != nil {
					return
				}
				if sysVars.Host != tt.wantHost {
					t.Errorf("Host = %q, want %q", sysVars.Host, tt.wantHost)
				}
				if sysVars.Port != tt.wantPort {
					t.Errorf("Port = %d, want %d", sysVars.Port, tt.wantPort)
				}
			})
		}
	})

	t.Run("StatementTimeout", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			desc     string
			value    string
			want     time.Duration
			errorMsg string
		}{
			{desc: "valid_seconds", value: "30s", want: 30 * time.Second},
			{desc: "valid_minutes", value: "5m", want: 5 * time.Minute},
			{desc: "valid_hours", value: "1h", want: 1 * time.Hour},
			{desc: "valid_mixed", value: "1h30m", want: 90 * time.Minute},
			{desc: "valid_zero", value: "0s", want: 0},
			{desc: "invalid_format", value: "invalid", errorMsg: "invalid duration"},
			{desc: "negative_value", value: "-30s", errorMsg: "duration -30s is less than minimum 0s"},
			{desc: "empty_string", value: "", errorMsg: "invalid duration"},
		}

		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				t.Parallel()
				sysVars := newSystemVariablesWithDefaultsForTest()
				err := sysVars.SetFromSimple("STATEMENT_TIMEOUT", test.value)

				assertError(t, err, test.errorMsg)
				if err != nil {
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
				assertNoError(t, err)

				expected := map[string]string{"STATEMENT_TIMEOUT": test.want.String()}
				if diff := cmp.Diff(expected, result); diff != "" {
					t.Errorf("STATEMENT_TIMEOUT getter mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})
}

// Boolean Variable Tests

func TestSystemVariables_BooleanTypes(t *testing.T) {
	t.Parallel()

	t.Run("CLI_SKIP_COLUMN_NAMES", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			desc     string
			value    string
			want     bool
			errorMsg string
		}{
			{desc: "set to true", value: "TRUE", want: true},
			{desc: "set to false", value: "FALSE", want: false},
			{desc: "set to 1", value: "1", want: true},
			{desc: "set to 0", value: "0", want: false},
			{desc: "invalid value", value: "invalid", errorMsg: "invalid syntax"},
		}

		for _, tt := range tests {
			t.Run(tt.desc, func(t *testing.T) {
				t.Parallel()
				sysVars := newSystemVariablesWithDefaultsForTest()
				err := sysVars.SetFromSimple("CLI_SKIP_COLUMN_NAMES", tt.value)

				assertError(t, err, tt.errorMsg)
				if err != nil {
					return
				}

				if sysVars.SkipColumnNames != tt.want {
					t.Errorf("expected SkipColumnNames to be %v, got %v", tt.want, sysVars.SkipColumnNames)
				}

				// Test GET
				got, err := sysVars.Get("CLI_SKIP_COLUMN_NAMES")
				assertNoError(t, err)

				expectedStr := "FALSE"
				if tt.want {
					expectedStr = "TRUE"
				}
				if got["CLI_SKIP_COLUMN_NAMES"] != expectedStr {
					t.Errorf("expected Get to return %s, got %s", expectedStr, got["CLI_SKIP_COLUMN_NAMES"])
				}
			})
		}
	})
}

// Enum Variable Tests

func TestSystemVariables_EnumTypes(t *testing.T) {
	t.Parallel()

	t.Run("DefaultIsolationLevel", func(t *testing.T) {
		t.Parallel()
		// Use SetFromSimple for this test as it's testing string values that would
		// come from config files or command-line flags, not GoogleSQL expressions
		tests := []struct {
			value string
			want  sppb.TransactionOptions_IsolationLevel
		}{
			{value: "REPEATABLE_READ", want: sppb.TransactionOptions_REPEATABLE_READ},
			{value: "repeatable_read", want: sppb.TransactionOptions_REPEATABLE_READ},
			{value: "serializable", want: sppb.TransactionOptions_SERIALIZABLE},
			{value: "SERIALIZABLE", want: sppb.TransactionOptions_SERIALIZABLE},
			{value: "ISOLATION_LEVEL_UNSPECIFIED", want: sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED},
		}
		for _, test := range tests {
			t.Run(test.value, func(t *testing.T) {
				t.Parallel()
				sysVars := newSystemVariablesWithDefaultsForTest()
				err := sysVars.SetFromSimple("DEFAULT_ISOLATION_LEVEL", test.value)
				assertNoError(t, err)

				if sysVars.DefaultIsolationLevel != test.want {
					t.Errorf("DefaultIsolationLevel should be %v, but %v", test.want, sysVars.DefaultIsolationLevel)
				}
			})
		}
	})
}

// Time and Duration Variable Tests

func TestSystemVariables_TimeAndDuration(t *testing.T) {
	t.Parallel()

	t.Run("ParseTimestampBound", func(t *testing.T) {
		t.Parallel()
		// Test valid timestamp bounds
		validTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		validTimeStr := "2024-01-01T00:00:00Z"

		tests := []struct {
			desc     string
			input    string
			want     spanner.TimestampBound
			errorMsg string
		}{
			// Valid cases - all 5 timestamp bound types
			{desc: "STRONG read", input: "STRONG", want: spanner.StrongRead()},
			{desc: "MIN_READ_TIMESTAMP with valid timestamp", input: "MIN_READ_TIMESTAMP " + validTimeStr, want: spanner.MinReadTimestamp(validTime)},
			{desc: "READ_TIMESTAMP with valid timestamp", input: "READ_TIMESTAMP " + validTimeStr, want: spanner.ReadTimestamp(validTime)},
			{desc: "MAX_STALENESS with valid duration", input: "MAX_STALENESS 30s", want: spanner.MaxStaleness(30 * time.Second)},
			{desc: "EXACT_STALENESS with valid duration", input: "EXACT_STALENESS 1h30m", want: spanner.ExactStaleness(90 * time.Minute)},

			// Case insensitivity tests
			{desc: "lowercase strong", input: "strong", want: spanner.StrongRead()},
			{desc: "mixed case Strong", input: "Strong", want: spanner.StrongRead()},
			{desc: "lowercase min_read_timestamp", input: "min_read_timestamp " + validTimeStr, want: spanner.MinReadTimestamp(validTime)},
			{desc: "mixed case Min_Read_Timestamp", input: "Min_Read_Timestamp " + validTimeStr, want: spanner.MinReadTimestamp(validTime)},
			{desc: "lowercase read_timestamp", input: "read_timestamp " + validTimeStr, want: spanner.ReadTimestamp(validTime)},
			{desc: "lowercase max_staleness", input: "max_staleness 15s", want: spanner.MaxStaleness(15 * time.Second)},
			{desc: "lowercase exact_staleness", input: "exact_staleness 45m", want: spanner.ExactStaleness(45 * time.Minute)},

			// Error cases - invalid timestamps
			{desc: "MIN_READ_TIMESTAMP with invalid timestamp format", input: "MIN_READ_TIMESTAMP invalid-date", errorMsg: "parsing time"},
			{desc: "MIN_READ_TIMESTAMP with invalid month", input: "MIN_READ_TIMESTAMP 2024-13-01T00:00:00Z", errorMsg: "parsing time"},
			{desc: "READ_TIMESTAMP with invalid timestamp format", input: "READ_TIMESTAMP not-a-timestamp", errorMsg: "parsing time"},
			{desc: "READ_TIMESTAMP with malformed RFC3339", input: "READ_TIMESTAMP 2024-01-01", errorMsg: "parsing time"},

			// Error cases - invalid durations
			{desc: "MAX_STALENESS with invalid duration", input: "MAX_STALENESS invalid-duration", errorMsg: "invalid duration"},
			{
				desc:     "MAX_STALENESS with negative duration",
				input:    "MAX_STALENESS -30s",
				errorMsg: "staleness duration \"-30s\" must be non-negative",
			},
			{desc: "EXACT_STALENESS with invalid duration", input: "EXACT_STALENESS not-a-duration", errorMsg: "invalid duration"},
			{
				desc:     "EXACT_STALENESS with negative duration",
				input:    "EXACT_STALENESS -1h",
				errorMsg: "staleness duration \"-1h\" must be non-negative",
			},

			// Error cases - unknown staleness types
			{
				desc:     "unknown staleness type",
				input:    "UNKNOWN_TYPE 30s",
				errorMsg: "unknown staleness: UNKNOWN_TYPE",
			},
			{
				desc:     "empty string",
				input:    "",
				errorMsg: "unknown staleness: \"\"",
			},
			{
				desc:     "random text",
				input:    "some random text",
				errorMsg: "some accepts at most one parameter",
			},

			// Edge cases
			{desc: "STRONG with extra text should fail", input: "STRONG extra text", errorMsg: "STRONG accepts at most one parameter"},
			{
				desc:     "MIN_READ_TIMESTAMP missing timestamp",
				input:    "MIN_READ_TIMESTAMP",
				errorMsg: "MIN_READ_TIMESTAMP requires a timestamp parameter",
			},
			{
				desc:     "READ_TIMESTAMP missing timestamp",
				input:    "READ_TIMESTAMP",
				errorMsg: "READ_TIMESTAMP requires a timestamp parameter",
			},
			{
				desc:     "MAX_STALENESS missing duration",
				input:    "MAX_STALENESS",
				errorMsg: "MAX_STALENESS requires a duration parameter",
			},
			{
				desc:     "EXACT_STALENESS missing duration",
				input:    "EXACT_STALENESS",
				errorMsg: "EXACT_STALENESS requires a duration parameter",
			},
			{desc: "extra whitespace before timestamp", input: "MIN_READ_TIMESTAMP   " + validTimeStr, want: spanner.MinReadTimestamp(validTime)},
			{desc: "tabs instead of spaces", input: "MAX_STALENESS	60s", want: spanner.MaxStaleness(60 * time.Second)},
			{desc: "zero duration for MAX_STALENESS", input: "MAX_STALENESS 0s", want: spanner.MaxStaleness(0)},
			{desc: "very large duration", input: "EXACT_STALENESS 999999h", want: spanner.ExactStaleness(999999 * time.Hour)},

			// Extra parameter validation tests
			{desc: "MIN_READ_TIMESTAMP with extra parameters", input: "MIN_READ_TIMESTAMP " + validTimeStr + " extra", errorMsg: "MIN_READ_TIMESTAMP accepts at most one parameter"},
			{desc: "READ_TIMESTAMP with extra parameters", input: "READ_TIMESTAMP " + validTimeStr + " extra param", errorMsg: "READ_TIMESTAMP accepts at most one parameter"},
			{desc: "MAX_STALENESS with extra parameters", input: "MAX_STALENESS 30s extra", errorMsg: "MAX_STALENESS accepts at most one parameter"},
			{desc: "EXACT_STALENESS with extra parameters", input: "EXACT_STALENESS 1h extra param", errorMsg: "EXACT_STALENESS accepts at most one parameter"},
		}

		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				t.Parallel()
				got, err := parseTimestampBound(test.input)

				assertError(t, err, test.errorMsg)

				if err == nil {
					// Compare the timestamp bounds
					if !timestampBoundsEqual(got, test.want) {
						t.Errorf("expected %v, got %v", test.want, got)
					}
				}
			})
		}
	})
}

// Special Behavior Tests

func TestSystemVariables_SpecialBehaviors(t *testing.T) {
	t.Parallel()

	t.Run("SessionInitOnlyVariables", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name           string
			variableName   string
			variableCase   string // Different casing for variable name test
			initialValue   string
			setValue       string
			hasSession     bool
			detached       bool // Whether session is detached (no client)
			expectedErrMsg string
		}{
			{
				name:         "set before session creation - uppercase",
				variableName: "CLI_ENABLE_ADC_PLUS",
				variableCase: "CLI_ENABLE_ADC_PLUS",
				initialValue: "true",
				setValue:     "false",
				hasSession:   false,
			},
			{
				name:         "set before session creation - lowercase",
				variableName: "CLI_ENABLE_ADC_PLUS",
				variableCase: "cli_enable_adc_plus",
				initialValue: "true",
				setValue:     "false",
				hasSession:   false,
			},
			{
				name:         "set before session creation - mixed case",
				variableName: "CLI_ENABLE_ADC_PLUS",
				variableCase: "Cli_Enable_Adc_Plus",
				initialValue: "true",
				setValue:     "false",
				hasSession:   false,
			},
			{
				name:           "change after session creation",
				variableName:   "CLI_ENABLE_ADC_PLUS",
				variableCase:   "CLI_ENABLE_ADC_PLUS",
				initialValue:   "true",
				setValue:       "false",
				hasSession:     true,
				expectedErrMsg: "CLI_ENABLE_ADC_PLUS cannot be changed after session creation",
			},
			{
				name:           "change after session with lowercase variable name",
				variableName:   "CLI_ENABLE_ADC_PLUS",
				variableCase:   "cli_enable_adc_plus",
				initialValue:   "true",
				setValue:       "false",
				hasSession:     true,
				expectedErrMsg: "CLI_ENABLE_ADC_PLUS cannot be changed after session creation",
			},
			{
				name:         "non-session-init-only variable can be changed",
				variableName: "CLI_ASYNC_DDL",
				variableCase: "CLI_ASYNC_DDL",
				initialValue: "false",
				setValue:     "true",
				hasSession:   true,
			},
			{
				name:           "change after detached session creation should fail",
				variableName:   "CLI_ENABLE_ADC_PLUS",
				variableCase:   "CLI_ENABLE_ADC_PLUS",
				initialValue:   "true",
				setValue:       "false",
				hasSession:     true,
				detached:       true,
				expectedErrMsg: "CLI_ENABLE_ADC_PLUS cannot be changed after session creation",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				// Create a new systemVariables instance
				sv := &systemVariables{
					EnableADCPlus: tt.initialValue == "true",
					AsyncDDL:      false,
				}

				// Simulate session creation if needed
				if tt.hasSession {
					sv.CurrentSession = &Session{}
					// Set client for non-detached sessions
					if !tt.detached {
						sv.CurrentSession.client = &spanner.Client{} // Mock client to simulate initialized session
					}
				}

				// Test Set operation
				err := sv.SetFromSimple(tt.variableCase, tt.setValue)

				// Check error expectation
				assertError(t, err, tt.expectedErrMsg)
				if err != nil {
					return
				}

				// Verify the value was set correctly (only for non-session cases)
				switch tt.variableName {
				case "CLI_ENABLE_ADC_PLUS":
					expectedValue := tt.setValue == "true" || tt.setValue == "TRUE"
					if !tt.hasSession {
						// Value should be updated
						if sv.EnableADCPlus != expectedValue {
							t.Errorf("expected EnableADCPlus to be %v, got %v", expectedValue, sv.EnableADCPlus)
						}
					}
				case "CLI_ASYNC_DDL":
					expectedValue := tt.setValue == "true" || tt.setValue == "TRUE"
					if sv.AsyncDDL != expectedValue {
						t.Errorf("expected AsyncDDL to be %v, got %v", expectedValue, sv.AsyncDDL)
					}
				}

				// Additional test: verify Get returns the correct value
				values, err := sv.Get(tt.variableName)
				assertNoError(t, err)

				// Check that the value returned by Get matches expectation
				gotValue := values[tt.variableName]
				var expectedGetValue string
				switch tt.variableName {
				case "CLI_ENABLE_ADC_PLUS":
					if sv.EnableADCPlus {
						expectedGetValue = "TRUE"
					} else {
						expectedGetValue = "FALSE"
					}
				case "CLI_ASYNC_DDL":
					if sv.AsyncDDL {
						expectedGetValue = "TRUE"
					} else {
						expectedGetValue = "FALSE"
					}
				}

				if gotValue != expectedGetValue {
					t.Errorf("Get(%s) returned %q, expected %q", tt.variableName, gotValue, expectedGetValue)
				}
			})
		}
	})
}

// Comprehensive Set/Get Operation Tests

func TestSystemVariables_SetGetOperations(t *testing.T) {
	t.Parallel()

	t.Run("SimpleMode", func(t *testing.T) {
		t.Parallel()
		// Test system variables using Simple mode (CLI flags, config files)
		tests := []struct {
			desc                               string
			sysVars                            *systemVariables
			name                               string
			value                              string
			want                               map[string]string
			unimplementedSet, unimplementedGet bool
		}{
			// Java-spanner compatible variables
			{
				desc: "READ_TIMESTAMP", name: "READ_TIMESTAMP", unimplementedSet: true,
				sysVars: newTestSysVars().withReadTimestamp(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)).build(),
				want:    singletonMap("READ_TIMESTAMP", "1970-01-01T00:00:00Z"),
			},
			{
				desc: "COMMIT_TIMESTAMP", name: "COMMIT_TIMESTAMP", unimplementedSet: true,
				sysVars: newTestSysVars().withCommitTimestamp(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)).build(),
				want:    singletonMap("COMMIT_TIMESTAMP", "1970-01-01T00:00:00Z"),
			},
			{
				desc: "COMMIT_RESPONSE", name: "COMMIT_RESPONSE", unimplementedSet: true,
				sysVars: newTestSysVars().
					withCommitTimestamp(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)).
					withCommitResponse(&sppb.CommitResponse{CommitStats: &sppb.CommitResponse_CommitStats{MutationCount: 10}}).
					build(),
				want: map[string]string{"COMMIT_TIMESTAMP": "1970-01-01T00:00:00Z", "MUTATION_COUNT": "10"},
			},

			// CLI_* variables
			{
				desc: "CLI_VERSION", name: "CLI_VERSION", unimplementedSet: true,
				want: singletonMap("CLI_VERSION", getVersion()),
			},
			{
				desc: "CLI_PROJECT", name: "CLI_PROJECT", unimplementedSet: true,
				sysVars: newTestSysVars().withProject("test-project").build(),
				want:    singletonMap("CLI_PROJECT", "test-project"),
			},
			{
				desc: "CLI_INSTANCE", name: "CLI_INSTANCE", unimplementedSet: true,
				sysVars: newTestSysVars().withInstance("test-instance").build(),
				want:    singletonMap("CLI_INSTANCE", "test-instance"),
			},
			{
				desc: "CLI_DATABASE", name: "CLI_DATABASE", unimplementedSet: true,
				sysVars: newTestSysVars().withDatabase("test-database").build(),
				want:    singletonMap("CLI_DATABASE", "test-database"),
			},
			{
				desc: "CLI_HISTORY_FILE", name: "CLI_HISTORY_FILE", unimplementedSet: true,
				sysVars: newTestSysVars().withHistoryFile("/tmp/spanner_mycli_readline.tmp").build(),
				want:    singletonMap("CLI_HISTORY_FILE", "/tmp/spanner_mycli_readline.tmp"),
			},
			{
				desc: "CLI_ENDPOINT getter", name: "CLI_ENDPOINT",
				sysVars: newTestSysVars().withHost("localhost").withPort(9010).build(),
				want:    singletonMap("CLI_ENDPOINT", "localhost:9010"),
			},
			{
				desc: "CLI_ENDPOINT setter", name: "CLI_ENDPOINT", value: "example.com:443",
				want: singletonMap("CLI_ENDPOINT", "example.com:443"),
			},
			{
				desc: "CLI_ENDPOINT setter with IPv6", name: "CLI_ENDPOINT", value: "[2001:db8::1]:443",
				want: singletonMap("CLI_ENDPOINT", "[2001:db8::1]:443"),
			},
			{
				desc: "CLI_HOST", name: "CLI_HOST", unimplementedSet: true,
				sysVars: newTestSysVars().withHost("example.com").build(),
				want:    singletonMap("CLI_HOST", "example.com"),
			},
			{
				desc: "CLI_PORT", name: "CLI_PORT", unimplementedSet: true,
				sysVars: newTestSysVars().withPort(443).build(),
				want:    singletonMap("CLI_PORT", "443"),
			},
			{
				desc: "CLI_DIRECT_READ", name: "CLI_DIRECT_READ", unimplementedSet: true,
				sysVars: newTestSysVars().withDirectedRead(&sppb.DirectedReadOptions{Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
						{Type: sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE, Location: "asia-northeast2"},
					}},
				}}).build(),
				want: singletonMap("CLI_DIRECT_READ", "asia-northeast2:READ_WRITE"),
			},
			// Java-spanner compatible boolean variables
			{
				desc: "READONLY", name: "READONLY", value: "TRUE",
				want: singletonMap("READONLY", "TRUE"),
			},
			{
				desc: "AUTO_PARTITION_MODE", name: "AUTO_PARTITION_MODE", value: "TRUE",
				want: singletonMap("AUTO_PARTITION_MODE", "TRUE"),
			},
			{
				desc: "AUTOCOMMIT", name: "AUTOCOMMIT", unimplementedSet: true, unimplementedGet: true,
				value: "FALSE",
				want:  singletonMap("AUTOCOMMIT", "FALSE"),
			},
			{
				desc: "RETRY_ABORTS_INTERNALLY", name: "RETRY_ABORTS_INTERNALLY",
				unimplementedSet: true, unimplementedGet: true,
			},
			{
				desc: "EXCLUDE_TXN_FROM_CHANGE_STREAMS", name: "EXCLUDE_TXN_FROM_CHANGE_STREAMS", value: "TRUE",
				want: singletonMap("EXCLUDE_TXN_FROM_CHANGE_STREAMS", "TRUE"),
			},
			{
				desc: "AUTO_BATCH_DML", name: "AUTO_BATCH_DML", value: "TRUE",
				want: singletonMap("AUTO_BATCH_DML", "TRUE"),
			},
			{
				desc: "DATA_BOOST_ENABLED", name: "DATA_BOOST_ENABLED", value: "TRUE",
				want: singletonMap("DATA_BOOST_ENABLED", "TRUE"),
			},

			// CLI_* boolean variables
			{
				desc: "CLI_VERBOSE", name: "CLI_VERBOSE", value: "TRUE",
				want: singletonMap("CLI_VERBOSE", "TRUE"),
			},
			{
				desc: "CLI_ECHO_EXECUTED_DDL", name: "CLI_ECHO_EXECUTED_DDL", value: "TRUE",
				want: singletonMap("CLI_ECHO_EXECUTED_DDL", "TRUE"),
			},
			{
				desc: "CLI_ECHO_INPUT", name: "CLI_ECHO_INPUT", value: "TRUE",
				want: singletonMap("CLI_ECHO_INPUT", "TRUE"),
			},
			{
				desc: "CLI_EXPLAIN_FORMAT", name: "CLI_EXPLAIN_FORMAT", value: "CURRENT",
				want: singletonMap("CLI_EXPLAIN_FORMAT", "CURRENT"),
			},
			{
				desc: "CLI_USE_PAGER", name: "CLI_USE_PAGER", value: "TRUE",
				want: singletonMap("CLI_USE_PAGER", "TRUE"),
			},
			{
				desc: "CLI_AUTOWRAP", name: "CLI_AUTOWRAP", value: "TRUE",
				want: singletonMap("CLI_AUTOWRAP", "TRUE"),
			},
			{
				desc: "CLI_ENABLE_HIGHLIGHT", name: "CLI_ENABLE_HIGHLIGHT", value: "TRUE",
				want: singletonMap("CLI_ENABLE_HIGHLIGHT", "TRUE"),
			},
			{
				desc: "CLI_PROTOTEXT_MULTILINE", name: "CLI_PROTOTEXT_MULTILINE", value: "TRUE",
				want: singletonMap("CLI_PROTOTEXT_MULTILINE", "TRUE"),
			},
			{
				desc: "CLI_MARKDOWN_CODEBLOCK", name: "CLI_MARKDOWN_CODEBLOCK", value: "TRUE",
				want: singletonMap("CLI_MARKDOWN_CODEBLOCK", "TRUE"),
			},
			{
				desc: "CLI_LINT_PLAN", name: "CLI_LINT_PLAN", value: "TRUE",
				want: singletonMap("CLI_LINT_PLAN", "TRUE"),
			},
			{
				desc: "CLI_SKIP_COLUMN_NAMES", name: "CLI_SKIP_COLUMN_NAMES", value: "TRUE",
				want: singletonMap("CLI_SKIP_COLUMN_NAMES", "TRUE"),
			},
			{
				desc: "CLI_INSECURE", name: "CLI_INSECURE", unimplementedSet: true,
				sysVars: newTestSysVars().withInsecure(true).build(),
				want:    singletonMap("CLI_INSECURE", "TRUE"),
			},
			{
				desc: "CLI_LOG_GRPC", name: "CLI_LOG_GRPC", unimplementedSet: true,
				sysVars: newTestSysVars().withLogGrpc(true).build(),
				want:    singletonMap("CLI_LOG_GRPC", "TRUE"),
			},

			// Java-spanner compatible string variables
			{
				desc: "MAX_COMMIT_DELAY", name: "MAX_COMMIT_DELAY", value: "100ms",
				want: singletonMap("MAX_COMMIT_DELAY", "100ms"),
			},
			{
				desc: "READ_ONLY_STALENESS", name: "READ_ONLY_STALENESS", value: "STRONG",
				want: singletonMap("READ_ONLY_STALENESS", "STRONG"),
			},
			{
				desc: "OPTIMIZER_VERSION", name: "OPTIMIZER_VERSION", value: "LATEST",
				want: singletonMap("OPTIMIZER_VERSION", "LATEST"),
			},
			{
				desc: "OPTIMIZER_STATISTICS_PACKAGE", name: "OPTIMIZER_STATISTICS_PACKAGE", value: "test-package",
				want: singletonMap("OPTIMIZER_STATISTICS_PACKAGE", "test-package"),
			},
			{
				desc: "RPC_PRIORITY", name: "RPC_PRIORITY", value: "HIGH",
				want: singletonMap("RPC_PRIORITY", "HIGH"),
			},
			{
				desc: "STATEMENT_TAG", name: "STATEMENT_TAG", value: "test-statement",
				want: singletonMap("STATEMENT_TAG", "test-statement"),
			},
			{
				desc: "TRANSACTION_TAG", name: "TRANSACTION_TAG", value: "test-tag",
				sysVars: newTestSysVars().withSession(&Session{tc: &transactionContext{
					attrs: transactionAttributes{mode: transactionModePending},
				}}).build(),
				want: singletonMap("TRANSACTION_TAG", "test-tag"),
			},

			// CLI_* string variables
			{
				desc: "CLI_OUTPUT_TEMPLATE_FILE", name: "CLI_OUTPUT_TEMPLATE_FILE", value: "output_default.tmpl",
				want: singletonMap("CLI_OUTPUT_TEMPLATE_FILE", "output_default.tmpl"),
			},
			{
				desc: "CLI_ROLE", name: "CLI_ROLE", unimplementedSet: true,
				sysVars: newTestSysVars().withRole("test-role").build(),
				want:    singletonMap("CLI_ROLE", "test-role"),
			},
			{
				desc: "CLI_PROMPT", name: "CLI_PROMPT", value: "test-prompt",
				want: singletonMap("CLI_PROMPT", "test-prompt"),
			},
			{
				desc: "CLI_PROMPT2", name: "CLI_PROMPT2", value: "test-prompt2",
				want: singletonMap("CLI_PROMPT2", "test-prompt2"),
			},
			{
				desc: "CLI_ANALYZE_COLUMNS", name: "CLI_ANALYZE_COLUMNS", value: "name:{{.template}}:LEFT",
				want: singletonMap("CLI_ANALYZE_COLUMNS", "name:{{.template}}:LEFT"),
			},
			{
				desc: "CLI_INLINE_STATS", name: "CLI_INLINE_STATS", value: "name:{{.template}}",
				want: singletonMap("CLI_INLINE_STATS", "name:{{.template}}"),
			},
			{
				desc: "CLI_PARSE_MODE", name: "CLI_PARSE_MODE", value: "FALLBACK",
				want: singletonMap("CLI_PARSE_MODE", "FALLBACK"),
			},
			{
				desc: "CLI_LOG_LEVEL", name: "CLI_LOG_LEVEL", value: "INFO",
				want: singletonMap("CLI_LOG_LEVEL", "INFO"),
			},
			{
				desc: "CLI_VERTEXAI_MODEL", name: "CLI_VERTEXAI_MODEL", value: "test",
				want: singletonMap("CLI_VERTEXAI_MODEL", "test"),
			},
			{
				desc: "CLI_VERTEXAI_PROJECT", name: "CLI_VERTEXAI_PROJECT", value: "example-project",
				want: singletonMap("CLI_VERTEXAI_PROJECT", "example-project"),
			},
			{
				desc: "CLI_PROTO_DESCRIPTOR_FILE", name: "CLI_PROTO_DESCRIPTOR_FILE", value: "testdata/protos/order_descriptors.pb",
				want: singletonMap("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/order_descriptors.pb"),
			},
			{
				desc: "STATEMENT_TIMEOUT", name: "STATEMENT_TIMEOUT", value: "30s",
				want: singletonMap("STATEMENT_TIMEOUT", "30s"),
			},

			// Java-spanner compatible integer variables
			{
				desc: "MAX_PARTITIONED_PARALLELISM", name: "MAX_PARTITIONED_PARALLELISM", value: "10",
				want: singletonMap("MAX_PARTITIONED_PARALLELISM", "10"),
			},

			// CLI_* integer variables
			{
				desc: "CLI_TAB_WIDTH", name: "CLI_TAB_WIDTH", value: "4",
				want: singletonMap("CLI_TAB_WIDTH", "4"),
			},

			// Java-spanner compatible enum variables
			{
				desc: "AUTOCOMMIT_DML_MODE", name: "AUTOCOMMIT_DML_MODE", value: "TRANSACTIONAL",
				want: singletonMap("AUTOCOMMIT_DML_MODE", "TRANSACTIONAL"),
			},
			{
				desc: "DEFAULT_ISOLATION_LEVEL", name: "DEFAULT_ISOLATION_LEVEL", value: "SERIALIZABLE",
				want: singletonMap("DEFAULT_ISOLATION_LEVEL", "SERIALIZABLE"),
			},

			// CLI_* enum variables
			{
				desc: "CLI_FORMAT", name: "CLI_FORMAT", value: "TABLE",
				want: singletonMap("CLI_FORMAT", "TABLE"),
			},
			{
				desc: "CLI_DATABASE_DIALECT", name: "CLI_DATABASE_DIALECT",
				value: "GOOGLE_STANDARD_SQL",
				want:  singletonMap("CLI_DATABASE_DIALECT", "GOOGLE_STANDARD_SQL"),
			},
			{
				desc: "CLI_QUERY_MODE", name: "CLI_QUERY_MODE", value: "PROFILE",
				want: singletonMap("CLI_QUERY_MODE", "PROFILE"),
			},

			// New CLI_* variables added for Issue #243
			{
				desc: "CLI_ENABLE_PROGRESS_BAR", name: "CLI_ENABLE_PROGRESS_BAR", value: "TRUE",
				want: singletonMap("CLI_ENABLE_PROGRESS_BAR", "TRUE"),
			},
			{
				desc: "CLI_IMPERSONATE_SERVICE_ACCOUNT", name: "CLI_IMPERSONATE_SERVICE_ACCOUNT", unimplementedSet: true,
				sysVars: newTestSysVars().withImpersonateServiceAccount("test@example.com").build(),
				want:    singletonMap("CLI_IMPERSONATE_SERVICE_ACCOUNT", "test@example.com"),
			},
			{
				desc: "CLI_ENABLE_ADC_PLUS", name: "CLI_ENABLE_ADC_PLUS", value: "true",
				sysVars: newTestSysVars().withEnableADCPlus(false).build(), // Start with false to test setting
				want:    singletonMap("CLI_ENABLE_ADC_PLUS", "TRUE"),
			},
			{
				desc: "CLI_MCP", name: "CLI_MCP", unimplementedSet: true,
				sysVars: newTestSysVars().withMCP(true).build(),
				want:    singletonMap("CLI_MCP", "TRUE"),
			},
			{
				desc: "RETURN_COMMIT_STATS true", name: "RETURN_COMMIT_STATS", value: "TRUE",
				want: singletonMap("RETURN_COMMIT_STATS", "TRUE"),
			},
			{
				desc: "RETURN_COMMIT_STATS false", name: "RETURN_COMMIT_STATS", value: "FALSE",
				want: singletonMap("RETURN_COMMIT_STATS", "FALSE"),
			},
		}

		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				t.Parallel()
				testSystemVariableSetGet(t, (*systemVariables).SetFromSimple, "SetFromSimple", test)
			})
		}
	})

	t.Run("GoogleSQLMode", func(t *testing.T) {
		t.Parallel()
		// Test system variables using GoogleSQL mode (REPL, SQL scripts)
		tests := []struct {
			desc                               string
			sysVars                            *systemVariables
			name                               string
			value                              string
			want                               map[string]string
			unimplementedSet, unimplementedGet bool
		}{
			// String variables with GoogleSQL syntax
			{desc: "CLI_PROMPT with quoted string", name: "CLI_PROMPT", value: `"test-prompt"`, want: singletonMap("CLI_PROMPT", "test-prompt")},
			{desc: "CLI_PROMPT2 with quoted string", name: "CLI_PROMPT2", value: `"test-prompt2"`, want: singletonMap("CLI_PROMPT2", "test-prompt2")},
			{desc: "STATEMENT_TAG with quoted string", name: "STATEMENT_TAG", value: `"test-statement"`, want: singletonMap("STATEMENT_TAG", "test-statement")},
			{desc: "CLI_EXPLAIN_FORMAT with quoted string", name: "CLI_EXPLAIN_FORMAT", value: `"CURRENT"`, want: singletonMap("CLI_EXPLAIN_FORMAT", "CURRENT")},
			{desc: "OPTIMIZER_VERSION with quoted string", name: "OPTIMIZER_VERSION", value: `"LATEST"`, want: singletonMap("OPTIMIZER_VERSION", "LATEST")},
			{desc: "OPTIMIZER_STATISTICS_PACKAGE with quoted string", name: "OPTIMIZER_STATISTICS_PACKAGE", value: `"test-package"`, want: singletonMap("OPTIMIZER_STATISTICS_PACKAGE", "test-package")},
			// Boolean variables with GoogleSQL syntax
			{desc: "CLI_USE_PAGER with TRUE keyword", name: "CLI_USE_PAGER", value: "TRUE", want: singletonMap("CLI_USE_PAGER", "TRUE")},
			{desc: "CLI_USE_PAGER with FALSE keyword", name: "CLI_USE_PAGER", value: "FALSE", want: singletonMap("CLI_USE_PAGER", "FALSE")},
			// Duration with quoted string
			{desc: "STATEMENT_TIMEOUT with quoted duration", name: "STATEMENT_TIMEOUT", value: `"30s"`, want: singletonMap("STATEMENT_TIMEOUT", "30s")},
			{desc: "MAX_COMMIT_DELAY with quoted duration", name: "MAX_COMMIT_DELAY", value: `"100ms"`, want: singletonMap("MAX_COMMIT_DELAY", "100ms")},
		}

		for _, test := range tests {
			t.Run(test.desc, func(t *testing.T) {
				t.Parallel()
				testSystemVariableSetGet(t, (*systemVariables).SetFromGoogleSQL, "SetFromGoogleSQL", test)
			})
		}
	})
}
