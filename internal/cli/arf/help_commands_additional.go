package arf

// initializeAdditionalCommands adds help for remaining commands
func (hs *HelpSystem) initializeAdditionalCommands() {
	// Recipe search command
	hs.commands["search"] = CommandHelp{
		Name:     "search",
		Synopsis: "Search recipes by name, description, or content",
		Description: `
Performs full-text search across recipe names, descriptions, and content.
Supports advanced query syntax and returns results ranked by relevance.`,
		Usage: "ploy arf recipe search <query> [flags]",
		Flags: []FlagHelp{
			{Long: "--output", Short: "-o", Description: "Output format: table, json, yaml", Default: "table"},
			{Long: "--limit", Description: "Maximum number of results", Default: "10"},
			{Long: "--verbose", Short: "-v", Description: "Show detailed search results", Default: "false"},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Search by keyword",
				Command:     "ploy arf recipe search 'spring boot migration'",
				Description: "Find recipes related to Spring Boot migration",
			},
			{
				Title:       "Limited results with JSON output",
				Command:     "ploy arf recipe search java --limit 5 --output json",
				Description: "Search for Java-related recipes, return top 5 results as JSON",
			},
		},
		SeeAlso: []string{"list", "show"},
	}

	// Recipe import command
	hs.commands["import"] = CommandHelp{
		Name:     "import",
		Synopsis: "Import recipes from archive files",
		Description: `
Imports multiple recipes from archive files (tar.gz, zip, tar). Supports
validation, conflict resolution, and bulk import operations.`,
		Usage: "ploy arf recipe import <archive-file> [flags]",
		Flags: []FlagHelp{
			{Long: "--overwrite", Description: "Overwrite existing recipes with same ID", Default: "false"},
			{Long: "--validate-only", Description: "Validate recipes without importing", Default: "false"},
			{Long: "--force", Short: "-f", Description: "Continue import despite validation warnings", Default: "false"},
			{Long: "--verbose", Short: "-v", Description: "Show detailed import progress", Default: "false"},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Import recipe archive",
				Command:     "ploy arf recipe import recipes-backup.tar.gz",
				Description: "Import all recipes from archive file",
			},
			{
				Title:       "Validate archive without importing",
				Command:     "ploy arf recipe import recipes.tar.gz --validate-only",
				Description: "Check archive contents without importing recipes",
			},
		},
		SeeAlso: []string{"export", "upload"},
	}

	// Recipe export command
	hs.commands["export"] = CommandHelp{
		Name:     "export",
		Synopsis: "Export recipes to archive files",
		Description: `
Exports recipes to archive files with filtering support. Creates portable
recipe collections for backup, sharing, or migration purposes.`,
		Usage: "ploy arf recipe export --output <archive-file> [flags]",
		Flags: []FlagHelp{
			{Long: "--output", Short: "-o", Description: "Output archive file path", Required: true},
			{Long: "--format", Short: "-f", Description: "Archive format: tar.gz, zip, tar", Default: "tar.gz"},
			{Long: "--tag", Description: "Export recipes with specific tag", Default: ""},
			{Long: "--author", Description: "Export recipes by specific author", Default: ""},
			{Long: "--category", Description: "Export recipes in specific category", Default: ""},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Export all recipes",
				Command:     "ploy arf recipe export --output all-recipes.tar.gz",
				Description: "Export all available recipes to compressed archive",
			},
			{
				Title:       "Export recipes by tag",
				Command:     "ploy arf recipe export --output spring-recipes.zip --tag spring --format zip",
				Description: "Export Spring-tagged recipes as ZIP archive",
			},
		},
		SeeAlso: []string{"import", "list"},
	}

	// Recipe create command
	hs.commands["create"] = CommandHelp{
		Name:     "create",
		Synopsis: "Create new recipe interactively using templates",
		Description: `
Interactive recipe creation using built-in templates. Guides users through
recipe creation with prompts, validation, and examples.`,
		Usage: "ploy arf recipe create [flags]",
		Flags: []FlagHelp{
			{Long: "--template", Short: "-t", Description: "Template to use: openrewrite, shell, composite", Default: ""},
			{Long: "--interactive", Short: "-i", Description: "Use interactive mode", Default: "true"},
			{Long: "--dry-run", Short: "-n", Description: "Preview recipe without creating", Default: "false"},
			{Long: "--verbose", Short: "-v", Description: "Show detailed template information", Default: "false"},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Interactive recipe creation",
				Command:     "ploy arf recipe create",
				Description: "Start interactive recipe creation with template selection",
			},
			{
				Title:       "Use specific template",
				Command:     "ploy arf recipe create --template openrewrite",
				Description: "Create recipe using OpenRewrite template",
			},
		},
		SeeAlso: []string{"upload", "validate", "templates"},
	}
}
