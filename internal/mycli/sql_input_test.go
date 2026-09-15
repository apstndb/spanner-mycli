//
// Copyright 2026 apstndb
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"github.com/apstndb/spanner-mycli/internal/mycli/filesafety"
)

func TestLocalPathFromFileURI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "empty authority", raw: "file:///tmp/script.sql", want: "/tmp/script.sql"},
		{name: "localhost", raw: "file://localhost/tmp/script.sql", want: "/tmp/script.sql"},
		{name: "percent-encoded space", raw: "file:///tmp/my%20script.sql", want: "/tmp/my script.sql"},
		{name: "single slash", raw: "file:/tmp/script.sql", want: "/tmp/script.sql"},
		{name: "reject host", raw: "file://example.com/tmp/script.sql", wantErr: "unsupported authority"},
		{name: "reject IP host", raw: "file://127.0.0.1/tmp/script.sql", wantErr: "unsupported authority"},
		{name: "empty path", raw: "file://localhost", wantErr: "empty path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatalf("url.Parse: %v", err)
			}
			got, err := localPathFromFileURI(u)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("localPathFromFileURI: %v", err)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadSQLInput_HTTPUsesCallerLimitNotSample(t *testing.T) {
	t.Parallel()
	body := bytes.Repeat([]byte("a"), 11)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "11")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	got, err := loadSQLInput(t.Context(), srv.URL+"/x.sql", &filesafety.FileSafetyOptions{MaxSize: 100})
	if err != nil {
		t.Fatalf("SQL caller cap 100: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q", got)
	}
	_, err = loadFromHTTPWithLimit(t.Context(), srv.URL+"/x.sql", 10)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("sample-style cap 10 error = %v, want rejection", err)
	}
}

func TestSQLInputMaxSizeIsNotSampleLimit(t *testing.T) {
	t.Parallel()
	got := sqlInputMaxSize(defaultSQLInputFileOptions())
	if got != filesafety.DefaultMaxFileSize {
		t.Fatalf("SQL input max = %d, want %d", got, filesafety.DefaultMaxFileSize)
	}
	if got <= filesafety.SampleDatabaseMaxFileSize {
		t.Fatalf("SQL input max %d must stay above the sample 10MiB cap %d", got, filesafety.SampleDatabaseMaxFileSize)
	}
}

func TestLoadSQLInput_BarePathAndFileURI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "SET CLI_PROMPT = 'ok';\nSET CLI_VERBOSE = TRUE;"
	path := filepath.Join(dir, "my script.sql")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	opts := defaultSQLInputFileOptions()

	got, err := loadSQLInput(ctx, path, opts)
	if err != nil {
		t.Fatalf("bare path: %v", err)
	}
	if string(got) != content {
		t.Fatalf("bare path = %q, want %q", got, content)
	}

	fileURI := (&url.URL{Scheme: "file", Path: path}).String()
	got, err = loadSQLInput(ctx, fileURI, opts)
	if err != nil {
		t.Fatalf("file URI: %v", err)
	}
	if string(got) != content {
		t.Fatalf("file URI = %q, want %q", got, content)
	}

	encoded := "file://" + filepath.ToSlash(filepath.Join(dir, "my%20script.sql"))
	got, err = loadSQLInput(ctx, encoded, opts)
	if err != nil {
		t.Fatalf("encoded file URI: %v", err)
	}
	if string(got) != content {
		t.Fatalf("encoded file URI = %q, want %q", got, content)
	}

	localhost := "file://localhost" + filepath.ToSlash(path)
	got, err = loadSQLInput(ctx, localhost, opts)
	if err != nil {
		t.Fatalf("localhost file URI: %v", err)
	}
	if string(got) != content {
		t.Fatalf("localhost file URI = %q, want %q", got, content)
	}
}

func TestLoadSQLInput_SchemeClassification(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	opts := defaultSQLInputFileOptions()

	_, err := loadSQLInput(ctx, "s3://bucket/script.sql", opts)
	if err == nil || !strings.Contains(err.Error(), "unsupported SQL input URI scheme") {
		t.Fatalf("unknown scheme error = %v", err)
	}
	if strings.Contains(err.Error(), "SELECT") {
		t.Fatalf("load error leaked SQL: %v", err)
	}

	_, err = loadSQLInput(ctx, `C:\does\not\exist.sql`, opts)
	if err == nil {
		t.Fatal("windows drive path: expected filesystem error")
	}
	if strings.Contains(err.Error(), "unsupported SQL input URI scheme") {
		t.Fatalf("windows drive path misclassified as URI: %v", err)
	}

	_, err = loadSQLInput(ctx, "file://remote.example/tmp/script.sql", opts)
	if err == nil || !strings.Contains(err.Error(), "unsupported authority") {
		t.Fatalf("remote file URI error = %v", err)
	}
}

