package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Resize keeps PLOY fixed and gives RUNS the remaining terminal width.
func TestWindowSizeUpdatesRunWidthAndListHeights(t *testing.T) {
	for _, tt := range []struct{ width, want int }{{120, 88}, {60, 28}, {20, 1}} {
		m := InitialModel(t.Context(), nil, nil)
		next, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: 33})
		m = next.(model)
		if m.rootList.Width() != 30 || m.rightPaneList.Width() != tt.want || m.rootList.Height() != 33 || m.rightPaneList.Height() != 33 || m.usersList.Width() != tt.want || m.usersList.Height() != 33 {
			t.Fatalf("terminal %d: PLOY width %d, RUNS width %d, heights %d/%d", tt.width, m.rootList.Width(), m.rightPaneList.Width(), m.rootList.Height(), m.rightPaneList.Height())
		}
	}
}

// List recreation retains the terminal dimensions and bounds long labels.
func TestWindowSizePersistsAcrossRunListRecreation(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 29})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.rightPaneList.Height() != 29 || m.rightPaneList.Width() != 48 {
		t.Fatal("Enter lost terminal dimensions")
	}
	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-1", SpecID: domaintypes.SpecID(strings.Repeat("s", 100)), RepoURL: "https://example.com/team/" + strings.Repeat("r", 100)}}})
	m = next.(model)
	if m.rightPaneList.Height() != 29 || m.rightPaneList.Width() != 48 || len(m.rightPaneList.Items()) != 1 {
		t.Fatal("loaded rows lost terminal dimensions or were not accepted")
	}
	for _, width := range []int{120, 80, 65, 64, 60, 30, 15} {
		next, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 29})
		m = next.(model)
		view := m.View().Content
		if width >= 60 && !strings.Contains(view, "STATUS") {
			t.Fatalf("missing RUN fields at width %d", width)
		}
		if got := lipgloss.Width(view); got > width {
			t.Fatalf("rendered width = %d, limit %d", got, width)
		}
	}
}
