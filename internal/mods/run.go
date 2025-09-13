package transflow

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mime"
)

// ModCmd provides the CLI entrypoint to run mods
func ModCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		printModHelp()
		return
	}

	switch args[0] {
	case "help":
		printModHelp()
		return
	case "run":
		if err := runTransflow(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "watch":
		if err := watchTransflow(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "render":
		if err := transflowRenderCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "plan":
		if err := transflowPlanCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "reduce":
		if err := transflowReduceCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "apply":
		if err := transflowApplyCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		printModHelp()
	}
}

func printModHelp() {
	fmt.Println("Usage: ploy mod <subcommand> [options]")
	fmt.Println("Subcommands:")
	fmt.Println("  run      - Execute full workflow remotely (default mode)")
	fmt.Println("  watch    - Attach to a running execution by ID")
	fmt.Println("  render   - Render planner inputs and HCL locally (no submission)")
	fmt.Println("  plan     - Render planner and optionally submit (use --submit)")
	fmt.Println("  reduce   - Render reducer and optionally submit (use --submit)")
	fmt.Println("  apply    - Apply a diff locally and run build gate (use --diff-path/--diff-url)")
	fmt.Println("  help     - Show this help message")
}

// runTransflow handles the actual transflow execution
func runTransflow(args []string, controllerURL string) error {
	// Parse command line flags
	fs := flag.NewFlagSet("transflow run", flag.ContinueOnError)
	file := fs.String("f", "", "transflow YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	dryRun := fs.Bool("dry-run", false, "validate configuration without executing")
	testMode := fs.Bool("test-mode", false, "use mock implementations for testing (no real builds/pushes)")
	renderPlanner := fs.Bool("render-planner", false, "render planner inputs and HCL (no submission)")
	submitPlanner := fs.Bool("plan", false, "render and (optionally) submit planner job; prints paths. Set TRANSFLOW_SUBMIT=1 to submit.")
	submitReducer := fs.Bool("reduce", false, "render and (optionally) submit reducer job; prints next actions. Set TRANSFLOW_SUBMIT=1 to submit.")
	execFirst := fs.Bool("execute-first", false, "after reading plan.json, print which first option would be executed (sequential stub)")
	execLLM := fs.Bool("exec-llm-first", false, "render and optionally submit llm-exec job for the first plan option of type llm-exec")
	execORW := fs.Bool("exec-orw-first", false, "render and optionally submit orw-apply job for the first plan option of type orw-gen (requires recipe envs)")
	applyFirst := fs.Bool("apply-first", false, "after fetching diff (TRANSFLOW_DIFF_URL/TRANSFLOW_DIFF_PATH), clone repo, validate/apply diff, commit, and run build gate")
	verbose := fs.Bool("v", false, "verbose output")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace (for debugging)")
	outputFmt := fs.String("output", "text", "output format: text|json (json prints execution_id and exits in remote mode)")

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
			return fmt.Errorf("missing -f <transflow.yaml>")
		}
		return executeRemoteTransflow(controllerURL, *file, *testMode, *verbose, *watch, *outputFmt)
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
		if *outputFmt == "json" {
			// Print a minimal JSON ack for tooling
			// Shape: {"ok":true,"mode":"dry-run","id":"<config id>"}
			fmt.Printf("{\"ok\":true,\"mode\":\"dry-run\",\"id\":\"%s\"}\n", config.ID)
		} else {
			fmt.Println("Configuration is valid")
		}
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
		if !*preserve {
			defer os.RemoveAll(workspaceDir)
		}
	}

	if *verbose {
		fmt.Printf("Using workspace: %s\n", workspaceDir)
	}

	// Create integrations and runner
	integrations := NewTransflowIntegrationsWithTestMode(controllerURL, workspaceDir, *testMode)
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
			planPath := os.Getenv("TRANSFLOW_PLAN_PATH")
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

	// Execute the transflow
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

// transflowRenderCmd: planner render (no submission)
func transflowRenderCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod render", flag.ContinueOnError)
	file := fs.String("f", "", "transflow YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <transflow.yaml>")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "transflow-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer os.RemoveAll(wd)
	}
	integrations := NewTransflowIntegrationsWithTestMode(controllerURL, wd, true)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	_ = verbose // reserved; executePlannerMode emits events/logs already
	return executePlannerMode(runner, *preserve, *verbose)
}

