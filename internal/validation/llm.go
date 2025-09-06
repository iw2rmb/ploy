package validation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/iw2rmb/ploy/internal/arf/models"
)

// LLMModelValidator provides validation for LLM models
type LLMModelValidator struct{}

// NewLLMModelValidator creates a new LLM model validator
func NewLLMModelValidator() *LLMModelValidator {
	return &LLMModelValidator{}
}

// ValidateLLMModel performs comprehensive validation of an LLM model
func (v *LLMModelValidator) ValidateLLMModel(model *models.LLMModel) error {
	if model == nil {
		return fmt.Errorf("model cannot be nil")
	}

	// Run the model's built-in validation first
	if err := model.Validate(); err != nil {
		return err
	}

	// Additional validations
	if err := v.validateModelID(model.ID); err != nil {
		return fmt.Errorf("invalid model ID: %w", err)
	}

	if err := v.validateProviderSpecificConfig(model.Provider, model.Config); err != nil {
		return fmt.Errorf("invalid provider config: %w", err)
	}

	if err := v.validateCapabilitiesForProvider(model.Provider, model.Capabilities); err != nil {
		return fmt.Errorf("invalid capabilities for provider: %w", err)
	}

	if err := v.validateTokenLimits(model.MaxTokens, model.Provider); err != nil {
		return fmt.Errorf("invalid token limits: %w", err)
	}

	return nil
}

// validateModelID provides additional ID format validation
func (v *LLMModelValidator) validateModelID(id string) error {
	if len(id) < 3 {
		return fmt.Errorf("model ID too short (minimum 3 characters)")
	}

	if len(id) > 100 {
		return fmt.Errorf("model ID too long (maximum 100 characters)")
	}

	// Check for invalid characters
	validIDPattern := regexp.MustCompile(`^[a-zA-Z0-9\-_@/.]+$`)
	if !validIDPattern.MatchString(id) {
		return fmt.Errorf("model ID contains invalid characters (allowed: letters, numbers, -, _, @, /, .)")
	}

	// Ensure it doesn't start or end with special characters
	if strings.HasPrefix(id, "-") || strings.HasSuffix(id, "-") ||
		strings.HasPrefix(id, "_") || strings.HasSuffix(id, "_") {
		return fmt.Errorf("model ID cannot start or end with dash or underscore")
	}

	return nil
}

// validateProviderSpecificConfig validates configuration based on provider
func (v *LLMModelValidator) validateProviderSpecificConfig(provider string, config map[string]string) error {
	switch provider {
	case "openai":
		return v.validateOpenAIConfig(config)
	case "anthropic":
		return v.validateAnthropicConfig(config)
	case "azure":
		return v.validateAzureConfig(config)
	case "local":
		return v.validateLocalConfig(config)
	default:
		// For unknown providers, just check for basic config structure
		return nil
	}
}

// validateOpenAIConfig validates OpenAI-specific configuration
func (v *LLMModelValidator) validateOpenAIConfig(config map[string]string) error {
	// Common OpenAI configurations
	validKeys := map[string]bool{
		"api_base":     true,
		"api_version":  true,
		"organization": true,
		"temperature":  true,
		"top_p":        true,
		"max_retries":  true,
	}

	for key := range config {
		if !validKeys[key] {
			return fmt.Errorf("invalid OpenAI config key: %s", key)
		}
	}

	return nil
}

// validateAnthropicConfig validates Anthropic-specific configuration
func (v *LLMModelValidator) validateAnthropicConfig(config map[string]string) error {
	validKeys := map[string]bool{
		"api_base":    true,
		"max_retries": true,
		"temperature": true,
		"top_k":       true,
		"top_p":       true,
	}

	for key := range config {
		if !validKeys[key] {
			return fmt.Errorf("invalid Anthropic config key: %s", key)
		}
	}

	return nil
}

