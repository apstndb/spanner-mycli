package mycli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

func TestDevKnowledgeAPIKey(t *testing.T) {
	tests := []struct {
		name      string
		devKey    string
		googleKey string
		want      string
	}{
		{name: "prefers DEVELOPERKNOWLEDGE_API_KEY", devKey: "dev-key", googleKey: "google-key", want: "dev-key"},
		{name: "falls back to GOOGLE_API_KEY", devKey: "", googleKey: "google-key", want: "google-key"},
		{name: "no keys set", devKey: "", googleKey: "", want: ""},
		{name: "only DEVELOPERKNOWLEDGE_API_KEY", devKey: "dev-key", googleKey: "", want: "dev-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEVELOPERKNOWLEDGE_API_KEY", tt.devKey)
			t.Setenv("GOOGLE_API_KEY", tt.googleKey)
			if got := devKnowledgeAPIKey(); got != tt.want {
				t.Errorf("devKnowledgeAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildToolDeclarations(t *testing.T) {
	toolNames := func(tools []*genai.Tool) []string {
		var names []string
		for _, tool := range tools {
			for _, decl := range tool.FunctionDeclarations {
				names = append(names, decl.Name)
			}
		}
		slices.Sort(names)
		return names
	}

	t.Run("without API", func(t *testing.T) {
		tools := buildToolDeclarations(false)
		names := toolNames(tools)
		want := []string{"get_cached_document", "search_cached_documents"}
		if !slices.Equal(names, want) {
			t.Errorf("tool names = %v, want %v", names, want)
		}
	})

	t.Run("with API", func(t *testing.T) {
		tools := buildToolDeclarations(true)
		names := toolNames(tools)
		want := []string{"get_cached_document", "get_developer_document", "search_cached_documents", "search_developer_docs"}
		if !slices.Equal(names, want) {
			t.Errorf("tool names = %v, want %v", names, want)
		}
	})
}

func TestBuildToolGuidance(t *testing.T) {
	cache, err := newDocCache()
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	t.Run("without API", func(t *testing.T) {
		guidance := buildToolGuidance(cache, false)
		if !strings.Contains(guidance, "MUST use the available tools") {
			t.Error("missing tool-use requirement")
		}
		if strings.Contains(guidance, "get_developer_document") {
			t.Error("should not mention API tools when hasAPI=false")
		}
		if !strings.Contains(guidance, "get_cached_document") {
			t.Error("should mention cache tools")
		}
	})

	t.Run("with API", func(t *testing.T) {
		guidance := buildToolGuidance(cache, true)
		if !strings.Contains(guidance, "get_developer_document") {
			t.Error("should mention API tools when hasAPI=true")
		}
		if !strings.Contains(guidance, "search_developer_docs") {
			t.Error("should mention API search tool when hasAPI=true")
		}
	})
}

func TestNormalizeDocName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
			want:  "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
		},
		{
			input: "docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
			want:  "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
		},
		{
			input: "https://docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
			want:  "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
		},
		{
			input: "http://docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
			want:  "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
		},
		{
			input: "reference/standard-sql/query-syntax",
			want:  "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax",
		},
		{
			input: "reference/standard-sql/graph-query-statements",
			want:  "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/graph-query-statements",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeDocName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDocName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// writeJSON is a test helper that encodes v as JSON to w and fails the test on error.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
}

func newTestDevKnowledgeClient(t *testing.T, handler http.HandlerFunc) (*devKnowledgeClient, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &devKnowledgeClient{
		baseURL: server.URL,
		apiKey:  "test-api-key",
		client:  server.Client(),
		limiter: rate.NewLimiter(rate.Inf, 1),
	}, server.URL
}

func newTestDocSearcher(t *testing.T, handler http.HandlerFunc) *devKnowledgeDocSearcher {
	t.Helper()
	client, _ := newTestDevKnowledgeClient(t, handler)
	return &devKnowledgeDocSearcher{client: client}
}

func TestDevKnowledgeClient_DoGet_APIKeyHeader(t *testing.T) {
	var gotHeader string
	client, serverURL := newTestDevKnowledgeClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-goog-api-key")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	_, err := client.doGet(context.Background(), serverURL+"/test")
	if err != nil {
		t.Fatalf("doGet failed: %v", err)
	}
	if gotHeader != "test-api-key" {
		t.Errorf("API key header = %q, want %q", gotHeader, "test-api-key")
	}
}

func TestDevKnowledgeClient_DoGet_RetryOn429(t *testing.T) {
	attempts := 0
	client, serverURL := newTestDevKnowledgeClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":429,"message":"rate limited"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	body, err := client.doGet(context.Background(), serverURL+"/test")
	if err != nil {
		t.Fatalf("doGet failed: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("expected ok:true, got %v", result)
	}
}

func TestDevKnowledgeClient_DoGet_MaxRetriesExceeded(t *testing.T) {
	attempts := 0
	client, serverURL := newTestDevKnowledgeClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"rate limited"}}`))
	})

	_, err := client.doGet(context.Background(), serverURL+"/test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// After maxRetries (3) attempts with 429, the last one is returned as an API error
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	var apiErr *devKnowledgeAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected devKnowledgeAPIError, got %T: %v", err, err)
	}
	if apiErr.Code != 429 {
		t.Errorf("error code = %d, want 429", apiErr.Code)
	}
}

func TestDevKnowledgeClient_DoGet_APIError(t *testing.T) {
	client, serverURL := newTestDevKnowledgeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"status":"NOT_FOUND","message":"document not found"}}`))
	})

	_, err := client.doGet(context.Background(), serverURL+"/test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *devKnowledgeAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected devKnowledgeAPIError, got %T: %v", err, err)
	}
	if apiErr.Code != 404 {
		t.Errorf("error code = %d, want 404", apiErr.Code)
	}
	if apiErr.Status != "NOT_FOUND" {
		t.Errorf("error status = %q, want %q", apiErr.Status, "NOT_FOUND")
	}
}

func TestDevKnowledgeAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *devKnowledgeAPIError
		want string
	}{
		{
			name: "with status",
			err:  &devKnowledgeAPIError{Code: 404, Status: "NOT_FOUND", Message: "doc not found"},
			want: "API error 404 (NOT_FOUND): doc not found",
		},
		{
			name: "without status",
			err:  &devKnowledgeAPIError{Code: 500, Message: "internal error"},
			want: "HTTP 500: internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDevKnowledgeDocSearcher_Search(t *testing.T) {
	searcher := newTestDocSearcher(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/documents:searchDocumentChunks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query().Get("query")
		if query != "spanner GQL" {
			t.Errorf("query = %q, want %q", query, "spanner GQL")
		}
		writeJSON(t, w, map[string]any{
			"results": []map[string]any{
				{"parent": "documents/docs.cloud.google.com/spanner/docs/graph", "id": "chunk1", "content": "Graph query language docs"},
			},
		})
	})

	results, err := searcher.Search(context.Background(), "spanner GQL")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Name != "documents/docs.cloud.google.com/spanner/docs/graph" {
		t.Errorf("result name = %q", results[0].Name)
	}
	if results[0].Snippet != "Graph query language docs" {
		t.Errorf("result snippet = %q", results[0].Snippet)
	}
}

func TestDevKnowledgeDocSearcher_GetDocument(t *testing.T) {
	searcher := newTestDocSearcher(t, func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		writeJSON(t, w, map[string]any{
			"content": "# Query Syntax\nSELECT ...",
		})
	})

	content, err := searcher.GetDocument(context.Background(), "https://docs.cloud.google.com/spanner/docs/reference/standard-sql/query-syntax")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if content != "# Query Syntax\nSELECT ..." {
		t.Errorf("content = %q", content)
	}
}

func TestDevKnowledgeDocSearcher_BatchGetDocuments_Success(t *testing.T) {
	searcher := newTestDocSearcher(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/documents:batchGet" {
			t.Errorf("path = %q, want /documents:batchGet", r.URL.Path)
		}
		names := r.URL.Query()["names"]
		if len(names) != 2 {
			t.Errorf("got %d names, want 2", len(names))
		}
		writeJSON(t, w, map[string]any{
			"documents": []map[string]any{
				{"name": names[0], "content": "content1"},
				{"name": names[1], "content": "content2"},
			},
		})
	})

	docs, err := searcher.BatchGetDocuments(context.Background(), []string{
		"docs.cloud.google.com/spanner/docs/ref1",
		"docs.cloud.google.com/spanner/docs/ref2",
	})
	if err != nil {
		t.Fatalf("BatchGetDocuments failed: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}
}

func TestDevKnowledgeDocSearcher_BatchGetDocuments_DoesNotMutateInput(t *testing.T) {
	searcher := newTestDocSearcher(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"documents": []map[string]any{
				{"name": "documents/docs.cloud.google.com/spanner/docs/ref1", "content": "c1"},
			},
		})
	})

	input := []string{"docs.cloud.google.com/spanner/docs/ref1"}
	inputCopy := make([]string, len(input))
	copy(inputCopy, input)

	_, err := searcher.BatchGetDocuments(context.Background(), input)
	if err != nil {
		t.Fatalf("BatchGetDocuments failed: %v", err)
	}

	for i, v := range input {
		if v != inputCopy[i] {
			t.Errorf("input[%d] was mutated: got %q, want %q", i, v, inputCopy[i])
		}
	}
}

