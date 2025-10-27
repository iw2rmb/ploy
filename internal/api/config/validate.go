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
	if strings.TrimSpace(cfg.ControlPlane.Endpoint) == "" {
		return errors.New("config: control_plane.endpoint is required")
	}
	if strings.TrimSpace(cfg.ControlPlane.CAPath) == "" {
		return errors.New("config: control_plane.ca is required")
	}
	if strings.TrimSpace(cfg.ControlPlane.Certificate) == "" {
		return errors.New("config: control_plane.certificate is required")
	}
	if strings.TrimSpace(cfg.ControlPlane.Key) == "" {
		return errors.New("config: control_plane.key is required")
	}

	if cfg.HTTP.TLS.Enabled {
		if strings.TrimSpace(cfg.HTTP.TLS.CertPath) == "" {
			return errors.New("config: http.tls.cert required when TLS enabled")
		}
		if strings.TrimSpace(cfg.HTTP.TLS.KeyPath) == "" {
			return errors.New("config: http.tls.key required when TLS enabled")
		}
	}

	if strings.TrimSpace(cfg.Admin.Socket) == "" && strings.TrimSpace(cfg.Admin.Listen) == "" {
		return errors.New("config: admin.socket or admin.listen must be configured")
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