func TestLoadSQLInput_HTTP(t *testing.T) {
	t.Parallel()
	const script = "SET CLI_PROMPT = 'first';\nSET CLI_PROMPT = 'second';"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/script.sql":
			_, _ = io.WriteString(w, script)
		case "/not-found.sql":
			http.NotFound(w, r)
		case "/over-declared.sql":
			w.Header().Set("Content-Length", fmt.Sprintf("%d", filesafety.DefaultMaxFileSize+1))
			w.WriteHeader(http.StatusOK)
		case "/over-chunked.sql":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("x"), 65))
		case "/hang.sql":
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	opts := defaultSQLInputFileOptions()
	small := &filesafety.FileSafetyOptions{MaxSize: 64, AllowNonRegular: true}

	t.Run("success", func(t *testing.T) {
		got, err := loadSQLInput(t.Context(), srv.URL+"/script.sql", opts)
		if err != nil {
			t.Fatalf("loadSQLInput: %v", err)
		}
		if string(got) != script {
			t.Fatalf("got %q, want %q", got, script)
		}
	})

	t.Run("https scheme via local http URL is http", func(t *testing.T) {
		got, err := loadSQLInput(t.Context(), strings.Replace(srv.URL, "http://", "HTTP://", 1)+"/script.sql", opts)
		if err != nil {
			t.Fatalf("case-insensitive http: %v", err)
		}
		if string(got) != script {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("non-200", func(t *testing.T) {
		_, err := loadSQLInput(t.Context(), srv.URL+"/not-found.sql", opts)
		if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
			t.Fatalf("error = %v, want HTTP 404", err)
		}
		if strings.Contains(err.Error(), script) {
			t.Fatalf("error leaked SQL: %v", err)
		}
	})

	t.Run("declared over-limit", func(t *testing.T) {
		_, err := loadSQLInput(t.Context(), srv.URL+"/over-declared.sql", opts)
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("error = %v, want declared size rejection", err)
		}
	})

	t.Run("chunked over-limit", func(t *testing.T) {
		_, err := loadSQLInput(t.Context(), srv.URL+"/over-chunked.sql", small)
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("error = %v, want streamed size rejection", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
		defer cancel()
		_, err := loadSQLInput(ctx, srv.URL+"/hang.sql", opts)
		if err == nil {
			t.Fatal("expected cancellation/timeout")
		}
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context") {
			t.Fatalf("error = %v, want context cancellation", err)
		}
	})
}

