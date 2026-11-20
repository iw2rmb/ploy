package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestLoadRunsPreservesModelAndCreatorFilter(t *testing.T) {
	t.Setenv("USER", " alice ")
	expected := []domaintypes.RunSummary{
		{ID: domaintypes.NewRunID(), MigID: domaintypes.NewMigID(), SpecID: domaintypes.NewSpecID(), Status: domaintypes.RunStatusRunning, SpecName: "upgrade", SpecSourceDomain: "git.example.com", SpecSourceRepo: "team/specs", RepoURL: "https://git.example.com/team/app.git", SourceCommitSHA: "0123456789"},
		{ID: domaintypes.NewRunID(), MigID: domaintypes.NewMigID(), SpecID: domaintypes.NewSpecID()},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs" || r.URL.Query().Get("created_by") != "alice" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected runs request: %s", r.URL)
		}
		if r.URL.Query().Has("all") {
			t.Error("runs request must use server creator filtering")
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"runs": expected}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	msg, ok := loadRunsCmd(t.Context(), server.Client(), base, 4, "alice", "", false)().(runsLoadedMsg)
	if !ok || msg.requestID != 4 || msg.err != nil || len(msg.runs) != 2 {
		t.Fatalf("runs response = %+v, error = %v", msg, msg.err)
	}
	if !reflect.DeepEqual(msg.runs, expected) {
		t.Fatalf("run model or API order was lost: got %+v, want %+v", msg.runs, expected)
	}
}

// All omits creator filters; a selected username reaches both list endpoints.
func TestLoadCommandsApplyUserScope(t *testing.T) {
	for _, username := range []string{"", "alice"} {
		t.Run("user="+username, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("creator") != username {
					t.Errorf("creator filter = %q, want %q", r.URL.Query().Get("creator"), username)
				}
				switch r.URL.Path {
				case "/v1/runs":
					if (r.URL.Query().Get("all") == "true") != (username == "") {
						t.Errorf("all filter = %q", r.URL.Query().Get("all"))
					}
					fmt.Fprint(w, `{"runs":[]}`)
				case "/v1/jobs":
					fmt.Fprint(w, `{"jobs":[],"total":0}`)
				default:
					t.Errorf("unexpected request: %s", r.URL)
				}
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			runs := loadRunsCmd(t.Context(), server.Client(), base, 1, "", username, username == "")().(runsLoadedMsg)
			jobs := loadJobsCmd(t.Context(), server.Client(), base, "", 1, nil, username)().(jobsLoadedMsg)
			if runs.err != nil || jobs.err != nil {
				t.Fatalf("list errors: %v / %v", runs.err, jobs.err)
			}
		})
	}
}

