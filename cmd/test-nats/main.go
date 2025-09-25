package main

import (
	"log"

	nats "github.com/nats-io/nats.go"
)

func main() {
	// Test basic NATS connectivity
	conn, err := nats.Connect("nats://nats.ploy.local:4223", nats.Name("connectivity-test"))
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}
	defer conn.Close()
	log.Println("✓ Connected to NATS successfully")

	// Test JetStream context
	js, err := conn.JetStream()
	if err != nil {
		log.Fatalf("failed to create JetStream context: %v", err)
	}
	log.Println("✓ JetStream context created")

	// Test object store access
	_, err = js.ObjectStore("routing_maps")
	if err != nil {
		log.Fatalf("failed to access routing_maps bucket: %v", err)
	}
	log.Println("✓ Routing object store accessible")

	// Test work queue stream
	info, err := js.StreamInfo("updates_control_plane")
	if err != nil {
		log.Fatalf("failed to access updates work queue: %v", err)
	}
	log.Printf("✓ Updates work queue stream accessible (msgs: %d)", info.State.Msgs)

	// Test status stream
	info, err = js.StreamInfo("updates_control_plane_status")
	if err != nil {
		log.Fatalf("failed to access updates status stream: %v", err)
	}
	log.Printf("✓ Updates status stream accessible (msgs: %d)", info.State.Msgs)

	// Test publishing to work queue
	testPayload := []byte(`{"deployment_id":"test","request":{"target_version":"test"},"lane":"d"}`)
	ack, err := js.Publish("updates.control-plane.tasks.d", testPayload)
	if err != nil {
		log.Fatalf("failed to publish test message: %v", err)
	}
	log.Printf("✓ Published test message to work queue (stream seq: %d)", ack.Sequence)

	// Test publishing status update
	statusPayload := []byte(`{"deployment_id":"test","phase":"testing","progress":50}`)
	_, err = js.Publish("updates.control-plane.status.test", statusPayload)
	if err != nil {
		log.Fatalf("failed to publish status message: %v", err)
	}
	log.Println("✓ Published test status message")

	log.Println("🎉 All NATS JetStream connectivity tests passed!")
}
