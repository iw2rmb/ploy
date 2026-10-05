package runs

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// FollowFrameRender is the rendered follow frame.
type FollowFrameRender struct {
	Text string
}

// FollowFrame is a reusable follow-style text frame.
type FollowFrame struct {
	Repos []FollowRepoFrame
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
	DetailLines  []string
}

// RenderFollowFrameTextLayout renders a follow frame.
func RenderFollowFrameTextLayout(frame FollowFrame) FollowFrameRender {
	var buf bytes.Buffer
	appendLine := func(line string) {
		_, _ = buf.WriteString(line)
		_ = buf.WriteByte('\n')
	}

	for i, repo := range frame.Repos {
		if i > 0 {
			appendLine("")
		}
		if strings.TrimSpace(repo.HeaderLine) != "" {
			appendLine(repo.HeaderLine)
		}

		if len(repo.Rows) == 0 {
			if strings.TrimSpace(repo.EmptyLine) != "" {
				appendLine(repo.EmptyLine)
			}
			continue
		}

		tableLines := renderFollowRepoTableLines(repo)
		tableLineOffset := 0
		if len(repo.Columns) > 0 && len(tableLines) > 0 {
			appendLine(tableLines[0])
			tableLineOffset = 1
		}
		for rowIndex, row := range repo.Rows {
			if idx := tableLineOffset + rowIndex; idx < len(tableLines) {
				appendLine(tableLines[idx])
			} else {
				appendLine("")
			}

			if strings.TrimSpace(row.ExitOneLiner) != "" {
				for _, exitLine := range strings.Split(row.ExitOneLiner, "\n") {
					appendLine(exitLine)
				}
			}
			for _, detailLine := range row.DetailLines {
				appendLine(detailLine)
			}
		}
	}

	return FollowFrameRender{Text: buf.String()}
}

func renderFollowRepoTableLines(repo FollowRepoFrame) []string {
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
	tableRows := make([][]string, 0, len(repo.Rows)+1)
	if len(repo.Columns) > 0 {
		tableRows = append(tableRows, renderFollowColumns(repo.Columns, columnCount))
	}
	for _, row := range repo.Rows {
		tableRows = append(tableRows, normalizeFollowCells(row.Cells, columnCount))
	}

	widths := make([]int, columnCount)
	for _, row := range tableRows {
		for col, cell := range row {
			cellWidth := visibleRuneWidth(cell)
			if cellWidth > widths[col] {
				widths[col] = cellWidth
			}
		}
	}

	out := make([]string, 0, len(tableRows))
	for _, row := range tableRows {
		renderedCells := make([]string, columnCount)
		for col, cell := range row {
			trimmed := strings.TrimSpace(cell)
			current := cell
			if durationColumn == col && trimmed != "" {
				current = trimmed
			}
			padding := widths[col] - visibleRuneWidth(current)
			if padding < 0 {
				padding = 0
			}
			if durationColumn == col && trimmed != "" {
				renderedCells[col] = strings.Repeat(" ", padding) + current
				continue
			}
			renderedCells[col] = current + strings.Repeat(" ", padding)
		}
		out = append(out, strings.TrimRight(strings.Join(renderedCells, "  "), " "))
	}
	return out
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

func terminalEscapeSeqEnd(value string, escPos int) int {
	if escPos+1 >= len(value) {
		return 0
	}
	switch value[escPos+1] {
	case '[':
		// CSI: ESC [ ... final-byte(0x40-0x7E)
		for i := escPos + 2; i < len(value); i++ {
			b := value[i]
			if b >= 0x40 && b <= 0x7e {
				return i + 1
			}
		}
		return 0
	case ']':
		// OSC: ESC ] ... BEL or ST (ESC \)
		for i := escPos + 2; i < len(value); i++ {
			switch value[i] {
			case 0x07:
				return i + 1
			case 0x1b:
				if i+1 < len(value) && value[i+1] == '\\' {
					return i + 2
				}
			}
		}
		return 0
	default:
		return 0
	}
}

func renderFollowColumns(columns []string, width int) []string {
	headerCells := normalizeFollowCells(columns, width)
	if len(columns) > 1 && strings.TrimSpace(columns[0]) == "" {
		// Keep an explicit first status column so table headers align with data rows.
		headerCells[0] = neutralGlyphStyle.Render(" ")
	}
	return headerCells
}

func visibleRuneWidth(value string) int {
	if value == "" {
		return 0
	}
	return utf8.RuneCountInString(stripTerminalControlSequences(value))
}

func stripTerminalControlSequences(value string) string {
	if !strings.Contains(value, "\x1b") {
		return value
	}
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != 0x1b {
			out.WriteByte(value[i])
			i++
			continue
		}
		end := terminalEscapeSeqEnd(value, i)
		if end <= i {
			i++
			continue
		}
		i = end
	}
	return out.String()
}
