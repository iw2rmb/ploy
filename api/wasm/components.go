// Package wasm provides WebAssembly Component Model support for Ploy
package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tetratelabs/wazero"
)

// ComponentManager manages WebAssembly Component Model applications
type ComponentManager struct {
	runtime wazero.Runtime
	modules map[string]wazero.CompiledModule
}

// ComponentSpec defines a multi-module WASM application configuration
type ComponentSpec struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	MainModule  string            `json:"main_module"`
	Dependencies []ComponentDep   `json:"dependencies"`
	Interfaces  []InterfaceSpec   `json:"interfaces"`
	Resources   ResourceLimits    `json:"resources"`
	Environment map[string]string `json:"environment"`
}

// ComponentDep represents a dependency module in a WASM component
type ComponentDep struct {
	Name     string `json:"name"`
	Module   string `json:"module"`
	Version  string `json:"version,omitempty"`
	Required bool   `json:"required"`
}

// InterfaceSpec defines exported and imported interfaces for components
type InterfaceSpec struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"` // "export" or "import"
	Methods []string `json:"methods"`
}

// ResourceLimits defines resource constraints for component execution
type ResourceLimits struct {
	MaxMemoryMB     int `json:"max_memory_mb"`
	MaxExecutionSec int `json:"max_execution_sec"`
	MaxModules      int `json:"max_modules"`
}

// NewComponentManager creates a new WebAssembly Component Manager
func NewComponentManager(ctx context.Context) *ComponentManager {
	return &ComponentManager{
		runtime: wazero.NewRuntime(ctx),
		modules: make(map[string]wazero.CompiledModule),
	}
}

// LoadComponent loads and validates a WASM component with its dependencies
func (cm *ComponentManager) LoadComponent(ctx context.Context, spec ComponentSpec, artifactPath string) error {
	// Validate component specification
	if err := cm.validateComponentSpec(spec); err != nil {
		return fmt.Errorf("invalid component specification: %w", err)
	}

	// Load main WASM module
	mainModulePath := filepath.Join(artifactPath, spec.MainModule)
	mainBytes, err := os.ReadFile(mainModulePath)
	if err != nil {
		return fmt.Errorf("failed to read main module %s: %w", spec.MainModule, err)
	}

	mainModule, err := cm.runtime.CompileModule(ctx, mainBytes)
	if err != nil {
		return fmt.Errorf("failed to compile main module: %w", err)
	}
	cm.modules[spec.Name+"_main"] = mainModule

	// Load dependency modules
	for _, dep := range spec.Dependencies {
		depPath := filepath.Join(artifactPath, dep.Module)
		depBytes, err := os.ReadFile(depPath)
		if err != nil {
			if dep.Required {
				return fmt.Errorf("failed to read required dependency %s: %w", dep.Name, err)
			}
			continue // Skip optional dependencies that aren't found
		}

		depModule, err := cm.runtime.CompileModule(ctx, depBytes)
		if err != nil {
			return fmt.Errorf("failed to compile dependency %s: %w", dep.Name, err)
		}
		cm.modules[spec.Name+"_"+dep.Name] = depModule
	}

	return nil
}

// InstantiateComponent creates and links component modules for execution
func (cm *ComponentManager) InstantiateComponent(ctx context.Context, spec ComponentSpec) error {
	// Apply resource limits
	moduleConfig := wazero.NewModuleConfig().WithName(spec.Name)

	// Configure environment variables
	for key, value := range spec.Environment {
		moduleConfig = moduleConfig.WithEnv(key, value)
	}

	// Configure memory limits if specified
	if spec.Resources.MaxMemoryMB > 0 {
		// Note: wazero memory limits are handled at runtime level
		// This would require custom runtime configuration
	}

	// Instantiate main module first
	mainModuleKey := spec.Name + "_main"
	mainModule, exists := cm.modules[mainModuleKey]
	if !exists {
		return fmt.Errorf("main module not found: %s", mainModuleKey)
	}

	_, err := cm.runtime.InstantiateModule(ctx, mainModule, moduleConfig)
	if err != nil {
		return fmt.Errorf("failed to instantiate main module: %w", err)
	}

	// Instantiate dependency modules
	for _, dep := range spec.Dependencies {
		depModuleKey := spec.Name + "_" + dep.Name
		depModule, exists := cm.modules[depModuleKey]
		if !exists {
			if dep.Required {
				return fmt.Errorf("required dependency module not found: %s", dep.Name)
			}
			continue // Skip optional dependencies
		}

		depConfig := wazero.NewModuleConfig().WithName(dep.Name)
		_, err := cm.runtime.InstantiateModule(ctx, depModule, depConfig)
		if err != nil {
			return fmt.Errorf("failed to instantiate dependency %s: %w", dep.Name, err)
		}
	}

	return nil
}

