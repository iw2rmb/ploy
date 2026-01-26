package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Regex patterns for Gradle version detection.
var (
	// sourceCompatibility = "17" or sourceCompatibility = 17
	// sourceCompatibility = JavaVersion.VERSION_17 or sourceCompatibility = VERSION_17
	sourceCompatibilityRegex = regexp.MustCompile(`sourceCompatibility\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	// targetCompatibility = "17" or targetCompatibility = 17
	// targetCompatibility = JavaVersion.VERSION_17 or targetCompatibility = VERSION_17
	targetCompatibilityRegex = regexp.MustCompile(`targetCompatibility\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	// kotlinOptions.jvmTarget = "17" or kotlinOptions.jvmTarget = JavaVersion.VERSION_17
	kotlinOptionsJvmTargetDirectRegex = regexp.MustCompile(`kotlinOptions\.jvmTarget\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	// kotlinOptions { ... jvmTarget = "17" } or kotlinOptions { ... jvmTarget = JavaVersion.VERSION_17 }
	kotlinOptionsJvmTargetBlockRegex = regexp.MustCompile(`(?s)kotlinOptions\s*\{.*?jvmTarget\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	// Dynamic logic patterns that should trigger "unknown".
	dynamicPatterns = []*regexp.Regexp{
		regexp.MustCompile(`findProperty\s*\(`),
		regexp.MustCompile(`getProperty\s*\(`),
		regexp.MustCompile(`System\.getenv\s*\(`),
		regexp.MustCompile(`project\.properties\s*\[`),
		regexp.MustCompile(`extra\s*\[`),
		regexp.MustCompile(`ext\s*\[`),
		regexp.MustCompile(`val\s+\w+\s*=.*JavaVersion`),
		regexp.MustCompile(`def\s+\w+\s*=.*JavaVersion`),
	}
)

// detectGradle detects Java version from Gradle build files.
// Precedence (strict order):
//  1. sourceCompatibility / targetCompatibility (must match if both present)
//  2. kotlinOptions.jvmTarget (best-effort; used only if source/target are absent)
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

	// 1. Check sourceCompatibility and targetCompatibility.
	sourceVersion := extractCompatibilityVersion(sourceCompatibilityRegex, text)
	targetVersion := extractCompatibilityVersion(targetCompatibilityRegex, text)

	if sourceVersion != "" || targetVersion != "" {
		var evidence []EvidenceItem

		if sourceVersion != "" {
			evidence = append(evidence, EvidenceItem{
				Path: relativePath, Key: "sourceCompatibility", Value: sourceVersion,
			})
		}
		if targetVersion != "" {
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

	// 2. Kotlin JVM hint: kotlinOptions.jvmTarget.
	jvmTargetDirect := extractCompatibilityVersion(kotlinOptionsJvmTargetDirectRegex, text)
	jvmTargetBlock := extractCompatibilityVersion(kotlinOptionsJvmTargetBlockRegex, text)

	if jvmTargetDirect != "" || jvmTargetBlock != "" {
		var evidence []EvidenceItem

		if jvmTargetDirect != "" {
			evidence = append(evidence, EvidenceItem{
				Path: relativePath, Key: "kotlinOptions.jvmTarget", Value: jvmTargetDirect,
			})
		}
		if jvmTargetBlock != "" {
			evidence = append(evidence, EvidenceItem{
				Path: relativePath, Key: "kotlinOptions.jvmTarget", Value: jvmTargetBlock,
			})
		}

		// If both present, they must match.
		if jvmTargetDirect != "" && jvmTargetBlock != "" && jvmTargetDirect != jvmTargetBlock {
			return nil, &DetectionError{
				Reason:   "unknown",
				Message:  "kotlinOptions.jvmTarget differs between assignments",
				Evidence: evidence,
			}
		}

		version := jvmTargetDirect
		if version == "" {
			version = jvmTargetBlock
		}

		return &Observation{
			Language: "java",
			Tool:     "gradle",
			Release:  &version,
			Evidence: evidence,
		}, nil
	}

	// No Java version found.
	for _, pattern := range dynamicPatterns {
		if pattern.MatchString(text) {
			return nil, &DetectionError{
				Reason:  "unknown",
				Message: "build.gradle contains dynamic version logic; cannot reliably detect Java version",
			}
		}
	}
	return nil, &DetectionError{
		Reason:  "unknown",
		Message: "no supported Java version configuration found in " + filepath.Base(gradlePath),
	}
}

// extractCompatibilityVersion extracts version from sourceCompatibility or targetCompatibility patterns.
// Returns the numeric version string or empty if not found.
func extractCompatibilityVersion(regex *regexp.Regexp, text string) string {
	matches := regex.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	for i := 1; i < len(matches); i++ {
		if matches[i] == "" {
			continue
		}
		return normalizeJavaVersion(matches[i])
	}
	return ""
}

// normalizeJavaVersion normalizes legacy version strings like "1.8" to "8".
func normalizeJavaVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "_", ".")
	if strings.HasPrefix(v, "1.") {
		if n, err := strconv.Atoi(strings.TrimPrefix(v, "1.")); err == nil {
			return strconv.Itoa(n)
		}
	}
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
