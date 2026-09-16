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
	"net"
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// Shared in-memory gRPC transport for fake Spanner and DatabaseAdmin servers.
//
// These helpers own only the listener, server, dialer, and cleanup order.
// Each test keeps its own fake server implementation, response fixtures, and
// Session wiring so that the scenario-specific parts stay next to the test.

// bufconnDialer serves the registered services on an in-memory listener and
// returns a gRPC context dialer for it. The server and listener are stopped
// via t.Cleanup, after any client connections registered later.
func bufconnDialer(t *testing.T, register func(*grpc.Server)) func(context.Context, string) (net.Conn, error) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	register(grpcServer)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		err := <-serveDone
		_ = listener.Close()
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	})
	// Keep Dial rather than DialContext: no migrated test covers cancelled
	// dials, and RecreateClient / USE / DETACH still dial the shared listener
	// after t.Context() is cancelled during cleanup.
	return func(context.Context, string) (net.Conn, error) { return listener.Dial() }
}

// bufconnClientOptions returns per-client dial options for tests that let
// NewSession, USE, DETACH, or RecreateClient build their own clients against
// the same in-memory server without closing the shared listener.
func bufconnClientOptions(t *testing.T, register func(*grpc.Server)) []option.ClientOption {
	t.Helper()
	return []option.ClientOption{
		option.WithoutAuthentication(),
		option.WithEndpoint("bufnet"),
		option.WithGRPCDialOption(grpc.WithContextDialer(bufconnDialer(t, register))),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
}

// dialBufconn returns a client connection to the in-memory server. The
// connection is closed via t.Cleanup before the server is stopped.
func dialBufconn(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(bufconnDialer(t, register)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// bufconnSpannerClientConfig is the client configuration every in-memory
// Spanner client and its TransactionManager use.
var bufconnSpannerClientConfig = spanner.ClientConfig{DisableNativeMetrics: true}

// newBufconnSpannerClient returns a Spanner client for the fake Spanner
// service, with native metrics disabled. Extra services (for example a
// DatabaseAdmin fake) may be registered through register.
func newBufconnSpannerClient(t *testing.T, database string, server sppb.SpannerServer, register ...func(*grpc.Server)) *spanner.Client {
	t.Helper()
	conn := dialBufconn(t, func(s *grpc.Server) {
		sppb.RegisterSpannerServer(s, server)
		for _, r := range register {
			r(s)
		}
	})
	client, err := spanner.NewClientWithConfig(t.Context(), database, bufconnSpannerClientConfig, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

// bufconnAdminServer is the pair of services a fake DatabaseAdmin needs to
// serve so that long-running operations can be polled.
type bufconnAdminServer interface {
	databasepb.DatabaseAdminServer
	longrunningpb.OperationsServer
}

// newBufconnAdminClient returns a DatabaseAdmin client for the fake admin
// service, registering both the admin and operations services.
func newBufconnAdminClient(t *testing.T, server bufconnAdminServer) *adminapi.DatabaseAdminClient {
	t.Helper()
	conn := dialBufconn(t, func(s *grpc.Server) {
		databasepb.RegisterDatabaseAdminServer(s, server)
		longrunningpb.RegisterOperationsServer(s, server)
	})
	adminClient, err := adminapi.NewDatabaseAdminClient(t.Context(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminClient.Close() })
	return adminClient
}

// bufconnTestIdentity is the connection identity used by admin-only sessions.
var bufconnTestIdentity = ConnectionVars{Project: "test", Instance: "test", Database: "test"}

// newBufconnAdminSession returns a Session whose only backend is the fake
// DatabaseAdmin service, with default system variables and a test identity.
func newBufconnAdminSession(t *testing.T, server bufconnAdminServer) *Session {
	t.Helper()
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Connection = bufconnTestIdentity
	return &Session{
		adminClient:     newBufconnAdminClient(t, server),
		systemVariables: sysVars,
		connection:      bufconnTestIdentity,
	}
}

// newBufconnQuerySession returns a DatabaseConnected Session backed by the
// fake Spanner service, with default system variables and a live
// TransactionManager, for statement-level query tests.
func newBufconnQuerySession(t *testing.T, server sppb.SpannerServer) (*Session, *systemVariables) {
	t.Helper()
	client := newBufconnSpannerClient(t, "projects/test/instances/test/databases/test", server)
	live := newSystemVariablesWithDefaultsForTest()
	session := &Session{
		mode:            DatabaseConnected,
		client:          client,
		systemVariables: live,
		txn:             NewTransactionManager(client, live, bufconnSpannerClientConfig),
	}
	live.inTransaction = session.txn.InTransaction
	return session, live
}
