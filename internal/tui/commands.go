package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	tea "charm.land/bubbletea/v2"

	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
	clitui "github.com/iw2rmb/ploy/internal/client/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// loadMigsCmd returns a tea.Cmd that fetches the migrations list.
func loadMigsCmd(client *http.Client, baseURL *url.URL) tea.Cmd {
	return func() tea.Msg {
		cmd := sharedclient.ListMigsCommand{
			Client:  client,
			BaseURL: baseURL,
			Limit:   100,
		}
		result, err := cmd.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return migsLoadedMsg{migs: result}
	}
}

// loadRunsCmd returns a tea.Cmd that fetches the runs list.
func loadRunsCmd(client *http.Client, baseURL *url.URL) tea.Cmd {
	return func() tea.Msg {
		cmd := sharedclient.ListRunsCommand{
			Client:  client,
			BaseURL: baseURL,
			Limit:   100,
		}
		result, err := cmd.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		runs := make([]runSummary, len(result))
		for i, r := range result {
			runs[i] = runSummary{ID: r.ID, MigID: r.MigID, MigName: r.MigName, CreatedAt: r.CreatedAt}
		}
		return runsLoadedMsg{runs: runs}
	}
}

// loadJobsCmd returns a tea.Cmd that fetches the jobs list.
func loadJobsCmd(client *http.Client, baseURL *url.URL, runID *domaintypes.RunID) tea.Cmd {
	return func() tea.Msg {
		cmd := sharedclient.ListJobsCommand{
			Client:  client,
			BaseURL: baseURL,
			Limit:   100,
			RunID:   runID,
		}
		result, err := cmd.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return jobsLoadedMsg{jobs: result.Jobs}
	}
}

// loadMigDetailsCmd returns a tea.Cmd that fetches the run total for the
// given migration, used to populate the S3 detail list.
func loadMigDetailsCmd(client *http.Client, baseURL *url.URL, migID domaintypes.MigID) tea.Cmd {
	return func() tea.Msg {
		runCount, err := clitui.CountMigRunsCommand{
			Client:  client,
			BaseURL: baseURL,
			MigID:   migID,
		}.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return migDetailsLoadedMsg{runTotal: runCount}
	}
}

// migDetailsLoadedMsg carries migration detail totals from async fetch.
type migDetailsLoadedMsg struct {
	runTotal int
}

// loadRunDetailsCmd returns a tea.Cmd that fetches the job total for the
// given run, used to populate the S5 detail list.
func loadRunDetailsCmd(client *http.Client, baseURL *url.URL, runID domaintypes.RunID) tea.Cmd {
	return func() tea.Msg {
		jobs, err := sharedclient.ListJobsCommand{
			Client:  client,
			BaseURL: baseURL,
			Limit:   1,
			RunID:   &runID,
		}.Run(context.Background())
		if err != nil {
			return errMsg{err: fmt.Errorf("get run totals: %w", err)}
		}
		return runDetailsLoadedMsg{jobTotal: int(jobs.Total)}
	}
}

// runDetailsLoadedMsg carries run detail totals from async fetch.
type runDetailsLoadedMsg struct {
	jobTotal int
}

// loadJobDetailsCmd fetches run job details for the confirmed job.
func loadJobDetailsCmd(client *http.Client, baseURL *url.URL, runID domaintypes.RunID, jobID domaintypes.JobID) tea.Cmd {
	return func() tea.Msg {
		result, err := cliruns.ListRunJobsCommand{
			Client:  client,
			BaseURL: baseURL,
			RunID:   runID,
		}.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		for _, job := range result.Jobs {
			if job.JobID == jobID {
				j := job
				return jobDetailsLoadedMsg{detail: &j}
			}
		}
		return jobDetailsLoadedMsg{detail: nil}
	}
}

// errMsg carries an error from an async command.
type errMsg struct{ err error }
