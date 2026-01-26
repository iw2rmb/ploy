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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	units "github.com/docker/go-units"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"

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
// normalized metadata about the outcome.
//
// Image selection (highest wins):
//   - `PLOY_BUILDGATE_IMAGE` (single image override)
//   - Default mapping file (`etc/ploy/gates/build-gate-images.yaml`) + mod YAML overrides (`build_gate.images[]`)
//
// The workspace is mounted at /workspace and used as the working directory.
// When stack detection fails, the gate fails with a static check report and
// a structured log finding (including evidence when available).
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
	envImage := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_IMAGE"))
	mappingPath := buildGateDefaultImagesFilePath()
	stackGateMode := spec.StackGate != nil && spec.StackGate.Enabled && spec.StackGate.Expect != nil

	obs, detectErr := stackdetect.Detect(ctx, workspace)

	var (
		image    string
		cmd      []string
		language string
		tool     string
		sgResult *contracts.StackGateResult
	)

	if stackGateMode {
		sgResult = &contracts.StackGateResult{
			Enabled:  true,
			Expected: spec.StackGate.Expect,
		}

		if detectErr != nil {
			var detErr *stackdetect.DetectionError
			var evidenceStr string
			if errors.As(detectErr, &detErr) {
				sgResult.Result = "unknown"
				sgResult.Reason = detErr.Message
				evidenceStr = formatEvidenceForLog(detErr.Evidence)
			} else {
				sgResult.Result = "unknown"
				sgResult.Reason = detectErr.Error()
			}
			return &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: spec.StackGate.Expect.Language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_UNKNOWN",
					Message:  sgResult.Reason,
					Evidence: evidenceStr,
				}},
			}, nil
		}

		sgResult.Detected = observationToStackExpectation(obs)

		if !stackMatchesExpectation(obs, spec.StackGate.Expect) {
			sgResult.Result = "mismatch"
			sgResult.Reason = formatMismatchReason(obs, spec.StackGate.Expect)
			evidenceStr := formatEvidenceForLog(obs.Evidence)
			return &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: spec.StackGate.Expect.Language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_MISMATCH",
					Message:  sgResult.Reason,
					Evidence: evidenceStr,
				}},
			}, nil
		}

		sgResult.Result = "pass"

		language = strings.TrimSpace(spec.StackGate.Expect.Language)
		if language == "" {
			language = strings.TrimSpace(obs.Language)
		}
		tool = strings.TrimSpace(obs.Tool)

		if envImage != "" {
			image = envImage
		} else {
			if strings.TrimSpace(spec.StackGate.Expect.Release) == "" {
				sgResult.Result = "unknown"
				sgResult.Reason = "stack gate expectation missing release; cannot resolve runtime image"
				return &contracts.BuildGateStageMetadata{
					StackGate: sgResult,
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     "stack-gate",
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "STACK_GATE_INVALID_EXPECTATION",
						Message:  sgResult.Reason,
					}},
				}, nil
			}

			resolver, err := NewBuildGateImageResolver(mappingPath, spec.ImageOverrides, true)
			if err != nil {
				sgResult.Result = "unknown"
				sgResult.Reason = fmt.Sprintf("image mapping error: %s", err.Error())
				return &contracts.BuildGateStageMetadata{
					StackGate: sgResult,
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     "stack-gate",
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "STACK_GATE_IMAGE_MAPPING_ERROR",
						Message:  sgResult.Reason,
					}},
				}, nil
			}

			resolvedImage, err := resolver.Resolve(*spec.StackGate.Expect)
			if err != nil {
				sgResult.Result = "unknown"
				sgResult.Reason = fmt.Sprintf("no matching image rule: %s", err.Error())
				return &contracts.BuildGateStageMetadata{
					StackGate: sgResult,
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     "stack-gate",
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "STACK_GATE_NO_IMAGE_RULE",
						Message:  sgResult.Reason,
					}},
				}, nil
			}
			image = resolvedImage
		}

		sgResult.RuntimeImage = image
		var err error
		cmd, err = buildCommandForTool(tool)
		if err != nil {
			sgResult.Result = "unknown"
			sgResult.Reason = err.Error()
			return &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_UNKNOWN",
					Message:  sgResult.Reason,
				}},
			}, nil
		}
	} else {
		if detectErr != nil {
			var detErr *stackdetect.DetectionError
			var evidenceStr string
			msg := detectErr.Error()
			if errors.As(detectErr, &detErr) {
				msg = detErr.Message
				evidenceStr = formatEvidenceForLog(detErr.Evidence)
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Tool:   "stackdetect",
					Passed: false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "BUILD_GATE_STACK_DETECT_FAILED",
					Message:  msg,
					Evidence: evidenceStr,
				}},
			}, nil
		}

		exp := observationToStackExpectation(obs)
		if exp == nil || strings.TrimSpace(exp.Language) == "" || strings.TrimSpace(exp.Release) == "" {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Tool:   "stackdetect",
					Passed: false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "BUILD_GATE_STACK_DETECT_FAILED",
					Message:  "stack detection produced incomplete result; language and release are required",
				}},
			}, nil
		}

		language = exp.Language
		tool = obs.Tool

		if envImage != "" {
			image = envImage
		} else {
			resolver, err := NewBuildGateImageResolver(mappingPath, spec.ImageOverrides, true)
			if err != nil {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     tool,
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "BUILD_GATE_IMAGE_MAPPING_ERROR",
						Message:  err.Error(),
					}},
				}, nil
			}
			resolved, err := resolver.Resolve(*exp)
			if err != nil {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     tool,
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "BUILD_GATE_NO_IMAGE_RULE",
						Message:  err.Error(),
					}},
				}, nil
			}
			image = resolved
		}

		var err error
		cmd, err = buildCommandForTool(tool)
		if err != nil {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: language,
					Tool:     tool,
					Passed:   false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "BUILD_GATE_UNKNOWN_TOOL",
					Message:  err.Error(),
				}},
			}, nil
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
			Language: language,
			Tool:     tool,
			Passed:   passed,
		}},
		RuntimeImage: image,
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
	// If we were in stack gate mode, attach the result.
	if sgResult != nil {
		meta.StackGate = sgResult
	}
	return meta, nil
}

