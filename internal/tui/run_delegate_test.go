package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Terminal status icons use semantic colors only on unselected rows, including filtered results.
func TestRunStatusIconColorsFollowSelection(t *testing.T) {
	for _, tc := range []struct {
		status domaintypes.RunStatus
		glyph  string
		color  string
	}{
		{domaintypes.RunStatusSuccess, "✓", "\x1b[92m"},
		{domaintypes.RunStatusFail, "✗", "\x1b[91m"},
	} {
		for _, filter := range []string{"", "run", tc.glyph} {
			for _, profile := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI256, colorprofile.ANSI, colorprofile.ASCII, colorprofile.NoTTY} {
				t.Run(string(tc.status)+"/filter="+filter+"/"+profile.String(), func(t *testing.T) {
					m := InitialModel(t.Context(), nil, nil)
					m.screen = ScreenRunsList
					next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{
						{ID: "1", SpecName: "run-first", Status: tc.status},
						{ID: "2", SpecName: "run-second", Status: tc.status},
					}})
					m = next.(model)
					m.rightPaneList.SetSize(80, 24)
					if filter != "" {
						for _, msg := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: rune(filter[0]), Text: filter}, {Code: tea.KeyEnter}} {
							next, cmd := m.Update(msg)
							m = applyRunFilterCommands(next.(model), cmd)
						}
					}
					for selected := 0; selected < 2; selected++ {
						m.rightPaneList.Select(selected)
						var output strings.Builder
						writer := colorprofile.Writer{Forward: &output, Profile: profile}
						if _, err := writer.WriteString(m.rightPaneList.View()); err != nil {
							t.Fatal(err)
						}
						view := output.String()
						for index, id := range []string{"run-first", "run-second"} {
							found := false
							for _, line := range strings.Split(view, "\n") {
								if !strings.Contains(ansi.Strip(line), id) {
									continue
								}
								found = true
								if !strings.Contains(ansi.Strip(line), tc.glyph+" "+id) {
									t.Fatalf("corrupted title: %q", line)
								}
								colored := strings.Contains(line, "\x1b[1m"+tc.color+tc.glyph)
								if colored != (index != selected && profile >= colorprofile.ANSI) {
									t.Fatalf("selected=%d row=%d: unexpected status color in %q", selected, index, line)
								}
								titleColor := ansi.NewStyle().ForegroundColor(profile.Convert(lipgloss.Color("#dddddd"))).String()
								if colored && !strings.Contains(line, tc.glyph+titleColor+"\x1b[22m") {
									t.Fatalf("status color or bold leaked into spec name: %q", line)
								}
								if filter == tc.glyph && index != selected && profile >= colorprofile.ANSI {
									underline := ansi.NewStyle().Underline(true).ForegroundColor(profile.Convert(lipgloss.Color("#dddddd"))).Underline(true).String()
									if !strings.Contains(line, underline+"\x1b[1m"+tc.color+tc.glyph) {
										t.Fatalf("filtered glyph lost its underline: %q", line)
									}
								}
								if index == selected {
									var expected strings.Builder
									writer := colorprofile.Writer{Forward: &expected, Profile: profile}
									delegate := newRunDelegate(true, 0).DefaultDelegate
									delegate.Render(&writer, m.rightPaneList, index, m.rightPaneList.VisibleItems()[index])
									want, _, _ := strings.Cut(expected.String(), "\n")
									if strings.TrimRight(line, " ") != strings.TrimRight(want, " ") {
										t.Fatalf("selected title styles changed: got %q, want %q", line, want)
									}
								}
							}
							if !found {
								t.Fatalf("missing %s in %q", id, view)
							}
						}
					}
				})
			}
		}
	}
}

// An unfocused Runs preview hides the border without moving row text or changing the spinner.
func TestRunDelegateFocusPreservesLayout(t *testing.T) {
	m := newRunsList("RUNS", []list.Item{runItem{run: domaintypes.RunSummary{
		ID: "run-preview", Status: domaintypes.RunStatusRunning, SpecID: "spec-1", RepoURL: "https://example.com/team/app.git",
	}}})
	m.SetSize(80, 24)
	m.SetDelegate(newRunDelegate(true, 1))
	focused := ansi.Strip(m.View())
	m.SetDelegate(newRunDelegate(false, 1))
	preview := ansi.Strip(m.View())
	if strings.Count(focused, "│") != 3 || strings.Contains(preview, "│") {
		t.Fatalf("focus borders differ: %q / %q", focused, preview)
	}
	if strings.ReplaceAll(focused, "│", " ") != preview {
		t.Fatalf("preview moved row text: %q / %q", focused, preview)
	}
	want := cliruns.StatusGlyph("running", 1) + " spec-1"
	if !strings.Contains(preview, want) {
		t.Fatalf("preview missing spinner frame %q: %q", want, preview)
	}
}
