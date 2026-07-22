package stackdetect

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var javaVersionNumberRegex = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

type gradlePropertiesResolver struct {
	workspace  string
	loaded     bool
	properties map[string]string
}

func (r *gradlePropertiesResolver) resolve(ref gradleVersionRef) (string, *EvidenceItem, error) {
	if ref.value != "" {
		return ref.value, nil, nil
	}
	if ref.property == "" {
		return "", nil, nil
	}

	if !r.loaded {
		properties, err := readGradleJavaVersionProperties(r.workspace)
		if err != nil {
			return "", nil, err
		}
		r.properties = properties
		r.loaded = true
	}

	value, ok := r.properties[ref.property]
	if !ok {
		return "", nil, nil
	}
	return value, &EvidenceItem{
		Path:  "gradle.properties",
		Key:   ref.property,
		Value: value,
	}, nil
}

func readGradleJavaVersionProperties(workspace string) (map[string]string, error) {
	const rel = "gradle.properties"
	content, err := os.ReadFile(filepath.Join(workspace, rel))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "failed to read " + rel + ": " + err.Error(),
		}
	}
	return parseGradleJavaVersionProperties(string(content)), nil
}

func parseGradleJavaVersionProperties(content string) map[string]string {
	properties := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		separator := strings.IndexAny(line, "=:")
		if separator < 0 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		if key == "" {
			continue
		}

		value, ok := parseGradlePropertyJavaVersion(line[separator+1:])
		if !ok {
			delete(properties, key)
			continue
		}
		properties[key] = value
	}
	return properties
}

func parseGradlePropertyJavaVersion(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	if !javaVersionNumberRegex.MatchString(value) {
		return "", false
	}
	return normalizeJavaVersion(value), true
}
