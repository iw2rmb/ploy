package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/sync/errgroup"

	clijob "github.com/iw2rmb/ploy/internal/cli/job"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func runJobItems(result migsapi.ListRunJobsResponse) []domainapi.JobListItem {
	jobs := make([]domainapi.JobListItem, len(result.Jobs))
	for i, job := range result.Jobs {
		jobs[i] = domainapi.JobListItem{
			JobID: job.JobID, RunID: result.RunID, RepoID: result.RepoID, RepoURL: result.RepoURL,
			Name: job.Name, DisplayName: job.DisplayName, JobType: job.JobType,
			Status: job.Status, DurationMs: job.DurationMs,
			StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
			JobImage: job.JobImage, NodeID: job.NodeID,
		}
	}
	return jobs
}

// The global page owns membership and lifecycle state; run snapshots own names and timestamps.
// Only unchanged terminal rows can reuse timestamps: a running job can be claimed again between polls.
func loadGlobalJobs(ctx context.Context, client *http.Client, baseURL *url.URL, previous []domainapi.JobListItem, createdBy string) ([]domainapi.JobListItem, error) {
	page, err := (sharedclient.ListJobsCommand{Client: client, BaseURL: baseURL, Limit: 100, Creator: createdBy}).Run(ctx)
	if err != nil {
		return nil, err
	}
	prior := make(map[domaintypes.JobID]domainapi.JobListItem, len(previous))
	for _, job := range previous {
		prior[job.JobID] = job
	}
	jobs := append([]domainapi.JobListItem{}, page.Jobs...)
	byRun := make(map[domaintypes.RunID][]int)
	for index, job := range jobs {
		if old, ok := prior[job.JobID]; ok && old.RunID == job.RunID {
			jobs[index].DisplayName = old.DisplayName
			if jobs[index].RepoURL == "" {
				jobs[index].RepoURL = old.RepoURL
			}
		}
		terminal := job.Status == domaintypes.JobStatusSuccess || job.Status == domaintypes.JobStatusFail || job.Status == domaintypes.JobStatusError || job.Status == domaintypes.JobStatusCancelled
		if old, ok := prior[job.JobID]; terminal && ok && old.RunID == job.RunID && old.Status == job.Status && old.StartedAt != nil {
			copyJobPresentation(&jobs[index], old)
			continue
		}
		byRun[job.RunID] = append(byRun[job.RunID], index)
	}
	var group errgroup.Group
	group.SetLimit(4)
	var mu sync.Mutex
	var failures []error
	recordFailure := func(err error) {
		mu.Lock()
		failures = append(failures, err)
		mu.Unlock()
	}
	for runID, indices := range byRun {
		group.Go(func() error {
			command := cliruns.ListRunJobsCommand{Client: client, BaseURL: baseURL, RunID: runID}
			snapshot, err := command.Run(ctx)
			if err != nil {
				recordFailure(err)
				return nil
			}
			byID := make(map[domaintypes.JobID]domainapi.JobListItem)
			for _, job := range runJobItems(snapshot) {
				byID[job.JobID] = job
			}
			attempts := map[int32]error{snapshot.Attempt: nil}
			for _, index := range indices {
				if page.Jobs[index].RepoURL == "" && snapshot.RepoURL != "" {
					jobs[index].RepoURL = snapshot.RepoURL
				}
				listed := page.Jobs[index]
				job, found := byID[listed.JobID]
				if !found {
					status, err := (clijob.GetStatusCommand{Client: client, BaseURL: baseURL, JobID: listed.JobID}).Run(ctx)
					if err != nil {
						recordFailure(err)
						continue
					}
					if _, loaded := attempts[status.Attempt]; !loaded {
						command.Attempt = &status.Attempt
						snapshot, err := command.Run(ctx)
						attempts[status.Attempt] = err
						if err != nil {
							recordFailure(err)
							continue
						}
						for _, job := range runJobItems(snapshot) {
							byID[job.JobID] = job
						}
					}
					if attempts[status.Attempt] != nil {
						continue
					}
					job, found = byID[listed.JobID]
					if !found {
						recordFailure(fmt.Errorf("job %s is missing from run %s attempt %d", listed.JobID, runID, status.Attempt))
						continue
					}
				}
				copyJobPresentation(&jobs[index], job)
			}
			return nil
		})
	}
	_ = group.Wait()
	return jobs, errors.Join(failures...)
}

func copyJobPresentation(dst *domainapi.JobListItem, source domainapi.JobListItem) {
	dst.DisplayName = source.DisplayName
	if dst.RepoURL == "" {
		dst.RepoURL = source.RepoURL
	}
	if dst.Status == domaintypes.JobStatusCreated || dst.Status == domaintypes.JobStatusQueued {
		dst.StartedAt, dst.FinishedAt = nil, nil
		return
	}
	dst.StartedAt = source.StartedAt
	dst.FinishedAt = source.FinishedAt
}

// Jobs responses can omit repo_url. Each run summary identifies the repository for all of its jobs.
func fillJobRepoURLs(ctx context.Context, client *http.Client, baseURL *url.URL, jobs []domainapi.JobListItem, repos map[domaintypes.RunID]string) error {
	if repos == nil {
		repos = make(map[domaintypes.RunID]string)
	}
	for _, job := range jobs {
		if job.RepoURL != "" {
			repos[job.RunID] = job.RepoURL
		}
	}
	missing := make(map[domaintypes.RunID][]int)
	for index := range jobs {
		job := &jobs[index]
		if job.RepoURL == "" {
			job.RepoURL = repos[job.RunID]
			if job.RepoURL == "" {
				missing[job.RunID] = append(missing[job.RunID], index)
			}
		}
	}
	var group errgroup.Group
	group.SetLimit(4)
	for runID, indices := range missing {
		group.Go(func() error {
			run, err := (cliruns.GetStatusCommand{Client: client, BaseURL: baseURL, RunID: runID}).Run(ctx)
			if err != nil {
				return err
			}
			for _, index := range indices {
				jobs[index].RepoURL = run.RepoURL
			}
			return nil
		})
	}
	return group.Wait()
}
