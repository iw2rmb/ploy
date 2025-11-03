package handlers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

// TestPKISignAdminHandlerRequiresAdminRole verifies that the PKI sign admin endpoint
// enforces cli-admin role restriction via the Authorizer middleware.
func TestPKISignAdminHandlerRequiresAdminRole(t *testing.T) {
	// Create authorizer with insecure mode for testing.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleWorker, // Non-admin role
	})

	// Create handler with admin role restriction (as in production).
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignAdminHandler())

	// Create test request.
	reqBody := map[string]string{
		"csr": "dummy-csr",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	// Execute request.
	handler.ServeHTTP(rr, req)

	// Verify that request is rejected with 403 Forbidden.
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for non-admin role, got %d", rr.Code)
	}
}

// TestPKISignAdminHandlerAllowsAdminRole verifies that requests with cli-admin role
// are allowed through the authorization layer.
func TestPKISignAdminHandlerAllowsAdminRole(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	// Create authorizer with insecure mode and admin role.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleCLIAdmin,
	})

	// Create handler with admin role restriction.
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignAdminHandler())

	// Generate a valid admin CSR.
	csrPEM := generateAdminCSR(t, "test-cluster")

	// Create test request.
	reqBody := map[string]string{
		"csr": csrPEM,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", bytes.NewReader(body))
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
}

// TestPKISignAdminHandlerRejectsMTLSWithoutRole verifies that requests with mTLS
// but missing role claim are rejected.
func TestPKISignAdminHandlerRejectsMTLSWithoutRole(t *testing.T) {
	// Create authorizer with secure mode (mTLS required).
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
	})

	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignAdminHandler())

	// Create request with TLS but certificate missing role claim.
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", nil)
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
}

// TestPKISignAdminHandlerValidatesCSR verifies CSR validation for admin requirements.
func TestPKISignAdminHandlerValidatesCSR(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleCLIAdmin,
	})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignAdminHandler())

	cases := []struct {
		name    string
		csr     string
		want    int
		wantErr string
	}{
		{
			name:    "empty csr",
			csr:     "",
			want:    http.StatusBadRequest,
			wantErr: "csr field is required",
		},
		{
			name:    "whitespace csr",
			csr:     "   ",
			want:    http.StatusBadRequest,
			wantErr: "csr field is required",
		},
		{
			name:    "invalid csr",
			csr:     "not-a-csr",
			want:    http.StatusBadRequest,
			wantErr: "invalid admin CSR",
		},
		{
			name:    "missing admin OU",
			csr:     generateCSRWithoutAdminOU(t),
			want:    http.StatusBadRequest,
			wantErr: "invalid admin CSR",
		},
		{
			name:    "missing EKU extension",
			csr:     generateCSRWithoutEKU(t),
			want:    http.StatusBadRequest,
			wantErr: "invalid admin CSR",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := map[string]string{
				"csr": tc.csr,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.want {
				t.Fatalf("expected status %d, got %d: %s", tc.want, rr.Code, rr.Body.String())
			}
			if tc.wantErr != "" && !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tc.wantErr, rr.Body.String())
			}
		})
	}
}

// TestPKISignAdminHandlerReturnsServiceUnavailableWhenCANotConfigured verifies
// that the handler returns 503 when CA is not configured.
func TestPKISignAdminHandlerReturnsServiceUnavailableWhenCANotConfigured(t *testing.T) {
	// Ensure CA env vars are not set.
	os.Unsetenv("PLOY_SERVER_CA_CERT")
	os.Unsetenv("PLOY_SERVER_CA_KEY")

	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleCLIAdmin,
	})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignAdminHandler())

	csrPEM := generateAdminCSR(t, "test-cluster")
	reqBody := map[string]string{
		"csr": csrPEM,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "PKI not configured") {
		t.Fatalf("expected error about PKI not configured, got: %s", rr.Body.String())
	}
}

