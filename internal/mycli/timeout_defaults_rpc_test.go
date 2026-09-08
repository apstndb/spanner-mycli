// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const blockedPDMLBeforeTransport = "blocked PDML before transport"

func TestStatementTimeoutSDKDeadline(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, nil, nil)

	for _, tc := range []struct {
		name   string
		args   []string
		parent time.Duration
		want   time.Duration
	}{
		{name: "omitted", want: 24 * time.Hour},
		{name: "null", args: []string{"--set", "STATEMENT_TIMEOUT=NULL"}, want: 24 * time.Hour},
		{name: "explicit_30s", args: []string{"--timeout", "30s"}, want: 30 * time.Second},
		{name: "explicit_10m", args: []string{"--timeout", "10m"}, want: 10 * time.Minute},
		{name: "short_parent", args: []string{"--set", "STATEMENT_TIMEOUT=NULL"}, parent: 5 * time.Second, want: 5 * time.Second},
	} {
		for _, sql := range []string{
			"PARTITIONED UPDATE T SET V = 1 WHERE TRUE",
			"UPDATE T SET V = 1 WHERE TRUE",
			"DELETE FROM T WHERE TRUE",
		} {
			t.Run(tc.name+"/"+sql, func(t *testing.T) {
				var pdmlBegin atomic.Int32
				var sqlCalls atomic.Int32
				var remaining atomic.Int64
				intercept := func(ctx context.Context, method string, request, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
					switch req := request.(type) {
					case *sppb.BeginTransactionRequest:
						if req.Options.GetPartitionedDml() != nil {
							pdmlBegin.Add(1)
							if deadline, ok := ctx.Deadline(); ok {
								remaining.Store(int64(time.Until(deadline)))
							}
							return status.Error(codes.PermissionDenied, blockedPDMLBeforeTransport)
						}
					case *sppb.ExecuteSqlRequest, *sppb.ExecuteBatchDmlRequest, *sppb.CommitRequest:
						sqlCalls.Add(1)
						return status.Error(codes.PermissionDenied, "blocked unexpected write")
					}
					return invoke(ctx, method, request, reply, cc, opts...)
				}
				args := append(withRequiredFlags("--enable-partitioned-dml"), tc.args...)
				flags, err := parseTestFlags(args)
				if err != nil {
					t.Fatal(err)
				}
				vars, err := initializeSystemVariables(&flags.Spanner)
				if err != nil {
					t.Fatal(err)
				}
				vars.Connection = ConnectionVars{Project: clients.ProjectID, Instance: clients.InstanceID, Database: clients.DatabaseID}
				vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
				opts := append(clients.ClientOptions(), option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(intercept)))
				session, err := NewSession(t.Context(), vars, opts...)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(session.Close)
				stmt, err := BuildStatement(sql)
				if err != nil {
					t.Fatal(err)
				}
				ctx := t.Context()
				if tc.parent != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tc.parent)
					defer cancel()
				}
				_, err = session.ExecuteStatement(ctx, stmt)
				if err == nil || !strings.Contains(err.Error(), blockedPDMLBeforeTransport) || pdmlBegin.Load() != 1 || sqlCalls.Load() != 0 {
					t.Fatalf("wrong path: err=%v PDML=%d SQL=%d", err, pdmlBegin.Load(), sqlCalls.Load())
				}
				got := time.Duration(remaining.Load())
				if got > tc.want || got < tc.want-2*time.Second {
					t.Errorf("PDML deadline remaining=%s want approximately %s", got, tc.want)
				}
			})
		}
	}
}

func TestInsertDoesNotTakePartitionedDMLDeadlinePath(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, nil, nil)
	var pdmlBegin atomic.Int32
	intercept := func(ctx context.Context, method string, request, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if req, ok := request.(*sppb.BeginTransactionRequest); ok && req.Options.GetPartitionedDml() != nil {
			pdmlBegin.Add(1)
			return status.Error(codes.PermissionDenied, blockedPDMLBeforeTransport)
		}
		return invoke(ctx, method, request, reply, cc, opts...)
	}
	flags, err := parseTestFlags(withRequiredFlags("--enable-partitioned-dml"))
	if err != nil {
		t.Fatal(err)
	}
	vars, err := initializeSystemVariables(&flags.Spanner)
	if err != nil {
		t.Fatal(err)
	}
	vars.Connection = ConnectionVars{Project: clients.ProjectID, Instance: clients.InstanceID, Database: clients.DatabaseID}
	vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	opts := append(clients.ClientOptions(), option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(intercept)))
	session, err := NewSession(t.Context(), vars, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	stmt, err := BuildStatement("INSERT INTO T (Id) VALUES (1)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.ExecuteStatement(t.Context(), stmt)
	if err == nil {
		t.Fatal("INSERT succeeded against empty schema")
	}
	if strings.Contains(err.Error(), blockedPDMLBeforeTransport) || pdmlBegin.Load() != 0 {
		t.Fatalf("INSERT used PDML path: err=%v PDML=%d", err, pdmlBegin.Load())
	}
}
