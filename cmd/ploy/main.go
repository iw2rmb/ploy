package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	cliapps "github.com/ploy/ploy/internal/cli-apps"
	clicerts "github.com/ploy/ploy/internal/cli-certs"
	clidebug "github.com/ploy/ploy/internal/cli-debug"
	clideploy "github.com/ploy/ploy/internal/cli-deploy"
	clidomains "github.com/ploy/ploy/internal/cli-domains"
	clienv "github.com/ploy/ploy/internal/cli-env"
	cliui "github.com/ploy/ploy/internal/cli-ui"
	cliutils "github.com/ploy/ploy/internal/cli-utils"
)

var controllerURL = cliutils.Getenv("PLOY_CONTROLLER", "http://localhost:8081/v1")

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "apps":
			cliapps.AppsCmd(os.Args[2:], controllerURL)
		case "push":
			clideploy.PushCmd(os.Args[2:], controllerURL)
		case "open":
			clideploy.OpenCmd(os.Args[2:])
		case "env":
			clienv.EnvCmd(os.Args[2:], controllerURL)
		case "domains":
			clidomains.DomainsCmd(os.Args[2:], controllerURL)
		case "certs":
			clicerts.CertsCmd(os.Args[2:], controllerURL)
		case "debug":
			clidebug.DebugCmd(os.Args[2:], controllerURL)
		case "rollback":
			clidebug.RollbackCmd(os.Args[2:], controllerURL)
		default:
			cliui.Usage()
		}
		return
	}
	p := tea.NewProgram(cliui.Model{})
	if err := p.Start(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}