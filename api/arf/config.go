package arf

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/api/arf/validation"
	internalstorage "github.com/iw2rmb/ploy/internal/storage"
)

// RecipeValidatorAdapter adapts validation.RecipeValidator to RecipeValidatorInterface
type RecipeValidatorAdapter struct {
	validator *validation.RecipeValidator
}

func (a *RecipeValidatorAdapter) ValidateRecipe(ctx context.Context, recipe *models.Recipe) error {
	return a.validator.ValidateRecipe(ctx, recipe)
}

func (a *RecipeValidatorAdapter) ValidateStructure(recipe *models.Recipe) error {
	return a.validator.ValidateRecipe(context.Background(), recipe) // Use base validation for now
}

func (a *RecipeValidatorAdapter) ValidateTransformations(recipe *models.Recipe) error {
	return a.validator.ValidateRecipe(context.Background(), recipe) // Use base validation for now
}

func (a *RecipeValidatorAdapter) ValidateSecurityRules(recipe *models.Recipe) error {
	return a.validator.ValidateSecurityRules(recipe) // Use proper method
}

func (a *RecipeValidatorAdapter) ValidateDependencies(recipe *models.Recipe) error {
	return a.validator.ValidateDependencies(recipe) // Use proper method
}

func (a *RecipeValidatorAdapter) ValidateAgainstSchema(recipe *models.Recipe, schema interface{}) error {
	return a.validator.ValidateSchema(recipe)
}

func (a *RecipeValidatorAdapter) ValidateCompatibility(recipe *models.Recipe, targetVersion string) error {
	return a.validator.ValidateRecipe(context.Background(), recipe) // Use base validation for now
}

func (a *RecipeValidatorAdapter) ValidateSyntax(recipe *models.Recipe) error {
	return a.validator.ValidateRecipe(context.Background(), recipe) // Use base validation for now
}

func (a *RecipeValidatorAdapter) ValidateSchema(recipe *models.Recipe) error {
	return a.validator.ValidateSchema(recipe)
}

func (a *RecipeValidatorAdapter) GetSchemaVersion() string {
	return a.validator.GetSchemaVersion()
}

// Config represents the ARF system configuration
type Config struct {
	Storage    StorageConfig    `yaml:"storage" json:"storage"`
	Index      IndexConfig      `yaml:"index" json:"index"`
	Validation ValidationConfig `yaml:"validation" json:"validation"`
	Security   SecurityConfig   `yaml:"security" json:"security"`
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

// ValidationConfig configures recipe validation
type ValidationConfig struct {
	Enabled       bool                `yaml:"enabled" json:"enabled"`
	SecurityRules SecurityRulesConfig `yaml:"security_rules" json:"security_rules"`
	SchemaStrict  bool                `yaml:"schema_strict" json:"schema_strict"`
}

// SecurityRulesConfig configures security validation rules
type SecurityRulesConfig struct {
	AllowedCommands      []string      `yaml:"allowed_commands" json:"allowed_commands"`
	ForbiddenCommands    []string      `yaml:"forbidden_commands" json:"forbidden_commands"`
	MaxExecutionTime     time.Duration `yaml:"max_execution_time" json:"max_execution_time"`
	AllowNetworkAccess   bool          `yaml:"allow_network_access" json:"allow_network_access"`
	AllowFileSystemWrite bool          `yaml:"allow_file_system_write" json:"allow_file_system_write"`
	SandboxRequired      bool          `yaml:"sandbox_required" json:"sandbox_required"`
	MaxMemoryUsageMB     int64         `yaml:"max_memory_usage_mb" json:"max_memory_usage_mb"`
	MaxCPUUsagePercent   float64       `yaml:"max_cpu_usage_percent" json:"max_cpu_usage_percent"`
}

// SecurityConfig configures general security settings
type SecurityConfig struct {
	EnableEncryption bool   `yaml:"enable_encryption" json:"enable_encryption"`
	EnableAuditLog   bool   `yaml:"enable_audit_log" json:"enable_audit_log"`
	AuditLogPath     string `yaml:"audit_log_path" json:"audit_log_path"`
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
		Validation: ValidationConfig{
			Enabled:      true,
			SchemaStrict: false,
			SecurityRules: SecurityRulesConfig{
				AllowedCommands: []string{
					"java", "javac", "mvn", "gradle",
					"go", "go build", "go test",
					"python", "pip",
					"npm", "yarn", "node",
				},
				ForbiddenCommands: []string{
					"rm -rf", "sudo", "su", "chmod 777",
					"curl", "wget", "ssh", "scp",
				},
				MaxExecutionTime:     15 * time.Minute,
				AllowNetworkAccess:   false,
				AllowFileSystemWrite: true,
				SandboxRequired:      true,
				MaxMemoryUsageMB:     2048,
				MaxCPUUsagePercent:   80.0,
			},
		},
		Security: SecurityConfig{
			EnableEncryption: false,
			EnableAuditLog:   true,
			AuditLogPath:     "/var/log/ploy/arf-audit.log",
		},
	}
}

