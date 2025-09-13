package platform

import (
	"flag"
	"fmt"
	"os"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// PushCmd handles platform service deployment to ployman.app domain
func PushCmd(args []string, controllerURL string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	service := fs.String("a", "", "platform service name")
	lane := fs.String("lane", "E", "lane override (default: E for containers)")
	sha := fs.String("sha", "", "git sha to annotate")
	env := fs.String("env", "dev", "target environment (dev, staging, prod)")
	_ = fs.Parse(args)

	// Platform services require explicit service name
	if *service == "" {
		fmt.Println("Error: platform service name required (-a flag)")
		fmt.Println("Example: ployman push -a ploy-api")
		return
	}

	// Display deployment info
	targetDomain := "ployman.app"
	if *env != "prod" {
		targetDomain = fmt.Sprintf("%s.ployman.app", *env)
	}
	fmt.Printf("🚀 Deploying platform service %s to %s.%s...\n", *service, *service, targetDomain)

	// Use platform-specific deployment
	result, err := DeployPlatformService(*service, *env, *sha, *lane)
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

// OpenCmd opens a platform service in the browser
func OpenCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ployman open <service>")
		return
	}
	service := args[0]
	domain := getPlatformDomain(service)
	fmt.Println("Opening platform service:", domain)
	utils.OpenURL("https://" + domain)
}

// getPlatformDomain returns the platform domain for a service
func getPlatformDomain(service string) string {
	// Check environment for platform domain
	platformDomain := os.Getenv("PLOY_PLATFORM_DOMAIN")
	if platformDomain == "" {
		platformDomain = "ployman.app"
	}

	// Check for dev environment
	environment := os.Getenv("PLOY_ENVIRONMENT")
	if environment == "dev" {
		return fmt.Sprintf("%s.dev.%s", service, platformDomain)
	}

	// Production domain
	return fmt.Sprintf("%s.%s", service, platformDomain)
}
