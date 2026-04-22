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
	var filename string
	if named, ok := r.(interface{ Name() string }); ok {
		filename = named.Name()
	}

	tree, err := toml.LoadReader(r)
	if err != nil {
		return nil, formatConfigError(filename, err)
	}
	return &underscoreCompatibleTOMLResolver{
		filename: filename,
		tree:     tree.ToMap(),
	}, nil
}

// underscoreCompatibleTOMLResolver is a lightly adapted copy of kong-toml's
// resolver so validation/lookup operate on the normalized alias map.
type underscoreCompatibleTOMLResolver struct {
	filename string
	tree     map[string]any
}

func (r *underscoreCompatibleTOMLResolver) Resolve(kctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
	value, ok, err := r.findValue(parent, flag)
	if err != nil {
		return nil, formatConfigError(r.filename, err)
	}
	if !ok {
		return nil, nil
	}
	return value, nil
}

func (r *underscoreCompatibleTOMLResolver) Validate(app *kong.Application) error {
	configKeys := map[string]bool{}
	flattenTOMLTree("", r.tree, configKeys)
	deleteMatchingNodeConfigKeys(configKeys, app.Node)
	if len(configKeys) > 0 {
		keys := slices.Collect(maps.Keys(configKeys))
		slices.Sort(keys)
		return formatConfigError(r.filename, fmt.Errorf("unknown configuration keys: %s", strings.Join(keys, ", ")))
	}
	return nil
}

func formatConfigError(filename string, err error) error {
	if filename == "" {
		return err
	}
	return fmt.Errorf("%s: %w", filename, err)
}

func (r *underscoreCompatibleTOMLResolver) findValue(parent *kong.Path, flag *kong.Flag) (any, bool, error) {
	keys := []string{
		strings.Join(append(strings.Split(parent.Node().Path(), "-"), flag.Name), "-"),
		flag.Name,
	}
	return r.findValueFromKeys(keys)
}

func (r *underscoreCompatibleTOMLResolver) findValueFromKeys(keys []string) (any, bool, error) {
	for _, key := range keys {
		parts := strings.Split(key, "-")
		value, ok, err := r.findValueParts(parts[0], parts[1:], r.tree)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return value, ok, nil
		}
	}
	return nil, false, nil
}

func (r *underscoreCompatibleTOMLResolver) findValueParts(prefix string, suffix []string, tree map[string]any) (any, bool, error) {
	value, ok, err := findAliasValue(tree, prefix)
	if err != nil {
		return nil, false, err
	}
	if ok {
		if len(suffix) == 0 {
			return value, true, nil
		}
		if branch, ok := value.(map[string]any); ok {
			return r.findValueParts(suffix[0], suffix[1:], branch)
		}
	}
	if len(suffix) > 0 {
		return r.findValueParts(prefix+"-"+suffix[0], suffix[1:], tree)
	}
	return nil, false, nil
}

func flattenTOMLTree(prefix string, tree any, flags map[string]bool) {
	switch tree := tree.(type) {
	case map[string]any:
		if prefix != "" && len(tree) == 0 {
			flags[prefix] = true
			return
		}
		for key, value := range tree {
			if prefix == "" {
				flattenTOMLTree(key, value, flags)
			} else {
				flattenTOMLTree(prefix+"-"+key, value, flags)
			}
		}
	default:
		flags[prefix] = true
	}
}

func deleteMatchingNodeConfigKeys(configKeys map[string]bool, node *kong.Node) {
	path := node.Path()
	for _, flag := range node.Flags {
		deleteMatchingConfigKeys(configKeys, flag, path)
	}
	for _, child := range node.Children {
		deleteMatchingNodeConfigKeys(configKeys, child)
	}
}

func deleteMatchingConfigKeys(configKeys map[string]bool, flag *kong.Flag, nodePath string) {
	prefixes := []string{flag.Name}
	if nodePath != "" {
		prefixes = append(prefixes, nodePath+"-"+flag.Name)
	}
	for _, prefix := range prefixes {
		for _, prefix := range []string{prefix, strings.ReplaceAll(prefix, "-", "_")} {
			delete(configKeys, prefix)
			if !flag.IsMap() {
				continue
			}
			for key := range configKeys {
				if strings.HasPrefix(key, prefix+"-") {
					delete(configKeys, key)
				}
			}
		}
	}
}

func findAliasValue(tree map[string]any, key string) (any, bool, error) {
	candidates := []string{key}
	if underscored := strings.ReplaceAll(key, "-", "_"); underscored != key {
		candidates = append(candidates, underscored)
	}

	var (
		foundKey   string
		foundValue any
	)
	for _, candidate := range candidates {
		value, ok := tree[candidate]
		if !ok {
			continue
		}
		if foundKey != "" {
			return nil, false, fmt.Errorf("duplicate configuration keys for %q: %s, %s", key, foundKey, candidate)
		}
		foundKey = candidate
		foundValue = value
	}
	if foundKey == "" {
		return nil, false, nil
	}
	return foundValue, true, nil
}
