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
	case "sandbox":
		return handleARFSandboxCommand(args[1:])
	case "benchmark":
		return handleARFBenchmarkCommand(args[1:])
	case "workflow":
		return handleARFWorkflowCommand(args[1:])
	case "health":
		return handleARFHealthCommand()
	case "cache":
		return handleARFCacheCommand(args[1:])
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
	fmt.Println("  transform  Execute code transformations")
	fmt.Println("  sandbox    Manage sandboxes")
	fmt.Println("  benchmark  Run and manage transformation benchmarks")
	fmt.Println("  workflow   Execute end-to-end transformation workflows")
	fmt.Println("  health     Check ARF system health")
	fmt.Println("  cache      Manage AST cache")
	fmt.Println()
	fmt.Println("Use 'ploy arf <subcommand> --help' for more information")
}