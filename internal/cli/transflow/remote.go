package transflow

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// executeRemoteTransflow handles execution via remote controller API
func executeRemoteTransflow(controllerURL, file string, testMode, verbose bool) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// POST /v1/transflow/run
	runURL := strings.TrimRight(controllerURL, "/") + "/transflow/run"
	payload := map[string]any{
		"config":    string(b),
		"test_mode": testMode,
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(runURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("controller request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("controller run error: %s", string(rb))
	}

	var ack struct {
		ExecutionID string `json:"execution_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	if ack.ExecutionID == "" {
		return fmt.Errorf("controller did not return execution_id")
	}

	// Poll status
	statusURL := strings.TrimRight(controllerURL, "/") + "/transflow/status/" + ack.ExecutionID
	start := time.Now()
	for {
		time.Sleep(2 * time.Second)
		sresp, err := http.Get(statusURL)
		if err != nil {
			continue
		}
		if sresp.StatusCode != 200 {
			sresp.Body.Close()
			continue
		}

		var st struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			Error  string         `json:"error"`
			Result map[string]any `json:"result"`
		}
		_ = json.NewDecoder(sresp.Body).Decode(&st)
		sresp.Body.Close()

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
				base := strings.TrimRight(controllerURL, "/") + "/transflow/artifacts/" + st.ID + "/"
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
				return fmt.Errorf("transflow failed: %s", st.Error)
			}
			return fmt.Errorf("transflow failed")
		}
	}
}
