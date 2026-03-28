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

	"github.com/iw2rmb/ploy/internal/pki"
)

func TestServerTLSWiring(t *testing.T) {
	// Generate test CA and certificates.
	now := time.Now().UTC()
	ca, err := pki.GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	// Issue server certificate.
	serverCert, err := pki.IssueServerCert(ca, "test-cluster", "127.0.0.1", now)
	if err != nil {
		t.Fatalf("issue server cert: %v", err)
	}

	// Generate node certificate (client cert).
	nodeKey, nodeCSR, err := pki.GenerateNodeCSR("node-001", "test-cluster", "127.0.0.1")
	if err != nil {
		t.Fatalf("generate node CSR: %v", err)
	}
	nodeCert, err := pki.SignNodeCSR(ca, nodeCSR, now)
	if err != nil {
		t.Fatalf("sign node CSR: %v", err)
	}

	// Write certificates to temp directory.
	tmpDir := t.TempDir()
	serverCertPath := filepath.Join(tmpDir, "server.crt")
	serverKeyPath := filepath.Join(tmpDir, "server.key")
	nodeCertPath := filepath.Join(tmpDir, "node.crt")
	nodeKeyPath := filepath.Join(tmpDir, "node.key")
	caPath := filepath.Join(tmpDir, "ca.crt")

	if err := os.WriteFile(serverCertPath, []byte(serverCert.CertPEM), 0600); err != nil {
		t.Fatalf("write server cert: %v", err)
	}
	if err := os.WriteFile(serverKeyPath, []byte(serverCert.KeyPEM), 0600); err != nil {
		t.Fatalf("write server key: %v", err)
	}
	if err := os.WriteFile(nodeCertPath, []byte(nodeCert.CertPEM), 0600); err != nil {
		t.Fatalf("write node cert: %v", err)
	}
	if err := os.WriteFile(nodeKeyPath, []byte(nodeKey.KeyPEM), 0600); err != nil {
		t.Fatalf("write node key: %v", err)
	}
	if err := os.WriteFile(caPath, []byte(ca.CertPEM), 0600); err != nil {
		t.Fatalf("write ca cert: %v", err)
	}

	// Create config with TLS enabled.
	cfg := Config{
		ServerURL: "https://127.0.0.1:8443",
		NodeID:    "node-001",
		HTTP: HTTPConfig{
			Listen: "127.0.0.1:0", // Random port
			TLS: TLSConfig{
				Enabled:  true,
				CertPath: serverCertPath,
				KeyPath:  serverKeyPath,
				CAPath:   caPath,
			},
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Concurrency: 1,
		Heartbeat: HeartbeatConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
	}

	// Create a mock controller.
	controller := &mockController{}

	server, err := NewServer(cfg, controller)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Stop(shutdownCtx)
	}()

	addr := server.Address()
	if addr == "" {
		t.Fatal("server address is empty")
	}

	// Test 1: Request with valid client certificate should succeed.
	t.Run("valid_client_cert", func(t *testing.T) {
		clientCert, err := tls.X509KeyPair([]byte(nodeCert.CertPEM), []byte(nodeKey.KeyPEM))
		if err != nil {
			t.Fatalf("load client cert: %v", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AddCert(ca.Cert)

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      caCertPool,
			MinVersion:   tls.VersionTLS13,
		}

		client := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}

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

		expected := `{"status":"ok"}`
		if string(body) != expected {
			t.Errorf("expected body %q, got %q", expected, string(body))
		}
	})

	// Test 2: Request without client certificate should fail.
	t.Run("missing_client_cert", func(t *testing.T) {
		caCertPool := x509.NewCertPool()
		caCertPool.AddCert(ca.Cert)

		tlsConfig := &tls.Config{
			RootCAs:    caCertPool,
			MinVersion: tls.VersionTLS13,
		}

		client := &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}

		resp, err := client.Get("https://" + addr + "/health")
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			t.Fatal("expected error for missing client cert, got nil")
		}

		// TLS handshake should fail due to missing client certificate.
		if err != nil && !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "handshake") {
			t.Logf("warning: unexpected error message: %v", err)
		}
	})

	// Test 3: Verify TLS 1.3 is enforced.
	t.Run("tls_version_enforcement", func(t *testing.T) {
		clientCert, err := tls.X509KeyPair([]byte(nodeCert.CertPEM), []byte(nodeKey.KeyPEM))
		if err != nil {
			t.Fatalf("load client cert: %v", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AddCert(ca.Cert)

		// Try to connect with TLS 1.2 (should fail).
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      caCertPool,
			MaxVersion:   tls.VersionTLS12,
		}

		client := &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}

		resp, err := client.Get("https://" + addr + "/health")
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			t.Fatal("expected error when using TLS 1.2, got nil")
		}
	})
}

