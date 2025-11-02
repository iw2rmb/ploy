package main

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

	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
)

func TestResolveControlPlaneHTTP_PlainWhenNoDescriptor(t *testing.T) {
	t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(controlPlaneURLEnv, "http://127.0.0.1:9094")

	u, client, err := resolveControlPlaneHTTP(context.TODO())
	if err != nil {
		t.Fatalf("resolveControlPlaneHTTP error: %v", err)
	}
	if got, want := u.Scheme, "http"; got != want {
		t.Fatalf("scheme=%s want %s", got, want)
	}
	// Plain client path uses zero-value http.Client without a custom Transport.
	if client == nil || client.Transport != nil {
		t.Fatalf("expected plain client with nil Transport; got %#v", client)
	}
}

func TestResolveControlPlaneHTTP_WithMTLSDescriptorTLS13(t *testing.T) {
	// Prepare a temp config home and descriptor with CA + client cert/key.
	cfgHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(controlPlaneURLEnv, "https://127.0.0.1:8443")

	caCertPEM, caKeyPEM := generateCACert(t)
	clientCertPEM, clientKeyPEM := generateClientCert(t, caCertPEM, caKeyPEM)

	mustWrite := func(name string, b []byte) string {
		p := filepath.Join(cfgHome, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	caPath := mustWrite("ca.crt", caCertPEM)
	certPath := mustWrite("client.crt", clientCertPEM)
	keyPath := mustWrite("client.key", clientKeyPEM)

	// Save descriptor and mark default.
	if _, err := cliconfig.SaveDescriptor(cliconfig.Descriptor{
		ClusterID: "test-cluster",
		Address:   "127.0.0.1:8443",
		CAPath:    caPath,
		CertPath:  certPath,
		KeyPath:   keyPath,
	}); err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	if err := cliconfig.SetDefault("test-cluster"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	u, client, err := resolveControlPlaneHTTP(context.TODO())
	if err != nil {
		t.Fatalf("resolveControlPlaneHTTP error: %v", err)
	}
	if got, want := u.Scheme, "https"; got != want {
		t.Fatalf("scheme=%s want %s", got, want)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil {
		t.Fatalf("expected transport with TLS config; got %#v", client.Transport)
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion=%v want TLS1.3", tr.TLSClientConfig.MinVersion)
	}
	if len(tr.TLSClientConfig.Certificates) == 0 {
		t.Fatalf("expected client certificate loaded")
	}
	if tr.TLSClientConfig.RootCAs == nil {
		t.Fatalf("expected RootCAs to be populated")
	}
}

// ---- helpers ----

func generateCACert(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

func generateClientCert(t *testing.T, caCertPEM, caKeyPEM []byte) ([]byte, []byte) {
	t.Helper()
	caBlock, _ := pem.Decode(caCertPEM)
	if caBlock == nil {
		t.Fatal("decode CA cert")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil {
		t.Fatal("decode CA key")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA key: %v", err)
	}
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen client key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "test-client"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})
	return certPEM, keyPEM
}
