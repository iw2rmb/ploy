package bluegreen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// BlueGreenCmd handles blue-green deployment commands
func BlueGreenCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		printUsage()
		return
	}

	command := args[0]
	switch command {
	case "deploy":
		handleDeploy(args[1:], controllerURL)
	case "status":
		handleStatus(args[1:], controllerURL)
	case "shift":
		handleShift(args[1:], controllerURL)
	case "auto-shift":
		handleAutoShift(args[1:], controllerURL)
	case "complete":
		handleComplete(args[1:], controllerURL)
	case "rollback":
		handleRollback(args[1:], controllerURL)
	default:
		fmt.Printf("Unknown blue-green command: %s\n", command)
		printUsage()
	}
}

func printUsage() {
	fmt.Println(`Usage: ploy bluegreen <command> [options]

Commands:
  deploy <app> <version>           Start a new blue-green deployment
  status <app>                     Show blue-green deployment status
  shift <app> <weight>            Manually shift traffic (0-100%)
  auto-shift <app>                Start automatic traffic shifting
  complete <app>                  Complete the blue-green deployment
  rollback <app>                  Rollback the blue-green deployment

Examples:
  ploy bluegreen deploy myapp v1.2.3
  ploy bluegreen status myapp
  ploy bluegreen shift myapp 25
  ploy bluegreen auto-shift myapp
  ploy bluegreen complete myapp
  ploy bluegreen rollback myapp`)
}

