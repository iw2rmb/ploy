package transflow

import (
    "testing"
)

func TestRenderMRDescription_IncludesHealingAndSteps(t *testing.T) {
    r := &TransflowRunner{config: &TransflowConfig{ID: "wf-1"}}
    res := &TransflowResult{
        BranchName:   "workflow/wf-1/123",
        BuildVersion: "v123",
        Duration:     0,
        StepResults: []StepResult{
            {StepID: "apply", Success: true, Message: "Applied ORW diff"},
            {StepID: "build", Success: true, Message: "Build passed"},
            {StepID: "mr", Success: true, Message: "MR created"}, // should be filtered out
        },
        HealingSummary: &TransflowHealingSummary{
            Enabled:       true,
            PlanID:        "plan-abc",
            AttemptsCount: 2,
            Winner:        &BranchResult{ID: "llm-exec"},
        },
    }
    s := renderMRDescription(r, res)
    mustContain(t, s, "Transflow Workflow")
    mustContain(t, s, "wf-1")
    mustContain(t, s, "workflow/wf-1/123")
    mustContain(t, s, "v123")
    mustContain(t, s, "Applied ORW diff")
    mustContain(t, s, "Build passed")
    // healing bits
    mustContain(t, s, "Self-Healing Applied")
    mustContain(t, s, "plan-abc")
    mustContain(t, s, "llm-exec")
}

func mustContain(t *testing.T, s, sub string) {
    t.Helper()
    if len(sub) == 0 { return }
    if len(s) < len(sub) { t.Fatalf("missing %q", sub) }
    // simple contains
    for i := 0; i <= len(s)-len(sub); i++ {
        if s[i:i+len(sub)] == sub {
            return
        }
    }
    t.Fatalf("missing %q", sub)
}
