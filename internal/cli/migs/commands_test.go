package migs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestArtifactsCommand(t *testing.T) {
	runID := domaintypes.NewRunID()
	buildJobID := domaintypes.NewJobID()
	testJobID := domaintypes.NewJobID()

	run := migsapi.RunSummary{
		RunID: runID,
		State: migsapi.RunStateSucceeded,
		Stages: map[domaintypes.JobID]migsapi.StageStatus{
			buildJobID: {State: migsapi.StageStateSucceeded, Artifacts: map[string]string{"bin": "cid1"}},
			testJobID:  {State: migsapi.StageStateSucceeded},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return RunSummary directly — the canonical response shape.
		_ = json.NewEncoder(w).Encode(run)
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	var out bytes.Buffer
	if err := (ArtifactsCommand{Client: srv.Client(), BaseURL: base, RunID: runID, Output: &out}).Run(context.Background()); err != nil {
		t.Fatalf("artifacts run: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected artifacts to write output")
	}
}

func TestCancelCommand(t *testing.T) {
	runID := domaintypes.NewRunID()
	runIDStr := runID.String()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs/"+runIDStr+"/cancel", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	if err := (runs.CancelCommand{Client: srv.Client(), BaseURL: base, RunID: runID}).Run(context.Background()); err != nil {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestCancelCommandError(t *testing.T) {
	runID := domaintypes.NewRunID()
	runIDStr := runID.String()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs/"+runIDStr+"/cancel", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusTeapot) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	if err := (runs.CancelCommand{Client: srv.Client(), BaseURL: base, RunID: runID}).Run(context.Background()); err == nil {
		t.Fatal("expected cancel error")
	}
}
