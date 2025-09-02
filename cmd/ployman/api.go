package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// DeploymentState tracks active deployment information
type DeploymentState struct {
	StartTime    time.Time `json:"start_time"`
	PID          int       `json:"pid"`
	TargetBranch string    `json:"target_branch"`
	TargetHost   string    `json:"target_host"`
	Timeout      int       `json:"timeout_minutes"`
	ExpectedCommit string  `json:"expected_commit"`
	LogFile      string    `json:"log_file"`
}

// ApiCmd handles API management commands
func ApiCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("API management commands:")
		fmt.Println("  ployman api deploy              Deploy latest code changes (runs in background)")
		fmt.Println("  ployman api status              Check API deployment status")
		fmt.Println("  ployman api rollback <version>  Rollback to specific version")
		fmt.Println("")
		fmt.Println("Deploy flags:")
		fmt.Println("  --foreground       Wait for deployment to complete (instead of background)")
		fmt.Println("  --monitor          Monitor deployment progress with live output")
		fmt.Println("  --timeout <mins>   Deployment timeout in minutes (default: 10, max: 10)")
		fmt.Println("")
		fmt.Println("Note: Deployments run in background by default to avoid timeout issues.")
		fmt.Println("      Use 'ployman api status' to check deployment progress.")
		fmt.Println("")
		fmt.Println("Environment variables:")
		fmt.Println("  TARGET_HOST        VPS host for deployment (required)")
		fmt.Println("  DEPLOY_BRANCH      Git branch to deploy (default: current branch or 'main')")
		fmt.Println("  PLOY_CONTROLLER    API endpoint (default: https://api.dev.ployman.app/v1)")
		return
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "deploy":
		runApiDeploy(subArgs)
	case "status":
		runApiStatus(subArgs)
	case "rollback":
		runApiRollback(subArgs)
	default:
		fmt.Printf("Unknown api command: %s\n", subcommand)
		fmt.Println("Run 'ployman api' for usage information")
	}
}

func runApiDeploy(args []string) {
	// Parse flags for deploy command
	deployCmd := flag.NewFlagSet("deploy", flag.ExitOnError)
	foreground := deployCmd.Bool("foreground", false, "Run deployment in foreground (wait for completion)")
	timeoutMinutes := deployCmd.Int("timeout", 10, "Deployment timeout in minutes (max 10)")
	monitor := deployCmd.Bool("monitor", false, "Monitor deployment progress in background")
	// Legacy background flag (now default behavior)
	legacyBackground := deployCmd.Bool("background", false, "[DEPRECATED] Run in background (now default)")
	
	deployCmd.Parse(args)
	
	// Validate timeout
	if *timeoutMinutes < 1 || *timeoutMinutes > 10 {
		fmt.Println("Error: timeout must be between 1 and 10 minutes")
		return
	}
	
	fmt.Println("Deploying latest API version...")
	
	// Always run full deployment to ensure latest code changes are deployed
	// This includes: git pull, build, upload to SeaweedFS, and Nomad deployment
	fmt.Println("Running full deployment to ensure latest code changes...")
	
	// Legacy background flag warning
	if *legacyBackground {
		fmt.Println("Note: --background flag is deprecated (deployments run in background by default)")
	}
	
	// Default to background mode unless foreground is explicitly requested
	runInBackground := !*foreground
	
	if runInBackground {
		if !*foreground {
			fmt.Println("Running in background mode (use --foreground to wait for completion)")
		}
		runAnsibleDeploymentBackground(*timeoutMinutes, *monitor)
	} else {
		fmt.Println("Running in foreground mode (waiting for completion)")
		runAnsibleDeployment(*timeoutMinutes)
	}
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

func runAnsibleDeployment(timeoutMinutes int) {
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
	
	// Create context with configurable timeout
	timeout := time.Duration(timeoutMinutes) * time.Minute
	fmt.Printf("Setting deployment timeout to %d minutes\n", timeoutMinutes)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
	
	// Save deployment state before starting
	deploymentState := &DeploymentState{
		StartTime:      time.Now(),
		PID:            0, // Will be updated after starting
		TargetBranch:   branch,
		TargetHost:     targetHost,
		Timeout:        timeoutMinutes,
		ExpectedCommit: getCurrentCommit(),
		LogFile:        "", // Foreground deployment doesn't use log file
	}
	
	fmt.Printf("Running Ansible playbook (%d-minute timeout)...\n", timeoutMinutes)
	
	// Start the command and get PID
	if err := ansibleCmd.Start(); err != nil {
		fmt.Printf("Error starting deployment: %v\n", err)
		return
	}
	
	// Update PID and save deployment state
	deploymentState.PID = ansibleCmd.Process.Pid
	saveDeploymentState(deploymentState)
	
	// Wait for completion
	if err := ansibleCmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Printf("Ansible deployment timed out after %d minutes\n", timeoutMinutes)
			fmt.Println("Tip: Use --background flag to run deployment in background")
			fmt.Println("     or --timeout flag to increase timeout (max 10 minutes)")
		} else {
			fmt.Printf("Ansible deployment failed: %v\n", err)
		}
		fmt.Println("Tip: Ensure you have SSH access to the target host and all required Ansible dependencies")
		return
	}
	
	fmt.Println("Ansible deployment completed successfully!")
	
	// Clear deployment state on successful completion
	clearDeploymentState()
}