// ExecuteComponent runs the main module of a component with proper error handling
func (cm *ComponentManager) ExecuteComponent(ctx context.Context, spec ComponentSpec, args []string) error {
	// Find the instantiated main module
	module := cm.runtime.Module(spec.Name)
	if module == nil {
		return fmt.Errorf("component not instantiated: %s", spec.Name)
	}

	// Execute the main function or _start
	if startFn := module.ExportedFunction("_start"); startFn != nil {
		_, err := startFn.Call(ctx)
		return err
	} else if mainFn := module.ExportedFunction("main"); mainFn != nil {
		_, err := mainFn.Call(ctx)
		return err
	}

	return fmt.Errorf("no executable function found in component %s", spec.Name)
}

// ValidateInterfaces checks that component interfaces are properly defined and linked
func (cm *ComponentManager) ValidateInterfaces(spec ComponentSpec) error {
	exportedInterfaces := make(map[string]InterfaceSpec)
	importedInterfaces := make(map[string]InterfaceSpec)

	// Categorize interfaces
	for _, iface := range spec.Interfaces {
		switch iface.Type {
		case "export":
			exportedInterfaces[iface.Name] = iface
		case "import":
			importedInterfaces[iface.Name] = iface
		default:
			return fmt.Errorf("invalid interface type: %s", iface.Type)
		}
	}

	// Check that imported interfaces have corresponding exports
	// (This would be more sophisticated in a real implementation)
	for importName := range importedInterfaces {
		if _, exists := exportedInterfaces[importName]; !exists {
			return fmt.Errorf("imported interface not satisfied: %s", importName)
		}
	}

	return nil
}

// Close releases all resources and compiled modules
func (cm *ComponentManager) Close(ctx context.Context) error {
	// Clear module cache
	cm.modules = make(map[string]wazero.CompiledModule)
	
	// Close runtime
	return cm.runtime.Close(ctx)
}

// validateComponentSpec validates the component specification for correctness
func (cm *ComponentManager) validateComponentSpec(spec ComponentSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("component name is required")
	}

	if spec.MainModule == "" {
		return fmt.Errorf("main module is required")
	}

	// Validate resource limits
	if spec.Resources.MaxMemoryMB < 0 {
		return fmt.Errorf("invalid memory limit: %d", spec.Resources.MaxMemoryMB)
	}

	if spec.Resources.MaxExecutionSec < 0 {
		return fmt.Errorf("invalid execution timeout: %d", spec.Resources.MaxExecutionSec)
	}

	// Validate dependencies
	depNames := make(map[string]bool)
	for _, dep := range spec.Dependencies {
		if dep.Name == "" {
			return fmt.Errorf("dependency name is required")
		}
		if dep.Module == "" {
			return fmt.Errorf("dependency module is required for %s", dep.Name)
		}
		if depNames[dep.Name] {
			return fmt.Errorf("duplicate dependency name: %s", dep.Name)
		}
		depNames[dep.Name] = true
	}

	// Validate interfaces
	for _, iface := range spec.Interfaces {
		if iface.Name == "" {
			return fmt.Errorf("interface name is required")
		}
		if iface.Type != "export" && iface.Type != "import" {
			return fmt.Errorf("invalid interface type: %s", iface.Type)
		}
	}

	return nil
}

// ParseComponentSpec loads and parses a component specification from JSON
func ParseComponentSpec(specPath string) (*ComponentSpec, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read component spec: %w", err)
	}

	var spec ComponentSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse component spec: %w", err)
	}

	return &spec, nil
}

// GenerateComponentSpec creates a basic component specification template
func GenerateComponentSpec(name, mainModule string, dependencies []string) *ComponentSpec {
	spec := &ComponentSpec{
		Name:        name,
		Version:     "1.0.0",
		MainModule:  mainModule,
		Dependencies: make([]ComponentDep, 0, len(dependencies)),
		Interfaces:  []InterfaceSpec{},
		Resources: ResourceLimits{
			MaxMemoryMB:     32,
			MaxExecutionSec: 30,
			MaxModules:      5,
		},
		Environment: make(map[string]string),
	}

	// Add dependencies
	for _, dep := range dependencies {
		spec.Dependencies = append(spec.Dependencies, ComponentDep{
			Name:     strings.TrimSuffix(filepath.Base(dep), ".wasm"),
			Module:   dep,
			Required: true,
		})
	}

	return spec
}

// ListComponents returns information about loaded components
func (cm *ComponentManager) ListComponents() []string {
	var components []string
	for moduleName := range cm.modules {
		if strings.HasSuffix(moduleName, "_main") {
			componentName := strings.TrimSuffix(moduleName, "_main")
			components = append(components, componentName)
		}
	}
	return components
}

// GetComponentInfo returns detailed information about a loaded component
func (cm *ComponentManager) GetComponentInfo(componentName string) map[string]interface{} {
	info := make(map[string]interface{})
	
	// Check if main module exists
	mainKey := componentName + "_main"
	if _, exists := cm.modules[mainKey]; exists {
		info["main_module"] = true
		info["status"] = "loaded"
	} else {
		info["main_module"] = false
		info["status"] = "not_loaded"
		return info
	}

	// Count dependencies
	depCount := 0
	for moduleName := range cm.modules {
		if strings.HasPrefix(moduleName, componentName+"_") && !strings.HasSuffix(moduleName, "_main") {
			depCount++
		}
	}
	info["dependencies_count"] = depCount

	return info
}