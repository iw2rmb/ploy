package security

import (
	"fmt"
)

var securityControllerURL string

// Run is the exported entry point for Security commands from main.go
func Run(args []string, controllerURL string) {
	securityControllerURL = controllerURL
	if err := handleSecurityCommand(args); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func handleSecurityCommand(args []string) error {
	if len(args) < 1 {
		printSecurityUsage()
		return nil
	}

	subcommand := args[0]
	switch subcommand {
	case "recipes":
		fmt.Println("Recipe commands have moved. Use 'ploy recipe <action>' instead.")
		return nil
	case "models":
		return handleSecurityModelsCommand(args[1:])
	// Removed commands - functionality integrated into transform:
	// - sandbox: deployment testing is now automatic in transform
	// - benchmark: multi-iteration testing is part of transform --iterations
	// - workflow: human approval not needed in automated system
	default:
		fmt.Printf("Unknown Security subcommand: %s\n", subcommand)
		printSecurityUsage()
		return nil
	}
}

func printSecurityUsage() {
	fmt.Println("Usage: ploy security <subcommand> [options]")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Println("  models     Manage LLM model configurations")
	fmt.Println()
	fmt.Println("Use 'ploy security <subcommand> --help' for more information")
	fmt.Println("Recipe management commands are now under 'ploy recipe'.")
}
