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
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/pelletier/go-toml"
)

// underscoreCompatibleTOMLLoader keeps kong-toml's hyphenated key model while
// also accepting snake_case aliases for user-facing configuration keys.
func underscoreCompatibleTOMLLoader(r io.Reader) (kong.Resolver, error) {
	tree, err := toml.LoadReader(r)
	if err != nil {
		return nil, err
	}
	normalizedTree, err := normalizeTOMLValue(tree.ToMap())
	if err != nil {
		return nil, err
	}

	var filename string
	if named, ok := r.(interface{ Name() string }); ok {
		filename = named.Name()
	}
	return &underscoreCompatibleTOMLResolver{
		filename: filename,
		tree:     normalizedTree.(map[string]any),
	}, nil
}

// underscoreCompatibleTOMLResolver is a lightly adapted copy of kong-toml's
// resolver so validation/lookup operate on the normalized alias map.
type underscoreCompatibleTOMLResolver struct {
	filename string
	tree     map[string]any
}

func (r *underscoreCompatibleTOMLResolver) Resolve(kctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
	value, ok := r.findValue(parent, flag)
	if !ok {
		return nil, nil
	}
	return value, nil
}

func (r *underscoreCompatibleTOMLResolver) Validate(app *kong.Application) error {
	configKeys := map[string]bool{}
	flattenNormalizedTOMLTree("", r.tree, configKeys)
	_ = kong.Visit(app, func(node kong.Visitable, next kong.Next) error {
		if flag, ok := node.(*kong.Flag); ok {
			delete(configKeys, flag.Name)
		}
		return next(nil)
	})
	if len(configKeys) > 0 {
		keys := slices.Collect(maps.Keys(configKeys))
		slices.Sort(keys)
		return fmt.Errorf("%s: unknown configuration keys: %s", r.filename, strings.Join(keys, ", "))
	}
	return nil
}

func (r *underscoreCompatibleTOMLResolver) findValue(parent *kong.Path, flag *kong.Flag) (any, bool) {
	keys := []string{
		strings.Join(append(strings.Split(parent.Node().Path(), "-"), flag.Name), "-"),
		flag.Name,
	}
	return r.findValueFromKeys(keys)
}

func (r *underscoreCompatibleTOMLResolver) findValueFromKeys(keys []string) (any, bool) {
	for _, key := range keys {
		parts := strings.Split(key, "-")
		if value, ok := r.findValueParts(parts[0], parts[1:], r.tree); ok {
			return value, ok
		}
	}
	return nil, false
}

func (r *underscoreCompatibleTOMLResolver) findValueParts(prefix string, suffix []string, tree map[string]any) (any, bool) {
	if value, ok := tree[prefix]; ok {
		if len(suffix) == 0 {
			return value, true
		}
		if branch, ok := value.(map[string]any); ok {
			return r.findValueParts(suffix[0], suffix[1:], branch)
		}
	}
	if len(suffix) > 0 {
		return r.findValueParts(prefix+"-"+suffix[0], suffix[1:], tree)
	}
	return nil, false
}

func flattenNormalizedTOMLTree(prefix string, tree any, flags map[string]bool) {
	switch tree := tree.(type) {
	case map[string]any:
		for key, value := range tree {
			if prefix == "" {
				flattenNormalizedTOMLTree(key, value, flags)
			} else {
				flattenNormalizedTOMLTree(prefix+"-"+key, value, flags)
			}
		}
	default:
		flags[prefix] = true
	}
}

func normalizeTOMLValue(value any) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(value))
		rawKeys := map[string]string{}
		for rawKey, rawValue := range value {
			normalizedKey := strings.ReplaceAll(rawKey, "_", "-")
			if previous, exists := rawKeys[normalizedKey]; exists {
				return nil, fmt.Errorf("duplicate configuration keys after underscore normalization: %s, %s", previous, rawKey)
			}
			normalizedValue, err := normalizeTOMLValue(rawValue)
			if err != nil {
				return nil, err
			}
			rawKeys[normalizedKey] = rawKey
			normalized[normalizedKey] = normalizedValue
		}
		return normalized, nil
	case []any:
		normalized := make([]any, len(value))
		for i, entry := range value {
			normalizedEntry, err := normalizeTOMLValue(entry)
			if err != nil {
				return nil, err
			}
			normalized[i] = normalizedEntry
		}
		return normalized, nil
	default:
		return value, nil
	}
}
