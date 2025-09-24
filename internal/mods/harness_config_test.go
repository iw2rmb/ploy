package mods

import (
	"os"
	"testing"
)

func TestResolveHarness_UsesEnvFallbacks(t *testing.T) {
	t.Setenv("PLOY_CONTROLLER", "")
	// Clear defaults that ResolveInfra might read
	for _, key := range []string{"MODS_SEAWEED_FALLBACKS", "MODS_SEAWEED_MASTER", "PLOY_SEAWEEDFS_URL"} {
		_ = os.Unsetenv(key)
	}

	h := ResolveHarnessFromEnv()
	if h.Infra.Controller == "" {
		t.Fatalf("expected controller fallback to be non-empty")
	}
	cands := h.SeaweedCandidates()
	if len(cands) == 0 {
		t.Fatalf("expected at least one seaweed candidate")
	}
}

func TestResolveHarness_CustomFallbacks(t *testing.T) {
	t.Setenv("PLOY_SEAWEEDFS_URL", "http://primary:8888")
	t.Setenv("MODS_SEAWEED_FALLBACKS", "http://one:8888, http://two:9000 ")
	t.Setenv("MODS_SEAWEED_MASTER", "seaweed-master:9333")
	t.Setenv("PLOY_CONTROLLER", "https://api.example/v1")

	h := ResolveHarnessFromEnv()
	cands := h.SeaweedCandidates()
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	if cands[0] != "http://primary:8888" {
		t.Fatalf("expected primary candidate first, got %s", cands[0])
	}
	if cands[1] != "http://one:8888" || cands[2] != "http://two:9000" {
		t.Fatalf("unexpected fallback ordering: %v", cands)
	}
	if got := h.SeaweedMasterHost(); got != "seaweed-master:9333" {
		t.Fatalf("expected master host override, got %s", got)
	}
}

func TestResolveHarness_DerivesMasterFromFiler(t *testing.T) {
	t.Setenv("PLOY_SEAWEEDFS_URL", "https://seaweed.example:8443/artifacts")
	_ = os.Unsetenv("MODS_SEAWEED_MASTER")

	h := ResolveHarnessFromEnv()
	if got := h.SeaweedFilerHost(); got != "seaweed.example:8443" {
		t.Fatalf("expected filer host port, got %s", got)
	}
	if got := h.SeaweedMasterHost(); got != "seaweed.example:9333" {
		t.Fatalf("expected derived master host, got %s", got)
	}
}
