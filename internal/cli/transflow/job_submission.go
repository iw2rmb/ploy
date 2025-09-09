package transflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/orchestration"
)

// Implementation of job submission helpers for the transflow healing workflow.
// This provides the GREEN phase implementation for the failing tests.

// ProductionJobSubmitter defines the interface for production job submission
type ProductionJobSubmitter interface {
	RenderPlannerAssets() (*PlannerAssets, error)
	RenderReducerAssets() (*ReducerAssets, error)
}

// jobSubmissionHelper implements the JobSubmissionHelper interface
type jobSubmissionHelper struct {
	submitter interface{}            // JobSubmitter interface for production job submission
	runner    ProductionJobSubmitter // For accessing asset rendering methods in production
}

// NewJobSubmissionHelper creates a new job submission helper
func NewJobSubmissionHelper(submitter interface{}) JobSubmissionHelper {
	return &jobSubmissionHelper{
		submitter: submitter,
		runner:    nil, // Will be nil when runner is not needed
	}
}

// NewJobSubmissionHelperWithRunner creates a new job submission helper with runner access for production
func NewJobSubmissionHelperWithRunner(submitter interface{}, runner ProductionJobSubmitter) JobSubmissionHelper {
	return &jobSubmissionHelper{
		submitter: submitter,
		runner:    runner,
	}
}

// substituteHCLTemplate performs environment variable substitution in HCL templates
func substituteHCLTemplate(hclPath string, runID string) (string, error) {
	return substituteHCLTemplateWithMCP(hclPath, runID, nil)
}

// substituteHCLTemplateWithMCP performs environment variable substitution with MCP support
func substituteHCLTemplateWithMCP(hclPath string, runID string, mcpConfig *MCPConfig) (string, error) {
	hclBytes, err := os.ReadFile(hclPath)
	if err != nil {
		return "", fmt.Errorf("failed to read HCL template: %w", err)
	}

	// Get core environment variables with defaults
	model := os.Getenv("TRANSFLOW_MODEL")
	if model == "" {
		model = "gpt-4o-mini@2024-08-06"
	}

	toolsJSON := os.Getenv("TRANSFLOW_TOOLS")
	if toolsJSON == "" {
		toolsJSON = `{"file":{"allow":["src/**","pom.xml"]},"search":{"provider":"rg","allow":["src/**"]}}`
	}

	limitsJSON := os.Getenv("TRANSFLOW_LIMITS")
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
	// Derive from typical workspace layout when present in env
	contextHostDir := os.Getenv("TRANSFLOW_CONTEXT_DIR")
	outHostDir := os.Getenv("TRANSFLOW_OUT_DIR")

	// Defaults for images (can be overridden via environment)
	defaultRegistry := os.Getenv("TRANSFLOW_REGISTRY")
	if defaultRegistry == "" {
		defaultRegistry = "registry.dev.ployman.app"
	}
	plannerImage := os.Getenv("TRANSFLOW_PLANNER_IMAGE")
	if plannerImage == "" {
		plannerImage = defaultRegistry + "/langgraph-runner:py-0.1.0"
	}
	reducerImage := os.Getenv("TRANSFLOW_REDUCER_IMAGE")
	if reducerImage == "" {
		reducerImage = plannerImage
	}
	llmExecImage := os.Getenv("TRANSFLOW_LLM_EXEC_IMAGE")
	if llmExecImage == "" {
		llmExecImage = plannerImage
	}
	orwApplyImage := os.Getenv("TRANSFLOW_ORW_APPLY_IMAGE")
	if orwApplyImage == "" {
		orwApplyImage = defaultRegistry + "/openrewrite-jvm:latest"
	}

	// Perform substitution
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
		"${MCP_TOOLS_JSON}", hclEscape(mcpEnvConfig.MCPToolsJSON),
		"${MCP_CONTEXT_JSON}", hclEscape(mcpEnvConfig.MCPContextJSON),
		"${MCP_ENDPOINTS_JSON}", hclEscape(mcpEnvConfig.MCPEndpointsJSON),
		"${MCP_BUDGETS_JSON}", hclEscape(mcpEnvConfig.MCPBudgetsJSON),
		"${MCP_PROMPTS_JSON}", hclEscape(mcpEnvConfig.MCPPromptsJSON),
		"${MCP_TIMEOUT}", hclEscape(mcpEnvConfig.MCPTimeout),
		"${MCP_SECURITY_MODE}", hclEscape(mcpEnvConfig.MCPSecurityMode),
	)
	rendered := replacer.Replace(string(hclBytes))

	// Write substituted HCL to a new file
	renderedPath := strings.ReplaceAll(hclPath, ".hcl", ".rendered.submitted.hcl")
	if err := os.WriteFile(renderedPath, []byte(rendered), 0644); err != nil {
		return "", fmt.Errorf("failed to write substituted HCL: %w", err)
	}

	return renderedPath, nil
}

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

