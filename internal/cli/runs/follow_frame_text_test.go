package runs

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/iw2rmb/ploy/internal/testutil/assertx"
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

	out, lines := RenderFollowFrameText(frame)

	assertx.Contains(t, out, "Repos: 1")
	assertx.Contains(t, out, "Repo 1/1: example.com/acme/repo")
	assertx.Contains(t, out, "Step")
	assertx.Contains(t, out, "Duration")
	assertx.Contains(t, out, "⣾")
	assertx.Contains(t, out, "mig")
	assertx.Contains(t, out, "└ build failed")
	assertx.Contains(t, out, "\n\x1b[31m└ build failed\x1b[0m\n")
	if lines != strings.Count(out, "\n") {
		t.Fatalf("line count mismatch: got %d want %d", lines, strings.Count(out, "\n"))
	}
}

func TestRenderFollowFrameText_RendersMultiLineExitOneLiner(t *testing.T) {
	t.Parallel()

	frame := FollowFrame{
		Repos: []FollowRepoFrame{
			{
				Columns: []string{"", "Step"},
				Rows: []FollowStepRow{
					{
						Cells:        []string{"✗", "pre_gate"},
						ExitOneLiner: "└  Exit 1: first line\n             second line",
					},
				},
			},
		},
	}

	out, lines := RenderFollowFrameText(frame)
	assertx.Contains(t, out, "└  Exit 1: first line")
	assertx.Contains(t, out, "             second line")
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

	out, _ := RenderFollowFrameText(frame)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
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

func TestRenderFollowFrameText_DoesNotInflatePaddingForANSIStateGlyphs(t *testing.T) {
	t.Parallel()

	frame := FollowFrame{
		Repos: []FollowRepoFrame{
			{
				Columns: []string{"", "Step"},
				Rows: []FollowStepRow{
					{Cells: []string{ColoredStatusGlyph("running", 0), "pre_gate"}},
					{Cells: []string{ColoredStatusGlyph("queued", 0), "mig"}},
				},
			},
		},
	}

	out, _ := RenderFollowFrameText(frame)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected header + 2 rows, got %q", out)
	}

	rowA := lines[len(lines)-2]
	rowB := lines[len(lines)-1]
	header := lines[len(lines)-3]
	replacer := strings.NewReplacer("\x1b[92m", "", "\x1b[39m", "", "\x1b[0m", "")
	rowA = replacer.Replace(rowA)
	rowB = replacer.Replace(rowB)
	header = replacer.Replace(header)

	idxHeader := strings.Index(header, "Step")
	idxA := strings.Index(rowA, "pre_gate")
	idxB := strings.Index(rowB, "mig")
	if idxHeader == -1 || idxA == -1 || idxB == -1 {
		t.Fatalf("failed to locate step values in output %q", out)
	}
	colHeader := utf8.RuneCountInString(header[:idxHeader])
	colA := utf8.RuneCountInString(rowA[:idxA])
	colB := utf8.RuneCountInString(rowB[:idxB])
	if colA != colB || colHeader != colA {
		t.Fatalf("expected aligned header/rows despite ANSI glyphs; got col header=%d pre_gate=%d mig=%d in %q", colHeader, colA, colB, out)
	}
}

func TestRenderFollowFrameText_ExitRowsDoNotShiftColumns(t *testing.T) {
	t.Parallel()

	frame := FollowFrame{
		Repos: []FollowRepoFrame{
			{
				Columns: []string{"", "Step", "Job"},
				Rows: []FollowStepRow{
					{
						Cells:        []string{ColoredStatusGlyph("failed", 0), "pre_gate", "job-1"},
						ExitOneLiner: "└ Exit 1: long failure details that should not affect table alignment",
					},
					{
						Cells: []string{ColoredStatusGlyph("running", 0), "heal", "job-2"},
					},
					{
						Cells: []string{ColoredStatusGlyph("queued", 0), "mig", "job-3"},
					},
				},
			},
		},
	}

	out, _ := RenderFollowFrameText(frame)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected header + rows + exit line, got %q", out)
	}

	replacer := strings.NewReplacer("\x1b[92m", "", "\x1b[39m", "", "\x1b[91m", "", "\x1b[0m", "")
	header := replacer.Replace(lines[0])
	rowPreGate := replacer.Replace(lines[1])
	rowHeal := replacer.Replace(lines[3])
	rowMig := replacer.Replace(lines[4])

	idxHeader := strings.Index(header, "Step")
	idxPreGate := strings.Index(rowPreGate, "pre_gate")
	idxHeal := strings.Index(rowHeal, "heal")
	idxMig := strings.Index(rowMig, "mig")
	if idxHeader == -1 || idxPreGate == -1 || idxHeal == -1 || idxMig == -1 {
		t.Fatalf("failed to locate step values in output %q", out)
	}

	colHeader := utf8.RuneCountInString(header[:idxHeader])
	colPreGate := utf8.RuneCountInString(rowPreGate[:idxPreGate])
	colHeal := utf8.RuneCountInString(rowHeal[:idxHeal])
	colMig := utf8.RuneCountInString(rowMig[:idxMig])
	if colHeader != colPreGate || colPreGate != colHeal || colHeal != colMig {
		t.Fatalf("expected stable step column with exit rows; got header=%d pre_gate=%d heal=%d mig=%d in %q", colHeader, colPreGate, colHeal, colMig, out)
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

	out, _ := RenderFollowFrameText(frame)
	assertx.Contains(t, out, "Repo:  [1/1] example.com/acme/repo main -> feature")
	assertx.Contains(t, out, "Jobs: none")
}