// validateAzureConfig validates Azure OpenAI-specific configuration
func (v *LLMModelValidator) validateAzureConfig(config map[string]string) error {
	// Azure requires deployment name
	if _, exists := config["deployment_name"]; !exists {
		return fmt.Errorf("Azure models require 'deployment_name' in config")
	}

	validKeys := map[string]bool{
		"api_base":        true,
		"api_version":     true,
		"deployment_name": true,
		"temperature":     true,
		"top_p":           true,
		"max_retries":     true,
	}

	for key := range config {
		if !validKeys[key] {
			return fmt.Errorf("invalid Azure config key: %s", key)
		}
	}

	return nil
}

// validateLocalConfig validates local model configuration
func (v *LLMModelValidator) validateLocalConfig(config map[string]string) error {
	validKeys := map[string]bool{
		"model_path":  true,
		"endpoint":    true,
		"port":        true,
		"temperature": true,
		"timeout":     true,
	}

	for key := range config {
		if !validKeys[key] {
			return fmt.Errorf("invalid local model config key: %s", key)
		}
	}

	return nil
}

// validateCapabilitiesForProvider ensures capabilities are valid for the provider
func (v *LLMModelValidator) validateCapabilitiesForProvider(provider string, capabilities []string) error {
	providerCapabilities := map[string][]string{
		"openai": {
			"code", "analysis", "planning", "reasoning", "multimodal", "function_calling",
		},
		"anthropic": {
			"code", "analysis", "planning", "reasoning", "multimodal",
		},
		"azure": {
			"code", "analysis", "planning", "reasoning", "multimodal", "function_calling",
		},
		"local": {
			"code", "analysis", "planning", "reasoning",
		},
	}

	validCaps, exists := providerCapabilities[provider]
	if !exists {
		// For unknown providers, allow all capabilities
		return nil
	}

	for _, capability := range capabilities {
		if !contains(validCaps, capability) {
			return fmt.Errorf("capability '%s' not supported by provider '%s'", capability, provider)
		}
	}

	return nil
}

// validateTokenLimits validates token limits based on provider and known model limits
func (v *LLMModelValidator) validateTokenLimits(maxTokens int, provider string) error {
	// General limits
	if maxTokens < 1000 {
		return fmt.Errorf("max_tokens too low (minimum 1000)")
	}

	if maxTokens > 2000000 {
		return fmt.Errorf("max_tokens too high (maximum 2000000)")
	}

	// Provider-specific limits
	switch provider {
	case "openai":
		if maxTokens > 1000000 {
			return fmt.Errorf("max_tokens for OpenAI models cannot exceed 1000000")
		}
	case "anthropic":
		if maxTokens > 200000 {
			return fmt.Errorf("max_tokens for Anthropic models cannot exceed 200000")
		}
	case "local":
		// More lenient for local models
		if maxTokens > 100000 {
			return fmt.Errorf("max_tokens for local models should not exceed 100000 (consider memory constraints)")
		}
	}

	return nil
}

// ValidateModelUpdate validates changes when updating a model
func (v *LLMModelValidator) ValidateModelUpdate(existing, updated *models.LLMModel) error {
	if existing == nil {
		return fmt.Errorf("existing model cannot be nil")
	}
	if updated == nil {
		return fmt.Errorf("updated model cannot be nil")
	}

	// ID cannot be changed
	if existing.ID != updated.ID {
		return fmt.Errorf("model ID cannot be changed (existing: %s, updated: %s)", existing.ID, updated.ID)
	}

	// Provider cannot be changed
	if existing.Provider != updated.Provider {
		return fmt.Errorf("model provider cannot be changed (existing: %s, updated: %s)", existing.Provider, updated.Provider)
	}

	// Validate the updated model
	return v.ValidateLLMModel(updated)
}

// ValidateModelIDFormat validates just the model ID format
func ValidateModelIDFormat(id string) error {
	validator := NewLLMModelValidator()
	return validator.validateModelID(id)
}

// contains is a helper function to check if a string slice contains a value
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
