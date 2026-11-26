package tui

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Pending capability blocks preview loads; denied or failed capability keeps Users inactive.
func TestUsersCapabilityControlsNavigationAndStartupScope(t *testing.T) {
	t.Setenv("USER", " alice ")
	for _, result := range []usersLoadedMsg{{}, {err: errors.New("token lookup failed")}, {enabled: true, users: []string{"alice"}}} {
		m := InitialModel(t.Context(), nil, nil)
		for _, key := range []rune{tea.KeyDown, tea.KeyDown, tea.KeyEnter} {
			next, cmd := m.Update(tea.KeyPressMsg{Code: key})
			m = next.(model)
			if cmd != nil || m.screen != ScreenPloyList || m.jobsLoading || m.jobsRequest != 0 || m.runsRequest != 1 {
				t.Fatal("pending capability allowed navigation or a preview request")
			}
		}
		next, cmd := m.Update(result)
		m = next.(model)
		if cmd == nil || m.usersLoading || m.usersErr != result.err {
			t.Fatal("capability did not finish with its startup load and error")
		}
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(model)
		createdBy, creator, all := m.runsCreatorScope()
		if result.enabled {
			if m.screen != ScreenUsersList || !m.usersVisible() || createdBy != "" || creator != "" || !all || m.usersList.SelectedItem().(userItem).username != "" {
				t.Fatal("cli-admin capability did not enable Users with All selected")
			}
		} else if m.screen != ScreenPloyList || m.usersVisible() || createdBy != "alice" || creator != "" || all || m.jobsCreatedBy() != "" {
			t.Fatal("inactive Users changed existing creator scope or allowed entry")
		}
		if !result.enabled {
			want := "cli-admin only"
			if result.err != nil {
				want = "unavailable"
			}
			if m.rootList.Items()[usersIndex].(listItem).description != want {
				t.Fatalf("Users capability label must be %q", want)
			}
		}
	}
}

// Each user change clears prior rows and requests, then scopes both lists to the selected creator.
func TestUsersSelectionScopesListsAndRejectsPriorResponses(t *testing.T) {
	requests := make(chan *url.URL, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL
		if r.URL.Path == "/v1/runs" {
			fmt.Fprint(w, `{"runs":[]}`)
		} else if r.URL.Path == "/v1/jobs" {
			fmt.Fprint(w, `{"jobs":[],"total":0}`)
		} else {
			t.Errorf("unexpected list request: %s", r.URL)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	m := InitialModel(t.Context(), server.Client(), base)
	next, _ := m.Update(usersLoadedMsg{enabled: true, users: []string{"All", "alice", "bob", "All (user)"}})
	m = next.(model)
	if m.usersList.Items()[0].(userItem).Title() != "All" || m.usersList.Items()[1].(userItem).Title() != "@All" {
		t.Fatal("All creators and the username All have ambiguous labels")
	}
	for _, selection := range []struct {
		index int
		user  string
	}{{2, "alice"}, {1, "All"}, {4, "All (user)"}, {0, ""}} {
		m.rightPaneList.SetItems([]list.Item{runItem{run: domaintypes.RunSummary{ID: "old-run"}}})
		m.rightPaneList.SetFilterText("old")
		m.rightPaneList.SetFilterState(list.FilterApplied)
		m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "old-job"}}).SetSelectedJobID("old-job")
		m.selectedRunID = "old-run"
		m.rootList.Select(usersIndex)
		m.screen = ScreenUsersList
		m.usersList.Select(selection.index)
		oldRuns, oldJobs := m.runsRequest, m.jobsRequest
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(model)
		if m.selectedUser != selection.user || m.screen != ScreenPloyList || m.rootList.Index() != usersIndex || !m.selectedRunID.IsZero() || !m.jobList.ConfirmedJobID().IsZero() || m.rightPaneList.IsFiltered() || len(m.rightPaneList.Items()) != 0 || len(m.jobList.Jobs()) != 0 || m.runsRequest <= oldRuns || m.jobsRequest <= oldJobs {
			t.Fatal("creator change retained old rows, selections, filters, or request identity")
		}
		for _, late := range []tea.Msg{
			runsLoadedMsg{requestID: oldRuns, runs: []domaintypes.RunSummary{{ID: "stale-run"}}, err: errors.New("old error")},
			jobsLoadedMsg{requestID: oldJobs, jobs: []domainapi.JobListItem{{JobID: "stale-job"}}},
			runDetailsLoadedMsg{requestID: m.detailsRequest, runDetails: runDetails{runID: "old-run", jobTotal: new(99)}},
		} {
			next, _ = m.Update(late)
			m = next.(model)
		}
		if len(m.rightPaneList.Items()) != 0 || len(m.jobList.Jobs()) != 0 || m.lastErr != nil || m.runDetails.jobTotal != nil || !m.runsLoading || !m.jobsLoading {
			t.Fatalf("prior creator response: runs=%d jobs=%d error=%v total=%v loading=%v/%v", len(m.rightPaneList.Items()), len(m.jobList.Jobs()), m.lastErr, m.runDetails.jobTotal, m.runsLoading, m.jobsLoading)
		}
		batch, ok := cmd().(tea.BatchMsg)
		if !ok || len(batch) != 2 {
			t.Fatal("creator change did not reload Runs and Jobs")
		}
		for _, load := range batch {
			next, _ = m.Update(load())
			m = next.(model)
			if m.lastErr != nil {
				t.Fatalf("list load failed: %v", m.lastErr)
			}
			u := <-requests
			if u.Query().Get("creator") != selection.user {
				t.Fatalf("creator request = %s, want %q", u, selection.user)
			}
			if u.Path == "/v1/runs" && (u.Query().Get("all") == "true") != (selection.user == "") {
				t.Fatalf("Runs All scope = %s", u)
			}
		}
		label := "All"
		if selection.user != "" {
			label = "@" + selection.user
		}
		if got := m.rootList.Items()[usersIndex].(listItem).description; got != label {
			t.Fatalf("Users context = %q, want %q", got, label)
		}
		m.rootList.Select(runsIndex)
		for _, key := range []rune{tea.KeyEnter, tea.KeyEscape} {
			next, _ = m.Update(tea.KeyPressMsg{Code: key})
			m = next.(model)
		}
		if m.selectedUser != selection.user {
			t.Fatal("navigation cleared the selected creator")
		}
	}
}

