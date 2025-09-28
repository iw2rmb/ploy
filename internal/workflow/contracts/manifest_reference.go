package contracts

import (
	"fmt"
	"strings"
)

type ManifestReference struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (m ManifestReference) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required")
	}
	return nil
}
