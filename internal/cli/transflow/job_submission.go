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
	submitter JobSubmitter           // Concrete job submitter (mock in tests, real in prod)
	runner    ProductionJobSubmitter // For accessing asset rendering methods in production
}

// NewJobSubmissionHelper creates a new job submission helper
func NewJobSubmissionHelper(submitter interface{}) JobSubmissionHelper {
	var js JobSubmitter
	switch s := submitter.(type) {
	case nil:
		js = nil
	case JobSubmitter:
		js = s
	default:
		js = NoopJobSubmitter{}
	}
	return &jobSubmissionHelper{submitter: js}
}

// NewJobSubmissionHelperWithRunner creates a new job submission helper with runner access for production
func NewJobSubmissionHelperWithRunner(submitter interface{}, runner ProductionJobSubmitter) JobSubmissionHelper {
	var js JobSubmitter
	switch s := submitter.(type) {
	case nil:
		js = nil
	case JobSubmitter:
		js = s
	default:
		js = NoopJobSubmitter{}
	}
	return &jobSubmissionHelper{submitter: js, runner: runner}
}

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
	model := get("TRANSFLOW_MODEL")
	if model == "" {
		model = "gpt-4o-mini@2024-08-06"
	}

	toolsJSON := get("TRANSFLOW_TOOLS")
	if toolsJSON == "" {
		toolsJSON = `{"file":{"allow":["src/**","pom.xml"]},"search":{"provider":"rg","allow":["src/**"]}}`
	}

	limitsJSON := get("TRANSFLOW_LIMITS")
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
	contextHostDir := get("TRANSFLOW_CONTEXT_DIR")
	outHostDir := get("TRANSFLOW_OUT_DIR")

	// Defaults for images (can be overridden via environment)
	d := ResolveDefaults(get)
	plannerImage := get("TRANSFLOW_PLANNER_IMAGE")
	if plannerImage == "" {
		plannerImage = d.PlannerImage
	}
	reducerImage := get("TRANSFLOW_REDUCER_IMAGE")
	if reducerImage == "" {
		reducerImage = d.ReducerImage
	}
	llmExecImage := get("TRANSFLOW_LLM_EXEC_IMAGE")
	if llmExecImage == "" {
		llmExecImage = d.LLMExecImage
	}
	orwApplyImage := get("TRANSFLOW_ORW_APPLY_IMAGE")
	if orwApplyImage == "" {
		orwApplyImage = d.ORWApplyImage
	}

	// Perform substitution
	controllerURL := get("PLOY_CONTROLLER")
	execID := get("PLOY_TRANSFLOW_EXECUTION_ID")

	dc := get("NOMAD_DC")
	if dc == "" {
		dc = d.DC
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
		"${MCP_TOOLS_JSON}", hclEscape(mcpEnvConfig.MCPToolsJSON),
		"${MCP_CONTEXT_JSON}", hclEscape(mcpEnvConfig.MCPContextJSON),
		"${MCP_ENDPOINTS_JSON}", hclEscape(mcpEnvConfig.MCPEndpointsJSON),
		"${MCP_BUDGETS_JSON}", hclEscape(mcpEnvConfig.MCPBudgetsJSON),
		"${MCP_PROMPTS_JSON}", hclEscape(mcpEnvConfig.MCPPromptsJSON),
		"${MCP_TIMEOUT}", hclEscape(mcpEnvConfig.MCPTimeout),
		"${MCP_SECURITY_MODE}", hclEscape(mcpEnvConfig.MCPSecurityMode),
		"${CONTROLLER_URL}", hclEscape(controllerURL),
		"${EXECUTION_ID}", hclEscape(execID),
		"${NOMAD_DC}", hclEscape(dc),
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
	// Prefer production runner path when available

	// Production implementation using real Nomad job submission
	if h.runner != nil {
		// Step 1: Render planner assets
		assets, err := h.runner.RenderPlannerAssets()
		if err != nil {
			return nil, fmt.Errorf("failed to render planner assets: %w", err)
		}

    // Step 2: Generate unique run ID for this planner job
    runID := PlannerRunID(config.ID)

		// Step 3: Substitute environment variables in HCL template without global env writes
		contextDir := filepath.Dir(assets.InputsPath)
		outDir := filepath.Join(workspace, "planner", "out")
		vars := map[string]string{
			"TRANSFLOW_CONTEXT_DIR":       contextDir,
			"TRANSFLOW_OUT_DIR":           outDir,
			"TRANSFLOW_REGISTRY":          os.Getenv("TRANSFLOW_REGISTRY"),
			"TRANSFLOW_PLANNER_IMAGE":     os.Getenv("TRANSFLOW_PLANNER_IMAGE"),
			"TRANSFLOW_REDUCER_IMAGE":     os.Getenv("TRANSFLOW_REDUCER_IMAGE"),
			"TRANSFLOW_LLM_EXEC_IMAGE":    os.Getenv("TRANSFLOW_LLM_EXEC_IMAGE"),
			"TRANSFLOW_ORW_APPLY_IMAGE":   os.Getenv("TRANSFLOW_ORW_APPLY_IMAGE"),
			"TRANSFLOW_MODEL":             os.Getenv("TRANSFLOW_MODEL"),
			"TRANSFLOW_TOOLS":             os.Getenv("TRANSFLOW_TOOLS"),
			"TRANSFLOW_LIMITS":            os.Getenv("TRANSFLOW_LIMITS"),
			"PLOY_CONTROLLER":             os.Getenv("PLOY_CONTROLLER"),
			"PLOY_TRANSFLOW_EXECUTION_ID": os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"),
			"NOMAD_DC":                    os.Getenv("NOMAD_DC"),
		}
		renderedHCLPath, err := substituteHCLTemplateWithMCPVars(assets.HCLPath, runID, vars, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute HCL template: %w", err)
		}

		// Step 4: Push start event and report job metadata
		if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
			rep := NewControllerEventReporter(controller, os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"))
			// Start event
			_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: "job started", JobName: runID, Time: time.Now()})
			// Report alloc id asynchronously
			go func(job string) {
				select {
				case <-time.After(1 * time.Second):
				case <-ctx.Done():
					return
				}
				if id := findFirstAllocID(job); id != "" {
					_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: "job submitted", JobName: job, AllocID: id, Time: time.Now()})
				}
			}(runID)
		}

		// Step 5: Preflight validate HCL, then submit job to Nomad and wait for completion
		if err := orchestration.ValidateJob(renderedHCLPath); err != nil {
			return nil, fmt.Errorf("planner HCL validation failed: %w", err)
		}
		timeout := ResolveDefaultsFromEnv().PlannerTimeout
		if err := orchestration.SubmitAndWaitTerminalCtx(ctx, renderedHCLPath, timeout); err != nil {
			if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
				rep := NewControllerEventReporter(controller, os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"))
				_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "error", Message: fmt.Sprintf("job failed: %v", err), JobName: runID, Time: time.Now()})
			}
			return nil, fmt.Errorf("planner job failed: %w", err)
		}

		// Step 6: Read and parse job output artifact
		// The planner job should write plan.json to the output directory
		artifactPath := filepath.Join(workspace, "planner", "out", "plan.json")
		var planResult PlanResult
		if err := readJobArtifact(artifactPath, &planResult); err != nil {
			return nil, fmt.Errorf("failed to read planner output: %w", err)
		}

		if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
			rep := NewControllerEventReporter(controller, os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"))
			_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: "job completed", JobName: runID, Time: time.Now()})
		}

		return &planResult, nil
	}

	// Fallback to test submitter path if runner is not provided
	if h.submitter != nil {
		spec := JobSpec{
			Name:    "planner",
			Type:    "planner",
			HCLPath: "", // Not used by mock submitter
			EnvVars: map[string]string{
				"BUILD_ERROR": buildError,
				"TARGET_REPO": config.TargetRepo,
				"BASE_REF":    config.BaseRef,
			},
			Timeout: ResolveDefaultsFromEnv().PlannerTimeout,
			Inputs: map[string]interface{}{
				"workspace": workspace,
			},
		}
		result, err := h.submitter.SubmitAndWaitTerminal(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("planner job failed: %w", err)
		}
		var planResult PlanResult
		if err := json.Unmarshal([]byte(result.Output), &planResult); err != nil {
			return nil, fmt.Errorf("failed to parse planner output: %w", err)
		}
		return &planResult, nil
	}
	// No runner or submitter provided
	return nil, fmt.Errorf("no production runner or submitter available for job submission")
}

