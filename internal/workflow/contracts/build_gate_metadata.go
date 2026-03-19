package contracts

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// BuildGateStageMetadata captures build gate metadata published with checkpoints.
type BuildGateStageMetadata struct {
	LogDigest    types.Sha256Digest           `json:"log_digest,omitempty"`
	StaticChecks []BuildGateStaticCheckReport `json:"static_checks,omitempty"`
	// Detected captures the resolved gate stack identity used for this
	// gate execution. It is the canonical source for stack-aware recovery
	// validation, including optional release matching.
	Detected    *StackExpectation     `json:"detected_stack,omitempty"`
	LogFindings []BuildGateLogFinding `json:"log_findings,omitempty"`
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
	// BugSummary is a short one-line description of the build failure produced
	// by the router container. Max 200 chars, no newlines.
	BugSummary string `json:"bug_summary,omitempty"`
	// Recovery carries loop context for the universal recovery loop contract.
	Recovery *BuildGateRecoveryMetadata `json:"recovery,omitempty"`
	// Skip captures claim-time gate skip decision details.
	Skip *BuildGateSkipMetadata `json:"skip,omitempty"`
	// ReportLinks captures uploaded gate report artifact references, including
	// per-report artifact links that can be surfaced by API clients.
	ReportLinks []BuildGateReportLink `json:"report_links,omitempty"`
}

// BuildGateSkipMetadata records why a gate execution was skipped.
type BuildGateSkipMetadata struct {
	Enabled         bool   `json:"enabled"`
	SourceProfileID int64  `json:"source_profile_id,omitempty"`
	MatchedTarget   string `json:"matched_target,omitempty"`
}

const (
	BuildGateReportTypeGradleJUnitXML = "gradle_junit_xml"
	BuildGateReportTypeGradleHTML     = "gradle_html"
)

