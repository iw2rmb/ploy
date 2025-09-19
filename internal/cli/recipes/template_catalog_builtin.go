package recipes

import (
	"time"

	models "github.com/iw2rmb/ploy/api/recipes/models"
)

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
					Type: "openrewrite",
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
					Type: "shell",
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

func getAvailableTemplates() []string {
	templates := make([]string, 0, len(builtInTemplates))
	for id := range builtInTemplates {
		templates = append(templates, id)
	}
	return templates
}
