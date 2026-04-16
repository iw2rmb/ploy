package nodeagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

type sbomFlowContract struct {
	requireClasspath bool
	persistClasspath bool
}

func sbomFlowContractForCycle(cycleName string) sbomFlowContract {
	if strings.TrimSpace(cycleName) == preGateCycleName {
		return sbomFlowContract{
			requireClasspath: true,
			persistClasspath: true,
		}
	}
	return sbomFlowContract{}
}

func (r *runController) finalizeSBOMFlowOutputs(runID types.RunID, cycleName, outDir, snapshotPath string) error {
	contract := sbomFlowContractForCycle(cycleName)
	if err := materializeValidatedSBOMOutput(outDir, snapshotPath, contract.requireClasspath); err != nil {
		return err
	}
	if !contract.persistClasspath {
		return nil
	}
	// The run-level java.classpath contract has a single source per run.
	// Keep the first valid pre-gate source and avoid overwriting it on retry sbom jobs.
	if err := validateJavaClasspathPath(runJavaClasspathPath(runID)); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		// Invalid cached classpath is treated as unset and repaired from current output.
	}
	classpathPath := filepath.Join(outDir, sbomJavaClasspathFileName)
	if err := persistRunJavaClasspath(runID, classpathPath); err != nil {
		return fmt.Errorf("persist run java classpath from sbom output: %w", err)
	}
	return nil
}

func materializeValidatedSBOMOutput(outDir string, snapshotPath string, requireClasspath bool) error {
	rawOutputPath := filepath.Join(outDir, sbomDependencyOutputFileName)
	raw, err := os.ReadFile(rawOutputPath)
	if err != nil {
		return fmt.Errorf("read /out/%s: %w", sbomDependencyOutputFileName, err)
	}
	if requireClasspath {
		classpathPath := filepath.Join(outDir, sbomJavaClasspathFileName)
		if err := validateJavaClasspathPath(classpathPath); err != nil {
			return fmt.Errorf("validate /out/%s: %w", sbomJavaClasspathFileName, err)
		}
	}

	canonicalRaw, err := canonicalSBOMFromDependencyOutput(raw)
	if err != nil {
		return fmt.Errorf("build canonical sbom from /out/%s: %w", sbomDependencyOutputFileName, err)
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
