package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

// Request validation tests for PKI sign handler (node_id, csr, CA presence).

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
