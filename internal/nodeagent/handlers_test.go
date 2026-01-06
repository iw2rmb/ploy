package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

type mockRunController struct {
	startCalled bool
	stopCalled  bool
	startErr    error
	stopErr     error
	lastStart   StartRunRequest
	lastStop    StopRunRequest

	// slotSem is a mock concurrency semaphore. If nil, AcquireSlot/ReleaseSlot
	// are no-ops. Tests can set this to simulate concurrency limiting.
	slotSem chan struct{}
}

func (m *mockRunController) StartRun(ctx context.Context, req StartRunRequest) error {
	m.startCalled = true
	m.lastStart = req
	return m.startErr
}

func (m *mockRunController) StopRun(ctx context.Context, req StopRunRequest) error {
	m.stopCalled = true
	m.lastStop = req
	return m.stopErr
}

// AcquireSlot implements RunController. If slotSem is set, blocks until a slot
// is available; otherwise returns immediately.
func (m *mockRunController) AcquireSlot(ctx context.Context) error {
	if m.slotSem == nil {
		return nil
	}
	select {
	case m.slotSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseSlot implements RunController. If slotSem is set, releases a slot.
func (m *mockRunController) ReleaseSlot() {
	if m.slotSem == nil {
		return
	}
	<-m.slotSem
}

func TestHandleRunStart(t *testing.T) {
	tests := []struct {
		name          string
		request       StartRunRequest
		controllerErr error
		wantStatus    int
		wantCalled    bool
	}{
		{
			name: "valid request",
			request: StartRunRequest{
				RunID:   types.RunID("run-123"),
				JobID:   types.JobID("job-123"),
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
				BaseRef: types.GitRef("main"),
			},
			wantStatus: http.StatusAccepted,
			wantCalled: true,
		},
		{
			name: "missing run_id",
			request: StartRunRequest{
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name: "missing repo_url",
			request: StartRunRequest{
				RunID: types.RunID("run-123"),
				JobID: types.JobID("job-123"),
			},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name: "missing job_id",
			request: StartRunRequest{
				RunID:   types.RunID("run-123"),
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name: "controller error",
			request: StartRunRequest{
				RunID:   types.RunID("run-123"),
				JobID:   types.JobID("job-123"),
				RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			},
			controllerErr: fmt.Errorf("execution failed"),
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRunController{startErr: tt.controllerErr}
			cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
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
		})
	}
}

func TestHandleRunStop(t *testing.T) {
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
				RunID:  "run-123",
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
				RunID: "run-123",
			},
			controllerErr: fmt.Errorf("stop failed"),
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRunController{stopErr: tt.controllerErr}
			cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
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
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
	mock := &mockRunController{}
	srv := &Server{cfg: cfg, controller: mock}

	req := httptest.NewRequest(http.MethodGet, "/v1/run/start", nil)
	w := httptest.NewRecorder()

	srv.handleRunStart(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRunStop_MethodNotAllowed(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
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
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
	mock := &mockRunController{}
	srv := &Server{cfg: cfg, controller: mock}

	req := httptest.NewRequest(http.MethodPost, "/v1/run/start", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleRunStart(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRunStop_InvalidJSON(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
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
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
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
