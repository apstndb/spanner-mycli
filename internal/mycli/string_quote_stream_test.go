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
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spancodec"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
)

type stringQuoteStreamRow struct {
	NullCol    spanner.NullString `spanner:"null_col"`
	StringNULL string             `spanner:"string_null"`
	EmptyStr   string             `spanner:"empty_str"`
	CommaStr   string             `spanner:"comma_str"`
	Ordinary   string             `spanner:"ordinary"`
}

func mustStringQuoteStreamTyped(t *testing.T) (*sppb.ResultSetMetadata, []*spanner.Row) {
	t.Helper()
	enc := spancodec.MustNewRowEncoder[stringQuoteStreamRow]()
	md, err := enc.ResultSetMetadata()
	if err != nil {
		t.Fatalf("ResultSetMetadata: %v", err)
	}
	var rows []*spanner.Row
	for row, err := range enc.Rows([]stringQuoteStreamRow{{
		NullCol:    spanner.NullString{Valid: false},
		StringNULL: "NULL",
		EmptyStr:   "",
		CommaStr:   "a,b",
		Ordinary:   "abc",
	}}) {
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		rows = append(rows, row)
	}
	return md, rows
}

var stringQuoteStreamModes = []enums.DisplayMode{
	enums.DisplayModeTable,
	enums.DisplayModeTableComment,
	enums.DisplayModeTableDetailComment,
	enums.DisplayModeVertical,
}

type stringQuoteStreamRPCServer struct {
	queryCacheRPCServer
}

func (s *stringQuoteStreamRPCServer) quoteResultSet() *sppb.ResultSet {
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "null_col", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{Name: "string_null", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{Name: "empty_str", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{Name: "comma_str", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{Name: "ordinary", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			}},
			Transaction: &sppb.Transaction{Id: []byte("qcache-ro"), ReadTimestamp: queryCacheFixedReadTS},
		},
		Stats: &sppb.ResultSetStats{
			QueryPlan:  s.plan,
			QueryStats: s.stats,
		},
	}
}

func (s *stringQuoteStreamRPCServer) quoteValues() []*structpb.Value {
	return []*structpb.Value{
		structpb.NewNullValue(),
		structpb.NewStringValue("NULL"),
		structpb.NewStringValue(""),
		structpb.NewStringValue("a,b"),
		structpb.NewStringValue("abc"),
	}
}

func (s *stringQuoteStreamRPCServer) ExecuteSql(context.Context, *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	if s.execErr != nil {
		return nil, s.execErr
	}
	rs := s.quoteResultSet()
	rs.Rows = []*structpb.ListValue{{Values: s.quoteValues()}}
	return rs, nil
}

func (s *stringQuoteStreamRPCServer) ExecuteStreamingSql(_ *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	if s.execErr != nil {
		return s.execErr
	}
	rs := s.quoteResultSet()
	return stream.Send(&sppb.PartialResultSet{
		Metadata: rs.Metadata,
		Values:   s.quoteValues(),
		Stats:    rs.Stats,
	})
}

func newStringQuoteStreamRPCSession(t *testing.T) (*Session, *systemVariables) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, &stringQuoteStreamRPCServer{
		queryCacheRPCServer: queryCacheRPCServer{
			plan:  testQueryPlan(t),
			stats: mustNewStruct(map[string]any{"elapsed_time": "1 msec", "query": "string-quote"}),
		},
	})
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///string-quote",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := spanner.NewClientWithConfig(t.Context(), "projects/test/instances/test/databases/test",
		spanner.ClientConfig{DisableNativeMetrics: true}, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	live := newSystemVariablesWithDefaultsForTest()
	session := &Session{
		mode:            DatabaseConnected,
		client:          client,
		systemVariables: live,
		txn:             NewTransactionManager(client, live, spanner.ClientConfig{DisableNativeMetrics: true}),
	}
	live.inTransaction = session.txn.InTransaction
	return session, live
}

