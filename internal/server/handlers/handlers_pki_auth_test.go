package handlers

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/server/auth"
)

// PKI handler auth and role/mTLS enforcement tests.

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

// TestPKISignHandlerRequiresAdminRoleMTLS verifies that when mTLS is enabled,
// the handler still enforces cli-admin role based on client certificate.
func TestPKISignHandlerRequiresAdminRoleMTLS(t *testing.T) {
	st := &mockStore{}

	// Create authorizer with secure mode (mTLS required).
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
	})

	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignHandler(st))

	// Create request with TLS but without any client certificate.
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", nil)
	req.TLS = &tls.ConnectionState{}
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should be rejected with 401 or 403 depending on implementation.
	if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 401/403 for missing client cert, got %d", rr.Code)
	}
}

// TestPKISignHandlerRejectsMTLSWithoutRole verifies that client certs without
// the required role claim are rejected even when mTLS is configured.
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
