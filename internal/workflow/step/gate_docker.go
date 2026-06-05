// gate_docker.go implements the Docker-based GateExecutor.
//
// This executor is the canonical source of gate validation results: it runs a
// language-specific image with the workspace mounted at /workspace, captures
// logs and resource usage, and returns BuildGateStageMetadata. Concerns are
// split across sibling files: mounts (gate_docker_mounts.go), log streaming
// (container_log_streamer.go + gate_docker_logs.go), env-driven resource
// limits (gate_docker_env.go), and result normalization
// (gate_docker_metadata.go). Stack detection + image resolution live in
// gate_plan_resolver.go.
package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	// gradleCacheHitsHostFile is the workspace-local file mounted into
	// Gradle gate containers for cache-hit signaling from the init script.
	gradleCacheHitsHostFile = ".ploy-gradle-build-cache-hits"
	// gradleCacheHitsContainerFile is the in-container path consumed by
	// the Gradle init script to write cache-hit markers.
	gradleCacheHitsContainerFile = "/tmp/gradle-build-cache-hits"

	// GateWorkspaceOutDir is a workspace-local host directory mounted
	// into gate containers as /out for deterministic artifact collection.
	GateWorkspaceOutDir = ".ploy-gate-out"
	// gateWorkspaceInDir is an optional workspace-local host directory
	// mounted into gate containers as /in for orchestrator-provided inputs.
	gateWorkspaceInDir = ".ploy-gate-in"
	// gateContainerInDir is the writable input mount path inside gate
	// containers used by orchestrator-provided runtime inputs.
	gateContainerInDir = "/in"
	// gateContainerOutDir is the writable output mount path inside gate
	// containers used by runtime-generated artifacts (for example Gradle reports).
	gateContainerOutDir = "/out"
	// gradleUserHomeDir is the native Gradle home path in gate-gradle images.
	gradleUserHomeDir = "/root/.gradle"
	// mavenUserHomeDir is the native Maven repository path in Maven gate images.
	mavenUserHomeDir = "/root/.m2"
)

var errGateRuntimeUnavailable = errors.New("build gate runtime unavailable")

// gateExecutor runs build validation inside language images using the
// same container runtime as step execution.
type gateExecutor struct {
	rt ContainerRuntime
}

// NewGateExecutor constructs a GateExecutor that uses the provided
// ContainerRuntime to run build commands.
func NewGateExecutor(rt ContainerRuntime) GateExecutor {
	return &gateExecutor{rt: rt}
}

// Execute resolves the gate plan (stack, image, command), runs the command in
// a container, and returns BuildGateStageMetadata. The workspace is mounted at
// /workspace; nil spec or Enabled=false yields (nil, nil). A nil runtime fails
// immediately with errGateRuntimeUnavailable.
func (e *gateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if spec == nil || !spec.Enabled {
		return nil, nil
	}
	if e.rt == nil {
		return nil, errGateRuntimeUnavailable
	}

	catalogPath := defaultCatalogFilePath()

	plan, terminal := resolveGateExecutionPlan(ctx, workspace, spec, catalogPath)
	if terminal != nil {
		if terminal.reportRuntimeImage {
			reportGateRuntimeImage(ctx, terminal.runtimeImage)
		}
		return terminal.meta, terminal.err
	}

	reportGateRuntimeImage(ctx, plan.image)

	mounts, err := assembleGateMounts(ctx, workspace, plan)
	if err != nil {
		return nil, err
	}

	limitMem, _ := parseBytesLimitEnv(gateLimitMemoryEnv)
	limitCPUMillis := parseInt64LimitEnv(gateLimitCPUEnv)
	limitDisk, storageSizeOpt := parseBytesLimitEnv(gateLimitDiskEnv)
	envCopy := contracts.MergeEnv(spec.Env, plan.env)
	mounts = appendDockerHostSocketMount(mounts, envCopy)

	specC := ContainerSpec{
		Image:            plan.image,
		Command:          plan.cmd,
		WorkingDir:       "/workspace",
		Mounts:           mounts,
		Env:              envCopy,
		Labels:           gateContainerLabels(ctx),
		LimitMemoryBytes: limitMem,
		LimitNanoCPUs:    limitCPUMillis * 1_000_000, // millis -> nanos
		LimitDiskBytes:   limitDisk,
		StorageSizeOpt:   storageSizeOpt,
	}
	h, err := e.rt.Create(ctx, specC)
	if err != nil {
		return nil, err
	}
	if err := e.rt.Start(ctx, h); err != nil {
		return nil, err
	}

	var streamedLogs bytes.Buffer
	liveWriter := executionLogWriterFromContext(ctx)
	streamDone := streamContainerLogs(ctx, e.rt, h, &streamedLogs, liveWriter)

	res, err := e.rt.Wait(ctx, h)
	if err != nil {
		return nil, err
	}

	var logs []byte
	if awaitStreamWithin(streamDone, 2*time.Second) {
		logs = append([]byte(nil), streamedLogs.Bytes()...)
	}
	if len(logs) == 0 {
		logs, _ = e.rt.Logs(ctx, h)
		if liveWriter != nil && len(logs) > 0 {
			_, _ = liveWriter.Write(logs)
		}
	}

	meta := gateExecutionMetadata(workspace, plan.language, plan.tool, plan.release, plan.image, res, logs)
	meta.Resources = collectDockerResourceUsage(ctx, e.rt, h, specC)

	if plan.stackGate != nil {
		meta.StackGate = plan.stackGate
	}
	return meta, nil
}

