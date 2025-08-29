package arf

import (
	"fmt"
)

var arfControllerURL string

// ARFCmd is the exported entry point for ARF commands from main.go
func ARFCmd(args []string, controllerURL string) {
	arfControllerURL = controllerURL
	if err := handleARFCommand(args); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func handleARFCommand(args []string) error {
	if len(args) < 1 {
		printARFUsage()
		return nil
	}

	subcommand := args[0]
	switch subcommand {
	case "recipes":
		return handleARFRecipesCommand(args[1:])
	case "transform":
		return handleARFTransformCommand(args[1:])
	case "models":
		return handleARFModelsCommand(args[1:])
	case "health":
		return handleARFHealthCommand()
	case "cache":
		return handleARFCacheCommand(args[1:])
	// Removed commands - functionality integrated into transform:
	// - sandbox: deployment testing is now automatic in transform
	// - benchmark: multi-iteration testing is part of transform --iterations
	// - workflow: human approval not needed in automated system
	default:
		fmt.Printf("Unknown ARF subcommand: %s\n", subcommand)
		printARFUsage()
		return nil
	}
}

func printARFUsage() {
	fmt.Println("Usage: ploy arf <subcommand> [options]")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Println("  recipes    Manage transformation recipes")
	fmt.Println("  transform  Execute code transformations with self-healing")
	fmt.Println("  models     Manage LLM model configurations")
	fmt.Println("  health     Check ARF system health")
	fmt.Println("  cache      Manage AST cache")
	fmt.Println()
	fmt.Println("Use 'ploy arf <subcommand> --help' for more information")
}