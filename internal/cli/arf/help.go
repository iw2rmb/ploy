package arf

import (
	"fmt"
	"strings"
)

// HelpSystem provides comprehensive help and usage information
type HelpSystem struct {
	commands map[string]CommandHelp
}

// CommandHelp represents help information for a specific command
type CommandHelp struct {
	Name        string
	Synopsis    string
	Description string
	Usage       string
	Flags       []FlagHelp
	Examples    []ExampleHelp
	SeeAlso     []string
}

// FlagHelp represents help for a command flag
type FlagHelp struct {
	Long        string
	Short       string
	Description string
	Default     string
	Required    bool
}

// ExampleHelp represents a usage example
type ExampleHelp struct {
	Title       string
	Command     string
	Description string
}

// NewHelpSystem creates a new help system with all command documentation
func NewHelpSystem() *HelpSystem {
	hs := &HelpSystem{
		commands: make(map[string]CommandHelp),
	}
	hs.initializeCommands()
	return hs
}

// initializeCommands initializes all command help information
func (hs *HelpSystem) initializeCommands() {
	// Recipe list command
	hs.commands["list"] = CommandHelp{
		Name:     "list",
		Synopsis: "List available recipes with optional filtering and sorting",
		Description: `
Lists all available transformation recipes in the ARF system with comprehensive
filtering, sorting, and pagination support. Results can be displayed in multiple
formats for integration with other tools.`,
		Usage: "ploy arf recipe list [flags]",
		Flags: []FlagHelp{
			{Long: "--language", Short: "-l", Description: "Filter by programming language", Default: ""},
			{Long: "--category", Short: "-c", Description: "Filter by recipe category", Default: ""},
			{Long: "--tag", Short: "-t", Description: "Filter by tag (can be used multiple times)", Default: ""},
			{Long: "--author", Short: "-a", Description: "Filter by recipe author", Default: ""},
			{Long: "--output", Short: "-o", Description: "Output format: table, json, yaml", Default: "table"},
			{Long: "--limit", Description: "Maximum number of results per page", Default: "20"},
			{Long: "--offset", Description: "Number of results to skip (for pagination)", Default: "0"},
			{Long: "--sort-by", Description: "Sort by field: name, created, updated, author, version", Default: "name"},
			{Long: "--sort-order", Description: "Sort order: asc, desc", Default: "asc"},
			{Long: "--verbose", Short: "-v", Description: "Show detailed information including creation dates", Default: "false"},
		},
		Examples: []ExampleHelp{
			{
				Title:       "List all recipes in table format",
				Command:     "ploy arf recipe list",
				Description: "Shows first 20 recipes in a formatted table",
			},
			{
				Title:       "Filter Java recipes by migration category",
				Command:     "ploy arf recipe list --language java --category migration",
				Description: "Shows only Java recipes in the migration category",
			},
			{
				Title:       "Search with multiple tags and JSON output",
				Command:     "ploy arf recipe list --tag spring --tag boot --output json",
				Description: "Find recipes tagged with both 'spring' and 'boot', output as JSON",
			},
			{
				Title:       "Paginated results with custom sort",
				Command:     "ploy arf recipe list --limit 10 --offset 20 --sort-by created --sort-order desc",
				Description: "Show recipes 21-30, sorted by creation date (newest first)",
			},
			{
				Title:       "Author-specific recipes with verbose details",
				Command:     "ploy arf recipe list --author openrewrite --verbose",
				Description: "List all recipes by 'openrewrite' author with detailed timestamps",
			},
		},
		SeeAlso: []string{"search", "show", "upload"},
	}

	// Recipe show command
	hs.commands["show"] = CommandHelp{
		Name:     "show",
		Synopsis: "Display detailed information about a specific recipe",
		Description: `
Shows comprehensive details about a specific recipe including metadata, steps,
execution configuration, and system information. Output can be formatted as
human-readable table or machine-parseable JSON/YAML.`,
		Usage: "ploy arf recipe show <recipe-id> [flags]",
		Flags: []FlagHelp{
			{Long: "--output", Short: "-o", Description: "Output format: table, json, yaml", Default: "table"},
			{Long: "--verbose", Short: "-v", Description: "Show all details including system fields", Default: "false"},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Show recipe summary",
				Command:     "ploy arf recipe show java11to17-1.0.0",
				Description: "Display basic recipe information in table format",
			},
			{
				Title:       "Show complete recipe details",
				Command:     "ploy arf recipe show java11to17-1.0.0 --verbose",
				Description: "Display all recipe information including system fields",
			},
			{
				Title:       "Export recipe as YAML",
				Command:     "ploy arf recipe show java11to17-1.0.0 --output yaml",
				Description: "Show recipe in YAML format suitable for editing or backup",
			},
			{
				Title:       "Machine-readable JSON output",
				Command:     "ploy arf recipe show java11to17-1.0.0 --output json",
				Description: "Export recipe metadata as JSON for API integration",
			},
		},
		SeeAlso: []string{"list", "download", "stats"},
	}

	// Recipe upload command
	hs.commands["upload"] = CommandHelp{
		Name:     "upload",
		Synopsis: "Upload a new transformation recipe from YAML file",
		Description: `
Uploads a new transformation recipe to the ARF system. The recipe file must be
in YAML format following the ARF recipe specification. Validation is performed
before upload to ensure recipe integrity.`,
		Usage: "ploy arf recipe upload <recipe-file> [flags]",
		Flags: []FlagHelp{
			{Long: "--dry-run", Short: "-n", Description: "Validate recipe without uploading", Default: "false"},
			{Long: "--force", Short: "-f", Description: "Skip validation warnings and upload anyway", Default: "false"},
			{Long: "--name", Description: "Override recipe name from file", Default: ""},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Upload recipe from file",
				Command:     "ploy arf recipe upload my-migration.yaml",
				Description: "Upload and validate recipe from YAML file",
			},
			{
				Title:       "Validate recipe without uploading",
				Command:     "ploy arf recipe upload my-migration.yaml --dry-run",
				Description: "Check recipe validity without storing it",
			},
			{
				Title:       "Force upload with warnings",
				Command:     "ploy arf recipe upload my-migration.yaml --force",
				Description: "Upload recipe even if validation warnings exist",
			},
			{
				Title:       "Upload with custom name",
				Command:     "ploy arf recipe upload recipe.yaml --name custom-spring-migration",
				Description: "Override the recipe name specified in the YAML file",
			},
		},
		SeeAlso: []string{"validate", "create", "update"},
	}

	// Recipe run command
	hs.commands["run"] = CommandHelp{
		Name:     "run",
		Synopsis: "Execute a recipe against a repository",
		Description: `
Executes a transformation recipe against a specified repository. Can work with
local directories or remote Git repositories. Integration with the benchmark
system provides comprehensive execution tracking and reporting.`,
		Usage: "ploy arf recipe run <recipe-id> [flags]",
		Flags: []FlagHelp{
			{Long: "--repo", Short: "-r", Description: "Repository URL or local path", Default: ".", Required: true},
			{Long: "--branch", Description: "Git branch to use for remote repositories", Default: "main"},
			{Long: "--output-dir", Description: "Directory for output files and reports", Default: ""},
			{Long: "--timeout", Description: "Execution timeout (e.g., 15m, 1h)", Default: "15m"},
			{Long: "--dry-run", Short: "-n", Description: "Show what would be executed without running", Default: "false"},
			{Long: "--report", Description: "Generate detailed execution report", Default: "false"},
			{Long: "--verbose", Short: "-v", Description: "Show detailed execution output", Default: "false"},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Run recipe on current directory",
				Command:     "ploy arf recipe run java11to17-1.0.0",
				Description: "Execute recipe against current directory",
			},
			{
				Title:       "Run recipe on remote repository",
				Command:     "ploy arf recipe run spring-boot-migration --repo https://github.com/user/app.git",
				Description: "Clone and execute recipe against remote repository",
			},
			{
				Title:       "Run with specific branch and reporting",
				Command:     "ploy arf recipe run java11to17-1.0.0 --repo . --branch develop --report",
				Description: "Execute on develop branch with detailed execution report",
			},
			{
				Title:       "Dry run to preview changes",
				Command:     "ploy arf recipe run migration-recipe --dry-run --verbose",
				Description: "Show what the recipe would do without making changes",
			},
		},
		SeeAlso: []string{"compose", "status", "benchmark"},
	}

	// Recipe compose command
	hs.commands["compose"] = CommandHelp{
		Name:     "compose",
		Synopsis: "Chain multiple recipes in sequence or parallel",
		Description: `
Creates and executes a composition of multiple recipes, allowing complex
transformation workflows. Recipes can be executed sequentially or in parallel,
with configurable error handling and rollback strategies.`,
		Usage: "ploy arf recipe compose <recipe-id1> <recipe-id2> [...] [flags]",
		Flags: []FlagHelp{
			{Long: "--name", Description: "Name for the recipe composition", Default: ""},
			{Long: "--repo", Short: "-r", Description: "Repository URL or local path", Default: "."},
			{Long: "--branch", Description: "Git branch to use", Default: "main"},
			{Long: "--parallel", Description: "Execute recipes in parallel", Default: "false"},
			{Long: "--continue-on-error", Description: "Continue if individual recipes fail", Default: "false"},
			{Long: "--timeout", Description: "Total composition timeout", Default: "30m"},
			{Long: "--report", Description: "Generate composition execution report", Default: "false"},
		},
		Examples: []ExampleHelp{
			{
				Title:       "Sequential recipe execution",
				Command:     "ploy arf recipe compose prep-migration java11to17 cleanup-deps",
				Description: "Execute three recipes in sequence",
			},
			{
				Title:       "Named composition with custom repository",
				Command:     "ploy arf recipe compose recipe1 recipe2 --name full-migration --repo https://github.com/user/app.git",
				Description: "Create named composition against remote repository",
			},
			{
				Title:       "Parallel execution with error tolerance",
				Command:     "ploy arf recipe compose test-recipe1 test-recipe2 --parallel --continue-on-error",
				Description: "Run recipes in parallel, continuing even if some fail",
			},
			{
				Title:       "Long-running composition with reporting",
				Command:     "ploy arf recipe compose migration1 migration2 migration3 --timeout 2h --report",
				Description: "Execute long composition with extended timeout and detailed reporting",
			},
		},
		SeeAlso: []string{"run", "list", "show"},
	}

	// Add more commands...
	hs.initializeAdditionalCommands()
}

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

