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
//   - Full gate history: The node agent captures every gate execution in
//     BuildGateStageMetadata, enabling
//     complete telemetry and audit trails.
//
//   - Authoritative results: In-container HTTP Build Gate API calls are advisory
//     only. The node agent always re-runs this Docker gate after healing migs
//     complete, regardless of any intermediate validation results.
//
// ## Usage Note for Healing Migs
//
// Direct HTTP Build Gate API calls from healing migs are now DISCOURAGED for
// codex. The node agent handles all gate orchestration, ensuring consistent
// behavior and complete history capture.
package step

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	units "github.com/docker/go-units"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	buildGateLimitMemoryEnv = "PLOY_BUILDGATE_LIMIT_MEMORY_BYTES"
	buildGateLimitCPUEnv    = "PLOY_BUILDGATE_LIMIT_CPU_MILLIS"
	buildGateLimitDiskEnv   = "PLOY_BUILDGATE_LIMIT_DISK_SPACE"
	buildGateCacheRootEnv   = "PLOY_BUILDGATE_CACHE_ROOT"
	buildGateCacheRootDir   = "/var/cache/ploy/gates"
	buildGateTmpCacheRoot   = "ploy/gates"
	// BuildGateGradleCacheHitsHostFile is the workspace-local file mounted into
	// Gradle gate containers for cache-hit signaling from the init script.
	BuildGateGradleCacheHitsHostFile = ".ploy-gradle-build-cache-hits"
	// BuildGateGradleCacheHitsContainerFile is the in-container path consumed by
	// the Gradle init script to write cache-hit markers.
	BuildGateGradleCacheHitsContainerFile = "/tmp/gradle-build-cache-hits"

	// BuildGateWorkspaceOutDir is a workspace-local host directory mounted
	// into gate containers as /out for deterministic artifact collection.
	BuildGateWorkspaceOutDir = ".ploy-gate-out"
	// BuildGateWorkspaceInDir is an optional workspace-local host directory
	// mounted into gate containers as /in for cross-step runtime inputs.
	BuildGateWorkspaceInDir = ".ploy-gate-in"
	// BuildGateContainerInDir is the writable input mount path inside gate
	// containers used by orchestrator-provided runtime inputs.
	BuildGateContainerInDir = "/in"
	// BuildGateContainerOutDir is the writable output mount path inside gate
	// containers used by runtime-generated artifacts (for example Gradle reports).
	BuildGateContainerOutDir = "/out"
	// BuildGateGradleUserHomeDir is the native Gradle home path in gate-gradle images.
	BuildGateGradleUserHomeDir = "/root/.gradle"
	// BuildGateMavenUserHomeDir is the native Maven repository path in Maven gate images.
	BuildGateMavenUserHomeDir = "/root/.m2"
)

var errBuildGateRuntimeUnavailable = errors.New("build gate runtime unavailable")

// dockerGateExecutor runs build validation inside language images using the
// same container runtime as step execution, mounting the workspace at /workspace.
//
// This executor is the CANONICAL source of gate validation results for the node
// agent. The node agent always uses this executor for gate validation phases,
// ensuring consistent
// behavior and complete history capture.
type dockerGateExecutor struct {
	rt ContainerRuntime
}

