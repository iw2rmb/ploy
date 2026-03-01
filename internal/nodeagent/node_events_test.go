package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestNodeEventUploader_UploadRunEvent_RequestShape(t *testing.T) {
	t.Parallel()

	var (
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("newBaseUploader() error = %v", err)
	}

	runID := types.NewRunID()
	jobID := types.NewJobID()
	err = uploader.UploadRunEvent(
		context.Background(),
		runID,
		&jobID,
		"error",
		"node exploded",
		map[string]any{"component": "test"},
	)
	if err != nil {
		t.Fatalf("UploadRunEvent() error = %v", err)
	}

	if want := "/v1/nodes/" + testNodeID + "/events"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}

	if gotPayload["run_id"] != runID.String() {
		t.Fatalf("run_id = %v, want %q", gotPayload["run_id"], runID.String())
	}

	events, ok := gotPayload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("events = %v, want single event", gotPayload["events"])
	}

	event, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("event[0] is not an object: %T", events[0])
	}
	if event["job_id"] != jobID.String() {
		t.Fatalf("job_id = %v, want %q", event["job_id"], jobID.String())
	}
	if event["level"] != "error" {
		t.Fatalf("level = %v, want error", event["level"])
	}
	if event["message"] != "node exploded" {
		t.Fatalf("message = %v, want node exploded", event["message"])
	}
	meta, ok := event["meta"].(map[string]any)
	if !ok || meta["component"] != "test" {
		t.Fatalf("meta = %v, want component=test", event["meta"])
	}
	if _, ok := event["time"].(string); !ok {
		t.Fatalf("time must be present and string, got %T", event["time"])
	}
}

func TestClaimManager_ClaimAndExecute_EmitsRunEventWhenStartRunFails(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	jobID := types.NewJobID()
	repoID := types.NewMigRepoID()

	var (
		mu                sync.Mutex
		eventsCalls       int
		lastEventsPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + testNodeID + "/claim":
			resp := ClaimResponse{
				RunID:     runID,
				RepoID:    repoID,
				JobID:     jobID,
				JobName:   "mig-0",
				RepoURL:   types.RepoURL("https://github.com/acme/repo.git"),
				Status:    "Started",
				NodeID:    testNodeID,
				BaseRef:   "main",
				TargetRef: "feature/x",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/nodes/" + testNodeID + "/events":
			mu.Lock()
			eventsCalls++
			_ = json.NewDecoder(r.Body).Decode(&lastEventsPayload)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}
	controller := &mockRunController{startErr: errors.New("boom")}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager() error = %v", err)
	}
	claimer.preClaimCleanup = nil // nil means always proceed

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	claimed, execErr := claimer.claimAndExecute(ctx)
	if !claimed {
		t.Fatalf("claimed = false, want true")
	}
	if execErr == nil || !strings.Contains(execErr.Error(), "start run") {
		t.Fatalf("claimAndExecute() error = %v, want start run failure", execErr)
	}

	mu.Lock()
	defer mu.Unlock()
	if eventsCalls != 1 {
		t.Fatalf("events calls = %d, want 1", eventsCalls)
	}
	if lastEventsPayload["run_id"] != runID.String() {
		t.Fatalf("run_id = %v, want %q", lastEventsPayload["run_id"], runID.String())
	}
	events, ok := lastEventsPayload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("events = %v, want single event", lastEventsPayload["events"])
	}
	event, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("event payload type = %T, want object", events[0])
	}
	if event["message"] != "failed to start claimed job execution" {
		t.Fatalf("message = %v, want failed to start claimed job execution", event["message"])
	}
}
