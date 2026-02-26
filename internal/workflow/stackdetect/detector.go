package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/fsutil"
)

const goModuleFile = "go." + "mo" + "d"

// scanResult holds the filesystem scan state shared between Detect and DetectTool.
type scanResult struct {
	workspace string

	hasPom           bool
	hasGradleGroovy  bool
	hasGradleKts     bool
	hasGradle        bool
	hasJava          bool
	hasGoMod         bool
	hasGo            bool
	hasCargo         bool
	hasRustToolchain bool
	hasRust          bool
	hasPyproject     bool
	hasPythonVersion bool
	hasRuntimeTxt    bool
	hasPython        bool

	pyprojectLoaded            bool
	pyprojectHasProjectSection bool
	pyprojectRequiresPython    string
	pyprojectHasPoetryTool     bool
	pyprojectHasPoetryDeps     bool
	pyprojectPoetryPython      string

	languages []string
	evidence  []EvidenceItem
}

// scanWorkspace performs filesystem-only detection of build files in the workspace
// and returns a scanResult with all detected languages, booleans, and evidence.
func scanWorkspace(workspace string) scanResult {
	pomPath := filepath.Join(workspace, "pom.xml")
	gradleGroovyPath := filepath.Join(workspace, "build.gradle")
	gradleKtsPath := filepath.Join(workspace, "build.gradle.kts")
	goModPath := filepath.Join(workspace, goModuleFile)
	cargoPath := filepath.Join(workspace, "Cargo.toml")
	rustToolchainTomlPath := filepath.Join(workspace, "rust-toolchain.toml")
	rustToolchainPath := filepath.Join(workspace, "rust-toolchain")
	pyprojectPath := filepath.Join(workspace, "pyproject.toml")
	pythonVersionPath := filepath.Join(workspace, ".python-version")
	runtimeTxtPath := filepath.Join(workspace, "runtime.txt")

	s := scanResult{
		workspace: workspace,
	}

	s.hasPom = fsutil.FileExists(pomPath)
	s.hasGradleGroovy = fsutil.FileExists(gradleGroovyPath)
	s.hasGradleKts = fsutil.FileExists(gradleKtsPath)
	s.hasGradle = s.hasGradleGroovy || s.hasGradleKts
	s.hasJava = s.hasPom || s.hasGradle

	s.hasGoMod = fsutil.FileExists(goModPath)
	s.hasGo = s.hasGoMod

	s.hasCargo = fsutil.FileExists(cargoPath)
	hasRustToolchainToml := fsutil.FileExists(rustToolchainTomlPath)
	s.hasRustToolchain = fsutil.FileExists(rustToolchainPath)
	s.hasRust = s.hasCargo || hasRustToolchainToml || s.hasRustToolchain

	s.hasPyproject = fsutil.FileExists(pyprojectPath)
	s.hasPythonVersion = fsutil.FileExists(pythonVersionPath)
	s.hasRuntimeTxt = fsutil.FileExists(runtimeTxtPath)
	if s.hasPyproject {
		content, err := os.ReadFile(pyprojectPath)
		if err == nil {
			text := string(content)
			s.pyprojectLoaded = true
			s.pyprojectHasProjectSection = projectSectionRegex.MatchString(text)
			s.pyprojectHasPoetryTool = poetryToolRegex.MatchString(text)
			s.pyprojectHasPoetryDeps = poetryDepsRegex.MatchString(text)
			if matches := requiresPythonRegex.FindStringSubmatch(text); matches != nil {
				s.pyprojectRequiresPython = strings.TrimSpace(matches[1])
			}
			if matches := poetryPythonRegex.FindStringSubmatch(text); matches != nil {
				s.pyprojectPoetryPython = strings.TrimSpace(matches[1])
			}
		}
	}
	hasPythonPyproject := (s.pyprojectHasProjectSection && s.pyprojectRequiresPython != "") ||
		(s.pyprojectHasPoetryTool && s.pyprojectHasPoetryDeps && s.pyprojectPoetryPython != "")
	s.hasPython = s.hasPythonVersion || s.hasRuntimeTxt || (s.hasPyproject && hasPythonPyproject)

	// Build detected languages and evidence.
	if s.hasJava {
		s.languages = append(s.languages, "java")
		if s.hasPom {
			s.evidence = append(s.evidence, EvidenceItem{Path: "pom.xml", Key: "build.file", Value: "exists"})
		}
		if s.hasGradleGroovy {
			s.evidence = append(s.evidence, EvidenceItem{Path: "build.gradle", Key: "build.file", Value: "exists"})
		} else if s.hasGradleKts {
			s.evidence = append(s.evidence, EvidenceItem{Path: "build.gradle.kts", Key: "build.file", Value: "exists"})
		}
	}
	if s.hasGo {
		s.languages = append(s.languages, "go")
		s.evidence = append(s.evidence, EvidenceItem{Path: goModuleFile, Key: "build.file", Value: "exists"})
	}
	if s.hasRust {
		s.languages = append(s.languages, "rust")
		if s.hasCargo {
			s.evidence = append(s.evidence, EvidenceItem{Path: "Cargo.toml", Key: "build.file", Value: "exists"})
		}
		if hasRustToolchainToml {
			s.evidence = append(s.evidence, EvidenceItem{Path: "rust-toolchain.toml", Key: "build.file", Value: "exists"})
		} else if s.hasRustToolchain {
			s.evidence = append(s.evidence, EvidenceItem{Path: "rust-toolchain", Key: "build.file", Value: "exists"})
		}
	}
	if s.hasPython {
		s.languages = append(s.languages, "python")
		if s.hasPythonVersion {
			s.evidence = append(s.evidence, EvidenceItem{Path: ".python-version", Key: "build.file", Value: "exists"})
		}
		if s.hasRuntimeTxt {
			s.evidence = append(s.evidence, EvidenceItem{Path: "runtime.txt", Key: "build.file", Value: "exists"})
		}
		if s.hasPyproject {
			s.evidence = append(s.evidence, EvidenceItem{Path: "pyproject.toml", Key: "build.file", Value: "exists"})
		}
	}

	return s
}

