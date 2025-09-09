package arf

import (
	"github.com/iw2rmb/ploy/api/arf/models"
)

// WASMRecipe represents WebAssembly specific transformation recipes
type WASMRecipe struct {
	*models.Recipe
	OptimizationLevel   int          `json:"optimization_level"`
	TargetFeatures      []string     `json:"target_features"`
	PolyfillsRequired   []string     `json:"polyfills_required"`
	MemoryConfiguration MemoryConfig `json:"memory_config"`
}

// MemoryConfig defines WASM memory configuration
type MemoryConfig struct {
	InitialPages int  `json:"initial_pages"`
	MaximumPages int  `json:"maximum_pages"`
	Shared       bool `json:"shared"`
}
