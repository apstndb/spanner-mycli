//
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
//

package mycli

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/samber/lo"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestValidateCustomTLSFlags(t *testing.T) {
	ca := filepath.Join(t.TempDir(), "ca.pem")
	cert := filepath.Join(t.TempDir(), "client.pem")
	key := filepath.Join(t.TempDir(), "client.key")

	tests := []struct {
		name    string
		opts    *spannerOptions
		env     map[string]string
		wantErr string
	}{
		{
			name: "no TLS is ok",
			opts: connectionOpts(nil),
		},
		{
			name: "ca with explicit endpoint",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = ca
			}),
		},
		{
			name: "ca with explicit host",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Host = "omni.example"
				o.CaCertFile = ca
			}),
		},
		{
			name: "ca with deployment-endpoint alias",
			opts: connectionOpts(func(o *spannerOptions) {
				o.DeploymentEndpoint = "omni.example:443"
				o.CaCertFile = ca
			}),
		},
		{
			name: "port only is not an explicit endpoint",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Port = 443
				o.CaCertFile = ca
			}),
			wantErr: "explicit --endpoint or --host",
		},
		{
			name: "client cert without key",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.ClientCertFile = cert
			}),
			wantErr: "must be set together",
		},
		{
			name: "client key without cert",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.ClientCertKey = key
			}),
			wantErr: "must be set together",
		},
		{
			name: "tls plus insecure",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = ca
				o.Insecure = lo.ToPtr(true)
			}),
			wantErr: "--insecure",
		},
		{
			name: "tls plus skip-tls-verify",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = ca
				o.SkipTlsVerify = lo.ToPtr(true)
			}),
			wantErr: "--insecure",
		},
		{
			name:    "tls plus embedded emulator",
			opts:    &spannerOptions{EmbeddedEmulator: true, CaCertFile: ca, Endpoint: "localhost:9010"},
			wantErr: "--embedded-emulator",
		},
		{
			name: "without-authentication without tls",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.WithoutAuthentication = true
			}),
			wantErr: "at least one of --ca-cert-file",
		},
		{
			name: "without-authentication with credential",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = ca
				o.WithoutAuthentication = true
				o.Credential = "/tmp/does-not-matter.json"
			}),
			wantErr: "--credential",
		},
		{
			name: "without-authentication with impersonation",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = ca
				o.WithoutAuthentication = true
				o.ImpersonateServiceAccount = "sa@example.com"
			}),
			wantErr: "--impersonate-service-account",
		},
		{
			name: "tls plus emulator host env",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = ca
			}),
			env:     map[string]string{"SPANNER_EMULATOR_HOST": "localhost:9010"},
			wantErr: "SPANNER_EMULATOR_HOST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			err := ValidateSpannerOptions(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateSpannerOptions() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateSpannerOptions() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestWithoutAuthenticationRejectsCredentialBeforeRead(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing-cred.json")
	_, err := runOffline(t, connectionOpts(func(o *spannerOptions) {
		o.Endpoint = "omni.example:443"
		o.CaCertFile = filepath.Join(t.TempDir(), "ca.pem")
		o.WithoutAuthentication = true
		o.Credential = missing
	}))
	if err == nil || !strings.Contains(err.Error(), "--credential") {
		t.Fatalf("error = %v, want --credential conflict before file read", err)
	}
	if strings.Contains(err.Error(), "failed to read the credential file") {
		t.Fatalf("credential file was read: %v", err)
	}
}