// Scoped rows and totals use the current-attempt endpoint without the global list's page limit.
func TestJobCommandsUseRequestedScope(t *testing.T) {
	runID, repoID := domaintypes.NewRunID(), domaintypes.NewRepoID()
	started := time.Date(2026, 11, 20, 12, 36, 33, 0, time.UTC)
	jobs := make([]migsapi.RunJob, 101)
	for i := range jobs {
		jobs[i] = migsapi.RunJob{JobID: domaintypes.NewJobID(), Status: domaintypes.JobStatusCreated}
	}
	jobs[0].Status = domaintypes.JobStatusRunning
	jobs[0].StartedAt = &started
	jobs[0].Name = "mig-3"
	jobs[0].DisplayName = "healing"
	jobs[0].JobType = domaintypes.JobTypeMig
	for _, tc := range []struct {
		name  string
		runID domaintypes.RunID
		total bool
	}{
		{name: "global jobs"},
		{name: "selected run jobs", runID: runID},
		{name: "selected run total", runID: runID, total: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if tc.runID.IsZero() {
					if r.URL.Path != "/v1/jobs" || r.URL.Query().Get("limit") != "100" || r.URL.Query().Has("run_id") {
						t.Errorf("unexpected global request: %s", r.URL)
					}
					fmt.Fprint(w, `{"jobs":[],"total":7}`)
					return
				}
				if r.URL.Path != "/v1/runs/"+runID.String()+"/jobs" || r.URL.RawQuery != "" {
					t.Errorf("unexpected current-attempt request: %s", r.URL)
				}
				if err := json.NewEncoder(w).Encode(migsapi.ListRunJobsResponse{RunID: runID, RepoID: repoID, Attempt: 2, Jobs: jobs}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			if tc.total {
				msg, ok := loadRunDetailsCmd(t.Context(), server.Client(), base, tc.runID)().(runDetailsLoadedMsg)
				if !ok || msg.runID != runID || msg.jobTotal != len(jobs) {
					t.Fatalf("current attempt total = %+v", msg)
				}
			} else {
				msg, ok := loadJobsCmd(t.Context(), server.Client(), base, tc.runID, 3, nil, "")().(jobsLoadedMsg)
				if !ok || msg.runID != tc.runID || msg.requestID != 3 || msg.err != nil {
					t.Fatalf("job load = %+v, err = %v", msg, msg.err)
				}
				if tc.runID.IsZero() {
					if len(msg.jobs) != 0 {
						t.Fatal("global list changed")
					}
				} else {
					if len(msg.jobs) != 101 {
						t.Fatalf("scoped jobs truncated: %d", len(msg.jobs))
					}
					found := false
					for _, job := range msg.jobs {
						if job.JobID != jobs[0].JobID {
							continue
						}
						found = true
						if job.RunID != runID || job.RepoID != repoID || job.DisplayName != "healing" || job.JobType != domaintypes.JobTypeMig || job.StartedAt == nil || !job.StartedAt.Equal(started) || job.DurationMs != 0 {
							t.Fatalf("running job metadata lost: %+v", job)
						}
					}
					if !found {
						t.Fatal("missing running job")
					}
				}
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want exactly one", requests)
			}
		})
	}
}

func TestLoadCommandsReturnRequestErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	for name, cmd := range map[string]tea.Cmd{
		"runs":        loadRunsCmd(t.Context(), server.Client(), base, 4, "alice", "", false),
		"jobs":        loadJobsCmd(t.Context(), server.Client(), base, "run-1", 3, nil, ""),
		"run details": loadRunDetailsCmd(t.Context(), server.Client(), base, "run-1"),
	} {
		t.Run(name, func(t *testing.T) {
			msg := cmd()
			if name == "jobs" {
				result, ok := msg.(jobsLoadedMsg)
				if !ok || result.err == nil || result.runID != "run-1" || result.requestID != 3 {
					t.Fatalf("jobs error lost request identity: %+v", msg)
				}
			} else if name == "runs" {
				result, ok := msg.(runsLoadedMsg)
				if !ok || result.err == nil || result.requestID != 4 {
					t.Fatalf("runs error lost request identity: %+v", msg)
				}
			} else {
				result, ok := msg.(errMsg)
				if !ok || result.err == nil {
					t.Fatalf("command must return its request error: %+v", msg)
				}
			}
		})
	}
}

