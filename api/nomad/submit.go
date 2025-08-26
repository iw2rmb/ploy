package nomad

import (
	"fmt"
	"os"
	"os/exec"
)

// Submit submits a job with basic monitoring (backward compatible)
func Submit(jobPath string) error {
	// Validate job first
	if err := ValidateJob(jobPath); err != nil {
		return fmt.Errorf("job validation failed: %w", err)
	}
	
	// Use robust submission with monitoring
	// Default to 2 instances and 3 retries
	return RobustSubmit(jobPath, 2, 3)
}

// SubmitBasic performs basic job submission without monitoring (original behavior)
func SubmitBasic(jobPath string) error {
	cmd := exec.Command("nomad", "job", "run", jobPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
