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
	shareDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)

	rawSBOM := []byte(`{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"ploy-generated-sbom","documentNamespace":"https://ploy.dev/sbom/generated","creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: ploy-nodeagent"]},"packages":[{"SPDXID":"SPDXRef-Package-000001","name":"org.apache.commons:commons-lang3","versionInfo":"3.17.0"}]}`)
	if err := os.WriteFile(filepath.Join(shareDir, sbomCanonicalOutputFileName), rawSBOM, 0o644); err != nil {
		t.Fatalf("write canonical sbom output: %v", err)
	}
	rawClasspath := []byte("/root/.gradle/caches/modules-2/files-2.1/a/b/c/a.jar\n")
	if err := os.WriteFile(filepath.Join(shareDir, sbomJavaClasspathFileName), rawClasspath, 0o644); err != nil {
		t.Fatalf("write java classpath output: %v", err)
	}

	if err := materializeValidatedSBOMOutput(outDir, shareDir, snapshotPath, true); err != nil {
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

func TestMaterializeValidatedSBOMOutput_ErrorsWhenSBOMOutputMissing(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	shareDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)

	err := materializeValidatedSBOMOutput(outDir, shareDir, snapshotPath, true)
	if err == nil {
		t.Fatal("expected error for missing sbom output")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
	if !strings.Contains(err.Error(), sbomCanonicalOutputFileName) {
		t.Fatalf("error = %q, want mention of %s", err, sbomCanonicalOutputFileName)
	}
}

func TestMaterializeValidatedSBOMOutput_ClasspathValidationDependsOnFlow(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	shareDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)
	rawSBOM := []byte(`{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"ploy-generated-sbom","documentNamespace":"https://ploy.dev/sbom/generated","creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: ploy-nodeagent"]},"packages":[{"SPDXID":"SPDXRef-Package-000001","name":"org.apache.commons:commons-lang3","versionInfo":"3.17.0"}]}`)
	if err := os.WriteFile(filepath.Join(shareDir, sbomCanonicalOutputFileName), rawSBOM, 0o644); err != nil {
		t.Fatalf("write canonical sbom output: %v", err)
	}

	if err := materializeValidatedSBOMOutput(outDir, shareDir, snapshotPath, true); err == nil {
		t.Fatal("expected error for missing java classpath when classpath is required")
	}
	if err := materializeValidatedSBOMOutput(outDir, shareDir, snapshotPath, false); err != nil {
		t.Fatalf("expected snapshot-only flow to pass without java classpath: %v", err)
	}
}

func TestMaterializeValidatedSBOMOutput_RejectsNonPortableGradleClasspath(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	shareDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)
	rawSBOM := []byte(`{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"ploy-generated-sbom","documentNamespace":"https://ploy.dev/sbom/generated","creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: ploy-nodeagent"]},"packages":[{"SPDXID":"SPDXRef-Package-000001","name":"org.apache.commons:commons-lang3","versionInfo":"3.17.0"}]}`)
	if err := os.WriteFile(filepath.Join(shareDir, sbomCanonicalOutputFileName), rawSBOM, 0o644); err != nil {
		t.Fatalf("write canonical sbom output: %v", err)
	}
	rawClasspath := []byte("/home/gradle/.gradle/caches/modules-2/files-2.1/a/b/c/a.jar\n")
	if err := os.WriteFile(filepath.Join(shareDir, sbomJavaClasspathFileName), rawClasspath, 0o644); err != nil {
		t.Fatalf("write java classpath output: %v", err)
	}

	err := materializeValidatedSBOMOutput(outDir, shareDir, snapshotPath, true)
	if err == nil {
		t.Fatal("expected error for non-portable gradle classpath")
	}
	if !strings.Contains(err.Error(), "non-portable gradle cache path") {
		t.Fatalf("error = %q, want mention of non-portable gradle cache path", err)
	}
}

func TestFinalizeSBOMFlowOutputs_UsesShareOutputs(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)
	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	shareDir := runRepoShareDir(runID, repoID)
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		t.Fatalf("mkdir share dir: %v", err)
	}

	preOutDir := t.TempDir()
	preSnapshotPath := filepath.Join(t.TempDir(), "pre-cycle", preGateCanonicalSBOMFileName)
	if err := os.WriteFile(filepath.Join(shareDir, sbomCanonicalOutputFileName), []byte(`{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"ploy-generated-sbom","documentNamespace":"https://ploy.dev/sbom/generated","creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: ploy-nodeagent"]},"packages":[{"SPDXID":"SPDXRef-Package-000001","name":"a:b","versionInfo":"1.0.0"}]}`), 0o644); err != nil {
		t.Fatalf("write pre sbom output: %v", err)
	}
	preClasspath := []byte("/repo/.m2/a.jar\n")
	classpathPath := filepath.Join(shareDir, sbomJavaClasspathFileName)
	if err := os.WriteFile(classpathPath, preClasspath, 0o644); err != nil {
		t.Fatalf("write pre java classpath output: %v", err)
	}
	rc := &runController{}
	if err := rc.finalizeSBOMFlowOutputs(runID, repoID, preGateCycleName, preOutDir, preSnapshotPath); err != nil {
		t.Fatalf("finalizeSBOMFlowOutputs pre-gate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(preOutDir, preGateCanonicalSBOMFileName)); err != nil {
		t.Fatalf("expected canonical sbom in /out after pre flow: %v", err)
	}

	if err := os.Remove(classpathPath); err != nil {
		t.Fatalf("remove share classpath before post flow: %v", err)
	}

	postOutDir := t.TempDir()
	postSnapshotPath := filepath.Join(t.TempDir(), "post-cycle", preGateCanonicalSBOMFileName)
	if err := os.WriteFile(filepath.Join(shareDir, sbomCanonicalOutputFileName), []byte(`{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"ploy-generated-sbom","documentNamespace":"https://ploy.dev/sbom/generated","creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: ploy-nodeagent"]},"packages":[{"SPDXID":"SPDXRef-Package-000001","name":"x:y","versionInfo":"2.0.0"}]}`), 0o644); err != nil {
		t.Fatalf("write post sbom output: %v", err)
	}
	if err := rc.finalizeSBOMFlowOutputs(runID, repoID, postGateCycleName, postOutDir, postSnapshotPath); err != nil {
		t.Fatalf("finalizeSBOMFlowOutputs post-gate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(postOutDir, preGateCanonicalSBOMFileName)); err != nil {
		t.Fatalf("expected canonical sbom in /out after post flow: %v", err)
	}
}
