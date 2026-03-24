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
			runs[i] = runSummary{ID: r.ID, MigName: r.MigName}
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

// errMsg carries an error from an async command.
type errMsg struct{ err error }
