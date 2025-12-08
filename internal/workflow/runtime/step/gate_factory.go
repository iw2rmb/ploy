// gate_factory.go provides factory functions for creating GateExecutor instances.
//
// Gate execution always uses the Docker-based executor (dockerGateExecutor) which
// runs validation containers locally via the container runtime. The factory exists
// to maintain a clean construction API and support future extensibility.
//
// ## Usage
//
// In node agent runtime initialization:
//
//	gateExecutor := step.NewGateExecutor(containerRuntime)
//
// ## Relationship to Other Gate Files
//
//   - gate_iface.go: Defines the GateExecutor interface.
//   - gate_docker.go: Implements dockerGateExecutor for local container-based execution.
package step

import (
	"log/slog"
)

// GateExecutorModeLocalDocker is the only supported gate execution mode.
// Gates run locally via the container runtime.
const GateExecutorModeLocalDocker = "local-docker"

// NewGateExecutor creates a GateExecutor using the local Docker-based executor.
//
// Parameters:
//   - rt: ContainerRuntime for local docker execution. If nil, the executor
//     returns empty metadata for enabled specs (graceful degradation).
//
// The mode parameter is accepted for backward compatibility but ignored;
// all gate execution uses the Docker-based executor.
func NewGateExecutor(mode string, rt ContainerRuntime) GateExecutor {
	return NewGateExecutorWithLogger(mode, rt, nil)
}

// NewGateExecutorWithLogger creates a GateExecutor with an optional logger.
// The logger is used for debug logging during executor creation.
//
// See NewGateExecutor for parameter documentation.
func NewGateExecutorWithLogger(mode string, rt ContainerRuntime, logger *slog.Logger) GateExecutor {
	if logger == nil {
		logger = slog.Default()
	}

	// Log mode for observability (all modes resolve to Docker executor).
	if mode != "" && mode != GateExecutorModeLocalDocker {
		logger.Warn("unrecognized gate executor mode, using local-docker",
			"mode", mode,
		)
	} else {
		logger.Debug("using local-docker gate executor", "mode", mode)
	}

	// Always return Docker-based executor.
	return NewDockerGateExecutor(rt)
}
