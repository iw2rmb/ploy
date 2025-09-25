package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	nats "github.com/nats-io/nats.go"
)

func main() {
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to NATS
	conn, err := nats.Connect("nats://nats.ploy.local:4223", nats.Name("jetstream-init"))
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}
	defer conn.Close()

	js, err := conn.JetStream()
	if err != nil {
		log.Fatalf("failed to create JetStream context: %v", err)
	}

	// Create routing object store bucket
	if err := ensureObjectStore(js, "routing_maps"); err != nil {
		log.Fatalf("failed to create routing object store: %v", err)
	}
	log.Println("Created routing_maps object store")

	// Create routing events stream
	if err := ensureStream(js, "routing_events", "routing.app"); err != nil {
		log.Fatalf("failed to create routing events stream: %v", err)
	}
	log.Println("Created routing_events stream")

	// Create self-update work queue stream
	if err := ensureStream(js, "updates_control_plane", "updates.control-plane.tasks"); err != nil {
		log.Fatalf("failed to create updates work queue stream: %v", err)
	}
	log.Println("Created updates_control_plane work queue stream")

	// Create self-update status stream
	if err := ensureStream(js, "updates_control_plane_status", "updates.control-plane.status"); err != nil {
		log.Fatalf("failed to create updates status stream: %v", err)
	}
	log.Println("Created updates_control_plane_status stream")

	log.Println("JetStream initialization completed successfully")
}

func ensureObjectStore(js nats.JetStreamContext, bucket string) error {
	_, err := js.ObjectStore(bucket)
	if err != nil {
		if errors.Is(err, nats.ErrStreamNotFound) || errors.Is(err, nats.ErrObjectNotFound) {
			cfg := &nats.ObjectStoreConfig{
				Bucket:   bucket,
				Replicas: 1,
			}
			_, err = js.CreateObjectStore(cfg)
		}
	}
	if err != nil {
		return fmt.Errorf("ensure object store %s: %w", bucket, err)
	}
	return nil
}

func ensureStream(js nats.JetStreamContext, streamName, subjectPattern string) error {
	_, err := js.StreamInfo(streamName)
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("lookup stream %s: %w", streamName, err)
	}

	var subjects []string
	switch subjectPattern {
	case "updates.control-plane.tasks":
		subjects = []string{"updates.control-plane.tasks.*"}
	case "updates.control-plane.status":
		subjects = []string{"updates.control-plane.status.*"}
	default:
		subjects = []string{fmt.Sprintf("%s.*", subjectPattern)}
	}

	cfg := &nats.StreamConfig{
		Name:     streamName,
		Subjects: subjects,
		Storage:  nats.FileStorage,
		Replicas: 1,
	}

	if streamName == "updates_control_plane" {
		cfg.Retention = nats.WorkQueuePolicy
		cfg.MaxMsgsPerSubject = 1
	} else {
		cfg.Retention = nats.LimitsPolicy
		cfg.MaxAge = 72 * time.Hour
	}

	if _, err := js.AddStream(cfg); err != nil {
		return fmt.Errorf("create stream %s: %w", streamName, err)
	}
	return nil
}
