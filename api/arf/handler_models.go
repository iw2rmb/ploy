package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/consul/api"
	llmmodel "github.com/iw2rmb/ploy/internal/arf/models"
	istorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/factory"
)

// ModelConfig represents a single LLM model configuration
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

// ModelRegistry represents the complete model registry
type ModelRegistry struct {
	Models []ModelConfig `json:"models" yaml:"models"`
}

const consulKVPath = "ploy/arf/models"

// getConsulClient creates a Consul client
func getConsulClient() (*api.Client, error) {
	config := api.DefaultConfig()
	// Use Consul service discovery if available
	if consulAddr := "consul.service.consul:8500"; consulAddr != "" {
		config.Address = consulAddr
	}
	return api.NewClient(config)
}

// GetModels handles GET /v1/arf/models
func (h *Handler) GetModels(c *fiber.Ctx) error {
	// Deprecated: ARF model registry moved to /v1/llms/models
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error":    "ARF model registry is deprecated",
		"message":  "Use /v1/llms/models for model operations",
		"redirect": "/v1/llms/models",
	})
	client, err := getConsulClient()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to connect to Consul",
			"details": err.Error(),
		})
	}

	kv := client.KV()
	pair, _, err := kv.Get(consulKVPath, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to fetch models from Consul",
			"details": err.Error(),
		})
	}

	registry := ModelRegistry{Models: []ModelConfig{}}
	if pair != nil && pair.Value != nil {
		if err := json.Unmarshal(pair.Value, &registry); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Failed to parse model registry",
				"details": err.Error(),
			})
		}
	}

	return c.JSON(registry)
}

// AddModel handles POST /v1/arf/models
func (h *Handler) AddModel(c *fiber.Ctx) error {
	// Deprecated: ARF model registry moved to /v1/llms/models
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error":    "ARF model registry is deprecated",
		"message":  "Use /v1/llms/models for model creation",
		"redirect": "/v1/llms/models",
	})
	var newModel ModelConfig
	if err := c.BodyParser(&newModel); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Validate required fields
	if newModel.Name == "" || newModel.Provider == "" || newModel.Endpoint == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Missing required fields",
			"details": "name, provider, and endpoint are required",
		})
	}

	// Set defaults
	if newModel.MaxTokens == 0 {
		newModel.MaxTokens = 4096
	}
	if newModel.Temperature == 0 {
		newModel.Temperature = 0.1
	}
	newModel.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	newModel.UpdatedAt = newModel.CreatedAt

	client, err := getConsulClient()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to connect to Consul",
			"details": err.Error(),
		})
	}

	kv := client.KV()

	// Get existing registry
	pair, _, err := kv.Get(consulKVPath, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to fetch existing models",
			"details": err.Error(),
		})
	}

	registry := ModelRegistry{Models: []ModelConfig{}}
	if pair != nil && pair.Value != nil {
		if err := json.Unmarshal(pair.Value, &registry); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Failed to parse existing registry",
				"details": err.Error(),
			})
		}
	}

	// Check for duplicate name
	for i, model := range registry.Models {
		if model.Name == newModel.Name {
			// Update existing model
			newModel.CreatedAt = model.CreatedAt
			registry.Models[i] = newModel
			goto save
		}
	}

	// Add new model
	registry.Models = append(registry.Models, newModel)

save:
	// If this model is set as default, unset others
	if newModel.Default {
		for i := range registry.Models {
			if registry.Models[i].Name != newModel.Name {
				registry.Models[i].Default = false
			}
		}
	}

	// Save updated registry
	data, err := json.Marshal(registry)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to marshal registry",
			"details": err.Error(),
		})
	}

	p := &api.KVPair{Key: consulKVPath, Value: data}
	_, err = kv.Put(p, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to save model registry",
			"details": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": fmt.Sprintf("Model '%s' added successfully", newModel.Name),
		"model":   newModel,
	})
}

