package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	nomadapi "github.com/hashicorp/nomad/api"
)

// handleAppStatusWatch performs a long-poll on Nomad allocations for the app's active job
// and returns a status snapshot when a change occurs or the wait times out.
// GET /v1/apps/:app/status/watch?wait=30s&timeout=90s
func (s *Server) handleAppStatusWatch(c *fiber.Ctx) error {
	app := c.Params("app")
	// Parse wait and timeout
	wait := 30 * time.Second
	if v := c.Query("wait", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			wait = d
		}
	}
	timeout := 90 * time.Second
	if v := c.Query("timeout", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}

	cfg := nomadapi.DefaultConfig()
	client, err := nomadapi.NewClient(cfg)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "nomad client init failed", "details": err.Error()})
	}

	lanes := []string{"d"}
	var jobName string
	var fallback string
	// Prefer a job with a running allocation; fallback to the first that exists
	for _, ln := range lanes {
		j := app + "-lane-" + ln
		allocs, _, err := client.Jobs().Allocations(j, false, nil)
		if err == nil {
			if fallback == "" {
				fallback = j
			}
			for _, a := range allocs {
				if a.ClientStatus == "running" {
					jobName = j
					break
				}
			}
			if jobName != "" {
				break
			}
		}
	}
	if jobName == "" {
		jobName = fallback
	}
	if jobName == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"status": "not_found", "message": "App not found or not deployed"})
	}

	deadline := time.Now().Add(timeout)
	var waitIdx uint64
	// One blocking loop: return on first change or deadline
	for time.Now().Before(deadline) {
		q := &nomadapi.QueryOptions{WaitIndex: waitIdx, WaitTime: wait, AllowStale: true}
		allocs, meta, err := client.Jobs().Allocations(jobName, false, q)
		if err != nil {
			// brief backoff then retry until deadline
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if meta != nil && meta.LastIndex > 0 {
			// If this is the first iteration, LastIndex may be 0 → set it and loop again to wait for a change.
			if waitIdx == 0 {
				waitIdx = meta.LastIndex
				// fall through to build a snapshot on initial fetch as well
			} else if meta.LastIndex == waitIdx {
				// nothing changed during wait; continue loop
				continue
			} else {
				waitIdx = meta.LastIndex
			}
		}
		// Build the same shape as /v1/apps/:app/status
		// Minimal job info from SDK is limited; synthesize fields similar to internal/build.Status
		type taskEvent struct {
			Type           string `json:"type"`
			Time           int64  `json:"time"`
			Message        string `json:"message"`
			DisplayMessage string `json:"display_message"`
		}
		type taskSummary struct {
			Name   string      `json:"name"`
			State  string      `json:"state"`
			Failed bool        `json:"failed"`
			Events []taskEvent `json:"events,omitempty"`
		}
		type allocSummary struct {
			ID            string        `json:"id"`
			ClientStatus  string        `json:"client_status"`
			DesiredStatus string        `json:"desired_status"`
			Healthy       *bool         `json:"healthy,omitempty"`
			Tasks         []taskSummary `json:"tasks,omitempty"`
		}
		out := struct {
			Status      string         `json:"status"`
			JobName     string         `json:"job_name"`
			Lane        string         `json:"lane"`
			WaitIndex   uint64         `json:"wait_index"`
			Allocations []allocSummary `json:"allocations"`
		}{Status: "running", JobName: jobName, Lane: "unknown", WaitIndex: waitIdx}
		// Convert allocs
		for _, a := range allocs {
			s := allocSummary{ID: a.ID, ClientStatus: a.ClientStatus, DesiredStatus: a.DesiredStatus}
			if a.DeploymentStatus != nil {
				s.Healthy = a.DeploymentStatus.Healthy
			}
			if ts := a.TaskStates; ts != nil {
				// Deterministic order not strictly required here
				for name, st := range ts {
					t := taskSummary{Name: name, State: st.State, Failed: st.Failed}
					if len(st.Events) > 0 {
						start := 0
						if len(st.Events) > 4 {
							start = len(st.Events) - 4
						}
						for _, ev := range st.Events[start:] {
							t.Events = append(t.Events, taskEvent{Type: ev.Type, Time: ev.Time, Message: ev.Message, DisplayMessage: ev.DisplayMessage})
						}
					}
					s.Tasks = append(s.Tasks, t)
				}
			}
			out.Allocations = append(out.Allocations, s)
		}
		// Write and return snapshot
		b, _ := json.Marshal(out)
		c.Type("json")
		return c.Send(b)
	}
	return c.Status(http.StatusGatewayTimeout).JSON(fiber.Map{"error": "timeout", "message": "no changes detected"})
}