func fileExists(path string) bool {
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return true
	}
	return false
}

func buildGateDefaultImagesFilePath() string {
	installed := "/etc/ploy/gates/build-gate-images.yaml"
	if fileExists(installed) {
		return installed
	}
	wd, err := os.Getwd()
	if err == nil {
		dir := wd
		for {
			if fileExists(filepath.Join(dir, "go.mod")) {
				candidate := filepath.Join(dir, DefaultMappingPath)
				if fileExists(candidate) {
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
	return DefaultMappingPath
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

// buildCommandForTool returns the appropriate build command for the given tool.
func buildCommandForTool(tool string) ([]string, error) {
	preamble := caPreambleScript()
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "maven":
		script := preamble + "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"
		return []string{"/bin/sh", "-lc", script}, nil
	case "gradle":
		script := preamble + "gradle -q --stacktrace test -p /workspace"
		return []string{"/bin/sh", "-lc", script}, nil
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

// observationToStackExpectation converts a stackdetect.Observation to a StackExpectation.
func observationToStackExpectation(obs *stackdetect.Observation) *contracts.StackExpectation {
	if obs == nil {
		return nil
	}
	exp := &contracts.StackExpectation{
		Language: obs.Language,
		Tool:     obs.Tool,
	}
	if obs.Release != nil {
		exp.Release = *obs.Release
	}
	return exp
}

// stackMatchesExpectation compares a detected observation against expected values.
// Returns true if all non-empty expected fields match the observation.
func stackMatchesExpectation(obs *stackdetect.Observation, expect *contracts.StackExpectation) bool {
	if expect == nil {
		return true
	}
	if expect.Language != "" && obs.Language != expect.Language {
		return false
	}
	if expect.Tool != "" && obs.Tool != expect.Tool {
		return false
	}
	if expect.Release != "" {
		if obs.Release == nil || *obs.Release != expect.Release {
			return false
		}
	}
	return true
}

// formatMismatchReason generates a human-readable explanation of stack mismatches.
func formatMismatchReason(obs *stackdetect.Observation, expect *contracts.StackExpectation) string {
	var mismatches []string
	if expect.Language != "" && obs.Language != expect.Language {
		mismatches = append(mismatches, fmt.Sprintf("language: expected %q, detected %q", expect.Language, obs.Language))
	}
	if expect.Tool != "" && obs.Tool != expect.Tool {
		mismatches = append(mismatches, fmt.Sprintf("tool: expected %q, detected %q", expect.Tool, obs.Tool))
	}
	if expect.Release != "" {
		detected := "<nil>"
		if obs.Release != nil {
			detected = *obs.Release
		}
		if obs.Release == nil || *obs.Release != expect.Release {
			mismatches = append(mismatches, fmt.Sprintf("release: expected %q, detected %q", expect.Release, detected))
		}
	}
	msg := "stack mismatch: " + strings.Join(mismatches, "; ")

	// Append evidence for debugging.
	if len(obs.Evidence) > 0 {
		msg += "\nevidence:"
		for _, e := range obs.Evidence {
			msg += fmt.Sprintf("\n  - %s: %s = %q", e.Path, e.Key, e.Value)
		}
	}
	return msg
}

// formatEvidenceForLog formats evidence items for the LogFinding.Evidence field.
func formatEvidenceForLog(evidence []stackdetect.EvidenceItem) string {
	if len(evidence) == 0 {
		return ""
	}
	var lines []string
	for _, e := range evidence {
		lines = append(lines, fmt.Sprintf("%s: %s = %q", e.Path, e.Key, e.Value))
	}
	return strings.Join(lines, "\n")
}
