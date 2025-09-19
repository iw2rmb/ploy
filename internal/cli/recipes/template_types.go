package recipes

import (
	models "github.com/iw2rmb/ploy/api/recipes/models"
)

// RecipeTemplate represents a template for creating recipes.
type RecipeTemplate struct {
	ID          string             `json:"id" yaml:"id"`
	Name        string             `json:"name" yaml:"name"`
	Description string             `json:"description" yaml:"description"`
	Category    string             `json:"category" yaml:"category"`
	Template    models.Recipe      `json:"template" yaml:"template"`
	Variables   []TemplateVariable `json:"variables,omitempty" yaml:"variables,omitempty"`
	Prompts     []TemplatePrompt   `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Examples    []RecipeExample    `json:"examples,omitempty" yaml:"examples,omitempty"`
}

// TemplateVariable represents a variable in a recipe template.
type TemplateVariable struct {
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	Type         string   `json:"type" yaml:"type"`
	Required     bool     `json:"required" yaml:"required"`
	DefaultValue string   `json:"default_value,omitempty" yaml:"default_value,omitempty"`
	Options      []string `json:"options,omitempty" yaml:"options,omitempty"`
	Pattern      string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
}

// TemplatePrompt represents an interactive prompt.
type TemplatePrompt struct {
	Field      string   `json:"field" yaml:"field"`
	Message    string   `json:"message" yaml:"message"`
	Type       string   `json:"type" yaml:"type"`
	Options    []string `json:"options,omitempty" yaml:"options,omitempty"`
	Default    string   `json:"default,omitempty" yaml:"default,omitempty"`
	Required   bool     `json:"required" yaml:"required"`
	Validation string   `json:"validation,omitempty" yaml:"validation,omitempty"`
}

// RecipeExample represents an example for a template.
type RecipeExample struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description" yaml:"description"`
	Values      map[string]string `json:"values" yaml:"values"`
}
