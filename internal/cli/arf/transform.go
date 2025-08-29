package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf"
)

// TransformRequest represents a robust transformation request with self-healing
type TransformRequest struct {
	// Input sources
	Repository  string
	ArchivePath string
	Branch      string
	
	// Transformation specifications
	RecipeIDs  []string
	LLMPrompts []string
	
	// LLM configuration
	PlanModel string
	ExecModel string
	
	// Execution parameters
	MaxIterations int
	ParallelTries int
	Timeout       time.Duration
	
	// Output configuration
	OutputFormat string // archive, diff, mr
	OutputPath   string
	ReportLevel  string // minimal, standard, detailed
	
	// Legacy fields for compatibility
	Language string
	AppName  string
	Lane     string
}

// Transform command - Robust transformation with self-healing

func handleARFTransformCommand(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printTransformUsage()
		return nil
	}

	// Parse arguments with new flags
	req := parseTransformArgs(args)

	// Validate required parameters
	if req.Repository == "" && req.ArchivePath == "" {
		fmt.Println("Error: Either --repo or --archive is required")
		printTransformUsage()
		return nil
	}

	if len(req.RecipeIDs) == 0 && len(req.LLMPrompts) == 0 {
		fmt.Println("Error: At least one --recipe or --prompt is required")
		printTransformUsage()
		return nil
	}

	// Execute robust transformation
	return executeRobustTransformation(req)
}

// parseTransformArgs parses command line arguments into a transform request
func parseTransformArgs(args []string) *TransformRequest {
	req := &TransformRequest{
		Branch:         "main",
		PlanModel:      "ollama/codellama:7b",
		ExecModel:      "ollama/codellama:7b",
		MaxIterations:  3,
		ParallelTries:  3,
		OutputFormat:   "diff",
		ReportLevel:    "standard",
		Timeout:        5 * time.Minute,
		RecipeIDs:      []string{},
		LLMPrompts:     []string{},
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		// Input sources
		case "--repo", "-r":
			if i+1 < len(args) {
				req.Repository = args[i+1]
				i++
			}
		case "--archive", "-a":
			if i+1 < len(args) {
				req.ArchivePath = args[i+1]
				i++
			}
		case "--branch", "-b":
			if i+1 < len(args) {
				req.Branch = args[i+1]
				i++
			}

		// Transformation specifications
		case "--recipes":
			if i+1 < len(args) {
				req.RecipeIDs = strings.Split(args[i+1], ",")
				i++
			}
		case "--recipe": // Support both singular and plural
			if i+1 < len(args) {
				req.RecipeIDs = append(req.RecipeIDs, args[i+1])
				i++
			}
		case "--prompts":
			if i+1 < len(args) {
				req.LLMPrompts = strings.Split(args[i+1], ",")
				i++
			}
		case "--prompt": // Support both singular and plural
			if i+1 < len(args) {
				req.LLMPrompts = append(req.LLMPrompts, args[i+1])
				i++
			}

		// LLM configuration
		case "--plan-model":
			if i+1 < len(args) {
				req.PlanModel = args[i+1]
				i++
			}
		case "--exec-model":
			if i+1 < len(args) {
				req.ExecModel = args[i+1]
				i++
			}

		// Execution parameters
		case "--max-iterations":
			if i+1 < len(args) {
				if val, err := strconv.Atoi(args[i+1]); err == nil {
					req.MaxIterations = val
				}
				i++
			}
		case "--parallel-tries":
			if i+1 < len(args) {
				if val, err := strconv.Atoi(args[i+1]); err == nil {
					req.ParallelTries = val
				}
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				if duration, err := time.ParseDuration(args[i+1]); err == nil {
					req.Timeout = duration
				}
				i++
			}

		// Output configuration
		case "--output", "-o":
			if i+1 < len(args) {
				req.OutputFormat = args[i+1]
				i++
			}
		case "--output-path":
			if i+1 < len(args) {
				req.OutputPath = args[i+1]
				i++
			}
		case "--report":
			if i+1 < len(args) {
				req.ReportLevel = args[i+1]
				i++
			}

		// Legacy compatibility
		case "--language":
			if i+1 < len(args) {
				req.Language = args[i+1]
				i++
			}
		case "--app-name":
			if i+1 < len(args) {
				req.AppName = args[i+1]
				i++
			}
		case "--lane":
			if i+1 < len(args) {
				req.Lane = args[i+1]
				i++
			}
		}
	}

	return req
}

