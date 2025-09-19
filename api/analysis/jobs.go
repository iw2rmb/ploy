package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

func (d *AnalysisDispatcher) GetJob(ctx context.Context, jobID string) (*AnalysisJob, error) {
	data, err := d.kv.Get(fmt.Sprintf("ploy/analysis/jobs/%s", jobID))
	if err != nil {
		return nil, fmt.Errorf("failed to get job from Consul: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	var job AnalysisJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	if status, _ := d.kv.Get(fmt.Sprintf("ploy/analysis/jobs/%s/status", jobID)); status != nil {
		job.Status = string(status)
	}

	if job.Status == "completed" {
		if resultBytes, _ := d.kv.Get(fmt.Sprintf("ploy/analysis/jobs/%s/result", jobID)); resultBytes != nil {
			var result LanguageAnalysisResult
			if err := json.Unmarshal(resultBytes, &result); err == nil {
				job.Result = &result
			}
		}
	}

	return &job, nil
}

func (d *AnalysisDispatcher) WaitForCompletion(ctx context.Context, jobID string, timeout time.Duration) (*AnalysisJob, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			job, err := d.GetJob(ctx, jobID)
			if err != nil {
				return nil, err
			}

			if job.Status == "completed" || job.Status == "failed" {
				return job, nil
			}

			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for job %s", jobID)
			}
		}
	}
}

func (d *AnalysisDispatcher) ListJobs(ctx context.Context, limit int) ([]*AnalysisJob, error) {
	keys, err := d.kv.Keys("ploy/analysis/jobs/", "/")
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	jobs := make([]*AnalysisJob, 0, len(keys))
	for i, key := range keys {
		if i >= limit && limit > 0 {
			break
		}
		if key == "ploy/analysis/jobs/" {
			continue
		}
		jobID := key[len("ploy/analysis/jobs/"):]
		if jobID == "" {
			continue
		}
		job, err := d.GetJob(ctx, jobID)
		if err == nil {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

func (d *AnalysisDispatcher) CleanupOldJobs(ctx context.Context, maxAge time.Duration) error {
	jobs, err := d.ListJobs(ctx, 0)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	for _, job := range jobs {
		if job.Status != "completed" && job.Status != "failed" {
			continue
		}
		if job.CompletedAt == nil || !job.CompletedAt.Before(cutoff) {
			continue
		}
		if err := d.kv.Delete(fmt.Sprintf("ploy/analysis/jobs/%s", job.ID)); err != nil {
			return fmt.Errorf("failed to delete job %s: %w", job.ID, err)
		}
		deregister := d.deregisterFn
		if deregister == nil {
			deregister = orchestration.DeregisterJob
		}
		jobName := fmt.Sprintf("analysis-%s-%s", job.Analyzer, job.ID)
		_ = deregister(jobName, false)
	}

	return nil
}

func (d *AnalysisDispatcher) getLanguageForAnalyzer(analyzer string) string {
	switch analyzer {
	case "pylint", "flake8", "mypy", "bandit":
		return "python"
	case "eslint", "jshint", "tslint":
		return "javascript"
	case "golangci-lint", "go-vet":
		return "go"
	case "rubocop":
		return "ruby"
	case "phpstan", "psalm":
		return "php"
	case "spotbugs", "checkstyle", "pmd":
		return "java"
	default:
		return "unknown"
	}
}
