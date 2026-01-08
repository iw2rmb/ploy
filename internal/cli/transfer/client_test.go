package transfer_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/transfer"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestUploadSlot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/transfers/upload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var payload transfer.UploadSlotRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.JobID != "job-1" {
			t.Fatalf("unexpected job id: %s", payload.JobID)
		}
		if payload.Stage != domaintypes.TransferStagePlan {
			t.Fatalf("unexpected stage: %s", payload.Stage)
		}
		resp := transfer.Slot{
			ID:         "slot-123",
			Kind:       payload.Kind,
			JobID:      payload.JobID,
			NodeID:     payload.NodeID,
			RemotePath: "/var/lib/ploy/slot-123",
			MaxSize:    1024,
			ExpiresAt:  time.Now().UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	base, _ := url.Parse(server.URL)
	client := transfer.Client{BaseURL: base, HTTPClient: server.Client()}
	slot, err := client.UploadSlot(context.Background(), transfer.UploadSlotRequest{
		JobID:  "job-1",
		Stage:  domaintypes.TransferStagePlan,
		Kind:   domaintypes.TransferKindRepo,
		NodeID: "node-a",
	})
	if err != nil {
		t.Fatalf("UploadSlot: %v", err)
	}
	if slot.ID != "slot-123" {
		t.Fatalf("unexpected slot id: %s", slot.ID)
	}
	if slot.RemotePath != "/var/lib/ploy/slot-123" {
		t.Fatalf("unexpected remote path: %s", slot.RemotePath)
	}
}

func TestTransferErrors(t *testing.T) {
	client := transfer.Client{}
	if _, err := client.UploadSlot(context.Background(), transfer.UploadSlotRequest{}); err == nil {
		t.Fatalf("expected base URL required error")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transfers/upload", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusTeapot) })
	mux.HandleFunc("/v1/transfers/x/commit", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad", http.StatusBadRequest) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	client = transfer.Client{BaseURL: base, HTTPClient: srv.Client()}
	if _, err := client.UploadSlot(context.Background(), transfer.UploadSlotRequest{JobID: "j", Kind: domaintypes.TransferKindRepo, NodeID: "node-a"}); err == nil {
		t.Fatalf("expected upload error")
	}
	if err := client.Commit(context.Background(), "x", transfer.CommitRequest{Size: 1, Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}); err == nil {
		t.Fatalf("expected commit error")
	}
}

func TestDigestValidate(t *testing.T) {
	var hit atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	client := transfer.Client{BaseURL: base, HTTPClient: srv.Client()}

	t.Run("invalid digest rejected before HTTP", func(t *testing.T) {
		hit.Store(false)
		if err := client.Commit(context.Background(), "slot-1", transfer.CommitRequest{Size: 1, Digest: "not-a-digest"}); err == nil {
			t.Fatalf("expected digest validation error")
		}
		if hit.Load() {
			t.Fatalf("expected digest validation to fail before HTTP")
		}
	})

	t.Run("whitespace-only digest rejected before HTTP", func(t *testing.T) {
		hit.Store(false)
		if err := client.Commit(context.Background(), "slot-1", transfer.CommitRequest{Size: 1, Digest: "   "}); err == nil {
			t.Fatalf("expected digest validation error")
		}
		if hit.Load() {
			t.Fatalf("expected digest validation to fail before HTTP")
		}
	})
}

func TestAbortSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transfers/x/abort", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	client := transfer.Client{BaseURL: base, HTTPClient: srv.Client()}
	if err := client.Abort(context.Background(), "x"); err != nil {
		t.Fatalf("Abort error: %v", err)
	}
}

func TestDownloadSlot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/transfers/download" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var payload transfer.DownloadSlotRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.JobID != "job-2" {
			t.Fatalf("unexpected job id: %s", payload.JobID)
		}
		if payload.Kind != domaintypes.TransferKindArtifact {
			t.Fatalf("unexpected kind: %s", payload.Kind)
		}
		resp := transfer.Slot{
			ID:         "slot-dl-456",
			Kind:       payload.Kind,
			JobID:      payload.JobID,
			NodeID:     payload.NodeID,
			RemotePath: "/var/lib/ploy/slot-dl-456",
			MaxSize:    2048,
			ExpiresAt:  time.Now().UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	base, _ := url.Parse(server.URL)
	client := transfer.Client{BaseURL: base, HTTPClient: server.Client()}
	slot, err := client.DownloadSlot(context.Background(), transfer.DownloadSlotRequest{
		JobID:      "job-2",
		Kind:       domaintypes.TransferKindArtifact,
		ArtifactID: "art-123",
		NodeID:     "node-b",
	})
	if err != nil {
		t.Fatalf("DownloadSlot: %v", err)
	}
	if slot.ID != "slot-dl-456" {
		t.Fatalf("unexpected slot id: %s", slot.ID)
	}
	if slot.RemotePath != "/var/lib/ploy/slot-dl-456" {
		t.Fatalf("unexpected remote path: %s", slot.RemotePath)
	}
}

func TestCommitSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transfers/slot-999/commit", func(w http.ResponseWriter, r *http.Request) {
		var req transfer.CommitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode commit request: %v", err)
		}
		if req.Size != 1024 {
			t.Fatalf("unexpected size: %d", req.Size)
		}
		if req.Digest != "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
			t.Fatalf("unexpected digest: %s", req.Digest)
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	client := transfer.Client{BaseURL: base, HTTPClient: srv.Client()}
	err := client.Commit(context.Background(), "slot-999", transfer.CommitRequest{
		Size:   1024,
		Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("Commit error: %v", err)
	}
}

func TestCommitNoContentAndEmptyIDValidation(t *testing.T) {
	// Empty slot id should be rejected client-side.
	baseURL, _ := url.Parse("http://127.0.0.1") // base is irrelevant; validation triggers before HTTP
	c := transfer.Client{BaseURL: baseURL, HTTPClient: http.DefaultClient}
	if err := c.Commit(context.Background(), "", transfer.CommitRequest{Size: 1, Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}); err == nil || err.Error() != "transfer: slot id required" {
		t.Fatalf("expected slot id validation error, got: %v", err)
	}

	// 204 No Content is a valid success for commit endpoints.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transfers/ok/commit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	client := transfer.Client{BaseURL: base, HTTPClient: srv.Client()}
	if err := client.Commit(context.Background(), "ok", transfer.CommitRequest{Size: 1, Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}); err != nil {
		t.Fatalf("Commit 204 No Content unexpected error: %v", err)
	}
}

func TestUploadSlotDecodeError(t *testing.T) {
	// Server returns invalid JSON; client should surface a decode error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/transfers/upload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not-json}"))
	}))
	defer server.Close()

	base, _ := url.Parse(server.URL)
	client := transfer.Client{BaseURL: base, HTTPClient: server.Client()}
	if _, err := client.UploadSlot(context.Background(), transfer.UploadSlotRequest{JobID: "j", Kind: domaintypes.TransferKindRepo, NodeID: "node-a"}); err == nil {
		t.Fatalf("expected JSON decode error")
	}
}
