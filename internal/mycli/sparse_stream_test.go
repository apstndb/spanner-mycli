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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
)

// sparseStreamServer exercises client transport/rendering, not queue service
// semantics. Tests explicitly release each response and EOF; no sleeps or queue
// parser support are needed to model a RECEIVE query waiting for more messages.
type sparseStreamServer struct {
	queryCacheRPCServer
	responses chan *sppb.PartialResultSet
	started   chan struct{}
	stopped   chan struct{}
}

func newSparseStreamServer() *sparseStreamServer {
	return &sparseStreamServer{
		responses: make(chan *sppb.PartialResultSet),
		started:   make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

func (s *sparseStreamServer) ExecuteStreamingSql(_ *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	close(s.started)
	defer close(s.stopped)
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case response, ok := <-s.responses:
			if !ok {
				return nil
			}
			if err := stream.Send(response); err != nil {
				return err
			}
		}
	}
}

func (s *sparseStreamServer) send(t *testing.T, response *sppb.PartialResultSet) {
	t.Helper()
	select {
	case s.responses <- response:
	case <-s.stopped:
		t.Fatal("RPC stopped before accepting response")
	case <-time.After(5 * time.Second):
		t.Fatal("watchdog: RPC did not accept response")
	}
}

// Resume tokens mark complete replayable boundaries. Without one, the SDK may
// buffer values until a later token or EOF even if the formatter streams rows.
func sparseStreamRow(value, token string) *sppb.PartialResultSet {
	return &sppb.PartialResultSet{
		Values:      []*structpb.Value{structpb.NewStringValue(value)},
		ResumeToken: []byte(token),
	}
}

func sparseStreamMetadata() *sppb.ResultSetMetadata {
	return &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "message", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		}},
		Transaction: &sppb.Transaction{Id: []byte("sparse-ro"), ReadTimestamp: queryCacheFixedReadTS},
	}
}

// observedStreamWriter permits concurrent assertions without racing bytes.Buffer.
// Notifications only wake the reader; the accumulated bytes are authoritative.
type observedStreamWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	changed chan struct{}
	failOn  string
}

func (w *observedStreamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failOn != "" && strings.Contains(string(p), w.failOn) {
		return 0, io.ErrClosedPipe
	}
	n, err := w.buf.Write(p)
	select {
	case w.changed <- struct{}{}:
	default:
	}
	return n, err
}

func (w *observedStreamWriter) waitFor(t *testing.T, text string) {
	t.Helper()
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	for {
		w.mu.Lock()
		got := w.buf.String()
		w.mu.Unlock()
		if strings.Contains(got, text) {
			return
		}
		select {
		case <-w.changed:
		case <-watchdog.C:
			t.Fatalf("watchdog: waiting for %q before EOF; output = %q", text, got)
		}
	}
}

func startSparseQuery(t *testing.T, mode enums.DisplayMode, preview int64, out io.Writer) (*sparseStreamServer, context.CancelFunc, <-chan error) {
	t.Helper()
	server := newSparseStreamServer()
	session, live := newBufconnQuerySession(t, server)
	live.Display.CLIFormat = mode
	live.Query.StreamingMode = enums.StreamingModeTrue
	live.Query.TablePreviewRows = preview
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_, err := executeSQL(ctx, session, "SELECT * FROM RECEIVE_Tasks(max_duration => '20m')", OperationOutput{w: out, screenWidth: func() int { return 80 }})
		done <- err
	}()
	// Always cancel before waiting, including when a pre-EOF assertion fails.
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			t.Error("watchdog: query did not stop during cleanup")
		}
	})
	select {
	case <-server.started:
	case <-time.After(5 * time.Second):
		t.Fatal("watchdog: RPC did not start")
	}
	return server, cancel, done
}

func TestSparseStreamRowsBeforeEOF(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode    enums.DisplayMode
		preview int64
	}{
		{enums.DisplayModeCSV, 50},
		{enums.DisplayModeJSONL, 50},
		{enums.DisplayModeTab, 50},
		{enums.DisplayModeVertical, 50},
		{enums.DisplayModeTable, 0},
		{enums.DisplayModeTable, 1},
	} {
		t.Run(tc.mode.String()+"/preview="+strconv.FormatInt(tc.preview, 10), func(t *testing.T) {
			t.Parallel()
			out := &observedStreamWriter{changed: make(chan struct{}, 1)}
			server, _, done := startSparseQuery(t, tc.mode, tc.preview, out)
			first := sparseStreamRow("first-message", "one")
			first.Metadata = sparseStreamMetadata()
			server.send(t, first)
			out.waitFor(t, "first-message")
			// A split value must become one row while the RPC remains open.
			server.send(t, &sppb.PartialResultSet{Values: []*structpb.Value{structpb.NewStringValue("second-")}, ChunkedValue: true})
			server.send(t, sparseStreamRow("message", "two"))
			out.waitFor(t, "second-message")
			select {
			case err := <-done:
				t.Fatalf("query finished before EOF: %v", err)
			default:
			}
			close(server.responses)
			if err := waitErr(t, done, 5*time.Second, "sparse stream EOF"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSparseStreamCancellation(t *testing.T) {
	t.Parallel()
	for _, afterRow := range []bool{false, true} {
		t.Run(strconv.FormatBool(afterRow), func(t *testing.T) {
			t.Parallel()
			out := &observedStreamWriter{changed: make(chan struct{}, 1)}
			server, cancel, done := startSparseQuery(t, enums.DisplayModeJSONL, 50, out)
			if afterRow {
				first := sparseStreamRow("first-message", "one")
				first.Metadata = sparseStreamMetadata()
				server.send(t, first)
				out.waitFor(t, "first-message")
			}
			cancel()
			if err := waitErr(t, done, 5*time.Second, "sparse stream cancellation"); spanner.ErrCode(err) != codes.Canceled {
				t.Fatalf("query error = %v, want Canceled", err)
			}
			select {
			case <-server.stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("watchdog: cancellation did not reach RPC")
			}
		})
	}
}

func TestSparseStreamWriterErrorStopsRPC(t *testing.T) {
	t.Parallel()
	out := &observedStreamWriter{changed: make(chan struct{}, 1), failOn: "first-message"}
	server, _, done := startSparseQuery(t, enums.DisplayModeJSONL, 50, out)
	first := sparseStreamRow("first-message", "one")
	first.Metadata = sparseStreamMetadata()
	server.send(t, first)
	if err := waitErr(t, done, 5*time.Second, "sparse stream writer failure"); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("query error = %v, want ErrClosedPipe", err)
	}
	select {
	case <-server.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("watchdog: writer failure left RPC running")
	}
}
