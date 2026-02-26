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
	tw := tabwriter.NewWriter(&buf, 0, 8, 2, ' ', 0)
	lineNo := 0
	sectionRanges := make([]followDynamicSectionRange, len(frame.Repos))

	for _, line := range frame.TopLines {
		_, _ = fmt.Fprintln(tw, line)
		lineNo++
	}

	for i, repo := range frame.Repos {
		if i > 0 || len(frame.TopLines) > 0 {
			_, _ = fmt.Fprintln(tw)
			lineNo++
		}
		if strings.TrimSpace(repo.HeaderLine) != "" {
			_, _ = fmt.Fprintln(tw, repo.HeaderLine)
			lineNo++
		}

		if len(repo.Rows) == 0 {
			sectionRanges[i] = followDynamicSectionRange{start: lineNo, count: 0}
			if strings.TrimSpace(repo.EmptyLine) != "" {
				_, _ = fmt.Fprintln(tw, repo.EmptyLine)
				sectionRanges[i] = followDynamicSectionRange{start: lineNo, count: 1}
				lineNo++
			}
			continue
		}

		if len(repo.Columns) > 0 {
			_, _ = fmt.Fprintln(tw, strings.Join(repo.Columns, "\t"))
			lineNo++
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
		sectionStart := lineNo
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
			lineNo++

			if strings.TrimSpace(row.ExitOneLiner) == "" {
				continue
			}
			_, _ = fmt.Fprintln(tw, row.ExitOneLiner)
			lineNo++
		}
		sectionRanges[i] = followDynamicSectionRange{start: sectionStart, count: lineNo - sectionStart}
	}

	_ = tw.Flush()
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
