package nodeagent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/pki"
)

func TestBuildURLBasic(t *testing.T) {
	u, err := buildURL("https://server.example.com:8443", "/v1/nodes/x/heartbeat")
	if err != nil {
		t.Fatalf("buildURL error: %v", err)
	}
	want := "https://server.example.com:8443/v1/nodes/x/heartbeat"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
	}
}

func TestBuildURLTrailingSlash(t *testing.T) {
	u, err := buildURL("https://server.example.com:8443/", "/v1/foo")
	if err != nil {
		t.Fatalf("buildURL error: %v", err)
	}
	want := "https://server.example.com:8443/v1/foo"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
	}
}

func TestBuildURLEscapesNodeID(t *testing.T) {
	base := "https://server.example.com:8443"
	nodeID := "node/01 abc"
	p := path.Join("/v1/nodes", url.PathEscape(nodeID), "heartbeat")
	u, err := buildURL(base, p)
	if err != nil {
		t.Fatalf("buildURL error: %v", err)
	}
	want := "https://server.example.com:8443/v1/nodes/node%2F01%20abc/heartbeat"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
	}
}

func TestNewHTTPClientWithoutTLS(t *testing.T) {
	cfg := Config{
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		t.Fatalf("newHTTPClient error: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", client.Timeout, 30*time.Second)
	}

	if client.Transport != nil {
		t.Error("expected nil transport for non-TLS client")
	}
}

func TestNewHTTPClientWithTLSRequiresPaths(t *testing.T) {
	certPEM, keyPEM, _ := generateTestCerts(t)
	certPath := writeTempFile(t, certPEM)
	keyPath := writeTempFile(t, keyPEM)

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "missing_cert",
			cfg: Config{
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled:  true,
						CertPath: "",
						KeyPath:  "/tmp/key.pem",
						CAPath:   "/tmp/ca.pem",
					},
				},
			},
			wantErr: "load certificate",
		},
		{
			name: "missing_key",
			cfg: Config{
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled:  true,
						CertPath: "/tmp/cert.pem",
						KeyPath:  "",
						CAPath:   "/tmp/ca.pem",
					},
				},
			},
			wantErr: "load certificate",
		},
		{
			name: "missing_ca",
			cfg: Config{
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled:  true,
						CertPath: certPath,
						KeyPath:  keyPath,
						CAPath:   "",
					},
				},
			},
			wantErr: "load ca certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHTTPClient(tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantErr != "" && !contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestSendHeartbeatSuccess(t *testing.T) {
	var receivedPayload HeartbeatPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		expectedPath := "/v1/nodes/test-node-123/heartbeat"
		if r.URL.Path != expectedPath {
			t.Errorf("path = %s, want %s", r.URL.Path, expectedPath)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %s, want application/json", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body error: %v", err)
		}

		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Fatalf("unmarshal payload error: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := Config{
		NodeID:    "test-node-123",
		ServerURL: srv.URL,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
	}

	mgr, err := NewHeartbeatManager(cfg)
	if err != nil {
		t.Fatalf("NewHeartbeatManager error: %v", err)
	}

	ctx := context.Background()
	if err := mgr.sendHeartbeat(ctx); err != nil {
		t.Fatalf("sendHeartbeat error: %v", err)
	}

	if receivedPayload.NodeID != "test-node-123" {
		t.Errorf("node_id = %s, want test-node-123", receivedPayload.NodeID)
	}

	if receivedPayload.Timestamp.IsZero() {
		t.Error("timestamp is zero")
	}

	if receivedPayload.CPUTotalMilli <= 0 {
		t.Error("cpu_total_millis should be > 0")
	}

	if receivedPayload.MemTotalMB <= 0 {
		t.Error("mem_total_mb should be > 0")
	}

	if receivedPayload.DiskTotalMB <= 0 {
		t.Error("disk_total_mb should be > 0")
	}
}

func TestSendHeartbeatHandlesServerError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    string
	}{
		{
			name:       "bad_request",
			statusCode: http.StatusBadRequest,
			wantErr:    "heartbeat failed with status 400",
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			wantErr:    "heartbeat failed with status 401",
		},
		{
			name:       "internal_error",
			statusCode: http.StatusInternalServerError,
			wantErr:    "heartbeat failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			cfg := Config{
				NodeID:    "test-node",
				ServerURL: srv.URL,
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
				Heartbeat: HeartbeatConfig{
					Timeout: 10 * time.Second,
				},
			}

			mgr, err := NewHeartbeatManager(cfg)
			if err != nil {
				t.Fatalf("NewHeartbeatManager error: %v", err)
			}

			ctx := context.Background()
			err = mgr.sendHeartbeat(ctx)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestSendHeartbeatRespectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := Config{
		NodeID:    "test-node",
		ServerURL: srv.URL,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Timeout: 10 * time.Millisecond,
		},
	}

	mgr, err := NewHeartbeatManager(cfg)
	if err != nil {
		t.Fatalf("NewHeartbeatManager error: %v", err)
	}

	ctx := context.Background()
	err = mgr.sendHeartbeat(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestNewHTTPClientConfiguresMTLS(t *testing.T) {
	certPEM, keyPEM, caPEM := generateTestCerts(t)

	certPath := writeTempFile(t, certPEM)
	keyPath := writeTempFile(t, keyPEM)
	caPath := writeTempFile(t, caPEM)

	cfg := Config{
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled:  true,
				CertPath: certPath,
				KeyPath:  keyPath,
				CAPath:   caPath,
			},
		},
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		t.Fatalf("newHTTPClient error: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", client.Timeout, 30*time.Second)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}

	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLS config")
	}

	tlsConfig := transport.TLSClientConfig

	if tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("min version = %v, want TLS 1.3", tlsConfig.MinVersion)
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("certificates count = %d, want 1", len(tlsConfig.Certificates))
	}

	if tlsConfig.RootCAs == nil {
		t.Fatal("expected RootCAs to be set")
	}
}

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-cert-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()

	if _, err := f.Write(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	t.Cleanup(func() {
		os.Remove(f.Name())
	})

	return f.Name()
}

func generateTestCerts(t *testing.T) (certPEM, keyPEM, caPEM []byte) {
	t.Helper()

	now := time.Now().UTC()

	ca, err := pki.GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	nodeKey, nodeCSR, err := pki.GenerateNodeCSR("test-node", "test-cluster", "127.0.0.1")
	if err != nil {
		t.Fatalf("generate node CSR: %v", err)
	}

	nodeCert, err := pki.SignNodeCSR(ca, nodeCSR, now)
	if err != nil {
		t.Fatalf("sign node CSR: %v", err)
	}

	certPEM = []byte(nodeCert.CertPEM)
	keyPEM = []byte(nodeKey.KeyPEM)
	caPEM = []byte(ca.CertPEM)

	return certPEM, keyPEM, caPEM
}
