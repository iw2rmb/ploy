package main

import (
	"encoding/json"
	"log"

	nats "github.com/nats-io/nats.go"
)

// DomainRoute represents a single routing rule
type DomainRoute struct {
	App    string `json:"app"`
	Domain string `json:"domain"`
	Target string `json:"target"`
	Secure bool   `json:"secure"`
	Path   string `json:"path,omitempty"`
}

func main() {

	// Connect to NATS
	conn, err := nats.Connect("nats://nats.ploy.local:4223", nats.Name("routing-init"))
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}
	defer conn.Close()

	js, err := conn.JetStream()
	if err != nil {
		log.Fatalf("failed to create JetStream context: %v", err)
	}

	// Get the routing object store
	bucket, err := js.ObjectStore("routing_maps")
	if err != nil {
		log.Fatalf("failed to open routing object store: %v", err)
	}

	// Create a default empty routing configuration for the ploy-api app
	routes := map[string]DomainRoute{
		"api.dev.ployman.app": {
			App:    "ploy-api",
			Domain: "api.dev.ployman.app",
			Target: "ploy-api.ploy.local",
			Secure: true,
		},
	}

	// Serialize the routing config
	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshal routes: %v", err)
	}

	// Store the routing configuration
	_, err = bucket.PutBytes("apps/ploy-api/routes.json", data)
	if err != nil {
		log.Fatalf("failed to store routing config: %v", err)
	}

	log.Println("Created initial routing configuration for ploy-api")
	log.Printf("Stored %d bytes of routing data", len(data))
}
