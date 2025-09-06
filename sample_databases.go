//
// Copyright 2020 Google LLC
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
	"os"
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
	return results[0], nil
}

// loadMultipleFromURIs efficiently loads multiple URIs in parallel, using batch loading for GCS
func loadMultipleFromURIs(ctx context.Context, uris []string) ([][]byte, error) {
	if len(uris) == 0 {
		return nil, nil
	}

	results := make([][]byte, len(uris))
	errors := make([]error, len(uris))

	// Group URIs by type for optimal processing
	type loadTask struct {
		uri   string
		index int
	}

	var gcsJobs []loadTask
	var httpJobs []loadTask
	var fileJobs []loadTask

	// Categorize URIs by type
	for i, uri := range uris {
		if uri == "" {
			continue
		}

		task := loadTask{uri: uri, index: i}
		switch {
		case strings.HasPrefix(uri, "gs://"):
			gcsJobs = append(gcsJobs, task)
		case strings.HasPrefix(uri, "file://"):
			fileJobs = append(fileJobs, task)
		case strings.HasPrefix(uri, "https://") || strings.HasPrefix(uri, "http://"):
			httpJobs = append(httpJobs, task)
		default:
			errors[i] = fmt.Errorf("unsupported URI scheme: %s", uri)
		}
	}

	var wg sync.WaitGroup

	// Process file:// URIs in parallel (local I/O is fast)
	for _, task := range fileJobs {
		wg.Go(func() {
			path := strings.TrimPrefix(task.uri, "file://")
			data, err := os.ReadFile(path)
			if err != nil {
				errors[task.index] = fmt.Errorf("failed to read file %s: %w", task.uri, err)
				return
			}
			results[task.index] = data
		})
	}

	// Process HTTP(S) URIs in parallel
	for _, task := range httpJobs {
		wg.Go(func() {
			data, err := loadFromHTTP(ctx, task.uri)
			if err != nil {
				errors[task.index] = err
				return
			}
			results[task.index] = data
		})
	}

	// Process GCS URIs in a single batch (single client for efficiency)
	if len(gcsJobs) > 0 {
		wg.Go(func() {
			// Extract just the URIs for batch loading
			gcsURIs := make([]string, len(gcsJobs))
			for i, job := range gcsJobs {
				gcsURIs[i] = job.uri
			}

			gcsResults, err := loadBatchFromGCS(ctx, gcsURIs)
			if err != nil {
				// Mark all GCS jobs as failed
				for _, job := range gcsJobs {
					errors[job.index] = err
				}
				return
			}

			// Map results back to their original positions
			for i, job := range gcsJobs {
				results[job.index] = gcsResults[i]
			}
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Check for any errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to load URI at index %d: %w", i, err)
		}
	}

	return results, nil
}

// loadBatchFromGCS efficiently loads multiple GCS objects using a single client
func loadBatchFromGCS(ctx context.Context, uris []string) ([][]byte, error) {
	if len(uris) == 0 {
		return nil, nil
	}

	// Create a single GCS client for all operations
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer func() { _ = client.Close() }()

	results := make([][]byte, len(uris))
	for i, uri := range uris {
		// Parse gs://bucket/path/to/object
		u, err := url.Parse(uri)
		if err != nil {
			return nil, fmt.Errorf("invalid GCS URI %q: %w", uri, err)
		}

		bucket := u.Host
		object := strings.TrimPrefix(u.Path, "/")

		reader, err := client.Bucket(bucket).Object(object).NewReader(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to open GCS object %s: %w", uri, err)
		}

		data, err := io.ReadAll(reader)
		reader.Close() // Close immediately after reading
		if err != nil {
			return nil, fmt.Errorf("failed to read GCS object %s: %w", uri, err)
		}

		results[i] = data
	}

	return results, nil
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

	return io.ReadAll(resp.Body)
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
