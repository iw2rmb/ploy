package transflow

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
)

// findFirstAllocID uses the VPS job manager wrapper to list allocations for a job and returns the first ID
func findFirstAllocID(jobName string) string {
	if jobName == "" {
		return ""
	}
	mgr := os.Getenv("NOMAD_JOB_MANAGER")
	if mgr == "" {
		mgr = "/opt/hashicorp/bin/nomad-job-manager.sh"
	}
	if _, err := os.Stat(mgr); err != nil {
		return ""
	}
	cmd := exec.Command(mgr, "allocs", "--job", jobName, "--format", "json")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	var allocs []struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(out.Bytes(), &allocs); err != nil {
		return ""
	}
	if len(allocs) == 0 {
		return ""
	}
	if allocs[0].ID == "" && len(allocs) > 1 {
		return allocs[1].ID
	}
	return allocs[0].ID
}
