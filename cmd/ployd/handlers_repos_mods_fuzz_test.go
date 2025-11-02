package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// FuzzCreateRepoHandlerDoesNotPanic ensures the repo creation handler is robust
// against arbitrary input bodies.
func FuzzCreateRepoHandlerDoesNotPanic(f *testing.F) {
	st := &mockStore{}
	handler := createRepoHandler(st)

	f.Add([]byte(`{}`))
	f.Add([]byte(`{"url":"https://example.com/repo.git"}`))
	f.Add([]byte(`not json`))

	f.Fuzz(func(t *testing.T, body []byte) {
		req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		_ = rr.Code
	})
}

// FuzzCreateModHandlerDoesNotPanic ensures the mod creation handler is robust
// against arbitrary input bodies.
func FuzzCreateModHandlerDoesNotPanic(f *testing.F) {
	st := &mockStore{}
	handler := createModHandler(st)

	f.Add([]byte(`{}`))
	f.Add([]byte(`{"repo_id":"00000000-0000-0000-0000-000000000000","spec":{}}`))
	f.Add([]byte(`not json`))

	f.Fuzz(func(t *testing.T, body []byte) {
		req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		_ = rr.Code
	})
}
