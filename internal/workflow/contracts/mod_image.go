// job_image.go provides stack-aware image resolution for Mods specs.
//
// This file implements the JobImage type that supports two canonical forms:
//   - Universal image (string): A single image used for all build stacks.
//   - Stack-specific image (map): Different images per detected build stack.
//
// Both forms are intentionally supported as first-class citizens of the Mods
// spec schema. The dual-form design enables:
//   - Simple configurations using a single image string for stack-agnostic mods.
//   - Optimized configurations using stack-specific images for tools like Maven
//     or Gradle that benefit from dedicated container environments.
//
// ## Stack Resolution Rules
//
// When resolving an image for a given stack:
//  1. If JobImage is a string, return that string (universal image).
//  2. If JobImage is a map:
//     a. Prefer an exact stack key match (e.g., "java-maven", "java-gradle").
//     b. Fall back to "default" when present and no exact match exists.
//     c. Return an error when neither a matching key nor "default" is present.
//
// ## Supported Stack Names
//
// The following stack names are recognized for image resolution:
//   - "java-maven": Maven-based Java projects (pom.xml detected)
//   - "java-gradle": Gradle-based Java projects (build.gradle detected)
//   - "java": Generic Java projects (no build tool detected)
//   - "unknown": No recognized stack markers found
//   - "default": Fallback key in stack maps
package contracts

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ModStack represents a detected build stack for image resolution.
type ModStack string

const (
	// ModStackJavaMaven indicates a Maven-based Java project (pom.xml present).
	ModStackJavaMaven ModStack = "java-maven"

	// ModStackJavaGradle indicates a Gradle-based Java project (build.gradle present).
	ModStackJavaGradle ModStack = "java-gradle"

	// ModStackJava indicates a generic Java project (no specific build tool).
	ModStackJava ModStack = "java"

	// ModStackUnknown indicates no recognized stack markers were found.
	ModStackUnknown ModStack = "unknown"

	// ModStackDefault is the fallback key used in stack-specific image maps.
	ModStackDefault ModStack = "default"
)

// JobImage represents a mod container image specification supporting two
// canonical forms: universal images (single string) and stack-specific images
// (map by stack). Both forms are first-class schema options.
//
// YAML/JSON Examples:
//
//	# Universal image (string form) — used for all stacks:
//	image: docker.io/user/mods-openrewrite:latest
//
//	# Stack-specific images (map form) — per-stack optimization:
//	image:
//	  default: docker.io/user/mods-openrewrite:latest
//	  java-maven: docker.io/user/mods-orw-maven:latest
//	  java-gradle: docker.io/user/mods-orw-gradle:latest
type JobImage struct {
	// Universal holds the image when specified as a single string.
	// When non-empty, this image is used regardless of detected stack.
	Universal string

	// ByStack holds stack-specific images when image is specified as a map.
	// Keys are stack names (e.g., "java-maven", "java-gradle", "default").
	// When non-nil and non-empty, stack resolution rules apply.
	ByStack map[ModStack]string
}

// IsEmpty returns true if no image is specified in either form.
func (m JobImage) IsEmpty() bool {
	return m.Universal == "" && len(m.ByStack) == 0
}

// IsUniversal returns true if the image is specified as a universal string.
func (m JobImage) IsUniversal() bool {
	return m.Universal != "" && len(m.ByStack) == 0
}

// IsStackSpecific returns true if the image is specified as a stack map.
func (m JobImage) IsStackSpecific() bool {
	return len(m.ByStack) > 0
}

// ResolveImage resolves the image for the given stack using resolution rules:
//  1. If JobImage is a universal string, return that string.
//  2. If JobImage is a stack map:
//     a. Prefer an exact stack key match.
//     b. Fall back to "default" when present.
//     c. Return an error when neither exists.
//
// The stack parameter should come from Build Gate detection (e.g., "java-maven").
// An empty JobImage returns an error. An empty stack uses "unknown" as default.
func (m JobImage) ResolveImage(stack ModStack) (string, error) {
	// Normalize empty stack to "unknown".
	if stack == "" {
		stack = ModStackUnknown
	}

	// Case 1: Universal image - return it regardless of stack.
	if m.Universal != "" {
		return m.Universal, nil
	}

	// Case 2: Empty image spec - error.
	if len(m.ByStack) == 0 {
		return "", fmt.Errorf("image not specified")
	}

	// Case 3: Stack map - try exact match first.
	if img, ok := m.ByStack[stack]; ok && strings.TrimSpace(img) != "" {
		return img, nil
	}

	// Case 4: Stack map - fall back to "default" key.
	if img, ok := m.ByStack[ModStackDefault]; ok && strings.TrimSpace(img) != "" {
		return img, nil
	}

	// Case 5: No matching key and no default - fail with actionable error.
	return "", fmt.Errorf(
		"no image specified for stack %q and no default provided; "+
			"add a %q key to the image map or specify the exact stack key",
		stack, ModStackDefault,
	)
}

