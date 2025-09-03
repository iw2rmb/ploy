package config

import "sync"

// AppConfig holds basic application configuration used in tests and core flows.
type AppConfig struct {
    Name    string `yaml:"name" json:"name"`
    Version string `yaml:"version" json:"version"`
}

// Config represents the minimal configuration shape needed for Phase 3 slice.
type Config struct {
    App     AppConfig     `yaml:"app" json:"app"`
    Storage StorageConfig `yaml:"storage" json:"storage"`
}

// Clone returns a deep copy safe for external mutation.
func (c *Config) Clone() *Config {
    if c == nil {
        return nil
    }
    out := *c
    // AppConfig is value type; the shallow copy is sufficient here.
    return &out
}

// Service provides centralized configuration access for the minimal slice.
type Service struct {
    mu     sync.RWMutex
    config *Config
    loader *CompositeLoader
}

// New creates a new configuration service, applying the provided options and
// loading configuration from registered sources.
func New(opts ...Option) (*Service, error) {
    s := &Service{loader: &CompositeLoader{}}
    for _, opt := range opts {
        if err := opt(s); err != nil {
            return nil, err
        }
    }

    cfg, err := s.loader.Load()
    if err != nil {
        return nil, err
    }

    s.mu.Lock()
    s.config = cfg
    s.mu.Unlock()
    return s, nil
}

// Get returns a clone of the current configuration to prevent external mutation.
func (s *Service) Get() *Config {
    s.mu.RLock()
    defer s.mu.RUnlock()
    if s.config == nil {
        return &Config{}
    }
    return s.config.Clone()
}
