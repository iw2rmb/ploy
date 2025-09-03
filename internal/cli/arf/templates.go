package arf

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"gopkg.in/yaml.v3"
)

// RecipeTemplate represents a template for creating recipes
type RecipeTemplate struct {
	ID          string             `json:"id" yaml:"id"`
	Name        string             `json:"name" yaml:"name"`
	Description string             `json:"description" yaml:"description"`
	Category    string             `json:"category" yaml:"category"`
	Template    models.Recipe      `json:"template" yaml:"template"`
	Variables   []TemplateVariable `json:"variables,omitempty" yaml:"variables,omitempty"`
	Prompts     []TemplatePrompt   `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Examples    []RecipeExample    `json:"examples,omitempty" yaml:"examples,omitempty"`
}

// TemplateVariable represents a variable in a recipe template
type TemplateVariable struct {
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	Type         string   `json:"type" yaml:"type"` // string, int, bool, array
	Required     bool     `json:"required" yaml:"required"`
	DefaultValue string   `json:"default_value,omitempty" yaml:"default_value,omitempty"`
	Options      []string `json:"options,omitempty" yaml:"options,omitempty"`
	Pattern      string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
}

// TemplatePrompt represents an interactive prompt
type TemplatePrompt struct {
	Field      string   `json:"field" yaml:"field"`
	Message    string   `json:"message" yaml:"message"`
	Type       string   `json:"type" yaml:"type"` // input, select, confirm, multiselect
	Options    []string `json:"options,omitempty" yaml:"options,omitempty"`
	Default    string   `json:"default,omitempty" yaml:"default,omitempty"`
	Required   bool     `json:"required" yaml:"required"`
	Validation string   `json:"validation,omitempty" yaml:"validation,omitempty"`
}

// RecipeExample represents an example for a template
type RecipeExample struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description" yaml:"description"`
	Values      map[string]string `json:"values" yaml:"values"`
}

