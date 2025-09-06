package models

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// LLMModel represents a language model in the registry
type LLMModel struct {
	ID           string            `json:"id"`           // e.g., "gpt-4o-mini@2024-08-06"
	Name         string            `json:"name"`         // Display name
	Provider     string            `json:"provider"`     // openai, anthropic, azure, local
	Version      string            `json:"version"`      // Model version
	Capabilities []string          `json:"capabilities"` // ["code", "analysis", "planning"]
	Config       map[string]string `json:"config"`       // Provider-specific config
	MaxTokens    int               `json:"max_tokens"`   // Context window size
	CostPerToken float64           `json:"cost_per_token,omitempty"`
	Created      Time              `json:"created"`
	Updated      Time              `json:"updated"`
}

// ValidProviders lists supported model providers
var ValidProviders = []string{"openai", "anthropic", "azure", "local"}

// ValidCapabilities lists supported model capabilities
var ValidCapabilities = []string{"code", "analysis", "planning", "reasoning", "multimodal", "function_calling"}

// ModelIDPattern matches valid model ID format: provider@version or provider/model@version
var ModelIDPattern = regexp.MustCompile(`^[a-z0-9\-]+(@[a-z0-9\-\.]+)?$`)

// Validate validates the LLM model data
func (m *LLMModel) Validate() error {
	// Required fields
	if m.ID == "" {
		return fmt.Errorf("model ID is required")
	}
	if m.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if m.Provider == "" {
		return fmt.Errorf("model provider is required")
	}
	if len(m.Capabilities) == 0 {
		return fmt.Errorf("at least one capability is required")
	}

	// Validate ID format
	if !ModelIDPattern.MatchString(m.ID) {
		return fmt.Errorf("invalid model ID format: %s (should match pattern: provider@version or provider/model@version)", m.ID)
	}

	// Validate provider
	if !isValidProvider(m.Provider) {
		return fmt.Errorf("invalid provider: %s (supported: %s)", m.Provider, strings.Join(ValidProviders, ", "))
	}

	// Validate capabilities
	for _, capability := range m.Capabilities {
		if !isValidCapability(capability) {
			return fmt.Errorf("invalid capability: %s (supported: %s)", capability, strings.Join(ValidCapabilities, ", "))
		}
	}

	// Validate numeric fields
	if m.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be greater than 0")
	}
	if m.CostPerToken < 0 {
		return fmt.Errorf("cost_per_token cannot be negative")
	}

	return nil
}

// SetSystemFields sets system-managed fields like timestamps
func (m *LLMModel) SetSystemFields() {
	now := Time(time.Now())
	if time.Time(m.Created).IsZero() {
		m.Created = now
	}
	m.Updated = now
}

// isValidProvider checks if the provider is in the allowed list
func isValidProvider(provider string) bool {
	for _, valid := range ValidProviders {
		if provider == valid {
			return true
		}
	}
	return false
}

// isValidCapability checks if the capability is in the allowed list
func isValidCapability(capability string) bool {
	for _, valid := range ValidCapabilities {
		if capability == valid {
			return true
		}
	}
	return false
}

// GetProviderFromID extracts the provider from the model ID
func (m *LLMModel) GetProviderFromID() string {
	if strings.Contains(m.ID, "@") {
		parts := strings.Split(m.ID, "@")
		if len(parts) > 0 {
			// Handle cases like "gpt-4o-mini@2024-08-06" or "openai/gpt-4@v1"
			providerPart := parts[0]
			if strings.Contains(providerPart, "/") {
				return strings.Split(providerPart, "/")[0]
			}
			return providerPart
		}
	}
	// Fallback to the provider field
	return m.Provider
}

// HasCapability checks if the model has a specific capability
func (m *LLMModel) HasCapability(capability string) bool {
	for _, cap := range m.Capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}
