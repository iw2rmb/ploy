package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLLMModel_Validate(t *testing.T) {
	tests := []struct {
		name    string
		model   *LLMModel
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid model",
			model: &LLMModel{
				ID:           "gpt-4o-mini@2024-08-06",
				Name:         "GPT-4o Mini",
				Provider:     "openai",
				Version:      "2024-08-06",
				Capabilities: []string{"code", "analysis"},
				MaxTokens:    128000,
				CostPerToken: 0.00015,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			model: &LLMModel{
				Name:         "GPT-4o Mini",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    128000,
			},
			wantErr: true,
			errMsg:  "model ID is required",
		},
		{
			name: "missing name",
			model: &LLMModel{
				ID:           "gpt-4o-mini@2024-08-06",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    128000,
			},
			wantErr: true,
			errMsg:  "model name is required",
		},
		{
			name: "missing provider",
			model: &LLMModel{
				ID:           "gpt-4o-mini@2024-08-06",
				Name:         "GPT-4o Mini",
				Capabilities: []string{"code"},
				MaxTokens:    128000,
			},
			wantErr: true,
			errMsg:  "model provider is required",
		},
		{
			name: "missing capabilities",
			model: &LLMModel{
				ID:        "gpt-4o-mini@2024-08-06",
				Name:      "GPT-4o Mini",
				Provider:  "openai",
				MaxTokens: 128000,
			},
			wantErr: true,
			errMsg:  "at least one capability is required",
		},
		{
			name: "invalid ID format",
			model: &LLMModel{
				ID:           "invalid/id/format!",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    128000,
			},
			wantErr: true,
			errMsg:  "invalid model ID format",
		},
		{
			name: "invalid provider",
			model: &LLMModel{
				ID:           "test-model@v1",
				Name:         "Test Model",
				Provider:     "invalid",
				Capabilities: []string{"code"},
				MaxTokens:    128000,
			},
			wantErr: true,
			errMsg:  "invalid provider: invalid",
		},
		{
			name: "invalid capability",
			model: &LLMModel{
				ID:           "test-model@v1",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"invalid"},
				MaxTokens:    128000,
			},
			wantErr: true,
			errMsg:  "invalid capability: invalid",
		},
		{
			name: "zero max tokens",
			model: &LLMModel{
				ID:           "test-model@v1",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    0,
			},
			wantErr: true,
			errMsg:  "max_tokens must be greater than 0",
		},
		{
			name: "negative cost per token",
			model: &LLMModel{
				ID:           "test-model@v1",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    128000,
				CostPerToken: -0.001,
			},
			wantErr: true,
			errMsg:  "cost_per_token cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.model.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("LLMModel.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("LLMModel.Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestLLMModel_SetSystemFields(t *testing.T) {
	model := &LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    128000,
	}

	// Set system fields
	model.SetSystemFields()

	// Check that timestamps are set
	if time.Time(model.Created).IsZero() {
		t.Error("Expected Created timestamp to be set")
	}
	if time.Time(model.Updated).IsZero() {
		t.Error("Expected Updated timestamp to be set")
	}

	// Test that Created is preserved on subsequent calls
	originalCreated := model.Created
	time.Sleep(1 * time.Millisecond) // Ensure different timestamp
	model.SetSystemFields()

	if model.Created != originalCreated {
		t.Error("Expected Created timestamp to be preserved")
	}
	if time.Time(model.Updated).Before(time.Time(originalCreated)) {
		t.Error("Expected Updated timestamp to be newer than or equal to Created")
	}
}

func TestLLMModel_HasCapability(t *testing.T) {
	model := &LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code", "analysis", "planning"},
		MaxTokens:    128000,
	}

	tests := []struct {
		capability string
		expected   bool
	}{
		{"code", true},
		{"analysis", true},
		{"planning", true},
		{"reasoning", false},
		{"multimodal", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			result := model.HasCapability(tt.capability)
			if result != tt.expected {
				t.Errorf("HasCapability(%s) = %v, want %v", tt.capability, result, tt.expected)
			}
		})
	}
}

