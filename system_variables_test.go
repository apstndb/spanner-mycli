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

// testBooleanVariable tests boolean variables with both TRUE and FALSE values
func testBooleanVariable(t *testing.T, setFunc func(*systemVariables, string, string) error, name string) {
	t.Helper()
	for _, value := range []string{"TRUE", "FALSE"} {
		t.Run(name+"_"+value, func(t *testing.T) {
			t.Parallel()
			sysVars := newSystemVariablesWithDefaultsForTest()
			err := setFunc(sysVars, name, value)
			assertNoError(t, err)

			got, err := sysVars.Get(name)
			assertNoError(t, err)
			want := map[string]string{name: value}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("sysVars.Get() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// testStringVariable tests string variables with a given value
func testStringVariable(t *testing.T, setFunc func(*systemVariables, string, string) error, name, value string) {
	t.Helper()
	sysVars := newSystemVariablesWithDefaultsForTest()
	err := setFunc(sysVars, name, value)
	assertNoError(t, err)

	got, err := sysVars.Get(name)
	assertNoError(t, err)
	want := map[string]string{name: value}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sysVars.Get() mismatch (-want +got):\n%s", diff)
	}
}

// testReadOnlyVariable tests variables that only have getters
func testReadOnlyVariable(t *testing.T, setFunc func(*systemVariables, string, string) error, name string, sysVars *systemVariables, want map[string]string) {
	t.Helper()
	if sysVars == nil {
		sysVars = newSystemVariablesWithDefaultsForTest()
	}

	// Verify setter is unimplemented
	err := setFunc(sysVars, name, "dummy")
	var e errSetterUnimplemented
	if !errors.As(err, &e) && !errors.Is(err, errSetterReadOnly) {
		t.Errorf("sysVars setter for %s is skipped, but implemented: %v", name, err)
	}

	// Test getter
	got, err := sysVars.Get(name)
	assertNoError(t, err)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sysVars.Get() mismatch (-want +got):\n%s", diff)
	}
}

// testUnimplementedVariable tests variables that have neither setter nor getter implemented
func testUnimplementedVariable(t *testing.T, setFunc func(*systemVariables, string, string) error, name string) {
	t.Helper()
	sysVars := newSystemVariablesWithDefaultsForTest()

	// Verify setter is unimplemented
	err := setFunc(sysVars, name, "dummy")
	var e errSetterUnimplemented
	if !errors.As(err, &e) && !errors.Is(err, errSetterReadOnly) {
		t.Errorf("sysVars setter for %s is skipped, but implemented: %v", name, err)
	}

	// Verify getter is unimplemented
	_, err = sysVars.Get(name)
	var eg errGetterUnimplemented
	if !errors.As(err, &eg) {
		t.Errorf("sysVars getter for %s is skipped, but implemented: %v", name, err)
	}
}

// testSpecialVariable tests variables that need custom setup or validation
func testSpecialVariable(t *testing.T, setFunc func(*systemVariables, string, string) error, desc, name, value string, sysVars *systemVariables, want map[string]string) {
	t.Helper()
	if sysVars == nil {
		sysVars = newSystemVariablesWithDefaultsForTest()
	}

	if value != "" {
		err := setFunc(sysVars, name, value)
		assertNoError(t, err)
	}

	got, err := sysVars.Get(name)
	assertNoError(t, err)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("%s: sysVars.Get() mismatch (-want +got):\n%s", desc, diff)
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
		setFunc := (*systemVariables).SetFromSimple

		// Boolean variables - test both TRUE and FALSE automatically
		boolVars := []string{
			"READONLY", "AUTO_PARTITION_MODE", "EXCLUDE_TXN_FROM_CHANGE_STREAMS",
			"AUTO_BATCH_DML", "DATA_BOOST_ENABLED", "RETURN_COMMIT_STATS",
			"CLI_VERBOSE", "CLI_ECHO_EXECUTED_DDL", "CLI_ECHO_INPUT", "CLI_USE_PAGER",
			"CLI_AUTOWRAP", "CLI_ENABLE_HIGHLIGHT", "CLI_PROTOTEXT_MULTILINE",
			"CLI_MARKDOWN_CODEBLOCK", "CLI_LINT_PLAN", "CLI_SKIP_COLUMN_NAMES",
			"CLI_ENABLE_PROGRESS_BAR", "CLI_ENABLE_ADC_PLUS", "CLI_ASYNC_DDL",
		}
		for _, name := range boolVars {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				testBooleanVariable(t, setFunc, name)
			})
		}

		// String variables with simple values
		stringTests := map[string]string{
			"MAX_COMMIT_DELAY":             "100ms",
			"READ_ONLY_STALENESS":          "STRONG",
			"OPTIMIZER_VERSION":            "LATEST",
			"OPTIMIZER_STATISTICS_PACKAGE": "test-package",
			"RPC_PRIORITY":                 "HIGH",
			"STATEMENT_TAG":                "test-statement",
			"CLI_OUTPUT_TEMPLATE_FILE":     "output_default.tmpl",
			"CLI_PROMPT":                   "test-prompt",
			"CLI_PROMPT2":                  "test-prompt2",
			"CLI_ANALYZE_COLUMNS":          "name:{{.template}}:LEFT",
			"CLI_INLINE_STATS":             "name:{{.template}}",
			"CLI_PARSE_MODE":               "FALLBACK",
			"CLI_LOG_LEVEL":                "INFO",
			"CLI_VERTEXAI_MODEL":           "test",
			"CLI_VERTEXAI_PROJECT":         "example-project",
			"CLI_PROTO_DESCRIPTOR_FILE":    "testdata/protos/order_descriptors.pb",
			"STATEMENT_TIMEOUT":            "30s",
			"MAX_PARTITIONED_PARALLELISM":  "10",
			"CLI_TAB_WIDTH":                "4",
			"AUTOCOMMIT_DML_MODE":          "TRANSACTIONAL",
			"DEFAULT_ISOLATION_LEVEL":      "SERIALIZABLE",
			"CLI_FORMAT":                   "TABLE",
			"CLI_DATABASE_DIALECT":         "GOOGLE_STANDARD_SQL",
			"CLI_QUERY_MODE":               "PROFILE",
			"CLI_EXPLAIN_FORMAT":           "CURRENT",
		}
		for name, value := range stringTests {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				testStringVariable(t, setFunc, name, value)
			})
		}

		// CLI_ENDPOINT special cases
		t.Run("CLI_ENDPOINT_setter", func(t *testing.T) {
			t.Parallel()
			testStringVariable(t, setFunc, "CLI_ENDPOINT", "example.com:443")
		})
		t.Run("CLI_ENDPOINT_setter_IPv6", func(t *testing.T) {
			t.Parallel()
			testStringVariable(t, setFunc, "CLI_ENDPOINT", "[2001:db8::1]:443")
		})
		t.Run("CLI_ENDPOINT_getter", func(t *testing.T) {
			t.Parallel()
			sysVars := newTestSysVars().withHost("localhost").withPort(9010).build()
			testSpecialVariable(t, setFunc, "CLI_ENDPOINT getter", "CLI_ENDPOINT", "", sysVars,
				singletonMap("CLI_ENDPOINT", "localhost:9010"))
		})

		// TRANSACTION_TAG needs active transaction
		t.Run("TRANSACTION_TAG", func(t *testing.T) {
			t.Parallel()
			sysVars := newTestSysVars().withSession(&Session{tc: &transactionContext{
				attrs: transactionAttributes{mode: transactionModePending},
			}}).build()
			testSpecialVariable(t, setFunc, "TRANSACTION_TAG", "TRANSACTION_TAG", "test-tag", sysVars,
				singletonMap("TRANSACTION_TAG", "test-tag"))
		})

		// Read-only variables
		readOnlyTests := []struct {
			name    string
			sysVars *systemVariables
			want    map[string]string
		}{
			{
				"READ_TIMESTAMP",
				newTestSysVars().withReadTimestamp(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)).build(),
				singletonMap("READ_TIMESTAMP", "1970-01-01T00:00:00Z"),
			},
			{
				"COMMIT_TIMESTAMP",
				newTestSysVars().withCommitTimestamp(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)).build(),
				singletonMap("COMMIT_TIMESTAMP", "1970-01-01T00:00:00Z"),
			},
			{
				"COMMIT_RESPONSE",
				newTestSysVars().
					withCommitTimestamp(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)).
					withCommitResponse(&sppb.CommitResponse{CommitStats: &sppb.CommitResponse_CommitStats{MutationCount: 10}}).
					build(),
				map[string]string{"COMMIT_TIMESTAMP": "1970-01-01T00:00:00Z", "MUTATION_COUNT": "10"},
			},
			{"CLI_VERSION", nil, singletonMap("CLI_VERSION", getVersion())},
			{
				"CLI_PROJECT",
				newTestSysVars().withProject("test-project").build(),
				singletonMap("CLI_PROJECT", "test-project"),
			},
			{
				"CLI_INSTANCE",
				newTestSysVars().withInstance("test-instance").build(),
				singletonMap("CLI_INSTANCE", "test-instance"),
			},
			{
				"CLI_DATABASE",
				newTestSysVars().withDatabase("test-database").build(),
				singletonMap("CLI_DATABASE", "test-database"),
			},
			{
				"CLI_HISTORY_FILE",
				newTestSysVars().withHistoryFile("/tmp/spanner_mycli_readline.tmp").build(),
				singletonMap("CLI_HISTORY_FILE", "/tmp/spanner_mycli_readline.tmp"),
			},
			{
				"CLI_HOST",
				newTestSysVars().withHost("example.com").build(),
				singletonMap("CLI_HOST", "example.com"),
			},
			{
				"CLI_PORT",
				newTestSysVars().withPort(443).build(),
				singletonMap("CLI_PORT", "443"),
			},
			{
				"CLI_INSECURE",
				newTestSysVars().withInsecure(true).build(),
				singletonMap("CLI_INSECURE", "TRUE"),
			},
			{
				"CLI_LOG_GRPC",
				newTestSysVars().withLogGrpc(true).build(),
				singletonMap("CLI_LOG_GRPC", "TRUE"),
			},
			{
				"CLI_ROLE",
				newTestSysVars().withRole("test-role").build(),
				singletonMap("CLI_ROLE", "test-role"),
			},
			{
				"CLI_IMPERSONATE_SERVICE_ACCOUNT",
				newTestSysVars().withImpersonateServiceAccount("test@example.com").build(),
				singletonMap("CLI_IMPERSONATE_SERVICE_ACCOUNT", "test@example.com"),
			},
			{
				"CLI_MCP",
				newTestSysVars().withMCP(true).build(),
				singletonMap("CLI_MCP", "TRUE"),
			},
			{
				"CLI_DIRECT_READ",
				newTestSysVars().withDirectedRead(&sppb.DirectedReadOptions{Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
						{Type: sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE, Location: "asia-northeast2"},
					}},
				}}).build(),
				singletonMap("CLI_DIRECT_READ", "asia-northeast2:READ_WRITE"),
			},
		}
		for _, test := range readOnlyTests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				testReadOnlyVariable(t, setFunc, test.name, test.sysVars, test.want)
			})
		}

		// Unimplemented variables
		unimplementedVars := []string{"AUTOCOMMIT", "RETRY_ABORTS_INTERNALLY"}
		for _, name := range unimplementedVars {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				testUnimplementedVariable(t, setFunc, name)
			})
		}
	})

	t.Run("GoogleSQLMode", func(t *testing.T) {
		t.Parallel()
		setFunc := (*systemVariables).SetFromGoogleSQL

		// String variables with GoogleSQL syntax (quoted)
		quotedStringTests := map[string]string{
			"CLI_PROMPT":                   `"test-prompt"`,
			"CLI_PROMPT2":                  `"test-prompt2"`,
			"STATEMENT_TAG":                `"test-statement"`,
			"CLI_EXPLAIN_FORMAT":           `"CURRENT"`,
			"OPTIMIZER_VERSION":            `"LATEST"`,
			"OPTIMIZER_STATISTICS_PACKAGE": `"test-package"`,
			"STATEMENT_TIMEOUT":            `"30s"`,
			"MAX_COMMIT_DELAY":             `"100ms"`,
		}
		for name, quotedValue := range quotedStringTests {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				sysVars := newSystemVariablesWithDefaultsForTest()
				err := setFunc(sysVars, name, quotedValue)
				assertNoError(t, err)

				// Strip quotes for expected value
				expectedValue := strings.Trim(quotedValue, `"`)
				got, err := sysVars.Get(name)
				assertNoError(t, err)
				want := map[string]string{name: expectedValue}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("sysVars.Get() mismatch (-want +got):\n%s", diff)
				}
			})
		}

		// Boolean variables with GoogleSQL keywords
		t.Run("CLI_USE_PAGER_TRUE", func(t *testing.T) {
			t.Parallel()
			testStringVariable(t, setFunc, "CLI_USE_PAGER", "TRUE")
		})
		t.Run("CLI_USE_PAGER_FALSE", func(t *testing.T) {
			t.Parallel()
			testStringVariable(t, setFunc, "CLI_USE_PAGER", "FALSE")
		})
	})
}
