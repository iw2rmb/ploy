package nodeagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHandleRunStart(t *testing.T) {
	runID := types.NewRunID()
	jobID := types.NewJobID()
	tests := []struct {
		name          string
		request       StartRunRequest
		controllerErr error
		wantStatus    int
		wantCalled    bool
		wantAcquire   int
		wantRelease   int
	}{
		{
			name: "valid request",
			request: StartRunRequest{
				RunID:   runID,
				JobID:   jobID,
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
				BaseRef: types.GitRef("main"),
				JobType: types.JobTypeMod,
			},
			wantStatus:  http.StatusAccepted,
			wantCalled:  true,
			wantAcquire: 1,
			wantRelease: 0,
		},
		{
			name: "missing run_id",
			request: StartRunRequest{
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
				JobType: types.JobTypeMod,
			},
			wantStatus:  http.StatusBadRequest,
			wantCalled:  false,
			wantAcquire: 0,
			wantRelease: 0,
		},
		{
			name: "missing repo_url",
			request: StartRunRequest{
				RunID:   runID,
				JobID:   jobID,
				JobType: types.JobTypeMod,
			},
			wantStatus:  http.StatusBadRequest,
			wantCalled:  false,
			wantAcquire: 0,
			wantRelease: 0,
		},
		{
			name: "missing job_id",
			request: StartRunRequest{
				RunID:   runID,
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
				JobType: types.JobTypeMod,
			},
			wantStatus:  http.StatusBadRequest,
			wantCalled:  false,
			wantAcquire: 0,
			wantRelease: 0,
		},
		{
			name: "controller error",
			request: StartRunRequest{
				RunID:   runID,
				JobID:   jobID,
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
				JobType: types.JobTypeMod,
			},
			controllerErr: fmt.Errorf("execution failed"),
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
			wantAcquire:   1,
			wantRelease:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRunController{startErr: tt.controllerErr}
			cfg := Config{NodeID: "aB3xY9", ServerURL: "http://127.0.0.1:8080"}
			srv := &Server{cfg: cfg, controller: mock}

			body, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/run/start", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.handleRunStart(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if mock.startCalled != tt.wantCalled {
				t.Errorf("controller.StartRun called = %v, want %v", mock.startCalled, tt.wantCalled)
			}

			if tt.wantCalled && mock.lastStart.RunID.String() != tt.request.RunID.String() {
				t.Errorf("controller received RunID = %q, want %q", mock.lastStart.RunID, tt.request.RunID)
			}

			if mock.acquireCalls != tt.wantAcquire {
				t.Errorf("controller.AcquireSlot calls = %d, want %d", mock.acquireCalls, tt.wantAcquire)
			}
			if mock.releaseCalls != tt.wantRelease {
				t.Errorf("controller.ReleaseSlot calls = %d, want %d", mock.releaseCalls, tt.wantRelease)
			}
		})
	}
}

func TestHandleRunStop(t *testing.T) {
	runID := types.NewRunID()
	tests := []struct {
		name          string
		request       StopRunRequest
		controllerErr error
		wantStatus    int
		wantCalled    bool
	}{
		{
			name: "valid request",
			request: StopRunRequest{
				RunID:  runID,
				Reason: "user requested",
			},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name: "missing run_id",
			request: StopRunRequest{
				Reason: "user requested",
			},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name: "controller error",
			request: StopRunRequest{
				RunID: runID,
			},
			controllerErr: fmt.Errorf("stop failed"),
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRunController{stopErr: tt.controllerErr}
			cfg := Config{NodeID: "aB3xY9", ServerURL: "http://127.0.0.1:8080"}
			srv := &Server{cfg: cfg, controller: mock}

			// Use raw JSON for "missing run_id" test case to avoid MarshalJSON error on empty RunID.
			var body []byte
			if tt.request.RunID.IsZero() {
				body = []byte(`{"reason":"user requested"}`)
			} else {
				var err error
				body, err = json.Marshal(tt.request)
				if err != nil {
					t.Fatalf("marshal request: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/run/stop", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.handleRunStop(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if mock.stopCalled != tt.wantCalled {
				t.Errorf("controller.StopRun called = %v, want %v", mock.stopCalled, tt.wantCalled)
			}

			if tt.wantCalled && mock.lastStop.RunID != tt.request.RunID {
				t.Errorf("controller received RunID = %q, want %q", mock.lastStop.RunID, tt.request.RunID)
			}
		})
	}
}

func TestHandleRunStart_MethodNotAllowed(t *testing.T) {
	cfg := Config{NodeID: "aB3xY9", ServerURL: "http://127.0.0.1:8080"}
	mock := &mockRunController{}
	srv := &Server{cfg: cfg, controller: mock}

	req := httptest.NewRequest(http.MethodGet, "/v1/run/start", nil)
	w := httptest.NewRecorder()

	srv.handleRunStart(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}

	if mock.acquireCalls != 0 || mock.releaseCalls != 0 {
		t.Fatalf("AcquireSlot/ReleaseSlot calls = %d/%d, want 0/0", mock.acquireCalls, mock.releaseCalls)
	}
}

func TestHandleRunStop_MethodNotAllowed(t *testing.T) {
	cfg := Config{NodeID: "aB3xY9", ServerURL: "http://127.0.0.1:8080"}
	mock := &mockRunController{}
	srv := &Server{cfg: cfg, controller: mock}

	req := httptest.NewRequest(http.MethodGet, "/v1/run/stop", nil)
	w := httptest.NewRecorder()

	srv.handleRunStop(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRunStart_InvalidJSON(t *testing.T) {
	cfg := Config{NodeID: testNodeID, ServerURL: "http://127.0.0.1:8080"}
	mock := &mockRunController{}
	srv := &Server{cfg: cfg, controller: mock}

	req := httptest.NewRequest(http.MethodPost, "/v1/run/start", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleRunStart(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if mock.acquireCalls != 0 || mock.releaseCalls != 0 {
		t.Fatalf("AcquireSlot/ReleaseSlot calls = %d/%d, want 0/0", mock.acquireCalls, mock.releaseCalls)
	}
}

func TestHandleRunStop_InvalidJSON(t *testing.T) {
	cfg := Config{NodeID: testNodeID, ServerURL: "http://127.0.0.1:8080"}
	mock := &mockRunController{}
	srv := &Server{cfg: cfg, controller: mock}

	req := httptest.NewRequest(http.MethodPost, "/v1/run/stop", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleRunStop(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleHealth(t *testing.T) {
	cfg := Config{NodeID: testNodeID, ServerURL: "http://127.0.0.1:8080"}
	mock := &mockRunController{}
	srv := &Server{cfg: cfg, controller: mock}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}
