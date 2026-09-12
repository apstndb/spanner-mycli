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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/google/go-cmp/cmp"
	"github.com/hymkor/go-multiline-ny"
	readline "github.com/nyaosorg/go-readline-ny"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFuzzyFinderCommandMetadata(t *testing.T) {
	t.Parallel()
	f := &fuzzyFinderCommand{}
	if got := f.String(); got != "FUZZY_FINDER" {
		t.Fatalf("String() = %q, want FUZZY_FINDER", got)
	}
	ed := &multiline.Editor{}
	f.SetEditor(ed)
	if f.editor != ed {
		t.Fatal("SetEditor did not store the editor")
	}
}

func TestCompletionHeader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ct   fuzzyCompletionType
		want string
	}{
		{0, "Statements"},
		{fuzzyCompleteDatabase, "Databases"},
		{fuzzyCompleteVariable, "System Variables"},
		{fuzzyCompleteTable, "Tables"},
		{fuzzyCompleteVariableValue, "Variable Values"},
		{fuzzyCompleteRole, "Database Roles"},
		{fuzzyCompleteOperation, "Operations"},
		{fuzzyCompleteView, "Views"},
		{fuzzyCompleteIndex, "Indexes"},
		{fuzzyCompleteChangeStream, "Change Streams"},
		{fuzzyCompleteSequence, "Sequences"},
		{fuzzyCompleteModel, "Models"},
		{fuzzyCompleteSchema, "Schemas"},
		{fuzzyCompleteParam, "Query Parameters"},
		{fuzzyCompleteSetTarget, "System Variables / PARAM"},
		{fuzzyCompletePlanNode, "Cached Plan Nodes"},
		{fuzzyCompletionType(99), "Statements"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.ct), func(t *testing.T) {
			if got := completionHeader(tt.ct); got != tt.want {
				t.Fatalf("completionHeader(%d) = %q, want %q", tt.ct, got, tt.want)
			}
		})
	}
}

func TestPrepareFzfOptions_EmptyLabelUsesValue(t *testing.T) {
	t.Parallel()
	prepared := prepareFzfOptions([]fzfItem{
		{Value: "plain"},
		{Value: "id", Label: "visible"},
	}, "Operations")
	if !prepared.hasLabels {
		t.Fatal("expected label/value separation")
	}
	want := []string{
		"plain" + fzfDelimiter + "plain",
		"id" + fzfDelimiter + "visible",
	}
	if diff := cmp.Diff(want, prepared.formattedLines); diff != "" {
		t.Fatalf("formattedLines mismatch (-want +got):\n%s", diff)
	}
}

func TestRunFzfRejectsInvalidExtraOptions(t *testing.T) {
	t.Parallel()
	candidates := []fzfItem{{Value: "alpha"}}
	got, ok := runFzf(candidates, "", "", "--header='unclosed")
	if ok || got != "" {
		t.Fatalf("unclosed quote: got (%q, %v), want empty/false", got, ok)
	}
	got, ok = runFzf(candidates, "alpha", "Statements", "--no-such-fzf-flag")
	if ok || got != "" {
		t.Fatalf("unknown flag: got (%q, %v), want empty/false", got, ok)
	}
}

func TestRunFzfFilterRejectsInvalidOptions(t *testing.T) {
	t.Parallel()
	if got := runFzfFilter([]fzfItem{{Value: "alpha"}}, "a", "", "--no-such-fzf-flag"); got != nil {
		t.Fatalf("unknown flag: got %v, want nil", got)
	}
}

