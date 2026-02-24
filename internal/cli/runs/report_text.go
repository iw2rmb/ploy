package runs

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TextRenderOptions controls optional features for the text report renderer.
type TextRenderOptions struct {
	EnableOSC8 bool
}

// RenderRunReportText renders a one-shot, follow-style run snapshot.
func RenderRunReportText(w io.Writer, report RunReport, opts TextRenderOptions) error {
	if w == nil {
		return fmt.Errorf("run report text: output writer required")
	}

	_, _ = fmt.Fprintf(w, "Run: %s\n", report.RunID)
	_, _ = fmt.Fprintf(w, "Status: %s\n", deriveRunStatus(report))
	_, _ = fmt.Fprintf(w, "Mig Name: %s\n", valueOrDash(strings.TrimSpace(report.MigName)))
	_, _ = fmt.Fprintf(w, "Mig ID: %s\n", valueOrDash(report.MigID.String()))
	_, _ = fmt.Fprintf(w, "Spec ID: %s\n", valueOrDash(report.SpecID.String()))
	_, _ = fmt.Fprintf(w, "Repos: %d\n", len(report.Repos))

	if len(report.Repos) == 0 {
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "No repos found in this run.")
		return nil
	}

	runByRepoID := make(map[domaintypes.MigRepoID]RunEntry, len(report.Runs))
	for _, entry := range report.Runs {
		runByRepoID[entry.RepoID] = entry
	}

	for i, repo := range report.Repos {
		if i > 0 {
			_, _ = fmt.Fprintln(w, "")
		}

		repoLabel := strings.TrimSpace(repo.RepoURL)
		if repoLabel != "" {
			repoLabel = domaintypes.NormalizeRepoURLSchemless(repoLabel)
		} else {
			repoLabel = repo.RepoID.String()
		}

		_, _ = fmt.Fprintf(w, "Repo: %s %s -> %s\n", repoLabel, valueOrDash(strings.TrimSpace(repo.BaseRef)), valueOrDash(strings.TrimSpace(repo.TargetRef)))
		_, _ = fmt.Fprintf(w, "  Repo Status: %s  Attempt: %d\n", statusToken(repo.Status), repo.Attempt)
		_, _ = fmt.Fprintf(w, "  Build Log: %s\n", renderLink("build-log", repo.BuildLogURL, opts.EnableOSC8))
		_, _ = fmt.Fprintf(w, "  Patch: %s\n", renderLink("patch", repo.PatchURL, opts.EnableOSC8))

		repoErr := formatErrorOneLiner(repo.LastError)
		if repoErr != "" {
			_, _ = fmt.Fprintf(w, "  Error: %s\n", repoErr)
		}

		entry, ok := runByRepoID[repo.RepoID]
		if !ok || len(entry.Jobs) == 0 {
			_, _ = fmt.Fprintln(w, "  Jobs: none")
			continue
		}

		tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  State\tStep\tJob ID\tNode\tImage\tDuration\tBuild Log\tPatch")
		for _, job := range entry.Jobs {
			buildLogURL := firstNonEmpty(strings.TrimSpace(job.BuildLogURL), strings.TrimSpace(entry.BuildLogURL), strings.TrimSpace(repo.BuildLogURL))
			patchURL := firstNonEmpty(strings.TrimSpace(job.PatchURL), strings.TrimSpace(entry.PatchURL), strings.TrimSpace(repo.PatchURL))

			_, _ = fmt.Fprintf(
				tw,
				"  %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				statusToken(job.Status),
				valueOrDash(strings.TrimSpace(job.JobType)),
				valueOrDash(job.JobID.String()),
				formatNodeID(job.NodeID),
				valueOrDash(strings.TrimSpace(job.JobImage)),
				formatDurationMs(job.DurationMs),
				renderLink("build-log", buildLogURL, opts.EnableOSC8),
				renderLink("patch", patchURL, opts.EnableOSC8),
			)
		}
		_ = tw.Flush()
	}

	return nil
}

func deriveRunStatus(report RunReport) string {
	statuses := make([]string, 0, len(report.Repos)+len(report.Runs))
	for _, repo := range report.Repos {
		status := strings.TrimSpace(repo.Status)
		if status != "" {
			statuses = append(statuses, status)
		}
	}
	if len(statuses) == 0 {
		for _, run := range report.Runs {
			status := strings.TrimSpace(run.Status)
			if status != "" {
				statuses = append(statuses, status)
			}
		}
	}

	if len(statuses) == 0 {
		return "Unknown"
	}

	hasRunning := false
	hasQueued := false
	hasFail := false
	hasSuccess := false
	hasCancelled := false

	for _, status := range statuses {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "running", "started":
			hasRunning = true
		case "queued", "created":
			hasQueued = true
		case "fail", "failed":
			hasFail = true
		case "success", "succeeded":
			hasSuccess = true
		case "cancelled", "canceled":
			hasCancelled = true
		}
	}

	switch {
	case hasRunning:
		return "Running"
	case hasQueued:
		return "Queued"
	case hasFail:
		return "Fail"
	case hasSuccess && hasCancelled:
		return "Partial"
	case hasSuccess:
		return "Success"
	case hasCancelled:
		return "Cancelled"
	default:
		return strings.TrimSpace(statuses[0])
	}
}

func statusToken(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "started":
		return "[RUN]"
	case "success", "succeeded":
		return "[OK]"
	case "fail", "failed":
		return "[FAIL]"
	case "cancelled", "canceled":
		return "[CANCEL]"
	case "queued", "created":
		return "[QUEUE]"
	default:
		clean := strings.TrimSpace(status)
		if clean == "" {
			return "[UNKNOWN]"
		}
		return "[" + strings.ToUpper(clean) + "]"
	}
}

func formatDurationMs(durationMs int64) string {
	if durationMs <= 0 {
		return "-"
	}
	if durationMs < 1000 {
		return fmt.Sprintf("%dms", durationMs)
	}
	return fmt.Sprintf("%.1fs", float64(durationMs)/1000.0)
}

func formatNodeID(nodeID *domaintypes.NodeID) string {
	if nodeID == nil || nodeID.IsZero() {
		return "-"
	}
	return nodeID.String()
}

func renderLink(label, rawURL string, enableOSC8 bool) string {
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return "-"
	}
	if !enableOSC8 {
		return fmt.Sprintf("%s (%s)", label, url)
	}
	return "\x1b]8;;" + url + "\x1b\\" + label + "\x1b]8;;\x1b\\"
}

func formatErrorOneLiner(lastErr *string) string {
	if lastErr == nil {
		return ""
	}
	fields := strings.Fields(*lastErr)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func valueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
