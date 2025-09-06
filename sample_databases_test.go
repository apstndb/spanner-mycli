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

	// Check that all samples are listed
	expectedSamples := []string{"banking", "finance", "finance-graph", "finance-pg", "gaming"}
	for _, sample := range expectedSamples {
		if !strings.Contains(output, sample) {
			t.Errorf("ListAvailableSamples() missing sample %q", sample)
		}
	}

	// Check for usage instruction
	if !strings.Contains(output, "Usage:") {
		t.Error("ListAvailableSamples() missing usage instruction")
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

func TestSampleDatabaseRegistry(t *testing.T) {
	// Test that all expected samples are in registry
	expectedSamples := map[string]databasepb.DatabaseDialect{
		"banking":       databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"finance":       databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"finance-graph": databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
		"finance-pg":    databasepb.DatabaseDialect_POSTGRESQL,
		"gaming":        databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL,
	}

	for name, expectedDialect := range expectedSamples {
		sample, ok := sampleDatabases[name]
		if !ok {
			t.Errorf("Sample %q not found in registry", name)
			continue
		}

		if sample.Dialect != expectedDialect {
			t.Errorf("Sample %q has dialect %v, want %v", name, sample.Dialect, expectedDialect)
		}

		// Check that URIs are properly formatted
		if !strings.HasPrefix(sample.SchemaURI, "gs://") {
			t.Errorf("Sample %q has invalid schema URI: %q", name, sample.SchemaURI)
		}

		// finance and finance-pg don't have data URIs
		if name != "finance" && name != "finance-pg" {
			if !strings.HasPrefix(sample.DataURI, "gs://") {
				t.Errorf("Sample %q has invalid data URI: %q", name, sample.DataURI)
			}
		}
	}
}
