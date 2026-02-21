//
// Copyright 2025 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package mycli

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	databasepb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/storage"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/goccy/go-yaml"
)

var (
	// Embed the samples directory at compile time
	// This includes all metadata files (*.yaml, *.json) and SQL files (*.sql)
	//go:embed samples/*
	embeddedSamples embed.FS

	// metadataFileRegex matches metadata files (.json, .yaml, .yml)
	// Used to identify which files contain sample database definitions
	metadataFileRegex = regexp.MustCompile(`\.(json|ya?ml)$`)
)

// SampleDatabase represents both metadata and runtime configuration
type SampleDatabase struct {
	Name        string `json:"name"`              // Unique identifier for the sample
	Description string `json:"description"`       // Human-readable description
	Dialect     string `json:"dialect"`           // SQL dialect (GOOGLE_STANDARD_SQL or POSTGRESQL)
	SchemaURI   string `json:"schemaURI"`         // URI to schema file (can be relative or absolute)
	DataURI     string `json:"dataURI,omitempty"` // URI to data file (optional)
	Source      string `json:"source,omitempty"`  // Documentation URL or description (optional)

	// Runtime fields (not in JSON)
	BaseDir       string                     `json:"-"` // Base directory for relative path resolution
	IsEmbedded    bool                       `json:"-"` // Distinguishes embedded:// from file:// base URIs
	ParsedDialect databasepb.DatabaseDialect `json:"-"` // Parsed dialect enum value
}

// ResolveURIs converts relative paths to absolute URIs based on BaseDir
func (s *SampleDatabase) ResolveURIs() {
	s.SchemaURI = s.resolveURI(s.SchemaURI)
	s.DataURI = s.resolveURI(s.DataURI)
}

// resolveURI converts a relative path to an absolute URI
func (s *SampleDatabase) resolveURI(uri string) string {
	if uri == "" {
		return ""
	}

	// Already an absolute URI (has scheme)
	if strings.Contains(uri, "://") {
		return uri
	}

	// Relative path - resolve based on whether it's embedded or file-based
	if s.IsEmbedded {
		// For embedded files, construct embedded:// URI
		// BaseDir is like "samples/fingraph" or "samples"
		relPath := filepath.ToSlash(filepath.Join(s.BaseDir, uri))
		// Remove the "samples/" prefix for the embedded:// URI
		return fmt.Sprintf("embedded://%s", strings.TrimPrefix(relPath, "samples/"))
	} else {
		// For user files, construct file:// URI
		absPath := filepath.Join(s.BaseDir, uri)
		return fmt.Sprintf("file://%s", absPath)
	}
}

// parseDialect converts the string dialect to the protobuf enum
func (s *SampleDatabase) parseDialect() error {
	switch s.Dialect {
	case "GOOGLE_STANDARD_SQL":
		s.ParsedDialect = databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL
	case "POSTGRESQL":
		s.ParsedDialect = databasepb.DatabaseDialect_POSTGRESQL
	default:
		return fmt.Errorf("unknown dialect: %s", s.Dialect)
	}
	return nil
}

// unmarshalMetadata unmarshal JSON or YAML data into SampleDatabase
func unmarshalMetadata(data []byte) (*SampleDatabase, error) {
	var sample SampleDatabase

	// goccy/go-yaml can parse both JSON and YAML formats transparently
	// This allows metadata files to be in either format without separate parsing logic
	if err := yaml.Unmarshal(data, &sample); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &sample, nil
}

// discoverBuiltinSamples discovers all built-in samples from embed.FS
func discoverBuiltinSamples() map[string]SampleDatabase {
	samples := make(map[string]SampleDatabase)

	// Read all JSON/YAML files in samples directory
	entries, err := embeddedSamples.ReadDir("samples")
	if err != nil {
		return samples
	}

	for _, entry := range entries {
		// Skip directories and non-metadata files
		if entry.IsDir() || entry.Name() == "README.md" {
			continue
		}

		// Accept .json, .yaml, .yml files
		name := entry.Name()
		if !metadataFileRegex.MatchString(name) {
			// Skip SQL files and other non-metadata files
			continue
		}

		path := filepath.Join("samples", name)
		data, err := embeddedSamples.ReadFile(path)
		if err != nil {
			slog.Error("failed to read embedded sample file", "path", path, "error", err)
			continue
		}

		sample, err := unmarshalMetadata(data)
		if err != nil {
			slog.Error("failed to unmarshal embedded sample metadata", "path", path, "error", err)
			continue
		}

		// Set runtime fields
		sample.BaseDir = "samples" // All files are in the same directory
		sample.IsEmbedded = true   // All built-in samples are from embedded FS
		if err := sample.parseDialect(); err != nil {
			slog.Error("failed to parse dialect for embedded sample", "path", path, "name", sample.Name, "error", err)
			continue
		}

		// Resolve relative URIs to absolute URIs (safe to call - only modifies relative paths)
		sample.ResolveURIs()

		samples[sample.Name] = *sample
	}

	return samples
}

