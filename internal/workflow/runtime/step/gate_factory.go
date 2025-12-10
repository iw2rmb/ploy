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

// NewGateExecutor creates a GateExecutor using the local Docker-based executor.
//
// Parameters:
//   - rt: ContainerRuntime for local docker execution. If nil, the executor
//     returns empty metadata for enabled specs (graceful degradation).
func NewGateExecutor(rt ContainerRuntime) GateExecutor {
	return NewGateExecutorWithLogger(rt, nil)
}

// NewGateExecutorWithLogger creates a GateExecutor with an optional logger.
// The logger parameter is currently unused; gate execution always uses the
// Docker-based executor.
//
// See NewGateExecutor for parameter documentation.
func NewGateExecutorWithLogger(rt ContainerRuntime, logger any) GateExecutor {
	return NewDockerGateExecutor(rt)
}
