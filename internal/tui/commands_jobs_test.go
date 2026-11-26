package tui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestGlobalJobsHydratesExactJobsAndReusesRunAttempts(t *testing.T) {
	runA, runB := domaintypes.NewRunID(), domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	current, olderA, olderB, other := domaintypes.NewJobID(), domaintypes.NewJobID(), domaintypes.NewJobID(), domaintypes.NewJobID()
	started := time.Date(2026, 11, 20, 12, 36, 33, 0, time.UTC)
	page := []domainapi.JobListItem{
		{JobID: olderA, RunID: runA, RepoID: repoID, Name: "mig-0"},
		{JobID: other, RunID: runB, RepoID: repoID, RepoURL: "https://git.example.com/team/app.git", Name: "mig-0"},
		{JobID: current, RunID: runA, RepoID: repoID, Name: "mig-0"},
		{JobID: olderB, RunID: runA, RepoID: repoID, Name: "mig-1"},
	}
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.RequestURI()]++
		mu.Unlock()
		var payload any
		switch r.URL.Path {
		case "/v1/jobs":
			payload = domainapi.JobListResponse{Jobs: page, Total: 4}
		case "/v1/runs/" + runA.String() + "/jobs":
			if r.URL.Query().Get("attempt") == "1" {
				payload = migsapi.ListRunJobsResponse{RunID: runA, RepoID: repoID, Attempt: 1, Jobs: []migsapi.RunJob{
					{JobID: olderA, DisplayName: "old-step-a", JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusSuccess, StartedAt: &started},
					{JobID: olderB, DisplayName: "old-step-b", JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusSuccess, StartedAt: &started},
				}}
			} else {
				payload = migsapi.ListRunJobsResponse{RunID: runA, RepoID: repoID, Attempt: 2, Jobs: []migsapi.RunJob{
					{JobID: current, DisplayName: "healing", JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusRunning, StartedAt: &started},
				}}
			}
		case "/v1/runs/" + runB.String() + "/jobs":
			payload = migsapi.ListRunJobsResponse{RunID: runB, RepoID: repoID, Attempt: 1, Jobs: []migsapi.RunJob{
				{JobID: other, DisplayName: "deprecations", JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusRunning, StartedAt: &started},
			}}
		case "/v1/runs/" + runA.String():
			payload = domaintypes.RunSummary{ID: runA, SpecID: domaintypes.NewSpecID(), MigID: domaintypes.NewMigID(), RepoURL: "https://git.example.com/team/app.git"}
		case "/v1/jobs/" + olderA.String() + "/status":
			payload = domainapi.JobStatusResponse{JobID: olderA, RunID: runA, RepoID: repoID, Attempt: 1}
		default:
			t.Errorf("unexpected metadata request: %s", r.URL.RequestURI())
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	msg := loadJobsCmd(t.Context(), server.Client(), base, "", 1, nil, "", nil)().(jobsLoadedMsg)
	jobs := msg.jobs
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	wantNames := []string{"old-step-a", "deprecations", "healing", "old-step-b"}
	if len(jobs) != len(page) {
		t.Fatalf("jobs count = %d, want %d", len(jobs), len(page))
	}
	for i, job := range jobs {
		if job.JobID != page[i].JobID || job.RunID != page[i].RunID || job.RepoID != repoID || job.RepoURL != "https://git.example.com/team/app.git" || job.DisplayName != wantNames[i] || job.StartedAt == nil || !job.StartedAt.Equal(started) {
			t.Fatalf("job %d lost page order or exact metadata: %+v", i, job)
		}
	}
	wantRequests := map[string]int{
		"/v1/runs/" + runA.String():                     1,
		"/v1/jobs?limit=100":                            1,
		"/v1/runs/" + runA.String() + "/jobs":           1,
		"/v1/runs/" + runB.String() + "/jobs":           1,
		"/v1/runs/" + runA.String() + "/jobs?attempt=1": 1,
		"/v1/jobs/" + olderA.String() + "/status":       1,
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("metadata requests = %v, want %v", requests, wantRequests)
	}
}

