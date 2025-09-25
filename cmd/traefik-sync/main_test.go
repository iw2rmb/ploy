package main

import "testing"

func TestLoadConfigDefaultsJetStreamPort4223(t *testing.T) {
	t.Setenv("PLOY_ROUTING_JETSTREAM_URL", "")
	t.Setenv("PLOY_ROUTING_JETSTREAM_CREDS", "")
	cfg := loadConfig()
	if cfg.URL != "nats://nats.ploy.local:4223" {
		t.Fatalf("expected JetStream URL default to nats://nats.ploy.local:4223, got %q", cfg.URL)
	}
}
