package mods

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPTool_ValidateEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		tool        MCPTool
		expectError bool
	}{
		{
			name: "valid mcp endpoint",
			tool: MCPTool{
				Name:     "test-tool",
				Endpoint: "mcp://fs",
			},
			expectError: false,
		},
		{
			name: "valid http endpoint",
			tool: MCPTool{
				Name:     "test-tool",
				Endpoint: "http://localhost:8080",
			},
			expectError: false,
		},
		{
			name: "valid https endpoint",
			tool: MCPTool{
				Name:     "test-tool",
				Endpoint: "https://api.example.com",
			},
			expectError: false,
		},
		{
			name: "empty endpoint",
			tool: MCPTool{
				Name:     "test-tool",
				Endpoint: "",
			},
			expectError: true,
		},
		{
			name: "invalid endpoint scheme",
			tool: MCPTool{
				Name:     "test-tool",
				Endpoint: "ftp://example.com",
			},
			expectError: true,
		},
		{
			name: "empty tool name",
			tool: MCPTool{
				Name:     "",
				Endpoint: "mcp://fs",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tool.ValidateEndpoint()
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestMCPBudgets_ValidateTimeout(t *testing.T) {
	tests := []struct {
		name        string
		budgets     MCPBudgets
		expectError bool
		expected    time.Duration
	}{
		{
			name: "valid timeout",
			budgets: MCPBudgets{
				Timeout: "30m",
			},
			expectError: false,
			expected:    30 * time.Minute,
		},
		{
			name:        "empty timeout uses default",
			budgets:     MCPBudgets{},
			expectError: false,
			expected:    30 * time.Minute,
		},
		{
			name: "invalid timeout format",
			budgets: MCPBudgets{
				Timeout: "invalid",
			},
			expectError: true,
		},
		{
			name: "negative timeout",
			budgets: MCPBudgets{
				Timeout: "-5m",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, err := tt.budgets.ValidateTimeout()
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
			if !tt.expectError && duration != tt.expected {
				t.Errorf("expected duration %v but got %v", tt.expected, duration)
			}
		})
	}
}

func TestMCPConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      MCPConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: MCPConfig{
				Tools: []MCPTool{
					{
						Name:     "fs",
						Endpoint: "mcp://fs",
					},
					{
						Name:     "search",
						Endpoint: "mcp://rg",
					},
				},
				Context: []string{"src/**"},
				Budgets: MCPBudgets{
					Timeout: "30m",
				},
			},
			expectError: false,
		},
		{
			name:        "empty config is valid",
			config:      MCPConfig{},
			expectError: false,
		},
		{
			name: "invalid tool endpoint",
			config: MCPConfig{
				Tools: []MCPTool{
					{
						Name:     "fs",
						Endpoint: "invalid://endpoint",
					},
				},
			},
			expectError: true,
		},
		{
			name: "invalid timeout",
			config: MCPConfig{
				Budgets: MCPBudgets{
					Timeout: "invalid",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestMCPConfig_ToEnvironmentConfig(t *testing.T) {
	config := MCPConfig{
		Tools: []MCPTool{
			{
				Name:     "file-system",
				Endpoint: "mcp://fs",
			},
			{
				Name:     "search",
				Endpoint: "mcp://rg",
			},
		},
		Context: []string{
			"src/**/*.java",
			"pom.xml",
		},
		Budgets: MCPBudgets{
			MaxTokens: 1000,
			MaxCost:   5,
			Timeout:   "20m",
		},
		Model: "gpt-4o",
		Prompts: []string{
			"Fix the null pointer exception",
			"Add proper error handling",
		},
	}

	envConfig, err := config.ToEnvironmentConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify tools JSON
	if envConfig.MCPToolsJSON == "" {
		t.Error("expected non-empty MCPToolsJSON")
	}

	var tools []MCPTool
	if err := json.Unmarshal([]byte(envConfig.MCPToolsJSON), &tools); err != nil {
		t.Errorf("failed to unmarshal tools JSON: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Verify context JSON
	if envConfig.MCPContextJSON == "" {
		t.Error("expected non-empty MCPContextJSON")
	}

	var context []string
	if err := json.Unmarshal([]byte(envConfig.MCPContextJSON), &context); err != nil {
		t.Errorf("failed to unmarshal context JSON: %v", err)
	}

	if len(context) != 2 {
		t.Errorf("expected 2 context items, got %d", len(context))
	}

	// Verify endpoints JSON
	if envConfig.MCPEndpointsJSON == "" {
		t.Error("expected non-empty MCPEndpointsJSON")
	}

	var endpoints map[string]string
	if err := json.Unmarshal([]byte(envConfig.MCPEndpointsJSON), &endpoints); err != nil {
		t.Errorf("failed to unmarshal endpoints JSON: %v", err)
	}

	if endpoints["file-system"] != "mcp://fs" {
		t.Errorf("expected file-system endpoint 'mcp://fs', got %s", endpoints["file-system"])
	}

	// Verify timeout
	if envConfig.MCPTimeout != "20m" {
		t.Errorf("expected timeout '20m', got %s", envConfig.MCPTimeout)
	}

	// Verify security mode
	if envConfig.MCPSecurityMode != "allowlist" {
		t.Errorf("expected security mode 'allowlist', got %s", envConfig.MCPSecurityMode)
	}
}

func TestMCPContextPrefetcher_PrefetchContext(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "mcp_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	config := &MCPConfig{
		Tools: []MCPTool{
			{
				Name:     "file-system",
				Endpoint: "mcp://fs",
			},
		},
		Context: []string{
			"src/**/*.java",
			"pom.xml",
		},
	}

	prefetcher := NewMCPContextPrefetcher(config, tempDir)

	// Test prefetching
	if err := prefetcher.PrefetchContext(); err != nil {
		t.Errorf("unexpected error during prefetch: %v", err)
	}

	// Verify context directory was created
	contextDir := filepath.Join(tempDir, "context")
	if _, err := os.Stat(contextDir); os.IsNotExist(err) {
		t.Error("context directory was not created")
	}

	// Verify pattern manifests were created
	pattern0Path := filepath.Join(contextDir, "pattern_0_manifest.json")
	if _, err := os.Stat(pattern0Path); os.IsNotExist(err) {
		t.Error("pattern_0_manifest.json was not created")
	}

	pattern1Path := filepath.Join(contextDir, "pattern_1_manifest.json")
	if _, err := os.Stat(pattern1Path); os.IsNotExist(err) {
		t.Error("pattern_1_manifest.json was not created")
	}

	// Verify manifest content
	manifestData, err := os.ReadFile(pattern0Path)
	if err != nil {
		t.Errorf("failed to read pattern manifest: %v", err)
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Errorf("failed to unmarshal pattern manifest: %v", err)
	}

	if manifest["pattern"] != "src/**/*.java" {
		t.Errorf("expected pattern 'src/**/*.java', got %v", manifest["pattern"])
	}

	if manifest["type"] != "file_pattern" {
		t.Errorf("expected type 'file_pattern', got %v", manifest["type"])
	}
}

func TestMCPContextPrefetcher_CreateContextManifest(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "mcp_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	config := &MCPConfig{
		Tools: []MCPTool{
			{
				Name:     "file-system",
				Endpoint: "mcp://fs",
			},
		},
		Context: []string{"src/**"},
	}

	prefetcher := NewMCPContextPrefetcher(config, tempDir)

	// Create context manifest
	if err := prefetcher.CreateContextManifest(); err != nil {
		t.Errorf("unexpected error creating context manifest: %v", err)
	}

	// Verify manifest file was created
	manifestPath := filepath.Join(tempDir, "context", "mcp_context_manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Error("context manifest was not created")
	}

	// Verify manifest content
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Errorf("failed to read context manifest: %v", err)
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Errorf("failed to unmarshal context manifest: %v", err)
	}

	if manifest["workspace_dir"] != tempDir {
		t.Errorf("expected workspace_dir '%s', got %v", tempDir, manifest["workspace_dir"])
	}

	contextItems, ok := manifest["context_items"].([]interface{})
	if !ok {
		t.Error("expected context_items to be an array")
	}

	if len(contextItems) != 1 {
		t.Errorf("expected 1 context item, got %d", len(contextItems))
	}

	if contextItems[0] != "src/**" {
		t.Errorf("expected context item 'src/**', got %v", contextItems[0])
	}
}

func TestGetMCPEnvironmentConfig_NilConfig(t *testing.T) {
	envConfig := getMCPEnvironmentConfig(nil)

	if envConfig.MCPToolsJSON != "[]" {
		t.Errorf("expected empty tools array, got %s", envConfig.MCPToolsJSON)
	}

	if envConfig.MCPContextJSON != "[]" {
		t.Errorf("expected empty context array, got %s", envConfig.MCPContextJSON)
	}

	if envConfig.MCPEndpointsJSON != "{}" {
		t.Errorf("expected empty endpoints object, got %s", envConfig.MCPEndpointsJSON)
	}

	if envConfig.MCPTimeout != "30m" {
		t.Errorf("expected default timeout '30m', got %s", envConfig.MCPTimeout)
	}

	if envConfig.MCPSecurityMode != "allowlist" {
		t.Errorf("expected security mode 'allowlist', got %s", envConfig.MCPSecurityMode)
	}
}

func TestGetDefaultMCPConfig(t *testing.T) {
	config := GetDefaultMCPConfig()

	if len(config.Tools) != 2 {
		t.Errorf("expected 2 default tools, got %d", len(config.Tools))
	}

	if config.Tools[0].Name != "file-system" {
		t.Errorf("expected first tool to be 'file-system', got %s", config.Tools[0].Name)
	}

	if config.Tools[0].Endpoint != "mcp://fs" {
		t.Errorf("expected first tool endpoint 'mcp://fs', got %s", config.Tools[0].Endpoint)
	}

	if config.Tools[1].Name != "search" {
		t.Errorf("expected second tool to be 'search', got %s", config.Tools[1].Name)
	}

	if config.Tools[1].Endpoint != "mcp://rg" {
		t.Errorf("expected second tool endpoint 'mcp://rg', got %s", config.Tools[1].Endpoint)
	}

	if len(config.Context) != 3 {
		t.Errorf("expected 3 default context items, got %d", len(config.Context))
	}

	if config.Budgets.Timeout != "30m" {
		t.Errorf("expected default timeout '30m', got %s", config.Budgets.Timeout)
	}
}

// Benchmark tests for MCP integration performance

func BenchmarkMCPConfig_Validate(b *testing.B) {
	config := MCPConfig{
		Tools: []MCPTool{
			{Name: "fs", Endpoint: "mcp://fs"},
			{Name: "search", Endpoint: "mcp://rg"},
			{Name: "web", Endpoint: "https://api.example.com"},
		},
		Context: []string{"src/**", "test/**", "docs/**"},
		Budgets: MCPBudgets{Timeout: "30m"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkMCPConfig_ToEnvironmentConfig(b *testing.B) {
	config := MCPConfig{
		Tools: []MCPTool{
			{Name: "file-system", Endpoint: "mcp://fs"},
			{Name: "search", Endpoint: "mcp://rg"},
			{Name: "web-scraper", Endpoint: "https://api.example.com"},
		},
		Context: []string{
			"src/**/*.java",
			"test/**/*.java",
			"pom.xml",
			"https://docs.example.com/api",
		},
		Budgets: MCPBudgets{
			MaxTokens: 8000,
			MaxCost:   10,
			Timeout:   "20m",
		},
		Model: "gpt-4o-mini",
		Prompts: []string{
			"Fix null pointer exceptions",
			"Add proper error handling",
			"Optimize performance",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = config.ToEnvironmentConfig()
	}
}

func BenchmarkMCPContextPrefetcher_PrefetchContext(b *testing.B) {
	config := &MCPConfig{
		Tools: []MCPTool{
			{Name: "file-system", Endpoint: "mcp://fs"},
		},
		Context: []string{
			"src/**/*.java",
			"pom.xml",
			"README.md",
		},
	}

	// Create temp directory once
	tempDir, err := os.MkdirTemp("", "mcp_bench")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create unique workspace for each iteration to avoid conflicts
		workspaceDir := filepath.Join(tempDir, fmt.Sprintf("workspace_%d", i))
		prefetcher := NewMCPContextPrefetcher(config, workspaceDir)
		_ = prefetcher.PrefetchContext()
	}
}

func BenchmarkGetMCPEnvironmentConfig(b *testing.B) {
	config := &MCPConfig{
		Tools: []MCPTool{
			{Name: "file-system", Endpoint: "mcp://fs"},
			{Name: "search", Endpoint: "mcp://rg"},
		},
		Context: []string{"src/**", "test/**"},
		Budgets: MCPBudgets{Timeout: "30m"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getMCPEnvironmentConfig(config)
	}
}

func BenchmarkParseMCPFromInputs(b *testing.B) {
	inputs := map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":     "file-system",
				"endpoint": "mcp://fs",
				"config": map[string]interface{}{
					"max_file_size": "1MB",
				},
			},
			map[string]interface{}{
				"name":     "search",
				"endpoint": "mcp://rg",
			},
		},
		"context": []interface{}{
			"src/**/*.java",
			"test/**/*.java",
		},
		"prompts": []interface{}{
			"Fix null pointers",
			"Add error handling",
		},
		"model": "gpt-4o-mini",
		"budgets": map[string]interface{}{
			"max_tokens": 8000,
			"max_cost":   10,
			"timeout":    "20m",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseMCPFromInputs(inputs)
	}
}
