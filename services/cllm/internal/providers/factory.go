package providers

import (
	"context"
	"fmt"
	"sync"
)

// DefaultFactory is the global provider factory instance
var DefaultFactory = NewFactory()

// Factory implements ProviderFactory interface
type Factory struct {
	mu           sync.RWMutex
	constructors map[string]ProviderConstructor
}

// NewFactory creates a new provider factory
func NewFactory() *Factory {
	f := &Factory{
		constructors: make(map[string]ProviderConstructor),
	}
	
	// Register built-in providers
	f.registerBuiltinProviders()
	
	return f
}

// CreateProvider creates a provider instance based on configuration
func (f *Factory) CreateProvider(config ProviderConfig) (Provider, error) {
	f.mu.RLock()
	constructor, exists := f.constructors[config.Type]
	f.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("unknown provider type: %s", config.Type)
	}
	
	return constructor(config)
}

// RegisterProvider registers a custom provider type
func (f *Factory) RegisterProvider(providerType string, constructor ProviderConstructor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	f.constructors[providerType] = constructor
}

// GetProviderTypes returns a list of registered provider types
func (f *Factory) GetProviderTypes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	types := make([]string, 0, len(f.constructors))
	for t := range f.constructors {
		types = append(types, t)
	}
	return types
}

// registerBuiltinProviders registers the built-in provider types
func (f *Factory) registerBuiltinProviders() {
	// Register Ollama provider
	f.RegisterProvider("ollama", func(config ProviderConfig) (Provider, error) {
		return NewOllamaProvider(config)
	})
	
	// Register OpenAI provider
	f.RegisterProvider("openai", func(config ProviderConfig) (Provider, error) {
		return NewOpenAIProvider(config)
	})
	
	// Register mock provider for testing
	f.RegisterProvider("mock", func(config ProviderConfig) (Provider, error) {
		return NewMockProvider(config)
	})
}

// CreateFromConfig creates the appropriate provider based on configuration
// This is a convenience function that uses the default factory
func CreateFromConfig(config ProviderConfig) (Provider, error) {
	return DefaultFactory.CreateProvider(config)
}

// RegisterCustomProvider registers a custom provider type with the default factory
func RegisterCustomProvider(providerType string, constructor ProviderConstructor) {
	DefaultFactory.RegisterProvider(providerType, constructor)
}

// GetAvailableProviders returns a list of all registered provider types
func GetAvailableProviders() []string {
	return DefaultFactory.GetProviderTypes()
}

// ProviderManager manages multiple providers with fallback support
type ProviderManager struct {
	mu        sync.RWMutex
	providers []Provider
	primary   Provider
}

// NewProviderManager creates a new provider manager
func NewProviderManager(configs []ProviderConfig) (*ProviderManager, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("at least one provider configuration required")
	}
	
	manager := &ProviderManager{
		providers: make([]Provider, 0, len(configs)),
	}
	
	for _, config := range configs {
		provider, err := CreateFromConfig(config)
		if err != nil {
			// Log warning but continue with other providers
			continue
		}
		manager.providers = append(manager.providers, provider)
	}
	
	if len(manager.providers) == 0 {
		return nil, fmt.Errorf("failed to create any providers")
	}
	
	// Set first provider as primary
	manager.primary = manager.providers[0]
	
	return manager, nil
}

// GetPrimary returns the primary provider
func (m *ProviderManager) GetPrimary() Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.primary
}

// SetPrimary sets a provider as primary by name
func (m *ProviderManager) SetPrimary(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, provider := range m.providers {
		if provider.Name() == name {
			m.primary = provider
			return nil
		}
	}
	
	return fmt.Errorf("provider %s not found", name)
}

// GetProvider returns a provider by name
func (m *ProviderManager) GetProvider(name string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, provider := range m.providers {
		if provider.Name() == name {
			return provider, nil
		}
	}
	
	return nil, fmt.Errorf("provider %s not found", name)
}

// GetAvailableProvider returns the first available provider
func (m *ProviderManager) GetAvailableProvider(ctx context.Context) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, provider := range m.providers {
		if provider.IsAvailable(ctx) {
			return provider, nil
		}
	}
	
	return nil, fmt.Errorf("no available providers")
}

// Close closes all managed providers
func (m *ProviderManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var lastErr error
	for _, provider := range m.providers {
		if err := provider.Close(); err != nil {
			lastErr = err
		}
	}
	
	return lastErr
}