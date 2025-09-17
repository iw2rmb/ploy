package recipes

import (
	"fmt"
	"strings"
)

// showGeneralHelp displays general ARF recipe help
func (hs *HelpSystem) showGeneralHelp() error {
	fmt.Printf("Ploy ARF Recipe Management\n")
	fmt.Printf("===========================\n\n")

	fmt.Printf("The Automated Remediation Framework (ARF) provides comprehensive code\n")
	fmt.Printf("transformation capabilities through a recipe system. Recipes define\n")
	fmt.Printf("multi-step transformations that can migrate, modernize, and fix codebases.\n\n")

	fmt.Printf("Usage: ploy recipe <command> [arguments] [flags]\n\n")

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
	fmt.Printf("  ploy recipe list --language java\n")
	// run command removed; execution handled by Mods
	fmt.Printf("  ploy recipe create --template openrewrite\n")
	fmt.Printf("  ploy recipe compose prep migration cleanup --repo .\n")

	fmt.Printf("\nFor detailed command help:\n")
	fmt.Printf("  ploy recipe <command> --help\n")

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
