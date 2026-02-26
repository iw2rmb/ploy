package runs

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TextRenderOptions controls optional features for the text report renderer.
type TextRenderOptions struct {
	EnableOSC8 bool
	AuthToken  string
	BaseURL    *url.URL
}

// RenderRunReportText renders a one-shot, follow-style run snapshot.
func RenderRunReportText(w io.Writer, report RunReport, opts TextRenderOptions) error {
	if w == nil {
		return fmt.Errorf("run report text: output writer required")
	}

	_, _ = fmt.Fprintf(w, "Mig:   %s\n", renderMigHeader(report.MigID.String(), report.MigName))
	_, _ = fmt.Fprintf(w, "Spec:  %s | %s\n", valueOrDash(report.SpecID.String()), renderOptionalLink("Download", buildSpecDownloadURL(report, opts.BaseURL), opts.EnableOSC8, opts.AuthToken))
	_, _ = fmt.Fprintf(w, "Repos: %d\n", len(report.Repos))
	_, _ = fmt.Fprintf(w, "Run:   %s\n", valueOrDash(report.RunID.String()))

	if len(report.Repos) == 0 {
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "No repos found in this run.")
		return nil
	}

	runByRepoID := make(map[domaintypes.MigRepoID]RunEntry, len(report.Runs))
	for _, entry := range report.Runs {
		runByRepoID[entry.RepoID] = entry
	}

	frame := FollowFrame{
		Repos: make([]FollowRepoFrame, 0, len(report.Repos)),
	}

	for i, repo := range report.Repos {
		repoLinkLabel := strings.TrimSpace(repo.RepoURL)
		if repoLinkLabel != "" {
			repoLinkLabel = domaintypes.NormalizeRepoURLSchemless(repoLinkLabel)
		} else {
			repoLinkLabel = repo.RepoID.String()
		}

		repoFrame := FollowRepoFrame{
			HeaderLine: fmt.Sprintf(
				"Repo:  [%d/%d] %s %s -> %s",
				i+1,
				len(report.Repos),
				renderOptionalLink(repoLinkLabel, repo.RepoURL, opts.EnableOSC8, ""),
				valueOrDash(strings.TrimSpace(repo.BaseRef)),
				valueOrDash(strings.TrimSpace(repo.TargetRef)),
			),
		}

		entry, ok := runByRepoID[repo.RepoID]
		if !ok || len(entry.Jobs) == 0 {
			repoFrame.EmptyLine = "  Jobs: none"
			frame.Repos = append(frame.Repos, repoFrame)
			continue
		}

		repoFrame.Columns = []string{"", "Step", "Job ID", "Node", "Image", "Duration", "Artifacts"}
		repoFrame.Rows = make([]FollowStepRow, 0, len(entry.Jobs))
		for _, job := range entry.Jobs {
			buildLogURL := firstNonEmpty(strings.TrimSpace(job.BuildLogURL), strings.TrimSpace(entry.BuildLogURL), strings.TrimSpace(repo.BuildLogURL))
			patchURL := firstNonEmpty(strings.TrimSpace(job.PatchURL), strings.TrimSpace(entry.PatchURL), strings.TrimSpace(repo.PatchURL))
			state := StatusGlyph(job.Status, 0)
			step := renderStepName(job.JobType)

			repoFrame.Rows = append(repoFrame.Rows, FollowStepRow{
				Cells: []string{
					"  " + state,
					step,
					valueOrDash(job.JobID.String()),
					FormatNodeID(job.NodeID),
					valueOrDash(strings.TrimSpace(job.JobImage)),
					FormatDurationCompact(job.DurationMs),
					renderArtifactsForStatus(job.Status, buildLogURL, patchURL, opts),
				},
				ExitOneLiner: renderExitOneLiner(job, entry.LastError),
			})
		}
		frame.Repos = append(frame.Repos, repoFrame)
	}

	rendered, _ := RenderFollowFrameText(frame, FollowFrameOptions{})
	if rendered != "" {
		_, _ = fmt.Fprint(w, rendered)
	}

	return nil
}

