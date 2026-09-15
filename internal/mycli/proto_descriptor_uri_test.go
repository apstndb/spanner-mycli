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

package mycli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"

	"github.com/apstndb/spanner-mycli/internal/mycli/filesafety"
)

func fileURLForPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

func TestFileProtoDescriptorURI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dep := writeProtoSource(t, filepath.Join(dir, "dep.proto"), `syntax="proto3"; package fileuri; message Dep { string value=1; }`)
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), fmt.Sprintf(`syntax="proto3"; package fileuri; import %q; message Root { Dep child=1; }`, filepath.ToSlash(dep)))
	percent := writeProtoSource(t, filepath.Join(dir, "migration-100%.proto"), `syntax="proto3"; package pct; message Root { string value=1; }`)
	space := writeProtoSource(t, filepath.Join(dir, "my proto.proto"), `syntax="proto3"; package spaced; message Root { string value=1; }`)
	binary := writeDescriptorSet(t, filepath.Join(dir, "loop.pb"), descriptorFile("loopbin.proto", "loopbin"))

	bare, err := readFileDescriptorProtoFromFile(root)
	if err != nil {
		t.Fatal(err)
	}
	uriFDS, err := readFileDescriptorProtoFromFile(fileURLForPath(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(bare, uriFDS) {
		t.Fatalf("file:// source identity differs from bare path:\nbare=%v\nuri=%v", bare, uriFDS)
	}
	requireUsableDescriptor(t, uriFDS)

	binBare, err := readFileDescriptorProtoFromFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	binURI, err := readFileDescriptorProtoFromFile(fileURLForPath(t, binary))
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(binBare, binURI) {
		t.Fatal("file:// binary identity differs from bare path")
	}

	percentFDS, err := readFileDescriptorProtoFromFile(percent)
	if err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, percentFDS)
	if !containsDescriptorPackage(percentFDS, "pct") {
		t.Fatal("bare percent filename was treated as a URI")
	}

	spaceURI := fileURLForPath(t, space)
	if !strings.Contains(spaceURI, "%20") {
		t.Fatalf("expected percent-encoded space in %q", spaceURI)
	}
	spaceFDS, err := readFileDescriptorProtoFromFile(spaceURI)
	if err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, spaceFDS)
	if !containsDescriptorPackage(spaceFDS, "spaced") {
		t.Fatal("percent-encoded file:// name did not decode")
	}

	imported := writeProtoSource(t, filepath.Join(dir, "via_file_import.proto"), fmt.Sprintf(
		`syntax="proto3"; package fileimp; import %q; message Root { fileuri.Dep child=1; }`,
		fileURLForPath(t, dep),
	))
	importedFDS, err := readFileDescriptorProtoFromFile(imported)
	if err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, importedFDS)
	var names []string
	for _, file := range importedFDS.File {
		names = append(names, file.GetName())
		if strings.HasPrefix(file.GetName(), "file:") {
			t.Fatalf("file:// import kept URI identity: %s", file.GetName())
		}
	}
	if !slices.Contains(names, filepath.ToSlash(func() string {
		abs, err := filepath.Abs(dep)
		if err != nil {
			t.Fatal(err)
		}
		return abs
	}())) && !slices.Contains(names, filepath.ToSlash(dep)) {
		t.Fatalf("decoded file:// import name missing: %v", names)
	}

	_, err = readFileDescriptorProtoFromFile("file://example.com/tmp/x.proto")
	if err == nil || !strings.Contains(err.Error(), "unsupported authority") {
		t.Fatalf("remote file:// error = %v", err)
	}

	_, err = readFileDescriptorProtoFromFile("s3://bucket/root.proto")
	if err == nil || !strings.Contains(err.Error(), "unsupported proto descriptor URI scheme") {
		t.Fatalf("unknown scheme error = %v", err)
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Fatalf("unknown scheme fell through to local open: %v", err)
	}

	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", fileURLForPath(t, root)); err != nil {
		t.Fatal(err)
	}
	beforeGraph, beforeFiles := cloneDescriptorState(sv)
	if err := sv.AddFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "file://example.com/tmp/missing.proto"); err == nil {
		t.Fatal("invalid ADD succeeded")
	}
	assertDescriptorState(t, sv, beforeGraph, beforeFiles)

	var started systemVariables
	if err := applyProtoDescriptors(&started, &spannerOptions{ProtoDescriptorFile: fileURLForPath(t, binary)}); err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, started.Internal.ProtoDescriptor)
	if !containsDescriptorPackage(started.Internal.ProtoDescriptor, "loopbin") {
		t.Fatal("startup file:// binary missing package")
	}
}

