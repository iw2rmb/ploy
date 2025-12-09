package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

	"github.com/iw2rmb/ploy/internal/store"
)

// FuzzCancelRun_Body exercises the cancel handler against arbitrary bodies
// to ensure robust JSON decoding and path parameter handling. This target is
// fast and deterministic; it does not hit external services.
func FuzzCancelRun_Body(f *testing.F) {
	// Seed with representative bodies.
	f.Add([]byte(""), true)
	f.Add([]byte("{}"), true)
	f.Add([]byte(`{"reason":"user requested"}`), true)
	f.Add([]byte(`{"reason":null}`), true)
	f.Add([]byte(`{"reason":""}`), true)
	f.Add([]byte("{bad json"), true)
	f.Add([]byte{0x00, 0xff, '{', '}', 0x7f}, true)
	f.Add([]byte(""), false) // invalid id path

	f.Fuzz(func(t *testing.T, body []byte, useValidID bool) {
		// Prepare store and id depending on validity flag.
		var idStr string
		st := &mockStore{}
		if useValidID {
			runID := domaintypes.NewRunID()
			idStr = runID.String()
			st.getRunResult = store.Run{ID: runID.String(), Status: store.RunStatusRunning}
		} else {
			idStr = "not-a-uuid"
		}

		handler := cancelRunHandler(st, nil)

		req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+idStr+"/cancel", bytes.NewReader(body))
		req.SetPathValue("id", idStr)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// The handler must not panic; any HTTP status is acceptable within
		// standard ranges. Validate basic invariants instead of golden values.
		handler.ServeHTTP(rr, req)

		if rr.Code < 200 || rr.Code >= 600 {
			t.Fatalf("unexpected HTTP status: %d", rr.Code)
		}
		if useValidID {
			// For valid ids, we must not return 400 for invalid uuid.
			if rr.Code == http.StatusBadRequest {
				t.Fatalf("unexpected 400 for valid uuid; body=%q", string(body))
			}
		}
	})
}