// SubmitPlannerJob submits a planner job after a build failure
func (h *jobSubmissionHelper) SubmitPlannerJob(ctx context.Context, config *TransflowConfig, buildError string, workspace string) (*PlanResult, error) {
	// Check if this is a test submitter (backward compatibility)
	if testSubmitter, ok := h.submitter.(interface {
		SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)
	}); ok {
		spec := JobSpec{
			Name:    "planner",
			Type:    "planner",
			HCLPath: "", // Would be set from rendered assets
			EnvVars: map[string]string{
				"BUILD_ERROR": buildError,
				"TARGET_REPO": config.TargetRepo,
				"BASE_REF":    config.BaseRef,
			},
			Timeout: 15 * time.Minute,
			Inputs: map[string]interface{}{
				"workspace": workspace,
			},
		}

		result, err := testSubmitter.SubmitAndWaitTerminal(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("planner job failed: %w", err)
		}

		// Parse the planner output
		var planResult PlanResult
		if err := json.Unmarshal([]byte(result.Output), &planResult); err != nil {
			return nil, fmt.Errorf("failed to parse planner output: %w", err)
		}

		return &planResult, nil
	}

	// Production implementation using real Nomad job submission
	if h.runner != nil {
		// Step 1: Render planner assets
		assets, err := h.runner.RenderPlannerAssets()
		if err != nil {
			return nil, fmt.Errorf("failed to render planner assets: %w", err)
		}

		// Step 2: Generate unique run ID for this planner job
		runID := fmt.Sprintf("%s-planner-%d", config.ID, time.Now().Unix())

		// Step 3: Substitute environment variables in HCL template
		// Provide host directories for bind mounts
		contextDir := filepath.Dir(assets.InputsPath)
		outDir := filepath.Join(workspace, "planner", "out")
		// Inform substitution via env to avoid signature churn
		os.Setenv("TRANSFLOW_CONTEXT_DIR", contextDir)
		os.Setenv("TRANSFLOW_OUT_DIR", outDir)
		renderedHCLPath, err := substituteHCLTemplate(assets.HCLPath, runID)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute HCL template: %w", err)
		}

		// Step 4: Submit job to Nomad and wait for completion
		timeout := 15 * time.Minute
		if err := orchestration.SubmitAndWaitTerminal(renderedHCLPath, timeout); err != nil {
			return nil, fmt.Errorf("planner job failed: %w", err)
		}

		// Step 5: Read and parse job output artifact
		// The planner job should write plan.json to the output directory
		artifactPath := filepath.Join(workspace, "planner", "out", "plan.json")
		var planResult PlanResult
		if err := readJobArtifact(artifactPath, &planResult); err != nil {
			return nil, fmt.Errorf("failed to read planner output: %w", err)
		}

		return &planResult, nil
	}

	// No runner provided and not a test submitter
	return nil, fmt.Errorf("no production runner or test submitter available for job submission")
}

// SubmitReducerJob submits a reducer job to determine the next action
func (h *jobSubmissionHelper) SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error) {
	// Check if this is a test submitter (backward compatibility)
	if testSubmitter, ok := h.submitter.(interface {
		SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)
	}); ok {
		spec := JobSpec{
			Name:    "reducer",
			Type:    "reducer",
			HCLPath: "", // Would be set from rendered assets
			EnvVars: map[string]string{
				"PLAN_ID": planID,
			},
			Timeout: 10 * time.Minute,
			Inputs: map[string]interface{}{
				"workspace": workspace,
				"results":   results,
				"winner":    winner,
			},
		}

		result, err := testSubmitter.SubmitAndWaitTerminal(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}

		// Parse the reducer output
		var nextAction NextAction
		if err := json.Unmarshal([]byte(result.Output), &nextAction); err != nil {
			return nil, fmt.Errorf("failed to parse reducer output: %w", err)
		}

		return &nextAction, nil
	}

	// Production implementation using real Nomad job submission
	if h.runner != nil {
		// Step 1: Render reducer assets
		assets, err := h.runner.RenderReducerAssets()
		if err != nil {
			return nil, fmt.Errorf("failed to render reducer assets: %w", err)
		}

		// Step 2: Generate unique run ID for this reducer job
		runID := fmt.Sprintf("%s-reducer-%d", planID, time.Now().Unix())

		// Step 3: Substitute environment variables in HCL template
		// Provide host directories for bind mounts
		contextDir := filepath.Dir(assets.HistoryPath)
		outDir := filepath.Join(workspace, "reducer", "out")
		os.Setenv("TRANSFLOW_CONTEXT_DIR", contextDir)
		os.Setenv("TRANSFLOW_OUT_DIR", outDir)
		renderedHCLPath, err := substituteHCLTemplate(assets.HCLPath, runID)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute HCL template: %w", err)
		}

		// Step 4: Submit job to Nomad and wait for completion
		timeout := 10 * time.Minute
		if err := orchestration.SubmitAndWaitTerminal(renderedHCLPath, timeout); err != nil {
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}

		// Step 5: Read and parse job output artifact
		// The reducer job should write next.json to the output directory
		artifactPath := filepath.Join(workspace, "reducer", "out", "next.json")
		var nextAction NextAction
		if err := readJobArtifact(artifactPath, &nextAction); err != nil {
			return nil, fmt.Errorf("failed to read reducer output: %w", err)
		}

		return &nextAction, nil
	}

	// No runner provided and not a test submitter
	return nil, fmt.Errorf("no production runner or test submitter available for job submission")
}
