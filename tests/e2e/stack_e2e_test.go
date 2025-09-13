//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

// TestStackReadiness verifies the Dev API stack is up and responsive.
func TestStackReadiness(t *testing.T) {
	controller := os.Getenv("PLOY_CONTROLLER")
	if controller == "" {
		t.Skip("PLOY_CONTROLLER not set; skipping stack readiness check")
	}
	mustGET := func(path string) {
		url := fmt.Sprintf("%s%s", controller, path)
		req, _ := http.NewRequest("GET", url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			t.Fatalf("GET %s status=%d body=%s", path, resp.StatusCode, string(body))
		}
	}
	mustGET("/health")
	mustGET("/version")
}
