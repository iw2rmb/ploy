package mods

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// executeORWGenBranch executes an OpenRewrite recipe generation and application branch
func (o *fanoutOrchestrator) executeORWGenBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Step 1: Render ORW apply assets
	hclPath, err := o.runner.RenderORWApplyAssets(branch.ID)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to render ORW apply assets: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 2: Pre-substitute recipe variables and prepare env
	rclass, rcoords, rtimeout, pluginVersion, cfgErr := buildORWRecipeConfig(branch.Inputs)
	if cfgErr != nil {
		result.Status = "failed"
		result.Notes = cfgErr.Error()
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		if rep := o.runner.GetEventReporter(); rep != nil {
			_ = rep.Report(ctx, Event{Phase: "fanout", Step: string(StepTypeORWApply), Level: "error", Message: cfgErr.Error(), Time: time.Now()})
		}
		return result
	}
	prePath, err := orwPreSubstitute(hclPath, rclass, rcoords, rtimeout)
	if err != nil {
		result.Status = "failed"
		result.Notes = err.Error()
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Provide host directories for bind mounts (no global env)
	baseDir := filepath.Dir(hclPath)
	modID := os.Getenv("MOD_ID")
	infra := ResolveInfraFromEnv()
	seaweedURL := infra.SeaweedURL
	runID := ORWRunID(branch.ID)
	diffKey := computeBranchDiffKey(modID, branch.ID, runID)

	// Prepare input tar from the repo and upload to SeaweedFS for task-side download (best effort)
	repoRoot := filepath.Join(o.runner.GetWorkspaceDir(), "repo")
	inputTar := filepath.Join(baseDir, "input.tar")
	if err := createTarFromDir(repoRoot, inputTar); err != nil {
		if rep := o.runner.GetEventReporter(); rep != nil {
			_ = rep.Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "error", Message: fmt.Sprintf("input.tar creation failed: %v", err), Time: time.Now()})
		} else {
			log.Printf("[Mods] ORW input.tar creation failed: %v", err)
		}
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply job failed: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	seaweedCandidates := []string{}
	if seaweedURL != "" {
		seaweedCandidates = append(seaweedCandidates, seaweedURL)
	}
	if !strings.Contains(seaweedURL, "storage.ploy.local") {
		seaweedCandidates = append(seaweedCandidates, "http://seaweedfs-filer.storage.ploy.local:8888")
	}
	if len(seaweedCandidates) == 0 {
		seaweedCandidates = append(seaweedCandidates, "http://seaweedfs-filer.storage.ploy.local:8888")
	}

	var uploadErr error
	for _, candidate := range seaweedCandidates {
		if candidate == "" || modID == "" {
			continue
		}
		if rep := o.runner.GetEventReporter(); rep != nil {
			_ = rep.Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "info", Message: fmt.Sprintf("uploading input.tar to %s", candidate), Time: time.Now()})
		}
		if err := uploadInputTar(candidate, modID, inputTar); err != nil {
			uploadErr = err
			if rep := o.runner.GetEventReporter(); rep != nil {
				_ = rep.Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "warn", Message: fmt.Sprintf("input.tar upload failed: %v", err), Time: time.Now()})
			} else {
				log.Printf("[Mods] ORW input.tar upload failed: %v", err)
			}
			continue
		}
		inputURL := strings.TrimRight(candidate, "/") + "/artifacts/mods/" + modID + "/input.tar"
		available := false
		for i := 0; i < 10; i++ {
			if headURLFn(inputURL) {
				available = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !available {
			uploadErr = fmt.Errorf("input.tar not reachable at %s", inputURL)
			if rep := o.runner.GetEventReporter(); rep != nil {
				_ = rep.Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "warn", Message: uploadErr.Error(), Time: time.Now()})
			} else {
				log.Printf("[Mods] %s", uploadErr.Error())
			}
			continue
		}
		seaweedURL = candidate
		uploadErr = nil
		if rep := o.runner.GetEventReporter(); rep != nil {
			_ = rep.Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "info", Message: fmt.Sprintf("input.tar available at %s", inputURL), Time: time.Now()})
		}
		break
	}
	if uploadErr != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply job failed: %v", uploadErr)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	vars := makeORWVars(baseDir, modID, diffKey, seaweedURL)
	if strings.TrimSpace(rclass) != "" {
		vars["RECIPE_CLASS"] = rclass
	}
	if parts := strings.SplitN(rcoords, ":", 3); len(parts) == 3 {
		vars["RECIPE_GROUP"] = parts[0]
		vars["RECIPE_ARTIFACT"] = parts[1]
		vars["RECIPE_VERSION"] = parts[2]
	}
	if strings.TrimSpace(pluginVersion) != "" {
		vars["MAVEN_PLUGIN_VERSION"] = pluginVersion
	}
	if rep := o.runner.GetEventReporter(); rep != nil {
		_ = rep.Report(ctx, Event{
			Phase: "fanout",
			Step:  string(StepTypeORWApply),
			Level: "info",
			Message: fmt.Sprintf(
				"orw env coords resolved: class=%q group=%q artifact=%q version=%q plugin=%q",
				rclass,
				vars["RECIPE_GROUP"],
				vars["RECIPE_ARTIFACT"],
				vars["RECIPE_VERSION"],
				vars["MAVEN_PLUGIN_VERSION"],
			),
			Time: time.Now(),
		})
	}

	// Step 2b: Substitute environment variables in HCL template
	renderedHCLPath, err := substituteORWTemplateVars(prePath, runID, vars)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to substitute ORW HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	if rep := o.runner.GetEventReporter(); rep != nil {
		summary := []string{}
		placeholderDetected := false
		if data, readErr := os.ReadFile(renderedHCLPath); readErr == nil {
			for _, line := range strings.Split(string(data), "\n") {
				trim := strings.TrimSpace(line)
				if strings.HasPrefix(trim, "RECIPE_") || strings.HasPrefix(trim, "MAVEN_PLUGIN_VERSION") {
					summary = append(summary, trim)
					hasPlaceholder := strings.Contains(trim, "${RECIPE_") || strings.Contains(trim, "${MAVEN_PLUGIN_VERSION}")
					if hasPlaceholder {
						if strings.Contains(trim, "${MAVEN_PLUGIN_VERSION}") && pluginVersion == "" {
							continue
						}
						placeholderDetected = true
					}
				}
			}
		} else {
			summary = append(summary, fmt.Sprintf("failed to read submitted HCL: %v", readErr))
		}
		_ = rep.Report(ctx, Event{
			Phase:   "fanout",
			Step:    string(StepTypeORWApply),
			Level:   "info",
			Message: fmt.Sprintf("orw submitted env: %s", strings.Join(summary, "; ")),
			Time:    time.Now(),
		})
		if placeholderDetected {
			placeholderErr := fmt.Errorf("ORW submitted HCL contains unresolved recipe placeholders")
			_ = rep.Report(ctx, Event{Phase: "fanout", Step: string(StepTypeORWApply), Level: "error", Message: placeholderErr.Error(), Time: time.Now()})
			result.Status = "failed"
			result.Notes = placeholderErr.Error()
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			return result
		}
	}

	// Step 3: Report job metadata asynchronously (job name == runID)
	var rep2 EventReporter
	if o.runner != nil {
		rep2 = o.runner.GetEventReporter()
	}
	reportJobSubmittedAsync(ctx, rep2, runID, "apply", string(StepTypeORWApply))

	// Step 4: Preflight validate HCL, then submit job to Nomad and wait for completion
	if err := orwValidateAndSubmit(ctx, o.hcl, renderedHCLPath, ResolveDefaultsFromEnv().AllowPartialORW); err != nil {
		result.Status = "failed"
		result.Notes = err.Error()
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		if o.runner != nil && o.runner.GetEventReporter() != nil {
			_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "error", Message: fmt.Sprintf("branch %s failed: %s", branch.ID, result.Notes), Time: time.Now()})
		}
		return result
	}

	// Step 5: Check for generated diff.patch artifact
	// Step 5: Finalize
	if o.runner != nil && o.runner.GetEventReporter() != nil {
		_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "info", Message: fmt.Sprintf("branch %s completed", branch.ID), Time: time.Now()})
	}
	result.JobID = runID
	result.DiffPath = filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	if modID != "" {
		result.DiffKey = diffKey
	}
	orwFinalize(&result, renderedHCLPath, branch.ID)
	return result
}
