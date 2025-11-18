package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// TLS tests cover TLS/mTLS configuration, certificate loading, and TLS policy.

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
		// Verify mTLS enforcement when TLS is enabled (mTLS is mandatory).
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

	t.Run("tls_version_policy", func(t *testing.T) {
		// Verify TLS 1.3 is enforced and TLS 1.2 is rejected.
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

	t.Run("invalid_server_cert_path", func(t *testing.T) {
		// Start should fail if server certificate path is invalid.
		tmpDir := t.TempDir()
		ca := getTestCA(t)
		caPath := filepath.Join(tmpDir, "ca.crt")
		if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}), 0644); err != nil {
			t.Fatalf("write CA cert: %v", err)
		}

		authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: false, DefaultRole: auth.RoleControlPlane})
		opts := Options{Config: config.HTTPConfig{Listen: "127.0.0.1:0", TLS: config.TLSConfig{Enabled: true, CertPath: filepath.Join(tmpDir, "nope.crt"), KeyPath: filepath.Join(tmpDir, "nope.key"), ClientCAPath: caPath}}, Authorizer: authorizer}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if err := srv.Start(context.Background()); err == nil {
			srv.Stop(context.Background())
			t.Fatal("Start() expected error for invalid server certificate/key paths")
		}
	})

	t.Run("invalid_client_ca_path", func(t *testing.T) {
		// Start should fail if client CA path does not exist.
		tmpDir := t.TempDir()
		// Create a valid server cert/key so we hit the CA path branch next.
		serverCert, serverKey := generateCertificate(t, "server", nil)
		serverCertPath := filepath.Join(tmpDir, "server.crt")
		serverKeyPath := filepath.Join(tmpDir, "server.key")
		writeCertAndKey(t, serverCertPath, serverKeyPath, serverCert, serverKey)

		authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: false, DefaultRole: auth.RoleControlPlane})
		opts := Options{Config: config.HTTPConfig{Listen: "127.0.0.1:0", TLS: config.TLSConfig{Enabled: true, CertPath: serverCertPath, KeyPath: serverKeyPath, ClientCAPath: filepath.Join(tmpDir, "missing-ca.crt")}}, Authorizer: authorizer}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if err := srv.Start(context.Background()); err == nil {
			srv.Stop(context.Background())
			t.Fatal("Start() expected error for missing client CA path")
		}
	})

	t.Run("invalid_client_ca_parse", func(t *testing.T) {
		// Start should fail if client CA file cannot be parsed as PEM.
		tmpDir := t.TempDir()

		// Write valid server cert/key.
		serverCert, serverKey := generateCertificate(t, "server", nil)
		serverCertPath := filepath.Join(tmpDir, "server.crt")
		serverKeyPath := filepath.Join(tmpDir, "server.key")
		writeCertAndKey(t, serverCertPath, serverKeyPath, serverCert, serverKey)

		// Write invalid CA file
		badCAPath := filepath.Join(tmpDir, "bad-ca.crt")
		if err := os.WriteFile(badCAPath, []byte("not-a-pem"), 0644); err != nil {
			t.Fatalf("write bad CA: %v", err)
		}

		authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: false, DefaultRole: auth.RoleControlPlane})
		opts := Options{Config: config.HTTPConfig{Listen: "127.0.0.1:0", TLS: config.TLSConfig{Enabled: true, CertPath: serverCertPath, KeyPath: serverKeyPath, ClientCAPath: badCAPath}}, Authorizer: authorizer}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if err := srv.Start(context.Background()); err == nil {
			srv.Stop(context.Background())
			t.Fatal("Start() expected error for unparsable client CA certificate")
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
