package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Regex patterns for Python version detection.
var (
	// pythonVersionRegex matches versions like "3.11", "3.11.6".
	pythonVersionRegex = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?$`)

	// runtimeTxtRegex matches "python-3.9.18" format in runtime.txt.
	runtimeTxtRegex = regexp.MustCompile(`^python-(\d+\.\d+(?:\.\d+)?)$`)

	// requiresPythonRegex matches requires-python = "..." in pyproject.toml.
	requiresPythonRegex = regexp.MustCompile(`(?m)^requires-python\s*=\s*"([^"]+)"`)

	// poetryPythonRegex matches python = "..." under [tool.poetry.dependencies].
	poetryPythonRegex = regexp.MustCompile(`(?m)^\s*python\s*=\s*"([^"]+)"`)

	// poetryToolRegex checks if [tool.poetry] section exists.
	poetryToolRegex = regexp.MustCompile(`(?m)^\[tool\.poetry\]`)

	// poetryDepsRegex matches the [tool.poetry.dependencies] section.
	poetryDepsRegex = regexp.MustCompile(`(?m)^\[tool\.poetry\.dependencies\]`)

	// projectSectionRegex matches the [project] section.
	projectSectionRegex = regexp.MustCompile(`(?m)^\[project\]`)

	// Specifier patterns for reducible ranges.
	// >=3.11,<3.12 pattern.
	rangeSpecifierRegex = regexp.MustCompile(`^>=\s*(\d+)\.(\d+)(?:\.\d+)?\s*,\s*<\s*(\d+)\.(\d+)$`)
	// ~=3.11.0 pattern (compatible release).
	tildeEqualRegex = regexp.MustCompile(`^~=\s*(\d+)\.(\d+)(?:\.(\d+))?$`)
	// >=3.9 alone (spans multiple minors).
	openEndedRegex = regexp.MustCompile(`^>=\s*(\d+)\.(\d+)(?:\.\d+)?$`)
	// ^3.11 pattern (Poetry caret).
	caretRegex = regexp.MustCompile(`^\^(\d+)\.(\d+)(?:\.\d+)?$`)
)

// detectPython detects Python version from various sources.
// Precedence order:
//  1. .python-version (pyenv file) → highest precedence
//  2. runtime.txt (python-3.9.18 format)
//  3. pyproject.toml [project] requires-python (PEP 621)
//  4. pyproject.toml [tool.poetry.dependencies] python (Poetry)
//
// Returns unknown for:
//   - Specifiers spanning multiple minors (e.g., >=3.9, ^3.11)
//   - Disagreement between sources
func detectPython(ctx context.Context, workspace string) (*Observation, error) {
	pythonVersionPath := filepath.Join(workspace, ".python-version")
	runtimeTxtPath := filepath.Join(workspace, "runtime.txt")
	pyprojectPath := filepath.Join(workspace, "pyproject.toml")

	var detections []pythonDetection

	// 1. Check .python-version (highest precedence).
	if fileExists(pythonVersionPath) {
		content, err := os.ReadFile(pythonVersionPath)
		if err == nil {
			version := strings.TrimSpace(string(content))
			if canonical := canonicalizePythonVersion(version); canonical != "" {
				detections = append(detections, pythonDetection{
					version:  canonical,
					path:     ".python-version",
					key:      "python",
					priority: 1,
				})
			}
		}
	}

	// 2. Check runtime.txt.
	if fileExists(runtimeTxtPath) {
		content, err := os.ReadFile(runtimeTxtPath)
		if err == nil {
			line := strings.TrimSpace(string(content))
			if matches := runtimeTxtRegex.FindStringSubmatch(line); matches != nil {
				if canonical := canonicalizePythonVersion(matches[1]); canonical != "" {
					detections = append(detections, pythonDetection{
						version:  canonical,
						path:     "runtime.txt",
						key:      "python",
						priority: 2,
					})
				}
			}
		}
	}

	// 3 & 4. Check pyproject.toml.
	if fileExists(pyprojectPath) {
		content, err := os.ReadFile(pyprojectPath)
		if err == nil {
			text := string(content)

			// Check for Poetry.
			isPoetry := poetryToolRegex.MatchString(text)

			// Check PEP 621 requires-python in [project] section.
			if projectSectionRegex.MatchString(text) {
				if matches := requiresPythonRegex.FindStringSubmatch(text); matches != nil {
					specifier := strings.TrimSpace(matches[1])
					version, err := reduceSpecifier(specifier)
					if err != nil {
						return nil, &DetectionError{
							Reason:  "unknown",
							Message: err.Error(),
							Evidence: []EvidenceItem{
								{Path: "pyproject.toml", Key: "requires-python", Value: specifier},
							},
						}
					}
					detections = append(detections, pythonDetection{
						version:  version,
						path:     "pyproject.toml",
						key:      "requires-python",
						priority: 3,
					})
				}
			}

			// Check Poetry python dependency.
			if isPoetry && poetryDepsRegex.MatchString(text) {
				// Find the [tool.poetry.dependencies] section and extract python.
				if matches := poetryPythonRegex.FindStringSubmatch(text); matches != nil {
					specifier := strings.TrimSpace(matches[1])
					version, err := reduceSpecifier(specifier)
					if err != nil {
						return nil, &DetectionError{
							Reason:  "unknown",
							Message: err.Error(),
							Evidence: []EvidenceItem{
								{Path: "pyproject.toml", Key: "tool.poetry.dependencies.python", Value: specifier},
							},
						}
					}
					detections = append(detections, pythonDetection{
						version:  version,
						path:     "pyproject.toml",
						key:      "tool.poetry.dependencies.python",
						priority: 4,
					})
				}
			}
		}
	}

	// No detections found.
	if len(detections) == 0 {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "no Python version configuration found",
		}
	}

	// Check for disagreement between sources.
	if len(detections) > 1 {
		firstVersion := detections[0].version
		for _, d := range detections[1:] {
			if d.version != firstVersion {
				var evidence []EvidenceItem
				for _, det := range detections {
					evidence = append(evidence, EvidenceItem{
						Path:  det.path,
						Key:   det.key,
						Value: det.version,
					})
				}
				return nil, &DetectionError{
					Reason:   "unknown",
					Message:  "conflicting Python versions detected",
					Evidence: evidence,
				}
			}
		}
	}

	// Use the highest-priority detection.
	best := detections[0]
	for _, d := range detections[1:] {
		if d.priority < best.priority {
			best = d
		}
	}

	// Determine tool.
	tool := "pip"
	if fileExists(pyprojectPath) {
		content, _ := os.ReadFile(pyprojectPath)
		if poetryToolRegex.MatchString(string(content)) {
			tool = "poetry"
		}
	}

	return &Observation{
		Language: "python",
		Tool:     tool,
		Release:  &best.version,
		Evidence: []EvidenceItem{
			{Path: best.path, Key: best.key, Value: best.version},
		},
	}, nil
}

