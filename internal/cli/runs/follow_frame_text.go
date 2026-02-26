package runs

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
)

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

// RenderFollowFrameText renders a reusable follow-style text frame and line count.
func RenderFollowFrameText(frame FollowFrame, opts FollowFrameOptions) (string, int) {
	_ = opts

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 8, 2, ' ', 0)

	for _, line := range frame.TopLines {
		_, _ = fmt.Fprintln(tw, line)
	}

	for i, repo := range frame.Repos {
		if i > 0 || len(frame.TopLines) > 0 {
			_, _ = fmt.Fprintln(tw)
		}
		if strings.TrimSpace(repo.HeaderLine) != "" {
			_, _ = fmt.Fprintln(tw, repo.HeaderLine)
		}

		if len(repo.Rows) == 0 {
			if strings.TrimSpace(repo.EmptyLine) != "" {
				_, _ = fmt.Fprintln(tw, repo.EmptyLine)
			}
			continue
		}

		if len(repo.Columns) > 0 {
			_, _ = fmt.Fprintln(tw, strings.Join(repo.Columns, "\t"))
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

		for _, row := range repo.Rows {
			cells := normalizeFollowCells(row.Cells, columnCount)
			_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))

			if strings.TrimSpace(row.ExitOneLiner) == "" {
				continue
			}
			exitCells := make([]string, columnCount)
			if columnCount > 1 {
				exitCells[1] = row.ExitOneLiner
			} else {
				exitCells[0] = row.ExitOneLiner
			}
			_, _ = fmt.Fprintln(tw, strings.Join(exitCells, "\t"))
		}
	}

	_ = tw.Flush()
	rendered := buf.String()
	return rendered, strings.Count(rendered, "\n")
}

func normalizeFollowCells(cells []string, width int) []string {
	if width <= 0 {
		return []string{}
	}
	out := make([]string, width)
	copy(out, cells)
	return out
}