// ProductionConfig returns a production-ready ARF configuration
func ProductionConfig() *Config {
	config := DefaultConfig()

	// Use SeaweedFS + Consul in production
	config.Storage.Backend = "seaweedfs"
	config.Index.Backend = "consul"

	// Stricter validation in production
	config.Validation.SchemaStrict = true
	config.Validation.SecurityRules.AllowNetworkAccess = false
	config.Validation.SecurityRules.MaxExecutionTime = 10 * time.Minute
	config.Validation.SecurityRules.MaxMemoryUsageMB = 1024

	// Enable security features
	config.Security.EnableEncryption = true
	config.Security.EnableAuditLog = true

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
		return fmt.Errorf("Consul address is required")
	}

	// Validate validation config
	if c.Validation.SecurityRules.MaxExecutionTime <= 0 {
		return fmt.Errorf("max execution time must be positive")
	}

	if c.Validation.SecurityRules.MaxMemoryUsageMB <= 0 {
		return fmt.Errorf("max memory usage must be positive")
	}

	if c.Validation.SecurityRules.MaxCPUUsagePercent <= 0 || c.Validation.SecurityRules.MaxCPUUsagePercent > 100 {
		return fmt.Errorf("max CPU usage must be between 0 and 100")
	}

	return nil
}

// InitializeStorage creates and configures the storage backend from config
func (c *Config) InitializeStorage() (RecipeStorage, error) {
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
		registry := NewRecipeRegistry(client)
		return NewRegistryStorageAdapter(registry), nil

	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", c.Storage.Backend)
	}
}

// InitializeIndex creates and configures the index backend from config
func (c *Config) InitializeIndex() (RecipeIndexStore, error) {
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

// InitializeValidator creates and configures the validator from config
func (c *Config) InitializeValidator() RecipeValidatorInterface {
	if !c.Validation.Enabled {
		return nil
	}

	securityRules := c.createSecurityRules()
	return &RecipeValidatorAdapter{validation.NewRecipeValidator(securityRules, c.Validation.SchemaStrict)}
}

// createSecurityRules converts config to validation.SecurityRuleSet
func (c *Config) createSecurityRules() *validation.SecurityRuleSet {
	return &validation.SecurityRuleSet{
		AllowedCommands:      c.Validation.SecurityRules.AllowedCommands,
		ForbiddenCommands:    c.Validation.SecurityRules.ForbiddenCommands,
		MaxExecutionTime:     c.Validation.SecurityRules.MaxExecutionTime,
		AllowNetworkAccess:   c.Validation.SecurityRules.AllowNetworkAccess,
		AllowFileSystemWrite: c.Validation.SecurityRules.AllowFileSystemWrite,
		SandboxRequired:      c.Validation.SecurityRules.SandboxRequired,
		MaxMemoryUsage:       c.Validation.SecurityRules.MaxMemoryUsageMB * 1024 * 1024, // Convert MB to bytes
		MaxCPUUsage:          c.Validation.SecurityRules.MaxCPUUsagePercent / 100.0,     // Convert percentage to decimal
	}
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

	// Validation configuration
	if enabled := os.Getenv("ARF_VALIDATION_ENABLED"); enabled != "" {
		config.Validation.Enabled = enabled == "true"
	}
	if strict := os.Getenv("ARF_VALIDATION_STRICT"); strict != "" {
		config.Validation.SchemaStrict = strict == "true"
	}

	// Security configuration
	if encryption := os.Getenv("ARF_ENABLE_ENCRYPTION"); encryption != "" {
		config.Security.EnableEncryption = encryption == "true"
	}
	if auditLog := os.Getenv("ARF_ENABLE_AUDIT_LOG"); auditLog != "" {
		config.Security.EnableAuditLog = auditLog == "true"
	}
	if auditPath := os.Getenv("ARF_AUDIT_LOG_PATH"); auditPath != "" {
		config.Security.AuditLogPath = auditPath
	}

	return config
}