// pythonDetection represents a detected Python version from a specific source.
type pythonDetection struct {
	version  string
	path     string
	key      string
	priority int // lower is higher priority
}

// canonicalizePythonVersion normalizes "3.11.6" to "3.11".
func canonicalizePythonVersion(v string) string {
	matches := pythonVersionRegex.FindStringSubmatch(strings.TrimSpace(v))
	if matches == nil {
		return ""
	}
	return matches[1] + "." + matches[2]
}

// reduceSpecifier attempts to reduce a Python version specifier to a single minor version.
// Returns error for specifiers spanning multiple minors.
func reduceSpecifier(specifier string) (string, error) {
	specifier = strings.TrimSpace(specifier)

	// Direct version: "3.11" or "3.11.6".
	if matches := pythonVersionRegex.FindStringSubmatch(specifier); matches != nil {
		return matches[1] + "." + matches[2], nil
	}

	// ==3.11 or ==3.11.* exact match.
	if strings.HasPrefix(specifier, "==") {
		version := strings.TrimPrefix(specifier, "==")
		version = strings.TrimSuffix(version, ".*")
		if matches := pythonVersionRegex.FindStringSubmatch(version); matches != nil {
			return matches[1] + "." + matches[2], nil
		}
	}

	// >=3.11,<3.12 range that reduces to single minor.
	if matches := rangeSpecifierRegex.FindStringSubmatch(specifier); matches != nil {
		lowerMajor, _ := strconv.Atoi(matches[1])
		lowerMinor, _ := strconv.Atoi(matches[2])
		upperMajor, _ := strconv.Atoi(matches[3])
		upperMinor, _ := strconv.Atoi(matches[4])

		// Check if upper bound is exactly next minor.
		if upperMajor == lowerMajor && upperMinor == lowerMinor+1 {
			return matches[1] + "." + matches[2], nil
		}
	}

	// ~=3.11.0 or ~=3.11 compatible release.
	if matches := tildeEqualRegex.FindStringSubmatch(specifier); matches != nil {
		// ~=3.11.0 is equivalent to >=3.11.0,<3.12.
		// ~=3.11 is equivalent to >=3.11,<4.0 (spans multiple minors within major).
		if matches[3] != "" {
			// Has patch version, reduces to minor.
			return matches[1] + "." + matches[2], nil
		}
		// No patch version - spans multiple minors.
		return "", &DetectionError{
			Reason:  "unknown",
			Message: "specifier ~=" + matches[1] + "." + matches[2] + " spans multiple minor versions",
		}
	}

	// ^3.11 Poetry caret - spans multiple minors.
	if matches := caretRegex.FindStringSubmatch(specifier); matches != nil {
		return "", &DetectionError{
			Reason:  "unknown",
			Message: "specifier ^" + matches[1] + "." + matches[2] + " spans multiple minor versions",
		}
	}

	// >=3.9 alone - spans multiple minors.
	if matches := openEndedRegex.FindStringSubmatch(specifier); matches != nil {
		return "", &DetectionError{
			Reason:  "unknown",
			Message: "specifier >=" + matches[1] + "." + matches[2] + " spans multiple minor versions",
		}
	}

	// Unknown specifier format.
	return "", &DetectionError{
		Reason:  "unknown",
		Message: "cannot reduce specifier: " + specifier,
	}
}
