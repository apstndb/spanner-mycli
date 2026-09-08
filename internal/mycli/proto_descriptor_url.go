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
)

// protoDescriptorLooksLikeSource reports whether filename should be compiled as
// protobuf source. HTTP(S) classification uses the parsed URL path only, so a
// query or fragment cannot change source vs binary. The original string is
// still the compiler root and the HTTP request URL. Local names keep
// filepath.Ext semantics and are not parsed as URLs.
func protoDescriptorLooksLikeSource(filename string) (bool, error) {
	if httpOrHTTPSRe.MatchString(filename) {
		u, err := url.Parse(filename)
		if err != nil {
			return false, fmt.Errorf("invalid proto descriptor URL %q: %w", filename, err)
		}
		return path.Ext(u.Path) == ".proto", nil
	}
	return filepath.Ext(filename) == ".proto", nil
}
