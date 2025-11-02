package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// FuzzCreateRunHandlerDoesNotPanic feeds arbitrary bodies to the createRunHandler
// to ensure it never panics on malformed input.
func FuzzCreateRunHandlerDoesNotPanic(f *testing.F) {
	st := &mockStore{}
	handler := createRunHandler(st)

	// Seed with a couple of interesting inputs.
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"mod_id":"00000000-0000-0000-0000-000000000000"}`))
	f.Add([]byte(`not json`))

	f.Fuzz(func(t *testing.T, body []byte) {
		// Just verify the handler completes without panic.
		req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		// Any status code is acceptable for the fuzz target; we're checking for panics only.
		_ = rr.Code
	})
}
