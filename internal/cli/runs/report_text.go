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

// RunStatusReportDynamicSection represents one mutable block of lines in rendered run text.
type RunStatusReportDynamicSection struct {
	StartLine int
	LineCount int
	Text      string
}

// RunStatusReportTextLayout is a rendered run status report with per-repo mutable sections.
type RunStatusReportTextLayout struct {
	Text            string
	LineCount       int
	DynamicSections []RunStatusReportDynamicSection
}

// RenderRunStatusReportText renders a one-shot, follow-style run snapshot.
func RenderRunStatusReportText(w io.Writer, report RunStatusReport, opts TextRenderOptions) error {
	if w == nil {
		return fmt.Errorf("run status report text: output writer required")
	}

	layout, err := RenderRunStatusReportTextLayout(report, opts)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(w, layout.Text)
	return nil
}

// RenderRunStatusSnapshotText renders a static status snapshot using the same
// output shape as `ploy run status`.
func RenderRunStatusSnapshotText(w io.Writer, report RunStatusReport, opts TextRenderOptions) error {
	opts.SpinnerFrame = 0
	opts.LiveDurations = false
	opts.Now = time.Time{}
	opts.JobIOPreviews = nil
	opts.ExpandStdout = false
	opts.ExpandStderr = false
	opts.FilterRunningRepos = false
	opts.EmptyReposLine = ""
	return RenderRunStatusReportText(w, report, opts)
}

