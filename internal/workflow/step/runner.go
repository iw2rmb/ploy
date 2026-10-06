package step

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Runner executes one workflow container. Nodeagent owns workspace preparation,
// gate orchestration, and whole-job timing around this execution boundary.
type Runner struct {
	Containers ContainerRuntime
	LogWriter  io.Writer // Optional: streams logs to server as gzipped chunks.
}

// Request describes a step execution request.
type Request struct {
	ResumeCount int
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
	ContainerResources *types.RunStatsJobResources
}

// StageTiming captures duration of each execution stage.
type StageTiming struct {
	HydrationDuration types.Duration
	ExecutionDuration types.Duration
	TotalDuration     types.Duration
}

// Run executes a step and returns the result.
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	var result Result

	if err := seedDirsFromStaging(req.Manifest, req.JobMounts); err != nil {
		return Result{}, fmt.Errorf("seed Hydra dirs from staging: %w", err)
	}

	// Execute the container via the configured runtime.
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
		if req.ResumeCount > 0 {
			if spec.Labels == nil {
				spec.Labels = make(map[string]string)
			}
			spec.Labels[types.LabelResumeCount] = strconv.Itoa(req.ResumeCount)
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

	return result, nil
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
