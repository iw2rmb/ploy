package transfer_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/transfer"
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
		if payload.Stage != "plan" {
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
		Stage:  "plan",
		Kind:   "repo",
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
    if _, err := client.UploadSlot(context.Background(), transfer.UploadSlotRequest{JobID: "j"}); err == nil {
        t.Fatalf("expected upload error")
    }
    if err := client.Commit(context.Background(), "x", transfer.CommitRequest{Size: 1, Digest: "d"}); err == nil {
        t.Fatalf("expected commit error")
    }
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
