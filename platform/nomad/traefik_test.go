package platformnomad

import (
	"strings"
	"testing"
)

func TestTraefikSpecUsesCoreDNSForJetStream(t *testing.T) {
	content := string(GetEmbeddedTemplate("platform/nomad/traefik.hcl"))
	if content == "" {
		t.Fatalf("traefik template not embedded")
	}

	if strings.Contains(content, "nats://127.0.0.1:4222") {
		t.Fatalf("traefik spec should not reference loopback JetStream URL\n%s", content)
	}

	if strings.Contains(content, "service.consul") {
		t.Fatalf("traefik spec should not reference consul DNS\n%s", content)
	}

	if !strings.Contains(content, "nats.ploy.local") {
		t.Fatalf("traefik spec should reference CoreDNS host nats.ploy.local\n%s", content)
	}

	if !strings.Contains(content, "PLOY_ROUTING_JETSTREAM_URL          = \"nats://nats.ploy.local:4223\"") {
		t.Fatalf("traefik spec must default JetStream URL to nats://nats.ploy.local:4223\n%s", content)
	}

	if strings.Contains(content, "nats://nats.ploy.local:4222") {
		t.Fatalf("traefik spec must not reference legacy JetStream port 4222\n%s", content)
	}
}
