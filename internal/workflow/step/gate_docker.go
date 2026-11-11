// gate_docker.go implements the Docker-based GateExecutor.
//
// This executor is the canonical source of gate validation results: it runs a
// language-specific image with the workspace mounted at /workspace, captures
// logs and resource usage, and returns BuildGateMetadata. Concerns are
// split across sibling files: common job mounts (job_mounts.go), log streaming
// (container_log_streamer.go), and result normalization
// (gate_docker_metadata.go). Stack detection + image resolution live in
// gate_plan_resolver.go.
package step

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	units "github.com/docker/go-units"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	gateLimitMemoryEnv = "PLOY_BUILDGATE_LIMIT_MEMORY_BYTES"
	gateLimitCPUEnv    = "PLOY_BUILDGATE_LIMIT_CPU_MILLIS"
	gateLimitDiskEnv   = "PLOY_BUILDGATE_LIMIT_DISK_SPACE"
)

// GateExecutor runs build validation inside language images using the
// same container runtime as step execution.
type GateExecutor struct {
	rt ContainerRuntime
}

// NewGateExecutor constructs a GateExecutor that uses the provided
// ContainerRuntime to run build commands.
func NewGateExecutor(rt ContainerRuntime) *GateExecutor {
	return &GateExecutor{rt: rt}
}

// Execute resolves the gate plan (stack, image, command), runs the command in
// a container, and returns BuildGateMetadata. The workspace is mounted at
// /workspace; nil spec or Enabled=false yields (nil, nil).
func (e *GateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string, jobMounts JobMounts) (*contracts.BuildGateMetadata, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if spec == nil || !spec.Enabled {
		return nil, nil
	}

	catalogPath := defaultCatalogFilePath()

	plan, terminal := resolveGateExecutionPlan(ctx, workspace, spec, catalogPath)
	if terminal != nil {
		reportGateRuntimeImage(ctx, terminal.meta.RuntimeImage)
		return terminal.meta, terminal.err
	}

	reportGateRuntimeImage(ctx, plan.image)

	homeDir, err := resolveJobHome(spec.Envs)
	if err != nil {
		return nil, fmt.Errorf("resolve build gate home: %w", err)
	}
	mounts, err := assembleGateMounts(workspace, jobMounts, homeDir)
	if err != nil {
		return nil, err
	}

	limitMem, _ := parseBytesLimitEnv(gateLimitMemoryEnv)
	limitCPUMillis := parseInt64LimitEnv(gateLimitCPUEnv)
	limitDisk, storageSizeOpt := parseBytesLimitEnv(gateLimitDiskEnv)
	env := applyReservedJobEnv(spec.Envs, jobMounts, homeDir)
	mounts = appendHostSocketMount(mounts, dockerHostSocketPathFromEnv(env))
	if err := validateContainerMounts(mounts, nil); err != nil {
		return nil, fmt.Errorf("validate build gate mounts: %w", err)
	}

	specC := ContainerSpec{
		Image:            plan.image,
		WorkingDir:       "/workspace",
		Mounts:           mounts,
		Env:              env,
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

	meta := gateExecutionMetadata(plan.language, plan.tool, plan.release, plan.image, res, logs)
	meta.Resources = collectDockerResourceUsage(ctx, e.rt, h, specC)

	if plan.stackGate != nil {
		meta.StackGate = plan.stackGate
	}
	return meta, nil
}

// assembleGateMounts adds gate-specific mounts to the common job projection.
func assembleGateMounts(workspace string, jobMounts JobMounts, homeDir string) ([]ContainerMount, error) {
	commonMounts, err := buildCommonJobMounts(jobMounts, homeDir)
	if err != nil {
		return nil, fmt.Errorf("prepare common job mounts: %w", err)
	}
	mounts := append([]ContainerMount{{Source: workspace, Target: "/workspace", ReadOnly: false}}, commonMounts...)

	return mounts, nil
}

func parseInt64LimitEnv(key string) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseBytesLimitEnv reads a byte-quantity env var. Accepts RAM (e.g. "2g"),
// human size ("2GB"), or a raw int64. Returns the parsed bytes and the original
// string for passthrough to Docker --storage-opt size=.
func parseBytesLimitEnv(key string) (int64, string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, ""
	}

	if n, err := units.RAMInBytes(value); err == nil {
		return n, value
	}
	if n, err := units.FromHumanSize(value); err == nil {
		return n, value
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return n, value
	}
	return 0, value
}
