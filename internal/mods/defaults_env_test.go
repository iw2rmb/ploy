package mods

import "testing"

func TestResolveDefaultsUsesJetStreamPort4223(t *testing.T) {
	t.Setenv("PLOY_JETSTREAM_URL", "")
	defaults := ResolveDefaultsFromEnv()
	if defaults.JetStreamURL != "nats://nats.ploy.local:4223" {
		t.Fatalf("expected JetStream URL default to nats://nats.ploy.local:4223, got %q", defaults.JetStreamURL)
	}
}
