package main

import (
	"fmt"
	"os"

	"github.com/iw2rmb/ploy/internal/cli/platform"
	"github.com/iw2rmb/ploy/internal/cli/utils"
	"github.com/iw2rmb/ploy/internal/cli/version"
)

var controllerURL = utils.ResolveControllerURLFromEnv()

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
		case "models":
			ModelsCmd(os.Args[2:])
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
  models    LLM model registry management
  version   Display version information
  help      Show this help message

API Commands:
  ployman api deploy              Deploy latest API version
  ployman api rollback <version>  Rollback to specific version

Model Commands:
  ployman models list             List all LLM models
  ployman models get <id>         Get model details
  ployman models add -f <file>    Add new model from file
  ployman models update <id>      Update existing model
  ployman models delete <id>      Delete model

Environment Variables:
  PLOY_CONTROLLER    API endpoint (default: https://api.dev.ployman.app/v1)
  TARGET_HOST        VPS host for Ansible fallback (for api deploy)

Examples:
  ployman api deploy               # Deploy latest API
  ployman api rollback v1.2.3      # Rollback to v1.2.3
  ployman push -a my-app           # Deploy application
  ployman models list              # List LLM models
  ployman models add -f model.json # Add new model
  ployman version                  # Show version info
`)
}
