// gate_docker.go implements the Docker-based GateExecutor for build validation.
//
// ## HTTP Build Gate and Docker Gate Consistency
//
// This GateExecutor provides the CANONICAL gate validation used by the node agent.
// While healing mods may optionally call the HTTP Build Gate API directly for
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
//     only. The node agent always re-runs this Docker gate after healing mods
//     complete, regardless of any intermediate validation results.
//
// ## Usage Note for Healing Mods
//
// Direct HTTP Build Gate API calls from healing mods are now DISCOURAGED for
// mods-codex. The node agent handles all gate orchestration, ensuring consistent
// behavior and complete history capture.
package step

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	units "github.com/docker/go-units"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"

	// Import moby client for ContainerStats/Inspect option types used in
	// resource usage gathering below. The actual client calls go through
	// DockerContainerRuntime.client which is a *client.Client.
	"github.com/moby/moby/client"
)

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

// NewDockerGateExecutor constructs a GateExecutor that uses the provided
// ContainerRuntime to run build commands.
func NewDockerGateExecutor(rt ContainerRuntime) GateExecutor {
	return &dockerGateExecutor{rt: rt}
}

// Execute runs the Build Gate inside a container image and returns
// normalized metadata about the outcome. Detection rules:
//   - Java (Maven): if pom.xml exists → run `mvn -B -q test` in
//     `PLOY_BUILDGATE_JAVA_IMAGE` or default `maven:3-eclipse-temurin-17`.
//   - Java (Gradle): if build.gradle(.kts) exists → run `gradle -q test`
//     in `PLOY_BUILDGATE_GRADLE_IMAGE` or default `gradle:8.8-jdk17`.
//
// The workspace is mounted at /workspace and used as the working directory.
// Unknown/unsupported projects return an empty metadata object without error.
// When the container runtime is nil, execution is skipped and empty metadata
// is returned. A non‑zero exit code is reported as a static check failure and
// a single log finding containing the captured logs or a synthesized message.
//
// ## Returned Metadata
//
// The BuildGateStageMetadata returned by this method is the CANONICAL gate result
// used by the node agent for decision-making and history capture. It includes:
//   - StaticChecks: Pass/fail status with language and tool information
//   - LogFindings: Structured error messages extracted from build output
//   - LogsText: Full build log text (truncated to 256 KiB) for debugging
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
	// Detect or force profile.
	// Supported explicit profiles: "java", "java-maven", "java-gradle".
	// When empty or unknown, auto-detect: pom.xml -> maven; build.gradle(.kts) -> gradle; else plain java.
	desiredProfile := strings.ToLower(strings.TrimSpace(spec.Profile))
	if desiredProfile == "" {
		desiredProfile = strings.ToLower(strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_PROFILE")))
	}

	hasMaven := fileExists(filepath.Join(workspace, "pom.xml"))
	hasGradle := fileExists(filepath.Join(workspace, "build.gradle")) || fileExists(filepath.Join(workspace, "build.gradle.kts"))

	var image string
	var cmd []string
	var tool string
	// Unified image override takes precedence for all stacks.
	if v := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_IMAGE")); v != "" {
		image = v
	}

	// Stack Gate mode: resolve image from mapping when Stack Gate is enabled with expectations.
	stackGateMode := spec.StackGate != nil && spec.StackGate.Enabled && spec.StackGate.Expect != nil
	if stackGateMode && image == "" {
		// Resolve image using the image mapping resolver.
		// Precedence: mod-level ImageOverrides > cluster-level ClusterImages > default file.
		resolver, err := NewBuildGateImageResolver(
			DefaultMappingPath,
			spec.StackGate.ClusterImages,  // cluster/global inline from node config
			spec.StackGate.ImageOverrides, // mod-level overrides from run spec
			true,                          // stackGateEnabled
		)
		if err != nil {
			return nil, fmt.Errorf("resolve stack gate image: %w", err)
		}
		resolvedImage, err := resolver.Resolve(*spec.StackGate.Expect)
		if err != nil {
			return nil, fmt.Errorf("resolve stack gate image: %w", err)
		}
		image = resolvedImage

		// Determine tool from expectation or detect from workspace.
		tool = spec.StackGate.Expect.Tool
		if tool == "" {
			// Tool-agnostic: detect from workspace.
			tool = detectToolFromWorkspace(workspace, hasMaven, hasGradle)
		}
		cmd = buildCommandForTool(tool)
	}

	chooseMaven := func() {
		if image == "" {
			image = defaultString(os.Getenv("PLOY_BUILDGATE_JAVA_IMAGE"), "maven:3-eclipse-temurin-17")
		}
		tool = "maven"
		// Always include -e for full ERROR-level stack traces. Keep -q to reduce noise.
		// Use --ff (fail-fast) to stop on the first failing module and run a full
		// `clean install` so compilation and tests are validated together.
		// Diagnostic guidance: switch to -X (drop -q) only for deep investigations.
		// Prepend CA preamble to honor CA_CERTS_PEM_BUNDLE from global config.
		script := caPreambleScript() + "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"
		cmd = []string{"/bin/sh", "-lc", script}
	}
	chooseGradle := func() {
		if image == "" {
			image = defaultString(os.Getenv("PLOY_BUILDGATE_GRADLE_IMAGE"), "gradle:8.8-jdk17")
		}
		tool = "gradle"
		// Include --stacktrace for error stack traces (similar to Maven -e). Keep -q to reduce noise.
		// Run Gradle tests for the workspace project and let Gradle's default failure semantics apply.
		// Prepend CA preamble to honor CA_CERTS_PEM_BUNDLE from global config.
		script := caPreambleScript() + "gradle -q --stacktrace test -p /workspace"
		cmd = []string{"/bin/sh", "-lc", script}
	}
	chooseJava := func() {
		if image == "" {
			image = defaultString(os.Getenv("PLOY_BUILDGATE_JAVA_IMAGE"), "eclipse-temurin:17-jdk")
		}
		tool = "java"
		// Compile all Java sources if present; succeed if none found.
		// Prepend CA preamble to honor CA_CERTS_PEM_BUNDLE from global config.
		script := caPreambleScript() + `set -e
tmpdir=$(mktemp -d)
find /workspace -type f -name "*.java" > "$tmpdir/sources.list" || true
if [ -s "$tmpdir/sources.list" ]; then
  mkdir -p "$tmpdir/classes"
  javac --release 17 -d "$tmpdir/classes" @"$tmpdir/sources.list"
  echo "javac compile OK"
else
  echo "No Java sources under /workspace"
fi`
		cmd = []string{"/bin/sh", "-lc", script}
	}

	// Legacy mode: existing profile-based selection (only when not already resolved via Stack Gate).
	if !stackGateMode || image == "" {
		switch desiredProfile {
		case "java-maven":
			chooseMaven()
		case "java-gradle":
			chooseGradle()
		case "java":
			chooseJava()
		default: // auto
			switch {
			case hasMaven:
				chooseMaven()
			case hasGradle:
				chooseGradle()
			default:
				// Fall back to plain Java compilation.
				chooseJava()
			}
		}
	}

	// Build container spec with workspace mount.
	mounts := []ContainerMount{{Source: workspace, Target: "/workspace", ReadOnly: false}}
	// Optional limits via env (human suffixes supported for memory/disk). 0 => unlimited.
	var (
		limitMem       int64
		limitCPUMillis int64
		limitDisk      int64
		storageSizeOpt string
	)
	if v := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_LIMIT_MEMORY_BYTES")); v != "" {
		if n, err := units.RAMInBytes(v); err == nil {
			limitMem = n
		} else if n2, err2 := units.FromHumanSize(v); err2 == nil {
			limitMem = n2
		} else if n3, err3 := parseInt64(v); err3 == nil {
			limitMem = n3
		}
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_LIMIT_CPU_MILLIS")); v != "" {
		if n, err := parseInt64(v); err == nil {
			limitCPUMillis = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_LIMIT_DISK_SPACE")); v != "" {
		storageSizeOpt = v
		if n, err := units.RAMInBytes(v); err == nil {
			limitDisk = n
		} else if n2, err2 := units.FromHumanSize(v); err2 == nil {
			limitDisk = n2
		} else if n3, err3 := parseInt64(v); err3 == nil {
			limitDisk = n3
		}
	}
	// Copy env from gate spec to pass through all environment variables to the
	// Docker container. This includes global env vars injected by the control plane
	// (e.g., CA_CERTS_PEM_BUNDLE, CODEX_AUTH_JSON) which image-level startup hooks
	// may consume.
	var envCopy map[string]string
	if len(spec.Env) > 0 {
		envCopy = make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			envCopy[k] = v
		}
	}
	specC := ContainerSpec{Image: image, Command: cmd, WorkingDir: "/workspace", Mounts: mounts,
		Env:              envCopy,
		LimitMemoryBytes: limitMem,
		LimitNanoCPUs:    limitCPUMillis * 1_000_000, // millis → nanos
		LimitDiskBytes:   limitDisk,
		StorageSizeOpt:   strings.TrimSpace(storageSizeOpt),
	}
	if e.rt == nil {
		return &contracts.BuildGateStageMetadata{}, nil
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

	passed := res.ExitCode == 0
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: "java",
			Tool:     tool,
			Passed:   passed,
		}},
	}
	if !passed {
		// For known tools (Maven, Gradle), trim logs down to the most relevant
		// failure region to keep diagnostics focused. Unknown tools receive the
		// raw logs without additional trimming.
		trimmed := TrimBuildGateLog(tool, string(logs))
		msg := strings.TrimSpace(trimmed)
		if msg == "" {
			msg = fmt.Sprintf("%s build failed (exit %d)", tool, res.ExitCode)
		}
		meta.LogFindings = append(meta.LogFindings, contracts.BuildGateLogFinding{Severity: "error", Message: msg})
	}
	// Attach logs text (truncated) for node-side artifact upload, and compute a digest.
	const maxLogBytes = 1 << 20 // 1 MiB safety cap in memory
	if len(logs) > maxLogBytes {
		logs = logs[:maxLogBytes]
	}
	meta.LogsText = string(logs)
	meta.LogDigest = sha256Digest(logs)

	// Gather resource usage via Docker stats when available.
	// Moby Engine v29 SDK uses client.ContainerStatsOptions{Stream: false} instead
	// of a boolean stream parameter. ContainerInspect requires ContainerInspectOptions
	// with Size: true to populate SizeRw, and returns ContainerInspectResult with
	// Container field containing the InspectResponse.
	if d, ok := e.rt.(*DockerContainerRuntime); ok && d != nil && d.client != nil {
		if stats, err := d.client.ContainerStats(ctx, h.ID, dockerStatsOptions()); err == nil && stats.Body != nil {
			var sj struct {
				MemoryStats struct{ Usage, MaxUsage uint64 } `json:"memory_stats"`
				CPUStats    struct {
					CPUUsage struct{ TotalUsage uint64 } `json:"cpu_usage"`
				} `json:"cpu_stats"`
				// Docker v1.41 Stats JSON fields
				BlkioStats struct {
					IoServiceBytesRecursive []struct {
						Op    string
						Value uint64
					}
				} `json:"blkio_stats"`
			}
			_ = json.NewDecoder(stats.Body).Decode(&sj)
			_ = stats.Body.Close()
			var readBytes, writeBytes uint64
			for _, rec := range sj.BlkioStats.IoServiceBytesRecursive {
				switch strings.ToLower(rec.Op) {
				case "read":
					readBytes += rec.Value
				case "write":
					writeBytes += rec.Value
				}
			}
			// Inspect for SizeRw if available. Moby v29 SDK requires Size: true in
			// options and returns ContainerInspectResult with Container.SizeRw.
			var sizeRw *int64
			if inspect, ierr := d.client.ContainerInspect(ctx, h.ID, dockerInspectOptionsWithSize()); ierr == nil {
				if inspect.Container.SizeRw != nil {
					size := *inspect.Container.SizeRw
					sizeRw = &size
				}
			}
			meta.Resources = &contracts.BuildGateResourceUsage{
				LimitNanoCPUs:    specC.LimitNanoCPUs,
				LimitMemoryBytes: specC.LimitMemoryBytes,
				CPUTotalNs:       sj.CPUStats.CPUUsage.TotalUsage,
				MemUsageBytes:    sj.MemoryStats.Usage,
				MemMaxBytes:      sj.MemoryStats.MaxUsage,
				BlkioReadBytes:   readBytes,
				BlkioWriteBytes:  writeBytes,
				SizeRwBytes:      sizeRw,
			}
		}
	}
	// Best-effort cleanup: remove container after logs and stats are collected.
	if remover, ok := e.rt.(interface {
		Remove(context.Context, ContainerHandle) error
	}); ok {
		_ = remover.Remove(ctx, h)
	}
	return meta, nil
}

