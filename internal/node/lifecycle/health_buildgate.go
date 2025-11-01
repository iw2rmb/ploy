package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// BuildGateJavaChecker validates that the Java build gate can run by probing for the
// configured Maven Docker image. Docker availability is separately checked by DockerChecker.
type BuildGateJavaChecker struct {
	image   string
	runner  commandRunner
	timeout time.Duration
	now     func() time.Time
}

// BuildGateJavaCheckerOptions configure the Java build gate checker.
type BuildGateJavaCheckerOptions struct {
	Image   string
	Runner  commandRunner
	Timeout time.Duration
	Clock   func() time.Time
}

// NewBuildGateJavaChecker constructs a checker that inspects the Maven image.
func NewBuildGateJavaChecker(opts BuildGateJavaCheckerOptions) *BuildGateJavaChecker {
	image := strings.TrimSpace(opts.Image)
	if image == "" {
		if env := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_JAVA_IMAGE")); env != "" {
			image = env
		} else {
			image = "maven:3-eclipse-temurin-17"
		}
	}
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &BuildGateJavaChecker{image: image, runner: runner, timeout: timeout, now: clock}
}

// Check probes for the docker image presence. If missing, it still returns ok with a hint,
// because the executor can pull on demand.
func (c *BuildGateJavaChecker) Check(ctx context.Context) ComponentStatus {
	if c == nil || c.runner == nil {
		return ComponentStatus{State: stateUnknown, CheckedAt: time.Now().UTC(), Message: "build gate checker unavailable"}
	}
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	// docker image inspect <image>
	stdout, stderr, err := c.runner.Run(checkCtx, "docker", "image", "inspect", c.image)
	status := ComponentStatus{
		State:     stateOK,
		Message:   fmt.Sprintf("image %s present", c.image),
		CheckedAt: c.now(),
		Details:   map[string]any{"image": c.image},
	}
	_ = stdout
	if err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound):
			status.State = stateDegraded
			status.Message = "docker not found; Java build gate may fallback to wrappers"
		default:
			// Image likely missing; not fatal — executor can pull on demand.
			status.Message = fmt.Sprintf("image not present: %s", strings.TrimSpace(stderr))
		}
	}
	return status
}
