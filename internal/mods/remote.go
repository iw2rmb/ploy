package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// executeRemoteMod handles execution via remote controller API
func executeRemoteMod(controllerURL, file string, testMode, verbose, watch bool, output string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Start remote execution and get execution id
	client := &http.Client{Timeout: 10 * time.Second}
	id, err := remoteStart(controllerURL, b, testMode, client)
	if err != nil {
		return err
	}

	// Output selection
	if output == "json" {
		// Print a single JSON line. If --watch is set, attach watch afterwards.
		resp := map[string]any{
			"mod_id":     id,
			"status":     "initializing",
			"status_url": "/v1/mods/" + id + "/status",
			"watch_hint": "ploy mod watch -id " + id,
		}
		b, _ := json.Marshal(resp)
		fmt.Println(string(b))
		if watch {
			// Attach live watch after emitting JSON
			if err := watchMod([]string{"-id", id}, controllerURL); err != nil {
				// Best-effort: do not fail JSON mode if watch cannot attach
				return nil
			}
		}
		return nil
	}

	// Text output: Print the execution id and a watch hint for convenience
	fmt.Printf("Execution ID: %s\n", id)
	// Ensure /v1 prefix for watch compatibility
	fmt.Printf("Watch: ploy mod watch -id %s\n", id)

	// Optional: attach a live watch
	if watch {
		// Use the same base controller URL
		if err := watchMod([]string{"-id", id}, controllerURL); err == nil {
			return nil
		}
		// Fall back to polling flow below if watch fails to attach
	}

	// Poll status
	statusURL := strings.TrimRight(controllerURL, "/") + "/mods/" + id + "/status"
	start := time.Now()
	for {
		time.Sleep(2 * time.Second)
		sresp, err := http.Get(statusURL)
		if err != nil {
			continue
		}
		if sresp.StatusCode != 200 {
			_ = sresp.Body.Close()
			continue
		}

		var st struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			Error  string         `json:"error"`
			Result map[string]any `json:"result"`
		}
		_ = json.NewDecoder(sresp.Body).Decode(&st)
		_ = sresp.Body.Close()

		if verbose {
			fmt.Printf("Status: %s (elapsed %s)\n", st.Status, time.Since(start).Round(time.Second))
		}

		if st.Status == "completed" {
			if arts, ok := st.Result["artifacts"].(map[string]any); ok && len(arts) > 0 {
				fmt.Println("Artifacts:")
				for k, v := range arts {
					fmt.Printf("  %s: %v\n", k, v)
				}
				// Printable download URLs via controller proxy
				base := strings.TrimRight(controllerURL, "/") + "/mods/" + st.ID + "/artifacts/"
				fmt.Println("Download URLs:")
				// Known names
				known := []string{"plan_json", "next_json", "diff_patch"}
				for _, name := range known {
					if _, ok := arts[name]; ok {
						fmt.Printf("  %s: %s%s\n", name, base, name)
					}
				}
			}
			return nil
		}

		if st.Status == "failed" {
			if st.Error != "" {
				return fmt.Errorf("mod failed: %s", st.Error)
			}
			return fmt.Errorf("mod failed")
		}
	}
}

// remoteStart POSTs the run request to the controller and returns the execution id
func remoteStart(controllerURL string, configBytes []byte, testMode bool, client *http.Client) (string, error) {
	runURL := strings.TrimRight(controllerURL, "/") + "/mods"
	payload := map[string]any{
		"config":    string(configBytes),
		"test_mode": testMode,
	}
	body, _ := json.Marshal(payload)
	resp, err := client.Post(runURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("controller request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("controller run error: %s", string(rb))
	}

	var ack struct {
		ModID string `json:"mod_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	if ack.ModID == "" {
		return "", fmt.Errorf("controller did not return mod_id")
	}
	return ack.ModID, nil
}
