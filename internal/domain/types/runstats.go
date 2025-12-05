package types

import (
	"fmt"
	"strings"
)

// RunStats represents the terminal statistics payload stored on a run.
//
// It is intentionally kept as a map-based type to preserve flexibility of the
// JSON schema while giving callers a distinct type and small helpers for
// common fields.
type RunStats map[string]any

// ExitCode returns the exit_code field as an int when present.
// It accepts int, int64, and float64 (from JSON decoding) representations.
func (s RunStats) ExitCode() (int, bool) {
	v, ok := s["exit_code"]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// Metadata returns a shallow copy of the metadata field interpreted as
// map[string]string. Non-string values are ignored.
func (s RunStats) Metadata() map[string]string {
	out := map[string]string{}
	raw, ok := s["metadata"]
	if !ok || raw == nil {
		return out
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for k, v := range m {
		str, ok := v.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(str); trimmed != "" {
			out[k] = trimmed
		}
	}
	return out
}

// MRURL returns the mr_url entry from the metadata map when present.
func (s RunStats) MRURL() string {
	meta := s.Metadata()
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta["mr_url"])
}

// ResumeCount returns the number of times this run has been resumed.
// Returns 0 if never resumed. Accepts int, int64, float64 representations.
func (s RunStats) ResumeCount() int {
	v, ok := s["resume_count"]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// LastResumedAt returns the RFC3339 timestamp of the last resume, or empty string if never resumed.
func (s RunStats) LastResumedAt() string {
	v, ok := s["last_resumed_at"]
	if !ok || v == nil {
		return ""
	}
	str, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

// GateSummary extracts build gate execution summary from the gate field.
// Returns a human-readable summary string suitable for CLI/API display.
// Format: "passed duration=123ms" or "failed pre-gate duration=45ms" or empty if no gate data.
//
// Priority order:
//  1. final_gate — The latest post-mod gate result. For runs with no mods executed,
//     final_gate is populated from the pre-mod gate to ensure consistent summary output.
//  2. last re-gate — The most recent healing re-gate attempt (from either pre- or post-mod phases).
//  3. pre_gate — The initial pre-mod gate before any mod execution (fallback when no final_gate).
//
// This priority ensures CLI and API consumers always get the most definitive gate result:
// final_gate represents the authoritative build validation status at run completion.
func (s RunStats) GateSummary() string {
	gateRaw, ok := s["gate"]
	if !ok || gateRaw == nil {
		return ""
	}
	gate, ok := gateRaw.(map[string]any)
	if !ok {
		return ""
	}

	// Check final_gate first (post-mod gate or pre-mod gate fallback for runs with no mods).
	if finalGate := extractGatePhase(gate, "final_gate"); finalGate != "" {
		return finalGate
	}

	// Check re_gates array (healing attempts from both pre- and post-mod phases).
	if reGatesRaw, ok := gate["re_gates"]; ok && reGatesRaw != nil {
		if reGates, ok := reGatesRaw.([]any); ok && len(reGates) > 0 {
			// Take the last re-gate run as the most recent healing result.
			if lastReGate, ok := reGates[len(reGates)-1].(map[string]any); ok {
				if summary := formatGatePhase(lastReGate, "re-gate"); summary != "" {
					return summary
				}
			}
		}
	}

	// Fall back to pre_gate (pre-mod gate) — only reached if no final_gate was populated.
	if preGate := extractGatePhase(gate, "pre_gate"); preGate != "" {
		return preGate
	}

	return ""
}

// extractGatePhase pulls a named gate phase from the gate map and formats it.
func extractGatePhase(gate map[string]any, phase string) string {
	phaseRaw, ok := gate[phase]
	if !ok || phaseRaw == nil {
		return ""
	}
	phaseMap, ok := phaseRaw.(map[string]any)
	if !ok {
		return ""
	}
	// For pre_gate and final_gate, use the phase name; for re-gate, caller supplies label.
	label := phase
	switch phase {
	case "pre_gate":
		label = "pre-gate"
	case "final_gate":
		label = "final-gate"
	}
	return formatGatePhase(phaseMap, label)
}

// formatGatePhase builds a summary string from a gate phase map.
// Format: "passed duration=123ms" or "failed pre-gate duration=45ms".
func formatGatePhase(phaseMap map[string]any, label string) string {
	passed, passedOK := phaseMap["passed"].(bool)
	durationRaw, durationOK := phaseMap["duration_ms"]

	if !passedOK {
		return ""
	}

	var durationMs int64
	if durationOK && durationRaw != nil {
		switch d := durationRaw.(type) {
		case int:
			durationMs = int64(d)
		case int64:
			durationMs = d
		case float64:
			durationMs = int64(d)
		}
	}

	status := "passed"
	if !passed {
		status = "failed"
	}

	// Include phase label only for failed gates or non-final phases for clarity.
	if !passed || label != "final-gate" {
		return fmt.Sprintf("%s %s duration=%dms", status, label, durationMs)
	}
	return fmt.Sprintf("%s duration=%dms", status, durationMs)
}
