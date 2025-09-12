package transflow

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// executePlannerMode renders and optionally submits planner job
func executePlannerMode(runner *TransflowRunner, preserve, verbose bool) error {
	assets, err := runner.RenderPlannerAssets()
	if err != nil {
		return fmt.Errorf("failed to render planner assets: %w", err)
	}

	// Substitute placeholders
	hclBytes, err := os.ReadFile(assets.HCLPath)
	if err != nil {
		return fmt.Errorf("failed to read planner HCL: %w", err)
	}

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

	// Defaults for images (env override) - prefer VPS registry
	plannerImage := os.Getenv("TRANSFLOW_PLANNER_IMAGE")
	if plannerImage == "" {
		reg := os.Getenv("TRANSFLOW_REGISTRY")
		if reg == "" {
			reg = "registry.dev.ployman.app"
		}
		plannerImage = reg + "/langgraph-runner:py-0.1.0"
	}

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

	fmt.Printf("Planner HCL rendered: %s\n", renderedPath)
	if preserve {
		fmt.Printf("Workspace preserved at: %s\n", runner.workspaceDir)
	}

	// Optionally submit if TRANSFLOW_SUBMIT=1
	if os.Getenv("TRANSFLOW_SUBMIT") != "1" {
		fmt.Println("Skipping submission (unset TRANSFLOW_SUBMIT).")
		return nil
	}

    timeout := ResolveDefaultsFromEnv().PlannerTimeout
    if err := orchestration.SubmitAndWaitTerminal(renderedPath, timeout); err != nil {
        return fmt.Errorf("planner job failed: %w", err)
    }

	// Attempt to read plan.json from URL or locally (also support SeaweedFS filer via bucket/key)
	if url := os.Getenv("TRANSFLOW_PLAN_URL"); url != "" {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if err := validatePlanJSON(b); err != nil {
				fmt.Printf("plan.json schema invalid: %v\n", err)
			} else {
				printPlanSummary(b)
			}
		} else {
			if err != nil {
				fmt.Printf("Failed to fetch plan URL: %v\n", err)
			} else {
				fmt.Printf("Failed to fetch plan URL: %s\n", resp.Status)
			}
		}
	}

	if filer := os.Getenv("TRANSFLOW_FILER"); filer != "" {
		if bucket := os.Getenv("TRANSFLOW_BUCKET"); bucket != "" {
			if key := os.Getenv("TRANSFLOW_PLAN_KEY"); key != "" {
				url := strings.TrimRight(filer, "/") + "/" + strings.TrimLeft(bucket, "/") + "/" + strings.TrimLeft(key, "/")
				if resp, err := http.Get(url); err == nil && resp.StatusCode == 200 {
					defer resp.Body.Close()
					if b, err := io.ReadAll(resp.Body); err == nil {
						if err := validatePlanJSON(b); err != nil {
							fmt.Printf("plan.json schema invalid: %v\n", err)
						} else {
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
			fmt.Printf("plan.json schema invalid: %v\n", err)
		} else {
			printPlanSummary(planBytes)
		}
	} else {
		// brief retry loop (up to ~8s)
		for i := 0; i < 8 && len(planBytes) == 0; i++ {
			time.Sleep(1 * time.Second)
			if b2, err2 := readPlan(planPath); err2 == nil {
				planBytes = b2
				if err := validatePlanJSON(planBytes); err != nil {
					fmt.Printf("plan.json schema invalid: %v\n", err)
				} else {
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
						fmt.Printf("plan.json schema invalid: %v\n", err)
					} else {
						fmt.Printf("Discovered plan.json at: %s\n", auto)
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
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if err := validateNextJSON(b); err != nil {
				fmt.Printf("next.json schema invalid: %v\n", err)
			} else {
				printNextSummary(b)
			}
		}
	}
	if np := os.Getenv("TRANSFLOW_NEXT_PATH"); np != "" {
		if b, err := os.ReadFile(np); err == nil {
			if err := validateNextJSON(b); err != nil {
				fmt.Printf("next.json schema invalid: %v\n", err)
			} else {
				printNextSummary(b)
			}
		}
	}

	return nil
}
