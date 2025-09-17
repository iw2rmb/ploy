package server

import (
	"bufio"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	nomadapi "github.com/hashicorp/nomad/api"
)

type allocEvent struct {
	AllocID string            `json:"alloc_id"`
	Task    string            `json:"task"`
	Type    string            `json:"type"`
	Time    int64             `json:"time"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// handleAllocEvents streams TaskStates.Events for a given allocation using Nomad blocking queries.
// Endpoint: GET /v1/nomad/allocs/:id/events
func (s *Server) handleAllocEvents(c *fiber.Ctx) error {
	allocID := c.Params("id")

	cfg := nomadapi.DefaultConfig()
	client, err := nomadapi.NewClient(cfg)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "nomad client init failed", "details": err.Error()})
	}

	// Prepare SSE response
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() { _ = w.Flush() }()
		// Track last seen event index per task
		lastIdx := map[string]int{}
		var waitIdx uint64
		waitTime := 30 * time.Second

		emit := func(ev allocEvent) {
			b, _ := json.Marshal(ev)
			_, _ = w.WriteString("data: ")
			_, _ = w.Write(b)
			_, _ = w.WriteString("\n\n")
			_ = w.Flush()
		}

		deadline := time.NewTimer(20 * time.Minute)
		defer deadline.Stop()
		for {
			select {
			case <-deadline.C:
				return
			case <-c.Context().Done():
				return
			default:
			}
			q := &nomadapi.QueryOptions{WaitIndex: waitIdx, WaitTime: waitTime, AllowStale: true}
			alloc, meta, err := client.Allocations().Info(allocID, q)
			if err != nil {
				// brief backoff before retrying
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if meta != nil && meta.LastIndex > 0 {
				waitIdx = meta.LastIndex
			}
			if alloc == nil || alloc.TaskStates == nil {
				continue
			}
			for task, st := range alloc.TaskStates {
				// Determine new events since last index
				start := lastIdx[task]
				if start < 0 {
					start = 0
				}
				if start > len(st.Events) {
					start = len(st.Events)
				}
				for i := start; i < len(st.Events); i++ {
					e := st.Events[i]
					msg := e.DisplayMessage
					if msg == "" {
						msg = e.Message
					}
					emit(allocEvent{
						AllocID: allocID,
						Task:    task,
						Type:    e.Type,
						Time:    e.Time,
						Message: msg,
						Details: e.Details,
					})
				}
				lastIdx[task] = len(st.Events)
			}
		}
	})
	return nil
}
