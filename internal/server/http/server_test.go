// Package httpserver_test contains infrastructure tests for the HTTP server.
//
// This file focuses on server lifecycle, TLS configuration, multiplexer API,
// and timeout behavior. Endpoint-specific tests (e.g., PKI, runs) are located
// in the handlers package (server_pki_test.go, server_runs_test.go).
//
// Test coverage:
//   - Server construction and validation (New)
//   - Start/Stop lifecycle and error cases
//   - Handler registration (HandleFunc, Handle)
//   - TLS/mTLS configuration and enforcement
//   - HTTP timeout settings (Read, Write, Idle)
//   - Address resolution and graceful shutdown
package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestNew verifies server construction with valid and invalid options.
// It ensures the authorizer is required and properly assigned.
func TestNew(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Create authorizer for testing (insecure mode allows requests without certs).
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: ":0",
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if srv == nil {
			t.Fatal("New() returned nil server")
		}
		if srv.authorizer != authorizer {
			t.Error("authorizer not set correctly")
		}
	})

	t.Run("error_missing_authorizer", func(t *testing.T) {
		// New() requires an authorizer; omitting it should fail fast.
		opts := Options{
			Config: config.HTTPConfig{
				Listen: ":0",
			},
		}
		srv, err := New(opts)
		if err == nil {
			t.Fatal("New() expected error for missing authorizer")
		}
		if srv != nil {
			t.Error("New() should return nil server on error")
		}
	})
}

// TestServer_StartStop validates server lifecycle management.
// It covers normal start/stop, double-start prevention, and idempotent stop.
func TestServer_StartStop(t *testing.T) {
	t.Run("plain_http", func(t *testing.T) {
		// Verify basic HTTP server startup and shutdown without TLS.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0", // OS-assigned port for parallel tests.
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Verify server is running and address is resolved.
		addr := srv.Addr()
		if addr == "" {
			t.Fatal("Addr() returned empty string")
		}

		// Stop the server gracefully.
		if err := srv.Stop(ctx); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}

		// Verify server state is updated after stop.
		if srv.running {
			t.Error("server still marked as running after Stop()")
		}
	})

	t.Run("already_running", func(t *testing.T) {
		// Verify Start() fails when called on a running server.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Attempt to start again should fail.
		if err := srv.Start(ctx); err == nil {
			t.Fatal("Start() expected error when already running")
		}
	})

	t.Run("stop_when_not_running", func(t *testing.T) {
		// Verify Stop() is idempotent and safe to call when not running.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		// Stop without starting should not error (idempotent behavior).
		if err := srv.Stop(ctx); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})
}

// TestServer_HandleFunc verifies the multiplexer API for handler registration.
// It validates both direct registration and role-based middleware enforcement.
func TestServer_HandleFunc(t *testing.T) {
	t.Run("without_middleware", func(t *testing.T) {
		// Verify basic handler registration without middleware.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		// Register a test handler without role restrictions.
		srv.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Make a request to verify handler is registered.
		resp, err := http.Get("http://" + srv.Addr() + "/test")
		if err != nil {
			t.Fatalf("GET /test error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("with_role_middleware", func(t *testing.T) {
		// Verify role-based access control via optional middleware.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane, // Insecure requests get ControlPlane role.
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		// Register handler requiring CLIAdmin role (higher than default).
		srv.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("admin"))
		}, auth.RoleCLIAdmin)

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Request should be forbidden (ControlPlane < CLIAdmin).
		resp, err := http.Get("http://" + srv.Addr() + "/admin")
		if err != nil {
			t.Fatalf("GET /admin error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", resp.StatusCode)
		}
	})
}

