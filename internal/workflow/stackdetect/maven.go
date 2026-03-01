package stackdetect

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pomProject represents the relevant portions of a Maven pom.xml.
type pomProject struct {
	XMLName    xml.Name      `xml:"project"`
	Parent     *pomParent    `xml:"parent"`
	Properties pomProperties `xml:"properties"`
}

// pomParent represents the parent POM reference.
type pomParent struct {
	RelativePath string `xml:"relativePath"`
}

// pomProperties holds all property elements as raw XML.
type pomProperties struct {
	Inner []byte `xml:",innerxml"`
}

// propertyRegex matches ${property.name} patterns.
var propertyRegex = regexp.MustCompile(`^\$\{([^}]+)\}$`)

// detectMaven detects Java version from Maven pom.xml.
// Precedence (strict order):
//  1. maven.compiler.release property
//  2. maven.compiler.source + maven.compiler.target (must match)
//  3. java.version property
func detectMaven(ctx context.Context, workspace, pomPath string) (*Observation, error) {
	// Parse the POM file.
	pom, err := parsePom(pomPath)
	if err != nil {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "failed to parse pom.xml: " + err.Error(),
		}
	}

	// Build property map from this POM and parent(s).
	props := buildPropertyMap(workspace, pomPath, pom, make(map[string]bool))

	relativePath := relPath(workspace, pomPath)

	// 1. Check maven.compiler.release property.
	if release, ok := props["maven.compiler.release"]; ok {
		resolved, err := resolveValue(release, props)
		if err != nil {
			return nil, &DetectionError{
				Reason:  "unknown",
				Message: "maven.compiler.release contains unresolved placeholder: " + release,
				Evidence: []EvidenceItem{
					{Path: relativePath, Key: "maven.compiler.release", Value: release},
				},
			}
		}
		if isValidVersion(resolved) {
			return &Observation{
				Language: "java",
				Tool:     "maven",
				Release:  &resolved,
				Evidence: []EvidenceItem{
					{Path: relativePath, Key: "maven.compiler.release", Value: resolved},
				},
			}, nil
		}
	}

	// 2. Check maven.compiler.source and maven.compiler.target.
	source, hasSource := props["maven.compiler.source"]
	target, hasTarget := props["maven.compiler.target"]
	if hasSource || hasTarget {
		var evidence []EvidenceItem
		var resolvedSource, resolvedTarget string
		var sourceErr, targetErr error

		if hasSource {
			resolvedSource, sourceErr = resolveValue(source, props)
			evidence = append(evidence, EvidenceItem{
				Path: relativePath, Key: "maven.compiler.source", Value: source,
			})
		}
		if hasTarget {
			resolvedTarget, targetErr = resolveValue(target, props)
			evidence = append(evidence, EvidenceItem{
				Path: relativePath, Key: "maven.compiler.target", Value: target,
			})
		}

		if sourceErr != nil || targetErr != nil {
			return nil, &DetectionError{
				Reason:   "unknown",
				Message:  "maven.compiler.source/target contains unresolved placeholder",
				Evidence: evidence,
			}
		}

		// Both must be present and equal.
		if hasSource && hasTarget {
			if resolvedSource == resolvedTarget && isValidVersion(resolvedSource) {
				return &Observation{
					Language: "java",
					Tool:     "maven",
					Release:  &resolvedSource,
					Evidence: evidence,
				}, nil
			}
			if resolvedSource != resolvedTarget {
				return nil, &DetectionError{
					Reason:   "unknown",
					Message:  "maven.compiler.source and target differ",
					Evidence: evidence,
				}
			}
		}
	}

	// 3. Check java.version property.
	if javaVersion, ok := props["java.version"]; ok {
		resolved, err := resolveValue(javaVersion, props)
		if err != nil {
			return nil, &DetectionError{
				Reason:  "unknown",
				Message: "java.version contains unresolved placeholder: " + javaVersion,
				Evidence: []EvidenceItem{
					{Path: relativePath, Key: "java.version", Value: javaVersion},
				},
			}
		}
		if isValidVersion(resolved) {
			return &Observation{
				Language: "java",
				Tool:     "maven",
				Release:  &resolved,
				Evidence: []EvidenceItem{
					{Path: relativePath, Key: "java.version", Value: resolved},
				},
			}, nil
		}
	}

	// No Java version found.
	return nil, &DetectionError{
		Reason:  "unknown",
		Message: "no Java version configuration found in pom.xml",
	}
}

