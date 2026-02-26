package runs

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// FollowDynamicSection describes one mutable line block in a rendered follow frame.
type FollowDynamicSection struct {
	StartLine int
	LineCount int
	Text      string
}

// FollowFrameRender is the rendered follow frame with dynamic section metadata.
type FollowFrameRender struct {
	Text      string
	LineCount int
	Sections  []FollowDynamicSection
}

// FollowFrame is a reusable follow-style text frame.
type FollowFrame struct {
	TopLines []string
	Repos    []FollowRepoFrame
}

// FollowRepoFrame is one repo block inside a follow-style frame.
type FollowRepoFrame struct {
	HeaderLine string
	Columns    []string
	Rows       []FollowStepRow
	EmptyLine  string
}

// FollowStepRow is one row in a repo table, with optional second-line summary.
type FollowStepRow struct {
	Cells        []string
	ExitOneLiner string
}

// FollowFrameOptions controls optional follow-frame rendering features.
type FollowFrameOptions struct{}

type followDynamicSectionRange struct {
	start int
	count int
}

// RenderFollowFrameText renders a reusable follow-style text frame and line count.
func RenderFollowFrameText(frame FollowFrame, opts FollowFrameOptions) (string, int) {
	layout := RenderFollowFrameTextLayout(frame, opts)
	return layout.Text, layout.LineCount
}

// RenderFollowFrameTextLayout renders a follow frame plus per-repo dynamic section metadata.
func RenderFollowFrameTextLayout(frame FollowFrame, opts FollowFrameOptions) FollowFrameRender {
	_ = opts

	var buf bytes.Buffer
	lineNo := 0
	sectionRanges := make([]followDynamicSectionRange, len(frame.Repos))
	appendLine := func(line string) {
		_, _ = buf.WriteString(line)
		_ = buf.WriteByte('\n')
		lineNo++
	}

	for _, line := range frame.TopLines {
		appendLine(line)
	}

	for i, repo := range frame.Repos {
		if i > 0 || len(frame.TopLines) > 0 {
			appendLine("")
		}
		if strings.TrimSpace(repo.HeaderLine) != "" {
			appendLine(repo.HeaderLine)
		}

		if len(repo.Rows) == 0 {
			sectionRanges[i] = followDynamicSectionRange{start: lineNo, count: 0}
			if strings.TrimSpace(repo.EmptyLine) != "" {
				appendLine(repo.EmptyLine)
				sectionRanges[i] = followDynamicSectionRange{start: lineNo, count: 1}
			}
			continue
		}

		tableLines := renderFollowRepoTableLines(repo)
		tableLineOffset := 0
		if len(repo.Columns) > 0 && len(tableLines) > 0 {
			appendLine(tableLines[0])
			tableLineOffset = 1
		}
		sectionStart := lineNo
		for rowIndex, row := range repo.Rows {
			if idx := tableLineOffset + rowIndex; idx < len(tableLines) {
				appendLine(tableLines[idx])
			} else {
				appendLine("")
			}

			if strings.TrimSpace(row.ExitOneLiner) == "" {
				continue
			}
			appendLine(row.ExitOneLiner)
		}
		sectionRanges[i] = followDynamicSectionRange{start: sectionStart, count: lineNo - sectionStart}
	}

	rendered := buf.String()
	renderedLines := strings.Count(rendered, "\n")
	lines := splitRenderedLines(rendered)
	sections := make([]FollowDynamicSection, len(sectionRanges))
	for i, section := range sectionRanges {
		sections[i] = FollowDynamicSection{
			StartLine: section.start,
			LineCount: section.count,
			Text:      joinRenderedLineRange(lines, section.start, section.count),
		}
	}

	return FollowFrameRender{
		Text:      rendered,
		LineCount: renderedLines,
		Sections:  sections,
	}
}

func renderFollowRepoTableLines(repo FollowRepoFrame) []string {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 8, 2, ' ', tabwriter.StripEscape)

	if len(repo.Columns) > 0 {
		_, _ = fmt.Fprintln(tw, renderFollowColumns(repo.Columns))
	}

	columnCount := len(repo.Columns)
	for _, row := range repo.Rows {
		if len(row.Cells) > columnCount {
			columnCount = len(row.Cells)
		}
	}
	if columnCount == 0 {
		columnCount = 1
	}

	durationColumn := followDurationColumnIndex(repo.Columns)
	durationWidth := 0
	if durationColumn >= 0 {
		durationWidth = utf8.RuneCountInString(strings.TrimSpace(repo.Columns[durationColumn]))
		for _, row := range repo.Rows {
			cells := normalizeFollowCells(row.Cells, columnCount)
			if durationColumn >= len(cells) {
				continue
			}
			width := utf8.RuneCountInString(strings.TrimSpace(cells[durationColumn]))
			if width > durationWidth {
				durationWidth = width
			}
		}
	}

	for _, row := range repo.Rows {
		cells := normalizeFollowCells(row.Cells, columnCount)
		if len(cells) > 0 {
			cells[0] = followShieldANSIForTabwriter(cells[0])
		}
		if durationColumn >= 0 && durationColumn < len(cells) {
			duration := strings.TrimSpace(cells[durationColumn])
			if duration != "" {
				padding := durationWidth - utf8.RuneCountInString(duration)
				if padding > 0 {
					cells[durationColumn] = strings.Repeat(" ", padding) + duration
				} else {
					cells[durationColumn] = duration
				}
			}
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}

	_ = tw.Flush()
	return splitRenderedLines(buf.String())
}

func normalizeFollowCells(cells []string, width int) []string {
	if width <= 0 {
		return []string{}
	}
	out := make([]string, width)
	copy(out, cells)
	return out
}

func followDurationColumnIndex(columns []string) int {
	for i, col := range columns {
		if strings.EqualFold(strings.TrimSpace(col), "Duration") {
			return i
		}
	}
	return -1
}

func splitRenderedLines(rendered string) []string {
	if rendered == "" {
		return nil
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func joinRenderedLineRange(lines []string, start int, count int) string {
	if count <= 0 || start < 0 || start >= len(lines) {
		return ""
	}
	end := start + count
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n") + "\n"
}

func followShieldANSIForTabwriter(value string) string {
	if !strings.Contains(value, "\x1b[") {
		return value
	}
	escape := string([]byte{tabwriter.Escape})
	replacer := strings.NewReplacer(
		ansiSuccessLightGreen, escape+ansiSuccessLightGreen+escape,
		ansiDefaultForeground, escape+ansiDefaultForeground+escape,
		ansiErrorLightRed, escape+ansiErrorLightRed+escape,
		ansiColorReset, escape+ansiColorReset+escape,
	)
	return replacer.Replace(value)
}

func renderFollowColumns(columns []string) string {
	if len(columns) > 1 && strings.TrimSpace(columns[0]) == "" {
		headerCells := make([]string, len(columns))
		copy(headerCells, columns)
		// Keep an explicit first status column so table headers align with data rows.
		headerCells[0] = followShieldANSIForTabwriter(ansiDefaultForeground + " " + ansiColorReset)
		return strings.Join(headerCells, "\t")
	}
	return strings.Join(columns, "\t")
}
