package mods

import "testing"

func TestResolveInfra_DefaultsAndDerivations(t *testing.T) {
	get := func(k string) string {
		switch k {
		case "PLOY_CONTROLLER":
			return "https://api.dev.ployman.app/v1"
		default:
			return ""
		}
	}
	inf := ResolveInfra(get)
	if inf.Controller == "" || inf.APIBase != "https://api.dev.ployman.app" {
		t.Fatalf("bad controller/apiBase: %+v", inf)
	}
	if inf.SeaweedURL == "" || inf.DC == "" {
		t.Fatalf("expected defaults for seaweed/dc: %+v", inf)
	}
	if inf.JetStreamURL == "" {
		t.Fatalf("expected default JetStream URL: %+v", inf)
	}
}

func TestResolveInfra_JetStreamFromEnv(t *testing.T) {
	get := func(k string) string {
		switch k {
		case "PLOY_CONTROLLER":
			return "https://api.dev.ployman.app/v1"
		case "PLOY_JETSTREAM_URL":
			return "nats://10.0.0.5:4224"
		default:
			return ""
		}
	}
	inf := ResolveInfra(get)
	if inf.JetStreamURL != "nats://10.0.0.5:4224" {
		t.Fatalf("expected custom JetStream URL, got %q", inf.JetStreamURL)
	}
}
