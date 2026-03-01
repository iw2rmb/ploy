package config

import (
	"errors"
	"fmt"
	"strings"
)

// validate checks the configuration for required fields and logical consistency.
func validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config: nil configuration")
	}

	if strings.TrimSpace(cfg.Admin.Socket) == "" && strings.TrimSpace(cfg.Admin.Listen) == "" {
		return errors.New("config: admin.socket or admin.listen must be configured")
	}

	// Validate listen addresses.
	if _, err := ValidateAddress(cfg.HTTP.Listen); err != nil {
		return fmt.Errorf("config: http.listen: %w", err)
	}
	if _, err := ValidateAddress(cfg.Metrics.Listen); err != nil {
		return fmt.Errorf("config: metrics.listen: %w", err)
	}
	if strings.TrimSpace(cfg.Admin.Listen) != "" {
		if _, err := ValidateAddress(cfg.Admin.Listen); err != nil {
			return fmt.Errorf("config: admin.listen: %w", err)
		}
	}

	return nil
}
