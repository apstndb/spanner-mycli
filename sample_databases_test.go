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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	databasepb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

func TestListAvailableSamples(t *testing.T) {
	output := ListAvailableSamples()

	// Check that GCS samples are listed
	expectedGCSSamples := []string{"banking", "finance", "finance-graph", "finance-pg", "gaming"}
	for _, sample := range expectedGCSSamples {
		if !strings.Contains(output, sample) {
			t.Errorf("ListAvailableSamples() missing GCS sample %q", sample)
		}
	}

	// Check that embedded samples are listed
	expectedEmbeddedSamples := []string{"fingraph", "singers"}
	for _, sample := range expectedEmbeddedSamples {
		if !strings.Contains(output, sample) {
			t.Errorf("ListAvailableSamples() missing embedded sample %q", sample)
		}
	}

	// Check for usage instruction
	if !strings.Contains(output, "metadata.json") {
		t.Error("ListAvailableSamples() missing metadata.json instruction")
	}
}

func TestLoadFromHTTP(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.sql" {
			_, _ = w.Write([]byte("CREATE TABLE test (id INT64);"))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()

	// Test successful fetch
	data, err := loadFromHTTP(ctx, server.URL+"/test.sql")
	if err != nil {
		t.Fatalf("loadFromHTTP() error = %v", err)
	}

	expected := "CREATE TABLE test (id INT64);"
	if string(data) != expected {
		t.Errorf("loadFromHTTP() = %q, want %q", string(data), expected)
	}

	// Test 404
	_, err = loadFromHTTP(ctx, server.URL+"/notfound.sql")
	if err == nil {
		t.Error("loadFromHTTP() expected error for 404, got nil")
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.sql")
	content := "INSERT INTO test VALUES (1);"
	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	ctx := context.Background()

	// Test successful read
	data, err := loadFromURI(ctx, "file://"+testFile)
	if err != nil {
		t.Fatalf("loadFromURI() error = %v", err)
	}

	if string(data) != content {
		t.Errorf("loadFromURI() = %q, want %q", string(data), content)
	}

	// Test non-existent file
	_, err = loadFromURI(ctx, "file:///nonexistent/file.sql")
	if err == nil {
		t.Error("loadFromURI() expected error for non-existent file, got nil")
	}
}

func TestSampleDatabaseURIResolution(t *testing.T) {
	// Test URI resolution for embedded sample
	embedded := SampleDatabase{
		Name:       "test-embedded",
		SchemaURI:  "test-schema.sql",
		DataURI:    "test-data.sql",
		BaseDir:    "samples",
		IsEmbedded: true,
	}
	embedded.ResolveURIs()
	if embedded.SchemaURI != "embedded://test-schema.sql" {
		t.Errorf("embedded SchemaURI = %q, want embedded://test-schema.sql", embedded.SchemaURI)
	}
	if embedded.DataURI != "embedded://test-data.sql" {
		t.Errorf("embedded DataURI = %q, want embedded://test-data.sql", embedded.DataURI)
	}

	// Test URI resolution for user sample (relative paths)
	user := SampleDatabase{
		Name:       "test-user",
		SchemaURI:  "schema.sql",
		DataURI:    "https://example.com/data.sql", // Already absolute
		BaseDir:    "/home/user/samples/test",
		IsEmbedded: false,
	}
	user.ResolveURIs()
	if user.SchemaURI != "file:///home/user/samples/test/schema.sql" {
		t.Errorf("user SchemaURI = %q, want file:///home/user/samples/test/schema.sql", user.SchemaURI)
	}
	if user.DataURI != "https://example.com/data.sql" {
		t.Errorf("user DataURI = %q, want https://example.com/data.sql (unchanged)", user.DataURI)
	}
}

func TestLoadSampleFromMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "metadata.json")

	// Test valid metadata
	validMetadata := `{
		"name": "test",
		"description": "Test sample",
		"dialect": "GOOGLE_STANDARD_SQL",
		"schemaURI": "schema.sql",
		"dataURI": "gs://bucket/data.sql"
	}`
	if err := os.WriteFile(metadataPath, []byte(validMetadata), 0o644); err != nil {
		t.Fatalf("Failed to write metadata: %v", err)
	}

	sample, err := loadSampleFromMetadata(metadataPath)
	if err != nil {
		t.Fatalf("loadSampleFromMetadata() error = %v", err)
	}

	if sample.Name != "test" {
		t.Errorf("sample.Name = %q, want %q", sample.Name, "test")
	}
	if sample.ParsedDialect != databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL {
		t.Errorf("sample.ParsedDialect = %v, want GOOGLE_STANDARD_SQL", sample.ParsedDialect)
	}
	expectedSchemaURI := fmt.Sprintf("file://%s/schema.sql", tmpDir)
	if sample.SchemaURI != expectedSchemaURI {
		t.Errorf("sample.SchemaURI = %q, want %q", sample.SchemaURI, expectedSchemaURI)
	}
	if sample.DataURI != "gs://bucket/data.sql" {
		t.Errorf("sample.DataURI = %q, want gs://bucket/data.sql", sample.DataURI)
	}

	// Test invalid dialect
	invalidDialectMetadata := `{
		"name": "test",
		"dialect": "INVALID_SQL",
		"schemaURI": "schema.sql"
	}`
	invalidPath := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte(invalidDialectMetadata), 0o644); err != nil {
		t.Fatalf("Failed to write invalid metadata: %v", err)
	}

	_, err = loadSampleFromMetadata(invalidPath)
	if err == nil {
		t.Error("loadSampleFromMetadata() expected error for invalid dialect, got nil")
	}

	// Test missing required fields
	missingFieldsMetadata := `{
		"name": "test",
		"dialect": "GOOGLE_STANDARD_SQL"
	}`
	missingPath := filepath.Join(tmpDir, "missing.json")
	if err := os.WriteFile(missingPath, []byte(missingFieldsMetadata), 0o644); err != nil {
		t.Fatalf("Failed to write metadata with missing fields: %v", err)
	}

	_, err = loadSampleFromMetadata(missingPath)
	if err == nil {
		t.Error("loadSampleFromMetadata() expected error for missing schemaURI, got nil")
	}
}

