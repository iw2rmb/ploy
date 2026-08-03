package specdiscovery

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry contains the root metadata needed to identify a named spec.
type Entry struct {
	Name        string
	Description string
}

// ProbeYAML accepts only root mappings with apiVersion ploy.mig/v1alpha1 and a
// non-empty scalar name. Nested fields do not participate in discovery.
func ProbeYAML(content []byte) (Entry, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return Entry{}, false
	}
	root := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return Entry{}, false
	}

	var apiVersion string
	var entry Entry
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		value := root.Content[i+1]
		if key.Kind != yaml.ScalarNode || value.Kind != yaml.ScalarNode {
			continue
		}
		switch key.Value {
		case "apiVersion":
			apiVersion = strings.TrimSpace(value.Value)
		case "name":
			entry.Name = strings.TrimSpace(value.Value)
		case "description":
			entry.Description = strings.TrimSpace(value.Value)
		}
	}
	if apiVersion != "ploy.mig/v1alpha1" || entry.Name == "" {
		return Entry{}, false
	}
	return entry, true
}
