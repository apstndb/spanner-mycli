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
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// PullRemoteProtoStatement copies selected remote PROTO/ENUM files and their
// import closure into the local descriptor store. It never submits remote DDL.
type PullRemoteProtoStatement struct {
	All   bool
	Names []string
}

func (s *PullRemoteProtoStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	if session.batch.IsActive() {
		return nil, errPullRemoteProtoBatchActive
	}

	resp, err := session.GetDatabaseDdlFresh(ctx)
	if err != nil {
		return nil, fmt.Errorf("PULL REMOTE PROTO: get database DDL: %w", err)
	}

	var remote descriptorpb.FileDescriptorSet
	if raw := resp.GetProtoDescriptors(); len(raw) > 0 {
		if err := proto.Unmarshal(raw, &remote); err != nil {
			return nil, fmt.Errorf("PULL REMOTE PROTO: decode remote descriptors: %w", err)
		}
	}

	var local *descriptorpb.FileDescriptorSet
	if session.systemVariables != nil {
		local = session.systemVariables.Internal.ProtoDescriptor
	}
	planned, err := planPullRemoteProto(local, &remote, s.All, s.Names)
	if err != nil {
		return nil, err
	}

	// Result rows are already computed. Stream/buffer them before touching
	// local descriptor fields so a CSV/JSONL write failure cannot leave a
	// failed PULL applied.
	result, err := executeStructRows(localProtoRowEncoder, planned.selected, session, out)
	if err != nil {
		return nil, err
	}

	if planned.changed && session.systemVariables != nil {
		if planned.candidate == nil || len(planned.candidate.GetFile()) == 0 {
			session.systemVariables.Internal.ProtoDescriptor = nil
		} else {
			session.systemVariables.Internal.ProtoDescriptor = proto.Clone(planned.candidate).(*descriptorpb.FileDescriptorSet)
		}
		session.systemVariables.Internal.ProtoDescriptorFile = nil
	}

	result.KeepVariables = true
	return result, nil
}

var errPullRemoteProtoBatchActive = fmt.Errorf("PULL REMOTE PROTO cannot run while a batch is active")

type pullRemoteProtoPlan struct {
	candidate *descriptorpb.FileDescriptorSet
	selected  []*descriptorInfo
	changed   bool
}

type protoTypeLoc struct {
	file    string
	message proto.Message
}

func planPullRemoteProto(local, remote *descriptorpb.FileDescriptorSet, all bool, names []string) (*pullRemoteProtoPlan, error) {
	remoteFiles := indexDescriptorFiles(remote)
	localFiles := indexDescriptorFiles(local)
	remoteTypes := indexDescriptorTypes(remote)

	selectedNames, err := resolvePullRemoteProtoNames(remote, remoteTypes, all, names)
	if err != nil {
		return nil, err
	}
	if len(selectedNames) == 0 {
		return &pullRemoteProtoPlan{candidate: local, changed: false}, nil
	}

	selectedFiles, err := containingFilesForTypes(selectedNames, remoteTypes)
	if err != nil {
		return nil, err
	}
	included, err := collectPullImportClosure(selectedFiles, remoteFiles, localFiles)
	if err != nil {
		return nil, err
	}

	incoming := &descriptorpb.FileDescriptorSet{File: included}
	if err := rejectTypeFileIdentityConflicts(local, incoming); err != nil {
		return nil, err
	}

	candidate := mergeFDS(local, incoming)
	if err := rejectPullDeletionOrDowngrade(local, candidate); err != nil {
		return nil, err
	}
	if len(candidate.GetFile()) > 0 {
		if _, err := protodesc.NewFiles(candidate); err != nil {
			return nil, fmt.Errorf("PULL REMOTE PROTO: invalid proto descriptor set: %w", err)
		}
	}

	selected := selectedDescriptorInfos(candidate, selectedNames)
	return &pullRemoteProtoPlan{
		candidate: candidate,
		selected:  selected,
		changed:   !fdsGraphEqual(local, candidate),
	}, nil
}

