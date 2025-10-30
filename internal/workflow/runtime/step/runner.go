package step

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ErrManifestInvalid indicates the provided manifest failed validation.
var ErrManifestInvalid = errors.New("step: manifest invalid")

// ErrWorkspaceUnavailable indicates workspace hydration failed.
var ErrWorkspaceUnavailable = errors.New("step: workspace unavailable")

// ErrBuildGateFailed indicates Build Gate validation failed.
var ErrBuildGateFailed = errors.New("step: build gate validation failed")

// Request captures the data required to execute a step manifest.
type Request struct {
	Manifest    contracts.StepManifest
	Workspace   string
	LogStreamID string
}

// Result summarises a completed step run.
type Result struct {
	ContainerID        string
	ExitCode           int
    DiffArtifact       PublishedArtifact
    LogArtifact        PublishedArtifact
    GateArtifact       PublishedArtifact
    GateReport         GateResult
	HydrationSnapshots map[string]PublishedArtifact
	RetentionTTL       string
	Retained           bool
}

// Runner executes step manifests using the injected collaborators.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
    Diffs      DiffGenerator
    Gate       GateClient
	Artifacts  ArtifactPublisher
	Logs       LogCollector
	Streams    LogStreamPublisher
}

// Run executes the step manifest and returns the execution result.
func (r Runner) Run(ctx context.Context, req Request) (Result, error) {
	manifest := req.Manifest
	streamID := strings.TrimSpace(req.LogStreamID)
	hasStream := streamID != "" && r.Streams != nil
	var runErr error
	defer func() {
		if !hasStream {
			return
		}
		status := "completed"
		if runErr != nil {
			status = "failed"
		}
		_ = r.Streams.PublishStatus(ctx, streamID, logstream.Status{Status: status})
	}()

	if err := manifest.Validate(); err != nil {
		runErr = fmt.Errorf("%w: %v", ErrManifestInvalid, err)
		return Result{}, runErr
	}
	if r.Workspace == nil {
		runErr = fmt.Errorf("%w: workspace hydrator missing", ErrWorkspaceUnavailable)
		return Result{}, runErr
	}
	workspace, err := r.Workspace.Prepare(ctx, WorkspaceRequest{Manifest: manifest})
	if err != nil {
		runErr = fmt.Errorf("%w: %v", ErrWorkspaceUnavailable, err)
		return Result{}, runErr
	}

	spec, err := buildContainerSpec(manifest, workspace)
	if err != nil {
		runErr = err
		return Result{}, runErr
	}

	if r.Containers == nil {
		runErr = errors.New("step: container runtime missing")
		return Result{}, runErr
	}
	handle, err := r.Containers.Create(ctx, spec)
	if err != nil {
		runErr = fmt.Errorf("step: create container: %w", err)
		return Result{}, runErr
	}
	if err := r.Containers.Start(ctx, handle); err != nil {
		runErr = fmt.Errorf("step: start container: %w", err)
		return Result{}, runErr
	}
	containerResult, err := r.Containers.Wait(ctx, handle)
	if err != nil {
		runErr = fmt.Errorf("step: wait container: %w", err)
		return Result{}, runErr
	}

	var logBytes []byte
	if r.Logs != nil {
		logBytes, err = r.Logs.Collect(ctx, handle)
	} else {
		logBytes, err = r.Containers.Logs(ctx, handle)
	}
	if err != nil {
		runErr = fmt.Errorf("step: collect logs: %w", err)
		return Result{}, runErr
	}
	if hasStream && len(logBytes) > 0 {
		r.publishLogStream(ctx, streamID, logBytes)
	}

	if r.Diffs == nil {
		runErr = errors.New("step: diff generator missing")
		return Result{}, runErr
	}
	diffResult, err := r.Diffs.Capture(ctx, DiffRequest{Manifest: manifest, Workspace: workspace})
	if err != nil {
		runErr = fmt.Errorf("step: capture diff: %w", err)
		return Result{}, runErr
	}

	var diffArtifact PublishedArtifact
	var logArtifact PublishedArtifact
    var gateArtifact PublishedArtifact
	if r.Artifacts != nil {
		diffArtifact, err = r.Artifacts.Publish(ctx, ArtifactRequest{Kind: ArtifactKindDiff, Path: diffResult.Path})
		if err != nil {
			runErr = fmt.Errorf("step: publish diff: %w", err)
			return Result{}, runErr
		}
		logArtifact, err = r.Artifacts.Publish(ctx, ArtifactRequest{Kind: ArtifactKindLogs, Buffer: logBytes})
		if err != nil {
			runErr = fmt.Errorf("step: publish logs: %w", err)
			return Result{}, runErr
		}
	}

    gateResult := GateResult{Passed: true}
    if r.Gate != nil && selectGateSpec(manifest) != nil {
        gateReq := GateRequest{
            Manifest:  manifest,
            Workspace: workspace,
        }
        if logArtifact.CID != "" {
            artifactCopy := logArtifact
            gateReq.LogArtifact = &artifactCopy
        }
        gateStart := time.Now()
        gateResult, err = r.Gate.Validate(ctx, gateReq)
        elapsed := time.Since(gateStart)
        if err != nil {
            runErr = fmt.Errorf("step: build gate validation: %w", err)
            return Result{}, runErr
        }
        if gateResult.Duration <= 0 {
            gateResult.Duration = elapsed
        }
        if r.Artifacts != nil && len(gateResult.Report) > 0 {
            gateArtifact, err = r.Artifacts.Publish(ctx, ArtifactRequest{
                Kind:   ArtifactKindGateReport,
                Buffer: append([]byte(nil), gateResult.Report...),
            })
            if err != nil {
                runErr = fmt.Errorf("step: publish gate report: %w", err)
                return Result{}, runErr
            }
        }
        if !gateResult.Passed {
            result := Result{
                ContainerID:        handle.ID,
                ExitCode:           containerResult.ExitCode,
                DiffArtifact:       diffArtifact,
                LogArtifact:        logArtifact,
                GateArtifact:       gateArtifact,
                GateReport:         gateResult,
                HydrationSnapshots: clonePublishedArtifacts(workspace.HydrationSnapshots),
                Retained:           manifest.Retention.RetainContainer,
                RetentionTTL:       manifest.Retention.TTL,
            }
            if hasStream {
                r.publishRetentionHint(ctx, streamID, result)
            }
            runErr = fmt.Errorf("%w: %s", ErrBuildGateFailed, gateResult.Message)
            return result, runErr
        }
    }

	result := Result{
		ContainerID:        handle.ID,
		ExitCode:           containerResult.ExitCode,
		DiffArtifact:       diffArtifact,
		LogArtifact:        logArtifact,
        GateArtifact:       gateArtifact,
        GateReport:         gateResult,
		HydrationSnapshots: clonePublishedArtifacts(workspace.HydrationSnapshots),
		Retained:           manifest.Retention.RetainContainer,
		RetentionTTL:       manifest.Retention.TTL,
	}
	if hasStream {
		r.publishRetentionHint(ctx, streamID, result)
	}
	return result, nil
}

func clonePublishedArtifacts(src map[string]PublishedArtifact) map[string]PublishedArtifact {
	if len(src) == 0 {
		return nil
	}
	dup := make(map[string]PublishedArtifact, len(src))
	for k, v := range src {
		dup[k] = v
	}
	return dup
}
