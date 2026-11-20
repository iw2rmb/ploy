package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	tea "charm.land/bubbletea/v2"

	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// loadRunsCmd returns a tea.Cmd that fetches the runs list.
func loadRunsCmd(ctx context.Context, client *http.Client, baseURL *url.URL, requestID uint64, createdBy, creator string, all bool) tea.Cmd {
	return func() tea.Msg {
		cmd := sharedclient.ListRunsCommand{
			CreatedBy: createdBy,
			Creator:   creator,
			All:       all,
			Client:    client,
			BaseURL:   baseURL,
			Limit:     100,
		}
		result, err := cmd.Run(ctx)
		return runsLoadedMsg{runs: result, requestID: requestID, err: err}
	}
}

// loadJobsCmd returns a tea.Cmd that fetches the jobs list.
func loadJobsCmd(ctx context.Context, client *http.Client, baseURL *url.URL, runID domaintypes.RunID, requestID uint64, previous []domainapi.JobListItem, createdBy string) tea.Cmd {
	return func() tea.Msg {
		if !runID.IsZero() {
			result, err := (cliruns.ListRunJobsCommand{Client: client, BaseURL: baseURL, RunID: runID}).Run(ctx)
			if err != nil {
				return jobsLoadedMsg{runID: runID, requestID: requestID, err: err}
			}
			jobs := runJobItems(result)
			return jobsLoadedMsg{runID: runID, requestID: requestID, jobs: jobs}
		}
		jobs, err := loadGlobalJobs(ctx, client, baseURL, previous, createdBy)
		return jobsLoadedMsg{runID: runID, requestID: requestID, jobs: jobs, err: err}
	}
}

// loadRunDetailsCmd returns a tea.Cmd that fetches the job total for the
// given run, used to populate its detail list.
func loadRunDetailsCmd(ctx context.Context, client *http.Client, baseURL *url.URL, runID domaintypes.RunID) tea.Cmd {
	return func() tea.Msg {
		jobs, err := cliruns.ListRunJobsCommand{
			Client:  client,
			BaseURL: baseURL,
			RunID:   runID,
		}.Run(ctx)
		if err != nil {
			return errMsg{err: fmt.Errorf("get run totals: %w", err)}
		}
		return runDetailsLoadedMsg{runID: runID, jobTotal: len(jobs.Jobs)}
	}
}

// runDetailsLoadedMsg carries run detail totals from async fetch.
type runDetailsLoadedMsg struct {
	runID    domaintypes.RunID
	jobTotal int
}

// errMsg carries an error from an async command.
type errMsg struct{ err error }
