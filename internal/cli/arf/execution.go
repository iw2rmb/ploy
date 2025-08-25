package arf

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ExecutionRequest represents a recipe execution request
type ExecutionRequest struct {
	RecipeID     string            `json:"recipe_id"`
	Repository   string            `json:"repository"`
	Branch       string            `json:"branch,omitempty"`
	OutputDir    string            `json:"output_dir,omitempty"`
	Environment  map[string]string `json:"environment,omitempty"`
	WorkingDir   string            `json:"working_dir,omitempty"`
	Timeout      string            `json:"timeout,omitempty"`
	DryRun       bool              `json:"dry_run,omitempty"`
	GenerateReport bool            `json:"generate_report,omitempty"`
}

// ExecutionResult represents the result of a recipe execution
type ExecutionResult struct {
	ID           string            `json:"id"`
	RecipeID     string            `json:"recipe_id"`
	Repository   string            `json:"repository"`
	Branch       string            `json:"branch"`
	Status       string            `json:"status"` // pending, running, completed, failed
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time,omitempty"`
	Duration     string            `json:"duration,omitempty"`
	Output       string            `json:"output,omitempty"`
	Error        string            `json:"error,omitempty"`
	Changes      ExecutionChanges  `json:"changes"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	BenchmarkID  string            `json:"benchmark_id,omitempty"` // Link to benchmark system
	ReportURL    string            `json:"report_url,omitempty"`
}

// ExecutionChanges represents changes made during execution
type ExecutionChanges struct {
	FilesAdded    []string `json:"files_added"`
	FilesModified []string `json:"files_modified"`
	FilesDeleted  []string `json:"files_deleted"`
	LinesAdded    int      `json:"lines_added"`
	LinesDeleted  int      `json:"lines_deleted"`
	LinesModified int      `json:"lines_modified"`
}

// BenchmarkIntegrationRequest represents integration with benchmark system
type BenchmarkIntegrationRequest struct {
	Name         string            `json:"name"`
	RecipeID     string            `json:"recipe_id"`
	Repository   string            `json:"repository"`
	Branch       string            `json:"branch"`
	AppName      string            `json:"app_name"`
	Lane         string            `json:"lane,omitempty"`
	Iterations   int               `json:"iterations,omitempty"`
	Environment  map[string]string `json:"environment,omitempty"`
	TestSuite    bool              `json:"test_suite,omitempty"`
	DeployTest   bool              `json:"deploy_test,omitempty"`
}

// runRecipe executes a recipe against a repository
func runRecipe(recipeID string, flags CommandFlags) error {
	// Validate recipe ID
	if err := ValidateRecipeID(recipeID); err != nil {
		PrintError(err)
		return err
	}

	// Parse execution parameters from command line args
	repository := ""  // TODO: Extract from flags
	branch := "main"  // TODO: Extract from flags
	outputDir := ""   // TODO: Extract from flags
	workingDir := ""  // TODO: Extract from flags
	timeout := "15m"  // Default timeout
	
	// For now, use dummy values - need to implement proper flag parsing
	if repository == "" {
		repository = "." // Default to current directory
	}

	// Create execution request
	request := ExecutionRequest{
		RecipeID:      recipeID,
		Repository:    repository,
		Branch:        branch,
		OutputDir:     outputDir,
		WorkingDir:    workingDir,
		Timeout:       timeout,
		DryRun:        flags.DryRun,
		GenerateReport: flags.Verbose,
		Environment:   make(map[string]string),
	}

	// Validate request
	if err := validateExecutionRequest(request); err != nil {
		PrintError(err)
		return err
	}

	// Execute via benchmark system integration
	if shouldUseBenchmarkSystem(request) {
		return executeViaBenchmarkSystem(request, flags)
	}

	// Direct execution
	return executeRecipeDirect(request, flags)
}

// executeViaBenchmarkSystem executes recipe through the benchmark system
func executeViaBenchmarkSystem(request ExecutionRequest, flags CommandFlags) error {
	PrintInfo(fmt.Sprintf("Executing recipe '%s' via benchmark system", request.RecipeID))

	// Create benchmark request
	benchmarkReq := BenchmarkIntegrationRequest{
		Name:        fmt.Sprintf("recipe-%s-%d", request.RecipeID, time.Now().Unix()),
		RecipeID:    request.RecipeID,
		Repository:  request.Repository,
		Branch:      request.Branch,
		AppName:     fmt.Sprintf("recipe-test-%s", request.RecipeID),
		Iterations:  1, // Single execution for recipe run
		Environment: request.Environment,
		TestSuite:   true,  // Enable test suite execution
		DeployTest:  false, // Don't deploy unless requested
	}

	// Send to benchmark system
	payload, err := json.Marshal(benchmarkReq)
	if err != nil {
		return NewCLIError("Failed to serialize benchmark request", 1).WithCause(err)
	}

	url := fmt.Sprintf("%s/arf/benchmark/run", arfControllerURL)
	response, err := makeAPIRequest("POST", url, payload)
	if err != nil {
		return NewCLIError("Failed to start recipe execution via benchmark system", 1).
			WithCause(err).
			WithSuggestion("Check if the ARF benchmark system is available")
	}

	var result struct {
		BenchmarkID   string `json:"benchmark_id"`
		ExecutionID   string `json:"execution_id"`
		Status        string `json:"status"`
		Message       string `json:"message"`
		EstimatedTime string `json:"estimated_time,omitempty"`
	}

	if err := json.Unmarshal(response, &result); err != nil {
		return NewCLIError("Failed to parse benchmark response", 1).WithCause(err)
	}

	PrintSuccess(fmt.Sprintf("Recipe execution started via benchmark system"))
	fmt.Printf("Benchmark ID: %s\n", result.BenchmarkID)
	fmt.Printf("Execution ID: %s\n", result.ExecutionID)
	fmt.Printf("Status: %s\n", result.Status)
	
	if result.EstimatedTime != "" {
		fmt.Printf("Estimated time: %s\n", result.EstimatedTime)
	}

	// Monitor execution if requested
	if flags.Verbose {
		return monitorExecution(result.ExecutionID, flags)
	}

	// Show how to check status
	fmt.Printf("\nTo check execution status:\n")
	fmt.Printf("  ploy arf recipe status %s\n", result.ExecutionID)
	fmt.Printf("  ploy arf benchmark status %s\n", result.BenchmarkID)

	return nil
}

// executeRecipeDirect executes recipe directly without benchmark system
func executeRecipeDirect(request ExecutionRequest, flags CommandFlags) error {
	PrintInfo(fmt.Sprintf("Executing recipe '%s' directly", request.RecipeID))

	if request.DryRun {
		PrintInfo("Dry run mode: recipe would be executed with the following parameters:")
		fmt.Printf("  Recipe ID: %s\n", request.RecipeID)
		fmt.Printf("  Repository: %s\n", request.Repository)
		fmt.Printf("  Branch: %s\n", request.Branch)
		if request.OutputDir != "" {
			fmt.Printf("  Output Dir: %s\n", request.OutputDir)
		}
		if request.WorkingDir != "" {
			fmt.Printf("  Working Dir: %s\n", request.WorkingDir)
		}
		fmt.Printf("  Timeout: %s\n", request.Timeout)
		return nil
	}

	// Send execution request
	payload, err := json.Marshal(request)
	if err != nil {
		return NewCLIError("Failed to serialize execution request", 1).WithCause(err)
	}

	url := fmt.Sprintf("%s/arf/recipes/%s/execute", arfControllerURL, request.RecipeID)
	response, err := makeAPIRequest("POST", url, payload)
	if err != nil {
		return NewCLIError("Failed to execute recipe", 1).
			WithCause(err).
			WithSuggestion("Check if the recipe exists and repository is accessible")
	}

	var result ExecutionResult
	if err := json.Unmarshal(response, &result); err != nil {
		return NewCLIError("Failed to parse execution response", 1).WithCause(err)
	}

	// Display execution results
	return displayExecutionResult(result, flags)
}

// displayExecutionResult displays the result of a recipe execution
func displayExecutionResult(result ExecutionResult, flags CommandFlags) error {
	fmt.Printf("\nRecipe Execution Result\n")
	fmt.Printf("=======================\n")
	fmt.Printf("Execution ID: %s\n", result.ID)
	fmt.Printf("Recipe ID:    %s\n", result.RecipeID)
	fmt.Printf("Repository:   %s\n", result.Repository)
	fmt.Printf("Branch:       %s\n", result.Branch)
	fmt.Printf("Status:       %s\n", result.Status)

	if !result.StartTime.IsZero() {
		fmt.Printf("Started:      %s\n", result.StartTime.Format("2006-01-02 15:04:05"))
	}
	
	if !result.EndTime.IsZero() {
		fmt.Printf("Completed:    %s\n", result.EndTime.Format("2006-01-02 15:04:05"))
	}
	
	if result.Duration != "" {
		fmt.Printf("Duration:     %s\n", result.Duration)
	}

	// Display changes
	changes := result.Changes
	if len(changes.FilesAdded) > 0 || len(changes.FilesModified) > 0 || len(changes.FilesDeleted) > 0 {
		fmt.Printf("\nChanges Made:\n")
		if len(changes.FilesAdded) > 0 {
			fmt.Printf("  Files Added (%d):\n", len(changes.FilesAdded))
			for _, file := range changes.FilesAdded {
				fmt.Printf("    + %s\n", file)
			}
		}
		if len(changes.FilesModified) > 0 {
			fmt.Printf("  Files Modified (%d):\n", len(changes.FilesModified))
			for _, file := range changes.FilesModified {
				fmt.Printf("    ~ %s\n", file)
			}
		}
		if len(changes.FilesDeleted) > 0 {
			fmt.Printf("  Files Deleted (%d):\n", len(changes.FilesDeleted))
			for _, file := range changes.FilesDeleted {
				fmt.Printf("    - %s\n", file)
			}
		}
		
		fmt.Printf("  Lines: +%d ~%d -%d\n", 
			changes.LinesAdded, changes.LinesModified, changes.LinesDeleted)
	}

	// Show error if execution failed
	if result.Status == "failed" && result.Error != "" {
		fmt.Printf("\nError:\n%s\n", result.Error)
	}

	// Show output if verbose
	if flags.Verbose && result.Output != "" {
		fmt.Printf("\nOutput:\n%s\n", result.Output)
	}

	// Show report URL if available
	if result.ReportURL != "" {
		fmt.Printf("\nDetailed report: %s\n", result.ReportURL)
	}

	// Show benchmark link if available
	if result.BenchmarkID != "" {
		fmt.Printf("Benchmark ID: %s\n", result.BenchmarkID)
	}

	// Show success or failure message
	switch result.Status {
	case "completed":
		PrintSuccess("Recipe executed successfully!")
	case "failed":
		PrintError(NewCLIError("Recipe execution failed", 1))
		return fmt.Errorf("recipe execution failed")
	case "running":
		PrintInfo("Recipe is still running...")
	default:
		PrintInfo(fmt.Sprintf("Recipe status: %s", result.Status))
	}

	return nil
}

// monitorExecution monitors a running execution
func monitorExecution(executionID string, flags CommandFlags) error {
	PrintInfo(fmt.Sprintf("Monitoring execution %s...", executionID))
	
	// Poll for status updates
	for i := 0; i < 60; i++ { // Max 60 polls (5 minutes with 5s intervals)
		time.Sleep(5 * time.Second)
		
		status, err := getExecutionStatus(executionID)
		if err != nil {
			PrintWarning(fmt.Sprintf("Failed to get status: %v", err))
			continue
		}
		
		fmt.Printf("Status: %s", status.Status)
		if status.Duration != "" {
			fmt.Printf(" (duration: %s)", status.Duration)
		}
		fmt.Println()
		
		if status.Status == "completed" || status.Status == "failed" {
			return displayExecutionResult(status, flags)
		}
	}
	
	PrintWarning("Monitoring timeout. Execution may still be running.")
	fmt.Printf("Check status manually: ploy arf recipe status %s\n", executionID)
	return nil
}

// getExecutionStatus gets the current status of an execution
func getExecutionStatus(executionID string) (ExecutionResult, error) {
	url := fmt.Sprintf("%s/arf/executions/%s", arfControllerURL, executionID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return ExecutionResult{}, err
	}

	var result ExecutionResult
	err = json.Unmarshal(response, &result)
	return result, err
}

// shouldUseBenchmarkSystem determines if execution should use benchmark system
func shouldUseBenchmarkSystem(request ExecutionRequest) bool {
	// Use benchmark system if:
	// 1. Repository is a remote URL (not local directory)
	// 2. Generation report is requested
	// 3. Test deployment is needed
	
	return strings.HasPrefix(request.Repository, "http") || 
		   strings.HasPrefix(request.Repository, "git@") ||
		   request.GenerateReport
}

// validateExecutionRequest validates an execution request
func validateExecutionRequest(request ExecutionRequest) error {
	if request.RecipeID == "" {
		return NewCLIError("Recipe ID is required", 1)
	}

	if request.Repository == "" {
		return NewCLIError("Repository is required", 1)
	}

	// Validate repository format
	if !strings.HasPrefix(request.Repository, "http") && 
	   !strings.HasPrefix(request.Repository, "git@") && 
	   !strings.HasPrefix(request.Repository, "/") && 
	   request.Repository != "." {
		return NewCLIError("Invalid repository format", 1).
			WithSuggestion("Use HTTP(S) URL, SSH URL, absolute path, or '.' for current directory")
	}

	// Validate timeout format if specified
	if request.Timeout != "" {
		if _, err := time.ParseDuration(request.Timeout); err != nil {
			return NewCLIError(fmt.Sprintf("Invalid timeout format: %s", request.Timeout), 1).
				WithSuggestion("Use format like '15m', '1h', '30s'")
		}
	}

	// Validate output directory if specified
	if request.OutputDir != "" {
		if _, err := os.Stat(request.OutputDir); os.IsNotExist(err) {
			return NewCLIError(fmt.Sprintf("Output directory does not exist: %s", request.OutputDir), 1).
				WithSuggestion("Create the directory or use an existing one")
		}
	}

	return nil
}

// listExecutions lists recipe executions
func listExecutions(outputFormat string, verbose bool) error {
	url := fmt.Sprintf("%s/arf/executions", arfControllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return NewCLIError("Failed to retrieve executions", 1).
			WithCause(err).
			WithSuggestion("Check network connectivity and controller status")
	}

	var data struct {
		Executions []ExecutionResult `json:"executions"`
		Count      int               `json:"count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return NewCLIError("Failed to parse executions data", 1).WithCause(err)
	}

	if data.Count == 0 {
		PrintInfo("No recipe executions found")
		return nil
	}

	// Format output
	switch strings.ToLower(outputFormat) {
	case "json":
		output, _ := json.MarshalIndent(data.Executions, "", "  ")
		fmt.Println(string(output))
	default: // table
		fmt.Printf("Recent Recipe Executions:\n\n")
		for _, exec := range data.Executions {
			status := "✅"
			if exec.Status == "failed" {
				status = "❌"
			} else if exec.Status == "running" {
				status = "🔄"
			} else if exec.Status == "pending" {
				status = "⏳"
			}
			
			fmt.Printf("%s %s (%s)\n", status, exec.RecipeID, exec.ID)
			fmt.Printf("  Repository: %s\n", exec.Repository)
			fmt.Printf("  Status: %s", exec.Status)
			if exec.Duration != "" {
				fmt.Printf(" (%s)", exec.Duration)
			}
			fmt.Printf("\n")
			
			if verbose {
				fmt.Printf("  Started: %s\n", exec.StartTime.Format("2006-01-02 15:04:05"))
				if exec.BenchmarkID != "" {
					fmt.Printf("  Benchmark: %s\n", exec.BenchmarkID)
				}
			}
			fmt.Println()
		}
		fmt.Printf("Total: %d executions\n", data.Count)
	}

	return nil
}