package guards

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoadmapVerifySkipsTargetedPhaseWhenNotDone(t *testing.T) {
	tmp := t.TempDir()
	phasePath := filepath.Join(tmp, "phase-3-delivery-gates-and-observability.yaml")
	indexPath := filepath.Join(tmp, "index.md")
	phase := strings.TrimSpace(`
title: "Phase 3"
done: false
reviews: []
items:
  - done: false
    label: "1"
    summary: "item"
`) + "\n"
	index := "- [ ] `phase-3-delivery-gates-and-observability.yaml` <!-- evidence:phase-3-delivery-gates-and-observability -->\n"
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
		t.Fatalf("expected verification to pass for targeted done=false phase, output: %s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "roadmap verification passed (0 phases checked)") {
		t.Fatalf("expected not-done phase to be skipped, output: %s", output)
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
	tmp := t.TempDir()
	phase1 := filepath.Join(tmp, "phase-1-conditional-planning-and-preflight.yaml")
	phase2 := filepath.Join(tmp, "phase-2-runtime-execution-and-ingestion.yaml")
	phase3 := filepath.Join(tmp, "phase-3-delivery-gates-and-observability.yaml")
	indexPath := filepath.Join(tmp, "index.md")

	donePhase := strings.TrimSpace(`
title: "Done"
done: true
reviews: []
items:
  - done: true
    label: "1"
    summary: "item"
    verification:
      - "assert"
    reviews:
      - commit: abcdef12
        gaps: []
`) + "\n"
	notDonePhase := strings.TrimSpace(`
title: "Not Done"
done: false
reviews: []
items:
  - done: false
    label: "1"
    summary: "item"
`) + "\n"
	index := strings.Join([]string{
		"- [x] `phase-1-conditional-planning-and-preflight.yaml` <!-- evidence:phase-1-conditional-planning-and-preflight -->",
		"- [x] `phase-2-runtime-execution-and-ingestion.yaml` <!-- evidence:phase-2-runtime-execution-and-ingestion -->",
		"- [ ] `phase-3-delivery-gates-and-observability.yaml` <!-- evidence:phase-3-delivery-gates-and-observability -->",
		"",
	}, "\n")
	if err := os.WriteFile(phase1, []byte(donePhase), 0o644); err != nil {
		t.Fatalf("write phase1: %v", err)
	}
	if err := os.WriteFile(phase2, []byte(donePhase), 0o644); err != nil {
		t.Fatalf("write phase2: %v", err)
	}
	if err := os.WriteFile(phase3, []byte(notDonePhase), 0o644); err != nil {
		t.Fatalf("write phase3: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte(index), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	repoRoot := mustFindRepoRoot(t)
	phases := []string{
		phase1,
		phase2,
		phase3,
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
