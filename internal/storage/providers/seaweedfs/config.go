package seaweedfs

import "time"

// Config represents SeaweedFS-specific configuration
type Config struct {
	Master      string `yaml:"master" json:"master"`           // master server address (e.g., "localhost:9333")
	Filer       string `yaml:"filer" json:"filer"`             // filer server address (e.g., "localhost:8888")
	Collection  string `yaml:"collection" json:"collection"`   // collection name for artifacts
	Replication string `yaml:"replication" json:"replication"` // replication strategy (e.g., "000" for dev, "001" for prod)
	Timeout     int    `yaml:"timeout" json:"timeout"`         // timeout in seconds
	DataCenter  string `yaml:"datacenter" json:"datacenter"`   // data center identifier
	Rack        string `yaml:"rack" json:"rack"`               // rack identifier
}

// DefaultConfig returns a sensible default configuration for SeaweedFS
func DefaultConfig() Config {
	return Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "artifacts",
		Replication: "000", // Changed to 000 for single-node dev environment
		Timeout:     30,
	}
}

// Validate checks if the configuration is valid
func (c Config) Validate() error {
	if c.Master == "" {
		return &ConfigError{Field: "master", Message: "master address is required"}
	}
	if c.Filer == "" {
		return &ConfigError{Field: "filer", Message: "filer address is required"}
	}
	return nil
}

// TimeoutDuration returns the timeout as a time.Duration
func (c Config) TimeoutDuration() time.Duration {
	if c.Timeout <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.Timeout) * time.Second
}

// ConfigError represents a configuration validation error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "seaweedfs config error: " + e.Field + " - " + e.Message
}