func TestClientTLSWiring(t *testing.T) {
	// Generate test CA and certificates.
	now := time.Now().UTC()
	ca, err := pki.GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	// Issue server certificate.
	serverCert, err := pki.IssueServerCert(ca, "test-cluster", "127.0.0.1", now)
	if err != nil {
		t.Fatalf("issue server cert: %v", err)
	}

	// Generate node certificate (client cert).
	nodeKey, nodeCSR, err := pki.GenerateNodeCSR("node-002", "test-cluster", "127.0.0.1")
	if err != nil {
		t.Fatalf("generate node CSR: %v", err)
	}
	nodeCert, err := pki.SignNodeCSR(ca, nodeCSR, now)
	if err != nil {
		t.Fatalf("sign node CSR: %v", err)
	}

	// Write certificates to temp directory.
	tmpDir := t.TempDir()
	nodeCertPath := filepath.Join(tmpDir, "node.crt")
	nodeKeyPath := filepath.Join(tmpDir, "node.key")
	caPath := filepath.Join(tmpDir, "ca.crt")

	if err := os.WriteFile(nodeCertPath, []byte(nodeCert.CertPEM), 0600); err != nil {
		t.Fatalf("write node cert: %v", err)
	}
	if err := os.WriteFile(nodeKeyPath, []byte(nodeKey.KeyPEM), 0600); err != nil {
		t.Fatalf("write node key: %v", err)
	}
	if err := os.WriteFile(caPath, []byte(ca.CertPEM), 0600); err != nil {
		t.Fatalf("write ca cert: %v", err)
	}

	// Set up a test HTTPS server that requires client certificates.
	testServerAddr := "127.0.0.1:0"
	testMux := http.NewServeMux()
	testMux.HandleFunc("/v1/nodes/node-002/heartbeat", func(w http.ResponseWriter, r *http.Request) {
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

	tlsCert, err := tls.X509KeyPair([]byte(serverCert.CertPEM), []byte(serverCert.KeyPEM))
	if err != nil {
		t.Fatalf("load server cert: %v", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(ca.Cert)

	testServerTLS := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS13,
	}

	testServer := &http.Server{
		Handler:   testMux,
		TLSConfig: testServerTLS,
	}

	listener, err := tls.Listen("tcp", testServerAddr, testServerTLS)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		_ = testServer.Serve(listener)
	}()

	serverURL := "https://" + listener.Addr().String()

	// Create node config with TLS enabled.
	cfg := Config{
		ServerURL: serverURL,
		NodeID:    "node-002",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled:  true,
				CertPath: nodeCertPath,
				KeyPath:  nodeKeyPath,
				CAPath:   caPath,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 30 * time.Second,
			Timeout:  5 * time.Second,
		},
	}

	// Set up bearer token for createHTTPClient.
	tokenPath := filepath.Join(tmpDir, "bearer-token")
	if err := os.WriteFile(tokenPath, []byte("test-bearer-token"), 0600); err != nil {
		t.Fatalf("write bearer token: %v", err)
	}
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)

	// Test that createHTTPClient creates a client with correct TLS config.
	client, err := createHTTPClient(cfg)
	if err != nil {
		t.Fatalf("create http client: %v", err)
	}

	// Make a request to verify TLS handshake succeeds.
	resp, err := client.Get(serverURL + "/v1/nodes/node-002/heartbeat")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify TLS connection state.
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
	cfg := Config{
		ServerURL: "http://127.0.0.1:8080",
		NodeID:    "node-003",
		HTTP: HTTPConfig{
			Listen: "127.0.0.1:0",
			TLS: TLSConfig{
				Enabled: false,
			},
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Concurrency: 1,
		Heartbeat: HeartbeatConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
	}

	controller := &mockController{}

	server, err := NewServer(cfg, controller)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Stop(shutdownCtx)
	}()

	addr := server.Address()
	if addr == "" {
		t.Fatal("server address is empty")
	}

	// Plain HTTP request should work.
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify TLS was not used.
	if resp.TLS != nil {
		t.Error("expected no TLS connection, but TLS was used")
	}
}

