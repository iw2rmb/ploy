package security

import (
	"fmt"
	"os"
	"strconv"
	"time"

	recipes "github.com/iw2rmb/ploy/api/recipes"
	internalstorage "github.com/iw2rmb/ploy/internal/storage"
)

// Config represents the ARF system configuration
type Config struct {
	Storage StorageConfig `yaml:"storage" json:"storage"`
	Index   IndexConfig   `yaml:"index" json:"index"`
	NVD     NVDConfig     `yaml:"nvd" json:"nvd"`
}

// StorageConfig configures recipe storage backend
type StorageConfig struct {
	Backend    string          `yaml:"backend" json:"backend"` // "seaweedfs", "memory"
	SeaweedFS  SeaweedFSConfig `yaml:"seaweedfs" json:"seaweedfs"`
	BucketName string          `yaml:"bucket_name" json:"bucket_name"`
	KeyPrefix  string          `yaml:"key_prefix" json:"key_prefix"`
	CacheTTL   time.Duration   `yaml:"cache_ttl" json:"cache_ttl"`
	Timeout    time.Duration   `yaml:"timeout" json:"timeout"`
	MaxRetries int             `yaml:"max_retries" json:"max_retries"`
}

// SeaweedFSConfig configures SeaweedFS connection
type SeaweedFSConfig struct {
	MasterURL string `yaml:"master_url" json:"master_url"`
	FilerURL  string `yaml:"filer_url" json:"filer_url"`
}

// IndexConfig configures recipe indexing backend
type IndexConfig struct {
	Backend         string        `yaml:"backend" json:"backend"` // "consul", "memory"
	ConsulAddr      string        `yaml:"consul_addr" json:"consul_addr"`
	KeyPrefix       string        `yaml:"key_prefix" json:"key_prefix"`
	BuildOnStartup  bool          `yaml:"build_on_startup" json:"build_on_startup"`
	RefreshInterval time.Duration `yaml:"refresh_interval" json:"refresh_interval"`
}

