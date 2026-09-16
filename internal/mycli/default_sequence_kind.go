// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"regexp"
	"slices"
	"strings"
	"time"

	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultSequenceKindVarName = "DEFAULT_SEQUENCE_KIND"
	defaultSequenceKindValue   = "bit_reversed_positive"
)

// Pinned go-sql-spanner@28b0df26 / java-spanner MissingDefaultSequenceKindException
// instructional sentence. Compiled once. Not a structured ErrorInfo reason.
var missingDefaultSequenceKindRe = regexp.MustCompile(
	`Please specify the sequence kind explicitly or set the database option\s+['\x60]?default_sequence_kind['\x60]?\s*\.`,
)

var defaultSequenceKindDatabaseIDRe = regexp.MustCompile(`^[a-z][-a-z0-9]*[a-z0-9]$`)

func DefaultSequenceKindVar(ptr *string) *VarHandler[string] {
	return &VarHandler[string]{
		ptr: ptr,
		format: func(s string) string {
			if s == "" {
				return "NULL"
			}
			return s
		},
		parse: parseDefaultSequenceKind,
		enumValues: []string{
			"NULL",
			"'" + defaultSequenceKindValue + "'",
		},
	}
}

func parseDefaultSequenceKind(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "NULL") {
		return "", nil
	}
	if trimmed == defaultSequenceKindValue {
		return defaultSequenceKindValue, nil
	}
	return "", fmt.Errorf("%s must be empty, NULL, or %s", defaultSequenceKindVarName, defaultSequenceKindValue)
}

func isMissingDefaultSequenceKindError(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) != codes.InvalidArgument {
		return false
	}
	return missingDefaultSequenceKindRe.MatchString(err.Error())
}

func shouldAttemptSequenceKindRepair(ctx context.Context, mode enums.DDLExecutionMode, kind string, err error) bool {
	if mode != enums.DDLExecutionModeSync || kind == "" || ctx.Err() != nil {
		return false
	}
	return isMissingDefaultSequenceKindError(err)
}

func isEmptyCommitTimestamp(ts *timestamppb.Timestamp) bool {
	return ts == nil || (ts.GetSeconds() == 0 && ts.GetNanos() == 0)
}

// provenSuccessfulPrefix returns the contiguous successful prefix length for
// an accepted terminal LRO. ok is false for missing, mismatched, holey, or
// otherwise inconsistent metadata. Empty timestamps on matching metadata are
// prefix 0. A trailing empty tail is unfinished. A success after a hole,
// invalid timestamp, extra timestamps, or a prefix covering every statement
// despite the failure is inconsistent.
func provenSuccessfulPrefix(database string, submitted []string, md *databasepb.UpdateDatabaseDdlMetadata) (int, bool) {
	if md == nil || database == "" || md.GetDatabase() != database {
		return 0, false
	}
	if len(md.GetStatements()) != len(submitted) {
		return 0, false
	}
	for i, stmt := range submitted {
		if md.GetStatements()[i] != stmt {
			return 0, false
		}
	}

	timestamps := md.GetCommitTimestamps()
	if len(timestamps) > len(submitted) {
		return 0, false
	}

	prefix := 0
	seenUnfinished := false
	for i, ts := range timestamps {
		if isEmptyCommitTimestamp(ts) {
			seenUnfinished = true
			continue
		}
		if err := ts.CheckValid(); err != nil {
			return 0, false
		}
		if seenUnfinished {
			return 0, false
		}
		prefix = i + 1
	}
	if prefix == len(submitted) {
		return 0, false
	}
	return prefix, true
}

func quoteGoogleSQLIdent(id string) string {
	return "`" + strings.ReplaceAll(id, "`", "``") + "`"
}

func quotePostgreSQLIdent(id string) string {
	return `"` + strings.ReplaceAll(id, `"`, `""`) + `"`
}

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func defaultSequenceKindAlterSQL(dialect databasepb.DatabaseDialect, databaseID, kind string) (string, error) {
	if !defaultSequenceKindDatabaseIDRe.MatchString(databaseID) {
		return "", fmt.Errorf("unsafe database id %q", databaseID)
	}
	if kind != defaultSequenceKindValue {
		return "", fmt.Errorf("unsafe DEFAULT_SEQUENCE_KIND %q", kind)
	}
	if dialect == databasepb.DatabaseDialect_POSTGRESQL {
		return "ALTER DATABASE " + quotePostgreSQLIdent(databaseID) + " SET spanner.default_sequence_kind = " + quoteSQLString(kind), nil
	}
	return "ALTER DATABASE " + quoteGoogleSQLIdent(databaseID) + " SET OPTIONS (default_sequence_kind = " + quoteSQLString(kind) + ")", nil
}

func wrapSequenceKindPhaseError(phase string, later, original error) error {
	return fmt.Errorf("DEFAULT_SEQUENCE_KIND %s failed: %w; original DDL error: %w", phase, later, original)
}

