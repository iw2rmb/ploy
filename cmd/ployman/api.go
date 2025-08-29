package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ApiCmd handles API management commands
func ApiCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("API management commands:")
		fmt.Println("  ployman api deploy              Deploy latest code changes via full build")
		fmt.Println("  ployman api rollback <version>  Rollback to specific version")
		fmt.Println("")
		fmt.Println("Environment variables:")
		fmt.Println("  TARGET_HOST        VPS host for deployment (required)")
		fmt.Println("  DEPLOY_BRANCH      Git branch to deploy (default: current branch or 'main')")
		fmt.Println("  PLOY_CONTROLLER    API endpoint for rollback (default: https://api.dev.ployman.app/v1)")
		return
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "deploy":
		runApiDeploy(subArgs)
	case "rollback":
		runApiRollback(subArgs)
	default:
		fmt.Printf("Unknown api command: %s\n", subcommand)
		fmt.Println("Run 'ployman api' for usage information")
	}
}

func runApiDeploy(args []string) {
	fmt.Println("Deploying latest API version...")
	
	// Always run full deployment to ensure latest code changes are deployed
	// This includes: git pull, build, upload to SeaweedFS, and Nomad deployment
	fmt.Println("Running full deployment to ensure latest code changes...")
	runAnsibleDeployment()
}

func runApiRollback(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ployman api rollback <version>")
		return
	}
	
	targetVersion := args[0]
	controllerURL := getControllerURL()
	
	// Call rollback endpoint
	rollbackURL := fmt.Sprintf("%s/rollback", controllerURL)
	
	payload := map[string]string{
		"version": targetVersion,
		"reason":  "Manual rollback via ployman CLI",
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error: failed to create request: %v\n", err)
		return
	}
	
	req, err := http.NewRequest("POST", rollbackURL, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Rollback failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}
	
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Rollback failed with status %d: %s\n", resp.StatusCode, string(body))
		return
	}
	
	fmt.Printf("Successfully rolled back to version %s\n", targetVersion)
}

func runAnsibleDeployment() {
	fmt.Println("Running Ansible deployment to build and deploy latest code...")
	
	// Get target host from environment
	targetHost := os.Getenv("TARGET_HOST")
	if targetHost == "" {
		fmt.Println("Error: TARGET_HOST environment variable not set")
		fmt.Println("Please set TARGET_HOST to your VPS IP address")
		return
	}
	
	// Check if Ansible is installed locally
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		fmt.Println("Error: ansible-playbook not found in PATH")
		fmt.Println("Please install Ansible locally: brew install ansible")
		return
	}
	
	// Determine which branch to deploy
	var branch string
	
	// First check if DEPLOY_BRANCH is explicitly set
	branch = os.Getenv("DEPLOY_BRANCH")
	
	if branch == "" {
		// Check if we're in a git repository
		cmd := exec.Command("git", "rev-parse", "--git-dir")
		cmd.Stderr = nil // Suppress error output
		if err := cmd.Run(); err == nil {
			// We're in a git repo, try to get the current branch
			cmd = exec.Command("git", "branch", "--show-current")
			output, err := cmd.Output()
			if err == nil {
				branch = strings.TrimSpace(string(output))
			}
			if branch == "" {
				// Might be in detached HEAD state, try to get commit hash
				cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
				if output, err := cmd.Output(); err == nil {
					branch = strings.TrimSpace(string(output))
					fmt.Printf("Note: In detached HEAD state, using commit %s\n", branch)
				}
			}
		}
	}
	
	// Default to main if no branch determined
	if branch == "" {
		branch = "main"
		fmt.Println("Note: Not in a git repository, defaulting to 'main' branch")
		fmt.Println("Tip: Set DEPLOY_BRANCH environment variable to deploy a specific branch")
	}
	
	fmt.Printf("Deploying branch '%s' to %s via Ansible...\n", branch, targetHost)
	
	// Find the repository root (where iac/dev directory should be)
	// First try to find it relative to the current working directory
	var iacPath string
	
	// Try common paths relative to where ployman might be run from
	possiblePaths := []string{
		"iac/dev",                                    // Running from repo root
		"../iac/dev",                                 // Running from cmd/ployman or bin
		"../../iac/dev",                              // Running from deeper directory
		"/Users/vk/@iw2rmb/ploy/iac/dev",            // Absolute fallback path
	}
	
	for _, path := range possiblePaths {
		if _, err := os.Stat(path + "/playbooks/api.yml"); err == nil {
			iacPath = path
			break
		}
	}
	
	if iacPath == "" {
		fmt.Println("Error: Could not find iac/dev/playbooks/api.yml")
		fmt.Println("Please run this command from the ploy repository root or set up the correct path")
		return
	}
	
	fmt.Printf("Using Ansible playbooks from: %s\n", iacPath)
	
	// Create context with 5-minute timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	
	// Execute Ansible playbook locally with timeout
	ansibleCmd := exec.CommandContext(ctx, "ansible-playbook",
		"playbooks/api.yml",
		"-e", fmt.Sprintf("target_host=%s", targetHost),
		"-e", fmt.Sprintf("deploy_branch=%s", branch),
	)
	
	// Set working directory to iac/dev
	ansibleCmd.Dir = iacPath
	ansibleCmd.Stdout = os.Stdout
	ansibleCmd.Stderr = os.Stderr
	
	// Set environment variables that might be needed
	ansibleCmd.Env = os.Environ()
	
	fmt.Println("Running Ansible playbook (5-minute timeout)...")
	if err := ansibleCmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Printf("Ansible deployment timed out after 5 minutes\n")
		} else {
			fmt.Printf("Ansible deployment failed: %v\n", err)
		}
		fmt.Println("Tip: Ensure you have SSH access to the target host and all required Ansible dependencies")
		return
	}
	
	fmt.Println("Ansible deployment completed successfully!")
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