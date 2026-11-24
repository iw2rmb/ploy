package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Startup resolves Users capability before choosing the Runs creator scope.
func TestModelInit(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(fmt.Sprint("admin=", admin), func(t *testing.T) {
			t.Setenv("USER", "alice")
			requests := []string{}
			runID := domaintypes.NewRunID()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				switch r.URL.Path {
				case "/v1/tokens":
					if !admin {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
					fmt.Fprint(w, `{"tokens":[{"username":"alice"}]}`)
				case "/v1/runs":
					if r.URL.Query().Get("limit") != "100" || (r.URL.Query().Get("all") == "true") != admin || (!admin && r.URL.Query().Get("created_by") != "alice") {
						t.Errorf("unexpected startup scope: %s", r.URL)
					}
					fmt.Fprintf(w, `{"runs":[{"id":%q,"status":"Success"}]}`, runID.String())
				default:
					t.Errorf("unexpected startup request: %s", r.URL)
				}
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			m := InitialModel(t.Context(), server.Client(), base)
			if m.screen != ScreenPloyList || m.rootList.Index() != runsIndex || !m.usersLoading || !m.runsLoading || m.runsRequest != 1 {
				t.Fatal("unexpected startup state")
			}
			msg, ok := m.Init()().(usersLoadedMsg)
			if !ok || msg.err != nil || msg.enabled != admin || len(requests) != 1 || requests[0] != "/v1/tokens" || !m.usersLoading {
				t.Fatalf("capability result = %+v, requests = %v", msg, requests)
			}
			next, cmd := m.Update(msg)
			m = next.(model)
			if m.usersLoading || m.usersEnabled != admin || cmd == nil {
				t.Fatal("capability did not start the Runs load")
			}
			runs, ok := cmd().(runsLoadedMsg)
			if !ok || runs.err != nil || runs.requestID != m.runsRequest || len(requests) != 2 || requests[1] != "/v1/runs" {
				t.Fatalf("Runs startup result = %+v, requests = %v", runs, requests)
			}
			next, _ = m.Update(runs)
			m = next.(model)
			if m.runsLoading || len(m.rightPaneList.Items()) != 1 || m.rightPaneList.Items()[0].(runItem).run.ID != runID {
				t.Fatal("startup result did not populate the root preview")
			}
		})
	}
}

// Esc returns to the parent screen without quitting.
func TestModelEscTransitions(t *testing.T) {
	tests := []struct {
		name       string
		from, want Screen
		scope      domaintypes.RunID
	}{
		{"root", ScreenPloyList, ScreenPloyList, ""},
		{"runs", ScreenRunsList, ScreenPloyList, ""},
		{"run details", ScreenRunDetails, ScreenRunsList, "run-1"},
		{"global jobs", ScreenJobsList, ScreenPloyList, ""},
		{"users", ScreenUsersList, ScreenPloyList, ""},
		{"scoped jobs", ScreenJobsList, ScreenRunDetails, "run-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(t.Context(), nil, nil)
			m.screen = tt.from
			m.selectedRunID = tt.scope
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			nm := next.(model)
			if nm.screen != tt.want || cmd != nil {
				t.Fatalf("Esc = screen %v, command %v; want screen %v without command", nm.screen, cmd, tt.want)
			}
			if tt.want == ScreenPloyList || tt.want == ScreenRunsList {
				assertPloyItems(t, nm, []listItem{{title: "Runs", description: "select run\n"}, {title: "Jobs", description: "select job\n"}, {title: "Users", description: "cli-admin only\n"}})
			}
		})
	}
}

// Ctrl+C quits on every screen; q remains a normal key.
func TestModelQuitKeys(t *testing.T) {
	for _, screen := range []Screen{ScreenPloyList, ScreenRunsList, ScreenRunDetails, ScreenJobsList, ScreenUsersList} {
		m := InitialModel(t.Context(), nil, nil)
		m.screen = screen
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatalf("screen %v: Ctrl+C returned no command", screen)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("screen %v: Ctrl+C did not quit", screen)
		}
		next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if next.(model).screen != screen {
			t.Fatalf("q changed screen %v", screen)
		}
		if cmd != nil {
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Fatalf("screen %v: q quit", screen)
			}
		}
	}
}

// Enter opens the root entry's list.
func TestModelEnterTransitionsFromRoot(t *testing.T) {
	for _, tt := range []struct {
		name  string
		index int
		want  Screen
	}{{"Runs", 0, ScreenRunsList}, {"Jobs", 1, ScreenJobsList}} {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(t.Context(), nil, nil)
			m.rootList.Select(tt.index)
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if next.(model).screen != tt.want || cmd != nil {
				t.Fatalf("Enter = screen %v, cmd %v; want %v without another load", next.(model).screen, cmd, tt.want)
			}
		})
	}
}

func TestNewRunsListInvariants(t *testing.T) {
	l := newRunsList("RUNS", nil)

	if l.Width() != runsListWidth {
		t.Errorf("runs list width: got %d, want %d", l.Width(), runsListWidth)
	}
	if l.Title != "RUNS" {
		t.Errorf("runs list title: got %q, want %q", l.Title, "RUNS")
	}
}

func assertPloyItems(t *testing.T, m model, want []listItem) {
	t.Helper()
	items := m.rootList.Items()
	if len(items) != len(want) {
		t.Fatalf("PLOY items count = %d, want %d", len(items), len(want))
	}
	for i, expected := range want {
		item, ok := items[i].(list.DefaultItem)
		if !ok {
			t.Fatalf("PLOY item %d has type %T, want list.DefaultItem", i, items[i])
		}
		if item.Title() != expected.title || item.Description() != expected.description {
			t.Errorf("PLOY item %d = %q/%q, want %q/%q", i, item.Title(), item.Description(), expected.title, expected.description)
		}
	}
}

func TestNewPloyListInvariants(t *testing.T) {
	l := newPloyList("PLOY", nil)

	if l.Width() != ployListWidth {
		t.Errorf("ploy list width: got %d, want %d", l.Width(), ployListWidth)
	}
	if l.Title != "PLOY" {
		t.Errorf("ploy list title: got %q, want %q", l.Title, "PLOY")
	}
}
