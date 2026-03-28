// gate_docker.go implements the Docker-based GateExecutor for build validation.
//
// ## HTTP Build Gate and Docker Gate Consistency
//
// This GateExecutor provides the CANONICAL gate validation used by the node agent.
// While healing migs may optionally call the HTTP Build Gate API directly for
// intermediate checks, the authoritative gate results are always produced by this
// Docker-based executor. This ensures:
//
//   - Consistent validation semantics: The Docker gate validates the workspace
//     directory directly, which is semantically equivalent to the HTTP Build Gate
//     API with a diff_patch containing all workspace modifications.
//
//   - Full gate history: The node agent captures every gate execution (pre-gate
//     and all re-gates after healing) in BuildGateStageMetadata, enabling
//     complete telemetry and audit trails.
//
//   - Authoritative results: In-container HTTP Build Gate API calls are advisory
//     only. The node agent always re-runs this Docker gate after healing migs
//     complete, regardless of any intermediate validation results.
//
// ## Usage Note for Healing Migs
//
// Direct HTTP Build Gate API calls from healing migs are now DISCOURAGED for
// migs-codex. The node agent handles all gate orchestration, ensuring consistent
// behavior and complete history capture.
package step

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	units "github.com/docker/go-units"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	buildGateLimitMemoryEnv = "PLOY_BUILDGATE_LIMIT_MEMORY_BYTES"
	buildGateLimitCPUEnv    = "PLOY_BUILDGATE_LIMIT_CPU_MILLIS"
	buildGateLimitDiskEnv   = "PLOY_BUILDGATE_LIMIT_DISK_SPACE"
	// BuildGateGradleCacheHitsHostFile is the workspace-local file mounted into
	// Gradle gate containers for cache-hit signaling from the init script.
	BuildGateGradleCacheHitsHostFile = ".ploy-gradle-build-cache-hits"
	// BuildGateGradleCacheHitsContainerFile is the in-container path consumed by
	// the Gradle init script to write cache-hit markers.
	BuildGateGradleCacheHitsContainerFile = "/tmp/gradle-build-cache-hits"

	// BuildGateWorkspaceOutDir is a workspace-local host directory mounted
	// into gate containers as /out for deterministic artifact collection.
	BuildGateWorkspaceOutDir = ".ploy-gate-out"
	// BuildGateContainerOutDir is the writable output mount path inside gate
	// containers used by runtime-generated artifacts (for example Gradle reports).
	BuildGateContainerOutDir = "/out"
)

var errBuildGateRuntimeUnavailable = errors.New("build gate runtime unavailable")

// dockerGateExecutor runs build validation inside language images using the
// same container runtime as step execution, mounting the workspace at /workspace.
//
// This executor is the CANONICAL source of gate validation results for the node
// agent. The node agent always uses this executor for both pre-gate (initial
// validation) and re-gate (post-healing validation) phases, ensuring consistent
// behavior and complete history capture.
type dockerGateExecutor struct {
	rt ContainerRuntime
}

func readGradleBuildCacheHits(workspace string) []string {
	path := filepath.Join(workspace, BuildGateGradleCacheHitsHostFile)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	defer func() { _ = os.Remove(path) }()

	seen := make(map[string]struct{})
	var hits []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		hits = append(hits, s)
	}
	if len(hits) == 0 {
		return nil
	}
	sort.Strings(hits)
	return hits
}

// NewDockerGateExecutor constructs a GateExecutor that uses the provided
// ContainerRuntime to run build commands.
func NewDockerGateExecutor(rt ContainerRuntime) GateExecutor {
	return &dockerGateExecutor{rt: rt}
}

