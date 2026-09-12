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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"google.golang.org/genai"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestGeminiSystemPrompt(t *testing.T) {
	t.Parallel()

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("example.proto")}},
	}
	got := geminiSystemPrompt(&adminpb.GetDatabaseDdlResponse{
		Statements: []string{"CREATE TABLE T (id INT64) PRIMARY KEY (id)"},
	}, fds)
	for _, want := range []string{
		"Cloud Spanner query composer",
		"CREATE TABLE T (id INT64) PRIMARY KEY (id);",
		"example.proto",
		"GoogleSQL syntax is not PostgreSQL syntax",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestBuildDocCache(t *testing.T) {
	t.Parallel()

	t.Run("embedded docs only", func(t *testing.T) {
		t.Parallel()
		cache, err := buildDocCache("")
		if err != nil {
			t.Fatalf("buildDocCache() error = %v", err)
		}
		holder := &docCacheHolder{cache: cache}
		t.Cleanup(func() {
			if err := holder.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
		if len(cache.Names()) == 0 {
			t.Fatal("embedded doc cache is empty")
		}
		if cache.fetch != nil || cache.batchFetch != nil || cache.apiSearch != nil {
			t.Fatal("embedded cache should not wire Developer Knowledge fetchers")
		}
	})

	t.Run("api key wires fetchers", func(t *testing.T) {
		t.Parallel()
		cache, err := buildDocCache("test-api-key")
		if err != nil {
			t.Fatalf("buildDocCache() error = %v", err)
		}
		t.Cleanup(cache.Close)
		if cache.fetch == nil || cache.batchFetch == nil || cache.apiSearch == nil {
			t.Fatal("API key should wire fetch, batch fetch, and API search")
		}
	})
}

func TestGeminiComposeQueryWithToolsOffline(t *testing.T) {
	t.Parallel()

	ddl := &adminpb.GetDatabaseDdlResponse{
		Statements: []string{"CREATE TABLE Singers (SingerId INT64) PRIMARY KEY (SingerId)"},
	}

	t.Run("missing API key", func(t *testing.T) {
		t.Parallel()
		_, err := geminiComposeQueryWithTools(t.Context(), ddl, &genai.ClientConfig{Backend: genai.BackendGeminiAPI}, defaultVertexAIModel, thinkingLevelUnspecified, "list singers", nil, false)
		if err == nil || !strings.Contains(err.Error(), "api key is required") {
			t.Fatalf("error = %v, want api key required", err)
		}
	})

	t.Run("invalid proto descriptors", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}))
		t.Cleanup(server.Close)
		_, err := geminiComposeQueryWithTools(t.Context(), &adminpb.GetDatabaseDdlResponse{
			ProtoDescriptors: []byte("not-a-protobuf"),
		}, fakeGeminiClientConfig(server), defaultVertexAIModel, thinkingLevelUnspecified, "list singers", nil, false)
		if err == nil {
			t.Fatal("expected proto unmarshal error")
		}
	})

	t.Run("tool-use HTTP error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":{"message":"model unavailable"}}`, http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)
		cache := newTestCache(t)
		_, err := geminiComposeQueryWithTools(t.Context(), ddl, fakeGeminiClientConfig(server), defaultVertexAIModel, "HIGH", "list singers", cache, false)
		if err == nil || !strings.Contains(err.Error(), "tool-use round 0") {
			t.Fatalf("error = %v, want tool-use round 0", err)
		}
	})

	t.Run("tool round then structured output", func(t *testing.T) {
		t.Parallel()
		cache := newTestCache(t)
		docName := docPrefix + "reference/standard-sql/query-syntax"
		cache.Put(docName, "SELECT statement reference")

		composedJSON := mustComposeJSON(t, &output{
			CandidateStatements: []*statement{{
				Text:                "SELECT SingerId FROM Singers;",
				FixedText:           "SELECT SingerId FROM Singers;",
				Reason:              "matches request",
				SyntaxDescription:   "select one column",
				SemanticDescription: "lists singer ids",
			}},
			Statement: &statement{
				Text:                "SELECT SingerId FROM Singers;",
				FixedText:           "SELECT SingerId FROM Singers;",
				Reason:              "matches request",
				SyntaxDescription:   "select one column",
				SemanticDescription: "lists singer ids",
			},
		})

		var requests []string
		call := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			requests = append(requests, r.URL.Path)
			call++
			w.Header().Set("Content-Type", "application/json")
			switch call {
			case 1:
				if !strings.Contains(r.URL.Path, ":generateContent") {
					t.Errorf("path = %q, want generateContent", r.URL.Path)
				}
				if !strings.Contains(string(body), "list singers") {
					t.Errorf("phase 1 body missing prompt: %s", body)
				}
				writeGenerateContentFunctionCall(t, w, "get_cached_document", map[string]any{
					"names": []any{docName},
				})
			case 2:
				if !strings.Contains(string(body), "SELECT statement reference") {
					t.Errorf("phase 1 follow-up missing tool result: %s", body)
				}
				writeGenerateContentText(t, w, "ready")
			default:
				writeGenerateContentText(t, w, composedJSON)
			}
		}))
		t.Cleanup(server.Close)

		got, err := geminiComposeQueryWithTools(t.Context(), ddl, fakeGeminiClientConfig(server), defaultVertexAIModel, "HIGH", "list singers", cache, false)
		if err != nil {
			t.Fatalf("geminiComposeQueryWithTools() error = %v", err)
		}
		if got == nil || got.Statement == nil {
			t.Fatalf("composed = %#v", got)
		}
		if got.Statement.Text != "SELECT SingerId FROM Singers;" {
			t.Errorf("statement text = %q", got.Statement.Text)
		}
		if got.Statement.SemanticDescription != "lists singer ids" {
			t.Errorf("semanticDescription = %q", got.Statement.SemanticDescription)
		}
		if len(requests) < 2 {
			t.Fatalf("generateContent calls = %d, want at least 2", len(requests))
		}
	})
}

func fakeGeminiClientConfig(server *httptest.Server) *genai.ClientConfig {
	return &genai.ClientConfig{
		Backend:    genai.BackendGeminiAPI,
		APIKey:     "test-api-key",
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	}
}

func mustComposeJSON(t *testing.T, composed *output) string {
	t.Helper()
	b, err := json.Marshal(composed)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

func writeGenerateContentText(t *testing.T, w http.ResponseWriter, text string) {
	t.Helper()
	writeJSON(t, w, map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": text}},
				},
			},
		},
	})
}

func writeGenerateContentFunctionCall(t *testing.T, w http.ResponseWriter, name string, args map[string]any) {
	t.Helper()
	writeJSON(t, w, map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{
						{
							"functionCall": map[string]any{
								"id":   "call-1",
								"name": name,
								"args": args,
							},
						},
					},
				},
			},
		},
	})
}
