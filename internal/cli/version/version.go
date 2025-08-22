package version

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ploy/ploy/internal/version"
)

// VersionCmd handles version command
func VersionCmd(args []string, controllerURL string) {
	if len(args) > 0 && args[0] == "--detailed" {
		showDetailedVersion(controllerURL)
	} else {
		showVersion(controllerURL)
	}
}

func showVersion(controllerURL string) {
	// Show CLI version
	fmt.Printf("Ploy CLI version: %s\n", version.Short())
	
	// Try to get controller version
	resp, err := http.Get(fmt.Sprintf("%s/version", controllerURL))
	if err == nil {
		defer resp.Body.Close()
		var result map[string]string
		if json.NewDecoder(resp.Body).Decode(&result) == nil {
			fmt.Printf("Controller version: %s\n", result["version"])
		}
	}
}

func showDetailedVersion(controllerURL string) {
	// Show detailed CLI version
	fmt.Println("Ploy CLI:")
	info := version.Get()
	fmt.Printf("  Version: %s\n", info.Version)
	fmt.Printf("  Commit: %s\n", info.GitCommit)
	fmt.Printf("  Branch: %s\n", info.GitBranch)
	fmt.Printf("  Built: %s\n", info.BuildTime)
	fmt.Printf("  Go Version: %s\n", info.GoVersion)
	fmt.Printf("  Platform: %s\n", info.Platform)
	
	// Try to get controller version
	resp, err := http.Get(fmt.Sprintf("%s/version/detailed", controllerURL))
	if err == nil {
		defer resp.Body.Close()
		var result version.Info
		if json.NewDecoder(resp.Body).Decode(&result) == nil {
			fmt.Println("\nPloy Controller:")
			fmt.Printf("  Version: %s\n", result.Version)
			fmt.Printf("  Commit: %s\n", result.GitCommit)
			fmt.Printf("  Branch: %s\n", result.GitBranch)
			fmt.Printf("  Built: %s\n", result.BuildTime)
			fmt.Printf("  Uptime: %s\n", result.Uptime)
		}
	}
}