// TestServer_TLS validates TLS/mTLS configuration and enforcement.
// It verifies certificate loading, client CA validation, and TLS 1.3 enforcement.
func TestServer_TLS(t *testing.T) {
	t.Run("tls_enabled", func(t *testing.T) {
		// Verify server accepts TLS connections with valid client certificates.
		tmpDir := t.TempDir()

		// Get or create CA certificate.
		ca := getTestCA(t)
		caPath := filepath.Join(tmpDir, "ca.crt")
		if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: ca.cert.Raw,
		}), 0644); err != nil {
			t.Fatalf("write CA cert: %v", err)
		}

		// Generate server certificate signed by CA.
		serverCert, serverKey := generateCertificate(t, "server", ca.cert)
		serverCertPath := filepath.Join(tmpDir, "server.crt")
		serverKeyPath := filepath.Join(tmpDir, "server.key")
		writeCertAndKey(t, serverCertPath, serverKeyPath, serverCert, serverKey)

		// Generate client certificate signed by CA.
		clientCert, clientKey := generateCertificate(t, "client", ca.cert)
		clientCertPath := filepath.Join(tmpDir, "client.crt")
		clientKeyPath := filepath.Join(tmpDir, "client.key")
		writeCertAndKey(t, clientCertPath, clientKeyPath, clientCert, clientKey)

		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled:      true,
					CertPath:     serverCertPath,
					KeyPath:      serverKeyPath,
					ClientCAPath: caPath,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		srv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Load client certificate for mTLS.
		clientTLSCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			t.Fatalf("load client cert: %v", err)
		}

		// Make a TLS request with client certificate.
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates:       []tls.Certificate{clientTLSCert},
					InsecureSkipVerify: true,
				},
			},
		}
		resp, err := client.Get("https://" + srv.Addr() + "/health")
		if err != nil {
			t.Fatalf("GET /health error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("mtls_enabled", func(t *testing.T) {
		// Verify mTLS enforcement with RequireClientCert option.
		tmpDir := t.TempDir()

		// Get or create CA certificate.
		ca := getTestCA(t)
		caPath := filepath.Join(tmpDir, "ca.crt")
		if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: ca.cert.Raw,
		}), 0644); err != nil {
			t.Fatalf("write CA cert: %v", err)
		}

		// Generate server certificate signed by CA.
		serverCert, serverKey := generateCertificate(t, "server", ca.cert)
		serverCertPath := filepath.Join(tmpDir, "server.crt")
		serverKeyPath := filepath.Join(tmpDir, "server.key")
		writeCertAndKey(t, serverCertPath, serverKeyPath, serverCert, serverKey)

		// Generate client certificate signed by CA.
		clientCert, clientKey := generateCertificate(t, "client", ca.cert)
		clientCertPath := filepath.Join(tmpDir, "client.crt")
		clientKeyPath := filepath.Join(tmpDir, "client.key")
		writeCertAndKey(t, clientCertPath, clientKeyPath, clientCert, clientKey)

		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: false,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled:           true,
					CertPath:          serverCertPath,
					KeyPath:           serverKeyPath,
					ClientCAPath:      caPath,
					RequireClientCert: true,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		srv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Load client certificate for mTLS.
		clientTLSCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			t.Fatalf("load client cert: %v", err)
		}

		// Make an mTLS request.
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates:       []tls.Certificate{clientTLSCert},
					InsecureSkipVerify: true,
				},
			},
		}
		resp, err := client.Get("https://" + srv.Addr() + "/health")
		if err != nil {
			t.Fatalf("GET /health error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("missing_client_ca_when_tls_enabled", func(t *testing.T) {
		// Verify Start() fails when TLS is enabled but ClientCAPath is missing.
		// mTLS is mandatory when TLS is enabled for security.
		tmpDir := t.TempDir()

		serverCert, serverKey := generateCertificate(t, "server", nil)
		serverCertPath := filepath.Join(tmpDir, "server.crt")
		serverKeyPath := filepath.Join(tmpDir, "server.key")
		writeCertAndKey(t, serverCertPath, serverKeyPath, serverCert, serverKey)

		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled:  true,
					CertPath: serverCertPath,
					KeyPath:  serverKeyPath,
					// ClientCAPath is missing - mTLS is mandatory when TLS is enabled.
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		err = srv.Start(ctx)
		if err == nil {
			srv.Stop(ctx)
			t.Fatal("Start() expected error when client CA is not provided (mTLS is mandatory)")
		}
	})

	t.Run("tls13_enforcement", func(t *testing.T) {
		// Verify server enforces TLS 1.3 and rejects older versions.
		tmpDir := t.TempDir()

		ca := getTestCA(t)
		caPath := filepath.Join(tmpDir, "ca.crt")
		if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: ca.cert.Raw,
		}), 0644); err != nil {
			t.Fatalf("write CA cert: %v", err)
		}

		serverCert, serverKey := generateCertificate(t, "server", ca.cert)
		serverCertPath := filepath.Join(tmpDir, "server.crt")
		serverKeyPath := filepath.Join(tmpDir, "server.key")
		writeCertAndKey(t, serverCertPath, serverKeyPath, serverCert, serverKey)

		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: false,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled:      true,
					CertPath:     serverCertPath,
					KeyPath:      serverKeyPath,
					ClientCAPath: caPath,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		srv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			// Verify TLS version in request.
			if r.TLS == nil {
				t.Error("expected TLS connection, got nil")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if r.TLS.Version != tls.VersionTLS13 {
				t.Errorf("expected TLS 1.3, got version %x", r.TLS.Version)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Generate client certificate.
		clientCert, clientKey := generateCertificate(t, "client", ca.cert)
		clientCertPath := filepath.Join(tmpDir, "client.crt")
		clientKeyPath := filepath.Join(tmpDir, "client.key")
		writeCertAndKey(t, clientCertPath, clientKeyPath, clientCert, clientKey)

		clientTLSCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			t.Fatalf("load client cert: %v", err)
		}

		// Make a request with TLS 1.3.
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates:       []tls.Certificate{clientTLSCert},
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionTLS13,
					MaxVersion:         tls.VersionTLS13,
				},
			},
		}
		resp, err := client.Get("https://" + srv.Addr() + "/health")
		if err != nil {
			t.Fatalf("GET /health error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		// Verify that TLS 1.2 is rejected.
		clientTLS12 := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates:       []tls.Certificate{clientTLSCert},
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionTLS12,
					MaxVersion:         tls.VersionTLS12,
				},
			},
		}
		_, err = clientTLS12.Get("https://" + srv.Addr() + "/health")
		if err == nil {
			t.Error("expected TLS 1.2 connection to fail, but it succeeded")
		}
	})
}

