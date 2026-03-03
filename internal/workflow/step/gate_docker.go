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
// ## Usage Note for Healing Mods
//
// Direct HTTP Build Gate API calls from healing migs are now DISCOURAGED for
// migs-codex. The node agent handles all gate orchestration, ensuring consistent
// behavior and complete history capture.
package step

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
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
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
	"github.com/moby/moby/client"
)

const (
	buildGateLimitMemoryEnv = "PLOY_BUILDGATE_LIMIT_MEMORY_BYTES"
	buildGateLimitCPUEnv    = "PLOY_BUILDGATE_LIMIT_CPU_MILLIS"
	buildGateLimitDiskEnv   = "PLOY_BUILDGATE_LIMIT_DISK_SPACE"

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
	path := filepath.Join(workspace, ".ploy-gradle-build-cache-hits")
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

	obs, detectErr := stackdetect.Detect(ctx, workspace)
	plan, terminal := resolveGateExecutionPlan(ctx, workspace, spec, obs, detectErr, catalogPath)
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

// caPreambleScript returns a shell preamble that installs CA certificates from the
// CA_CERTS_PEM_BUNDLE environment variable into the system trust store and Java
// cacerts keystore. This enables build-gate containers to trust corporate proxies
// and private registries when the global config provides a CA bundle.
//
// The preamble:
// 1. Splits CA_CERTS_PEM_BUNDLE into individual PEM files
// 2. Installs them into /usr/local/share/ca-certificates and runs update-ca-certificates
// 3. Imports each cert into the Java cacerts keystore via keytool (if available)
//
// This preamble is prepended to Maven, Gradle, and plain Java build commands so that
// custom CA certificates injected via `ploy config env set --key CA_CERTS_PEM_BUNDLE ...`
// are honored inside gate containers.
func caPreambleScript() string {
	return `# --- CA bundle injection preamble (ploy global config) ---
if [ -n "${CA_CERTS_PEM_BUNDLE:-}" ]; then
  pem_file="$(mktemp)"
  printf '%s\n' "${CA_CERTS_PEM_BUNDLE}" > "${pem_file}"
  pem_dir="$(mktemp -d)"
  # Split bundle into individual cert files: cert1.crt, cert2.crt, ...
  awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="${pem_dir}" "${pem_file}"
  # Update system CA store if update-ca-certificates is available
  if command -v update-ca-certificates >/dev/null 2>&1; then
    sys_ca_dir="/usr/local/share/ca-certificates/ploy-gate"
    mkdir -p "$sys_ca_dir"
    cp "${pem_dir}"/*.crt "$sys_ca_dir"/ 2>/dev/null || true
    update-ca-certificates >/dev/null 2>&1 || true
  fi
  # Import into Java cacerts keystore if keytool is available
  if command -v keytool >/dev/null 2>&1; then
    for cert_path in "${pem_dir}"/*.crt; do
      [ -f "$cert_path" ] || continue
      base="$(basename "${cert_path}" .crt)"
      alias="ploy_gate_pem_${base}"
      keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias "${alias}" -file "${cert_path}" >/dev/null 2>&1 || true
    done
  fi
fi
# --- End CA bundle preamble ---
`
}

// buildCommandForTool returns the default all-tests command for the given tool.
func buildCommandForTool(workspace string, tool string) ([]string, error) {
	return buildCommandForToolTarget(workspace, tool, contracts.GateProfileTargetAllTests)
}

// buildCommandForToolTarget returns a deterministic command for a tool/target pair.
func buildCommandForToolTarget(workspace string, tool string, target string) ([]string, error) {
	preamble := caPreambleScript()
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "maven":
		switch strings.TrimSpace(target) {
		case contracts.GateProfileTargetBuild:
			script := preamble + "mvn --ff -B -q -e -DskipTests=true -Dstyle.color=never -f /workspace/pom.xml clean install"
			return []string{"/bin/sh", "-lc", script}, nil
		case contracts.GateProfileTargetAllTests:
			script := preamble + "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"
			return []string{"/bin/sh", "-lc", script}, nil
		default:
			return nil, fmt.Errorf("unsupported maven target: %q", target)
		}
	case "gradle":
		gradleExec := "gradle"
		if hasGradleWrapperSpecified(workspace) {
			gradleExec = "./gradlew"
		}
		switch strings.TrimSpace(target) {
		case contracts.GateProfileTargetBuild:
			script := preamble + gradleExec + " -q --stacktrace --build-cache build -x test -p /workspace"
			return []string{"/bin/sh", "-lc", script}, nil
		case contracts.GateProfileTargetAllTests:
			script := preamble + gradleExec + " -q --stacktrace --build-cache test -p /workspace"
			return []string{"/bin/sh", "-lc", script}, nil
		default:
			return nil, fmt.Errorf("unsupported gradle target: %q", target)
		}
	case "go":
		script := preamble + "go test ./..."
		return []string{"/bin/sh", "-lc", script}, nil
	case "cargo":
		script := preamble + "cargo test"
		return []string{"/bin/sh", "-lc", script}, nil
	case "pip", "poetry":
		script := preamble + "python -m compileall -q /workspace"
		return []string{"/bin/sh", "-lc", script}, nil
	default:
		return nil, fmt.Errorf("unsupported build tool: %q", tool)
	}
}

