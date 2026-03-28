package nodeagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Agent mock-server helpers
// ---------------------------------------------------------------------------

type agentServerConfig struct {
	heartbeatStatus  int
	heartbeatCounter *int
}

type agentServerOption func(*agentServerConfig)

func withHeartbeatStatus(code int) agentServerOption {
	return func(c *agentServerConfig) { c.heartbeatStatus = code }
}

func withHeartbeatCounter(counter *int) agentServerOption {
	return func(c *agentServerConfig) { c.heartbeatCounter = counter }
}

// newAgentMockServer creates an httptest.Server that handles heartbeat and claim
// endpoints for the given nodeID. By default heartbeat returns 200 and claim
// returns 200 with an empty JSON object (no work).
func newAgentMockServer(t *testing.T, nodeID string, opts ...agentServerOption) *httptest.Server {
	t.Helper()
	sc := agentServerConfig{heartbeatStatus: http.StatusOK}
	for _, o := range opts {
		o(&sc)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeID + "/heartbeat":
			if sc.heartbeatCounter != nil {
				*sc.heartbeatCounter++
			}
			w.WriteHeader(sc.heartbeatStatus)
		case "/v1/nodes/" + nodeID + "/claim":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// Claim server helpers
// ---------------------------------------------------------------------------

// newSingleClaimServer returns a server that serves the given ClaimResponse
// on the /v1/nodes/{nodeID}/claim endpoint.
func newSingleClaimServer(t *testing.T, nodeID string, claim ClaimResponse) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeID + "/claim":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(claim)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// Artifact upload server helpers
// ---------------------------------------------------------------------------

type artifactServerConfig struct {
	status int
}

type artifactServerOption func(*artifactServerConfig)

func withArtifactStatus(code int) artifactServerOption {
	return func(c *artifactServerConfig) { c.status = code }
}

// artifactUploadCall records a single artifact upload.
type artifactUploadCall struct {
	Name   string
	Bundle []byte
}

// newArtifactUploadServer returns a test server that handles
// POST /v1/runs/{runID}/jobs/{jobID}/artifact and records every upload call.
// Each response gets a unique artifact ID. Default status is 201 Created.
func newArtifactUploadServer(t *testing.T, runID, jobID string, opts ...artifactServerOption) (*httptest.Server, *[]artifactUploadCall) {
	t.Helper()
	sc := artifactServerConfig{status: http.StatusCreated}
	for _, o := range opts {
		o(&sc)
	}
	calls := &[]artifactUploadCall{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/v1/runs/%s/jobs/%s/artifact", runID, jobID)
		if r.URL.Path != wantPath {
			return
		}
		var payload struct {
			Name   string `json:"name"`
			Bundle []byte `json:"bundle"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		*calls = append(*calls, artifactUploadCall{
			Name:   payload.Name,
			Bundle: payload.Bundle,
		})
		n := len(*calls)
		w.WriteHeader(sc.status)
		if sc.status == http.StatusCreated {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"artifact_bundle_id": fmt.Sprintf("artifact-id-%d", n),
				"cid":                fmt.Sprintf("bafy-test-cid-%d", n),
			})
		}
	}))
	t.Cleanup(ts.Close)
	return ts, calls
}

// ---------------------------------------------------------------------------
// Diff upload server helpers
// ---------------------------------------------------------------------------

type diffServerConfig struct {
	status int
}

type diffServerOption func(*diffServerConfig)

func withDiffStatus(code int) diffServerOption {
	return func(c *diffServerConfig) { c.status = code }
}

// diffUploadCall records a single diff upload.
type diffUploadCall struct {
	Patch   []byte
	Summary json.RawMessage
}

// newDiffUploadServer returns a test server that handles
// POST /v1/runs/{runID}/jobs/{jobID}/diff and records every upload call.
// Default status is 201 Created.
func newDiffUploadServer(t *testing.T, runID, jobID string, opts ...diffServerOption) (*httptest.Server, *[]diffUploadCall) {
	t.Helper()
	sc := diffServerConfig{status: http.StatusCreated}
	for _, o := range opts {
		o(&sc)
	}
	calls := &[]diffUploadCall{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/v1/runs/%s/jobs/%s/diff", runID, jobID)
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != wantPath {
			t.Errorf("expected path %s, got %s", wantPath, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		var payload struct {
			RunID   *json.RawMessage `json:"run_id"`
			Patch   []byte           `json:"patch"`
			Summary json.RawMessage  `json:"summary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
		}
		if payload.RunID != nil {
			t.Error("run_id should not be in payload (it's in URL)")
		}
		if payload.Patch == nil {
			t.Error("patch not present in payload")
		}
		*calls = append(*calls, diffUploadCall{
			Patch:   payload.Patch,
			Summary: payload.Summary,
		})
		w.WriteHeader(sc.status)
		if sc.status == http.StatusCreated {
			_ = json.NewEncoder(w).Encode(map[string]string{"diff_id": "test-diff-id"})
		}
	}))
	t.Cleanup(ts.Close)
	return ts, calls
}

// newUncalledServer returns a test server that fails the test if any request
// is received. Useful for verifying that client-side validation prevents
// network calls (e.g., size cap checks).
func newUncalledServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// Status capture server helpers
// ---------------------------------------------------------------------------

// statusCapture records the status string sent to a job-complete endpoint.
type statusCapture struct {
	Status string
}

type statusServerConfig struct {
	httpStatus int
}

type statusServerOption func(*statusServerConfig)

// withStatusHTTPCode sets the HTTP response code for the status capture server.
func withStatusHTTPCode(code int) statusServerOption {
	return func(c *statusServerConfig) { c.httpStatus = code }
}

// newStatusCaptureServer returns an httptest server that captures the status
// field from POST /v1/jobs/{jobID}/complete requests.
func newStatusCaptureServer(t *testing.T, jobID string, opts ...statusServerOption) (*httptest.Server, *statusCapture) {
	t.Helper()
	sc := statusServerConfig{httpStatus: http.StatusOK}
	for _, o := range opts {
		o(&sc)
	}
	cap := &statusCapture{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/jobs/"+jobID+"/complete" {
			var body struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				cap.Status = body.Status
			}
		}
		w.WriteHeader(sc.httpStatus)
	}))
	t.Cleanup(ts.Close)
	return ts, cap
}
