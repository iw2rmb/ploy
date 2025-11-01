//go:build legacy
// +build legacy

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

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

// printArtifactSummary surfaces per-stage artifact references captured in checkpoints.
func printArtifactSummary(w io.Writer, checkpoints []contracts.WorkflowCheckpoint) {
	if len(checkpoints) == 0 {
		return
	}
	type stageArtifacts struct {
		order     int
		artifacts []contracts.CheckpointArtifact
		retention *contracts.CheckpointRetention
	}
	stages := make(map[string]*stageArtifacts)
	order := 0
	for _, checkpoint := range checkpoints {
		if checkpoint.Status != contracts.CheckpointStatusCompleted {
			continue
		}
		if checkpoint.StageMetadata == nil {
			continue
		}
		if len(checkpoint.Artifacts) == 0 {
			continue
		}
		name := strings.TrimSpace(checkpoint.StageMetadata.Name)
		if name == "" {
			name = strings.TrimSpace(checkpoint.Stage)
		}
		if name == "" {
			continue
		}
		entry, ok := stages[name]
		if !ok {
			entry = &stageArtifacts{order: order}
			order++
			stages[name] = entry
		}
		entry.retention = checkpoint.StageMetadata.Retention
		entry.artifacts = make([]contracts.CheckpointArtifact, len(checkpoint.Artifacts))
		copy(entry.artifacts, checkpoint.Artifacts)
	}
	if len(stages) == 0 {
		return
	}
	names := make([]string, 0, len(stages))
	for name := range stages {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return stages[names[i]].order < stages[names[j]].order
	})
	_, _ = fmt.Fprintln(w, "Stage Artifacts:")
	for _, name := range names {
		entry := stages[name]
		line := fmt.Sprintf("  %s", name)
		if entry.retention != nil {
			ttl := strings.TrimSpace(entry.retention.TTL)
			switch {
			case entry.retention.Retained && ttl != "":
				line += fmt.Sprintf(" (retained ttl=%s)", ttl)
			case entry.retention.Retained:
				line += " (retained)"
			case ttl != "":
				line += fmt.Sprintf(" (ttl=%s)", ttl)
			}
		}
		line += ":"
		_, _ = fmt.Fprintln(w, line)
		for _, artifact := range entry.artifacts {
			artifactName := strings.TrimSpace(artifact.Name)
			if artifactName == "" {
				artifactName = "artifact"
			}
			cid := strings.TrimSpace(artifact.ArtifactCID)
			line := fmt.Sprintf("    - %s: %s", artifactName, cid)
			digest := strings.TrimSpace(artifact.Digest)
			if digest != "" {
				line += fmt.Sprintf(" (%s)", digest)
			}
			_, _ = fmt.Fprintln(w, line)
		}
	}
}

// printArchiveSummary surfaces archive export metadata captured during stage execution.
func printArchiveSummary(w io.Writer, invocations []runner.StageInvocation) {
	if len(invocations) == 0 {
		return
	}
	type archiveEntry struct {
		stage   string
		runID   string
		archive *runner.StageArchive
	}
	entries := make([]archiveEntry, 0)
	for _, invocation := range invocations {
		if invocation.Archive == nil {
			continue
		}
		entries = append(entries, archiveEntry{
			stage:   invocation.Stage.Name,
			runID:   strings.TrimSpace(invocation.RunID),
			archive: invocation.Archive,
		})
	}
	if len(entries) == 0 {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].stage == entries[j].stage {
			return entries[i].runID < entries[j].runID
		}
		return entries[i].stage < entries[j].stage
	})
	_, _ = fmt.Fprintln(w, "Archive Requests:")
	for _, entry := range entries {
		queued := ""
		if !entry.archive.QueuedAt.IsZero() {
			queued = entry.archive.QueuedAt.UTC().Format(time.RFC3339)
		}
		stageName := strings.TrimSpace(entry.stage)
		if stageName == "" {
			stageName = "unknown-stage"
		}
		runLabel := entry.runID
		if runLabel == "" {
			runLabel = "(unknown)"
		}
		class := entry.archive.Class
		if class == "" {
			class = "unspecified"
		}
		if queued != "" {
			_, _ = fmt.Fprintf(w, "  %s: run=%s id=%s class=%s queued=%s\n", stageName, runLabel, entry.archive.ID, class, queued)
		} else {
			_, _ = fmt.Fprintf(w, "  %s: run=%s id=%s class=%s\n", stageName, runLabel, entry.archive.ID, class)
		}
	}
}
