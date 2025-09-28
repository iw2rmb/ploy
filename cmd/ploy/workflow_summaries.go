package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// printBuildGateSummary renders static check and knowledge base findings from the build gate stage.
func printBuildGateSummary(w io.Writer, checkpoints []contracts.WorkflowCheckpoint) {
	if len(checkpoints) == 0 {
		return
	}
	var metadata *contracts.BuildGateStageMetadata
	for i := range checkpoints {
		checkpoint := checkpoints[i]
		if strings.TrimSpace(checkpoint.Stage) != "build-gate" {
			continue
		}
		if checkpoint.StageMetadata == nil || checkpoint.StageMetadata.BuildGate == nil {
			continue
		}
		metadata = checkpoint.StageMetadata.BuildGate
	}
	if metadata == nil {
		return
	}
	digest := strings.TrimSpace(metadata.LogDigest)
	hasStaticChecks := len(metadata.StaticChecks) > 0
	hasFindings := len(metadata.LogFindings) > 0
	if !hasStaticChecks && !hasFindings && digest == "" {
		return
	}
	_, _ = fmt.Fprintln(w, "Build Gate Summary:")
	if hasStaticChecks {
		_, _ = fmt.Fprintln(w, "  Static Checks:")
		for _, check := range metadata.StaticChecks {
			language := strings.TrimSpace(check.Language)
			tool := strings.TrimSpace(check.Tool)
			if tool == "" {
				tool = "unknown"
			}
			status := "PASSED"
			if !check.Passed {
				status = "FAILED"
			}
			failureCount := len(check.Failures)
			issueSuffix := ""
			if failureCount > 0 {
				label := "issues"
				if failureCount == 1 {
					label = "issue"
				}
				issueSuffix = fmt.Sprintf(" (%d %s)", failureCount, label)
			}
			if language != "" {
				_, _ = fmt.Fprintf(w, "    - %s (%s): %s%s\n", tool, language, status, issueSuffix)
			} else {
				_, _ = fmt.Fprintf(w, "    - %s: %s%s\n", tool, status, issueSuffix)
			}
			for _, failure := range check.Failures {
				severity := strings.ToUpper(strings.TrimSpace(failure.Severity))
				if severity == "" {
					severity = "ERROR"
				}
				parts := []string{fmt.Sprintf("[%s]", severity)}
				if trimmed := strings.TrimSpace(failure.RuleID); trimmed != "" {
					parts = append(parts, trimmed)
				}
				message := strings.TrimSpace(failure.Message)
				if message != "" {
					parts = append(parts, message)
				}
				location := strings.TrimSpace(failure.File)
				if location != "" {
					if failure.Line > 0 {
						location = fmt.Sprintf("%s:%d", location, failure.Line)
						if failure.Column > 0 {
							location = fmt.Sprintf("%s:%d", location, failure.Column)
						}
					}
					parts = append(parts, fmt.Sprintf("(%s)", location))
				}
				_, _ = fmt.Fprintf(w, "      • %s\n", strings.Join(parts, " "))
			}
		}
	}
	if hasFindings {
		_, _ = fmt.Fprintln(w, "  Knowledge Base Findings:")
		for _, finding := range metadata.LogFindings {
			severity := strings.ToUpper(strings.TrimSpace(finding.Severity))
			if severity == "" {
				severity = "ERROR"
			}
			code := strings.TrimSpace(finding.Code)
			message := strings.TrimSpace(finding.Message)
			if message == "" {
				continue
			}
			if code != "" {
				_, _ = fmt.Fprintf(w, "    - [%s] %s: %s\n", severity, code, message)
			} else {
				_, _ = fmt.Fprintf(w, "    - [%s] %s\n", severity, message)
			}
			if evidence := strings.TrimSpace(finding.Evidence); evidence != "" {
				_, _ = fmt.Fprintf(w, "      Evidence: %s\n", evidence)
			}
		}
	}
	if digest != "" {
		_, _ = fmt.Fprintf(w, "  Log Digest: %s\n", digest)
	}
}

// printAsterSummary summarises the most recent Aster bundle usage per stage.
func printAsterSummary(w io.Writer, invocations []runner.StageInvocation) {
	if len(invocations) == 0 {
		return
	}
	latest := make(map[string]runner.Stage)
	for _, invocation := range invocations {
		stage := invocation.Stage
		if strings.TrimSpace(stage.Name) == "" {
			continue
		}
		latest[stage.Name] = stage
	}
	if len(latest) == 0 {
		return
	}
	names := make([]string, 0, len(latest))
	for name := range latest {
		names = append(names, name)
	}
	sort.Strings(names)
	_, _ = fmt.Fprintln(w, "Aster Bundles:")
	for _, name := range names {
		stage := latest[name]
		if !stage.Aster.Enabled || len(stage.Aster.Bundles) == 0 {
			_, _ = fmt.Fprintf(w, "  %s: disabled\n", name)
			continue
		}
		bundleSummaries := make([]string, len(stage.Aster.Bundles))
		for i, bundle := range stage.Aster.Bundles {
			id := strings.TrimSpace(bundle.BundleID)
			if id == "" {
				id = fmt.Sprintf("%s-%s", bundle.Stage, bundle.Toggle)
			}
			if bundle.ArtifactCID != "" {
				bundleSummaries[i] = fmt.Sprintf("%s (%s)", id, bundle.ArtifactCID)
			} else if bundle.Digest != "" {
				bundleSummaries[i] = fmt.Sprintf("%s [%s]", id, bundle.Digest)
			} else {
				bundleSummaries[i] = id
			}
		}
		_, _ = fmt.Fprintf(w, "  %s: %s (toggles: %s)\n", name, strings.Join(bundleSummaries, ", "), strings.Join(stage.Aster.Toggles, ", "))
	}
}
