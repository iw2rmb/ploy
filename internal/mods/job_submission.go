package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Implementation of job submission helpers for the mods healing workflow.
// This provides the GREEN phase implementation for the failing tests.

// ProductionJobSubmitter defines the interface for production job submission
type ProductionJobSubmitter interface {
	RenderPlannerAssets() (*PlannerAssets, error)
	RenderReducerAssets() (*ReducerAssets, error)
	GetHCLSubmitter() HCLSubmitter
}

// jobSubmissionHelper implements the JobSubmissionHelper interface
type jobSubmissionHelper struct {
	submitter JobSubmitter           // Concrete job submitter (mock in tests, real in prod)
	runner    ProductionJobSubmitter // For accessing asset rendering methods in production
}

// NewJobSubmissionHelper creates a new job submission helper
func NewJobSubmissionHelper(submitter JobSubmitter) JobSubmissionHelper {
	return &jobSubmissionHelper{submitter: submitter}
}

// NewJobSubmissionHelperWithRunner creates a new job submission helper with runner access for production
func NewJobSubmissionHelperWithRunner(submitter JobSubmitter, runner ProductionJobSubmitter) JobSubmissionHelper {
	return &jobSubmissionHelper{submitter: submitter, runner: runner}
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
	// Derive from typical workspace layout when present in env
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
		"${SBOM_LATEST_URL}", hclEscape(get("SBOM_LATEST_URL")),
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
func (h *jobSubmissionHelper) SubmitPlannerJob(ctx context.Context, config *ModConfig, buildError string, workspace string) (*PlanResult, error) {
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

		// Step 3: Determine model from mods.yaml (if provided), provision in registry, then substitute env placeholders
		contextDir := filepath.Dir(assets.InputsPath)
		outDir := filepath.Join(workspace, "planner", "out")
		imgs := ResolveImagesFromEnv()
		infra := ResolveInfraFromEnv()
		modID := os.Getenv("MOD_ID")
		if modID == "" {
			return nil, fmt.Errorf("MOD_ID is required for planner job submission")
		}
		if !strings.HasPrefix(modID, "mod-") {
			modID = "mod-" + modID
		}
		llm := ResolveLLMDefaultsFromEnv()
		if config != nil {
			if pref := config.PreferredModel(); pref != "" {
				llm.Model = pref
			}
		}
		vars := map[string]string{
			"MODS_CONTEXT_DIR":     contextDir,
			"MODS_OUT_DIR":         outDir,
			"MODS_REGISTRY":        imgs.Registry,
			"MODS_PLANNER_IMAGE":   imgs.Planner,
			"MODS_REDUCER_IMAGE":   imgs.Reducer,
			"MODS_LLM_EXEC_IMAGE":  imgs.LLMExec,
			"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
			"MODS_MODEL":           llm.Model,
			"MODS_TOOLS":           llm.ToolsJSON,
			"MODS_LIMITS":          llm.LimitsJSON,
			"PLOY_CONTROLLER":      infra.Controller,
			"MOD_ID":               modID,
			"PLOY_SEAWEEDFS_URL":   infra.SeaweedURL,
			"NOMAD_DC":             infra.DC,
		}

		// Upload planner context as a tar to SeaweedFS and provide URL for artifact fetch
		if infra.SeaweedURL != "" {
			// Ensure non-empty by adding .keep
			_ = os.WriteFile(filepath.Join(contextDir, ".keep"), []byte("planner-context"), 0644)
			tarPath := filepath.Join(workspace, "planner", "context.tar")
			if err := createTarFromDir(contextDir, tarPath); err == nil {
				if modID != "" {
					key := fmt.Sprintf("mods/%s/contexts/%s.tar", modID, runID)
					_ = putFileFn(infra.SeaweedURL, key, tarPath, "application/octet-stream")
					vars["MODS_CONTEXT_URL"] = strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
				}
			}
		}
		// Inject SBOM_LATEST_URL for job reuse of last SBOM
		if infra.Controller != "" && config != nil && config.TargetRepo != "" {
			vars["SBOM_LATEST_URL"] = fmt.Sprintf("%s/sbom/latest?repo=%s", strings.TrimRight(infra.Controller, "/"), url.QueryEscape(config.TargetRepo))
		}
		renderedHCLPath, err := substituteHCLTemplateWithMCPVars(assets.HCLPath, runID, vars, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute HCL template: %w", err)
		}

		// Persist submitted planner HCL for diagnostics
		if modID != "" {
			persistDir := filepath.Join("/tmp/mods-submitted", modID, "planner")
			_ = os.MkdirAll(persistDir, 0755)
			dest := filepath.Join(persistDir, "planner.submitted.hcl")
			if b, e := os.ReadFile(renderedHCLPath); e == nil {
				_ = os.WriteFile(dest, b, 0644)
				if controller := ResolveInfraFromEnv().Controller; controller != "" {
					rep := NewControllerEventReporter(controller, modID)
					_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: fmt.Sprintf("Saved submitted HCL to %s", dest), JobName: runID, Time: time.Now()})
				}
			}
		}

		// Step 4: Push start event and report job metadata
		if controller := ResolveInfraFromEnv().Controller; controller != "" {
			rep := NewControllerEventReporter(controller, modID)
			_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: "job started", JobName: runID, Time: time.Now()})
			reportJobSubmittedAsync(ctx, rep, runID, "planner", "planner")
		}

		// Step 5: Preflight validate HCL, then submit job to Nomad and wait for completion
		if err := h.runner.GetHCLSubmitter().Validate(renderedHCLPath); err != nil {
			return nil, fmt.Errorf("planner HCL validation failed: %w", err)
		}
		timeout := ResolveDefaultsFromEnv().PlannerTimeout
		if err := h.runner.GetHCLSubmitter().SubmitCtx(ctx, renderedHCLPath, timeout); err != nil {
			if controller := ResolveInfraFromEnv().Controller; controller != "" {
				rep := NewControllerEventReporter(controller, modID)
				_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "error", Message: fmt.Sprintf("job failed: %v", err), JobName: runID, Time: time.Now()})
			}
			return nil, fmt.Errorf("planner job failed: %w", err)
		}

		// Step 6: Always fetch planner plan.json from SeaweedFS
		artifactPath := filepath.Join(workspace, "planner", "out", "plan.json")
		if infra.SeaweedURL == "" || modID == "" {
			return nil, fmt.Errorf("planner artifact fetch requires SeaweedFS URL and execution ID")
		}
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err != nil {
			return nil, fmt.Errorf("planner artifact path prep: %w", err)
		}
		key := fmt.Sprintf("mods/%s/planner/%s/plan.json", modID, runID)
		url := strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
		// Emit download attempt event for diagnostics
		if controller := ResolveInfraFromEnv().Controller; controller != "" {
			rep := NewControllerEventReporter(controller, modID)
			_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: fmt.Sprintf("download plan from %s", key), JobName: runID, Time: time.Now()})
		}
		if err := downloadToFileFn(url, artifactPath); err != nil {
			return nil, fmt.Errorf("failed to download planner output from SeaweedFS: %w", err)
		}
		if b, err := os.ReadFile(artifactPath); err == nil {
			if err := validatePlanJSON(b); err != nil {
				return nil, fmt.Errorf("planner output schema invalid: %w", err)
			}
		}
		var planResult PlanResult
		if err := readJobArtifact(artifactPath, &planResult); err != nil {
			return nil, fmt.Errorf("failed to read planner output: %w", err)
		}

		if controller := ResolveInfraFromEnv().Controller; controller != "" {
			rep := NewControllerEventReporter(controller, modID)
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
		imgs := ResolveImagesFromEnv()
		infra := ResolveInfraFromEnv()
		llm := ResolveLLMDefaultsFromEnv()
		vars := map[string]string{
			"MODS_CONTEXT_DIR":     contextDir,
			"MODS_OUT_DIR":         outDir,
			"MODS_REGISTRY":        imgs.Registry,
			"MODS_PLANNER_IMAGE":   imgs.Planner,
			"MODS_REDUCER_IMAGE":   imgs.Reducer,
			"MODS_LLM_EXEC_IMAGE":  imgs.LLMExec,
			"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
			"MODS_MODEL":           llm.Model,
			"MODS_TOOLS":           llm.ToolsJSON,
			"MODS_LIMITS":          llm.LimitsJSON,
			"PLOY_CONTROLLER":      infra.Controller,
			"MOD_ID":               os.Getenv("MOD_ID"),
			"NOMAD_DC":             infra.DC,
		}

		// Upload reducer history/context as tar to SeaweedFS and provide URL
		if infra.SeaweedURL != "" {
			_ = os.WriteFile(filepath.Join(contextDir, ".keep"), []byte("reducer-context"), 0644)
			tarPath := filepath.Join(workspace, "reducer", "context.tar")
			if err := createTarFromDir(contextDir, tarPath); err == nil {
				modID := os.Getenv("MOD_ID")
				if modID != "" {
					key := fmt.Sprintf("mods/%s/contexts/%s.tar", modID, runID)
					_ = putFileFn(infra.SeaweedURL, key, tarPath, "application/octet-stream")
					vars["MODS_CONTEXT_URL"] = strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
				}
			}
		}
		// vars already carries PLOY_CONTROLLER; nothing else to do here.
		renderedHCLPath, err := substituteHCLTemplateWithMCPVars(assets.HCLPath, runID, vars, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute HCL template: %w", err)
		}

		// Step 4: Push start event and report job metadata
		if controller := ResolveInfraFromEnv().Controller; controller != "" {
			rep := NewControllerEventReporter(controller, os.Getenv("MOD_ID"))
			_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "info", Message: "job started", JobName: runID, Time: time.Now()})
			reportJobSubmittedAsync(ctx, rep, runID, "reducer", "reducer")
		}

		// Step 5: Preflight validate HCL, then submit job to Nomad and wait for completion
		if err := h.runner.GetHCLSubmitter().Validate(renderedHCLPath); err != nil {
			return nil, fmt.Errorf("reducer HCL validation failed: %w", err)
		}
		timeout := ResolveDefaultsFromEnv().ReducerTimeout
		if err := h.runner.GetHCLSubmitter().SubmitCtx(ctx, renderedHCLPath, timeout); err != nil {
			if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
				rep := NewControllerEventReporter(controller, os.Getenv("MOD_ID"))
				_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "error", Message: fmt.Sprintf("job failed: %v", err), JobName: runID, Time: time.Now()})
			}
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}

		// Step 6: Read and validate job output artifact (next.json)
		artifactPath := filepath.Join(workspace, "reducer", "out", "next.json")
		if b, err := os.ReadFile(artifactPath); err == nil {
			if err := validateNextJSON(b); err != nil {
				return nil, fmt.Errorf("reducer output schema invalid: %w", err)
			}
		}
		var nextAction NextAction
		if err := readJobArtifact(artifactPath, &nextAction); err != nil {
			return nil, fmt.Errorf("failed to read reducer output: %w", err)
		}

		if controller := os.Getenv("PLOY_CONTROLLER"); controller != "" {
			rep := NewControllerEventReporter(controller, os.Getenv("MOD_ID"))
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
