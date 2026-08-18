package migs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// SubmitCommand submits a Migs run to the control plane.
// The command submits a single-repo run via POST /v1/runs, then fetches the
// canonical Migs-style RunSummary via GET /v1/runs/{id}/status for display.
type SubmitCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Request domainapi.RunSubmitRequest
}

// Run executes the submission against the control plane endpoint.
// POST /v1/runs returns 201 Created with {run_id, mig_id, spec_id}.
// GET /v1/runs/{id}/status returns the canonical RunSummary.
func (c SubmitCommand) Run(ctx context.Context) (migsapi.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return migsapi.RunSummary{}, fmt.Errorf("migs submit: %w", err)
	}

	reqBody := c.Request
	reqBody.RepoURL = domaintypes.RepoURL(strings.TrimSpace(reqBody.RepoURL.String()))
	if err := reqBody.RepoURL.Validate(); err != nil {
		return migsapi.RunSummary{}, fmt.Errorf("migs submit: repo_url: %w", err)
	}
	reqBody.Ref = domaintypes.GitRef(strings.TrimSpace(reqBody.Ref.String()))
	if err := reqBody.Ref.Validate(); err != nil {
		return migsapi.RunSummary{}, fmt.Errorf("migs submit: ref: %w", err)
	}
	if len(reqBody.Spec) == 0 {
		return migsapi.RunSummary{}, fmt.Errorf("migs submit: spec is required")
	}

	// Control-plane submission endpoint: POST /v1/runs
	endpoint := c.BaseURL.JoinPath("v1", "runs")

	// Server returns 201 Created with {run_id, mig_id, spec_id}.
	created, err := httpx.DoJSON[domainapi.CreateSingleRepoRunResponse](ctx, c.Client, http.MethodPost, endpoint.String(), reqBody, http.StatusCreated, "migs submit")
	if err != nil {
		return migsapi.RunSummary{}, err
	}
	if created.RunID.IsZero() {
		return migsapi.RunSummary{}, fmt.Errorf("migs submit: empty run_id in response")
	}
	return fetchRunSummary(ctx, c.BaseURL, c.Client, created.RunID)
}

func fetchRunSummary(ctx context.Context, baseURL *url.URL, httpClient *http.Client, runID domaintypes.RunID) (migsapi.RunSummary, error) {
	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "status")
	summary, err := httpx.DoJSON[migsapi.RunSummary](ctx, httpClient, http.MethodGet, endpoint.String(), nil, http.StatusOK, "migs submit")
	if err != nil {
		return migsapi.RunSummary{}, err
	}
	summary.RunID = runID
	return summary, nil
}