// transflowPlanCmd: render planner and optionally submit when --submit provided
func transflowPlanCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod plan", flag.ContinueOnError)
	file := fs.String("f", "", "transflow YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	submit := fs.Bool("submit", false, "submit planner job after rendering")
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <transflow.yaml>")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "transflow-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer os.RemoveAll(wd)
	}
	integrations := NewTransflowIntegrationsWithTestMode(controllerURL, wd, false)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	if *submit {
		os.Setenv("TRANSFLOW_SUBMIT", "1")
	} else {
		os.Unsetenv("TRANSFLOW_SUBMIT")
	}
	return executePlannerMode(runner, *preserve, *verbose)
}

// transflowReduceCmd: render reducer and optionally submit when --submit
func transflowReduceCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod reduce", flag.ContinueOnError)
	file := fs.String("f", "", "transflow YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	submit := fs.Bool("submit", false, "submit reducer job after rendering")
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <transflow.yaml>")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "transflow-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer os.RemoveAll(wd)
	}
	integrations := NewTransflowIntegrationsWithTestMode(controllerURL, wd, false)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	if *submit {
		os.Setenv("TRANSFLOW_SUBMIT", "1")
	} else {
		os.Unsetenv("TRANSFLOW_SUBMIT")
	}
	_ = verbose
	return executeReducerMode(runner, *preserve)
}

// transflowApplyCmd: apply a diff to repo and run build gate
func transflowApplyCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod apply", flag.ContinueOnError)
	file := fs.String("f", "", "transflow YAML file")
	diffPath := fs.String("diff-path", "", "local unified diff file path")
	diffURL := fs.String("diff-url", "", "URL to download unified diff")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <transflow.yaml>")
	}
	if *diffPath == "" && *diffURL == "" {
		return fmt.Errorf("provide --diff-path or --diff-url")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "transflow-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer os.RemoveAll(wd)
	}
	integrations := NewTransflowIntegrationsWithTestMode(controllerURL, wd, false)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	// Wire env for executeApplyFirst (compat)
	if *diffURL != "" {
		os.Setenv("TRANSFLOW_DIFF_URL", *diffURL)
	} else {
		os.Unsetenv("TRANSFLOW_DIFF_URL")
	}
	if *diffPath != "" {
		os.Setenv("TRANSFLOW_DIFF_PATH", *diffPath)
	} else {
		os.Unsetenv("TRANSFLOW_DIFF_PATH")
	}
	return executeApplyFirst(runner)
}

