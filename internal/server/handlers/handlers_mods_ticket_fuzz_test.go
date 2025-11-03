package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// FuzzSubmitTicketHandler ensures the ticket submission handler is resilient to
// arbitrary inputs and does not panic. It only exercises the decoding/validation
// path with a nil-backed mock store; success is not required.
func FuzzSubmitTicketHandler(f *testing.F) {
	st := &mockStore{}
	h := submitTicketHandler(st)

	// Seed with a few typical cases.
	seeds := [][]byte{
		[]byte(`{"repo_url":"https://example.com/repo.git","base_ref":"main","target_ref":"feature"}`),
		[]byte(`{"repo_url":"","base_ref":"","target_ref":""}`),
		[]byte(`{"repo_url":" https://x ","base_ref":" m ","target_ref":" t ","spec":{}}`),
		[]byte(`{invalid`),
	}
	for _, s := range seeds {
		f.Add(string(s))
	}

	f.Fuzz(func(t *testing.T, body string) {
		req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewBufferString(body))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		// Any status code is acceptable; the important property is no panic.
		_ = rr.Code
	})
}