// Execute runs the Build Gate inside a container image and returns
// normalized metadata about the outcome.
//
// Image selection:
//   - Default stacks catalog (`gates/stacks.yaml`) + mig YAML overrides (`build_gate.images[]`)
//
// The workspace is mounted at /workspace and used as the working directory.
// When stack detection fails, the gate fails with a static check report and
// a structured log finding (including evidence when available).
// When the container runtime is nil, execution fails immediately. A non-zero
// exit code is reported as a static check failure and a single log finding
// containing the captured logs or a synthesized message.
//
// ## Returned Metadata
//
// The BuildGateStageMetadata returned by this method is the CANONICAL gate result
// used by the node agent for decision-making and history capture. It includes:
//   - StaticChecks: Pass/fail status with language and tool information
//   - LogFindings: Structured error messages extracted from build output
//   - LogsText: Full build log text (truncated to 10 MiB) for debugging
//   - LogDigest: SHA-256 hash of logs for deduplication and verification
//   - Resources: Container resource usage metrics (CPU, memory, disk I/O)
//
// This metadata is captured by the node agent in both pre-gate (initial validation)
// and re-gate (post-healing validation) phases, ensuring complete gate history.
func (e *dockerGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if spec == nil || !spec.Enabled {
		return nil, nil
	}
	if spec.Skip != nil && spec.Skip.Enabled {
		meta := &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{
				{Tool: "skip", Passed: true},
			},
			Skip: &contracts.BuildGateSkipMetadata{
				Enabled:         true,
				SourceProfileID: spec.Skip.SourceProfileID,
				MatchedTarget:   spec.Skip.MatchedTarget,
			},
		}
		if spec.GateProfile != nil && spec.GateProfile.Stack != nil {
			stack := spec.GateProfile.Stack
			meta.Detected = &contracts.StackExpectation{
				Language: strings.TrimSpace(stack.Language),
				Tool:     strings.TrimSpace(stack.Tool),
				Release:  strings.TrimSpace(stack.Release),
			}
			if meta.Detected.Tool != "" {
				meta.StaticChecks[0].Tool = meta.Detected.Tool
			}
			if meta.Detected.Language != "" {
				meta.StaticChecks[0].Language = meta.Detected.Language
			}
		}
		return meta, nil
	}
	if e.rt == nil {
		return nil, errBuildGateRuntimeUnavailable
	}

	catalogPath := buildGateDefaultStacksCatalogPath()

	plan, terminal := resolveGateExecutionPlan(ctx, workspace, spec, catalogPath)
	if terminal != nil {
		if terminal.reportRuntimeImage {
			reportGateRuntimeImage(ctx, terminal.runtimeImage)
		}
		return terminal.meta, terminal.err
	}

	reportGateRuntimeImage(ctx, plan.image)

	// Build container spec with workspace + /out mounts.
	gateOutDir := filepath.Join(workspace, BuildGateWorkspaceOutDir)
	if err := os.MkdirAll(gateOutDir, 0o755); err != nil {
		return nil, fmt.Errorf("prepare build gate out dir: %w", err)
	}
	mounts := []ContainerMount{
		{Source: workspace, Target: "/workspace", ReadOnly: false},
		{Source: gateOutDir, Target: BuildGateContainerOutDir, ReadOnly: false},
	}
	if strings.EqualFold(plan.tool, "gradle") {
		gradleCacheHitsHostPath := filepath.Join(workspace, BuildGateGradleCacheHitsHostFile)
		if err := os.WriteFile(gradleCacheHitsHostPath, nil, 0o644); err != nil {
			return nil, fmt.Errorf("prepare gradle cache hits file: %w", err)
		}
		mounts = append(mounts, ContainerMount{
			Source:   gradleCacheHitsHostPath,
			Target:   BuildGateGradleCacheHitsContainerFile,
			ReadOnly: false,
		})
	}
	// Optional limits via env (human suffixes supported for memory/disk). 0 => unlimited.
	limitMem, _ := parseBytesLimitEnv(buildGateLimitMemoryEnv)
	limitCPUMillis := parseInt64LimitEnv(buildGateLimitCPUEnv)
	limitDisk, storageSizeOpt := parseBytesLimitEnv(buildGateLimitDiskEnv)
	// Copy env from gate spec and apply prep override env (if present).
	// Prep override keys win on conflicts.
	envCopy := contracts.MergeEnv(spec.Env, plan.env)
	mounts = appendDockerHostSocketMount(mounts, envCopy)
	specC := ContainerSpec{Image: plan.image, Command: plan.cmd, WorkingDir: "/workspace", Mounts: mounts,
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
	res, err := e.rt.Wait(ctx, h)
	if err != nil {
		return nil, err
	}
	logs, _ := e.rt.Logs(ctx, h)

	meta := buildGateExecutionMetadata(workspace, plan.language, plan.tool, plan.release, plan.image, res, logs)
	meta.Resources = collectDockerResourceUsage(ctx, e.rt, h, specC)

	if plan.stackGate != nil {
		meta.StackGate = plan.stackGate
	}
	return meta, nil
}

