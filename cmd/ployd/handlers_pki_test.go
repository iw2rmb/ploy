package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/pki"
)

// TestPKISignHandlerRequiresAdminRole verifies that the PKI sign endpoint
// enforces cli-admin role restriction via the Authorizer middleware.
func TestPKISignHandlerRequiresAdminRole(t *testing.T) {
	// Create mock store (not needed for authorization check).
	st := &mockStore{}

	// Create authorizer with insecure mode for testing.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleWorker, // Non-admin role
	})

	// Create handler with admin role restriction (as in production).
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignHandler(st))

	// Create test request.
	reqBody := map[string]string{
		"node_id": uuid.New().String(),
		"csr":     "dummy-csr",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	// Execute request.
	handler.ServeHTTP(rr, req)

	// Verify that request is rejected with 403 Forbidden.
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for non-admin role, got %d", rr.Code)
	}
}

// TestPKISignHandlerAllowsAdminRole verifies that requests with cli-admin role
// are allowed through the authorization layer.
func TestPKISignHandlerAllowsAdminRole(t *testing.T) {
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

	// Create authorizer with insecure mode and admin role.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleCLIAdmin,
	})

	// Create handler with admin role restriction.
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignHandler(st))

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

	// Verify that request succeeds (gets past authorization).
	// It should return 200 OK with signed certificate.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for admin role, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify response contains certificate data.
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
		t.Fatal("expected certificate in response")
	}
	if resp.Serial == "" {
		t.Fatal("expected serial in response")
	}

	// Verify that UpdateNodeCertMetadata was called.
	if !st.updateCertMetadataCalled {
		t.Fatal("expected UpdateNodeCertMetadata to be called")
	}
}

// TestPKISignHandlerRejectsMTLSWithoutRole verifies that requests with mTLS
// but missing role claim are rejected.
func TestPKISignHandlerRejectsMTLSWithoutRole(t *testing.T) {
	st := &mockStore{}

	// Create authorizer with secure mode (mTLS required).
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
	})

	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignHandler(st))

	// Create request with TLS but certificate missing role claim.
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{
				Subject: pkix.Name{
					CommonName: "test-client",
					// Missing OrganizationalUnit with role claim.
				},
			},
		},
	}
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should be rejected with 403.
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for cert without role, got %d", rr.Code)
	}
	// The error message should indicate forbidden access (either "forbidden" or "missing role claim").
	body := rr.Body.String()
	if !strings.Contains(body, "forbidden") && !strings.Contains(body, "missing role claim") {
		t.Fatalf("expected error about forbidden or missing role claim, got: %s", body)
	}
}

// TestPKISignHandlerValidatesNodeID verifies that invalid node_id is rejected.
func TestPKISignHandlerValidatesNodeID(t *testing.T) {
	st := &mockStore{}
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleCLIAdmin,
	})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignHandler(st))

	cases := []struct {
		name   string
		nodeID string
		want   int
	}{
		{"empty", "", http.StatusBadRequest},
		{"invalid uuid", "not-a-uuid", http.StatusBadRequest},
		{"whitespace", "   ", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := map[string]string{
				"node_id": tc.nodeID,
				"csr":     "dummy-csr",
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.want {
				t.Fatalf("expected status %d, got %d", tc.want, rr.Code)
			}
		})
	}
}

// TestPKISignHandlerValidatesCSR verifies CSR validation and subject CN matching.
func TestPKISignHandlerValidatesCSR(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	st := &mockStore{}
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleCLIAdmin,
	})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignHandler(st))

	nodeID := uuid.New().String()

	cases := []struct {
		name string
		csr  string
		want int
	}{
		{"empty csr", "", http.StatusBadRequest},
		{"whitespace csr", "   ", http.StatusBadRequest},
		{"invalid csr", "not-a-csr", http.StatusBadRequest},
		{"mismatched CN", generateTestCSR(t, "wrong-cn"), http.StatusBadRequest},
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

			if rr.Code != tc.want {
				t.Fatalf("expected status %d, got %d: %s", tc.want, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestPKISignHandlerReturnsServiceUnavailableWhenCANotConfigured verifies
// that the handler returns 503 when CA is not configured.
func TestPKISignHandlerReturnsServiceUnavailableWhenCANotConfigured(t *testing.T) {
	// Ensure CA env vars are not set.
	os.Unsetenv("PLOY_SERVER_CA_CERT")
	os.Unsetenv("PLOY_SERVER_CA_KEY")

	st := &mockStore{}
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleCLIAdmin,
	})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignHandler(st))

	nodeID := uuid.New().String()
	reqBody := map[string]string{
		"node_id": nodeID,
		"csr":     "dummy-csr",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "PKI not configured") {
		t.Fatalf("expected error about PKI not configured, got: %s", rr.Body.String())
	}
}

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