// assembleGateMounts builds the mount set for a gate container: workspace,
// /out, optional /in, optional /share, tool cache, and gradle cache-hits file
// when the tool is gradle.
func assembleGateMounts(ctx context.Context, workspace string, plan gateExecutionPlan) ([]ContainerMount, error) {
	gateOutDir := filepath.Join(workspace, GateWorkspaceOutDir)
	if err := os.MkdirAll(gateOutDir, 0o750); err != nil {
		return nil, fmt.Errorf("prepare build gate out dir: %w", err)
	}
	mounts := []ContainerMount{
		{Source: workspace, Target: "/workspace", ReadOnly: false},
		{Source: gateOutDir, Target: gateContainerOutDir, ReadOnly: false},
	}

	gateInDir := filepath.Join(workspace, gateWorkspaceInDir)
	if info, statErr := os.Stat(gateInDir); statErr == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("build gate in path is not a directory: %s", gateInDir)
		}
		mounts = append(mounts, ContainerMount{Source: gateInDir, Target: gateContainerInDir, ReadOnly: false})
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("stat build gate in dir: %w", statErr)
	}

	if gateShareDir := gateShareDirFromContext(ctx); gateShareDir != "" {
		info, statErr := os.Stat(gateShareDir)
		if statErr != nil {
			return nil, fmt.Errorf("stat build gate share dir: %w", statErr)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("build gate share path is not a directory: %s", gateShareDir)
		}
		mounts = append(mounts, ContainerMount{Source: gateShareDir, Target: containerShareDir, ReadOnly: false})
	}

	toolCacheMounts, err := buildToolCacheMounts(plan.language, plan.tool, plan.release)
	if err != nil {
		return nil, fmt.Errorf("prepare build gate tool cache mounts: %w", err)
	}
	mounts = append(mounts, toolCacheMounts...)

	if strings.EqualFold(plan.tool, "gradle") {
		gradleCacheHitsHostPath := filepath.Join(workspace, gradleCacheHitsHostFile)
		if err := os.WriteFile(gradleCacheHitsHostPath, nil, 0o600); err != nil {
			return nil, fmt.Errorf("prepare gradle cache hits file: %w", err)
		}
		mounts = append(mounts, ContainerMount{
			Source:   gradleCacheHitsHostPath,
			Target:   gradleCacheHitsContainerFile,
			ReadOnly: false,
		})
	}

	return mounts, nil
}

// defaultCatalogFilePath resolves the active gates catalog file:
// the installed copy at /etc/ploy/gates/gates.yaml when present, otherwise
// the repo-relative defaultCatalogPath discovered by walking up to a
// go.mod ancestor.
func defaultCatalogFilePath() string {
	installed := "/etc/ploy/gates/gates.yaml"
	if info, err := os.Stat(installed); err == nil && !info.IsDir() {
		return installed
	}
	wd, err := os.Getwd()
	if err == nil {
		dir := wd
		for {
			if info, serr := os.Stat(filepath.Join(dir, "go.mod")); serr == nil && !info.IsDir() {
				candidate := filepath.Join(dir, defaultCatalogPath)
				if info, serr := os.Stat(candidate); serr == nil && !info.IsDir() {
					return candidate
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return defaultCatalogPath
}
