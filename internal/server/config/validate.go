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
	// Note: control_plane config is NOT required for servers (ployd).
	// The server IS the control plane; only nodes (ployd-node) need control_plane config.
	// The ControlPlaneConfig fields are kept in the struct for backwards compatibility
	// and potential future use, but are not validated or required.

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

	// Validate runtime plugins.
	names := make(map[string]struct{}, len(cfg.Runtime.Plugins))
	for _, plugin := range cfg.Runtime.Plugins {
		if strings.TrimSpace(plugin.Name) == "" {
			return errors.New("config: runtime plugin name required")
		}
		if _, exists := names[plugin.Name]; exists {
			return fmt.Errorf("config: duplicate runtime plugin %q", plugin.Name)
		}
		names[plugin.Name] = struct{}{}
	}
	if len(cfg.Runtime.Plugins) > 0 {
		if _, exists := names[cfg.Runtime.DefaultAdapter]; !exists {
			return fmt.Errorf("config: runtime.default_adapter %q not registered", cfg.Runtime.DefaultAdapter)
		}
	}

	if strings.TrimSpace(cfg.Transfers.BaseDir) == "" {
		return errors.New("config: transfers.base_dir is required")
	}
	if strings.TrimSpace(cfg.Transfers.GuardBinary) == "" {
		return errors.New("config: transfers.guard_binary is required")
	}
	if cfg.Transfers.JanitorInterval <= 0 {
		return errors.New("config: transfers.janitor_interval must be positive")
	}
	return nil
}
