package transflow

import (
    "context"
    "io"
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    orchestration "github.com/iw2rmb/ploy/internal/orchestration"
    "encoding/json"
    "net/http"
)
// lightweight JSON unmarshal to avoid adding deps
func jsonUnmarshal(b []byte, v any) error {
    // use stdlib encoding/json
    return json.Unmarshal(b, v)
}

func printPlanSummary(b []byte) {
    var parsed struct{
        PlanID string `json:"plan_id"`
        Options []map[string]any `json:"options"`
    }
    if err := jsonUnmarshal(b, &parsed); err == nil && parsed.PlanID != "" && len(parsed.Options) > 0 {
        fmt.Printf("Planner produced plan (id=%s) with %d option(s):\n", parsed.PlanID, len(parsed.Options))
        for i, o := range parsed.Options {
            id, _ := o["id"].(string)
            typ, _ := o["type"].(string)
            fmt.Printf("  %d) %s (%s)\n", i+1, id, typ)
        }
    } else {
        fmt.Printf("Planner finished but plan.json validation failed or missing keys: %v\n", err)
    }
}

func printNextSummary(b []byte) {
    var parsed struct{
        Action string `json:"action"`
        Notes  string `json:"notes"`
    }
    if err := jsonUnmarshal(b, &parsed); err == nil && parsed.Action != "" {
        fmt.Printf("Reducer next action: %s", parsed.Action)
        if parsed.Notes != "" { fmt.Printf(" (%s)", parsed.Notes) }
        fmt.Println()
    } else {
        fmt.Printf("Reducer output invalid or missing keys: %v\n", err)
    }
}

// TransflowCmd provides the CLI entrypoint to run transflows
func TransflowCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Usage: ploy transflow run -f <transflow.yaml>")
		return
	}

	switch args[0] {
	case "run":
		if err := runTransflow(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Println("Usage: ploy transflow run -f <transflow.yaml>")
	}
}

