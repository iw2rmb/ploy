package migs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/runs"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
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

func TestCancelResumeSubmitCommands(t *testing.T) {
	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	runIDStr := runID.String()
	migIDStr := migID.String()
	specIDStr := specID.String()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Server returns 201 Created with {run_id, mig_id, spec_id}.
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			RunID  string `json:"run_id"`
			MigID  string `json:"mig_id"`
			SpecID string `json:"spec_id"`
		}{
			RunID:  runIDStr,
			MigID:  migIDStr,
			SpecID: specIDStr,
		})
	})
	mux.HandleFunc("/v1/runs/"+runIDStr+"/status", func(w http.ResponseWriter, r *http.Request) {
		// Canonical RunSummary response shape for status.
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(migsapi.RunSummary{
			RunID:      runID,
			State:      migsapi.RunStatePending,
			Repository: "https://example.com/repo.git",
			Metadata: map[string]string{
				"repo_base_ref": "main",
			},
			Stages: make(map[domaintypes.JobID]migsapi.StageStatus),
		})
	})
	mux.HandleFunc("/v1/runs/"+runIDStr+"/cancel", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	// Submit
	sum, err := (SubmitCommand{
		Client:  srv.Client(),
		BaseURL: base,
		Request: domainapi.RunSubmitRequest{
			RepoURL: domaintypes.RepoURL("https://example.com/repo.git"),
			Ref:     domaintypes.GitRef("main"),
			Spec:    []byte("{}"),
		},
	}).Run(context.Background())
	if err != nil || sum.RunID != runID {
		t.Fatalf("submit err=%v run=%+v", err, sum)
	}
	// Cancel
	if err := (runs.CancelCommand{Client: srv.Client(), BaseURL: base, RunID: runID}).Run(context.Background()); err != nil {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestSubmitCommand_InvalidRepoURLScheme(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP request: %s %s", r.Method, r.URL.String())
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	_, err := (SubmitCommand{
		Client:  srv.Client(),
		BaseURL: base,
		Request: domainapi.RunSubmitRequest{
			RepoURL: domaintypes.RepoURL("http://example.com/repo.git"),
			Ref:     domaintypes.GitRef("main"),
		},
	}).Run(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid repo URL scheme")
	}
	if !strings.Contains(err.Error(), "repo_url") {
		t.Fatalf("expected error to mention repo_url, got %q", err.Error())
	}
}

func TestMigsCommandsErrorPaths(t *testing.T) {
	runID := domaintypes.NewRunID()
	runIDStr := runID.String()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad req"}`))
	})
	mux.HandleFunc("/v1/runs/"+runIDStr+"/status", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/v1/runs/"+runIDStr+"/cancel", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusTeapot) })
	mux.HandleFunc("/v1/migs/t", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	// Submit
	if _, err := (SubmitCommand{
		Client:  srv.Client(),
		BaseURL: base,
		Request: domainapi.RunSubmitRequest{
			RepoURL: domaintypes.RepoURL("https://example.com/repo.git"),
			Ref:     domaintypes.GitRef("main"),
			Spec:    []byte("{}"),
		},
	}).Run(context.Background()); err == nil {
		t.Fatal("expected submit error")
	}
	// Cancel
	if err := (runs.CancelCommand{Client: srv.Client(), BaseURL: base, RunID: runID}).Run(context.Background()); err == nil {
		t.Fatal("expected cancel error")
	}
}
