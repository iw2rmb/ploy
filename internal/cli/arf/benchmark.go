package arf

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Benchmark commands

func handleARFBenchmarkCommand(args []string) error {
	if len(args) == 0 {
		printBenchmarkUsage()
		return nil
	}

	action := args[0]
	switch action {
	case "run":
		return handleBenchmarkRun(args[1:])
	case "list":
		return handleBenchmarkList(args[1:])
	case "status":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark status <benchmark-id>")
			return nil
		}
		return handleBenchmarkStatus(args[1])
	case "logs":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark logs <benchmark-id> [--stage <stage>]")
			return nil
		}
		return handleBenchmarkLogs(args[1:])
	case "stop":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark stop <benchmark-id>")
			return nil
		}
		return handleBenchmarkStop(args[1])
	case "--help":
		printBenchmarkUsage()
		return nil
	default:
		fmt.Printf("Unknown benchmark action: %s\n", action)
		printBenchmarkUsage()
		return nil
	}
}

func printBenchmarkUsage() {
	fmt.Println("Usage: ploy arf benchmark <action> [options]")
	fmt.Println()
	fmt.Println("Available actions:")
	fmt.Println("  run <name>           Run a new benchmark")
	fmt.Println("  list                 List benchmarks")
	fmt.Println("  status <id>          Get benchmark status")
	fmt.Println("  logs <id>            Get benchmark logs")
	fmt.Println("  stop <id>            Stop running benchmark")
	fmt.Println()
	fmt.Println("Run options:")
	fmt.Println("  --repository <url>   Repository URL (required)")
	fmt.Println("  --branch <name>      Git branch (default: main)")
	fmt.Println("  --transformations <recipes> Comma-separated recipe IDs")
	fmt.Println("  --app-name <name>    Application name for deployment")
	fmt.Println("  --lane <A-G>         Deployment lane (default: auto)")
	fmt.Println("  --iterations <num>   Max iterations (default: 3)")
	fmt.Println()
	fmt.Println("Predefined benchmarks:")
	fmt.Println("  java11to17_migration Uses default Java 11->17 migration recipes")
}

