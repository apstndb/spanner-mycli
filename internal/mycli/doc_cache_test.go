package mycli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func newTestCache(t *testing.T, opts ...docCacheOption) *docCache {
	t.Helper()
	c, err := newDocCache(opts...)
	if err != nil {
		t.Fatalf("newDocCache: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestDocCache_PutAndGet(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.Put("documents/test/doc1", "hello world")

	content, ok := c.Get(context.Background(), "documents/test/doc1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if content != "hello world" {
		t.Errorf("content = %q, want %q", content, "hello world")
	}
}

func TestDocCache_GetNormalizesName(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.Put("documents/docs.cloud.google.com/spanner/docs/ref", "content")

	// Get with https:// prefix should still hit
	content, ok := c.Get(context.Background(), "https://docs.cloud.google.com/spanner/docs/ref")
	if !ok {
		t.Fatal("expected cache hit with normalized name")
	}
	if content != "content" {
		t.Errorf("content = %q", content)
	}
}

func TestDocCache_LoadCompressed(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	// Compress manually, then load
	compressed := c.compress("pre-compressed content")
	c.LoadCompressed("documents/test/compressed", compressed)

	content, ok := c.Get(context.Background(), "documents/test/compressed")
	// Embedded docs (fetchedAt=zero) are stale, but returned when no API
	if !ok {
		t.Fatal("expected cache hit for embedded doc")
	}
	if content != "pre-compressed content" {
		t.Errorf("content = %q", content)
	}
}

func TestDocCache_LoadContent(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.LoadContent("documents/test/doc", "loaded content")

	content, ok := c.Get(context.Background(), "documents/test/doc")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if content != "loaded content" {
		t.Errorf("content = %q", content)
	}
}

func TestDocCache_TTL_FreshEntry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fetchCalls := 0

	c := newTestCache(t,
		withNowFunc(func() time.Time { return now }),
		withDocFetcher(func(_ context.Context, name string) (string, error) {
			fetchCalls++
			return "refreshed", nil
		}),
	)

	c.Put("documents/test/doc", "original")

	// Access within TTL — should not call fetcher
	content, ok := c.Get(context.Background(), "documents/test/doc")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if content != "original" {
		t.Errorf("content = %q, want %q", content, "original")
	}
	if fetchCalls != 0 {
		t.Errorf("fetchCalls = %d, want 0", fetchCalls)
	}
}

func TestDocCache_TTL_StaleEntryRefreshes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fetchCalls := 0

	c := newTestCache(t,
		withCacheTTL(1*time.Hour),
		withNowFunc(func() time.Time { return now }),
		withDocFetcher(func(_ context.Context, name string) (string, error) {
			fetchCalls++
			return "refreshed", nil
		}),
	)

	c.Put("documents/test/doc", "original")

	// Advance time past TTL
	now = now.Add(2 * time.Hour)

	content, ok := c.Get(context.Background(), "documents/test/doc")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if content != "refreshed" {
		t.Errorf("content = %q, want %q", content, "refreshed")
	}
	if fetchCalls != 1 {
		t.Errorf("fetchCalls = %d, want 1", fetchCalls)
	}
}

func TestDocCache_TTL_StaleEntryReturnedOnFetchError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	c := newTestCache(t,
		withCacheTTL(1*time.Hour),
		withNowFunc(func() time.Time { return now }),
		withDocFetcher(func(_ context.Context, name string) (string, error) {
			return "", fmt.Errorf("API down")
		}),
	)

	c.Put("documents/test/doc", "stale content")

	// Advance past TTL
	now = now.Add(2 * time.Hour)

	content, ok := c.Get(context.Background(), "documents/test/doc")
	if !ok {
		t.Fatal("expected stale cache hit")
	}
	if content != "stale content" {
		t.Errorf("content = %q, want %q", content, "stale content")
	}
}

