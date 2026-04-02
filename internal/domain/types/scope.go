package types

import (
	"fmt"
	"strings"
)

// GlobalEnvTarget identifies the injection target for a global environment variable.
// It determines which components or job types receive the env var.
//
// Known persisted values:
//   - GlobalEnvTargetServer: Inject into the server process
//   - GlobalEnvTargetNodes: Inject into node agent processes
//   - GlobalEnvTargetGates: Inject into gate jobs (pre_gate, re_gate, post_gate)
//   - GlobalEnvTargetSteps: Inject into step jobs (mig, heal)
//
// Unknown values should be rejected at API boundaries using Validate() or ParseGlobalEnvTarget().
// Empty values are rejected (no default target).
type GlobalEnvTarget string

const (
	// GlobalEnvTargetServer injects into the server process.
	GlobalEnvTargetServer GlobalEnvTarget = "server"
	// GlobalEnvTargetNodes injects into node agent processes.
	GlobalEnvTargetNodes GlobalEnvTarget = "nodes"
	// GlobalEnvTargetGates injects into gate jobs (pre_gate, re_gate, post_gate).
	GlobalEnvTargetGates GlobalEnvTarget = "gates"
	// GlobalEnvTargetSteps injects into step jobs (mig, heal).
	GlobalEnvTargetSteps GlobalEnvTarget = "steps"
)

// String returns the underlying string value.
func (t GlobalEnvTarget) String() string { return string(t) }

// IsZero reports whether the value is empty (after trimming spaces).
func (t GlobalEnvTarget) IsZero() bool { return IsEmpty(string(t)) }

// Validate ensures the value is one of the known GlobalEnvTarget constants.
// Returns an error for unknown or empty values.
func (t GlobalEnvTarget) Validate() error {
	normalized := GlobalEnvTarget(strings.TrimSpace(string(t)))
	switch normalized {
	case GlobalEnvTargetServer, GlobalEnvTargetNodes, GlobalEnvTargetGates, GlobalEnvTargetSteps:
		return nil
	default:
		if normalized == "" {
			return fmt.Errorf("target is required")
		}
		return fmt.Errorf("invalid target %q (must be one of: server, nodes, gates, steps)", t)
	}
}

// ParseGlobalEnvTarget parses a string into a GlobalEnvTarget, returning an error
// if the value is not one of the known constants. Empty strings are rejected.
func ParseGlobalEnvTarget(s string) (GlobalEnvTarget, error) {
	normalized := strings.TrimSpace(s)
	if normalized == "" {
		return "", fmt.Errorf("target is required")
	}
	target := GlobalEnvTarget(normalized)
	if err := target.Validate(); err != nil {
		return "", err
	}
	return target, nil
}

// MatchesJobType determines whether this target applies to the given job type (JobType).
// This is the core target-matching logic for global env var injection.
//
// Target semantics:
//   - "gates" → inject into pre_gate, re_gate, and post_gate jobs
//   - "steps" → inject into mig and heal jobs
//   - "server" / "nodes" → not job-routed (returns false)
func (t GlobalEnvTarget) MatchesJobType(jobType JobType) bool {
	switch t {
	case GlobalEnvTargetGates:
		return jobType == JobTypePreGate || jobType == JobTypeReGate || jobType == JobTypePostGate
	case GlobalEnvTargetSteps:
		return jobType == JobTypeMig || jobType == JobTypeHeal
	default:
		return false
	}
}
