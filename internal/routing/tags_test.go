package routing

import (
	"fmt"
	"testing"
)

func TestBuildTraefikTags_SubsetPresence(t *testing.T) {
	route := &AppRoute{
		App:        "myapp",
		Domain:     "myapp.ployd.app",
		Port:       8080,
		AllocID:    "alloc-123",
		AllocIP:    "10.0.0.5",
		HealthPath: "/healthz",
	}
	cfg := &RouteConfig{
		EnableTLS:           true,
		CertResolver:        "letsencrypt",
		HealthPath:          "/healthz",
		HealthCheckInterval: "10s",
		HealthCheckTimeout:  "5s",
		LoadBalanceMode:     "weighted_round_robin",
		StickySession:       false,
		Middlewares:         []string{"platform-security-headers"},
		RetryAttempts:       3,
		RateLimit:           50,
		SecurityHeaders:     true,
	}

	tags := BuildTraefikTags(route, cfg)
	mustContain := []string{
		"traefik.enable=true",
		"traefik.http.routers.myapp-router.rule=Host(`myapp.ployd.app`)",
		"traefik.http.routers.myapp-router.entrypoints=websecure",
		"traefik.http.routers.myapp-router.tls=true",
		"traefik.http.routers.myapp-router.tls.certresolver=letsencrypt",
		"traefik.http.services.myapp-service.loadbalancer.server.port=8080",
		"traefik.http.services.myapp-service.loadbalancer.healthcheck.path=/healthz",
		"traefik.http.services.myapp-service.loadbalancer.healthcheck.interval=10s",
		"traefik.http.services.myapp-service.loadbalancer.healthcheck.timeout=5s",
		// strategy
		"traefik.http.services.myapp-service.loadbalancer.strategy=weighted_round_robin",
	}
	for _, want := range mustContain {
		if !contains(tags, want) {
			t.Fatalf("expected tag not found: %s\nGot: %v", want, tags)
		}
	}

	// Aliases should collapse into a single Host() rule with multiple domains
	route.Aliases = []string{"alt.ployd.app", "extra.ployd.app"}
	tags = BuildTraefikTags(route, cfg)
	aliasRule := fmt.Sprintf("traefik.http.routers.%s-router.rule=Host(`%s`,`%s`,`%s`)", route.App, route.Domain, route.Aliases[0], route.Aliases[1])
	if !contains(tags, aliasRule) {
		t.Fatalf("expected alias rule not found: %s\nGot: %v", aliasRule, tags)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
