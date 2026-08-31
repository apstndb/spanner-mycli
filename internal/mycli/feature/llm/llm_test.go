// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package llm

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genai"
)

func TestResultFromComposedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		composed     *output
		wantErr      string
		wantPreInput string
		wantRows     int
	}{
		{
			name:     "null structured output",
			wantErr:  "GEMINI returned no response",
			wantRows: 0,
		},
		{
			name:     "missing statement",
			composed: &output{},
			wantErr:  "GEMINI returned no statement",
		},
		{
			name: "missing statement with error description",
			composed: &output{
				ErrorDescription: "model rejected request",
			},
			wantErr: "GEMINI returned no statement: model rejected request",
		},
		{
			name: "valid statement",
			composed: &output{
				Statement: &statement{
					Text:                "SELECT 1;",
					SemanticDescription: "returns one",
					SyntaxDescription:   "select literal",
				},
			},
			wantPreInput: "SELECT 1;",
			wantRows:     3,
		},
		{
			name: "valid statement with error description",
			composed: &output{
				Statement: &statement{
					Text:                "SELECT 1;",
					SemanticDescription: "returns one",
					SyntaxDescription:   "select literal",
				},
				ErrorDescription: "input required correction",
			},
			wantPreInput: "SELECT 1;",
			wantRows:     4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resultFromComposedOutput(tt.composed)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resultFromComposedOutput() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("resultFromComposedOutput() error = %q, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resultFromComposedOutput() error = %v", err)
			}
			if got.PreInput != tt.wantPreInput {
				t.Errorf("PreInput = %q, want %q", got.PreInput, tt.wantPreInput)
			}
			if len(got.Rows) != tt.wantRows {
				t.Errorf("len(Rows) = %d, want %d", len(got.Rows), tt.wantRows)
			}
		})
	}
}

func TestFirstCandidateContent(t *testing.T) {
	t.Parallel()

	content := &genai.Content{Parts: []*genai.Part{genai.NewPartFromText("tool call")}}
	tests := []struct {
		name   string
		result *genai.GenerateContentResponse
		want   *genai.Content
	}{
		{name: "nil response"},
		{name: "no candidates", result: &genai.GenerateContentResponse{}},
		{
			name: "nil candidate",
			result: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{nil},
			},
		},
		{
			name: "nil candidate content",
			result: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{}},
			},
		},
		{
			name: "candidate content",
			result: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{Content: content}},
			},
			want: content,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := firstCandidateContent(tt.result); got != tt.want {
				t.Errorf("firstCandidateContent() = %p, want %p", got, tt.want)
			}
		})
	}
}

func TestNewGenAIClientConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		cfg            *config
		sessionProject string
		wantBackend    genai.Backend
		wantProject    string
		wantLocation   string
		wantAPIVersion string
	}{
		{
			name:           "enterprise uses connected Spanner project",
			cfg:            newConfig(),
			sessionProject: "session-project",
			wantBackend:    genai.BackendEnterprise,
			wantProject:    "session-project",
			wantLocation:   defaultVertexAILocation,
			wantAPIVersion: "v1",
		},
		{
			name: "enterprise project override wins",
			cfg: &config{
				Backend:  genAIBackendEnterprise,
				Project:  "override-project",
				Location: "us-central1",
			},
			sessionProject: "session-project",
			wantBackend:    genai.BackendEnterprise,
			wantProject:    "override-project",
			wantLocation:   "us-central1",
			wantAPIVersion: "v1",
		},
		{
			name: "Gemini API does not send Enterprise routing fields",
			cfg: &config{
				Backend:  genAIBackendGeminiAPI,
				Project:  "must-not-leak",
				Location: "must-not-leak",
			},
			sessionProject: "must-not-leak",
			wantBackend:    genai.BackendGeminiAPI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := newGenAIClientConfig(tt.cfg, tt.sessionProject)
			if got.Backend != tt.wantBackend {
				t.Errorf("Backend = %v, want %v", got.Backend, tt.wantBackend)
			}
			if got.Project != tt.wantProject {
				t.Errorf("Project = %q, want %q", got.Project, tt.wantProject)
			}
			if got.Location != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got.Location, tt.wantLocation)
			}
			if got.APIKey != "" {
				t.Errorf("APIKey = %q, want empty so the SDK reads environment credentials", got.APIKey)
			}
			if got.HTTPOptions.APIVersion != tt.wantAPIVersion {
				t.Errorf("APIVersion = %q, want %q", got.HTTPOptions.APIVersion, tt.wantAPIVersion)
			}
		})
	}
}

func TestNewThinkingConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level string
		want  genai.ThinkingLevel
	}{
		{level: thinkingLevelUnspecified},
		{level: "MINIMAL", want: genai.ThinkingLevelMinimal},
		{level: "LOW", want: genai.ThinkingLevelLow},
		{level: "MEDIUM", want: genai.ThinkingLevelMedium},
		{level: "HIGH", want: genai.ThinkingLevelHigh},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Parallel()

			got := newThinkingConfig(tt.level)
			if tt.level == thinkingLevelUnspecified {
				if got != nil {
					t.Fatalf("newThinkingConfig(%q) = %#v, want nil", tt.level, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("newThinkingConfig(%q) = nil", tt.level)
			}
			if got.ThinkingLevel != tt.want {
				t.Errorf("ThinkingLevel = %q, want %q", got.ThinkingLevel, tt.want)
			}
		})
	}
}

func TestNewFunctionResponsePart(t *testing.T) {
	t.Parallel()

	response := map[string]any{"result": "ok"}
	call := &genai.FunctionCall{
		ID:   "call-123",
		Name: "search_documents",
	}

	got := newFunctionResponsePart(call, response)
	if got.FunctionResponse == nil {
		t.Fatal("FunctionResponse = nil")
	}
	if got.FunctionResponse.ID != call.ID {
		t.Errorf("FunctionResponse.ID = %q, want %q", got.FunctionResponse.ID, call.ID)
	}
	if got.FunctionResponse.Name != call.Name {
		t.Errorf("FunctionResponse.Name = %q, want %q", got.FunctionResponse.Name, call.Name)
	}
	if diff := cmp.Diff(response, got.FunctionResponse.Response); diff != "" {
		t.Errorf("FunctionResponse.Response mismatch (-want +got):\n%s", diff)
	}
}