// RemoveModel handles DELETE /v1/arf/models/:name
func (h *Handler) RemoveModel(c *fiber.Ctx) error {
	// Deprecated: ARF model registry moved to /v1/llms/models
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error":    "ARF model registry is deprecated",
		"message":  "Use /v1/llms/models for model deletion",
		"redirect": "/v1/llms/models",
	})
	modelName := c.Params("name")
	if modelName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Model name is required",
		})
	}

	client, err := getConsulClient()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to connect to Consul",
			"details": err.Error(),
		})
	}

	kv := client.KV()

	// Get existing registry
	pair, _, err := kv.Get(consulKVPath, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to fetch existing models",
			"details": err.Error(),
		})
	}

	if pair == nil || pair.Value == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Model not found",
		})
	}

	registry := ModelRegistry{Models: []ModelConfig{}}
	if err := json.Unmarshal(pair.Value, &registry); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to parse existing registry",
			"details": err.Error(),
		})
	}

	// Find and remove model
	found := false
	newModels := []ModelConfig{}
	for _, model := range registry.Models {
		if model.Name != modelName {
			newModels = append(newModels, model)
		} else {
			found = true
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("Model '%s' not found", modelName),
		})
	}

	registry.Models = newModels

	// Save updated registry
	data, err := json.Marshal(registry)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to marshal registry",
			"details": err.Error(),
		})
	}

	p := &api.KVPair{Key: consulKVPath, Value: data}
	_, err = kv.Put(p, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to save model registry",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Model '%s' removed successfully", modelName),
	})
}

// SetDefaultModel handles POST /v1/arf/models/:name/set-default
func (h *Handler) SetDefaultModel(c *fiber.Ctx) error {
	// Deprecated: ARF model registry moved to /v1/llms/models
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error":    "ARF model registry is deprecated",
		"message":  "Use /v1/llms/models/{id} to update models",
		"redirect": "/v1/llms/models",
	})
	modelName := c.Params("name")
	if modelName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Model name is required",
		})
	}

	client, err := getConsulClient()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to connect to Consul",
			"details": err.Error(),
		})
	}

	kv := client.KV()

	// Get existing registry
	pair, _, err := kv.Get(consulKVPath, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to fetch existing models",
			"details": err.Error(),
		})
	}

	if pair == nil || pair.Value == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No models configured",
		})
	}

	registry := ModelRegistry{Models: []ModelConfig{}}
	if err := json.Unmarshal(pair.Value, &registry); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to parse existing registry",
			"details": err.Error(),
		})
	}

	// Find model and set as default
	found := false
	for i := range registry.Models {
		if registry.Models[i].Name == modelName {
			registry.Models[i].Default = true
			registry.Models[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			found = true
		} else {
			registry.Models[i].Default = false
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("Model '%s' not found", modelName),
		})
	}

	// Save updated registry
	data, err := json.Marshal(registry)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to marshal registry",
			"details": err.Error(),
		})
	}

	p := &api.KVPair{Key: consulKVPath, Value: data}
	_, err = kv.Put(p, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to save model registry",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Model '%s' set as default", modelName),
	})
}

// ImportModels handles PUT /v1/arf/models
func (h *Handler) ImportModels(c *fiber.Ctx) error {
	// Deprecated: ARF model registry moved to /v1/llms/models
	return c.Status(fiber.StatusGone).JSON(fiber.Map{
		"error":    "ARF model registry is deprecated",
		"message":  "Use /v1/llms/models for model import",
		"redirect": "/v1/llms/models",
	})
	var registry ModelRegistry
	if err := c.BodyParser(&registry); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Validate and set defaults for all models
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range registry.Models {
		if registry.Models[i].Name == "" || registry.Models[i].Provider == "" || registry.Models[i].Endpoint == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   fmt.Sprintf("Model at index %d missing required fields", i),
				"details": "name, provider, and endpoint are required",
			})
		}
		if registry.Models[i].MaxTokens == 0 {
			registry.Models[i].MaxTokens = 4096
		}
		if registry.Models[i].Temperature == 0 {
			registry.Models[i].Temperature = 0.1
		}
		registry.Models[i].CreatedAt = now
		registry.Models[i].UpdatedAt = now
	}

	client, err := getConsulClient()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to connect to Consul",
			"details": err.Error(),
		})
	}

	// Save registry
	data, err := json.Marshal(registry)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to marshal registry",
			"details": err.Error(),
		})
	}

	kv := client.KV()
	p := &api.KVPair{Key: consulKVPath, Value: data}
	_, err = kv.Put(p, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to save model registry",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Imported %d models successfully", len(registry.Models)),
		"models":  registry.Models,
	})
}

