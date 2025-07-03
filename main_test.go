package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/creack/pty"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_initializeSystemVariables(t *testing.T) {
	// Helper to convert ast.Node map to string map for comparison
	nodeMapToStringMap := func(m map[string]ast.Node) map[string]string {
		if m == nil {
			return nil
		}
		strMap := make(map[string]string)
		for k, v := range m {
			strMap[k] = v.SQL()
		}
		return strMap
	}

	tests := []struct {
		name    string
		opts    *spannerOptions
		want    systemVariables
		wantErr bool
	}{
		{
			name: "default values",
			opts: &spannerOptions{},
			want: systemVariables{
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
				Params:               make(map[string]ast.Node),
			},
			wantErr: false,
		},
		{
			name: "explicitly set values",
			opts: &spannerOptions{
				ProjectId:    "test-project",
				InstanceId:   "test-instance",
				DatabaseId:   "test-database",
				Verbose:      true,
				Prompt:       lo.ToPtr("my-prompt> "),
				Prompt2:      lo.ToPtr("my-prompt2> "),
				HistoryFile:  lo.ToPtr("/path/to/history.txt"),
				Priority:     "HIGH",
				Role:         "test-role",
				Endpoint:     "test-endpoint:443",
				DirectedRead: "us-east1:READ_ONLY",
				SQL:          "SELECT 1", // Should not affect sysVars directly
				Set: map[string]string{
					"CLI_FORMAT": "VERTICAL",
					"READONLY":   "true",
				},
				Param: map[string]string{
					"p1": "'string_value'",
					"p2": "FLOAT64",
				},
				ProtoDescriptorFile:       "testdata/protos/singer.proto",
				Insecure:                  lo.ToPtr(true),
				SkipTlsVerify:             lo.ToPtr(false), // Insecure takes precedence
				LogGrpc:                   true,
				LogLevel:                  "INFO",
				QueryMode:                 "PLAN",
				Strong:                    true,
				ReadTimestamp:             "", // Strong takes precedence
				VertexAIProject:           "vertex-project",
				VertexAIModel:             lo.ToPtr("gemini-1.0-pro"),
				DatabaseDialect:           "POSTGRESQL",
				ImpersonateServiceAccount: "test-sa@example.com",
				EnablePartitionedDML:      true,
			},
			want: systemVariables{
				Project:                   "test-project",
				Instance:                  "test-instance",
				Database:                  "test-database",
				Verbose:                   true,
				Prompt:                    "my-prompt> ",
				Prompt2:                   "my-prompt2> ",
				HistoryFile:               "/path/to/history.txt",
				Role:                      "test-role",
				Host:                      "test-endpoint",
				Port:                      443,
				Insecure:                  true,
				LogGrpc:                   true,
				LogLevel:                  slog.LevelInfo,
				ImpersonateServiceAccount: "test-sa@example.com",
				VertexAIProject:           "vertex-project",
				VertexAIModel:             "gemini-1.0-pro",
				EnableADCPlus:             true,
				ReturnCommitStats:         true,
				AnalyzeColumns:            DefaultAnalyzeColumns,
				ParsedAnalyzeColumns:      DefaultParsedAnalyzeColumns,
				RPCPriority:               sppb.RequestOptions_PRIORITY_HIGH,
				QueryMode:                 sppb.ExecuteSqlRequest_PLAN.Enum(),
				ReadOnlyStaleness:         lo.ToPtr(spanner.StrongRead()),
				DatabaseDialect:           databasepb.DatabaseDialect_POSTGRESQL,
				AutocommitDMLMode:         AutocommitDMLModePartitionedNonAtomic,
				ProtoDescriptorFile:       []string{"testdata/protos/singer.proto"},
				CLIFormat:                 DisplayModeVertical,
				ReadOnly:                  true,
				DirectedRead: &sppb.DirectedReadOptions{
					Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
						IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
							ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
								{
									Location: "us-east1",
									Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
								},
							},
							AutoFailoverDisabled: true,
						},
					},
				},
				OutputTemplateFile: "",
				OutputTemplate:     defaultOutputFormat,
				Params: map[string]ast.Node{
					"p1": lo.Must(memefish.ParseExpr("", "'string_value'")),
					"p2": lo.Must(memefish.ParseType("", "FLOAT64")),
				},
			},
			wantErr: false,
		},
		{
			name: "error: invalid log level",
			opts: &spannerOptions{
				LogLevel: "INVALID",
			},
			want:    systemVariables{},
			wantErr: true,
		},
		{
			name: "error: strong and read-timestamp mutually exclusive",
			opts: &spannerOptions{
				Strong:        true,
				ReadTimestamp: "2023-01-01T00:00:00Z",
			},
			want: systemVariables{
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
				Params:               make(map[string]ast.Node),
				ReadOnlyStaleness:    lo.ToPtr(spanner.ReadTimestamp(lo.Must(time.Parse(time.RFC3339Nano, "2023-01-01T00:00:00Z")))),
			},
			wantErr: false,
		},
		{
			name: "error: invalid read-timestamp format",
			opts: &spannerOptions{
				ReadTimestamp: "invalid-timestamp",
			},
			want:    systemVariables{},
			wantErr: true,
		},
		{
			name: "error: invalid priority",
			opts: &spannerOptions{
				Priority: "INVALID",
			},
			want:    systemVariables{},
			wantErr: true,
		},
		{
			name: "valid timeout flag",
			opts: &spannerOptions{
				Timeout: "30s",
			},
			want: systemVariables{
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
				Params:               make(map[string]ast.Node),
				StatementTimeout:     lo.ToPtr(30 * time.Second),
			},
			wantErr: false,
		},
		{
			name: "insecure flag precedence - both true",
			opts: &spannerOptions{
				Insecure:      lo.ToPtr(true),
				SkipTlsVerify: lo.ToPtr(true),
			},
			want: systemVariables{
				Insecure:             true, // --insecure takes precedence
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "insecure flag precedence - insecure false, skip-tls-verify true",
			opts: &spannerOptions{
				Insecure:      lo.ToPtr(false),
				SkipTlsVerify: lo.ToPtr(true),
			},
			want: systemVariables{
				Insecure:             false, // --insecure takes precedence even when false
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "only skip-tls-verify set",
			opts: &spannerOptions{
				SkipTlsVerify: lo.ToPtr(true),
			},
			want: systemVariables{
				Insecure:             true, // Uses skip-tls-verify value
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "error: invalid timeout format",
			opts: &spannerOptions{
				Timeout: "invalid",
			},
			want:    systemVariables{},
			wantErr: true,
		},
		{
			name: "error: invalid directed read option",
			opts: &spannerOptions{
				DirectedRead: "invalid-option",
			},
			want: systemVariables{
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
				Params:               make(map[string]ast.Node),
				DirectedRead: &sppb.DirectedReadOptions{
					Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
						IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
							ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
								{
									Location: "invalid-option",
									Type:     sppb.DirectedReadOptions_ReplicaSelection_TYPE_UNSPECIFIED,
								},
							},
							AutoFailoverDisabled: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error: invalid set value",
			opts: &spannerOptions{
				Set: map[string]string{
					"READONLY": "not-a-bool",
				},
			},
			want:    systemVariables{},
			wantErr: true,
		},
		{
			name: "error: invalid proto descriptor file",
			opts: &spannerOptions{
				ProtoDescriptorFile: "non-existent-file.proto",
			},
			want:    systemVariables{},
			wantErr: true,
		},
		{
			name: "embedded emulator with user values",
			opts: &spannerOptions{
				EmbeddedEmulator: true,
				ProjectId:        "user-project",
				InstanceId:       "user-instance",
				DatabaseId:       "user-database",
				Insecure:         lo.ToPtr(false), // should be overridden by embedded emulator
			},
			want: systemVariables{
				Project:              "user-project",
				Instance:             "user-instance",
				Database:             "user-database",
				Insecure:             true, // embedded emulator always sets this
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "embedded emulator with no user values",
			opts: &spannerOptions{
				EmbeddedEmulator: true,
			},
			want: systemVariables{
				Project:              "emulator-project", // Default value set in initializeSystemVariables
				Instance:             "emulator-instance", // Default value set in initializeSystemVariables
				Database:             "emulator-database", // Default value set in initializeSystemVariables
				Insecure:             true,
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "embedded emulator with detached mode",
			opts: &spannerOptions{
				EmbeddedEmulator: true,
				Detached:        true,
			},
			want: systemVariables{
				Project:              "emulator-project",  // Default set for emulator
				Instance:             "emulator-instance", // Default set for emulator
				Database:             "",                  // Empty - respects detached mode
				Insecure:             true,
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "CLI_ANALYZE_COLUMNS set",
			opts: &spannerOptions{
				Set: map[string]string{
					"CLI_ANALYZE_COLUMNS": "Col1:{{.Col1}},Col2:{{.Col2}}",
				},
			},
			want: systemVariables{
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       "Col1:{{.Col1}},Col2:{{.Col2}}",
				ParsedAnalyzeColumns: lo.Must(customListToTableRenderDefs("Col1:{{.Col1}},Col2:{{.Col2}}")),
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
			},
			wantErr: false,
		},
		{
			name: "CLI_OUTPUT_TEMPLATE_FILE set",
			opts: &spannerOptions{
				OutputTemplate: "output_full.tmpl",
			},
			want: systemVariables{
				Prompt:             defaultPrompt,
				Prompt2:            defaultPrompt2,
				HistoryFile:        defaultHistoryFile,
				LogLevel:           slog.LevelWarn,
				VertexAIModel:      defaultVertexAIModel,
				EnableADCPlus:      true,
				ReturnCommitStats:  true,
				AnalyzeColumns:     DefaultAnalyzeColumns,
				RPCPriority:        defaultPriority,
				OutputTemplateFile: "output_full.tmpl",
				// OutputTemplate:       should be parsed from file, hard to compare directly
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "CLI_OUTPUT_TEMPLATE_FILE set to NULL",
			opts: &spannerOptions{
				Set: map[string]string{
					"CLI_OUTPUT_TEMPLATE_FILE": "NULL",
				},
			},
			want: systemVariables{
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
		{
			name: "CLI_OUTPUT_TEMPLATE_FILE set to empty string",
			opts: &spannerOptions{
				Set: map[string]string{
					"CLI_OUTPUT_TEMPLATE_FILE": "",
				},
			},
			want: systemVariables{
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
				ReturnCommitStats:    true,
				AnalyzeColumns:       DefaultAnalyzeColumns,
				RPCPriority:          defaultPriority,
				OutputTemplateFile:   "",
				OutputTemplate:       defaultOutputFormat,
				ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := initializeSystemVariables(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("initializeSystemVariables() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Convert Params map to string map for comparison and compare separately
			gotParamsStr := nodeMapToStringMap(got.Params)
			wantParamsStr := nodeMapToStringMap(tt.want.Params)

			// Use cmp.Diff for comparison, ignoring unexported fields and specific fields
			// that are hard to compare directly (e.g., *template.Template, *descriptorpb.FileDescriptorSet)
			// and those that are set later in run() (e.g., EnableProgressBar, CurrentSession, WithoutAuthentication)
			if diff := cmp.Diff(tt.want, got,
				cmpopts.IgnoreUnexported(systemVariables{}),
				cmpopts.IgnoreFields(systemVariables{}, "OutputTemplate", "ProtoDescriptor", "EnableProgressBar", "CurrentSession", "WithoutAuthentication"), // Removed Params from here
				cmpopts.IgnoreFields(systemVariables{}, "ParsedAnalyzeColumns"),
				cmpopts.EquateApproxTime(time.Microsecond),
				protocmp.Transform(),
				cmpopts.EquateEmpty(), // Added EquateEmpty
				cmp.Comparer(func(x, y *spanner.TimestampBound) bool {
					if x == nil && y == nil {
						return true
					}
					if x == nil || y == nil {
						return false
					}
					return (*x).String() == (*y).String()
				}),
			); diff != "" {
				t.Errorf("initializeSystemVariables() mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(wantParamsStr, gotParamsStr, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("initializeSystemVariables() Params mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_newSystemVariablesWithDefaults(t *testing.T) {
	got := newSystemVariablesWithDefaults()
	
	want := systemVariables{
		ReturnCommitStats:    true,
		RPCPriority:          defaultPriority,
		EnableADCPlus:        true,
		AnalyzeColumns:       DefaultAnalyzeColumns,
		ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns,
		Prompt:               defaultPrompt,
		Prompt2:              defaultPrompt2,
		HistoryFile:          defaultHistoryFile,
		VertexAIModel:        defaultVertexAIModel,
	}
	
	if diff := cmp.Diff(want, got, 
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreFields(systemVariables{}, "OutputTemplate", "ParsedAnalyzeColumns"), // Ignore template and function pointer comparisons
	); diff != "" {
		t.Errorf("newSystemVariablesWithDefaults() mismatch (-want +got):\n%s", diff)
	}
	
	// Separately check OutputTemplate is not nil
	if got.OutputTemplate == nil {
		t.Errorf("newSystemVariablesWithDefaults() OutputTemplate should not be nil")
	}
	
	// Separately check ParsedAnalyzeColumns is not nil
	if got.ParsedAnalyzeColumns == nil {
		t.Errorf("newSystemVariablesWithDefaults() ParsedAnalyzeColumns should not be nil")
	}
}

func Test_parseParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		want    map[string]ast.Node
		wantErr bool
	}{
		{
			name:   "empty params",
			params: map[string]string{},
			want:   map[string]ast.Node{},
		},
		{
			name: "valid string param",
			params: map[string]string{
				"p1": "'hello'",
			},
			want: map[string]ast.Node{
				"p1": lo.Must(memefish.ParseExpr("", "'hello'")),
			},
		},
		{
			name: "valid type param",
			params: map[string]string{
				"p1": "STRING",
			},
			want: map[string]ast.Node{
				"p1": lo.Must(memefish.ParseType("", "STRING")),
			},
		},
		{
			name: "invalid param",
			params: map[string]string{
				"p1": "invalid syntax",
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseParams(test.params)
			if test.wantErr {
				if err == nil {
					t.Errorf("parseParams() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("parseParams() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			
			// Compare using string representation since AST nodes are complex
			if len(got) != len(test.want) {
				t.Errorf("parseParams() got %d params, want %d", len(got), len(test.want))
				return
			}
			
			for k, wantNode := range test.want {
				gotNode, exists := got[k]
				if !exists {
					t.Errorf("parseParams() missing key %s", k)
					continue
				}
				if gotNode.SQL() != wantNode.SQL() {
					t.Errorf("parseParams() key %s = %v, want %v", k, gotNode.SQL(), wantNode.SQL())
				}
			}
		})
	}
}

func Test_createSystemVariablesFromOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    *spannerOptions
		want    systemVariables
		wantErr bool
	}{
		{
			name: "empty options preserve defaults",
			opts: &spannerOptions{},
			want: func() systemVariables {
				sv := newSystemVariablesWithDefaults()
				sv.LogLevel = slog.LevelWarn
				sv.Params = make(map[string]ast.Node)
				return sv
			}(),
		},
		{
			name: "override specific values",
			opts: &spannerOptions{
				ProjectId:  "test-project",
				InstanceId: "test-instance",
				DatabaseId: "test-database",
				Prompt:     lo.ToPtr("custom> "),
				LogLevel:   "INFO",
			},
			want: func() systemVariables {
				sv := newSystemVariablesWithDefaults()
				sv.Project = "test-project"
				sv.Instance = "test-instance"
				sv.Database = "test-database"
				sv.Prompt = "custom> "
				sv.LogLevel = slog.LevelInfo
				sv.Params = make(map[string]ast.Node)
				return sv
			}(),
		},
		{
			name: "skip-system-command flag",
			opts: &spannerOptions{
				SkipSystemCommand: true,
			},
			want: func() systemVariables {
				sv := newSystemVariablesWithDefaults()
				sv.LogLevel = slog.LevelWarn
				sv.SkipSystemCommand = true
				sv.Params = make(map[string]ast.Node)
				return sv
			}(),
		},
		{
			name: "system-command=OFF",
			opts: &spannerOptions{
				SystemCommand: lo.ToPtr("OFF"),
			},
			want: func() systemVariables {
				sv := newSystemVariablesWithDefaults()
				sv.LogLevel = slog.LevelWarn
				sv.SkipSystemCommand = true
				sv.Params = make(map[string]ast.Node)
				return sv
			}(),
		},
		{
			name: "system-command=ON",
			opts: &spannerOptions{
				SystemCommand: lo.ToPtr("ON"),
			},
			want: func() systemVariables {
				sv := newSystemVariablesWithDefaults()
				sv.LogLevel = slog.LevelWarn
				sv.SkipSystemCommand = false
				sv.Params = make(map[string]ast.Node)
				return sv
			}(),
		},
		{
			name: "skip-system-command takes precedence over system-command=ON",
			opts: &spannerOptions{
				SkipSystemCommand: true,
				SystemCommand:     lo.ToPtr("ON"),
			},
			want: func() systemVariables {
				sv := newSystemVariablesWithDefaults()
				sv.LogLevel = slog.LevelWarn
				sv.SkipSystemCommand = true
				sv.Params = make(map[string]ast.Node)
				return sv
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := createSystemVariablesFromOptions(test.opts)
			if test.wantErr {
				if err == nil {
					t.Errorf("createSystemVariablesFromOptions() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("createSystemVariablesFromOptions() error = %v, wantErr %v", err, test.wantErr)
				return
			}

			// Compare key fields
			if diff := cmp.Diff(test.want, got,
				cmpopts.IgnoreFields(systemVariables{}, "ParsedAnalyzeColumns", "OutputTemplate"), // Ignore complex fields for this test
				cmpopts.EquateEmpty(),
			); diff != "" {
				t.Errorf("createSystemVariablesFromOptions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}


func TestDetermineInputAndMode(t *testing.T) {
	// Helper to create stdin based on usePTY flag
	type stdinProvider func() (io.Reader, func(), error)
	
	nonPTYStdin := func(content string) stdinProvider {
		return func() (io.Reader, func(), error) {
			return strings.NewReader(content), func() {}, nil
		}
	}
	
	ptyStdin := func() stdinProvider {
		return func() (io.Reader, func(), error) {
			pty, tty, err := pty.Open()
			if err != nil {
				return nil, nil, err
			}
			cleanup := func() {
				pty.Close()
				tty.Close()
			}
			return tty, cleanup, nil
		}
	}

	tests := []struct {
		name            string
		opts            *spannerOptions
		stdinProvider   stdinProvider
		wantInput       string
		wantInteractive bool
		wantErr         bool
	}{
		// Command line options tests (non-PTY)
		{
			name:            "SQL option",
			opts:            &spannerOptions{SQL: "SELECT 1"},
			stdinProvider:   nonPTYStdin(""),
			wantInput:       "SELECT 1",
			wantInteractive: false,
		},
		{
			name:            "Execute option",
			opts:            &spannerOptions{Execute: "SELECT 2"},
			stdinProvider:   nonPTYStdin(""),
			wantInput:       "SELECT 2",
			wantInteractive: false,
		},
		{
			name:            "File option from stdin",
			opts:            &spannerOptions{File: "-"},
			stdinProvider:   nonPTYStdin("SELECT 3;"),
			wantInput:       "SELECT 3;",
			wantInteractive: false,
		},
		{
			name:            "Source option from stdin (alias of File)",
			opts:            &spannerOptions{Source: "-"},
			stdinProvider:   nonPTYStdin("SELECT 3;"),
			wantInput:       "SELECT 3;",
			wantInteractive: false,
		},
		{
			name:            "Both File and Source - File takes precedence",
			opts:            &spannerOptions{File: "-", Source: "ignored.sql"},
			stdinProvider:   nonPTYStdin("SELECT FROM FILE;"),
			wantInput:       "SELECT FROM FILE;",
			wantInteractive: false,
		},
		{
			name:            "Both Execute and SQL - Execute takes precedence",
			opts:            &spannerOptions{Execute: "SELECT 1", SQL: "SELECT 2"},
			stdinProvider:   nonPTYStdin(""),
			wantInput:       "SELECT 1",
			wantInteractive: false,
		},
		// Piped input tests (non-PTY)
		{
			name:            "No options with piped input",
			opts:            &spannerOptions{},
			stdinProvider:   nonPTYStdin("SELECT 4;"),
			wantInput:       "SELECT 4;",
			wantInteractive: false,
		},
		{
			name:            "Empty stdin (like /dev/null)",
			opts:            &spannerOptions{},
			stdinProvider:   nonPTYStdin(""),
			wantInput:       "",
			wantInteractive: false,
		},
		// PTY tests
		{
			name:            "No options with terminal (PTY)",
			opts:            &spannerOptions{},
			stdinProvider:   ptyStdin(),
			wantInput:       "",
			wantInteractive: true,
		},
		// PTY with command line options should still use batch mode
		{
			name:            "SQL option with terminal (PTY)",
			opts:            &spannerOptions{SQL: "SELECT 5"},
			stdinProvider:   ptyStdin(),
			wantInput:       "SELECT 5",
			wantInteractive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin, cleanup, err := tt.stdinProvider()
			if err != nil {
				t.Skipf("Failed to create stdin: %v", err)
			}
			defer cleanup()
			
			input, interactive, err := determineInputAndMode(tt.opts, stdin)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("determineInputAndMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if input != tt.wantInput {
				t.Errorf("determineInputAndMode() input = %v, want %v", input, tt.wantInput)
			}
			if interactive != tt.wantInteractive {
				t.Errorf("determineInputAndMode() interactive = %v, want %v", interactive, tt.wantInteractive)
			}
		})
	}
}
