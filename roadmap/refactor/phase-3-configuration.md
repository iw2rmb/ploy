# Phase 3: Configuration Management Centralization

## Objective

Implement a single configuration service with caching, validation, and functional options pattern, eliminating duplicate configuration loading logic and providing a consistent configuration experience across all modules.

## Current State Analysis

### Problems Identified

1. **Triple Implementation of Storage Config**:
   - `api/config/config.go:306` - CreateStorageClientFromConfig
   - `api/config/config.go:413` - CreateStorageClientFromConfig (duplicate)
   - `controller/config/config.go:306` - CreateStorageClientFromConfig

2. **Scattered Configuration Files**:
   - `/configs/` directory with various YAML files
   - Inline configuration in code
   - Environment variable parsing in multiple locations
   - No central validation

3. **Inconsistent Loading Patterns**:
   - Some modules use viper
   - Others parse YAML directly
   - Environment variables handled differently
   - No configuration hot-reload

## Proposed Architecture

```
internal/config/
├── README.md                    # Configuration documentation
├── service.go                   # Core configuration service
├── loader.go                    # Configuration loading strategies
├── validator.go                 # Configuration validation
├── cache.go                     # Configuration caching
├── watcher.go                   # File watching for hot-reload
├── sources/
│   ├── file.go                 # File-based configuration
│   ├── env.go                  # Environment variables
│   ├── consul.go               # Consul KV store
│   └── defaults.go             # Default values
├── schemas/
│   ├── app.go                  # Application config schema
│   ├── storage.go              # Storage config schema
│   ├── nomad.go                # Nomad config schema
│   └── arf.go                  # ARF config schema
└── options.go                  # Functional options
```

## Core Service Design

```go
// internal/config/service.go
package config

import (
    "context"
    "sync"
    "time"
)

// Service provides centralized configuration management
type Service struct {
    mu       sync.RWMutex
    config   *Config
    cache    *Cache
    loader   Loader
    watchers []Watcher
    onChange []func(*Config)
}

// Config represents the complete application configuration
type Config struct {
    App      AppConfig      `yaml:"app" json:"app"`
    Storage  StorageConfig  `yaml:"storage" json:"storage"`
    Nomad    NomadConfig    `yaml:"nomad" json:"nomad"`
    Consul   ConsulConfig   `yaml:"consul" json:"consul"`
    ARF      ARFConfig      `yaml:"arf" json:"arf"`
    Logging  LoggingConfig  `yaml:"logging" json:"logging"`
    Metrics  MetricsConfig  `yaml:"metrics" json:"metrics"`
}

// New creates a new configuration service
func New(opts ...Option) (*Service, error) {
    s := &Service{
        cache:  NewCache(5 * time.Minute),
        loader: NewCompositeLoader(),
    }
    
    for _, opt := range opts {
        if err := opt(s); err != nil {
            return nil, err
        }
    }
    
    if err := s.Load(); err != nil {
        return nil, err
    }
    
    return s, nil
}

// Get returns the current configuration
func (s *Service) Get() *Config {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.config.Clone()
}

// GetWithCache returns cached configuration if available
func (s *Service) GetWithCache(key string) (*Config, bool) {
    if cached := s.cache.Get(key); cached != nil {
        return cached.(*Config), true
    }
    
    config := s.Get()
    s.cache.Set(key, config)
    return config, false
}

// Reload forces a configuration reload
func (s *Service) Reload() error {
    config, err := s.loader.Load()
    if err != nil {
        return err
    }
    
    if err := s.validate(config); err != nil {
        return err
    }
    
    s.mu.Lock()
    oldConfig := s.config
    s.config = config
    s.mu.Unlock()
    
    // Notify watchers
    for _, fn := range s.onChange {
        go fn(config)
    }
    
    return nil
}

// Watch registers a callback for configuration changes
func (s *Service) Watch(fn func(*Config)) {
    s.mu.Lock()
    s.onChange = append(s.onChange, fn)
    s.mu.Unlock()
}
```

## Functional Options Pattern

