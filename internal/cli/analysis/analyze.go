package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	amodels "github.com/iw2rmb/ploy/internal/analysis/models"
)

// AnalyzeCmd handles the analyze command
func AnalyzeCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		usage()
		return
	}

	switch args[0] {
	case "help", "--help", "-h":
		usage()
	case "status":
		if len(args) < 2 {
			fmt.Println("Error: analysis ID required")
			fmt.Println("Usage: ploy analyze status <analysis-id>")
			os.Exit(1)
		}
		getAnalysisStatus(args[1], controllerURL)
	case "results":
		if len(args) < 2 {
			fmt.Println("Error: analysis ID required")
			fmt.Println("Usage: ploy analyze results <analysis-id>")
			os.Exit(1)
		}
		getAnalysisResults(args[1], controllerURL)
	case "list":
		listAnalyses(args[1:], controllerURL)
	case "config":
		handleConfig(args[1:], controllerURL)
	case "report":
		generateReport(args[1:], controllerURL)
	case "languages":
		listSupportedLanguages(controllerURL)
	default:
		// Handle main analyze command with flags
		analyzeRepository(args, controllerURL)
	}
}

func usage() {
	fmt.Println(`Usage: ploy analyze [OPTIONS] [COMMAND]

Run static analysis on applications or repositories

Commands:
  status <id>           Check analysis progress
  results <id>          View detailed analysis results
  list                  List historical analyses
  config                Manage analysis configuration
  report                Generate analysis report
  languages             List supported languages

Options:
  --app <app>           Analyze specific deployed app
  --repository <path>   Analyze local repository
  --language <lang>     Language-specific analysis
  --config <file>       Use custom configuration file
  --fix                 Run with ARF auto-remediation
  --dry-run             Preview fixes without applying
  --format <format>     Output format (json, table, html)

Examples:
  ploy analyze --app myapp
  ploy analyze --app myapp --fix
  ploy analyze --repository ./path/to/code --language java
  ploy analyze status abc123
  ploy analyze results abc123 --format json
  ploy analyze list --app myapp
  ploy analyze config --show
  ploy analyze report --app myapp --timeframe 30d`)
	os.Exit(0)
}

func analyzeRepository(args []string, controllerURL string) {
	var (
		appName    string
		repository string
		language   string
		configFile string
		fix        bool
		dryRun     bool
		format     = "table"
	)

	// Parse command line arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--app", "-a":
			if i+1 < len(args) {
				appName = args[i+1]
				i++
			}
		case "--repository", "-r":
			if i+1 < len(args) {
				repository = args[i+1]
				i++
			}
		case "--language", "-l":
			if i+1 < len(args) {
				language = args[i+1]
				i++
			}
		case "--config", "-c":
			if i+1 < len(args) {
				configFile = args[i+1]
				i++
			}
		case "--fix", "-f":
			fix = true
		case "--dry-run":
			dryRun = true
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		}
	}

	if appName == "" && repository == "" {
		fmt.Println("Error: Either --app or --repository must be specified")
		usage()
		return
	}

	// Prepare request
	repoName := appName
	if repoName == "" && repository != "" {
		repoName = filepath.Base(repository)
	}

	req := amodels.AnalysisRequest{
		Repository: amodels.Repository{
			Name: repoName,
			URL:  repository, // Use URL field for local path
		},
		Config: amodels.AnalysisConfig{
			Enabled:        true,
			FailOnCritical: false,
			ARFIntegration: fix,
			Timeout:        30 * time.Minute,
		},
		FixIssues: fix,
		DryRun:    dryRun,
	}

	if language != "" {
		req.Config.Languages = map[string]interface{}{
			language: map[string]bool{"enabled": true},
		}
	}

	// Load custom configuration if provided
	if configFile != "" {
		// TODO: Load configuration from file
		fmt.Printf("Loading configuration from %s\n", configFile)
	}

	// Make API request
	fmt.Printf("Starting analysis for %s...\n", getTargetName(appName, repository))

	body, err := json.Marshal(req)
	if err != nil {
		fmt.Printf("Error: Failed to prepare request: %v\n", err)
		os.Exit(1)
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/analysis/analyze", controllerURL),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		fmt.Printf("Error: Failed to start analysis: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: Analysis failed with status %d: %s\n", resp.StatusCode, string(bodyBytes))
		os.Exit(1)
	}

	// Parse response
	var result amodels.AnalysisResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	// Display results
	displayAnalysisResult(&result, format)

	if fix && len(result.ARFTriggers) > 0 {
		fmt.Printf("\n%d issues queued for automatic remediation\n", len(result.ARFTriggers))
		if dryRun {
			fmt.Println("(dry-run mode - no changes will be applied)")
		}
	}
}

