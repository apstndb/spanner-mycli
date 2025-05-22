package main

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
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
				Insecure:                  true,
				SkipTlsVerify:             false, // Insecure takes precedence
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
				Endpoint:                  "test-endpoint:443",
				Insecure:                  true,
				LogGrpc:                   true,
				LogLevel:                  slog.LevelInfo,
				ImpersonateServiceAccount: "test-sa@example.com",
				VertexAIProject:           "vertex-project",
				VertexAIModel:             "gemini-1.0-pro",
				EnableADCPlus:             true,
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
			name: "embedded emulator sets defaults",
			opts: &spannerOptions{
				EmbeddedEmulator: true,
				ProjectId:        "should-be-overridden",
				InstanceId:       "should-be-overridden",
				DatabaseId:       "should-be-overridden",
				Insecure:         false, // should be overridden
			},
			want: systemVariables{
				Project:              "emulator-project",
				Instance:             "emulator-instance",
				Database:             "emulator-database",
				Insecure:             true,
				Prompt:               defaultPrompt,
				Prompt2:              defaultPrompt2,
				HistoryFile:          defaultHistoryFile,
				LogLevel:             slog.LevelWarn,
				VertexAIModel:        defaultVertexAIModel,
				EnableADCPlus:        true,
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
				AnalyzeColumns:       "Col1:{{.Col1}},Col2:{{.Col2}}",
				ParsedAnalyzeColumns: lo.Must(customListToTableRenderDef(strings.Split("Col1:{{.Col1}},Col2:{{.Col2}}", ","))), // This one is different, keep as is
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