```go
// internal/config/options.go
package config

type Option func(*Service) error

// WithFile loads configuration from a file
func WithFile(path string) Option {
    return func(s *Service) error {
        s.loader.AddSource(NewFileSource(path))
        return nil
    }
}

// WithEnvironment loads configuration from environment variables
func WithEnvironment(prefix string) Option {
    return func(s *Service) error {
        s.loader.AddSource(NewEnvSource(prefix))
        return nil
    }
}

// WithConsul loads configuration from Consul KV
func WithConsul(addr, prefix string) Option {
    return func(s *Service) error {
        source, err := NewConsulSource(addr, prefix)
        if err != nil {
            return err
        }
        s.loader.AddSource(source)
        return nil
    }
}

// WithDefaults sets default configuration values
func WithDefaults(defaults *Config) Option {
    return func(s *Service) error {
        s.loader.AddSource(NewDefaultsSource(defaults))
        return nil
    }
}

// WithValidation adds custom validation
func WithValidation(validator Validator) Option {
    return func(s *Service) error {
        s.validators = append(s.validators, validator)
        return nil
    }
}

// WithHotReload enables configuration hot-reload
func WithHotReload(interval time.Duration) Option {
    return func(s *Service) error {
        watcher := NewFileWatcher(interval)
        s.watchers = append(s.watchers, watcher)
        go s.watchForChanges(watcher)
        return nil
    }
}
```

## Configuration Loading Strategy

```go
// internal/config/loader.go
package config

import (
    "fmt"
    "github.com/mitchellh/mapstructure"
)

// Source represents a configuration source
type Source interface {
    Name() string
    Load() (map[string]interface{}, error)
    Priority() int // Higher priority overrides lower
}

// CompositeLoader loads configuration from multiple sources
type CompositeLoader struct {
    sources []Source
}

func (l *CompositeLoader) Load() (*Config, error) {
    // Sort sources by priority
    sort.Slice(l.sources, func(i, j int) bool {
        return l.sources[i].Priority() < l.sources[j].Priority()
    })
    
    result := make(map[string]interface{})
    
    // Merge configurations from all sources
    for _, source := range l.sources {
        data, err := source.Load()
        if err != nil {
            log.Printf("Warning: failed to load from %s: %v", source.Name(), err)
            continue
        }
        
        if err := mergeConfigs(result, data); err != nil {
            return nil, fmt.Errorf("merge config from %s: %w", source.Name(), err)
        }
    }
    
    // Decode into Config struct
    var config Config
    decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
        Result:           &config,
        WeaklyTypedInput: true,
        TagName:          "yaml",
    })
    if err != nil {
        return nil, err
    }
    
    if err := decoder.Decode(result); err != nil {
        return nil, err
    }
    
    return &config, nil
}
```

## Validation Framework

```go
// internal/config/validator.go
package config

import (
    "fmt"
    "github.com/go-playground/validator/v10"
)

type Validator interface {
    Validate(*Config) error
}

// StructValidator uses struct tags for validation
type StructValidator struct {
    v *validator.Validate
}

func NewStructValidator() *StructValidator {
    v := validator.New()
    
    // Register custom validators
    v.RegisterValidation("endpoint", validateEndpoint)
    v.RegisterValidation("port", validatePort)
    
    return &StructValidator{v: v}
}

func (sv *StructValidator) Validate(cfg *Config) error {
    return sv.v.Struct(cfg)
}

// SchemaValidator validates against JSON schema
type SchemaValidator struct {
    schema string
}

func (sv *SchemaValidator) Validate(cfg *Config) error {
    // Validate against JSON schema
    return nil
}

// BusinessLogicValidator implements custom validation rules
type BusinessLogicValidator struct{}

func (blv *BusinessLogicValidator) Validate(cfg *Config) error {
    // Validate storage configuration
    if cfg.Storage.Provider == "s3" && cfg.Storage.Region == "" {
        return fmt.Errorf("S3 storage requires region")
    }
    
    // Validate ARF configuration
    if cfg.ARF.Enabled && cfg.Storage.Provider == "" {
        return fmt.Errorf("ARF requires storage provider")
    }
    
    return nil
}
```

## Caching Layer

```go
// internal/config/cache.go
package config

import (
    "sync"
    "time"
)

type Cache struct {
    mu      sync.RWMutex
    items   map[string]*cacheItem
    ttl     time.Duration
}

type cacheItem struct {
    value     interface{}
    expiresAt time.Time
}

func NewCache(ttl time.Duration) *Cache {
    c := &Cache{
        items: make(map[string]*cacheItem),
        ttl:   ttl,
    }
    
    // Start cleanup goroutine
    go c.cleanup()
    
    return c
}

func (c *Cache) Get(key string) interface{} {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    item, exists := c.items[key]
    if !exists {
        return nil
    }
    
    if time.Now().After(item.expiresAt) {
        return nil
    }
    
    return item.value
}

func (c *Cache) Set(key string, value interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    c.items[key] = &cacheItem{
        value:     value,
        expiresAt: time.Now().Add(c.ttl),
    }
}
```

## Migration Plan

### Step 1: Create Configuration Service

