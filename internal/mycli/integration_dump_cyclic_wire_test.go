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
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// Alter one real emulator response after transport decoding, before the SDK
// constructs its rows. This exercises the production iterator/encoder/planner
// error boundary without a mutable bypass or row-replacement hook in product code.
type dumpWireFaultStream struct {
	grpc.ClientStream
	query, fault string
	active       bool
	columnCount  int
	valueOffset  int
	injected     *atomic.Bool
	writeCalls   *atomic.Int32
}

// The observed catalog queries put a newline after SELECT. This test guard
// checks the first word, not a literal "SELECT " prefix; it is not a general
// SQL read-only classifier.
func dumpWireSelect(sql string) bool {
	words := strings.Fields(sql)
	return len(words) > 0 && strings.EqualFold(words[0], "SELECT")
}

func (s *dumpWireFaultStream) SendMsg(message any) error {
	if request, ok := message.(*sppb.ExecuteSqlRequest); ok {
		if !dumpWireSelect(request.Sql) {
			s.writeCalls.Add(1)
			return status.Errorf(codes.PermissionDenied, "unexpected source SQL write during DUMP: %s", request.Sql)
		}
		s.active = request.Sql == s.query
	}
	return s.ClientStream.SendMsg(message)
}

func (s *dumpWireFaultStream) RecvMsg(message any) error {
	if err := s.ClientStream.RecvMsg(message); err != nil {
		return err
	}
	result, ok := message.(*sppb.PartialResultSet)
	if !ok || !s.active || s.fault == "none" {
		return nil
	}
	if fields := result.GetMetadata().GetRowType().GetFields(); len(fields) > 0 {
		if len(fields) != 3 || fields[2].Name != "V" {
			return status.Error(codes.Internal, "unexpected wire-fault fixture projection")
		}
		s.columnCount = len(fields)
		switch s.fault {
		case "unknown_type":
			fields[2].Type = &sppb.Type{Code: sppb.TypeCode(999)}
			s.injected.Store(true)
		case "malformed_proto":
			fields[2].Type = &sppb.Type{Code: sppb.TypeCode_PROTO, ProtoTypeFqn: "example.Message"}
		case "malformed_enum":
			fields[2].Type = &sppb.Type{Code: sppb.TypeCode_ENUM, ProtoTypeFqn: "example.Kind"}
		case "malformed_date":
			fields[2].Type = &sppb.Type{Code: sppb.TypeCode_DATE}
		}
	}
	for i := range result.Values {
		if s.columnCount == 0 {
			return status.Error(codes.Internal, "wire-fault values arrived without metadata")
		}
		if (s.valueOffset+i)%s.columnCount != 2 || s.injected.Load() {
			continue
		}
		switch s.fault {
		case "malformed_int", "malformed_enum":
			result.Values[i] = structpb.NewStringValue("not-an-integer")
		case "malformed_proto":
			result.Values[i] = structpb.NewStringValue("*")
		case "malformed_date":
			result.Values[i] = structpb.NewStringValue("not-a-date")
		default:
			continue
		}
		s.injected.Store(true)
	}
	s.valueOffset += len(result.Values)
	return nil
}

func TestDumpCyclicMutateWireFailureBeforeOutput(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	clients, source := initializeWithRandomDB(t, twoSelfCyclesDDL(), nil)
	replayDumpSQL(t, source, twoSelfCyclesSeed)
	for _, table := range []string{"A", "Z"} {
		for _, fault := range []string{"none", "unknown_type", "malformed_int", "malformed_proto", "malformed_enum", "malformed_date"} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/streaming_%v", table, fault, streaming), func(t *testing.T) {
					var injected atomic.Bool
					var writeCalls atomic.Int32
					streamInterceptor := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
						if method == "/google.spanner.v1.Spanner/BatchWrite" {
							writeCalls.Add(1)
							return nil, status.Error(codes.PermissionDenied, "unexpected source BatchWrite during DUMP")
						}
						stream, err := streamer(ctx, desc, cc, method, opts...)
						if err != nil {
							return nil, err
						}
						return &dumpWireFaultStream{
							ClientStream: stream, query: "SELECT `Id`, `Ref`, `V` FROM `" + table + "`", fault: fault,
							injected: &injected, writeCalls: &writeCalls,
						}, nil
					}
					unaryInterceptor := func(ctx context.Context, method string, request, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
						write := false
						switch r := request.(type) {
						case *sppb.CommitRequest, *sppb.ExecuteBatchDmlRequest:
							write = true
						case *sppb.BeginTransactionRequest:
							write = r.Options.GetReadOnly() == nil
						case *sppb.ExecuteSqlRequest:
							write = !dumpWireSelect(r.Sql)
						}
						if write {
							writeCalls.Add(1)
							return status.Errorf(codes.PermissionDenied, "unexpected source write RPC during DUMP: %s %v", method, request)
						}
						return invoke(ctx, method, request, reply, cc, opts...)
					}
					opts := append(clients.ClientOptions(), option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(streamInterceptor)), option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(unaryInterceptor)))
					vars := newSystemVariablesWithDefaultsForTest()
					vars.Connection = ConnectionVars{Project: clients.ProjectID, Instance: clients.InstanceID, Database: clients.DatabaseID}
					vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
					session, err := NewSession(t.Context(), vars, opts...)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(session.Close)
					enableDumpCyclicMutations(t, session)
					var visited []string
					outputSeen := false
					session.dumpReadTxnProbe = func(phase string, _ *spanner.ReadOnlyTransaction) {
						outputSeen = outputSeen || phase == "output"
					}
					session.dumpCyclePreflightProbe = func(id tableID, _ *spanner.ReadOnlyTransaction) error {
						visited = append(visited, id.Name)
						return nil
					}
					var out strings.Builder
					var result *Result
					var opOut OperationOutput
					if streaming {
						opOut = OperationOutput{w: &out}
					}
					result, err = (&DumpDatabaseStatement{}).Execute(t.Context(), session, opOut)
					if writeCalls.Load() != 0 {
						t.Fatalf("DUMP attempted %d source write RPCs: %v", writeCalls.Load(), err)
					}
					if fault == "none" {
						if err != nil || result == nil || !outputSeen || injected.Load() {
							t.Fatalf("unmodified transport control failed: %v", err)
						}
						return
					}
					if !injected.Load() || err == nil || !strings.Contains(err.Error(), "encode cyclic DUMP table "+table) || !strings.Contains(err.Error(), "column V:") {
						t.Fatalf("did not reach intended row encoder failure: injected=%v error=%v", injected.Load(), err)
					}
					if result != nil || outputSeen || out.Len() != 0 {
						t.Fatalf("wire failure emitted partial SQL: result=%v output=%v text=%s", result, outputSeen, out.String())
					}
					wantVisited := table
					if table == "Z" {
						wantVisited = "A,Z"
					}
					if strings.Join(visited, ",") != wantVisited {
						t.Fatalf("wrong preflight order: %v", visited)
					}
				})
			}
		}
	}
}
