package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ploy/ploy/internal/cli/apps"
	"github.com/ploy/ploy/internal/cli/certs"
	"github.com/ploy/ploy/internal/cli/debug"
	"github.com/ploy/ploy/internal/cli/deploy"
	"github.com/ploy/ploy/internal/cli/domains"
	"github.com/ploy/ploy/internal/cli/env"
	"github.com/ploy/ploy/internal/cli/ui"
	"github.com/ploy/ploy/internal/cli/utils"
)

var controllerURL = utils.Getenv("PLOY_CONTROLLER", "http://localhost:8081/v1")

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
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
			if err := handleARFCommand(os.Args); err != nil {
				fmt.Printf("ARF command failed: %v\n", err)
				os.Exit(1)
			}
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