package main

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
)

func TestServerDeployCAGeneration(t *testing.T) {
	clusterID, err := deploy.GenerateClusterID()
	if err != nil {
		t.Fatalf("GenerateClusterID failed: %v", err)
	}
	if clusterID == "" || !regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9]{4}$`).MatchString(clusterID) {
		t.Fatalf("unexpected cluster ID: %q", clusterID)
	}
	now := time.Now()
	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}
	if ca == nil || !strings.Contains(ca.CertPEM, "BEGIN CERTIFICATE") || !strings.Contains(ca.KeyPEM, "PRIVATE KEY") {
		t.Fatal("invalid CA bundle")
	}
	serverAddress := "192.168.1.10"
	serverCert, err := pki.IssueServerCert(ca, clusterID, serverAddress, now)
	if err != nil {
		t.Fatalf("IssueServerCert failed: %v", err)
	}
	if serverCert == nil || serverCert.CertPEM == "" || serverCert.KeyPEM == "" || serverCert.Serial == "" || serverCert.Fingerprint == "" {
		t.Fatal("invalid server cert bundle")
	}
	if !strings.Contains(serverCert.Cert.Subject.CommonName, clusterID) {
		t.Fatalf("missing cluster id in CN: %s", serverCert.Cert.Subject.CommonName)
	}
	found := false
	for _, ip := range serverCert.Cert.IPAddresses {
		if ip.String() == serverAddress {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing server IP %s in SANs", serverAddress)
	}
}

func TestGenerateAdminCSR(t *testing.T) {
	clusterID := "test-cluster-csr"
	csrPEM, keyPEM, err := generateAdminCSR(clusterID)
	if err != nil {
		t.Fatalf("generateAdminCSR failed: %v", err)
	}
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatal("invalid CSR PEM")
	}
	if len(keyPEM) == 0 || !strings.Contains(string(keyPEM), "PRIVATE KEY") {
		t.Fatal("invalid private key PEM")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}
	if !strings.Contains(strings.Join(csr.Subject.OrganizationalUnit, ","), "cli-admin") {
		t.Fatal("missing OU role in CSR")
	}
	hasClientAuth := false
	for _, ext := range csr.Extensions {
		if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 37}) {
			var oids []asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(ext.Value, &oids); err == nil {
				for _, oid := range oids {
					if oid.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}) {
						hasClientAuth = true
					}
				}
			}
		}
	}
	if !hasClientAuth {
		t.Fatal("expected CSR to include clientAuth EKU")
	}
}