// NVDConfig configures the NVD CVE database integration
type NVDConfig struct {
	Enabled bool          `yaml:"enabled" json:"enabled"`
	APIKey  string        `yaml:"api_key" json:"api_key"`
	BaseURL string        `yaml:"base_url" json:"base_url"`
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// DefaultConfig returns the default ARF configuration
func DefaultConfig() *Config {
	return &Config{
		Storage: StorageConfig{
			Backend:    "seaweedfs",
			BucketName: "ploy-recipes",
			KeyPrefix:  "recipes",
			CacheTTL:   5 * time.Minute,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
			SeaweedFS: SeaweedFSConfig{
				MasterURL: "http://localhost:9333",
				FilerURL:  "http://localhost:8888",
			},
		},
		Index: IndexConfig{
			Backend:         "memory",
			ConsulAddr:      "localhost:8500",
			KeyPrefix:       "ploy/arf/recipes",
			BuildOnStartup:  true,
			RefreshInterval: 10 * time.Minute,
		},
		NVD: NVDConfig{
			Enabled: true,
			APIKey:  "",
			BaseURL: "https://services.nvd.nist.gov/rest/json/cves/2.0",
			Timeout: 30 * time.Second,
		},
	}
}

// ProductionConfig returns a production-ready ARF configuration
func ProductionConfig() *Config {
	config := DefaultConfig()

	// Use SeaweedFS + Consul in production
	config.Storage.Backend = "seaweedfs"
	config.Index.Backend = "consul"

	return config
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate storage config
	if c.Storage.Backend == "" {
		return fmt.Errorf("storage backend is required")
	}

	if c.Storage.Backend == "seaweedfs" {
		if c.Storage.SeaweedFS.MasterURL == "" {
			return fmt.Errorf("SeaweedFS master URL is required")
		}
		if c.Storage.SeaweedFS.FilerURL == "" {
			return fmt.Errorf("SeaweedFS filer URL is required")
		}
	}

	// Validate index config
	if c.Index.Backend == "" {
		return fmt.Errorf("index backend is required")
	}

	if c.Index.Backend == "consul" && c.Index.ConsulAddr == "" {
		return fmt.Errorf("consul address is required")
	}

	return nil
}

// InitializeStorage creates and configures the storage backend from config
func (c *Config) InitializeStorage() (recipes.RecipeStorage, error) {
	switch c.Storage.Backend {
	case "seaweedfs":
		// Create SeaweedFS client
		storageConfig := internalstorage.Config{
			Master:      c.Storage.SeaweedFS.MasterURL,
			Filer:       c.Storage.SeaweedFS.FilerURL,
			Collection:  c.Storage.BucketName,
			Replication: "000", // Use 000 for single-node dev environment
			Timeout:     int(c.Storage.Timeout.Seconds()),
		}

		client, err := internalstorage.New(storageConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create SeaweedFS client: %w", err)
		}

		// Create RecipeRegistry with SeaweedFS storage provider and expose it
		// via a RecipeStorage-compatible adapter (SeaweedFS only; no memory fallback)
		registry := recipes.NewRecipeRegistry(client)
		return recipes.NewRegistryStorageAdapter(registry), nil

	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", c.Storage.Backend)
	}
}

// InitializeIndex creates and configures the index backend from config
func (c *Config) InitializeIndex() (recipes.RecipeIndexStore, error) {
	switch c.Index.Backend {
	case "consul":
		// TODO: Implement NewConsulRecipeIndex
		// return NewConsulRecipeIndex(c.Index.ConsulAddr, c.Index.KeyPrefix)
		return nil, fmt.Errorf("consul index not yet migrated")
	case "memory":
		// Memory index is handled internally by storage
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported index backend: %s", c.Index.Backend)
	}
}

// InitializeValidator exists for compatibility; recipe-level validation is no longer provided here.
func (c *Config) InitializeValidator() recipes.RecipeValidatorInterface {
	return nil
}

// LoadConfigFromEnv loads ARF configuration from environment variables
func LoadConfigFromEnv() *Config {
	// Use production config in production environment
	if os.Getenv("PLOY_ENVIRONMENT") == "production" {
		config := ProductionConfig()

		// Override with environment-specific values
		if seaweedfsURL := os.Getenv("ARF_SEAWEEDFS_MASTER_URL"); seaweedfsURL != "" {
			config.Storage.SeaweedFS.MasterURL = seaweedfsURL
		}
		if seaweedfsURL := os.Getenv("ARF_SEAWEEDFS_FILER_URL"); seaweedfsURL != "" {
			config.Storage.SeaweedFS.FilerURL = seaweedfsURL
		}
		if consulAddr := os.Getenv("ARF_CONSUL_ADDR"); consulAddr != "" {
			config.Index.ConsulAddr = consulAddr
		}
		if keyPrefix := os.Getenv("ARF_CONSUL_PREFIX"); keyPrefix != "" {
			config.Index.KeyPrefix = keyPrefix
		}

		// NVD configuration
		if enabled := os.Getenv("NVD_ENABLED"); enabled != "" {
			config.NVD.Enabled = enabled == "1" || enabled == "true" || enabled == "TRUE" || enabled == "yes"
		}
		if apiKey := os.Getenv("NVD_API_KEY"); apiKey != "" {
			config.NVD.APIKey = apiKey
		}
		if baseURL := os.Getenv("NVD_BASE_URL"); baseURL != "" {
			config.NVD.BaseURL = baseURL
		}
		if to := os.Getenv("NVD_TIMEOUT_MS"); to != "" {
			if ms, err := strconv.Atoi(to); err == nil && ms > 0 {
				config.NVD.Timeout = time.Duration(ms) * time.Millisecond
			}
		}

		return config
	}

	// Use default config with environment overrides for development/test
	config := DefaultConfig()

	// Storage backend selection
	if backend := os.Getenv("ARF_STORAGE_BACKEND"); backend != "" {
		config.Storage.Backend = backend
	}
	if backend := os.Getenv("ARF_INDEX_BACKEND"); backend != "" {
		config.Index.Backend = backend
	}

	// SeaweedFS configuration
	if seaweedfsURL := os.Getenv("ARF_SEAWEEDFS_MASTER_URL"); seaweedfsURL != "" {
		config.Storage.SeaweedFS.MasterURL = seaweedfsURL
	}
	if seaweedfsURL := os.Getenv("ARF_SEAWEEDFS_FILER_URL"); seaweedfsURL != "" {
		config.Storage.SeaweedFS.FilerURL = seaweedfsURL
	}

	// Consul configuration
	if consulAddr := os.Getenv("ARF_CONSUL_ADDR"); consulAddr != "" {
		config.Index.ConsulAddr = consulAddr
	}
	if keyPrefix := os.Getenv("ARF_CONSUL_PREFIX"); keyPrefix != "" {
		config.Index.KeyPrefix = keyPrefix
	}

	return config
}
