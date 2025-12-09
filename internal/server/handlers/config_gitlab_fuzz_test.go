package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// FuzzPutGitLabConfig_NoPanic ensures the PUT handler never panics
// on arbitrary inputs and responds with either 200 (valid JSON)
// or 400 (invalid JSON). The handler must not leak secrets in errors.
func FuzzPutGitLabConfig_NoPanic(f *testing.F) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	// Seed with a few valid and invalid payloads.
	f.Add([]byte(`{"domain":"https://gitlab.com","token":"glpat-seed"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"domain":123,"token":true}`))

	h := putGitLabConfigHandler(holder)

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			// Keep fuzz inputs small and fast.
			t.Skip()
			return
		}
		req := httptest.NewRequest(http.MethodPut, "/v1/config/gitlab", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		// Accept 200 for valid payloads, 400 for invalid JSON.
		switch rr.Code {
		case http.StatusOK, http.StatusBadRequest:
			// ok
		default:
			t.Fatalf("unexpected status: %d", rr.Code)
		}
	})
}
