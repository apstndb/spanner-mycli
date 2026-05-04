package mycli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	spannerdocs "github.com/apstndb/spanner-docs-embed"
	"github.com/samber/lo"
)

// docInfo describes a known Spanner reference document.
type docInfo struct {
	// Name is the Developer Knowledge API document name
	// (e.g., "documents/docs.cloud.google.com/spanner/docs/reference/standard-sql/graph-patterns").
	Name string
	// Description is a short summary shown to the LLM.
	Description string
}

// docEmbedBase returns the filename stem used for embedded .zst files.
// It extracts the last path segment from the document name
// (e.g., "documents/.../graph-patterns" → "graph-patterns").
func docEmbedBase(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// stdSQLPrefix is the path prefix for reference/standard-sql documents within the doc name.
const stdSQLPrefix = "reference/standard-sql/"

// docCategory derives the display category from a document name.
// For reference/standard-sql/ documents:
//   - "graph-*" → "GQL"
//   - names containing "function" → "GoogleSQL Functions"
//   - everything else → "GoogleSQL"
//
// For other paths, falls back to "Other".
func docCategory(name string) string {
	relPath := strings.TrimPrefix(name, docPrefix)
	if base, ok := strings.CutPrefix(relPath, stdSQLPrefix); ok {
		switch {
		case strings.HasPrefix(base, "graph-"):
			return "GQL"
		case strings.Contains(base, "function"):
			return "GoogleSQL Functions"
		default:
			return "GoogleSQL"
		}
	}
	return "Other"
}

// docCatalog is the single source of truth for all known Spanner reference documents.
// Each entry's embed base name is derived from Name via docEmbedBase().
var docCatalog = []docInfo{
	// GQL (Graph Query Language)
	{Name: docPrefix + "reference/standard-sql/graph-query-statements", Description: "GQL statements: MATCH, RETURN, FILTER, LET, WITH, NEXT, set operations"},
	{Name: docPrefix + "reference/standard-sql/graph-patterns", Description: "GQL patterns: node/edge, subpaths, quantified paths, label expressions, path modes"},
	{Name: docPrefix + "reference/standard-sql/graph-gql-functions", Description: "GQL functions: LABELS, NODES, EDGES, PATH_LENGTH, etc."},
	{Name: docPrefix + "reference/standard-sql/graph-operators", Description: "GQL operators: PROPERTY_EXISTS, IS LABELED, IS SOURCE/DESTINATION, SAME"},
	{Name: docPrefix + "reference/standard-sql/graph-sql-queries", Description: "GRAPH_TABLE operator for using GQL within SQL"},
	{Name: docPrefix + "reference/standard-sql/graph-subqueries", Description: "GQL subqueries: EXISTS, IN, ARRAY, VALUE"},
	{Name: docPrefix + "reference/standard-sql/graph-schema-statements", Description: "CREATE/ALTER/DROP PROPERTY GRAPH"},
	{Name: docPrefix + "reference/standard-sql/graph-sql-functions", Description: "Graph functions usable in SQL: GRAPH_ANCESTORS, GRAPH_DESCENDANTS, etc."},
	{Name: docPrefix + "reference/standard-sql/graph-data-types", Description: "Graph element types: GRAPH_NODE, GRAPH_EDGE, GRAPH_PATH"},
	{Name: docPrefix + "reference/standard-sql/graph-intro", Description: "Introduction to Spanner Graph"},
	{Name: docPrefix + "reference/standard-sql/graph-conditional-expressions", Description: "Graph conditional expressions"},

	// GoogleSQL core
	{Name: docPrefix + "reference/standard-sql/overview", Description: "GoogleSQL overview and feature summary"},
	{Name: docPrefix + "reference/standard-sql/query-syntax", Description: "SELECT, JOIN, WHERE, GROUP BY, window functions, etc."},
	{Name: docPrefix + "reference/standard-sql/data-definition-language", Description: "CREATE/ALTER/DROP TABLE, INDEX, VIEW, etc."},
	{Name: docPrefix + "reference/standard-sql/dml-syntax", Description: "INSERT, UPDATE, DELETE, MERGE"},
	{Name: docPrefix + "reference/standard-sql/data-types", Description: "Data types reference"},
	{Name: docPrefix + "reference/standard-sql/lexical", Description: "Lexical structure, identifiers, literals, operators"},
	{Name: docPrefix + "reference/standard-sql/operators", Description: "Operators: arithmetic, comparison, logical, IN, BETWEEN, LIKE, IS, etc."},
	{Name: docPrefix + "reference/standard-sql/subqueries", Description: "Subqueries: scalar, array, IN, EXISTS, correlated"},
	{Name: docPrefix + "reference/standard-sql/arrays", Description: "Working with arrays: constructing, accessing, flattening, querying"},
	{Name: docPrefix + "reference/standard-sql/conditional_expressions", Description: "CASE, IF, IFNULL, NULLIF, COALESCE, IIF"},
	{Name: docPrefix + "reference/standard-sql/format-elements", Description: "Format elements for DATE, TIME, TIMESTAMP, NUMERIC formatting"},
	{Name: docPrefix + "reference/standard-sql/conversion_rules", Description: "Implicit/explicit casting and type coercion rules"},
	{Name: docPrefix + "reference/standard-sql/collation-concepts", Description: "Collation: Unicode sorting, case sensitivity, comparison rules"},
	{Name: docPrefix + "reference/standard-sql/protocol-buffers", Description: "Protocol buffer support in GoogleSQL"},
	{Name: docPrefix + "reference/standard-sql/procedural-language", Description: "Procedural language: DECLARE, SET, IF, WHILE, BEGIN...END"},
	{Name: docPrefix + "reference/standard-sql/stored-procedures", Description: "Stored procedures: CREATE/ALTER/DROP PROCEDURE"},

	// GoogleSQL functions — prefer individual pages over functions-all for detailed lookup
	{Name: docPrefix + "reference/standard-sql/functions-all", Description: "All built-in functions overview/index (prefer individual *_functions pages for details)"},
	{Name: docPrefix + "reference/standard-sql/functions-reference", Description: "Function reference: categorized index of all functions"},
	{Name: docPrefix + "reference/standard-sql/aggregate_functions", Description: "Aggregate functions: COUNT, SUM, AVG, MIN, MAX, ARRAY_AGG, STRING_AGG, etc."},
	{Name: docPrefix + "reference/standard-sql/aggregate-function-calls", Description: "Aggregate function call syntax: DISTINCT, HAVING, ORDER BY, LIMIT in aggregates"},
	{Name: docPrefix + "reference/standard-sql/statistical_aggregate_functions", Description: "Statistical aggregates: CORR, COVAR, STDDEV, VARIANCE, etc."},
	{Name: docPrefix + "reference/standard-sql/array_functions", Description: "Array functions: ARRAY_CONCAT, ARRAY_LENGTH, ARRAY_TO_STRING, GENERATE_ARRAY, etc."},
	{Name: docPrefix + "reference/standard-sql/bit_functions", Description: "Bit functions: BIT_COUNT, BIT_AND, BIT_OR, BIT_XOR"},
	{Name: docPrefix + "reference/standard-sql/compression-functions", Description: "Compression functions: GZIP_COMPRESS, GZIP_DECOMPRESS"},
	{Name: docPrefix + "reference/standard-sql/conversion_functions", Description: "Conversion functions: CAST, SAFE_CAST, FORMAT, PARSE_*"},
	{Name: docPrefix + "reference/standard-sql/date_functions", Description: "Date functions: DATE, DATE_ADD, DATE_DIFF, DATE_TRUNC, FORMAT_DATE, etc."},
	{Name: docPrefix + "reference/standard-sql/debugging_functions", Description: "Debugging functions: ERROR"},
	{Name: docPrefix + "reference/standard-sql/hash_functions", Description: "Hash functions: FARM_FINGERPRINT, MD5, SHA1, SHA256, SHA512"},
	{Name: docPrefix + "reference/standard-sql/interval_functions", Description: "Interval functions: MAKE_INTERVAL, EXTRACT, JUSTIFY_*"},
	{Name: docPrefix + "reference/standard-sql/json_functions", Description: "JSON functions: JSON_VALUE, JSON_QUERY, JSON_ARRAY, TO_JSON, etc."},
	{Name: docPrefix + "reference/standard-sql/mathematical_functions", Description: "Math functions: ABS, CEIL, FLOOR, ROUND, MOD, POW, SQRT, LOG, etc."},
	{Name: docPrefix + "reference/standard-sql/ml-functions", Description: "ML functions: ML.PREDICT, ML.FORECAST, ML.DETECT_ANOMALIES"},
	{Name: docPrefix + "reference/standard-sql/net_functions", Description: "Net functions: NET.IP_FROM_STRING, NET.SAFE_IP_FROM_STRING, etc."},
	{Name: docPrefix + "reference/standard-sql/protocol_buffer_functions", Description: "Protocol buffer functions: PROTO_DEFAULT_IF_NULL, FROM_PROTO, TO_PROTO"},
	{Name: docPrefix + "reference/standard-sql/search_functions", Description: "Full-text search functions: SEARCH, SEARCH_NGRAMS, SNIPPET, SCORE, TOKENIZE_*"},
	{Name: docPrefix + "reference/standard-sql/sequence_functions", Description: "Sequence functions: GET_NEXT_SEQUENCE_VALUE, GET_INTERNAL_SEQUENCE_STATE"},
	{Name: docPrefix + "reference/standard-sql/string_functions", Description: "String functions: CONCAT, LENGTH, SUBSTR, REPLACE, REGEXP_*, TRIM, etc."},
	{Name: docPrefix + "reference/standard-sql/timestamp_functions", Description: "Timestamp functions: CURRENT_TIMESTAMP, TIMESTAMP_ADD, FORMAT_TIMESTAMP, etc."},
	{Name: docPrefix + "reference/standard-sql/utility-functions", Description: "Utility functions: GENERATE_UUID, PENDING_COMMIT_TIMESTAMP"},
}

// loadEmbeddedDocs loads all pre-compressed embedded docs into the cache.
// It iterates docCatalog directly and reads the corresponding .zst file from spanner-docs-embed.
func loadEmbeddedDocs(cache *docCache) error {
	for _, doc := range docCatalog {
		base := docEmbedBase(doc.Name)
		data, err := spannerdocs.ReadCompressed(base)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", base, err)
		}
		cache.LoadCompressed(doc.Name, data)
	}
	return nil
}