```go
// cmd/ploy/main.go
func main() {
    // Initialize configuration service
    configSvc, err := config.New(
        config.WithFile("/etc/ploy/config.yaml"),
        config.WithEnvironment("PLOY_"),
        config.WithDefaults(defaultConfig()),
        config.WithValidation(config.NewStructValidator()),
        config.WithHotReload(30 * time.Second),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // Use configuration
    cfg := configSvc.Get()
    
    // Watch for changes
    configSvc.Watch(func(newCfg *config.Config) {
        log.Println("Configuration updated")
        // Handle configuration change
    })
}
```

### Step 2: Replace Duplicate Functions

Remove all instances of `CreateStorageClientFromConfig` and replace with:

```go
// internal/config/factory.go
package config

import "github.com/ploy/internal/storage"

func (c *Config) CreateStorageClient() (storage.Storage, error) {
    return storage.New(storage.Config{
        Provider: c.Storage.Provider,
        Endpoint: c.Storage.Endpoint,
        Bucket:   c.Storage.Bucket,
        Region:   c.Storage.Region,
        Retry: storage.RetryConfig{
            Enabled:     c.Storage.Retry.Enabled,
            MaxAttempts: c.Storage.Retry.MaxAttempts,
        },
    })
}
```

### Step 3: Update All Consumers

```go
// Before
config, err := config.Load(configPath)
storageClient, err := config.CreateStorageClientFromConfig(configPath)

// After
configSvc := app.ConfigService() // Injected dependency
cfg := configSvc.Get()
storageClient, err := cfg.CreateStorageClient()
```

## Environment Variable Mapping

```yaml
# config.yaml with env var substitution
app:
  name: ${PLOY_APP_NAME:ploy}
  version: ${PLOY_VERSION:latest}
  
storage:
  provider: ${PLOY_STORAGE_PROVIDER:seaweedfs}
  endpoint: ${PLOY_STORAGE_ENDPOINT:http://localhost:9333}
  
nomad:
  address: ${NOMAD_ADDR:http://localhost:4646}
  token: ${NOMAD_TOKEN}
```

## Testing Strategy

```go
// internal/config/service_test.go
func TestConfigurationService(t *testing.T) {
    t.Run("loads from file", func(t *testing.T) {
        svc, err := New(WithFile("testdata/config.yaml"))
        require.NoError(t, err)
        
        cfg := svc.Get()
        assert.Equal(t, "test-app", cfg.App.Name)
    })
    
    t.Run("environment overrides file", func(t *testing.T) {
        os.Setenv("PLOY_APP_NAME", "override")
        defer os.Unsetenv("PLOY_APP_NAME")
        
        svc, err := New(
            WithFile("testdata/config.yaml"),
            WithEnvironment("PLOY_"),
        )
        require.NoError(t, err)
        
        cfg := svc.Get()
        assert.Equal(t, "override", cfg.App.Name)
    })
    
    t.Run("validates configuration", func(t *testing.T) {
        svc, err := New(
            WithFile("testdata/invalid.yaml"),
            WithValidation(NewStructValidator()),
        )
        assert.Error(t, err)
    })
}
```

## Validation Checklist

- [x] Initial configuration service slice implemented (defaults, env, file loaders)
- [ ] All duplicate config functions removed
- [x] Environment variable handling unified (baseline via service options)
- [ ] Configuration validation working
 - [x] Configuration validation working
 - [ ] Hot-reload implemented
 - [x] Caching layer functional
- [x] Unit tests added for file/env loading and storage client creation
- [x] Documentation updated (progress logged)

## Implementation Steps

- Implement core service and loaders
- Add validation and caching
- Replace duplicate functions
- Update all consumers and test

## Progress Update (2025-09-03)

- Completed (slice 1):
  - `internal/config` Service with `Get()` clone semantics
  - Composite loader with sources: defaults, environment (`WithEnvironment`), and YAML file (`WithFile`)
  - Minimal storage schema in `internal/config` and adapter method `Config.CreateStorageClient()` using unified storage factory (memory provider for this slice)
  - Unit tests for file loading, env override, and storage client creation
- Pending (next slices):
  - Wire `api/server` to prefer `internal/config` service for storage initialization (keep legacy fallback)
  - Migrate duplicate config helpers to the centralized service
  - Add hot‑reload
  - Broaden storage config mapping beyond provider (endpoint/bucket/etc.)

## Expected Outcomes

### Before
- Configuration functions: 3+ duplicates
- Configuration loading: Scattered across codebase
- Configuration LOC: ~2,000

### After
- Configuration functions: 1 service
- Configuration loading: Centralized
- Configuration LOC: ~1,000 (50% reduction)
- Features gained: Hot-reload, validation, caching
- Maintainability: Single point of configuration management
