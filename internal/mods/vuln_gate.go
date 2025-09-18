package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	nvdapi "github.com/iw2rmb/ploy/api/nvd"
	sbomanalysis "github.com/iw2rmb/ploy/api/sbom"
	securityapi "github.com/iw2rmb/ploy/api/security"
	"github.com/iw2rmb/ploy/internal/utils"
)

// modsSBOMEnabled returns whether controller-side SBOM generation is enabled for Mods
func (r *ModRunner) sbomEnabled() bool {
	if r.config != nil && r.config.SBOM != nil {
		return r.config.SBOM.Enabled
	}
	v := strings.ToLower(os.Getenv("PLOY_MODS_SBOM_ENABLED"))
	return v != "false" && v != "0" && v != "off"
}

func (r *ModRunner) sbomFailOnError() bool {
	if r.config != nil && r.config.SBOM != nil {
		return r.config.SBOM.FailOnError
	}
	v := strings.ToLower(os.Getenv("PLOY_MODS_SBOM_FAIL_ON_ERROR"))
	return v == "true" || v == "1" || v == "on"
}

// Vulnerability gate config helpers
func (r *ModRunner) vulnEnabled() bool {
	if r.config != nil && r.config.Security != nil {
		return r.config.Security.Enabled
	}
	v := strings.ToLower(os.Getenv("PLOY_MODS_VULN_ENABLED"))
	return v == "true" || v == "1" || v == "on"
}

func (r *ModRunner) vulnMinSeverity() string {
	if r.config != nil && r.config.Security != nil && r.config.Security.MinSeverity != "" {
		return strings.ToLower(r.config.Security.MinSeverity)
	}
	v := strings.ToLower(os.Getenv("PLOY_MODS_VULN_MIN_SEVERITY"))
	if v == "" {
		return "high"
	}
	return v
}

func (r *ModRunner) vulnFailOnFindings() bool {
	if r.config != nil && r.config.Security != nil {
		return r.config.Security.FailOnFindings
	}
	v := strings.ToLower(os.Getenv("PLOY_MODS_VULN_FAIL_ON_FINDINGS"))
	return v != "false" && v != "0" && v != "off"
}

// runVulnerabilityGate performs a lightweight NVD query using SBOM dependencies
func (r *ModRunner) runVulnerabilityGate(ctx context.Context, repoPath string) error {
	sbomPath := filepath.Join(repoPath, ".sbom.json")
	if !utils.FileExists(sbomPath) {
		r.emit(ctx, "vuln", "nvd", "warn", "SBOM not found; skipping vulnerability gate")
		return nil
	}

	// Load SBOM and extract dependencies via analyzer
	var sbomData map[string]interface{}
	if b, err := os.ReadFile(sbomPath); err == nil {
		_ = json.Unmarshal(b, &sbomData)
	}
	deps, _ := sbomanalysis.NewSyftSBOMAnalyzer().ExtractDependencies(sbomData)
	if len(deps) == 0 {
		r.emit(ctx, "vuln", "nvd", "info", "No dependencies found in SBOM; skipping")
		return nil
	}

	// Configure NVD client from env (NVD_*), consistent with server wiring
	nvd := nvdapi.NewNVDDatabase()
	if apiKey := os.Getenv("NVD_API_KEY"); apiKey != "" {
		nvd.SetAPIKey(apiKey)
	}
	if base := os.Getenv("NVD_BASE_URL"); base != "" {
		nvd.SetBaseURL(base)
	}
	if to := os.Getenv("NVD_TIMEOUT_MS"); to != "" {
		if ms, err := strconv.Atoi(to); err == nil && ms > 0 {
			nvd.SetHTTPTimeout(time.Duration(ms) * time.Millisecond)
		}
	}

	// Query NVD per dependency name (keyword search); coarse but effective
	sevRank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	threshold := sevRank[r.vulnMinSeverity()]
	total := 0
	hitsAtOrAbove := 0

	for _, d := range deps {
		q := securityapi.VulnerabilityQuery{PackageName: d.Name}
		vulns, err := nvd.QueryVulnerabilities(q)
		if err != nil {
			// Non-fatal; log and continue
			r.emit(ctx, "vuln", "nvd", "warn", fmt.Sprintf("query failed for %s: %v", d.Name, err))
			continue
		}
		for _, v := range vulns {
			total++
			if sevRank[strings.ToLower(v.Severity)] >= threshold {
				hitsAtOrAbove++
			}
		}
	}

	msg := fmt.Sprintf("NVD scan complete: total=%d findings>=min=%d min_severity=%s", total, hitsAtOrAbove, r.vulnMinSeverity())
	if hitsAtOrAbove > 0 {
		if r.vulnFailOnFindings() {
			r.emit(ctx, "vuln", "nvd", "error", msg)
			return fmt.Errorf("vulnerability gate failed: %s", msg)
		}
		r.emit(ctx, "vuln", "nvd", "warn", msg)
		return nil
	}
	r.emit(ctx, "vuln", "nvd", "info", msg)
	return nil
}
