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
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// protoDescriptorLooksLikeSource reports whether filename should be compiled as
// protobuf source. Explicit URI schemes are classified before url.Parse so
// bare names that contain % and Windows drive paths stay filesystem paths.
// file://, gs://, and HTTP(S) use the decoded URL path extension, so a query
// or fragment cannot change source vs binary. Local non-URI names keep
// filepath.Ext semantics. Unknown schemes are rejected instead of local-open.
func protoDescriptorLooksLikeSource(filename string) (bool, error) {
	switch strings.ToLower(explicitSQLInputScheme(filename)) {
	case "":
		return filepath.Ext(filename) == ".proto", nil
	case "http", "https", "file", "gs":
		u, err := url.Parse(filename)
		if err != nil {
			return false, fmt.Errorf("invalid proto descriptor URL %q: %w", filename, err)
		}
		return path.Ext(u.Path) == ".proto", nil
	default:
		return false, fmt.Errorf("unsupported proto descriptor URI scheme %q", explicitSQLInputScheme(filename))
	}
}
