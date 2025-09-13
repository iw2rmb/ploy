package mods

import (
	"os"
	"testing"
)

func TestResolveDefaults_UsesBuiltinsWhenEnvMissing(t *testing.T) {
	d := ResolveDefaults(func(string) string { return "" })
	if d.Registry != "registry.dev.ployman.app" {
		t.Fatalf("registry default: %s", d.Registry)
	}
	if d.PlannerImage == "" || d.ORWApplyImage == "" {
		t.Fatalf("images should be set: %v", d)
	}
	if d.DC != "dc1" {
		t.Fatalf("dc default: %s", d.DC)
	}
	if len(d.Allowlist) == 0 || d.Allowlist[0] != "src/**" {
		t.Fatalf("allowlist default: %v", d.Allowlist)
	}
	if d.SeaweedURL == "" {
		t.Fatalf("seaweed default missing")
	}
	if d.PlannerTimeout <= 0 || d.ReducerTimeout <= 0 || d.LLMExecTimeout <= 0 || d.ORWApplyTimeout <= 0 || d.BuildApplyTimeout <= 0 {
		t.Fatalf("timeouts not set: %+v", d)
	}
}

func TestResolveDefaults_RespectsEnvOverrides(t *testing.T) {
	get := func(k string) string {
		switch k {
		case "TRANSFLOW_REGISTRY":
			return "reg.local"
		case "TRANSFLOW_PLANNER_IMAGE":
			return "custom/planner:1"
		case "NOMAD_DC":
			return "dc9"
		case "TRANSFLOW_ALLOWLIST":
			return "a/**,b.txt"
		default:
			return ""
		}
	}
	d := ResolveDefaults(get)
	if d.Registry != "reg.local" {
		t.Fatalf("registry override: %s", d.Registry)
	}
	if d.PlannerImage != "custom/planner:1" {
		t.Fatalf("planner override: %s", d.PlannerImage)
	}
	if d.DC != "dc9" {
		t.Fatalf("dc override: %s", d.DC)
	}
	if len(d.Allowlist) != 2 || d.Allowlist[1] != "b.txt" {
		t.Fatalf("allowlist override: %v", d.Allowlist)
	}
}

func TestResolveDefaultsFromEnv_Smoke(t *testing.T) {
	os.Setenv("TRANSFLOW_REGISTRY", "r.example")
	defer os.Unsetenv("TRANSFLOW_REGISTRY")
	d := ResolveDefaultsFromEnv()
	if d.Registry != "r.example" {
		t.Fatalf("env registry: %s", d.Registry)
	}
}