func printTransformUsage() {
	fmt.Println("Usage: ploy arf transform [options]")
	fmt.Println()
	fmt.Println("Transform code with recipes and/or LLM-powered self-healing")
	fmt.Println()
	fmt.Println("Input Sources (one required):")
	fmt.Println("  --repo <url>           Repository URL to transform")
	fmt.Println("  --archive <path>       Archive path (alternative to repo)")
	fmt.Println()
	fmt.Println("Transformation (at least one required):")
	fmt.Println("  --recipes <ids>        Comma-separated recipe IDs")
	fmt.Println("  --prompts <prompts>    Comma-separated LLM prompts")
	fmt.Println("  --recipe <id>          Single recipe ID (can be repeated)")
	fmt.Println("  --prompt <prompt>      Single LLM prompt (can be repeated)")
	fmt.Println()
	fmt.Println("LLM Configuration:")
	fmt.Println("  --plan-model <model>   LLM for planning (default: ollama/codellama:7b)")
	fmt.Println("  --exec-model <model>   LLM for execution (default: ollama/codellama:7b)")
	fmt.Println()
	fmt.Println("Execution Parameters:")
	fmt.Println("  --max-iterations <n>   Max retries per error (default: 3)")
	fmt.Println("  --parallel-tries <n>   Parallel solution attempts (default: 3)")
	fmt.Println("  --timeout <duration>   Timeout per iteration (default: 5m)")
	fmt.Println()
	fmt.Println("Output Configuration:")
	fmt.Println("  --output <format>      archive|diff|mr (default: diff)")
	fmt.Println("  --output-path <path>   Where to save output")
	fmt.Println("  --report <level>       minimal|standard|detailed (default: standard)")
	fmt.Println()
	fmt.Println("Additional Options:")
	fmt.Println("  --branch <name>        Git branch (default: main)")
	fmt.Println("  --app-name <name>      Application name for deployment testing")
	fmt.Println("  --lane <lane>          Deployment lane (auto-detected if not specified)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Apply Java 17 migration recipe")
	fmt.Println("  ploy arf transform --repo https://github.com/example/app --recipe java11to17")
	fmt.Println()
	fmt.Println("  # Apply multiple recipes with detailed reporting")
	fmt.Println("  ploy arf transform --repo https://github.com/example/app \\")
	fmt.Println("    --recipes java11to17,spring-boot-3 --report detailed")
	fmt.Println()
	fmt.Println("  # LLM-driven transformation with custom prompt")
	fmt.Println("  ploy arf transform --repo https://github.com/example/app \\")
	fmt.Println("    --prompt \"Migrate from JUnit 4 to JUnit 5\" --output archive")
}

