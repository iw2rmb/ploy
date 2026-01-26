package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Detect performs filesystem-only, deterministic detection of the project stack
// from build files in the workspace.
//
// Supported languages (in detection order):
//   - Java: pom.xml (Maven), build.gradle or build.gradle.kts (Gradle)
//   - Go: go.mod
//   - Rust: Cargo.toml, rust-toolchain.toml, rust-toolchain
//   - Python: .python-version, runtime.txt, or pyproject.toml with Python markers
//
// It returns an Observation on success, or a DetectionError when:
//   - Multiple languages detected (reason: "ambiguous")
//   - No build files found (reason: "unknown")
//   - Version cannot be determined (reason: "unknown")
func Detect(ctx context.Context, workspace string) (*Observation, error) {
	// Check for marker files for each language.
	pomPath := filepath.Join(workspace, "pom.xml")
	gradleGroovyPath := filepath.Join(workspace, "build.gradle")
	gradleKtsPath := filepath.Join(workspace, "build.gradle.kts")
	goModPath := filepath.Join(workspace, "go.mod")
	cargoPath := filepath.Join(workspace, "Cargo.toml")
	rustToolchainTomlPath := filepath.Join(workspace, "rust-toolchain.toml")
	rustToolchainPath := filepath.Join(workspace, "rust-toolchain")
	pyprojectPath := filepath.Join(workspace, "pyproject.toml")
	pythonVersionPath := filepath.Join(workspace, ".python-version")
	runtimeTxtPath := filepath.Join(workspace, "runtime.txt")

	hasPom := fileExists(pomPath)
	hasGradleGroovy := fileExists(gradleGroovyPath)
	hasGradleKts := fileExists(gradleKtsPath)
	hasGradle := hasGradleGroovy || hasGradleKts
	hasJava := hasPom || hasGradle

	hasGoMod := fileExists(goModPath)
	hasGo := hasGoMod

	hasCargo := fileExists(cargoPath)
	hasRustToolchainToml := fileExists(rustToolchainTomlPath)
	hasRustToolchain := fileExists(rustToolchainPath)
	hasRust := hasCargo || hasRustToolchainToml || hasRustToolchain

	hasPyproject := fileExists(pyprojectPath)
	hasPythonVersion := fileExists(pythonVersionPath)
	hasRuntimeTxt := fileExists(runtimeTxtPath)
	// pyproject.toml alone is not Python-specific (could be Rust via maturin).
	// Only consider Python when pyproject.toml has Python markers OR explicit Python files exist.
	hasPython := hasPythonVersion || hasRuntimeTxt || (hasPyproject && isPythonPyproject(pyprojectPath))

	// Count detected languages.
	var detectedLanguages []string
	var detectedEvidence []EvidenceItem

	if hasJava {
		detectedLanguages = append(detectedLanguages, "java")
		if hasPom {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "pom.xml", Key: "build.file", Value: "exists"})
		}
		if hasGradleGroovy {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "build.gradle", Key: "build.file", Value: "exists"})
		} else if hasGradleKts {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "build.gradle.kts", Key: "build.file", Value: "exists"})
		}
	}

	if hasGo {
		detectedLanguages = append(detectedLanguages, "go")
		detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "go.mod", Key: "build.file", Value: "exists"})
	}

	if hasRust {
		detectedLanguages = append(detectedLanguages, "rust")
		if hasCargo {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "Cargo.toml", Key: "build.file", Value: "exists"})
		}
		if hasRustToolchainToml {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "rust-toolchain.toml", Key: "build.file", Value: "exists"})
		} else if hasRustToolchain {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "rust-toolchain", Key: "build.file", Value: "exists"})
		}
	}

	if hasPython {
		detectedLanguages = append(detectedLanguages, "python")
		if hasPythonVersion {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: ".python-version", Key: "build.file", Value: "exists"})
		}
		if hasRuntimeTxt {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "runtime.txt", Key: "build.file", Value: "exists"})
		}
		if hasPyproject {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "pyproject.toml", Key: "build.file", Value: "exists"})
		}
	}

	// Check for ambiguous case: multiple languages detected.
	if len(detectedLanguages) > 1 {
		return nil, &DetectionError{
			Reason:   "ambiguous",
			Message:  "multiple languages detected: " + strings.Join(detectedLanguages, ", "),
			Evidence: detectedEvidence,
		}
	}

	// Check for unknown case: no build files found.
	if len(detectedLanguages) == 0 {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "no supported build files found",
		}
	}

	// Dispatch to language-specific detector.
	switch detectedLanguages[0] {
	case "java":
		return detectJava(ctx, workspace, hasPom, hasGradle, hasGradleKts, pomPath, gradleGroovyPath, gradleKtsPath)
	case "go":
		return detectGo(ctx, workspace, goModPath)
	case "rust":
		return detectRust(ctx, workspace)
	case "python":
		return detectPython(ctx, workspace)
	default:
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "unsupported language: " + detectedLanguages[0],
		}
	}
}

