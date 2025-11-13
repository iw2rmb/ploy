package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/pki"
)

type mockPKIServer struct {
	t           *testing.T
	clusterID   string
	ca          *pki.CABundle
	expectRole  string
	receivedCSR bool
}

func (m *mockPKIServer) start() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pki/sign/admin", m.handleSignAdmin)
	return httptest.NewServer(mux)
}

func (m *mockPKIServer) handleSignAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CSR string `json:"csr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	m.receivedCSR = true
	cert, err := pki.SignNodeCSR(m.ca, []byte(req.CSR), time.Now())
	if err != nil {
		http.Error(w, fmt.Sprintf("sign failed: %v", err), http.StatusInternalServerError)
		return
	}
	resp := struct{ Certificate, CABundle, Serial, Fingerprint, NotBefore, NotAfter string }{
		Certificate: cert.CertPEM,
		CABundle:    m.ca.CertPEM,
		Serial:      cert.Serial,
		Fingerprint: cert.Fingerprint,
		NotBefore:   cert.NotBefore.Format(time.RFC3339),
		NotAfter:    cert.NotAfter.Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func TestRefreshAdminCertFromServer(t *testing.T) {
	IsolatePloyConfigHomeAllowDefault(t)
	clusterID := "test-cluster-refresh"
	now := time.Now()
	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}
	adminCert, err := pki.IssueClientCert(ca, clusterID, now)
	if err != nil {
		t.Fatalf("IssueClientCert failed: %v", err)
	}
	caPath, certPath, keyPath, err := writeLocalAdminBundle(clusterID, ca.CertPEM, adminCert.CertPEM, adminCert.KeyPEM)
	if err != nil {
		t.Fatalf("writeLocalAdminBundle failed: %v", err)
	}
	desc := config.Descriptor{ClusterID: config.ClusterID(clusterID), Address: "https://10.0.0.5:8443", Scheme: "https", CAPath: caPath, CertPath: certPath, KeyPath: keyPath}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}
	if err := config.SetDefault(config.ClusterID(clusterID)); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}
	mockServer := &mockPKIServer{t: t, clusterID: clusterID, ca: ca, expectRole: "cli-admin"}
	server := mockServer.start()
	defer server.Close()
	desc.Address = server.URL
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor(update address): %v", err)
	}
	if err := config.SetDefault(config.ClusterID(clusterID)); err != nil {
		t.Fatalf("SetDefault(update): %v", err)
	}
	stderr := &bytes.Buffer{}
	if err := handleRefreshAdminCert(context.Background(), stderr); err != nil {
		t.Fatalf("handleRefreshAdminCert failed: %v", err)
	}
	out := stderr.String()
	for _, s := range []string{"Refreshing admin certificate", "Admin certificate issued successfully", "Admin certificate refreshed successfully"} {
		if !strings.Contains(out, s) {
			t.Errorf("expected %q in output", s)
		}
	}
	for _, p := range []string{caPath, certPath, keyPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file present: %s", p)
		}
	}
	updated, err := config.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if updated.CAPath != caPath || updated.CertPath != certPath || updated.KeyPath != keyPath {
		t.Errorf("descriptor paths not updated")
	}
	if !mockServer.receivedCSR {
		t.Error("expected mock server to receive CSR")
	}
}

func TestRefreshAdminCertFromServerServerError(t *testing.T) {
	IsolatePloyConfigHomeAllowDefault(t)
	clusterID := "test-cluster-error"
	desc := config.Descriptor{ClusterID: config.ClusterID(clusterID), Address: "https://127.0.0.1:8443", Scheme: "https"}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}
	if err := config.SetDefault(config.ClusterID(clusterID)); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pki/sign/admin", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad csr", http.StatusBadRequest) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if _, err := config.SaveDescriptor(config.Descriptor{ClusterID: config.ClusterID(clusterID), Address: srv.URL}); err != nil {
		t.Fatalf("SaveDescriptor(update): %v", err)
	}
	if err := config.SetDefault(config.ClusterID(clusterID)); err != nil {
		t.Fatalf("SetDefault(update): %v", err)
	}
	err := handleRefreshAdminCert(context.Background(), bytes.NewBuffer(nil))
	if err == nil || !strings.Contains(err.Error(), "server returned status 400") {
		t.Fatalf("expected 400 error, got: %v", err)
	}
}

func TestRefreshAdminCertFromServerInvalidJSON(t *testing.T) {
	IsolatePloyConfigHomeAllowDefault(t)
	clusterID := "test-cluster-invalid-json"
	desc := config.Descriptor{ClusterID: config.ClusterID(clusterID), Address: "https://127.0.0.1:8443", Scheme: "https"}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}
	if err := config.SetDefault(config.ClusterID(clusterID)); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pki/sign/admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not json}"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if _, err := config.SaveDescriptor(config.Descriptor{ClusterID: config.ClusterID(clusterID), Address: srv.URL}); err != nil {
		t.Fatalf("SaveDescriptor(update): %v", err)
	}
	if err := config.SetDefault(config.ClusterID(clusterID)); err != nil {
		t.Fatalf("SetDefault(update): %v", err)
	}
	err := handleRefreshAdminCert(context.Background(), bytes.NewBuffer(nil))
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestHandleRefreshAdminCertMissingClusterID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(tmpDir, "clusters")) })
	clusters := filepath.Join(tmpDir, "clusters")
	if err := os.MkdirAll(clusters, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	id := "malformed"
	data := []byte(`{"cluster_id":"","address":"https://127.0.0.1:8443","scheme":"https"}`)
	if err := os.WriteFile(filepath.Join(clusters, id+".json"), data, 0o644); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clusters, "default"), []byte(id), 0o644); err != nil {
		t.Fatalf("write default marker: %v", err)
	}
	err := handleRefreshAdminCert(context.Background(), bytes.NewBuffer(nil))
	if err == nil || !strings.Contains(err.Error(), "cluster ID not found in descriptor") {
		t.Fatalf("expected missing cluster ID error, got: %v", err)
	}
}