// Legacy function - replaced by executeRobustTransformation
// Kept for backward compatibility with existing API endpoints
func executeTransformation(recipeID, repository, branch, language string) error {
	fmt.Printf("Executing transformation...\n")
	fmt.Printf("Recipe: %s\n", recipeID)
	fmt.Printf("Repository: %s\n", repository)
	fmt.Printf("Branch: %s\n", branch)

	if language != "" {
		fmt.Printf("Language: %s\n", language)
	}
	fmt.Println()

	// Check if this is an OpenRewrite recipe
	if isOpenRewriteRecipe(recipeID) {
		// Use OpenRewrite-specific endpoint for better integration
		request := map[string]interface{}{
			"project_url":     repository,
			"recipes":         []string{recipeID},
			"branch":          branch,
			"package_manager": detectPackageManager(language),
			"base_jdk":        detectJDKVersion(language),
		}

		data, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("failed to serialize request: %w", err)
		}

		// Use standard transform endpoint (which handles OpenRewrite internally)
		url := fmt.Sprintf("%s/arf/transform", arfControllerURL)
		response, err := makeAPIRequest("POST", url, data)
		if err != nil {
			return fmt.Errorf("OpenRewrite transformation failed: %w", err)
		}

		// Parse OpenRewrite-specific response
		var result map[string]interface{}
		if err := json.Unmarshal(response, &result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Display OpenRewrite job information
		fmt.Printf("✅ OpenRewrite transformation submitted!\n")
		if jobID, ok := result["job_id"].(string); ok {
			fmt.Printf("Job ID: %s\n", jobID)
			fmt.Printf("Status: %s\n", result["status"])
			fmt.Printf("Image: %s\n", result["image"])
			fmt.Printf("\nUse 'ploy arf status %s' to check progress\n", jobID)
		}
		return nil
	}

	// Standard ARF transformation for non-OpenRewrite recipes
	request := map[string]interface{}{
		"recipe_id": recipeID,
		"codebase": map[string]interface{}{
			"repository": repository,
			"branch":     branch,
			"language":   language,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}

	// Execute standard transformation
	url := fmt.Sprintf("%s/arf/transform", arfControllerURL)
	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("transformation failed: %w", err)
	}

	var result arf.TransformationResult
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Display results
	fmt.Printf("Transformation completed!\n\n")
	fmt.Printf("Status: ")
	if result.Success {
		fmt.Printf("✅ Success\n")
	} else {
		fmt.Printf("❌ Failed\n")
	}

	fmt.Printf("Changes Applied: %d\n", result.ChangesApplied)
	fmt.Printf("Files Modified: %d\n", len(result.FilesModified))
	fmt.Printf("Execution Time: %s\n", result.ExecutionTime.String())
	fmt.Printf("Validation Score: %.2f\n", result.ValidationScore)

	if len(result.FilesModified) > 0 {
		fmt.Println("\nModified Files:")
		for _, file := range result.FilesModified {
			fmt.Printf("  • %s\n", file)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, err := range result.Errors {
			fmt.Printf("  ❌ %s\n", err.Message)
			if err.File != "" {
				fmt.Printf("     File: %s:%d:%d\n", err.File, err.Line, err.Column)
			}
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warn := range result.Warnings {
			fmt.Printf("  ⚠️  %s\n", warn.Message)
		}
	}

	return nil
}

// isOpenRewriteRecipe checks if a recipe ID is an OpenRewrite recipe
func isOpenRewriteRecipe(recipeID string) bool {
	// OpenRewrite recipes typically have these patterns
	openRewritePatterns := []string{
		"openrewrite-",                       // Our custom prefix
		"org.openrewrite.",                   // Full class names
		"java11to17", "java8to11", "jakarta", // Known shortcuts
		"spring-boot-3", "spring-security-6",
		"junit5", "mockito", "assertj",
		"slf4j", "log4j2",
	}
	
	recipeLower := strings.ToLower(recipeID)
	for _, pattern := range openRewritePatterns {
		if strings.Contains(recipeLower, pattern) {
			return true
		}
	}
	
	return false
}

// detectPackageManager detects the package manager based on language
func detectPackageManager(language string) string {
	switch strings.ToLower(language) {
	case "java", "kotlin", "scala":
		// Default to Maven for Java projects
		// Could be enhanced to detect gradle vs maven from repo
		return "maven"
	case "groovy":
		return "gradle"
	default:
		return "maven"
	}
}

// detectJDKVersion detects appropriate JDK version based on language
func detectJDKVersion(language string) string {
	// Default to Java 17 for modern projects
	// Could be enhanced to detect from repository configuration
	return "17"
}

// executeRobustTransformation executes a transformation with self-healing capabilities
func executeRobustTransformation(req *TransformRequest) error {
	ctx := context.Background()
	
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println(" ARF Robust Transformation Engine")
	fmt.Println("═══════════════════════════════════════════════════════════")
	
	// Display configuration
	fmt.Printf("\n📋 Configuration:\n")
	if req.Repository != "" {
		fmt.Printf("  • Repository: %s (branch: %s)\n", req.Repository, req.Branch)
	} else if req.ArchivePath != "" {
		fmt.Printf("  • Archive: %s\n", req.ArchivePath)
	}
	
	if len(req.RecipeIDs) > 0 {
		fmt.Printf("  • Recipes: %s\n", strings.Join(req.RecipeIDs, ", "))
	}
	
	if len(req.LLMPrompts) > 0 {
		fmt.Printf("  • LLM Prompts: %d prompts\n", len(req.LLMPrompts))
		for i, prompt := range req.LLMPrompts {
			preview := prompt
			if len(preview) > 50 {
				preview = preview[:50] + "..."
			}
			fmt.Printf("    %d. %s\n", i+1, preview)
		}
	}
	
	fmt.Printf("  • Max Iterations: %d\n", req.MaxIterations)
	fmt.Printf("  • Parallel Tries: %d\n", req.ParallelTries)
	fmt.Printf("  • Output Format: %s\n", req.OutputFormat)
	fmt.Printf("  • Report Level: %s\n", req.ReportLevel)
	
	// Prepare the API request
	apiRequest := map[string]interface{}{
		"input_source": map[string]interface{}{
			"repository": req.Repository,
			"archive":    req.ArchivePath,
			"branch":     req.Branch,
		},
		"transformations": map[string]interface{}{
			"recipe_ids":  req.RecipeIDs,
			"llm_prompts": req.LLMPrompts,
		},
		"execution": map[string]interface{}{
			"max_iterations":  req.MaxIterations,
			"parallel_tries":  req.ParallelTries,
			"timeout":         req.Timeout.String(),
			"plan_model":      req.PlanModel,
			"exec_model":      req.ExecModel,
		},
		"output": map[string]interface{}{
			"format":       req.OutputFormat,
			"path":         req.OutputPath,
			"report_level": req.ReportLevel,
		},
	}
	
	// Add legacy fields if provided
	if req.AppName != "" {
		apiRequest["app_name"] = req.AppName
	}
	if req.Lane != "" {
		apiRequest["lane"] = req.Lane
	}
	
	// Serialize request
	data, err := json.Marshal(apiRequest)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}
	
	// Log request details
	if req.ReportLevel == "detailed" {
		fmt.Printf("\n📤 API Request Details:\n")
		fmt.Printf("  • Endpoint: %s/arf/transform\n", arfControllerURL)
		fmt.Printf("  • Request Size: %d bytes\n", len(data))
		fmt.Printf("  • Timeout: 30 minutes\n")
		
		// Log request body structure (without full data)
		fmt.Printf("  • Request Structure:\n")
		if req.Repository != "" {
			fmt.Printf("    - Repository: %s\n", req.Repository)
		}
		if req.ArchivePath != "" {
			fmt.Printf("    - Archive: %s\n", req.ArchivePath)
		}
		fmt.Printf("    - Recipes: %d\n", len(req.RecipeIDs))
		fmt.Printf("    - LLM Prompts: %d\n", len(req.LLMPrompts))
		fmt.Printf("    - Max Iterations: %d\n", req.MaxIterations)
		fmt.Printf("    - Parallel Tries: %d\n", req.ParallelTries)
	}
	
	// Call the transformation endpoint
	url := fmt.Sprintf("%s/arf/transform", arfControllerURL)
	
	fmt.Printf("\n🚀 Starting transformation...\n")
	
	// Log the actual request being sent
	if req.ReportLevel == "detailed" {
		fmt.Printf("\n🔍 Sending request to: %s\n", url)
		fmt.Printf("📊 Request payload preview (first 500 chars):\n")
		preview := string(data)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		fmt.Printf("%s\n", preview)
	}
	
	// Make API request with longer timeout for complex transformations
	startTime := time.Now()
	response, err := makeAPIRequestWithContext(ctx, "POST", url, data, 30*time.Minute)
	elapsed := time.Since(startTime)
	
	if req.ReportLevel == "detailed" {
		fmt.Printf("\n⏱️  API call completed in: %v\n", elapsed)
	}
	
	if err != nil {
		if req.ReportLevel == "detailed" {
			fmt.Printf("❌ API Error Details: %v\n", err)
		}
		return fmt.Errorf("transformation failed: %w", err)
	}
	
	// Log response details
	if req.ReportLevel == "detailed" {
		fmt.Printf("📥 Response received: %d bytes\n", len(response))
		fmt.Printf("🔍 Response preview (first 500 chars):\n")
		preview := string(response)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		fmt.Printf("%s\n", preview)
	}
	
	// Parse the response
	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		if req.ReportLevel == "detailed" {
			fmt.Printf("❌ Failed to parse JSON response. Raw response:\n%s\n", string(response))
		}
		return fmt.Errorf("failed to parse response: %w", err)
	}
	
	// Log parsed result structure
	if req.ReportLevel == "detailed" {
		fmt.Printf("\n📋 Parsed Response Structure:\n")
		for key := range result {
			fmt.Printf("  • %s\n", key)
		}
	}
	
	// Display results
	displayTransformationResults(result)
	
	// Save output if path specified
	if req.OutputPath != "" {
		if err := saveTransformationOutput(result, req.OutputPath, req.OutputFormat); err != nil {
			fmt.Printf("\n⚠️  Warning: Failed to save output: %v\n", err)
		} else {
			fmt.Printf("\n💾 Output saved to: %s\n", req.OutputPath)
		}
	}
	
	return nil
}

