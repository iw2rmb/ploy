package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MCPTool represents a Model Context Protocol tool configuration
type MCPTool struct {
	Name     string            `yaml:"name" json:"name"`
	Endpoint string            `yaml:"endpoint" json:"endpoint"`
	Config   map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
}

// MCPBudgets represents resource limits for MCP-enabled LLM execution
type MCPBudgets struct {
	MaxTokens int    `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	MaxCost   int    `yaml:"max_cost,omitempty" json:"max_cost,omitempty"`
	Timeout   string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// MCPConfig represents the complete MCP configuration for a workflow step
type MCPConfig struct {
	Tools   []MCPTool  `yaml:"tools,omitempty" json:"tools,omitempty"`
	Context []string   `yaml:"context,omitempty" json:"context,omitempty"`
	Budgets MCPBudgets `yaml:"budgets,omitempty" json:"budgets,omitempty"`
	Model   string     `yaml:"model,omitempty" json:"model,omitempty"`
	Prompts []string   `yaml:"prompts,omitempty" json:"prompts,omitempty"`
}

// MCPEnvironmentConfig represents the environment variables for MCP integration
type MCPEnvironmentConfig struct {
	MCPToolsJSON     string `json:"mcp_tools_json"`
	MCPContextJSON   string `json:"mcp_context_json"`
	MCPEndpointsJSON string `json:"mcp_endpoints_json"`
	MCPBudgetsJSON   string `json:"mcp_budgets_json"`
	MCPPromptsJSON   string `json:"mcp_prompts_json"`
	MCPTimeout       string `json:"mcp_timeout"`
	MCPSecurityMode  string `json:"mcp_security_mode"`
}

// ValidateTimeout validates a timeout string and returns the parsed duration
func (m *MCPBudgets) ValidateTimeout() (time.Duration, error) {
	if m.Timeout == "" {
		return 30 * time.Minute, nil // default timeout
	}

	duration, err := time.ParseDuration(m.Timeout)
	if err != nil {
		return 0, fmt.Errorf("invalid MCP timeout format: %v", err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("MCP timeout must be positive")
	}

	return duration, nil
}

// ValidateEndpoint validates an MCP endpoint URL
func (m *MCPTool) ValidateEndpoint() error {
	if m.Name == "" {
		return fmt.Errorf("MCP tool must have a name")
	}

	if m.Endpoint == "" {
		return fmt.Errorf("MCP tool '%s' must have an endpoint", m.Name)
	}

	// Basic endpoint format validation
	if len(m.Endpoint) < 6 {
		return fmt.Errorf("MCP tool '%s' endpoint is too short", m.Name)
	}

	if !strings.HasPrefix(m.Endpoint, "mcp://") && !strings.HasPrefix(m.Endpoint, "http://") && !strings.HasPrefix(m.Endpoint, "https://") {
		return fmt.Errorf("MCP tool '%s' endpoint must start with mcp://, http://, or https://", m.Name)
	}

	return nil
}

// Validate validates the complete MCP configuration
func (m *MCPConfig) Validate() error {
	// Validate each MCP tool
	for _, tool := range m.Tools {
		if tool.Name == "" {
			return fmt.Errorf("MCP tool must have a name")
		}

		if err := tool.ValidateEndpoint(); err != nil {
			return err
		}
	}

	// Validate budgets timeout if provided
	if _, err := m.Budgets.ValidateTimeout(); err != nil {
		return fmt.Errorf("invalid MCP budgets: %w", err)
	}

	return nil
}

// ToEnvironmentConfig converts MCP configuration to environment variables
func (m *MCPConfig) ToEnvironmentConfig() (*MCPEnvironmentConfig, error) {
	config := &MCPEnvironmentConfig{
		MCPSecurityMode: "allowlist", // default security mode
	}

	// Marshal tools to JSON
	if len(m.Tools) > 0 {
		toolsJSON, err := json.Marshal(m.Tools)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal MCP tools: %w", err)
		}
		config.MCPToolsJSON = string(toolsJSON)

		// Create endpoints mapping
		endpoints := make(map[string]string)
		for _, tool := range m.Tools {
			endpoints[tool.Name] = tool.Endpoint
		}
		endpointsJSON, err := json.Marshal(endpoints)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal MCP endpoints: %w", err)
		}
		config.MCPEndpointsJSON = string(endpointsJSON)
	}

	// Marshal context to JSON
	if len(m.Context) > 0 {
		contextJSON, err := json.Marshal(m.Context)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal MCP context: %w", err)
		}
		config.MCPContextJSON = string(contextJSON)
	}

	// Marshal budgets to JSON
	budgetsJSON, err := json.Marshal(m.Budgets)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP budgets: %w", err)
	}
	config.MCPBudgetsJSON = string(budgetsJSON)

	// Marshal prompts to JSON
	if len(m.Prompts) > 0 {
		promptsJSON, err := json.Marshal(m.Prompts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal MCP prompts: %w", err)
		}
		config.MCPPromptsJSON = string(promptsJSON)
	}

	// Set timeout
	config.MCPTimeout = m.Budgets.Timeout
	if config.MCPTimeout == "" {
		config.MCPTimeout = "30m"
	}

	return config, nil
}

// GetDefaultMCPConfig returns default MCP configuration
func GetDefaultMCPConfig() *MCPConfig {
	return &MCPConfig{
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
			"src/**",
			"pom.xml",
			"package.json",
		},
		Budgets: MCPBudgets{
			MaxTokens: 0, // unlimited
			MaxCost:   0, // unlimited
			Timeout:   "30m",
		},
	}
}

// MCPContextPrefetcher handles context prefetching through MCP tools
type MCPContextPrefetcher struct {
	config       *MCPConfig
	workspaceDir string
	contextDir   string
}

// NewMCPContextPrefetcher creates a new context prefetcher
func NewMCPContextPrefetcher(config *MCPConfig, workspaceDir string) *MCPContextPrefetcher {
	return &MCPContextPrefetcher{
		config:       config,
		workspaceDir: workspaceDir,
		contextDir:   filepath.Join(workspaceDir, "context"),
	}
}

// PrefetchContext prefetches context files and URLs through MCP tools
func (p *MCPContextPrefetcher) PrefetchContext() error {
	if p.config == nil || len(p.config.Context) == 0 {
		return nil // nothing to prefetch
	}

	// Ensure context directory exists
	if err := os.MkdirAll(p.contextDir, 0755); err != nil {
		return fmt.Errorf("failed to create context directory: %w", err)
	}

	// Process each context item
	for i, contextItem := range p.config.Context {
		if err := p.processContextItem(contextItem, i); err != nil {
			return fmt.Errorf("failed to process context item '%s': %w", contextItem, err)
		}
	}

	return nil
}

// processContextItem processes a single context item (file pattern or URL)
func (p *MCPContextPrefetcher) processContextItem(item string, index int) error {
	// Check if it's a URL
	if strings.HasPrefix(item, "http://") || strings.HasPrefix(item, "https://") {
		return p.prefetchURL(item, index)
	}

	// Otherwise, treat as file pattern
	return p.prefetchFilePattern(item, index)
}

// prefetchURL fetches content from HTTP/HTTPS URLs
func (p *MCPContextPrefetcher) prefetchURL(url string, index int) error {
	// For now, use basic HTTP client as MCP web tools would require
	// actual MCP server implementation. This is a placeholder for
	// future MCP web tool integration.

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP error %d for URL %s", resp.StatusCode, url)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Save to context directory
	filename := fmt.Sprintf("url_%d_%s", index, filepath.Base(url))
	if filepath.Ext(filename) == "" {
		filename += ".html"
	}

	outputPath := filepath.Join(p.contextDir, filename)
	if err := os.WriteFile(outputPath, body, 0644); err != nil {
		return fmt.Errorf("failed to write URL content: %w", err)
	}

	return nil
}

// prefetchFilePattern processes file patterns through MCP file-system tools
func (p *MCPContextPrefetcher) prefetchFilePattern(pattern string, index int) error {
	// For file patterns, we'll create a manifest file that describes
	// what files should be available. The actual MCP tool execution
	// happens in the containerized environment.

	manifest := map[string]interface{}{
		"type":    "file_pattern",
		"pattern": pattern,
		"index":   index,
		"tools":   p.getFileSystemTools(),
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal file pattern manifest: %w", err)
	}

	// Save manifest to context directory
	manifestPath := filepath.Join(p.contextDir, fmt.Sprintf("pattern_%d_manifest.json", index))
	if err := os.WriteFile(manifestPath, manifestJSON, 0644); err != nil {
		return fmt.Errorf("failed to write pattern manifest: %w", err)
	}

	return nil
}

// getFileSystemTools returns MCP tools that can handle file system operations
func (p *MCPContextPrefetcher) getFileSystemTools() []MCPTool {
	var fsTools []MCPTool
	for _, tool := range p.config.Tools {
		if tool.Name == "file-system" || tool.Name == "search" {
			fsTools = append(fsTools, tool)
		}
	}
	return fsTools
}

// CreateContextManifest creates a comprehensive context manifest for the containerized job
func (p *MCPContextPrefetcher) CreateContextManifest() error {
	// Ensure context directory exists
	if err := os.MkdirAll(p.contextDir, 0755); err != nil {
		return fmt.Errorf("failed to create context directory: %w", err)
	}

	manifest := map[string]interface{}{
		"mcp_config":    p.config,
		"context_items": p.config.Context,
		"tools":         p.config.Tools,
		"workspace_dir": p.workspaceDir,
		"context_dir":   p.contextDir,
		"prefetch_time": time.Now().UTC(),
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal context manifest: %w", err)
	}

	manifestPath := filepath.Join(p.contextDir, "mcp_context_manifest.json")
	if err := os.WriteFile(manifestPath, manifestJSON, 0644); err != nil {
		return fmt.Errorf("failed to write context manifest: %w", err)
	}

	return nil
}