// ShowHelp displays help for a specific command or general help
func (hs *HelpSystem) ShowHelp(command string) error {
	if command == "" {
		return hs.showGeneralHelp()
	}

	cmdHelp, exists := hs.commands[command]
	if !exists {
		return NewCLIError(fmt.Sprintf("No help available for command '%s'", command), 1).
			WithSuggestion("Use 'ploy arf recipe --help' to see all available commands")
	}

	return hs.showCommandHelp(cmdHelp)
}

// showGeneralHelp displays general ARF recipe help
func (hs *HelpSystem) showGeneralHelp() error {
	fmt.Printf("Ploy ARF Recipe Management\n")
	fmt.Printf("===========================\n\n")

	fmt.Printf("The Automated Remediation Framework (ARF) provides comprehensive code\n")
	fmt.Printf("transformation capabilities through a recipe system. Recipes define\n")
	fmt.Printf("multi-step transformations that can migrate, modernize, and fix codebases.\n\n")

	fmt.Printf("Usage: ploy arf recipe <command> [arguments] [flags]\n\n")

	fmt.Printf("Available Commands:\n")
	categories := map[string][]string{
		"Recipe Management": {"list", "show", "search", "upload", "update", "delete", "download"},
		"Recipe Creation":   {"create", "validate"},
		"Recipe Execution":  {"run", "compose"},
		"Bulk Operations":   {"import", "export"},
		"Information":       {"stats", "templates"},
	}

	for category, commands := range categories {
		fmt.Printf("\n%s:\n", category)
		for _, cmd := range commands {
			if cmdHelp, exists := hs.commands[cmd]; exists {
				fmt.Printf("  %-12s %s\n", cmd, cmdHelp.Synopsis)
			}
		}
	}

	fmt.Printf("\nGlobal Flags:\n")
	fmt.Printf("  --help, -h       Show help information\n")
	fmt.Printf("  --output, -o     Output format: table, json, yaml (default: table)\n")
	fmt.Printf("  --verbose, -v    Show detailed information\n")
	fmt.Printf("  --dry-run, -n    Preview operations without executing\n")
	fmt.Printf("  --force, -f      Skip confirmations and warnings\n")

	fmt.Printf("\nExamples:\n")
	fmt.Printf("  ploy arf recipe list --language java\n")
	fmt.Printf("  ploy arf recipe run spring-migration --repo https://github.com/user/app.git\n")
	fmt.Printf("  ploy arf recipe create --template openrewrite\n")
	fmt.Printf("  ploy arf recipe compose prep migration cleanup --repo .\n")

	fmt.Printf("\nFor detailed command help:\n")
	fmt.Printf("  ploy arf recipe <command> --help\n")

	fmt.Printf("\nDocumentation:\n")
	fmt.Printf("  Recipe Format: https://docs.ployd.app/arf/recipes\n")
	fmt.Printf("  ARF Guide:     https://docs.ployd.app/arf/guide\n")
	fmt.Printf("  Examples:      https://docs.ployd.app/arf/examples\n")

	return nil
}

