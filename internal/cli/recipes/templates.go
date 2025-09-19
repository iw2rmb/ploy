package recipes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func createRecipeInteractive(flags CommandFlags) error {
	PrintInfo("Creating new recipe interactively")
	fmt.Println()

	template, err := selectTemplate(flags)
	if err != nil {
		return err
	}

	PrintInfo(fmt.Sprintf("Using template: %s", template.Name))
	fmt.Printf("Description: %s\n\n", template.Description)

	if len(template.Examples) > 0 && flags.Verbose {
		fmt.Printf("Examples:\n")
		for _, example := range template.Examples {
			fmt.Printf("  • %s: %s\n", example.Name, example.Description)
		}
		fmt.Println()
	}

	values, err := collectTemplateValues(template)
	if err != nil {
		return err
	}

	recipe, err := generateRecipeFromTemplate(template, values)
	if err != nil {
		return NewCLIError("Failed to generate recipe from template", 1).WithCause(err)
	}

	if err := recipe.Validate(); err != nil {
		if !flags.Force {
			PrintError(NewCLIError("Generated recipe validation failed", 1).WithCause(err))
			if !promptConfirm("Continue anyway?", false) {
				return NewCLIError("Recipe creation cancelled", 0)
			}
		} else {
			PrintWarning(fmt.Sprintf("Recipe validation warnings: %v", err))
		}
	}

	if err := previewRecipe(recipe, flags); err != nil {
		return err
	}

	if !flags.Force && !promptConfirm("Create this recipe?", true) {
		PrintInfo("Recipe creation cancelled")
		return nil
	}

	if flags.DryRun {
		PrintSuccess("Recipe would be created (dry run mode)")
		return nil
	}

	if err := createRecipeFromGenerated(recipe); err != nil {
		return NewCLIError("Failed to create recipe", 1).WithCause(err)
	}

	PrintSuccess(fmt.Sprintf("Recipe '%s' created successfully (ID: %s)", recipe.Metadata.Name, recipe.ID))
	return nil
}

func selectTemplate(flags CommandFlags) (RecipeTemplate, error) {
	if flags.Template != "" {
		template, exists := builtInTemplates[flags.Template]
		if !exists {
			return RecipeTemplate{}, NewCLIError(fmt.Sprintf("Template '%s' not found", flags.Template), 1).
				WithSuggestion("Available templates: " + strings.Join(getAvailableTemplates(), ", "))
		}
		return template, nil
	}

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

func listTemplates(outputFormat string, verbose bool) error {
	templates := getAvailableTemplates()

	switch strings.ToLower(outputFormat) {
	case "json":
		data, _ := json.MarshalIndent(builtInTemplates, "", "  ")
		fmt.Println(string(data))
	default:
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
