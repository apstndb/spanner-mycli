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

package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	databasepb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/storage"
	"github.com/cloudspannerecosystem/memefish"
)

const (
	// gcsSamplesBase is the base path for Google Cloud Spanner sample databases
	gcsSamplesBase = "gs://cloud-spanner-samples/"
)

// SampleDatabase represents a sample database configuration
type SampleDatabase struct {
	Dialect     databasepb.DatabaseDialect // SQL dialect
	SchemaURI   string                     // URI to schema file (gs://, file://, https://)
	DataURI     string                     // URI to data file (optional)
	Description string                     // Human-readable description
}

// sampleDatabases is the registry of available sample databases
var sampleDatabases = map[string]SampleDatabase{
	"banking": {
		Dialect:     databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		SchemaURI:   gcsSamplesBase + "banking/schema/banking-schema.sdl",
		DataURI:     gcsSamplesBase + "banking/data-insert-statements/banking-inserts.sql",
		Description: "Banking application with accounts and transactions",
	},
	"finance": {
		Dialect:     databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		SchemaURI:   gcsSamplesBase + "finance/schema/finance-schema.sdl",
		DataURI:     "",
		Description: "Finance application schema (GoogleSQL)",
	},
	"finance-graph": {
		Dialect:     databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		SchemaURI:   gcsSamplesBase + "finance-graph/schema/finance-graph-schema.sdl",
		DataURI:     gcsSamplesBase + "finance-graph/data-insert-statements/finance-graph-inserts.sql",
		Description: "Finance application with graph features",
	},
	"finance-pg": {
		Dialect:     databasepb.DatabaseDialect_POSTGRESQL,
		SchemaURI:   gcsSamplesBase + "finance/schema/finance-schema-pg.sdl",
		DataURI:     "",
		Description: "Finance application (PostgreSQL dialect)",
	},
	"gaming": {
		Dialect:     databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		SchemaURI:   gcsSamplesBase + "gaming/schema/gaming-schema.sdl",
		DataURI:     gcsSamplesBase + "gaming/data-insert-statements/gaming-inserts.sql",
		Description: "Gaming application with players and scores",
	},
}

// loadFromURI loads content from various URI schemes (for single URI compatibility)
func loadFromURI(ctx context.Context, uri string) ([]byte, error) {
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

	// Check if we need a GCS client
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
		wg.Go(func() {
			var data []byte
			var err error

			switch {
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
	var sb strings.Builder
	sb.WriteString("Available sample databases:\n\n")

	// Get sorted list of sample names
	names := slices.Collect(maps.Keys(sampleDatabases))
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
		sample := sampleDatabases[name]
		fmt.Fprintf(&sb, "  %-*s  %-11s  %s\n", maxNameLen, name, dialectToString(sample.Dialect), sample.Description)
	}

	sb.WriteString("\nUsage: spanner-mycli --embedded-emulator --sample-database=<name>\n")
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