// parsePom parses a pom.xml file.
func parsePom(path string) (*pomProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pom pomProject
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, err
	}

	return &pom, nil
}

// buildPropertyMap extracts properties from a POM and its local parents.
// Child properties take precedence over parent properties.
func buildPropertyMap(workspace, pomPath string, pom *pomProject, visited map[string]bool) map[string]string {
	// Prevent cycles.
	absPath, _ := filepath.Abs(pomPath)
	if visited[absPath] {
		return make(map[string]string)
	}
	visited[absPath] = true

	props := make(map[string]string)

	// First, load parent properties (if local).
	if pom.Parent != nil {
		parentPath := resolveParentPath(workspace, pomPath, pom.Parent.RelativePath)
		if parentPath != "" && isWithinWorkspace(workspace, parentPath) && fileExists(parentPath) {
			parentPom, err := parsePom(parentPath)
			if err == nil {
				parentProps := buildPropertyMap(workspace, parentPath, parentPom, visited)
				for k, v := range parentProps {
					props[k] = v
				}
			}
		}
	}

	// Override with this POM's properties.
	localProps := parseProperties(pom.Properties.Inner)
	for k, v := range localProps {
		props[k] = v
	}

	return props
}

// resolveParentPath resolves the parent POM path.
func resolveParentPath(workspace, pomPath, relativePath string) string {
	pomDir := filepath.Dir(pomPath)

	if relativePath == "" {
		// Default: ../pom.xml
		relativePath = "../pom.xml"
	}

	// Resolve relative to current pom's directory.
	parentPath := filepath.Join(pomDir, relativePath)

	// If relativePath points to a directory, append pom.xml.
	if info, err := os.Stat(parentPath); err == nil && info.IsDir() {
		parentPath = filepath.Join(parentPath, "pom.xml")
	}

	return parentPath
}

// isWithinWorkspace checks if the path is within the workspace directory.
func isWithinWorkspace(workspace, path string) bool {
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absWorkspace, absPath)
	if err != nil {
		return false
	}
	// Path is within workspace if it doesn't start with ".."
	return !strings.HasPrefix(rel, "..")
}

// parseProperties parses the inner XML of <properties> into a map.
func parseProperties(inner []byte) map[string]string {
	props := make(map[string]string)
	if len(inner) == 0 {
		return props
	}

	// Wrap in a root element for parsing.
	wrapped := "<root>" + string(inner) + "</root>"

	// Use a simple XML decoder to extract elements.
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	var currentKey string
	var inRoot bool

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "root" {
				inRoot = true
			} else if inRoot {
				currentKey = t.Name.Local
			}
		case xml.CharData:
			if currentKey != "" {
				value := strings.TrimSpace(string(t))
				if value != "" {
					props[currentKey] = value
				}
			}
		case xml.EndElement:
			if t.Name.Local == "root" {
				inRoot = false
			}
			currentKey = ""
		}
	}

	return props
}

// resolveValue resolves ${property} references in a value.
// Returns an error if a placeholder cannot be resolved.
func resolveValue(value string, props map[string]string) (string, error) {
	match := propertyRegex.FindStringSubmatch(value)
	if match == nil {
		// No placeholder, return as-is.
		return value, nil
	}

	propName := match[1]
	resolved, ok := props[propName]
	if !ok {
		return "", &DetectionError{
			Reason:  "unknown",
			Message: "unresolved property: " + propName,
		}
	}

	// Recursively resolve in case the value also contains a placeholder.
	return resolveValue(resolved, props)
}

// isValidVersion checks if the version string is a valid integer.
func isValidVersion(v string) bool {
	if v == "" {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// relPath returns the relative path from workspace to the given path.
func relPath(workspace, path string) string {
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return filepath.Base(path)
	}
	return rel
}
