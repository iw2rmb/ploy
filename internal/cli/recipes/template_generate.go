package recipes

import (
	"encoding/json"
	"fmt"
	"strings"

	models "github.com/iw2rmb/ploy/api/recipes/models"
	"gopkg.in/yaml.v3"
)

func generateRecipeFromTemplate(template RecipeTemplate, values map[string]string) (*models.Recipe, error) {
	templateJSON, err := json.Marshal(template.Template)
	if err != nil {
		return nil, err
	}

	templateStr := string(templateJSON)
	for key, value := range values {
		placeholder := fmt.Sprintf("{{.%s}}", key)
		templateStr = strings.ReplaceAll(templateStr, placeholder, value)
	}

	var recipe models.Recipe
	if err := json.Unmarshal([]byte(templateStr), &recipe); err != nil {
		return nil, err
	}

	recipe.SetSystemFields("cli-template")

	return &recipe, nil
}

func previewRecipe(recipe *models.Recipe, flags CommandFlags) error {
	fmt.Printf("\nRecipe Preview:\n")
	fmt.Printf("===============\n")

	if flags.Verbose {
		data, err := yaml.Marshal(recipe)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
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

func createRecipeFromGenerated(recipe *models.Recipe) error {
	recipeJSON, err := json.Marshal(recipe)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/arf/recipes", controllerURL)
	_, err = makeAPIRequest("POST", url, recipeJSON)
	return err
}
