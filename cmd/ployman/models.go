package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/arf/models"
	"gopkg.in/yaml.v3"
)

// ModelsCmd handles model management commands
func ModelsCmd(args []string) {
	if len(args) == 0 {
		printModelsUsage()
		return
	}

	action := args[0]
	switch action {
	case "list", "ls":
		handleModelsList(args[1:])
	case "get", "show":
		if len(args) < 2 {
			fmt.Println("Usage: ployman models get <model-id>")
			return
		}
		handleModelsGet(args[1], args[2:])
	case "add", "create":
		handleModelsAdd(args[1:])
	case "update":
		if len(args) < 2 {
			fmt.Println("Usage: ployman models update <model-id> [flags]")
			return
		}
		handleModelsUpdate(args[1], args[2:])
	case "delete", "del", "rm":
		if len(args) < 2 {
			fmt.Println("Usage: ployman models delete <model-id> [--force]")
			return
		}
		handleModelsDelete(args[1], args[2:])
	case "stats":
		if len(args) < 2 {
			fmt.Println("Usage: ployman models stats <model-id>")
			return
		}
		handleModelsStats(args[1], args[2:])
	case "--help", "-h", "help":
		printModelsUsage()
	default:
		fmt.Printf("Unknown models action: %s\n", action)
		printModelsUsage()
	}
}

func printModelsUsage() {
	fmt.Println(`Usage: ployman models <action> [options]

Available actions:
  list, ls                         List all LLM models
  get, show <model-id>             Display model details
  add, create -f <model-file>      Add new model from file
  update <model-id> -f <file>      Update existing model
  delete, del, rm <model-id>       Delete a model
  stats <model-id>                 Get model usage statistics

Common flags:
  --output, -o <format>            Output format: table, json, yaml
  --file, -f <path>                Model file path (JSON or YAML)
  --force                          Force operation (skip confirmations)
  --provider <provider>            Filter by provider (list only)
  --capability <capability>        Filter by capability (list only)

Examples:
  ployman models list
  ployman models list --provider openai
  ployman models get gpt-4o-mini@2024-08-06
  ployman models add -f my-model.json
  ployman models update gpt-4 -f updated-model.yaml
  ployman models delete old-model --force
  ployman models stats gpt-4o-mini@2024-08-06`)
}

