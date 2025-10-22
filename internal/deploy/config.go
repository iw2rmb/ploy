package deploy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// BootstrapConfig describes remote host bootstrap parameters loaded from YAML.
type BootstrapConfig struct {
	Host          string `yaml:"host"`
	User          string `yaml:"user"`
	Port          int    `yaml:"port"`
	IdentityFile  string `yaml:"identity_file"`
	MinDiskGB     int    `yaml:"min_disk_gb"`
	RequiredPorts []int  `yaml:"required_ports"`
}

// LoadBootstrapConfig reads and normalises bootstrap configuration from disk.
func LoadBootstrapConfig(path string) (BootstrapConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BootstrapConfig{}, fmt.Errorf("bootstrap: read config: %w", err)
	}
	if len(raw) == 0 {
		return BootstrapConfig{}, fmt.Errorf("bootstrap: config %s is empty", path)
	}
	var cfg BootstrapConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return BootstrapConfig{}, fmt.Errorf("bootstrap: parse config: %w", err)
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultSSHPort
	}
	if cfg.MinDiskGB == 0 {
		cfg.MinDiskGB = DefaultMinDiskGB
	}
	if len(cfg.RequiredPorts) == 0 {
		cfg.RequiredPorts = append([]int(nil), defaultRequiredPorts...)
	}
	return cfg, nil
}

// ToOptions projects the config into bootstrap runtime options.
func (cfg BootstrapConfig) ToOptions() Options {
	opts := Options{
		Host:          cfg.Host,
		User:          cfg.User,
		Port:          cfg.Port,
		IdentityFile:  cfg.IdentityFile,
		MinDiskGB:     cfg.MinDiskGB,
		RequiredPorts: append([]int(nil), cfg.RequiredPorts...),
	}
	return opts
}
