//go:build e2e

package e2e

// Scenario captures the intent and prerequisites for a Mods E2E case.
type Scenario struct {
	ID              string
	Title           string
	RepoURL         string
	BaseRef         string
	FailureRef      string
	Description     string
	MissingFeatures []string
}

var scenarioRegistry = map[string]Scenario{
	"simple-openrewrite": {
		ID:          "simple-openrewrite",
		Title:       "OpenRewrite recipe applies cleanly",
		RepoURL:     "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		BaseRef:     "main",
		FailureRef:  "main",
		Description: "Runs mods-plan → orw-apply on the Java 11→17 playground repo and expects the build gate to pass without healing.",
		MissingFeatures: []string{
            "Lane catalog migration pending so legacy smoke can use shared job specs",
		},
	},
	"buildgate-self-heal": {
		ID:          "buildgate-self-heal",
		Title:       "Build gate failure triggers Mods healing",
		RepoURL:     "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		BaseRef:     "e2e/fail-missing-symbol",
		FailureRef:  "e2e/fail-missing-symbol",
		Description: "Applies OpenRewrite, observes the build gate failure, then relies on llm-plan/llm-exec/human stages to self-heal before the build gate succeeds.",
		MissingFeatures: []string{
            "Legacy-backed smoke test pending until catalog lanes publish",
		},
	},
	"parallel-healing-options": {
		ID:          "parallel-healing-options",
		Title:       "Parallel healing choices fan out",
		RepoURL:     "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		BaseRef:     "e2e/fail-parallel",
		FailureRef:  "e2e/fail-parallel",
		Description: "Exercises planner support for running OpenRewrite and LLM fixes concurrently when multiple blocking issues exist.",
		MissingFeatures: []string{
            "Parallel branch reconciliation on legacy system scheduled for catalog rollout",
		},
	},
}

// ScenarioByID returns the recorded scenario definition.
func ScenarioByID(id string) (Scenario, bool) {
	scenario, ok := scenarioRegistry[id]
	return scenario, ok
}

// ScenarioIDs returns the known scenario keys.
func ScenarioIDs() []string {
	ids := make([]string, 0, len(scenarioRegistry))
	for id := range scenarioRegistry {
		ids = append(ids, id)
	}
	return ids
}
