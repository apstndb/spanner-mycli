package mycli

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"strings"
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

	if got := s.txn.TransactionMode(); got != transactionModeUndetermined {
		t.Errorf("New session should have undetermined transaction mode, got %v", got)
	}

	s.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
	if got := s.txn.TransactionMode(); got != transactionModeReadWrite {
		t.Errorf("Session with read-write transaction should return read-write mode, got %v", got)
	}

	s.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadOnly}}
	if got := s.txn.TransactionMode(); got != transactionModeReadOnly {
		t.Errorf("Session with read-only transaction should return read-only mode, got %v", got)
	}

	s.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModePending}}
	if got := s.txn.TransactionMode(); got != transactionModePending {
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
		ConnectionVars{
			Project:  "test-project",
			Instance: "test-instance",
			Database: "test-database",
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
		Config: StartupConfig{
			Host:                  "localhost",
			Port:                  9010,
			WithoutAuthentication: true,
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

// TestAuthOptionsSkipsEmulatorAuthForNonSpannerClients verifies that
// Session.AuthOptions with allowWithoutAuthentication=false — the path features
// like BigQuery use to build non-Spanner clients — does NOT apply the Spanner
// emulator's WithoutAuthentication option, so it falls back to ADC. This
// preserves the coverage of the removed createBigQueryClientOptions helper.
func TestAuthOptionsSkipsEmulatorAuthForNonSpannerClients(t *testing.T) {
	t.Parallel()

	sysVars := &systemVariables{
		Config: StartupConfig{
			Host:                  "localhost",
			Port:                  9010,
			WithoutAuthentication: true,
		},
	}
	session := &Session{systemVariables: sysVars}

	opts, err := session.AuthOptions(t.Context(), nil, false)
	if err != nil {
		t.Fatalf("AuthOptions() error = %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("AuthOptions() len = %d, want 0 (ADC, not emulator auth)", len(opts))
	}
}

// TestSessionCredentialBytesFromDurableConfig is the #775 credential-handoff
// regression, now proven by construction (#778 §4.6): the raw --credential lives
// on the durable startup config, so Session.CredentialBytes() returns it (a
// defensive copy) and every session sharing that config — including USE/DETACH
// replacement sessions — resolves the same credential without any carry-over
// machinery.
func TestSessionCredentialBytesFromDurableConfig(t *testing.T) {
	t.Parallel()

	credential := []byte(`{"type":"authorized_user","client_id":"operator"}`)
	sysVars := &systemVariables{}
	sysVars.Config.Credential = append([]byte(nil), credential...)

	session := &Session{systemVariables: sysVars}

	got := session.CredentialBytes()
	if !bytes.Equal(got, credential) {
		t.Fatalf("CredentialBytes() = %q, want %q", got, credential)
	}

	// Defensive copy out: mutating the returned slice must not affect the stored
	// credential or later reads.
	got[0] = '['
	if !bytes.Equal(session.CredentialBytes(), credential) {
		t.Fatal("CredentialBytes() returned a slice aliasing the durable config")
	}

	// A replacement session built around the same systemVariables (USE/DETACH)
	// resolves the identical credential by construction.
	replacement := &Session{systemVariables: sysVars}
	if !bytes.Equal(replacement.CredentialBytes(), credential) {
		t.Fatalf("replacement CredentialBytes() = %q, want %q", replacement.CredentialBytes(), credential)
	}

	// No credential configured yields nil (not an empty non-nil slice).
	if got := (&Session{systemVariables: &systemVariables{}}).CredentialBytes(); got != nil {
		t.Fatalf("CredentialBytes() with no credential = %q, want nil", got)
	}
}

func TestCredentialsJSONOption(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name: "service account",
			json: `{"type":"service_account"}`,
		},
		{
			name: "authorized user",
			json: `{"type":"authorized_user"}`,
		},
		{
			name: "impersonated service account",
			json: `{"type":"impersonated_service_account"}`,
		},
		{
			name: "external account",
			json: `{"type":"external_account"}`,
		},
		{
			name:    "unsupported type",
			json:    `{"type":"unknown"}`,
			wantErr: `unsupported credential type "unknown"`,
		},
		{
			name:    "missing type",
			json:    `{}`,
			wantErr: "credential JSON missing type",
		},
		{
			name:    "invalid JSON",
			json:    `{`,
			wantErr: "parse credential type:",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opt, err := credentialsJSONOption([]byte(tt.json))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("credentialsJSONOption() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("credentialsJSONOption() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				if opt != nil {
					t.Fatalf("credentialsJSONOption() option = %#v, want nil", opt)
				}
				return
			}
			if err != nil {
				t.Fatalf("credentialsJSONOption() error = %v", err)
			}
			if opt == nil {
				t.Fatal("credentialsJSONOption() option = nil, want non-nil")
			}
		})
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
		Config: StartupConfig{
			EmbeddedClientConfig: &spanner.ClientConfig{
				DisableNativeMetrics: true,
				Type:                 spanner.OMNI,
				DisableRouteToLeader: true,
				UserAgent:            "embedded-omni-test",
			},
		},
	}

	var gotConfig spanner.ClientConfig
	session, err := newSessionWithFactories(
		context.Background(),
		sysVars,
		sysVars.Connection,
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
	if gotConfig.Type != spanner.OMNI {
		t.Errorf("Type = %q, want %q", gotConfig.Type, spanner.OMNI)
	}
	if !gotConfig.DisableRouteToLeader {
		t.Error("DisableRouteToLeader = false, want true")
	}
	if gotConfig.UserAgent != "embedded-omni-test" {
		t.Errorf("UserAgent = %q, want %q", gotConfig.UserAgent, "embedded-omni-test")
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
		},
		Config: StartupConfig{
			Insecure: true,
			EmbeddedClientOptions: []option.ClientOption{
				option.WithoutAuthentication(),
			},
		},
	}

	var gotOpts []option.ClientOption
	session, err := newSessionWithFactories(
		context.Background(),
		sysVars,
		sysVars.Connection,
		func(_ context.Context, _ string, _ spanner.ClientConfig, opts ...option.ClientOption) (*spanner.Client, error) {
			gotOpts = append([]option.ClientOption(nil), opts...)
			return &spanner.Client{}, nil
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return &adminapi.DatabaseAdminClient{}, nil
		},
		func(*spanner.Client) {},
		sysVars.Config.EmbeddedClientOptions...,
	)
	if err != nil {
		t.Fatalf("newSessionWithFactories() error = %v", err)
	}
	if session == nil {
		t.Fatal("newSessionWithFactories() returned nil session")
	}
	if len(gotOpts) != len(sysVars.Config.EmbeddedClientOptions)+len(defaultClientOpts) {
		t.Fatalf("len(opts) = %d, want %d", len(gotOpts), len(sysVars.Config.EmbeddedClientOptions)+len(defaultClientOpts))
	}
}

func TestSessionConstructionIdentityIndependentOfLiveConnection(t *testing.T) {
	t.Parallel()

	identityA := ConnectionVars{
		Project:  "project-a",
		Instance: "instance-a",
		Database: "database-a",
		Role:     "role-a",
	}
	identityB := ConnectionVars{
		Project:  "project-b",
		Instance: "instance-b",
		Database: "database-b",
		Role:     "role-b",
	}
	directedRead := &sppb.DirectedReadOptions{}
	sysVars := &systemVariables{
		Connection: identityA,
		Query:      QueryVars{DirectedRead: directedRead},
	}

	var pathA string
	var configA spanner.ClientConfig
	var clientOptsA, adminOptsA []option.ClientOption
	sessionA, err := newSessionWithFactories(
		t.Context(),
		sysVars,
		sysVars.Connection,
		func(_ context.Context, dbPath string, cfg spanner.ClientConfig, opts ...option.ClientOption) (*spanner.Client, error) {
			pathA = dbPath
			configA = cfg
			clientOptsA = append([]option.ClientOption(nil), opts...)
			return &spanner.Client{}, nil
		},
		func(_ context.Context, opts ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			adminOptsA = append([]option.ClientOption(nil), opts...)
			return &adminapi.DatabaseAdminClient{}, nil
		},
		func(*spanner.Client) {},
	)
	if err != nil {
		t.Fatalf("construct session A: %v", err)
	}
	if pathA != identityA.DatabasePath() {
		t.Fatalf("client path = %q, want %q", pathA, identityA.DatabasePath())
	}
	if configA.DatabaseRole != identityA.Role {
		t.Fatalf("DatabaseRole = %q, want %q", configA.DatabaseRole, identityA.Role)
	}
	if diff := cmp.Diff(directedRead, configA.DirectedReadOptions, protocmp.Transform()); diff != "" {
		t.Errorf("DirectedReadOptions mismatch (-want +got):\n%s", diff)
	}
	if len(clientOptsA) != len(adminOptsA) {
		t.Fatalf("client opts %d, admin opts %d", len(clientOptsA), len(adminOptsA))
	}
	if sessionA.DatabasePath() != identityA.DatabasePath() {
		t.Fatalf("session A DatabasePath = %q, want %q", sessionA.DatabasePath(), identityA.DatabasePath())
	}
	if sessionA.InstancePath() != identityA.InstancePath() {
		t.Fatalf("session A InstancePath = %q, want %q", sessionA.InstancePath(), identityA.InstancePath())
	}
	if sessionA.ProjectID() != identityA.Project {
		t.Fatalf("session A ProjectID = %q, want %q", sessionA.ProjectID(), identityA.Project)
	}

	sysVars.Connection = identityB
	if sysVars.DatabasePath() != identityB.DatabasePath() {
		t.Fatalf("live DatabasePath = %q, want %q", sysVars.DatabasePath(), identityB.DatabasePath())
	}
	if sessionA.DatabasePath() != identityA.DatabasePath() {
		t.Fatalf("session A DatabasePath after live mutation = %q, want %q", sessionA.DatabasePath(), identityA.DatabasePath())
	}
	if sessionA.InstancePath() != identityA.InstancePath() {
		t.Fatalf("session A InstancePath after live mutation = %q, want %q", sessionA.InstancePath(), identityA.InstancePath())
	}
	if sessionA.clientConfig.DatabaseRole != identityA.Role {
		t.Fatalf("session A DatabaseRole after live mutation = %q, want %q", sessionA.clientConfig.DatabaseRole, identityA.Role)
	}
	if sessionA.ProjectID() != identityA.Project {
		t.Fatalf("session A ProjectID after live mutation = %q, want %q", sessionA.ProjectID(), identityA.Project)
	}

	var pathB string
	var configB spanner.ClientConfig
	sessionB, err := newSessionWithFactories(
		t.Context(),
		sysVars,
		sysVars.Connection,
		func(_ context.Context, dbPath string, cfg spanner.ClientConfig, _ ...option.ClientOption) (*spanner.Client, error) {
			pathB = dbPath
			configB = cfg
			return &spanner.Client{}, nil
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return &adminapi.DatabaseAdminClient{}, nil
		},
		func(*spanner.Client) {},
	)
	if err != nil {
		t.Fatalf("construct session B: %v", err)
	}
	if pathB != identityB.DatabasePath() {
		t.Fatalf("client path B = %q, want %q", pathB, identityB.DatabasePath())
	}
	if configB.DatabaseRole != identityB.Role {
		t.Fatalf("session B DatabaseRole = %q, want %q", configB.DatabaseRole, identityB.Role)
	}
	if sessionB.DatabasePath() != identityB.DatabasePath() {
		t.Fatalf("session B DatabasePath = %q, want %q", sessionB.DatabasePath(), identityB.DatabasePath())
	}
	if sessionA.DatabasePath() != identityA.DatabasePath() {
		t.Fatalf("session A DatabasePath after constructing B = %q, want %q", sessionA.DatabasePath(), identityA.DatabasePath())
	}

	var explicitPath string
	var explicitConfig spanner.ClientConfig
	explicit, err := newSessionWithFactories(
		t.Context(),
		sysVars,
		identityA,
		func(_ context.Context, dbPath string, cfg spanner.ClientConfig, _ ...option.ClientOption) (*spanner.Client, error) {
			explicitPath = dbPath
			explicitConfig = cfg
			return &spanner.Client{}, nil
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return &adminapi.DatabaseAdminClient{}, nil
		},
		func(*spanner.Client) {},
	)
	if err != nil {
		t.Fatalf("construct explicit identity A while live is B: %v", err)
	}
	if sysVars.DatabasePath() != identityB.DatabasePath() {
		t.Fatalf("explicit construction mutated live DatabasePath = %q, want %q", sysVars.DatabasePath(), identityB.DatabasePath())
	}
	if explicitPath != identityA.DatabasePath() {
		t.Fatalf("explicit client path = %q, want %q", explicitPath, identityA.DatabasePath())
	}
	if explicitConfig.DatabaseRole != identityA.Role {
		t.Fatalf("explicit DatabaseRole = %q, want %q", explicitConfig.DatabaseRole, identityA.Role)
	}
	if explicit.DatabasePath() != identityA.DatabasePath() {
		t.Fatalf("explicit session DatabasePath = %q, want %q", explicit.DatabasePath(), identityA.DatabasePath())
	}
}

func TestAdminSessionConstructionIdentityIndependentOfLiveConnection(t *testing.T) {
	t.Parallel()

	identityA := ConnectionVars{
		Project:  "project-a",
		Instance: "instance-a",
		Role:     "role-a",
	}
	identityB := ConnectionVars{
		Project:  "project-b",
		Instance: "instance-b",
		Role:     "role-b",
	}
	sysVars := &systemVariables{Connection: identityA}

	sessionA, err := newAdminSessionWithFactories(
		t.Context(),
		sysVars,
		sysVars.Connection,
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return &adminapi.DatabaseAdminClient{}, nil
		},
	)
	if err != nil {
		t.Fatalf("construct detached session A: %v", err)
	}
	if !sessionA.IsDetached() {
		t.Fatal("session A mode: want detached")
	}
	if sessionA.InstancePath() != identityA.InstancePath() {
		t.Fatalf("session A InstancePath = %q, want %q", sessionA.InstancePath(), identityA.InstancePath())
	}
	if sessionA.DatabasePath() != identityA.DatabasePath() {
		t.Fatalf("session A DatabasePath = %q, want %q", sessionA.DatabasePath(), identityA.DatabasePath())
	}
	if sessionA.clientConfig.DatabaseRole != identityA.Role {
		t.Fatalf("session A DatabaseRole = %q, want %q", sessionA.clientConfig.DatabaseRole, identityA.Role)
	}

	sysVars.Connection = identityB
	if sysVars.InstancePath() != identityB.InstancePath() {
		t.Fatalf("live InstancePath = %q, want %q", sysVars.InstancePath(), identityB.InstancePath())
	}
	if sessionA.InstancePath() != identityA.InstancePath() {
		t.Fatalf("session A InstancePath after live mutation = %q, want %q", sessionA.InstancePath(), identityA.InstancePath())
	}
	if sessionA.clientConfig.DatabaseRole != identityA.Role {
		t.Fatalf("session A DatabaseRole after live mutation = %q, want %q", sessionA.clientConfig.DatabaseRole, identityA.Role)
	}

	sessionB, err := newAdminSessionWithFactories(
		t.Context(),
		sysVars,
		identityB,
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return &adminapi.DatabaseAdminClient{}, nil
		},
	)
	if err != nil {
		t.Fatalf("construct detached session B: %v", err)
	}
	if !sessionB.IsDetached() {
		t.Fatal("session B mode: want detached")
	}
	if sessionB.InstancePath() != identityB.InstancePath() {
		t.Fatalf("session B InstancePath = %q, want %q", sessionB.InstancePath(), identityB.InstancePath())
	}
	if sessionA.InstancePath() != identityA.InstancePath() {
		t.Fatalf("session A InstancePath after constructing B = %q, want %q", sessionA.InstancePath(), identityA.InstancePath())
	}
}

func TestNewAdminSessionWithFactoriesClosesNothingWhenAdminCreationFails(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("admin client creation failed")
	session, err := newAdminSessionWithFactories(
		t.Context(),
		&systemVariables{
			Connection: ConnectionVars{
				Project:  "test-project",
				Instance: "test-instance",
			},
		},
		ConnectionVars{
			Project:  "test-project",
			Instance: "test-instance",
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			return nil, expectedErr
		},
	)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("newAdminSessionWithFactories() error = %v, want %v", err, expectedErr)
	}
	if session != nil {
		t.Fatalf("newAdminSessionWithFactories() session = %#v, want nil", session)
	}
}