func handleBenchmarkRun(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf benchmark run <name> --repository <url> [options]")
		return nil
	}

	benchmarkName := args[0]
	repository := ""
	branch := "main"
	transformations := ""
	appName := ""
	lane := "auto"
	maxIterations := 3

	// Parse arguments
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--repository":
			if i+1 < len(args) {
				repository = args[i+1]
				i++
			}
		case "--branch":
			if i+1 < len(args) {
				branch = args[i+1]
				i++
			}
		case "--transformations":
			if i+1 < len(args) {
				transformations = args[i+1]
				i++
			}
		case "--app-name":
			if i+1 < len(args) {
				appName = args[i+1]
				i++
			}
		case "--lane":
			if i+1 < len(args) {
				lane = args[i+1]
				i++
			}
		case "--iterations":
			if i+1 < len(args) {
				if val, err := strconv.Atoi(args[i+1]); err == nil {
					maxIterations = val
				}
				i++
			}
		}
	}

	if repository == "" {
		fmt.Println("Error: --repository is required")
		return nil
	}

	if appName == "" {
		appName = fmt.Sprintf("bench-%s-%d", benchmarkName, time.Now().Unix())
	}

	// Create recipe IDs list from transformations or use default Java migration recipes
	var recipeIDs []string
	if transformations != "" {
		recipeIDs = strings.Split(transformations, ",")
		for i, recipe := range recipeIDs {
			recipeIDs[i] = strings.TrimSpace(recipe)
		}
	} else if benchmarkName == "java11to17_migration" {
		// Default Java migration recipes
		recipeIDs = []string{
			"org.openrewrite.java.migrate.JavaVersion11to17",
			"org.openrewrite.java.migrate.javax.JavaxToJakarta",
		}
	} else {
		fmt.Println("Error: --transformations is required or use a predefined benchmark")
		return nil
	}

	// Create benchmark config with proper field mapping to match BenchmarkConfig struct
	benchmarkConfig := map[string]interface{}{
		// Test identification
		"name":        benchmarkName,
		"description": fmt.Sprintf("Benchmark test for %s", benchmarkName),

		// Repository configuration - FIXED FIELD MAPPING
		"repo_url":    repository, // Fixed: was "repository", now "repo_url"
		"repo_branch": branch,     // Added: missing field

		// Task configuration
		"task_type":   "migration",
		"source_lang": "java",
		"target_spec": "java:17",
		"recipe_ids":  recipeIDs, // Fixed: proper recipe format

		// Iteration control
		"max_iterations":        maxIterations,
		"timeout_per_iteration": 600000000000, // 10 minutes in nanoseconds
		"stop_on_success":       true,

		// Output configuration
		"output_dir":              fmt.Sprintf("/tmp/arf-benchmark-%s", benchmarkName),
		"capture_full_diffs":      true,
		"capture_partial_diffs":   false,
		"save_intermediate_state": true,

		// Deployment metadata
		"deployment_config": map[string]interface{}{
			"app_name": appName,
			"lane":     lane,
		},
	}

	// Add LLM provider configuration (required for benchmark suite)
	llmProvider := os.Getenv("ARF_LLM_PROVIDER")
	llmModel := os.Getenv("ARF_LLM_MODEL")

	// Set defaults if not configured
	if llmProvider == "" {
		llmProvider = "ollama"
	}
	if llmModel == "" {
		llmModel = "codellama:7b"
	}

	// Add LLM configuration to benchmark config
	benchmarkConfig["llm_provider"] = llmProvider
	benchmarkConfig["llm_model"] = llmModel
	benchmarkConfig["llm_options"] = map[string]string{
		"base_url":    "http://localhost:11434",
		"temperature": "0.1",
		"max_tokens":  "4096",
	}

	// Wrap config in the format controller expects
	request := map[string]interface{}{
		"config": benchmarkConfig,
	}

	// Submit benchmark
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize benchmark request: %w", err)
	}

	fmt.Printf("Starting benchmark '%s'...\n", benchmarkName)
	fmt.Printf("Repository: %s\n", repository)
	fmt.Printf("App Name: %s\n", appName)
	fmt.Printf("Lane: %s\n", lane)
	if transformations != "" {
		fmt.Printf("Transformations: %s\n", transformations)
	}
	fmt.Println()

	url := fmt.Sprintf("%s/arf/benchmark/run", arfControllerURL)

	// Debug: print request body
	if debugMode := os.Getenv("DEBUG_ARF"); debugMode != "" {
		fmt.Printf("DEBUG: Request URL: %s\n", url)
		fmt.Printf("DEBUG: Request body: %s\n", string(data))
	}

	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to start benchmark: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if benchmarkID, ok := result["benchmark_id"].(string); ok {
		fmt.Printf("✅ Benchmark started successfully!\n")
		fmt.Printf("Benchmark ID: %s\n", benchmarkID)
		fmt.Println()
		fmt.Printf("Use 'ploy arf benchmark status %s' to check progress\n", benchmarkID)
		fmt.Printf("Use 'ploy arf benchmark logs %s' to view logs\n", benchmarkID)
	} else {
		fmt.Println("✅ Benchmark started, but no ID returned")
	}

	return nil
}

