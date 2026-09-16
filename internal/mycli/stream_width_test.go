// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/protobuf/types/known/structpb"
)

const streamWidthFullValue = "abcdefghij"

func TestTablePreviewProcessorStreamingPreservesValue(t *testing.T) {
	t.Parallel()
	config := format.FormatConfig{PreviewRows: 50}
	md := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		}},
	}

	for _, mode := range []format.Mode{format.ModeTable, format.ModeTableComment, format.ModeTableDetailComment} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			formatter := format.NewTableStreamingFormatter(&buf, config, 80, 50, mode)
			proc := NewTablePreviewProcessor(formatter, 50)
			if err := proc.Init(md, config); err != nil {
				t.Fatal(err)
			}
			if err := proc.ProcessRow(toRow(streamWidthFullValue)); err != nil {
				t.Fatal(err)
			}
			if err := proc.Finish(QueryStats{}, 1); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			if !strings.Contains(got, streamWidthFullValue) {
				t.Fatalf("processor truncated %q", got)
			}
		})
	}
}

type streamWidthRPCServer struct {
	queryCacheRPCServer
	value string
}

func (s *streamWidthRPCServer) stringResultSet() *sppb.ResultSet {
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			}},
			Transaction: &sppb.Transaction{Id: []byte("qcache-ro"), ReadTimestamp: queryCacheFixedReadTS},
		},
		Stats: &sppb.ResultSetStats{
			QueryPlan:  s.plan,
			QueryStats: s.stats,
		},
	}
}

func (s *streamWidthRPCServer) ExecuteSql(context.Context, *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	if s.execErr != nil {
		return nil, s.execErr
	}
	rs := s.stringResultSet()
	rs.Rows = []*structpb.ListValue{{Values: []*structpb.Value{structpb.NewStringValue(s.value)}}}
	return rs, nil
}

func (s *streamWidthRPCServer) ExecuteStreamingSql(_ *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	if s.execErr != nil {
		return s.execErr
	}
	rs := s.stringResultSet()
	return stream.Send(&sppb.PartialResultSet{
		Metadata: rs.Metadata,
		Values:   []*structpb.Value{structpb.NewStringValue(s.value)},
		Stats:    rs.Stats,
	})
}

func newStreamWidthRPCSession(t *testing.T, value string) (*Session, *systemVariables) {
	t.Helper()
	return newBufconnQuerySession(t, &streamWidthRPCServer{
		queryCacheRPCServer: queryCacheRPCServer{
			plan:  testQueryPlan(t),
			stats: mustNewStruct(map[string]any{"elapsed_time": "1 msec", "query": "stream-width"}),
		},
		value: value,
	})
}

func TestExecuteSQLStreamingTablePreservesValue(t *testing.T) {
	t.Parallel()
	for _, mode := range []enums.DisplayMode{enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			session, live := newStreamWidthRPCSession(t, streamWidthFullValue)
			var buf bytes.Buffer
			live.Display.CLIFormat = mode
			live.Query.StreamingMode = enums.StreamingModeTrue
			live.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &buf, io.Discard)
			result, err := executeSQL(t.Context(), session, "SELECT id FROM Items", OperationOutput{
				w:           &buf,
				screenWidth: func() int { return 80 },
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.alreadyDelivered() {
				t.Fatal("TABLE streaming with a writer must alreadyDeliver")
			}
			got := buf.String()
			if !strings.Contains(got, streamWidthFullValue) {
				t.Fatalf("streamed output truncated: %q", got)
			}
		})
	}
}
