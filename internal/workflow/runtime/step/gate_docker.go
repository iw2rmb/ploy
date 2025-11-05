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

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// dockerGateExecutor runs build validation inside language images using the
// same container runtime as step execution, mounting the workspace at /workspace.
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
func (e *dockerGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if spec == nil || !spec.Enabled {
		return nil, nil
	}
	// Detect Java build system.
	// Prefer Maven when pom.xml exists, otherwise Gradle when build.gradle(.kts) exists.
	hasMaven := fileExists(filepath.Join(workspace, "pom.xml"))
	hasGradle := fileExists(filepath.Join(workspace, "build.gradle")) || fileExists(filepath.Join(workspace, "build.gradle.kts"))

	var image string
	var cmd []string
	var tool string
	switch {
	case hasMaven:
		image = defaultString(os.Getenv("PLOY_BUILDGATE_JAVA_IMAGE"), "maven:3-eclipse-temurin-17")
		tool = "maven"
		cmd = []string{"/bin/sh", "-lc", "mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test"}
	case hasGradle:
		image = defaultString(os.Getenv("PLOY_BUILDGATE_GRADLE_IMAGE"), "gradle:8.8-jdk17")
		tool = "gradle"
		cmd = []string{"/bin/sh", "-lc", "gradle -q test -p /workspace"}
	default:
		// Unknown project; return empty metadata rather than failing gate.
		return &contracts.BuildGateStageMetadata{}, nil
	}

	// Build container spec with workspace mount.
	mounts := []ContainerMount{{Source: workspace, Target: "/workspace", ReadOnly: false}}
	// Optional limits via env (bytes and millis). 0 => unlimited.
	var limitMem int64
	var limitCPUMillis int64
	if v := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_LIMIT_MEMORY_BYTES")); v != "" {
		if n, err := parseInt64(v); err == nil {
			limitMem = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_LIMIT_CPU_MILLIS")); v != "" {
		if n, err := parseInt64(v); err == nil {
			limitCPUMillis = n
		}
	}
	specC := ContainerSpec{Image: image, Command: cmd, WorkingDir: "/workspace", Mounts: mounts,
		LimitMemoryBytes: limitMem,
		LimitNanoCPUs:    limitCPUMillis * 1_000_000, // millis → nanos
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
		msg := strings.TrimSpace(string(logs))
		if msg == "" {
			msg = fmt.Sprintf("%s build failed (exit %d)", tool, res.ExitCode)
		}
		meta.LogFindings = append(meta.LogFindings, contracts.BuildGateLogFinding{Severity: "error", Message: msg})
	}
	// Attach logs text (truncated) for node-side artifact upload, and compute a digest.
	const maxLogBytes = 256 << 10 // 256 KiB safety cap in memory
	if len(logs) > maxLogBytes {
		logs = logs[:maxLogBytes]
	}
	meta.LogsText = string(logs)
	meta.LogDigest = sha256Hex(logs)

	// Gather resource usage via Docker stats when available.
	if d, ok := e.rt.(*DockerContainerRuntime); ok && d != nil && d.client != nil {
		if stats, err := d.client.ContainerStats(ctx, h.ID, false); err == nil && stats.Body != nil {
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
			// Inspect for SizeRw if available
			var sizeRw *int64
			if inspect, ierr := d.client.ContainerInspect(ctx, h.ID); ierr == nil {
				if inspect.SizeRw != nil {
					size := *inspect.SizeRw
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

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:])
}