// RenderRunStatusReportTextLayout renders run status report text plus mutable per-repo row sections.
func RenderRunStatusReportTextLayout(report RunStatusReport, opts TextRenderOptions) (RunStatusReportTextLayout, error) {
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
	headerRepo := firstRunRepo(report.Repos)

	headerLines := []string{
		"",
		fmt.Sprintf("   Run:   %s", valueOrDash(report.RunID.String())),
		fmt.Sprintf("   Repo:  %s", renderRepoHeaderValue(headerRepo, opts)),
		fmt.Sprintf("   Spec:  %s", renderOptionalLink(valueOrDash(report.SpecID.String()), buildSpecDownloadURL(report, opts.BaseURL), opts.EnableOSC8, opts.AuthToken)),
		fmt.Sprintf("   Node:  %s", colorizeNeutralText(repoNodeID(headerRepo))),
		"",
	}

	if len(repos) == 0 {
		block := strings.Join(append(headerLines, emptyReposLine), "\n")
		rendered := lipgloss.NewStyle().Render(block) + "\n"
		return RunStatusReportTextLayout{
			Text:            rendered,
			LineCount:       strings.Count(rendered, "\n"),
			DynamicSections: nil,
		}, nil
	}

	frame := FollowFrame{
		Repos: make([]FollowRepoFrame, 0, len(repos)),
	}

	for _, repo := range repos {
		repoFrame := FollowRepoFrame{
			HeaderLine: "",
		}

		if len(repo.Jobs) == 0 {
			repoFrame.EmptyLine = "  Jobs: none"
			frame.Repos = append(frame.Repos, repoFrame)
			continue
		}

		repoFrame.Columns = nil
		repoFrame.Rows = make([]FollowStepRow, 0, len(repo.Jobs))
		repoErrorOwnerIdx := lastFailedOrCrashedJobIndex(repo.Jobs)
		for _, job := range repo.Jobs {
			jobIdx := len(repoFrame.Rows)
			patchURL := strings.TrimSpace(job.PatchURL)
			state := ColoredStatusGlyph(job.Status.String(), opts.SpinnerFrame)
			step := renderStepName(job.DisplayName, job.JobType.String())
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

			repoFrame.Rows = append(repoFrame.Rows, FollowStepRow{
				Cells: []string{
					state,
					durationCell,
					step,
					renderArtifactsForStatus(job.Status.String(), patchURL, opts),
					jobIDCell,
					valueOrDash(strings.TrimSpace(job.JobImage)),
				},
				ExitOneLiner: renderExitOneLiner(job, repo.LastError, jobIdx == repoErrorOwnerIdx),
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
	if len(report.SBOMDiff) > 0 {
		out.WriteByte('\n')
		out.WriteString(formatSBOMDiffBlock(report.SBOMDiff))
		out.WriteByte('\n')
		out.WriteByte('\n')
	}
	rendered := lipgloss.NewStyle().Render(out.String())

	dynamicSections := make([]RunStatusReportDynamicSection, len(frameLayout.Sections))
	headerLineCount := len(headerLines)
	for i, section := range frameLayout.Sections {
		dynamicSections[i] = RunStatusReportDynamicSection{
			StartLine: headerLineCount + section.StartLine,
			LineCount: section.LineCount,
			Text:      section.Text,
		}
	}

	return RunStatusReportTextLayout{
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

func renderOptionalOSC8Link(label, rawURL string, enableOSC8 bool) string {
	if strings.TrimSpace(rawURL) == "" || !enableOSC8 {
		return label
	}
	return renderLink(label, rawURL, true, "")
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

func renderStepName(displayName string, jobType string) string {
	step := strings.TrimSpace(displayName)
	if step == "" {
		step = normalizeStatus(jobType)
	}
	if step == "" {
		step = "-"
	}
	return lipgloss.NewStyle().Bold(true).Render(step)
}

func firstRunRepo(repos []RunEntry) RunEntry {
	if len(repos) == 0 {
		return RunEntry{}
	}
	return repos[0]
}

func renderRepoHeaderValue(repo RunEntry, opts TextRenderOptions) string {
	repoLabel := renderOptionalOSC8Link(renderRepoPathLabel(repo), repo.RepoURL, opts.EnableOSC8)
	shortSHA := formatShortSHA(strings.TrimSpace(repo.SourceCommitSHA))
	if repoLabel == "-" && shortSHA == "-" {
		return "-"
	}

	return fmt.Sprintf("%s:%s", repoLabel, colorizeNeutralText(shortSHA))
}

func renderRepoPathLabel(repo RunEntry) string {
	label := strings.TrimSpace(repo.RepoURL)
	if label != "" {
		label = domaintypes.NormalizeRepoURLSchemless(label)
		label = strings.TrimSuffix(label, ".git")
		if slash := strings.Index(label, "/"); slash >= 0 && slash+1 < len(label) {
			label = label[slash+1:]
		}
	}
	if label == "" {
		label = repo.RepoID.String()
	}
	return valueOrDash(label)
}

func repoNodeID(repo RunEntry) string {
	for _, job := range repo.Jobs {
		nodeID := FormatNodeID(job.NodeID)
		if nodeID != "-" {
			return nodeID
		}
	}
	return "-"
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

func renderExitOneLiner(job RunJobEntry, repoLastError *string, useRepoLastError bool) string {
	if !isFailedOrCrashedStatus(job.Status.String()) {
		return ""
	}

	msg := strings.Join(strings.Fields(strings.TrimSpace(job.BugSummary)), " ")
	if isGateJobType(job.JobType.String()) {
		if !useRepoLastError {
			return ""
		}
		msg = FormatErrorOneLiner(repoLastError)
		if msg == "" {
			return ""
		}
		return renderWrappedExitOneLiner(renderExitCode(job.ExitCode), msg, true)
	}
	if msg == "" && useRepoLastError {
		msg = FormatErrorOneLiner(repoLastError)
	}
	if msg == "" {
		msg = normalizeStatus(job.Status.String())
	}

	return renderWrappedExitOneLiner(renderExitCode(job.ExitCode), msg, true)
}

func lastFailedOrCrashedJobIndex(jobs []RunJobEntry) int {
	ownerIdx := -1
	for i := range jobs {
		if isFailedOrCrashedStatus(jobs[i].Status.String()) {
			ownerIdx = i
		}
	}
	return ownerIdx
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
	case "pre_gate", "post_gate":
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

func buildSpecDownloadURL(report RunStatusReport, baseURL *url.URL) string {
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
