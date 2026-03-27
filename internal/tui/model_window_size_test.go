package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	clitui "github.com/iw2rmb/ploy/internal/client/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestWindowSizeSetsAllListHeights(t *testing.T) {
	m := InitialModel(nil, nil)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 33})
	nm := next.(model)

	if nm.ploy.Height() != 33 {
		t.Fatalf("ploy height = %d, want 33", nm.ploy.Height())
	}
	if nm.secondary.Height() != 33 {
		t.Fatalf("secondary height = %d, want 33", nm.secondary.Height())
	}
	if nm.detail.Height() != 33 {
		t.Fatalf("detail height = %d, want 33", nm.detail.Height())
	}
}

func TestWindowHeightPersistsAcrossListRecreation(t *testing.T) {
	m := InitialModel(nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 29})
	nm := next.(model)

	nm.ploy.Select(0)
	next, _ = nm.handleEnter()
	nm = next.(model)
	if nm.secondary.Height() != 29 {
		t.Fatalf("secondary height after enter = %d, want 29", nm.secondary.Height())
	}

	next, _ = nm.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-1"), Name: "alpha"},
	}})
	nm = next.(model)
	if nm.secondary.Height() != 29 {
		t.Fatalf("secondary height after migs load = %d, want 29", nm.secondary.Height())
	}

	nm.secondary.Select(0)
	next, _ = nm.handleEnter()
	nm = next.(model)
	if nm.detail.Height() != 29 {
		t.Fatalf("detail height after enter = %d, want 29", nm.detail.Height())
	}
}
