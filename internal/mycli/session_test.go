package mycli

import (
	"context"
	_ "embed"
	"errors"
	"testing"

	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestParseDirectedReadOption(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		desc   string
		option string
		want   *sppb.DirectedReadOptions
	}{
		{
			desc:   "use directed read location option only",
			option: "us-central1",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-central1",
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
		},
		{
			desc:   "use directed read location and type option (READ_ONLY)",
			option: "us-central1:READ_ONLY",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-central1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
		},
		{
			desc:   "use directed read location and type option (READ_WRITE)",
			option: "us-central1:READ_WRITE",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-central1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
		},
		{
			desc:   "use invalid type option",
			option: "us-central1:READONLY",
			want:   nil,
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			got, _ := parseDirectedReadOption(tt.option)

			if !cmp.Equal(got, tt.want, protocmp.Transform()) {
				t.Errorf("Got = %v, but want = %v", got, tt.want)
			}
		})
	}
}

func TestSession_TransactionMode(t *testing.T) {
	t.Parallel()
	s := &Session{txn: &TransactionManager{}}

	if got := s.TransactionMode(); got != transactionModeUndetermined {
		t.Errorf("New session should have undetermined transaction mode, got %v", got)
	}

	s.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
	if got := s.TransactionMode(); got != transactionModeReadWrite {
		t.Errorf("Session with read-write transaction should return read-write mode, got %v", got)
	}

	s.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadOnly}}
	if got := s.TransactionMode(); got != transactionModeReadOnly {
		t.Errorf("Session with read-only transaction should return read-only mode, got %v", got)
	}

	s.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModePending}}
	if got := s.TransactionMode(); got != transactionModePending {
		t.Errorf("Session with pending transaction should return pending mode, got %v", got)
	}
}

func TestSession_FailStatementIfReadOnly(t *testing.T) {
	t.Parallel()
	s := &Session{systemVariables: &systemVariables{Transaction: TransactionVars{ReadOnly: true}}}
	err := s.failStatementIfReadOnly()
	if err == nil {
		t.Errorf("failStatementIfReadOnly should return an error when ReadOnly is true")
	}
	if !errors.Is(err, errReadOnly) {
		t.Errorf("failStatementIfReadOnly should return specific error, got %v", err)
	}

	s = &Session{systemVariables: &systemVariables{Transaction: TransactionVars{ReadOnly: false}}}
	err = s.failStatementIfReadOnly()
	if err != nil {
		t.Errorf("failStatementIfReadOnly should not return an error when ReadOnly is false")
	}
}

func TestNewSessionClosesClientWhenAdminClientCreationFails(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("admin client creation failed")
	fakeClient := &spanner.Client{}

	var closedClient *spanner.Client

	session, err := newSessionWithFactories(
		context.Background(),
		&systemVariables{
			Connection: ConnectionVars{
				Project:  "test-project",
				Instance: "test-instance",
				Database: "test-database",
			},
		},
		func(context.Context, string, spanner.ClientConfig, ...option.ClientOption) (*spanner.Client, error) {
			return fakeClient, nil
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return nil, expectedErr
		},
		func(client *spanner.Client) {
			closedClient = client
		},
	)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("newSessionWithFactories() error = %v, want %v", err, expectedErr)
	}
	if session != nil {
		t.Fatalf("newSessionWithFactories() session = %#v, want nil", session)
	}
	if closedClient != fakeClient {
		t.Fatalf("newSessionWithFactories() closed client = %p, want %p", closedClient, fakeClient)
	}
}

