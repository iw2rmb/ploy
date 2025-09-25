package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"

	eventsfabric "github.com/iw2rmb/ploy/internal/events/fabric"
)

// handleBuildEvents streams build status transitions via SSE for a given async build id.
// Endpoint: GET /v1/apps/:app/builds/:id/events
func (s *Server) handleBuildEvents(c *fiber.Ctx) error {
	id := c.Params("id")
	if fabric := s.dependencies.EventFabric; fabric != nil {
		if err := s.streamBuildEventsJetStream(c, id, fabric); err == nil {
			return nil
		}
	}
	return s.streamBuildEventsFallback(c, id)
}

func (s *Server) streamBuildEventsJetStream(c *fiber.Ctx, id string, fabric *eventsfabric.Fabric) error {
	js := fabric.JetStream()
	stream := fabric.BuildStream()
	subject := fabric.BuildSubject(id)
	if strings.TrimSpace(stream) == "" || strings.TrimSpace(subject) == "" {
		return errors.New("event fabric stream/subject not configured")
	}

	durable := fmt.Sprintf("build-events-sse-%s", sanitizeDurable(id))
	cfg := &nats.ConsumerConfig{
		Durable:       durable,
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverAllPolicy,
		FilterSubject: subject,
		MaxAckPending: 64,
	}
	if _, err := js.AddConsumer(stream, cfg); err != nil && !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
		return err
	}

	sub, err := js.PullSubscribe(subject, durable, nats.BindStream(stream))
	if err != nil {
		return err
	}
	defer func() { _ = sub.Unsubscribe() }()

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() { _ = w.Flush() }()

		emit := func(st buildStatus) {
			b, _ := json.Marshal(st)
			_, _ = w.WriteString("data: ")
			_, _ = w.Write(b)
			_, _ = w.WriteString("\n\n")
			_ = w.Flush()
		}

		for {
			if err := c.Context().Err(); err != nil {
				return
			}
			msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) {
					continue
				}
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("build events: fetch error: %v", err)
				return
			}
			for _, msg := range msgs {
				var st buildStatus
				if err := json.Unmarshal(msg.Data, &st); err != nil {
					_ = msg.Ack()
					continue
				}
				emit(st)
				_ = msg.Ack()
				if st.Status == "completed" || st.Status == "failed" {
					return
				}
			}
		}
	})
	return nil
}

func (s *Server) streamBuildEventsFallback(c *fiber.Ctx, id string) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() { _ = w.Flush() }()

		lastStatus := ""
		emit := func(st buildStatus) {
			b, _ := json.Marshal(st)
			_, _ = w.WriteString("data: ")
			_, _ = w.Write(b)
			_, _ = w.WriteString("\n\n")
			_ = w.Flush()
		}

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

var durableSanitizer = regexp.MustCompile(`[^a-z0-9_-]`)

func sanitizeDurable(id string) string {
	clean := strings.ToLower(strings.TrimSpace(id))
	if clean == "" {
		return "default"
	}
	clean = durableSanitizer.ReplaceAllString(clean, "-")
	if len(clean) > 48 {
		clean = clean[:48]
	}
	return clean
}