func handleModelsList(args []string) {
	// Parse flags
	outputFormat := "table"
	provider := ""
	capability := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 < len(args) {
				outputFormat = args[i+1]
				i++
			}
		case "--provider":
			if i+1 < len(args) {
				provider = args[i+1]
				i++
			}
		case "--capability":
			if i+1 < len(args) {
				capability = args[i+1]
				i++
			}
		}
	}

	// Build query parameters
	queryParams := make([]string, 0)
	if provider != "" {
		queryParams = append(queryParams, fmt.Sprintf("provider=%s", provider))
	}
	if capability != "" {
		queryParams = append(queryParams, fmt.Sprintf("capability=%s", capability))
	}

	url := fmt.Sprintf("%s/llms/models", controllerURL)
	if len(queryParams) > 0 {
		url += "?" + strings.Join(queryParams, "&")
	}

	// Make API request
	response, err := makeHTTPRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error: Failed to list models: %v\n", err)
		return
	}

	var data struct {
		Models []models.LLMModel `json:"models"`
		Count  int               `json:"count"`
		Total  int               `json:"total"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		return
	}

	// Display results
	switch outputFormat {
	case "json":
		output, _ := json.MarshalIndent(data.Models, "", "  ")
		fmt.Println(string(output))
	case "yaml":
		output, _ := yaml.Marshal(data.Models)
		fmt.Println(string(output))
	default: // table
		if len(data.Models) == 0 {
			fmt.Println("No models found")
			return
		}

		fmt.Printf("%-30s %-12s %-10s %-15s %-s\n", "ID", "PROVIDER", "VERSION", "CAPABILITIES", "MAX_TOKENS")
		fmt.Printf("%-30s %-12s %-10s %-15s %-s\n", strings.Repeat("-", 30), strings.Repeat("-", 12), strings.Repeat("-", 10), strings.Repeat("-", 15), strings.Repeat("-", 10))

		for _, model := range data.Models {
			capabilities := strings.Join(model.Capabilities, ",")
			if len(capabilities) > 15 {
				capabilities = capabilities[:12] + "..."
			}
			fmt.Printf("%-30s %-12s %-10s %-15s %d\n", model.ID, model.Provider, model.Version, capabilities, model.MaxTokens)
		}
		fmt.Printf("\nTotal: %d models\n", data.Count)
	}
}

func handleModelsGet(modelID string, args []string) {
	// Parse flags
	outputFormat := "table"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 < len(args) {
				outputFormat = args[i+1]
				i++
			}
		}
	}

	url := fmt.Sprintf("%s/llms/models/%s", controllerURL, modelID)

	// Make API request
	response, err := makeHTTPRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error: Failed to get model: %v\n", err)
		return
	}

	var model models.LLMModel
	if err := json.Unmarshal(response, &model); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		return
	}

	// Display model
	switch outputFormat {
	case "json":
		output, _ := json.MarshalIndent(model, "", "  ")
		fmt.Println(string(output))
	case "yaml":
		output, _ := yaml.Marshal(model)
		fmt.Println(string(output))
	default: // table
		fmt.Printf("Model Details: %s\n", model.ID)
		fmt.Printf("  Name: %s\n", model.Name)
		fmt.Printf("  Provider: %s\n", model.Provider)
		fmt.Printf("  Version: %s\n", model.Version)
		fmt.Printf("  Capabilities: %s\n", strings.Join(model.Capabilities, ", "))
		fmt.Printf("  Max Tokens: %d\n", model.MaxTokens)
		if model.CostPerToken > 0 {
			fmt.Printf("  Cost Per Token: $%.6f\n", model.CostPerToken)
		}
		fmt.Printf("  Created: %s\n", time.Time(model.Created).Format(time.RFC3339))
		fmt.Printf("  Updated: %s\n", time.Time(model.Updated).Format(time.RFC3339))
		if len(model.Config) > 0 {
			fmt.Println("  Configuration:")
			for key, value := range model.Config {
				fmt.Printf("    %s: %s\n", key, value)
			}
		}
	}
}

func handleModelsAdd(args []string) {
	// Parse flags
	var modelFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 < len(args) {
				modelFile = args[i+1]
				i++
			}
		}
	}

	if modelFile == "" {
		fmt.Println("Error: Model file is required. Use --file or -f flag.")
		return
	}

	// Read model file
	data, err := os.ReadFile(modelFile)
	if err != nil {
		fmt.Printf("Error: Failed to read model file: %v\n", err)
		return
	}

	// Parse model file (try JSON first, then YAML)
	var model models.LLMModel
	if err := json.Unmarshal(data, &model); err != nil {
		// Try YAML
		if err := yaml.Unmarshal(data, &model); err != nil {
			fmt.Printf("Error: Failed to parse model file (tried both JSON and YAML): %v\n", err)
			return
		}
	}

	// Validate model locally
	if err := model.Validate(); err != nil {
		fmt.Printf("Error: Model validation failed: %v\n", err)
		return
	}

	// Send to API
	modelJSON, err := json.Marshal(model)
	if err != nil {
		fmt.Printf("Error: Failed to serialize model: %v\n", err)
		return
	}

	url := fmt.Sprintf("%s/llms/models", controllerURL)
	response, err := makeHTTPRequest("POST", url, bytes.NewReader(modelJSON))
	if err != nil {
		fmt.Printf("Error: Failed to create model: %v\n", err)
		return
	}

	var result map[string]interface{}
	json.Unmarshal(response, &result)

	fmt.Printf("Model '%s' created successfully\n", model.ID)
}

func handleModelsUpdate(modelID string, args []string) {
	// Parse flags
	var modelFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 < len(args) {
				modelFile = args[i+1]
				i++
			}
		}
	}

	if modelFile == "" {
		fmt.Println("Error: Model file is required. Use --file or -f flag.")
		return
	}

	// Read model file
	data, err := os.ReadFile(modelFile)
	if err != nil {
		fmt.Printf("Error: Failed to read model file: %v\n", err)
		return
	}

	// Parse model file (try JSON first, then YAML)
	var model models.LLMModel
	if err := json.Unmarshal(data, &model); err != nil {
		// Try YAML
		if err := yaml.Unmarshal(data, &model); err != nil {
			fmt.Printf("Error: Failed to parse model file (tried both JSON and YAML): %v\n", err)
			return
		}
	}

	// Ensure model ID matches
	if model.ID != modelID {
		fmt.Printf("Error: Model ID in file (%s) does not match ID in command (%s)\n", model.ID, modelID)
		return
	}

	// Validate model locally
	if err := model.Validate(); err != nil {
		fmt.Printf("Error: Model validation failed: %v\n", err)
		return
	}

	// Send to API
	modelJSON, err := json.Marshal(model)
	if err != nil {
		fmt.Printf("Error: Failed to serialize model: %v\n", err)
		return
	}

	url := fmt.Sprintf("%s/llms/models/%s", controllerURL, modelID)
	response, err := makeHTTPRequest("PUT", url, bytes.NewReader(modelJSON))
	if err != nil {
		fmt.Printf("Error: Failed to update model: %v\n", err)
		return
	}

	var result map[string]interface{}
	json.Unmarshal(response, &result)

	fmt.Printf("Model '%s' updated successfully\n", modelID)
}

func handleModelsDelete(modelID string, args []string) {
	// Parse flags
	force := false

	for _, arg := range args {
		if arg == "--force" {
			force = true
		}
	}

	// Confirm deletion unless force flag is set
	if !force {
		fmt.Printf("Are you sure you want to delete model '%s'? (y/N): ", modelID)
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("Deletion cancelled")
			return
		}
	}

	url := fmt.Sprintf("%s/llms/models/%s", controllerURL, modelID)
	_, err := makeHTTPRequest("DELETE", url, nil)
	if err != nil {
		fmt.Printf("Error: Failed to delete model: %v\n", err)
		return
	}

	fmt.Printf("Model '%s' deleted successfully\n", modelID)
}

func handleModelsStats(modelID string, args []string) {
	// Parse flags
	outputFormat := "table"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 < len(args) {
				outputFormat = args[i+1]
				i++
			}
		}
	}

	url := fmt.Sprintf("%s/llms/models/%s/stats", controllerURL, modelID)

	// Make API request
	response, err := makeHTTPRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error: Failed to get model stats: %v\n", err)
		return
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(response, &stats); err != nil {
		fmt.Printf("Error: Failed to parse response: %v\n", err)
		return
	}

	// Display stats
	switch outputFormat {
	case "json":
		output, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(output))
	case "yaml":
		output, _ := yaml.Marshal(stats)
		fmt.Println(string(output))
	default: // table
		fmt.Printf("Model Statistics: %s\n", stats["model_id"])
		if usageCount, ok := stats["usage_count"].(float64); ok {
			fmt.Printf("Usage Count: %d\n", int(usageCount))
		}
		if totalRequests, ok := stats["total_requests"].(float64); ok {
			fmt.Printf("Total Requests: %d\n", int(totalRequests))
		}
		if successfulRequests, ok := stats["successful_requests"].(float64); ok {
			fmt.Printf("Successful Requests: %d\n", int(successfulRequests))
		}
		if successRate, ok := stats["success_rate"].(float64); ok {
			fmt.Printf("Success Rate: %.2f%%\n", successRate*100)
		}
		if costMetrics, ok := stats["cost_metrics"].(map[string]interface{}); ok {
			if totalCost, ok := costMetrics["total_cost"].(float64); ok {
				fmt.Printf("Total Cost: $%.4f\n", totalCost)
			}
		}
	}
}

// makeHTTPRequest is a helper function to make HTTP requests to the API
func makeHTTPRequest(method, url string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		var errorResponse map[string]interface{}
		if json.Unmarshal(responseBody, &errorResponse) == nil {
			if errorMsg, ok := errorResponse["error"].(string); ok {
				return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, errorMsg)
			}
		}
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(responseBody))
	}

	return responseBody, nil
}