// DetectTool performs deterministic tool/language detection from build files in the
// workspace, without requiring a release version to be present.
//
// This is used by Build Gate when a default stack is configured: even when strict
// detection cannot determine a release, Build Gate can still determine the tool
// (e.g., Maven vs Gradle) to select the correct build command.
func DetectTool(ctx context.Context, workspace string) (*Observation, error) {
	pomPath := filepath.Join(workspace, "pom.xml")
	gradleGroovyPath := filepath.Join(workspace, "build.gradle")
	gradleKtsPath := filepath.Join(workspace, "build.gradle.kts")
	goModPath := filepath.Join(workspace, "go.mod")
	cargoPath := filepath.Join(workspace, "Cargo.toml")
	rustToolchainTomlPath := filepath.Join(workspace, "rust-toolchain.toml")
	rustToolchainPath := filepath.Join(workspace, "rust-toolchain")
	pyprojectPath := filepath.Join(workspace, "pyproject.toml")
	pythonVersionPath := filepath.Join(workspace, ".python-version")
	runtimeTxtPath := filepath.Join(workspace, "runtime.txt")

	hasPom := fileExists(pomPath)
	hasGradleGroovy := fileExists(gradleGroovyPath)
	hasGradleKts := fileExists(gradleKtsPath)
	hasGradle := hasGradleGroovy || hasGradleKts
	hasJava := hasPom || hasGradle

	hasGoMod := fileExists(goModPath)
	hasGo := hasGoMod

	hasCargo := fileExists(cargoPath)
	hasRustToolchainToml := fileExists(rustToolchainTomlPath)
	hasRustToolchain := fileExists(rustToolchainPath)
	hasRust := hasCargo || hasRustToolchainToml || hasRustToolchain

	hasPyproject := fileExists(pyprojectPath)
	hasPythonVersion := fileExists(pythonVersionPath)
	hasRuntimeTxt := fileExists(runtimeTxtPath)
	hasPython := hasPythonVersion || hasRuntimeTxt || (hasPyproject && isPythonPyproject(pyprojectPath))

	var detectedLanguages []string
	var detectedEvidence []EvidenceItem

	if hasJava {
		detectedLanguages = append(detectedLanguages, "java")
		if hasPom {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "pom.xml", Key: "build.file", Value: "exists"})
		}
		if hasGradleGroovy {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "build.gradle", Key: "build.file", Value: "exists"})
		} else if hasGradleKts {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "build.gradle.kts", Key: "build.file", Value: "exists"})
		}
	}
	if hasGo {
		detectedLanguages = append(detectedLanguages, "go")
		detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "go.mod", Key: "build.file", Value: "exists"})
	}
	if hasRust {
		detectedLanguages = append(detectedLanguages, "rust")
		if hasCargo {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "Cargo.toml", Key: "build.file", Value: "exists"})
		}
		if hasRustToolchainToml {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "rust-toolchain.toml", Key: "build.file", Value: "exists"})
		} else if hasRustToolchain {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "rust-toolchain", Key: "build.file", Value: "exists"})
		}
	}
	if hasPython {
		detectedLanguages = append(detectedLanguages, "python")
		if hasPythonVersion {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: ".python-version", Key: "build.file", Value: "exists"})
		}
		if hasRuntimeTxt {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "runtime.txt", Key: "build.file", Value: "exists"})
		}
		if hasPyproject {
			detectedEvidence = append(detectedEvidence, EvidenceItem{Path: "pyproject.toml", Key: "build.file", Value: "exists"})
		}
	}

	if len(detectedLanguages) > 1 {
		return nil, &DetectionError{
			Reason:   "ambiguous",
			Message:  "multiple languages detected: " + strings.Join(detectedLanguages, ", "),
			Evidence: detectedEvidence,
		}
	}
	if len(detectedLanguages) == 0 {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "no supported build files found",
		}
	}

	lang := detectedLanguages[0]
	switch lang {
	case "java":
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
		if hasPom {
			return &Observation{
				Language: "java",
				Tool:     "maven",
				Release:  nil,
				Evidence: detectedEvidence,
			}, nil
		}
		return &Observation{
			Language: "java",
			Tool:     "gradle",
			Release:  nil,
			Evidence: detectedEvidence,
		}, nil
	case "go":
		return &Observation{
			Language: "go",
			Tool:     "go",
			Release:  nil,
			Evidence: detectedEvidence,
		}, nil
	case "rust":
		return &Observation{
			Language: "rust",
			Tool:     "cargo",
			Release:  nil,
			Evidence: detectedEvidence,
		}, nil
	case "python":
		tool := "pip"
		if hasPyproject {
			content, _ := os.ReadFile(pyprojectPath)
			if poetryToolRegex.MatchString(string(content)) {
				tool = "poetry"
			}
		}
		return &Observation{
			Language: "python",
			Tool:     tool,
			Release:  nil,
			Evidence: detectedEvidence,
		}, nil
	default:
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "unsupported language: " + lang,
		}
	}
}

// detectJava handles Java detection with Maven/Gradle disambiguation.
func detectJava(ctx context.Context, workspace string, hasPom, hasGradle, hasGradleKts bool, pomPath, gradleGroovyPath, gradleKtsPath string) (*Observation, error) {
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

// isPythonPyproject checks if a pyproject.toml file contains Python-specific markers.
// Returns true if it contains [project] with requires-python or [tool.poetry.dependencies] with python.
func isPythonPyproject(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(content)

	// Check for PEP 621 requires-python in [project] section.
	if projectSectionRegex.MatchString(text) && requiresPythonRegex.MatchString(text) {
		return true
	}

	// Check for Poetry python dependency.
	if poetryToolRegex.MatchString(text) && poetryDepsRegex.MatchString(text) && poetryPythonRegex.MatchString(text) {
		return true
	}

	return false
}

// fileExists returns true if the path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
