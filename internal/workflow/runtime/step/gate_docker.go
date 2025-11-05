package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	specC := ContainerSpec{Image: image, Command: cmd, WorkingDir: "/workspace", Mounts: mounts}
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
