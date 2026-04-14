package nodeagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerTLSWiring(t *testing.T) {
	pk := generateTestPKI(t)
	paths := pk.writeFiles(t, t.TempDir())

	cfg := newAgentConfig("https://127.0.0.1:8443",
		withTLS(paths.ServerCert, paths.ServerKey, paths.CA),
		withListen("127.0.0.1:0"),
		withHTTPTimeouts(30*time.Second, 30*time.Second, 120*time.Second),
		withConcurrency(1),
	)

	server := startNodeServer(t, cfg)
	addr := server.Address()

	// Local helpers to reduce TLS client boilerplate across subtests.
	type clientOpt func(*tls.Config)
	withClientCert := func(cfg *tls.Config) {
		cert, err := tls.X509KeyPair([]byte(pk.NodeCert.CertPEM), []byte(pk.NodeKey.KeyPEM))
		if err != nil {
			t.Fatalf("load client cert: %v", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	withMaxVersion := func(v uint16) clientOpt {
		return func(cfg *tls.Config) { cfg.MaxVersion = v }
	}
	makeClient := func(t *testing.T, timeout time.Duration, opts ...clientOpt) *http.Client {
		t.Helper()
		pool := x509.NewCertPool()
		pool.AddCert(pk.CA.Cert)
		tlsCfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}
		for _, o := range opts {
			o(tlsCfg)
		}
		return &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: tlsCfg}}
	}

	t.Run("valid_client_cert", func(t *testing.T) {
		client := makeClient(t, 5*time.Second, withClientCert)

		resp, err := client.Get("https://" + addr + "/health")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body: %v", err)
		}
		if got := string(body); got != `{"status":"ok"}` {
			t.Errorf("expected body %q, got %q", `{"status":"ok"}`, got)
		}
	})

	t.Run("missing_client_cert", func(t *testing.T) {
		client := makeClient(t, 2*time.Second) // no client cert

		resp, err := client.Get("https://" + addr + "/health")
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			t.Fatal("expected error for missing client cert, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "handshake") {
			t.Logf("warning: unexpected error message: %v", err)
		}
	})

	t.Run("tls_version_enforcement", func(t *testing.T) {
		client := makeClient(t, 2*time.Second, withClientCert, withMaxVersion(tls.VersionTLS12))

		resp, err := client.Get("https://" + addr + "/health")
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			t.Fatal("expected error when using TLS 1.2, got nil")
		}
	})
}

func TestClientTLSWiring(t *testing.T) {
	pk := generateTestPKI(t)
	paths := pk.writeFiles(t, t.TempDir())

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/nodes/"+string(testNodeID)+"/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			http.Error(w, "no TLS", http.StatusBadRequest)
			return
		}
		if len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no client cert", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	addr := startTestTLSServer(t, pk.CA, pk.ServerCert, mux)

	cfg := newAgentConfig("https://"+addr,
		withTLS(paths.NodeCert, paths.NodeKey, paths.CA),
	)

	// Set up bearer token for createHTTPClient.
	tokenPath := filepath.Join(t.TempDir(), "bearer-token")
	if err := os.WriteFile(tokenPath, []byte("test-bearer-token"), 0600); err != nil {
		t.Fatalf("write bearer token: %v", err)
	}
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)

	client, err := createHTTPClient(cfg)
	if err != nil {
		t.Fatalf("create http client: %v", err)
	}

	resp, err := client.Get("https://" + addr + "/v1/nodes/" + string(testNodeID) + "/heartbeat")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.TLS == nil {
		t.Fatal("expected TLS connection, got nil")
	}
	if resp.TLS.Version != tls.VersionTLS13 {
		t.Errorf("expected TLS 1.3, got version %x", resp.TLS.Version)
	}
	if len(resp.TLS.PeerCertificates) == 0 {
		t.Fatal("expected server certificate in peer certificates")
	}
}

func TestTLSDisabled(t *testing.T) {
	cfg := newAgentConfig("http://127.0.0.1:8080",
		withNodeID("node-003"),
		withListen("127.0.0.1:0"),
		withHTTPTimeouts(30*time.Second, 30*time.Second, 120*time.Second),
		withConcurrency(1),
	)

	server := startNodeServer(t, cfg)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + server.Address() + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.TLS != nil {
		t.Error("expected no TLS connection, but TLS was used")
	}
}

func TestBootstrapTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		useSeparateCAs bool   // server uses different CA than client pins
		bootstrapCA    string // "correct", "wrong", or "" (triggers CAPath fallback)
		caPath         string // "correct", "nonexistent", or ""
		wantErr        bool
		wantErrSubstr  string
	}{
		{
			name:        "pinned_ca_success",
			bootstrapCA: "correct",
			caPath:      "nonexistent",
		},
		{
			name:           "pinned_ca_wrong_ca",
			useSeparateCAs: true,
			bootstrapCA:    "wrong",
			wantErr:        true,
			wantErrSubstr:  "certificate",
		},
		{
			name:   "ca_path_fallback",
			caPath: "correct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverPKI := generateTestPKI(t)
			tmpDir := t.TempDir()

			var bootstrapCAPath, caPath string
			switch tt.bootstrapCA {
			case "correct":
				bootstrapCAPath = writeTempFile(t, []byte(serverPKI.CA.CertPEM))
			case "wrong":
				wrongPKI := generateTestPKI(t)
				bootstrapCAPath = writeTempFile(t, []byte(wrongPKI.CA.CertPEM))
			}

			switch tt.caPath {
			case "correct":
				caPath = writeTempFile(t, []byte(serverPKI.CA.CertPEM))
			case "nonexistent":
				caPath = filepath.Join(tmpDir, "nonexistent-ca.crt")
			}

			addr := startTestTLSServer(t, nil, serverPKI.ServerCert, bootstrapHandler())

			cfg := newAgentConfig("https://"+addr, withBootstrapCA(bootstrapCAPath))
			cfg.HTTP.TLS.CAPath = caPath

			agent := &Agent{cfg: cfg}

			ctx := context.Background()
			if tt.wantErr {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
			}

			cert, caCert, err := agent.requestCertificate(ctx, "test-token", []byte("csr-data"))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErrSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("requestCertificate failed: %v", err)
			}
			if cert != "cert-data" {
				t.Errorf("expected cert=cert-data, got %s", cert)
			}
			if caCert != "ca-data" {
				t.Errorf("expected caCert=ca-data, got %s", caCert)
			}
		})
	}
}

func TestBootstrapTLS_CAFileErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		writeFile bool
		content   string
		errSubstr string
	}{
		{
			name:      "invalid_content",
			writeFile: true,
			content:   "not a valid certificate",
			errSubstr: "no valid certificates found",
		},
		{
			name:      "missing_file",
			writeFile: false,
			errSubstr: "read bootstrap CA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			caPath := filepath.Join(tmpDir, "ca.crt")

			if tt.writeFile {
				if err := os.WriteFile(caPath, []byte(tt.content), 0600); err != nil {
					t.Fatalf("write CA file: %v", err)
				}
			}

			cfg := newAgentConfig("https://127.0.0.1:9999", withBootstrapCA(caPath))

			agent := &Agent{cfg: cfg}
			_, _, err := agent.requestCertificate(context.Background(), "test-token", []byte("csr-data"))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("expected error containing %q, got: %v", tt.errSubstr, err)
			}
		})
	}
}