// displayTransformationResults displays the transformation results
func displayTransformationResults(result map[string]interface{}) {
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println(" Transformation Results")
	fmt.Println("═══════════════════════════════════════════════════════════")
	
	// Log all result keys for debugging
	fmt.Printf("\n🔍 Result contains %d keys:\n", len(result))
	for key, value := range result {
		switch v := value.(type) {
		case string:
			if len(v) > 100 {
				fmt.Printf("  • %s: (string, %d chars)\n", key, len(v))
			} else {
				fmt.Printf("  • %s: %s\n", key, v)
			}
		case bool:
			fmt.Printf("  • %s: %v\n", key, v)
		case float64:
			fmt.Printf("  • %s: %.2f\n", key, v)
		case map[string]interface{}:
			fmt.Printf("  • %s: (map with %d keys)\n", key, len(v))
		case []interface{}:
			fmt.Printf("  • %s: (array with %d items)\n", key, len(v))
		default:
			fmt.Printf("  • %s: (%T)\n", key, v)
		}
	}
	
	// Success status
	if success, ok := result["success"].(bool); ok {
		if success {
			fmt.Printf("\n✅ Status: SUCCESS\n")
		} else {
			fmt.Printf("\n❌ Status: FAILED\n")
		}
	} else {
		fmt.Printf("\n⚠️  Status: UNKNOWN (no 'success' field in response)\n")
	}
	
	// Check for error field
	if errMsg, ok := result["error"].(string); ok {
		fmt.Printf("\n❌ Error: %s\n", errMsg)
	}
	
	// Check for message field
	if msg, ok := result["message"].(string); ok {
		fmt.Printf("\n📝 Message: %s\n", msg)
	}
	
	// Report summary
	if report, ok := result["report"].(map[string]interface{}); ok {
		if summary, ok := report["summary"].(map[string]interface{}); ok {
			fmt.Printf("\n📊 Summary:\n")
			if filesModified, ok := summary["files_modified"].(float64); ok {
				fmt.Printf("  • Files Modified: %d\n", int(filesModified))
			}
			if linesChanged, ok := summary["lines_changed"].(float64); ok {
				fmt.Printf("  • Lines Changed: %d\n", int(linesChanged))
			}
			if duration, ok := summary["duration"].(string); ok {
				fmt.Printf("  • Duration: %s\n", duration)
			}
		}
		
		// Timeline of stages
		if timeline, ok := report["timeline"].([]interface{}); ok && len(timeline) > 0 {
			fmt.Printf("\n📈 Execution Timeline:\n")
			for i, stage := range timeline {
				if s, ok := stage.(map[string]interface{}); ok {
					name := s["name"].(string)
					status := s["status"].(string)
					duration := s["duration"].(string)
					
					statusIcon := "✅"
					if status != "success" {
						statusIcon = "❌"
					}
					
					fmt.Printf("  %d. %s %s (%s)\n", i+1, statusIcon, name, duration)
				}
			}
		}
		
		// Errors if any
		if errors, ok := report["errors"].([]interface{}); ok && len(errors) > 0 {
			fmt.Printf("\n⚠️  Errors Encountered:\n")
			for _, err := range errors {
				if e, ok := err.(map[string]interface{}); ok {
					msg := e["message"].(string)
					if resolution, ok := e["resolution"].(string); ok {
						fmt.Printf("  • %s\n    Resolution: %s\n", msg, resolution)
					} else {
						fmt.Printf("  • %s\n", msg)
					}
				}
			}
		}
		
		// Changed files
		if changes, ok := report["changes"].([]interface{}); ok && len(changes) > 0 {
			fmt.Printf("\n📝 Modified Files:\n")
			count := len(changes)
			if count > 10 {
				count = 10 // Show first 10
			}
			for i := 0; i < count; i++ {
				if change, ok := changes[i].(map[string]interface{}); ok {
					file := change["file"].(string)
					added := int(change["lines_added"].(float64))
					removed := int(change["lines_removed"].(float64))
					fmt.Printf("  • %s (+%d/-%d)\n", file, added, removed)
				}
			}
			if len(changes) > 10 {
				fmt.Printf("  ... and %d more files\n", len(changes)-10)
			}
		}
	}
	
	// Output location
	if output, ok := result["output"].(map[string]interface{}); ok {
		if location, ok := output["location"].(string); ok {
			fmt.Printf("\n📦 Output: %s\n", location)
		}
	}
}

