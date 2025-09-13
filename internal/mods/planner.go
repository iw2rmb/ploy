package transflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// executePlannerMode renders and optionally submits planner job
func executePlannerMode(runner *TransflowRunner, preserve, verbose bool) error {
	ctx := context.Background()
	assets, err := runner.RenderPlannerAssets()
	if err != nil {
		return fmt.Errorf("failed to render planner assets: %w", err)
	}

	// Substitute placeholders
	hclBytes, err := os.ReadFile(assets.HCLPath)
	if err != nil {
		return fmt.Errorf("failed to read planner HCL: %w", err)
	}

	llm := ResolveLLMDefaultsFromEnv()
	model := llm.Model
	toolsJSON := llm.ToolsJSON
	limitsJSON := llm.LimitsJSON

	runID := PlannerRunID(runner.config.ID)

	// Compute host bind mount directories for planner
	contextDir := filepath.Dir(assets.InputsPath)
	outDir := filepath.Join(runner.workspaceDir, "planner", "out")

	// Escape values for safe HCL string embedding
	hclEscape := func(s string) string {
		s = strings.ReplaceAll(s, "\\", "\\\\")
		s = strings.ReplaceAll(s, "\"", "\\\"")
		return s
	}

	// Resolve planner image via centralized resolver
	plannerImage := ResolveImagesFromEnv().Planner

	rendered := strings.NewReplacer(
		"${MODEL}", hclEscape(model),
		"${TOOLS_JSON}", hclEscape(toolsJSON),
		"${LIMITS_JSON}", hclEscape(limitsJSON),
		"${RUN_ID}", runID,
		"${CONTEXT_HOST_DIR}", hclEscape(contextDir),
		"${OUT_HOST_DIR}", hclEscape(outDir),
		"${PLANNER_IMAGE}", hclEscape(plannerImage),
	).Replace(string(hclBytes))

	renderedPath := filepath.Join(filepath.Dir(assets.HCLPath), "planner.rendered.hcl")
	if err := os.WriteFile(renderedPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("failed to write rendered HCL: %w", err)
	}

	runner.emit(ctx, "planner", "render", "info", fmt.Sprintf("Planner HCL rendered: %s", renderedPath))
	if preserve {
		runner.emit(ctx, "planner", "preserve", "info", fmt.Sprintf("Workspace preserved at: %s", runner.workspaceDir))
	}

	// Optionally submit if TRANSFLOW_SUBMIT=1
	if os.Getenv("TRANSFLOW_SUBMIT") != "1" {
		runner.emit(ctx, "planner", "submit", "info", "Skipping submission (unset TRANSFLOW_SUBMIT)")
		return nil
	}

	timeout := ResolveDefaultsFromEnv().PlannerTimeout
	if err := runner.hcl.Submit(renderedPath, timeout); err != nil {
		runner.emit(ctx, "planner", "submit", "error", fmt.Sprintf("planner job failed: %v", err))
		return fmt.Errorf("planner job failed: %w", err)
	}

	// Attempt to read plan.json from URL or locally (also support SeaweedFS filer via bucket/key)
	if url := os.Getenv("TRANSFLOW_PLAN_URL"); url != "" {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if err := validatePlanJSON(b); err != nil {
				runner.emit(ctx, "planner", "validate", "error", fmt.Sprintf("plan.json schema invalid: %v", err))
			} else {
				runner.emit(ctx, "planner", "validate", "info", "plan.json schema valid")
				printPlanSummary(b)
			}
		} else {
			if err != nil {
				runner.emit(ctx, "planner", "fetch", "error", fmt.Sprintf("Failed to fetch plan URL: %v", err))
			} else {
				runner.emit(ctx, "planner", "fetch", "error", fmt.Sprintf("Failed to fetch plan URL: %s", resp.Status))
			}
		}
	}

	if filer := os.Getenv("TRANSFLOW_FILER"); filer != "" {
		client := &http.Client{Timeout: 15 * time.Second}
		if bucket := os.Getenv("TRANSFLOW_BUCKET"); bucket != "" {
			if key := os.Getenv("TRANSFLOW_PLAN_KEY"); key != "" {
				url := strings.TrimRight(filer, "/") + "/" + strings.TrimLeft(bucket, "/") + "/" + strings.TrimLeft(key, "/")
				if resp, err := client.Get(url); err == nil && resp.StatusCode == 200 {
					defer resp.Body.Close()
					if b, err := io.ReadAll(resp.Body); err == nil {
						if err := validatePlanJSON(b); err != nil {
							runner.emit(ctx, "planner", "validate", "error", fmt.Sprintf("plan.json schema invalid: %v", err))
						} else {
							runner.emit(ctx, "planner", "validate", "info", "plan.json schema valid")
							printPlanSummary(b)
						}
					}
				}
			}
		}
	}

	// Attempt to read plan.json locally if provided
	planPath := os.Getenv("TRANSFLOW_PLAN_PATH")
	if planPath == "" {
		// Fallback to workspace/planner/out/plan.json (works if out volume maps locally)
		planPath = filepath.Join(filepath.Dir(renderedPath), "out", "plan.json")
	}

	var planBytes []byte
	// try immediate read, then short retries
	readPlan := func(p string) ([]byte, error) { return os.ReadFile(p) }
	if b, err := readPlan(planPath); err == nil {
		planBytes = b
		if err := validatePlanJSON(planBytes); err != nil {
			runner.emit(ctx, "planner", "validate", "error", fmt.Sprintf("plan.json schema invalid: %v", err))
		} else {
			runner.emit(ctx, "planner", "validate", "info", "plan.json schema valid")
			printPlanSummary(planBytes)
		}
	} else {
		// brief retry loop (up to ~8s)
		for i := 0; i < 8 && len(planBytes) == 0; i++ {
			time.Sleep(1 * time.Second)
			if b2, err2 := readPlan(planPath); err2 == nil {
				planBytes = b2
				if err := validatePlanJSON(planBytes); err != nil {
					runner.emit(ctx, "planner", "validate", "error", fmt.Sprintf("plan.json schema invalid: %v", err))
				} else {
					runner.emit(ctx, "planner", "validate", "info", "plan.json schema valid")
					printPlanSummary(planBytes)
				}
				break
			}
		}
	}

	if len(planBytes) == 0 {
		// Try to auto-discover plan.json under preserved workspace
		if preserve {
			if auto := findPlanJSON(runner.workspaceDir); auto != "" {
				if b, err2 := os.ReadFile(auto); err2 == nil {
					planBytes = b
					if err := validatePlanJSON(planBytes); err != nil {
						runner.emit(ctx, "planner", "validate", "error", fmt.Sprintf("plan.json schema invalid: %v", err))
					} else {
						runner.emit(ctx, "planner", "discover", "info", fmt.Sprintf("Discovered plan.json at: %s", auto))
						printPlanSummary(planBytes)
					}
				}
			}
		}
	}

	if len(planBytes) == 0 {
		fmt.Println("Planner job completed. Could not read plan.json locally; set TRANSFLOW_PLAN_PATH or TRANSFLOW_PLAN_URL.")
	}

	// Optional reducer artifact print (if provided externally)
	if url := os.Getenv("TRANSFLOW_NEXT_URL"); url != "" {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if err := validateNextJSON(b); err != nil {
				runner.emit(ctx, "reducer", "validate", "error", fmt.Sprintf("next.json schema invalid: %v", err))
			} else {
				runner.emit(ctx, "reducer", "validate", "info", "next.json schema valid (from URL)")
				printNextSummary(b)
			}
		}
	}
	if np := os.Getenv("TRANSFLOW_NEXT_PATH"); np != "" {
		if b, err := os.ReadFile(np); err == nil {
			if err := validateNextJSON(b); err != nil {
				runner.emit(ctx, "reducer", "validate", "error", fmt.Sprintf("next.json schema invalid: %v", err))
			} else {
				runner.emit(ctx, "reducer", "validate", "info", "next.json schema valid (from path)")
				printNextSummary(b)
			}
		}
	}

	return nil
}