// TestBootstrapTLS_PinnedCA verifies that requestCertificate uses the configured
// BootstrapCAPath to verify the server during bootstrap.
func TestBootstrapTLS_PinnedCA(t *testing.T) {
	// Generate test CA and server certificate.
	now := time.Now().UTC()
	ca, err := pki.GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	serverCert, err := pki.IssueServerCert(ca, "test-cluster", "127.0.0.1", now)
	if err != nil {
		t.Fatalf("issue server cert: %v", err)
	}

	// Write CA to temp file.
	tmpDir := t.TempDir()
	bootstrapCAPath := filepath.Join(tmpDir, "bootstrap-ca.crt")
	if err := os.WriteFile(bootstrapCAPath, []byte(ca.CertPEM), 0600); err != nil {
		t.Fatalf("write bootstrap CA: %v", err)
	}

	// Set up HTTPS test server using the generated certificate.
	tlsCert, err := tls.X509KeyPair([]byte(serverCert.CertPEM), []byte(serverCert.KeyPEM))
	if err != nil {
		t.Fatalf("load server cert: %v", err)
	}

	testMux := http.NewServeMux()
	testMux.HandleFunc("/v1/pki/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"certificate":"cert-data","ca_bundle":"ca-data"}`))
	})

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLSConfig)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	testServer := &http.Server{Handler: testMux}
	go func() { _ = testServer.Serve(listener) }()

	// Configure agent with BootstrapCAPath pointing to our test CA.
	cfg := Config{
		ServerURL: "https://" + listener.Addr().String(),
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled:         true,
				BootstrapCAPath: bootstrapCAPath,
				CAPath:          filepath.Join(tmpDir, "nonexistent-ca.crt"), // Doesn't exist.
			},
		},
	}

	agent := &Agent{cfg: cfg}
	ctx := context.Background()

	// Request should succeed because we verify with the pinned CA.
	cert, caCert, err := agent.requestCertificate(ctx, "test-token", []byte("csr-data"))
	if err != nil {
		t.Fatalf("requestCertificate failed: %v", err)
	}

	if cert != "cert-data" {
		t.Errorf("expected cert=cert-data, got %s", cert)
	}
	if caCert != "ca-data" {
		t.Errorf("expected caCert=ca-data, got %s", caCert)
	}
}

