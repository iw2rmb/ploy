package arf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// ModelConfig represents a single LLM model configuration
type ModelConfig struct {
	Name     string `json:"name" yaml:"name"`
	Provider string `json:"provider" yaml:"provider"`
	Endpoint string `json:"endpoint" yaml:"endpoint"`
	APIKey   string `json:"api_key" yaml:"api_key"`
	Model    string `json:"model" yaml:"model"`
	Default  bool   `json:"default,omitempty" yaml:"default,omitempty"`
	MaxTokens int   `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
}

// ModelRegistry represents the complete model registry
type ModelRegistry struct {
	Models []ModelConfig `json:"models" yaml:"models"`
}

func handleARFModelsCommand(args []string) error {
	if len(args) < 1 {
		return handleARFModelsList()
	}

	subcommand := args[0]
	switch subcommand {
	case "list":
		return handleARFModelsList()
	case "add":
		return handleARFModelsAdd(args[1:])
	case "remove":
		return handleARFModelsRemove(args[1:])
	case "set-default":
		return handleARFModelsSetDefault(args[1:])
	case "import":
		return handleARFModelsImport(args[1:])
	case "export":
		return handleARFModelsExport(args[1:])
	case "--help", "-h":
		printARFModelsUsage()
		return nil
	default:
		fmt.Printf("Unknown models subcommand: %s\n", subcommand)
		printARFModelsUsage()
		return nil
	}
}

func handleARFModelsList() error {
	// Fetch model registry from API
	resp, err := http.Get(fmt.Sprintf("%s/arf/models", arfControllerURL))
	if err != nil {
		return fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch models: %s", string(body))
	}

	var registry ModelRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(registry.Models) == 0 {
		fmt.Println("No models configured. Use 'ploy arf models add' to add a model.")
		return nil
	}

	// Display models in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROVIDER\tMODEL\tENDPOINT\tDEFAULT")
	for _, model := range registry.Models {
		defaultStr := ""
		if model.Default {
			defaultStr = "✓"
		}
		endpoint := model.Endpoint
		if len(endpoint) > 40 {
			endpoint = endpoint[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", 
			model.Name, model.Provider, model.Model, endpoint, defaultStr)
	}
	w.Flush()

	return nil
}

func handleARFModelsAdd(args []string) error {
	var config ModelConfig
	var configFile string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				config.Name = args[i+1]
				i++
			}
		case "--provider":
			if i+1 < len(args) {
				config.Provider = args[i+1]
				i++
			}
		case "--endpoint":
			if i+1 < len(args) {
				config.Endpoint = args[i+1]
				i++
			}
		case "--api-key":
			if i+1 < len(args) {
				config.APIKey = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				config.Model = args[i+1]
				i++
			}
		case "--default":
			config.Default = true
		case "--max-tokens":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &config.MaxTokens)
				i++
			}
		case "--temperature":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%f", &config.Temperature)
				i++
			}
		case "--file", "-f":
			if i+1 < len(args) {
				configFile = args[i+1]
				i++
			}
		}
	}

	// If config file is provided, load from file
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, &config); err != nil {
			if err := json.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("failed to parse config file (tried YAML and JSON): %w", err)
			}
		}
	}

	// Validate required fields
	if config.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if config.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if config.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}

	// Set defaults
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}
	if config.Temperature == 0 {
		config.Temperature = 0.1
	}

	// Send to API
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/arf/models", arfControllerURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to add model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to add model: %s", string(body))
	}

	fmt.Printf("Model '%s' added successfully\n", config.Name)
	return nil
}

func handleARFModelsRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("model name is required")
	}

	name := args[0]

	req, err := http.NewRequest(
		"DELETE",
		fmt.Sprintf("%s/arf/models/%s", arfControllerURL, name),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to remove model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to remove model: %s", string(body))
	}

	fmt.Printf("Model '%s' removed successfully\n", name)
	return nil
}

func handleARFModelsSetDefault(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("model name is required")
	}

	name := args[0]

	resp, err := http.Post(
		fmt.Sprintf("%s/arf/models/%s/set-default", arfControllerURL, name),
		"application/json",
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to set default model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set default model: %s", string(body))
	}

	fmt.Printf("Model '%s' set as default\n", name)
	return nil
}

func handleARFModelsImport(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("file path is required")
	}

	filePath := args[0]
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var registry ModelRegistry
	
	// Try YAML first, then JSON
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
		if err := yaml.Unmarshal(data, &registry); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &registry); err != nil {
			// Try YAML if JSON fails
			if err := yaml.Unmarshal(data, &registry); err != nil {
				return fmt.Errorf("failed to parse file (tried JSON and YAML): %w", err)
			}
		}
	}

	// Send to API
	body, err := json.Marshal(registry)
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("%s/arf/models", arfControllerURL),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to import models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to import models: %s", string(body))
	}

	fmt.Printf("Imported %d models successfully\n", len(registry.Models))
	return nil
}

func handleARFModelsExport(args []string) error {
	format := "yaml"
	outputFile := ""

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		case "--output", "-o":
			if i+1 < len(args) {
				outputFile = args[i+1]
				i++
			}
		}
	}

	// Fetch model registry from API
	resp, err := http.Get(fmt.Sprintf("%s/arf/models", arfControllerURL))
	if err != nil {
		return fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch models: %s", string(body))
	}

	var registry ModelRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Marshal to desired format
	var output []byte
	switch format {
	case "yaml", "yml":
		output, err = yaml.Marshal(registry)
		if err != nil {
			return fmt.Errorf("failed to marshal to YAML: %w", err)
		}
	case "json":
		output, err = json.MarshalIndent(registry, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal to JSON: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s (use 'yaml' or 'json')", format)
	}

	// Write to file or stdout
	if outputFile != "" {
		if err := os.WriteFile(outputFile, output, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("Exported models to %s\n", outputFile)
	} else {
		fmt.Print(string(output))
	}

	return nil
}

func printARFModelsUsage() {
	fmt.Println("Usage: ploy arf models <subcommand> [options]")
	fmt.Println()
	fmt.Println("Manage LLM model configurations for ARF transformations")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  list                   List all configured models")
	fmt.Println("  add                    Add a new model configuration")
	fmt.Println("  remove <name>          Remove a model configuration")
	fmt.Println("  set-default <name>     Set a model as the default")
	fmt.Println("  import <file>          Import models from YAML/JSON file")
	fmt.Println("  export                 Export models to YAML/JSON")
	fmt.Println()
	fmt.Println("Add options:")
	fmt.Println("  --name <name>          Model configuration name")
	fmt.Println("  --provider <provider>  Provider (openai, anthropic, custom)")
	fmt.Println("  --endpoint <url>       API endpoint URL")
	fmt.Println("  --api-key <key>        API key for authentication")
	fmt.Println("  --model <model>        Model name (e.g., gpt-4, claude-3)")
	fmt.Println("  --default              Set as default model")
	fmt.Println("  --max-tokens <n>       Maximum tokens (default: 4096)")
	fmt.Println("  --temperature <f>      Temperature (default: 0.1)")
	fmt.Println("  --file <file>          Load configuration from file")
	fmt.Println()
	fmt.Println("Export options:")
	fmt.Println("  --format <format>      Output format: yaml or json (default: yaml)")
	fmt.Println("  --output <file>        Output file (default: stdout)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Add OpenAI model")
	fmt.Println("  ploy arf models add --name gpt4 --provider openai \\")
	fmt.Println("    --endpoint https://api.openai.com/v1 --api-key sk-... \\")
	fmt.Println("    --model gpt-4-turbo --default")
	fmt.Println()
	fmt.Println("  # Import models from file")
	fmt.Println("  ploy arf models import models.yaml")
	fmt.Println()
	fmt.Println("  # Export current models")
	fmt.Println("  ploy arf models export --format yaml --output models.yaml")
}