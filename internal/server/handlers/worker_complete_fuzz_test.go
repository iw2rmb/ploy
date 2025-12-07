package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/store"
)

// FuzzCompleteRun_StatsShapes fuzzes the complete handler with various JSON
// object shapes for the `stats` field to ensure acceptance of arbitrary
// objects and rejection of non-objects. This target is fast and deterministic
// and only runs under -fuzz; normal `go test` is unaffected.
func FuzzCompleteRun_StatsShapes(f *testing.F) {
	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	// Seeds: valid object, nested object, empty object, array (invalid), string (invalid)
	seeds := [][]byte{
		[]byte(`{"stats": {"exit_code": 0}}`),
		[]byte(`{"stats": {"nested": {"a": 1}}}`),
		[]byte(`{"stats": {}}`),
		[]byte(`{"stats": [1,2,3]}`),
		[]byte(`{"stats": "oops"}`),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, statsJSON []byte) {
		// Set up job for GetJob lookup (job_id-based).
		nodeIDStr := nodeID.String()
		job := store.Job{
			ID:        jobID.String(),
			RunID:     runID.String(),
			NodeID:    &nodeIDStr,
			Status:    store.JobStatusRunning,
			StepIndex: 1000,
		}
		st := &mockStore{
			getNodeResult: store.Node{ID: nodeID.String()},
			getRunResult: store.Run{
				ID:     runID.String(),
				Status: store.RunStatusRunning,
			},
			getJobResult:        job,
			listJobsByRunResult: []store.Job{job},
		}
		h := completeRunHandler(st, nil)

		// Build payload with fuzzed stats; always send a terminal status with job_id.
		var payload map[string]any
		_ = json.Unmarshal([]byte(`{"run_id":"`+runID.String()+`","job_id":"`+jobID.String()+`","status":"succeeded","step_index":1000}`), &payload)
		var stats any
		if err := json.Unmarshal(statsJSON, &stats); err == nil {
			if m, ok := stats.(map[string]any); ok {
				payload["stats"] = m
			} else {
				// Non-object should be rejected; encode as-is for negative path.
				payload["stats"] = stats
			}
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
		req.SetPathValue("id", nodeID.String())
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		// Accept 204 for object stats; 400 for non-object stats.
		if _, isMap := payload["stats"].(map[string]any); isMap || payload["stats"] == nil {
			if rr.Code != http.StatusNoContent {
				t.Fatalf("want 204 for object stats, got %d", rr.Code)
			}
		} else {
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("want 400 for non-object stats, got %d", rr.Code)
			}
		}
	})
}
