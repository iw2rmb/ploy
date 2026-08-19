package contracts

import (
	"fmt"
	"strings"
)

// ManifestReference identifies a workflow manifest by name and version.
type ManifestReference struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Validate requires both name and version to be non‑empty strings.
func (m ManifestReference) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required")
	}
	return nil
}
