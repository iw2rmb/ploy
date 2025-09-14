package mods

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSubstituteHCLTemplateWithMCPVars_UsesProvidedVars(t *testing.T) {
	tmp := t.TempDir()
	hcl := filepath.Join(tmp, "planner.hcl")
	body := "env { MODEL=\"${MODEL}\" CONTROLLER=\"${CONTROLLER_URL}\" MOD=\"${MOD_ID}\" CTX=${CONTEXT_HOST_DIR} OUT=${OUT_HOST_DIR} IMG=${PLANNER_IMAGE} DC=${NOMAD_DC} }\n"
	if err := os.WriteFile(hcl, []byte(body), 0644); err != nil {
		t.Fatalf("write hcl: %v", err)
	}
	vars := map[string]string{
		"MODS_MODEL":         "gpt-x",
		"PLOY_CONTROLLER":    "https://api.dev.ployman.app/v1",
		"MOD_ID":             "mod-e-22",
		"MODS_CONTEXT_DIR":   tmp,
		"MODS_OUT_DIR":       filepath.Join(tmp, "out"),
		"MODS_PLANNER_IMAGE": "registry.dev.ployman.app/langgraph-runner:latest",
		"NOMAD_DC":           "dc77",
	}
	out, err := substituteHCLTemplateWithMCPVars(hcl, "run-1", vars, nil)
	if err != nil {
		t.Fatalf("subst err: %v", err)
	}
	b, _ := os.ReadFile(out)
	s := string(b)
	if want := "MODEL=\"gpt-x\""; !contains(s, want) {
		t.Fatalf("missing %s in %s", want, s)
	}
	if want := "CONTROLLER=\"https://api.dev.ployman.app/v1\""; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "MOD=\"mod-e-22\""; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "CTX=" + vars["MODS_CONTEXT_DIR"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "OUT=" + vars["MODS_OUT_DIR"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "IMG=" + vars["MODS_PLANNER_IMAGE"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "DC=dc77"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
}
