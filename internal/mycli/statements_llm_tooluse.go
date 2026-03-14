package mycli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

// DocSearchResult represents a single search result chunk.
type DocSearchResult struct {
	Name    string // Document identifier (e.g., "documents/docs.cloud.google.com/spanner/docs/...")
	Snippet string // Brief text snippet from the document
}

// DocResult represents a fetched document with its name and content.
type DocResult struct {
	Name    string
	Content string
}

const maxToolCallRounds = 5

// docPrefix is the common prefix for Spanner reference document names.
const docPrefix = "documents/docs.cloud.google.com/spanner/docs/"

// --- Tool declarations ---

// Cache tools (always available):
//   get_cached_document      - retrieve one or more docs from local cache (batch)
//   search_cached_documents  - keyword search over cache (multi-query)
//
// API tools (only when API key is set):
//   search_developer_docs    - semantic search via Developer Knowledge API (multi-query)
//   get_developer_document   - fetch one or more docs from API → cache (batch)

func buildToolDeclarations(hasAPI bool) []*genai.Tool {
	decls := []*genai.FunctionDeclaration{
		{
			Name:        "get_cached_document",
			Description: "Retrieve one or more documents from the local cache. Returns cached content (may be from embedded docs or previously fetched). Use document names from the available documents list.",
			Parameters: &genai.Schema{
				Type: "OBJECT",
				Properties: map[string]*genai.Schema{
					"names": {
						Type:        "ARRAY",
						Description: "List of document names to retrieve (e.g., ['documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/graph-query-statements'])",
						Items: &genai.Schema{
							Type: "STRING",
						},
					},
				},
				Required: []string{"names"},
			},
		},
		{
			Name:        "search_cached_documents",
			Description: "Search locally cached documents by keyword(s). Accepts multiple queries; returns matching document names and snippets for each query. All query words must appear in the document (case-insensitive).",
			Parameters: &genai.Schema{
				Type: "OBJECT",
				Properties: map[string]*genai.Schema{
					"queries": {
						Type:        "ARRAY",
						Description: "List of space-separated keyword queries (e.g., ['graph pattern matching', 'aggregate functions'])",
						Items: &genai.Schema{
							Type: "STRING",
						},
					},
				},
				Required: []string{"queries"},
			},
		},
	}

	if hasAPI {
		decls = append(decls,
			&genai.FunctionDeclaration{
				Name:        "search_developer_docs",
				Description: "Search Google developer documentation using semantic search. Accepts multiple queries; returns relevant document names and snippets for each query. Use this to discover documents not in the local cache.",
				Parameters: &genai.Schema{
					Type: "OBJECT",
					Properties: map[string]*genai.Schema{
						"queries": {
							Type:        "ARRAY",
							Description: "List of search queries (e.g., ['Spanner GQL graph pattern matching', 'GoogleSQL aggregate functions'])",
							Items: &genai.Schema{
								Type: "STRING",
							},
						},
					},
					Required: []string{"queries"},
				},
			},
			&genai.FunctionDeclaration{
				Name:        "get_developer_document",
				Description: "Fetch one or more documents from the Developer Knowledge API. Results are cached for future use. Use for documents not in the local cache.",
				Parameters: &genai.Schema{
					Type: "OBJECT",
					Properties: map[string]*genai.Schema{
						"names": {
							Type:        "ARRAY",
							Description: "List of document names to retrieve (up to 20). Uses batch API when possible.",
							Items: &genai.Schema{
								Type: "STRING",
							},
						},
					},
					Required: []string{"names"},
				},
			},
		)
	}

	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// buildToolGuidance adds tool-use instructions to the system prompt.
func buildToolGuidance(cache *docCache, hasAPI bool) string {
	var b strings.Builder
	b.WriteString("\n\nIMPORTANT: You MUST use the available tools to look up relevant documentation before composing any query.\n")
	b.WriteString("Do NOT skip tool use. Always fetch at least the most relevant document(s) for the query topic.\n")

	b.WriteString("IMPORTANT: All tools accept arrays/lists to reduce round trips. Batch multiple operations per call.\n")

	if hasAPI {
		b.WriteString("Use get_developer_document with multiple names to fetch several documents at once.\n")
		b.WriteString("Use search_developer_docs with multiple queries for broader semantic searches.\n")
	}

	b.WriteString("Use get_cached_document with multiple names to retrieve several documents from the local cache.\n")
	b.WriteString("Use search_cached_documents with multiple queries to search cached documents by keyword.\n")
	b.WriteString("Documents marked [cached] can be retrieved immediately with get_cached_document.\n")

	b.WriteString("\nAvailable Spanner reference documents (use full name with \"" + docPrefix + "\" prefix):\n")
	b.WriteString(formatDocCatalog(cache))

	return b.String()
}

// argsToStringSlice extracts a string slice from a function call argument.
func argsToStringSlice(args map[string]any, key string) []string {
	raw, _ := args[key].([]any)
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// executeToolCall dispatches a function call to the appropriate handler.
func executeToolCall(ctx context.Context, fc *genai.FunctionCall, cache *docCache) map[string]any {
	switch fc.Name {
	case "get_cached_document":
		names := argsToStringSlice(fc.Args, "names")
		documents := make(map[string]any, len(names))
		for _, name := range names {
			content, ok := cache.Get(ctx, name)
			if ok {
				documents[name] = content
			} else {
				documents[name] = map[string]any{"error": "not found in cache"}
			}
		}
		return map[string]any{"documents": documents, "count": len(names)}

	case "search_cached_documents":
		queries := argsToStringSlice(fc.Args, "queries")
		queryResults := make(map[string]any, len(queries))
		for _, query := range queries {
			results := cache.Search(query)
			items := make([]map[string]any, len(results))
			for i, r := range results {
				items[i] = map[string]any{"name": r.Name, "snippet": r.Snippet}
			}
			queryResults[query] = map[string]any{"results": items, "count": len(items)}
		}
		return map[string]any{"query_results": queryResults}

	case "search_developer_docs":
		queries := argsToStringSlice(fc.Args, "queries")
		queryResults := make(map[string]any, len(queries))
		for _, query := range queries {
			results, ok := cache.APISearch(ctx, query)
			if !ok {
				// Fallback to local search if API is unavailable
				localResults := cache.Search(query)
				items := make([]map[string]any, len(localResults))
				for i, r := range localResults {
					items[i] = map[string]any{"name": r.Name, "snippet": r.Snippet}
				}
				queryResults[query] = map[string]any{"results": items, "count": len(items), "source": "local_cache"}
				continue
			}
			items := make([]map[string]any, len(results))
			for i, r := range results {
				items[i] = map[string]any{"name": r.Name, "snippet": r.Snippet}
			}
			queryResults[query] = map[string]any{"results": items, "count": len(items)}
		}
		return map[string]any{"query_results": queryResults}

	case "get_developer_document":
		names := argsToStringSlice(fc.Args, "names")
		if len(names) == 1 {
			content, ok := cache.Get(ctx, names[0])
			if !ok {
				return map[string]any{"error": "failed to fetch document: " + names[0]}
			}
			return map[string]any{"documents": map[string]string{names[0]: content}, "count": 1}
		}
		docs := cache.BatchGet(ctx, names)
		docResults := make(map[string]string, len(docs))
		for _, doc := range docs {
			docResults[doc.Name] = doc.Content
		}
		return map[string]any{"documents": docResults, "count": len(docs)}

	default:
		return map[string]any{"error": fmt.Sprintf("unknown function: %s", fc.Name)}
	}
}

// --- Developer Knowledge REST API client ---

const defaultDevKnowledgeBaseURL = "https://developerknowledge.googleapis.com/v1alpha"

// devKnowledgeAPIError represents a non-OK HTTP response from the API.
type devKnowledgeAPIError struct {
	Code    int
	Status  string
	Message string
}

func (e *devKnowledgeAPIError) Error() string {
	if e.Status != "" {
		return fmt.Sprintf("API error %d (%s): %s", e.Code, e.Status, e.Message)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Code, e.Message)
}

// devKnowledgeClient is a REST client for the Developer Knowledge API.
type devKnowledgeClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	limiter *rate.Limiter
}

func newDevKnowledgeClient(apiKey string) *devKnowledgeClient {
	return &devKnowledgeClient{
		baseURL: defaultDevKnowledgeBaseURL,
		apiKey:  apiKey,
		client:  http.DefaultClient,
		// 100 RPM = ~1.67 RPS, burst of 5 for short request bursts
		limiter: rate.NewLimiter(rate.Every(600*time.Millisecond), 5),
	}
}

func (c *devKnowledgeClient) doGet(ctx context.Context, reqURL string) ([]byte, error) {
	const maxRetries = 3
	backoff := 1 * time.Second

	for attempt := range maxRetries {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", c.apiKey)

		// Network errors are not retried: doc fetching is best-effort
		// (stale cache + embedded docs as fallback).
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries-1 {
			wait := backoff
			if v := resp.Header.Get("Retry-After"); v != "" {
				if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			slog.Debug("Rate limited, retrying", "attempt", attempt, "wait", wait)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			backoff *= 2
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var apiErr struct {
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
					Status  string `json:"status"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
				return nil, &devKnowledgeAPIError{Code: apiErr.Error.Code, Status: apiErr.Error.Status, Message: apiErr.Error.Message}
			}
			return nil, &devKnowledgeAPIError{Code: resp.StatusCode, Message: string(body)}
		}

		return body, nil
	}
	return nil, fmt.Errorf("unreachable: exceeded max retries") // required by compiler
}

// devKnowledgeDocSearcher implements document operations using the Developer Knowledge REST API.
type devKnowledgeDocSearcher struct {
	client *devKnowledgeClient
}

func (d *devKnowledgeDocSearcher) Search(ctx context.Context, query string) ([]DocSearchResult, error) {
	params := url.Values{}
	params.Set("query", query)

	body, err := d.client.doGet(ctx, d.client.baseURL+"/documents:searchDocumentChunks?"+params.Encode())
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	var resp struct {
		Results []struct {
			Parent  string `json:"parent"`
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	results := make([]DocSearchResult, len(resp.Results))
	for i, r := range resp.Results {
		snippet := r.Content
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		results[i] = DocSearchResult{Name: r.Parent, Snippet: snippet}
	}
	return results, nil
}

func (d *devKnowledgeDocSearcher) GetDocument(ctx context.Context, name string) (string, error) {
	name = normalizeDocName(name)
	body, err := d.client.doGet(ctx, d.client.baseURL+"/"+name)
	if err != nil {
		return "", fmt.Errorf("get_document failed: %w", err)
	}

	var doc struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", err
	}
	return doc.Content, nil
}

func (d *devKnowledgeDocSearcher) BatchGetDocuments(ctx context.Context, names []string) ([]DocResult, error) {
	// Copy and normalize names to avoid mutating the caller's slice.
	normalized := make([]string, len(names))
	for i, name := range names {
		normalized[i] = normalizeDocName(name)
	}

	docs, err := d.fetchBatchGet(ctx, normalized)
	if err == nil {
		return docs, nil
	}

	var apiErr *devKnowledgeAPIError
	if errors.As(err, &apiErr) && apiErr.Code >= 400 && apiErr.Code < 500 {
		slog.Debug("batch_get failed, falling back to individual gets", "error", err)
		return d.fetchIndividual(ctx, normalized)
	}

	return nil, fmt.Errorf("batch_get_documents failed: %w", err)
}

func (d *devKnowledgeDocSearcher) fetchBatchGet(ctx context.Context, names []string) ([]DocResult, error) {
	params := url.Values{}
	for _, name := range names {
		params.Add("names", name)
	}

	body, err := d.client.doGet(ctx, d.client.baseURL+"/documents:batchGet?"+params.Encode())
	if err != nil {
		return nil, err
	}

	var resp struct {
		Documents []struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	results := make([]DocResult, len(resp.Documents))
	for i, doc := range resp.Documents {
		results[i] = DocResult{Name: doc.Name, Content: doc.Content}
	}
	return results, nil
}

func (d *devKnowledgeDocSearcher) fetchIndividual(ctx context.Context, names []string) ([]DocResult, error) {
	var results []DocResult
	for _, name := range names {
		content, err := d.GetDocument(ctx, name)
		if err != nil {
			slog.Debug("get_document failed in fallback", "name", name, "error", err)
			continue
		}
		results = append(results, DocResult{Name: name, Content: content})
	}
	return results, nil
}

func normalizeDocName(name string) string {
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	switch {
	case strings.HasPrefix(name, "documents/"):
		// Already fully qualified
	case strings.HasPrefix(name, "docs.cloud.google.com/"):
		name = "documents/" + name
	default:
		// Short form like "reference/standard-sql/query-syntax" — prepend full prefix
		name = docPrefix + name
	}
	return name
}