func hasGradleWrapperSpecified(workspace string) bool {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return false
	}
	p := filepath.Join(workspace, "gradle", "wrapper", "gradle-wrapper.properties")
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// --- Image selection helpers (merged from gate_docker_image_selection.go) ---

var (
	errBuildGateImageMapping   = errors.New("build gate image mapping")
	errBuildGateImageRuleMatch = errors.New("build gate image rule match")
)

func resolveImageForExpectation(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	exp contracts.StackExpectation,
	required bool,
) (string, error) {
	resolver, err := NewBuildGateImageResolver(mappingPath, overrides, required)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errBuildGateImageMapping, err)
	}
	resolved, err := resolver.Resolve(exp)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errBuildGateImageRuleMatch, err)
	}
	return resolved, nil
}

func resolveExpectedRuntimeImageForStackGate(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	expect *contracts.StackExpectation,
) (string, error) {
	if expect == nil || strings.TrimSpace(expect.Release) == "" {
		return "", fmt.Errorf("stack gate expectation missing release")
	}
	return resolveImageForExpectation(mappingPath, overrides, *expect, true)
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

// --- Resource usage helpers (merged from gate_docker_resources.go) ---

func collectDockerResourceUsage(
	ctx context.Context,
	rt ContainerRuntime,
	h ContainerHandle,
	spec ContainerSpec,
) *contracts.BuildGateResourceUsage {
	d, ok := rt.(*DockerContainerRuntime)
	if !ok || d == nil || d.stats == nil {
		return nil
	}

	stats, err := d.stats.ContainerStats(ctx, string(h), dockerStatsOptions())
	if err != nil || stats.Body == nil {
		return nil
	}
	defer func() { _ = stats.Body.Close() }()

	var sj struct {
		MemoryStats struct{ Usage, MaxUsage uint64 } `json:"memory_stats"`
		CPUStats    struct {
			CPUUsage struct{ TotalUsage uint64 } `json:"cpu_usage"`
		} `json:"cpu_stats"`
		BlkioStats struct {
			IoServiceBytesRecursive []struct {
				Op    string
				Value uint64
			}
		} `json:"blkio_stats"`
	}
	_ = json.NewDecoder(stats.Body).Decode(&sj)

	var readBytes, writeBytes uint64
	for _, rec := range sj.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(rec.Op) {
		case "read":
			readBytes += rec.Value
		case "write":
			writeBytes += rec.Value
		}
	}

	var sizeRw *int64
	if inspect, ierr := d.client.ContainerInspect(ctx, string(h), dockerInspectOptionsWithSize()); ierr == nil {
		if inspect.Container.SizeRw != nil {
			size := *inspect.Container.SizeRw
			sizeRw = &size
		}
	}

	return &contracts.BuildGateResourceUsage{
		LimitNanoCPUs:    spec.LimitNanoCPUs,
		LimitMemoryBytes: spec.LimitMemoryBytes,
		CPUTotalNs:       sj.CPUStats.CPUUsage.TotalUsage,
		MemUsageBytes:    sj.MemoryStats.Usage,
		MemMaxBytes:      sj.MemoryStats.MaxUsage,
		BlkioReadBytes:   readBytes,
		BlkioWriteBytes:  writeBytes,
		SizeRwBytes:      sizeRw,
	}
}

func dockerStatsOptions() client.ContainerStatsOptions {
	return client.ContainerStatsOptions{Stream: false}
}

func dockerInspectOptionsWithSize() client.ContainerInspectOptions {
	return client.ContainerInspectOptions{Size: true}
}
