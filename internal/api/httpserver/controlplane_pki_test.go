package httpserver

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore implements store.Store for testing.
type mockPKIStore struct {
	store.Store
	updateNodeCertMetadataFunc func(ctx context.Context, arg store.UpdateNodeCertMetadataParams) error
}

func (m *mockPKIStore) UpdateNodeCertMetadata(ctx context.Context, arg store.UpdateNodeCertMetadataParams) error {
	if m.updateNodeCertMetadataFunc != nil {
		return m.updateNodeCertMetadataFunc(ctx, arg)
	}
	return nil
}

func (m *mockPKIStore) Close() {}

func TestHandlePKISign(t *testing.T) {
	// Generate a CA for testing.
	ca, err := pki.GenerateCA("test-cluster", time.Now().UTC())
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	// Set CA environment variables.
	t.Setenv("PLOY_SERVER_CA_CERT", ca.CertPEM)
	t.Setenv("PLOY_SERVER_CA_KEY", ca.KeyPEM)

	nodeID := uuid.New()

	// Generate a CSR.
	nodeKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate node key: %v", err)
	}

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "node:" + nodeID.String(),
			Organization: []string{"Ploy"},
		},
		DNSNames:    []string{"ploy-node." + nodeID.String() + ".test-cluster.ploy"},
		IPAddresses: []net.IP{net.ParseIP("10.0.0.5")},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, nodeKey)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}
	csrPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	// Create request.
	reqBody := PKISignRequest{
		NodeID: nodeID.String(),
		CSR:    csrPEM,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var capturedParams store.UpdateNodeCertMetadataParams
	mockStore := &mockPKIStore{
		updateNodeCertMetadataFunc: func(ctx context.Context, arg store.UpdateNodeCertMetadataParams) error {
			capturedParams = arg
			return nil
		},
	}

	server := &controlPlaneServer{
		store: mockStore,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp PKISignResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Certificate == "" {
		t.Fatal("expected non-empty certificate")
	}
	if resp.CABundle == "" {
		t.Fatal("expected non-empty CA bundle")
	}
	if resp.Serial == "" {
		t.Fatal("expected non-empty serial")
	}
	if resp.Fingerprint == "" {
		t.Fatal("expected non-empty fingerprint")
	}
	if resp.NotBefore.IsZero() {
		t.Fatal("expected non-zero NotBefore")
	}
	if resp.NotAfter.IsZero() {
		t.Fatal("expected non-zero NotAfter")
	}

	// Verify the certificate is valid and signed by the CA.
	certBlock, _ := pem.Decode([]byte(resp.Certificate))
	if certBlock == nil {
		t.Fatal("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	opts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	if _, err := cert.Verify(opts); err != nil {
		t.Fatalf("certificate verification failed: %v", err)
	}

	// Verify metadata was persisted.
	if !capturedParams.ID.Valid {
		t.Fatal("expected valid node ID in params")
	}
	if capturedParams.ID.Bytes != nodeID {
		t.Fatalf("expected node ID %v, got %v", nodeID, capturedParams.ID.Bytes)
	}
	if capturedParams.CertSerial == nil || *capturedParams.CertSerial == "" {
		t.Fatal("expected non-empty serial in params")
	}
	if capturedParams.CertFingerprint == nil || *capturedParams.CertFingerprint == "" {
		t.Fatal("expected non-empty fingerprint in params")
	}
	if !capturedParams.CertNotBefore.Valid {
		t.Fatal("expected valid NotBefore in params")
	}
	if !capturedParams.CertNotAfter.Valid {
		t.Fatal("expected valid NotAfter in params")
	}
}

func TestHandlePKISignInvalidMethod(t *testing.T) {
	server := &controlPlaneServer{}
	req := httptest.NewRequest(http.MethodGet, "/v1/pki/sign", nil)
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestHandlePKISignNoStore(t *testing.T) {
	server := &controlPlaneServer{
		store: nil,
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestHandlePKISignInvalidJSON(t *testing.T) {
	t.Setenv("PLOY_SERVER_CA_CERT", "dummy")
	t.Setenv("PLOY_SERVER_CA_KEY", "dummy")

	server := &controlPlaneServer{
		store: &mockPKIStore{},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHandlePKISignMissingNodeID(t *testing.T) {
	t.Setenv("PLOY_SERVER_CA_CERT", "dummy")
	t.Setenv("PLOY_SERVER_CA_KEY", "dummy")

	server := &controlPlaneServer{
		store: &mockPKIStore{},
	}
	reqBody := PKISignRequest{
		CSR: "some csr",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHandlePKISignMissingCSR(t *testing.T) {
	t.Setenv("PLOY_SERVER_CA_CERT", "dummy")
	t.Setenv("PLOY_SERVER_CA_KEY", "dummy")

	server := &controlPlaneServer{
		store: &mockPKIStore{},
	}
	reqBody := PKISignRequest{
		NodeID: uuid.New().String(),
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHandlePKISignInvalidNodeID(t *testing.T) {
	t.Setenv("PLOY_SERVER_CA_CERT", "dummy")
	t.Setenv("PLOY_SERVER_CA_KEY", "dummy")

	server := &controlPlaneServer{
		store: &mockPKIStore{},
	}
	reqBody := PKISignRequest{
		NodeID: "not-a-uuid",
		CSR:    "some csr",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHandlePKISignNoCAConfigured(t *testing.T) {
	t.Setenv("PLOY_SERVER_CA_CERT", "")
	t.Setenv("PLOY_SERVER_CA_KEY", "")

	server := &controlPlaneServer{
		store: &mockPKIStore{},
	}
	reqBody := PKISignRequest{
		NodeID: uuid.New().String(),
		CSR:    "some csr",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handlePKISign(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}