// loadSampleFromMetadata loads sample from metadata file path (JSON or YAML)
func loadSampleFromMetadata(metadataPath string) (*SampleDatabase, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	sample, err := unmarshalMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata format: %w", err)
	}

	// Validate required fields
	if sample.Name == "" {
		return nil, fmt.Errorf("sample name is required")
	}
	if sample.SchemaURI == "" {
		return nil, fmt.Errorf("schemaURI is required")
	}

	// Set runtime fields for relative path resolution
	sample.BaseDir = filepath.Dir(metadataPath)
	sample.IsEmbedded = false
	if err := sample.parseDialect(); err != nil {
		return nil, err
	}
	sample.ResolveURIs()

	return sample, nil
}

// loadFromURI loads content from various URI schemes (for single URI compatibility)
func loadFromURI(ctx context.Context, uri string) ([]byte, error) {
	// Handle embedded:// URIs directly here to avoid unnecessary parallelism setup
	if strings.HasPrefix(uri, "embedded://") {
		path := strings.TrimPrefix(uri, "embedded://")
		return embeddedSamples.ReadFile(filepath.Join("samples", path))
	}

	// For other URI schemes, delegate to the parallel loader
	// This ensures consistent error handling and GCS client management
	results, err := loadMultipleFromURIs(ctx, []string{uri})
	if err != nil {
		return nil, err
	}
	return results[uri], nil
}

// loadMultipleFromURIs efficiently loads multiple URIs in parallel, returning a map keyed by URI
func loadMultipleFromURIs(ctx context.Context, uris []string) (map[string][]byte, error) {
	if len(uris) == 0 {
		return nil, nil
	}

	results := make(map[string][]byte)
	errors := make(map[string]error)

	var wg sync.WaitGroup
	var mu sync.Mutex // Protect concurrent map writes

	// Pre-check if we need a GCS client to avoid creating it unnecessarily
	// Create a single GCS client that will be shared across all goroutines for efficiency
	var gcsClient *storage.Client
	var gcsClientErr error
	for _, uri := range uris {
		if strings.HasPrefix(uri, "gs://") {
			// Create GCS client once, lazily
			gcsClient, gcsClientErr = storage.NewClient(ctx)
			if gcsClientErr != nil {
				return nil, fmt.Errorf("failed to create GCS client: %w", gcsClientErr)
			}
			defer func() { _ = gcsClient.Close() }()
			break
		}
	}

	for _, uri := range uris {
		if uri == "" {
			continue
		}

		// Process all URIs uniformly in parallel
		// Note: wg.Go requires Go 1.25+ (combines Add(1) and go func with automatic Done())
		wg.Go(func() {
			var data []byte
			var err error

			switch {
			case strings.HasPrefix(uri, "embedded://"):
				path := strings.TrimPrefix(uri, "embedded://")
				data, err = embeddedSamples.ReadFile(filepath.Join("samples", path))
			case strings.HasPrefix(uri, "gs://"):
				data, err = loadFromGCSWithClient(ctx, gcsClient, uri)
			case strings.HasPrefix(uri, "file://"):
				path := strings.TrimPrefix(uri, "file://")
				// Use common file safety checks with sample database size limit
				data, err = SafeReadFile(path, &FileSafetyOptions{
					MaxSize: SampleDatabaseMaxFileSize,
				})
			case strings.HasPrefix(uri, "https://") || strings.HasPrefix(uri, "http://"):
				data, err = loadFromHTTP(ctx, uri)
			default:
				err = fmt.Errorf("unsupported URI scheme: %s", uri)
			}

			mu.Lock()
			if err != nil {
				errors[uri] = err
			} else {
				results[uri] = data
			}
			mu.Unlock()
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Check for any errors and report all of them
	if len(errors) > 0 {
		var errStrings []string
		// Sort URIs for deterministic error output
		errorURIs := make([]string, 0, len(errors))
		for uri := range errors {
			errorURIs = append(errorURIs, uri)
		}
		slices.Sort(errorURIs)

		for _, uri := range errorURIs {
			errStrings = append(errStrings, fmt.Sprintf("failed to load %s: %v", uri, errors[uri]))
		}
		return nil, fmt.Errorf("multiple errors occurred:\n- %s", strings.Join(errStrings, "\n- "))
	}

	return results, nil
}

// loadFromGCSWithClient loads a single GCS object using an existing client
func loadFromGCSWithClient(ctx context.Context, client *storage.Client, uri string) ([]byte, error) {
	// Parse gs://bucket/path/to/object
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid GCS URI %q: %w", uri, err)
	}

	bucket := u.Host
	object := strings.TrimPrefix(u.Path, "/")

	// Check object size before reading
	attrs, err := client.Bucket(bucket).Object(object).Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCS object attributes for %s: %w", uri, err)
	}
	if attrs.Size > SampleDatabaseMaxFileSize {
		return nil, fmt.Errorf("GCS object %s too large: %d bytes (max %d)", uri, attrs.Size, SampleDatabaseMaxFileSize)
	}

	reader, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open GCS object %s: %w", uri, err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read GCS object %s: %w", uri, err)
	}

	return data, nil
}

