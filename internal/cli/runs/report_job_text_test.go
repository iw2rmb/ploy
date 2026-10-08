package runs

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestReportRendersFullTextAndSanitizesControls(t *testing.T) {
	report := "first\nsecond\nthird\nfourth\n\x1b[2Jlast\r\u202e"
	full := strings.Join(renderJobReportLines(report, true, true), "\n")
	for _, want := range []string{"REPORT", "first", "second", "third", "fourth", "last"} {
		if !strings.Contains(full, want) {
			t.Fatalf("missing %q in %q", want, full)
		}
	}
	if strings.ContainsAny(strings.Join(renderJobReportLines(report, true, true)[3:], "\n"), "\x1b\r\u202e") {
		t.Fatalf("unsafe controls in %q", full)
	}
	if got := renderJobReportLines(report, false, true); len(got) != 3 || !strings.Contains(got[1], "first") {
		t.Fatalf("collapsed: %v", got)
	}
	if got := renderJobReportLines("", true, true); len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
}

// The initial flag and subsequent key toggles must survive the final snapshot.
func TestReportRemainsVisibleOnFinishedJobsAndRToggles(t *testing.T) {
	for _, collapsed := range []bool{false, true} {
		m := newFollowModel(TextRenderOptions{ReportsCollapsed: collapsed}, true)
		report := RunStatusReport{Repos: []RunEntry{{Jobs: []RunJobEntry{{JobID: types.NewJobID(), Status: types.JobStatusSuccess, Report: "first\nlast"}}}}}
		m.report = &report
		for i := 0; i < 2; i++ {
			var final bytes.Buffer
			if err := writeFinalStatusSnapshot(&final, report, m.renderOpts, false); err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{stripTerminalControlSequences(m.View().Content), stripTerminalControlSequences(final.String())} {
				want := "\n\n    REPORT\n\n      first\n      last\n\n"
				if m.renderOpts.ReportsCollapsed {
					want = "\n\n    REPORT first\n\n"
				}
				if !strings.Contains(text, want) {
					t.Fatalf("collapsed=%v: missing %q in %q", m.renderOpts.ReportsCollapsed, want, text)
				}
				if m.renderOpts.ReportsCollapsed && strings.Contains(text, "last") {
					t.Fatalf("collapsed report includes full content: %q", text)
				}
			}
			updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			m = updated.(followModel)
			if m.renderOpts.ReportsCollapsed == collapsed && i == 0 {
				t.Fatal("r did not toggle report")
			}
		}
	}
}
