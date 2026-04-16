package nodeagent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestMaterializeValidatedSBOMOutput_WritesCanonicalSnapshot(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)

	rawDeps := strings.Join([]string{
		"[INFO]    com.fasterxml.jackson.core:jackson-databind:jar:2.17.2:compile",
		"\\--- org.apache.commons:commons-lang3:3.17.0",
		"+--- org.openapi.generator:org.openapi.generator.gradle.plugin:6.6.0",
		"noise line that should be ignored",
	}, "\n")
	if err := os.WriteFile(filepath.Join(outDir, sbomDependencyOutputFileName), []byte(rawDeps), 0o644); err != nil {
		t.Fatalf("write raw dependency output: %v", err)
	}
	rawClasspath := []byte("/home/gradle/.gradle/caches/modules-2/files-2.1/a/b/c/a.jar\n")
	if err := os.WriteFile(filepath.Join(outDir, sbomJavaClasspathFileName), rawClasspath, 0o644); err != nil {
		t.Fatalf("write java classpath output: %v", err)
	}

	if err := materializeValidatedSBOMOutput(outDir, snapshotPath, true); err != nil {
		t.Fatalf("materializeValidatedSBOMOutput: %v", err)
	}

	canonicalPath := filepath.Join(outDir, preGateCanonicalSBOMFileName)
	canonicalRaw, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical sbom output: %v", err)
	}
	if err := validateCanonicalSBOMDocument(canonicalRaw); err != nil {
		t.Fatalf("validate canonical sbom output: %v", err)
	}
	stagedRaw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read staged sbom snapshot: %v", err)
	}
	if string(stagedRaw) != string(canonicalRaw) {
		t.Fatalf("staged snapshot mismatch with canonical output")
	}
}

func TestMaterializeValidatedSBOMOutput_ErrorsWhenDependencyOutputMissing(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)

	err := materializeValidatedSBOMOutput(outDir, snapshotPath, true)
	if err == nil {
		t.Fatal("expected error for missing dependency output")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
	if !strings.Contains(err.Error(), sbomDependencyOutputFileName) {
		t.Fatalf("error = %q, want mention of %s", err, sbomDependencyOutputFileName)
	}
}

func TestMaterializeValidatedSBOMOutput_ClasspathValidationDependsOnFlow(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)
	rawDeps := "[INFO]    com.fasterxml.jackson.core:jackson-databind:jar:2.17.2:compile\n"
	if err := os.WriteFile(filepath.Join(outDir, sbomDependencyOutputFileName), []byte(rawDeps), 0o644); err != nil {
		t.Fatalf("write raw dependency output: %v", err)
	}

	if err := materializeValidatedSBOMOutput(outDir, snapshotPath, true); err == nil {
		t.Fatal("expected error for missing java classpath when classpath is required")
	}
	if err := materializeValidatedSBOMOutput(outDir, snapshotPath, false); err != nil {
		t.Fatalf("expected snapshot-only flow to pass without java classpath: %v", err)
	}
}

func TestFinalizeSBOMFlowOutputs_PersistsRunClasspathOnlyForPreGate(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)
	runID := types.NewRunID()
	classpathPath := runJavaClasspathPath(runID)

	preOutDir := t.TempDir()
	preSnapshotPath := filepath.Join(t.TempDir(), "pre-cycle", preGateCanonicalSBOMFileName)
	if err := os.WriteFile(filepath.Join(preOutDir, sbomDependencyOutputFileName), []byte("a:b:1.0.0\n"), 0o644); err != nil {
		t.Fatalf("write pre dependency output: %v", err)
	}
	preClasspath := []byte("/repo/.m2/a.jar\n")
	if err := os.WriteFile(filepath.Join(preOutDir, sbomJavaClasspathFileName), preClasspath, 0o644); err != nil {
		t.Fatalf("write pre java classpath output: %v", err)
	}
	rc := &runController{}
	if err := rc.finalizeSBOMFlowOutputs(runID, preGateCycleName, preOutDir, preSnapshotPath); err != nil {
		t.Fatalf("finalizeSBOMFlowOutputs pre-gate: %v", err)
	}
	persistedPreClasspath, err := os.ReadFile(classpathPath)
	if err != nil {
		t.Fatalf("read persisted pre-gate classpath: %v", err)
	}
	if string(persistedPreClasspath) != string(preClasspath) {
		t.Fatalf("persisted pre-gate classpath mismatch")
	}

	postOutDir := t.TempDir()
	postSnapshotPath := filepath.Join(t.TempDir(), "post-cycle", preGateCanonicalSBOMFileName)
	if err := os.WriteFile(filepath.Join(postOutDir, sbomDependencyOutputFileName), []byte("x:y:2.0.0\n"), 0o644); err != nil {
		t.Fatalf("write post dependency output: %v", err)
	}
	if err := rc.finalizeSBOMFlowOutputs(runID, postGateCycleName, postOutDir, postSnapshotPath); err != nil {
		t.Fatalf("finalizeSBOMFlowOutputs post-gate: %v", err)
	}
	persistedPostClasspath, err := os.ReadFile(classpathPath)
	if err != nil {
		t.Fatalf("read persisted classpath after post-gate flow: %v", err)
	}
	if string(persistedPostClasspath) != string(preClasspath) {
		t.Fatalf("post-gate flow overwrote persisted classpath")
	}
}