func appendDockerHostSocketMount(mounts []ContainerMount, env map[string]string) []ContainerMount {
	socketPath := dockerHostSocketPathFromEnv(env)
	if socketPath == "" {
		return mounts
	}
	info, err := os.Stat(socketPath)
	if err != nil || info.IsDir() {
		return mounts
	}
	for _, mount := range mounts {
		if mount.Target == socketPath {
			return mounts
		}
	}
	return append(mounts, ContainerMount{
		Source:   socketPath,
		Target:   socketPath,
		ReadOnly: false,
	})
}

func dockerHostSocketPathFromEnv(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	dockerHost := strings.TrimSpace(env[contracts.GateProfileDockerHostEnv])
	if dockerHost == "" || !strings.HasPrefix(dockerHost, "unix://") {
		return ""
	}
	socketPath := strings.TrimSpace(strings.TrimPrefix(dockerHost, "unix://"))
	if socketPath == "" || !filepath.IsAbs(socketPath) {
		return ""
	}
	return socketPath
}

func buildGateDefaultStacksCatalogPath() string {
	goModuleFile := "go." + "mo" + "d"
	installed := "/etc/ploy/gates/stacks.yaml"
	if info, err := os.Stat(installed); err == nil && !info.IsDir() {
		return installed
	}
	wd, err := os.Getwd()
	if err == nil {
		dir := wd
		for {
			if info, serr := os.Stat(filepath.Join(dir, goModuleFile)); serr == nil && !info.IsDir() {
				candidate := filepath.Join(dir, DefaultStacksCatalogPath)
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
	return DefaultStacksCatalogPath
}

func parseInt64(s string) (int64, error) { return strconv.ParseInt(strings.TrimSpace(s), 10, 64) }

func parseInt64LimitEnv(key string) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	n, err := parseInt64(value)
	if err != nil {
		return 0
	}
	return n
}

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
	if n, err := parseInt64(value); err == nil {
		return n, value
	}
	return 0, value
}

// --- Metadata helpers (merged from gate_docker_metadata.go) ---

func buildGateExecutionMetadata(
	workspace string,
	language string,
	tool string,
	release string,
	image string,
	res ContainerResult,
	logs []byte,
) *contracts.BuildGateStageMetadata {
	passed := res.ExitCode == 0
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: language,
			Tool:     tool,
			Passed:   passed,
		}},
		Detected: &contracts.StackExpectation{
			Language: strings.TrimSpace(language),
			Tool:     strings.TrimSpace(tool),
			Release:  strings.TrimSpace(release),
		},
		RuntimeImage: image,
	}
	if passed && strings.EqualFold(tool, "gradle") {
		if hits := readGradleBuildCacheHits(workspace); len(hits) > 0 {
			meta.LogFindings = append(meta.LogFindings, contracts.BuildGateLogFinding{
				Severity: "info",
				Code:     "GRADLE_BUILD_CACHE_HIT",
				Message:  fmt.Sprintf("gradle build cache hits (%d): %s", len(hits), strings.Join(hits, ", ")),
			})
		}
	}
	if !passed {
		trimmed := TrimBuildGateLog(tool, string(logs))
		msg := strings.TrimSpace(trimmed)
		if msg == "" {
			msg = fmt.Sprintf("%s build failed (exit %d)", tool, res.ExitCode)
		}
		meta.LogFindings = append(meta.LogFindings, contracts.BuildGateLogFinding{Severity: "error", Message: msg})
	}
	attachLogsTextAndDigest(meta, logs)
	return meta
}

func attachLogsTextAndDigest(meta *contracts.BuildGateStageMetadata, logs []byte) {
	const maxLogBytes = 10 << 20 // 10 MiB safety cap in memory
	if len(logs) > maxLogBytes {
		logs = logs[:maxLogBytes]
	}
	meta.LogsText = string(logs)
	meta.LogDigest = sha256Digest(logs)
}

func sha256Digest(b []byte) types.Sha256Digest {
	h := sha256.Sum256(b)
	return types.Sha256Digest(fmt.Sprintf("sha256:%x", h[:]))
}
