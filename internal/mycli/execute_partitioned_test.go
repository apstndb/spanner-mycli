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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type partitionFanInServer struct {
	sppb.UnimplementedSpannerServer
	nPartitions int
	rowsPer     int
	errToken    string
	hang        bool
	started     chan struct{}
	startedOnce sync.Once
	execs       atomic.Int32
}

func (s *partitionFanInServer) markStarted() {
	if s.started == nil {
		return
	}
	s.startedOnce.Do(func() { close(s.started) })
}

func (s *partitionFanInServer) CreateSession(_ context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Database + "/sessions/fanin", Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *partitionFanInServer) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *partitionFanInServer) BeginTransaction(context.Context, *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	return &sppb.Transaction{Id: []byte("fanin"), ReadTimestamp: timestamppb.Now()}, nil
}

func (s *partitionFanInServer) PartitionQuery(context.Context, *sppb.PartitionQueryRequest) (*sppb.PartitionResponse, error) {
	parts := make([]*sppb.Partition, s.nPartitions)
	for i := range s.nPartitions {
		parts[i] = &sppb.Partition{PartitionToken: []byte(fmt.Sprintf("%d", i))}
	}
	return &sppb.PartitionResponse{Partitions: parts}, nil
}

func (s *partitionFanInServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	s.execs.Add(1)
	s.markStarted()
	if s.hang {
		<-stream.Context().Done()
		return stream.Context().Err()
	}
	token := string(r.PartitionToken)
	if s.errToken != "" && token == s.errToken {
		return status.Error(codes.Internal, "injected partition error")
	}
	md := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "value", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		}},
	}
	values := make([]*structpb.Value, s.rowsPer)
	for i := range s.rowsPer {
		values[i] = structpb.NewStringValue(fmt.Sprintf("%s-%d", token, i))
	}
	return stream.Send(&sppb.PartialResultSet{Metadata: md, Values: values})
}

func startPartitionFanIn(t *testing.T, server *partitionFanInServer) (*spanner.BatchReadOnlyTransaction, []*spanner.Partition) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, server)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///fanin",
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
	tx, err := client.BatchReadOnlyTransaction(t.Context(), spanner.StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Close)
	parts, err := tx.PartitionQuery(t.Context(), spanner.Statement{SQL: "SELECT value FROM audit"}, spanner.PartitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return tx, parts
}

func collectPartitionValues(t *testing.T, md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) ([]string, error) {
	t.Helper()
	if md == nil {
		return nil, errors.New("nil metadata")
	}
	var values []string
	for row, err := range rows {
		if err != nil {
			return values, err
		}
		var v string
		if err := row.Column(0, &v); err != nil {
			return values, err
		}
		values = append(values, v)
	}
	return values, nil
}

func runWithTimeout(t *testing.T, d time.Duration, fn func(context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), d)
	defer cancel()
	return fn(ctx)
}

func isCancelErr(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled || spanner.ErrCode(err) == codes.Canceled)
}

func TestRunPartitionedRowSeqMorePartitionsThanWorkers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		partitions  int
		parallelism int
		rowsPer     int
		wantRows    int
	}{
		{name: "two_partitions_one_worker", partitions: 2, parallelism: 1, rowsPer: 1, wantRows: 2},
		{name: "two_partitions_two_workers", partitions: 2, parallelism: 2, rowsPer: 1, wantRows: 2},
		{name: "five_partitions_one_worker_multi_row", partitions: 5, parallelism: 1, rowsPer: 3, wantRows: 15},
		{name: "empty_partitions", partitions: 3, parallelism: 1, rowsPer: 0, wantRows: 0},
		{name: "no_partitions", partitions: 0, parallelism: 1, rowsPer: 0, wantRows: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: tc.partitions, rowsPer: tc.rowsPer})
			if len(parts) != tc.partitions {
				t.Fatalf("got %d partitions, want %d", len(parts), tc.partitions)
			}
			err := runWithTimeout(t, 3*time.Second, func(ctx context.Context) error {
				return runPartitionedRowSeq(ctx, tx, parts, tc.parallelism, func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error {
					if md == nil {
						if tc.wantRows != 0 {
							return errors.New("nil metadata")
						}
						for _, err := range rows {
							if err != nil {
								return err
							}
						}
						return nil
					}
					got, err := collectPartitionValues(t, md, rows)
					if err != nil {
						return err
					}
					if len(got) != tc.wantRows {
						t.Errorf("got %d rows %v, want %d", len(got), got, tc.wantRows)
					}
					return nil
				})
			})
			if tc.wantRows == 0 {
				if err != nil {
					t.Fatalf("empty run: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("runPartitionedRowSeq: %v", err)
			}
		})
	}
}

func TestRunPartitionedRowSeqWorkerError(t *testing.T) {
	t.Parallel()
	tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 2, rowsPer: 1, errToken: "0"})
	err := runWithTimeout(t, 3*time.Second, func(ctx context.Context) error {
		return runPartitionedRowSeq(ctx, tx, parts, 1, func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error {
			_, err := collectPartitionValues(t, md, rows)
			return err
		})
	})
	if err == nil {
		t.Fatal("want partition error, got nil")
	}
}

func TestRunPartitionedRowSeqParentCancel(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 2, rowsPer: 1, hang: true, started: started})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- runPartitionedRowSeq(ctx, tx, parts, 1, func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error {
			for _, err := range rows {
				if err != nil {
					return err
				}
			}
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("workers did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !isCancelErr(err) {
			t.Fatalf("got %v, want cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parent cancel did not join producers")
	}
}

func TestRunPartitionedRowSeqEarlyConsumerStop(t *testing.T) {
	t.Parallel()
	tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 3, rowsPer: 20})
	err := runWithTimeout(t, 3*time.Second, func(ctx context.Context) error {
		return runPartitionedRowSeq(ctx, tx, parts, 1, func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error {
			if md == nil {
				return errors.New("nil metadata")
			}
			for row, err := range rows {
				if err != nil {
					return err
				}
				if row != nil {
					return errors.New("consumer stop")
				}
			}
			return nil
		})
	})
	if err == nil || err.Error() != "consumer stop" {
		t.Fatalf("got %v, want consumer stop", err)
	}
}

