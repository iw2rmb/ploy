package step

import (
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

type stackMatchOptions struct {
	includeEvidence                 bool
	requireDetectedToolForToolMatch bool
}

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
	return matchStackWithOptions(obs, expect, stackMatchOptions{
		includeEvidence:                 true,
		requireDetectedToolForToolMatch: false,
	})
}

func matchStackForStackDetectConfig(obs *stackdetect.Observation, expect *contracts.StackExpectation) (bool, string) {
	return matchStackWithOptions(obs, expect, stackMatchOptions{
		includeEvidence:                 false,
		requireDetectedToolForToolMatch: true,
	})
}

func matchStackWithOptions(obs *stackdetect.Observation, expect *contracts.StackExpectation, opts stackMatchOptions) (bool, string) {
	if expect == nil {
		return true, ""
	}
	if obs == nil {
		return false, "stack mismatch: detected stack is unavailable"
	}
	obsRelease := ""
	if obs.Release != nil {
		obsRelease = *obs.Release
	}
	languageMismatch := expect.Language != "" &&
		!contracts.StackFieldsMatch(obs.Language, "", "", expect.Language, "", "")
	toolMismatch := expect.Tool != ""
	if opts.requireDetectedToolForToolMatch && strings.TrimSpace(obs.Tool) == "" {
		toolMismatch = false
	} else {
		toolMismatch = toolMismatch && !contracts.StackFieldsMatch("", obs.Tool, "", "", expect.Tool, "")
	}
	releaseMismatch := expect.Release != "" &&
		(obs.Release == nil || !contracts.StackFieldsMatch("", "", obsRelease, "", "", expect.Release))

	if !languageMismatch && !toolMismatch && !releaseMismatch {
		return true, ""
	}

	// Build detailed mismatch report for diagnostics.
	var mismatches []string
	if languageMismatch {
		mismatches = append(mismatches, fmt.Sprintf("language: expected %q, detected %q", expect.Language, obs.Language))
	}
	if toolMismatch {
		mismatches = append(mismatches, fmt.Sprintf("tool: expected %q, detected %q", expect.Tool, obs.Tool))
	}
	if releaseMismatch {
		detected := "<nil>"
		if obs.Release != nil {
			detected = *obs.Release
		}
		mismatches = append(mismatches, fmt.Sprintf("release: expected %q, detected %q", expect.Release, detected))
	}
	msg := "stack mismatch: " + strings.Join(mismatches, "; ")
	if opts.includeEvidence && len(obs.Evidence) > 0 {
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
