package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"strings"
	"testing"
	"time"
)

func TestGenerateCA(t *testing.T) {
	now := time.Now().UTC()
	ca, err := GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	if ca == nil {
		t.Fatal("expected non-nil CA bundle")
	}
	if ca.CertPEM == "" {
		t.Fatal("expected non-empty certificate PEM")
	}
	if ca.KeyPEM == "" {
		t.Fatal("expected non-empty key PEM")
	}
	if !strings.Contains(ca.CertPEM, "BEGIN CERTIFICATE") {
		t.Fatalf("invalid certificate PEM: %s", ca.CertPEM)
	}
	if !strings.Contains(ca.KeyPEM, "PRIVATE KEY") {
		t.Fatalf("invalid key PEM: %s", ca.KeyPEM)
	}

	if ca.Cert == nil {
		t.Fatal("expected parsed certificate")
	}
	if ca.Key == nil {
		t.Fatal("expected parsed private key")
	}
	if !ca.Cert.IsCA {
		t.Fatal("expected CA certificate to have IsCA=true")
	}
	if ca.Cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Fatal("expected CA certificate to have CertSign key usage")
	}
	if !strings.Contains(ca.Cert.Subject.CommonName, "test-cluster") {
		t.Fatalf("expected cluster ID in subject CN, got: %s", ca.Cert.Subject.CommonName)
	}
}

func TestIssueServerCert(t *testing.T) {
	now := time.Now().UTC()
	ca, err := GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	serverIP := "192.168.1.10"
	cert, err := IssueServerCert(ca, "test-cluster", serverIP, now)
	if err != nil {
		t.Fatalf("IssueServerCert failed: %v", err)
	}

	if cert == nil {
		t.Fatal("expected non-nil issued certificate")
	}
	if cert.CertPEM == "" {
		t.Fatal("expected non-empty certificate PEM")
	}
	if cert.KeyPEM == "" {
		t.Fatal("expected non-empty key PEM")
	}
	if cert.Serial == "" {
		t.Fatal("expected non-empty serial number")
	}
	if cert.Fingerprint == "" {
		t.Fatal("expected non-empty fingerprint")
	}
	if cert.NotBefore.IsZero() {
		t.Fatal("expected non-zero NotBefore")
	}
	if cert.NotAfter.IsZero() {
		t.Fatal("expected non-zero NotAfter")
	}

	// Verify certificate is signed by CA.
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	opts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if _, err := cert.Cert.Verify(opts); err != nil {
		t.Fatalf("certificate verification failed: %v", err)
	}

	// Check SANs include the server IP.
	found := false
	for _, ip := range cert.Cert.IPAddresses {
		if ip.String() == serverIP {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected server IP %s in SANs, got: %v", serverIP, cert.Cert.IPAddresses)
	}

	// New naming: CN should be "ployd-<cluster>", DNS SAN should be "ployd.<cluster>.ploy".
	if got, want := cert.Cert.Subject.CommonName, "ployd-test-cluster"; got != want {
		t.Fatalf("unexpected server cert CN: got %q want %q", got, want)
	}
	wantDNS := "ployd.test-cluster.ploy"
	hasDNS := false
	for _, dns := range cert.Cert.DNSNames {
		if dns == wantDNS {
			hasDNS = true
			break
		}
	}
	if !hasDNS {
		t.Fatalf("expected DNS SAN %q, got: %v", wantDNS, cert.Cert.DNSNames)
	}
}

func TestSignNodeCSR(t *testing.T) {
	now := time.Now().UTC()
	ca, err := GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Generate a CSR.
	nodeKey, err := ecdsa.GenerateKey(elliptic256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate node key: %v", err)
	}

	nodeIP := net.ParseIP("10.0.0.5")
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "node:test-node-1",
			Organization: []string{"Ploy"},
		},
		DNSNames:    []string{"node.test-node-1.test-cluster.ploy"},
		IPAddresses: []net.IP{nodeIP},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, nodeKey)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Sign the CSR.
	cert, err := SignNodeCSR(ca, csrPEM, now)
	if err != nil {
		t.Fatalf("SignNodeCSR failed: %v", err)
	}

	if cert == nil {
		t.Fatal("expected non-nil issued certificate")
	}
	if cert.CertPEM == "" {
		t.Fatal("expected non-empty certificate PEM")
	}
	if cert.Serial == "" {
		t.Fatal("expected non-empty serial number")
	}
	if cert.Fingerprint == "" {
		t.Fatal("expected non-empty fingerprint")
	}

	// Verify certificate is signed by CA.
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	opts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	if _, err := cert.Cert.Verify(opts); err != nil {
		t.Fatalf("certificate verification failed: %v", err)
	}

	// Verify the subject and SANs are preserved from CSR.
	if cert.Cert.Subject.CommonName != "node:test-node-1" {
		t.Fatalf("expected CN 'node:test-node-1', got: %s", cert.Cert.Subject.CommonName)
	}
	if len(cert.Cert.DNSNames) != 1 || cert.Cert.DNSNames[0] != "node.test-node-1.test-cluster.ploy" {
		t.Fatalf("expected DNS name preserved, got: %v", cert.Cert.DNSNames)
	}
	if len(cert.Cert.IPAddresses) != 1 || !cert.Cert.IPAddresses[0].Equal(nodeIP) {
		t.Fatalf("expected IP address preserved, got: %v", cert.Cert.IPAddresses)
	}

	// Verify ExtKeyUsage includes both client and server auth.
	hasClientAuth := false
	hasServerAuth := false
	for _, usage := range cert.Cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			hasClientAuth = true
		}
		if usage == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
	}
	if !hasClientAuth {
		t.Fatal("expected ExtKeyUsageClientAuth")
	}
	if !hasServerAuth {
		t.Fatal("expected ExtKeyUsageServerAuth")
	}
}

