package mods

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func runMod(args []string, controllerURL string) error {
	// Parse command line flags
	fs := flag.NewFlagSet("mod run", flag.ContinueOnError)
	file := fs.String("f", "", "mods YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	dryRun := fs.Bool("dry-run", false, "validate configuration without executing")
	testMode := fs.Bool("test-mode", false, "use mock implementations for testing (no real builds/pushes)")
	renderPlanner := fs.Bool("render-planner", false, "render planner inputs and HCL (no submission)")
	submitPlanner := fs.Bool("plan", false, "render and (optionally) submit planner job; prints paths. Set MODS_SUBMIT=1 to submit.")
	submitReducer := fs.Bool("reduce", false, "render and (optionally) submit reducer job; prints next actions. Set MODS_SUBMIT=1 to submit.")
	execFirst := fs.Bool("execute-first", false, "after reading plan.json, print which first option would be executed (sequential stub)")
	execLLM := fs.Bool("exec-llm-first", false, "render and optionally submit llm-exec job for the first plan option of type llm-exec")
	execORW := fs.Bool("exec-orw-first", false, "render and optionally submit orw-apply job for the first plan option of type orw-gen (requires recipe envs)")
	applyFirst := fs.Bool("apply-first", false, "after fetching diff (MODS_DIFF_URL/MODS_DIFF_PATH), clone repo, validate/apply diff, commit, and run build gate")
	verbose := fs.Bool("v", false, "verbose output")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace (for debugging)")
	outputFmt := fs.String("output", "text", "output format: text|json (json prints mod_id and exits in remote mode)")
	// SBOM flags to override config
	sbomEnabled := fs.String("sbom", "", "override SBOM enabled (true|false)")
	sbomFail := fs.String("sbom-fail-on-error", "", "override SBOM fail_on_error (true|false)")

	// Optional: auto-attach watch after starting remote run
	watch := fs.Bool("watch", false, "after starting remote run, attach a live watch")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("flag parsing failed: %w", err)
	}

	// Validate output format
	if *outputFmt != "text" && *outputFmt != "json" {
		return fmt.Errorf("invalid --output value: %s (expected text|json)", *outputFmt)
	}

	// Remote mode (default when a controller URL is provided): thin client calling Controller API to execute on VPS
	if controllerURL != "" && !*dryRun && !*renderPlanner && !*submitPlanner && !*submitReducer {
		if *file == "" {
			return fmt.Errorf("missing -f <mods.yaml>")
		}
		return executeRemoteMod(controllerURL, *file, *testMode, *verbose, *watch, *outputFmt)
	}

	if *file == "" {
		return fmt.Errorf("missing -f <mods.yaml>")
	}

	// Load and validate configuration
	config, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %w", *file, err)
	}

	if *verbose {
		fmt.Printf("Loaded mods config: %s\n", config.ID)
	}

	if *dryRun {
		if *outputFmt == "json" {
			// Print a minimal JSON ack for tooling
			// Shape: {"ok":true,"mode":"dry-run","id":"<config id>"}
			fmt.Printf("{\"ok\":true,\"mode\":\"dry-run\",\"id\":\"%s\"}\n", config.ID)
		} else {
			fmt.Println("Configuration is valid")
		}
		return nil
	}

	// Apply SBOM flag overrides if provided
	if *sbomEnabled != "" {
		if strings.EqualFold(*sbomEnabled, "true") || *sbomEnabled == "1" {
			if config.SBOM == nil {
				config.SBOM = &SBOMConfig{}
			}
			config.SBOM.Enabled = true
		} else if strings.EqualFold(*sbomEnabled, "false") || *sbomEnabled == "0" {
			if config.SBOM == nil {
				config.SBOM = &SBOMConfig{}
			}
			config.SBOM.Enabled = false
		}
	}
	if *sbomFail != "" {
		if strings.EqualFold(*sbomFail, "true") || *sbomFail == "1" {
			if config.SBOM == nil {
				config.SBOM = &SBOMConfig{}
			}
			config.SBOM.FailOnError = true
		} else if strings.EqualFold(*sbomFail, "false") || *sbomFail == "0" {
			if config.SBOM == nil {
				config.SBOM = &SBOMConfig{}
			}
			config.SBOM.FailOnError = false
		}
	}

	// Create working directory
	var workspaceDir string
	if *workDir != "" {
		workspaceDir = *workDir
		if err := os.MkdirAll(workspaceDir, 0755); err != nil {
			return fmt.Errorf("failed to create work directory %s: %w", workspaceDir, err)
		}
	} else {
		workspaceDir, err = os.MkdirTemp("", "mods-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		if !*preserve {
			defer func() { _ = os.RemoveAll(workspaceDir) }()
		}
	}

	if *verbose {
		fmt.Printf("Using workspace: %s\n", workspaceDir)
	}

	// Create integrations and runner
	integrations := NewModIntegrationsWithTestMode(controllerURL, workspaceDir, *testMode)
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
		if err := executePlannerMode(runner, *preserve, *verbose); err != nil {
			return err
		}

		// Handle execution modes that require plan.json
		if *execFirst || *execLLM || *execORW || *applyFirst {
			// Try to get plan bytes from various sources
			var planBytes []byte
			// This is a simplified version - the original has complex plan.json retrieval logic
			planPath := os.Getenv("MODS_PLAN_PATH")
			if planPath == "" {
				planPath = findPlanJSON(runner.workspaceDir)
			}
			if planPath != "" {
				if b, err := os.ReadFile(planPath); err == nil {
					planBytes = b
				}
			}
			return executeWithPlan(runner, planBytes, *execFirst, *execLLM, *execORW, *applyFirst)
		}
		return nil
	}

	if *submitReducer {
		return executeReducerMode(runner, *preserve)
	}

	// Execute the Mods run
	ctx := context.Background()
	startTime := time.Now()

	fmt.Printf("Starting mod execution: %s\n", config.ID)

	result, err := runner.Run(ctx)
	if err != nil {
		fmt.Printf("Mod failed after %v\n", time.Since(startTime))
		if result != nil {
			fmt.Println(result.Summary())
		}
		return fmt.Errorf("mod execution failed: %w", err)
	}

	// Print results
	fmt.Printf("Mod completed successfully in %v\n", time.Since(startTime))
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