func TestDocCache_EmbeddedDocRefreshedWhenAPIAvailable(t *testing.T) {
	t.Parallel()
	c := newTestCache(t,
		withDocFetcher(func(_ context.Context, name string) (string, error) {
			return "fresh from API", nil
		}),
	)

	// Embedded doc (fetchedAt=zero, always stale)
	c.LoadContent("documents/test/embedded", "embedded content")

	content, ok := c.Get(context.Background(), "documents/test/embedded")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if content != "fresh from API" {
		t.Errorf("content = %q, want %q", content, "fresh from API")
	}
}

func TestDocCache_EmbeddedDocServedWithoutAPI(t *testing.T) {
	t.Parallel()
	// No fetcher — embedded docs should be served as-is
	c := newTestCache(t)

	c.LoadContent("documents/test/embedded", "embedded content")

	content, ok := c.Get(context.Background(), "documents/test/embedded")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if content != "embedded content" {
		t.Errorf("content = %q, want %q", content, "embedded content")
	}
}

func TestDocCache_GetMissingWithoutAPI(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	_, ok := c.Get(context.Background(), "documents/nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestDocCache_GetMissingWithAPI(t *testing.T) {
	t.Parallel()
	c := newTestCache(t,
		withDocFetcher(func(_ context.Context, name string) (string, error) {
			return "fetched from API", nil
		}),
	)

	content, ok := c.Get(context.Background(), "documents/test/new")
	if !ok {
		t.Fatal("expected cache hit from API fetch")
	}
	if content != "fetched from API" {
		t.Errorf("content = %q", content)
	}

	// Second access should be from cache (fresh)
	content2, ok := c.Get(context.Background(), "documents/test/new")
	if !ok || content2 != "fetched from API" {
		t.Errorf("second Get: content = %q, ok = %v", content2, ok)
	}
}

func TestDocCache_BatchGet_AllFresh(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.Put("documents/doc1", "content1")
	c.Put("documents/doc2", "content2")

	results := c.BatchGet(context.Background(), []string{"documents/doc1", "documents/doc2"})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestDocCache_BatchGet_MixedCacheAndFetch(t *testing.T) {
	t.Parallel()
	batchCalls := 0

	c := newTestCache(t,
		withDocBatchFetcher(func(_ context.Context, names []string) ([]DocResult, error) {
			batchCalls++
			var results []DocResult
			for _, name := range names {
				results = append(results, DocResult{Name: name, Content: "fetched:" + name})
			}
			return results, nil
		}),
	)

	c.Put("documents/cached", "cached content")
	// documents/uncached is not in cache

	results := c.BatchGet(context.Background(), []string{"documents/cached", "documents/uncached"})

	want := []DocResult{
		{Name: "documents/cached", Content: "cached content"},
		{Name: "documents/uncached", Content: "fetched:documents/uncached"},
	}

	sortByName := cmpopts.SortSlices(func(a, b DocResult) bool { return a.Name < b.Name })
	if diff := cmp.Diff(want, results, sortByName); diff != "" {
		t.Errorf("BatchGet mismatch (-want +got):\n%s", diff)
	}
	if batchCalls != 1 {
		t.Errorf("batchCalls = %d, want 1", batchCalls)
	}
}

func TestDocCache_BatchGet_FallbackToIndividual(t *testing.T) {
	t.Parallel()
	fetchCalls := 0

	c := newTestCache(t,
		withDocBatchFetcher(func(_ context.Context, names []string) ([]DocResult, error) {
			return nil, fmt.Errorf("batch failed")
		}),
		withDocFetcher(func(_ context.Context, name string) (string, error) {
			fetchCalls++
			return "individual:" + name, nil
		}),
	)

	results := c.BatchGet(context.Background(), []string{"documents/doc1", "documents/doc2"})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if fetchCalls != 2 {
		t.Errorf("fetchCalls = %d, want 2", fetchCalls)
	}
}

func TestDocCache_BatchGet_StaleOnFetchFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	c := newTestCache(t,
		withCacheTTL(1*time.Hour),
		withNowFunc(func() time.Time { return now }),
		withDocFetcher(func(_ context.Context, name string) (string, error) {
			return "", fmt.Errorf("API down")
		}),
	)

	c.Put("documents/doc1", "stale content")

	// Advance past TTL
	now = now.Add(2 * time.Hour)

	results := c.BatchGet(context.Background(), []string{"documents/doc1"})
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Content != "stale content" {
		t.Errorf("content = %q, want %q", results[0].Content, "stale content")
	}
}

func TestDocCache_Search(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.Put("documents/doc1", "Cloud Spanner GQL graph patterns")
	c.Put("documents/doc2", "Cloud Spanner GoogleSQL query syntax")
	c.Put("documents/doc3", "Unrelated content about bigtable")

	results := c.Search("spanner GQL")
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Name != "documents/doc1" {
		t.Errorf("result name = %q, want %q", results[0].Name, "documents/doc1")
	}
}

func TestDocCache_Search_CaseInsensitive(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.Put("documents/doc1", "SELECT * FROM MyTable")

	results := c.Search("select")
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

func TestDocCache_Search_MultipleWords(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.Put("documents/doc1", "Cloud Spanner graph query")
	c.Put("documents/doc2", "Cloud Spanner SQL query")

	// Both words must match
	results := c.Search("graph query")
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Name != "documents/doc1" {
		t.Errorf("result name = %q", results[0].Name)
	}
}

func TestDocCache_Names(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	c.Put("documents/a", "a")
	c.Put("documents/b", "b")

	names := c.Names()
	sortStrings := cmpopts.SortSlices(func(a, b string) bool { return a < b })
	if diff := cmp.Diff([]string{"documents/a", "documents/b"}, names, sortStrings); diff != "" {
		t.Errorf("Names mismatch (-want +got):\n%s", diff)
	}
}

func TestDocCache_CompressionRoundtrip(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	// Test with various content types
	contents := []string{
		"",
		"short",
		"# Heading\n\nSome markdown with `code` and **bold**",
		string(make([]byte, 100000)), // large content
	}

	for i, content := range contents {
		name := fmt.Sprintf("documents/test/%d", i)
		c.Put(name, content)

		got, ok := c.Get(context.Background(), name)
		if !ok {
			t.Errorf("case %d: expected cache hit", i)
			continue
		}
		if got != content {
			t.Errorf("case %d: roundtrip mismatch, len(got)=%d, len(want)=%d", i, len(got), len(content))
		}
	}
}

func TestExtractSnippet(t *testing.T) {
	t.Parallel()

	t.Run("short content unchanged", func(t *testing.T) {
		t.Parallel()
		result := extractSnippet("hello world", "hello", 300)
		if result != "hello world" {
			t.Errorf("got %q, want %q", result, "hello world")
		}
	})

	t.Run("term not found truncates from start", func(t *testing.T) {
		t.Parallel()
		result := extractSnippet("some content here", "missing", 8)
		if !strings.HasSuffix(result, "...") {
			t.Errorf("expected ellipsis suffix, got %q", result)
		}
		// 8 runes + "..." = "some con..."
		runes := []rune(strings.TrimSuffix(result, "..."))
		if len(runes) > 8 {
			t.Errorf("rune count = %d, want <= 8; snippet = %q", len(runes), result)
		}
	})

	t.Run("long content centered around match", func(t *testing.T) {
		t.Parallel()
		padding := strings.Repeat("x", 500)
		content := padding + "TARGET" + padding
		result := extractSnippet(content, "TARGET", 100)
		if !strings.Contains(result, "TARGET") {
			t.Errorf("snippet should contain TARGET: %q", result)
		}
		if !strings.HasPrefix(result, "...") {
			t.Error("expected leading ellipsis for centered snippet")
		}
	})

	t.Run("UTF-8 safe truncation", func(t *testing.T) {
		t.Parallel()
		// Japanese text should not be split mid-rune
		content := strings.Repeat("あ", 200)
		result := extractSnippet(content, "あ", 20)
		// Every rune in the result should be valid
		for i, r := range result {
			if r == '\uFFFD' {
				t.Errorf("invalid rune at position %d in %q", i, result)
			}
		}
	})
}

func TestDocCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want string
	}{
		{name: docPrefix + "reference/standard-sql/graph-patterns", want: "GQL"},
		{name: docPrefix + "reference/standard-sql/graph-sql-functions", want: "GQL"},
		{name: docPrefix + "reference/standard-sql/graph-intro", want: "GQL"},
		{name: docPrefix + "reference/standard-sql/query-syntax", want: "GoogleSQL"},
		{name: docPrefix + "reference/standard-sql/data-types", want: "GoogleSQL"},
		{name: docPrefix + "reference/standard-sql/operators", want: "GoogleSQL"},
		{name: docPrefix + "reference/standard-sql/aggregate_functions", want: "GoogleSQL Functions"},
		{name: docPrefix + "reference/standard-sql/functions-all", want: "GoogleSQL Functions"},
		{name: docPrefix + "reference/standard-sql/aggregate-function-calls", want: "GoogleSQL Functions"},
		{name: docPrefix + "apis", want: "Other"},
		{name: docPrefix + "some/other/path", want: "Other"},
	}
	for _, tt := range tests {
		t.Run(tt.want+"/"+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := docCategory(tt.name); got != tt.want {
				t.Errorf("docCategory(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestFormatDocCatalog(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	// Simulate embedded docs being loaded (matching catalog entries)
	c.Put(docPrefix+"reference/standard-sql/graph-patterns", "graph patterns content")
	// Add an extra cached doc not in the catalog
	c.Put("documents/docs.cloud.google.com/spanner/docs/some/extra/doc", "extra")

	result := formatDocCatalog(c)

	// Catalog entries should be present with categories
	if !strings.Contains(result, "GQL:") {
		t.Error("missing GQL category")
	}
	if !strings.Contains(result, "GoogleSQL:") {
		t.Error("missing GoogleSQL category")
	}
	if !strings.Contains(result, "GoogleSQL Functions:") {
		t.Error("missing GoogleSQL Functions category")
	}
	// Cached doc should be marked
	if !strings.Contains(result, "graph-patterns") || !strings.Contains(result, "[cached]") {
		t.Error("cached graph-patterns not marked")
	}
	// Non-cached catalog entry should appear without [cached]
	if !strings.Contains(result, "graph-schema-statements") {
		t.Error("missing catalog entry graph-schema-statements")
	}
	// Extra cached doc should appear in additional section
	if !strings.Contains(result, "Additional cached documents:") {
		t.Error("missing additional cached documents section")
	}
	if !strings.Contains(result, "some/extra/doc") {
		t.Error("missing extra cached doc")
	}
}

func TestLoadEmbeddedDocs(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	if err := loadEmbeddedDocs(c); err != nil {
		t.Fatalf("loadEmbeddedDocs: %v", err)
	}

	// Should have loaded the embedded docs
	names := c.Names()
	if len(names) == 0 {
		t.Fatal("no embedded docs loaded")
	}

	// All loaded docs should be decompressible
	for _, name := range names {
		content, ok := c.Get(context.Background(), name)
		if !ok {
			t.Errorf("failed to get embedded doc %q", name)
			continue
		}
		if len(content) == 0 {
			t.Errorf("embedded doc %q has empty content", name)
		}
	}

	// Every catalog entry should have been loaded
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	for _, doc := range docCatalog {
		if !nameSet[doc.Name] {
			t.Errorf("catalog entry %q (base=%q) not loaded from embedded docs", doc.Name, docEmbedBase(doc.Name))
		}
	}
}
