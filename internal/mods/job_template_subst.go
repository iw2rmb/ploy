package mods

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// substituteHCLTemplate performs environment variable substitution in HCL templates
func substituteHCLTemplate(hclPath string, runID string) (string, error) {
	return substituteHCLTemplateWithMCP(hclPath, runID, nil)
}

// substituteHCLTemplateWithMCP performs environment variable substitution with MCP support
// substituteHCLTemplateWithMCP reads process env to assemble substitutions.
// Prefer substituteHCLTemplateWithMCPVars to avoid global env reliance.
func substituteHCLTemplateWithMCP(hclPath string, runID string, mcpConfig *MCPConfig) (string, error) {
	return substituteHCLTemplateWithMCPVars(hclPath, runID, nil, mcpConfig)
}

// substituteHCLTemplateWithMCPVars performs HCL substitution using provided vars (no global env mutation).
// If vars is nil, falls back to reading from environment for backward compatibility.
func substituteHCLTemplateWithMCPVars(hclPath string, runID string, vars map[string]string, mcpConfig *MCPConfig) (string, error) {
	hclBytes, err := os.ReadFile(hclPath)
	if err != nil {
		return "", fmt.Errorf("failed to read HCL template: %w", err)
	}

	get := func(k string) string {
		if vars != nil {
			if v, ok := vars[k]; ok {
				return v
			}
		}
		return os.Getenv(k)
	}

	// Get core variables with defaults
	model := get("MODS_MODEL")
	if model == "" {
		model = "gpt-4o-mini@2024-08-06"
	}

	toolsJSON := get("MODS_TOOLS")
	if toolsJSON == "" {
		toolsJSON = `{"file":{"allow":["src/**","pom.xml"]},"search":{"provider":"rg","allow":["src/**"]}}`
	}

	limitsJSON := get("MODS_LIMITS")
	if limitsJSON == "" {
		limitsJSON = `{"max_steps":8,"max_tool_calls":12,"timeout":"30m"}`
	}

	// Get MCP environment variables
	mcpEnvConfig := getMCPEnvironmentConfig(mcpConfig)

	// Escape values for safe inclusion inside quoted HCL strings
	hclEscape := func(s string) string {
		s = strings.ReplaceAll(s, "\\", "\\\\")
		s = strings.ReplaceAll(s, "\"", "\\\"")
		return s
	}

	// Compute optional host directories for bind mounts
	contextHostDir := get("MODS_CONTEXT_DIR")
	outHostDir := get("MODS_OUT_DIR")

	// Defaults for images (can be overridden via environment)
	d := ResolveDefaults(get)
	plannerImage := get("MODS_PLANNER_IMAGE")
	if plannerImage == "" {
		plannerImage = d.PlannerImage
	}
	reducerImage := get("MODS_REDUCER_IMAGE")
	if reducerImage == "" {
		reducerImage = d.ReducerImage
	}
	llmExecImage := get("MODS_LLM_EXEC_IMAGE")
	if llmExecImage == "" {
		llmExecImage = d.LLMExecImage
	}
	orwApplyImage := get("MODS_ORW_APPLY_IMAGE")
	if orwApplyImage == "" {
		orwApplyImage = d.ORWApplyImage
	}

	// Perform substitution
	controllerURL := get("PLOY_CONTROLLER")
	modID := get("MOD_ID")
	if modID != "" && !strings.HasPrefix(modID, "mod-") {
		modID = "mod-" + modID
	}
	dc := get("NOMAD_DC")
	if dc == "" {
		dc = d.DC
	}
	// Ensure SBOM_LATEST_URL has a non-empty value to satisfy Nomad template validation
	sbomURL := get("SBOM_LATEST_URL")
	if strings.TrimSpace(sbomURL) == "" {
		sbomURL = "#"
	}

	replacer := strings.NewReplacer(
		"${MODEL}", hclEscape(model),
		"${TOOLS_JSON}", hclEscape(toolsJSON),
		"${LIMITS_JSON}", hclEscape(limitsJSON),
		"${RUN_ID}", runID,
		"${CONTEXT_HOST_DIR}", hclEscape(contextHostDir),
		"${OUT_HOST_DIR}", hclEscape(outHostDir),
		"${PLANNER_IMAGE}", hclEscape(plannerImage),
		"${REDUCER_IMAGE}", hclEscape(reducerImage),
		"${LLM_EXEC_IMAGE}", hclEscape(llmExecImage),
		"${ORW_APPLY_IMAGE}", hclEscape(orwApplyImage),
		"${MODS_CONTEXT_URL}", hclEscape(get("MODS_CONTEXT_URL")),
		"${MCP_TOOLS_JSON}", hclEscape(mcpEnvConfig.MCPToolsJSON),
		"${MCP_CONTEXT_JSON}", hclEscape(mcpEnvConfig.MCPContextJSON),
		"${MCP_ENDPOINTS_JSON}", hclEscape(mcpEnvConfig.MCPEndpointsJSON),
		"${MCP_BUDGETS_JSON}", hclEscape(mcpEnvConfig.MCPBudgetsJSON),
		"${MCP_PROMPTS_JSON}", hclEscape(mcpEnvConfig.MCPPromptsJSON),
		"${MCP_TIMEOUT}", hclEscape(mcpEnvConfig.MCPTimeout),
		"${MCP_SECURITY_MODE}", hclEscape(mcpEnvConfig.MCPSecurityMode),
		"${CONTROLLER_URL}", hclEscape(controllerURL),
		"${MOD_ID}", hclEscape(modID),
		"${NOMAD_DC}", hclEscape(dc),
		"${SBOM_LATEST_URL}", hclEscape(sbomURL),
		"${PLOY_SEAWEEDFS_URL}", hclEscape(get("PLOY_SEAWEEDFS_URL")),
	)
	rendered := replacer.Replace(string(hclBytes))

	// Write substituted HCL to a new file
	renderedPath := strings.ReplaceAll(hclPath, ".hcl", ".rendered.submitted.hcl")
	if err := os.WriteFile(renderedPath, []byte(rendered), 0644); err != nil {
		return "", fmt.Errorf("failed to write substituted HCL: %w", err)
	}
	return renderedPath, nil
}

// readJobArtifact reads and parses a JSON artifact from a job execution
func readJobArtifact(artifactPath string, target interface{}) error {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return fmt.Errorf("failed to read artifact %s: %w", artifactPath, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to parse artifact JSON: %w", err)
	}
	return nil
}
