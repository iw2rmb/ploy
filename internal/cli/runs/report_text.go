package runs

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

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
		block := strings.Join(append(headerLines, "No repos found in this run."), "\n")
		rendered := lipgloss.NewStyle().Render(block) + "\n"
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

		if len(repo.Jobs) == 0 {
			repoFrame.EmptyLine = "  Jobs: none"
			frame.Repos = append(frame.Repos, repoFrame)
			continue
		}

		repoFrame.Columns = []string{"", "Step", "Job", "Node", "Image", "Duration", "Artefacts"}
		repoFrame.Rows = make([]FollowStepRow, 0, len(repo.Jobs))
		for _, job := range repo.Jobs {
			patchURL := strings.TrimSpace(job.PatchURL)
			state := ColoredStatusGlyph(job.Status.String(), opts.SpinnerFrame)
			step := renderStepName(job.JobType.String())
			jobIDCell := renderOptionalLink(valueOrDash(job.JobID.String()), job.JobLogURL, opts.EnableOSC8, opts.AuthToken)
			duration := FormatDurationForStatus(job.Status.String(), job.DurationMs, job.StartedAt, job.FinishedAt, now)
			if !opts.LiveDurations && !isTerminalJobStatus(job.Status.String()) {
				duration = FormatDurationCompact(job.DurationMs)
			}

			repoFrame.Rows = append(repoFrame.Rows, FollowStepRow{
				Cells: []string{
					state,
					step,
					jobIDCell,
					FormatNodeID(job.NodeID),
					valueOrDash(strings.TrimSpace(job.JobImage)),
					duration,
					renderArtifactsForStatus(job.Status.String(), patchURL, opts),
				},
				ExitOneLiner: renderExitOneLiner(job, repo.LastError),
			})
		}
		frame.Repos = append(frame.Repos, repoFrame)
	}

	frameLayout := RenderFollowFrameTextLayout(frame)

	var out strings.Builder
	for _, line := range headerLines {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	out.WriteString(frameLayout.Text)
	rendered := lipgloss.NewStyle().Render(out.String())

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

func renderArtifacts(patchURL string, opts TextRenderOptions) string {
	patchURL = strings.TrimSpace(patchURL)
	if patchURL == "" {
		return "-"
	}
	return renderLink("Patch", patchURL, opts.EnableOSC8, opts.AuthToken)
}

func renderArtifactsForStatus(status, patchURL string, opts TextRenderOptions) string {
	s := normalizeStatus(status)
	if s == "cancelled" || s == "canceled" || !isTerminalJobStatus(status) {
		return "-"
	}
	return renderArtifacts(patchURL, opts)
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
	switch normalizeStatus(jobType) {
	case "heal":
		return "Heal"
	default:
		return valueOrDash(strings.TrimSpace(jobType))
	}
}

func renderExitOneLiner(job RunJobEntry, repoLastError *string) string {
	shouldRender := isFailedOrCrashedStatus(job.Status.String()) || normalizeStatus(job.JobType.String()) == "heal"
	if !shouldRender {
		return ""
	}

	if isGateJobType(job.JobType.String()) {
		return renderWrappedExitOneLiner(renderExitCode(job.ExitCode), "Error", true)
	}

	msg := ""
	colorizeContent := false
	if normalizeStatus(job.JobType.String()) == "heal" {
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
			msg = normalizeStatus(job.Status.String())
		}
		colorizeContent = true
	}

	return renderWrappedExitOneLiner(renderExitCode(job.ExitCode), msg, colorizeContent)
}

func renderWrappedExitOneLiner(exitCode, content string, colorizeContent bool) string {
	const wrapWidth = 100

	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	prefix := "└  Exit " + exitCode + ": "
	indent := strings.Repeat(" ", len(prefix))
	wrapped := wrapFixedWidth(content, wrapWidth)
	lines := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		if colorizeContent {
			line = colorizeErrorText(line)
		}
		if i == 0 {
			lines = append(lines, prefix+line)
			continue
		}
		lines = append(lines, indent+line)
	}
	return strings.Join(lines, "\n")
}

func wrapFixedWidth(content string, width int) []string {
	if width <= 0 {
		return []string{content}
	}
	runes := []rune(content)
	if len(runes) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > 0 {
		chunkLen := width
		if len(runes) < width {
			chunkLen = len(runes)
		}
		lines = append(lines, string(runes[:chunkLen]))
		runes = runes[chunkLen:]
	}
	return lines
}

func isGateJobType(jobType string) bool {
	switch normalizeStatus(jobType) {
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

