//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestLaneDeployments triggers lane-specific deployments via the shell E2E script
// for all lanes with configured repos (LANE_A_REPO..LANE_G_REPO). It uses the
// Dev API (PLOY_CONTROLLER) and the ploy CLI as per AGENTS.md.
func TestLaneDeployments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	lanes := []struct {
		letter string
		envVar string
	}{
		{"A", "LANE_A_REPO"},
		{"B", "LANE_B_REPO"},
		{"C", "LANE_C_REPO"},
		{"D", "LANE_D_REPO"},
		{"E", "LANE_E_REPO"},
		{"F", "LANE_F_REPO"},
		{"G", "LANE_G_REPO"},
	}

	script := filepath.Join("tests", "lanes", "test-lane-deploy.sh")

	for _, lane := range lanes {
		repo := os.Getenv(lane.envVar)
		if repo == "" {
			t.Logf("%s not set; skipping lane %s", lane.envVar, lane.letter)
			continue
		}

		t.Run("Lane"+lane.letter, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", script)
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env,
				"LANE="+lane.letter,
				"HELLO_APP_REPO="+repo,
			)

			out, err := cmd.CombinedOutput()
			t.Logf("%s output:\n%s", script, string(out))
			if err != nil {
				t.Fatalf("lane %s E2E failed: %v", lane.letter, err)
			}
		})
	}
}
