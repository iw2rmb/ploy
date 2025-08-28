package main

import (
	"fmt"
	"os"

	"github.com/iw2rmb/ploy/internal/cli/platform"
	"github.com/iw2rmb/ploy/internal/cli/version"
)

var controllerURL = getControllerURL()

func getControllerURL() string {
	// First check if PLOY_CONTROLLER is explicitly set
	if url := os.Getenv("PLOY_CONTROLLER"); url != "" {
		return url
	}
	
	// Check if PLOY_PLATFORM_DOMAIN is set for SSL endpoint
	if domain := os.Getenv("PLOY_PLATFORM_DOMAIN"); domain != "" {
		// Platform services use ployman.app domain
		return fmt.Sprintf("https://api.%s/v1", domain)
	}
	
	// Default to platform domain
	return "https://api.dev.ployman.app/v1"
}

func main() {
	// Set platform mode environment variable
	os.Setenv("PLOY_PLATFORM_MODE", "true")
	
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "api":
			ApiCmd(os.Args[2:])
		case "push":
			// Platform push deploys to ployman.app domain
			platform.PushCmd(os.Args[2:], controllerURL)
		case "rollback":
			// Alias for api rollback
			if len(os.Args) > 2 {
				ApiCmd([]string{"rollback", os.Args[2]})
			} else {
				fmt.Println("Usage: ployman rollback <version>")
			}
		case "version":
			version.VersionCmd(os.Args[2:], controllerURL)
		case "help":
			printUsage()
		default:
			printUsage()
		}
		return
	}
	
	printUsage()
}

func printUsage() {
	fmt.Print(`Ployman - Platform Services Management

Usage:
  ployman <command> [options]

Commands:
  api       API management (deploy, rollback)
  push      Deploy code to platform
  rollback  Rollback to previous version (alias for api rollback)
  version   Display version information
  help      Show this help message

API Commands:
  ployman api deploy              Deploy latest API version
  ployman api rollback <version>  Rollback to specific version

Environment Variables:
  PLOY_CONTROLLER    API endpoint (default: https://api.dev.ployman.app/v1)
  TARGET_HOST        VPS host for SSH fallback (for api deploy)

Examples:
  ployman api deploy               # Deploy latest API
  ployman api rollback v1.2.3      # Rollback to v1.2.3
  ployman push -a my-app           # Deploy application
  ployman version                  # Show version info
`)
}