// Built-in templates
var builtInTemplates = map[string]RecipeTemplate{
	"openrewrite": {
		ID:          "openrewrite-basic",
		Name:        "OpenRewrite Basic",
		Description: "Basic OpenRewrite transformation template",
		Category:    "transformation",
		Template: models.Recipe{
			Metadata: models.RecipeMetadata{
				Name:        "{{.RecipeName}}",
				Description: "{{.Description}}",
				Version:     "1.0.0",
				Author:      "{{.Author}}",
				Languages:   []string{"{{.Language}}"},
				Categories:  []string{"transformation", "{{.Category}}"},
				Tags:        []string{"openrewrite", "{{.Language}}"},
				License:     "MIT",
			},
			Steps: []models.RecipeStep{
				{
					Name: "Apply OpenRewrite Recipe",
					Type: models.StepTypeOpenRewrite,
					Config: map[string]interface{}{
						"recipe":    "{{.OpenRewriteRecipe}}",
						"dataTable": map[string]interface{}{},
						"options":   map[string]interface{}{},
					},
					Timeout: models.Duration{Duration: 10 * time.Minute},
				},
			},
			Execution: models.ExecutionConfig{
				MaxDuration: models.Duration{Duration: 15 * time.Minute},
				Sandbox: models.SandboxConfig{
					Enabled:   true,
					MaxMemory: "1GB",
				},
				Environment: map[string]string{},
			},
		},
		Prompts: []TemplatePrompt{
			{Field: "RecipeName", Message: "Recipe name", Type: "input", Required: true},
			{Field: "Description", Message: "Recipe description", Type: "input", Required: true},
			{Field: "Author", Message: "Author name", Type: "input", Required: true, Default: "ploy-user"},
			{Field: "Language", Message: "Primary language", Type: "select", Options: []string{"java", "kotlin", "groovy"}, Default: "java"},
			{Field: "Category", Message: "Recipe category", Type: "input", Default: "migration"},
			{Field: "OpenRewriteRecipe", Message: "OpenRewrite recipe class", Type: "input", Required: true},
		},
		Examples: []RecipeExample{
			{
				Name:        "Spring Boot 2 to 3 Migration",
				Description: "Migrate Spring Boot application from version 2 to 3",
				Values: map[string]string{
					"RecipeName":        "spring-boot-2-to-3",
					"Description":       "Migrate Spring Boot 2.x application to 3.x",
					"Language":          "java",
					"Category":          "migration",
					"OpenRewriteRecipe": "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
				},
			},
		},
	},
	"shell": {
		ID:          "shell-basic",
		Name:        "Shell Script Basic",
		Description: "Basic shell script transformation template",
		Category:    "scripting",
		Template: models.Recipe{
			Metadata: models.RecipeMetadata{
				Name:        "{{.RecipeName}}",
				Description: "{{.Description}}",
				Version:     "1.0.0",
				Author:      "{{.Author}}",
				Languages:   []string{"{{.Language}}"},
				Categories:  []string{"scripting", "{{.Category}}"},
				Tags:        []string{"shell", "{{.Language}}"},
				License:     "MIT",
			},
			Steps: []models.RecipeStep{
				{
					Name: "Execute Shell Script",
					Type: models.StepTypeShellScript,
					Config: map[string]interface{}{
						"script":      "{{.Script}}",
						"interpreter": "{{.Interpreter}}",
						"workingDir":  "{{.WorkingDir}}",
					},
					Timeout: models.Duration{Duration: 5 * time.Minute},
				},
			},
			Execution: models.ExecutionConfig{
				MaxDuration: models.Duration{Duration: 10 * time.Minute},
				Sandbox: models.SandboxConfig{
					Enabled:   true,
					MaxMemory: "512MB",
				},
				Environment: map[string]string{},
			},
		},
		Prompts: []TemplatePrompt{
			{Field: "RecipeName", Message: "Recipe name", Type: "input", Required: true},
			{Field: "Description", Message: "Recipe description", Type: "input", Required: true},
			{Field: "Author", Message: "Author name", Type: "input", Required: true, Default: "ploy-user"},
			{Field: "Language", Message: "Target language", Type: "select", Options: []string{"bash", "python", "nodejs", "go"}, Default: "bash"},
			{Field: "Category", Message: "Recipe category", Type: "input", Default: "automation"},
			{Field: "StepDescription", Message: "Step description", Type: "input", Required: true},
			{Field: "Script", Message: "Shell script content", Type: "input", Required: true},
			{Field: "Interpreter", Message: "Script interpreter", Type: "select", Options: []string{"bash", "sh", "python", "node"}, Default: "bash"},
			{Field: "WorkingDir", Message: "Working directory", Type: "input", Default: "."},
		},
	},
	"composite": {
		ID:          "composite-basic",
		Name:        "Composite Recipe",
		Description: "Template for creating composite recipes with multiple steps",
		Category:    "composition",
		Template: models.Recipe{
			Metadata: models.RecipeMetadata{
				Name:        "{{.RecipeName}}",
				Description: "{{.Description}}",
				Version:     "1.0.0",
				Author:      "{{.Author}}",
				Languages:   []string{},
				Categories:  []string{"composite", "{{.Category}}"},
				Tags:        []string{"composite", "multi-step"},
				License:     "MIT",
			},
			Steps: []models.RecipeStep{},
			Execution: models.ExecutionConfig{
				MaxDuration: models.Duration{Duration: 30 * time.Minute},
				Sandbox: models.SandboxConfig{
					Enabled:   true,
					MaxMemory: "2GB",
				},
				Environment: map[string]string{},
			},
		},
		Prompts: []TemplatePrompt{
			{Field: "RecipeName", Message: "Recipe name", Type: "input", Required: true},
			{Field: "Description", Message: "Recipe description", Type: "input", Required: true},
			{Field: "Author", Message: "Author name", Type: "input", Required: true, Default: "ploy-user"},
			{Field: "Category", Message: "Recipe category", Type: "input", Default: "workflow"},
		},
	},
}