type logStreamingRuntime interface {
	StreamLogs(ctx context.Context, handle ContainerHandle, stdout, stderr io.Writer) error
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
// This metadata is captured by the node agent for gate validation phases,
// ensuring complete gate history.
func (e *dockerGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if spec == nil || !spec.Enabled {
		return nil, nil
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
	gateInDir := filepath.Join(workspace, BuildGateWorkspaceInDir)
	if info, statErr := os.Stat(gateInDir); statErr == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("build gate in path is not a directory: %s", gateInDir)
		}
		mounts = append(mounts, ContainerMount{Source: gateInDir, Target: BuildGateContainerInDir, ReadOnly: false})
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("stat build gate in dir: %w", statErr)
	}
	gateShareDir := gateShareDirFromContext(ctx)
	if gateShareDir != "" {
		info, statErr := os.Stat(gateShareDir)
		if statErr != nil {
			return nil, fmt.Errorf("stat build gate share dir: %w", statErr)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("build gate share path is not a directory: %s", gateShareDir)
		}
		mounts = append(mounts, ContainerMount{Source: gateShareDir, Target: containerShareDir, ReadOnly: false})
	}
	toolCacheMounts, err := buildGateToolCacheMounts(plan.language, plan.tool, plan.release)
	if err != nil {
		return nil, fmt.Errorf("prepare build gate tool cache mounts: %w", err)
	}
	mounts = append(mounts, toolCacheMounts...)
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

	var streamedLogs bytes.Buffer
	liveWriter := executionLogWriterFromContext(ctx)
	var streamDone <-chan error
	if streamer, ok := e.rt.(logStreamingRuntime); ok {
		stdoutWriter := io.Writer(&streamedLogs)
		stderrWriter := io.Writer(&streamedLogs)
		if liveWriter != nil {
			liveStdout := liveWriter
			liveStderr := liveWriter
			if split, ok := liveWriter.(splitLogWriter); ok {
				liveStdout = split.StdoutWriter()
				liveStderr = split.StderrWriter()
			}
			stdoutWriter = io.MultiWriter(&streamedLogs, liveStdout)
			stderrWriter = io.MultiWriter(&streamedLogs, liveStderr)
		}
		done := make(chan error, 1)
		streamDone = done
		go func() {
			done <- streamer.StreamLogs(ctx, h, stdoutWriter, stderrWriter)
		}()
	}

	res, err := e.rt.Wait(ctx, h)
	if err != nil {
		return nil, err
	}

	var logs []byte
	if streamDone != nil {
		select {
		case streamErr := <-streamDone:
			if streamErr == nil {
				logs = append([]byte(nil), streamedLogs.Bytes()...)
			}
		case <-time.After(2 * time.Second):
			// Fall back to one-shot log fetch if stream completion hangs unexpectedly.
		}
	}
	if len(logs) == 0 {
		logs, _ = e.rt.Logs(ctx, h)
		if liveWriter != nil && len(logs) > 0 {
			_, _ = liveWriter.Write(logs)
		}
	}

	executedCommand := gateProfileCommandFromContainerCommand(plan.cmd)
	meta := buildGateExecutionMetadata(workspace, plan.language, plan.tool, plan.release, plan.image, executedCommand, res, logs)
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

func buildGateToolCacheMounts(language, tool, release string) ([]ContainerMount, error) {
	target := buildGateToolCacheTarget(tool)
	if target == "" {
		return nil, nil
	}
	cacheRoot, err := resolveBuildGateCacheRoot()
	if err != nil {
		return nil, err
	}
	hostPath := filepath.Join(
		cacheRoot,
		sanitizeCachePathPart(language, "unknown-lang"),
		sanitizeCachePathPart(tool, "unknown-tool"),
		sanitizeCachePathPart(release, "unknown-release"),
	)
	if err := os.MkdirAll(hostPath, 0o755); err != nil {
		return nil, err
	}
	if err := pruneGateCacheDirOldestFirst(hostPath); err != nil {
		return nil, err
	}
	return []ContainerMount{{
		Source:   hostPath,
		Target:   target,
		ReadOnly: false,
	}}, nil
}

func resolveBuildGateCacheRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(buildGateCacheRootEnv)); override != "" {
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", err
		}
		return override, nil
	}
	if err := os.MkdirAll(buildGateCacheRootDir, 0o755); err == nil {
		return buildGateCacheRootDir, nil
	} else if !os.IsPermission(err) {
		return "", err
	}
	fallback := filepath.Join(os.TempDir(), buildGateTmpCacheRoot)
	if err := os.MkdirAll(fallback, 0o755); err != nil {
		return "", err
	}
	return fallback, nil
}

func buildGateToolCacheTarget(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "gradle":
		return BuildGateGradleUserHomeDir
	case "maven":
		return BuildGateMavenUserHomeDir
	default:
		return ""
	}
}

func sanitizeCachePathPart(value, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	if s == "" {
		return fallback
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
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
	executedCommand string,
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
		RuntimeImage:    image,
		ExecutedCommand: strings.TrimSpace(executedCommand),
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
		trimmed, evidence := BuildGateLogFindingContent(tool, string(logs))
		msg := strings.TrimSpace(trimmed)
		if msg == "" {
			msg = fmt.Sprintf("%s build failed (exit %d)", tool, res.ExitCode)
		}
		finding := contracts.BuildGateLogFinding{Severity: "error", Message: msg}
		if strings.TrimSpace(evidence) != "" {
			finding.Evidence = evidence
		}
		meta.LogFindings = append(meta.LogFindings, finding)
	}
	attachLogsTextAndDigest(meta, logs)
	return meta
}

func gateProfileCommandFromContainerCommand(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	if len(cmd) >= 3 && cmd[0] == "/bin/sh" && (cmd[1] == "-c" || cmd[1] == "-lc") {
		shell := strings.TrimSpace(cmd[2])
		if shell == "" {
			return ""
		}
		// Drop internal wrapper preamble from fallback-generated commands so
		// the persisted gate profile stores the runnable tool command itself.
		prefixes := []string{
			"set -eu; " + gateCAPreamble + "; ",
			gateCAPreamble + "; ",
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(shell, prefix) {
				shell = strings.TrimSpace(strings.TrimPrefix(shell, prefix))
				break
			}
		}
		return shell
	}
	return strings.TrimSpace(strings.Join(cmd, " "))
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