func getAnalysisStatus(analysisID, controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/analysis/results/%s", controllerURL, analysisID))
	if err != nil {
		fmt.Printf("Error: Failed to get analysis status: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("Analysis %s not found\n", analysisID)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Failed with status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var result amodels.AnalysisResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Analysis ID: %s\n", result.ID)
	fmt.Printf("Status: %s\n", getStatusString(result.Success, result.Error))
	fmt.Printf("Repository: %s\n", result.Repository.Name)
	fmt.Printf("Timestamp: %s\n", result.Timestamp.Format(time.RFC3339))
	fmt.Printf("Overall Score: %.1f/100\n", result.OverallScore)
	fmt.Printf("Total Issues: %d\n", len(result.Issues))

	if result.Metrics.AnalysisTime > 0 {
		fmt.Printf("Analysis Time: %v\n", result.Metrics.AnalysisTime)
	}
}

func getAnalysisResults(analysisID, controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/analysis/results/%s", controllerURL, analysisID))
	if err != nil {
		fmt.Printf("Error: Failed to get analysis results: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("Analysis %s not found\n", analysisID)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Failed with status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var result amodels.AnalysisResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	displayAnalysisResult(&result, "table")
}

func listAnalyses(args []string, controllerURL string) {
	var appName string
	limit := "10"

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--app", "-a":
			if i+1 < len(args) {
				appName = args[i+1]
				i++
			}
		case "--limit":
			if i+1 < len(args) {
				limit = args[i+1]
				i++
			}
		}
	}

	if appName == "" {
		fmt.Println("Error: --app is required")
		fmt.Println("Usage: ploy analyze list --app <appname> [--limit N]")
		os.Exit(1)
	}

	url := fmt.Sprintf("%s/analysis/results?repository_id=%s&limit=%s", controllerURL, appName, limit)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error: Failed to list analyses: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Failed with status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var response struct {
		Results []*amodels.AnalysisResult `json:"results"`
		Count   int                       `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	if response.Count == 0 {
		fmt.Printf("No analyses found for app: %s\n", appName)
		return
	}

	// Display results in table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTIMESTAMP\tSCORE\tISSUES\tSTATUS")
	fmt.Fprintln(w, strings.Repeat("-", 60))

	for _, result := range response.Results {
		status := "Success"
		if !result.Success {
			status = "Failed"
		}
		fmt.Fprintf(w, "%s\t%s\t%.1f\t%d\t%s\n",
			result.ID,
			result.Timestamp.Format("2006-01-02 15:04"),
			result.OverallScore,
			len(result.Issues),
			status,
		)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d analyses\n", response.Count)
}

func handleConfig(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Error: config subcommand required")
		fmt.Println("Usage: ploy analyze config [--show|--validate|--update]")
		os.Exit(1)
	}

	switch args[0] {
	case "--show", "show":
		showConfig(controllerURL)
	case "--validate", "validate":
		if len(args) < 2 {
			fmt.Println("Error: config file required")
			fmt.Println("Usage: ploy analyze config --validate <file>")
			os.Exit(1)
		}
		validateConfig(args[1], controllerURL)
	case "--update", "update":
		if len(args) < 2 {
			fmt.Println("Error: config file required")
			fmt.Println("Usage: ploy analyze config --update <file>")
			os.Exit(1)
		}
		updateConfig(args[1], controllerURL)
	default:
		fmt.Printf("Unknown config command: %s\n", args[0])
		os.Exit(1)
	}
}

func showConfig(controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/analysis/config", controllerURL))
	if err != nil {
		fmt.Printf("Error: Failed to get configuration: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Failed with status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var config amodels.AnalysisConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	// Display configuration
	configJSON, _ := json.MarshalIndent(config, "", "  ")
	fmt.Println(string(configJSON))
}

func validateConfig(configFile, controllerURL string) {
	// Read config file
	data, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Printf("Error: Failed to read config file: %v\n", err)
		os.Exit(1)
	}

	// Validate via API
	resp, err := http.Post(
		fmt.Sprintf("%s/analysis/config/validate", controllerURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		fmt.Printf("Error: Failed to validate configuration: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result struct {
		Valid bool   `json:"valid"`
		Error string `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	if result.Valid {
		fmt.Println("Configuration is valid")
	} else {
		fmt.Printf("Configuration is invalid: %s\n", result.Error)
		os.Exit(1)
	}
}

func updateConfig(configFile, controllerURL string) {
	// Read config file
	data, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Printf("Error: Failed to read config file: %v\n", err)
		os.Exit(1)
	}

	// Update via API
	req, _ := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/analysis/config", controllerURL),
		bytes.NewBuffer(data),
	)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: Failed to update configuration: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: Failed to update configuration: %s\n", string(bodyBytes))
		os.Exit(1)
	}

	fmt.Println("Configuration updated successfully")
}

