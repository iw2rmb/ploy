package stackdetect

import (
	"context"
	"os"
	"regexp"
)

// Regex patterns for Go version detection.
var (
	// goDirectiveRegex matches "go 1.xx" directive in go.mod.
	goDirectiveRegex = regexp.MustCompile(`(?m)^go\s+(1\.\d+)`)

	// goToolchainRegex matches "toolchain go1.xx.x" directive in go.mod.
	goToolchainRegex = regexp.MustCompile(`(?m)^toolchain\s+go(1\.\d+(?:\.\d+)?)`)
)

// detectGo detects Go version from go.mod.
// Returns the version from the "go 1.xx" directive.
// Optionally captures "toolchain go1.xx.x" as additional evidence.
func detectGo(ctx context.Context, workspace, goModPath string) (*Observation, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "failed to read go.mod: " + err.Error(),
		}
	}

	text := string(content)
	relativePath := relPath(workspace, goModPath)

	// Look for the "go 1.xx" directive.
	matches := goDirectiveRegex.FindStringSubmatch(text)
	if matches == nil {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "no go directive found in go.mod",
		}
	}

	version := matches[1]
	evidence := []EvidenceItem{
		{Path: relativePath, Key: "go", Value: version},
	}

	// Optionally capture toolchain directive as additional evidence.
	if toolchainMatches := goToolchainRegex.FindStringSubmatch(text); toolchainMatches != nil {
		evidence = append(evidence, EvidenceItem{
			Path:  relativePath,
			Key:   "toolchain",
			Value: toolchainMatches[1],
		})
	}

	return &Observation{
		Language: "go",
		Tool:     "go",
		Release:  &version,
		Evidence: evidence,
	}, nil
}
