package validation

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/arf/models"
)

func TestLLMModelValidator_ValidateLLMModel(t *testing.T) {
	validator := NewLLMModelValidator()

	tests := []struct {
		name    string
		model   *models.LLMModel
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid openai model",
			model: &models.LLMModel{
				ID:           "gpt-4o-mini@2024-08-06",
				Name:         "GPT-4o Mini",
				Provider:     "openai",
				Version:      "2024-08-06",
				Capabilities: []string{"code", "analysis"},
				Config:       map[string]string{"temperature": "0.7"},
				MaxTokens:    128000,
				CostPerToken: 0.00015,
			},
			wantErr: false,
		},
		{
			name: "valid anthropic model",
			model: &models.LLMModel{
				ID:           "claude-3-sonnet@v1",
				Name:         "Claude 3 Sonnet",
				Provider:     "anthropic",
				Version:      "v1",
				Capabilities: []string{"code", "analysis", "reasoning"},
				Config:       map[string]string{"top_p": "0.9"},
				MaxTokens:    200000,
				CostPerToken: 0.003,
			},
			wantErr: false,
		},
		{
			name: "valid azure model",
			model: &models.LLMModel{
				ID:           "azure-gpt-4@v1",
				Name:         "Azure GPT-4",
				Provider:     "azure",
				Version:      "v1",
				Capabilities: []string{"code", "multimodal"},
				Config:       map[string]string{"deployment_name": "gpt-4-deployment"},
				MaxTokens:    8192,
				CostPerToken: 0.03,
			},
			wantErr: false,
		},
		{
			name:    "nil model",
			model:   nil,
			wantErr: true,
			errMsg:  "model cannot be nil",
		},
		{
			name: "model with short ID",
			model: &models.LLMModel{
				ID:           "ab",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1000,
			},
			wantErr: true,
			errMsg:  "model ID too short",
		},
		{
			name: "model with long ID",
			model: &models.LLMModel{
				ID:           "this-is-an-extremely-long-model-id-that-exceeds-the-maximum-allowed-length-for-model-identifiers-and-should-definitely-fail",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1000,
			},
			wantErr: true,
			errMsg:  "model ID too long",
		},
		{
			name: "model with invalid ID characters",
			model: &models.LLMModel{
				ID:           "invalid$model#id",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1000,
			},
			wantErr: true,
			errMsg:  "invalid model ID format",
		},
		{
			name: "azure model without deployment_name",
			model: &models.LLMModel{
				ID:           "azure-gpt-4@v1",
				Name:         "Azure GPT-4",
				Provider:     "azure",
				Capabilities: []string{"code"},
				Config:       map[string]string{"temperature": "0.7"},
				MaxTokens:    8192,
			},
			wantErr: true,
			errMsg:  "Azure models require 'deployment_name'",
		},
		{
			name: "model with low token limit",
			model: &models.LLMModel{
				ID:           "small-model@v1",
				Name:         "Small Model",
				Provider:     "local",
				Capabilities: []string{"code"},
				MaxTokens:    500,
			},
			wantErr: true,
			errMsg:  "max_tokens too low",
		},
		{
			name: "openai model with high token limit",
			model: &models.LLMModel{
				ID:           "huge-model@v1",
				Name:         "Huge Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    2000000,
			},
			wantErr: true,
			errMsg:  "max_tokens for OpenAI models cannot exceed",
		},
		{
			name: "anthropic model with multimodal (not supported)",
			model: &models.LLMModel{
				ID:           "claude-3@v1",
				Name:         "Claude 3",
				Provider:     "anthropic",
				Capabilities: []string{"multimodal"},
				MaxTokens:    200000,
			},
			wantErr: false, // multimodal is now supported for anthropic
		},
		{
			name: "local model with function_calling (not supported)",
			model: &models.LLMModel{
				ID:           "local-model@v1",
				Name:         "Local Model",
				Provider:     "local",
				Capabilities: []string{"function_calling"},
				MaxTokens:    8000,
			},
			wantErr: true,
			errMsg:  "capability 'function_calling' not supported by provider 'local'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateLLMModel(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLLMModel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("ValidateLLMModel() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestLLMModelValidator_ValidateModelUpdate(t *testing.T) {
	validator := NewLLMModelValidator()

	existingModel := &models.LLMModel{
		ID:           "gpt-4@v1",
		Name:         "GPT-4",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8192,
	}

	tests := []struct {
		name     string
		existing *models.LLMModel
		updated  *models.LLMModel
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid update",
			existing: existingModel,
			updated: &models.LLMModel{
				ID:           "gpt-4@v1",
				Name:         "GPT-4 Updated",
				Provider:     "openai",
				Capabilities: []string{"code", "analysis"},
				MaxTokens:    8192,
			},
			wantErr: false,
		},
		{
			name:     "nil existing model",
			existing: nil,
			updated:  existingModel,
			wantErr:  true,
			errMsg:   "existing model cannot be nil",
		},
		{
			name:     "nil updated model",
			existing: existingModel,
			updated:  nil,
			wantErr:  true,
			errMsg:   "updated model cannot be nil",
		},
		{
			name:     "changed ID",
			existing: existingModel,
			updated: &models.LLMModel{
				ID:           "gpt-4@v2",
				Name:         "GPT-4",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    8192,
			},
			wantErr: true,
			errMsg:  "model ID cannot be changed",
		},
		{
			name:     "changed provider",
			existing: existingModel,
			updated: &models.LLMModel{
				ID:           "gpt-4@v1",
				Name:         "GPT-4",
				Provider:     "anthropic",
				Capabilities: []string{"code"},
				MaxTokens:    8192,
			},
			wantErr: true,
			errMsg:  "model provider cannot be changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateModelUpdate(tt.existing, tt.updated)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateModelUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("ValidateModelUpdate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestLLMModelValidator_validateProviderSpecificConfig(t *testing.T) {
	validator := NewLLMModelValidator()

	tests := []struct {
		name     string
		provider string
		config   map[string]string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid openai config",
			provider: "openai",
			config:   map[string]string{"temperature": "0.7", "api_base": "https://api.openai.com"},
			wantErr:  false,
		},
		{
			name:     "invalid openai config key",
			provider: "openai",
			config:   map[string]string{"invalid_key": "value"},
			wantErr:  true,
			errMsg:   "invalid OpenAI config key",
		},
		{
			name:     "valid anthropic config",
			provider: "anthropic",
			config:   map[string]string{"temperature": "0.7", "top_k": "40"},
			wantErr:  false,
		},
		{
			name:     "invalid anthropic config key",
			provider: "anthropic",
			config:   map[string]string{"invalid_key": "value"},
			wantErr:  true,
			errMsg:   "invalid Anthropic config key",
		},
		{
			name:     "valid azure config",
			provider: "azure",
			config:   map[string]string{"deployment_name": "gpt-4", "api_version": "2023-05-15"},
			wantErr:  false,
		},
		{
			name:     "azure config missing deployment_name",
			provider: "azure",
			config:   map[string]string{"api_version": "2023-05-15"},
			wantErr:  true,
			errMsg:   "Azure models require 'deployment_name'",
		},
		{
			name:     "valid local config",
			provider: "local",
			config:   map[string]string{"endpoint": "http://localhost:8080", "timeout": "30s"},
			wantErr:  false,
		},
		{
			name:     "unknown provider",
			provider: "unknown",
			config:   map[string]string{"any_key": "any_value"},
			wantErr:  false, // Unknown providers are allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateProviderSpecificConfig(tt.provider, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProviderSpecificConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("validateProviderSpecificConfig() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestLLMModelValidator_validateTokenLimits(t *testing.T) {
	validator := NewLLMModelValidator()

	tests := []struct {
		name      string
		maxTokens int
		provider  string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid openai tokens",
			maxTokens: 128000,
			provider:  "openai",
			wantErr:   false,
		},
		{
			name:      "too low tokens",
			maxTokens: 500,
			provider:  "openai",
			wantErr:   true,
			errMsg:    "max_tokens too low",
		},
		{
			name:      "too high tokens general",
			maxTokens: 3000000,
			provider:  "openai",
			wantErr:   true,
			errMsg:    "max_tokens too high",
		},
		{
			name:      "too high tokens for openai",
			maxTokens: 1500000,
			provider:  "openai",
			wantErr:   true,
			errMsg:    "max_tokens for OpenAI models cannot exceed",
		},
		{
			name:      "too high tokens for anthropic",
			maxTokens: 300000,
			provider:  "anthropic",
			wantErr:   true,
			errMsg:    "max_tokens for Anthropic models cannot exceed",
		},
		{
			name:      "high tokens for local",
			maxTokens: 150000,
			provider:  "local",
			wantErr:   true,
			errMsg:    "max_tokens for local models should not exceed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateTokenLimits(tt.maxTokens, tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTokenLimits() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("validateTokenLimits() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateModelIDFormat(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid ID",
			id:      "gpt-4o-mini@2024-08-06",
			wantErr: false,
		},
		{
			name:    "valid simple ID",
			id:      "claude-3-sonnet",
			wantErr: false,
		},
		{
			name:    "too short",
			id:      "ab",
			wantErr: true,
			errMsg:  "model ID too short",
		},
		{
			name:    "too long",
			id:      "this-is-an-extremely-long-model-id-that-exceeds-the-maximum-allowed-length-for-model-identifiers",
			wantErr: true,
			errMsg:  "model ID too long",
		},
		{
			name:    "invalid characters",
			id:      "invalid$model#id",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "starts with dash",
			id:      "-invalid-model",
			wantErr: true,
			errMsg:  "cannot start or end with dash",
		},
		{
			name:    "ends with underscore",
			id:      "invalid-model_",
			wantErr: true,
			errMsg:  "cannot start or end with dash or underscore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateModelIDFormat(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateModelIDFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("ValidateModelIDFormat() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
