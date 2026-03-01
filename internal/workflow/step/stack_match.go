package step

import (
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

// observationToStackExpectation converts a stackdetect.Observation to a StackExpectation.
func observationToStackExpectation(obs *stackdetect.Observation) *contracts.StackExpectation {
	if obs == nil {
		return nil
	}
	exp := &contracts.StackExpectation{
		Language: obs.Language,
		Tool:     obs.Tool,
	}
	if obs.Release != nil {
		exp.Release = *obs.Release
	}
	return exp
}

// matchStack compares a detected observation against expected values.
// Returns (true, "") if all non-empty expected fields match, or
// (false, reason) with a human-readable mismatch explanation.
func matchStack(obs *stackdetect.Observation, expect *contracts.StackExpectation) (bool, string) {
	if expect == nil {
		return true, ""
	}
	var mismatches []string
	if expect.Language != "" && obs.Language != expect.Language {
		mismatches = append(mismatches, fmt.Sprintf("language: expected %q, detected %q", expect.Language, obs.Language))
	}
	if expect.Tool != "" && obs.Tool != expect.Tool {
		mismatches = append(mismatches, fmt.Sprintf("tool: expected %q, detected %q", expect.Tool, obs.Tool))
	}
	if expect.Release != "" {
		detected := "<nil>"
		if obs.Release != nil {
			detected = *obs.Release
		}
		if obs.Release == nil || *obs.Release != expect.Release {
			mismatches = append(mismatches, fmt.Sprintf("release: expected %q, detected %q", expect.Release, detected))
		}
	}
	if len(mismatches) == 0 {
		return true, ""
	}
	msg := "stack mismatch: " + strings.Join(mismatches, "; ")
	if len(obs.Evidence) > 0 {
		msg += "\nevidence:"
		for _, e := range obs.Evidence {
			msg += fmt.Sprintf("\n  - %s: %s = %q", e.Path, e.Key, e.Value)
		}
	}
	return false, msg
}

// formatEvidenceForLog formats evidence items for the LogFinding.Evidence field.
func formatEvidenceForLog(evidence []stackdetect.EvidenceItem) string {
	if len(evidence) == 0 {
		return ""
	}
	var lines []string
	for _, e := range evidence {
		lines = append(lines, fmt.Sprintf("%s: %s = %q", e.Path, e.Key, e.Value))
	}
	return strings.Join(lines, "\n")
}