func TestInitializeCustomTLSCapturesTransportAndRejectsBadPEM(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ca, cert, key := mustWriteTLSBundle(t, dir, "ok", true)
	notPEM := filepath.Join(dir, "not.pem")
	if err := os.WriteFile(notPEM, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("valid capture", func(t *testing.T) {
		sysVars, err := initializeSystemVariables(connectionOpts(func(o *spannerOptions) {
			o.Endpoint = "omni.example:443"
			o.CaCertFile = ca
			o.ClientCertFile = cert
			o.ClientCertKey = key
			o.WithoutAuthentication = true
		}))
		if err != nil {
			t.Fatalf("initializeSystemVariables() error = %v", err)
		}
		if sysVars.Config.CaCertFile != ca || sysVars.Config.ClientCertFile != cert || sysVars.Config.ClientCertKey != key {
			t.Fatalf("paths not stored: %+v", sysVars.Config)
		}
		if !sysVars.Config.WithoutAuthentication {
			t.Fatal("WithoutAuthentication not stored")
		}
		if len(sysVars.Config.TLSClientOptions) == 0 {
			t.Fatal("TLSClientOptions not captured")
		}
	})

	t.Run("invalid PEM fails before factories", func(t *testing.T) {
		var clients, admins atomic.Int32
		_, err := initializeSystemVariables(connectionOpts(func(o *spannerOptions) {
			o.Endpoint = "omni.example:443"
			o.CaCertFile = notPEM
		}))
		if err == nil || !strings.Contains(err.Error(), "invalid custom TLS") {
			t.Fatalf("error = %v, want invalid custom TLS", err)
		}
		if clients.Load() != 0 || admins.Load() != 0 {
			t.Fatalf("factories called clients=%d admins=%d", clients.Load(), admins.Load())
		}
	})

	t.Run("missing file fails before factories", func(t *testing.T) {
		_, err := initializeSystemVariables(connectionOpts(func(o *spannerOptions) {
			o.Endpoint = "omni.example:443"
			o.CaCertFile = filepath.Join(dir, "missing-ca.pem")
		}))
		if err == nil || !strings.Contains(err.Error(), "CA certificate file") {
			t.Fatalf("error = %v, want CA certificate file read failure", err)
		}
	})
}

func TestCreateClientOptionsOrdinaryAuthAndTLS(t *testing.T) {
	t.Parallel()

	baseline := &systemVariables{
		Config: StartupConfig{
			EnableADCPlus: false,
			Host:          "spanner.googleapis.com",
			Port:          443,
		},
	}
	plain, err := createClientOptions(t.Context(), nil, baseline)
	if err != nil {
		t.Fatalf("baseline createClientOptions: %v", err)
	}

	dir := t.TempDir()
	ca, _, _ := mustWriteTLSBundle(t, dir, "cloud", true)
	sysVars, err := initializeSystemVariables(connectionOpts(func(o *spannerOptions) {
		o.Endpoint = "omni.example:443"
		o.CaCertFile = ca
	}))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sysVars.Config.EnableADCPlus = false
	withTLS, err := createClientOptions(t.Context(), nil, sysVars)
	if err != nil {
		t.Fatalf("tls createClientOptions: %v", err)
	}
	if len(withTLS) != len(plain)+len(sysVars.Config.TLSClientOptions) {
		t.Fatalf("tls option count = %d, baseline = %d, tls extras = %d", len(withTLS), len(plain), len(sysVars.Config.TLSClientOptions))
	}

	noTLS := &systemVariables{
		Config: StartupConfig{
			EnableADCPlus: false,
			Host:          "omni.example",
			Port:          443,
		},
	}
	got, err := createClientOptions(t.Context(), nil, noTLS)
	if err != nil {
		t.Fatalf("ordinary createClientOptions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ordinary Cloud option count = %d, want 1 (endpoint only, ADC disabled)", len(got))
	}

	cred := []byte(`{"type":"authorized_user","client_id":"operator","client_secret":"x","refresh_token":"y"}`)
	withCred, err := createClientOptions(t.Context(), cred, noTLS)
	if err != nil {
		t.Fatalf("credential createClientOptions: %v", err)
	}
	if len(withCred) != 2 {
		t.Fatalf("ordinary+credential option count = %d, want 2", len(withCred))
	}
}

func TestAuthOptionsStaySeparateFromSpannerWithoutAuthentication(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ca, _, _ := mustWriteTLSBundle(t, dir, "feat", true)
	sysVars, err := initializeSystemVariables(connectionOpts(func(o *spannerOptions) {
		o.Endpoint = "omni.example:443"
		o.CaCertFile = ca
		o.WithoutAuthentication = true
	}))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sysVars.Config.EnableADCPlus = false
	session := &Session{systemVariables: sysVars}

	spannerOpts, err := createAuthClientOptions(t.Context(), nil, sysVars, true)
	if err != nil {
		t.Fatalf("spanner auth: %v", err)
	}
	if len(spannerOpts) != 1 {
		t.Fatalf("spanner WithoutAuthentication option count = %d, want 1", len(spannerOpts))
	}

	featureOpts, err := session.AuthOptions(t.Context(), nil, false)
	if err != nil {
		t.Fatalf("feature AuthOptions: %v", err)
	}
	if len(featureOpts) != 0 {
		t.Fatalf("feature AuthOptions len = %d, want 0 (ADC path, not Spanner no-auth)", len(featureOpts))
	}
}

func TestCustomTLSClientOptionsSurviveFileRemoval(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ca, cert, key := mustWriteTLSBundle(t, dir, "reuse", true)
	sysVars, err := initializeSystemVariables(connectionOpts(func(o *spannerOptions) {
		o.Endpoint = "127.0.0.1:443"
		o.CaCertFile = ca
		o.ClientCertFile = cert
		o.ClientCertKey = key
		o.WithoutAuthentication = true
	}))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := os.Remove(ca); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cert); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	opts, err := createClientOptions(t.Context(), nil, sysVars)
	if err != nil {
		t.Fatalf("createClientOptions after deleting cert files: %v", err)
	}
	if len(opts) < 2 {
		t.Fatalf("expected captured TLS + endpoint options, got %d", len(opts))
	}
}