// ParseJobImage parses an image specification from an untyped value.
// Both canonical forms are accepted:
//   - string: Parsed as a universal image (used for all stacks).
//   - map[string]any or map[string]string: Parsed as stack-specific images.
//
// Returns an empty JobImage for nil input without error.
func ParseJobImage(v any) (JobImage, error) {
	if v == nil {
		return JobImage{}, nil
	}

	// Case 1: String - universal image.
	if s, ok := v.(string); ok {
		return JobImage{Universal: strings.TrimSpace(s)}, nil
	}

	// Case 2: Map - stack-specific images.
	// Handle both map[string]any (from JSON/YAML) and map[string]string.
	switch m := v.(type) {
	case map[string]any:
		byStack := make(map[ModStack]string, len(m))
		for k, val := range m {
			img, ok := val.(string)
			if !ok {
				return JobImage{}, fmt.Errorf(
					"image[%q]: expected string, got %T", k, val,
				)
			}
			byStack[ModStack(strings.TrimSpace(k))] = strings.TrimSpace(img)
		}
		return JobImage{ByStack: byStack}, nil

	case map[string]string:
		byStack := make(map[ModStack]string, len(m))
		for k, img := range m {
			byStack[ModStack(strings.TrimSpace(k))] = strings.TrimSpace(img)
		}
		return JobImage{ByStack: byStack}, nil

	default:
		return JobImage{}, fmt.Errorf(
			"image: expected string or map, got %T", v,
		)
	}
}

// String returns a human-readable representation for debugging.
func (m JobImage) String() string {
	if m.Universal != "" {
		return m.Universal
	}
	if len(m.ByStack) == 0 {
		return "<empty>"
	}
	// Build a compact representation for stack maps.
	var parts []string
	for stack, img := range m.ByStack {
		parts = append(parts, fmt.Sprintf("%s=%s", stack, img))
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}

// MarshalJSON implements json.Marshaler for JobImage.
// Serializes as a string when Universal is set, or as a map when ByStack is set.
func (m JobImage) MarshalJSON() ([]byte, error) {
	if m.Universal != "" {
		return json.Marshal(m.Universal)
	}
	if len(m.ByStack) > 0 {
		result := make(map[string]string, len(m.ByStack))
		for k, v := range m.ByStack {
			result[string(k)] = v
		}
		return json.Marshal(result)
	}
	return json.Marshal(nil)
}

// UnmarshalJSON implements json.Unmarshaler for JobImage.
// Accepts both string and map forms from JSON.
func (m *JobImage) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	// Try string first (universal form).
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		m.Universal = strings.TrimSpace(s)
		return nil
	}

	// Try map (stack-specific form).
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err == nil {
		m.ByStack = make(map[ModStack]string, len(raw))
		for k, v := range raw {
			m.ByStack[ModStack(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
		return nil
	}

	return fmt.Errorf("image: expected string or map, got %s", string(data))
}

// ToolToModStack converts a Build Gate tool name to a ModStack constant.
// Tool names come from BuildGateStaticCheckReport.Tool after gate execution.
//
// Conversion rules:
//   - "maven" → ModStackJavaMaven
//   - "gradle" → ModStackJavaGradle
//   - "java" → ModStackJava
//   - "" or unknown → ModStackUnknown
//
// This function enables deterministic stack-aware image selection after
// Build Gate detection, ensuring Mods steps use the correct stack-specific
// images based on the workspace's detected build system.
func ToolToModStack(tool string) ModStack {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "maven":
		return ModStackJavaMaven
	case "gradle":
		return ModStackJavaGradle
	case "java":
		return ModStackJava
	default:
		return ModStackUnknown
	}
}
