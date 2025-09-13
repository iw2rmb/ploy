package platformnomad

import (
	"embed"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed *.hcl
var hclFS embed.FS

// ListEmbeddedTemplatePaths returns all embedded template paths as
// repository-relative paths like "platform/nomad/<name>.hcl".
func ListEmbeddedTemplatePaths() []string {
	entries, _ := hclFS.ReadDir(".")
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".hcl") {
			out = append(out, filepath.ToSlash(filepath.Join("platform", "nomad", name)))
		}
	}
	sort.Strings(out)
	return out
}

// GetEmbeddedTemplate returns the bytes for a given repository-relative path
// like "platform/nomad/<name>.hcl". Returns nil if not found.
func GetEmbeddedTemplate(path string) []byte {
	base := filepath.Base(path)
	b, err := hclFS.ReadFile(base)
	if err != nil {
		return nil
	}
	return b
}
