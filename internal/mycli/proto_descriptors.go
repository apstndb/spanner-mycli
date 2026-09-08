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
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

const protoDescriptorsVarName = "PROTO_DESCRIPTORS"

func encodeProtoDescriptors(fds *descriptorpb.FileDescriptorSet) (string, error) {
	if fds == nil {
		return "", nil
	}
	raw, err := proto.Marshal(fds)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func decodeProtoDescriptorBytes(value string) ([]byte, error) {
	if raw, err := base64.StdEncoding.DecodeString(value); err == nil {
		return raw, nil
	}
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid PROTO_DESCRIPTORS base64: %w", err)
	}
	return raw, nil
}

func parseProtoDescriptorsGraph(raw []byte) (*descriptorpb.FileDescriptorSet, error) {
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &fds); err != nil {
		return nil, fmt.Errorf("invalid PROTO_DESCRIPTORS protobuf: %w", err)
	}
	if _, err := protodesc.NewFiles(&fds); err != nil {
		return nil, fmt.Errorf("invalid proto descriptor set: %w", err)
	}
	return &fds, nil
}

func namedTypeFullName(n *ast.NamedType) string {
	parts := make([]string, len(n.Path))
	for i, ident := range n.Path {
		parts[i] = ident.Name
	}
	return strings.Join(parts, ".")
}

func looksLikeCreateOrAlterProtoBundle(statement string) bool {
	upper := strings.ToUpper(strings.TrimSpace(statement))
	return strings.HasPrefix(upper, "CREATE PROTO BUNDLE") || strings.HasPrefix(upper, "ALTER PROTO BUNDLE")
}

func createProtoBundleTypeNames(statements []string) ([]string, error) {
	var names []string
	for _, statement := range statements {
		if !looksLikeCreateOrAlterProtoBundle(statement) {
			continue
		}
		ddl, err := parseMemefishDDL("dump-proto-bundle", statement)
		if err != nil {
			return nil, fmt.Errorf("dump proto bundle DDL: %w", err)
		}
		switch stmt := ddl.(type) {
		case *ast.CreateProtoBundle:
			if stmt.Types == nil {
				continue
			}
			for _, named := range stmt.Types.Types {
				names = append(names, namedTypeFullName(named))
			}
		case *ast.AlterProtoBundle:
			if stmt.Insert != nil && stmt.Insert.Types != nil {
				for _, named := range stmt.Insert.Types.Types {
					names = append(names, namedTypeFullName(named))
				}
			}
			if stmt.Update != nil && stmt.Update.Types != nil {
				for _, named := range stmt.Update.Types.Types {
					names = append(names, namedTypeFullName(named))
				}
			}
		}
	}
	return names, nil
}

func protoDescriptorFullNames(fds *descriptorpb.FileDescriptorSet) map[string]struct{} {
	present := make(map[string]struct{})
	for info := range fdsToInfoSeq(fds) {
		present[info.FullName] = struct{}{}
	}
	return present
}

func dumpProtoDescriptorsPreamble(raw []byte, statements []string) ([]byte, error) {
	names, err := createProtoBundleTypeNames(statements)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		if len(names) > 0 {
			return nil, fmt.Errorf("dump requires proto descriptors for PROTO BUNDLE types: %s", strings.Join(names, ", "))
		}
		return nil, nil
	}
	fds, err := parseProtoDescriptorsGraph(raw)
	if err != nil {
		return nil, fmt.Errorf("dump proto descriptors: %w", err)
	}
	present := protoDescriptorFullNames(fds)
	for _, name := range names {
		if _, ok := present[name]; !ok {
			return nil, fmt.Errorf("dump proto descriptors missing PROTO BUNDLE type %s", name)
		}
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	return []byte("SET " + protoDescriptorsVarName + " = '" + encoded + "';\n\n"), nil
}

func renderDumpDDL(statements []string, protoDesc []byte) ([]byte, error) {
	replayDDL, err := prepareDumpDDLForReplay(statements)
	if err != nil {
		return nil, err
	}
	preamble, err := dumpProtoDescriptorsPreamble(protoDesc, replayDDL)
	if err != nil {
		return nil, err
	}
	return append(preamble, renderDDLStatements(replayDDL)...), nil
}
