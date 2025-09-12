package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	llmmodel "github.com/iw2rmb/ploy/internal/arf/models"
	istorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/factory"
)

// ModelConfig represents a single LLM model configuration (ARF-internal adapter)
type ModelConfig struct {
	Name        string  `json:"name" yaml:"name"`
	Provider    string  `json:"provider" yaml:"provider"`
	Endpoint    string  `json:"endpoint" yaml:"endpoint"`
	APIKey      string  `json:"api_key" yaml:"api_key"`
	Model       string  `json:"model" yaml:"model"`
	Default     bool    `json:"default,omitempty" yaml:"default,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

func fetchLLMSModels(ctx context.Context) ([]llmmodel.LLMModel, error) {
	stor, err := factory.New(factory.FactoryConfig{
		Provider:   "seaweedfs",
		Monitoring: factory.MonitoringConfig{Enabled: false},
		Cache:      factory.CacheConfig{Enabled: false},
		Retry:      factory.RetryConfig{Enabled: true, MaxAttempts: 3},
	})
	if err != nil {
		return nil, err
	}
	objects, err := stor.List(ctx, istorage.ListOptions{Prefix: "llms/models/"})
	if err != nil {
		return nil, err
	}
	models := make([]llmmodel.LLMModel, 0, len(objects))
	for _, obj := range objects {
		if obj.Key == "llms/models/__default" {
			continue
		}
		r, err := stor.Get(ctx, obj.Key)
		if err != nil {
			continue
		}
		var m llmmodel.LLMModel
		if json.NewDecoder(r).Decode(&m) == nil {
			models = append(models, m)
		}
		r.Close()
	}
	return models, nil
}

func mapLLMToARF(m llmmodel.LLMModel) *ModelConfig {
	endpoint := ""
	apiKey := ""
	temp := 0.1
	if m.Config != nil {
		if v, ok := m.Config["endpoint"]; ok {
			endpoint = v
		}
		if v, ok := m.Config["api_key"]; ok {
			apiKey = v
		}
		// Best-effort: temperature may not be a float string
	}
	return &ModelConfig{
		Name:        m.Name,
		Provider:    m.Provider,
		Endpoint:    endpoint,
		APIKey:      apiKey,
		Model:       m.ID,
		Default:     false,
		MaxTokens:   m.MaxTokens,
		Temperature: temp,
		CreatedAt:   time.Time(m.Created).UTC().Format(time.RFC3339),
		UpdatedAt:   time.Time(m.Updated).UTC().Format(time.RFC3339),
	}
}

// GetDefaultModel resolves a default model from LLMS registry.
func GetDefaultModel(ctx context.Context) (*ModelConfig, error) {
	models, err := fetchLLMSModels(ctx)
	if err != nil || len(models) == 0 {
		if err == nil {
			err = fmt.Errorf("no models available")
		}
		return nil, err
	}
	for _, m := range models {
		if m.HasCapability("code") {
			return mapLLMToARF(m), nil
		}
	}
	return mapLLMToARF(models[0]), nil
}

// GetModelByName fetches a model by name or id from LLMS registry.
func GetModelByName(ctx context.Context, name string) (*ModelConfig, error) {
	models, err := fetchLLMSModels(ctx)
	if err != nil || len(models) == 0 {
		if err == nil {
			err = fmt.Errorf("no models available")
		}
		return nil, err
	}
	for _, m := range models {
		if m.Name == name || m.ID == name {
			return mapLLMToARF(m), nil
		}
	}
	return nil, fmt.Errorf("model '%s' not found", name)
}
