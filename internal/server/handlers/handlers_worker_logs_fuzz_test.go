package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// FuzzCreateNodeLogsHandler ensures robust decoding and size checks across arbitrary inputs.
func FuzzCreateNodeLogsHandler(f *testing.F) {
	// Seed corpus: small valid and boundary cases.
	f.Add([]byte("hello"), int32(0))
	f.Add(bytes.Repeat([]byte("a"), 1024), int32(1))

	f.Fuzz(func(t *testing.T, data []byte, chunkNo int32) {
		mockStore := &mockStoreForLogs{nodeExists: true}
		handler := createNodeLogsHandler(mockStore)

		payload := map[string]any{
			"run_id":   uuid.New().String(),
			"chunk_no": chunkNo,
			"data":     data,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Skip("marshal failed")
		}

		req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+uuid.New().String()+"/logs", bytes.NewReader(body))
		req.SetPathValue("id", uuid.New().String())
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler(w, req)

		// Accept any response class; just ensure no panic or crash.
		_ = w.Code
	})
}
