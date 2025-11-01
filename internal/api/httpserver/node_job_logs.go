package httpserver

import (
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"strings"
	"time"
)

// handleNodeJobLogsSnapshot serves GET /v1/node/jobs/:jobID/logs/snapshot.
func (s *Server) handleNodeJobLogsSnapshot(c *fiber.Ctx) error {
	if s.streams == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "log streaming unavailable")
	}
	id := strings.TrimSpace(c.Params("jobID"))
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job id required")
	}
	events := s.streams.Snapshot(id)
	dto := buildLogEventDTOs(events)
	return c.Status(fiber.StatusOK).JSON(map[string]any{"events": dto})
}

type nodeLogEntry struct {
	Stream    string `json:"stream"`
	Line      string `json:"line"`
	Timestamp string `json:"timestamp"`
}

// handleNodeJobLogsEntry serves POST /v1/node/jobs/:jobID/logs/entries.
func (s *Server) handleNodeJobLogsEntry(c *fiber.Ctx) error {
	if s.streams == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "log streaming unavailable")
	}
	id := strings.TrimSpace(c.Params("jobID"))
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job id required")
	}
	var req nodeLogEntry
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
	}
	stream := strings.TrimSpace(req.Stream)
	if stream == "" {
		stream = "stdout"
	}
	ts := strings.TrimSpace(req.Timestamp)
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339Nano)
	}
	record := logstream.LogRecord{Timestamp: ts, Stream: stream, Line: req.Line}
	if err := s.streams.PublishLog(c.UserContext(), id, record); err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return c.SendStatus(fiber.StatusAccepted)
}
