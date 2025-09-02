package deploy

import (
	"flag"
	"fmt"
	"path/filepath"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

func PushCmd(args []string, controllerURL string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	app := fs.String("a", filepath.Base(utils.MustGetwd()), "app name")
	lane := fs.String("lane", "", "lane override (A..G)")
	main := fs.String("main", "", "Java main class for lane C")
	sha := fs.String("sha", "", "git sha to annotate")
	bluegreen := fs.Bool("blue-green", false, "use blue-green deployment")
	fs.Parse(args)

	// Check if blue-green deployment is requested
	if *bluegreen {
		fmt.Printf("🔄 Starting blue-green deployment for %s...\n", *app)
		fmt.Println("Blue-green deployments are handled via the bluegreen command")
		fmt.Printf("Use: ploy bluegreen deploy %s\n", *app)
		return
	}

	// Display deployment info
	fmt.Printf("🚀 Deploying %s to %s.ployd.app...\n", *app, *app)

	// Use app-specific deployment (no platform logic, simplified)
	result, err := DeployApp(*app, *lane, *main, *sha, false)
	if err != nil {
		fmt.Printf("❌ Deployment failed: %v\n", err)
		return
	}

	// Display result
	if result.Success {
		fmt.Printf("✅ Successfully deployed to %s\n", result.URL)
		if result.DeploymentID != "" {
			fmt.Printf("📋 Deployment ID: %s\n", result.DeploymentID)
		}
	} else {
		fmt.Printf("❌ %s\n", result.Message)
	}
}

func OpenCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy open <app>")
		return
	}
	app := args[0]
	domain := utils.DefaultDomainFor(app)
	fmt.Println("Opening:", domain)
	utils.OpenURL("https://" + domain)
}