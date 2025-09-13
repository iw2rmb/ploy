package mods

import "testing"

func TestResolveImages_UsesDefaultsWhenUnset(t *testing.T) {
	get := func(k string) string { return "" }
	imgs := ResolveImages(get)
	if imgs.Registry == "" || imgs.Planner == "" || imgs.ORWApply == "" || imgs.LLMExec == "" || imgs.Reducer == "" {
		t.Fatalf("expected non-empty defaults: %+v", imgs)
	}
}

func TestResolveImages_PrefersEnv(t *testing.T) {
	get := func(k string) string {
		if k == "TRANSFLOW_PLANNER_IMAGE" {
			return "reg/custom-planner:1"
		}
		if k == "TRANSFLOW_REGISTRY" {
			return "reg"
		}
		return ""
	}
	imgs := ResolveImages(get)
	if imgs.Planner != "reg/custom-planner:1" {
		t.Fatalf("env override not applied: %+v", imgs)
	}
}