// Users supports filtering, Enter confirmation, and Esc without changing the creator during filter editing.
func TestUsersFilteringAndEscPreserveCreator(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	next, _ := m.Update(usersLoadedMsg{enabled: true, users: []string{"alice", "bob"}})
	m = next.(model)
	m.rootList.Select(usersIndex)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	for _, key := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'b', Text: "bob"}, {Code: tea.KeyEnter}} {
		next, cmd := m.Update(key)
		m = applyRunFilterCommands(next.(model), cmd)
	}
	if m.selectedUser != "" || !m.usersList.IsFiltered() || m.usersList.SelectedItem().(userItem).username != "bob" {
		t.Fatal("filter Enter changed creator or lost its matching username")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.selectedUser != "bob" || m.screen != ScreenPloyList {
		t.Fatal("Enter did not confirm the filtered creator")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.screen != ScreenUsersList || m.usersList.IsFiltered() || m.selectedUser != "bob" {
		t.Fatal("first Esc did not clear the filter while retaining the creator")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.screen != ScreenPloyList || m.selectedUser != "bob" || !strings.Contains(m.View().Content, "USERS") {
		t.Fatal("second Esc did not return to the Users preview")
	}
}

// A delayed filter result preserves cursor movement and the next Enter target.
func TestUsersDelayedFilterPreservesHighlightedUsername(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	next, _ := m.Update(usersLoadedMsg{enabled: true, users: []string{"bob", "bobby"}})
	m = next.(model)
	m.rootList.Select(usersIndex)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = next.(model)
	result := findFilterResult(t, cmd)
	next, _ = m.Update(result)
	m = next.(model)
	for _, key := range []rune{tea.KeyEnter, tea.KeyDown} {
		next, _ = m.Update(tea.KeyPressMsg{Code: key})
		m = next.(model)
	}
	if m.usersList.SelectedItem().(userItem).username != "bobby" {
		t.Fatal("cursor did not move to bobby")
	}
	next, _ = m.Update(result)
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.selectedUser != "bobby" || m.screen != ScreenPloyList {
		t.Fatal("delayed filter result changed the Enter target")
	}
}
