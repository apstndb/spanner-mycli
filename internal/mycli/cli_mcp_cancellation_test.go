// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"
	"errors"
	"io"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpAdmissionBlockingStatement struct {
	MarksDetachedCompatible
	entered chan struct{}
	release <-chan struct{}
}

func (s *mcpAdmissionBlockingStatement) Execute(ctx context.Context, _ *Session) (*Result, error) {
	close(s.entered)
	select {
	case <-s.release:
		return &Result{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestMCPQueuedAdmission(t *testing.T) {
	// Nonparallel: the synthetic first statement temporarily extends the real
	// parser table. Cleanup drains all SDK handlers before restoring that table.
	for _, cancelled := range []bool{true, false} {
		name := "live request stays serialized"
		if cancelled {
			name = "cancelled request leaves queue without executing"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			session := newDetachedTestSession(io.Discard)
			t.Cleanup(session.Close)
			cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}
			entered, release := make(chan struct{}), make(chan struct{})
			releaseFirst := sync.OnceFunc(func() { close(release) })
			UseStatementDefsForTest(t, append(MergedStatementDefs(), &StatementDef{
				Pattern: regexp.MustCompile(`^MCPWAIT$`),
				HandleGroups: func(map[string]string) (Statement, error) {
					return &mcpAdmissionBlockingStatement{entered: entered, release: release}, nil
				},
			}))

			const queuedSQL = "SET CLI_FORMAT = 'TABLE'"
			handler := executeStatementHandler(cli)
			secondEntered := make(chan context.Context, 1)
			secondDone := make(chan struct{})
			server := mcp.NewServer(&mcp.Implementation{Name: "admission-test", Version: "test"}, nil)
			mcp.AddTool(server, &mcp.Tool{Name: "execute_statement"}, func(ctx context.Context, req *mcp.CallToolRequest, args ExecuteStatementArgs) (*mcp.CallToolResult, any, error) {
				if args.Statement == queuedSQL {
					secondEntered <- ctx
					defer close(secondDone)
				}
				return handler(ctx, req, args)
			})
			ct, st := mcp.NewInMemoryTransports()
			ss, err := server.Connect(ctx, st, nil)
			if err != nil {
				t.Fatal(err)
			}
			var client *mcp.ClientSession
			var callers sync.WaitGroup
			t.Cleanup(func() {
				cancel()
				releaseFirst()
				if client != nil {
					_ = client.Close()
				}
				_ = ss.Close() // Waits for handlers before the parser table is restored.
				callers.Wait()
			})
			client, err = mcp.NewClient(&mcp.Implementation{Name: "admission-client", Version: "test"}, nil).Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			type callResult struct {
				result *mcp.CallToolResult
				err    error
			}
			call := func(callCtx context.Context, sql string) <-chan callResult {
				done := make(chan callResult, 1)
				callers.Go(func() {
					result, err := client.CallTool(callCtx, &mcp.CallToolParams{
						Name: "execute_statement", Arguments: map[string]any{"statement": sql},
					})
					done <- callResult{result, err}
				})
				return done
			}
			assertSuccess := func(done <-chan callResult) {
				t.Helper()
				select {
				case got := <-done:
					if got.err != nil || got.result == nil || got.result.IsError {
						t.Fatalf("live call failed: result=%+v, error=%v", got.result, got.err)
					}
				case <-ctx.Done():
					t.Fatal("live call did not finish")
				}
			}
			first := call(ctx, "MCPWAIT")
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("first handler did not enter the statement")
			}
			callCtx, cancelCall := context.WithCancel(ctx)
			t.Cleanup(cancelCall)
			second := call(callCtx, queuedSQL)
			var serverCtx context.Context
			select {
			case serverCtx = <-secondEntered:
			case <-ctx.Done():
				t.Fatal("second handler was not dispatched")
			}
			if cancelled {
				cancelCall()
				select {
				case <-serverCtx.Done():
				case <-ctx.Done():
					t.Fatal("cancellation was not delivered to the server")
				}
				select {
				case <-secondDone:
				case <-time.After(time.Second):
					t.Fatal("cancelled handler still waits for the first request")
				}
				select {
				case got := <-second:
					if !errors.Is(got.err, context.Canceled) {
						t.Fatalf("client error=%v, want context.Canceled", got.err)
					}
				case <-ctx.Done():
					t.Fatal("cancelled client did not return")
				}
			} else {
				select {
				case <-secondDone:
					t.Fatal("live queued request ran concurrently with the first request")
				case <-time.After(100 * time.Millisecond):
				}
			}
			releaseFirst()
			assertSuccess(first)
			if !cancelled {
				assertSuccess(second)
			}
			want := enums.DisplayModeCSV
			if !cancelled {
				want = enums.DisplayModeTable
			}
			if got := session.systemVariables.Display.CLIFormat; got != want {
				t.Fatalf("format=%v, want %v", got, want)
			}
			assertSuccess(call(ctx, "SET CLI_FORMAT = 'TSV'"))
			if got := session.systemVariables.Display.CLIFormat; got != enums.DisplayModeTSV {
				t.Fatalf("later live SET format=%v, want TSV", got)
			}
		})
	}
}

func TestMCPAdmissionAlreadyCancelled(t *testing.T) {
	t.Parallel()
	for _, deadline := range []bool{false, true} {
		name := "cancelled"
		if deadline {
			name = "deadline exceeded"
		}
		t.Run(name, func(t *testing.T) {
			session := newDetachedTestSession(io.Discard)
			t.Cleanup(session.Close)
			handler := executeStatementHandler(&Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables})
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if deadline {
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				defer cancel()
			}
			// With both cancellation and an idle slot ready, select may choose
			// either. Repeat to exercise the post-acquisition cancellation check.
			for range 128 {
				result, _, err := handler(ctx, nil, ExecuteStatementArgs{Statement: "SET CLI_FORMAT = 'TABLE'"})
				if got := session.systemVariables.Display.CLIFormat; got != enums.DisplayModeCSV {
					t.Fatalf("cancelled SET changed format to %v", got)
				}
				if err != nil || result == nil || !result.IsError {
					t.Fatalf("cancelled admission: result=%+v, error=%v", result, err)
				}
				if got, want := mcpResultText(t, result), "ERROR: "+ctx.Err().Error(); got != want {
					t.Fatalf("error text=%q, want %q", got, want)
				}
			}
			result, _, err := handler(t.Context(), nil, ExecuteStatementArgs{Statement: "SET CLI_FORMAT = 'TSV'"})
			if err != nil || result == nil || result.IsError || session.systemVariables.Display.CLIFormat != enums.DisplayModeTSV {
				t.Fatalf("later live SET failed: result=%+v, error=%v", result, err)
			}
		})
	}
}
