package guards

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoadmapVerifyFailsWhenTargetedPhaseNotDone(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	phasePath := filepath.Join("roadmap", "sbom-hooks-remediation", "phase-3-delivery-gates-and-observability.yaml")
	cmd := exec.Command("bash", "tools/roadmap/verify_done.sh", phasePath)
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected verification to fail for done=false phase, got success; output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "targeted phase is not done") {
		t.Fatalf("expected not-done failure message, output: %s", output)
	}
	if !strings.Contains(output, "checked 0 targeted done phases") {
		t.Fatalf("expected zero-checked failure message, output: %s", output)
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
		t.Fatalf("expected verification to fail with unchecked evidence markers, got success; output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "is present but unchecked") {
		t.Fatalf("expected unchecked evidence marker failure, output: %s", output)
	}
}
