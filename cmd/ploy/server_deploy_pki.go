package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
)

// handleRefreshAdminCert is deprecated - bearer token authentication replaces mTLS certificates.
func handleRefreshAdminCert(ctx context.Context, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}

	_, _ = fmt.Fprintln(stderr, "ERROR: --refresh-admin-cert is deprecated")
	_, _ = fmt.Fprintln(stderr, "")
	_, _ = fmt.Fprintln(stderr, "Bearer token authentication has replaced mTLS certificate authentication.")
	_, _ = fmt.Fprintln(stderr, "")
	_, _ = fmt.Fprintln(stderr, "To authenticate with the server:")
	_, _ = fmt.Fprintln(stderr, "  1. Create a new API token:")
	_, _ = fmt.Fprintln(stderr, "     ploy cluster token create --role cli-admin --description \"My token\"")
	_, _ = fmt.Fprintln(stderr, "")
	_, _ = fmt.Fprintln(stderr, "  2. Add the token to your cluster descriptor:")
	_, _ = fmt.Fprintln(stderr, "     Edit ~/.config/ploy/<cluster-id>/auth.json")
	_, _ = fmt.Fprintln(stderr, `     Add: "token": "your-token-here"`)
	_, _ = fmt.Fprintln(stderr, "")

	return errors.New("--refresh-admin-cert is deprecated, use bearer tokens instead")
}

// generateAdminCSR generates a CSR for cli-admin with proper OU and ExtKeyUsage.
func generateAdminCSR(clusterID string) (csrPEM, keyPEM []byte, err error) {
	// Generate ECDSA private key.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate admin key: %w", err)
	}

	// Create CSR template with proper OU and CN.
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "cli-admin-" + clusterID,
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"Ploy role=cli-admin"},
		},
	}

	// Add ExtKeyUsage extension for ClientAuth (1.3.6.1.5.5.7.3.2).
	clientAuthOID := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}
	ekuValue, err := asn1.Marshal([]asn1.ObjectIdentifier{clientAuthOID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal EKU: %w", err)
	}
	template.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Value: ekuValue}}

	// Create CSR.
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR: %w", err)
	}

	// Encode CSR to PEM.
	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Encode private key to PEM.
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal admin private key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return csrPEM, keyPEM, nil
}
