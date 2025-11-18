package handlers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/pki"
)

// Core PKI signing behavior tests (success path and CA/CSR error conditions).

// setupTestCA creates a test CA certificate and returns the PEM cert and PEM key.
func setupTestCA(t *testing.T) (string, string) {
	t.Helper()

	// Use the pki package to generate a CA.
	ca, err := pki.GenerateCA("test-cluster", time.Now())
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	return ca.CertPEM, ca.KeyPEM
}

// generateTestCSR generates a test CSR with the given common name.
func generateTestCSR(t *testing.T, cn string) string {
	t.Helper()

	// Generate private key.
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Create CSR template.
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         cn,
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"worker"},
		},
	}

	// Create CSR.
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}

	// Encode to PEM.
	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return string(csrPEM)
}

// TestPKISignHandlerSuccess verifies the complete success path for signing a CSR.
func TestPKISignHandlerSuccess(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	// Create mock store.
	st := &mockStore{}

	// Create handler without authorization (test handler directly).
	handler := pkiSignHandler(st)

	// Generate a valid CSR.
	nodeID := uuid.New().String()
	csrPEM := generateTestCSR(t, "node:"+nodeID)

	// Create test request.
	reqBody := map[string]string{
		"node_id": nodeID,
		"csr":     csrPEM,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	// Execute request.
	handler.ServeHTTP(rr, req)

	// Verify response status.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify response body.
	var resp struct {
		Certificate string `json:"certificate"`
		CABundle    string `json:"ca_bundle"`
		Serial      string `json:"serial"`
		Fingerprint string `json:"fingerprint"`
		NotBefore   string `json:"not_before"`
		NotAfter    string `json:"not_after"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Certificate == "" {
		t.Errorf("expected certificate in response")
	}
	if resp.CABundle == "" {
		t.Errorf("expected ca_bundle in response")
	}
	if resp.Serial == "" {
		t.Errorf("expected serial in response")
	}
	if resp.Fingerprint == "" {
		t.Errorf("expected fingerprint in response")
	}
	if resp.NotBefore == "" || resp.NotAfter == "" {
		t.Errorf("expected not_before and not_after in response")
	}

	// Verify that UpdateNodeCertMetadata was called and node ID matches.
	if !st.updateCertMetadataCalled {
		t.Fatal("expected UpdateNodeCertMetadata to be called")
	}
	expectedUUID, _ := uuid.Parse(nodeID)
	if st.updateCertMetadataParams.ID.Bytes != expectedUUID {
		t.Errorf("expected node_id %v, got %v", expectedUUID, st.updateCertMetadataParams.ID.Bytes)
	}
}

// TestPKISignHandlerPersistsFailure verifies handler behavior when metadata persistence fails.
func TestPKISignHandlerPersistsFailure(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	// Create mock store with error on UpdateNodeCertMetadata. This should surface
	// as an internal error while still attempting to persist.
	st := &mockStore{
		updateCertMetadataErr: errors.New("database connection failed"),
	}

	handler := pkiSignHandler(st)

	nodeID := uuid.New().String()
	csrPEM := generateTestCSR(t, "node:"+nodeID)

	reqBody := map[string]string{
		"node_id": nodeID,
		"csr":     csrPEM,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Persistence failure should surface as 500 with an error message.
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 for persistence failure, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to persist certificate metadata") {
		t.Errorf("expected error about persistence failure, got: %s", rr.Body.String())
	}
}

// TestPKISignHandlerInvalidCSRPEM verifies that invalid PEM is rejected.
func TestPKISignHandlerInvalidCSRPEM(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	st := &mockStore{}
	handler := pkiSignHandler(st)

	nodeID := uuid.New().String()

	cases := []struct {
		name string
		csr  string
	}{
		{
			name: "not PEM encoded",
			csr:  "random garbage data",
		},
		{
			name: "wrong PEM type",
			csr: string(pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: []byte("not a certificate"),
			})),
		},
		{
			name: "corrupted PEM block",
			csr: "-----BEGIN CERTIFICATE REQUEST-----\n" +
				"corrupted base64 data !!!\n" +
				"-----END CERTIFICATE REQUEST-----\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := map[string]string{
				"node_id": nodeID,
				"csr":     tc.csr,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 for invalid PEM, got %d: %s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "sign failed") {
				t.Errorf("expected error about sign failure, got: %s", rr.Body.String())
			}
		})
	}
}

// TestPKISignHandlerInvalidCAConfiguration verifies handling of bad CA config.
func TestPKISignHandlerInvalidCAConfiguration(t *testing.T) {
	st := &mockStore{}
	handler := pkiSignHandler(st)

	nodeID := uuid.New().String()
	csrPEM := generateTestCSR(t, "node:"+nodeID)

	cases := []struct {
		name     string
		caCert   string
		caKey    string
		wantCode int
		wantErr  string
	}{
		{
			name:     "whitespace CA cert",
			caCert:   "   ",
			caKey:    "some-key",
			wantCode: http.StatusServiceUnavailable,
			wantErr:  "PKI not configured",
		},
		{
			name:     "whitespace CA key",
			caCert:   "some-cert",
			caKey:    "   ",
			wantCode: http.StatusServiceUnavailable,
			wantErr:  "PKI not configured",
		},
		{
			name:     "invalid PEM in CA cert",
			caCert:   "invalid-pem-data",
			caKey:    "invalid-key-data",
			wantCode: http.StatusInternalServerError,
			wantErr:  "failed to load CA",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("PLOY_SERVER_CA_CERT", tc.caCert)
			os.Setenv("PLOY_SERVER_CA_KEY", tc.caKey)
			defer func() {
				os.Unsetenv("PLOY_SERVER_CA_CERT")
				os.Unsetenv("PLOY_SERVER_CA_KEY")
			}()

			reqBody := map[string]string{
				"node_id": nodeID,
				"csr":     csrPEM,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Fatalf("expected status %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tc.wantErr, rr.Body.String())
			}
		})
	}
}

// TestPKISignHandlerCSRSignatureFailed verifies that CSRs with invalid signatures are rejected.
func TestPKISignHandlerCSRSignatureFailed(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	st := &mockStore{}
	handler := pkiSignHandler(st)

	nodeID := uuid.New().String()

	// Generate a CSR and then corrupt it to fail signature verification.
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "node:" + nodeID,
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"worker"},
		},
	}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, template, priv)

	// Corrupt the DER bytes to make signature invalid.
	csrDER[len(csrDER)-1] ^= 0xFF

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	reqBody := map[string]string{
		"node_id": nodeID,
		"csr":     string(csrPEM),
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should be rejected with 400 Bad Request.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid signature, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "sign failed") {
		t.Errorf("expected error about sign failure, got: %s", rr.Body.String())
	}
}