func TestStartupTLSVariablesAreReadOnly(t *testing.T) {
	t.Parallel()

	sysVars, err := initializeSystemVariables(connectionOpts(nil))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	for _, name := range []string{"CLI_CA_CERT_FILE", "CLI_CLIENT_CERT_FILE", "CLI_CLIENT_CERT_KEY", "CLI_WITHOUT_AUTHENTICATION"} {
		value := "TRUE"
		if name != "CLI_WITHOUT_AUTHENTICATION" {
			value = "/tmp/other.pem"
		}
		if err := sysVars.SetFromSimple(name, value); err == nil || !strings.Contains(err.Error(), "read-only") {
			t.Fatalf("SET %s: %v, want read-only", name, err)
		}
		if err := sysVars.Reset(name); err == nil || !strings.Contains(err.Error(), "does not support RESET") {
			t.Fatalf("RESET %s: %v, want RESET rejection", name, err)
		}
	}
	vars := sysVars.ListVariables()
	if vars["CLI_CLIENT_CERT_KEY"] != "" {
		t.Fatalf("empty key path SHOW = %q", vars["CLI_CLIENT_CERT_KEY"])
	}
}

func TestCreateClientOptionsTLSGRPC(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ca, serverCert, serverKey, clientCert, clientKey := mustWriteSharedTLSBundle(t, dir, "rpc")

	t.Run("trust and application RPC", func(t *testing.T) {
		srv := startTLSAdminServer(t, ca, serverCert, serverKey, false)
		sysVars := tlsSysVars(t, srv.addr, ca, "", "", true)
		if err := invokeAdminDDL(t, sysVars); err != nil {
			t.Fatalf("trusted RPC: %v", err)
		}
		if srv.rpcs.Load() == 0 {
			t.Fatal("server did not observe an application RPC")
		}
	})

	t.Run("wrong CA rejects before success", func(t *testing.T) {
		otherCA, _, _, _, _ := mustWriteSharedTLSBundle(t, t.TempDir(), "other")
		srv := startTLSAdminServer(t, ca, serverCert, serverKey, false)
		sysVars := tlsSysVars(t, srv.addr, otherCA, "", "", true)
		if err := invokeAdminDDL(t, sysVars); err == nil {
			t.Fatal("wrong CA RPC succeeded")
		}
		if srv.rpcs.Load() != 0 {
			t.Fatalf("server observed RPC with untrusted CA: %d", srv.rpcs.Load())
		}
	})

	t.Run("hostname mismatch rejects", func(t *testing.T) {
		dnsOnlyDir := t.TempDir()
		dnsCA, dnsServer, dnsKey, _, _ := writeTLSBundle(t, dnsOnlyDir, "dns", false)
		srv := startTLSAdminServer(t, dnsCA, dnsServer, dnsKey, false)
		sysVars := tlsSysVars(t, srv.addr, dnsCA, "", "", true)
		err := invokeAdminDDL(t, sysVars)
		if err == nil {
			t.Fatal("hostname mismatch RPC succeeded")
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "certificate") &&
			!strings.Contains(msg, "auth") &&
			!strings.Contains(msg, "127.0.0.1") &&
			!strings.Contains(msg, "localhost") &&
			!strings.Contains(msg, "unavailable") &&
			!strings.Contains(msg, "deadline") &&
			!strings.Contains(msg, "timeout") {
			t.Fatalf("hostname mismatch error = %v, want a TLS handshake or RPC failure", err)
		}
		if srv.rpcs.Load() != 0 {
			t.Fatalf("server observed application RPC on hostname mismatch: %d", srv.rpcs.Load())
		}
	})

	t.Run("mtls required with client cert", func(t *testing.T) {
		srv := startTLSAdminServer(t, ca, serverCert, serverKey, true)
		sysVars := tlsSysVars(t, srv.addr, ca, clientCert, clientKey, true)
		if err := invokeAdminDDL(t, sysVars); err != nil {
			t.Fatalf("mTLS RPC: %v", err)
		}
		if srv.peerCerts.Load() == 0 {
			t.Fatal("server did not see a client certificate")
		}
	})

	t.Run("mtls required without client cert", func(t *testing.T) {
		srv := startTLSAdminServer(t, ca, serverCert, serverKey, true)
		sysVars := tlsSysVars(t, srv.addr, ca, "", "", true)
		if err := invokeAdminDDL(t, sysVars); err == nil {
			t.Fatal("mTLS without client cert succeeded")
		}
		if srv.rpcs.Load() != 0 {
			t.Fatalf("application RPC ran without a client certificate: %d", srv.rpcs.Load())
		}
	})

	t.Run("no plaintext fallback", func(t *testing.T) {
		srv := startTLSAdminServer(t, ca, serverCert, serverKey, false)
		sysVars := tlsSysVars(t, srv.addr, ca, "", "", true)
		sysVars.Config.Insecure = true
		opts, err := createClientOptions(t.Context(), nil, sysVars)
		if err != nil {
			t.Fatalf("createClientOptions: %v", err)
		}
		opts = appendSessionClientOptions(sysVars, opts)
		client, err := adminapi.NewDatabaseAdminClient(t.Context(), opts...)
		if err != nil {
			// Construction may fail if last credentials win as plaintext against TLS.
			return
		}
		t.Cleanup(func() { _ = client.Close() })
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		_, err = client.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{Database: "projects/p/instances/i/databases/d"})
		if err == nil {
			t.Fatal("plaintext-appended options completed a TLS RPC")
		}
		if srv.rpcs.Load() != 0 {
			t.Fatalf("plaintext fallback reached the application handler: %d", srv.rpcs.Load())
		}
	})
}