// SubmitReducerJob submits a reducer job to determine the next action
func (h *jobSubmissionHelper) SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error) {
	// Test submitter path via JobSubmitter
	if h.submitter != nil {
		spec := JobSpec{
			Name:    "reducer",
			Type:    "reducer",
			HCLPath: "",
			EnvVars: map[string]string{
				"PLAN_ID": planID,
			},
			Timeout: ResolveDefaultsFromEnv().ReducerTimeout,
			Inputs: map[string]interface{}{
				"workspace": workspace,
				"results":   results,
				"winner":    winner,
			},
		}
		result, err := h.submitter.SubmitAndWaitTerminal(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}
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
    runID := ReducerRunID(planID)

		// Step 3: Substitute environment variables in HCL template without global env writes
		contextDir := filepath.Dir(assets.HistoryPath)
		outDir := filepath.Join(workspace, "reducer", "out")
		vars := map[string]string{
			"TRANSFLOW_CONTEXT_DIR":       contextDir,
			"TRANSFLOW_OUT_DIR":           outDir,
			"TRANSFLOW_REGISTRY":          os.Getenv("TRANSFLOW_REGISTRY"),
			"TRANSFLOW_PLANNER_IMAGE":     os.Getenv("TRANSFLOW_PLANNER_IMAGE"),
			"TRANSFLOW_REDUCER_IMAGE":     os.Getenv("TRANSFLOW_REDUCER_IMAGE"),
			"TRANSFLOW_LLM_EXEC_IMAGE":    os.Getenv("TRANSFLOW_LLM_EXEC_IMAGE"),
			"TRANSFLOW_ORW_APPLY_IMAGE":   os.Getenv("TRANSFLOW_ORW_APPLY_IMAGE"),
			"TRANSFLOW_MODEL":             os.Getenv("TRANSFLOW_MODEL"),
			"TRANSFLOW_TOOLS":             os.Getenv("TRANSFLOW_TOOLS"),
			"TRANSFLOW_LIMITS":            os.Getenv("TRANSFLOW_LIMITS"),
			"PLOY_CONTROLLER":             os.Getenv("PLOY_CONTROLLER"),
			"PLOY_TRANSFLOW_EXECUTION_ID": os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"),
			"NOMAD_DC":                    os.Getenv("NOMAD_DC"),
		}
		renderedHCLPath, err := substituteHCLTemplateWithMCPVars(assets.HCLPath, runID, vars, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute HCL template: %w", err)
		}

		// Step 4: Push start event and report job metadata
		if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
			rep := NewControllerEventReporter(controller, os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"))
			_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "info", Message: "job started", JobName: runID, Time: time.Now()})
			go func(job string) {
				select {
				case <-time.After(1 * time.Second):
				case <-ctx.Done():
					return
				}
				if id := findFirstAllocID(job); id != "" {
					_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "info", Message: "job submitted", JobName: job, AllocID: id, Time: time.Now()})
				}
			}(runID)
		}

		// Step 5: Preflight validate HCL, then submit job to Nomad and wait for completion
		if err := orchestration.ValidateJob(renderedHCLPath); err != nil {
			return nil, fmt.Errorf("reducer HCL validation failed: %w", err)
		}
		timeout := ResolveDefaultsFromEnv().ReducerTimeout
		if err := orchestration.SubmitAndWaitTerminalCtx(ctx, renderedHCLPath, timeout); err != nil {
			if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
				rep := NewControllerEventReporter(controller, os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"))
				_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "error", Message: fmt.Sprintf("job failed: %v", err), JobName: runID, Time: time.Now()})
			}
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}

		// Step 6: Read and parse job output artifact
		// The reducer job should write next.json to the output directory
		artifactPath := filepath.Join(workspace, "reducer", "out", "next.json")
		var nextAction NextAction
		if err := readJobArtifact(artifactPath, &nextAction); err != nil {
			return nil, fmt.Errorf("failed to read reducer output: %w", err)
		}

		if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
			rep := NewControllerEventReporter(controller, os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"))
			_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "info", Message: "job completed", JobName: runID, Time: time.Now()})
		}

		return &nextAction, nil
	}

	// Fallback to test submitter path if runner is not provided
	if h.submitter != nil {
		spec := JobSpec{
			Name:    "reducer",
			Type:    "reducer",
			HCLPath: "", // not used by mock submitter
			EnvVars: map[string]string{
				"PLAN_ID": planID,
			},
			Timeout: ResolveDefaultsFromEnv().ReducerTimeout,
			Inputs: map[string]interface{}{
				"workspace": workspace,
				"results":   results,
				"winner":    winner,
			},
		}
		result, err := h.submitter.SubmitAndWaitTerminal(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}
		var nextAction NextAction
		if err := json.Unmarshal([]byte(result.Output), &nextAction); err != nil {
			return nil, fmt.Errorf("failed to parse reducer output: %w", err)
		}
		return &nextAction, nil
	}
	// No runner or submitter provided
	return nil, fmt.Errorf("no production runner or submitter available for job submission")
}
