package mods

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// StreamLogs provides a basic Server-Sent Events (SSE) stub for live mod logs.
// For now, it emits a single init event and returns; future work will stream steps and job tails.
func (h *Handler) StreamLogs(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fiber.Map{"code": "missing_id", "message": "Execution ID is required"}})
	}
	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	follow := strings.ToLower(c.Query("follow", "true")) != "false"
	interval := 2 * time.Second
	if v := os.Getenv("PLOY_MODS_SSE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	// Optional time cap
	maxDur := 30 * time.Minute
	if v := os.Getenv("PLOY_MODS_SSE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			maxDur = d
		}
	}

	start := time.Now()
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// helper to write event
		writeEvent := func(event string, data string) bool {
			if _, err := w.WriteString("event: " + event + "\n"); err != nil {
				return false
			}
			if _, err := w.WriteString("data: " + data + "\n\n"); err != nil {
				return false
			}
			if err := w.Flush(); err != nil {
				return false
			}
			return true
		}

		// Send init
		initPayload := fmt.Sprintf(`{"id":"%s","message":"SSE connected"}`, id)
		if !writeEvent("init", initPayload) {
			return
		}

		// Always send current snapshot of steps
		lastCount := 0
		st, err := h.getStatus(id)
		if err == nil && st != nil {
			if len(st.Steps) > 0 {
				for i := 0; i < len(st.Steps); i++ {
					b, _ := json.Marshal(st.Steps[i])
					if !writeEvent("step", string(b)) {
						return
					} else if !follow {
						// No status available but follow=false: end immediately
						_ = writeEvent("end", `{"status":"unknown"}`)
						return
					}
				}
				lastCount = len(st.Steps)
			}
			// If not following, end now
			if !follow {
				fin := map[string]any{"status": st.Status, "phase": st.Phase, "duration": st.Duration}
				b, _ := json.Marshal(fin)
				_ = writeEvent("end", string(b))
				return
			}
			// If already terminal, end
			if st.Status == "completed" || st.Status == "failed" || st.Status == "cancelled" {
				fin := map[string]any{"status": st.Status, "phase": st.Phase, "duration": st.Duration}
				b, _ := json.Marshal(fin)
				_ = writeEvent("end", string(b))
				return
			}
		}

		// Follow mode: poll for new steps and status
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var lastLogPreview string
		for {
			if time.Since(start) > maxDur {
				_ = writeEvent("end", `{"status":"timeout"}`)
				return
			}
			select {
			case <-ticker.C:
				st, err := h.getStatus(id)
				if err != nil || st == nil {
					if !writeEvent("ping", `{"ok":false}`) {
						return
					}
					continue
				}
				// Phase/status updates as event
				meta := map[string]any{"status": st.Status, "phase": st.Phase, "duration": st.Duration, "overdue": st.Overdue}
				if b, e := json.Marshal(meta); e == nil {
					if !writeEvent("meta", string(b)) {
						return
					}
				}

				// Optional: stream last job log preview if available and changed
				if st.LastJob != nil && st.LastJob.AllocID != "" {
					task := taskForJob(st.LastJob.JobName)
					if preview := tailAllocLogs(st.LastJob.AllocID, task, 50); preview != "" && preview != lastLogPreview {
						payload := map[string]any{"task": task, "preview": preview}
						if b, e := json.Marshal(payload); e == nil {
							if !writeEvent("log", string(b)) {
								return
							}
							lastLogPreview = preview
						}
					}
				}
				// Stream new steps
				if len(st.Steps) > lastCount {
					for i := lastCount; i < len(st.Steps); i++ {
						b, _ := json.Marshal(st.Steps[i])
						if !writeEvent("step", string(b)) {
							return
						}
					}
					lastCount = len(st.Steps)
				}
				// Terminal?
				if st.Status == "completed" || st.Status == "failed" || st.Status == "cancelled" {
					b, _ := json.Marshal(meta)
					_ = writeEvent("end", string(b))
					return
				}
			default:
				// Best-effort CPU yield
				time.Sleep(10 * time.Millisecond)
			}
		}
	})
	return nil
}

// tailAllocLogs fetches a short preview of allocation logs using the VPS job manager wrapper.
// Returns empty string on any error.
func tailAllocLogs(allocID, task string, lines int) string {
	mgr := os.Getenv("NOMAD_JOB_MANAGER")
	if mgr == "" {
		mgr = "/opt/hashicorp/bin/nomad-job-manager.sh"
	}
	if _, err := os.Stat(mgr); err != nil {
		return ""
	}
	if task == "" {
		task = "api"
	}
	if lines <= 0 {
		lines = 50
	}
	cmd := exec.Command(mgr, "logs", "--alloc-id", allocID, "--task", task, "--both", "--lines", fmt.Sprintf("%d", lines))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	s := string(out)
	if len(s) > 4000 {
		s = s[len(s)-4000:]
	}
	return s
}

// taskForJob maps a job name to its task name for log tailing
func taskForJob(jobName string) string {
	n := strings.ToLower(jobName)
	switch {
	case strings.Contains(n, "orw-apply"):
		return "openrewrite-apply"
	case strings.Contains(n, "planner"):
		return "planner"
	case strings.Contains(n, "reducer"):
		return "reducer"
	case strings.Contains(n, "llm-exec") || strings.Contains(n, "llm_exec"):
		return "llm-exec"
	default:
		return "api"
	}
}
