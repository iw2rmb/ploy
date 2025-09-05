package transflow

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

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