// BuildGateReportLink points to an uploaded gate report artifact bundle.
type BuildGateReportLink struct {
	Type string `json:"type"`
	// Path is the in-container report path (for example /out/gradle-test-results).
	Path string `json:"path"`
	// ArtifactID is the artifact bundle identifier returned by artifact upload.
	ArtifactID string `json:"artifact_id"`
	// BundleCID is the immutable content identifier of the uploaded bundle.
	BundleCID string `json:"bundle_cid,omitempty"`
	// URL is an artifact metadata URL (GET /v1/artifacts/{id}).
	URL string `json:"url"`
	// DownloadURL is a direct artifact bundle download URL
	// (GET /v1/artifacts/{id}?download=true).
	DownloadURL string `json:"download_url,omitempty"`
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
// This method ensures the same stack value is visible to both mig and healing
// executions, enabling consistent image resolution across re-gates.
func (m BuildGateStageMetadata) DetectedStack() ModStack {
	if m.Detected != nil && strings.TrimSpace(m.Detected.Tool) != "" {
		return ToolToModStack(m.Detected.Tool)
	}
	if len(m.StaticChecks) == 0 {
		return ModStackUnknown
	}
	return ToolToModStack(m.StaticChecks[0].Tool)
}

// DetectedStackExpectation returns the normalized detected stack expectation.
// For backward compatibility with older metadata payloads, it falls back to
// static_checks[0] when detected_stack is absent.
func (m BuildGateStageMetadata) DetectedStackExpectation() *StackExpectation {
	if m.Detected != nil {
		language := strings.TrimSpace(m.Detected.Language)
		tool := strings.TrimSpace(m.Detected.Tool)
		release := strings.TrimSpace(m.Detected.Release)
		if language == "" || tool == "" {
			return nil
		}
		return &StackExpectation{
			Language: language,
			Tool:     tool,
			Release:  release,
		}
	}
	if len(m.StaticChecks) == 0 {
		return nil
	}
	language := strings.TrimSpace(m.StaticChecks[0].Language)
	tool := strings.TrimSpace(m.StaticChecks[0].Tool)
	if language == "" || tool == "" {
		return nil
	}
	return &StackExpectation{
		Language: language,
		Tool:     tool,
	}
}

// Validate ensures build gate metadata entries are well formed.
func (m BuildGateStageMetadata) Validate() error {
	if m.Detected != nil {
		if strings.TrimSpace(m.Detected.Language) == "" {
			return fmt.Errorf("detected_stack.language: required")
		}
		if strings.TrimSpace(m.Detected.Tool) == "" {
			return fmt.Errorf("detected_stack.tool: required")
		}
	}
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
	if m.BugSummary != "" {
		if strings.ContainsAny(m.BugSummary, "\n\r") {
			return fmt.Errorf("bug_summary: must be single-line")
		}
		if utf8.RuneCountInString(m.BugSummary) > 200 {
			return fmt.Errorf("bug_summary: must be at most 200 characters, got %d", utf8.RuneCountInString(m.BugSummary))
		}
	}
	if m.Recovery != nil {
		if err := m.Recovery.Validate(); err != nil {
			return fmt.Errorf("recovery invalid: %w", err)
		}
	}
	if m.Skip != nil {
		if !m.Skip.Enabled {
			return fmt.Errorf("skip.enabled: must be true when skip metadata is present")
		}
		if m.Skip.SourceProfileID <= 0 {
			return fmt.Errorf("skip.source_profile_id: must be > 0")
		}
		switch strings.TrimSpace(m.Skip.MatchedTarget) {
		case GateProfileTargetBuild, GateProfileTargetUnit, GateProfileTargetAllTests:
		default:
			return fmt.Errorf("skip.matched_target: invalid value %q", m.Skip.MatchedTarget)
		}
	}
	for i, link := range m.ReportLinks {
		if err := link.Validate(); err != nil {
			return fmt.Errorf("report link %d invalid: %w", i, err)
		}
	}
	return nil
}

// BuildGateRecoveryMetadata captures router classification and strategy context
// for a failed gate within the universal recovery loop.
type BuildGateRecoveryMetadata struct {
	LoopKind   string   `json:"loop_kind"`
	ErrorKind  string   `json:"error_kind"`
	StrategyID string   `json:"strategy_id,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	// Expectations carries strategy-specific structured expectations emitted by
	// router classification. Reserved for downstream strategy/artifact handling.
	Expectations json.RawMessage `json:"expectations,omitempty"`
	// DepsBumps carries cumulative dependency bump state for deps healing loops.
	// Values are non-empty versions or nil (meaning dependency disable/remove).
	DepsBumps map[string]*string `json:"deps_bumps,omitempty"`
	// CandidateSchemaID is the declared schema id for infra recovery candidate.
	CandidateSchemaID string `json:"candidate_schema_id,omitempty"`
	// CandidateArtifactPath is the artifact path declared in expectations.
	CandidateArtifactPath string `json:"candidate_artifact_path,omitempty"`
	// CandidateValidationStatus captures whether candidate resolution+validation passed.
	CandidateValidationStatus string `json:"candidate_validation_status,omitempty"`
	// CandidateValidationError captures the validation/load error when status is not valid.
	CandidateValidationError string `json:"candidate_validation_error,omitempty"`
	// CandidateGateProfile stores validated candidate payload used for re-gate override.
	CandidateGateProfile json.RawMessage `json:"candidate_gate_profile,omitempty"`
	// CandidatePromoted reports whether a validated candidate has been promoted
	// into repo gate_profile after successful re-gate completion.
	CandidatePromoted *bool `json:"candidate_promoted,omitempty"`
	// RouterCmd is the exact argv slice used to invoke the router container,
	// e.g. ["amata","run","/in/amata.yaml","--set","error_kind=code"].
	// Present only when the router is amata-mode; nil for direct-Codex routers.
	RouterCmd []string `json:"router_cmd,omitempty"`
}

// RecoveryClaimContext carries typed recovery inputs in node claim responses
// for healing/re-gate jobs. This payload makes recovery execution independent
// from node-local run cache files.
type RecoveryClaimContext struct {
	LoopKind string `json:"loop_kind,omitempty"`
	// SelectedErrorKind is the resolved healing error kind selected by server.
	SelectedErrorKind string `json:"selected_error_kind,omitempty"`
	// DetectedStack is the gate-detected stack used for image resolution.
	DetectedStack ModStack `json:"detected_stack,omitempty"`
	// ResolvedHealingImage is the concrete healing image selected for this chain.
	ResolvedHealingImage string `json:"resolved_healing_image,omitempty"`
	// Expectations carries router/recovery expectations payload.
	Expectations json.RawMessage `json:"expectations,omitempty"`
	// DepsBumps carries cumulative dependency bump state for deps healing.
	DepsBumps map[string]*string `json:"deps_bumps,omitempty"`
	// DepsCompatEndpoint is a stack-prefilled SBOM compatibility endpoint.
	DepsCompatEndpoint string `json:"deps_compat_endpoint,omitempty"`
	// BuildGateLog is the failed gate log snippet intended for /in/build-gate.log.
	BuildGateLog string `json:"build_gate_log,omitempty"`
	// GateProfile carries failed gate profile JSON for infra healing context.
	GateProfile json.RawMessage `json:"gate_profile,omitempty"`
	// GateProfileSchemaJSON carries schema JSON for infra healing context.
	GateProfileSchemaJSON string `json:"gate_profile_schema_json,omitempty"`
}

const (
	RecoveryCandidateStatusMissing     = "missing"
	RecoveryCandidateStatusUnavailable = "unavailable"
	RecoveryCandidateStatusInvalid     = "invalid"
	RecoveryCandidateStatusValid       = "valid"
)

// Validate ensures recovery metadata entries are well formed.
func (m BuildGateRecoveryMetadata) Validate() error {
	if strings.TrimSpace(m.LoopKind) == "" {
		return fmt.Errorf("loop_kind is required")
	}
	if _, ok := ParseRecoveryLoopKind(m.LoopKind); !ok {
		return fmt.Errorf("loop_kind invalid: %q", m.LoopKind)
	}
	if strings.TrimSpace(m.ErrorKind) == "" {
		return fmt.Errorf("error_kind is required")
	}
	if _, ok := ParseRecoveryErrorKind(m.ErrorKind); !ok {
		return fmt.Errorf("error_kind invalid: %q", m.ErrorKind)
	}
	if m.StrategyID != "" {
		if strings.ContainsAny(m.StrategyID, "\n\r") {
			return fmt.Errorf("strategy_id: must be single-line")
		}
		if utf8.RuneCountInString(m.StrategyID) > 200 {
			return fmt.Errorf("strategy_id: must be at most 200 characters, got %d", utf8.RuneCountInString(m.StrategyID))
		}
	}
	if m.Confidence != nil {
		if math.IsNaN(*m.Confidence) || math.IsInf(*m.Confidence, 0) {
			return fmt.Errorf("confidence: must be finite")
		}
		if *m.Confidence < 0 || *m.Confidence > 1 {
			return fmt.Errorf("confidence: must be between 0 and 1, got %v", *m.Confidence)
		}
	}
	if m.Reason != "" {
		if strings.ContainsAny(m.Reason, "\n\r") {
			return fmt.Errorf("reason: must be single-line")
		}
		if utf8.RuneCountInString(m.Reason) > 200 {
			return fmt.Errorf("reason: must be at most 200 characters, got %d", utf8.RuneCountInString(m.Reason))
		}
	}
	if len(m.Expectations) > 0 {
		var raw any
		if err := json.Unmarshal(m.Expectations, &raw); err != nil {
			return fmt.Errorf("expectations: invalid JSON: %w", err)
		}
		switch raw.(type) {
		case map[string]any, []any:
			// allowed
		default:
			return fmt.Errorf("expectations: must be object or array JSON")
		}
	}
	if m.DepsBumps != nil {
		for lib, ver := range m.DepsBumps {
			if strings.TrimSpace(lib) == "" {
				return fmt.Errorf("deps_bumps: key must be non-empty")
			}
			if ver != nil && strings.TrimSpace(*ver) == "" {
				return fmt.Errorf("deps_bumps[%q]: version must be non-empty when present", lib)
			}
		}
	}
	if m.CandidateSchemaID != "" || m.CandidateArtifactPath != "" {
		if err := ValidateGateProfileArtifactContract(
			m.CandidateArtifactPath,
			m.CandidateSchemaID,
			"candidate",
		); err != nil {
			return err
		}
	}
	if m.CandidateValidationStatus != "" {
		switch m.CandidateValidationStatus {
		case RecoveryCandidateStatusMissing, RecoveryCandidateStatusUnavailable, RecoveryCandidateStatusInvalid, RecoveryCandidateStatusValid:
		default:
			return fmt.Errorf("candidate_validation_status invalid: %q", m.CandidateValidationStatus)
		}
	}
	if len(m.CandidateGateProfile) > 0 {
		if !json.Valid(m.CandidateGateProfile) {
			return fmt.Errorf("candidate_gate_profile: invalid JSON")
		}
	}
	if m.CandidateValidationStatus == RecoveryCandidateStatusValid {
		if len(m.CandidateGateProfile) == 0 {
			return fmt.Errorf("candidate_gate_profile: required when candidate_validation_status=%q", RecoveryCandidateStatusValid)
		}
		if strings.TrimSpace(m.CandidateValidationError) != "" {
			return fmt.Errorf("candidate_validation_error: must be empty when candidate_validation_status=%q", RecoveryCandidateStatusValid)
		}
	}
	if m.CandidateValidationStatus != RecoveryCandidateStatusValid && len(m.CandidateGateProfile) > 0 {
		return fmt.Errorf("candidate_gate_profile: forbidden when candidate_validation_status=%q", m.CandidateValidationStatus)
	}
	if m.CandidatePromoted != nil && *m.CandidatePromoted {
		if m.CandidateValidationStatus != RecoveryCandidateStatusValid {
			return fmt.Errorf("candidate_promoted: true requires candidate_validation_status=%q", RecoveryCandidateStatusValid)
		}
		if len(m.CandidateGateProfile) == 0 {
			return fmt.Errorf("candidate_promoted: true requires candidate_gate_profile")
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

// Validate ensures report link entries include required artifact link details.
func (l BuildGateReportLink) Validate() error {
	switch strings.TrimSpace(l.Type) {
	case BuildGateReportTypeGradleJUnitXML, BuildGateReportTypeGradleHTML:
	default:
		return fmt.Errorf("type invalid: %q", l.Type)
	}
	if path := strings.TrimSpace(l.Path); path == "" {
		return fmt.Errorf("path is required")
	} else if !strings.HasPrefix(path, "/out/") {
		return fmt.Errorf("path must start with /out/: %q", l.Path)
	}
	if strings.TrimSpace(l.ArtifactID) == "" {
		return fmt.Errorf("artifact_id is required")
	}
	if strings.TrimSpace(l.URL) == "" {
		return fmt.Errorf("url is required")
	}
	if strings.ContainsAny(l.URL, "\n\r") {
		return fmt.Errorf("url must be single-line")
	}
	if l.DownloadURL != "" && strings.ContainsAny(l.DownloadURL, "\n\r") {
		return fmt.Errorf("download_url must be single-line")
	}
	return nil
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