// GetDefaultModel retrieves the default model configuration
func GetDefaultModel(ctx context.Context) (*ModelConfig, error) {
	client, err := getConsulClient()
	if err != nil {
		// Fallback to LLMS registry storage when Consul is unavailable
		return getDefaultModelFromLLMS(ctx)
	}

	kv := client.KV()
	pair, _, err := kv.Get(consulKVPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}

	if pair == nil || pair.Value == nil {
		// Fallback to LLMS registry if ARF registry empty
		return getDefaultModelFromLLMS(ctx)
	}

	registry := ModelRegistry{Models: []ModelConfig{}}
	if err := json.Unmarshal(pair.Value, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	// Find default model
	for _, model := range registry.Models {
		if model.Default {
			return &model, nil
		}
	}

	// Return first model if no default
	if len(registry.Models) > 0 {
		return &registry.Models[0], nil
	}

	// Fallback to LLMS registry
	return getDefaultModelFromLLMS(ctx)
}

// GetModelByName retrieves a specific model configuration
func GetModelByName(ctx context.Context, name string) (*ModelConfig, error) {
	client, err := getConsulClient()
	if err != nil {
		return getModelByNameFromLLMS(ctx, name)
	}

	kv := client.KV()
	pair, _, err := kv.Get(consulKVPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}

	if pair == nil || pair.Value == nil {
		return getModelByNameFromLLMS(ctx, name)
	}

	registry := ModelRegistry{Models: []ModelConfig{}}
	if err := json.Unmarshal(pair.Value, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	for _, model := range registry.Models {
		if model.Name == name {
			return &model, nil
		}
	}
	// Fallback search in LLMS registry
	return getModelByNameFromLLMS(ctx, name)
}

// RegisterModelRoutes registers all model management routes
func RegisterModelRoutes(app *fiber.App, handler *Handler) {
	models := app.Group("/v1/arf/models")

	models.Get("/", handler.GetModels)
	models.Post("/", handler.AddModel)
	models.Put("/", handler.ImportModels)
	models.Delete("/:name", handler.RemoveModel)
	models.Post("/:name/set-default", handler.SetDefaultModel)
}

// fetchLLMSModels loads all LLMS models from the storage-backed registry.
func fetchLLMSModels(ctx context.Context) ([]llmmodel.LLMModel, error) {
	// Use factory defaults (SeaweedFS) driven by env; matches other API components
	stor, err := factory.New(factory.FactoryConfig{
		Provider:   "seaweedfs",
		Monitoring: factory.MonitoringConfig{Enabled: false},
		Cache:      factory.CacheConfig{Enabled: false},
		Retry:      factory.RetryConfig{Enabled: true, MaxAttempts: 3},
	})
	if err != nil {
		return nil, err
	}
	// List models under keyspace llms/models/
	objects, err := stor.List(ctx, istorage.ListOptions{Prefix: "llms/models/"})
	if err != nil {
		return nil, err
	}
	models := make([]llmmodel.LLMModel, 0, len(objects))
	for _, obj := range objects {
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

// mapLLMToARF converts LLMS model to ARF ModelConfig best-effort.
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
		if v, ok := m.Config["temperature"]; ok {
			if f, err := fmt.Sscanf(v, "%f", &temp); err == nil && f == 1 {
				// parsed into temp
			}
		}
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

func getDefaultModelFromLLMS(ctx context.Context) (*ModelConfig, error) {
	models, err := fetchLLMSModels(ctx)
	if err != nil || len(models) == 0 {
		if err == nil {
			err = fmt.Errorf("no models available")
		}
		return nil, err
	}
	// Heuristic: prefer models that include 'code' capability, else first
	for _, m := range models {
		if m.HasCapability("code") {
			return mapLLMToARF(m), nil
		}
	}
	return mapLLMToARF(models[0]), nil
}

func getModelByNameFromLLMS(ctx context.Context, name string) (*ModelConfig, error) {
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
