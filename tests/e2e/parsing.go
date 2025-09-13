//go:build e2e
// +build e2e

package e2e

import (
	"regexp"
	"strconv"
	"strings"
)

func parseHealingAttempts(output string) []HealingAttempt {
	var attempts []HealingAttempt

	// Simple parsing for healing attempts from output
	lines := strings.Split(output, "\n")

	var currentAttempt *HealingAttempt

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for healing attempt markers
		if strings.Contains(strings.ToLower(line), "healing attempt") {
			if currentAttempt != nil {
				attempts = append(attempts, *currentAttempt)
			}
			currentAttempt = &HealingAttempt{}
		}

		if currentAttempt == nil {
			continue
		}

		// Parse error signature
		if strings.Contains(strings.ToLower(line), "error signature") ||
			strings.Contains(strings.ToLower(line), "error:") {
			currentAttempt.ErrorSignature = extractErrorSignature(line)
		}

		// Parse success
		if strings.Contains(strings.ToLower(line), "success") &&
			strings.Contains(strings.ToLower(line), "healing") {
			currentAttempt.Success = true
		}

		// Parse confidence
		confidenceRegex := regexp.MustCompile(`confidence:\s+([0-9.]+)`)
		if matches := confidenceRegex.FindStringSubmatch(line); len(matches) > 1 {
			if conf, err := strconv.ParseFloat(matches[1], 64); err == nil {
				currentAttempt.Confidence = conf
			}
		}
	}

	if currentAttempt != nil {
		attempts = append(attempts, *currentAttempt)
	}

	return attempts
}

func extractErrorSignature(line string) string {
	// Extract error signature from line
	if idx := strings.Index(strings.ToLower(line), "error"); idx != -1 {
		remaining := line[idx:]
		if colonIdx := strings.Index(remaining, ":"); colonIdx != -1 {
			sig := strings.TrimSpace(remaining[colonIdx+1:])
			if len(sig) > 100 {
				sig = sig[:100] + "..."
			}
			return sig
		}
	}
	return "unknown-error"
}
