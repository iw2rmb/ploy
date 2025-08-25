package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// Config holds the CLI configuration
type Config struct {
	ControllerURL string
	Timeout       time.Duration
	Verbose       bool
}

// Handler handles analysis CLI commands
type Handler struct {
	config *Config
	client *http.Client
}

// NewHandler creates a new analysis CLI handler
func NewHandler(config *Config) *Handler {
	return &Handler{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GetCommands returns all analysis CLI commands
func (h *Handler) GetCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Static code analysis operations",
		Long:  "Perform static code analysis with automatic issue detection and remediation",
	}

	// Add subcommands
	cmd.AddCommand(h.analyzeCommand())
	cmd.AddCommand(h.languagesCommand())
	cmd.AddCommand(h.configCommand())
	cmd.AddCommand(h.resultsCommand())
	cmd.AddCommand(h.fixCommand())

	return cmd
}

// analyzeCommand creates the analyze command
func (h *Handler) analyzeCommand() *cobra.Command {
	var (
		appName    string
		language   string
		fix        bool
		dryRun     bool
		maxIssues  int
		failOnCrit bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run static analysis on an application",
		Long:  "Analyze code for bugs, security issues, and code quality problems",
		Example: `  ploy analyze run --app myapp
  ploy analyze run --app myapp --fix
  ploy analyze run --app myapp --language java
  ploy analyze run --app myapp --max-issues 100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if appName == "" {
				return fmt.Errorf("application name is required")
			}

			// Prepare analysis request
			request := map[string]interface{}{
				"repository": map[string]interface{}{
					"name": appName,
					"id":   appName,
				},
				"config": map[string]interface{}{
					"enabled":           true,
					"fail_on_critical":  failOnCrit,
					"arf_integration":   fix,
					"max_issues":        maxIssues,
				},
				"fix_issues": fix,
				"dry_run":    dryRun,
			}

			if language != "" {
				request["languages"] = []string{language}
			}

			// Send analysis request
			result, err := h.sendRequest("POST", "/v1/analysis/analyze", request)
			if err != nil {
				return fmt.Errorf("analysis failed: %w", err)
			}

			// Display results
			h.displayAnalysisResult(result)

			// Check if we should fail the command
			if failOnCrit {
				if issues, ok := result["issues"].([]interface{}); ok {
					for _, issue := range issues {
						if issueMap, ok := issue.(map[string]interface{}); ok {
							if severity, ok := issueMap["severity"].(string); ok && severity == "critical" {
								return fmt.Errorf("critical issues found")
							}
						}
					}
				}
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&appName, "app", "a", "", "Application name (required)")
	cmd.Flags().StringVarP(&language, "language", "l", "", "Specific language to analyze")
	cmd.Flags().BoolVarP(&fix, "fix", "f", false, "Automatically fix issues using ARF")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be fixed without applying changes")
	cmd.Flags().IntVar(&maxIssues, "max-issues", 1000, "Maximum number of issues to report")
	cmd.Flags().BoolVar(&failOnCrit, "fail-on-critical", true, "Exit with error if critical issues found")

	return cmd
}

// languagesCommand creates the languages command
func (h *Handler) languagesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "languages",
		Short: "List supported programming languages",
		Long:  "Display all programming languages supported by the static analysis engine",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := h.sendRequest("GET", "/v1/analysis/languages", nil)
			if err != nil {
				return fmt.Errorf("failed to get languages: %w", err)
			}

			h.displayLanguages(result)
			return nil
		},
	}

	return cmd
}

// configCommand creates the config command
func (h *Handler) configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage analysis configuration",
		Long:  "View and update static analysis configuration",
	}

	// Add subcommands
	cmd.AddCommand(h.configGetCommand())
	cmd.AddCommand(h.configSetCommand())
	cmd.AddCommand(h.configValidateCommand())

	return cmd
}

// configGetCommand creates the config get command
func (h *Handler) configGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get current analysis configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := h.sendRequest("GET", "/v1/analysis/config", nil)
			if err != nil {
				return fmt.Errorf("failed to get configuration: %w", err)
			}

			// Pretty print configuration
			jsonBytes, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
		},
	}
}

// configSetCommand creates the config set command
func (h *Handler) configSetCommand() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update analysis configuration",
		Example: `  ploy analyze config set --file config.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configFile == "" {
				return fmt.Errorf("configuration file is required")
			}

			// Read configuration file
			data, err := os.ReadFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to read config file: %w", err)
			}

			// Parse as JSON (could also support YAML)
			var config map[string]interface{}
			if err := json.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("failed to parse config: %w", err)
			}

			// Send update request
			result, err := h.sendRequest("PUT", "/v1/analysis/config", config)
			if err != nil {
				return fmt.Errorf("failed to update configuration: %w", err)
			}

			fmt.Println(color.GreenString("✓ Configuration updated successfully"))
			if message, ok := result["message"].(string); ok {
				fmt.Println(message)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "file", "f", "", "Configuration file path (required)")
	return cmd
}