// showCommandHelp displays detailed help for a specific command
func (hs *HelpSystem) showCommandHelp(cmdHelp CommandHelp) error {
	fmt.Printf("%s\n", strings.Repeat("=", len(cmdHelp.Name)+8))
	fmt.Printf("Command: %s\n", cmdHelp.Name)
	fmt.Printf("%s\n\n", strings.Repeat("=", len(cmdHelp.Name)+8))

	fmt.Printf("Synopsis:\n")
	fmt.Printf("  %s\n\n", cmdHelp.Synopsis)

	if cmdHelp.Description != "" {
		fmt.Printf("Description:\n")
		fmt.Printf("%s\n\n", strings.TrimSpace(cmdHelp.Description))
	}

	fmt.Printf("Usage:\n")
	fmt.Printf("  %s\n\n", cmdHelp.Usage)

	if len(cmdHelp.Flags) > 0 {
		fmt.Printf("Flags:\n")
		for _, flag := range cmdHelp.Flags {
			flagStr := fmt.Sprintf("  --%s", flag.Long)
			if flag.Short != "" {
				flagStr += fmt.Sprintf(", -%s", flag.Short)
			}

			// Pad to consistent width
			for len(flagStr) < 25 {
				flagStr += " "
			}

			fmt.Printf("%s %s", flagStr, flag.Description)
			
			if flag.Default != "" && flag.Default != "false" {
				fmt.Printf(" (default: %s)", flag.Default)
			}
			
			if flag.Required {
				fmt.Printf(" [required]")
			}
			
			fmt.Println()
		}
		fmt.Println()
	}

	if len(cmdHelp.Examples) > 0 {
		fmt.Printf("Examples:\n\n")
		for i, example := range cmdHelp.Examples {
			fmt.Printf("%d. %s:\n", i+1, example.Title)
			fmt.Printf("   %s\n", example.Command)
			if example.Description != "" {
				fmt.Printf("   → %s\n", example.Description)
			}
			fmt.Println()
		}
	}

	if len(cmdHelp.SeeAlso) > 0 {
		fmt.Printf("See Also:\n")
		fmt.Printf("  %s\n", strings.Join(cmdHelp.SeeAlso, ", "))
		fmt.Println()
	}

	return nil
}

