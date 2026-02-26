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
	if lines != strings.Count(out, "\n") {
		t.Fatalf("line count mismatch: got %d want %d", lines, strings.Count(out, "\n"))
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