func TestProtoDescriptorMixedFileURIImportIdentity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dep := writeProtoSource(t, filepath.Join(dir, "dep.proto"), `syntax="proto3"; package mixed; message Dep { string value=1; }`)
	abs, err := filepath.Abs(dep)
	if err != nil {
		t.Fatal(err)
	}
	slash := filepath.ToSlash(abs)
	fileURI := fileURLForPath(t, dep)
	a := writeProtoSource(t, filepath.Join(dir, "a.proto"), fmt.Sprintf(`syntax="proto3"; package mixed; import %q; message A { Dep d=1; }`, slash))
	b := writeProtoSource(t, filepath.Join(dir, "b.proto"), fmt.Sprintf(`syntax="proto3"; package mixed; import %q; message B { Dep d=1; }`, fileURI))
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), fmt.Sprintf(`syntax="proto3"; package mixed; import %q; import %q; message Root { A a=1; B b=2; }`, a, b))

	if _, err := readFileDescriptorProtoFromFile(a); err != nil {
		t.Fatalf("bare-path branch alone: %v", err)
	}
	if _, err := readFileDescriptorProtoFromFile(b); err != nil {
		t.Fatalf("file:// branch alone: %v", err)
	}

	fds, err := readFileDescriptorProtoFromFile(root)
	if err != nil {
		t.Fatalf("mixed file:// / bare-path graph: %v", err)
	}
	files := requireUsableDescriptor(t, fds)
	if _, err := files.FindDescriptorByName("mixed.Dep"); err != nil {
		t.Fatal(err)
	}
	if _, err := files.FindDescriptorByName("mixed.A"); err != nil {
		t.Fatal(err)
	}
	if _, err := files.FindDescriptorByName("mixed.B"); err != nil {
		t.Fatal(err)
	}
	if _, err := files.FindDescriptorByName("mixed.Root"); err != nil {
		t.Fatal(err)
	}

	var names []string
	depCount := 0
	for _, file := range fds.File {
		names = append(names, file.GetName())
		if strings.HasPrefix(file.GetName(), "file:") {
			t.Fatalf("file:// identity leaked into mixed graph: %v", names)
		}
		if file.GetName() == slash || file.GetName() == filepath.ToSlash(dep) {
			depCount++
		}
	}
	if depCount != 1 {
		t.Fatalf("want one decoded dep identity, got %d in %v", depCount, names)
	}
}

func TestFileProtoDescriptorSourceRootPolicy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("oversized source root", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(dir, "huge.proto")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("!")); err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(filesafety.DefaultMaxFileSize + 1); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		_, err = readFileDescriptorProtoFromFile(fileURLForPath(t, path))
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("error = %v, want size rejection before compile", err)
		}
		if strings.Contains(err.Error(), "invalid character") {
			t.Fatalf("oversized file:// source reached the parser: %v", err)
		}
	})

	t.Run("non-regular source root", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("named pipes are not created with Mkfifo on Windows")
		}
		path := filepath.Join(dir, "fifo.proto")
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Skipf("Skipping FIFO test: %v", err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := readFileDescriptorProtoFromFile(fileURLForPath(t, path))
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "cannot read named pipe") {
				t.Fatalf("error = %v, want named-pipe rejection before open", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("file:// FIFO source root hung instead of rejecting")
		}
	})
}