func TestLoadSQLInput_GCS(t *testing.T) {
	const (
		bucket = "sql-input-bucket"
		object = "dir/my script.sql"
	)
	script := "SET CLI_PROMPT = 'gcs';"
	uri := fmt.Sprintf("gs://%s/dir/my%%20script.sql", bucket)

	orig := newSQLInputGCSClient
	t.Cleanup(func() { newSQLInputGCSClient = orig })

	useFake := func(t *testing.T, srv *httptest.Server) {
		t.Helper()
		newSQLInputGCSClient = func(ctx context.Context) (*storage.Client, error) {
			return storage.NewClient(ctx, option.WithEndpoint(srv.URL), option.WithoutAuthentication())
		}
	}

	t.Run("success decodes object", func(t *testing.T) {
		srv := httptest.NewServer(gcsObjectHandler(t, bucket, object, int64(len(script)), []byte(script)))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		got, err := loadSQLInput(t.Context(), uri, defaultSQLInputFileOptions())
		if err != nil {
			t.Fatalf("loadSQLInput GCS: %v", err)
		}
		if string(got) != script {
			t.Fatalf("got %q, want %q", got, script)
		}
	})

	t.Run("declared attribute over-limit", func(t *testing.T) {
		srv := httptest.NewServer(gcsObjectHandler(t, bucket, object, filesafety.DefaultMaxFileSize+1, []byte(script)))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		_, err := loadSQLInput(t.Context(), uri, defaultSQLInputFileOptions())
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("error = %v, want attribute size rejection", err)
		}
		if strings.Contains(err.Error(), script) {
			t.Fatalf("error leaked SQL: %v", err)
		}
	})

	t.Run("streamed over-limit", func(t *testing.T) {
		body := bytes.Repeat([]byte("z"), 65)
		srv := httptest.NewServer(gcsObjectHandler(t, bucket, object, 1, body))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		_, err := loadSQLInput(t.Context(), uri, &filesafety.FileSafetyOptions{MaxSize: 64})
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("error = %v, want streamed size rejection", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
		defer cancel()
		_, err := loadSQLInput(ctx, uri, defaultSQLInputFileOptions())
		if err == nil {
			t.Fatal("expected cancellation/timeout")
		}
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context") {
			t.Fatalf("error = %v, want context cancellation", err)
		}
	})

	t.Run("client closed after success and failure", func(t *testing.T) {
		var created atomic.Int64
		okSrv := httptest.NewServer(gcsObjectHandler(t, bucket, object, int64(len(script)), []byte(script)))
		t.Cleanup(okSrv.Close)
		newSQLInputGCSClient = func(ctx context.Context) (*storage.Client, error) {
			created.Add(1)
			return storage.NewClient(ctx, option.WithEndpoint(okSrv.URL), option.WithoutAuthentication())
		}
		if _, err := loadSQLInput(t.Context(), uri, defaultSQLInputFileOptions()); err != nil {
			t.Fatalf("success close path: %v", err)
		}
		failSrv := httptest.NewServer(gcsObjectHandler(t, bucket, object, filesafety.DefaultMaxFileSize+1, []byte(script)))
		t.Cleanup(failSrv.Close)
		newSQLInputGCSClient = func(ctx context.Context) (*storage.Client, error) {
			created.Add(1)
			return storage.NewClient(ctx, option.WithEndpoint(failSrv.URL), option.WithoutAuthentication())
		}
		if _, err := loadSQLInput(t.Context(), uri, defaultSQLInputFileOptions()); err == nil {
			t.Fatal("expected failure so client still closes")
		}
		if created.Load() != 2 {
			t.Fatalf("created %d clients, want 2 (no cache)", created.Load())
		}
	})
}

func gcsObjectHandler(t *testing.T, bucket, object string, attrsSize int64, body []byte) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, _ := url.PathUnescape(r.URL.Path)
		alt := r.URL.Query().Get("alt")
		if !strings.Contains(path, "my script.sql") && !strings.Contains(r.URL.Path, "my%20script.sql") && !strings.Contains(path, object) {
			t.Errorf("unexpected GCS path %s", r.URL.String())
			http.NotFound(w, r)
			return
		}
		if alt == "media" || (!strings.Contains(path, "/b/") && strings.Contains(path, bucket)) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(body)
			return
		}
		_, _ = fmt.Fprintf(w, `{
			"bucket": %q,
			"name": %q,
			"size": "%d",
			"contentType": "text/plain",
			"timeCreated": "2026-09-15T00:00:00Z",
			"updated": "2026-09-15T00:00:00Z"
		}`, bucket, object, attrsSize)
	})
}