func TestDevKnowledgeDocSearcher_BatchGetDocuments_FallbackOnBatchError(t *testing.T) {
	callCount := 0
	searcher := newTestDocSearcher(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/documents:batchGet" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"invalid name"}}`))
			return
		}
		writeJSON(t, w, map[string]any{
			"content": "individual content",
		})
	})

	docs, err := searcher.BatchGetDocuments(context.Background(), []string{
		"documents/docs.cloud.google.com/spanner/docs/ref1",
	})
	if err != nil {
		t.Fatalf("BatchGetDocuments failed: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1 (from individual fallback)", len(docs))
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 calls (batch + individual), got %d", callCount)
	}
}

func TestDevKnowledgeDocSearcher_Search_SnippetTruncation(t *testing.T) {
	longContent := make([]byte, 400)
	for i := range longContent {
		longContent[i] = 'a'
	}

	searcher := newTestDocSearcher(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"results": []map[string]any{
				{"parent": "documents/test", "id": "1", "content": string(longContent)},
			},
		})
	})

	results, err := searcher.Search(context.Background(), "test")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	// 300 chars + "..."
	if len(results[0].Snippet) != 303 {
		t.Errorf("snippet length = %d, want 303", len(results[0].Snippet))
	}
}

func TestExecuteToolCall_GetCachedDocument(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	c.Put("documents/docs.cloud.google.com/spanner/docs/ref/doc1", "content1")

	resp := executeToolCall(context.Background(), &genai.FunctionCall{
		Name: "get_cached_document",
		Args: map[string]any{
			"names": []any{"documents/docs.cloud.google.com/spanner/docs/ref/doc1", "documents/missing"},
		},
	}, c)

	docs, ok := resp["documents"].(map[string]any)
	if !ok {
		t.Fatalf("expected documents map, got %T", resp["documents"])
	}
	if docs["documents/docs.cloud.google.com/spanner/docs/ref/doc1"] != "content1" {
		t.Errorf("expected content1, got %v", docs["documents/docs.cloud.google.com/spanner/docs/ref/doc1"])
	}
	// Missing doc should have error
	errMap, ok := docs["documents/missing"].(map[string]any)
	if !ok {
		t.Fatalf("expected error map for missing doc, got %T", docs["documents/missing"])
	}
	if errMap["error"] != "not found in cache" {
		t.Errorf("expected 'not found in cache', got %v", errMap["error"])
	}
}

func TestExecuteToolCall_SearchCachedDocuments(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	c.Put("documents/doc1", "Cloud Spanner GQL graph patterns")
	c.Put("documents/doc2", "Unrelated content")

	resp := executeToolCall(context.Background(), &genai.FunctionCall{
		Name: "search_cached_documents",
		Args: map[string]any{
			"queries": []any{"spanner GQL"},
		},
	}, c)

	queryResults, ok := resp["query_results"].(map[string]any)
	if !ok {
		t.Fatalf("expected query_results map, got %T", resp["query_results"])
	}
	qr, ok := queryResults["spanner GQL"].(map[string]any)
	if !ok {
		t.Fatalf("expected query result map, got %T", queryResults["spanner GQL"])
	}
	count, _ := qr["count"].(int)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestExecuteToolCall_UnknownFunction(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	resp := executeToolCall(context.Background(), &genai.FunctionCall{
		Name: "nonexistent_tool",
		Args: map[string]any{},
	}, c)

	errMsg, ok := resp["error"].(string)
	if !ok {
		t.Fatalf("expected error string, got %T", resp["error"])
	}
	if !strings.Contains(errMsg, "nonexistent_tool") {
		t.Errorf("error should mention function name: %q", errMsg)
	}
}

func TestExecuteToolCall_SearchDeveloperDocs_FallbackToLocal(t *testing.T) {
	t.Parallel()
	// Cache without API searcher — should fall back to local search
	c := newTestCache(t)
	c.Put("documents/doc1", "Spanner query syntax reference")

	resp := executeToolCall(context.Background(), &genai.FunctionCall{
		Name: "search_developer_docs",
		Args: map[string]any{
			"queries": []any{"spanner query"},
		},
	}, c)

	queryResults, ok := resp["query_results"].(map[string]any)
	if !ok {
		t.Fatalf("expected query_results map, got %T", resp["query_results"])
	}
	qr, ok := queryResults["spanner query"].(map[string]any)
	if !ok {
		t.Fatalf("expected query result, got %T", queryResults["spanner query"])
	}
	// Should indicate local_cache fallback
	if qr["source"] != "local_cache" {
		t.Errorf("expected source=local_cache, got %v", qr["source"])
	}
}
