package build

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// getJobLogsSnippet fetches recent logs for a job via the nomad-job-manager wrapper.
func getJobLogsSnippet(job string, lines int) string {
	if job == "" {
		return ""
	}
	// Resolve running allocation ID first
	allocID := func() string {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/opt/hashicorp/bin/nomad-job-manager.sh", "running-alloc", "--job", job)
		out, err := cmd.CombinedOutput()
		if err == nil {
			s := strings.TrimSpace(string(out))
			// Extract UUID-like alloc ID from noisy output
			re := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
			if m := re.FindString(s); m != "" {
				return m
			}
			// Fallback: last line
			if i := strings.LastIndex(s, "\n"); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
			}
			if s != "" {
				return s
			}
		}
		return ""
	}()
	if allocID == "" {
		// Fallback: show allocs (human) for visibility
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/opt/hashicorp/bin/nomad-job-manager.sh", "allocs", "--job", job, "--format", "human")
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	// Fetch logs for resolved alloc
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/opt/hashicorp/bin/nomad-job-manager.sh", "logs", "--alloc-id", allocID, "--lines", fmt.Sprintf("%d", lines))
	out, _ := cmd.CombinedOutput()
	b := out
	if len(b) > 4000 {
		b = b[len(b)-4000:]
	}
	return string(b)
}
