package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidateBuildGate_DeprecatedReturnsGone verifies that POST /v1/buildgate/validate
// is still mounted but returns 410 Gone now that HTTP Build Gate has been removed.
func TestValidateBuildGate_DeprecatedReturnsGone(t *testing.T) {
	t.Parallel()

	st := &mockStore{}

	req := httptest.NewRequest(http.MethodPost, "/v1/buildgate/validate", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h := validateBuildGateHandler(st)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone, got status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "HTTP Build Gate API has been removed") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}