func resolvePullRemoteProtoNames(remote *descriptorpb.FileDescriptorSet, remoteTypes map[string][]protoTypeLoc, all bool, names []string) ([]string, error) {
	if all {
		var selected []string
		seen := make(map[string]struct{})
		for info := range fdsToInfoSeq(remote) {
			if _, ok := seen[info.FullName]; ok {
				continue
			}
			locs := remoteTypes[info.FullName]
			if !hasSelectableProtoType(locs) {
				continue
			}
			if err := rejectAmbiguousType(info.FullName, locs); err != nil {
				return nil, err
			}
			seen[info.FullName] = struct{}{}
			selected = append(selected, info.FullName)
		}
		return selected, nil
	}

	selected := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		locs := remoteTypes[name]
		if len(locs) == 0 {
			return nil, fmt.Errorf("PULL REMOTE PROTO: unknown type %q", name)
		}
		if err := rejectAmbiguousType(name, locs); err != nil {
			return nil, err
		}
		switch msg := locs[0].message.(type) {
		case *descriptorpb.EnumDescriptorProto:
		case *descriptorpb.DescriptorProto:
			if hasPlaceholderDescriptorProto(msg) {
				return nil, fmt.Errorf("PULL REMOTE PROTO: %q is a placeholder descriptor", name)
			}
			if msg.GetOptions().GetMapEntry() {
				return nil, fmt.Errorf("PULL REMOTE PROTO: %q is a synthetic map entry", name)
			}
		default:
			return nil, fmt.Errorf("PULL REMOTE PROTO: %q is not a message or enum", name)
		}
		selected = append(selected, name)
	}
	return selected, nil
}

func containingFilesForTypes(names []string, types map[string][]protoTypeLoc) ([]string, error) {
	var files []string
	seen := make(map[string]struct{})
	for _, name := range names {
		locs := types[name]
		if err := rejectAmbiguousType(name, locs); err != nil {
			return nil, err
		}
		if len(locs) == 0 {
			return nil, fmt.Errorf("PULL REMOTE PROTO: unknown type %q", name)
		}
		file := locs[0].file
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		files = append(files, file)
	}
	return files, nil
}