func TestInvalidStartupDoesNotCallClientFactories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	notPEM := filepath.Join(dir, "not.pem")
	if err := os.WriteFile(notPEM, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		opts *spannerOptions
	}{
		{
			name: "unpaired client cert",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.ClientCertFile = filepath.Join(dir, "only-cert.pem")
			}),
		},
		{
			name: "missing CA file",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = filepath.Join(dir, "missing.pem")
			}),
		},
		{
			name: "invalid PEM",
			opts: connectionOpts(func(o *spannerOptions) {
				o.Endpoint = "omni.example:443"
				o.CaCertFile = notPEM
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clients, admins atomic.Int32
			err := startSessionCounting(t.Context(), tt.opts, &clients, &admins)
			if err == nil {
				t.Fatal("invalid startup succeeded")
			}
			if clients.Load() != 0 || admins.Load() != 0 {
				t.Fatalf("factories called after invalid startup: clients=%d admins=%d err=%v", clients.Load(), admins.Load(), err)
			}
		})
	}
}

func startSessionCounting(ctx context.Context, opts *spannerOptions, clients, admins *atomic.Int32) error {
	if err := ValidateSpannerOptions(opts); err != nil {
		return err
	}
	sysVars, err := initializeSystemVariables(opts)
	if err != nil {
		return err
	}
	sysVars.Config.EnableADCPlus = false
	_, err = createSessionCounting(ctx, sysVars, clients, admins)
	return err
}

func createSessionCounting(ctx context.Context, sysVars *systemVariables, clients, admins *atomic.Int32) (*Session, error) {
	opts, err := createClientOptions(ctx, nil, sysVars)
	if err != nil {
		return nil, err
	}
	return newSessionWithFactories(ctx, sysVars, sysVars.Connection,
		func(ctx context.Context, db string, cfg spanner.ClientConfig, _ ...option.ClientOption) (*spanner.Client, error) {
			clients.Add(1)
			return nil, errors.New("client factory should not run")
		},
		func(context.Context, ...option.ClientOption) (*adminapi.DatabaseAdminClient, error) {
			admins.Add(1)
			return nil, errors.New("admin factory should not run")
		},
		func(*spanner.Client) {},
		opts...,
	)
}

