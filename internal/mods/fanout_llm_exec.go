package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/orchestration"
)

// executeLLMExecBranch executes an LLM-based code generation branch
func (o *fanoutOrchestrator) executeLLMExecBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Step 1: Render LLM exec assets
	hclPath, err := o.runner.RenderLLMExecAssets(branch.ID)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to render LLM exec assets: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	baseDir := filepath.Dir(hclPath)
	_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
	infra := ResolveInfraFromEnv()
	modID := os.Getenv("MOD_ID")
	vars := llmMakeVars(baseDir)

	// Step 2: Generate unique run ID for this branch
	runID := LLMRunID(branch.ID)

	// Prepare and upload a context tar for artifact download (mirror planner/reducer behavior)
	if infra.SeaweedURL != "" {
		repoRoot := filepath.Join(o.runner.GetWorkspaceDir(), "repo")
		if ctxDir, err := llmPrepareContext(baseDir, branch, repoRoot, o.runner.GetEventReporter(), ctx); err == nil {
			tarPath := filepath.Join(baseDir, "llm-context.tar")
			if err := createTarFromDir(ctxDir, tarPath); err == nil {
				if modID != "" {
					key := fmt.Sprintf("mods/%s/contexts/%s.tar", modID, runID)
					if uploader := o.runner.GetArtifactUploader(); uploader != nil {
						_ = uploader.UploadFile(ctx, infra.SeaweedURL, key, tarPath, "application/octet-stream")
					}
					vars["MODS_CONTEXT_URL"] = strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
				}
			}
		}
	}

	// Step 3: Extract MCP configuration from branch inputs
	var mcpConfig *MCPConfig = nil
	if mcpData, ok := branch.Inputs["mcp_config"]; ok {
		if mcpConfigMap, ok := mcpData.(map[string]interface{}); ok {
			// Convert map to MCPConfig struct
			if parsedMCP, err := parseMCPFromInputs(mcpConfigMap); err == nil {
				mcpConfig = parsedMCP
			}
		}
	}

	// Step 4: Substitute environment variables in HCL template with MCP support
	// If branch MCP config specifies a model, prefer it
	if mcpConfig != nil && strings.TrimSpace(mcpConfig.Model) != "" {
		vars["MODS_MODEL"] = strings.TrimSpace(mcpConfig.Model)
	}
	renderedHCLPath, err := substituteHCLTemplateWithMCPVars(hclPath, runID, vars, mcpConfig)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to substitute HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Persist submitted HCL and emit controller event
	llmPersistSubmittedHCL(renderedHCLPath, branch.ID, runID, o.runner.GetEventReporter(), ctx)

	// Step 4: Report job metadata asynchronously (job name == runID)
	var rep EventReporter
	if o.runner != nil {
		rep = o.runner.GetEventReporter()
	}
	reportJobSubmittedAsync(ctx, rep, runID, string(StepTypeLLMExec), string(StepTypeLLMExec))

	// Step 5: Preflight validate HCL, then submit job to Nomad and wait for completion
	if err := llmValidateAndSubmit(ctx, o.hcl, renderedHCLPath, runID, o.runner.GetEventReporter()); err != nil {
		result.Status = "failed"
		result.Notes = err.Error()
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 6: Fetch diff.patch from SeaweedFS in prod; in tests (no HCL submitter), rely on local artifact existence
	if o.hcl != nil {
		if err := llmFetchDiffIfProd(ctx, o.runner.GetEventReporter(), infra.SeaweedURL, modID, branch.ID, runID, renderedHCLPath); err != nil {
			result.Status = "failed"
			result.Notes = err.Error()
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			return result
		}
	}

	// Step 6b: Upload LLM diff to SeaweedFS with step-scoped key (align with ORW convention)
	// mods/<modID>/branches/<branchID>/steps/<stepID>/diff.patch (reuse computed IDs)
	if o.hcl != nil {
		llmUploadDiffAndMeta(ctx, o.runner.GetArtifactUploader(), infra.SeaweedURL, modID, branch.ID, runID, renderedHCLPath)
	}

	// Cleanup job registration after successful artifact retrieval
	_ = orchestration.DeregisterJob(runID, true)
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	result.Status = "completed"
	result.JobID = runID
	result.DiffPath = diffPath
	if modID != "" {
		result.DiffKey = computeBranchDiffKey(modID, branch.ID, runID)
	}
	result.Notes = fmt.Sprintf("LLM exec job completed successfully, diff.patch at: %s", diffPath)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}