func fileExists(path string) bool {
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return true
	}
	return false
}

func defaultString(v, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func parseInt64(s string) (int64, error) { return strconv.ParseInt(strings.TrimSpace(s), 10, 64) }

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

func sha256Digest(b []byte) types.Sha256Digest {
	h := sha256.Sum256(b)
	return types.Sha256Digest(fmt.Sprintf("sha256:%x", h[:]))
}

// dockerStatsOptions returns the moby client.ContainerStatsOptions for a
// one-shot (non-streaming) stats call. Stream: false tells the daemon to
// return a single stats sample and close the connection.
func dockerStatsOptions() client.ContainerStatsOptions {
	return client.ContainerStatsOptions{Stream: false}
}

// dockerInspectOptionsWithSize returns the moby client.ContainerInspectOptions
// with Size: true to populate SizeRw and SizeRootFs in the response.
func dockerInspectOptionsWithSize() client.ContainerInspectOptions {
	return client.ContainerInspectOptions{Size: true}
}

// detectToolFromWorkspace detects the build tool from workspace files.
// Returns "maven" if pom.xml exists, "gradle" if build.gradle exists, otherwise "java".
func detectToolFromWorkspace(workspace string, hasMaven, hasGradle bool) string {
	switch {
	case hasMaven:
		return "maven"
	case hasGradle:
		return "gradle"
	default:
		return "java"
	}
}

// buildCommandForTool returns the appropriate build command for the given tool.
func buildCommandForTool(tool string) []string {
	preamble := caPreambleScript()
	switch tool {
	case "maven":
		script := preamble + "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"
		return []string{"/bin/sh", "-lc", script}
	case "gradle":
		script := preamble + "gradle -q --stacktrace test -p /workspace"
		return []string{"/bin/sh", "-lc", script}
	default:
		script := preamble + `set -e
tmpdir=$(mktemp -d)
find /workspace -type f -name "*.java" > "$tmpdir/sources.list" || true
if [ -s "$tmpdir/sources.list" ]; then
  mkdir -p "$tmpdir/classes"
  javac --release 17 -d "$tmpdir/classes" @"$tmpdir/sources.list"
  echo "javac compile OK"
else
  echo "No Java sources under /workspace"
fi`
		return []string{"/bin/sh", "-lc", script}
	}
}
