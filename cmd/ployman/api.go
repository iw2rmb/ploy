package main

import (
	"fmt"
)

// ApiCmd handles API management commands
func ApiCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("API management commands:")
		fmt.Println("  ployman api deploy              Deploy latest code changes (runs in background)")
		fmt.Println("  ployman api status              Check API deployment status")
		fmt.Println("  ployman api rollback <version>  Rollback to specific version")
		fmt.Println("")
		fmt.Println("Deploy flags:")
		fmt.Println("  --foreground       Wait for deployment to complete (instead of background)")
		fmt.Println("  --monitor          Monitor deployment progress with live output")
		fmt.Println("  --timeout <mins>   Deployment timeout in minutes (default: 10, max: 10)")
		fmt.Println("")
		fmt.Println("Note: Deployments run in background by default to avoid timeout issues.")
		fmt.Println("      Use 'ployman api status' to check deployment progress.")
		fmt.Println("")
		fmt.Println("Environment variables:")
		fmt.Println("  TARGET_HOST        VPS host for deployment (required)")
		fmt.Println("  DEPLOY_BRANCH      Git branch to deploy (default: current branch or 'main')")
		fmt.Println("  PLOY_CONTROLLER    API endpoint (default: https://api.dev.ployman.app/v1)")
		return
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "deploy":
		runApiDeploy(subArgs)
	case "status":
		runApiStatus(subArgs)
	case "rollback":
		runApiRollback(subArgs)
	default:
		fmt.Printf("Unknown api command: %s\n", subcommand)
		fmt.Println("Run 'ployman api' for usage information")
	}
}
