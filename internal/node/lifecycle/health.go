package lifecycle

import (
    "bytes"
    "context"
    "os/exec"
    "strings"
    "time"

    "github.com/docker/docker/api/types"
    typesystem "github.com/docker/docker/api/types/system"
    "github.com/docker/docker/client"
)

// DockerChecker probes the Docker Engine for availability and version info.
type DockerChecker struct {
	client  dockerAPI
	timeout time.Duration
	now     func() time.Time
}

type dockerAPI interface {
	Ping(ctx context.Context) (types.Ping, error)
	Info(ctx context.Context) (typesystem.Info, error)
	Close() error
}

// DockerCheckerOptions configure the Docker health checker.
type DockerCheckerOptions struct {
	Client  dockerAPI
	Host    string
	Timeout time.Duration
	Clock   func() time.Time
}

// NewDockerChecker constructs a Docker checker using the environment or provided client.
func NewDockerChecker(opts DockerCheckerOptions) (*DockerChecker, error) {
	clientAPI := opts.Client
	if clientAPI == nil {
		cliOpts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
		if trimmed := strings.TrimSpace(opts.Host); trimmed != "" {
			cliOpts = append(cliOpts, client.WithHost(trimmed))
		}
		cli, err := client.NewClientWithOpts(cliOpts...)
		if err != nil {
			return nil, err
		}
		clientAPI = cli
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &DockerChecker{
		client:  clientAPI,
		timeout: timeout,
		now:     clock,
	}, nil
}

// Close releases the underlying Docker client when it implements Close.
func (c *DockerChecker) Close() error {
	if closer, ok := c.client.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// Check reports Docker health by issuing ping and info calls.
func (c *DockerChecker) Check(ctx context.Context) ComponentStatus {
	if c == nil || c.client == nil {
		return ComponentStatus{State: stateUnknown, CheckedAt: time.Now().UTC(), Message: "docker client unavailable"}
	}
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ping, pingErr := c.client.Ping(checkCtx)
	status := ComponentStatus{
		State:     stateOK,
		CheckedAt: c.now(),
		Details: map[string]any{
			"api_version": ping.APIVersion,
		},
	}
	if pingErr != nil {
		status.State = stateError
		status.Message = pingErr.Error()
		return status
	}
	info, infoErr := c.client.Info(checkCtx)
	if infoErr != nil {
		status.State = stateDegraded
		status.Message = infoErr.Error()
		return status
	}
	status.Version = strings.TrimSpace(info.ServerVersion)
	status.Details["containers_running"] = info.ContainersRunning
	status.Details["driver"] = strings.TrimSpace(info.Driver)
	return status
}

// commandRunner abstracts simple command execution for health checkers.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, string, error)
}

type execRunner struct{}

// Run executes the named command and returns stdout, stderr, and error.
func (execRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func extractFirstLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