// ShowExamples displays curated examples for common use cases
func (hs *HelpSystem) ShowExamples() error {
	fmt.Printf("ARF Recipe Examples\n")
	fmt.Printf("===================\n\n")

	examples := []struct {
		Title       string
		Description string
		Commands    []string
	}{
		{
			Title:       "Basic Recipe Management",
			Description: "Common operations for managing recipes",
			Commands: []string{
				"# List all available recipes",
				"ploy arf recipe list",
				"",
				"# Search for Spring Boot recipes", 
				"ploy arf recipe search 'spring boot'",
				"",
				"# Show details of a specific recipe",
				"ploy arf recipe show spring-boot-2-to-3",
				"",
				"# Upload a new recipe from file",
				"ploy arf recipe upload my-recipe.yaml",
			},
		},
		{
			Title:       "Recipe Execution Workflows",
			Description: "Execute recipes against repositories",
			Commands: []string{
				"# Run recipe on current directory",
				"ploy arf recipe run java11to17-migration",
				"",
				"# Run recipe on remote repository",
				"ploy arf recipe run spring-migration \\",
				"  --repo https://github.com/user/app.git \\",
				"  --branch develop",
				"",
				"# Chain multiple recipes",
				"ploy arf recipe compose prep-migration java11to17 cleanup \\",
				"  --name complete-migration \\",
				"  --repo . \\",
				"  --report",
			},
		},
		{
			Title:       "Advanced Filtering and Search",
			Description: "Find specific recipes using filters",
			Commands: []string{
				"# Filter by language and category",
				"ploy arf recipe list --language java --category migration",
				"",
				"# Multiple tag filtering with JSON output",
				"ploy arf recipe list --tag spring --tag boot --output json",
				"",
				"# Sort by creation date, show recent first",
				"ploy arf recipe list --sort-by created --sort-order desc --limit 5",
				"",
				"# Find recipes by specific author",
				"ploy arf recipe list --author openrewrite --verbose",
			},
		},
		{
			Title:       "Recipe Creation and Templates",
			Description: "Create new recipes using templates",
			Commands: []string{
				"# Interactive recipe creation",
				"ploy arf recipe create",
				"",
				"# Create using specific template",
				"ploy arf recipe create --template openrewrite",
				"",
				"# Preview without creating",
				"ploy arf recipe create --template shell --dry-run",
				"",
				"# Validate existing recipe file",
				"ploy arf recipe validate my-recipe.yaml --strict",
			},
		},
		{
			Title:       "Bulk Operations and Backup",
			Description: "Import, export, and manage recipe collections",
			Commands: []string{
				"# Export all recipes to archive",
				"ploy arf recipe export --output backup.tar.gz",
				"",
				"# Export specific category as ZIP",
				"ploy arf recipe export --output migrations.zip \\",
				"  --category migration --format zip",
				"",
				"# Import recipes from archive",
				"ploy arf recipe import backup.tar.gz --verbose",
				"",
				"# Validate archive without importing",
				"ploy arf recipe import recipes.tar.gz --validate-only",
			},
		},
		{
			Title:       "Integration with Development Workflow",
			Description: "Use recipes in CI/CD and development processes",
			Commands: []string{
				"# Automated testing of recipe",
				"ploy arf recipe run test-recipe --repo . --dry-run",
				"",
				"# Generate execution report for compliance",
				"ploy arf recipe run security-fix \\",
				"  --repo . \\",
				"  --report \\",
				"  --output-dir ./reports",
				"",
				"# Parallel execution for independent changes",
				"ploy arf recipe compose style-fix license-update \\",
				"  --parallel \\",
				"  --continue-on-error",
			},
		},
	}

	for i, example := range examples {
		if i > 0 {
			fmt.Println()
		}
		
		fmt.Printf("%d. %s\n", i+1, example.Title)
		fmt.Printf("   %s\n\n", example.Description)
		
		for _, cmd := range example.Commands {
			if cmd == "" {
				fmt.Println()
			} else if strings.HasPrefix(cmd, "#") {
				fmt.Printf("   \033[32m%s\033[0m\n", cmd) // Green for comments
			} else {
				fmt.Printf("   %s\n", cmd)
			}
		}
	}

	fmt.Printf("\nFor more examples and tutorials:\n")
	fmt.Printf("  https://docs.ployd.app/arf/examples\n")

	return nil
}

