package runs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestGetStatusCommandDelegates validates that GetStatusCommand issues a
// GET /v1/runs/{id} request and returns a Summary.
func TestGetStatusCommandDelegates(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/"+runID.String() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":"` + runID.String() + `","status":"running","mig_id":"` + migID.String() + `","spec_id":"` + specID.String() + `","created_at":"2024-01-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)

	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	cmd := GetStatusCommand{
		Client:  srv.Client(),
		BaseURL: base,
		RunID:   runID,
	}

	summary, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("GetStatusCommand.Run error: %v", err)
	}
	if summary.ID != runID {
		t.Fatalf("unexpected run id: %s", summary.ID)
	}
}