// handleDeploy starts a new blue-green deployment
func handleDeploy(args []string, controllerURL string) {
	if len(args) < 2 {
		fmt.Println("Error: App name and version are required")
		fmt.Println("Usage: ploy bluegreen deploy <app> <version>")
		return
	}

	appName := args[0]
	version := args[1]

	fmt.Printf("🚀 Starting blue-green deployment for %s version %s...\n", appName, version)

	// Prepare request body
	reqBody := map[string]string{
		"version": version,
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Make request to controller
	url := fmt.Sprintf("%s/apps/%s/deploy/blue-green", controllerURL, appName)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("Error: Failed to start blue-green deployment: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 201 {
		fmt.Printf("Error: Blue-green deployment failed: %s\n", string(body))
		return
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	fmt.Println("✅ Blue-green deployment started successfully")
	if deployment, ok := result["deployment"].(map[string]interface{}); ok {
		fmt.Printf("Status: %v\n", deployment["status"])
		fmt.Printf("Active Color: %v\n", deployment["active_color"])
		fmt.Printf("Blue Weight: %.0f%%\n", deployment["blue_weight"])
		fmt.Printf("Green Weight: %.0f%%\n", deployment["green_weight"])
	}
}

// handleStatus shows the current blue-green deployment status
func handleStatus(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("Error: App name is required")
		fmt.Println("Usage: ploy bluegreen status <app>")
		return
	}

	appName := args[0]

	// Make request to controller
	url := fmt.Sprintf("%s/apps/%s/blue-green/status", controllerURL, appName)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error: Failed to get deployment status: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		fmt.Printf("Error: Failed to get deployment status: %s\n", string(body))
		return
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if deployment, ok := result["deployment"].(map[string]interface{}); ok {
		fmt.Printf("📊 Blue-Green Deployment Status for %s\n\n", appName)
		
		fmt.Printf("Status: %v\n", deployment["status"])
		fmt.Printf("Active Color: %v\n", deployment["active_color"])
		
		if blueVersion, ok := deployment["blue_version"]; ok && blueVersion != nil {
			fmt.Printf("Blue Version: %v\n", blueVersion)
		}
		if greenVersion, ok := deployment["green_version"]; ok && greenVersion != nil {
			fmt.Printf("Green Version: %v\n", greenVersion)
		}
		
		fmt.Printf("Traffic Distribution:\n")
		fmt.Printf("  🔵 Blue:  %.0f%%\n", deployment["blue_weight"])
		fmt.Printf("  🟢 Green: %.0f%%\n", deployment["green_weight"])
		
		if lastShift, ok := deployment["last_shift_time"]; ok {
			if shiftTime, err := time.Parse(time.RFC3339, lastShift.(string)); err == nil {
				fmt.Printf("Last Shift: %s\n", shiftTime.Format("2006-01-02 15:04:05"))
			}
		}
	}
}

// handleShift manually shifts traffic between blue and green deployments
func handleShift(args []string, controllerURL string) {
	if len(args) < 2 {
		fmt.Println("Error: App name and target weight are required")
		fmt.Println("Usage: ploy bluegreen shift <app> <weight>")
		return
	}

	appName := args[0]
	weightStr := args[1]
	
	weight, err := strconv.Atoi(weightStr)
	if err != nil || weight < 0 || weight > 100 {
		fmt.Println("Error: Target weight must be a number between 0 and 100")
		return
	}

	fmt.Printf("⚖️ Shifting traffic to %d%% for %s...\n", weight, appName)

	// Prepare request body
	reqBody := map[string]int{
		"target_weight": weight,
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Make request to controller
	url := fmt.Sprintf("%s/apps/%s/blue-green/shift", controllerURL, appName)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("Error: Failed to shift traffic: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		fmt.Printf("Error: Traffic shift failed: %s\n", string(body))
		return
	}

	fmt.Printf("✅ Traffic shifted to %d%% successfully\n", weight)
}

// handleAutoShift starts automatic traffic shifting using the default strategy
func handleAutoShift(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("Error: App name is required")
		fmt.Println("Usage: ploy bluegreen auto-shift <app>")
		return
	}

	appName := args[0]

	fmt.Printf("🔄 Starting automatic traffic shifting for %s...\n", appName)

	// Make request to controller
	url := fmt.Sprintf("%s/apps/%s/blue-green/auto-shift", controllerURL, appName)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("Error: Failed to start auto shift: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		fmt.Printf("Error: Auto shift failed: %s\n", string(body))
		return
	}

	fmt.Println("✅ Automatic traffic shifting started")
	fmt.Println("Traffic will be gradually shifted: 0% → 10% → 25% → 50% → 75% → 100%")
	fmt.Println("Monitor progress with: ploy bluegreen status " + appName)
}

// handleComplete completes the blue-green deployment
func handleComplete(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("Error: App name is required")
		fmt.Println("Usage: ploy bluegreen complete <app>")
		return
	}

	appName := args[0]

	fmt.Printf("✅ Completing blue-green deployment for %s...\n", appName)

	// Make request to controller
	url := fmt.Sprintf("%s/apps/%s/blue-green/complete", controllerURL, appName)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("Error: Failed to complete deployment: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		fmt.Printf("Error: Complete deployment failed: %s\n", string(body))
		return
	}

	fmt.Println("✅ Blue-green deployment completed successfully")
	fmt.Println("New version is now serving 100% of traffic")
	fmt.Println("Old deployment will be cleaned up automatically")
}

// handleRollback rolls back the blue-green deployment
func handleRollback(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("Error: App name is required")
		fmt.Println("Usage: ploy bluegreen rollback <app>")
		return
	}

	appName := args[0]

	fmt.Printf("🔄 Rolling back blue-green deployment for %s...\n", appName)

	// Ask for confirmation
	fmt.Print("Are you sure you want to rollback? This will route all traffic back to the previous version. (y/N): ")
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		fmt.Println("Rollback cancelled")
		return
	}

	// Make request to controller
	url := fmt.Sprintf("%s/apps/%s/blue-green/rollback", controllerURL, appName)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("Error: Failed to rollback deployment: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		fmt.Printf("Error: Rollback failed: %s\n", string(body))
		return
	}

	fmt.Println("✅ Blue-green deployment rolled back successfully")
	fmt.Println("Traffic has been routed back to the previous version")
}