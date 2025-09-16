package mods

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// waitForStepContaining polls the controller status for the given MOD_ID until a step
// message contains the given substring (case-sensitive) or an error condition occurs.
// Returns nil when the substring is observed, or an error if an error step is detected or timeout elapses.
func waitForStepContaining(controller, modID, phase, contains string, timeout time.Duration) error {
	if controller == "" || modID == "" || contains == "" {
		return fmt.Errorf("invalid wait parameters")
	}
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimRight(controller, "/") + "/mods/" + modID + "/status"
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil && resp != nil && resp.Body != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			// Minimal JSON scan to avoid full struct; look for our contains string and phase/level hints
			s := string(body)
			if strings.Contains(s, contains) {
				return nil
			}
			// Detect explicit job failed message in this phase
			if strings.Contains(s, "\"phase\":\""+phase+"\"") && strings.Contains(strings.ToLower(s), "job failed") {
				return fmt.Errorf("job in phase %s reported failure", phase)
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for event: %s", contains)
}
