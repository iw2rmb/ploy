package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
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

// ShiftChecker verifies the SHIFT CLI is callable and returns its version.
type ShiftChecker struct {
	binary string
	runner commandRunner
	cache  struct {
		mu     sync.Mutex
		status ComponentStatus
	}
	timeout time.Duration
	now     func() time.Time
}

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, string, error)
}

// ShiftCheckerOptions configure the SHIFT health checker.
type ShiftCheckerOptions struct {
	Binary  string
	Runner  commandRunner
	Timeout time.Duration
	Clock   func() time.Time
}

// NewShiftChecker constructs a SHIFT CLI checker.
func NewShiftChecker(opts ShiftCheckerOptions) *ShiftChecker {
	binary := strings.TrimSpace(opts.Binary)
	if binary == "" {
		if envBinary := strings.TrimSpace(os.Getenv("PLOY_SHIFT_BINARY")); envBinary != "" {
			binary = envBinary
		} else {
			binary = "shift"
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
	return &ShiftChecker{
		binary:  binary,
		runner:  runner,
		timeout: timeout,
		now:     clock,
	}
}

// Check executes `shift --version` and caches the latest result.
func (s *ShiftChecker) Check(ctx context.Context) ComponentStatus {
	if s == nil || s.runner == nil {
		return ComponentStatus{State: stateUnknown, CheckedAt: time.Now().UTC(), Message: "shift runner unavailable"}
	}

	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	stdout, stderr, err := s.runner.Run(checkCtx, s.binary, "--version")
	status := ComponentStatus{
		State:     stateOK,
		Message:   strings.TrimSpace(stdout),
		Version:   extractFirstLine(stdout),
		CheckedAt: s.now(),
	}
	if err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound):
			status.State = stateError
			status.Message = fmt.Sprintf("%s not found", s.binary)
		default:
			status.State = stateError
			if strings.TrimSpace(stderr) != "" {
				status.Message = strings.TrimSpace(stderr)
			} else {
				status.Message = err.Error()
			}
		}
	}
	s.cache.status = status
	return status
}

// IPFSChecker validates IPFS Cluster health using the REST API.
type IPFSChecker struct {
	baseURL   string
	authToken string
	username  string
	password  string
	client    *http.Client
	timeout   time.Duration
	now       func() time.Time
}

// IPFSCheckerOptions configure the IPFS health checker.
type IPFSCheckerOptions struct {
	BaseURL    string
	AuthToken  string
	Username   string
	Password   string
	HTTPClient *http.Client
	Timeout    time.Duration
	Clock      func() time.Time
}

// NewIPFSChecker constructs an IPFS checker hitting `/health`.
func NewIPFSChecker(opts IPFSCheckerOptions) *IPFSChecker {
	base := strings.TrimSpace(opts.BaseURL)
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 4 * time.Second}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &IPFSChecker{
		baseURL:   base,
		authToken: strings.TrimSpace(opts.AuthToken),
		username:  strings.TrimSpace(opts.Username),
		password:  strings.TrimSpace(opts.Password),
		client:    client,
		timeout:   timeout,
		now:       clock,
	}
}

// Check issues a health request to the IPFS Cluster endpoint.
func (i *IPFSChecker) Check(ctx context.Context) ComponentStatus {
	if i == nil || strings.TrimSpace(i.baseURL) == "" {
		return ComponentStatus{State: stateUnknown, CheckedAt: time.Now().UTC(), Message: "ipfs cluster endpoint not configured"}
	}
	healthURL, err := i.resolve("/health")
	status := ComponentStatus{
		State:     stateOK,
		CheckedAt: i.now(),
		Details: map[string]any{
			"endpoint": healthURL,
		},
	}
	if err != nil {
		status.State = stateError
		status.Message = err.Error()
		return status
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		status.State = stateError
		status.Message = err.Error()
		return status
	}
	if i.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+i.authToken)
	} else if i.username != "" || i.password != "" {
		req.SetBasicAuth(i.username, i.password)
	}

	checkCtx, cancel := context.WithTimeout(req.Context(), i.timeout)
	defer cancel()
	req = req.WithContext(checkCtx)

	resp, err := i.client.Do(req)
	if err != nil {
		status.State = stateError
		status.Message = err.Error()
		return status
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		status.State = stateError
		status.Message = fmt.Sprintf("status %d", resp.StatusCode)
		status.Details["response"] = strings.TrimSpace(string(body))
		return status
	}
	status.Message = strings.TrimSpace(string(body))
	return status
}

func (i *IPFSChecker) resolve(path string) (string, error) {
	base, err := url.Parse(i.baseURL)
	if err != nil {
		return "", err
	}
	relative := &url.URL{Path: path}
	return base.ResolveReference(relative).String(), nil
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