// watchTransflow polls the controller for status updates and streams step events
func watchTransflow(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod watch", flag.ContinueOnError)
	id := fs.String("id", "", "execution id to watch")
	interval := fs.Duration("interval", 2*time.Second, "poll interval")
	noSSE := fs.Bool("no-sse", false, "disable SSE and use polling")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("flag parsing failed: %w", err)
	}
	if *id == "" {
		return fmt.Errorf("missing -id <execution_id>")
	}
	base := controllerURL
	if base == "" {
		base = GetDefaultControllerURL()
	}
	base = strings.TrimRight(base, "/")
	// The controller uses /v1/mods/* endpoints; ensure /v1 prefix exists
	if !strings.HasSuffix(base, "/v1") {
		base = base + "/v1"
	}
	statusURL := base + "/mods/" + *id + "/status"
	artsURL := base + "/mods/" + *id + "/artifacts"

	if !*noSSE {
		if err := watchTransflowSSE(base, *id); err == nil {
			return nil
		}
		fmt.Println("SSE unavailable; falling back to polling...")
	}

	seen := 0
	lastStatus := ""
	client := &http.Client{Timeout: 10 * time.Second}
	fmt.Printf("Watching transflow %s (poll %s)\n", *id, interval.String())
	for {
		req, _ := http.NewRequest(http.MethodGet, statusURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("status error: %v\n", err)
			time.Sleep(*interval)
			continue
		}
		var st struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Phase    string `json:"phase"`
			Duration string `json:"duration"`
			Overdue  bool   `json:"overdue"`
			Steps    []struct {
				Step    string    `json:"step"`
				Phase   string    `json:"phase"`
				Level   string    `json:"level"`
				Message string    `json:"message"`
				Time    time.Time `json:"time"`
			} `json:"steps"`
			Result map[string]interface{} `json:"result"`
			Error  string                 `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&st)
		_ = resp.Body.Close()

		if st.Status != lastStatus || st.Phase != "" {
			fmt.Printf("[%s] phase=%s status=%s duration=%s overdue=%v\n", time.Now().Format(time.RFC3339), st.Phase, st.Status, st.Duration, st.Overdue)
			lastStatus = st.Status
		}

		if len(st.Steps) > seen {
			for _, ev := range st.Steps[seen:] {
				ts := ev.Time.Format(time.RFC3339)
				if ev.Level == "" {
					ev.Level = "info"
				}
				if ev.Step == "" {
					ev.Step = "?"
				}
				fmt.Printf("%s [%s] %s: %s\n", ts, ev.Level, ev.Step, ev.Message)
			}
			seen = len(st.Steps)
		}

		if st.Status == "completed" || st.Status == "failed" || st.Status == "cancelled" {
			// Print error log if available
			if st.Error != "" {
				fmt.Printf("Error: %s\n", st.Error)
			}
			// Try to fetch error_log artifact
			req2, _ := http.NewRequest(http.MethodGet, artsURL, nil)
			resp2, err2 := client.Do(req2)
			if err2 == nil && resp2.StatusCode == 200 {
				var arts struct {
					Artifacts map[string]string `json:"artifacts"`
				}
				_ = json.NewDecoder(resp2.Body).Decode(&arts)
				_ = resp2.Body.Close()
				if key := arts.Artifacts["error_log"]; key != "" {
					// Server provides a download endpoint alias using logical name
					dl := base + "/mods/" + *id + "/artifacts/error_log"
					if r3, e3 := client.Get(dl); e3 == nil && r3.StatusCode == 200 {
						defer r3.Body.Close()
						fmt.Println("--- error.log ---")
						io.Copy(os.Stdout, r3.Body)
						fmt.Println("\n--- end error.log ---")
					}
				}
			}
			break
		}
		time.Sleep(*interval)
	}
	return nil
}

func watchTransflowSSE(base, id string) error {
	url := fmt.Sprintf("%s/mods/%s/logs?follow=true", base, id)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected SSE response: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// Accept text/event-stream with optional charset
	if ctype := resp.Header.Get("Content-Type"); ctype != "" {
		if mt, _, err := mime.ParseMediaType(ctype); err == nil {
			if mt != "text/event-stream" {
				return fmt.Errorf("unexpected SSE response: %d %s", resp.StatusCode, ctype)
			}
		} else if !strings.HasPrefix(strings.ToLower(ctype), "text/event-stream") {
			return fmt.Errorf("unexpected SSE response: %d %s", resp.StatusCode, ctype)
		}
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	curEvent := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			curEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			switch curEvent {
			case "init":
				fmt.Printf("[SSE] %s\n", data)
			case "meta":
				fmt.Printf("[meta] %s\n", data)
			case "step":
				// Try to decode minimally to format nicer; fallback to raw
				var ev struct{ Level, Step, Message string }
				if _ = json.Unmarshal([]byte(data), &ev); ev.Step != "" {
					lvl := ev.Level
					if lvl == "" {
						lvl = "info"
					}
					fmt.Printf("[%s] %s: %s\n", lvl, ev.Step, ev.Message)
				} else {
					fmt.Printf("%s\n", data)
				}
			case "log":
				fmt.Printf("[log]\n%s\n", data)
			case "ping":
				// ignore
			case "end":
				fmt.Printf("[end] %s\n", data)
				return nil
			default:
				fmt.Printf("[%s] %s\n", curEvent, data)
			}
		}
		if line == "" {
			curEvent = ""
		}
	}
	return scanner.Err()
}
