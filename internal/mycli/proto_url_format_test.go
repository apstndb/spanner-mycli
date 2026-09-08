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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

const loopbackProtoSource = `syntax = "proto3";
package loop;
import "google/protobuf/timestamp.proto";
message Root { google.protobuf.Timestamp value = 1; }
`

func TestProtoDescriptorLooksLikeSource(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, filename string
		want           bool
		wantErr        string
	}{
		{name: "http source query", filename: "http://example.test/root.proto?fixture=1", want: true},
		{name: "http source fragment", filename: "http://example.test/root.proto#section", want: true},
		{name: "http binary query looks like proto", filename: "http://example.test/binary.pb?filename=.proto", want: false},
		{name: "https percent path", filename: "https://example.test/dir%2Froot.proto", want: true},
		{name: "malformed escape", filename: "http://example.test/%zz.proto", wantErr: "invalid URL escape"},
		{name: "local source", filename: "testdata/protos/singer.proto", want: true},
		{name: "local binary", filename: "testdata/protos/order_descriptors.pb", want: false},
		{name: "local literal query suffix", filename: "weird.pb?filename=.proto", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := protoDescriptorLooksLikeSource(tt.filename)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTPProtoFormatDetection(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	binary := mustMarshalLoopbackBinary(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RequestURI())
		mu.Unlock()
		switch r.URL.Path {
		case "/root.proto", "/dir/root.proto":
			_, _ = w.Write([]byte(loopbackProtoSource))
		case "/binary.pb":
			_, _ = w.Write(binary)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	assertRequest := func(t *testing.T, wantURI string) {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if len(requests) == 0 {
			t.Fatal("no HTTP request recorded")
		}
		got := requests[len(requests)-1]
		if got != wantURI {
			t.Fatalf("RequestURI = %q, want exact %q", got, wantURI)
		}
		if strings.Contains(got, "#") {
			t.Fatalf("fragment leaked into request %q", got)
		}
	}

	t.Run("source variants", func(t *testing.T) {
		for _, tt := range []struct {
			name, suffix, wantURI string
		}{
			{name: "bare", suffix: "/root.proto", wantURI: "/root.proto"},
			{name: "query", suffix: "/root.proto?fixture=1", wantURI: "/root.proto?fixture=1"},
			{name: "fragment", suffix: "/root.proto#section", wantURI: "/root.proto"},
			{name: "query and fragment", suffix: "/root.proto?fixture=1#section", wantURI: "/root.proto?fixture=1"},
			{name: "percent path", suffix: "/dir%2Froot.proto", wantURI: "/dir%2Froot.proto"},
			{name: "percent query", suffix: "/root.proto?q=%3D1", wantURI: "/root.proto?q=%3D1"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				fds, err := readFileDescriptorProtoFromFile(srv.URL + tt.suffix)
				if err != nil {
					t.Fatal(err)
				}
				requireUsableDescriptor(t, fds)
				if !containsDescriptorPackage(fds, "loop") {
					t.Fatalf("missing loop package: %v", fds)
				}
				assertRequest(t, tt.wantURI)
			})
		}
	})

	t.Run("binary variants", func(t *testing.T) {
		for _, tt := range []struct {
			name, suffix, wantURI string
		}{
			{name: "bare", suffix: "/binary.pb", wantURI: "/binary.pb"},
			{name: "query looks like proto", suffix: "/binary.pb?filename=.proto", wantURI: "/binary.pb?filename=.proto"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				fds, err := readFileDescriptorProtoFromFile(srv.URL + tt.suffix)
				if err != nil {
					t.Fatal(err)
				}
				requireUsableDescriptor(t, fds)
				if !containsDescriptorPackage(fds, "loopbin") {
					t.Fatalf("missing loopbin package: %v", fds)
				}
				assertRequest(t, tt.wantURI)
			})
		}
	})

	t.Run("malformed escape does not fetch", func(t *testing.T) {
		mu.Lock()
		before := len(requests)
		mu.Unlock()
		_, err := readFileDescriptorProtoFromFile(srv.URL + "/%zz.proto")
		if err == nil || !strings.Contains(err.Error(), "invalid URL escape") {
			t.Fatalf("error = %v, want invalid URL escape", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(requests) != before {
			t.Fatalf("malformed URL issued %d extra requests", len(requests)-before)
		}
	})
}

func TestHTTPProtoDescriptorGraphAndStartup(t *testing.T) {
	binary := mustMarshalLoopbackBinary(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root.proto":
			_, _ = w.Write([]byte(loopbackProtoSource))
		case "/binary.pb":
			_, _ = w.Write(binary)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	sourceURL := srv.URL + "/root.proto?fixture=1"
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", sourceURL); err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
	if !containsDescriptorPackage(sv.Internal.ProtoDescriptor, "loop") {
		t.Fatal("SET HTTP source missing loop package")
	}
	beforeGraph, beforeFiles := cloneDescriptorState(sv)

	if err := sv.AddFromSimple("PROTO_DESCRIPTORS_FILE_PATH", srv.URL+"/missing.pb"); err == nil {
		t.Fatal("invalid ADD succeeded")
	}
	assertDescriptorState(t, sv, beforeGraph, beforeFiles)

	var started systemVariables
	if err := applyProtoDescriptors(&started, &spannerOptions{ProtoDescriptorFile: srv.URL + "/binary.pb?filename=.proto"}); err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, started.Internal.ProtoDescriptor)
	if !containsDescriptorPackage(started.Internal.ProtoDescriptor, "loopbin") {
		t.Fatal("startup HTTP binary query classified as source")
	}
}

func TestLocalProtoFormatControls(t *testing.T) {
	t.Parallel()
	source, err := readFileDescriptorProtoFromFile("testdata/protos/singer.proto")
	if err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, source)
	binary, err := readFileDescriptorProtoFromFile("testdata/protos/order_descriptors.pb")
	if err != nil {
		t.Fatal(err)
	}
	requireUsableDescriptor(t, binary)

	if runtime.GOOS == "windows" {
		t.Skip("literal ?/# filenames are not asserted on Windows")
	}
	dir := t.TempDir()
	for _, name := range []string{"weird?.proto", "weird#.proto"} {
		literal := filepath.Join(dir, name)
		if err := os.WriteFile(literal, []byte(`syntax="proto3"; package localq; message Root { string value=1; }`), 0o600); err != nil {
			t.Fatal(err)
		}
		fds, err := readFileDescriptorProtoFromFile(literal)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		requireUsableDescriptor(t, fds)
		if !containsDescriptorPackage(fds, "localq") {
			t.Fatalf("local literal %s missing package: %v", name, fds)
		}
	}
}

func mustMarshalLoopbackBinary(t *testing.T) []byte {
	t.Helper()
	b, err := proto.Marshal(&descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		descriptorFile("loopbin.proto", "loopbin"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestProtoDescriptorLooksLikeSourcePercentPathExt(t *testing.T) {
	t.Parallel()
	u, err := url.Parse("https://example.test/dir%2Froot.proto")
	if err != nil {
		t.Fatal(err)
	}
	if path.Ext(u.Path) != ".proto" {
		t.Fatalf("decoded path %q ext %q", u.Path, path.Ext(u.Path))
	}
}
