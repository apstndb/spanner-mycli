package mycli

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

// docCacheEntry stores a compressed document with its fetch timestamp.
type docCacheEntry struct {
	data      []byte    // zstd compressed
	fetchedAt time.Time // zero for embedded docs
}

// docFetcher fetches a single document by name from an external API.
type docFetcher func(ctx context.Context, name string) (string, error)

// docBatchFetcher fetches multiple documents by name from an external API.
type docBatchFetcher func(ctx context.Context, names []string) ([]DocResult, error)

// docAPISearcher performs semantic search via an external API.
type docAPISearcher func(ctx context.Context, query string) ([]DocSearchResult, error)

// docCache is a session-scoped, zstd-compressed document cache with TTL-based
// revalidation. Entries are populated from embedded docs at startup and
// enriched by API fetches during the session.
type docCache struct {
	mu         sync.RWMutex
	entries    map[string]docCacheEntry
	ttl        time.Duration
	fetch      docFetcher      // nil if no API key
	batchFetch docBatchFetcher // nil if no API key
	apiSearch  docAPISearcher  // nil if no API key
	encoder    *zstd.Encoder
	decoder    *zstd.Decoder
	nowFunc    func() time.Time // for testing; defaults to time.Now
}

// docCacheOption configures a docCache.
type docCacheOption func(*docCache)

// withDocFetcher sets the single-document fetch function (requires API key).
func withDocFetcher(f docFetcher) docCacheOption {
	return func(c *docCache) { c.fetch = f }
}

// withDocBatchFetcher sets the batch fetch function (requires API key).
func withDocBatchFetcher(f docBatchFetcher) docCacheOption {
	return func(c *docCache) { c.batchFetch = f }
}

// withDocAPISearcher sets the API semantic search function (requires API key).
func withDocAPISearcher(f docAPISearcher) docCacheOption {
	return func(c *docCache) { c.apiSearch = f }
}

// withCacheTTL overrides the default TTL.
func withCacheTTL(ttl time.Duration) docCacheOption {
	return func(c *docCache) { c.ttl = ttl }
}

// withNowFunc overrides time.Now for testing.
func withNowFunc(f func() time.Time) docCacheOption {
	return func(c *docCache) { c.nowFunc = f }
}

const defaultCacheTTL = 3 * time.Hour

