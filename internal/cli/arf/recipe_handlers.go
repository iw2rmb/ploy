package arf

import (
	"fmt"
	"strings"
)

// handleARFRecipesCommand is the main entry point for recipe commands
func handleARFRecipesCommand(args []string) error {
	if len(args) == 0 {
		return listRecipes(RecipeFilter{}, "table", false)
	}

	action := args[0]
	switch action {
	case "list", "ls":
		return handleRecipeList(args[1:])
	case "unified":
		return handleUnifiedRecipes(args[1:])
	case "get", "show":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe show <recipe-id> [--verbose] [--output json|yaml|table]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return showRecipe(args[1], flags)
	case "search":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe search <query> [--output json|yaml|table] [--limit <n>]")
			return nil
		}
		// Extract search query (everything except flags)
		query := ""
		queryArgs := []string{}
		for i := 1; i < len(args); i++ {
			if !strings.HasPrefix(args[i], "--") {
				queryArgs = append(queryArgs, args[i])
			} else {
				// Stop at first flag
				break
			}
		}
		query = strings.Join(queryArgs, " ")

		flags := parseCommonFlags(args[len(queryArgs)+1:])
		return searchRecipes(query, flags)
	case "upload", "u":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe upload <recipe-file> [--name <name>] [--dry-run] [--force]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return uploadRecipe(args[1], flags)
	case "update":
		if len(args) < 3 {
			fmt.Println("Usage: ploy arf recipe update <recipe-id> <recipe-file> [--force]")
			return nil
		}
		flags := parseCommonFlags(args[3:])
		return updateRecipe(args[1], args[2], flags)
	case "delete", "del", "rm":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe delete <recipe-id> [--force]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return deleteRecipe(args[1], flags)
	case "download", "dl":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe download <recipe-id> [--output <file>]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return downloadRecipe(args[1], flags)
	case "validate":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe validate <recipe-file> [--strict]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return validateRecipe(args[1], flags)
	case "stats":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe stats <recipe-id> [--output json|yaml|table]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return getRecipeStats(args[1], flags)
	case "create", "init":
		flags := parseCommonFlags(args[1:])
		return createRecipeInteractive(flags)
		// "run" removed; use Mods for execution
	case "compose":
		if len(args) < 3 {
			fmt.Println("Usage: ploy arf recipe compose <recipe-id1> <recipe-id2> [...] [--name <composition-name>] [--repo <url>] [options]")
			return nil
		}
		return composeRecipes(args[1:])
	case "import":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe import <archive-file> [--overwrite] [--validate-only]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return importRecipes(args[1], flags)
	case "export":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe export --output <archive-file> [--tag <tag>] [--author <author>] [--format tar.gz|zip]")
			return nil
		}
		flags := parseCommonFlags(args[1:])
		return exportRecipes(flags)
	case "--help", "-h":
		printRecipesUsage()
		return nil
	case "help":
		if len(args) > 1 {
			// Show help for specific command
			helpSys := NewHelpSystem()
			return helpSys.ShowHelp(args[1])
		}
		printRecipesUsage()
		return nil
	case "examples":
		helpSys := NewHelpSystem()
		return helpSys.ShowExamples()
	case "quickstart":
		helpSys := NewHelpSystem()
		return helpSys.ShowQuickStart()
	case "templates":
		flags := parseCommonFlags(args[1:])
		return listTemplates(flags.OutputFormat, flags.Verbose)
	case "config":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe config <show|set|reset|list> [key] [value]")
			return nil
		}
		return handleConfigCommand(args[1:])
	default:
		fmt.Printf("Unknown recipes action: %s\n", action)
		printRecipesUsage()
		return nil
	}
}

// printRecipesUsage prints the usage information for recipe commands
func printRecipesUsage() {
	fmt.Println("Usage: ploy arf recipe <action> [options]")
	fmt.Println()
	fmt.Println("Available actions:")
	fmt.Println("  list, ls [--filter]              List available recipes with optional filtering")
	fmt.Println("  show <recipe-id>                 Display recipe details")
	fmt.Println("  search <query>                   Search recipes by name/description")
	fmt.Println("  upload, u <recipe-file>          Upload a new recipe from YAML file")
	fmt.Println("  update <id> <recipe-file>        Update existing recipe")
	fmt.Println("  delete, del, rm <recipe-id>      Delete a recipe")
	fmt.Println("  download, dl <recipe-id>         Download recipe to YAML file")
	fmt.Println("  validate <recipe-file>           Validate recipe without uploading")
	fmt.Println("  stats <recipe-id>                Get recipe usage statistics")
	fmt.Println("  create, init                     Create new recipe interactively")
	// run removed: execution handled by Mods
	fmt.Println("  compose <recipe-ids...>          Chain multiple recipes in sequence")
	fmt.Println("  import <archive-file>            Import recipes from archive")
	fmt.Println("  export --output <file>           Export recipes to archive")
	fmt.Println()
	fmt.Println("Common flags:")
	fmt.Println("  --output, -o <format>            Output format: table, json, yaml")
	fmt.Println("  --verbose, -v                    Show detailed information")
	fmt.Println("  --force, -f                      Force operation (skip confirmations)")
	fmt.Println("  --dry-run, -n                    Validate without executing")
	fmt.Println("  --strict, -s                     Enable strict validation")
	fmt.Println()
	fmt.Println("List filters:")
	fmt.Println("  --language <lang>                Filter by programming language")
	fmt.Println("  --category <cat>                 Filter by category")
	fmt.Println("  --tag <tag>                      Filter by tag (can be used multiple times)")
	fmt.Println("  --author <author>                Filter by author")
	fmt.Println("  --pack, -p <pack>                Filter by recipe pack")
	fmt.Println("  --version, -V <version>          Filter by pack version")
	fmt.Println("  --limit <n>                      Maximum number of results (default: 20)")
	fmt.Println("  --offset <n>                     Offset for pagination (default: 0)")
	fmt.Println("  --sort-by <field>                Sort by: name, created, updated, rating")
	fmt.Println("  --sort-order <order>             Sort order: asc, desc (default: asc)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ploy arf recipe list --language java --output json")
	fmt.Println("  ploy arf recipe list --pack rewrite-spring --version 5.0.0")
	fmt.Println("  ploy arf recipe upload my-recipe.yaml --dry-run")
	fmt.Println("  ploy arf recipe search 'spring migration' --limit 5")
	// Example removed: use Mods for execution
	fmt.Println("  ploy arf recipe compose recipe1 recipe2 --name 'full-migration'")
	fmt.Println("  ploy arf recipe export --output recipes-backup.tar.gz --tag migration")
}
