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
