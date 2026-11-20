package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Run rows show execution identity and shared spec/repository labels.
func TestRunsItemsShowCurrentModel(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{
		{ID: "run-abc", Status: domaintypes.RunStatusRunning, SpecName: "java", SpecSourceDomain: "example.com", SpecSourceRepo: "team/specs", RepoURL: "https://example.com/team/app.git", SourceCommitSHA: "1234567890"},
		{ID: "run-anon", SpecID: "spec-1"},
	}})
	m = next.(model)
	if m.rightPaneList.Title != "RUNS" || m.rightPaneList.Width() != runsListWidth {
		t.Fatalf("runs list title/width = %q/%d", m.rightPaneList.Title, m.rightPaneList.Width())
	}
	items := m.rightPaneList.Items()
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	named := items[0].(runItem)
	if named.Title() != cliruns.StatusGlyph("Running", 0)+" run-abc" || named.Description() != "team/app:12345678\nexample.com/team/specs:java" {
		t.Fatalf("run row = %q / %q", named.Title(), named.Description())
	}
	if got := items[1].(runItem).Description(); got != "-\nspec-1" {
		t.Fatalf("anonymous description = %q", got)
	}
	if strings.Contains(named.FilterValue(), "\x1b") || !strings.Contains(named.FilterValue(), "Running") {
		t.Fatalf("filter text must retain status without color escapes: %q", named.FilterValue())
	}

	m.rightPaneList.SetSize(90, 24)
	lines := strings.Split(ansi.Strip(m.rightPaneList.View()), "\n")
	for _, row := range [][]string{
		{cliruns.StatusGlyph("Running", 0) + " run-abc", "team/app:12345678", "example.com/team/specs:java"},
		{"run-anon", "-", "spec-1"},
	} {
		start := -1
		for n, line := range lines {
			if strings.Contains(line, row[0]) {
				start = n
				break
			}
		}
		if start < 0 || start+2 >= len(lines) {
			t.Fatalf("missing three-line row %q in %q", row[0], lines)
		}
		for offset, want := range row {
			if !strings.Contains(lines[start+offset], want) {
				t.Fatalf("row %q line %d = %q, want %q", row[0], offset+1, lines[start+offset], want)
			}
		}
	}
	m.screen = ScreenRunsList
	for _, msg := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'r', Text: "run-abc"}, {Code: tea.KeyEnter}} {
		next, cmd := m.Update(msg)
		m = applyRunFilterCommands(next.(model), cmd)
	}
	filtered := ansi.Strip(m.rightPaneList.View())
	if !strings.Contains(filtered, cliruns.StatusGlyph("Running", 0)+" run-abc") {
		t.Fatalf("filter corrupted the status icon or run ID: %q", filtered)
	}
}

// Preserve the API order, including its timestamp tie-breaker.
func TestRunsPreserveAPIOrder(t *testing.T) {
	now := time.Now()
	runs := []domaintypes.RunSummary{{ID: "run-z", CreatedAt: now}, {ID: "run-a", CreatedAt: now}, {ID: "run-old", CreatedAt: now.Add(-time.Hour)}}
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: runs})
	for i, item := range next.(model).rightPaneList.Items() {
		if item.(runItem).run.ID != runs[i].ID {
			t.Fatalf("item %d = %s, want %s", i, item.(runItem).run.ID, runs[i].ID)
		}
	}
}

// Accepting a filter does not navigate; the next Enter selects the visible run.
func TestFilteredRunSelectionUsesVisibleIdentity(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-first"}, {ID: "run-q"}}})
	m = next.(model)
	for _, msg := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'q', Text: "q"}} {
		next, cmd := m.Update(msg)
		m = applyRunFilterCommands(next.(model), cmd)
	}
	if !m.rightPaneList.SettingFilter() || m.rightPaneList.FilterValue() != "q" {
		t.Fatal("q did not enter the filter")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.screen != ScreenRunsList || !m.rightPaneList.IsFiltered() {
		t.Fatal("filter Enter navigated or failed to accept")
	}
	m = applyRunFilterCommands(m, cmd)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.screen != ScreenRunDetails || m.selectedRunID != "run-q" || cmd == nil {
		t.Fatalf("selected run = %s, screen = %v", m.selectedRunID, m.screen)
	}
	assertPloyItems(t, m, []listItem{{title: "Run", description: "run-q"}, {title: "Jobs", description: "total: —"}})
}

// Esc cancels an active or applied filter before returning to the root.
func TestRunFilterEscAndCtrlC(t *testing.T) {
	for _, accept := range []bool{false, true} {
		m := InitialModel(t.Context(), nil, nil)
		m.usersLoading = false
		m.screen = ScreenRunsList
		next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-q"}}})
		m = next.(model)
		for _, msg := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'q', Text: "q"}} {
			next, cmd := m.Update(msg)
			m = applyRunFilterCommands(next.(model), cmd)
		}
		if accept {
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = applyRunFilterCommands(next.(model), cmd)
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("Ctrl+C has no quit command while filtering")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("Ctrl+C did not quit while filtering")
		}
		next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		m = applyRunFilterCommands(next.(model), cmd)
		if m.screen != ScreenRunsList || m.rightPaneList.SettingFilter() || m.rightPaneList.IsFiltered() {
			t.Fatal("Esc did not clear the filter in place")
		}
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if next.(model).screen != ScreenPloyList {
			t.Fatal("second Esc did not return to root")
		}
	}
}

// Empty lists remain on their list screen when Enter has no selection.
func TestEmptyListsIgnoreEnter(t *testing.T) {
	for _, screen := range []Screen{ScreenRunsList, ScreenJobsList} {
		m := InitialModel(t.Context(), nil, nil)
		m.usersLoading = false
		m.screen = screen
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if next.(model).screen != screen || cmd != nil {
			t.Fatalf("empty screen %v navigated or issued command", screen)
		}
	}
}

func applyRunFilterCommands(m model, cmd tea.Cmd) model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, child := range msg {
			m = applyRunFilterCommands(m, child)
		}
	case list.FilterMatchesMsg:
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

// Revisiting the root Runs entry starts a new request and rejects older responses.
func TestRunsLatestRequestWins(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	previousRequest := m.runsRequest
	next, _ := m.Update(runsLoadedMsg{requestID: previousRequest, runs: []domaintypes.RunSummary{{ID: "run-previous"}}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(model)
	if m.runsRequest <= previousRequest {
		t.Fatal("root Runs revisit did not supersede its completed request")
	}

	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-current"}}})
	m = next.(model)
	for _, late := range []runsLoadedMsg{
		{requestID: previousRequest, runs: []domaintypes.RunSummary{{ID: "run-old"}}},
		{requestID: previousRequest, err: errors.New("old failure")},
	} {
		next, _ = m.Update(late)
		m = next.(model)
	}
	if len(m.rightPaneList.Items()) != 1 || m.rightPaneList.Items()[0].(runItem).run.ID != "run-current" || m.lastErr != nil {
		t.Fatal("stale response replaced the current runs or error")
	}
	currentErr := errors.New("current failure")
	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, err: currentErr})
	m = next.(model)
	if m.lastErr != currentErr {
		t.Fatal("current run request error was ignored")
	}
}