// checkAmbiguousOrUnknown returns an error if the scan found multiple or zero languages.
func (s *scanResult) checkAmbiguousOrUnknown() error {
	if len(s.languages) > 1 {
		return &DetectionError{
			Reason:   "ambiguous",
			Message:  "multiple languages detected: " + strings.Join(s.languages, ", "),
			Evidence: s.evidence,
		}
	}
	if len(s.languages) == 0 {
		return &DetectionError{
			Reason:  "unknown",
			Message: "no supported build files found",
		}
	}
	return nil
}

// Detect performs filesystem-only, deterministic detection of the project stack
// from build files in the workspace.
//
// Supported languages (in detection order):
//   - Java: pom.xml (Maven), build.gradle or build.gradle.kts (Gradle)
//   - Go: Go module file
//   - Rust: Cargo.toml, rust-toolchain.toml, rust-toolchain
//   - Python: .python-version, runtime.txt, or pyproject.toml with Python markers
//
// It returns an Observation on success, or a DetectionError when:
//   - Multiple languages detected (reason: "ambiguous")
//   - No build files found (reason: "unknown")
//   - Version cannot be determined (reason: "unknown")
func Detect(ctx context.Context, workspace string) (*Observation, error) {
	s := scanWorkspace(workspace)
	if err := s.checkAmbiguousOrUnknown(); err != nil {
		return nil, err
	}

	switch s.languages[0] {
	case "java":
		return detectJava(ctx, s)
	case "go":
		return detectGo(ctx, workspace, filepath.Join(workspace, goModuleFile))
	case "rust":
		return detectRust(ctx, workspace)
	case "python":
		return detectPython(ctx, s)
	default:
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "unsupported language: " + s.languages[0],
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
	s := scanWorkspace(workspace)
	if err := s.checkAmbiguousOrUnknown(); err != nil {
		return nil, err
	}

	lang := s.languages[0]
	switch lang {
	case "java":
		tool, err := detectJavaTool(s)
		if err != nil {
			return nil, err
		}
		return &Observation{
			Language: "java",
			Tool:     tool,
			Release:  nil,
			Evidence: s.evidence,
		}, nil
	case "go":
		return &Observation{
			Language: "go",
			Tool:     "go",
			Release:  nil,
			Evidence: s.evidence,
		}, nil
	case "rust":
		return &Observation{
			Language: "rust",
			Tool:     "cargo",
			Release:  nil,
			Evidence: s.evidence,
		}, nil
	case "python":
		tool := "pip"
		if s.pyprojectHasPoetryTool {
			tool = "poetry"
		}
		return &Observation{
			Language: "python",
			Tool:     tool,
			Release:  nil,
			Evidence: s.evidence,
		}, nil
	default:
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "unsupported language: " + lang,
		}
	}
}

// detectJava handles Java detection with Maven/Gradle disambiguation.
func detectJava(ctx context.Context, s scanResult) (*Observation, error) {
	tool, err := detectJavaTool(s)
	if err != nil {
		return nil, err
	}

	if tool == "maven" {
		return detectMaven(ctx, s.workspace, filepath.Join(s.workspace, "pom.xml"))
	}

	// Prefer Kotlin DSL if present.
	gradlePath := filepath.Join(s.workspace, gradleBuildFile(s))
	return detectGradle(ctx, s.workspace, gradlePath)
}

func detectJavaTool(s scanResult) (string, error) {
	if s.hasPom && s.hasGradle {
		return "", javaAmbiguityError(s)
	}
	if s.hasPom {
		return "maven", nil
	}
	return "gradle", nil
}

func javaAmbiguityError(s scanResult) *DetectionError {
	return &DetectionError{
		Reason:  "ambiguous",
		Message: "both pom.xml and build.gradle(.kts) present; tool selection is ambiguous",
		Evidence: []EvidenceItem{
			{Path: "pom.xml", Key: "build.file", Value: "exists"},
			{Path: gradleBuildFile(s), Key: "build.file", Value: "exists"},
		},
	}
}

func gradleBuildFile(s scanResult) string {
	if s.hasGradleKts {
		return "build.gradle.kts"
	}
	return "build.gradle"
}
