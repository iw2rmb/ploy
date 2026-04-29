// recovery_io.go contains shared recovery parsers/helpers used by discrete gate/heal jobs.
package nodeagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

const (
	orwStatsMetadataErrorKind = "orw_error_kind"
	orwStatsMetadataReason    = "orw_reason"
)

var (
	rebuildGateLogPattern = regexp.MustCompile(`^re_build-gate-(\d+)\.log$`)
	rebuildErrorsPattern  = regexp.MustCompile(`^errors-(\d+)\.yaml$`)
)

// parseORWFailureMetadata reads /out/report.json and extracts deterministic ORW
// failure fields for run stats metadata. Missing report.json returns (nil, nil).
func parseORWFailureMetadata(outDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(outDir, contracts.ORWCLIReportFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", contracts.ORWCLIReportFileName, err)
	}

	report, err := contracts.ParseORWCLIReport(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", contracts.ORWCLIReportFileName, err)
	}
	if report.Success {
		return nil, nil
	}

	meta := map[string]string{
		orwStatsMetadataErrorKind: string(report.ErrorKind),
	}
	if strings.TrimSpace(report.Reason) != "" {
		meta[orwStatsMetadataReason] = strings.TrimSpace(report.Reason)
	}
	return meta, nil
}

func gateLogPayloadFromMetadata(gateMetadata *contracts.BuildGateStageMetadata) string {
	if gateMetadata == nil {
		return ""
	}
	logPayload := gateMetadata.LogsText
	if len(gateMetadata.LogFindings) > 0 {
		if trimmed := strings.TrimSpace(gateMetadata.LogFindings[0].Message); trimmed != "" {
			logPayload = trimmed
			if !strings.HasSuffix(logPayload, "\n") {
				logPayload += "\n"
			}
		}
	}
	return logPayload
}

func parseStructuredErrorsPayload(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	switch parsed.(type) {
	case map[string]any, []any:
		return parsed, nil
	default:
		return nil, fmt.Errorf("expected object or array")
	}
}

func structuredErrorsYAML(raw json.RawMessage) ([]byte, error) {
	parsed, err := parseStructuredErrorsPayload(raw)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, nil
	}
	out, err := yaml.Marshal(parsed)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type gateLineagePair struct {
	logPayload    []byte
	errorsPayload []byte
}

func materializeParentGateLineageArtifacts(outDir string, recoveryCtx *contracts.RecoveryClaimContext) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("out dir is required")
	}

	indexed, err := loadIndexedGateLineageArtifacts(outDir)
	if err != nil {
		return err
	}

	baseline, hasBaseline, err := baselineGateLineagePair(recoveryCtx)
	if err != nil {
		return err
	}

	keys := make([]int, 0, len(indexed))
	for idx := range indexed {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	children := make([]gateLineagePair, 0, len(keys))
	for _, idx := range keys {
		if hasBaseline && idx == 1 && gateLineagePairsEqual(indexed[idx], baseline) {
			continue
		}
		children = append(children, indexed[idx])
	}
	if !hasBaseline && len(children) == 0 {
		return nil
	}

	if err := removeIndexedGateLineageArtifacts(outDir); err != nil {
		return err
	}

	nextIndex := 1
	if hasBaseline {
		if err := writeGateLineagePair(outDir, nextIndex, baseline); err != nil {
			return err
		}
		nextIndex++
	}
	for _, child := range children {
		if err := writeGateLineagePair(outDir, nextIndex, child); err != nil {
			return err
		}
		nextIndex++
	}
	return nil
}

func baselineGateLineagePair(recoveryCtx *contracts.RecoveryClaimContext) (gateLineagePair, bool, error) {
	if recoveryCtx == nil || strings.TrimSpace(recoveryCtx.BuildGateLog) == "" {
		return gateLineagePair{}, false, nil
	}
	pair := gateLineagePair{
		logPayload: []byte(recoveryCtx.BuildGateLog),
	}
	if len(recoveryCtx.Errors) > 0 {
		errorsYAML, err := structuredErrorsYAML(recoveryCtx.Errors)
		if err != nil {
			return gateLineagePair{}, false, fmt.Errorf("parse recovery_context.errors for baseline lineage: %w", err)
		}
		pair.errorsPayload = errorsYAML
	}
	return pair, true, nil
}

func loadIndexedGateLineageArtifacts(outDir string) (map[int]gateLineagePair, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[int]gateLineagePair{}, nil
		}
		return nil, fmt.Errorf("read out dir %q: %w", outDir, err)
	}

	pairs := map[int]gateLineagePair{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		switch {
		case rebuildGateLogPattern.MatchString(name):
			idx, convErr := strconv.Atoi(rebuildGateLogPattern.FindStringSubmatch(name)[1])
			if convErr != nil || idx <= 0 {
				continue
			}
			raw, readErr := os.ReadFile(filepath.Join(outDir, name))
			if readErr != nil {
				return nil, fmt.Errorf("read %s: %w", filepath.Join(outDir, name), readErr)
			}
			p := pairs[idx]
			p.logPayload = raw
			pairs[idx] = p
		case rebuildErrorsPattern.MatchString(name):
			idx, convErr := strconv.Atoi(rebuildErrorsPattern.FindStringSubmatch(name)[1])
			if convErr != nil || idx <= 0 {
				continue
			}
			raw, readErr := os.ReadFile(filepath.Join(outDir, name))
			if readErr != nil {
				return nil, fmt.Errorf("read %s: %w", filepath.Join(outDir, name), readErr)
			}
			p := pairs[idx]
			p.errorsPayload = raw
			pairs[idx] = p
		}
	}
	return pairs, nil
}

func removeIndexedGateLineageArtifacts(outDir string) error {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read out dir %q: %w", outDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if !rebuildGateLogPattern.MatchString(name) && !rebuildErrorsPattern.MatchString(name) {
			continue
		}
		if remErr := os.Remove(filepath.Join(outDir, name)); remErr != nil && !errors.Is(remErr, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", filepath.Join(outDir, name), remErr)
		}
	}
	return nil
}

func writeGateLineagePair(outDir string, index int, pair gateLineagePair) error {
	logPath := filepath.Join(outDir, fmt.Sprintf("re_build-gate-%d.log", index))
	if err := os.WriteFile(logPath, pair.logPayload, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", logPath, err)
	}
	errorsPath := filepath.Join(outDir, fmt.Sprintf("errors-%d.yaml", index))
	if err := os.WriteFile(errorsPath, pair.errorsPayload, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", errorsPath, err)
	}
	return nil
}

func gateLineagePairsEqual(a, b gateLineagePair) bool {
	return bytes.Equal(a.logPayload, b.logPayload) && bytes.Equal(a.errorsPayload, b.errorsPayload)
}