// createRecipeInteractive creates a recipe interactively using templates
func createRecipeInteractive(flags CommandFlags) error {
	PrintInfo("Creating new recipe interactively")
	fmt.Println()

	// Select template
	template, err := selectTemplate(flags)
	if err != nil {
		return err
	}

	PrintInfo(fmt.Sprintf("Using template: %s", template.Name))
	fmt.Printf("Description: %s\n\n", template.Description)

	// Show examples if available
	if len(template.Examples) > 0 && flags.Verbose {
		fmt.Printf("Examples:\n")
		for _, example := range template.Examples {
			fmt.Printf("  • %s: %s\n", example.Name, example.Description)
		}
		fmt.Println()
	}

	// Collect values through prompts
	values, err := collectTemplateValues(template)
	if err != nil {
		return err
	}

	// Generate recipe from template
	recipe, err := generateRecipeFromTemplate(template, values)
	if err != nil {
		return NewCLIError("Failed to generate recipe from template", 1).WithCause(err)
	}

	// Validate generated recipe
	if err := recipe.Validate(); err != nil {
		if !flags.Force {
			PrintError(NewCLIError("Generated recipe validation failed", 1).WithCause(err))
			confirm := promptConfirm("Continue anyway?", false)
			if !confirm {
				return NewCLIError("Recipe creation cancelled", 0)
			}
		} else {
			PrintWarning(fmt.Sprintf("Recipe validation warnings: %v", err))
		}
	}

	// Preview recipe
	if err := previewRecipe(recipe, flags); err != nil {
		return err
	}

	// Confirm creation
	if !flags.Force {
		if !promptConfirm("Create this recipe?", true) {
			PrintInfo("Recipe creation cancelled")
			return nil
		}
	}

	// Dry run mode
	if flags.DryRun {
		PrintSuccess("Recipe would be created (dry run mode)")
		return nil
	}

	// Create recipe
	if err := createRecipeFromGenerated(recipe); err != nil {
		return NewCLIError("Failed to create recipe", 1).WithCause(err)
	}

	PrintSuccess(fmt.Sprintf("Recipe '%s' created successfully (ID: %s)", recipe.Metadata.Name, recipe.ID))
	return nil
}

// selectTemplate allows user to select a template
func selectTemplate(flags CommandFlags) (RecipeTemplate, error) {
	// If template is specified via flag
	if flags.Template != "" {
		template, exists := builtInTemplates[flags.Template]
		if !exists {
			return RecipeTemplate{}, NewCLIError(fmt.Sprintf("Template '%s' not found", flags.Template), 1).
				WithSuggestion("Available templates: " + strings.Join(getAvailableTemplates(), ", "))
		}
		return template, nil
	}

	// Interactive template selection
	fmt.Printf("Available templates:\n")
	templates := getAvailableTemplates()
	for i, templateID := range templates {
		template := builtInTemplates[templateID]
		fmt.Printf("  %d. %s - %s\n", i+1, template.Name, template.Description)
	}
	fmt.Println()

	for {
		choice := promptInput("Select template (1-" + fmt.Sprintf("%d", len(templates)) + "): ")
		if choice == "" {
			continue
		}

		index, err := strconv.Atoi(choice)
		if err != nil || index < 1 || index > len(templates) {
			PrintWarning("Invalid selection. Please enter a number between 1 and " + fmt.Sprintf("%d", len(templates)))
			continue
		}

		templateID := templates[index-1]
		return builtInTemplates[templateID], nil
	}
}

// collectTemplateValues collects values for template variables through prompts
func collectTemplateValues(template RecipeTemplate) (map[string]string, error) {
	values := make(map[string]string)

	fmt.Printf("Please provide values for the recipe:\n\n")

	for _, prompt := range template.Prompts {
		value, err := executePrompt(prompt)
		if err != nil {
			return nil, err
		}
		values[prompt.Field] = value
	}

	return values, nil
}

