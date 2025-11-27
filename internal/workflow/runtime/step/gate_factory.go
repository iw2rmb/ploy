// gate_factory.go provides configuration-driven selection between GateExecutor implementations.
//
// This factory enables pluggable gate execution modes:
//   - "local-docker" (default): Uses dockerGateExecutor to run gates locally via container runtime.
//   - "remote-http": Uses HTTPGateExecutor to delegate gates to remote Build Gate workers.
//
// Mode selection is driven by the PLOY_BUILDGATE_MODE environment variable or explicit
// configuration in the node agent. When mode is unset, empty, or unrecognized, the factory
// defaults to local-docker mode for backward compatibility.
//
// ## Usage
//
// In node agent runtime initialization:
//
//	mode := os.Getenv("PLOY_BUILDGATE_MODE")
//	gateExecutor := step.NewGateExecutor(mode, containerRuntime, httpClient)
//
// ## Relationship to Other Gate Files
//
//   - gate_iface.go: Defines the GateExecutor and BuildGateHTTPClient interfaces.
//   - gate_docker.go: Implements dockerGateExecutor for local container-based execution.
//   - gate_http.go: Implements HTTPGateExecutor for remote HTTP-based execution.
//   - gate_http_client.go: Implements BuildGateHTTPClient for HTTP communication.
package step

import (
	"log/slog"
)

// GateExecutorMode defines the available gate execution modes.
// These values can be set via PLOY_BUILDGATE_MODE environment variable.
const (
	// GateExecutorModeLocalDocker uses the local container runtime to execute gates.
	// This is the default mode when PLOY_BUILDGATE_MODE is unset or empty.
	GateExecutorModeLocalDocker = "local-docker"

	// GateExecutorModeRemoteHTTP delegates gate execution to remote Build Gate workers
	// via the HTTP Build Gate API. Requires a configured BuildGateHTTPClient.
	GateExecutorModeRemoteHTTP = "remote-http"
)

// NewGateExecutor creates a GateExecutor based on the specified mode.
//
// Parameters:
//   - mode: The execution mode. Valid values are "local-docker", "remote-http".
//     Empty string defaults to local-docker for backward compatibility.
//   - rt: ContainerRuntime for local docker execution. Required for local-docker mode.
//   - httpClient: BuildGateHTTPClient for remote HTTP execution. Required for remote-http mode.
//
// Mode selection:
//   - "" or "local-docker": Returns a dockerGateExecutor using the provided ContainerRuntime.
//   - "remote-http": Returns an HTTPGateExecutor using the provided BuildGateHTTPClient.
//   - Any other value: Logs a warning and falls back to local-docker mode.
func NewGateExecutor(mode string, rt ContainerRuntime, httpClient BuildGateHTTPClient) GateExecutor {
	return NewGateExecutorWithLogger(mode, rt, httpClient, nil)
}

// NewGateExecutorWithLogger creates a GateExecutor with a custom logger for HTTP mode.
// The logger is used for structured logging during async job polling in remote-http mode.
// If logger is nil, slog.Default() is used.
//
// See NewGateExecutor for parameter documentation.
func NewGateExecutorWithLogger(mode string, rt ContainerRuntime, httpClient BuildGateHTTPClient, logger *slog.Logger) GateExecutor {
	if logger == nil {
		logger = slog.Default()
	}

	switch mode {
	case GateExecutorModeRemoteHTTP:
		// Remote HTTP mode: delegate to Build Gate workers via HTTP API.
		if httpClient == nil {
			logger.Warn("remote-http gate mode requested but httpClient is nil, falling back to local-docker")
			return NewDockerGateExecutor(rt)
		}
		logger.Info("using remote-http gate executor")
		return NewHTTPGateExecutorWithLogger(httpClient, logger)

	case GateExecutorModeLocalDocker, "":
		// Local docker mode (default): execute gates via container runtime.
		logger.Debug("using local-docker gate executor", "mode", mode)
		return NewDockerGateExecutor(rt)

	default:
		// Unrecognized mode: log warning and fall back to local-docker.
		logger.Warn("unrecognized gate executor mode, falling back to local-docker",
			"mode", mode,
			"valid_modes", []string{GateExecutorModeLocalDocker, GateExecutorModeRemoteHTTP},
		)
		return NewDockerGateExecutor(rt)
	}
}