// TestServer_Timeouts validates HTTP timeout configuration.
// It verifies default timeout application and custom timeout override behavior.
func TestServer_Timeouts(t *testing.T) {
	t.Run("default_timeouts", func(t *testing.T) {
		// Verify server applies safe default timeouts when not configured.
		// Per GOLANG.md, timeouts are mandatory for production servers.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled: false,
				},
				// No timeouts set - defaults should be applied.
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Verify default timeouts were applied.
		srv.mu.Lock()
		httpSrv := srv.httpServer
		srv.mu.Unlock()

		if httpSrv.ReadTimeout != 30*time.Second {
			t.Errorf("expected ReadTimeout 30s, got %v", httpSrv.ReadTimeout)
		}
		if httpSrv.WriteTimeout != 30*time.Second {
			t.Errorf("expected WriteTimeout 30s, got %v", httpSrv.WriteTimeout)
		}
		if httpSrv.IdleTimeout != 120*time.Second {
			t.Errorf("expected IdleTimeout 120s, got %v", httpSrv.IdleTimeout)
		}
	})

	t.Run("custom_timeouts", func(t *testing.T) {
		// Verify server respects custom timeout configuration.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen:       "127.0.0.1:0",
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  60 * time.Second,
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Verify custom timeouts were applied.
		srv.mu.Lock()
		httpSrv := srv.httpServer
		srv.mu.Unlock()

		if httpSrv.ReadTimeout != 5*time.Second {
			t.Errorf("expected ReadTimeout 5s, got %v", httpSrv.ReadTimeout)
		}
		if httpSrv.WriteTimeout != 10*time.Second {
			t.Errorf("expected WriteTimeout 10s, got %v", httpSrv.WriteTimeout)
		}
		if httpSrv.IdleTimeout != 60*time.Second {
			t.Errorf("expected IdleTimeout 60s, got %v", httpSrv.IdleTimeout)
		}
	})
}

// Helper functions for generating test certificates.

// generateCACertificate creates a self-signed CA certificate for testing.
// The CA is used to sign server and client certificates in TLS tests.
func generateCACertificate(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	// Generate RSA private key for the CA.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	// Create CA certificate template with cert signing capability.
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
			CommonName:   "Test CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour), // Allow for clock skew.
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Self-sign the CA certificate.
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	return cert, priv
}

