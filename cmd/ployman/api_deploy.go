package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

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

	branch := determineBranch()
	fmt.Printf("Deploying branch '%s' to %s via Ansible...\n", branch, targetHost)

	iacPath := findIacPath()
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

	branch := determineBranch()
	fmt.Printf("Deploying branch '%s' to %s via Ansible...\n", branch, targetHost)

	iacPath := findIacPath()
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

// determineBranch determines which branch to deploy based on environment and git state
func determineBranch() string {
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

	return branch
}

// findIacPath finds the repository root where iac/dev directory should be
func findIacPath() string {
	// Try common paths relative to where ployman might be run from
	possiblePaths := []string{
		"iac/dev",                        // Running from repo root
		"../iac/dev",                     // Running from cmd/ployman or bin
		"../../iac/dev",                  // Running from deeper directory
		"/Users/vk/@iw2rmb/ploy/iac/dev", // Absolute fallback path
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path + "/playbooks/api.yml"); err == nil {
			return path
		}
	}

	return ""
}