// Session cancellation stops each loader, including retained-attempt metadata requests.
func TestSessionCancellationStopsBlockedRequests(t *testing.T) {
	for _, tc := range []struct {
		name        string
		blockedPath string
		retained    bool
		load        func(model) tea.Cmd
	}{
		{name: "initial runs", blockedPath: "/v1/runs", load: func(m model) tea.Cmd {
			return loadRunsCmd(m.ctx, m.client, m.baseURL, m.runsRequest, "alice", "", false)
		}},
		{name: "refreshed runs", blockedPath: "/v1/runs", load: func(m model) tea.Cmd { return m.loadRuns() }},
		{name: "run jobs", blockedPath: "/v1/runs/3LwwG1Z0WzQIVtw5r5WE93j0t8M/jobs", load: func(m model) tea.Cmd {
			m.selectedRunID = "3LwwG1Z0WzQIVtw5r5WE93j0t8M"
			return m.loadJobs()
		}},
		{name: "run total", blockedPath: "/v1/runs/3LwwG1Z0WzQIVtw5r5WE93j0t8M/jobs", load: func(m model) tea.Cmd {
			return loadRunDetailsCmd(m.ctx, m.client, m.baseURL, "3LwwG1Z0WzQIVtw5r5WE93j0t8M")
		}},
		{name: "global jobs", blockedPath: "/v1/jobs", load: func(m model) tea.Cmd { return m.loadJobs() }},
		{name: "global current attempt", blockedPath: "/v1/runs/3LwwG1Z0WzQIVtw5r5WE93j0t8M/jobs", load: func(m model) tea.Cmd { return m.loadJobs() }},
		{name: "global job status", blockedPath: "/v1/jobs/3LwwG1Z0WzQIVtw5r5WE93j0t8N/status", retained: true, load: func(m model) tea.Cmd { return m.loadJobs() }},
		{name: "global retained attempt", blockedPath: "/v1/runs/3LwwG1Z0WzQIVtw5r5WE93j0t8M/jobs?attempt=1", retained: true, load: func(m model) tea.Cmd { return m.loadJobs() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := make(chan struct{})
			cancelled := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RequestURI() == tc.blockedPath || (!strings.Contains(tc.blockedPath, "?") && r.URL.Path == tc.blockedPath) {
					close(started)
					select {
					case <-r.Context().Done():
						close(cancelled)
					case <-release:
					}
					return
				}
				switch r.URL.Path {
				case "/v1/jobs":
					fmt.Fprint(w, `{"jobs":[{"job_id":"3LwwG1Z0WzQIVtw5r5WE93j0t8N","run_id":"3LwwG1Z0WzQIVtw5r5WE93j0t8M","status":"Running"}],"total":1}`)
				case "/v1/runs/3LwwG1Z0WzQIVtw5r5WE93j0t8M/jobs":
					if tc.retained {
						fmt.Fprint(w, `{"run_id":"3LwwG1Z0WzQIVtw5r5WE93j0t8M","attempt":2,"jobs":[]}`)
					} else {
						fmt.Fprint(w, `{"run_id":"3LwwG1Z0WzQIVtw5r5WE93j0t8M","attempt":2,"jobs":[{"job_id":"3LwwG1Z0WzQIVtw5r5WE93j0t8N","status":"Running"}]}`)
					}
				case "/v1/jobs/3LwwG1Z0WzQIVtw5r5WE93j0t8N/status":
					fmt.Fprint(w, `{"job_id":"3LwwG1Z0WzQIVtw5r5WE93j0t8N","run_id":"3LwwG1Z0WzQIVtw5r5WE93j0t8M","attempt":1}`)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			defer close(release)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			base, _ := url.Parse(server.URL)
			m := InitialModel(ctx, server.Client(), base)
			m.usersLoading = false
			cmd := tc.load(m)
			done := make(chan tea.Msg, 1)
			go func() { done <- cmd() }()
			select {
			case <-started:
			case msg := <-done:
				t.Fatalf("loader returned before blocked request: %+v", msg)
			case <-time.After(3 * time.Second):
				t.Fatal("request did not reach the blocked endpoint")
			}
			cancel()
			select {
			case msg := <-done:
				var err error
				switch msg := msg.(type) {
				case runsLoadedMsg:
					err = msg.err
				case jobsLoadedMsg:
					err = msg.err
				case errMsg:
					err = msg.err
				default:
					t.Fatalf("unexpected response: %#v", msg)
				}
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("loader error = %v, want context cancellation", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("loader did not stop after session cancellation")
			}
			select {
			case <-cancelled:
			case <-time.After(3 * time.Second):
				t.Fatal("server request context was not cancelled")
			}
		})
	}
}