func TestFetchLocalCandidates(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaults()
	sv.ensureRegistry()
	intLit := &ast.IntLiteral{Base: 10, Value: "1"}
	typ, err := memefish.ParseType("", "STRING")
	if err != nil {
		t.Fatal(err)
	}
	sv.Params = map[string]ast.Node{
		"limit": intLit,
		"name":  typ,
	}
	f := &fuzzyFinderCommand{cli: &Cli{SystemVariables: &sv}}

	names := f.fetchVariableCandidates()
	if len(names) == 0 || !containsString(names, "CLI_FORMAT") {
		t.Fatalf("variable names = %v, want CLI_FORMAT", names)
	}

	nilFinder := &fuzzyFinderCommand{cli: &Cli{}}
	if got := nilFinder.fetchVariableCandidates(); got != nil {
		t.Fatalf("nil SystemVariables: %v", got)
	}
	if got := nilFinder.fetchVariableValueCandidates("CLI_FORMAT"); got != nil {
		t.Fatalf("nil SystemVariables values: %v", got)
	}
	if got := nilFinder.fetchParamCandidates(); got != nil {
		t.Fatalf("nil SystemVariables params: %v", got)
	}

	if got := f.fetchVariableValueCandidates("not_a_variable"); got != nil {
		t.Fatalf("unknown variable: %v", got)
	}
	boolVals := f.fetchVariableValueCandidates("AUTO_PARTITION_MODE")
	if diff := cmp.Diff([]string{"TRUE", "FALSE"}, boolVals); diff != "" {
		t.Fatalf("AUTO_PARTITION_MODE values mismatch (-want +got):\n%s", diff)
	}
	queryMode := f.fetchVariableValueCandidates("CLI_QUERY_MODE")
	if !containsString(queryMode, "'PLAN'") {
		t.Fatalf("CLI_QUERY_MODE values = %v, want quoted PLAN", queryMode)
	}
	if got := f.fetchVariableValueCandidates("DIRECTED_READ"); got != nil {
		t.Fatalf("DIRECTED_READ (CustomVar with nil base) = %v, want nil", got)
	}

	params := f.fetchParamCandidates()
	wantParams := []fzfItem{
		{Value: "limit", Label: fmt.Sprintf("limit [%s] %s", paramKind(intLit), intLit.SQL())},
		{Value: "name", Label: fmt.Sprintf("name [%s] %s", paramKind(typ), typ.SQL())},
	}
	if diff := cmp.Diff(wantParams, params); diff != "" {
		t.Fatalf("params mismatch (-want +got):\n%s", diff)
	}
	empty := newSystemVariablesWithDefaults()
	emptyFinder := &fuzzyFinderCommand{cli: &Cli{SystemVariables: &empty}}
	if got := emptyFinder.fetchParamCandidates(); got != nil {
		t.Fatalf("empty params: %v", got)
	}

	got, err := f.fetchCandidates(t.Context(), 0, "")
	if err != nil || got != nil {
		t.Fatalf("unknown completion type: %v, %v", got, err)
	}
	items, err := f.resolveCandidates(t.Context(), 0, "")
	if err != nil || len(items) == 0 {
		t.Fatalf("statement names: n=%d err=%v", len(items), err)
	}
	local, err := f.resolveCandidates(t.Context(), fuzzyCompleteVariable, "")
	if err != nil || len(local) == 0 || local[0].Value != names[0] {
		t.Fatalf("variable resolve: %v err=%v", local, err)
	}
	values, err := f.resolveCandidates(t.Context(), fuzzyCompleteVariableValue, "AUTO_PARTITION_MODE")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(toFzfItems(boolVals), values); diff != "" {
		t.Fatalf("value resolve mismatch (-want +got):\n%s", diff)
	}
	setTarget, err := f.resolveCandidates(t.Context(), fuzzyCompleteSetTarget, "")
	if err != nil || len(setTarget) < 3 || setTarget[0].Value != "PARAM" {
		t.Fatalf("set target: %v err=%v", setTarget, err)
	}
	resolvedParams, err := f.resolveCandidates(t.Context(), fuzzyCompleteParam, "")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantParams, resolvedParams); diff != "" {
		t.Fatalf("param resolve mismatch (-want +got):\n%s", diff)
	}
}