// loadFromHTTP loads content from HTTP/HTTPS URLs
func loadFromHTTP(ctx context.Context, uri string) ([]byte, error) {
	// Add timeout to prevent hanging indefinitely
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", uri, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, uri)
	}

	// Check Content-Length header if present
	if resp.ContentLength > 0 && resp.ContentLength > SampleDatabaseMaxFileSize {
		return nil, fmt.Errorf("HTTP response from %s too large: %d bytes (max %d)", uri, resp.ContentLength, SampleDatabaseMaxFileSize)
	}

	// Use io.LimitReader as a safety measure even if Content-Length is not set
	limitedReader := io.LimitReader(resp.Body, SampleDatabaseMaxFileSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response from %s: %w", uri, err)
	}

	// Check if we hit the limit
	if len(data) > SampleDatabaseMaxFileSize {
		return nil, fmt.Errorf("HTTP response from %s too large: exceeded %d bytes", uri, SampleDatabaseMaxFileSize)
	}

	return data, nil
}

// dialectToString converts a database dialect enum to a display string
func dialectToString(dialect databasepb.DatabaseDialect) string {
	if dialect == databasepb.DatabaseDialect_POSTGRESQL {
		return "PostgreSQL"
	}
	return "GoogleSQL"
}

// ListAvailableSamples returns a formatted list of available sample databases
func ListAvailableSamples() string {
	samples := discoverBuiltinSamples()

	var sb strings.Builder
	sb.WriteString("Available sample databases:\n\n")

	// Get sorted list of sample names
	names := slices.Collect(maps.Keys(samples))
	slices.Sort(names)

	// Calculate max width for formatting
	maxNameLen := 0
	for _, name := range names {
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	// Print samples in sorted order
	for _, name := range names {
		sample := samples[name]
		fmt.Fprintf(&sb, "  %-*s  %-11s  %s\n", maxNameLen, name, dialectToString(sample.ParsedDialect), sample.Description)
	}

	sb.WriteString("\nUsage: spanner-mycli --embedded-emulator --sample-database=<name>\n")
	sb.WriteString("       spanner-mycli --embedded-emulator --sample-database=/path/to/metadata.json\n")
	return sb.String()
}

// ParseStatements parses DDL or DML statements from SQL/SDL content using memefish
func ParseStatements(content []byte, filename string) ([]string, error) {
	// Use memefish's SplitRawStatements to properly split statements
	rawStmts, err := memefish.SplitRawStatements(filename, string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to split statements: %w", err)
	}

	var statements []string
	for _, rawStmt := range rawStmts {
		stmt := strings.TrimSpace(rawStmt.Statement)
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements, nil
}
