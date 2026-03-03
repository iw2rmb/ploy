package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestBaseUploader_GetJobStatus(t *testing.T) {
	t.Parallel()

	jobID := types.NewJobID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/jobs/"+jobID.String()+"/status" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/jobs/"+jobID.String()+"/status")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job_id":"` + jobID.String() + `","status":"Running"}`))
	}))
	defer server.Close()

	uploader := &baseUploader{
		cfg:    Config{ServerURL: server.URL, NodeID: testNodeID},
		client: server.Client(),
	}

	status, err := uploader.GetJobStatus(context.Background(), jobID)
	if err != nil {
		t.Fatalf("GetJobStatus() error = %v", err)
	}
	if status != "Running" {
		t.Fatalf("status = %q, want %q", status, "Running")
	}
}

func TestBaseUploader_GetJobStatus_NonOK(t *testing.T) {
	t.Parallel()

	jobID := types.NewJobID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	uploader := &baseUploader{
		cfg:    Config{ServerURL: server.URL, NodeID: testNodeID},
		client: server.Client(),
	}

	_, err := uploader.GetJobStatus(context.Background(), jobID)
	if err == nil {
		t.Fatal("GetJobStatus() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("error = %v, want status code details", err)
	}
}