// executePrompt executes a single template prompt
func executePrompt(prompt TemplatePrompt) (string, error) {
	switch prompt.Type {
	case "input":
		return executeInputPrompt(prompt)
	case "select":
		return executeSelectPrompt(prompt)
	case "confirm":
		return executeConfirmPrompt(prompt)
	case "multiselect":
		return executeMultiSelectPrompt(prompt)
	default:
		return executeInputPrompt(prompt)
	}
}

// executeInputPrompt executes an input prompt
func executeInputPrompt(prompt TemplatePrompt) (string, error) {
	message := prompt.Message
	if prompt.Default != "" {
		message += fmt.Sprintf(" [%s]", prompt.Default)
	}
	message += ": "

	for {
		value := promptInput(message)

		// Use default if empty input
		if value == "" && prompt.Default != "" {
			value = prompt.Default
		}

		// Check required
		if prompt.Required && value == "" {
			PrintWarning("This field is required")
			continue
		}

		return value, nil
	}
}

// executeSelectPrompt executes a select prompt
func executeSelectPrompt(prompt TemplatePrompt) (string, error) {
	fmt.Printf("%s:\n", prompt.Message)
	for i, option := range prompt.Options {
		marker := " "
		if option == prompt.Default {
			marker = "*"
		}
		fmt.Printf("  %s %d. %s\n", marker, i+1, option)
	}

	for {
		choice := promptInput("Select option (1-" + fmt.Sprintf("%d", len(prompt.Options)) + "): ")

		// Use default if empty
		if choice == "" && prompt.Default != "" {
			return prompt.Default, nil
		}

		index, err := strconv.Atoi(choice)
		if err != nil || index < 1 || index > len(prompt.Options) {
			PrintWarning("Invalid selection")
			continue
		}

		return prompt.Options[index-1], nil
	}
}

// executeConfirmPrompt executes a confirm prompt
func executeConfirmPrompt(prompt TemplatePrompt) (string, error) {
	defaultBool := prompt.Default == "true"
	result := promptConfirm(prompt.Message, defaultBool)
	if result {
		return "true", nil
	}
	return "false", nil
}

// executeMultiSelectPrompt executes a multi-select prompt
func executeMultiSelectPrompt(prompt TemplatePrompt) (string, error) {
	fmt.Printf("%s (select multiple, comma-separated):\n", prompt.Message)
	for i, option := range prompt.Options {
		fmt.Printf("  %d. %s\n", i+1, option)
	}

	input := promptInput("Select options (e.g., 1,3,5): ")
	if input == "" {
		return prompt.Default, nil
	}

	// Parse selections
	var selected []string
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		index, err := strconv.Atoi(part)
		if err != nil || index < 1 || index > len(prompt.Options) {
			continue
		}
		selected = append(selected, prompt.Options[index-1])
	}

	return strings.Join(selected, ","), nil
}

// generateRecipeFromTemplate generates a recipe from template and values
func generateRecipeFromTemplate(template RecipeTemplate, values map[string]string) (*models.Recipe, error) {
	// Serialize template recipe to JSON
	templateJSON, err := json.Marshal(template.Template)
	if err != nil {
		return nil, err
	}

	// Replace template variables
	templateStr := string(templateJSON)
	for key, value := range values {
		placeholder := fmt.Sprintf("{{.%s}}", key)
		templateStr = strings.ReplaceAll(templateStr, placeholder, value)
	}

	// Parse back to recipe
	var recipe models.Recipe
	if err := json.Unmarshal([]byte(templateStr), &recipe); err != nil {
		return nil, err
	}

	// Set system fields
	recipe.SetSystemFields("cli-template")

	return &recipe, nil
}

