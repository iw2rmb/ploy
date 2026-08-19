package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Runner executes workflow steps.
//
// # Execution Stages (Pre-mig Gate per Call)
//
// Runner.Run processes each step call through the following stages in order:
//
//  1. Hydration — Prepare the workspace by fetching repository sources via
//     WorkspaceHydrator. Errors here abort the run immediately.
//
//  2. Pre-mig Build Gate — When Gate is enabled (Manifest.Gate.Enabled), run static validation on the
//     workspace before executing the mig container. If the gate fails,
//     Runner.Run returns ErrGateFailed without executing container
//     stages.
//
//  3. Container Execution — Create, start, and wait on the container via
//     ContainerRuntime. Logs are forwarded to LogWriter if present.
//     Container cleanup is owned by node-runtime pre-claim disk-pressure flow.
//
// # Gate Ownership Contract
//
// Runner supports an optional pre-mig gate when Manifest.Gate.Enabled=true.
// This capability exists for direct invocations (e.g., standalone testing)
// where Runner manages its own gate lifecycle.
//
// However, nodeagent step execution MUST pass manifests with Gate.Enabled=false.
// The nodeagent orchestration layer owns all gate lifecycle management via
// the gate job chain, which handles:
//   - A single pre-run gate before the step loop begins.
//   - Per-step post-mig gates after each container execution.
//
// Passing Gate.Enabled=true from nodeagent would cause duplicate pre-mig gates
// (one from the nodeagent, one from Runner.Run) and break the single-gate-
// per-run invariant. The nodeagent is the authoritative gate orchestrator.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
	Gate       GateExecutor
	LogWriter  io.Writer // Optional: streams logs to server as gzipped chunks.
}

// Request describes a step execution request.
type Request struct {
	// RunID threads the workflow run identifier for correlation/labels.
	// Container labels and telemetry use this value via LabelRunID.
	RunID types.RunID
	// JobID threads the workflow job identifier for correlation/labels.
	// Container labels and telemetry use this value via LabelJobID.
	JobID     types.JobID
	Manifest  contracts.StepManifest
	Workspace string
	JobMounts JobMounts
}

// Result contains the outcome of a step execution.
type Result struct {
	ExitCode int
	// Docker container identity and inspect output when a container was run.
	ContainerID          string
	ContainerInspectJSON []byte
	// Per-stage timings captured during execution.
	Timings            StageTiming
	Gate               *contracts.BuildGateStageMetadata
	ContainerResources *types.RunStatsJobResources
}

// StageTiming captures duration of each execution stage.
type StageTiming struct {
	HydrationDuration types.Duration
	ExecutionDuration types.Duration
	GateDuration      types.Duration
	DiffDuration      types.Duration
	PublishDuration   types.Duration
	TotalDuration     types.Duration
}

// ErrGateFailed is returned when the pre-mig Build Gate fails.
var ErrGateFailed = errors.New("build gate failed")