func TestCli_executeSourceFile_URI(t *testing.T) {
	t.Parallel()
	scriptOK := "SET CLI_PROMPT = 'first';\nSET CLI_PROMPT = 'second';"
	scriptParse := "SET CLI_PROMPT = 'changed';\nINVALID SYNTAX;"
	scriptNested := "SET CLI_PROMPT = 'changed';\n\\. https://example.com/other.sql;"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.sql":
			_, _ = io.WriteString(w, scriptOK)
		case "/parse.sql":
			_, _ = io.WriteString(w, scriptParse)
		case "/nested.sql":
			_, _ = io.WriteString(w, scriptNested)
		case "/fail.sql":
			http.Error(w, "nope", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Run("http multi-statement", func(t *testing.T) {
		var out bytes.Buffer
		cli := newDetachedEchoCli(t, &out)
		if err := cli.executeSourceFile(t.Context(), srv.URL+"/ok.sql"); err != nil {
			t.Fatalf("executeSourceFile: %v", err)
		}
		if cli.SystemVariables.Display.Prompt != "second" {
			t.Fatalf("prompt = %q, want second", cli.SystemVariables.Display.Prompt)
		}
	})

	t.Run("file URI multi-statement", func(t *testing.T) {
		var out bytes.Buffer
		cli := newDetachedEchoCli(t, &out)
		path := writeTempSQL(t, scriptOK)
		uri := (&url.URL{Scheme: "file", Path: path}).String()
		if err := cli.executeSourceFile(t.Context(), uri); err != nil {
			t.Fatalf("executeSourceFile file URI: %v", err)
		}
		if cli.SystemVariables.Display.Prompt != "second" {
			t.Fatalf("prompt = %q, want second", cli.SystemVariables.Display.Prompt)
		}
	})

	t.Run("late syntax error executes nothing", func(t *testing.T) {
		var out bytes.Buffer
		cli := newDetachedEchoCli(t, &out)
		err := cli.executeSourceFile(t.Context(), srv.URL+"/parse.sql")
		if err == nil || !strings.Contains(err.Error(), "failed to parse SQL from file") {
			t.Fatalf("error = %v, want parse failure", err)
		}
		if cli.SystemVariables.Display.Prompt != defaultPrompt {
			t.Fatalf("prompt = %q, want unchanged", cli.SystemVariables.Display.Prompt)
		}
	})

	t.Run("nested meta-command rejected", func(t *testing.T) {
		var out bytes.Buffer
		cli := newDetachedEchoCli(t, &out)
		err := cli.executeSourceFile(t.Context(), srv.URL+"/nested.sql")
		if err == nil || !strings.Contains(err.Error(), "meta commands are not supported") {
			t.Fatalf("error = %v, want nested SOURCE rejection", err)
		}
		if cli.SystemVariables.Display.Prompt != defaultPrompt {
			t.Fatalf("prompt = %q, want unchanged", cli.SystemVariables.Display.Prompt)
		}
	})

	t.Run("transport failure executes nothing", func(t *testing.T) {
		var out bytes.Buffer
		cli := newDetachedEchoCli(t, &out)
		err := cli.executeSourceFile(t.Context(), srv.URL+"/fail.sql")
		if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
			t.Fatalf("error = %v, want HTTP 502", err)
		}
		if cli.SystemVariables.Display.Prompt != defaultPrompt {
			t.Fatalf("prompt = %q, want unchanged", cli.SystemVariables.Display.Prompt)
		}
	})
}

func TestDetermineInputAndMode_URI(t *testing.T) {
	t.Parallel()
	script := "SELECT 1;\nSELECT 2;"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, script)
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "input.sql")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("http --file", func(t *testing.T) {
		input, interactive, err := determineInputAndMode(t.Context(), &spannerOptions{File: srv.URL + "/script.sql"}, bytes.NewReader(nil))
		if err != nil {
			t.Fatalf("determineInputAndMode: %v", err)
		}
		if interactive {
			t.Fatal("expected batch mode")
		}
		if input != script {
			t.Fatalf("input = %q, want %q", input, script)
		}
	})

	t.Run("file URI --source alias", func(t *testing.T) {
		uri := (&url.URL{Scheme: "file", Path: path}).String()
		input, interactive, err := determineInputAndMode(t.Context(), &spannerOptions{Source: uri}, bytes.NewReader(nil))
		if err != nil {
			t.Fatalf("determineInputAndMode: %v", err)
		}
		if interactive || input != script {
			t.Fatalf("interactive=%v input=%q", interactive, input)
		}
	})

	t.Run("execute still wins", func(t *testing.T) {
		input, interactive, err := determineInputAndMode(t.Context(), &spannerOptions{
			Execute: "SELECT 9;",
			File:    srv.URL + "/script.sql",
		}, bytes.NewReader(nil))
		if err != nil {
			t.Fatalf("determineInputAndMode: %v", err)
		}
		if interactive || input != "SELECT 9;" {
			t.Fatalf("interactive=%v input=%q", interactive, input)
		}
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		_, _, err := determineInputAndMode(t.Context(), &spannerOptions{File: "s3://bucket/obj.sql"}, bytes.NewReader(nil))
		if err == nil || !strings.Contains(err.Error(), "unsupported SQL input URI scheme") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("file dash stdin unchanged", func(t *testing.T) {
		input, interactive, err := determineInputAndMode(t.Context(), &spannerOptions{File: "-"}, strings.NewReader("SELECT 3;"))
		if err != nil {
			t.Fatalf("determineInputAndMode: %v", err)
		}
		if interactive || input != "SELECT 3;" {
			t.Fatalf("interactive=%v input=%q", interactive, input)
		}
	})
}

func TestRemoteSQLInputContext_PreservesCallerDeadline(t *testing.T) {
	t.Parallel()
	parent, cancel := context.WithTimeout(t.Context(), 15*time.Millisecond)
	defer cancel()
	ctx, stop := remoteSQLInputContext(parent)
	defer stop()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	parentDeadline, _ := parent.Deadline()
	if !deadline.Equal(parentDeadline) {
		t.Fatalf("deadline = %v, want parent %v", deadline, parentDeadline)
	}
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("err = %v, want parent DeadlineExceeded", ctx.Err())
	}
}
