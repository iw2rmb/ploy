package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/iw2rmb/ploy/internal/cli/apps"
	"github.com/iw2rmb/ploy/internal/cli/arf"
	"github.com/iw2rmb/ploy/internal/cli/certs"
	"github.com/iw2rmb/ploy/internal/cli/debug"
	"github.com/iw2rmb/ploy/internal/cli/domains"
	"github.com/iw2rmb/ploy/internal/cli/env"
	"github.com/iw2rmb/ploy/internal/cli/platform"
	"github.com/iw2rmb/ploy/internal/cli/ui"
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
	return "https://api.ployman.app/v1"
}

func main() {
	// Set platform mode environment variable
	os.Setenv("PLOY_PLATFORM_MODE", "true")
	
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "apps":
			// Platform apps are created with special domain routing
			apps.AppsCmd(os.Args[2:], controllerURL)
		case "push":
			// Platform push deploys to ployman.app domain
			platform.PushCmd(os.Args[2:], controllerURL)
		case "open":
			// Open platform service in browser
			platform.OpenCmd(os.Args[2:])
		case "env":
			env.EnvCmd(os.Args[2:], controllerURL)
		case "domains":
			domains.DomainsCmd(os.Args[2:], controllerURL)
		case "certs":
			certs.CertsCmd(os.Args[2:], controllerURL)
		case "debug":
			debug.DebugCmd(os.Args[2:], controllerURL)
		case "rollback":
			debug.RollbackCmd(os.Args[2:], controllerURL)
		case "arf":
			// ARF commands are specific to platform services
			arf.ARFCmd(os.Args[2:], controllerURL)
		case "version":
			version.VersionCmd(os.Args[2:], controllerURL)
		case "help":
			printUsage()
		default:
			printUsage()
		}
		return
	}
	
	// Interactive mode
	p := tea.NewProgram(ui.Model{})
	if err := p.Start(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Ployman - Platform Services Management for Ploy

Usage:
  ployman <command> [options]

Commands:
  apps      Manage platform applications
  push      Deploy a platform service to ployman.app domain
  open      Open platform service in browser
  env       Manage environment variables
  domains   Manage custom domains
  certs     Manage SSL certificates
  debug     Debug platform services
  rollback  Rollback to previous version
  arf       ARF benchmark management
  version   Display version information
  help      Show this help message

Platform Services:
  Platform services are deployed to the ployman.app domain:
  - api.ployman.app (production)
  - api.dev.ployman.app (development)
  - openrewrite.ployman.app (OpenRewrite service)

Examples:
  ployman push -a openrewrite-service     # Deploy OpenRewrite to openrewrite.ployman.app
  ployman apps new metrics-service        # Create new platform service
  ployman env set -a api PORT=8081        # Set environment variable

Environment Variables:
  PLOY_PLATFORM_DOMAIN    Platform services domain (default: ployman.app)
  PLOY_CONTROLLER         Override controller URL
`)
}