func joinedStreamingQuoteText(out string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "/*")
		line = strings.TrimSuffix(line, "*/")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "+") {
			continue
		}
		line = strings.TrimPrefix(line, "|")
		line = strings.TrimSuffix(line, "|")
		b.WriteString(strings.TrimSpace(line))
	}
	return b.String()
}

func assertStringQuoteAUTOContent(t *testing.T, mode enums.DisplayMode, got string) {
	t.Helper()
	haystack := got
	if mode != enums.DisplayModeVertical {
		haystack = joinedStreamingQuoteText(got)
	}
	for _, tok := range []string{strconv.Quote("NULL"), strconv.Quote(""), strconv.Quote("a,b"), "abc"} {
		if !strings.Contains(haystack, tok) {
			t.Fatalf("%s missing complete quoted content %q:\n%s", mode, tok, got)
		}
	}
	if mode != enums.DisplayModeVertical {
		return
	}
	for _, line := range []string{
		`string_null: "NULL"`,
		`empty_str: ""`,
		`comma_str: "a,b"`,
		"ordinary: abc",
		"null_col: NULL",
	} {
		if !strings.Contains(got, line) {
			t.Fatalf("VERTICAL missing %q:\n%s", line, got)
		}
	}
	if strings.Contains(got, `null_col: "NULL"`) {
		t.Fatalf("VERTICAL quoted SQL NULL:\n%s", got)
	}
}

func TestCLIStringQuoteModeStreamingProcessorQuotes(t *testing.T) {
	t.Parallel()
	md, rawRows := mustStringQuoteStreamTyped(t)
	for _, mode := range stringQuoteStreamModes {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := stringQuoteSysVars(mode, enums.StringQuoteModeAuto, enums.StyledModeFalse)
			render, err := prepareFormatConfig("SELECT * FROM Items", &sv, queryRenderingFrom(&sv))
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			proc, err := streamingProcessorForMode(render, &buf, 80)
			if err != nil {
				t.Fatal(err)
			}
			if proc == nil {
				t.Fatal("streamingProcessorForMode returned nil")
			}
			if _, ok := proc.(*TablePreviewProcessor); mode != enums.DisplayModeVertical && !ok {
				t.Fatalf("table mode processor %T, want *TablePreviewProcessor", proc)
			}
			if _, ok := proc.(*StreamingProcessor); mode == enums.DisplayModeVertical && !ok {
				t.Fatalf("VERTICAL processor %T, want *StreamingProcessor", proc)
			}
			if err := proc.Init(md, render.Formatter); err != nil {
				t.Fatal(err)
			}
			row, err := spannerRowToRow(render.Spanvalue, render.TypeStyles, render.NullStyle)(rawRows[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := proc.ProcessRow(row); err != nil {
				t.Fatal(err)
			}
			if err := proc.Finish(QueryStats{}, 1); err != nil {
				t.Fatal(err)
			}
			assertStringQuoteAUTOContent(t, mode, buf.String())
		})
	}
}

func TestCLIStringQuoteModeStreamingExecuteSQLQuotes(t *testing.T) {
	t.Parallel()
	for _, mode := range stringQuoteStreamModes {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			session, live := newStringQuoteStreamRPCSession(t)
			var buf bytes.Buffer
			live.Display.CLIFormat = mode
			live.Display.StringQuoteMode = enums.StringQuoteModeAuto
			live.Display.StyledOutput = enums.StyledModeFalse
			live.Query.StreamingMode = enums.StreamingModeTrue
			live.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &buf, io.Discard)
			result, err := executeSQL(t.Context(), session, "SELECT * FROM Items", OperationOutput{
				w:           &buf,
				screenWidth: func() int { return 80 },
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.alreadyDelivered() {
				t.Fatal("streaming AUTO quote path must alreadyDeliver")
			}
			assertStringQuoteAUTOContent(t, mode, buf.String())
		})
	}
}