func collectPullImportClosure(roots []string, remoteFiles, localFiles map[string][]*descriptorpb.FileDescriptorProto) ([]*descriptorpb.FileDescriptorProto, error) {
	seen := make(map[string]struct{})
	var out []*descriptorpb.FileDescriptorProto
	var walk func(name, from string) error
	walk = func(name, from string) error {
		if _, ok := seen[name]; ok {
			return nil
		}
		seen[name] = struct{}{}
		fdp, err := choosePullFile(name, from, remoteFiles, localFiles)
		if err != nil {
			return err
		}
		out = append(out, fdp)
		for _, dep := range fdp.GetDependency() {
			if err := walk(dep, name); err != nil {
				return err
			}
		}
		return nil
	}
	for _, name := range roots {
		if err := walk(name, ""); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func choosePullFile(name, from string, remoteFiles, localFiles map[string][]*descriptorpb.FileDescriptorProto) (*descriptorpb.FileDescriptorProto, error) {
	if files := remoteFiles[name]; len(files) > 0 {
		if err := rejectAmbiguousFile(name, files); err != nil {
			return nil, err
		}
		return files[0], nil
	}
	if files := localFiles[name]; len(files) > 0 {
		if err := rejectAmbiguousFile(name, files); err != nil {
			return nil, err
		}
		return files[0], nil
	}
	if from == "" {
		return nil, fmt.Errorf("PULL REMOTE PROTO: missing file %q", name)
	}
	return nil, fmt.Errorf("PULL REMOTE PROTO: missing import %q required by %q", name, from)
}

func rejectTypeFileIdentityConflicts(local, incoming *descriptorpb.FileDescriptorSet) error {
	localTypes := indexDescriptorTypes(local)
	incomingTypes := indexDescriptorTypes(incoming)
	for name, incomingLocs := range incomingTypes {
		localLocs := localTypes[name]
		if len(localLocs) == 0 {
			continue
		}
		for _, localLoc := range localLocs {
			for _, incomingLoc := range incomingLocs {
				if localLoc.file == incomingLoc.file {
					continue
				}
				return fmt.Errorf("PULL REMOTE PROTO: type %q exists in local file %q and remote file %q; PULL does not rename files", name, localLoc.file, incomingLoc.file)
			}
		}
	}
	return nil
}

func rejectPullDeletionOrDowngrade(local, candidate *descriptorpb.FileDescriptorSet) error {
	localTypes := indexDescriptorTypes(local)
	candidateTypes := indexDescriptorTypes(candidate)
	for name, locs := range localTypes {
		for _, loc := range locs {
			if !selectableProtoDescriptor(loc.message) {
				continue
			}
			found := candidateTypes[name]
			if len(found) == 0 {
				return fmt.Errorf("PULL REMOTE PROTO: would remove complete type %q from file %q; PULL does not delete local definitions", name, loc.file)
			}
			if !selectableProtoDescriptor(found[0].message) {
				return fmt.Errorf("PULL REMOTE PROTO: would downgrade complete type %q in file %q to a placeholder; PULL does not delete local definitions", name, loc.file)
			}
		}
	}
	return nil
}

func selectedDescriptorInfos(fds *descriptorpb.FileDescriptorSet, names []string) []*descriptorInfo {
	byName := make(map[string]*descriptorInfo)
	for info := range fdsToInfoSeq(fds) {
		if _, ok := byName[info.FullName]; !ok {
			byName[info.FullName] = info
		}
	}
	selected := make([]*descriptorInfo, 0, len(names))
	for _, name := range names {
		if info, ok := byName[name]; ok {
			selected = append(selected, info)
		}
	}
	return selected
}

func indexDescriptorFiles(fds *descriptorpb.FileDescriptorSet) map[string][]*descriptorpb.FileDescriptorProto {
	out := make(map[string][]*descriptorpb.FileDescriptorProto)
	for _, fdp := range fds.GetFile() {
		out[fdp.GetName()] = append(out[fdp.GetName()], fdp)
	}
	return out
}

func indexDescriptorTypes(fds *descriptorpb.FileDescriptorSet) map[string][]protoTypeLoc {
	out := make(map[string][]protoTypeLoc)
	for _, fdp := range fds.GetFile() {
		for name, message := range fdpToSeq(fdp) {
			switch message.(type) {
			case *descriptorpb.DescriptorProto, *descriptorpb.EnumDescriptorProto:
				out[name] = append(out[name], protoTypeLoc{file: fdp.GetName(), message: message})
			}
		}
	}
	return out
}

func rejectAmbiguousType(name string, locs []protoTypeLoc) error {
	if len(locs) <= 1 {
		return nil
	}
	files := make([]string, 0, len(locs))
	seen := make(map[string]struct{})
	for _, loc := range locs {
		if _, ok := seen[loc.file]; ok {
			continue
		}
		seen[loc.file] = struct{}{}
		files = append(files, loc.file)
	}
	if len(files) <= 1 {
		return nil
	}
	return fmt.Errorf("PULL REMOTE PROTO: type %q is declared in multiple files: %s", name, quoteJoin(files))
}

func rejectAmbiguousFile(name string, files []*descriptorpb.FileDescriptorProto) error {
	if len(files) <= 1 {
		return nil
	}
	return fmt.Errorf("PULL REMOTE PROTO: file %q is declared more than once", name)
}

func hasSelectableProtoType(locs []protoTypeLoc) bool {
	for _, loc := range locs {
		if selectableProtoDescriptor(loc.message) {
			return true
		}
	}
	return false
}

func fdsGraphEqual(left, right *descriptorpb.FileDescriptorSet) bool {
	if len(left.GetFile()) == 0 && len(right.GetFile()) == 0 {
		return true
	}
	return proto.Equal(left, right)
}

func quoteJoin(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return strings.Join(quoted, " and ")
}
