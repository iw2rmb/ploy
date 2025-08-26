package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/iw2rmb/ploy/internal/cli/analyze"
	"github.com/iw2rmb/ploy/internal/cli/apps"
	"github.com/iw2rmb/ploy/internal/cli/arf"
	"github.com/iw2rmb/ploy/internal/cli/bluegreen"
	"github.com/iw2rmb/ploy/internal/cli/certs"
	"github.com/iw2rmb/ploy/internal/cli/debug"
	"github.com/iw2rmb/ploy/internal/cli/deploy"
	"github.com/iw2rmb/ploy/internal/cli/domains"
	"github.com/iw2rmb/ploy/internal/cli/env"
	"github.com/iw2rmb/ploy/internal/cli/ui"
	"github.com/iw2rmb/ploy/internal/cli/version"
)

var controllerURL = getControllerURL()

func getControllerURL() string {
	// First check if PLOY_CONTROLLER is explicitly set
	if url := os.Getenv("PLOY_CONTROLLER"); url != "" {
		return url
	}
	
	// Check if PLOY_APPS_DOMAIN is set for SSL endpoint
	if domain := os.Getenv("PLOY_APPS_DOMAIN"); domain != "" {
		return fmt.Sprintf("https://api.%s/v1", domain)
	}
	
	// Default to localhost
	return "http://localhost:8081/v1"
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "analyze":
			analyze.AnalyzeCmd(os.Args[2:], controllerURL)
		case "apps":
			apps.AppsCmd(os.Args[2:], controllerURL)
		case "push":
			deploy.PushCmd(os.Args[2:], controllerURL)
		case "open":
			deploy.OpenCmd(os.Args[2:])
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
			arf.ARFCmd(os.Args[2:], controllerURL)
		case "bluegreen":
			bluegreen.BlueGreenCmd(os.Args[2:], controllerURL)
		case "version":
			version.VersionCmd(os.Args[2:], controllerURL)
		default:
			ui.Usage()
		}
		return
	}
	p := tea.NewProgram(ui.Model{})
	if err := p.Start(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}