func runAnsibleDeploymentBackground(timeoutMinutes int, monitor bool) {
	fmt.Println("Starting deployment in background...")
	
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
	
	// Create log file for background deployment
	logFile := fmt.Sprintf("/tmp/ploy-deploy-%s.log", time.Now().Format("20060102-150405"))
	log, err := os.Create(logFile)
	if err != nil {
		fmt.Printf("Error creating log file: %v\n", err)
		return
	}
	defer log.Close()
	
	// Create context with configurable timeout
	timeout := time.Duration(timeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	// Execute Ansible playbook in background
	ansibleCmd := exec.CommandContext(ctx, "ansible-playbook",
		"playbooks/api.yml",
		"-e", fmt.Sprintf("target_host=%s", targetHost),
		"-e", fmt.Sprintf("deploy_branch=%s", branch),
	)
	
	// Set working directory to iac/dev
	ansibleCmd.Dir = iacPath
	
	// Prepare deployment state
	deploymentState := &DeploymentState{
		StartTime:      time.Now(),
		PID:            0, // Will be updated after starting
		TargetBranch:   branch,
		TargetHost:     targetHost,
		Timeout:        timeoutMinutes,
		ExpectedCommit: getCurrentCommit(),
		LogFile:        logFile,
	}
	
	if monitor {
		// For monitoring, use pipes to capture output
		stdout, err := ansibleCmd.StdoutPipe()
		if err != nil {
			fmt.Printf("Error creating stdout pipe: %v\n", err)
			return
		}
		
		stderr, err := ansibleCmd.StderrPipe()
		if err != nil {
			fmt.Printf("Error creating stderr pipe: %v\n", err)
			return
		}
		
		// Start the command
		if err := ansibleCmd.Start(); err != nil {
			fmt.Printf("Error starting deployment: %v\n", err)
			return
		}
		
		// Update PID and save deployment state
		deploymentState.PID = ansibleCmd.Process.Pid
		saveDeploymentState(deploymentState)
		
		fmt.Printf("Deployment started in background (PID: %d)\n", ansibleCmd.Process.Pid)
		fmt.Printf("Monitoring deployment progress (timeout: %d minutes)...\n", timeoutMinutes)
		fmt.Printf("Log file: %s\n\n", logFile)
		
		// Monitor output in real-time
		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					output := string(buf[:n])
					fmt.Print(output)
					log.WriteString(output)
				}
				if err != nil {
					break
				}
			}
		}()
		
		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := stderr.Read(buf)
				if n > 0 {
					output := string(buf[:n])
					fmt.Print(output)
					log.WriteString(output)
				}
				if err != nil {
					break
				}
			}
		}()
		
		// Wait for completion
		if err := ansibleCmd.Wait(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Printf("\nDeployment timed out after %d minutes\n", timeoutMinutes)
			} else {
				fmt.Printf("\nDeployment failed: %v\n", err)
			}
		} else {
			fmt.Println("\nDeployment completed successfully!")
			// Check deployment status
			checkDeploymentStatus(getControllerURL())
		}
	} else {
		// For background without monitoring, redirect to log file
		ansibleCmd.Stdout = log
		ansibleCmd.Stderr = log
		
		// Set environment variables that might be needed
		ansibleCmd.Env = os.Environ()
		
		// Start the command in background
		if err := ansibleCmd.Start(); err != nil {
			fmt.Printf("Error starting deployment: %v\n", err)
			return
		}
		
		// Update PID and save deployment state
		deploymentState.PID = ansibleCmd.Process.Pid
		saveDeploymentState(deploymentState)
		
		fmt.Printf("Deployment started in background (PID: %d)\n", ansibleCmd.Process.Pid)
		fmt.Printf("Timeout: %d minutes\n", timeoutMinutes)
		fmt.Printf("Log file: %s\n", logFile)
		fmt.Println("")
		fmt.Println("To monitor progress:")
		fmt.Printf("  tail -f %s\n", logFile)
		fmt.Println("")
		fmt.Println("To check deployment status:")
		fmt.Println("  ployman api status")
		fmt.Println("")
		fmt.Println("Note: The deployment will continue running even if you close this terminal.")
		
		// Start a goroutine to wait for completion and log the result
		go func() {
			if err := ansibleCmd.Wait(); err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					log.WriteString(fmt.Sprintf("\n\nDeployment timed out after %d minutes\n", timeoutMinutes))
				} else {
					log.WriteString(fmt.Sprintf("\n\nDeployment failed: %v\n", err))
				}
			} else {
				log.WriteString("\n\nDeployment completed successfully!\n")
			}
		}()
	}
}

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

func getDeploymentStateFile() string {
	return "/tmp/ploy-deployment-state.json"
}

func saveDeploymentState(state *DeploymentState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getDeploymentStateFile(), data, 0644)
}

func loadDeploymentState() (*DeploymentState, error) {
	data, err := os.ReadFile(getDeploymentStateFile())
	if err != nil {
		return nil, err
	}
	
	var state DeploymentState
	err = json.Unmarshal(data, &state)
	return &state, err
}

func clearDeploymentState() error {
	return os.Remove(getDeploymentStateFile())
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	
	// Send signal 0 to check if process exists without affecting it
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func getCurrentCommit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
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