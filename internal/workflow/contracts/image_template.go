package contracts

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	imageTemplateStackPlaceholderRE = regexp.MustCompile(`\$\{stack\.([^}]+)\}`)
	imageTemplateEnvPlaceholderRE   = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
)

// ExpandImageTemplate expands stack and env placeholders in image templates.
//
// Supported stack placeholders:
//   - ${stack.language}
//   - ${stack.release}
//   - ${stack.tool}
//
// Supported env placeholders:
//   - $VAR
//   - ${VAR}
//
// Returns an error when:
//   - an unknown stack placeholder is used
//   - a required stack value is unavailable
//   - an environment variable placeholder is unresolved
func ExpandImageTemplate(image string, stack *StackExpectation) (string, error) {
	return expandImageTemplateWithLookup(image, stack, os.LookupEnv)
}

func expandImageTemplateWithLookup(
	image string,
	stack *StackExpectation,
	lookup func(string) (string, bool),
) (string, error) {
	stack = NormalizeStackExpectation(stack)

	unknownStack := map[string]struct{}{}
	missingStack := map[string]struct{}{}

	stackExpanded := imageTemplateStackPlaceholderRE.ReplaceAllStringFunc(image, func(match string) string {
		parts := imageTemplateStackPlaceholderRE.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		key := strings.TrimSpace(parts[1])
		placeholder := "stack." + key

		var value string
		switch key {
		case "language":
			if stack != nil {
				value = stack.Language
			}
		case "release":
			if stack != nil {
				value = stack.Release
			}
		case "tool":
			if stack != nil {
				value = stack.Tool
			}
		default:
			unknownStack[placeholder] = struct{}{}
			return ""
		}

		if strings.TrimSpace(value) == "" {
			missingStack[placeholder] = struct{}{}
			return ""
		}
		return value
	})

	if len(unknownStack) > 0 {
		return "", fmt.Errorf("unknown stack placeholders: %s", joinSortedKeys(unknownStack))
	}
	if len(missingStack) > 0 {
		return "", fmt.Errorf("unresolved stack placeholders: %s", joinSortedKeys(missingStack))
	}

	missingEnv := map[string]struct{}{}
	envExpanded := imageTemplateEnvPlaceholderRE.ReplaceAllStringFunc(stackExpanded, func(match string) string {
		var name string
		if strings.HasPrefix(match, "${") {
			name = strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		} else {
			name = strings.TrimPrefix(match, "$")
		}
		if value, ok := lookup(name); ok {
			return value
		}
		missingEnv[name] = struct{}{}
		return ""
	})

	if len(missingEnv) > 0 {
		return "", fmt.Errorf("unresolved environment variables: %s", joinSortedKeys(missingEnv))
	}

	return envExpanded, nil
}

func joinSortedKeys(values map[string]struct{}) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
