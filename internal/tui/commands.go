package tui

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	tea "charm.land/bubbletea/v2"

	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// loadRunsCmd returns a tea.Cmd that fetches the runs list.
func loadRunsCmd(ctx context.Context, client *http.Client, baseURL *url.URL, requestID uint64, createdBy, creator string, all bool, selectedRunID domaintypes.RunID) tea.Cmd {
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
		msg := runsLoadedMsg{runs: result, requestID: requestID, err: err}
		if selectedRunID.IsZero() || err != nil {
			return msg
		}
		// RUNS keeps its first page even when a confirmed RUN moves to an older page.
		for page := result; ; {
			for _, run := range page {
				if run.ID == selectedRunID {
					if cmd.Offset > 0 {
						msg.offPageRun = &run
					}
					return msg
				}
			}
			if len(page) < int(cmd.Limit) {
				return msg
			}
			cmd.Offset += cmd.Limit
			page, msg.err = cmd.Run(ctx)
			if msg.err != nil {
				return msg
			}
		}
	}
}

// loadJobsCmd returns a tea.Cmd that fetches the jobs list.
func loadJobsCmd(ctx context.Context, client *http.Client, baseURL *url.URL, runID domaintypes.RunID, requestID uint64, previous []domainapi.JobListItem, createdBy string, repos map[domaintypes.RunID]string) tea.Cmd {
	return func() tea.Msg {
		msg := jobsLoadedMsg{runID: runID, requestID: requestID}
		if client != nil && client.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, client.Timeout)
			defer cancel()
		}
		if runID.IsZero() {
			msg.jobs, msg.err = loadGlobalJobs(ctx, client, baseURL, previous, createdBy)
		} else {
			result, err := (cliruns.ListRunJobsCommand{Client: client, BaseURL: baseURL, RunID: runID}).Run(ctx)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.jobs = runJobItems(result)
		}
		repoErr := fillJobRepoURLs(ctx, client, baseURL, msg.jobs, repos)
		msg.err = errors.Join(msg.err, repoErr)
		return msg
	}
}

type runDetails struct {
	runID    domaintypes.RunID
	attempt  int32
	jobTotal *int
	nodeName string
}

type runDetailsLoadedMsg struct {
	runDetails
	requestID uint64
	err       error
}

func loadRunDetailsCmd(ctx context.Context, client *http.Client, baseURL *url.URL, details runDetails, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := runDetailsLoadedMsg{runDetails: details, requestID: requestID}
		cmd := cliruns.ListRunJobsCommand{Client: client, BaseURL: baseURL, RunID: details.runID}
		if details.attempt > 0 {
			cmd.Attempt = &details.attempt
		}
		jobs, err := cmd.Run(ctx)
		if err != nil {
			msg.err = err
			return msg
		}
		total := len(jobs.Jobs)
		msg.jobTotal = &total
		if details.nodeName != "" {
			return msg
		}
		for _, job := range jobs.Jobs {
			if job.NodeID == nil || job.NodeID.IsZero() {
				continue
			}
			nodes, err := (sharedclient.ListNodesCommand{Client: client, BaseURL: baseURL}).Run(ctx)
			msg.err = err
			for _, node := range nodes {
				if node.ID == *job.NodeID {
					msg.nodeName = node.Name
					break
				}
			}
			break
		}
		return msg
	}
}