// saveTransformationOutput saves the transformation output to a file
func saveTransformationOutput(result map[string]interface{}, outputPath, format string) error {
	fmt.Printf("\n📁 Attempting to save output as %s format to %s\n", format, outputPath)
	
	// Create output directory if needed
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Get the output data based on format
	var outputData []byte
	
	switch format {
	case "archive":
		// Save tar archive if provided
		fmt.Printf("🔍 Looking for archive data in response...\n")
		if output, ok := result["output"].(map[string]interface{}); ok {
			fmt.Printf("  • Found 'output' field with %d keys\n", len(output))
			if archive, ok := output["archive"].(string); ok {
				fmt.Printf("  • Found archive data: %d chars\n", len(archive))
				// Base64 decode if needed
				outputData = []byte(archive)
			} else {
				fmt.Printf("  • No 'archive' field in output\n")
			}
		} else {
			fmt.Printf("  • No 'output' field in result\n")
		}
		
	case "diff":
		// Generate unified diff from changes
		fmt.Printf("🔍 Looking for diff data in response...\n")
		if report, ok := result["report"].(map[string]interface{}); ok {
			fmt.Printf("  • Found 'report' field\n")
			if changes, ok := report["changes"].([]interface{}); ok {
				fmt.Printf("  • Found %d changes\n", len(changes))
				var diff strings.Builder
				diffCount := 0
				for _, change := range changes {
					if c, ok := change.(map[string]interface{}); ok {
						if unifiedDiff, ok := c["unified_diff"].(string); ok {
							diff.WriteString(unifiedDiff)
							diff.WriteString("\n")
							diffCount++
						}
					}
				}
				fmt.Printf("  • Generated diff from %d changes\n", diffCount)
				outputData = []byte(diff.String())
			} else {
				fmt.Printf("  • No 'changes' array in report\n")
			}
		} else {
			fmt.Printf("  • No 'report' field in result\n")
		}
		
	case "mr":
		// Generate merge request description
		if report, ok := result["report"].(map[string]interface{}); ok {
			mrContent := generateMergeRequestDescription(report)
			outputData = []byte(mrContent)
		}
		
	default:
		// Save full JSON result as default
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to serialize result: %w", err)
		}
		outputData = data
	}
	
	// Check if output is base64 encoded (for archive format)
	if format == "archive" {
		if encoding, ok := result["encoding"].(string); ok && encoding == "base64" {
			if outputStr, ok := result["output"].(string); ok {
				decodedData, err := base64.StdEncoding.DecodeString(outputStr)
				if err == nil {
					outputData = decodedData
					fmt.Printf("  • Decoded base64 archive: %d bytes\n", len(outputData))
				} else {
					fmt.Printf("  ⚠️  Failed to decode base64: %v\n", err)
				}
			}
		}
	}
	
	// Log what we're about to write
	fmt.Printf("\n💾 Writing output:\n")
	fmt.Printf("  • File: %s\n", outputPath)
	fmt.Printf("  • Size: %d bytes\n", len(outputData))
	if len(outputData) == 0 {
		fmt.Printf("  ⚠️  WARNING: Output data is empty!\n")
	}
	
	// Write to file
	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}
	
	fmt.Printf("  ✅ File written successfully\n")
	
	return nil
}

