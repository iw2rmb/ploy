package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Ensures ClaimResponse fields map 1:1 into StartRunRequest.
func TestClaimLoop_MapsClaimToStartRunRequest(t *testing.T) {
	t.Parallel()

	commit := "deadbeef"
	claim := ClaimResponse{
		ID:        "run-map-1",
		RepoURL:   "https://github.com/acme/thing.git",
		Status:    "assigned",
		NodeID:    "test-node",
		BaseRef:   "main",
		TargetRef: "feature/x",
		CommitSha: &commit,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// HTTP test server that returns a single claim and accepts ack.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/test-node/buildgate/claim":
			// No buildgate jobs for this mapping test.
			w.WriteHeader(http.StatusNoContent)
			return
		case "/v1/nodes/test-node/claim":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(claim)
		case "/v1/nodes/test-node/ack":
			w.WriteHeader(http.StatusNoContent)
		case "/v1/nodes/test-node/complete":
			// Allow terminal status attempts to succeed quickly.
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Capture StartRunRequest via a mock controller.
	mock := &mockRunController{}

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    "test-node",
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}

	claimer, err := NewClaimManager(cfg, mock)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	claimer.minBackoff = 10 * time.Millisecond
	claimer.maxBackoff = 20 * time.Millisecond

	// Run briefly to process one claim.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = claimer.Start(ctx)

	if !mock.startCalled {
		t.Fatalf("controller.StartRun not called")
	}
	got := mock.lastStart
	if got.RunID.String() != claim.ID {
		t.Errorf("RunID=%q want %q", got.RunID, claim.ID)
	}
	if got.RepoURL.String() != claim.RepoURL {
		t.Errorf("RepoURL=%q want %q", got.RepoURL, claim.RepoURL)
	}
	if got.BaseRef.String() != claim.BaseRef {
		t.Errorf("BaseRef=%q want %q", got.BaseRef, claim.BaseRef)
	}
	if got.TargetRef.String() != claim.TargetRef {
		t.Errorf("TargetRef=%q want %q", got.TargetRef, claim.TargetRef)
	}
	if got.CommitSHA.String() != *claim.CommitSha {
		t.Errorf("CommitSHA=%q want %q", got.CommitSHA, *claim.CommitSha)
	}
}
