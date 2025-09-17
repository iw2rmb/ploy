package server

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// handleBuildEvents streams build status transitions via SSE for a given async build id.
// Endpoint: GET /v1/apps/:app/builds/:id/events
func (s *Server) handleBuildEvents(c *fiber.Ctx) error {
	id := c.Params("id")

	// Prepare SSE response
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	// Stream writer
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() { _ = w.Flush() }()

		lastStatus := ""
		// helper to emit current status
		emit := func(st buildStatus) {
			b, _ := json.Marshal(st)
			_, _ = w.WriteString("data: ")
			_, _ = w.Write(b)
			_, _ = w.WriteString("\n\n")
			_ = w.Flush()
		}

		// Send initial snapshot if present
		if data, err := os.ReadFile(statusPath(id)); err == nil {
			var st buildStatus
			if json.Unmarshal(data, &st) == nil {
				lastStatus = st.Status
				emit(st)
				if st.Status == "completed" || st.Status == "failed" {
					return
				}
			}
		}

		// Watch loop: poll status file for changes at a low frequency until terminal
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.NewTimer(15 * time.Minute)
		defer timeout.Stop()
		for {
			select {
			case <-ticker.C:
				if data, err := os.ReadFile(statusPath(id)); err == nil {
					var st buildStatus
					if json.Unmarshal(data, &st) == nil {
						if st.Status != lastStatus {
							lastStatus = st.Status
							emit(st)
							if st.Status == "completed" || st.Status == "failed" {
								return
							}
						}
					}
				}
			case <-timeout.C:
				return
			case <-c.Context().Done():
				return
			}
		}
	})
	return nil
}
