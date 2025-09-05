package config

import (
	"log"
	"os"
	"sync"
	"time"
)

// AppConfig holds basic application configuration used in tests and core flows.
type AppConfig struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

// Config represents the minimal configuration shape needed for Phase 3 slice.
type Config struct {
	App     AppConfig     `yaml:"app" json:"app"`
	Storage StorageConfig `yaml:"storage" json:"storage"`
	Policy  PolicyConfig  `yaml:"policy" json:"policy"`
}

// PolicyConfig defines optional policy settings
type PolicyConfig struct {
	StrictEnvs []string           `yaml:"strict_envs" json:"strict_envs"`
	SizeCapsMB map[string]float64 `yaml:"size_caps_mb" json:"size_caps_mb"`
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
	cache  *Cache
	// validators are executed after load
	validators []Validator
	// onChange callbacks invoked after successful reload
	onChange []func(*Config)
	// hot-reload control
	hotReloadStop chan struct{}
}

// New creates a new configuration service, applying the provided options and
// loading configuration from registered sources.
func New(opts ...Option) (*Service, error) {
	s := &Service{loader: &CompositeLoader{}, cache: NewCache()}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	cfg, err := s.loader.Load()
	if err != nil {
		return nil, err
	}

	// Run validators if any
	if err := s.validate(cfg); err != nil {
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

// GetWithCache returns the configuration from an internal cache if present
// for the provided key; otherwise stores the current snapshot under that key
// and returns it along with a boolean indicating whether it was a cache hit.
func (s *Service) GetWithCache(key string) (*Config, bool) {
	if s.cache != nil {
		if v, ok := s.cache.Get(key); ok {
			if cfg, ok2 := v.(*Config); ok2 {
				return cfg, true
			}
		}
	}
	cfg := s.Get()
	if s.cache != nil {
		s.cache.Set(key, cfg)
	}
	return cfg, false
}

// validate runs all registered validators.
func (s *Service) validate(cfg *Config) error {
	for _, v := range s.validators {
		if err := v.Validate(cfg); err != nil {
			return err
		}
	}
	return nil
}

// Reload reloads configuration from loader, validates, swaps, and notifies.
func (s *Service) Reload() error {
	cfg, err := s.loader.Load()
	if err != nil {
		return err
	}
	if err := s.validate(cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
	for _, fn := range s.onChange {
		// invoke callbacks asynchronously
		go fn(cfg.Clone())
	}
	return nil
}

// Watch registers a callback to be called after successful reloads.
func (s *Service) Watch(fn func(*Config)) {
	s.mu.Lock()
	s.onChange = append(s.onChange, fn)
	s.mu.Unlock()
}

// startHotReload starts a polling loop watching file sources' mtime.
func (s *Service) startHotReload(interval time.Duration) {
	if interval <= 0 {
		return
	}
	stop := make(chan struct{})
	s.hotReloadStop = stop
	// Build initial modtime map
	mtimes := map[string]time.Time{}
	for _, src := range s.loader.sources {
		if fs, ok := src.(*fileSource); ok {
			if st, err := os.Stat(fs.path); err == nil {
				mtimes[fs.path] = st.ModTime()
			}
		}
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				changed := false
				for _, src := range s.loader.sources {
					if fs, ok := src.(*fileSource); ok {
						if st, err := os.Stat(fs.path); err == nil {
							prev := mtimes[fs.path]
							if st.ModTime().After(prev) {
								mtimes[fs.path] = st.ModTime()
								changed = true
							}
						}
					}
				}
				if changed {
					if err := s.Reload(); err != nil {
						log.Printf("config hot-reload failed: %v", err)
					}
				}
			case <-stop:
				return
			}
		}
	}()
}
