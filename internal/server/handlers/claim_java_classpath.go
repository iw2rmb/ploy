package handlers

import (
	"context"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func resolveJavaClasspathClaimContext(
	ctx context.Context,
	st store.Store,
	job store.Job,
) (*contracts.JavaClasspathClaimContext, error) {
	jobType := domaintypes.JobType(job.JobType)
	if jobType == domaintypes.JobTypeSBOM || jobType.IsZero() {
		return nil, nil
	}

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   job.RunID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return nil, fmt.Errorf("list run repo jobs: %w", err)
	}

	predecessorByID, err := buildJobPredecessorIndex(jobs)
	if err != nil {
		return nil, err
	}
	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for i := range jobs {
		jobsByID[jobs[i].ID] = jobs[i]
	}
	predecessor, ok := predecessorByID[job.ID]
	if !ok {
		return nil, nil
	}
	if jobType == domaintypes.JobTypeHeal && domaintypes.JobType(predecessor.JobType) == domaintypes.JobTypeSBOM {
		return nil, nil
	}
	sourceSBOM, ok := resolveRunClasspathSourceSBOM(predecessorByID, jobsByID, predecessor.ID)
	if !ok {
		return nil, nil
	}

	source, err := resolveEffectiveSourceJob(ctx, st, sourceSBOM.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve java classpath source job: %w", err)
	}
	artifactID, err := resolveJavaClasspathSourceArtifactID(ctx, st, source)
	if err != nil {
		return nil, err
	}

	return &contracts.JavaClasspathClaimContext{
		Required:         true,
		SourceArtifactID: artifactID,
		SourceJobID:      source.ID,
		SourceJobType:    domaintypes.JobType(source.JobType),
	}, nil
}

func buildJobPredecessorIndex(jobs []store.Job) (map[domaintypes.JobID]store.Job, error) {
	predecessorByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for i := range jobs {
		nextID := jobs[i].NextID
		if nextID == nil || nextID.IsZero() {
			continue
		}
		if _, exists := predecessorByID[*nextID]; exists {
			return nil, fmt.Errorf("multiple predecessor jobs reference %s", *nextID)
		}
		predecessorByID[*nextID] = jobs[i]
	}
	return predecessorByID, nil
}

func resolveRunClasspathSourceSBOM(
	predecessorByID map[domaintypes.JobID]store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
	start domaintypes.JobID,
) (store.Job, bool) {
	if start.IsZero() {
		return store.Job{}, false
	}
	visited := make(map[domaintypes.JobID]struct{}, len(jobsByID))
	current := start
	var selected store.Job
	for {
		if _, seen := visited[current]; seen {
			return store.Job{}, false
		}
		visited[current] = struct{}{}

		job, ok := jobsByID[current]
		if !ok {
			return store.Job{}, false
		}
		if isSuccessfulPreGateSBOMJob(job) {
			// Preserve the earliest successful pre-gate source in chain traversal.
			selected = job
		}
		next, ok := predecessorByID[current]
		if !ok {
			if selected.ID.IsZero() {
				return store.Job{}, false
			}
			return selected, true
		}
		current = next.ID
	}
}

func isSuccessfulPreGateSBOMJob(job store.Job) bool {
	if domaintypes.JobType(job.JobType) != domaintypes.JobTypeSBOM ||
		domaintypes.JobStatus(job.Status) != domaintypes.JobStatusSuccess {
		return false
	}
	ctx, ok := sbomCycleContextFromJob(job)
	if !ok {
		return false
	}
	return strings.TrimSpace(ctx.Phase) == contracts.SBOMPhasePre
}

func resolveJavaClasspathSourceArtifactID(
	ctx context.Context,
	st store.Store,
	sourceJob store.Job,
) (string, error) {
	bundles, err := st.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: sourceJob.RunID,
		JobID: &sourceJob.ID,
	})
	if err != nil {
		return "", fmt.Errorf("list java classpath source artifacts: %w", err)
	}
	preferredNames := preferredClasspathBundleNames(domaintypes.JobType(sourceJob.JobType))
	for _, preferredName := range preferredNames {
		for i := range bundles {
			if !classpathBundleNameMatches(bundles[i].Name, preferredName) {
				continue
			}
			artifactID := strings.TrimSpace(bundleToSummary(bundles[i]).ID)
			if artifactID != "" {
				return artifactID, nil
			}
		}
	}
	return "", nil
}

func preferredClasspathBundleNames(jobType domaintypes.JobType) []string {
	if jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate || jobType == domaintypes.JobTypeReGate {
		return []string{"build-gate-out", "mig-out", ""}
	}
	return []string{"mig-out", "build-gate-out", ""}
}

func classpathBundleNameMatches(name *string, expected string) bool {
	actual := ""
	if name != nil {
		actual = strings.TrimSpace(*name)
	}
	return actual == expected
}
