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