func TestCreateClientOptionsUsesEmbeddedClientOptions(t *testing.T) {
	t.Parallel()

	sysVars := &systemVariables{
		Connection: ConnectionVars{
			Host:                  "localhost",
			Port:                  9010,
			WithoutAuthentication: true,
		},
		Internal: InternalVars{
			EmbeddedClientOptions: []option.ClientOption{
				option.WithoutAuthentication(),
			},
		},
	}

	opts, err := createClientOptions(context.Background(), nil, sysVars)
	if err != nil {
		t.Fatalf("createClientOptions() error = %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("createClientOptions() len = %d, want 1", len(opts))
	}
}

func TestNewSessionWithFactoriesUsesEmbeddedClientConfig(t *testing.T) {
	t.Parallel()

	directedRead := &sppb.DirectedReadOptions{}
	sysVars := &systemVariables{
		Connection: ConnectionVars{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "test-database",
			Role:     "test-role",
		},
		Query: QueryVars{
			DirectedRead: directedRead,
		},
		Internal: InternalVars{
			EmbeddedClientConfig: &spanner.ClientConfig{
				DisableNativeMetrics: true,
				IsExperimentalHost:   true,
				DisableRouteToLeader: true,
				UserAgent:            "embedded-omni-test",
			},
		},
	}

	var gotConfig spanner.ClientConfig
	session, err := newSessionWithFactories(
		context.Background(),
		sysVars,
		func(_ context.Context, _ string, cfg spanner.ClientConfig, _ ...option.ClientOption) (*spanner.Client, error) {
			gotConfig = cfg
			return &spanner.Client{}, nil
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return &adminapi.DatabaseAdminClient{}, nil
		},
		func(*spanner.Client) {},
	)
	if err != nil {
		t.Fatalf("newSessionWithFactories() error = %v", err)
	}
	if session == nil {
		t.Fatal("newSessionWithFactories() returned nil session")
	}
	if !gotConfig.DisableNativeMetrics {
		t.Error("DisableNativeMetrics = false, want true")
	}
	if !gotConfig.IsExperimentalHost {
		t.Error("IsExperimentalHost = false, want true")
	}
	if !gotConfig.DisableRouteToLeader {
		t.Error("DisableRouteToLeader = false, want true")
	}
	if gotConfig.UserAgent != "embedded-omni-test" {
		t.Errorf("UserAgent = %q, want %q", gotConfig.UserAgent, "embedded-omni-test")
	}
	if gotConfig.MinOpened != defaultClientConfig.MinOpened {
		t.Errorf("SessionPoolConfig.MinOpened = %d, want %d", gotConfig.MinOpened, defaultClientConfig.MinOpened)
	}
	if gotConfig.MaxOpened != defaultClientConfig.MaxOpened {
		t.Errorf("SessionPoolConfig.MaxOpened = %d, want %d", gotConfig.MaxOpened, defaultClientConfig.MaxOpened)
	}
	if gotConfig.DatabaseRole != "test-role" {
		t.Errorf("DatabaseRole = %q, want %q", gotConfig.DatabaseRole, "test-role")
	}
	if diff := cmp.Diff(directedRead, gotConfig.DirectedReadOptions, protocmp.Transform()); diff != "" {
		t.Errorf("DirectedReadOptions mismatch (-want +got):\n%s", diff)
	}
}

func TestNewSessionWithFactoriesDoesNotAppendInsecureForEmbeddedOptions(t *testing.T) {
	t.Parallel()

	sysVars := &systemVariables{
		Connection: ConnectionVars{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "test-database",
			Insecure: true,
		},
		Internal: InternalVars{
			EmbeddedClientOptions: []option.ClientOption{
				option.WithoutAuthentication(),
			},
		},
	}

	var gotOpts []option.ClientOption
	session, err := newSessionWithFactories(
		context.Background(),
		sysVars,
		func(_ context.Context, _ string, _ spanner.ClientConfig, opts ...option.ClientOption) (*spanner.Client, error) {
			gotOpts = append([]option.ClientOption(nil), opts...)
			return &spanner.Client{}, nil
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return &adminapi.DatabaseAdminClient{}, nil
		},
		func(*spanner.Client) {},
		sysVars.Internal.EmbeddedClientOptions...,
	)
	if err != nil {
		t.Fatalf("newSessionWithFactories() error = %v", err)
	}
	if session == nil {
		t.Fatal("newSessionWithFactories() returned nil session")
	}
	if len(gotOpts) != len(sysVars.Internal.EmbeddedClientOptions)+len(defaultClientOpts) {
		t.Fatalf("len(opts) = %d, want %d", len(gotOpts), len(sysVars.Internal.EmbeddedClientOptions)+len(defaultClientOpts))
	}
}
