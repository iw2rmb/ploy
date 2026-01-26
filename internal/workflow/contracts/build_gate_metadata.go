package contracts

import (
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// BuildGateStageMetadata captures build gate metadata published with checkpoints.
type BuildGateStageMetadata struct {
	LogDigest    types.Sha256Digest           `json:"log_digest,omitempty"`
	StaticChecks []BuildGateStaticCheckReport `json:"static_checks,omitempty"`
	LogFindings  []BuildGateLogFinding        `json:"log_findings,omitempty"`
	// RuntimeImage is the container image name used to run the gate container.
	// Not serialized in JSON APIs.
	RuntimeImage string `json:"-"`
	// StackGate captures the outcome of Stack Gate pre-check validation.
	// Present only when Stack Gate mode is enabled.
	StackGate *StackGateResult `json:"stack_gate,omitempty"`
	// LogsText carries the raw build logs text for node-local processing.
	// Not serialized in JSON APIs.
	LogsText string `json:"-"`
	// Resources summarizes container limits and observed usage for the gate run.
	// Not serialized in JSON APIs.
	Resources *BuildGateResourceUsage `json:"-"`
}

// DetectedStack returns the ModStack derived from the first static check's tool.
// This provides deterministic stack identification for stack-aware image selection
// in Mods steps and healing jobs.
//
// The detected stack is derived from the Build Gate's tool detection:
//   - "maven" tool → ModStackJavaMaven
//   - "gradle" tool → ModStackJavaGradle
//   - "java" tool → ModStackJava
//   - unknown/empty → ModStackUnknown
//
// This method ensures the same stack value is visible to both mod and healing
// executions, enabling consistent image resolution across re-gates.
func (m BuildGateStageMetadata) DetectedStack() ModStack {
	if len(m.StaticChecks) == 0 {
		return ModStackUnknown
	}
	return ToolToModStack(m.StaticChecks[0].Tool)
}

// Validate ensures build gate metadata entries are well formed.
func (m BuildGateStageMetadata) Validate() error {
	for i, check := range m.StaticChecks {
		if err := check.Validate(); err != nil {
			return fmt.Errorf("static check %d invalid: %w", i, err)
		}
	}
	for i, finding := range m.LogFindings {
		if err := finding.Validate(); err != nil {
			return fmt.Errorf("log finding %d invalid: %w", i, err)
		}
	}
	if m.StackGate != nil {
		if err := m.StackGate.Validate(); err != nil {
			return fmt.Errorf("stack_gate invalid: %w", err)
		}
	}
	return nil
}

// BuildGateResourceUsage captures container limits and observed usage metrics
// from the gate execution container.
type BuildGateResourceUsage struct {
	// Limits configured for the container (0 means unlimited/not set).
	LimitNanoCPUs    int64 `json:"limit_nano_cpus"`
	LimitMemoryBytes int64 `json:"limit_memory_bytes"`

	// Observed usage during the container lifetime.
	CPUTotalNs      uint64 `json:"cpu_total_ns"`
	MemUsageBytes   uint64 `json:"mem_usage_bytes"`
	MemMaxBytes     uint64 `json:"mem_max_bytes"`
	BlkioReadBytes  uint64 `json:"blkio_read_bytes"`
	BlkioWriteBytes uint64 `json:"blkio_write_bytes"`
	SizeRwBytes     *int64 `json:"size_rw_bytes,omitempty"`
}

// BuildGateStaticCheckReport summarises an individual static analysis invocation.
type BuildGateStaticCheckReport struct {
	Language string                        `json:"language,omitempty"`
	Tool     string                        `json:"tool"`
	Passed   bool                          `json:"passed"`
	Failures []BuildGateStaticCheckFailure `json:"failures,omitempty"`
}

// Validate ensures the static check report is well formed.
func (r BuildGateStaticCheckReport) Validate() error {
	if strings.TrimSpace(r.Tool) == "" {
		return fmt.Errorf("tool is required")
	}
	for i, failure := range r.Failures {
		if err := failure.Validate(); err != nil {
			return fmt.Errorf("failure %d invalid: %w", i, err)
		}
	}
	return nil
}

// BuildGateStaticCheckFailure captures a single diagnostic from a static check tool.
type BuildGateStaticCheckFailure struct {
	RuleID   string `json:"rule_id,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Validate ensures static check failure entries include required details.
func (f BuildGateStaticCheckFailure) Validate() error {
	if strings.TrimSpace(f.Message) == "" {
		return fmt.Errorf("message is required")
	}
	if strings.TrimSpace(f.Severity) == "" {
		return fmt.Errorf("severity is required")
	}
	if f.Line < 0 {
		return fmt.Errorf("line cannot be negative")
	}
	if f.Column < 0 {
		return fmt.Errorf("column cannot be negative")
	}
	return nil
}

// BuildGateLogFinding records a normalized build log finding used for guidance.
type BuildGateLogFinding struct {
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

// Validate ensures log finding entries include required guidance details.
func (f BuildGateLogFinding) Validate() error {
	if strings.TrimSpace(f.Message) == "" {
		return fmt.Errorf("message is required")
	}
	if strings.TrimSpace(f.Severity) == "" {
		return fmt.Errorf("severity is required")
	}
	return nil
}

// StackGateResult captures the outcome of Stack Gate pre-check validation.
type StackGateResult struct {
	Enabled      bool              `json:"enabled"`
	Expected     *StackExpectation `json:"expected,omitempty"`
	Detected     *StackExpectation `json:"detected,omitempty"`
	RuntimeImage string            `json:"runtime_image,omitempty"`
	Result       string            `json:"result,omitempty"` // "pass", "mismatch", "unknown"
	Reason       string            `json:"reason,omitempty"`
}

// Validate ensures stack gate result is well formed.
func (r StackGateResult) Validate() error {
	if !r.Enabled {
		return nil
	}
	switch r.Result {
	case "pass", "mismatch", "unknown":
		// Valid results.
	case "":
		return fmt.Errorf("stack_gate.result required when enabled")
	default:
		return fmt.Errorf("stack_gate.result invalid: %q", r.Result)
	}
	return nil
}
