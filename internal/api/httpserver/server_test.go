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

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
)

func TestNew(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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

func TestServer_StartStop(t *testing.T) {
	t.Run("plain_http", func(t *testing.T) {
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

		// Verify server is running.
		addr := srv.Addr()
		if addr == "" {
			t.Fatal("Addr() returned empty string")
		}

		// Stop the server.
		if err := srv.Stop(ctx); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}

		// Verify server is stopped.
		if srv.running {
			t.Error("server still marked as running after Stop()")
		}
	})

	t.Run("already_running", func(t *testing.T) {
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

		// Try to start again.
		if err := srv.Start(ctx); err == nil {
			t.Fatal("Start() expected error when already running")
		}
	})

	t.Run("stop_when_not_running", func(t *testing.T) {
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
		// Stop without starting should not error.
		if err := srv.Stop(ctx); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})
}

func TestServer_HandleFunc(t *testing.T) {
	t.Run("without_middleware", func(t *testing.T) {
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

		// Register a test handler.
		srv.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Make a request.
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

		// Register a test handler with role restriction.
		srv.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("admin"))
		}, auth.RoleCLIAdmin)

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Make a request (should be forbidden since AllowInsecure gives RoleControlPlane).
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

func TestServer_TLS(t *testing.T) {
	t.Run("tls_enabled", func(t *testing.T) {
		// Create temporary directory for certificates.
		tmpDir := t.TempDir()

		// Generate server certificate.
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

		// Make a TLS request (skip verification for self-signed cert).
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
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
		// Create temporary directory for certificates.
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

	t.Run("missing_client_ca_when_required", func(t *testing.T) {
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
					Enabled:           true,
					CertPath:          serverCertPath,
					KeyPath:           serverKeyPath,
					RequireClientCert: true,
					// ClientCAPath is missing.
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
			t.Fatal("Start() expected error when client CA is required but not provided")
		}
	})
}

func TestServer_Timeouts(t *testing.T) {
	t.Run("default_timeouts", func(t *testing.T) {
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
				// No timeouts set.
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

		// Check that defaults were applied.
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

		// Check that custom timeouts were applied.
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

func generateCACertificate(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
			CommonName:   "Test CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

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

var testCACache *certPair

func getTestCA(t *testing.T) *certPair {
	t.Helper()
	if testCACache != nil {
		return testCACache
	}
	cert, key := generateCACertificate(t)
	testCACache = &certPair{cert: cert, key: key}
	return testCACache
}

func generateCertificate(t *testing.T, cn string, ca *x509.Certificate) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial number: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   cn,
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	parent := template
	signerKey := priv
	if ca != nil {
		parent = ca
		// Use cached CA key for signing.
		signerKey = getTestCA(t).key
	}

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

func writeCertAndKey(t *testing.T, certPath, keyPath string, cert *x509.Certificate, key *rsa.PrivateKey) {
	t.Helper()

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func TestServer_Addr(t *testing.T) {
	t.Run("before_start", func(t *testing.T) {
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

		addr := srv.Addr()
		if addr == "" || addr == "127.0.0.1:0" {
			t.Errorf("expected resolved addr, got '%s'", addr)
		}
	})
}

// TestServer_Handle verifies the Handle method works correctly.
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

	// Register a handler using Handle.
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

// TestServer_GracefulShutdown verifies that the server shuts down gracefully.
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

	// Register a slow handler.
	srv.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Start a request in the background.
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

	// Give the request time to start.
	time.Sleep(10 * time.Millisecond)

	// Stop the server.
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Wait for the request to complete.
	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("request error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("request did not complete in time")
	}
}
