package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// JobStatsPayload is the typed structure for the stats field in job completion.
// This replaces untyped map[string]any decoding at the API boundary, providing
// schema control over incoming stats payloads.
//
// Wire format example:
//
//	{
//	  "job_meta": { "kind": "gate", "gate": { ... } },
//	  "metadata": { "mr_url": "https://..." },
//	  "duration_ms": 1234
//	}
//
// The job_meta field, when present, must be valid per contracts.UnmarshalJobMeta.
// The metadata field contains string key-value pairs for run-level metadata merging.
type JobStatsPayload struct {
	// JobMeta is the structured gate/build/mig metadata to persist in jobs.meta JSONB.
	// When present, it is validated via contracts.UnmarshalJobMeta before persisting.
	// Empty/null values are treated as "no job meta" (not persisted).
	JobMeta json.RawMessage `json:"job_meta,omitempty"`

	// Metadata contains optional string key-value pairs for run-level context.
	// The mr_url key is used by MR jobs to report merge request URLs.
	Metadata map[string]string `json:"metadata,omitempty"`

	// DurationMs is the job execution duration in milliseconds (informational).
	DurationMs int64 `json:"duration_ms,omitempty"`

	// JobResources carries per-job container resource consumption metrics.
	// When present, the handler persists a row in ploy.job_metrics.
	JobResources *JobResourcesPayload `json:"job_resources,omitempty"`
}

// JobResourcesPayload contains per-job container resource consumption metrics.
type JobResourcesPayload struct {
	CPUConsumedNs     int64 `json:"cpu_consumed_ns,omitempty"`
	DiskConsumedBytes int64 `json:"disk_consumed_bytes,omitempty"`
	MemConsumedBytes  int64 `json:"mem_consumed_bytes,omitempty"`
}

// MRURL returns the merge request URL from metadata, if present.
// Returns empty string if metadata is nil or mr_url key is absent/empty.
func (p JobStatsPayload) MRURL() string {
	if p.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(p.Metadata["mr_url"])
}

// HasJobMeta returns true if job_meta is present and non-empty.
// Empty JSON objects ("{}") and null are treated as "no job meta".
func (p JobStatsPayload) HasJobMeta() bool {
	trimmed := bytes.TrimSpace(p.JobMeta)
	if len(trimmed) == 0 {
		return false
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	if bytes.Equal(trimmed, []byte("{}")) {
		return false
	}

	// Treat any empty object form as "no job meta", even if whitespace is present ("{ }").
	// This keeps the API forgiving for clients that emit pretty-printed JSON.
	if trimmed[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err == nil && len(obj) == 0 {
			return false
		}
	}
	return true
}

// HasJobResources returns true if job_resources is present.
func (p JobStatsPayload) HasJobResources() bool {
	return p.JobResources != nil
}

// ValidateJobResources validates non-negative job resource values.
func (p JobStatsPayload) ValidateJobResources() error {
	if p.JobResources == nil {
		return nil
	}
	if p.JobResources.CPUConsumedNs < 0 {
		return fmt.Errorf("invalid job_resources.cpu_consumed_ns: must be non-negative")
	}
	if p.JobResources.DiskConsumedBytes < 0 {
		return fmt.Errorf("invalid job_resources.disk_consumed_bytes: must be non-negative")
	}
	if p.JobResources.MemConsumedBytes < 0 {
		return fmt.Errorf("invalid job_resources.mem_consumed_bytes: must be non-negative")
	}
	return nil
}

// ValidateJobMeta validates the job_meta field using contracts.UnmarshalJobMeta.
// Returns nil if job_meta is absent/empty or if it passes validation.
// Returns an error describing the validation failure if job_meta is invalid.
func (p JobStatsPayload) ValidateJobMeta() error {
	trimmed := bytes.TrimSpace(p.JobMeta)
	if len(trimmed) == 0 {
		return nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	// Treat any empty object form as "no job meta", even if whitespace is present ("{ }").
	if trimmed[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err == nil && len(obj) == 0 {
			return nil
		}
	}

	// Use the canonical JobMeta unmarshaler for structural validation.
	// This ensures the job_meta adheres to the contracts.JobMeta schema
	// (valid kind, consistent gate/build metadata presence, etc.).
	if _, err := contracts.UnmarshalJobMeta(trimmed); err != nil {
		return fmt.Errorf("invalid job_meta: %w", err)
	}
	return nil
}

// formatStackGateError formats a Stack Gate failure for run_repos.last_error.
// Returns nil if job meta doesn't contain a stack gate failure.
func formatStackGateError(jobType domaintypes.JobType, jobMeta json.RawMessage) *string {
	if len(jobMeta) == 0 {
		return nil
	}
	meta, err := contracts.UnmarshalJobMeta(jobMeta)
	if err != nil || meta.Kind != "gate" || meta.GateMetadata == nil || meta.GateMetadata.StackGate == nil {
		return nil
	}
	sg := meta.GateMetadata.StackGate
	if sg.Result == "pass" {
		return nil
	}

	// Derive phase from job_type
	phase := "unknown"
	switch jobType {
	case domaintypes.JobTypePreGate:
		phase = "inbound"
	case domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		phase = "outbound"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Stack Gate [%s]: %s\n", phase, sg.Result))

	if sg.Expected != nil {
		sb.WriteString(fmt.Sprintf("  Expected: {language: %s", sg.Expected.Language))
		if sg.Expected.Tool != "" {
			sb.WriteString(fmt.Sprintf(", tool: %s", sg.Expected.Tool))
		}
		if sg.Expected.Release != "" {
			sb.WriteString(fmt.Sprintf(", release: %q", sg.Expected.Release))
		}
		sb.WriteString("}\n")
	}

	if sg.Detected != nil {
		sb.WriteString(fmt.Sprintf("  Detected: {language: %s", sg.Detected.Language))
		if sg.Detected.Tool != "" {
			sb.WriteString(fmt.Sprintf(", tool: %s", sg.Detected.Tool))
		}
		if sg.Detected.Release != "" {
			sb.WriteString(fmt.Sprintf(", release: %q", sg.Detected.Release))
		}
		sb.WriteString("}\n")
	}

	// Extract evidence from LogFindings
	if meta.GateMetadata != nil && len(meta.GateMetadata.LogFindings) > 0 {
		for _, finding := range meta.GateMetadata.LogFindings {
			if finding.Evidence != "" {
				sb.WriteString("  Evidence:\n")
				for _, line := range strings.Split(finding.Evidence, "\n") {
					if line = strings.TrimSpace(line); line != "" {
						sb.WriteString(fmt.Sprintf("    - %s\n", line))
					}
				}
				break
			}
		}
	}

	result := strings.TrimSuffix(sb.String(), "\n")
	return &result
}

// formatExit137Error formats a deterministic run_repos.last_error message for
// jobs that exited with code 137 (typically SIGKILL/OOM kill).
func formatExit137Error(jobName string, exitCode *int32) *string {
	if exitCode == nil || *exitCode != 137 {
		return nil
	}
	name := strings.TrimSpace(jobName)
	if name == "" {
		msg := "job failed with exit code 137 (killed; likely out of memory)"
		return &msg
	}
	msg := fmt.Sprintf("job %s failed with exit code 137 (killed; likely out of memory)", name)
	return &msg
}