func TestFetchCandidatesNilSession(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaults()
	f := &fuzzyFinderCommand{
		cli:    &Cli{SystemVariables: &sv, SessionHandler: NewSessionHandler(&Session{systemVariables: &sv})},
		editor: dummyFuzzyEditor(io.Discard),
	}
	network := []fuzzyCompletionType{
		fuzzyCompleteDatabase, fuzzyCompleteTable, fuzzyCompleteRole, fuzzyCompleteOperation,
		fuzzyCompleteView, fuzzyCompleteIndex, fuzzyCompleteChangeStream, fuzzyCompleteSequence,
		fuzzyCompleteModel, fuzzyCompleteSchema,
	}
	for _, ct := range network {
		got, err := f.resolveCandidates(t.Context(), ct, "db")
		if err != nil || got != nil {
			t.Fatalf("%s: got %v err=%v, want empty", ct, got, err)
		}
		direct, err := f.fetchCandidates(t.Context(), ct, "db")
		if err != nil || direct != nil {
			t.Fatalf("%s fetch: got %v err=%v, want empty", ct, direct, err)
		}
	}
}

func TestSetCachedCandidatesNilSession(t *testing.T) {
	t.Parallel()
	f := &fuzzyFinderCommand{cli: &Cli{SessionHandler: NewSessionHandler(nil)}}
	f.setCachedCandidates(fuzzyCompleteTable, []fzfItem{{Value: "t"}})
	if f.cache != nil {
		t.Fatalf("nil session stored cache: %v", f.cache)
	}
}

func TestFuzzyFinderCallEmptyNetworkCompletion(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaults()
	f := &fuzzyFinderCommand{
		cli:    &Cli{SystemVariables: &sv, SessionHandler: NewSessionHandler(nil)},
		editor: dummyFuzzyEditor(io.Discard),
	}
	b := &readline.Buffer{Editor: &readline.Editor{}}
	input := "USE "
	b.Cursor = b.InsertString(0, input)
	if result := f.Call(t.Context(), b); result != readline.CONTINUE || b.String() != input {
		t.Fatalf("result=%v buffer=%q", result, b.String())
	}
}

func TestResolveCandidatesShowsLoadingThenCaches(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server := &fuzzyAdminServer{
		databases: []*databasepb.Database{
			{Name: "projects/p/instances/i/databases/alpha"},
			{Name: "not-a-resource"},
			{Name: "projects/p/instances/i/databases/beta"},
		},
	}
	admin := newFuzzyAdminSession(t, server)
	var buf bytes.Buffer
	f := &fuzzyFinderCommand{
		cli:    &Cli{SessionHandler: NewSessionHandler(admin)},
		editor: dummyFuzzyEditor(&buf),
	}
	got, err := f.resolveCandidates(ctx, fuzzyCompleteDatabase, "")
	if err != nil {
		t.Fatal(err)
	}
	if server.dbParent != admin.InstancePath() {
		t.Fatalf("ListDatabases parent = %q, want %q", server.dbParent, admin.InstancePath())
	}
	want := []fzfItem{{Value: "alpha"}, {Value: "beta"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("databases mismatch (-want +got):\n%s", diff)
	}
	if !strings.Contains(buf.String(), "Loading...") {
		t.Fatalf("loading indicator missing: %q", buf.String())
	}
	cached := f.getCachedCandidates(fuzzyCompleteDatabase)
	if diff := cmp.Diff(want, cached); diff != "" {
		t.Fatalf("cache mismatch (-want +got):\n%s", diff)
	}
	buf.Reset()
	again, err := f.resolveCandidates(ctx, fuzzyCompleteDatabase, "")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, again); diff != "" {
		t.Fatalf("cache hit mismatch (-want +got):\n%s", diff)
	}
	if buf.Len() != 0 {
		t.Fatalf("cache hit wrote loading output: %q", buf.String())
	}
}