// newDocCache creates a new document cache.
func newDocCache(opts ...docCacheOption) (*docCache, error) {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("zstd encoder: %w", err)
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("zstd decoder: %w", err)
	}

	c := &docCache{
		entries: make(map[string]docCacheEntry),
		ttl:     defaultCacheTTL,
		encoder: enc,
		decoder: dec,
		nowFunc: time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Close releases zstd resources.
func (c *docCache) Close() {
	_ = c.encoder.Close()
	c.decoder.Close()
}

// compress compresses content using zstd.
func (c *docCache) compress(content string) []byte {
	return c.encoder.EncodeAll([]byte(content), nil)
}

// decompress decompresses zstd data.
func (c *docCache) decompress(data []byte) (string, error) {
	b, err := c.decoder.DecodeAll(data, nil)
	if err != nil {
		return "", fmt.Errorf("zstd decompress: %w", err)
	}
	return string(b), nil
}

// isFresh returns whether an entry is within TTL.
func (c *docCache) isFresh(e docCacheEntry) bool {
	if e.fetchedAt.IsZero() {
		return false // embedded docs are always considered stale if API is available
	}
	return c.nowFunc().Sub(e.fetchedAt) < c.ttl
}

// LoadCompressed loads a pre-compressed document into the cache (e.g., from embedded .zst files).
// The fetchedAt is set to zero, meaning it will be refreshed from API on first access if available.
func (c *docCache) LoadCompressed(name string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[name] = docCacheEntry{data: data}
}

// LoadContent loads an uncompressed document into the cache.
// The fetchedAt is set to zero (treated as embedded).
func (c *docCache) LoadContent(name, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[name] = docCacheEntry{data: c.compress(content)}
}

// Put stores a freshly fetched document in the cache.
func (c *docCache) Put(name, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(name, content)
}

// putLocked stores a document without acquiring the lock. Caller must hold c.mu.
func (c *docCache) putLocked(name, content string) {
	c.entries[name] = docCacheEntry{
		data:      c.compress(content),
		fetchedAt: c.nowFunc(),
	}
}

// Get retrieves a document from the cache with TTL-based revalidation.
// If the entry is stale and a fetch function is available, it attempts to refresh.
// Stale content is returned if refresh fails.
// The entire operation is atomic: the lock is held during API fetch to prevent
// concurrent check-then-act races (see .gemini/styleguide.md §Concurrency).
func (c *docCache) Get(ctx context.Context, name string) (string, bool) {
	name = normalizeDocName(name)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[name]
	if exists && c.isFresh(entry) {
		content, err := c.decompress(entry.data)
		if err != nil {
			slog.Warn("Failed to decompress cached document", "name", name, "error", err)
			return "", false
		}
		return content, true
	}

	// Stale or missing — try API refresh if available.
	// Network I/O under lock is acceptable for infrequent CLI operations.
	if c.fetch != nil {
		content, err := c.fetch(ctx, name)
		if err == nil {
			c.putLocked(name, content)
			return content, true
		}
		slog.Debug("API refresh failed, using stale cache", "name", name, "error", err)
	}

	// Return stale content if available
	if exists {
		content, err := c.decompress(entry.data)
		if err != nil {
			return "", false
		}
		return content, true
	}

	return "", false
}

// BatchGet retrieves multiple documents, fetching only missing/stale ones from the API.
// The entire operation is atomic: the lock is held during API fetch to prevent
// concurrent check-then-act races (see .gemini/styleguide.md §Concurrency).
func (c *docCache) BatchGet(ctx context.Context, names []string) []DocResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	var results []DocResult
	var toFetch []string
	staleEntries := make(map[string]docCacheEntry)

	// Classify each name: fresh hit, stale, or miss
	for _, name := range names {
		name = normalizeDocName(name)
		entry, exists := c.entries[name]
		if exists && c.isFresh(entry) {
			content, err := c.decompress(entry.data)
			if err == nil {
				results = append(results, DocResult{Name: name, Content: content})
				continue
			}
		}
		toFetch = append(toFetch, name)
		if exists {
			staleEntries[name] = entry
		}
	}

	if len(toFetch) == 0 {
		return results
	}

	// Try batch fetch for stale/missing entries.
	// Network I/O under lock is acceptable for infrequent CLI operations.
	if c.batchFetch != nil {
		fetched, err := c.batchFetch(ctx, toFetch)
		if err == nil {
			fetchedSet := make(map[string]bool, len(fetched))
			for _, doc := range fetched {
				c.putLocked(doc.Name, doc.Content)
				results = append(results, doc)
				fetchedSet[doc.Name] = true
			}
			// For names not in fetch result, fall back to stale
			for _, name := range toFetch {
				if fetchedSet[name] {
					continue
				}
				if stale, ok := staleEntries[name]; ok {
					if content, err := c.decompress(stale.data); err == nil {
						results = append(results, DocResult{Name: name, Content: content})
					}
				}
			}
			return results
		}
		slog.Debug("Batch fetch failed, falling back to individual fetch or stale", "error", err)
	}

	// Fall back to individual fetch or stale entries
	for _, name := range toFetch {
		if c.fetch != nil {
			content, err := c.fetch(ctx, name)
			if err == nil {
				c.putLocked(name, content)
				results = append(results, DocResult{Name: name, Content: content})
				continue
			}
		}
		// Use stale entry if available
		if stale, ok := staleEntries[name]; ok {
			if content, err := c.decompress(stale.data); err == nil {
				results = append(results, DocResult{Name: name, Content: content})
			}
		}
	}

	return results
}

// APISearch performs a semantic search via the Developer Knowledge API.
// Returns nil, false if no API searcher is configured.
func (c *docCache) APISearch(ctx context.Context, query string) ([]DocSearchResult, bool) {
	if c.apiSearch == nil {
		return nil, false
	}
	results, err := c.apiSearch(ctx, query)
	if err != nil {
		slog.Debug("API search failed", "error", err)
		return nil, false
	}
	return results, true
}

// Search performs a simple text search across all cached documents.
func (c *docCache) Search(query string) []DocSearchResult {
	query = strings.ToLower(query)
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []DocSearchResult
	for name, entry := range c.entries {
		content, err := c.decompress(entry.data)
		if err != nil {
			continue
		}
		lower := strings.ToLower(content)

		// All query words must appear in the document
		allMatch := true
		for _, word := range words {
			if !strings.Contains(lower, word) {
				allMatch = false
				break
			}
		}
		if !allMatch {
			continue
		}

		// Extract a snippet around the first match
		snippet := extractSnippet(content, words[0], 300)
		results = append(results, DocSearchResult{Name: name, Snippet: snippet})
	}
	return results
}

// Names returns all cached document names.
func (c *docCache) Names() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		names = append(names, name)
	}
	return names
}

// extractSnippet extracts a text snippet around the first occurrence of term.
func extractSnippet(content, term string, maxLen int) string {
	lower := strings.ToLower(content)
	term = strings.ToLower(term)
	idx := strings.Index(lower, term)
	if idx < 0 {
		if len(content) > maxLen {
			return content[:maxLen] + "..."
		}
		return content
	}

	// Center the snippet around the match
	start := idx - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}
	return snippet
}
