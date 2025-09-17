//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"os"
	"testing"
)

// TestPlatformStatusAndLogsE2E exercises platform status/logs via the Dev API.
// Requires PLOY_CONTROLLER to be set, e.g., https://api.dev.ployman.app/v1
func TestPlatformStatusAndLogsE2E(t *testing.T) {
	base := os.Getenv("PLOY_CONTROLLER")
	if base == "" {
		t.Skip("PLOY_CONTROLLER not set; skipping e2e")
	}

	// status endpoint
	statusURL := fmt.Sprintf("%s/platform/api/status", base)
	resp, err := http.Get(statusURL)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		t.Fatalf("unexpected status code from platform status: %d", resp.StatusCode)
	}

	// logs endpoint (non-fatal snapshot)
	logsURL := fmt.Sprintf("%s/platform/traefik/logs?lines=50", base)
	resp2, err := http.Get(logsURL)
	if err != nil {
		t.Fatalf("logs request failed: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode < 200 || resp2.StatusCode >= 500 {
		t.Fatalf("unexpected status code from platform logs: %d", resp2.StatusCode)
	}
}