func renderLink(label, rawURL string, enableOSC8 bool, authToken string) string {
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return "-"
	}
	url = appendAuthToken(url, authToken)
	if !enableOSC8 {
		return fmt.Sprintf("%s (%s)", label, url)
	}
	return "\x1b]8;;" + url + "\x1b\\" + label + "\x1b]8;;\x1b\\"
}

func renderOptionalLink(label, rawURL string, enableOSC8 bool, authToken string) string {
	if strings.TrimSpace(rawURL) == "" {
		return label
	}
	return renderLink(label, rawURL, enableOSC8, authToken)
}

func renderArtifacts(logURL, patchURL string, opts TextRenderOptions) string {
	logURL = strings.TrimSpace(logURL)
	patchURL = strings.TrimSpace(patchURL)
	switch {
	case logURL == "" && patchURL == "":
		return "-"
	case logURL != "" && patchURL != "":
		return renderLink("Logs", logURL, opts.EnableOSC8, opts.AuthToken) + " | " + renderLink("Patch", patchURL, opts.EnableOSC8, opts.AuthToken)
	case logURL != "":
		return renderLink("Logs", logURL, opts.EnableOSC8, opts.AuthToken)
	default:
		return renderLink("Patch", patchURL, opts.EnableOSC8, opts.AuthToken)
	}
}

func renderArtifactsForStatus(status, logURL, patchURL string, opts TextRenderOptions) string {
	if !isTerminalJobStatus(status) {
		return "-"
	}
	return renderArtifacts(logURL, patchURL, opts)
}

func appendAuthToken(rawURL, token string) string {
	token = strings.TrimSpace(token)
	if strings.TrimSpace(rawURL) == "" || token == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	if strings.TrimSpace(query.Get("auth_token")) == "" {
		query.Set("auth_token", token)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func renderStepName(jobType string) string {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "heal":
		return "Heal"
	default:
		return valueOrDash(strings.TrimSpace(jobType))
	}
}

func renderExitOneLiner(job RunJobEntry, repoLastError *string) string {
	shouldRender := isFailedOrCrashedStatus(job.Status) || strings.EqualFold(strings.TrimSpace(job.JobType), "heal")
	if !shouldRender {
		return ""
	}

	msg := ""
	if strings.EqualFold(strings.TrimSpace(job.JobType), "heal") {
		msg = strings.TrimSpace(job.ActionSummary)
		if msg == "" {
			msg = "healer output unavailable"
		}
	} else {
		msg = FormatErrorOneLiner(repoLastError)
		if msg == "" {
			msg = strings.ToLower(strings.TrimSpace(job.Status))
		}
		msg = "\x1b[91m" + msg + "\x1b[0m"
	}

	return "└  Exit " + renderExitCode(job.ExitCode) + ": " + msg
}

func renderExitCode(exitCode *int32) string {
	if exitCode == nil {
		return "?"
	}
	return strconv.FormatInt(int64(*exitCode), 10)
}

func isFailedOrCrashedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fail", "failed", "crash", "crashed", "error":
		return true
	default:
		return false
	}
}

func isTerminalJobStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "finished", "completed", "fail", "failed", "crash", "crashed", "error":
		return true
	default:
		return false
	}
}

func renderMigHeader(migID, migName string) string {
	migID = valueOrDash(strings.TrimSpace(migID))
	migName = strings.TrimSpace(migName)
	if migName == "" || migName == migID {
		return migID
	}
	return migID + "   | " + migName
}

func buildSpecDownloadURL(report RunReport, baseURL *url.URL) string {
	if baseURL == nil || report.MigID.IsZero() {
		return ""
	}
	return baseURL.JoinPath("v1", "migs", report.MigID.String(), "specs", "latest").String()
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