// previewRecipe shows a preview of the generated recipe
func previewRecipe(recipe *models.Recipe, flags CommandFlags) error {
	fmt.Printf("\nRecipe Preview:\n")
	fmt.Printf("===============\n")

	if flags.Verbose {
		// Show full YAML
		data, err := yaml.Marshal(recipe)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		// Show summary
		fmt.Printf("Name:        %s\n", recipe.Metadata.Name)
		fmt.Printf("Description: %s\n", recipe.Metadata.Description)
		fmt.Printf("Version:     %s\n", recipe.Metadata.Version)
		fmt.Printf("Author:      %s\n", recipe.Metadata.Author)
		fmt.Printf("Languages:   %s\n", strings.Join(recipe.Metadata.Languages, ", "))
		fmt.Printf("Categories:  %s\n", strings.Join(recipe.Metadata.Categories, ", "))
		fmt.Printf("Steps:       %d\n", len(recipe.Steps))

		if len(recipe.Steps) > 0 {
			fmt.Printf("\nSteps:\n")
			for i, step := range recipe.Steps {
				fmt.Printf("  %d. %s (%s)\n", i+1, step.Name, step.Type)
			}
		}
	}
	fmt.Println()

	return nil
}

// createRecipeFromGenerated creates a recipe from the generated data
func createRecipeFromGenerated(recipe *models.Recipe) error {
	// Serialize recipe
	recipeJSON, err := json.Marshal(recipe)
	if err != nil {
		return err
	}

	// Send to API
	url := fmt.Sprintf("%s/arf/recipes", arfControllerURL)
	_, err = makeAPIRequest("POST", url, recipeJSON)
	return err
}

// Helper functions

func getAvailableTemplates() []string {
	var templates []string
	for id := range builtInTemplates {
		templates = append(templates, id)
	}
	return templates
}

func promptInput(message string) string {
	fmt.Print(message)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func promptConfirm(message string, defaultValue bool) bool {
	defaultStr := "y/N"
	if defaultValue {
		defaultStr = "Y/n"
	}

	response := promptInput(fmt.Sprintf("%s (%s): ", message, defaultStr))

	if response == "" {
		return defaultValue
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// listTemplates lists available recipe templates
func listTemplates(outputFormat string, verbose bool) error {
	templates := getAvailableTemplates()

	switch strings.ToLower(outputFormat) {
	case "json":
		data, _ := json.MarshalIndent(builtInTemplates, "", "  ")
		fmt.Println(string(data))
	default: // table
		fmt.Printf("Available Recipe Templates:\n\n")
		for _, templateID := range templates {
			template := builtInTemplates[templateID]
			fmt.Printf("• %s (%s)\n", template.Name, template.ID)
			fmt.Printf("  %s\n", template.Description)
			fmt.Printf("  Category: %s\n", template.Category)

			if verbose {
				fmt.Printf("  Prompts: %d\n", len(template.Prompts))
				if len(template.Examples) > 0 {
					fmt.Printf("  Examples: %d\n", len(template.Examples))
				}
			}
			fmt.Println()
		}
		fmt.Printf("Total: %d templates\n", len(templates))
	}

	return nil
}

// validateTemplate validates a recipe template
func validateTemplate(template RecipeTemplate) error {
	if template.ID == "" {
		return NewCLIError("Template ID is required", 1)
	}

	if template.Name == "" {
		return NewCLIError("Template name is required", 1)
	}

	// Validate template recipe
	if err := template.Template.Validate(); err != nil {
		return NewCLIError("Template recipe is invalid", 1).WithCause(err)
	}

	// Validate prompts
	for _, prompt := range template.Prompts {
		if prompt.Field == "" {
			return NewCLIError("Prompt field is required", 1)
		}
		if prompt.Message == "" {
			return NewCLIError(fmt.Sprintf("Prompt message is required for field '%s'", prompt.Field), 1)
		}
	}

	return nil
}
