package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func runApiStatus(args []string) {
	controllerURL := getControllerURL()

	fmt.Println("Checking API deployment status...")
	fmt.Println("")

	// Check for active deployment first
	deploymentState, err := loadDeploymentState()
	if err == nil {
		// Deployment state exists, check if still active
		if isProcessRunning(deploymentState.PID) {
			// Deployment is still running
			elapsed := time.Since(deploymentState.StartTime)
			timeoutDuration := time.Duration(deploymentState.Timeout) * time.Minute
			remaining := timeoutDuration - elapsed

			fmt.Printf("🔄 Deployment in progress (PID: %d)\n", deploymentState.PID)
			fmt.Printf("Branch: %s\n", deploymentState.TargetBranch)
			fmt.Printf("Target: %s\n", deploymentState.TargetHost)
			fmt.Printf("Started: %s (%s ago)\n", deploymentState.StartTime.Format("15:04:05"), elapsed.Round(time.Second))

			if remaining > 0 {
				fmt.Printf("Timeout: %s remaining\n", remaining.Round(time.Second))
			} else {
				fmt.Printf("⚠️  Deployment has exceeded timeout (%d minutes)\n", deploymentState.Timeout)
			}

			if deploymentState.LogFile != "" {
				fmt.Printf("Log file: %s\n", deploymentState.LogFile)
				fmt.Println("")
				fmt.Println("To monitor progress:")
				fmt.Printf("  tail -f %s\n", deploymentState.LogFile)
			}

			fmt.Println("")
			fmt.Printf("Progress: Deployment started %s ago\n", elapsed.Round(time.Second))
			return
		} else {
			// Process is no longer running, check deployment completion
			fmt.Printf("📋 Previous deployment completed (PID %d no longer running)\n", deploymentState.PID)

			// Check if deployment was successful by comparing versions
			if deploymentState.ExpectedCommit != "" {
				if checkVersionMatch(controllerURL, deploymentState.ExpectedCommit) {
					fmt.Printf("✅ Deployment completed successfully\n")
					fmt.Printf("Branch '%s' deployed to %s\n", deploymentState.TargetBranch, deploymentState.TargetHost)
					clearDeploymentState() // Clean up successful deployment state
				} else {
					fmt.Printf("⚠️  Deployment may have failed - version mismatch\n")
					fmt.Printf("Expected commit: %s\n", deploymentState.ExpectedCommit)
				}
			}
			fmt.Println("")
		}
	}

	// Check current API status
	versionURL := fmt.Sprintf("%s/version", controllerURL)
	req, err := http.NewRequest("GET", versionURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ API is not responding: %v\n", err)
		fmt.Println("")
		fmt.Println("Troubleshooting:")
		fmt.Println("  1. Check if deployment is still running: ps aux | grep ansible-playbook")
		fmt.Println("  2. Check deployment logs: tail -f /tmp/ploy-deploy-*.log")
		fmt.Println("  3. SSH to VPS and check Nomad: ssh root@$TARGET_HOST")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			var versionInfo map[string]interface{}
			if err := json.Unmarshal(body, &versionInfo); err == nil {
				fmt.Println("✅ API is running and healthy")
				fmt.Println("")
				if version, ok := versionInfo["version"]; ok {
					fmt.Printf("Version: %v\n", version)
				}
				if buildTime, ok := versionInfo["build_time"]; ok {
					fmt.Printf("Build Time: %v\n", buildTime)
				}
				if gitCommit, ok := versionInfo["git_commit"]; ok {
					fmt.Printf("Git Commit: %v\n", gitCommit)
				}
			} else {
				fmt.Printf("✅ API is responding (status: %d)\n", resp.StatusCode)
			}
		}
	} else {
		fmt.Printf("⚠️  API returned status %d\n", resp.StatusCode)
	}

	// Check health endpoint
	healthURL := fmt.Sprintf("%s/health", controllerURL)
	req, err = http.NewRequest("GET", healthURL, nil)
	if err == nil {
		resp, err = client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Println("Health check: ✅ Healthy")
			} else {
				fmt.Printf("Health check: ⚠️  Status %d\n", resp.StatusCode)
			}
		}
	}

	fmt.Println("")
	fmt.Printf("API Endpoint: %s\n", controllerURL)
}

func checkVersionMatch(controllerURL, expectedCommit string) bool {
	versionURL := fmt.Sprintf("%s/version", controllerURL)

	req, err := http.NewRequest("GET", versionURL, nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var versionInfo map[string]interface{}
	if err := json.Unmarshal(body, &versionInfo); err != nil {
		return false
	}

	if gitCommit, ok := versionInfo["git_commit"]; ok {
		if commitStr, ok := gitCommit.(string); ok {
			// Compare full commit hash or shortened versions
			if commitStr == expectedCommit {
				return true
			}
			// Check if either is a prefix of the other (for short vs long commit hashes)
			if len(commitStr) > len(expectedCommit) && strings.HasPrefix(commitStr, expectedCommit) {
				return true
			}
			if len(expectedCommit) > len(commitStr) && strings.HasPrefix(expectedCommit, commitStr) {
				return true
			}
		}
	}

	return false
}

func checkDeploymentStatus(controllerURL string) {
	versionURL := fmt.Sprintf("%s/version", controllerURL)

	req, err := http.NewRequest("GET", versionURL, nil)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("API is responding normally")
	}
}
