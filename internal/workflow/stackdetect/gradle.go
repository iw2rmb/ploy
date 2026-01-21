package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Regex patterns for Gradle version detection.
var (
	// JavaLanguageVersion.of(17) - toolchain API
	javaLanguageVersionRegex = regexp.MustCompile(`JavaLanguageVersion\.of\((\d+)\)`)

	// sourceCompatibility = "17" or sourceCompatibility = 17
	sourceCompatibilityRegex = regexp.MustCompile(`sourceCompatibility\s*=\s*"?(\d+)"?`)

	// targetCompatibility = "17" or targetCompatibility = 17
	targetCompatibilityRegex = regexp.MustCompile(`targetCompatibility\s*=\s*"?(\d+)"?`)

	// Dynamic logic patterns that should trigger "unknown".
	dynamicPatterns = []*regexp.Regexp{
		regexp.MustCompile(`findProperty\s*\(`),
		regexp.MustCompile(`getProperty\s*\(`),
		regexp.MustCompile(`System\.getenv\s*\(`),
		regexp.MustCompile(`project\.properties\s*\[`),
		regexp.MustCompile(`extra\s*\[`),
		regexp.MustCompile(`ext\s*\[`),
		regexp.MustCompile(`val\s+\w+\s*=.*JavaLanguageVersion`),
		regexp.MustCompile(`val\s+\w+\s*=.*JavaVersion`),
		regexp.MustCompile(`def\s+\w+\s*=.*JavaLanguageVersion`),
		regexp.MustCompile(`def\s+\w+\s*=.*JavaVersion`),
	}
)

// detectGradle detects Java version from Gradle build files.
// Precedence (strict order):
//  1. JavaLanguageVersion.of(N) - toolchain API
//  2. sourceCompatibility / targetCompatibility (must match if both present)
func detectGradle(ctx context.Context, workspace, gradlePath string) (*Observation, error) {
	content, err := os.ReadFile(gradlePath)
	if err != nil {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "failed to read build.gradle: " + err.Error(),
		}
	}

	text := string(content)
	relativePath := relPath(workspace, gradlePath)

	// Check for dynamic logic that makes detection unreliable.
	for _, pattern := range dynamicPatterns {
		if pattern.MatchString(text) {
			return nil, &DetectionError{
				Reason:  "unknown",
				Message: "build.gradle contains dynamic version logic; cannot reliably detect Java version",
			}
		}
	}

	// 1. Check JavaLanguageVersion.of(N) - toolchain API (highest precedence).
	if matches := javaLanguageVersionRegex.FindStringSubmatch(text); matches != nil {
		version := matches[1]
		return &Observation{
			Language: "java",
			Tool:     "gradle",
			Release:  &version,
			Evidence: []EvidenceItem{
				{Path: relativePath, Key: "toolchain.languageVersion", Value: version},
			},
		}, nil
	}

	// 2. Check sourceCompatibility and targetCompatibility.
	sourceVersion := extractCompatibilityVersion(sourceCompatibilityRegex, text)
	targetVersion := extractCompatibilityVersion(targetCompatibilityRegex, text)

	if sourceVersion != "" || targetVersion != "" {
		var evidence []EvidenceItem

		if sourceVersion != "" {
			sourceVersion = normalizeJavaVersion(sourceVersion)
			evidence = append(evidence, EvidenceItem{
				Path: relativePath, Key: "sourceCompatibility", Value: sourceVersion,
			})
		}
		if targetVersion != "" {
			targetVersion = normalizeJavaVersion(targetVersion)
			evidence = append(evidence, EvidenceItem{
				Path: relativePath, Key: "targetCompatibility", Value: targetVersion,
			})
		}

		// If both present, they must match.
		if sourceVersion != "" && targetVersion != "" && sourceVersion != targetVersion {
			return nil, &DetectionError{
				Reason:   "unknown",
				Message:  "sourceCompatibility and targetCompatibility differ",
				Evidence: evidence,
			}
		}

		// Use whichever is present (or the matching value).
		version := sourceVersion
		if version == "" {
			version = targetVersion
		}

		return &Observation{
			Language: "java",
			Tool:     "gradle",
			Release:  &version,
			Evidence: evidence,
		}, nil
	}

	// No Java version found.
	return nil, &DetectionError{
		Reason:  "unknown",
		Message: "no Java version configuration found in " + filepath.Base(gradlePath),
	}
}

// extractCompatibilityVersion extracts version from sourceCompatibility or targetCompatibility patterns.
// Returns the numeric version string or empty if not found.
func extractCompatibilityVersion(regex *regexp.Regexp, text string) string {
	matches := regex.FindStringSubmatch(text)
	if matches == nil || len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// normalizeJavaVersion normalizes legacy version strings like "1.8" to "8".
func normalizeJavaVersion(v string) string {
	v = strings.TrimSpace(v)
	switch v {
	case "1.8":
		return "8"
	case "1.7":
		return "7"
	case "1.6":
		return "6"
	case "1.5":
		return "5"
	default:
		return v
	}
}