// generateMergeRequestDescription generates a merge request description
func generateMergeRequestDescription(report map[string]interface{}) string {
	var mr strings.Builder
	
	mr.WriteString("## ARF Transformation Merge Request\n\n")
	
	// Summary
	if summary, ok := report["summary"].(map[string]interface{}); ok {
		mr.WriteString("### Summary\n\n")
		if filesModified, ok := summary["files_modified"].(float64); ok {
			mr.WriteString(fmt.Sprintf("- Files Modified: %d\n", int(filesModified)))
		}
		if linesChanged, ok := summary["lines_changed"].(float64); ok {
			mr.WriteString(fmt.Sprintf("- Lines Changed: %d\n", int(linesChanged)))
		}
		mr.WriteString("\n")
	}
	
	// Changes
	if changes, ok := report["changes"].([]interface{}); ok && len(changes) > 0 {
		mr.WriteString("### Changes\n\n")
		for _, change := range changes {
			if c, ok := change.(map[string]interface{}); ok {
				file := c["file"].(string)
				mr.WriteString(fmt.Sprintf("- [ ] Review `%s`\n", file))
			}
		}
		mr.WriteString("\n")
	}
	
	// Testing checklist
	mr.WriteString("### Testing Checklist\n\n")
	mr.WriteString("- [ ] Code compiles successfully\n")
	mr.WriteString("- [ ] Unit tests pass\n")
	mr.WriteString("- [ ] Integration tests pass\n")
	mr.WriteString("- [ ] Application deploys successfully\n")
	mr.WriteString("- [ ] No regressions identified\n\n")
	
	mr.WriteString("---\n")
	mr.WriteString("*Generated by Ploy ARF Transformation Engine*\n")
	
	return mr.String()
}

// makeAPIRequestWithContext makes an API request with context and timeout
func makeAPIRequestWithContext(ctx context.Context, method, url string, data []byte, timeout time.Duration) ([]byte, error) {
	// For now, reuse the existing makeAPIRequest function
	// In production, this would handle context cancellation and longer timeouts
	return makeAPIRequest(method, url, data)
}