func TestGCSProtoDescriptorURI(t *testing.T) {
	const (
		bucket     = "proto-uri-bucket"
		rootObject = "dir/root.proto"
		depObject  = "dir/dep.proto"
		binObject  = "dir/loop.pb"
		spaceObj   = "dir/my proto.proto"
	)
	depSrc := `syntax="proto3"; package gcsuri; message Dep { string value=1; }`
	rootSrc := `syntax="proto3"; package gcsuri; import "gs://proto-uri-bucket/dir/dep.proto"; message Root { Dep child=1; }`
	relativeSrc := `syntax="proto3"; package gcsrel; import "dep.proto"; message Root { Dep child=1; }`
	spaceSrc := `syntax="proto3"; package gcsspace; message Root { string value=1; }`
	binary := mustMarshalLoopbackBinary(t)
	rootURI := "gs://proto-uri-bucket/dir/root.proto"
	depURI := "gs://proto-uri-bucket/dir/dep.proto"
	binURI := "gs://proto-uri-bucket/dir/loop.pb"
	spaceURI := "gs://proto-uri-bucket/dir/my%20proto.proto"

	orig := newSQLInputGCSClient
	t.Cleanup(func() { newSQLInputGCSClient = orig })

	useFake := func(t *testing.T, srv *httptest.Server) {
		t.Helper()
		newSQLInputGCSClient = func(ctx context.Context) (*storage.Client, error) {
			return storage.NewClient(ctx, option.WithEndpoint(srv.URL), option.WithoutAuthentication())
		}
	}

	t.Run("source and explicit import keep URI identity", func(t *testing.T) {
		srv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + depObject:  {attrsSize: int64(len(depSrc)), body: []byte(depSrc)},
			bucket + "/" + rootObject: {attrsSize: int64(len(rootSrc)), body: []byte(rootSrc)},
		}))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		fds, err := readFileDescriptorProtoFromFileContext(t.Context(), rootURI)
		if err != nil {
			t.Fatal(err)
		}
		requireUsableDescriptor(t, fds)
		var names []string
		for _, file := range fds.File {
			names = append(names, file.GetName())
		}
		if !slices.Contains(names, rootURI) || !slices.Contains(names, depURI) {
			t.Fatalf("gs:// identity missing: %v", names)
		}
	})

	t.Run("relative import is not rewritten onto the bucket", func(t *testing.T) {
		srv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + rootObject: {attrsSize: int64(len(relativeSrc)), body: []byte(relativeSrc)},
		}))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		_, err := readFileDescriptorProtoFromFileContext(t.Context(), rootURI)
		if err == nil {
			t.Fatal("relative import from gs:// root unexpectedly compiled")
		}
		if strings.Contains(err.Error(), "syntax =") || strings.Contains(err.Error(), relativeSrc) {
			t.Fatalf("error leaked proto bytes: %v", err)
		}
	})

	t.Run("binary and percent-encoded object", func(t *testing.T) {
		srv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + binObject: {attrsSize: int64(len(binary)), body: binary},
			bucket + "/" + spaceObj:  {attrsSize: int64(len(spaceSrc)), body: []byte(spaceSrc)},
		}))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		fds, err := readFileDescriptorProtoFromFileContext(t.Context(), binURI)
		if err != nil {
			t.Fatal(err)
		}
		requireUsableDescriptor(t, fds)
		if !containsDescriptorPackage(fds, "loopbin") {
			t.Fatal("gs:// binary missing package")
		}
		spaceFDS, err := readFileDescriptorProtoFromFileContext(t.Context(), spaceURI)
		if err != nil {
			t.Fatal(err)
		}
		requireUsableDescriptor(t, spaceFDS)
		if !containsDescriptorPackage(spaceFDS, "gcsspace") {
			t.Fatal("percent-encoded gs:// object missing package")
		}
	})

	t.Run("declared over-limit", func(t *testing.T) {
		srv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + binObject: {attrsSize: filesafety.DefaultMaxFileSize + 1, body: binary},
		}))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		_, err := readFileDescriptorProtoFromFileContext(t.Context(), binURI)
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("error = %v, want size rejection", err)
		}
		if bytes.Contains([]byte(err.Error()), binary) {
			t.Fatalf("error leaked descriptor bytes: %v", err)
		}
	})

	t.Run("streamed over-limit", func(t *testing.T) {
		body := bytes.Repeat([]byte("z"), 65)
		srv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + binObject: {attrsSize: 1, body: body},
		}))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		// The production loader always uses the 100 MiB cap. Exercise the
		// shared transport helper with a tighter limit so the streamed path
		// stays covered without a 100 MiB body.
		origClient := newSQLInputGCSClient
		t.Cleanup(func() { newSQLInputGCSClient = origClient })
		client, err := storage.NewClient(t.Context(), option.WithEndpoint(srv.URL), option.WithoutAuthentication())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		_, err = loadFromGCSWithClientAndLimit(t.Context(), client, binURI, 64)
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
		_, err := readFileDescriptorProtoFromFileContext(ctx, binURI)
		if err == nil {
			t.Fatal("expected cancellation/timeout")
		}
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context") {
			t.Fatalf("error = %v, want context cancellation", err)
		}
	})

	t.Run("creates a new client for success and failure", func(t *testing.T) {
		var created atomic.Int64
		okSrv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + binObject: {attrsSize: int64(len(binary)), body: binary},
		}))
		t.Cleanup(okSrv.Close)
		newSQLInputGCSClient = func(ctx context.Context) (*storage.Client, error) {
			created.Add(1)
			return storage.NewClient(ctx, option.WithEndpoint(okSrv.URL), option.WithoutAuthentication())
		}
		if _, err := readFileDescriptorProtoFromFileContext(t.Context(), binURI); err != nil {
			t.Fatalf("success path: %v", err)
		}
		failSrv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + binObject: {attrsSize: filesafety.DefaultMaxFileSize + 1, body: binary},
		}))
		t.Cleanup(failSrv.Close)
		newSQLInputGCSClient = func(ctx context.Context) (*storage.Client, error) {
			created.Add(1)
			return storage.NewClient(ctx, option.WithEndpoint(failSrv.URL), option.WithoutAuthentication())
		}
		if _, err := readFileDescriptorProtoFromFileContext(t.Context(), binURI); err == nil {
			t.Fatal("expected failure on a second uncached client")
		}
		if created.Load() != 2 {
			t.Fatalf("created %d clients, want 2 (no cache)", created.Load())
		}
	})

	t.Run("SET ADD and startup stay atomic", func(t *testing.T) {
		srv := httptest.NewServer(protoGCSObjectsHandler(t, map[string]protoGCSObject{
			bucket + "/" + binObject: {attrsSize: int64(len(binary)), body: binary},
		}))
		t.Cleanup(srv.Close)
		useFake(t, srv)
		sv := newSystemVariablesWithDefaultsForTest()
		sv.ensureRegistry()
		if err := sv.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", binURI); err != nil {
			t.Fatal(err)
		}
		beforeGraph, beforeFiles := cloneDescriptorState(sv)
		if err := sv.AddFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "gs://proto-uri-bucket/dir/missing.pb"); err == nil {
			t.Fatal("invalid ADD succeeded")
		}
		assertDescriptorState(t, sv, beforeGraph, beforeFiles)

		var started systemVariables
		if err := applyProtoDescriptorsContext(t.Context(), &started, &spannerOptions{ProtoDescriptorFile: binURI}); err != nil {
			t.Fatal(err)
		}
		requireUsableDescriptor(t, started.Internal.ProtoDescriptor)
	})
}

type protoGCSObject struct {
	attrsSize int64
	body      []byte
}

func protoGCSObjectsHandler(t *testing.T, objects map[string]protoGCSObject) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoded, _ := url.PathUnescape(r.URL.Path)
		for key, obj := range objects {
			bucket, object, ok := strings.Cut(key, "/")
			if !ok {
				continue
			}
			if !strings.Contains(decoded, bucket) || (!strings.Contains(decoded, object) && !strings.Contains(r.URL.Path, url.PathEscape(object))) {
				continue
			}
			if r.URL.Query().Get("alt") == "media" || (!strings.Contains(decoded, "/b/") && strings.Contains(decoded, bucket)) {
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(obj.body)
				return
			}
			_, _ = fmt.Fprintf(w, `{
				"bucket": %q,
				"name": %q,
				"size": "%d",
				"contentType": "application/octet-stream",
				"timeCreated": "2026-09-15T00:00:00Z",
				"updated": "2026-09-15T00:00:00Z"
			}`, bucket, object, obj.attrsSize)
			return
		}
		http.NotFound(w, r)
	})
}