func TestFetchAdminCandidates(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("database list error", func(t *testing.T) {
		session := newFuzzyAdminSession(t, &fuzzyAdminServer{dbErr: status.Error(codes.PermissionDenied, "denied")})
		f := fuzzyFinderForSession(session)
		_, err := f.fetchDatabaseCandidates(ctx)
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("err=%v", err)
		}
		_, err = f.resolveCandidates(ctx, fuzzyCompleteDatabase, "")
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("resolve err=%v", err)
		}
	})

	t.Run("roles sorted and filtered", func(t *testing.T) {
		server := &fuzzyAdminServer{
			roles: []*databasepb.DatabaseRole{
				{Name: "projects/p/instances/i/databases/db/databaseRoles/zeta"},
				{Name: "not-a-role"},
				{Name: "projects/p/instances/i/databases/db/databaseRoles/alpha"},
			},
		}
		session := newFuzzyAdminSession(t, server)
		f := fuzzyFinderForSession(session)
		got, err := f.fetchRoleCandidates(ctx, "db")
		if err != nil {
			t.Fatal(err)
		}
		want := []fzfItem{{Value: "alpha"}, {Value: "zeta"}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("roles mismatch (-want +got):\n%s", diff)
		}
		if server.rolePath != session.InstancePath()+"/databases/db" {
			t.Fatalf("ListDatabaseRoles parent = %q", server.rolePath)
		}
		_, err = f.fetchCandidates(ctx, fuzzyCompleteRole, "db")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("role list error", func(t *testing.T) {
		session := newFuzzyAdminSession(t, &fuzzyAdminServer{roleErr: status.Error(codes.NotFound, "missing")})
		f := fuzzyFinderForSession(session)
		_, err := f.fetchRoleCandidates(ctx, "db")
		if status.Code(err) != codes.NotFound {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("operations skip non-DDL and bad metadata", func(t *testing.T) {
		ddl, err := anypb.New(&databasepb.UpdateDatabaseDdlMetadata{
			Statements: []string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)", "CREATE INDEX i ON t (id)"},
		})
		if err != nil {
			t.Fatal(err)
		}
		bad := &anypb.Any{
			TypeUrl: "type.googleapis.com/google.spanner.admin.database.v1.UpdateDatabaseDdlMetadata",
			Value:   []byte("not-protobuf"),
		}
		other, err := anypb.New(&emptypb.Empty{})
		if err != nil {
			t.Fatal(err)
		}
		server := &fuzzyAdminServer{
			ops: []*longrunningpb.Operation{
				{Name: "projects/p/instances/i/databases/db/operations/other", Metadata: other},
				{Name: "projects/p/instances/i/databases/db/operations/bad", Metadata: bad},
				{Name: "projects/p/instances/i/databases/db/operations/op-1", Metadata: ddl},
			},
		}
		session := newFuzzyAdminSession(t, server)
		f := fuzzyFinderForSession(session)
		got, err := f.fetchOperationCandidates(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []fzfItem{{
			Value: "op-1",
			Label: "CREATE TABLE t (id INT64) PRIMARY KEY (id);\nCREATE INDEX i ON t (id);",
		}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("operations mismatch (-want +got):\n%s", diff)
		}
		if server.opName != session.DatabasePath()+"/operations" {
			t.Fatalf("ListOperations name = %q", server.opName)
		}
		if _, err := f.fetchCandidates(ctx, fuzzyCompleteOperation, ""); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("operation list error", func(t *testing.T) {
		session := newFuzzyAdminSession(t, &fuzzyAdminServer{opErr: status.Error(codes.Unavailable, "down")})
		f := fuzzyFinderForSession(session)
		_, err := f.fetchOperationCandidates(ctx)
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestFetchSchemaObjectCandidates(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("qualified and default schema names", func(t *testing.T) {
		session := newFuzzySchemaSession(t, &fuzzySchemaServer{})
		f := fuzzyFinderForSession(session)
		got, err := f.fetchTableCandidates(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []fzfItem{{Value: "Singers"}, {Value: "app.Users"}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("tables mismatch (-want +got):\n%s", diff)
		}
		checks := []struct {
			ct   fuzzyCompletionType
			want string
		}{
			{fuzzyCompleteView, "MyView"},
			{fuzzyCompleteIndex, "Idx"},
			{fuzzyCompleteChangeStream, "Cs"},
			{fuzzyCompleteSequence, "Seq"},
			{fuzzyCompleteModel, "mdl.Model"},
			{fuzzyCompleteSchema, "app"},
		}
		for _, tt := range checks {
			items, err := f.fetchCandidates(ctx, tt.ct, "")
			if err != nil {
				t.Fatalf("%s: %v", tt.ct, err)
			}
			if !containsFzfValue(items, tt.want) {
				t.Fatalf("%s candidates = %v, want %q", tt.ct, items, tt.want)
			}
		}
	})

	t.Run("query error", func(t *testing.T) {
		session := newFuzzySchemaSession(t, &fuzzySchemaServer{queryErr: status.Error(codes.Internal, "boom")})
		f := fuzzyFinderForSession(session)
		_, err := f.fetchTableCandidates(ctx)
		if err == nil || !strings.Contains(err.Error(), "fetchTableCandidates") {
			t.Fatalf("err=%v", err)
		}
		_, err = f.fetchSchemaCandidates(ctx)
		if err == nil || !strings.Contains(err.Error(), "fetchSchemaCandidates") {
			t.Fatalf("schema err=%v", err)
		}
	})

	t.Run("column type error", func(t *testing.T) {
		session := newFuzzySchemaSession(t, &fuzzySchemaServer{intColumns: true})
		f := fuzzyFinderForSession(session)
		_, err := f.fetchViewCandidates(ctx)
		if err == nil || !strings.Contains(err.Error(), "fetchViewCandidates") {
			t.Fatalf("err=%v", err)
		}
		_, err = f.fetchSchemaCandidates(ctx)
		if err == nil || !strings.Contains(err.Error(), "fetchSchemaCandidates") {
			t.Fatalf("schema err=%v", err)
		}
	})
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func containsFzfValue(items []fzfItem, want string) bool {
	for _, item := range items {
		if item.Value == want {
			return true
		}
	}
	return false
}

func dummyFuzzyEditor(w io.Writer) *multiline.Editor {
	ed := &multiline.Editor{}
	ed.LineEditor.Writer = w
	ed.LineEditor.Out = bufio.NewWriter(w)
	return ed
}

func fuzzyFinderForSession(session *Session) *fuzzyFinderCommand {
	return &fuzzyFinderCommand{
		cli:    &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables},
		editor: dummyFuzzyEditor(io.Discard),
	}
}

type fuzzyAdminServer struct {
	databasepb.UnimplementedDatabaseAdminServer
	longrunningpb.UnimplementedOperationsServer

	databases []*databasepb.Database
	dbErr     error
	dbParent  string

	roles    []*databasepb.DatabaseRole
	roleErr  error
	rolePath string

	ops    []*longrunningpb.Operation
	opErr  error
	opName string
}

func (s *fuzzyAdminServer) ListDatabases(_ context.Context, req *databasepb.ListDatabasesRequest) (*databasepb.ListDatabasesResponse, error) {
	s.dbParent = req.GetParent()
	if s.dbErr != nil {
		return nil, s.dbErr
	}
	return &databasepb.ListDatabasesResponse{Databases: s.databases}, nil
}

func (s *fuzzyAdminServer) ListDatabaseRoles(_ context.Context, req *databasepb.ListDatabaseRolesRequest) (*databasepb.ListDatabaseRolesResponse, error) {
	s.rolePath = req.GetParent()
	if s.roleErr != nil {
		return nil, s.roleErr
	}
	return &databasepb.ListDatabaseRolesResponse{DatabaseRoles: s.roles}, nil
}

func (s *fuzzyAdminServer) ListOperations(_ context.Context, req *longrunningpb.ListOperationsRequest) (*longrunningpb.ListOperationsResponse, error) {
	s.opName = req.GetName()
	if s.opErr != nil {
		return nil, s.opErr
	}
	return &longrunningpb.ListOperationsResponse{Operations: s.ops}, nil
}

func newFuzzyAdminSession(t *testing.T, server *fuzzyAdminServer) *Session {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	databasepb.RegisterDatabaseAdminServer(grpcServer, server)
	longrunningpb.RegisterOperationsServer(grpcServer, server)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///fuzzy-admin",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	adminClient, err := adminapi.NewDatabaseAdminClient(t.Context(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminClient.Close() })
	sv := newSystemVariablesWithDefaultsForTest()
	identity := ConnectionVars{Project: "p", Instance: "i", Database: "db"}
	sv.Connection = identity
	return &Session{
		adminClient:     adminClient,
		systemVariables: sv,
		connection:      identity,
	}
}

type fuzzySchemaServer struct {
	sppb.UnimplementedSpannerServer
	queryErr   error
	intColumns bool
}

func (s *fuzzySchemaServer) CreateSession(_ context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Database + "/sessions/fuzzy", Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *fuzzySchemaServer) BatchCreateSessions(_ context.Context, r *sppb.BatchCreateSessionsRequest) (*sppb.BatchCreateSessionsResponse, error) {
	n := int(r.SessionCount)
	if n <= 0 {
		n = 1
	}
	sessions := make([]*sppb.Session, n)
	for i := range n {
		sessions[i] = &sppb.Session{Name: fmt.Sprintf("%s/sessions/%d", r.Database, i), CreateTime: timestamppb.Now()}
	}
	return &sppb.BatchCreateSessionsResponse{Session: sessions}, nil
}

func (s *fuzzySchemaServer) GetSession(_ context.Context, r *sppb.GetSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Name, Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *fuzzySchemaServer) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *fuzzySchemaServer) BeginTransaction(context.Context, *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	return &sppb.Transaction{Id: []byte("fuzzy-ro"), ReadTimestamp: timestamppb.Now()}, nil
}

func (s *fuzzySchemaServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	if s.queryErr != nil {
		return s.queryErr
	}
	names, rows := fuzzySchemaFixture(r.Sql)
	code := sppb.TypeCode_STRING
	if s.intColumns {
		code = sppb.TypeCode_INT64
	}
	fields := make([]*sppb.StructType_Field, len(names))
	for i, name := range names {
		fields[i] = &sppb.StructType_Field{Name: name, Type: &sppb.Type{Code: code}}
	}
	var values []*structpb.Value
	for _, row := range rows {
		for _, v := range row {
			if s.intColumns {
				values = append(values, structpb.NewStringValue("1"))
				continue
			}
			values = append(values, structpb.NewStringValue(v))
		}
	}
	return stream.Send(&sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType:     &sppb.StructType{Fields: fields},
			Transaction: &sppb.Transaction{Id: []byte("fuzzy-ro"), ReadTimestamp: timestamppb.Now()},
		},
		Values: values,
	})
}

func fuzzySchemaFixture(sql string) ([]string, [][]string) {
	switch {
	case strings.Contains(sql, "INFORMATION_SCHEMA.TABLES"):
		return []string{"TABLE_SCHEMA", "TABLE_NAME"}, [][]string{{"", "Singers"}, {"app", "Users"}}
	case strings.Contains(sql, "INFORMATION_SCHEMA.VIEWS"):
		return []string{"TABLE_SCHEMA", "TABLE_NAME"}, [][]string{{"", "MyView"}}
	case strings.Contains(sql, "INFORMATION_SCHEMA.INDEXES"):
		return []string{"TABLE_SCHEMA", "INDEX_NAME"}, [][]string{{"", "Idx"}}
	case strings.Contains(sql, "INFORMATION_SCHEMA.CHANGE_STREAMS"):
		return []string{"CHANGE_STREAM_SCHEMA", "CHANGE_STREAM_NAME"}, [][]string{{"", "Cs"}}
	case strings.Contains(sql, "INFORMATION_SCHEMA.SEQUENCES"):
		return []string{"SCHEMA", "NAME"}, [][]string{{"", "Seq"}}
	case strings.Contains(sql, "INFORMATION_SCHEMA.MODELS"):
		return []string{"MODEL_SCHEMA", "MODEL_NAME"}, [][]string{{"mdl", "Model"}}
	default:
		return []string{"SCHEMA_NAME"}, [][]string{{"app"}}
	}
}

func newFuzzySchemaSession(t *testing.T, server *fuzzySchemaServer) *Session {
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
	conn, err := grpc.NewClient("passthrough:///fuzzy-schema",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := spanner.NewClientWithConfig(t.Context(), "projects/p/instances/i/databases/db",
		spanner.ClientConfig{DisableNativeMetrics: true}, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	sv := newSystemVariablesWithDefaultsForTest()
	sv.Connection = ConnectionVars{Project: "p", Instance: "i", Database: "db"}
	return &Session{
		mode:            DatabaseConnected,
		client:          client,
		systemVariables: sv,
		connection:      sv.Connection,
		txn:             NewTransactionManager(client, sv, spanner.ClientConfig{DisableNativeMetrics: true}),
	}
}