// configValidateCommand creates the config validate command
func (h *Handler) configValidateCommand() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate analysis configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configFile == "" {
				return fmt.Errorf("configuration file is required")
			}

			// Read configuration file
			data, err := os.ReadFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to read config file: %w", err)
			}

			// Parse as JSON
			var config map[string]interface{}
			if err := json.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("failed to parse config: %w", err)
			}

			// Send validation request
			result, err := h.sendRequest("POST", "/v1/analysis/config/validate", config)
			if err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			if valid, ok := result["valid"].(bool); ok && valid {
				fmt.Println(color.GreenString("✓ Configuration is valid"))
			} else {
				fmt.Println(color.RedString("✗ Configuration is invalid"))
				if errorMsg, ok := result["error"].(string); ok {
					fmt.Printf("  Error: %s\n", errorMsg)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "file", "f", "", "Configuration file path (required)")
	return cmd
}

// resultsCommand creates the results command
func (h *Handler) resultsCommand() *cobra.Command {
	var (
		resultID string
		repoID   string
		limit    int
	)

	cmd := &cobra.Command{
		Use:   "results",
		Short: "View analysis results",
		Long:  "Retrieve and display previous analysis results",
		RunE: func(cmd *cobra.Command, args []string) error {
			if resultID != "" {
				// Get specific result
				result, err := h.sendRequest("GET", fmt.Sprintf("/v1/analysis/results/%s", resultID), nil)
				if err != nil {
					return fmt.Errorf("failed to get result: %w", err)
				}
				h.displayAnalysisResult(result)
			} else if repoID != "" {
				// List results for repository
				url := fmt.Sprintf("/v1/analysis/results?repository_id=%s&limit=%d", repoID, limit)
				result, err := h.sendRequest("GET", url, nil)
				if err != nil {
					return fmt.Errorf("failed to list results: %w", err)
				}
				h.displayResultsList(result)
			} else {
				return fmt.Errorf("either --id or --repo is required")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&resultID, "id", "", "Analysis result ID")
	cmd.Flags().StringVar(&repoID, "repo", "", "Repository ID to list results for")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of results to show")

	return cmd
}

// fixCommand creates the fix command
func (h *Handler) fixCommand() *cobra.Command {
	var (
		issueID  string
		fixIndex int
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Apply fixes for detected issues",
		Long:  "Apply automatic fixes for issues using ARF remediation",
		Example: `  ploy analyze fix --issue issue-123
  ploy analyze fix --issue issue-123 --index 0 --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" {
				return fmt.Errorf("issue ID is required")
			}

			// First get fix suggestions
			suggestions, err := h.sendRequest("GET", fmt.Sprintf("/v1/analysis/issues/%s/fixes", issueID), nil)
			if err != nil {
				return fmt.Errorf("failed to get fix suggestions: %w", err)
			}

			// Display suggestions
			h.displayFixSuggestions(suggestions)

			// Apply fix if not dry run
			if !dryRun {
				request := map[string]interface{}{
					"fix_index": fixIndex,
					"dry_run":   false,
				}

				result, err := h.sendRequest("POST", fmt.Sprintf("/v1/analysis/issues/%s/fix", issueID), request)
				if err != nil {
					return fmt.Errorf("failed to apply fix: %w", err)
				}

				fmt.Println(color.GreenString("\n✓ Fix applied successfully"))
				if status, ok := result["status"].(string); ok {
					fmt.Printf("Status: %s\n", status)
				}
				if message, ok := result["message"].(string); ok {
					fmt.Printf("Message: %s\n", message)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "Issue ID to fix (required)")
	cmd.Flags().IntVar(&fixIndex, "index", 0, "Index of fix suggestion to apply")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be fixed without applying")

	return cmd
}

// sendRequest sends an HTTP request to the controller
func (h *Handler) sendRequest(method, path string, body interface{}) (map[string]interface{}, error) {
	url := h.config.ControllerURL + path

	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBytes)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("invalid response: %s", string(respBody))
	}

	if resp.StatusCode >= 400 {
		if errorMsg, ok := result["error"].(string); ok {
			return nil, fmt.Errorf("API error: %s", errorMsg)
		}
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	return result, nil
}

// Display helper methods

func (h *Handler) displayAnalysisResult(result map[string]interface{}) {
	// Display summary
	fmt.Println(color.CyanString("\n=== Analysis Summary ==="))
	
	if repo, ok := result["repository"].(map[string]interface{}); ok {
		if name, ok := repo["name"].(string); ok {
			fmt.Printf("Repository: %s\n", name)
		}
	}
	
	if score, ok := result["overall_score"].(float64); ok {
		scoreColor := color.GreenString
		if score < 50 {
			scoreColor = color.RedString
		} else if score < 80 {
			scoreColor = color.YellowString
		}
		fmt.Printf("Overall Score: %s\n", scoreColor("%.1f/100", score))
	}

	// Display metrics
	if metrics, ok := result["metrics"].(map[string]interface{}); ok {
		fmt.Println(color.CyanString("\n=== Metrics ==="))
		if totalIssues, ok := metrics["total_issues"].(float64); ok {
			fmt.Printf("Total Issues: %d\n", int(totalIssues))
		}
		if bySeverity, ok := metrics["issues_by_severity"].(map[string]interface{}); ok {
			fmt.Println("\nBy Severity:")
			for severity, count := range bySeverity {
				fmt.Printf("  %s: %v\n", severity, count)
			}
		}
	}

	// Display issues
	if issues, ok := result["issues"].([]interface{}); ok && len(issues) > 0 {
		fmt.Println(color.CyanString("\n=== Issues ==="))
		h.displayIssuesTable(issues)
	}

	// Display ARF triggers
	if triggers, ok := result["arf_triggers"].([]interface{}); ok && len(triggers) > 0 {
		fmt.Printf("\n%s ARF recipes available for automatic remediation\n", 
			color.GreenString("%d", len(triggers)))
	}
}

func (h *Handler) displayIssuesTable(issues []interface{}) {
	table := tablewriter.NewTable(os.Stdout)
	table.Header("ID", "Severity", "Category", "File", "Line", "Message")

	for _, issue := range issues {
		if issueMap, ok := issue.(map[string]interface{}); ok {
			id := h.getString(issueMap, "id")
			severity := h.getString(issueMap, "severity")
			category := h.getString(issueMap, "category")
			file := h.getString(issueMap, "file")
			line := fmt.Sprintf("%d", int(h.getFloat(issueMap, "line")))
			message := h.getString(issueMap, "message")

			// Truncate long paths
			if len(file) > 30 {
				file = "..." + file[len(file)-27:]
			}

			// Truncate long messages
			if len(message) > 40 {
				message = message[:37] + "..."
			}

			// Color severity
			switch severity {
			case "critical":
				severity = color.RedString(severity)
			case "high":
				severity = color.MagentaString(severity)
			case "medium":
				severity = color.YellowString(severity)
			case "low":
				severity = color.BlueString(severity)
			}

			table.Append([]string{id, severity, category, file, line, message})
		}
	}

	table.Render()
}

func (h *Handler) displayLanguages(result map[string]interface{}) {
	if languages, ok := result["languages"].([]interface{}); ok {
		fmt.Println(color.CyanString("\nSupported Languages:"))
		for _, lang := range languages {
			fmt.Printf("  • %s\n", lang)
		}
		fmt.Printf("\nTotal: %d languages\n", len(languages))
	}
}

func (h *Handler) displayResultsList(result map[string]interface{}) {
	if results, ok := result["results"].([]interface{}); ok {
		fmt.Println(color.CyanString("\n=== Analysis Results ==="))
		
		table := tablewriter.NewTable(os.Stdout)
		table.Header("ID", "Repository", "Timestamp", "Score", "Issues")
		
		for _, r := range results {
			if resultMap, ok := r.(map[string]interface{}); ok {
				id := h.getString(resultMap, "id")
				repo := "unknown"
				if repoMap, ok := resultMap["repository"].(map[string]interface{}); ok {
					repo = h.getString(repoMap, "name")
				}
				timestamp := h.getString(resultMap, "timestamp")
				score := fmt.Sprintf("%.1f", h.getFloat(resultMap, "overall_score"))
				issues := "0"
				if issuesList, ok := resultMap["issues"].([]interface{}); ok {
					issues = fmt.Sprintf("%d", len(issuesList))
				}
				
				table.Append([]string{id, repo, timestamp, score, issues})
			}
		}
		
		table.Render()
	}
}

func (h *Handler) displayFixSuggestions(result map[string]interface{}) {
	if suggestions, ok := result["suggestions"].([]interface{}); ok {
		fmt.Println(color.CyanString("\n=== Fix Suggestions ==="))
		for i, suggestion := range suggestions {
			if suggMap, ok := suggestion.(map[string]interface{}); ok {
				fmt.Printf("\n[%d] %s\n", i, h.getString(suggMap, "description"))
				if confidence, ok := suggMap["confidence"].(float64); ok {
					fmt.Printf("    Confidence: %.0f%%\n", confidence*100)
				}
				if recipe, ok := suggMap["arf_recipe"].(string); ok && recipe != "" {
					fmt.Printf("    ARF Recipe: %s\n", recipe)
				}
			}
		}
	}
}

// Helper methods
func (h *Handler) getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func (h *Handler) getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0
}

// RegisterAnalysisCommands registers analysis commands with the root command
func RegisterAnalysisCommands(rootCmd *cobra.Command, controllerURL string) {
	config := &Config{
		ControllerURL: controllerURL,
		Timeout:       30 * time.Second,
		Verbose:       false,
	}
	
	handler := NewHandler(config)
	rootCmd.AddCommand(handler.GetCommands())
}