// certPair holds a certificate and its private key for test CA scenarios.
type certPair struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

// testCACache caches a single CA for reuse across all TLS tests.
// This improves test performance by avoiding repeated CA generation.
var testCACache *certPair

// getTestCA returns a cached test CA, creating it on first call.
// Reusing the CA across tests is safe since tests run in isolated temp directories.
func getTestCA(t *testing.T) *certPair {
	t.Helper()
	if testCACache != nil {
		return testCACache
	}
	cert, key := generateCACertificate(t)
	testCACache = &certPair{cert: cert, key: key}
	return testCACache
}

// generateCertificate creates a certificate signed by the provided CA.
// If ca is nil, the certificate is self-signed. The certificate is valid
// for both server and client authentication (mTLS support).
func generateCertificate(t *testing.T, cn string, ca *x509.Certificate) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	// Generate private key for this certificate.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Generate random serial number (required for x509 validity).
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial number: %v", err)
	}

	// Create certificate template for server/client use.
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   cn,
		},
		NotBefore:   time.Now().Add(-1 * time.Hour), // Allow for clock skew.
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	// Determine parent certificate and signing key.
	parent := template
	signerKey := priv
	if ca != nil {
		parent = ca
		// Use cached CA key for signing (CA-signed certificate).
		signerKey = getTestCA(t).key
	}

	// Sign the certificate.
	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, &priv.PublicKey, signerKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	return cert, priv
}

// writeCertAndKey writes a certificate and private key to disk in PEM format.
// The certificate is world-readable (0644); the key is owner-only (0600).
func writeCertAndKey(t *testing.T, certPath, keyPath string, cert *x509.Certificate, key *rsa.PrivateKey) {
	t.Helper()

	// Encode and write certificate.
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	// Encode and write private key (restricted permissions).
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

// TestServer_Addr validates address resolution behavior.
// It verifies the server returns the configured address before start
// and the resolved address (with actual port) after start.
func TestServer_Addr(t *testing.T) {
	t.Run("before_start", func(t *testing.T) {
		// Before Start(), Addr() returns the configured listen address.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: ":8443",
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		addr := srv.Addr()
		if addr != ":8443" {
			t.Errorf("expected addr ':8443', got '%s'", addr)
		}
	})

	t.Run("after_start", func(t *testing.T) {
		// After Start(), Addr() returns the resolved address (port 0 becomes actual port).
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0", // Port 0 requests OS-assigned port.
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		addr := srv.Addr()
		if addr == "" || addr == "127.0.0.1:0" {
			t.Errorf("expected resolved addr, got '%s'", addr)
		}
	})
}

// TestServer_Handle verifies the Handle method for registering http.Handler.
// This complements HandleFunc by supporting the full http.Handler interface.
func TestServer_Handle(t *testing.T) {
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleControlPlane,
	})
	opts := Options{
		Config: config.HTTPConfig{
			Listen: "127.0.0.1:0",
			TLS: config.TLSConfig{
				Enabled: false,
			},
		},
		Authorizer: authorizer,
	}
	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Register a handler using Handle (http.Handler interface).
	srv.Handle("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer srv.Stop(ctx)

	resp, err := http.Get("http://" + srv.Addr() + "/test")
	if err != nil {
		t.Fatalf("GET /test error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestServer_GracefulShutdown verifies graceful shutdown behavior.
// It ensures in-flight requests complete before the server stops.
func TestServer_GracefulShutdown(t *testing.T) {
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleControlPlane,
	})
	opts := Options{
		Config: config.HTTPConfig{
			Listen: "127.0.0.1:0",
			TLS: config.TLSConfig{
				Enabled: false,
			},
		},
		Authorizer: authorizer,
	}
	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Register a slow handler to simulate in-flight request.
	srv.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Start a request in the background before shutdown.
	errChan := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + srv.Addr() + "/slow")
		if err != nil {
			errChan <- err
			return
		}
		resp.Body.Close()
		errChan <- nil
	}()

	// Give the request time to start processing.
	time.Sleep(10 * time.Millisecond)

	// Stop the server (should wait for in-flight request).
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify in-flight request completed or got expected error.
	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("request error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("request did not complete in time")
	}
}