// TestBootstrapTLS_PinnedCA_WrongCA verifies that requestCertificate fails when
// the server presents a certificate not signed by the configured BootstrapCAPath.
func TestBootstrapTLS_PinnedCA_WrongCA(t *testing.T) {
	// Generate two different CAs.
	now := time.Now().UTC()
	serverCA, err := pki.GenerateCA("server-cluster", now)
	if err != nil {
		t.Fatalf("generate server CA: %v", err)
	}
	clientCA, err := pki.GenerateCA("client-cluster", now)
	if err != nil {
		t.Fatalf("generate client CA: %v", err)
	}

	// Server uses serverCA.
	serverCert, err := pki.IssueServerCert(serverCA, "server-cluster", "127.0.0.1", now)
	if err != nil {
		t.Fatalf("issue server cert: %v", err)
	}

	// Client is configured with clientCA (different from serverCA).
	tmpDir := t.TempDir()
	bootstrapCAPath := filepath.Join(tmpDir, "wrong-ca.crt")
	if err := os.WriteFile(bootstrapCAPath, []byte(clientCA.CertPEM), 0600); err != nil {
		t.Fatalf("write bootstrap CA: %v", err)
	}

	// Set up HTTPS test server.
	tlsCert, err := tls.X509KeyPair([]byte(serverCert.CertPEM), []byte(serverCert.KeyPEM))
	if err != nil {
		t.Fatalf("load server cert: %v", err)
	}

	testMux := http.NewServeMux()
	testMux.HandleFunc("/v1/pki/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"certificate":"cert-data","ca_bundle":"ca-data"}`))
	})

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLSConfig)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	testServer := &http.Server{Handler: testMux}
	go func() { _ = testServer.Serve(listener) }()

	// Configure agent with wrong BootstrapCAPath.
	cfg := Config{
		ServerURL: "https://" + listener.Addr().String(),
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled:         true,
				BootstrapCAPath: bootstrapCAPath,
			},
		},
	}

	agent := &Agent{cfg: cfg}

	// Use a short timeout context to avoid long waits on retries.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Request should fail because server cert is not signed by the pinned CA.
	_, _, err = agent.requestCertificate(ctx, "test-token", []byte("csr-data"))
	if err == nil {
		t.Fatal("expected TLS verification error, got nil")
	}

	// Verify the error is related to certificate verification.
	if !strings.Contains(err.Error(), "certificate") {
		t.Logf("error message: %v", err)
	}
}

// TestBootstrapTLS_CAPathFallback verifies that when BootstrapCAPath is empty
// but CAPath exists, the cluster CA is used for bootstrap TLS verification.
func TestBootstrapTLS_CAPathFallback(t *testing.T) {
	// Generate test CA and server certificate.
	now := time.Now().UTC()
	ca, err := pki.GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	serverCert, err := pki.IssueServerCert(ca, "test-cluster", "127.0.0.1", now)
	if err != nil {
		t.Fatalf("issue server cert: %v", err)
	}

	// Write CA to CAPath (simulating previous bootstrap).
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "ca.crt")
	if err := os.WriteFile(caPath, []byte(ca.CertPEM), 0600); err != nil {
		t.Fatalf("write CA: %v", err)
	}

	// Set up HTTPS test server.
	tlsCert, err := tls.X509KeyPair([]byte(serverCert.CertPEM), []byte(serverCert.KeyPEM))
	if err != nil {
		t.Fatalf("load server cert: %v", err)
	}

	testMux := http.NewServeMux()
	testMux.HandleFunc("/v1/pki/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"certificate":"cert-data","ca_bundle":"ca-data"}`))
	})

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLSConfig)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	testServer := &http.Server{Handler: testMux}
	go func() { _ = testServer.Serve(listener) }()

	// Configure agent without BootstrapCAPath but with existing CAPath.
	cfg := Config{
		ServerURL: "https://" + listener.Addr().String(),
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled:         true,
				BootstrapCAPath: "", // Empty - should fall back to CAPath.
				CAPath:          caPath,
			},
		},
	}

	agent := &Agent{cfg: cfg}
	ctx := context.Background()

	// Request should succeed using CAPath as fallback.
	cert, caCert, err := agent.requestCertificate(ctx, "test-token", []byte("csr-data"))
	if err != nil {
		t.Fatalf("requestCertificate failed: %v", err)
	}

	if cert != "cert-data" {
		t.Errorf("expected cert=cert-data, got %s", cert)
	}
	if caCert != "ca-data" {
		t.Errorf("expected caCert=ca-data, got %s", caCert)
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

			cfg := Config{
				ServerURL: "https://127.0.0.1:9999",
				NodeID:    testNodeID,
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled:         true,
						BootstrapCAPath: caPath,
					},
				},
			}

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
