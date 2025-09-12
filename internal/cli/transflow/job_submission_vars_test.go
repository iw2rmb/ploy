package transflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSubstituteHCLTemplateWithMCPVars_UsesProvidedVars(t *testing.T) {
	tmp := t.TempDir()
	hcl := filepath.Join(tmp, "planner.hcl")
	body := "env { MODEL=\"${MODEL}\" CONTROLLER=\"${CONTROLLER_URL}\" EXEC=\"${EXECUTION_ID}\" CTX=${CONTEXT_HOST_DIR} OUT=${OUT_HOST_DIR} IMG=${PLANNER_IMAGE} DC=${NOMAD_DC} }\n"
	if err := os.WriteFile(hcl, []byte(body), 0644); err != nil {
		t.Fatalf("write hcl: %v", err)
	}
	vars := map[string]string{
		"TRANSFLOW_MODEL":             "gpt-x",
		"PLOY_CONTROLLER":             "https://api.dev.ployman.app/v1",
		"PLOY_TRANSFLOW_EXECUTION_ID": "e-22",
		"TRANSFLOW_CONTEXT_DIR":       tmp,
		"TRANSFLOW_OUT_DIR":           filepath.Join(tmp, "out"),
		"TRANSFLOW_PLANNER_IMAGE":     "registry.dev.ployman.app/langgraph-runner:py-0.1.0",
		"NOMAD_DC":                    "dc77",
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
	if want := "EXEC=\"e-22\""; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "CTX=" + vars["TRANSFLOW_CONTEXT_DIR"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "OUT=" + vars["TRANSFLOW_OUT_DIR"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "IMG=" + vars["TRANSFLOW_PLANNER_IMAGE"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "DC=dc77"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
}
