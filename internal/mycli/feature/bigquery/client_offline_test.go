// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bq "cloud.google.com/go/bigquery"
	"google.golang.org/api/option"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

func TestClientCacheCloseNil(t *testing.T) {
	t.Parallel()

	c := &clientCache{}
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestClientCacheGetRequiresProject(t *testing.T) {
	t.Parallel()

	session := &mycli.Session{}
	c := &clientCache{cfg: &config{}}
	_, err := c.get(t.Context(), session)
	if err == nil || !strings.Contains(err.Error(), "CLI_BIGQUERY_PROJECT") {
		t.Fatalf("get() error = %v, want project configuration error", err)
	}
}

func TestClientCacheGetReusesMatchingClient(t *testing.T) {
	t.Parallel()

	session := &mycli.Session{}
	fake, err := bq.NewClient(t.Context(), "bq-project", option.WithoutAuthentication(), option.WithEndpoint("http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("bq.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = fake.Close() })

	c := &clientCache{
		cfg:    &config{Project: "bq-project", Location: "US"},
		client: fake,
		key:    clientKey{project: "bq-project", location: "US"},
	}
	got, err := c.get(t.Context(), session)
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if got != fake {
		t.Fatal("get() rebuilt the client instead of returning the cached instance")
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if c.client != nil {
		t.Fatal("Close() left a cached client")
	}
}

func TestBigQueryExecuteRequiresProject(t *testing.T) {
	t.Parallel()

	session := &mycli.Session{}
	stmt := newBigQueryStatement("SELECT 1", &config{})
	_, err := stmt.Execute(t.Context(), session)
	if err == nil || !strings.Contains(err.Error(), "CLI_BIGQUERY_PROJECT") {
		t.Fatalf("Execute() error = %v, want project configuration error", err)
	}
}

func TestBigQueryExecuteFakeClient(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"jobComplete": true,
			"totalRows": "1",
			"jobReference": {"projectId": "bq-project", "jobId": "job-1", "location": "US"},
			"schema": {"fields": [{"name": "col", "type": "STRING", "mode": "NULLABLE"}]},
			"rows": [{"f": [{"v": "hello"}]}]
		}`))
	}))
	t.Cleanup(server.Close)

	client, err := bq.NewClient(t.Context(), "bq-project",
		option.WithoutAuthentication(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("bq.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	maxBytes := int64(12345)
	cfg := &config{Project: "bq-project", Location: "US", MaxBytesBilled: &maxBytes}
	session := &mycli.Session{}
	if _, err := mycli.FeatureState(t.Context(), session, clientStateKey, func(context.Context, *mycli.Session) (*clientCache, error) {
		return &clientCache{
			cfg:    cfg,
			client: client,
			key:    clientKey{project: "bq-project", location: "US"},
		}, nil
	}); err != nil {
		t.Fatalf("FeatureState: %v", err)
	}

	stmt := newBigQueryStatement("SELECT col FROM t", cfg)
	got, err := stmt.Execute(t.Context(), session)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", got.AffectedRows)
	}
	if got.TableHeader == nil {
		t.Fatal("TableHeader is nil")
	}
	if gotNames := got.TableHeader.Render(false); len(gotNames) != 1 || gotNames[0] != "col" {
		t.Errorf("headers = %v, want [col]", gotNames)
	}
	if len(got.Rows) != 1 || len(got.Rows[0]) != 1 || got.Rows[0][0].RawText() != "hello" {
		t.Errorf("rows = %#v, want hello", got.Rows)
	}
	if !strings.Contains(gotPath, "/projects/bq-project/queries") {
		t.Errorf("request path = %q, want jobs.query", gotPath)
	}
	if gotBody["query"] != "SELECT col FROM t" {
		t.Errorf("query = %v, want SELECT col FROM t", gotBody["query"])
	}
	if gotBody["location"] != "US" {
		t.Errorf("location = %v, want US", gotBody["location"])
	}
	switch billed := gotBody["maximumBytesBilled"].(type) {
	case string:
		if billed != "12345" {
			t.Errorf("maximumBytesBilled = %q, want 12345", billed)
		}
	case float64:
		if billed != 12345 {
			t.Errorf("maximumBytesBilled = %v, want 12345", billed)
		}
	default:
		t.Errorf("maximumBytesBilled = %#v, want 12345", gotBody["maximumBytesBilled"])
	}
}

func TestBigQueryExecuteFakeClientReadError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"job failed"}}`, http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	client, err := bq.NewClient(t.Context(), "bq-project",
		option.WithoutAuthentication(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("bq.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config{Project: "bq-project"}
	session := &mycli.Session{}
	if _, err := mycli.FeatureState(t.Context(), session, clientStateKey, func(context.Context, *mycli.Session) (*clientCache, error) {
		return &clientCache{
			cfg:    cfg,
			client: client,
			key:    clientKey{project: "bq-project"},
		}, nil
	}); err != nil {
		t.Fatalf("FeatureState: %v", err)
	}

	_, err = newBigQueryStatement("SELECT 1", cfg).Execute(t.Context(), session)
	if err == nil {
		t.Fatal("Execute() error = nil, want query read failure")
	}
}
