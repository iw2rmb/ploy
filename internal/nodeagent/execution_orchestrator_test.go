package nodeagent

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Verify gate stats shape includes an explicit final_gate key when only a final
// gate run is present (no pre_gate/regates), and does not replace the root map.
func TestBuildGateStats_FinalOnlyShape(t *testing.T) {
	rc := &runController{cfg: Config{}}
	result := step.Result{
		BuildGate: &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}}},
		Timings:   step.StageTiming{BuildGateDuration: 123},
	}
	execRes := executionResult{}

	got := rc.buildGateStats("run-x", result, execRes)

	if _, has := got["duration_ms"]; has {
		t.Fatalf("unexpected flat stats at root; want nested 'final_gate'")
	}
	fg, has := got["final_gate"]
	if !has {
		t.Fatalf("missing final_gate key in gate stats")
	}
	if m, ok := fg.(map[string]any); !ok || m["passed"] != true {
		t.Fatalf("final_gate stats malformed or missing passed=true: %#v", fg)
	}
}
