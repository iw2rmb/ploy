package contracts

import (
	"fmt"
	"strings"
)

// StageName identifies a workflow stage by name.
//
// It is a distinct type to prevent mixing arbitrary strings with stage
// identifiers in contracts while preserving JSON compatibility (marshals as a
// plain string).
type StageName string

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
