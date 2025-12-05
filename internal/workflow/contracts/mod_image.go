// mod_image.go provides stack-aware image resolution for Mods specs.
//
// This file implements the ModImage type that supports both universal images
// (a single string) and stack-specific images (a map keyed by stack name).
// The type preserves backward compatibility with existing single-string specs
// while enabling new stack-aware image selection based on Build Gate detection.
//
// ## Stack Resolution Rules
//
// When resolving an image for a given stack:
//  1. If ModImage is a string, return that string (universal image).
//  2. If ModImage is a map:
//     a. Prefer an exact stack key match (e.g., "java-maven", "java-gradle").
//     b. Fall back to "default" when present and no exact match exists.
//     c. Return an error when neither a matching key nor "default" is present.
//
// ## Supported Stack Names
//
// The following stack names are recognized and correspond to Build Gate profiles:
//   - "java-maven": Maven-based Java projects (pom.xml detected)
//   - "java-gradle": Gradle-based Java projects (build.gradle detected)
//   - "java": Generic Java projects (no build tool detected)
//   - "unknown": No recognized stack markers found
//   - "default": Fallback key in stack maps
package contracts

import (
	"fmt"
	"strings"
)

// ModStack represents a detected build stack for image resolution.
// Stack values correspond to Build Gate profiles used during validation.
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

// ModImage represents a mod container image specification that supports both
// universal images (single string) and stack-specific images (map by stack).
//
// YAML/JSON Examples:
//
//	# Universal image (string form):
//	image: docker.io/user/mods-openrewrite:latest
//
//	# Stack-specific images (map form):
//	image:
//	  default: docker.io/user/mods-openrewrite:latest
//	  java-maven: docker.io/user/mods-orw-maven:latest
//	  java-gradle: docker.io/user/mods-orw-gradle:latest
type ModImage struct {
	// Universal holds the image when specified as a single string.
	// When non-empty, this image is used for all stacks (backward compatible).
	Universal string

	// ByStack holds stack-specific images when image is specified as a map.
	// Keys are stack names (e.g., "java-maven", "java-gradle", "default").
	// When non-nil and non-empty, stack resolution rules apply.
	ByStack map[ModStack]string
}

// IsEmpty returns true if no image is specified in either form.
func (m ModImage) IsEmpty() bool {
	return m.Universal == "" && len(m.ByStack) == 0
}

// IsUniversal returns true if the image is specified as a universal string.
func (m ModImage) IsUniversal() bool {
	return m.Universal != "" && len(m.ByStack) == 0
}

// IsStackSpecific returns true if the image is specified as a stack map.
func (m ModImage) IsStackSpecific() bool {
	return len(m.ByStack) > 0
}

// ResolveImage resolves the image for the given stack using resolution rules:
//  1. If ModImage is a universal string, return that string.
//  2. If ModImage is a stack map:
//     a. Prefer an exact stack key match.
//     b. Fall back to "default" when present.
//     c. Return an error when neither exists.
//
// The stack parameter should come from Build Gate detection (e.g., "java-maven").
// An empty ModImage returns an error. An empty stack uses "unknown" as default.
func (m ModImage) ResolveImage(stack ModStack) (string, error) {
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

// ParseModImage parses an image specification from an untyped value.
// Accepts either a string (universal image) or map[string]any (stack map).
// Returns an empty ModImage for nil input without error.
func ParseModImage(v any) (ModImage, error) {
	if v == nil {
		return ModImage{}, nil
	}

	// Case 1: String - universal image.
	if s, ok := v.(string); ok {
		return ModImage{Universal: strings.TrimSpace(s)}, nil
	}

	// Case 2: Map - stack-specific images.
	// Handle both map[string]any (from JSON/YAML) and map[string]string.
	switch m := v.(type) {
	case map[string]any:
		byStack := make(map[ModStack]string, len(m))
		for k, val := range m {
			img, ok := val.(string)
			if !ok {
				return ModImage{}, fmt.Errorf(
					"image[%q]: expected string, got %T", k, val,
				)
			}
			byStack[ModStack(strings.TrimSpace(k))] = strings.TrimSpace(img)
		}
		return ModImage{ByStack: byStack}, nil

	case map[string]string:
		byStack := make(map[ModStack]string, len(m))
		for k, img := range m {
			byStack[ModStack(strings.TrimSpace(k))] = strings.TrimSpace(img)
		}
		return ModImage{ByStack: byStack}, nil

	default:
		return ModImage{}, fmt.Errorf(
			"image: expected string or map, got %T", v,
		)
	}
}

// String returns a human-readable representation for debugging.
func (m ModImage) String() string {
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
