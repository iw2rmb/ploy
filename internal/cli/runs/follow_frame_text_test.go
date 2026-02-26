package runs

import (
	"strings"
	"testing"
)

func TestRenderFollowFrameText_RendersRowsAndExitOneLiner(t *testing.T) {
	t.Parallel()

	frame := FollowFrame{
		TopLines: []string{"  Repos: 1"},
		Repos: []FollowRepoFrame{
			{
				HeaderLine: "  Repo 1/1: example.com/acme/repo",
				Columns:    []string{"", "Step", "Job ID", "Node", "Image", "Duration"},
				Rows: []FollowStepRow{
					{
						Cells:        []string{"⣾", "mig", "job-1", "node-1", "ubuntu:latest", "1.5s"},
						ExitOneLiner: "\x1b[31m└ build failed\x1b[0m",
					},
				},
			},
		},
	}

	out, lines := RenderFollowFrameText(frame, FollowFrameOptions{})

	if !strings.Contains(out, "Repos: 1") {
		t.Fatalf("expected repo count in output, got %q", out)
	}
	if !strings.Contains(out, "Repo 1/1: example.com/acme/repo") {
		t.Fatalf("expected repo block header, got %q", out)
	}
	if !strings.Contains(out, "Step") || !strings.Contains(out, "Duration") {
		t.Fatalf("expected table headers, got %q", out)
	}
	if !strings.Contains(out, "⣾") || !strings.Contains(out, "mig") {
		t.Fatalf("expected row content, got %q", out)
	}
	if !strings.Contains(out, "└ build failed") {
		t.Fatalf("expected exit one-liner row, got %q", out)
	}
	if !strings.Contains(out, "\n\x1b[31m└ build failed\x1b[0m\n") {
		t.Fatalf("expected exit row to start at column 0 on a dedicated line, got %q", out)
	}
	if lines != strings.Count(out, "\n") {
		t.Fatalf("line count mismatch: got %d want %d", lines, strings.Count(out, "\n"))
	}
}

func TestRenderFollowFrameText_RightAlignsDurationColumn(t *testing.T) {
	t.Parallel()

	frame := FollowFrame{
		Repos: []FollowRepoFrame{
			{
				Columns: []string{"", "Step", "Duration"},
				Rows: []FollowStepRow{
					{Cells: []string{"✓", "short", "1.0s"}},
					{Cells: []string{"✓", "long", "123.4s"}},
				},
			},
		},
	}

	out, _ := RenderFollowFrameText(frame, FollowFrameOptions{})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least header + 2 rows, got %q", out)
	}

	rowShort := lines[len(lines)-2]
	rowLong := lines[len(lines)-1]
	idxShort := strings.Index(rowShort, "1.0s")
	idxLong := strings.Index(rowLong, "123.4s")
	if idxShort == -1 || idxLong == -1 {
		t.Fatalf("failed to locate duration cells in output %q", out)
	}
	endShort := idxShort + len("1.0s")
	endLong := idxLong + len("123.4s")
	if endShort != endLong {
		t.Fatalf("expected right-aligned duration values; got end short=%d long=%d in %q", endShort, endLong, out)
	}
}

func TestRenderFollowFrameText_RendersEmptyLineForRepoWithoutRows(t *testing.T) {
	t.Parallel()

	frame := FollowFrame{
		Repos: []FollowRepoFrame{
			{
				HeaderLine: "Repo:  [1/1] example.com/acme/repo main -> feature",
				EmptyLine:  "  Jobs: none",
			},
		},
	}

	out, _ := RenderFollowFrameText(frame, FollowFrameOptions{})
	if !strings.Contains(out, "Repo:  [1/1] example.com/acme/repo main -> feature") {
		t.Fatalf("expected repo header, got %q", out)
	}
	if !strings.Contains(out, "Jobs: none") {
		t.Fatalf("expected empty jobs line, got %q", out)
	}
}
