package arf

// initializeBasicCommands initializes basic recipe command help information
func (hs *HelpSystem) initializeBasicCommands() {
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
local directories or remote Git repositories, providing comprehensive execution
tracking and reporting.`,
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
		SeeAlso: []string{"compose", "status"},
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
}
