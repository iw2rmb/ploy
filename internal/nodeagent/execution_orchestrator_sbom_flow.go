package nodeagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

type sbomFlowContract struct {
	requireClasspath bool
}

func sbomFlowContractForCycle(cycleName string) sbomFlowContract {
	if strings.TrimSpace(cycleName) == preGateCycleName {
		return sbomFlowContract{
			requireClasspath: true,
		}
	}
	return sbomFlowContract{}
}

func (r *runController) finalizeSBOMFlowOutputs(runID types.RunID, repoID types.MigRepoID, cycleName, outDir, snapshotPath string) error {
	contract := sbomFlowContractForCycle(cycleName)
	shareDir := runRepoShareDir(runID, repoID)
	if strings.TrimSpace(shareDir) == "" {
		return fmt.Errorf("run/repo share dir is required for sbom outputs")
	}
	if err := materializeValidatedSBOMOutput(outDir, shareDir, snapshotPath, contract.requireClasspath); err != nil {
		return err
	}
	return nil
}

func materializeValidatedSBOMOutput(outDir, shareDir string, snapshotPath string, requireClasspath bool) error {
	rawOutputPath := filepath.Join(shareDir, sbomDependencyOutputFileName)
	raw, err := os.ReadFile(rawOutputPath)
	if err != nil {
		return fmt.Errorf("read /share/%s: %w", sbomDependencyOutputFileName, err)
	}
	if requireClasspath {
		classpathPath := filepath.Join(shareDir, sbomJavaClasspathFileName)
		if err := validateJavaClasspathPath(classpathPath); err != nil {
			return fmt.Errorf("validate /share/%s: %w", sbomJavaClasspathFileName, err)
		}
	}

	canonicalRaw, err := canonicalSBOMFromDependencyOutput(raw)
	if err != nil {
		return fmt.Errorf("build canonical sbom from /share/%s: %w", sbomDependencyOutputFileName, err)
	}
	if err := validateCanonicalSBOMDocument(canonicalRaw); err != nil {
		return fmt.Errorf("validate canonical sbom payload: %w", err)
	}

	canonicalPath := filepath.Join(outDir, preGateCanonicalSBOMFileName)
	if err := os.WriteFile(canonicalPath, canonicalRaw, 0o644); err != nil {
		return fmt.Errorf("write /out/%s: %w", preGateCanonicalSBOMFileName, err)
	}
	if err := validateCanonicalSBOMPath(canonicalPath); err != nil {
		return fmt.Errorf("validate /out/%s: %w", preGateCanonicalSBOMFileName, err)
	}
	if err := copyFileBytes(canonicalPath, snapshotPath); err != nil {
		return fmt.Errorf("stage cycle sbom snapshot: %w", err)
	}
	if err := validateCanonicalSBOMPath(snapshotPath); err != nil {
		return fmt.Errorf("validate staged cycle sbom snapshot: %w", err)
	}
	return nil
}
