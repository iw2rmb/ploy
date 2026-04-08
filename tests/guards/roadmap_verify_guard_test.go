package guards

import (
	"os"
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
		t.Fatalf("expected verification to fail for targeted done=false phase, output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "error: targeted phase not done") {
		t.Fatalf("expected targeted not-done failure output, output: %s", output)
	}
}

func TestRoadmapVerifyFailsWhenNotDonePhaseHasCheckedEvidence(t *testing.T) {
	tmp := t.TempDir()
	phasePath := filepath.Join(tmp, "phase-1-sample.yaml")
	indexPath := filepath.Join(tmp, "index.md")

	phase := strings.TrimSpace(`
title: "Sample"
done: false
reviews: []
items:
  - done: false
    label: "1"
    summary: "item"
`) + "\n"

	index := "- [x] `phase-1-sample.yaml` <!-- evidence:phase-1-sample -->\n"

	if err := os.WriteFile(phasePath, []byte(phase), 0o644); err != nil {
		t.Fatalf("write phase: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte(index), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	repoRoot := mustFindRepoRoot(t)
	cmd := exec.Command("bash", "tools/roadmap/verify_done.sh", phasePath)
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected verification to fail when not-done phase has checked evidence, output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "error: evidence marker 'evidence:phase-1-sample' is checked while") {
		t.Fatalf("expected checked-evidence-on-not-done error output, output: %s", output)
	}
}

func TestRoadmapVerifyPassesForSbomRemediationPhasesWhenDoneAndEvidenceConsistent(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	phases := []string{
		filepath.Join("roadmap", "sbom-hooks-remediation", "phase-1-conditional-planning-and-preflight.yaml"),
		filepath.Join("roadmap", "sbom-hooks-remediation", "phase-2-runtime-execution-and-ingestion.yaml"),
	}

	args := append([]string{"tools/roadmap/verify_done.sh"}, phases...)
	cmd := exec.Command("bash", args...)
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected verification to pass when done/evidence are consistent, output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "roadmap verification passed (2 phases checked)") {
		t.Fatalf("expected successful verification output with checked count, output: %s", output)
	}
}

func TestRoadmapVerifyPassesWhenDoneAndEvidenceConsistent(t *testing.T) {
	tmp := t.TempDir()
	phasePath := filepath.Join(tmp, "phase-1-sample.yaml")
	indexPath := filepath.Join(tmp, "index.md")

	phase := strings.TrimSpace(`
title: "Sample"
done: true
reviews: []
items:
  - done: true
    label: "1"
    summary: "item"
    verification:
      - "assert something"
    reviews:
      - commit: abcdef12
        gaps: []
`) + "\n"

	index := "- [x] `phase-1-sample.yaml` <!-- evidence:phase-1-sample -->\n"

	if err := os.WriteFile(phasePath, []byte(phase), 0o644); err != nil {
		t.Fatalf("write phase: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte(index), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	repoRoot := mustFindRepoRoot(t)
	cmd := exec.Command("bash", "tools/roadmap/verify_done.sh", phasePath)
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected verification to pass when done/evidence are consistent, output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "roadmap verification passed (1 phase checked)") {
		t.Fatalf("expected successful verification output, got: %s", output)
	}
}