// TestPKISignAdminHandlerSuccess verifies the complete success path for signing an admin CSR.
func TestPKISignAdminHandlerSuccess(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	defer func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	}()

	// Create handler without authorization (test handler directly).
	handler := pkiSignAdminHandler()

	// Generate a valid admin CSR.
	csrPEM := generateAdminCSR(t, "test-cluster")

	// Create test request.
	reqBody := map[string]string{
		"csr": csrPEM,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", bytes.NewReader(body))
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

	// Verify all required fields are present.
	if resp.Certificate == "" {
		t.Error("expected certificate in response")
	}
	if resp.CABundle == "" {
		t.Error("expected ca_bundle in response")
	}
	if resp.Serial == "" {
		t.Error("expected serial in response")
	}
	if resp.Fingerprint == "" {
		t.Error("expected fingerprint in response")
	}
	if resp.NotBefore == "" {
		t.Error("expected not_before in response")
	}
	if resp.NotAfter == "" {
		t.Error("expected not_after in response")
	}

	// Verify timestamps are parseable.
	if _, err := time.Parse(time.RFC3339, resp.NotBefore); err != nil {
		t.Errorf("not_before is not valid RFC3339: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, resp.NotAfter); err != nil {
		t.Errorf("not_after is not valid RFC3339: %v", err)
	}

	// Verify the certificate is valid PEM.
	block, _ := pem.Decode([]byte(resp.Certificate))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Error("expected valid certificate PEM")
	}

	// Parse the certificate and verify it has ClientAuth EKU.
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	// Verify ExtKeyUsage includes ClientAuth.
	hasClientAuth := false
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageClientAuth {
			hasClientAuth = true
			break
		}
	}
	if !hasClientAuth {
		t.Error("expected certificate to have ClientAuth ExtKeyUsage")
	}

	// Verify OU contains the admin role.
	hasAdminOU := false
	for _, ou := range cert.Subject.OrganizationalUnit {
		if ou == "Ploy role=cli-admin" {
			hasAdminOU = true
			break
		}
	}
	if !hasAdminOU {
		t.Error("expected certificate to have OU=\"Ploy role=cli-admin\"")
	}
}

// TestPKISignAdminHandlerMalformedJSON verifies that malformed JSON is rejected.
func TestPKISignAdminHandlerMalformedJSON(t *testing.T) {
	handler := pkiSignAdminHandler()

	// Send malformed JSON.
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", strings.NewReader("{invalid json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed JSON, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error about invalid request, got: %s", rr.Body.String())
	}
}

// TestPKISignAdminHandlerInvalidCAConfiguration verifies handling of bad CA config.
func TestPKISignAdminHandlerInvalidCAConfiguration(t *testing.T) {
	handler := pkiSignAdminHandler()

	csrPEM := generateAdminCSR(t, "test-cluster")

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
				"csr": csrPEM,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/admin", bytes.NewReader(body))
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

// generateAdminCSR generates a test CSR with the correct admin OU and EKU extension.
func generateAdminCSR(t *testing.T, clusterID string) string {
	t.Helper()

	// Generate private key.
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Create CSR template with admin OU.
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "cli-admin-" + clusterID,
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"Ploy role=cli-admin"},
		},
	}

	// Add ExtKeyUsage extension for ClientAuth.
	// OID for ExtKeyUsage: 2.5.29.37
	// OID for ClientAuth: 1.3.6.1.5.5.7.3.2
	clientAuthOID := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}
	ekuValue, err := asn1.Marshal([]asn1.ObjectIdentifier{clientAuthOID})
	if err != nil {
		t.Fatalf("marshal EKU: %v", err)
	}
	template.ExtraExtensions = []pkix.Extension{
		{
			Id:    asn1.ObjectIdentifier{2, 5, 29, 37},
			Value: ekuValue,
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

// generateCSRWithoutAdminOU generates a CSR without the admin OU.
func generateCSRWithoutAdminOU(t *testing.T) string {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "test-client",
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"worker"}, // Wrong OU
		},
	}

	// Add ExtKeyUsage extension.
	clientAuthOID := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}
	ekuValue, err := asn1.Marshal([]asn1.ObjectIdentifier{clientAuthOID})
	if err != nil {
		t.Fatalf("marshal EKU: %v", err)
	}
	template.ExtraExtensions = []pkix.Extension{
		{
			Id:    asn1.ObjectIdentifier{2, 5, 29, 37},
			Value: ekuValue,
		},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return string(csrPEM)
}

// generateCSRWithoutEKU generates a CSR without the ExtKeyUsage extension.
func generateCSRWithoutEKU(t *testing.T) string {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "cli-admin-test",
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"Ploy role=cli-admin"},
		},
		// No ExtraExtensions - missing EKU extension
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return string(csrPEM)
}