func handleBenchmarkList(args []string) error {
	activeOnly := false
	completedOnly := false

	// Parse filters
	for _, arg := range args {
		switch arg {
		case "--active":
			activeOnly = true
		case "--completed":
			completedOnly = true
		}
	}

	url := fmt.Sprintf("%s/arf/benchmark/list", arfControllerURL)
	if activeOnly {
		url += "?status=running"
	} else if completedOnly {
		url += "?status=completed"
	}

	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to list benchmarks: %w", err)
	}

	var data struct {
		Benchmarks []struct {
			ID          string     `json:"id"`
			Name        string     `json:"name"`
			Status      string     `json:"status"`
			Repository  string     `json:"repository"`
			StartedAt   time.Time  `json:"started_at"`
			CompletedAt *time.Time `json:"completed_at"`
			AppName     string     `json:"app_name"`
		} `json:"benchmarks"`
		Count int `json:"count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if data.Count == 0 {
		fmt.Println("No benchmarks found")
		return nil
	}

	// Display benchmarks in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tREPOSITORY\tAPP NAME\tSTARTED\tDURATION")
	fmt.Fprintln(w, "--\t----\t------\t----------\t--------\t-------\t--------")

	for _, benchmark := range data.Benchmarks {
		started := benchmark.StartedAt.Format("15:04:05")
		duration := ""

		if benchmark.CompletedAt != nil {
			duration = benchmark.CompletedAt.Sub(benchmark.StartedAt).Round(time.Second).String()
		} else if benchmark.Status == "running" {
			duration = time.Since(benchmark.StartedAt).Round(time.Second).String()
		}

		// Truncate repository URL for display
		repo := benchmark.Repository
		if len(repo) > 40 {
			repo = repo[:37] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			benchmark.ID, benchmark.Name, benchmark.Status, repo, benchmark.AppName, started, duration)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d benchmarks\n", data.Count)
	return nil
}

func handleBenchmarkStatus(benchmarkID string) error {
	url := fmt.Sprintf("%s/arf/benchmark/status/%s", arfControllerURL, benchmarkID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get benchmark status: %w", err)
	}

	var status struct {
		ID                   string     `json:"id"`
		Name                 string     `json:"name"`
		Status               string     `json:"status"`
		CurrentStage         string     `json:"current_stage"`
		Progress             float64    `json:"progress"`
		StartedAt            time.Time  `json:"started_at"`
		CompletedAt          *time.Time `json:"completed_at"`
		Repository           string     `json:"repository"`
		AppName              string     `json:"app_name"`
		Lane                 string     `json:"lane"`
		TransformationsCount int        `json:"transformations_count"`
		SuccessfulStages     []string   `json:"successful_stages"`
		FailedStages         []string   `json:"failed_stages"`
	}

	if err := json.Unmarshal(response, &status); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Benchmark Status: %s\n", status.ID)
	fmt.Printf("Name: %s\n", status.Name)
	fmt.Printf("Status: %s\n", status.Status)

	if status.CurrentStage != "" {
		fmt.Printf("Current Stage: %s\n", status.CurrentStage)
	}

	fmt.Printf("Progress: %.1f%%\n", status.Progress*100)
	fmt.Printf("Repository: %s\n", status.Repository)
	fmt.Printf("App Name: %s\n", status.AppName)
	fmt.Printf("Lane: %s\n", status.Lane)
	fmt.Printf("Started: %s\n", status.StartedAt.Format("2006-01-02 15:04:05"))

	if status.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", status.CompletedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Duration: %s\n", status.CompletedAt.Sub(status.StartedAt).Round(time.Second))
	} else if status.Status == "running" {
		fmt.Printf("Running for: %s\n", time.Since(status.StartedAt).Round(time.Second))
	}

	if len(status.SuccessfulStages) > 0 {
		fmt.Printf("Completed Stages: %s\n", strings.Join(status.SuccessfulStages, ", "))
	}

	if len(status.FailedStages) > 0 {
		fmt.Printf("Failed Stages: %s\n", strings.Join(status.FailedStages, ", "))
	}

	return nil
}

func handleBenchmarkLogs(args []string) error {
	benchmarkID := args[0]
	stage := "all"

	// Parse stage filter
	for i := 1; i < len(args); i++ {
		if args[i] == "--stage" && i+1 < len(args) {
			stage = args[i+1]
			break
		}
	}

	url := fmt.Sprintf("%s/arf/benchmark/logs/%s", arfControllerURL, benchmarkID)
	if stage != "all" {
		url += "?stage=" + stage
	}

	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get benchmark logs: %w", err)
	}

	var logs struct {
		BenchmarkID string `json:"benchmark_id"`
		Stage       string `json:"stage"`
		Logs        []struct {
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level"`
			Stage     string    `json:"stage"`
			Message   string    `json:"message"`
		} `json:"logs"`
	}

	if err := json.Unmarshal(response, &logs); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Benchmark Logs: %s\n", logs.BenchmarkID)
	if logs.Stage != "" {
		fmt.Printf("Stage: %s\n", logs.Stage)
	}
	fmt.Println(strings.Repeat("=", 60))

	for _, log := range logs.Logs {
		timestamp := log.Timestamp.Format("15:04:05")
		fmt.Printf("[%s] [%s] [%s] %s\n", timestamp, log.Level, log.Stage, log.Message)
	}

	return nil
}

func handleBenchmarkStop(benchmarkID string) error {
	url := fmt.Sprintf("%s/arf/benchmark/stop/%s", arfControllerURL, benchmarkID)
	_, err := makeAPIRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to stop benchmark: %w", err)
	}

	fmt.Printf("Benchmark %s stopped successfully\n", benchmarkID)
	return nil
}