func TestRunPartitionedRowSeqCancelDuringSubmit(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 8, rowsPer: 1, hang: true, started: started})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- runPartitionedRowSeq(ctx, tx, parts, 1, func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error {
			for _, err := range rows {
				if err != nil {
					return err
				}
			}
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first worker did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !isCancelErr(err) {
			t.Fatalf("got %v, want cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel during submission did not join producers")
	}
}

func TestRunPartitionedRowSeqSkipsUnsubmittedAfterCancel(t *testing.T) {
	// Package-level submit hook; do not run in parallel with other fan-in tests.
	const n = 32
	var submitted atomic.Int32
	testOnPartitionSubmit = func() { submitted.Add(1) }
	t.Cleanup(func() { testOnPartitionSubmit = nil })

	started := make(chan struct{})
	server := &partitionFanInServer{nPartitions: n, rowsPer: 1, hang: true, started: started}
	tx, parts := startPartitionFanIn(t, server)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- runPartitionedRowSeq(ctx, tx, parts, 1, func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error {
			for _, err := range rows {
				if err != nil {
					return err
				}
			}
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first worker did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !isCancelErr(err) {
			t.Fatalf("got %v, want cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not join producers")
	}
	gotSubmit := int(submitted.Load())
	gotRPC := int(server.execs.Load())
	t.Logf("p.Go submissions=%d ExecuteStreamingSql=%d partitions=%d", gotSubmit, gotRPC, n)
	if gotSubmit > 2 {
		t.Fatalf("submitted %d partitions after cancel; later unsubmitted work should be skipped", gotSubmit)
	}
}

type failAfterWrites struct {
	after int
	n     int
}

func (w *failAfterWrites) Write(p []byte) (int, error) {
	if w.n >= w.after {
		return 0, errors.New("injected write failure")
	}
	w.n++
	return len(p), nil
}

func TestStreamPartitionedQueryWriterPath(t *testing.T) {
	t.Parallel()
	tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 2, rowsPer: 1})
	sysVars := newSystemVariablesWithDefaults()
	sysVars.Display.CLIFormat = enums.DisplayModeCSV
	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = runWithTimeout(t, 3*time.Second, func(ctx context.Context) error {
		result, handled, err := streamPartitionedQuery(ctx, &buf, tx, parts, 1, &sysVars, fc, format.DisplayValues)
		if err != nil {
			return err
		}
		if !handled {
			t.Fatal("CSV should stream")
		}
		if result.AffectedRows != 2 {
			t.Errorf("AffectedRows=%d, want 2", result.AffectedRows)
		}
		if !strings.Contains(buf.String(), "0-0") || !strings.Contains(buf.String(), "1-0") {
			t.Errorf("CSV output missing partition rows: %q", buf.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamPartitionedQueryJSONLAndSQLInsert(t *testing.T) {
	t.Parallel()
	for _, mode := range []enums.DisplayMode{enums.DisplayModeJSONL, enums.DisplayModeSQLInsert} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 2, rowsPer: 1})
			sysVars := newSystemVariablesWithDefaults()
			sysVars.Display.CLIFormat = mode
			sysVars.Display.SQLTableName = "Audit"
			fc, vfm, sysVarsPtr, err := prepareFormatConfig("SELECT value FROM Audit", &sysVars)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			err = runWithTimeout(t, 3*time.Second, func(ctx context.Context) error {
				result, handled, err := streamPartitionedQuery(ctx, &buf, tx, parts, 1, sysVarsPtr, fc, vfm)
				if err != nil {
					return err
				}
				if !handled {
					t.Fatalf("%s should stream", mode)
				}
				if result.AffectedRows != 2 {
					t.Errorf("AffectedRows=%d, want 2", result.AffectedRows)
				}
				out := buf.String()
				if !strings.Contains(out, "0-0") || !strings.Contains(out, "1-0") {
					t.Errorf("%s output missing partition rows: %q", mode, out)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStreamPartitionedQueryWriteFailure(t *testing.T) {
	t.Parallel()
	tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 2, rowsPer: 8})
	sysVars := newSystemVariablesWithDefaults()
	sysVars.Display.CLIFormat = enums.DisplayModeCSV
	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	err = runWithTimeout(t, 3*time.Second, func(ctx context.Context) error {
		_, _, err := streamPartitionedQuery(ctx, &failAfterWrites{after: 1}, tx, parts, 1, &sysVars, fc, format.DisplayValues)
		if err == nil {
			return errors.New("want write failure")
		}
		if !strings.Contains(err.Error(), "injected write failure") {
			return fmt.Errorf("got %v, want injected write failure", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBufferPartitionedQueryMorePartitionsThanWorkers(t *testing.T) {
	t.Parallel()
	tx, parts := startPartitionFanIn(t, &partitionFanInServer{nPartitions: 2, rowsPer: 1})
	err := runWithTimeout(t, 3*time.Second, func(ctx context.Context) error {
		result, err := bufferPartitionedQuery(ctx, tx, parts, 1, format.DisplayValues)
		if err != nil {
			return err
		}
		if result.AffectedRows != 2 {
			t.Errorf("AffectedRows=%d, want 2", result.AffectedRows)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var _ io.Writer = (*failAfterWrites)(nil)