func TestDiscoverBuiltinSamples(t *testing.T) {
	samples := discoverBuiltinSamples()

	// Check that embedded samples are discovered
	if _, ok := samples["fingraph"]; !ok {
		t.Error("discoverBuiltinSamples() missing 'fingraph' sample")
	}
	if _, ok := samples["singers"]; !ok {
		t.Error("discoverBuiltinSamples() missing 'singers' sample")
	}

	// Verify fingraph sample properties
	if fingraph, ok := samples["fingraph"]; ok {
		if fingraph.ParsedDialect != databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL {
			t.Errorf("fingraph.ParsedDialect = %v, want GOOGLE_STANDARD_SQL", fingraph.ParsedDialect)
		}
		if !fingraph.IsEmbedded {
			t.Error("fingraph.IsEmbedded = false, want true")
		}
		if !strings.HasPrefix(fingraph.SchemaURI, "embedded://") {
			t.Errorf("fingraph.SchemaURI = %q, should start with embedded://", fingraph.SchemaURI)
		}
	}

	// Verify singers sample properties
	if singers, ok := samples["singers"]; ok {
		if singers.ParsedDialect != databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL {
			t.Errorf("singers.ParsedDialect = %v, want GOOGLE_STANDARD_SQL", singers.ParsedDialect)
		}
		if !singers.IsEmbedded {
			t.Error("singers.IsEmbedded = false, want true")
		}
		if !strings.HasPrefix(singers.SchemaURI, "embedded://") {
			t.Errorf("singers.SchemaURI = %q, should start with embedded://", singers.SchemaURI)
		}
	}
}

