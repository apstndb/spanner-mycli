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
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/spanner/omni"
	"github.com/apstndb/spanner-mycli/internal/mycli/filesafety"
	"google.golang.org/api/option"
)

// tlsCertFileMaxSize caps a custom CA or client certificate/key read. PEM
// bundles are small; 1 MiB is enough for a chain and is not the credential-JSON
// cap (those bytes are a different authority layer).
const tlsCertFileMaxSize = 1 << 20

func explicitEndpointConfigured(opts *spannerOptions) bool {
	return strings.TrimSpace(opts.Endpoint) != "" ||
		strings.TrimSpace(opts.DeploymentEndpoint) != "" ||
		strings.TrimSpace(opts.Host) != ""
}

func customTLSFilesSpecified(opts *spannerOptions) bool {
	return strings.TrimSpace(opts.CaCertFile) != "" ||
		strings.TrimSpace(opts.ClientCertFile) != "" ||
		strings.TrimSpace(opts.ClientCertKey) != ""
}

func insecureRequested(opts *spannerOptions) bool {
	if opts.Insecure != nil {
		return *opts.Insecure
	}
	if opts.SkipTlsVerify != nil {
		return *opts.SkipTlsVerify
	}
	return false
}

// validateCustomTLSFlags checks cheap conflicts, pairing, and the explicit
// endpoint / --without-authentication contract. It does not read files or
// resolve credentials.
func validateCustomTLSFlags(opts *spannerOptions) error {
	hasTLS := customTLSFilesSpecified(opts)
	hasEndpoint := explicitEndpointConfigured(opts)
	cert := strings.TrimSpace(opts.ClientCertFile)
	key := strings.TrimSpace(opts.ClientCertKey)

	if (cert == "") != (key == "") {
		return fmt.Errorf("--client-cert-file and --client-cert-key must be set together")
	}

	if hasTLS && !hasEndpoint {
		return fmt.Errorf("custom TLS files require an explicit --endpoint or --host")
	}
	if hasTLS && insecureRequested(opts) {
		return fmt.Errorf("custom TLS files cannot be combined with --insecure or --skip-tls-verify")
	}
	if hasTLS && opts.usesEmbeddedRuntime() {
		return fmt.Errorf("custom TLS files cannot be combined with --embedded-emulator or --embedded-omni")
	}
	if hasTLS && os.Getenv("SPANNER_EMULATOR_HOST") != "" {
		return fmt.Errorf("custom TLS files cannot be combined with SPANNER_EMULATOR_HOST")
	}

	if opts.WithoutAuthentication {
		if !hasTLS {
			return fmt.Errorf("--without-authentication requires at least one of --ca-cert-file, --client-cert-file, or --client-cert-key")
		}
		if !hasEndpoint {
			return fmt.Errorf("--without-authentication requires an explicit --endpoint or --host")
		}
		if strings.TrimSpace(opts.Credential) != "" {
			return fmt.Errorf("--without-authentication cannot be combined with --credential")
		}
		if strings.TrimSpace(opts.ImpersonateServiceAccount) != "" {
			return fmt.Errorf("--without-authentication cannot be combined with --impersonate-service-account")
		}
	}

	return nil
}

func readTLSFileOrDiscard(path, label string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	_, err := filesafety.SafeReadFile(path, &filesafety.FileSafetyOptions{MaxSize: tlsCertFileMaxSize})
	if err != nil {
		return fmt.Errorf("read %s %q failed: %w", label, path, err)
	}
	return nil
}

// captureTLSTransport performs the one bounded pre-read of configured TLS
// files, then asks the pinned SDK to build immutable transport options. The
// pre-read bytes are discarded; the SDK load is the retained transport.
func captureTLSTransport(opts *spannerOptions) ([]option.ClientOption, error) {
	if !customTLSFilesSpecified(opts) {
		return nil, nil
	}
	ca := strings.TrimSpace(opts.CaCertFile)
	cert := strings.TrimSpace(opts.ClientCertFile)
	key := strings.TrimSpace(opts.ClientCertKey)
	if err := readTLSFileOrDiscard(ca, "CA certificate file"); err != nil {
		return nil, err
	}
	if err := readTLSFileOrDiscard(cert, "client certificate file"); err != nil {
		return nil, err
	}
	if err := readTLSFileOrDiscard(key, "client certificate key file"); err != nil {
		return nil, err
	}
	tlsOpts, err := omni.ConnectionOptions(false, ca, cert, key)
	if err != nil {
		return nil, fmt.Errorf("invalid custom TLS configuration: %w", err)
	}
	return tlsOpts, nil
}
