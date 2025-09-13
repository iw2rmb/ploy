package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Recipe represents a complete code transformation recipe
type Recipe struct {
	// Metadata for recipe identification and management
	Metadata RecipeMetadata `json:"metadata" yaml:"metadata"`

	// Sequential transformation steps
	Steps []RecipeStep `json:"steps" yaml:"steps"`

	// Execution configuration and constraints
	Execution ExecutionConfig `json:"execution,omitempty" yaml:"execution,omitempty"`

	// Validation rules for target codebases
	Validation ValidationRules `json:"validation,omitempty" yaml:"validation,omitempty"`

	// System fields (managed automatically)
	ID         string    `json:"id" yaml:"-"`
	CreatedAt  time.Time `json:"created_at" yaml:"-"`
	UpdatedAt  time.Time `json:"updated_at" yaml:"-"`
	UploadedBy string    `json:"uploaded_by" yaml:"-"`
	Hash       string    `json:"hash" yaml:"-"`    // Content hash for integrity
	Version    string    `json:"version" yaml:"-"` // Semantic version
}

// GenerateID creates a unique ID for the recipe based on name and version
func (r *Recipe) GenerateID() string {
	if r.Metadata.Name == "" {
		return ""
	}
	version := r.Metadata.Version
	if version == "" {
		version = "latest"
	}
	return fmt.Sprintf("%s-%s", r.Metadata.Name, version)
}

// CalculateHash computes the SHA256 hash of the recipe content
func (r *Recipe) CalculateHash() (string, error) {
	// Create a copy without system fields for consistent hashing
	recipeCopy := Recipe{
		Metadata:   r.Metadata,
		Steps:      r.Steps,
		Execution:  r.Execution,
		Validation: r.Validation,
	}

	data, err := json.Marshal(recipeCopy)
	if err != nil {
		return "", fmt.Errorf("failed to marshal recipe for hashing: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Validate performs basic validation on the recipe
func (r *Recipe) Validate() error {
	if r.Metadata.Name == "" {
		return fmt.Errorf("recipe name is required")
	}

	if r.Metadata.Description == "" {
		return fmt.Errorf("recipe description is required")
	}

	if len(r.Steps) == 0 {
		return fmt.Errorf("recipe must have at least one step")
	}

	// Validate each step
	for i, step := range r.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("step %d validation failed: %w", i+1, err)
		}
	}

	// Validate execution config if present
	if err := r.Execution.Validate(); err != nil {
		return fmt.Errorf("execution config validation failed: %w", err)
	}

	// Validate validation rules if present
	if err := r.Validation.Validate(); err != nil {
		return fmt.Errorf("validation rules validation failed: %w", err)
	}

	return nil
}

// SetSystemFields updates system-managed fields
func (r *Recipe) SetSystemFields(uploadedBy string) {
	now := time.Now()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	r.UploadedBy = uploadedBy
	r.ID = r.GenerateID()

	// Calculate content hash
	hash, _ := r.CalculateHash()
	r.Hash = hash

	// Use metadata version as recipe version
	if r.Metadata.Version != "" {
		r.Version = r.Metadata.Version
	} else {
		r.Version = "1.0.0"
	}
}

// IsCompatibleWith checks if the recipe is compatible with the given platform
func (r *Recipe) IsCompatibleWith(platform string) bool {
	// If no platform requirements specified, assume compatible
	if r.Metadata.MinPlatform == "" && r.Metadata.MaxPlatform == "" {
		return true
	}

	// TODO: Implement semantic version comparison
	// For now, return true
	return true
}

// GetRequiredTools returns a list of tools required for recipe execution
func (r *Recipe) GetRequiredTools() []string {
	tools := make(map[string]bool)

	for _, step := range r.Steps {
		switch step.Type {
		case StepTypeOpenRewrite:
			tools["maven"] = true
			tools["java"] = true
		case StepTypeShellScript:
			tools["bash"] = true
		case StepTypeASTTransform:
			// Depends on language
			if config, ok := step.Config["language"].(string); ok {
				switch config {
				case "java":
					tools["java"] = true
				case "go":
					tools["go"] = true
				case "python":
					tools["python"] = true
				}
			}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(tools))
	for tool := range tools {
		result = append(result, tool)
	}

	return result
}

// Clone creates a deep copy of the recipe
func (r *Recipe) Clone() *Recipe {
	// Marshal and unmarshal to create a deep copy
	data, _ := json.Marshal(r)
	var clone Recipe
	_ = json.Unmarshal(data, &clone)
	return &clone
}
