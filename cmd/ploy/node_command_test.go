package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleNodeRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleNode(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing node subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy node") {
		t.Fatalf("expected node usage output, got: %q", out)
	}
}

func TestHandleNodeAddRequiresClusterIDAndAddress(t *testing.T) {
	buf := &bytes.Buffer{}
	// No flags at all -> cluster-id required first
	err := handleNodeAdd(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --cluster-id is missing")
	}
	if !strings.Contains(err.Error(), "cluster-id is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Provide cluster-id but no address
	buf.Reset()
	err = handleNodeAdd([]string{"--cluster-id", "abc"}, buf)
	if err == nil {
		t.Fatalf("expected error when --address is missing")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleNodeAddRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleNodeAdd([]string{"--cluster-id", "c1", "--address", "1.2.3.4", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleNodeAddRequiresServerURL(t *testing.T) {
	buf := &bytes.Buffer{}
	// Provide cluster-id and address but no server-url (and no binary path)
	err := handleNodeAdd([]string{
		"--cluster-id", "c1",
		"--address", "10.0.0.5",
		"--ployd-node-binary", "/dev/null",
	}, buf)
	if err == nil {
		t.Fatalf("expected error when --server-url is missing")
	}
	if !strings.Contains(err.Error(), "server-url is required") {
		t.Fatalf("expected server-url required error, got: %v", err)
	}
}

func TestSignNodeCSR_Success(t *testing.T) {
	t.Parallel()
	// Arrange a fake PKI sign endpoint
	var gotPath, gotContentType string
	var gotBody pkiSignRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificate": "CERT-PEM",
			"ca_bundle":   "CA-PEM",
			"serial":      "01",
			"fingerprint": "ff",
			"not_before":  "2025-11-01T00:00:00Z",
			"not_after":   "2026-11-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	// Act
	cert, ca, err := signNodeCSR(context.Background(), srv.URL, "node-abc", []byte("CSR-PEM"))
	if err != nil {
		t.Fatalf("signNodeCSR error: %v", err)
	}

	// Assert
	if gotPath != "/v1/pki/sign" {
		t.Fatalf("expected path /v1/pki/sign, got: %s", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json, got: %s", gotContentType)
	}
	if gotBody.NodeID != "node-abc" || gotBody.CSR != "CSR-PEM" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
	if cert != "CERT-PEM" || ca != "CA-PEM" {
		t.Fatalf("unexpected response: cert=%q ca=%q", cert, ca)
	}
}

func TestSignNodeCSR_Non200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad csr", http.StatusBadRequest)
	}))
	defer srv.Close()

	_, _, err := signNodeCSR(context.Background(), srv.URL, "node-abc", []byte("CSR-PEM"))
	if err == nil || !strings.Contains(err.Error(), "server returned 400: bad csr") {
		t.Fatalf("expected status error, got: %v", err)
	}
}

func TestResolvePloydNodeBinaryPath_Explicit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "ployd-node-test")
	if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write temp binary: %v", err)
	}
	out, err := resolvePloydNodeBinaryPath(stringValue{set: true, value: p})
	if err != nil {
		t.Fatalf("resolvePloydNodeBinaryPath error: %v", err)
	}
	if out != p {
		t.Fatalf("expected %q, got %q", p, out)
	}
}
