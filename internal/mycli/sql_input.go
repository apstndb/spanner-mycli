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
	"context"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"
	"unicode"

	"cloud.google.com/go/storage"

	"github.com/apstndb/spanner-mycli/internal/mycli/filesafety"
)

const sqlInputRemoteTimeout = 30 * time.Second

// newSQLInputGCSClient creates the production GCS client for SQL-input gs://
// loads. Tests replace this to point at a fake endpoint; it is not a loader
// interface and does not change the sample-database client-reuse path.
var newSQLInputGCSClient = func(ctx context.Context) (*storage.Client, error) {
	return storage.NewClient(ctx)
}

func sqlInputMaxSize(opts *filesafety.FileSafetyOptions) int64 {
	if opts == nil || opts.MaxSize == 0 {
		return filesafety.DefaultMaxFileSize
	}
	return opts.MaxSize
}

func defaultSQLInputFileOptions() *filesafety.FileSafetyOptions {
	return &filesafety.FileSafetyOptions{AllowNonRegular: true}
}

// remoteSQLInputContext bounds a remote SQL fetch to 30 seconds, or the
// earlier caller deadline. When the caller is already tighter, the parent
// context is returned so its cancellation cause is preserved.
func remoteSQLInputContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= sqlInputRemoteTimeout {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, sqlInputRemoteTimeout)
}

func isWindowsDriveScheme(scheme string) bool {
	return len(scheme) == 1 && unicode.IsLetter(rune(scheme[0]))
}

func localPathFromFileURI(u *url.URL) (string, error) {
	host := strings.ToLower(u.Hostname())
	if host != "" && host != "localhost" {
		return "", fmt.Errorf("file URI is not a local file: unsupported authority %q", u.Host)
	}

	path := u.Path
	if path == "" && u.Opaque != "" {
		decoded, err := url.PathUnescape(u.Opaque)
		if err != nil {
			return "", fmt.Errorf("file URI has an invalid path: %w", err)
		}
		path = decoded
	}
	if path == "" {
		return "", fmt.Errorf("file URI has an empty path")
	}

	// RFC 8089 file:///C:/path becomes "/C:/path". On Windows the leading
	// slash is not part of the filesystem path.
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && unicode.IsLetter(rune(path[1])) && path[2] == ':' {
		path = path[1:]
	}
	return path, nil
}

// loadSQLInput loads one SQL script from a bare path or an explicit
// file/http/https/gs URI. Callers choose the size and non-regular-file policy;
// SOURCE and --file/--source use the 100 MiB AllowNonRegular SQL-input policy,
// not the 10 MiB sample-database loader. Errors never include fetched bytes.
func loadSQLInput(ctx context.Context, source string, opts *filesafety.FileSafetyOptions) ([]byte, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid SQL input URI %q: %w", source, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == "" || isWindowsDriveScheme(scheme) {
		return filesafety.SafeReadFile(source, opts)
	}

	switch scheme {
	case "file":
		path, err := localPathFromFileURI(u)
		if err != nil {
			return nil, err
		}
		return filesafety.SafeReadFile(path, opts)
	case "http", "https":
		ctx, cancel := remoteSQLInputContext(ctx)
		defer cancel()
		return loadFromHTTPWithLimit(ctx, source, sqlInputMaxSize(opts))
	case "gs":
		ctx, cancel := remoteSQLInputContext(ctx)
		defer cancel()
		return loadSQLInputFromGCS(ctx, source, sqlInputMaxSize(opts))
	default:
		return nil, fmt.Errorf("unsupported SQL input URI scheme %q", u.Scheme)
	}
}

func loadSQLInputFromGCS(ctx context.Context, uri string, maxSize int64) ([]byte, error) {
	client, err := newSQLInputGCSClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer func() { _ = client.Close() }()
	return loadFromGCSWithClientAndLimit(ctx, client, uri, maxSize)
}