func TestSignNodeCSRInvalidPEM(t *testing.T) {
	now := time.Now().UTC()
	ca, err := GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	tests := []struct {
		name   string
		csrPEM []byte
	}{
		{"empty", []byte("")},
		{"garbage", []byte("not a PEM block")},
		{"wrong block type", []byte("-----BEGIN CERTIFICATE-----\ndata\n-----END CERTIFICATE-----")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SignNodeCSR(ca, tt.csrPEM, now)
			if err == nil {
				t.Fatal("expected error for invalid CSR PEM")
			}
		})
	}
}

func TestLoadCA(t *testing.T) {
	now := time.Now().UTC()
	original, err := GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Reload the CA from PEM strings.
	loaded, err := LoadCA(original.CertPEM, original.KeyPEM)
	if err != nil {
		t.Fatalf("LoadCA failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("expected non-nil CA bundle")
	}
	if loaded.CertPEM != original.CertPEM {
		t.Fatal("certificate PEM mismatch")
	}
	if loaded.KeyPEM != original.KeyPEM {
		t.Fatal("key PEM mismatch")
	}
	if loaded.Cert.SerialNumber.Cmp(original.Cert.SerialNumber) != 0 {
		t.Fatal("certificate serial number mismatch")
	}
	if !loaded.Key.Equal(original.Key) {
		t.Fatal("private key mismatch")
	}
}

func TestLoadCAInvalidPEM(t *testing.T) {
	tests := []struct {
		name    string
		certPEM string
		keyPEM  string
	}{
		{"empty cert", "", "-----BEGIN EC PRIVATE KEY-----\ndata\n-----END EC PRIVATE KEY-----"},
		{"empty key", "-----BEGIN CERTIFICATE-----\ndata\n-----END CERTIFICATE-----", ""},
		{"garbage cert", "not a PEM", "-----BEGIN EC PRIVATE KEY-----\ndata\n-----END EC PRIVATE KEY-----"},
		{"garbage key", "-----BEGIN CERTIFICATE-----\ndata\n-----END CERTIFICATE-----", "not a PEM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadCA(tt.certPEM, tt.keyPEM)
			if err == nil {
				t.Fatal("expected error for invalid PEM")
			}
		})
	}
}

func TestGenerateNodeCSR(t *testing.T) {
	nodeID := "node-abc123"
	clusterID := "cluster-xyz789"
	nodeIP := "192.168.1.20"

	keyBundle, csrPEM, err := GenerateNodeCSR(nodeID, clusterID, nodeIP)
	if err != nil {
		t.Fatalf("GenerateNodeCSR failed: %v", err)
	}

	// Check key bundle.
	if keyBundle == nil {
		t.Fatal("expected non-nil key bundle")
	}
	if keyBundle.KeyPEM == "" {
		t.Fatal("expected non-empty key PEM")
	}
	if keyBundle.Key == nil {
		t.Fatal("expected parsed private key")
	}

	// Check CSR PEM.
	if len(csrPEM) == 0 {
		t.Fatal("expected non-empty CSR PEM")
	}

	// Parse and validate the CSR.
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatal("expected valid CSR PEM block")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}

	// Verify CSR signature.
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature verification failed: %v", err)
	}

	// Check subject CN.
	expectedCN := "node:" + nodeID
	if csr.Subject.CommonName != expectedCN {
		t.Fatalf("expected CN %q, got: %q", expectedCN, csr.Subject.CommonName)
	}

	// Check DNS names.
	expectedDNS := "node-" + nodeID + "." + clusterID + ".ploy"
	if len(csr.DNSNames) != 1 || csr.DNSNames[0] != expectedDNS {
		t.Fatalf("expected DNS name %q, got: %v", expectedDNS, csr.DNSNames)
	}

	// Check IP addresses.
	expectedIP := net.ParseIP(nodeIP)
	if len(csr.IPAddresses) != 1 || !csr.IPAddresses[0].Equal(expectedIP) {
		t.Fatalf("expected IP %s, got: %v", nodeIP, csr.IPAddresses)
	}
}

func elliptic256() elliptic.Curve {
	return elliptic.P256()
}
