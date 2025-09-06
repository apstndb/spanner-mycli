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

// loadFromURI loads content from various URI schemes
func loadFromURI(ctx context.Context, uri string) ([]byte, error) {
	switch {
	case strings.HasPrefix(uri, "gs://"):
		// Google Cloud Storage
		return loadFromGCS(ctx, uri)
	case strings.HasPrefix(uri, "file://"):
		// Local file system
		path := strings.TrimPrefix(uri, "file://")
		return os.ReadFile(path)
	case strings.HasPrefix(uri, "https://") || strings.HasPrefix(uri, "http://"):
		// HTTP(S) download
		return loadFromHTTP(ctx, uri)
	default:
		return nil, fmt.Errorf("unsupported URI scheme: %s", uri)
	}
}

// loadFromGCS loads content from Google Cloud Storage
func loadFromGCS(ctx context.Context, uri string) ([]byte, error) {
	// Parse gs://bucket/path/to/object
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid GCS URI: %w", err)
	}

	bucket := u.Host
	object := strings.TrimPrefix(u.Path, "/")

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer func() { _ = client.Close() }()

	reader, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open GCS object %s: %w", uri, err)
	}
	defer func() { _ = reader.Close() }()

	return io.ReadAll(reader)
}

// loadFromHTTP loads content from HTTP/HTTPS URLs
func loadFromHTTP(ctx context.Context, uri string) ([]byte, error) {
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
