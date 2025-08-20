package nomad

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// SubmitResult contains the result of a job submission
type SubmitResult struct {
	JobID        string
	DeploymentID string
	EvalID       string
	Success      bool
	Message      string
}

// SubmitWithMonitoring submits a Nomad job and monitors its deployment
func SubmitWithMonitoring(jobPath string, timeout time.Duration) (*SubmitResult, error) {
	// Submit the job and capture output
	result, err := submitJob(jobPath)
	if err != nil {
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}
	
	if !result.Success {
		return result, fmt.Errorf("job submission failed: %s", result.Message)
	}
	
	fmt.Printf("Job submitted successfully: ID=%s, Deployment=%s\n", result.JobID, result.DeploymentID)
	
	// Monitor the deployment if we have a deployment ID
	if result.DeploymentID != "" {
		monitor := NewHealthMonitor()
		if err := monitor.MonitorDeployment(result.DeploymentID, timeout); err != nil {
			return result, fmt.Errorf("deployment monitoring failed: %w", err)
		}
	}
	
	return result, nil
}

// submitJob submits a job and parses the output
func submitJob(jobPath string) (*SubmitResult, error) {
	cmd := exec.Command("nomad", "job", "run", jobPath)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try to parse error output
		if len(output) > 0 {
			return &SubmitResult{
				Success: false,
				Message: string(output),
			}, nil
		}
		return nil, fmt.Errorf("command failed: %w", err)
	}
	
	// Parse text output (standard nomad job run output)
	return parseTextOutput(string(output))
}

// parseTextOutput parses the text output from nomad job run
func parseTextOutput(output string) (*SubmitResult, error) {
	result := &SubmitResult{Success: true}
	
	// Parse Job ID
	if match := regexp.MustCompile(`Job ID\s*=\s*"?([^"\s]+)`).FindStringSubmatch(output); len(match) > 1 {
		result.JobID = match[1]
	}
	
	// Parse Deployment ID
	if match := regexp.MustCompile(`Deployment ID\s*=\s*"?([^"\s]+)`).FindStringSubmatch(output); len(match) > 1 {
		result.DeploymentID = match[1]
	}
	
	// Parse Eval ID
	if match := regexp.MustCompile(`Evaluation ID\s*=\s*"?([^"\s]+)`).FindStringSubmatch(output); len(match) > 1 {
		result.EvalID = match[1]
	}
	
	// Check for errors
	if strings.Contains(output, "Error") || strings.Contains(output, "error") {
		result.Success = false
		result.Message = output
	}
	
	return result, nil
}

// SubmitAndWaitHealthy submits a job and waits for it to become healthy
func SubmitAndWaitHealthy(jobPath string, expectedCount int, timeout time.Duration) error {
	// Submit the job
	result, err := submitJob(jobPath)
	if err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
	}
	
	if !result.Success {
		return fmt.Errorf("job submission failed: %s", result.Message)
	}
	
	fmt.Printf("Submitted job %s (deployment: %s)\n", result.JobID, result.DeploymentID)
	
	// Create health monitor
	monitor := NewHealthMonitor()
	
	// If we have a deployment ID, monitor it
	if result.DeploymentID != "" {
		deploymentComplete := make(chan error, 1)
		
		// Monitor deployment in background
		go func() {
			deploymentComplete <- monitor.MonitorDeployment(result.DeploymentID, timeout)
		}()
		
		// Also monitor job health
		healthComplete := make(chan error, 1)
		go func() {
			// Give deployment a moment to start
			time.Sleep(2 * time.Second)
			healthComplete <- monitor.MonitorJobHealth(result.JobID, expectedCount, timeout)
		}()
		
		// Wait for both to complete
		var deploymentErr, healthErr error
		
		select {
		case deploymentErr = <-deploymentComplete:
			healthErr = <-healthComplete
		case healthErr = <-healthComplete:
			deploymentErr = <-deploymentComplete
		}
		
		if deploymentErr != nil {
			return fmt.Errorf("deployment failed: %w", deploymentErr)
		}
		if healthErr != nil {
			return fmt.Errorf("health check failed: %w", healthErr)
		}
		
		return nil
	}
	
	// No deployment ID, just monitor job health
	return monitor.MonitorJobHealth(result.JobID, expectedCount, timeout)
}

// RobustSubmit submits a job with retry logic and comprehensive monitoring
func RobustSubmit(jobPath string, expectedCount int, maxRetries int) error {
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("Submission attempt %d/%d\n", attempt, maxRetries)
		
		err := SubmitAndWaitHealthy(jobPath, expectedCount, 90*time.Second)
		if err == nil {
			fmt.Printf("Job deployed successfully on attempt %d\n", attempt)
			return nil
		}
		
		lastErr = err
		fmt.Printf("Attempt %d failed: %v\n", attempt, err)
		
		// Check if error is retryable
		if !isRetryableError(err) {
			fmt.Printf("Error is not retryable, aborting\n")
			return err
		}
		
		// Wait before retry
		if attempt < maxRetries {
			waitTime := time.Duration(attempt*5) * time.Second
			fmt.Printf("Waiting %v before retry...\n", waitTime)
			time.Sleep(waitTime)
		}
	}
	
	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	errStr := err.Error()
	
	// Non-retryable errors
	nonRetryable := []string{
		"policy enforcement failed",
		"OPA policy",
		"image not found",
		"invalid job specification",
		"constraint",
	}
	
	for _, pattern := range nonRetryable {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return false
		}
	}
	
	// Retryable errors
	retryable := []string{
		"timeout",
		"connection refused",
		"no leader",
		"500",
		"502",
		"503",
		"504",
		"pending",
	}
	
	for _, pattern := range retryable {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}
	
	// Default to retryable for unknown errors
	return true
}

// StreamJobLogs streams logs from a job's allocations
func StreamJobLogs(jobID string, follow bool) error {
	monitor := NewHealthMonitor()
	
	// Get allocations
	allocations, err := monitor.GetJobAllocations(jobID)
	if err != nil {
		return fmt.Errorf("failed to get allocations: %w", err)
	}
	
	// Find a running allocation
	var runningAllocID string
	for _, alloc := range allocations {
		if alloc.ClientStatus == "running" {
			runningAllocID = alloc.ID
			break
		}
	}
	
	if runningAllocID == "" {
		return fmt.Errorf("no running allocation found for job %s", jobID)
	}
	
	// Stream logs
	args := []string{"alloc", "logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, runningAllocID)
	
	cmd := exec.Command("nomad", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	return cmd.Run()
}

// ValidateJob validates a job specification without running it
func ValidateJob(jobPath string) error {
	cmd := exec.Command("nomad", "job", "validate", jobPath)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("validation failed: %s", string(output))
	}
	
	if strings.Contains(string(output), "Job validation successful") {
		return nil
	}
	
	return fmt.Errorf("validation output: %s", string(output))
}

// PlanJob runs nomad job plan to see what changes would be made
func PlanJob(jobPath string) (string, error) {
	cmd := exec.Command("nomad", "job", "plan", jobPath)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Plan can return non-zero for updates, check output
		if strings.Contains(string(output), "Plan result") {
			return string(output), nil
		}
		return "", fmt.Errorf("plan failed: %s", string(output))
	}
	
	return string(output), nil
}