func echoExecutedDDLRows(ddls []string, timestamps []*timestamppb.Timestamp) []Row {
	n := min(len(ddls), len(timestamps))
	rows := make([]Row, 0, n)
	for i := range n {
		ts := timestamps[i]
		if isEmptyCommitTimestamp(ts) {
			continue
		}
		rows = append(rows, toRow(ddls[i]+";", ts.AsTime().Format(time.RFC3339Nano)))
	}
	return rows
}

func lastCommitTimestamp(timestamps []*timestamppb.Timestamp) (ts *timestamppb.Timestamp) {
	for _, timestamp := range slices.Backward(timestamps) {
		if !isEmptyCommitTimestamp(timestamp) {
			return timestamp
		}
	}
	return nil
}

func mergeEchoedDDLResult(prefixDDLs []string, prefixTS []*timestamppb.Timestamp, alterSQL string, alterTS []*timestamppb.Timestamp, suffixDDLs []string, suffixTS []*timestamppb.Timestamp, echo bool) *Result {
	allTS := append(append(append([]*timestamppb.Timestamp{}, prefixTS...), alterTS...), suffixTS...)
	result := &Result{}
	if last := lastCommitTimestamp(allTS); last != nil {
		result.CommitTimestamp = last.AsTime()
	}
	if !echo {
		return result
	}
	var rows []Row
	rows = append(rows, echoExecutedDDLRows(prefixDDLs, prefixTS)...)
	rows = append(rows, echoExecutedDDLRows([]string{alterSQL}, alterTS)...)
	rows = append(rows, echoExecutedDDLRows(suffixDDLs, suffixTS)...)
	result.TableHeader = toTableHeader("Executed", "Commit Timestamp")
	result.Body = PresentationBody(rows)
	return result
}

func tryDefaultSequenceKindRepair(ctx context.Context, session *Session, ddls []string, descriptors []byte, kind string, op *adminapi.UpdateDatabaseDdlOperation, origErr error) (*Result, error, bool) {
	mode := session.systemVariables.Feature.DDLExecutionMode
	if !shouldAttemptSequenceKindRepair(ctx, mode, kind, origErr) {
		return nil, nil, false
	}

	prefix := 0
	var originalMD *databasepb.UpdateDatabaseDdlMetadata
	if op == nil {
		// Create-RPC rejection: no accepted operation, prefix 0 is allowed.
	} else {
		if !op.Done() {
			return nil, nil, false
		}
		md, err := op.Metadata()
		if err != nil || md == nil {
			return nil, nil, false
		}
		var ok bool
		prefix, ok = provenSuccessfulPrefix(session.DatabasePath(), ddls, md)
		if !ok {
			return nil, nil, false
		}
		originalMD = md
	}

	alterSQL, err := defaultSequenceKindAlterSQL(session.systemVariables.Feature.DatabaseDialect, session.connection.Database, kind)
	if err != nil {
		return nil, wrapSequenceKindPhaseError("encoding", err, origErr), true
	}

	alterOp, err := session.adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   session.DatabasePath(),
		Statements: []string{alterSQL},
	})
	if err != nil {
		return nil, wrapSequenceKindPhaseError("ALTER DATABASE", err, origErr), true
	}
	alterResult, err := waitForDdlOperation(ctx, session, alterOp, []string{alterSQL}, nil, nil, func() {}, time.Time{})
	if err != nil {
		return nil, wrapSequenceKindPhaseError("ALTER DATABASE", err, origErr), true
	}

	suffix := ddls[prefix:]
	suffixOp, err := session.adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:         session.DatabasePath(),
		Statements:       suffix,
		ProtoDescriptors: descriptors,
	})
	if err != nil {
		return nil, wrapSequenceKindPhaseError("suffix retry", err, origErr), true
	}
	suffixResult, err := waitForDdlOperation(ctx, session, suffixOp, suffix, nil, nil, func() {}, time.Time{})
	if err != nil {
		return nil, wrapSequenceKindPhaseError("suffix retry", err, origErr), true
	}

	echo := session.systemVariables.Feature.EchoExecutedDDL
	var prefixDDLs []string
	var prefixTS []*timestamppb.Timestamp
	if originalMD != nil && prefix > 0 {
		prefixDDLs = ddls[:prefix]
		prefixTS = originalMD.GetCommitTimestamps()[:prefix]
	}
	alterTS := alterMetadataTimestamps(alterOp)
	suffixTS := suffixMetadataTimestamps(suffixOp)
	if echo {
		return mergeEchoedDDLResult(prefixDDLs, prefixTS, alterSQL, alterTS, suffix, suffixTS, true), nil, true
	}
	if suffixResult != nil {
		return suffixResult, nil, true
	}
	return alterResult, nil, true
}

func alterMetadataTimestamps(op *adminapi.UpdateDatabaseDdlOperation) []*timestamppb.Timestamp {
	if op == nil {
		return nil
	}
	md, err := op.Metadata()
	if err != nil || md == nil {
		return nil
	}
	return md.GetCommitTimestamps()
}

func suffixMetadataTimestamps(op *adminapi.UpdateDatabaseDdlOperation) []*timestamppb.Timestamp {
	return alterMetadataTimestamps(op)
}
