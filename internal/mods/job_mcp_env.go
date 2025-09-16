package mods

// getMCPEnvironmentConfig generates MCP environment configuration from MCP config
func getMCPEnvironmentConfig(mcpConfig *MCPConfig) *MCPEnvironmentConfig {
	// If no MCP config provided, return empty environment
	if mcpConfig == nil {
		return &MCPEnvironmentConfig{
			MCPToolsJSON:     "[]",
			MCPContextJSON:   "[]",
			MCPEndpointsJSON: "{}",
			MCPBudgetsJSON:   `{"max_tokens":0,"max_cost":0,"timeout":"30m"}`,
			MCPPromptsJSON:   "[]",
			MCPTimeout:       "30m",
			MCPSecurityMode:  "allowlist",
		}
	}
	// Convert MCP config to environment config
	envConfig, err := mcpConfig.ToEnvironmentConfig()
	if err != nil {
		// If conversion fails, return safe defaults
		return &MCPEnvironmentConfig{
			MCPToolsJSON:     "[]",
			MCPContextJSON:   "[]",
			MCPEndpointsJSON: "{}",
			MCPBudgetsJSON:   `{"max_tokens":0,"max_cost":0,"timeout":"30m"}`,
			MCPPromptsJSON:   "[]",
			MCPTimeout:       "30m",
			MCPSecurityMode:  "allowlist",
		}
	}
	return envConfig
}
