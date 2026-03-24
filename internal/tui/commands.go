package tui

import (
	"context"
	"net/http"
	"net/url"

	tea "charm.land/bubbletea/v2"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// loadMigsCmd returns a tea.Cmd that fetches the migrations list.
func loadMigsCmd(client *http.Client, baseURL *url.URL) tea.Cmd {
	return func() tea.Msg {
		cmd := clitui.ListMigsCommand{
			Client:  client,
			BaseURL: baseURL,
			Limit:   100,
		}
		result, err := cmd.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return migsLoadedMsg{migs: result.Migs}
	}
}

// loadRunsCmd returns a tea.Cmd that fetches the runs list.
func loadRunsCmd(client *http.Client, baseURL *url.URL) tea.Cmd {
	return func() tea.Msg {
		cmd := clitui.ListRunsCommand{
			Client:  client,
			BaseURL: baseURL,
			Limit:   100,
		}
		result, err := cmd.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		runs := make([]runSummary, len(result.Runs))
		for i, r := range result.Runs {
			runs[i] = runSummary{ID: r.ID, MigName: r.MigName, CreatedAt: r.CreatedAt}
		}
		return runsLoadedMsg{runs: runs}
	}
}

// loadJobsCmd returns a tea.Cmd that fetches the jobs list.
func loadJobsCmd(client *http.Client, baseURL *url.URL, runID *domaintypes.RunID) tea.Cmd {
	return func() tea.Msg {
		cmd := clitui.ListJobsCommand{
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

// loadMigDetailsCmd returns a tea.Cmd that fetches repo and run totals for the
// given migration, used to populate the S3 detail list.
func loadMigDetailsCmd(client *http.Client, baseURL *url.URL, migID domaintypes.MigID) tea.Cmd {
	return func() tea.Msg {
		repoCount, err := clitui.CountMigReposCommand{
			Client:  client,
			BaseURL: baseURL,
			MigID:   migID,
		}.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		runCount, err := clitui.CountMigRunsCommand{
			Client:  client,
			BaseURL: baseURL,
			MigID:   migID,
		}.Run(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return migDetailsLoadedMsg{repoTotal: repoCount, runTotal: runCount}
	}
}

// migDetailsLoadedMsg carries migration detail totals from async fetch.
type migDetailsLoadedMsg struct {
	repoTotal int
	runTotal  int
}

// errMsg carries an error from an async command.
type errMsg struct{ err error }