func tlsSysVars(t *testing.T, addr, ca, cert, key string, noAuth bool) *systemVariables {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	opts := connectionOpts(func(o *spannerOptions) {
		o.Host = host
		o.Port = mustAtoi(t, port)
		o.CaCertFile = ca
		o.ClientCertFile = cert
		o.ClientCertKey = key
		o.WithoutAuthentication = noAuth
	})
	sysVars, err := initializeSystemVariables(opts)
	if err != nil {
		t.Fatal(err)
	}
	sysVars.Config.EnableADCPlus = false
	return sysVars
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("port %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func invokeAdminDDL(t *testing.T, sysVars *systemVariables) error {
	t.Helper()
	opts, err := createClientOptions(t.Context(), nil, sysVars)
	if err != nil {
		return err
	}
	client, err := adminapi.NewDatabaseAdminClient(t.Context(), opts...)
	if err != nil {
		return err
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err = client.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{
		Database: "projects/p/instances/i/databases/d",
	}, gax.WithGRPCOptions(grpc.WaitForReady(false)))
	return err
}

type tlsAdminServer struct {
	adminpb.UnimplementedDatabaseAdminServer
	addr      string
	rpcs      atomic.Int32
	peerCerts atomic.Int32
}

func (s *tlsAdminServer) GetDatabaseDdl(ctx context.Context, _ *adminpb.GetDatabaseDdlRequest) (*adminpb.GetDatabaseDdlResponse, error) {
	s.rpcs.Add(1)
	if p, ok := peer.FromContext(ctx); ok {
		if info, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			s.peerCerts.Store(int32(len(info.State.PeerCertificates)))
		}
	}
	return &adminpb.GetDatabaseDdlResponse{Statements: []string{"CREATE TABLE T (K INT64) PRIMARY KEY (K)"}}, nil
}

func startTLSAdminServer(t *testing.T, caFile, certFile, keyFile string, requireClient bool) *tlsAdminServer {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if requireClient {
		cfg.ClientCAs = mustCertPool(t, caFile)
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(cfg)))
	srv := &tlsAdminServer{addr: ln.Addr().String()}
	adminpb.RegisterDatabaseAdminServer(gs, srv)
	go func() {
		if err := gs.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		gs.Stop()
		_ = ln.Close()
	})
	return srv
}

func mustCertPool(t *testing.T, caFile string) *x509.CertPool {
	t.Helper()
	b, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(b) {
		t.Fatal("append ca")
	}
	return p
}

func mustWriteTLSBundle(t *testing.T, dir, name string, includeIP bool) (caFile, certFile, keyFile string) {
	t.Helper()
	ca, server, skey, _, _ := writeTLSBundle(t, dir, name, includeIP)
	return ca, server, skey
}

func mustWriteSharedTLSBundle(t *testing.T, dir, name string) (caFile, serverCert, serverKey, clientCert, clientKey string) {
	t.Helper()
	return writeTLSBundle(t, dir, name, true)
}

func writeTLSBundle(t *testing.T, dir, name string, includeIP bool) (caFile, serverCert, serverKey, clientCert, clientKey string) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"issue-968-" + name}},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caFile = filepath.Join(dir, name+"-ca.pem")
	writeTestPEM(t, caFile, "CERTIFICATE", caDER, 0o644)
	serverCert, serverKey = writeTestLeaf(t, dir, name+"-server", x509.ExtKeyUsageServerAuth, caTmpl, caKey, includeIP)
	clientCert, clientKey = writeTestLeaf(t, dir, name+"-client", x509.ExtKeyUsageClientAuth, caTmpl, caKey, includeIP)
	return
}

func writeTestLeaf(t *testing.T, dir, name string, usage x509.ExtKeyUsage, caTmpl *x509.Certificate, caKey *ecdsa.PrivateKey, includeIP bool) (certFile, keyFile string) {
	t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{Organization: []string{name}, CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		DNSNames:     []string{"localhost"},
	}
	if includeIP {
		leafTmpl.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1)}
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	certFile = filepath.Join(dir, name+".pem")
	keyFile = filepath.Join(dir, name+".key")
	writeTestPEM(t, certFile, "CERTIFICATE", leafDER, 0o644)
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	writeTestPEM(t, keyFile, "EC PRIVATE KEY", keyDER, 0o600)
	return
}

func writeTestPEM(t *testing.T, path, typ string, der []byte, mode os.FileMode) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: der}); err != nil {
		t.Fatal(err)
	}
}

func TestHelpDocumentsCustomTLSAndPlaintextInsecure(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	_, _, err := parseFlagsArgs([]string{"--help"}, "built from source", nil, &stdout, io.Discard)
	if !isHelpRequested(err) {
		t.Fatalf("help err = %v", err)
	}
	help := stdout.String()
	for _, want := range []string{
		"--ca-cert-file",
		"--client-cert-file",
		"--client-cert-key",
		"--without-authentication",
		"Permit plaintext gRPC",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("full help missing %q", want)
		}
	}
	if strings.Contains(help, "Skip TLS verification and permit plaintext") {
		t.Error("old insecure skip-verify help is still present")
	}
}
