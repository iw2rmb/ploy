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
	return contracts.NormalizeStackExpectation(exp)
}

// matchStack compares a detected observation against expected values.
// Returns (true, "") if all non-empty expected fields match, or
// (false, reason) with a human-readable mismatch explanation.
func matchStack(obs *stackdetect.Observation, expect *contracts.StackExpectation) (bool, string) {
	languageMismatch := !stackFieldMatches(obs.Language, expect.Language, true)
	toolMismatch := !stackFieldMatches(obs.Tool, expect.Tool, true)
	releaseMismatch := expect.Release != "" &&
		(obs.Release == nil || !stackFieldMatches(*obs.Release, expect.Release, false))

	if !languageMismatch && !toolMismatch && !releaseMismatch {
		return true, ""
	}

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
	return false, "stack mismatch: " + strings.Join(mismatches, "; ")
}

// stackFieldMatches reports whether a detected stack field satisfies the
// expected one. An empty want is a wildcard. Surrounding whitespace is ignored.
// foldCase selects case-insensitive comparison (language and tool names);
// releases compare case-sensitively.
func stackFieldMatches(got, want string, foldCase bool) bool {
	if want == "" {
		return true
	}
	if foldCase {
		got, want = strings.ToLower(got), strings.ToLower(want)
	}
	return strings.TrimSpace(got) == strings.TrimSpace(want)
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