func TestLoadEmbeddedURI(t *testing.T) {
	ctx := context.Background()

	// Test loading embedded fingraph schema
	data, err := loadFromURI(ctx, "embedded://fingraph-schema.sql")
	if err != nil {
		t.Fatalf("loadFromURI(embedded://fingraph-schema.sql) error = %v", err)
	}

	// Check that it contains expected content
	if !strings.Contains(string(data), "CREATE TABLE Person") {
		t.Error("embedded fingraph schema missing 'CREATE TABLE Person'")
	}
	if !strings.Contains(string(data), "CREATE OR REPLACE PROPERTY GRAPH FinGraph") {
		t.Error("embedded fingraph schema missing graph definition")
	}

	// Test loading embedded singers data
	data, err = loadFromURI(ctx, "embedded://singers-data.sql")
	if err != nil {
		t.Fatalf("loadFromURI(embedded://singers-data.sql) error = %v", err)
	}

	// Check that it contains expected content
	if !strings.Contains(string(data), "INSERT INTO Singers") {
		t.Error("embedded singers data missing 'INSERT INTO Singers'")
	}
}

func TestLoadFromURI_UnsupportedScheme(t *testing.T) {
	ctx := context.Background()

	_, err := loadFromURI(ctx, "ftp://example.com/file.sql")
	if err == nil {
		t.Error("loadFromURI() expected error for unsupported scheme, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported URI scheme") {
		t.Errorf("loadFromURI() error = %v, want error containing 'unsupported URI scheme'", err)
	}
}

func TestParseStatements(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		filename string
		want     []string
	}{
		{
			name: "DDL statements",
			content: `CREATE TABLE users (id INT64) PRIMARY KEY (id);
CREATE INDEX idx_users ON users(id);`,
			filename: "schema.sql",
			want: []string{
				"CREATE TABLE users (id INT64) PRIMARY KEY (id)",
				"CREATE INDEX idx_users ON users(id)",
			},
		},
		{
			name: "DML statements",
			content: `INSERT INTO users VALUES (1);
INSERT INTO users VALUES (2);`,
			filename: "data.sql",
			want: []string{
				"INSERT INTO users VALUES (1)",
				"INSERT INTO users VALUES (2)",
			},
		},
		{
			name: "Statements with comments",
			content: `-- This is a comment
CREATE TABLE test (id INT64);
/* Multi-line
   comment */
INSERT INTO test VALUES (1);`,
			filename: "mixed.sql",
			// memefish.SplitRawStatements returns:
			// - First statement includes the leading line comment
			// - Second statement does not include the block comment that appears before it
			// TODO: This behavior may change when https://github.com/cloudspannerecosystem/memefish/pull/322 is merged
			want: []string{
				"-- This is a comment\nCREATE TABLE test (id INT64)",
				"INSERT INTO test VALUES (1)",
			},
		},
		{
			name:     "Empty content",
			content:  "",
			filename: "empty.sql",
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatements([]byte(tt.content), tt.filename)
			if err != nil {
				t.Fatalf("ParseStatements() error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Errorf("ParseStatements() returned %d statements, want %d", len(got), len(tt.want))
				t.Errorf("Got: %v", got)
				t.Errorf("Want: %v", tt.want)
			}

			for i := range got {
				if i < len(tt.want) && got[i] != tt.want[i] {
					t.Errorf("ParseStatements()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuiltinSampleRegistry(t *testing.T) {
	// Test that all expected samples are discovered correctly
	samples := discoverBuiltinSamples()

	expectedSamples := map[string]databasepb.DatabaseDialect{
		"banking":       databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"finance":       databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"finance-graph": databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"finance-pg":    databasepb.DatabaseDialect_POSTGRESQL,
		"gaming":        databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"fingraph":      databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"singers":       databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
	}

	for name, expectedDialect := range expectedSamples {
		sample, ok := samples[name]
		if !ok {
			t.Errorf("Sample %q not found in builtin samples", name)
			continue
		}

		if sample.ParsedDialect != expectedDialect {
			t.Errorf("Sample %q has dialect %v, want %v", name, sample.ParsedDialect, expectedDialect)
		}

		// Check that URIs are properly formatted
		if sample.SchemaURI == "" {
			t.Errorf("Sample %q has empty schema URI", name)
		}

		// Check that the URIs have valid schemes or are embedded
		if !strings.Contains(sample.SchemaURI, "://") {
			t.Errorf("Sample %q has invalid schema URI: %q", name, sample.SchemaURI)
		}
	}
}