func TestLLMModel_GetProviderFromID(t *testing.T) {
	tests := []struct {
		name     string
		model    *LLMModel
		expected string
	}{
		{
			name: "simple ID with @",
			model: &LLMModel{
				ID:       "gpt-4o-mini@2024-08-06",
				Provider: "openai",
			},
			expected: "gpt-4o-mini",
		},
		{
			name: "ID with provider/model@version",
			model: &LLMModel{
				ID:       "openai/gpt-4@v1",
				Provider: "openai",
			},
			expected: "openai",
		},
		{
			name: "ID without @",
			model: &LLMModel{
				ID:       "claude-3-sonnet",
				Provider: "anthropic",
			},
			expected: "anthropic", // Falls back to provider field
		},
		{
			name: "empty ID",
			model: &LLMModel{
				ID:       "",
				Provider: "openai",
			},
			expected: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.model.GetProviderFromID()
			if result != tt.expected {
				t.Errorf("GetProviderFromID() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestValidProviders(t *testing.T) {
	expectedProviders := []string{"openai", "anthropic", "azure", "local"}

	if len(ValidProviders) != len(expectedProviders) {
		t.Errorf("ValidProviders length = %d, want %d", len(ValidProviders), len(expectedProviders))
	}

	for _, expected := range expectedProviders {
		found := false
		for _, provider := range ValidProviders {
			if provider == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected provider %s not found in ValidProviders", expected)
		}
	}
}

func TestValidCapabilities(t *testing.T) {
	expectedCapabilities := []string{"code", "analysis", "planning", "reasoning", "multimodal", "function_calling"}

	if len(ValidCapabilities) != len(expectedCapabilities) {
		t.Errorf("ValidCapabilities length = %d, want %d", len(ValidCapabilities), len(expectedCapabilities))
	}

	for _, expected := range expectedCapabilities {
		found := false
		for _, capability := range ValidCapabilities {
			if capability == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected capability %s not found in ValidCapabilities", expected)
		}
	}
}

func TestIsValidProvider(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"openai", true},
		{"anthropic", true},
		{"azure", true},
		{"local", true},
		{"invalid", false},
		{"", false},
		{"OPENAI", false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			result := isValidProvider(tt.provider)
			if result != tt.expected {
				t.Errorf("isValidProvider(%s) = %v, want %v", tt.provider, result, tt.expected)
			}
		})
	}
}

func TestIsValidCapability(t *testing.T) {
	tests := []struct {
		capability string
		expected   bool
	}{
		{"code", true},
		{"analysis", true},
		{"planning", true},
		{"reasoning", true},
		{"multimodal", true},
		{"function_calling", true},
		{"invalid", false},
		{"", false},
		{"CODE", false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			result := isValidCapability(tt.capability)
			if result != tt.expected {
				t.Errorf("isValidCapability(%s) = %v, want %v", tt.capability, result, tt.expected)
			}
		})
	}
}

// TestTime_JSONMarshaling tests JSON marshaling and unmarshaling of the custom Time type
func TestTime_JSONMarshaling(t *testing.T) {
	now := time.Date(2025, time.January, 15, 10, 30, 45, 0, time.UTC)

	// Test cases for JSON marshaling/unmarshaling
	tests := []struct {
		name     string
		time     Time
		expected string
	}{
		{
			name:     "valid_time",
			time:     Time(now),
			expected: `"` + now.Format(time.RFC3339) + `"`,
		},
		{
			name:     "zero_time",
			time:     Time{},
			expected: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			marshaled, err := tt.time.MarshalJSON()
			if err != nil {
				t.Errorf("MarshalJSON() error = %v", err)
				return
			}

			if string(marshaled) != tt.expected {
				t.Errorf("MarshalJSON() = %s, want %s", string(marshaled), tt.expected)
			}

			// Test unmarshaling (skip for zero time as it becomes null)
			if tt.name != "zero_time" {
				var unmarshaled Time
				err = unmarshaled.UnmarshalJSON(marshaled)
				if err != nil {
					t.Errorf("UnmarshalJSON() error = %v", err)
					return
				}

				// Compare as timestamps since exact precision might differ
				originalTime := time.Time(tt.time)
				unmarshaledTime := time.Time(unmarshaled)
				if !originalTime.Equal(unmarshaledTime) {
					t.Errorf("UnmarshalJSON() time mismatch: got %v, want %v", unmarshaledTime, originalTime)
				}
			}
		})
	}

	// Test unmarshaling edge cases
	edgeCases := []struct {
		name     string
		input    []byte
		wantErr  bool
		expected Time
	}{
		{
			name:     "null_input",
			input:    []byte("null"),
			wantErr:  false,
			expected: Time{},
		},
		{
			name:     "empty_string",
			input:    []byte(`""`),
			wantErr:  false,
			expected: Time{},
		},
		{
			name:    "invalid_format",
			input:   []byte(`"invalid-date"`),
			wantErr: true,
		},
		{
			name:     "valid_rfc3339",
			input:    []byte(`"2023-01-15T10:30:45Z"`),
			wantErr:  false,
			expected: Time(time.Date(2023, 1, 15, 10, 30, 45, 0, time.UTC)),
		},
	}

	for _, tt := range edgeCases {
		t.Run("unmarshal_"+tt.name, func(t *testing.T) {
			var result Time
			err := result.UnmarshalJSON(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				expectedTime := time.Time(tt.expected)
				resultTime := time.Time(result)
				if !expectedTime.Equal(resultTime) {
					t.Errorf("UnmarshalJSON() = %v, want %v", resultTime, expectedTime)
				}
			}
		})
	}
}

