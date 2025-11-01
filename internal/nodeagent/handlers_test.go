package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockRunController struct {
	startCalled bool
	stopCalled  bool
	startErr    error
	stopErr     error
	lastStart   StartRunRequest
	lastStop    StopRunRequest
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
				RunID:   "run-123",
				RepoURL: "https://github.com/example/repo.git",
				BaseRef: "main",
			},
			wantStatus: http.StatusAccepted,
			wantCalled: true,
		},
		{
			name: "missing run_id",
			request: StartRunRequest{
				RepoURL: "https://github.com/example/repo.git",
			},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name: "missing repo_url",
			request: StartRunRequest{
				RunID: "run-123",
			},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name: "controller error",
			request: StartRunRequest{
				RunID:   "run-123",
				RepoURL: "https://github.com/example/repo.git",
			},
			controllerErr: fmt.Errorf("execution failed"),
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRunController{startErr: tt.controllerErr}
			cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
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

			if tt.wantCalled && mock.lastStart.RunID != tt.request.RunID {
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
			cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
			srv := &Server{cfg: cfg, controller: mock}

			body, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
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

func TestHandleHealth(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
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
