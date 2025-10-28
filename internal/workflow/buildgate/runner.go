package buildgate

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrRunnerSandboxMissing indicates the build gate runner is missing a sandbox runner dependency.
var ErrRunnerSandboxMissing = errors.New("buildgate: sandbox runner missing")

// ErrRunnerStaticCheckRegistryMissing indicates the static check registry dependency is missing.
var ErrRunnerStaticCheckRegistryMissing = errors.New("buildgate: static check registry missing")

// ErrRunnerLogIngestorMissing indicates the log ingestor dependency is missing when log ingestion is requested.
var ErrRunnerLogIngestorMissing = errors.New("buildgate: log ingestor missing")

// Runner orchestrates sandbox builds, static checks, and log ingestion for the build gate stage.
type Runner struct {
	Sandbox      *SandboxRunner
	StaticChecks *StaticCheckRegistry
	Logs         *LogIngestor
}

// RunSpec describes the requested build gate execution.
type RunSpec struct {
	Sandbox      SandboxSpec
	StaticChecks *StaticCheckSpec
	LogArtifact  *ArtifactReference
}

// RunResult captures the outcomes of the build gate runner execution.
type RunResult struct {
	Sandbox      SandboxOutcome
	StaticChecks []StaticCheckReport
	Log          *LogIngestionResult
	Metadata     Metadata
	Report       []byte
}

// Run executes the sandbox build, static checks, and log ingestion according to the provided spec.
func (r *Runner) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	if r == nil || r.Sandbox == nil {
		return RunResult{}, ErrRunnerSandboxMissing
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sandboxOutcome, err := r.Sandbox.Run(ctx, spec.Sandbox)
	if err != nil {
		return RunResult{}, fmt.Errorf("buildgate: sandbox run: %w", err)
	}

	result := RunResult{Sandbox: sandboxOutcome}

	if spec.StaticChecks != nil {
		if r.StaticChecks == nil {
			return RunResult{}, ErrRunnerStaticCheckRegistryMissing
		}
		reports, err := r.StaticChecks.Execute(ctx, *spec.StaticChecks)
		if err != nil {
			return RunResult{}, fmt.Errorf("buildgate: static checks: %w", err)
		}
		result.StaticChecks = reports
	}

	if spec.LogArtifact != nil {
		if r.Logs == nil {
			return RunResult{}, ErrRunnerLogIngestorMissing
		}
		ingestion, err := r.Logs.Ingest(ctx, LogIngestionSpec{Artifact: *spec.LogArtifact})
		if err != nil {
			return RunResult{}, fmt.Errorf("buildgate: log ingestion: %w", err)
		}
		result.Log = &ingestion
	}

	metadata := sandboxOutcome.Metadata
	if strings.TrimSpace(metadata.LogDigest) == "" {
		metadata.LogDigest = sandboxOutcome.LogDigest
	}
	if len(result.StaticChecks) > 0 {
		metadata.StaticChecks = append(metadata.StaticChecks, result.StaticChecks...)
	}
	if result.Log != nil {
		if result.Log.Digest != "" {
			metadata.LogDigest = result.Log.Digest
		}
		if len(result.Log.Findings) > 0 {
			metadata.LogFindings = append(metadata.LogFindings, result.Log.Findings...)
		}
	}

	sanitized := Sanitize(metadata)
	result.Metadata = sanitized
	result.StaticChecks = sanitized.StaticChecks
	if len(sandboxOutcome.Report) > 0 {
		result.Report = append(result.Report, sandboxOutcome.Report...)
	}

	return result, nil
}
