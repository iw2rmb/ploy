package storage

import (
	"fmt"
)

// Config represents the storage configuration
type Config struct {
	// SeaweedFS Master server address (e.g., "localhost:9333")
	Master string `yaml:"master"`
	
	// SeaweedFS Filer server address (e.g., "localhost:8888")
	Filer string `yaml:"filer"`
	
	// Collection name for artifacts (e.g., "ploy-artifacts")
	Collection string `yaml:"collection"`
	
	// Replication strategy (e.g., "001")
	Replication string `yaml:"replication"`
	
	// Timeout in seconds for requests
	Timeout int `yaml:"timeout"`
	
	// Data center identifier
	DataCenter string `yaml:"datacenter"`
	
	// Rack identifier  
	Rack string `yaml:"rack"`
	
	// Collection organization for different artifact types
	Collections struct {
		Artifacts string `yaml:"artifacts"`
		Metadata  string `yaml:"metadata"`
		Debug     string `yaml:"debug"`
	} `yaml:"collections"`
}

// Client is the simplified SeaweedFS-only storage client
type Client = SeaweedFSClient

// New creates a new SeaweedFS storage client
func New(cfg Config) (*Client, error) {
	if cfg.Master == "" {
		return nil, fmt.Errorf("seaweedfs master address is required")
	}
	if cfg.Filer == "" {
		return nil, fmt.Errorf("seaweedfs filer address is required")
	}

	// Convert old config format to SeaweedFS client config
	seaweedCfg := SeaweedFSConfig{
		Master:      cfg.Master,
		Filer:       cfg.Filer,
		Collection:  cfg.Collection,
		Replication: cfg.Replication,
		Timeout:     cfg.Timeout,
		DataCenter:  cfg.DataCenter,
		Rack:        cfg.Rack,
	}

	// Use collections.artifacts if available, otherwise fall back to collection
	if cfg.Collections.Artifacts != "" {
		seaweedCfg.Collection = cfg.Collections.Artifacts
	}

	return NewSeaweedFSClient(seaweedCfg)
}

// NewFromYAML creates a client from YAML config file (for backward compatibility)
func NewFromYAML(configPath string) (*Client, error) {
	// This would typically load YAML config, but for now we'll return an error
	// directing users to use the new simplified config
	return nil, fmt.Errorf("YAML config loading not implemented in simplified storage - use direct Config struct")
}