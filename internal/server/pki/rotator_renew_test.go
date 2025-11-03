package pki_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corepki "github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/server/config"
	apipki "github.com/iw2rmb/ploy/internal/server/pki"
)

func TestDefaultRotator_Renew_WithEnvCA(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()

	// Create a CA and an initial server cert/key on disk
	ca, err := corepki.GenerateCA("test", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	issued, err := corepki.IssueServerCert(ca, "alpha", "127.0.0.1", now)
	if err != nil {
		t.Fatalf("issue server cert: %v", err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	if err := os.WriteFile(certPath, []byte(issued.CertPEM), 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte(issued.KeyPEM), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	// Force rotation by making RenewBefore larger than time until expiry
	// (until ~= 1y; choose 2y so renewal triggers immediately)
	cfg := config.PKIConfig{
		Certificate: certPath,
		Key:         keyPath,
		RenewBefore: 2 * 365 * 24 * time.Hour,
	}

	// Provide CA materials via env the rotator expects
	t.Setenv("PLOY_SERVER_CA_CERT", ca.CertPEM)
	t.Setenv("PLOY_SERVER_CA_KEY", ca.KeyPEM)

	rot := apipki.NewDefaultRotator(nil)
	if err := rot.Renew(context.Background(), cfg); err != nil {
		t.Fatalf("renew: %v", err)
	}

	// Read the rotated cert and verify NotAfter moved forward and SANs preserved
	newPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read rotated cert: %v", err)
	}
	newCert, err := parseX509Cert(newPEM)
	if err != nil {
		t.Fatalf("parse rotated cert: %v", err)
	}
	if !newCert.NotAfter.After(issued.NotAfter) {
		t.Fatalf("expected NotAfter advanced, got old=%v new=%v", issued.NotAfter, newCert.NotAfter)
	}
	// Subject should be preserved
	if newCert.Subject.String() != issued.Cert.Subject.String() {
		t.Fatalf("subject mismatch: old=%q new=%q", issued.Cert.Subject.String(), newCert.Subject.String())
	}
	// IP SAN should include 127.0.0.1
	found127 := false
	for _, ip := range newCert.IPAddresses {
		if ip.String() == "127.0.0.1" {
			found127 = true
			break
		}
	}
	if !found127 {
		t.Fatalf("expected rotated cert to include 127.0.0.1 in IP SANs")
	}
}

func parseX509Cert(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid CERTIFICATE PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
