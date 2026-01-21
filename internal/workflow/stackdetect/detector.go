package stackdetect

import (
	"context"
	"os"
	"path/filepath"
)

// Detect performs filesystem-only, deterministic detection of the Java stack
// from Maven or Gradle build files in the workspace.
//
// It returns an Observation on success, or a DetectionError when:
//   - Both pom.xml and build.gradle(.kts) exist (reason: "ambiguous")
//   - Neither build file exists (reason: "unknown")
//   - Version cannot be determined (reason: "unknown")
func Detect(ctx context.Context, workspace string) (*Observation, error) {
	pomPath := filepath.Join(workspace, "pom.xml")
	gradleGroovyPath := filepath.Join(workspace, "build.gradle")
	gradleKtsPath := filepath.Join(workspace, "build.gradle.kts")

	hasPom := fileExists(pomPath)
	hasGradleGroovy := fileExists(gradleGroovyPath)
	hasGradleKts := fileExists(gradleKtsPath)
	hasGradle := hasGradleGroovy || hasGradleKts

	// Check for ambiguous case: both Maven and Gradle present.
	if hasPom && hasGradle {
		gradleFile := "build.gradle"
		if hasGradleKts {
			gradleFile = "build.gradle.kts"
		}
		return nil, &DetectionError{
			Reason:  "ambiguous",
			Message: "both pom.xml and build.gradle(.kts) present; tool selection is ambiguous",
			Evidence: []EvidenceItem{
				{Path: "pom.xml", Key: "build.file", Value: "exists"},
				{Path: gradleFile, Key: "build.file", Value: "exists"},
			},
		}
	}

	// Check for unknown case: neither build file present.
	if !hasPom && !hasGradle {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "no Maven or Gradle build files found",
		}
	}

	// Dispatch to appropriate detector.
	if hasPom {
		return detectMaven(ctx, workspace, pomPath)
	}

	// Prefer Kotlin DSL if present.
	gradlePath := gradleGroovyPath
	if hasGradleKts {
		gradlePath = gradleKtsPath
	}
	return detectGradle(ctx, workspace, gradlePath)
}

// fileExists returns true if the path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
