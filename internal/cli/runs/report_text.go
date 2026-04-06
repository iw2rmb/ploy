package runs

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TextRenderOptions controls optional features for the text report renderer.
type TextRenderOptions struct {
	EnableOSC8         bool
	AuthToken          string
	BaseURL            *url.URL
	SpinnerFrame       int
	LiveDurations      bool
	Now                time.Time
	JobIOPreviews      map[domaintypes.JobID]RunJobIOPreview
	ExpandStdout       bool
	ExpandStderr       bool
	FilterRunningRepos bool
	EmptyReposLine     string
}

// RunJobIOPreview contains bounded stdout/stderr previews for a single job.
type RunJobIOPreview struct {
	Stdout []string
	Stderr []string
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
	repos := report.Repos
	if opts.FilterRunningRepos {
		repos = filterRunningRepos(report.Repos)
	}
	emptyReposLine := strings.TrimSpace(opts.EmptyReposLine)
	if emptyReposLine == "" {
		emptyReposLine = "No repos found in this run."
	}

	headerLines := []string{
		"",
		fmt.Sprintf("   Mig:   %s", renderMigHeader(report.MigID.String(), report.MigName)),
		fmt.Sprintf("   Spec:  %s", renderOptionalLink(valueOrDash(report.SpecID.String()), buildSpecDownloadURL(report, opts.BaseURL), opts.EnableOSC8, opts.AuthToken)),
		fmt.Sprintf("   Repos: %d", len(repos)),
		fmt.Sprintf("   Run:   %s", valueOrDash(report.RunID.String())),
		"",
	}

	if len(repos) == 0 {
		block := strings.Join(append(headerLines, emptyReposLine), "\n")
		rendered := lipgloss.NewStyle().Render(block) + "\n"
		return RunReportTextLayout{
			Text:            rendered,
			LineCount:       strings.Count(rendered, "\n"),
			DynamicSections: nil,
		}, nil
	}

	frame := FollowFrame{
		Repos: make([]FollowRepoFrame, 0, len(repos)),
	}

	for _, repo := range repos {
		repoLinkLabel := strings.TrimSpace(repo.RepoURL)
		if repoLinkLabel != "" {
			repoLinkLabel = domaintypes.NormalizeRepoURLSchemless(repoLinkLabel)
		} else {
			repoLinkLabel = repo.RepoID.String()
		}

		repoFrame := FollowRepoFrame{
			HeaderLine: renderRepoHeaderLine(repo, repoLinkLabel, opts),
		}

		if len(repo.Jobs) == 0 {
			repoFrame.EmptyLine = "  Jobs: none"
			frame.Repos = append(frame.Repos, repoFrame)
			continue
		}

		repoFrame.Columns = nil
		repoFrame.Rows = make([]FollowStepRow, 0, len(repo.Jobs))
		for _, job := range repo.Jobs {
			patchURL := strings.TrimSpace(job.PatchURL)
			state := ColoredStatusGlyph(job.Status.String(), opts.SpinnerFrame)
			step := renderStepName(job.JobType.String())
			jobIDLabel := valueOrDash(job.JobID.String())
			if jobIDLabel != "-" {
				jobIDLabel = colorizeNeutralText(jobIDLabel)
			}
			jobIDCell := renderOptionalLink(jobIDLabel, job.JobLogURL, opts.EnableOSC8, opts.AuthToken)
			duration := FormatDurationForStatus(job.Status.String(), job.DurationMs, job.StartedAt, job.FinishedAt, now)
			if !opts.LiveDurations && !isTerminalJobStatus(job.Status.String()) {
				duration = FormatDurationCompact(job.DurationMs)
			}
			durationCell := fmt.Sprintf("%8s", duration)
			nodeIDCell := FormatNodeID(job.NodeID)
			if nodeIDCell != "-" {
				nodeIDCell = colorizeNeutralText(nodeIDCell)
			}

			repoFrame.Rows = append(repoFrame.Rows, FollowStepRow{
				Cells: []string{
					state,
					durationCell,
					step,
					jobIDCell,
					valueOrDash(strings.TrimSpace(job.JobImage)),
					renderArtifactsForStatus(job.Status.String(), patchURL, opts),
					nodeIDCell,
				},
				ExitOneLiner: renderExitOneLiner(job, repo.LastError),
				DetailLines:  renderJobIOPreviewLines(job, opts),
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
	step := normalizeStatus(jobType)
	if step == "" {
		step = "-"
	}
	return lipgloss.NewStyle().Bold(true).Render(step)
}

func renderRepoHeaderLine(repo RunEntry, repoLinkLabel string, opts TextRenderOptions) string {
	repoIDCell := valueOrDash(repo.RepoID.String())
	repoIDCell = colorizeNeutralText("[" + repoIDCell + "]")
	repoLabel := renderOptionalLink(repoLinkLabel, repo.RepoURL, opts.EnableOSC8, "")
	baseRef := valueOrDash(strings.TrimSpace(repo.BaseRef))
	shortSHA := formatShortSHA(strings.TrimSpace(repo.SourceCommitSHA))
	shaPart := colorizeNeutralText(fmt.Sprintf("(%s)", shortSHA))
	basePart := fmt.Sprintf("@ %s %s", boldBranchName(baseRef), shaPart)

	header := fmt.Sprintf("   %s %s %s", repoIDCell, repoLabel, basePart)
	if repo.MROnSuccess || repo.MROnFail {
		targetRef := valueOrDash(strings.TrimSpace(repo.TargetRef))
		header += fmt.Sprintf(" -> %s", boldBranchName(targetRef))
	}
	return header
}

func formatShortSHA(raw string) string {
	if len(raw) != 40 {
		return "-"
	}
	for _, r := range raw {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return "-"
		}
	}
	return raw[:8]
}

func boldBranchName(name string) string {
	return lipgloss.NewStyle().Bold(true).Render(name)
}

func renderExitOneLiner(job RunJobEntry, repoLastError *string) string {
	isHeal := normalizeStatus(job.JobType.String()) == "heal"
	if isHeal {
		exit := renderExitCode(job.ExitCode)
		if exit != "0" && isFailedOrCrashedStatus(job.Status.String()) {
			return renderWrappedExitOneLiner(exit, "Error", true)
		}
		return renderHealSummaryBlock(job)
	}

	if !isFailedOrCrashedStatus(job.Status.String()) {
		return ""
	}

	if isGateJobType(job.JobType.String()) {
		return renderWrappedExitOneLiner(renderExitCode(job.ExitCode), "Error", true)
	}

	msg := strings.Join(strings.Fields(strings.TrimSpace(job.BugSummary)), " ")
	if msg == "" {
		msg = FormatErrorOneLiner(repoLastError)
	}
	if msg == "" {
		msg = normalizeStatus(job.Status.String())
	}

	return renderWrappedExitOneLiner(renderExitCode(job.ExitCode), msg, true)
}

func renderJobIOPreviewLines(job RunJobEntry, opts TextRenderOptions) []string {
	status := normalizeStatus(job.Status.String())
	if status != "running" && status != "started" {
		return nil
	}

	preview := RunJobIOPreview{}
	if opts.JobIOPreviews != nil {
		if got, ok := opts.JobIOPreviews[job.JobID]; ok {
			preview = got
		}
	}

	expandStdout := opts.ExpandStdout
	expandStderr := opts.ExpandStderr
	return renderStreamPreviewLines(preview, expandStdout, expandStderr)
}

func filterRunningRepos(repos []RunEntry) []RunEntry {
	filtered := make([]RunEntry, 0, len(repos))
	for _, repo := range repos {
		if repoHasRunningJob(repo) {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}

func repoHasRunningJob(repo RunEntry) bool {
	for _, job := range repo.Jobs {
		if isRunningStatus(job.Status.String()) {
			return true
		}
	}
	return false
}

func renderStreamPreviewLines(preview RunJobIOPreview, expandStdout, expandStderr bool) []string {
	const (
		collapsedWidth = 80
		expandedWidth  = 80
		labelIndent    = "    "
		lineIndent     = "     "
	)

	stdoutLast := placeholderLastLine(preview.Stdout)
	stderrLast := placeholderLastLine(preview.Stderr)
	stdoutLabel := renderStreamPreviewLabel("STD[O]UT")
	stderrLabel := renderStreamPreviewLabel("STD[E]RR")

	lines := []string{""}
	if expandStdout {
		lines = append(lines, labelIndent+stdoutLabel)
	} else {
		lines = append(lines, labelIndent+stdoutLabel+" "+truncateRunesWithEllipsis(stdoutLast, collapsedWidth))
	}
	if expandStdout {
		lines = append(lines, "")
		for _, line := range expandedPreviewLines(preview.Stdout, expandedWidth) {
			lines = append(lines, lineIndent+line)
		}
		lines = append(lines, "")
	}

	stderrCollapsed := truncateRunesWithEllipsis(stderrLast, collapsedWidth)
	if expandStderr {
		lines = append(lines, labelIndent+stderrLabel)
	} else {
		lines = append(lines, labelIndent+stderrLabel+" "+colorizeErrorText(stderrCollapsed))
		lines = append(lines, "")
	}
	if expandStderr {
		lines = append(lines, "")
		for _, line := range expandedPreviewLines(preview.Stderr, expandedWidth) {
			lines = append(lines, lineIndent+colorizeErrorText(line))
		}
		lines = append(lines, "")
	}
	return lines
}

func renderStreamPreviewLabel(label string) string {
	return colorizeNeutralText(label)
}

func placeholderLastLine(lines []string) string {
	if len(lines) == 0 {
		return "(none yet)"
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return "(none yet)"
	}
	return last
}

func expandedPreviewLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return []string{"(none yet)"}
	}
	var out []string
	start := 0
	if len(lines) > 3 {
		start = len(lines) - 3
	}
	for _, line := range lines[start:] {
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(trimmed) == "" {
			trimmed = "(none yet)"
		}
		out = append(out, collapsedWrappedPreviewRows(wrapRunesFixed(trimmed, width))...)
	}
	if len(out) == 0 {
		return []string{"(none yet)"}
	}
	return out
}

func collapsedWrappedPreviewRows(rows []string) []string {
	const maxRowsBeforeCollapse = 7
	if len(rows) <= maxRowsBeforeCollapse {
		return rows
	}
	hidden := len(rows) - 6
	out := make([]string, 0, 7)
	out = append(out, rows[:3]...)
	out = append(out, fmt.Sprintf("... +%d rows", hidden))
	out = append(out, rows[len(rows)-3:]...)
	return out
}

func truncateRunesWithEllipsis(value string, width int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= width {
		return value
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	runes := []rune(value)
	return string(runes[:width-3]) + "..."
}

func wrapRunesFixed(value string, width int) []string {
	if width <= 0 {
		return []string{value}
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > 0 {
		chunk := width
		if len(runes) < width {
			chunk = len(runes)
		}
		lines = append(lines, string(runes[:chunk]))
		runes = runes[chunk:]
	}
	return lines
}

func renderWrappedExitOneLiner(exitCode, content string, colorizeContent bool) string {
	const wrapWidth = 100

	if exitCode == "0" {
		return ""
	}
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return ""
	}
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

func renderHealSummaryBlock(job RunJobEntry) string {
	lines := make([]string, 0, 2)
	errorKind := strings.TrimSpace(job.ErrorKind)
	bugSummary := strings.TrimSpace(job.BugSummary)
	actionSummary := strings.TrimSpace(job.ActionSummary)

	if errorKind != "" && bugSummary != "" {
		lines = append(lines, renderWrappedLabelLine("└  Issue ["+errorKind+"]: ", bugSummary))
	}
	if actionSummary != "" {
		lines = append(lines, renderWrappedLabelLine("└  Action: ", actionSummary))
	}
	return strings.Join(lines, "\n")
}

func renderWrappedLabelLine(prefix, content string) string {
	const wrapWidth = 80

	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return ""
	}
	wrapped := lipgloss.Wrap(content, wrapWidth, " ")
	rows := strings.Split(wrapped, "\n")
	if len(rows) == 0 {
		return ""
	}

	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		if i == 0 {
			lines = append(lines, prefix+row)
			continue
		}
		lines = append(lines, indent+row)
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
