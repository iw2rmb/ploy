package server

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// handleAppProbe performs an internal HTTP probe against the app's allocation endpoint.
// Optional query param: lane=A..G (defaults to E first).
func (s *Server) handleAppProbe(c *fiber.Ctx) error {
	app := c.Params("app")
	lane := c.Query("lane", "E")
	lanes := []string{lane, "E", "C", "D", "B", "A", "F", "G"}
	hm := orchestration.NewHealthMonitor()
	var endpoint string
	for _, l := range lanes {
		job := fmt.Sprintf("%s-lane-%s", app, string([]rune(l)[0]))
		if ep, err := hm.GetJobEndpoint(job); err == nil && ep != "" {
			endpoint = ep
			break
		}
	}
	if endpoint == "" {
		return c.Status(404).JSON(fiber.Map{"error": "no running allocation endpoint found"})
	}
	// Perform internal HTTP GET to /healthz
	url := fmt.Sprintf("http://%s/healthz", endpoint)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"endpoint": endpoint, "status": "bad_gateway", "error": err.Error()})
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return c.JSON(fiber.Map{
		"endpoint": endpoint,
		"code":     resp.StatusCode,
		"body":     string(body),
	})
}
