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
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

func TestPKISignClientHandlerRequiresAdminRole(t *testing.T) {
	authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: auth.RoleWorker})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignClientHandler())

	body, _ := json.Marshal(map[string]string{"csr": "dummy"})
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/client", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestPKISignClientHandlerValidatesCSRAndSigns(t *testing.T) {
	// Setup test CA.
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	t.Cleanup(func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	})

	authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: auth.RoleCLIAdmin})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignClientHandler())

	csrPEM := generateClientCSR(t)
	body, _ := json.Marshal(map[string]string{"csr": csrPEM})
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/client", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Certificate string `json:"certificate"`
		CABundle    string `json:"ca_bundle"`
		Serial      string `json:"serial"`
		Fingerprint string `json:"fingerprint"`
		NotBefore   string `json:"not_before"`
		NotAfter    string `json:"not_after"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Certificate == "" || resp.CABundle == "" || resp.Serial == "" || resp.Fingerprint == "" {
		t.Fatalf("missing fields in response: %+v", resp)
	}
}

func TestPKISignClientHandlerRejectsInvalidCSR(t *testing.T) {
	caPEM, caKeyPEM := setupTestCA(t)
	os.Setenv("PLOY_SERVER_CA_CERT", caPEM)
	os.Setenv("PLOY_SERVER_CA_KEY", caKeyPEM)
	t.Cleanup(func() {
		os.Unsetenv("PLOY_SERVER_CA_CERT")
		os.Unsetenv("PLOY_SERVER_CA_KEY")
	})

	authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: auth.RoleCLIAdmin})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignClientHandler())

	cases := []struct {
		name string
		csr  string
	}{
		{"empty", ""},
		{"garbage", "not-a-csr"},
		{"missing-ou", generateCSRWithoutClientOU(t)},
		{"missing-eku", generateCSRClientWithoutEKU(t)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"csr": tc.csr})
			req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/client", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rr.Code)
			}
		})
	}
}

func TestPKISignClientHandlerCANotConfigured(t *testing.T) {
	os.Unsetenv("PLOY_SERVER_CA_CERT")
	os.Unsetenv("PLOY_SERVER_CA_KEY")
	authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: auth.RoleCLIAdmin})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignClientHandler())

	csr := generateClientCSR(t)
	body, _ := json.Marshal(map[string]string{"csr": csr})
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/client", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

// --- helpers ---

func generateClientCSR(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	// Include OU Ploy role=client and ClientAuth EKU
	clientAuthOID := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}
	ekuValue, err := asn1.Marshal([]asn1.ObjectIdentifier{clientAuthOID})
	if err != nil {
		t.Fatalf("eku marshal: %v", err)
	}
	tpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "client-user",
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"Ploy role=client"},
		},
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Value: ekuValue}},
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tpl, priv)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

func generateCSRWithoutClientOU(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	clientAuthOID := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}
	ekuValue, err := asn1.Marshal([]asn1.ObjectIdentifier{clientAuthOID})
	if err != nil {
		t.Fatalf("eku marshal: %v", err)
	}
	tpl := &x509.CertificateRequest{
		Subject:         pkix.Name{CommonName: "client-user", Organization: []string{"Ploy"}},
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Value: ekuValue}},
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tpl, priv)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

func generateCSRClientWithoutEKU(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "client-user",
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"Ploy role=client"},
		},
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tpl, priv)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

// Ensure middleware rejects missing mTLS role if secure mode is used.
func TestPKISignClientHandlerRejectsNoRoleWhenSecure(t *testing.T) {
	authorizer := auth.NewAuthorizer(auth.Options{AllowInsecure: false})
	handler := authorizer.Middleware(auth.RoleCLIAdmin)(pkiSignClientHandler())
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign/client", nil)
	// Simulate TLS without OU role.
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "no-role"}}}}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