// Run executes a step and returns the result.
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	result, totalStart, err := r.runHydrationAndGate(ctx, req, "pre-mig validation failed")
	if err != nil {
		return result, err
	}

	// Seed the job output before execution so its single mount contains both
	// Hydra content and container writes.
	if err := SeedOutDirFromStaging(req.Manifest, req.JobMounts.Staging, req.JobMounts.Out); err != nil {
		return Result{}, fmt.Errorf("seed out dir from staging: %w", err)
	}
	// Seed the job input before execution to avoid nested /in bind mounts.
	if err := SeedInDirFromStaging(req.Manifest, req.JobMounts.Staging, req.JobMounts.In); err != nil {
		return Result{}, fmt.Errorf("seed in dir from staging: %w", err)
	}
	if err := SeedTmpDirFromStaging(req.Manifest, req.JobMounts.Staging, req.JobMounts.Tmp); err != nil {
		return Result{}, fmt.Errorf("seed tmp dir from staging: %w", err)
	}
	if err := SeedHomeDirFromStaging(req.Manifest, req.JobMounts.Staging, req.JobMounts.Home); err != nil {
		return Result{}, fmt.Errorf("seed home dir from staging: %w", err)
	}

	// Stage 3: Execute container via configured runtime.
	executionStart := time.Now()
	if r.Containers == nil {
		if r.LogWriter != nil {
			_, _ = fmt.Fprintf(r.LogWriter, "Starting execution for manifest %s\n", req.Manifest.ID)
		}
		result.ExitCode = 0
		result.Timings.ExecutionDuration = types.Duration(time.Since(executionStart))
	} else {
		spec, err := buildContainerSpec(req.RunID, req.JobID, req.Manifest, req.Workspace, req.JobMounts)
		if err != nil {
			return Result{}, fmt.Errorf("build container spec: %w", err)
		}
		handle, err := r.Containers.Create(ctx, spec)
		if err != nil {
			return Result{}, fmt.Errorf("container create failed: %w", err)
		}
		if err := r.Containers.Start(ctx, handle); err != nil {
			return Result{}, fmt.Errorf("container start failed: %w", err)
		}

		var streamDone <-chan error
		if r.LogWriter != nil {
			streamDone = streamContainerLogs(ctx, r.Containers, handle, nil, r.LogWriter)
		}

		cRes, err := r.Containers.Wait(ctx, handle)
		if err != nil {
			return Result{}, fmt.Errorf("container wait failed: %w", err)
		}
		if usage := collectDockerResourceUsage(ctx, r.Containers, handle, spec); usage != nil {
			result.ContainerResources = NormalizeContainerResourceUsage(usage)
		}
		if r.LogWriter != nil {
			if !awaitStreamWithin(streamDone, 2*time.Second) {
				if logs, err := r.Containers.Logs(ctx, handle); err == nil && len(logs) > 0 {
					_, _ = r.LogWriter.Write(logs)
				}
			}
		}
		result.ExitCode = cRes.ExitCode
		result.ContainerID = cRes.ContainerID
		result.ContainerInspectJSON = cRes.InspectJSON
		result.Timings.ExecutionDuration = types.Duration(time.Since(executionStart))

	}

	result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
	return result, nil
}

func (r *Runner) runHydrationAndGate(ctx context.Context, req Request, gateFailureMessage string) (Result, time.Time, error) {
	totalStart := time.Now()
	var result Result

	hydrationDuration, err := r.hydrate(ctx, req)
	if err != nil {
		return Result{}, totalStart, err
	}
	result.Timings.HydrationDuration = hydrationDuration

	gateMetadata, gateDuration, err := r.runGate(ctx, req, gateFailureMessage)
	result.Gate = gateMetadata
	result.Timings.GateDuration = gateDuration
	if err != nil {
		result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
		return result, totalStart, err
	}

	return result, totalStart, nil
}

// NormalizeContainerResourceUsage converts Docker counters to persisted job metrics.
func NormalizeContainerResourceUsage(usage *contracts.BuildGateResourceUsage) *types.RunStatsJobResources {
	if usage == nil {
		return nil
	}
	memConsumed := usage.MemMaxBytes
	if memConsumed == 0 {
		memConsumed = usage.MemUsageBytes
	}
	var diskConsumed int64
	if usage.SizeRwBytes != nil && *usage.SizeRwBytes > 0 {
		diskConsumed = *usage.SizeRwBytes
	}
	return &types.RunStatsJobResources{
		CPUConsumedNs:     saturatingInt64FromUint64(usage.CPUTotalNs),
		DiskConsumedBytes: diskConsumed,
		MemConsumedBytes:  saturatingInt64FromUint64(memConsumed),
	}
}

func saturatingInt64FromUint64(v uint64) int64 {
	const maxInt64 = ^uint64(0) >> 1
	if v > maxInt64 {
		return int64(maxInt64)
	}
	return int64(v)
}
