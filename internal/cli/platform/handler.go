package platform

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// PushCmd handles platform service deployment to ployman.app domain
func PushCmd(args []string, controllerURL string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	app := fs.String("a", filepath.Base(utils.MustGetwd()), "app name")
	lane := fs.String("lane", "", "lane override (A..F)")
	main := fs.String("main", "com.ploy.ordersvc.Main", "Java main class for lane C")
	sha := fs.String("sha", "", "git sha to annotate")
	fs.Parse(args)

	if *sha == "" {
		if v := utils.GitSHA(); v != "" {
			*sha = v
		} else {
			*sha = time.Now().Format("20060102-150405")
		}
	}

	// Mark this as a platform service push
	fmt.Printf("🚀 Deploying platform service %s to ployman.app...\n", *app)

	ign, _ := utils.ReadGitignore(".")
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_ = utils.TarDir(".", pw, ign)
	}()

	// Add platform flag to indicate this should use ployman.app domain
	url := fmt.Sprintf("%s/apps/%s/builds?sha=%s&main=%s&platform=true", 
		controllerURL, *app, *sha, utils.URLQueryEsc(*main))
	if *lane != "" {
		url += "&lane=" + *lane
	}
	
	req, _ := http.NewRequest("POST", url, pr)
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("X-Platform-Service", "true") // Header to indicate platform service
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("push error:", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
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