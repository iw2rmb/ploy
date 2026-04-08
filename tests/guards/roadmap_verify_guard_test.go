package guards

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoadmapVerifySkipsTargetedPhaseWhenNotDone(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	phasePath := filepath.Join("roadmap", "sbom-hooks-remediation", "phase-3-delivery-gates-and-observability.yaml")
	cmd := exec.Command("bash", "tools/roadmap/verify_done.sh", phasePath)
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected verification to pass when only targeted phase is done=false, got error: %v; output: %s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "warning: targeted phase not done (skipped)") {
		t.Fatalf("expected not-done warning message, output: %s", output)
	}
	if !strings.Contains(output, "checked 0 targeted done phases") {
		t.Fatalf("expected zero-checked warning message, output: %s", output)
	}
}

func TestRoadmapVerifyWarnsUncheckedEvidenceForDonePhases(t *testing.T) {
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
	if err != nil {
		t.Fatalf("expected verification to pass when only evidence markers are unchecked, got error: %v; output: %s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "warning: evidence marker") || !strings.Contains(output, "is present but unchecked") {
		t.Fatalf("expected unchecked evidence marker warning, output: %s", output)
	}
}
