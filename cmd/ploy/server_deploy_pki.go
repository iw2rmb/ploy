package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	_, _ = fmt.Fprintln(stderr, "     ploy token create --role cli-admin --description \"My token\"")
	_, _ = fmt.Fprintln(stderr, "")
	_, _ = fmt.Fprintln(stderr, "  2. Add the token to your cluster descriptor:")
	_, _ = fmt.Fprintln(stderr, "     Edit ~/.config/ploy/clusters/<cluster-id>.json")
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

// refreshAdminCertFromServer generates a CSR and calls the server PKI endpoint to sign it.
//
//nolint:unused // reserved for future server PKI rotation flow
func refreshAdminCertFromServer(ctx context.Context, clusterID string, stderr io.Writer) (caPEM, certPEM, keyPEM string, err error) {
	if stderr == nil {
		stderr = io.Discard
	}

	// Generate CSR and private key.
	_, _ = fmt.Fprintln(stderr, "Generating admin certificate signing request...")
	csrPEMBytes, keyPEMBytes, err := generateAdminCSR(clusterID)
	if err != nil {
		return "", "", "", fmt.Errorf("generate admin CSR: %w", err)
	}

	// Get server URL and HTTP client from descriptor.
	serverURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve control plane URL: %w", err)
	}

	// Build request body.
	reqBody := struct {
		CSR string `json:"csr"`
	}{CSR: string(csrPEMBytes)}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal request: %w", err)
	}

	// Call server PKI endpoint.
	endpoint := strings.TrimSuffix(serverURL.String(), "/") + "/v1/pki/sign/admin"
	_, _ = fmt.Fprintf(stderr, "Requesting admin certificate from server: %s\n", endpoint)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("call server PKI endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr != "" {
			return "", "", "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, bodyStr)
		}
		return "", "", "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Decode response.
	var signResp struct {
		Certificate string `json:"certificate"`
		CABundle    string `json:"ca_bundle"`
		Serial      string `json:"serial"`
		Fingerprint string `json:"fingerprint"`
		NotBefore   string `json:"not_before"`
		NotAfter    string `json:"not_after"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&signResp); err != nil {
		return "", "", "", fmt.Errorf("decode response: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "Admin certificate issued successfully\n")
	_, _ = fmt.Fprintf(stderr, "  Serial: %s\n", signResp.Serial)
	_, _ = fmt.Fprintf(stderr, "  Fingerprint: %s\n", signResp.Fingerprint)
	_, _ = fmt.Fprintf(stderr, "  Valid: %s to %s\n", signResp.NotBefore, signResp.NotAfter)

	return signResp.CABundle, signResp.Certificate, string(keyPEMBytes), nil
}
