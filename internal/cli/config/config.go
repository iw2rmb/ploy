package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func configBaseDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("descriptor: find home: %w", err)
		}
		base = filepath.Join(home, ".config", "ploy")
	}
	return base, nil
}