// formatDocCatalog formats the document catalog for the LLM system prompt.
// It groups documents by category, marks cached ones, and includes descriptions.
func formatDocCatalog(cache *docCache) string {
	cachedNames := lo.Associate(cache.Names(), func(name string) (string, bool) {
		return name, true
	})

	var b strings.Builder

	// Group by category, preserving order
	var currentCategory string
	for _, doc := range docCatalog {
		cat := docCategory(doc.Name)
		if cat != currentCategory {
			currentCategory = cat
			b.WriteString("\n" + currentCategory + ":\n")
		}

		// Show short path (strip common prefix)
		shortName := strings.TrimPrefix(doc.Name, docPrefix)

		cached := ""
		if cachedNames[doc.Name] {
			cached = " [cached]"
			delete(cachedNames, doc.Name) // track extras below
		}

		fmt.Fprintf(&b, "- %s (%s)%s\n", shortName, doc.Description, cached)
	}

	// List any extra cached documents not in the catalog (fetched during session)
	if len(cachedNames) > 0 {
		b.WriteString("\nAdditional cached documents:\n")
		for _, name := range slices.Sorted(maps.Keys(cachedNames)) {
			shortName := strings.TrimPrefix(name, docPrefix)
			b.WriteString("- " + shortName + " [cached]\n")
		}
	}

	return b.String()
}
