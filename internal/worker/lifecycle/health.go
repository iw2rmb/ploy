package lifecycle

import (
	"context"
	"strings"
	"time"

	// Docker Engine v29 SDK module (moby). This replaces the deprecated
	// github.com/docker/docker imports with supported Engine v29 equivalents.
	// The client package provides PingResult, PingOptions, SystemInfoResult,
	// and InfoOptions used by DockerChecker for health checks.
	"github.com/moby/moby/client"
)

// DockerChecker probes the Docker Engine for availability and version info.
// It uses the moby Engine v29 SDK (github.com/moby/moby/client) for Ping and
// Info calls to determine daemon availability and running container state.
//
// # Engine v29 Ping/Info Field Reconciliation
//
// The checker exposes a stable set of Details keys across Engine v28 and v29:
//
//	api_version        - from PingResult.APIVersion (e.g., "1.44", "1.45")
//	os_type            - from PingResult.OSType (e.g., "linux", "windows")
//	containers_running - from system.Info.ContainersRunning
//	driver             - from system.Info.Driver (e.g., "overlay2")
//
// ComponentStatus.Version is set from system.Info.ServerVersion, which returns
// the Docker daemon version (e.g., "27.0.0", "29.0.0").
//
// These fields are stable across Docker Engine v28.x and v29.x. The moby SDK
// types (client.PingResult, system.Info) have not changed the field names or
// semantics for these core fields between versions.
type DockerChecker struct {
	client  dockerAPI
	timeout time.Duration
	now     func() time.Time
}

// dockerAPI abstracts the Docker client methods used for health checks.
// The interface matches the moby Engine v29 SDK method signatures:
//   - Ping returns PingResult with APIVersion, OSType, Experimental fields.
//   - Info returns SystemInfoResult wrapping system.Info with ServerVersion,
//     ContainersRunning, Driver, and other daemon state.
type dockerAPI interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	Info(ctx context.Context, options client.InfoOptions) (client.SystemInfoResult, error)
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
		cliOpts := []client.Opt{client.FromEnv}
		if trimmed := strings.TrimSpace(opts.Host); trimmed != "" {
			cliOpts = append(cliOpts, client.WithHost(trimmed))
		}
		cli, err := client.New(cliOpts...)
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
// Uses the moby Engine v29 SDK for daemon health checks:
//   - Ping verifies daemon connectivity and retrieves API version.
//   - Info retrieves server version, container count, and storage driver.
//
// ComponentStatus.State is set to OK, Degraded, or Error based on results.
//
// # Stable Details Keys (Engine v28/v29)
//
// The following Details keys are populated and remain stable across versions:
//
//	api_version        - Docker API version from Ping (string, e.g., "1.44")
//	os_type            - OS type from Ping (string, "linux" or "windows")
//	containers_running - Running container count from Info (int)
//	driver             - Storage driver from Info (string, e.g., "overlay2")
//
// Version is set from system.Info.ServerVersion (e.g., "29.0.0").
func (c *DockerChecker) Check(ctx context.Context) ComponentStatus {
	if c == nil || c.client == nil {
		return ComponentStatus{State: stateUnknown, CheckedAt: time.Now().UTC(), Message: "docker client unavailable"}
	}
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Moby Engine v29 SDK uses client.PingOptions{} for the ping call.
	// PingResult fields used:
	//   - APIVersion: API version string (e.g., "1.44", "1.45")
	//   - OSType: OS type string ("linux" or "windows")
	// These fields are stable across Engine v28.x and v29.x.
	ping, pingErr := c.client.Ping(checkCtx, client.PingOptions{})
	status := ComponentStatus{
		State:     stateOK,
		CheckedAt: c.now(),
		Details: map[string]any{
			// api_version: stable across Engine v28/v29; identifies negotiated API level.
			"api_version": ping.APIVersion,
			// os_type: stable across Engine v28/v29; indicates daemon's host OS.
			"os_type": ping.OSType,
		},
	}
	if pingErr != nil {
		status.State = stateError
		status.Message = pingErr.Error()
		return status
	}
	// Moby Engine v29 SDK uses client.InfoOptions{} for the info call.
	// SystemInfoResult wraps system.Info in the .Info field.
	// system.Info fields used:
	//   - ServerVersion: daemon version (e.g., "27.0.0", "29.0.0")
	//   - ContainersRunning: count of running containers
	//   - Driver: storage driver name (e.g., "overlay2", "vfs")
	// These fields are stable across Engine v28.x and v29.x.
	infoResult, infoErr := c.client.Info(checkCtx, client.InfoOptions{})
	if infoErr != nil {
		status.State = stateDegraded
		status.Message = infoErr.Error()
		return status
	}
	// Extract daemon state from system.Info inside SystemInfoResult.
	info := infoResult.Info
	// Version: stable field; identifies Docker daemon release version.
	status.Version = strings.TrimSpace(info.ServerVersion)
	// containers_running: stable field; count of running containers.
	status.Details["containers_running"] = info.ContainersRunning
	// driver: stable field; storage driver in use (overlay2, vfs, etc.).
	status.Details["driver"] = strings.TrimSpace(info.Driver)
	return status
}

func extractFirstLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
