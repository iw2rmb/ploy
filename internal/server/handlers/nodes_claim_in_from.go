package handlers

import (
	"context"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const javaClasspathFileName = "java.classpath"

func resolveMigInFromClaimEntries(
	ctx context.Context,
	st store.Store,
	job store.Job,
	spec *contracts.MigSpec,
	stepIndex int,
) ([]contracts.ResolvedInFromRef, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec is required")
	}
	if stepIndex < 0 || stepIndex >= len(spec.Steps) {
		return nil, fmt.Errorf("step index %d out of range", stepIndex)
	}

	step := spec.Steps[stepIndex]
	if len(step.InFrom) == 0 {
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
	jobIndex := buildInFromSourceJobIndex(jobs)

	resolved := make([]contracts.ResolvedInFromRef, 0, len(step.InFrom))
	for i := range step.InFrom {
		ref := step.InFrom[i]
		parsed, err := contracts.ParseInFromURI(ref.From)
		if err != nil {
			return nil, fmt.Errorf("steps[%d].in_from[%d].from: %w", stepIndex, i, err)
		}
		if parsed.SourceType == domaintypes.JobTypeSBOM && parsed.OutPath == "/out/"+javaClasspathFileName {
			return nil, fmt.Errorf("steps[%d].in_from[%d].from: sbom://out/%s is unsupported; java classpath is available at /share/%s", stepIndex, i, javaClasspathFileName, javaClasspathFileName)
		}
		target, err := contracts.NormalizeInFromTarget(ref.To, parsed.OutPath)
		if err != nil {
			return nil, fmt.Errorf("steps[%d].in_from[%d].to: %w", stepIndex, i, err)
		}

		sourceJob, err := resolveInFromSourceJob(parsed, jobIndex)
		if err != nil {
			return nil, fmt.Errorf("steps[%d].in_from[%d].from: %w", stepIndex, i, err)
		}

		artifactID, err := resolveSourceArtifactID(ctx, st, sourceJob)
		if err != nil {
			return nil, fmt.Errorf("steps[%d].in_from[%d].from: resolve source artifact: %w", stepIndex, i, err)
		}
		if artifactID == "" {
			return nil, fmt.Errorf("steps[%d].in_from[%d].from: source %q artifact is missing", stepIndex, i, formatInFromSelector(parsed))
		}

		resolved = append(resolved, contracts.ResolvedInFromRef{
			From:             strings.TrimSpace(ref.From),
			To:               target,
			SourceStepName:   parsed.SourceName,
			SourceOutPath:    parsed.OutPath,
			SourceArtifactID: artifactID,
		})
	}

	return resolved, nil
}

type inFromSourceJobIndex struct {
	latestSuccessByType    map[domaintypes.JobType]store.Job
	latestSuccessMigByName map[string]store.Job
}

func buildInFromSourceJobIndex(jobs []store.Job) inFromSourceJobIndex {
	idx := inFromSourceJobIndex{
		latestSuccessByType:    make(map[domaintypes.JobType]store.Job, 4),
		latestSuccessMigByName: make(map[string]store.Job, 4),
	}
	for i := range jobs {
		job := jobs[i]
		jobType := domaintypes.JobType(job.JobType)
		if domaintypes.JobStatus(job.Status) != domaintypes.JobStatusSuccess {
			continue
		}
		idx.latestSuccessByType[jobType] = job
		if jobType != domaintypes.JobTypeMig {
			continue
		}

		meta, err := contracts.UnmarshalJobMeta(job.Meta)
		if err != nil || meta == nil {
			continue
		}
		name := strings.TrimSpace(meta.MigStepName)
		if name == "" {
			continue
		}
		idx.latestSuccessMigByName[name] = job
	}
	return idx
}

func resolveInFromSourceJob(parsed contracts.InFromURI, idx inFromSourceJobIndex) (store.Job, error) {
	if parsed.SourceName != "" && parsed.SourceType != domaintypes.JobTypeMig {
		return store.Job{}, fmt.Errorf("named selector is supported only for type %q", domaintypes.JobTypeMig)
	}
	if parsed.SourceType == domaintypes.JobTypeMig && parsed.SourceName != "" {
		sourceJob, ok := idx.latestSuccessMigByName[parsed.SourceName]
		if !ok {
			return store.Job{}, fmt.Errorf("source mig step %q successful job is not available", parsed.SourceName)
		}
		return sourceJob, nil
	}

	sourceJob, ok := idx.latestSuccessByType[parsed.SourceType]
	if !ok {
		return store.Job{}, fmt.Errorf("source job type %q successful job is not available", parsed.SourceType)
	}
	return sourceJob, nil
}

func formatInFromSelector(parsed contracts.InFromURI) string {
	if parsed.SourceName == "" {
		return parsed.SourceType.String()
	}
	return fmt.Sprintf("%s@%s", parsed.SourceName, parsed.SourceType)
}

func resolveSourceArtifactID(ctx context.Context, st store.Store, sourceJob store.Job) (string, error) {
	bundles, err := listArtifactBundlesByEffectiveJob(ctx, st, sourceJob)
	if err != nil {
		return "", fmt.Errorf("list source artifacts: %w", err)
	}
	return selectPreferredArtifactID(bundles, preferredClasspathBundleNames(domaintypes.JobType(sourceJob.JobType))), nil
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

func selectPreferredArtifactID(bundles []store.ArtifactBundle, preferredNames []string) string {
	for _, preferredName := range preferredNames {
		for i := range bundles {
			if !classpathBundleNameMatches(bundles[i].Name, preferredName) {
				continue
			}
			artifactID := strings.TrimSpace(bundleToSummary(bundles[i]).ID)
			if artifactID != "" {
				return artifactID
			}
		}
	}
	return ""
}