func TestGlobalJobsIncludesRunningJobsOutsideFirstPage(t *testing.T) {
	runID, repoID := domaintypes.NewRunID(), domaintypes.NewRepoID()
	start := time.Now()
	job := func(status domaintypes.JobStatus) domainapi.JobListItem {
		return domainapi.JobListItem{JobID: domaintypes.NewJobID(), RepoID: repoID, RunID: runID, Status: status, StartedAt: &start}
	}
	firstPage := []domainapi.JobListItem{job(domaintypes.JobStatusRunning)}
	for i := 1; i < 100; i++ {
		firstPage = append(firstPage, job(domaintypes.JobStatusSuccess))
	}
	running := []domainapi.JobListItem{firstPage[0], job(domaintypes.JobStatusRunning)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jobs":
			creator := r.URL.Query().Get("creator")
			if creator != "alice" && creator != "bob" {
				t.Error("missing creator filter")
			}
			response := domainapi.JobListResponse{Jobs: firstPage, Total: 101}
			if r.URL.Query().Get("status") == "Running" {
				if creator == "bob" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				response = domainapi.JobListResponse{Jobs: running, Total: 2}
			}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Error(err)
			}
		case "/v1/runs/" + runID.String() + "/jobs":
			if err := json.NewEncoder(w).Encode(migsapi.ListRunJobsResponse{RunID: runID, RepoID: repoID, Jobs: []migsapi.RunJob{
				{JobID: running[0].JobID}, {JobID: running[1].JobID},
			}}); err != nil {
				t.Error(err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	jobs, err := loadGlobalJobs(t.Context(), server.Client(), base, firstPage, "alice")
	if err != nil {
		t.Fatal(err)
	}
	want := append(running, firstPage[1:99]...)
	if len(jobs) != len(want) {
		t.Fatalf("got %d jobs, want %d", len(jobs), len(want))
	}
	for i := range want {
		if jobs[i].JobID != want[i].JobID {
			t.Fatalf("job %d = %s, want %s", i, jobs[i].JobID, want[i].JobID)
		}
	}
	jobs, err = loadGlobalJobs(t.Context(), server.Client(), base, firstPage, "bob")
	if err == nil || len(jobs) != len(firstPage) || jobs[len(jobs)-1].JobID != firstPage[99].JobID {
		t.Fatalf("running query failure returned %d jobs, err = %v", len(jobs), err)
	}
}

func TestGlobalJobsPreservesMembershipWhenExactMetadataFails(t *testing.T) {
	for _, failure := range []string{"current", "status", "historical", "missing job"} {
		t.Run(failure, func(t *testing.T) {
			runID, jobID, repoID := domaintypes.NewRunID(), domaintypes.NewJobID(), domaintypes.NewRepoID()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload any
				switch {
				case r.URL.Path == "/v1/jobs":
					payload = domainapi.JobListResponse{Jobs: []domainapi.JobListItem{{JobID: jobID, RunID: runID, RepoID: repoID, Name: "mig-0", JobType: domaintypes.JobTypeMig}}}
				case strings.HasSuffix(r.URL.Path, "/status"):
					if failure == "status" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					payload = domainapi.JobStatusResponse{JobID: jobID, RunID: runID, RepoID: repoID, Attempt: 1}
				default:
					if (failure == "current" && r.URL.RawQuery == "") || (failure == "historical" && r.URL.Query().Get("attempt") == "1") {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					payload = migsapi.ListRunJobsResponse{RunID: runID, RepoID: repoID, Attempt: 2, Jobs: []migsapi.RunJob{}}
				}
				if err := json.NewEncoder(w).Encode(payload); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			jobs, err := loadGlobalJobs(context.Background(), server.Client(), base, nil, "")
			if err == nil || len(jobs) != 1 || jobs[0].JobID != jobID || jobs[0].DisplayName != "" {
				t.Fatalf("metadata failure must preserve listed row and report error, jobs=%+v error=%v", jobs, err)
			}
			if failure == "missing job" && !strings.Contains(err.Error(), jobID.String()) {
				t.Fatalf("missing-job error lost identity: %v", err)
			}
		})
	}
}

func TestGlobalJobsCancelsMetadataRequest(t *testing.T) {
	runID, jobID, repoID := domaintypes.NewRunID(), domaintypes.NewJobID(), domaintypes.NewRepoID()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requested := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/jobs" {
			if err := json.NewEncoder(w).Encode(domainapi.JobListResponse{Jobs: []domainapi.JobListItem{{JobID: jobID, RunID: runID, RepoID: repoID}}}); err != nil {
				t.Error(err)
			}
			return
		}
		close(requested)
		<-r.Context().Done()
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	done := make(chan error, 1)
	go func() {
		_, err := loadGlobalJobs(ctx, server.Client(), base, nil, "")
		done <- err
	}()
	select {
	case <-requested:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("metadata request did not start")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("metadata request did not stop after cancellation")
	}
}

func TestGlobalJobsReusesOnlyMatchingUnchangedRows(t *testing.T) {
	for _, variant := range []string{"unchanged", "status changed", "different run", "missing start", "running claimed again", "created page", "queued page"} {
		t.Run(variant, func(t *testing.T) {
			runID, jobID, repoID := domaintypes.NewRunID(), domaintypes.NewJobID(), domaintypes.NewRepoID()
			started := time.Date(2026, 11, 20, 12, 36, 33, 0, time.UTC)
			listed := domainapi.JobListItem{JobID: jobID, RunID: runID, RepoID: repoID, Name: "mig-0", JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusSuccess, DurationMs: 5678}
			prior := listed
			prior.DisplayName, prior.StartedAt, prior.DurationMs = "cached-name", &started, 0
			switch variant {
			case "status changed":
				prior.Status = domaintypes.JobStatusQueued
			case "different run":
				prior.RunID = domaintypes.NewRunID()
			case "missing start":
				prior.StartedAt = nil
			case "created page":
				listed.Status = domaintypes.JobStatusCreated
			case "queued page":
				listed.Status = domaintypes.JobStatusQueued
			case "running claimed again":
				listed.Status, prior.Status = domaintypes.JobStatusRunning, domaintypes.JobStatusRunning
				oldStart := started.Add(-time.Hour)
				prior.StartedAt = &oldStart
			}
			finished := started.Add(time.Minute)
			metadataRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload any
				if r.URL.Path == "/v1/jobs" {
					payload = domainapi.JobListResponse{Jobs: []domainapi.JobListItem{listed}}
				} else {
					metadataRequests++
					payload = migsapi.ListRunJobsResponse{RunID: runID, RepoID: repoID, Attempt: 1, Jobs: []migsapi.RunJob{{JobID: jobID, Name: "different-name", DisplayName: "fresh-name", JobType: domaintypes.JobTypePreGate, StartedAt: &started, FinishedAt: &finished, Status: domaintypes.JobStatusSuccess, DurationMs: 9999}}}
				}
				if err := json.NewEncoder(w).Encode(payload); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			jobs, err := loadGlobalJobs(context.Background(), server.Client(), base, []domainapi.JobListItem{prior}, "")
			if err != nil || len(jobs) != 1 {
				t.Fatalf("jobs=%+v error=%v", jobs, err)
			}
			wantName, wantRequests := "fresh-name", 1
			if variant == "unchanged" {
				wantName, wantRequests = "cached-name", 0
			}
			job := jobs[0]
			if metadataRequests != wantRequests || job.DisplayName != wantName {
				t.Fatalf("metadata requests=%d job=%+v", metadataRequests, job)
			}
			if variant == "created page" || variant == "queued page" {
				if job.StartedAt != nil || job.FinishedAt != nil {
					t.Fatalf("pending page row inherited execution timestamps: %+v", job)
				}
			} else if job.StartedAt == nil || !job.StartedAt.Equal(started) {
				t.Fatalf("start time = %v, want %v", job.StartedAt, started)
			}
			job.DisplayName, job.StartedAt, job.FinishedAt = listed.DisplayName, listed.StartedAt, listed.FinishedAt
			if !reflect.DeepEqual(job, listed) {
				t.Fatalf("metadata overwrote page-owned fields: got %+v, want %+v", job, listed)
			}
		})
	}
}

func TestGlobalJobsMetadataFailureKeepsHealthyRun(t *testing.T) {
	runA, runB, repoID := domaintypes.NewRunID(), domaintypes.NewRunID(), domaintypes.NewRepoID()
	jobA, jobB := domaintypes.NewJobID(), domaintypes.NewJobID()
	started := time.Date(2026, 11, 20, 12, 36, 33, 0, time.UTC)
	page := []domainapi.JobListItem{{JobID: jobA, RunID: runA, RepoID: repoID, Status: domaintypes.JobStatusRunning}, {JobID: jobB, RunID: runB, RepoID: repoID}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload any
		switch r.URL.Path {
		case "/v1/jobs":
			payload = domainapi.JobListResponse{Jobs: page}
		case "/v1/runs/" + runA.String() + "/jobs":
			http.NotFound(w, r)
			return
		default:
			payload = migsapi.ListRunJobsResponse{RunID: runB, RepoID: repoID, Attempt: 1, Jobs: []migsapi.RunJob{{JobID: jobB, DisplayName: "healing", StartedAt: &started}}}
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	previous := []domainapi.JobListItem{{JobID: jobA, RunID: runA, RepoID: repoID, DisplayName: "known-step", StartedAt: &started, Status: domaintypes.JobStatusRunning}}
	jobs, err := loadGlobalJobs(context.Background(), server.Client(), base, previous, "")
	if err == nil || len(jobs) != 2 || jobs[0].JobID != jobA || jobs[0].DisplayName != "known-step" || jobs[0].StartedAt != nil || jobs[1].JobID != jobB || jobs[1].DisplayName != "healing" || jobs[1].StartedAt == nil {
		t.Fatalf("must preserve membership and healthy metadata with partial error: jobs=%+v error=%v", jobs, err)
	}
}

func TestGlobalJobsListFailureReturnsNoRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	jobs, err := loadGlobalJobs(context.Background(), server.Client(), base, nil, "")
	if err == nil || jobs != nil {
		t.Fatalf("list failure must return no rows: jobs=%+v error=%v", jobs, err)
	}
}