// TestLLMModel_JSONRoundTrip tests complete JSON marshaling roundtrip for LLM models
func TestLLMModel_JSONRoundTrip(t *testing.T) {
	originalModel := &LLMModel{
		ID:           "gpt-4o-mini@2024-08-06",
		Name:         "GPT-4o Mini",
		Provider:     "openai",
		Version:      "2024-08-06",
		Capabilities: []string{"code", "analysis", "reasoning"},
		Config: map[string]string{
			"temperature": "0.7",
			"max_retries": "3",
		},
		MaxTokens:    128000,
		CostPerToken: 0.00015,
		Created:      Time(time.Date(2024, time.July, 1, 12, 0, 0, 0, time.UTC)),
		Updated:      Time(time.Date(2024, time.July, 1, 13, 0, 0, 0, time.UTC)),
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(originalModel)
	if err != nil {
		t.Fatalf("Failed to marshal model: %v", err)
	}

	// Unmarshal back
	var reconstructedModel LLMModel
	err = json.Unmarshal(jsonData, &reconstructedModel)
	if err != nil {
		t.Fatalf("Failed to unmarshal model: %v", err)
	}

	// Verify all fields match
	if reconstructedModel.ID != originalModel.ID {
		t.Errorf("ID mismatch: got %s, want %s", reconstructedModel.ID, originalModel.ID)
	}
	if reconstructedModel.Name != originalModel.Name {
		t.Errorf("Name mismatch: got %s, want %s", reconstructedModel.Name, originalModel.Name)
	}
	if reconstructedModel.Provider != originalModel.Provider {
		t.Errorf("Provider mismatch: got %s, want %s", reconstructedModel.Provider, originalModel.Provider)
	}
	if reconstructedModel.MaxTokens != originalModel.MaxTokens {
		t.Errorf("MaxTokens mismatch: got %d, want %d", reconstructedModel.MaxTokens, originalModel.MaxTokens)
	}

	// Check timestamps
	if !time.Time(reconstructedModel.Created).Equal(time.Time(originalModel.Created)) {
		t.Errorf("Created timestamp mismatch")
	}
	if !time.Time(reconstructedModel.Updated).Equal(time.Time(originalModel.Updated)) {
		t.Errorf("Updated timestamp mismatch")
	}
}

// TestLLMModel_EdgeCases tests edge cases and boundary conditions
func TestLLMModel_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		model   *LLMModel
		wantErr bool
		errMsg  string
	}{
		{
			name: "invalid_id_format",
			model: &LLMModel{
				ID:           "INVALID FORMAT",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1000,
			},
			wantErr: true,
			errMsg:  "invalid model id format",
		},
		{
			name: "special_characters_in_name",
			model: &LLMModel{
				ID:           "test@v1",
				Name:         "Test Model with émojis 🤖 and specîal chars",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1000,
			},
			wantErr: false,
		},
		{
			name: "maximum_capabilities",
			model: &LLMModel{
				ID:           "test@v1",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code", "analysis", "planning", "reasoning", "multimodal", "function_calling"},
				MaxTokens:    1000,
			},
			wantErr: false,
		},
		{
			name: "empty_config_map",
			model: &LLMModel{
				ID:           "test@v1",
				Name:         "Test Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				Config:       map[string]string{},
				MaxTokens:    1000,
			},
			wantErr: false,
		},
		{
			name: "very_high_cost",
			model: &LLMModel{
				ID:           "expensive@v1",
				Name:         "Expensive Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1000,
				CostPerToken: 999.99,
			},
			wantErr: false,
		},
		{
			name: "boundary_max_tokens",
			model: &LLMModel{
				ID:           "boundary@v1",
				Name:         "Boundary Model",
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.model.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), strings.ToLower(strings.Fields(tt.errMsg)[0])) {
					t.Errorf("Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				indexOf(s, substr) >= 0))
}

// Helper function to find index of substring
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
