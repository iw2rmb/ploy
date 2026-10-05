package runs

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestReportRendersFullTextAndSanitizesControls(t *testing.T) {
	report := "first\nsecond\nthird\nfourth\n\x1b[2Jlast\r\u202e"
	full := strings.Join(renderJobReportLines(report, true), "\n")
	for _, want := range []string{"[R]EPORT", "first", "second", "third", "fourth", "last"} {
		if !strings.Contains(full, want) {
			t.Fatalf("missing %q in %q", want, full)
		}
	}
	if strings.ContainsAny(strings.Join(renderJobReportLines(report, true)[1:], "\n"), "\x1b\r\u202e") {
		t.Fatalf("unsafe controls in %q", full)
	}
	if got := renderJobReportLines(report, false); len(got) != 1 || !strings.Contains(got[0], "first") {
		t.Fatalf("collapsed: %v", got)
	}
	if got := renderJobReportLines("", true); len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
}

func TestReportRemainsVisibleOnFinishedJobsAndRToggles(t *testing.T) {
	m := newFollowModel(TextRenderOptions{}, true)
	report := RunStatusReport{Repos: []RunEntry{{Jobs: []RunJobEntry{{JobID: types.NewJobID(), Status: types.JobStatusSuccess, Report: "first\nlast"}}}}}
	m.report = &report
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(followModel)
	if !m.expandReport {
		t.Fatal("r did not expand report")
	}
	layout, err := RenderRunStatusReportTextLayout(report, TextRenderOptions{ExpandReport: m.expandReport, Now: time.Now()})
	if err != nil || !strings.Contains(layout.Text, "first") || !strings.Contains(layout.Text, "last") {
		t.Fatalf("finished report: %s, %v", layout.Text, err)
	}
}