// runTransflow handles the actual transflow execution
func runTransflow(args []string, controllerURL string) error {
	// Parse command line flags
	fs := flag.NewFlagSet("transflow run", flag.ContinueOnError)
	file := fs.String("f", "", "transflow YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
    dryRun := fs.Bool("dry-run", false, "validate configuration without executing")
    renderPlanner := fs.Bool("render-planner", false, "render planner inputs and HCL (no submission)")
    submitPlanner := fs.Bool("plan", false, "render and (optionally) submit planner job; prints paths. Set TRANSFLOW_SUBMIT=1 to submit.")
    execFirst := fs.Bool("execute-first", false, "after reading plan.json, print which first option would be executed (sequential stub)")
	verbose := fs.Bool("v", false, "verbose output")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("flag parsing failed: %w", err)
	}

	if *file == "" {
		return fmt.Errorf("missing -f <transflow.yaml>")
	}

	// Load and validate configuration
	config, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %w", *file, err)
	}

	if *verbose {
		fmt.Printf("Loaded transflow config: %s\n", config.ID)
	}

    if *dryRun {
        fmt.Println("Configuration is valid")
        return nil
    }

	// Create working directory
	var workspaceDir string
	if *workDir != "" {
		workspaceDir = *workDir
		if err := os.MkdirAll(workspaceDir, 0755); err != nil {
			return fmt.Errorf("failed to create work directory %s: %w", workspaceDir, err)
		}
	} else {
		workspaceDir, err = os.MkdirTemp("", "transflow-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		defer os.RemoveAll(workspaceDir)
	}

	if *verbose {
		fmt.Printf("Using workspace: %s\n", workspaceDir)
	}

    // Create integrations and runner
    integrations := NewTransflowIntegrations(controllerURL, workspaceDir)
    runner, err := integrations.CreateConfiguredRunner(config)
    if err != nil {
        return fmt.Errorf("failed to create runner: %w", err)
    }

    if *renderPlanner {
        // Prepare minimal planner inputs/HCL without submitting
        assets, err := runner.RenderPlannerAssets()
        if err != nil {
            return fmt.Errorf("failed to render planner assets: %w", err)
        }
        fmt.Printf("Planner assets rendered:\n  inputs: %s\n  hcl:    %s\n", assets.InputsPath, assets.HCLPath)
        return nil
    }

    if *submitPlanner {
        assets, err := runner.RenderPlannerAssets()
        if err != nil {
            return fmt.Errorf("failed to render planner assets: %w", err)
        }
        // Substitute placeholders
        hclBytes, err := os.ReadFile(assets.HCLPath)
        if err != nil { return fmt.Errorf("failed to read planner HCL: %w", err) }
        model := os.Getenv("TRANSFLOW_MODEL")
        if model == "" { model = "gpt-4o-mini@2024-08-06" }
        toolsJSON := os.Getenv("TRANSFLOW_TOOLS")
        if toolsJSON == "" { toolsJSON = `{"file":{"allow":["src/**","pom.xml"]},"search":{"provider":"rg","allow":["src/**"]}}` }
        limitsJSON := os.Getenv("TRANSFLOW_LIMITS")
        if limitsJSON == "" { limitsJSON = `{"max_steps":8,"max_tool_calls":12,"timeout":"30m"}` }
        runID := fmt.Sprintf("%s-%d", runner.config.ID, time.Now().Unix())
        rendered := strings.NewReplacer(
            "${MODEL}", model,
            "${TOOLS_JSON}", toolsJSON,
            "${LIMITS_JSON}", limitsJSON,
            "${RUN_ID}", runID,
        ).Replace(string(hclBytes))
        renderedPath := filepath.Join(filepath.Dir(assets.HCLPath), "planner.rendered.hcl")
        if err := os.WriteFile(renderedPath, []byte(rendered), 0644); err != nil {
            return fmt.Errorf("failed to write rendered HCL: %w", err)
        }
        fmt.Printf("Planner HCL rendered: %s\n", renderedPath)
        // Optionally submit if TRANSFLOW_SUBMIT=1
        if os.Getenv("TRANSFLOW_SUBMIT") == "1" {
            timeout := 30 * time.Minute
            if err := orchestration.SubmitAndWaitTerminal(renderedPath, timeout); err != nil {
                return fmt.Errorf("planner job failed: %w", err)
            }
            // Attempt to read plan.json from URL or locally
            if url := os.Getenv("TRANSFLOW_PLAN_URL"); url != "" {
                resp, err := http.Get(url)
                if err == nil && resp.StatusCode == 200 {
                    defer resp.Body.Close()
                    b, _ := io.ReadAll(resp.Body)
                    printPlanSummary(b)
                } else {
                    if err != nil { fmt.Printf("Failed to fetch plan URL: %v\n", err) } else { fmt.Printf("Failed to fetch plan URL: %s\n", resp.Status) }
                }
            }
            // Attempt to read plan.json locally if provided
            planPath := os.Getenv("TRANSFLOW_PLAN_PATH")
            if planPath == "" {
                // Fallback to workspace/planner/out/plan.json (works if out volume maps locally)
                planPath = filepath.Join(filepath.Dir(renderedPath), "out", "plan.json")
            }
            var planBytes []byte
            if b, err := os.ReadFile(planPath); err == nil {
                planBytes = b
                printPlanSummary(planBytes)
            } else {
                fmt.Println("Planner job completed. Could not read plan.json locally; set TRANSFLOW_PLAN_PATH or TRANSFLOW_PLAN_URL.")
            }
            // Optional reducer artifact print (if provided externally)
            if url := os.Getenv("TRANSFLOW_NEXT_URL"); url != "" {
                resp, err := http.Get(url)
                if err == nil && resp.StatusCode == 200 {
                    defer resp.Body.Close()
                    b, _ := io.ReadAll(resp.Body)
                    printNextSummary(b)
                }
            }
            if np := os.Getenv("TRANSFLOW_NEXT_PATH"); np != "" {
                if b, err := os.ReadFile(np); err == nil { printNextSummary(b) }
            }
            if *execFirst && len(planBytes) > 0 {
                // Sequential stub: select first option and print intended action
                var parsed struct{ PlanID string `json:"plan_id"`; Options []map[string]any `json:"options"` }
                if err := jsonUnmarshal(planBytes, &parsed); err == nil && len(parsed.Options) > 0 {
                    o := parsed.Options[0]
                    id, _ := o["id"].(string)
                    typ, _ := o["type"].(string)
                    fmt.Printf("Sequential stub: would execute first option %s (%s) next.\n", id, typ)
                }
            }
        } else {
            fmt.Println("Skipping submission (unset TRANSFLOW_SUBMIT).")
        }
        return nil
    }

	// Execute the transflow
	ctx := context.Background()
	startTime := time.Now()

	fmt.Printf("Starting transflow execution: %s\n", config.ID)

	result, err := runner.Run(ctx)
	if err != nil {
		fmt.Printf("Transflow failed after %v\n", time.Since(startTime))
		if result != nil {
			fmt.Println(result.Summary())
		}
		return fmt.Errorf("transflow execution failed: %w", err)
	}

	// Print results
	fmt.Printf("Transflow completed successfully in %v\n", time.Since(startTime))
	if *verbose && result != nil {
		fmt.Println(result.Summary())
	} else {
		fmt.Printf("Branch: %s\n", result.BranchName)
		if result.BuildVersion != "" {
			fmt.Printf("Build: %s\n", result.BuildVersion)
		}
	}

	return nil
}
