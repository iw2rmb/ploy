package runs

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TextRenderOptions controls optional features for the text report renderer.
type TextRenderOptions struct {
	EnableOSC8    bool
	AuthToken     string
	BaseURL       *url.URL
	SpinnerFrame  int
	LiveDurations bool
	Now           time.Time
}

// RunReportDynamicSection represents one mutable block of lines in rendered run text.
type RunReportDynamicSection struct {
	StartLine int
	LineCount int
	Text      string
}

// RunReportTextLayout is a rendered run report with per-repo mutable sections.
type RunReportTextLayout struct {
	Text            string
	LineCount       int
	DynamicSections []RunReportDynamicSection
}

// RenderRunReportText renders a one-shot, follow-style run snapshot.
func RenderRunReportText(w io.Writer, report RunReport, opts TextRenderOptions) error {
	if w == nil {
		return fmt.Errorf("run report text: output writer required")
	}

	layout, err := RenderRunReportTextLayout(report, opts)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(w, layout.Text)
	return nil
}

// RenderRunReportTextLayout renders run report text plus mutable per-repo row sections.
func RenderRunReportTextLayout(report RunReport, opts TextRenderOptions) (RunReportTextLayout, error) {
	runByRepoID := make(map[domaintypes.MigRepoID]RunEntry, len(report.Runs))
	for _, entry := range report.Runs {
		runByRepoID[entry.RepoID] = entry
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	headerLines := []string{
		fmt.Sprintf("   Mig:   %s", renderMigHeader(report.MigID.String(), report.MigName)),
		fmt.Sprintf("   Spec:  %s | %s", valueOrDash(report.SpecID.String()), renderOptionalLink("Download", buildSpecDownloadURL(report, opts.BaseURL), opts.EnableOSC8, opts.AuthToken)),
		fmt.Sprintf("   Repos: %d", len(report.Repos)),
		fmt.Sprintf("   Run:   %s", valueOrDash(report.RunID.String())),
		"",
	}

	if len(report.Repos) == 0 {
		var out strings.Builder
		for _, line := range headerLines {
			out.WriteString(line)
			out.WriteByte('\n')
		}
		out.WriteString("No repos found in this run.\n")
		rendered := out.String()
		return RunReportTextLayout{
			Text:            rendered,
			LineCount:       strings.Count(rendered, "\n"),
			DynamicSections: nil,
		}, nil
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
				"   [%d/%d] %s %s -> %s",
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

		repoFrame.Columns = []string{"", "Step", "Job", "Node", "Image", "Duration", "Artefacts"}
		repoFrame.Rows = make([]FollowStepRow, 0, len(entry.Jobs))
		for _, job := range entry.Jobs {
			buildLogURL := firstNonEmpty(strings.TrimSpace(job.BuildLogURL), strings.TrimSpace(entry.BuildLogURL), strings.TrimSpace(repo.BuildLogURL))
			patchURL := strings.TrimSpace(job.PatchURL)
			state := ColoredStatusGlyph(job.Status, opts.SpinnerFrame)
			step := renderStepName(job.JobType)
			duration := FormatDurationForStatus(job.Status, job.DurationMs, job.StartedAt, job.FinishedAt, now)
			if !opts.LiveDurations && !isTerminalJobStatus(job.Status) {
				duration = FormatDurationCompact(job.DurationMs)
			}

			repoFrame.Rows = append(repoFrame.Rows, FollowStepRow{
				Cells: []string{
					state,
					step,
					valueOrDash(job.JobID.String()),
					FormatNodeID(job.NodeID),
					valueOrDash(strings.TrimSpace(job.JobImage)),
					duration,
					renderArtifactsForStatus(job.Status, buildLogURL, patchURL, opts),
				},
				ExitOneLiner: renderExitOneLiner(job, entry.LastError),
			})
		}
		frame.Repos = append(frame.Repos, repoFrame)
	}

	frameLayout := RenderFollowFrameTextLayout(frame, FollowFrameOptions{})

	var out strings.Builder
	for _, line := range headerLines {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	out.WriteString(frameLayout.Text)
	rendered := out.String()

	dynamicSections := make([]RunReportDynamicSection, len(frameLayout.Sections))
	headerLineCount := len(headerLines)
	for i, section := range frameLayout.Sections {
		dynamicSections[i] = RunReportDynamicSection{
			StartLine: headerLineCount + section.StartLine,
			LineCount: section.LineCount,
			Text:      section.Text,
		}
	}

	return RunReportTextLayout{
		Text:            rendered,
		LineCount:       strings.Count(rendered, "\n"),
		DynamicSections: dynamicSections,
	}, nil
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
	statusNorm := strings.ToLower(strings.TrimSpace(status))
	if statusNorm == "cancelled" || statusNorm == "canceled" || !isTerminalJobStatus(status) {
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
	prefix := ""
	if strings.EqualFold(strings.TrimSpace(job.JobType), "heal") {
		msg = strings.TrimSpace(job.ActionSummary)
		if msg == "" {
			msg = "healer output unavailable"
		}
	} else {
		msg = strings.Join(strings.Fields(strings.TrimSpace(job.BugSummary)), " ")
		if msg == "" {
			msg = FormatErrorOneLiner(repoLastError)
		}
		if msg == "" {
			msg = strings.ToLower(strings.TrimSpace(job.Status))
		}
		if isGateJobType(job.JobType) {
			errorKind := "unknown"
			if job.Recovery != nil && strings.TrimSpace(job.Recovery.ErrorKind) != "" {
				errorKind = strings.ToLower(strings.TrimSpace(job.Recovery.ErrorKind))
			}
			prefix = "\x1b[1;91m<" + errorKind + ">\x1b[0m "
		}
		msg = "\x1b[91m" + msg + "\x1b[0m"
	}

	return prefix + "└  Exit " + renderExitCode(job.ExitCode) + ": " + msg
}

func isGateJobType(jobType string) bool {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "pre_gate", "post_gate", "re_gate":
		return true
	default:
		return false
	}
}

func renderExitCode(exitCode *int32) string {
	if exitCode == nil {
		return "?"
	}
	return strconv.FormatInt(int64(*exitCode), 10)
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