func generateReport(args []string, controllerURL string) {
	var (
		appName    string
		analysisID string
		timeframe  string
		format     = "html"
	)

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--app", "-a":
			if i+1 < len(args) {
				appName = args[i+1]
				i++
			}
		case "--analysis-id":
			if i+1 < len(args) {
				analysisID = args[i+1]
				i++
			}
		case "--timeframe":
			if i+1 < len(args) {
				timeframe = args[i+1]
				i++
			}
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		}
	}

	if appName == "" && analysisID == "" {
		fmt.Println("Error: Either --app or --analysis-id required")
		fmt.Println("Usage: ploy analyze report [--app <app> | --analysis-id <id>] [--format html|json]")
		os.Exit(1)
	}

	// TODO: Implement report generation
	fmt.Printf("Generating %s report", format)
	if appName != "" {
		fmt.Printf(" for app: %s", appName)
	}
	if analysisID != "" {
		fmt.Printf(" for analysis: %s", analysisID)
	}
	if timeframe != "" {
		fmt.Printf(" (timeframe: %s)", timeframe)
	}
	fmt.Println()
	fmt.Println("Report generation not yet implemented")
}

func listSupportedLanguages(controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/analysis/languages", controllerURL))
	if err != nil {
		fmt.Printf("Error: Failed to get supported languages: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: Failed with status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var response struct {
		Languages []string `json:"languages"`
		Count     int      `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Supported Languages:")
	for _, lang := range response.Languages {
		fmt.Printf("  - %s\n", lang)
	}
	fmt.Printf("\nTotal: %d languages\n", response.Count)
}

// Helper functions

func getTargetName(appName, repository string) string {
	if appName != "" {
		return fmt.Sprintf("app '%s'", appName)
	}
	return fmt.Sprintf("repository '%s'", filepath.Base(repository))
}

func getStatusString(success bool, errorMsg string) string {
	if success {
		return "Completed"
	}
	if errorMsg != "" {
		return fmt.Sprintf("Failed: %s", errorMsg)
	}
	return "Failed"
}

func displayAnalysisResult(result *amodels.AnalysisResult, format string) {
	switch format {
	case "json":
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
	case "table":
		displayResultTable(result)
	default:
		displayResultTable(result)
	}
}

func displayResultTable(result *amodels.AnalysisResult) {
	fmt.Println("\n=== ANALYSIS RESULTS ===")
	fmt.Printf("ID: %s\n", result.ID)
	fmt.Printf("Repository: %s\n", result.Repository.Name)
	fmt.Printf("Timestamp: %s\n", result.Timestamp.Format(time.RFC3339))
	fmt.Printf("Overall Score: %.1f/100\n", result.OverallScore)

	// Display metrics
	if result.Metrics.AnalysisTime > 0 {
		fmt.Printf("\n=== METRICS ===\n")
		fmt.Printf("Analysis Time: %v\n", result.Metrics.AnalysisTime)
		fmt.Printf("Total Files: %d\n", result.Metrics.TotalFiles)
		fmt.Printf("Analyzed Files: %d\n", result.Metrics.AnalyzedFiles)
		fmt.Printf("Total Issues: %d\n", result.Metrics.TotalIssues)
	}

	// Display issues by severity
	if len(result.Metrics.IssuesBySeverity) > 0 {
		fmt.Printf("\n=== ISSUES BY SEVERITY ===\n")
		for severity, count := range result.Metrics.IssuesBySeverity {
			fmt.Printf("  %s: %d\n", severity, count)
		}
	}

	// Display language results
	if len(result.LanguageResults) > 0 {
		fmt.Printf("\n=== LANGUAGE ANALYSIS ===\n")
		for lang, langResult := range result.LanguageResults {
			fmt.Printf("\n%s (%s):\n", strings.Title(lang), langResult.Analyzer)
			fmt.Printf("  Issues: %d\n", len(langResult.Issues))
			if langResult.Success {
				fmt.Printf("  Status: Success\n")
			} else {
				fmt.Printf("  Status: Failed - %s\n", langResult.Error)
			}
		}
	}

	// Display top issues (first 10)
	if len(result.Issues) > 0 {
		fmt.Printf("\n=== TOP ISSUES ===\n")
		displayCount := 10
		if len(result.Issues) < displayCount {
			displayCount = len(result.Issues)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SEVERITY\tCATEGORY\tFILE\tLINE\tMESSAGE")
		fmt.Fprintln(w, strings.Repeat("-", 80))

		for i := 0; i < displayCount; i++ {
			issue := result.Issues[i]
			file := filepath.Base(issue.File)
			if len(file) > 20 {
				file = "..." + file[len(file)-17:]
			}
			message := issue.Message
			if len(message) > 40 {
				message = message[:37] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
				issue.Severity,
				issue.Category,
				file,
				issue.Line,
				message,
			)
		}
		w.Flush()

		if len(result.Issues) > displayCount {
			fmt.Printf("\n... and %d more issues\n", len(result.Issues)-displayCount)
		}
	}
}
