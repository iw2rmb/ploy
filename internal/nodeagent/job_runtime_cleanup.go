package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const abandonedRuntimeSweepTimeout = 30 * time.Second

type jobStatusReader interface {
	GetJobStatus(context.Context, types.JobID) (string, error)
}

type abandonedRuntimeCandidate struct {
	jobID types.JobID
	dirs  JobDirectories
}

func (r *runController) sweepAbandonedRuntimeIfIdle() {
	if r == nil || r.uploader == nil {
		return
	}

	r.sweepMu.Lock()
	defer r.sweepMu.Unlock()
	if !r.nodeIsIdle() {
		return
	}

	candidates, err := abandonedRuntimeCandidates()
	if err != nil {
		slog.Warn("failed to list abandoned job runtime directories", "error", err)
		return
	}
	if len(candidates) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), abandonedRuntimeSweepTimeout)
	defer cancel()
	terminal, err := terminalRuntimeCandidates(ctx, r.uploader, candidates)
	if err != nil {
		slog.Warn("abandoned job runtime status check was incomplete", "error", err)
	}
	if len(terminal) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.jobs) != 0 || len(r.jobSem) != 0 {
		return
	}
	for _, candidate := range terminal {
		if err := cleanupJobRuntime(candidate.dirs); err != nil {
			slog.Warn("failed to sweep abandoned job runtime", "job_id", candidate.jobID, "path", candidate.dirs.Root, "error", err)
		}
	}
}

func (r *runController) nodeIsIdle() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.jobs) == 0 && len(r.jobSem) == 0
}

func abandonedRuntimeCandidates() ([]abandonedRuntimeCandidate, error) {
	runEntries, err := os.ReadDir(cacheRootDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read runs directory: %w", err)
	}

	var candidates []abandonedRuntimeCandidate
	for _, runEntry := range runEntries {
		if !runEntry.IsDir() {
			continue
		}
		var runID types.RunID
		if err := runID.UnmarshalText([]byte(runEntry.Name())); err != nil {
			continue
		}
		jobEntries, err := os.ReadDir(jobsDir(runID))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read jobs for run %s: %w", runID, err)
		}
		for _, jobEntry := range jobEntries {
			if !jobEntry.IsDir() {
				continue
			}
			var jobID types.JobID
			if err := jobID.UnmarshalText([]byte(jobEntry.Name())); err != nil {
				continue
			}
			dirs := jobDirectories(runID, jobID)
			if jobRuntimeExists(dirs) {
				candidates = append(candidates, abandonedRuntimeCandidate{jobID: jobID, dirs: dirs})
			}
		}
	}
	return candidates, nil
}

func jobRuntimeExists(dirs JobDirectories) bool {
	for _, dir := range []string{dirs.Cache, dirs.Home, dirs.Staging, dirs.Tmp} {
		if _, err := os.Lstat(dir); err == nil {
			return true
		}
	}
	return false
}

func terminalRuntimeCandidates(ctx context.Context, reader jobStatusReader, candidates []abandonedRuntimeCandidate) ([]abandonedRuntimeCandidate, error) {
	terminal := make([]abandonedRuntimeCandidate, 0, len(candidates))
	var firstErr error
	for _, candidate := range candidates {
		statusText, err := reader.GetJobStatus(ctx, candidate.jobID)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("get status for job %s: %w", candidate.jobID, err)
			}
			continue
		}
		status, err := types.ParseJobStatus(statusText)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("parse status for job %s: %w", candidate.jobID, err)
			}
			continue
		}
		if isTerminalRuntimeStatus(status) {
			terminal = append(terminal, candidate)
		}
	}
	return terminal, firstErr
}

func isTerminalRuntimeStatus(status types.JobStatus) bool {
	switch status {
	case types.JobStatusSuccess, types.JobStatusFail, types.JobStatusError, types.JobStatusCancelled:
		return true
	default:
		return false
	}
}