// ShowQuickStart displays a quick start guide
func (hs *HelpSystem) ShowQuickStart() error {
	fmt.Printf("ARF Recipe Quick Start\n")
	fmt.Printf("======================\n\n")

	steps := []struct {
		Step        string
		Description string
		Command     string
	}{
		{
			Step:        "1. List Available Recipes",
			Description: "See what transformation recipes are available",
			Command:     "ploy arf recipe list",
		},
		{
			Step:        "2. Find Relevant Recipes",
			Description: "Search for recipes matching your technology stack",
			Command:     "ploy arf recipe search 'java spring'",
		},
		{
			Step:        "3. Examine Recipe Details", 
			Description: "Review what a recipe does before running it",
			Command:     "ploy arf recipe show java11to17-migration",
		},
		{
			Step:        "4. Test Recipe (Safe Mode)",
			Description: "Preview changes without modifying files",
			Command:     "ploy arf recipe run java11to17-migration --dry-run",
		},
		{
			Step:        "5. Execute Recipe",
			Description: "Apply the transformation to your codebase",
			Command:     "ploy arf recipe run java11to17-migration --repo .",
		},
		{
			Step:        "6. Create Custom Recipe",
			Description: "Build your own transformation recipe",
			Command:     "ploy arf recipe create --template openrewrite",
		},
	}

	for _, step := range steps {
		fmt.Printf("%s\n", step.Step)
		fmt.Printf("   %s\n", step.Description)
		fmt.Printf("   \033[36m$ %s\033[0m\n\n", step.Command) // Cyan for commands
	}

	fmt.Printf("Next Steps:\n")
	fmt.Printf("  • Read the recipe format guide: https://docs.ployd.app/arf/recipes\n") 
	fmt.Printf("  • Browse example recipes: https://docs.ployd.app/arf/examples\n")
	fmt.Printf("  • Join the community: https://github.com/ploy/recipes\n")

	return nil
}

// GetAvailableHelp returns list of available help topics
func (hs *HelpSystem) GetAvailableHelp() []string {
	var topics []string
	for cmd := range hs.commands {
		topics = append(topics, cmd)
	}
	return topics
}