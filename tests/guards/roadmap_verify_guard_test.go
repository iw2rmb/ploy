package guards

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoadmapVerifyFailsTargetedPhaseWhenNotDone(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	phasePath := filepath.Join("roadmap", "sbom-hooks-remediation", "phase-3-delivery-gates-and-observability.yaml")
	cmd := exec.Command("bash", "tools/roadmap/verify_done.sh", phasePath)
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected verification to fail when targeted phase is done=false, output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "error: targeted phase not done:") {
		t.Fatalf("expected not-done error message, output: %s", output)
	}
	if !strings.Contains(output, "error: roadmap verification checked 0 targeted done phases") {
		t.Fatalf("expected zero-checked error message, output: %s", output)
	}
}

func TestRoadmapVerifyFailsUncheckedEvidenceForDonePhases(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	phases := []string{
		filepath.Join("roadmap", "sbom-hooks-remediation", "phase-1-conditional-planning-and-preflight.yaml"),
		filepath.Join("roadmap", "sbom-hooks-remediation", "phase-2-runtime-execution-and-ingestion.yaml"),
		filepath.Join("roadmap", "sbom-hooks-remediation", "phase-3-delivery-gates-and-observability.yaml"),
	}

	args := append([]string{"tools/roadmap/verify_done.sh"}, phases...)
	cmd := exec.Command("bash", args...)
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected verification to fail when evidence markers are unchecked, output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "error: evidence marker") || !strings.Contains(output, "is present but unchecked") {
		t.Fatalf("expected unchecked evidence marker error, output: %s", output)
	}
}
